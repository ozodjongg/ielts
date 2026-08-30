package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/example/ielts-platform/internal/dbx"
	"github.com/example/ielts-platform/internal/passwordhash"
)

func env(name string) string { return strings.TrimSpace(os.Getenv(name)) }

func identityDSN(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("search_path", "identity")
	u.RawQuery = q.Encode()
	return u.String()
}

func main() {
	email := flag.String("email", "", "account email")
	password := flag.String("password", "", "new password (10-128 characters)")
	flag.Parse()

	cleanEmail := strings.ToLower(strings.TrimSpace(*email))
	if cleanEmail == "" || !strings.Contains(cleanEmail, "@") || len(*password) < 10 || len(*password) > 128 {
		log.Fatal("--email and --password (10-128 characters) are required")
	}
	dsn := env("IDENTITY_DATABASE_URL")
	if dsn == "" {
		dsn = identityDSN(env("DATABASE_URL"))
	}
	if dsn == "" {
		log.Fatal("DATABASE_URL or IDENTITY_DATABASE_URL must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	db, err := dbx.Open(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	hash, err := passwordhash.Hash(*password)
	if err != nil {
		log.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var userID string
	if err = tx.QueryRow(ctx, `SELECT user_id FROM profiles WHERE lower(email)=lower($1)`, cleanEmail).Scan(&userID); err != nil {
		log.Fatal("account not found or database error: ", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE auth_credentials SET password_hash=$2,failed_attempts=0,locked_until=NULL,password_changed_at=now(),updated_at=now() WHERE user_id=$1`, userID, hash); err != nil {
		log.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE profiles SET auth_version=auth_version+1,updated_at=now() WHERE user_id=$1`, userID); err != nil {
		log.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, userID); err != nil {
		log.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("password reset: %s (%s); all sessions revoked\n", cleanEmail, userID)
}
