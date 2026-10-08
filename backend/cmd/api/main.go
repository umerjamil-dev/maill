package main

import (
	"log"

	"emailsender/internal/config"
	"emailsender/internal/crypto"
	"emailsender/internal/httpapi"
	"emailsender/internal/queue"
	"emailsender/internal/store"
)

func main() {
	cfg := config.Load()

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer db.Close()

	qc, err := queue.NewClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Fatalf("queue: %v", err)
	}
	defer qc.Close()

	enc, err := crypto.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("crypto: %v", err)
	}
	if enc == nil {
		log.Println("warning: ENCRYPTION_KEY not set — SMTP credentials cannot be stored")
	}

	srv := httpapi.NewServer(cfg, db, qc, enc)
	log.Printf("api listening on :%s", cfg.Port)
	if err := srv.Router().Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
