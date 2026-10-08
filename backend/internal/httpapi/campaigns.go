package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"emailsender/internal/mailer"
	"emailsender/internal/queue"

	"github.com/gin-gonic/gin"
)

type campaignReq struct {
	ListID        string `json:"list_id" binding:"required"`
	SMTPAccountID string `json:"smtp_account_id" binding:"required"`
	Subject       string `json:"subject" binding:"required"`
	Preheader     string `json:"preheader"`
	HTMLBody      string `json:"html_body" binding:"required"`
}

type campaignView struct {
	ID            string `json:"id"`
	ListID        string `json:"list_id"`
	SMTPAccountID string `json:"smtp_account_id"`
	Subject       string `json:"subject"`
	Preheader     string `json:"preheader"`
	Status        string `json:"status"`
	Total         int    `json:"total_count"`
	Sent          int    `json:"sent_count"`
	Failed        int    `json:"failed_count"`
	CreatedAt     string `json:"created_at"`
}

// listCampaigns returns campaigns with counts.
func (s *Server) listCampaigns(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id::text, list_id::text, COALESCE(smtp_account_id::text,''),
		       subject, preheader, status, total_count, sent_count, failed_count, created_at::text
		FROM campaigns WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantFrom(c))
	if err != nil {
		s.fail(c, err)
		return
	}
	defer rows.Close()

	out := []campaignView{}
	for rows.Next() {
		var v campaignView
		if err := rows.Scan(&v.ID, &v.ListID, &v.SMTPAccountID, &v.Subject, &v.Preheader,
			&v.Status, &v.Total, &v.Sent, &v.Failed, &v.CreatedAt); err != nil {
			s.fail(c, err)
			return
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, out)
}

// getCampaign returns a single campaign plus message-status breakdown.
func (s *Server) getCampaign(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	var v campaignView
	err := s.db.QueryRowContext(c.Request.Context(), `
		SELECT id::text, list_id::text, COALESCE(smtp_account_id::text,''),
		       subject, preheader, status, total_count, sent_count, failed_count, created_at::text
		FROM campaigns WHERE id = $1 AND tenant_id = $2`, id, tenantFrom(c)).
		Scan(&v.ID, &v.ListID, &v.SMTPAccountID, &v.Subject, &v.Preheader,
			&v.Status, &v.Total, &v.Sent, &v.Failed, &v.CreatedAt)
	if err != nil {
		s.fail(c, err)
		return
	}

	rows, err := s.db.QueryContext(c.Request.Context(),
		`SELECT status, COUNT(*)::int FROM messages WHERE campaign_id = $1 GROUP BY status`, id)
	if err != nil {
		s.fail(c, err)
		return
	}
	defer rows.Close()
	breakdown := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err == nil {
			breakdown[st] = n
		}
	}
	c.JSON(http.StatusOK, gin.H{"campaign": v, "breakdown": breakdown})
}

// createCampaign stores a new draft campaign.
func (s *Server) createCampaign(c *gin.Context) {
	var req campaignReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Validate list + smtp belong to tenant.
	if !s.owns(c, "lists", req.ListID) || !s.owns(c, "smtp_accounts", req.SMTPAccountID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid list or smtp account"})
		return
	}

	var id string
	err := s.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO campaigns (tenant_id, list_id, smtp_account_id, subject, preheader, html_body)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
		tenantFrom(c), req.ListID, req.SMTPAccountID, req.Subject, req.Preheader, req.HTMLBody).Scan(&id)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// deleteCampaign removes a draft campaign.
func (s *Server) deleteCampaign(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	if _, err := s.db.ExecContext(c.Request.Context(),
		`DELETE FROM campaigns WHERE id = $1 AND tenant_id = $2 AND status IN ('draft','failed')`,
		id, tenantFrom(c)); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// dispatchCampaign queues the campaign for sending via the worker.
func (s *Server) dispatchCampaign(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	tenant := tenantFrom(c)

	var status string
	err := s.db.QueryRowContext(c.Request.Context(),
		`SELECT status FROM campaigns WHERE id = $1 AND tenant_id = $2`, id, tenant).Scan(&status)
	if err != nil {
		s.fail(c, err)
		return
	}
	if status != "draft" && status != "failed" {
		c.JSON(http.StatusConflict, gin.H{"error": "campaign not dispatchable", "status": status})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if _, err := s.queue.EnqueueCampaignSend(ctx, queue.CampaignSendPayload{
		TenantID: tenant, CampaignID: id,
	}); err != nil {
		s.fail(c, err)
		return
	}
	if _, err := s.db.ExecContext(c.Request.Context(),
		`UPDATE campaigns SET status = 'queued', updated_at = now() WHERE id = $1`, id); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

// owns checks that a row belongs to the tenant.
func (s *Server) owns(c *gin.Context, table, id string) bool {
	var exists bool
	err := s.db.QueryRowContext(c.Request.Context(),
		`SELECT EXISTS(SELECT 1 FROM `+table+` WHERE id = $1 AND tenant_id = $2)`, id, tenantFrom(c)).Scan(&exists)
	return err == nil && exists
}

type campaignTestReq struct {
	To string `json:"to" binding:"required,email"`
}

// testCampaign renders the campaign with sample merge tags and sends one
// proof email through the selected SMTP account.
func (s *Server) testCampaign(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	var req campaignTestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var subject, html, smtpID string
	err := s.db.QueryRowContext(c.Request.Context(),
		`SELECT subject, html_body, smtp_account_id::text FROM campaigns WHERE id = $1 AND tenant_id = $2`,
		id, tenantFrom(c)).Scan(&subject, &html, &smtpID)
	if err != nil {
		s.fail(c, err)
		return
	}

	var acc mailer.Account
	var pwEnc []byte
	err = s.db.QueryRowContext(c.Request.Context(),
		`SELECT host, port, username, password_enc, from_email, from_name, tls_mode, auth_method
		 FROM smtp_accounts WHERE id = $1`, smtpID).
		Scan(&acc.Host, &acc.Port, &acc.Username, &pwEnc, &acc.FromEmail, &acc.FromName, &acc.TLSMode, &acc.AuthMethod)
	if err != nil {
		s.fail(c, err)
		return
	}
	pw, err := s.encryptor.Open(pwEnc)
	if err != nil {
		s.fail(c, err)
		return
	}
	acc.Password = pw

	vars := map[string]string{
		"name":            "Subscriber",
		"email":           req.To,
		"unsubscribe_url": "https://app.emailsender.test/unsubscribe/sample",
	}
	html = mailer.Render(html, vars)
	subject = mailer.RenderSubject("[TEST] "+subject, vars)

	if err := mailer.Send(acc, mailer.Message{To: req.To, Subject: subject, HTML: html}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "sent": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": true, "to": req.To, "at": fmt.Sprint(time.Now().Unix())})
}
