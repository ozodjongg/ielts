package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/example/assessment-platform-v5/internal/migrate"
)

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func main() {
	dsn := env("DATABASE_URL", env("POSTGRES_URL", ""))
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := migrate.ApplyAll(ctx, dsn, env("MIGRATIONS_DIR", "./migrations")); err != nil {
		log.Fatal(err)
	}
	log.Printf("migrations complete: %d module schemas ready", len(migrate.Schemas))
}
