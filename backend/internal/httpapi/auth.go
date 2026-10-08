package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type registerRequest struct {
	Company  string `json:"company" binding:"required,min=2"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	FullName string `json:"full_name"`
}

// handleRegister creates a tenant + owner user in one transaction.
func (s *Server) handleRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db"})
		return
	}
	defer tx.Rollback()

	var tenantID string
	slug := slugify(req.Company)
	err = tx.QueryRowContext(ctx,
		`INSERT INTO tenants (name, slug) VALUES ($1, $2) ON CONFLICT (slug) DO NOTHING RETURNING id`,
		req.Company, slug).Scan(&tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusConflict, gin.H{"error": "company slug already exists"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create tenant"})
		return
	}

	var userID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (tenant_id, email, password, full_name, role)
		 VALUES ($1, $2, $3, $4, 'owner') RETURNING id`,
		tenantID, strings.ToLower(req.Email), string(hash), req.FullName).Scan(&userID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "commit"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"tenant_id": tenantID,
		"user_id":   userID,
		"message":   "registered",
	})
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// handleLogin verifies credentials and issues a JWT.
func (s *Server) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var userID, tenantID, hash, role string
	err := s.db.QueryRowContext(c.Request.Context(),
		`SELECT u.id, u.tenant_id, u.password, u.role FROM users u WHERE u.email = $1`,
		strings.ToLower(req.Email)).Scan(&userID, &tenantID, &hash, &role)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := s.signToken(userID, tenantID, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token, "tenant_id": tenantID, "role": role})
}

// handleMe returns the authenticated user's context.
func (s *Server) handleMe(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"user_id":   c.GetString(ctxUserID),
		"tenant_id": c.GetString(ctxTenant),
		"role":      c.GetString(ctxRole),
	})
}

const (
	ctxUserID = "userID"
	ctxTenant = "tenantID"
	ctxRole   = "role"
	tokenTTL  = 24 * time.Hour
)

type claims struct {
	UserID   string `json:"uid"`
	TenantID string `json:"tid"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func (s *Server) signToken(userID, tenantID, role string) (string, error) {
	cl := claims{
		UserID: userID, TenantID: tenantID, Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, cl).SignedString([]byte(s.cfg.JWTSecret))
}

// requireAuth validates the Bearer token and injects identity into context.
func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		tokStr := strings.TrimPrefix(h, "Bearer ")
		if tokStr == "" || tokStr == h && !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		tok, err := jwt.ParseWithClaims(tokStr, &claims{}, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(s.cfg.JWTSecret), nil
		})
		if err != nil || !tok.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		cl, ok := tok.Claims.(*claims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			return
		}

		c.Set(ctxUserID, cl.UserID)
		c.Set(ctxTenant, cl.TenantID)
		c.Set(ctxRole, cl.Role)
		c.Next()
	}
}

func slugify(s string) string {
	var b strings.Builder
	prevDash := true
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case !prevDash:
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
