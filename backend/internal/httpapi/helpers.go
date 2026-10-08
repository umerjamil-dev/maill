package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// tenantFrom returns the authenticated tenant id injected by requireAuth.
func tenantFrom(c *gin.Context) string { return c.GetString(ctxTenant) }

// dbError maps common database errors to HTTP responses.
func (s *Server) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// uuidPath reads a required :id path param.
func uuidPath(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return "", false
	}
	return id, true
}
