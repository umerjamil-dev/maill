// Command worker runs the asynq background worker that delivers email
// campaigns through customer SMTP accounts with rate limiting.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"emailsender/internal/config"
	"emailsender/internal/crypto"
	"emailsender/internal/ratelimit"
	"emailsender/internal/store"
	"emailsender/internal/worker"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer db.Close()

	enc, err := crypto.NewEncryptor(cfg.EncryptionKey)
	if err != nil || enc == nil {
		log.Fatal("ENCRYPTION_KEY is required by the worker")
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	limiter := ratelimit.New(rdb)

	concurrency := 10
	h, err := worker.NewHandler(db, limiter, enc, cfg)
	if err != nil {
		log.Fatalf("handler: %v", err)
	}

	mux := asynq.NewServeMux()
	h.Register(mux)

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB},
		asynq.Config{
			Concurrency: concurrency,
			Queues: map[string]int{
				"campaigns": 6, // fan-out first
				"messages":  3, // delivery bulk
				"imports":   1,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, t *asynq.Task, err error) {
				log.Printf("task %s failed: %v", t.Type(), err)
			}),
		},
	)

	go func() {
		if err := srv.Run(mux); err != nil {
			log.Fatalf("worker server: %v", err)
		}
	}()

	log.Printf("worker started (concurrency=%d)", concurrency)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down worker...")
	srv.Shutdown()
	fmt.Println("worker stopped")
}
