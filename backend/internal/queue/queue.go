package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Queue names used across API and worker.
const (
	QueueCampaigns = "campaigns"
	QueueMessages  = "messages"
	QueueImports   = "imports"
)

// Task types processed by the worker.
const (
	TypeCampaignSend     = "campaign:send"     // fan-out: enqueue messages for a campaign
	TypeMessageSend      = "message:send"      // per-recipient SMTP delivery
	TypeSubscriberImport = "subscriber:import" // CSV import
)

// CampaignSendPayload fans out one campaign into message tasks.
type CampaignSendPayload struct {
	TenantID   string `json:"tenant_id"`
	CampaignID string `json:"campaign_id"`
}

// MessageSendPayload delivers a single prepared message.
type MessageSendPayload struct {
	TenantID         string `json:"tenant_id"`
	CampaignID       string `json:"campaign_id"`
	MessageID        string `json:"message_id"`
	SMTPAccountID    string `json:"smtp_account_id"`
	SubscriberID     string `json:"subscriber_id"`
	ToEmail          string `json:"to_email"`
	Subject          string `json:"subject"`
	HTML             string `json:"html"`
	UnsubscribeToken string `json:"unsubscribe_token"`
}

// SubscriberImportPayload processes an uploaded CSV for a list.
type SubscriberImportPayload struct {
	TenantID    string `json:"tenant_id"`
	ListID      string `json:"list_id"`
	FilePath    string `json:"file_path"`
	DoubleOptIn bool   `json:"double_opt_in"`
}

// Client wraps asynq.Client for enqueuing tasks from the API.
type Client struct {
	c *asynq.Client
}

func NewClient(redisAddr, redisPassword string, redisDB int) (*Client, error) {
	opt := asynq.RedisClientOpt{Addr: redisAddr, Password: redisPassword, DB: redisDB}
	c := asynq.NewClient(opt)
	if err := c.Ping(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &Client{c: c}, nil
}

func (cl *Client) Close() error { return cl.c.Close() }

// EnqueueCampaignSend kicks off a campaign fan-out.
func (cl *Client) EnqueueCampaignSend(ctx context.Context, p CampaignSendPayload) (*asynq.TaskInfo, error) {
	b, _ := json.Marshal(p)
	t := asynq.NewTask(TypeCampaignSend, b)
	// Unique for 30m prevents double-dispatch of the same campaign.
	info, err := cl.c.EnqueueContext(ctx, t,
		asynq.Queue(QueueCampaigns),
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Minute),
		asynq.Unique(30*time.Minute),
	)
	if err != nil {
		return nil, fmt.Errorf("enqueue campaign send: %w", err)
	}
	return info, nil
}

// EnqueueMessageSend schedules a single message for delivery. Actual
// per-SMTP-account throttling is enforced by the worker's token-bucket
// limiter (see internal/ratelimit).
func (cl *Client) EnqueueMessageSend(ctx context.Context, p MessageSendPayload) (*asynq.TaskInfo, error) {
	b, _ := json.Marshal(p)
	t := asynq.NewTask(TypeMessageSend, b)
	info, err := cl.c.EnqueueContext(ctx, t,
		asynq.Queue(QueueMessages),
		asynq.MaxRetry(5),
		asynq.Timeout(60*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("enqueue message send: %w", err)
	}
	return info, nil
}

// EnqueueSubscriberImport schedules a CSV import job.
func (cl *Client) EnqueueSubscriberImport(ctx context.Context, p SubscriberImportPayload) (*asynq.TaskInfo, error) {
	b, _ := json.Marshal(p)
	t := asynq.NewTask(TypeSubscriberImport, b)
	info, err := cl.c.EnqueueContext(ctx, t,
		asynq.Queue(QueueImports),
		asynq.MaxRetry(3),
		asynq.Timeout(15*time.Minute),
	)
	if err != nil {
		return nil, fmt.Errorf("enqueue subscriber import: %w", err)
	}
	return info, nil
}

// ServerOptions builds asynq server options for the worker binary.
func ServerOptions(redisAddr, redisPassword string, redisDB int) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{Addr: redisAddr, Password: redisPassword, DB: redisDB}
}

// RetryTaskLater re-enqueues a copy of the currently-processing task with a
// delay, for rate-limit backpressure without burning a retry attempt.
func (cl *Client) RetryTaskLater(ctx context.Context, t *asynq.Task, delay time.Duration) error {
	copied := asynq.NewTask(t.Type(), t.Payload())
	if _, err := cl.c.EnqueueContext(ctx, copied, asynq.ProcessIn(delay)); err != nil {
		return fmt.Errorf("retry later: %w", err)
	}
	return nil
}
