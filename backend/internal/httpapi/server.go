package httpapi

import (
	"net/http"
	"time"

	"emailsender/internal/config"
	"emailsender/internal/crypto"
	"emailsender/internal/queue"
	"emailsender/internal/store"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Server bundles dependencies for all HTTP handlers.
type Server struct {
	cfg       *config.Config
	db        *store.DB
	queue     *queue.Client
	encryptor *crypto.Encryptor
}

func NewServer(cfg *config.Config, db *store.DB, qc *queue.Client, enc *crypto.Encryptor) *Server {
	return &Server{cfg: cfg, db: db, queue: qc, encryptor: enc}
}

// Router wires all v1 endpoints.
func (s *Server) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:4173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/healthz", s.handleHealth)

	v1 := r.Group("/api/v1")
	{
		v1.POST("/auth/register", s.handleRegister)
		v1.POST("/auth/login", s.handleLogin)

		// Authenticated routes (JWT bearer middleware).
		auth := v1.Group("", s.requireAuth())
		{
			auth.GET("/me", s.handleMe)
			auth.GET("/overview", s.overview)
			auth.GET("/deliverability", s.deliverability)

			// SMTP accounts (customer BYO, parallel sending via rotation).
			auth.GET("/smtp-accounts", s.listSMTP)
			auth.POST("/smtp-accounts", s.createSMTP)
			auth.DELETE("/smtp-accounts/:id", s.deleteSMTP)
			auth.POST("/smtp-accounts/:id/test", s.testSMTP)

			// Sending domains + DNS records (SPF/DKIM/DMARC).
			auth.GET("/domains", s.listDomains)
			auth.POST("/domains", s.createDomain)
			auth.DELETE("/domains/:id", s.deleteDomain)
			auth.POST("/domains/:id/verify", s.verifyDomain)

			// Lists.
			auth.GET("/lists", s.listLists)
			auth.POST("/lists", s.createList)
			auth.DELETE("/lists/:id", s.deleteList)

			// Subscribers (scoped to a list).
			auth.GET("/lists/:id/subscribers", s.listSubscribers)
			auth.POST("/lists/:id/subscribers", s.addSubscriber)
			auth.POST("/lists/:id/subscribers/bulk", s.bulkAddSubscribers)
			auth.DELETE("/subscribers/:id", s.deleteSubscriber)

			// Campaigns.
			auth.GET("/campaigns", s.listCampaigns)
			auth.POST("/campaigns", s.createCampaign)
			auth.GET("/campaigns/:id", s.getCampaign)
			auth.DELETE("/campaigns/:id", s.deleteCampaign)
			auth.POST("/campaigns/:id/dispatch", s.dispatchCampaign)
			auth.POST("/campaigns/:id/test", s.testCampaign)
		}
	}

	// Serve built frontend SPA in production (frontend/dist).
	r.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "not found")
	})

	return r
}

func (s *Server) handleHealth(c *gin.Context) {
	dbStatus := "ok"
	if err := s.db.Health(); err != nil {
		dbStatus = "error: " + err.Error()
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "up",
		"db":     dbStatus,
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}
