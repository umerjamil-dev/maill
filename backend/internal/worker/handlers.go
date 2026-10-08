// Package worker contains asynq task handlers for campaign fan-out
// and per-message SMTP delivery with distributed rate limiting.
package worker

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"emailsender/internal/config"
	"emailsender/internal/crypto"
	"emailsender/internal/mailer"
	"emailsender/internal/queue"
	"emailsender/internal/ratelimit"
	"emailsender/internal/store"

	"github.com/hibiken/asynq"
)

// Handler holds worker dependencies for asynq task handlers.
type Handler struct {
	db        *store.DB
	limiter   *ratelimit.Limiter
	encryptor *crypto.Encryptor
	qc        *queue.Client
}

func NewHandler(db *store.DB, limiter *ratelimit.Limiter, enc *crypto.Encryptor, cfg *config.Config) (*Handler, error) {
	qc, err := queue.NewClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		return nil, err
	}
	return &Handler{db: db, limiter: limiter, encryptor: enc, qc: qc}, nil
}

// Register mounts handlers on an asynq.ServeMux.
func (h *Handler) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(queue.TypeCampaignSend, h.HandleCampaignSend)
	mux.HandleFunc(queue.TypeMessageSend, h.HandleMessageSend)
}

// smtpCred is a decrypted SMTP account row.
type smtpCred struct {
	ID         string
	Host       string
	Port       int
	Username   string
	Password   string
	FromEmail  string
	FromName   string
	TLSMode    string
	AuthMethod string
	MaxPerMin  int
}

// campaignRow is the denormalized campaign snapshot used for fan-out.
type campaignRow struct {
	ID, TenantID, ListID, SMTPAccountID, Subject, HTMLBody string
	Status                                                 string
}

// HandleCampaignSend fans a campaign out into per-subscriber message tasks.
func (h *Handler) HandleCampaignSend(ctx context.Context, t *asynq.Task) error {
	var p queue.CampaignSendPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return asynq.SkipRetry
	}

	camp, err := h.loadCampaign(ctx, p.CampaignID)
	if err != nil {
		return fmt.Errorf("load campaign: %w", err)
	}
	if camp.Status != "queued" {
		return nil // already dispatched or not sendable
	}

	// Claim the campaign atomically so a duplicate task can't double-send.
	res, err := h.db.ExecContext(ctx,
		`UPDATE campaigns SET status = 'sending', started_at = now(), updated_at = now()
		 WHERE id = $1 AND status = 'queued'`, camp.ID)
	if err != nil {
		return fmt.Errorf("claim campaign: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}

	acc, err := h.loadSMTPAccount(ctx, camp.SMTPAccountID)
	if err != nil {
		return fmt.Errorf("load smtp account: %w", err)
	}

	rows, err := h.db.QueryContext(ctx, `
		SELECT s.id, s.email
		FROM subscribers s
		JOIN lists l ON l.id = s.list_id
		WHERE l.tenant_id = $1 AND s.list_id = $2 AND s.unsubscribed_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM suppressions sup WHERE sup.tenant_id = $1 AND sup.email = s.email)`,
		camp.TenantID, camp.ListID)
	if err != nil {
		return fmt.Errorf("query subscribers: %w", err)
	}
	defer rows.Close()

	total := 0
	for rows.Next() {
		var subID, email string
		if err := rows.Scan(&subID, &email); err != nil {
			log.Printf("scan subscriber: %v", err)
			continue
		}

		msgID, err := h.createMessage(ctx, camp.ID, camp.TenantID, subID, email)
		if err != nil {
			log.Printf("create message for %s: %v", email, err)
			continue
		}

		payload := queue.MessageSendPayload{
			TenantID:         camp.TenantID,
			CampaignID:       camp.ID,
			MessageID:        msgID,
			SMTPAccountID:    acc.ID,
			SubscriberID:     subID,
			ToEmail:          email,
			Subject:          camp.Subject,
			HTML:             camp.HTMLBody,
			UnsubscribeToken: randomToken(),
		}
		if _, err := h.qc.EnqueueMessageSend(ctx, payload); err != nil {
			if strings.Contains(err.Error(), "duplicate") {
				continue
			}
			return fmt.Errorf("enqueue message: %w", err)
		}
		total++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate subscribers: %w", err)
	}

	if _, err := h.db.ExecContext(ctx,
		`UPDATE campaigns SET total_count = $2, updated_at = now() WHERE id = $1`,
		camp.ID, total); err != nil {
		log.Printf("update total_count: %v", err)
	}
	log.Printf("campaign %s fanned out to %d messages", camp.ID, total)
	return nil
}

// HandleMessageSend delivers one message, respecting per-account rate limits.
func (h *Handler) HandleMessageSend(ctx context.Context, t *asynq.Task) error {
	var p queue.MessageSendPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return asynq.SkipRetry
	}

	acc, err := h.loadSMTPAccount(ctx, p.SMTPAccountID)
	if err != nil {
		return fmt.Errorf("load smtp account: %w", err)
	}

	// Token-bucket throttle shared across all worker instances.
	allowed, err := h.limiter.Allow(ctx, acc.ID, acc.MaxPerMin, acc.MaxPerMin)
	if err != nil {
		return fmt.Errorf("rate limit: %w", err)
	}
	if !allowed {
		// Reschedule without burning a retry attempt.
		if err := h.qc.RetryTaskLater(ctx, t, 5*time.Second); err != nil {
			return fmt.Errorf("requeue rate-limited task: %w", err)
		}
		return nil
	}

	// Suppression re-check at send time (bounces may have arrived mid-fan-out).
	var suppressed bool
	if err := h.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM suppressions WHERE tenant_id = $1 AND email = $2)`,
		p.TenantID, p.ToEmail).Scan(&suppressed); err != nil {
		return fmt.Errorf("suppression check: %w", err)
	}
	if suppressed {
		h.markMessage(ctx, p.MessageID, "skipped", "suppressed")
		return nil
	}

	vars, err := h.subscriberVars(ctx, p)
	if err != nil {
		return fmt.Errorf("subscriber vars: %w", err)
	}
	vars["unsubscribe_token"] = p.UnsubscribeToken

	msg := mailer.Message{
		To:      p.ToEmail,
		Subject: mailer.RenderSubject(p.Subject, vars),
		HTML:    mailer.Render(p.HTML, vars),
		ExtraHeaders: map[string]string{
			"List-Unsubscribe":      "<https://app.emailsender.test/unsubscribe/" + p.UnsubscribeToken + ">",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
			"X-Message-ID":          p.MessageID,
		},
	}

	start := time.Now()
	sendErr := mailer.Send(mailer.Account{
		Host:       acc.Host,
		Port:       acc.Port,
		Username:   acc.Username,
		Password:   acc.Password,
		FromEmail:  acc.FromEmail,
		FromName:   acc.FromName,
		TLSMode:    acc.TLSMode,
		AuthMethod: acc.AuthMethod,
	}, msg)

	if sendErr != nil {
		h.markMessage(ctx, p.MessageID, "failed", sendErr.Error())
		h.recordEvent(ctx, p.TenantID, p.CampaignID, p.MessageID, "delivery_failed", map[string]string{"error": sendErr.Error()})
		h.refreshCampaign(ctx, p.CampaignID)
		return nil // terminal for this message; re-sends happen via a new dispatch
	}

	h.markMessage(ctx, p.MessageID, "sent", "")
	h.recordEvent(ctx, p.TenantID, p.CampaignID, p.MessageID, "delivered",
		map[string]string{"latency_ms": fmt.Sprint(time.Since(start).Milliseconds())})

	h.refreshCampaign(ctx, p.CampaignID)
	return nil
}

// --- helpers -----------------------------------------------------------

func (h *Handler) loadCampaign(ctx context.Context, id string) (*campaignRow, error) {
	c := &campaignRow{}
	err := h.db.QueryRowContext(ctx,
		`SELECT id, tenant_id::text, list_id::text, COALESCE(smtp_account_id::text,''), subject, html_body, status
		 FROM campaigns WHERE id = $1`, id).
		Scan(&c.ID, &c.TenantID, &c.ListID, &c.SMTPAccountID, &c.Subject, &c.HTMLBody, &c.Status)
	if err != nil {
		return nil, err
	}
	if c.SMTPAccountID == "" {
		return nil, fmt.Errorf("campaign %s has no smtp account", id)
	}
	return c, nil
}

func (h *Handler) loadSMTPAccount(ctx context.Context, id string) (*smtpCred, error) {
	var a smtpCred
	var pwEnc []byte
	err := h.db.QueryRowContext(ctx,
		`SELECT id::text, host, port, username, password_enc, from_email, from_name, tls_mode, auth_method, max_emails_per_min
		 FROM smtp_accounts WHERE id = $1 AND status = 'active'`, id).
		Scan(&a.ID, &a.Host, &a.Port, &a.Username, &pwEnc, &a.FromEmail, &a.FromName, &a.TLSMode, &a.AuthMethod, &a.MaxPerMin)
	if err != nil {
		return nil, err
	}
	if h.encryptor == nil {
		return nil, fmt.Errorf("encryption key not configured")
	}
	pw, err := h.encryptor.Open(pwEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypt smtp password: %w", err)
	}
	a.Password = pw
	return &a, nil
}

// createMessage inserts a pending message row (idempotent per campaign+subscriber).
func (h *Handler) createMessage(ctx context.Context, campaignID, tenantID, subscriberID, email string) (string, error) {
	var msgID string
	err := h.db.QueryRowContext(ctx, `
		INSERT INTO messages (campaign_id, tenant_id, subscriber_id, to_email)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (campaign_id, subscriber_id) DO UPDATE SET to_email = EXCLUDED.to_email
		RETURNING id::text`,
		campaignID, tenantID, subscriberID, email).Scan(&msgID)
	if err == sql.ErrNoRows {
		// already exists — fetch its id
		err = h.db.QueryRowContext(ctx,
			`SELECT id::text FROM messages WHERE campaign_id = $1 AND subscriber_id = $2`,
			campaignID, subscriberID).Scan(&msgID)
	}
	return msgID, err
}

// subscriberVars builds merge-tag values from the subscriber row.
func (h *Handler) subscriberVars(ctx context.Context, p queue.MessageSendPayload) (map[string]string, error) {
	var name, email, fieldsJSON string
	err := h.db.QueryRowContext(ctx,
		`SELECT name, email, fields::text FROM subscribers WHERE id = $1`, p.SubscriberID).
		Scan(&name, &email, &fieldsJSON)
	if err != nil {
		return nil, err
	}
	vars := map[string]string{
		"email": mailer.EscapeHTMLValue(email),
	}
	if name != "" {
		vars["name"] = mailer.EscapeHTMLValue(name)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err == nil {
		for k, v := range fields {
			if s, ok := v.(string); ok {
				vars[k] = mailer.EscapeHTMLValue(s)
			}
		}
	}
	return vars, nil
}

func (h *Handler) markMessage(ctx context.Context, messageID, status, errMsg string) {
	_, err := h.db.ExecContext(ctx,
		`UPDATE messages SET status = $2, error = $3, attempts = attempts + 1, sent_at = CASE WHEN $2 = 'sent' THEN now() ELSE sent_at END
		 WHERE id = $1`, messageID, status, errMsg)
	if err != nil {
		log.Printf("markMessage %s: %v", messageID, err)
	}
}

func (h *Handler) recordEvent(ctx context.Context, tenantID, campaignID, messageID, typ string, meta map[string]string) {
	b, _ := json.Marshal(meta)
	_, err := h.db.ExecContext(ctx,
		`INSERT INTO events (tenant_id, campaign_id, message_id, type, meta) VALUES ($1,$2,$3,$4,$5)`,
		tenantID, campaignID, messageID, typ, b)
	if err != nil {
		log.Printf("recordEvent %s: %v", typ, err)
	}
}

// refreshCampaign recomputes a campaign's counters from its message rows and
// marks it completed once every message has reached a terminal state.
func (h *Handler) refreshCampaign(ctx context.Context, campaignID string) {
	if _, err := h.db.ExecContext(ctx, `
		UPDATE campaigns
		SET sent_count = (SELECT COUNT(*) FROM messages WHERE campaign_id = $1 AND status = 'sent'),
		    failed_count = (SELECT COUNT(*) FROM messages WHERE campaign_id = $1 AND status IN ('failed','bounced')),
		    updated_at = now()
		WHERE id = $1`, campaignID); err != nil {
		log.Printf("refresh counters %s: %v", campaignID, err)
	}

	var pending int
	if err := h.db.QueryRowContext(ctx,
		`SELECT count(*) FROM messages WHERE campaign_id = $1 AND status = 'pending'`, campaignID).Scan(&pending); err != nil {
		log.Printf("count pending: %v", err)
		return
	}
	if pending == 0 {
		if _, err := h.db.ExecContext(ctx,
			`UPDATE campaigns SET status = 'completed', finished_at = now(), updated_at = now()
			 WHERE id = $1 AND status = 'sending'`, campaignID); err != nil {
			log.Printf("finalize campaign: %v", err)
		}
	}
}

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprint(time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
