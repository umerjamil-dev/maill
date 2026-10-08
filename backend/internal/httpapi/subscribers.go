package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func parseStringMap(s string) map[string]string {
	m := map[string]string{}
	if s == "" {
		return m
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return m
	}
	for k, v := range raw {
		if str, ok := v.(string); ok {
			m[k] = str
		}
	}
	return m
}

func encodeStringMap(m map[string]string) string {
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

type listReq struct {
	Name        string `json:"name" binding:"required,min=1"`
	Description string `json:"description"`
}

type listView struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	SubscriberCount int    `json:"subscriber_count"`
	CreatedAt       string `json:"created_at"`
}

// listLists returns lists with subscriber counts.
func (s *Server) listLists(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT l.id::text, l.name, l.description,
		       COUNT(s.id)::int, l.created_at::text
		FROM lists l
		LEFT JOIN subscribers s ON s.list_id = l.id
		WHERE l.tenant_id = $1
		GROUP BY l.id ORDER BY l.created_at DESC`, tenantFrom(c))
	if err != nil {
		s.fail(c, err)
		return
	}
	defer rows.Close()

	out := []listView{}
	for rows.Next() {
		var v listView
		if err := rows.Scan(&v.ID, &v.Name, &v.Description, &v.SubscriberCount, &v.CreatedAt); err != nil {
			s.fail(c, err)
			return
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, out)
}

// createList creates an empty list.
func (s *Server) createList(c *gin.Context) {
	var req listReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var id string
	err := s.db.QueryRowContext(c.Request.Context(),
		`INSERT INTO lists (tenant_id, name, description) VALUES ($1,$2,$3) RETURNING id::text`,
		tenantFrom(c), req.Name, req.Description).Scan(&id)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// deleteList removes a list and its subscribers (cascade).
func (s *Server) deleteList(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	if _, err := s.db.ExecContext(c.Request.Context(),
		`DELETE FROM lists WHERE id = $1 AND tenant_id = $2`, id, tenantFrom(c)); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

type subscriberReq struct {
	Email  string            `json:"email" binding:"required,email"`
	Name   string            `json:"name"`
	Fields map[string]string `json:"fields"`
}

type bulkSubscribersReq struct {
	DoubleOptIn bool            `json:"double_opt_in"`
	Subscribers []subscriberReq `json:"subscribers" binding:"required,min=1"`
}

type subscriberView struct {
	ID           string            `json:"id"`
	Email        string            `json:"email"`
	Name         string            `json:"name"`
	Fields       map[string]string `json:"fields"`
	Unsubscribed bool              `json:"unsubscribed"`
	CreatedAt    string            `json:"created_at"`
}

// listSubscribers returns a page of subscribers for a list.
func (s *Server) listSubscribers(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id::text, email, name, fields::text, (unsubscribed_at IS NOT NULL), created_at::text
		FROM subscribers WHERE tenant_id = $1 AND list_id = $2
		ORDER BY created_at DESC LIMIT 500`, tenantFrom(c), id)
	if err != nil {
		s.fail(c, err)
		return
	}
	defer rows.Close()

	out := []subscriberView{}
	for rows.Next() {
		var v subscriberView
		var fieldsJSON string
		if err := rows.Scan(&v.ID, &v.Email, &v.Name, &fieldsJSON, &v.Unsubscribed, &v.CreatedAt); err != nil {
			s.fail(c, err)
			return
		}
		v.Fields = parseStringMap(fieldsJSON)
		out = append(out, v)
	}
	c.JSON(http.StatusOK, out)
}

// addSubscriber inserts or updates a single subscriber (upsert on email).
func (s *Server) addSubscriber(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	var req subscriberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.upsertSubscriber(c, id, req); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"added": true})
}

// bulkAddSubscribers imports many subscribers at once (CSV parsed client-side).
func (s *Server) bulkAddSubscribers(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	var req bulkSubscribersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	added, skipped := 0, 0
	for _, sub := range req.Subscribers {
		if strings.TrimSpace(sub.Email) == "" {
			skipped++
			continue
		}
		if err := s.upsertSubscriber(c, id, sub); err != nil {
			skipped++
			continue
		}
		added++
	}
	c.JSON(http.StatusOK, gin.H{"added": added, "skipped": skipped})
}

// deleteSubscriber removes a subscriber.
func (s *Server) deleteSubscriber(c *gin.Context) {
	id, ok := uuidPath(c)
	if !ok {
		return
	}
	if _, err := s.db.ExecContext(c.Request.Context(),
		`DELETE FROM subscribers WHERE id = $1 AND tenant_id = $2`, id, tenantFrom(c)); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) upsertSubscriber(c *gin.Context, listID string, req subscriberReq) error {
	fields := "{}"
	if len(req.Fields) > 0 {
		fields = encodeStringMap(req.Fields)
	}
	_, err := s.db.ExecContext(c.Request.Context(), `
		INSERT INTO subscribers (tenant_id, list_id, email, name, fields)
		VALUES ($1,$2, LOWER($3), $4, $5::jsonb)
		ON CONFLICT (tenant_id, email)
		DO UPDATE SET name = EXCLUDED.name, fields = subscribers.fields || EXCLUDED.fields`,
		tenantFrom(c), listID, req.Email, req.Name, fields)
	return err
}
