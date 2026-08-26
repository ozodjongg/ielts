package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

var Schemas = []string{"identity", "tenant", "assessment", "vocabulary", "listening", "review", "sat", "points", "analytics"}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ApplyAll applies each migration exactly once and records its SHA-256 checksum.
// A PostgreSQL advisory lock serializes concurrent Railway/container startups.
// If an already-applied migration file changes, startup fails with migration drift
// instead of silently mutating production schema history.
func ApplyAll(ctx context.Context, dsn, root string) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtext('assessment-platform-v5:migrations'))`); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('assessment-platform-v5:migrations'))`)
	}()

	for _, ext := range []string{"pgcrypto", "pg_trgm"} {
		if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+quoteIdent(ext)+" WITH SCHEMA public"); err != nil {
			return fmt.Errorf("create extension %s: %w", ext, err)
		}
	}

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS public.app_schema_migrations (
			schema_name text NOT NULL,
			filename text NOT NULL,
			checksum_sha256 text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY(schema_name, filename)
		)`); err != nil {
		return fmt.Errorf("create migration history: %w", err)
	}

	for _, schema := range Schemas {
		if _, err := conn.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+quoteIdent(schema)); err != nil {
			return fmt.Errorf("create schema %s: %w", schema, err)
		}
		dir := filepath.Join(root, schema)
		files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
		if err != nil {
			return err
		}
		sort.Strings(files)
		if len(files) == 0 {
			return fmt.Errorf("no migrations found for %s in %s", schema, dir)
		}

		for _, file := range files {
			sqlBytes, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read %s: %w", file, err)
			}
			filename := filepath.Base(file)
			hash := checksum(sqlBytes)

			var existing string
			err = conn.QueryRow(ctx, `SELECT checksum_sha256 FROM public.app_schema_migrations WHERE schema_name=$1 AND filename=$2`, schema, filename).Scan(&existing)
			switch {
			case err == nil:
				if existing != hash {
					return fmt.Errorf("migration drift detected schema=%s file=%s: applied checksum=%s current checksum=%s", schema, filename, existing, hash)
				}
				continue
			case err != pgx.ErrNoRows:
				return fmt.Errorf("check migration history schema=%s file=%s: %w", schema, filename, err)
			}

			tx, err := conn.Begin(ctx)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "SET LOCAL search_path TO "+quoteIdent(schema)+", public"); err == nil {
				_, err = tx.Exec(ctx, string(sqlBytes))
			}
			if err == nil {
				_, err = tx.Exec(ctx, `INSERT INTO public.app_schema_migrations(schema_name,filename,checksum_sha256) VALUES($1,$2,$3)`, schema, filename, hash)
			}
			if err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("migration failed schema=%s file=%s: %w", schema, filename, err)
			}
			if err = tx.Commit(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}
