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

	"github.com/example/assessment-platform-v5/internal/dbx"
	"github.com/example/assessment-platform-v5/internal/passwordhash"
	"github.com/google/uuid"
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
	email := flag.String("email", "", "platform administrator email")
	password := flag.String("password", "", "initial password (minimum 10 characters)")
	name := flag.String("name", "Platform Administrator", "administrator display name")
	flag.Parse()

	cleanEmail := strings.ToLower(strings.TrimSpace(*email))
	cleanName := strings.TrimSpace(*name)
	if cleanEmail == "" || !strings.Contains(cleanEmail, "@") || len(*password) < 10 || len(*password) > 128 || cleanName == "" {
		log.Fatal("--email, --name and --password (10-128 characters) are required")
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
	userID := uuid.New()
	tx, err := db.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO profiles(user_id,organization_id,role,email,full_name,status) VALUES($1,NULL,'platform_admin',$2,$3,'active')`, userID, cleanEmail, cleanName); err != nil {
		log.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth_credentials(user_id,password_hash) VALUES($1,$2)`, userID, hash); err != nil {
		log.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("platform_admin created: %s (%s)\n", cleanEmail, userID)
}
