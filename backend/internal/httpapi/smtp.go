package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"emailsender/internal/mailer"

	"github.com/gin-gonic/gin"
)

type smtpAccountReq struct {
	Host       string `json:"host" binding:"required"`
	Port       int    `json:"port" binding:"required"`
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	FromEmail  string `json:"from_email" binding:"required,email"`
	FromName   string `json:"from_name"`
	AuthMethod string `json:"auth_method"`
	TLSMode    string `json:"tls_mode"`
	MaxPerMin  int    `json:"max_emails_per_min"`
	MaxConcur  int    `json:"max_concurrency"`
	RotationW  int    `json:"rotation_weight"`
}

type smtpAccountView struct {
	ID         string `json:"id"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	FromEmail  string `json:"from_email"`
	FromName   string `json:"from_name"`
	AuthMethod string `json:"auth_method"`
	TLSMode    string `json:"tls_mode"`
	MaxPerMin  int    `json:"max_emails_per_min"`
	MaxConcur  int    `json:"max_concurrency"`
	RotationW  int    `json:"rotation_weight"`
	Status     string `json:"status"`
	Verified   bool   `json:"verified"`
	CreatedAt  string `json:"created_at"`
}

// listSMTP returns all SMTP accounts for the tenant.
func (s *Server) listSMTP(c *gin.Context) {
	tenant := tenantFrom(c)
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id::text, host, port, username, from_email, from_name,
		       auth_method, tls_mode, max_emails_per_min, max_concurrency,
		       rotation_weight, status, verified, created_at::text
		FROM smtp_accounts WHERE tenant_id = $1 ORDER BY created_at DESC`, tenant)
	if err != nil {
		s.fail(c, err)
		return
	}
	defer rows.Close()

	out := []smtpAccountView{}
	for rows.Next() {
		var v smtpAccountView
		if err := rows.Scan(&v.ID, &v.Host, &v.Port, &v.Username, &v.FromEmail, &v.FromName,
			&v.AuthMethod, &v.TLSMode, &v.MaxPerMin, &v.MaxConcur,
			&v.RotationW, &v.Status, &v.Verified, &v.CreatedAt); err != nil {
			s.fail(c, err)
			return
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, out)
}

// createSMTP stores an encrypted SMTP account.
func (s *Server) createSMTP(c *gin.Context) {
	var req smtpAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if s.encryptor == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server encryption key missing"})
		return
	}
	enc, err := s.encryptor.Seal(req.Password)
	if err != nil {
		s.fail(c, err)
		return
	}

	applySMTPDefaults(&req)

	var id string
	err = s.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO smtp_accounts
			(tenant_id, host, port, username, password_enc, from_email, from_name,
			 auth_method, tls_mode, max_emails_per_min, max_concurrency, rotation_weight)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id::text`,
		tenantFrom(c), req.Host, req.Port, req.Username, enc, req.FromEmail, req.FromName,
		req.AuthMethod, req.TLSMode, req.MaxPerMin, req.MaxConcur, req.RotationW).Scan(&id)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// deleteSMTP removes an account.
func (s *Server) deleteSMTP(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	_, err := s.db.ExecContext(c.Request.Context(),
		`DELETE FROM smtp_accounts WHERE id = $1 AND tenant_id = $2`, id, tenantFrom(c))
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

type testSendReq struct {
	To string `json:"to" binding:"required,email"`
}

// testSMTP sends a single probe email through the account to verify credentials.
func (s *Server) testSMTP(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	var req testSendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var acc mailer.Account
	var pwEnc []byte
	err := s.db.QueryRowContext(c.Request.Context(), `
		SELECT host, port, username, password_enc, from_email, from_name, tls_mode, auth_method
		FROM smtp_accounts WHERE id = $1 AND tenant_id = $2`, id, tenantFrom(c)).
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

	subject := "EmailSender test — " + acc.FromEmail
	html := fmt.Sprintf(`<p>This is a delivery test from your SMTP account <b>%s</b> at %s.</p><p>Sent %s.</p>`,
		acc.Host, acc.FromEmail, time.Now().UTC().Format(time.RFC3339))

	if err := mailer.Send(acc, mailer.Message{To: req.To, Subject: subject, HTML: html}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "verified": false})
		return
	}
	if _, err := s.db.ExecContext(c.Request.Context(),
		`UPDATE smtp_accounts SET verified = TRUE WHERE id = $1`, id); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"verified": true, "sent_to": req.To})
}

func applySMTPDefaults(req *smtpAccountReq) {
	if req.AuthMethod == "" {
		req.AuthMethod = "login"
	}
	if req.TLSMode == "" {
		req.TLSMode = "starttls"
	}
	if req.MaxPerMin <= 0 {
		req.MaxPerMin = 60
	}
	if req.MaxConcur <= 0 {
		req.MaxConcur = 2
	}
	if req.RotationW <= 0 {
		req.RotationW = 1
	}
}
