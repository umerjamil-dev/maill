package httpapi

import (
	"net/http"

	"emailsender/internal/dkim"

	"github.com/gin-gonic/gin"
)

type createDomainReq struct {
	Domain      string `json:"domain" binding:"required"`
	Selector    string `json:"selector"`
	DKIMBits    int    `json:"dkim_bits"`
	ReportEmail string `json:"dmarc_report_email"`
}

type domainView struct {
	ID            string        `json:"id"`
	Domain        string        `json:"domain"`
	Selector      string        `json:"selector"`
	SPFVerified   bool          `json:"spf_verified"`
	DKIMVerified  bool          `json:"dkim_verified"`
	DMARCVerified bool          `json:"dmarc_verified"`
	Records       []dkim.Record `json:"records"`
}

// listDomains returns each tenant domain with its required DNS records.
func (s *Server) listDomains(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id::text, domain, dkim_selector, spf_verified, dkim_verified, dmarc_verified,
		       dkim_private_key_enc
		FROM sending_domains WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantFrom(c))
	if err != nil {
		s.fail(c, err)
		return
	}
	defer rows.Close()

	out := []domainView{}
	for rows.Next() {
		var v domainView
		var privEnc []byte
		if err := rows.Scan(&v.ID, &v.Domain, &v.Selector, &v.SPFVerified, &v.DKIMVerified,
			&v.DMARCVerified, &privEnc); err != nil {
			s.fail(c, err)
			return
		}
		privPEM := ""
		if s.encryptor != nil && len(privEnc) > 0 {
			if p, err := s.encryptor.Open(privEnc); err == nil {
				privPEM = p
			}
		}
		v.Records = buildDNSRecords(v.Domain, v.Selector, privPEM)
		out = append(out, v)
	}
	c.JSON(http.StatusOK, out)
}

// createDomain generates a DKIM keypair and stores the domain for DNS setup.
func (s *Server) createDomain(c *gin.Context) {
	var req createDomainReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Selector == "" {
		req.Selector = "default"
	}
	kp, err := dkim.GenerateRSAKey(req.Selector, req.Domain, req.DKIMBits)
	if err != nil {
		s.fail(c, err)
		return
	}

	var encPriv []byte
	if s.encryptor != nil {
		encPriv, err = s.encryptor.Seal(kp.PrivateKeyPEM)
		if err != nil {
			s.fail(c, err)
			return
		}
	}

	var id string
	err = s.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO sending_domains (tenant_id, domain, dkim_selector, dkim_private_key_enc)
		VALUES ($1,$2,$3,$4) RETURNING id::text`,
		tenantFrom(c), req.Domain, req.Selector, encPriv).Scan(&id)
	if err != nil {
		s.fail(c, err)
		return
	}

	v := domainView{ID: id, Domain: req.Domain, Selector: req.Selector}
	v.Records = buildDNSRecords(req.Domain, req.Selector, kp.PrivateKeyPEM)
	c.JSON(http.StatusCreated, v)
}

// verifyDomain marks a domain as manually verified (real DNS lookup later).
func (s *Server) verifyDomain(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	_, err := s.db.ExecContext(c.Request.Context(), `
		UPDATE sending_domains SET spf_verified = TRUE, dkim_verified = TRUE, dmarc_verified = TRUE
		WHERE id = $1 AND tenant_id = $2`, id, tenantFrom(c))
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"verified": true})
}

// deleteDomain removes a sending domain.
func (s *Server) deleteDomain(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	if _, err := s.db.ExecContext(c.Request.Context(),
		`DELETE FROM sending_domains WHERE id = $1 AND tenant_id = $2`, id, tenantFrom(c)); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// buildDNSRecords derives the DKIM TXT value from the stored private key.
func buildDNSRecords(domain, selector, privPEM string) []dkim.Record {
	dnsValue := "(key pending)"
	if privPEM != "" {
		// Re-derive public record deterministically when key present.
		if rec, err := dkim.PublicRecordFromPEM(privPEM); err == nil {
			dnsValue = rec
		}
	}
	return []dkim.Record{
		dkim.SPFRecord(domain),
		dkim.DKIMRecord(selector, domain, dnsValue),
		dkim.DMARCRecord(domain, ""),
	}
}
