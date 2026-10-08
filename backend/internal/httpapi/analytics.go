package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// overview returns tenant-wide counters for the dashboard.
func (s *Server) overview(c *gin.Context) {
	tenant := tenantFrom(c)
	count := func(query string, args ...any) int {
		var n int
		_ = s.db.QueryRowContext(c.Request.Context(), query, args...).Scan(&n)
		return n
	}

	campaignsByStatus := map[string]int{}
	if rows, err := s.db.QueryContext(c.Request.Context(),
		`SELECT status, COUNT(*)::int FROM campaigns WHERE tenant_id = $1 GROUP BY status`, tenant); err == nil {
		defer rows.Close()
		for rows.Next() {
			var k string
			var n int
			if rows.Scan(&k, &n) == nil {
				campaignsByStatus[k] = n
			}
		}
	}

	messagesByStatus := map[string]int{}
	if rows, err := s.db.QueryContext(c.Request.Context(),
		`SELECT m.status, COUNT(*)::int
		 FROM messages m JOIN campaigns cp ON cp.id = m.campaign_id
		 WHERE cp.tenant_id = $1 GROUP BY m.status`, tenant); err == nil {
		defer rows.Close()
		for rows.Next() {
			var k string
			var n int
			if rows.Scan(&k, &n) == nil {
				messagesByStatus[k] = n
			}
		}
	}

	eventsByType := map[string]int{}
	if rows, err := s.db.QueryContext(c.Request.Context(),
		`SELECT type, COUNT(*)::int FROM events WHERE tenant_id = $1 GROUP BY type`, tenant); err == nil {
		defer rows.Close()
		for rows.Next() {
			var k string
			var n int
			if rows.Scan(&k, &n) == nil {
				eventsByType[k] = n
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"lists":               count(`SELECT COUNT(*) FROM lists WHERE tenant_id = $1`, tenant),
		"subscribers":         count(`SELECT COUNT(*) FROM subscribers WHERE tenant_id = $1 AND unsubscribed_at IS NULL`, tenant),
		"smtp_accounts":       count(`SELECT COUNT(*) FROM smtp_accounts WHERE tenant_id = $1`, tenant),
		"domains":             count(`SELECT COUNT(*) FROM sending_domains WHERE tenant_id = $1`, tenant),
		"campaigns":           count(`SELECT COUNT(*) FROM campaigns WHERE tenant_id = $1`, tenant),
		"emails_sent":         messagesByStatus["sent"],
		"emails_failed":       messagesByStatus["failed"],
		"opens":               eventsByType["open"],
		"clicks":              eventsByType["click"],
		"bounces":             eventsByType["bounce"],
		"campaigns_by_status": campaignsByStatus,
		"messages_by_status":  messagesByStatus,
	})
}

// deliverability returns aggregate rates computed from counters.
func (s *Server) deliverability(c *gin.Context) {
	var sent, failed, opens, clicks, bounces int
	_ = s.db.QueryRowContext(c.Request.Context(), `
		SELECT COALESCE(SUM(sent_count),0), COALESCE(SUM(failed_count),0)
		FROM campaigns WHERE tenant_id = $1`, tenantFrom(c)).Scan(&sent, &failed)
	_ = s.db.QueryRowContext(c.Request.Context(), `
		SELECT COUNT(*) FILTER (WHERE type='open'), COUNT(*) FILTER (WHERE type='click'),
		       COUNT(*) FILTER (WHERE type='bounce')
		FROM events WHERE tenant_id = $1`, tenantFrom(c)).Scan(&opens, &clicks, &bounces)

	rate := func(num, den int) float64 {
		if den == 0 {
			return 0
		}
		return float64(num) / float64(den) * 100
	}

	c.JSON(http.StatusOK, gin.H{
		"sent":          sent,
		"failed":        failed,
		"open_rate":     rate(opens, sent),
		"click_rate":    rate(clicks, sent),
		"bounce_rate":   rate(bounces, sent+failed),
		"delivery_rate": rate(sent, sent+failed),
	})
}
