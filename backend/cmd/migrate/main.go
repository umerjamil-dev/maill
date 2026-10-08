// Command migrate applies or reverts database migrations using goose.
// Usage: go run ./cmd/migrate up | down | status
package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"os"

	"emailsender/internal/config"
	"emailsender/migrations"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

func main() {
	dir := os.Args[1]
	if dir != "up" && dir != "down" && dir != "status" {
		log.Fatal("usage: migrate <up|down|status>")
	}

	cfg := config.Load()
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	fsys, err := fs.Sub(migrations.FS, ".")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	goose.SetBaseFS(fsys)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("dialect: %v", err)
	}

	if err := goose.RunContext(context.Background(), dir, db, "."); err != nil {
		fmt.Fprintln(os.Stderr, "goose failed:", err)
		os.Exit(1)
	}
}
