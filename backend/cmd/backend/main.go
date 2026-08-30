package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/example/ielts-platform/internal/auth"
	"github.com/example/ielts-platform/internal/bank"
	"github.com/example/ielts-platform/internal/clientx"
	"github.com/example/ielts-platform/internal/config"
	"github.com/example/ielts-platform/internal/dbx"
	"github.com/example/ielts-platform/internal/migrate"
	"github.com/example/ielts-platform/internal/webx"
	"github.com/example/ielts-platform/modules/analytics"
	"github.com/example/ielts-platform/modules/assessment"
	"github.com/example/ielts-platform/modules/gateway"
	"github.com/example/ielts-platform/modules/identity"
	"github.com/example/ielts-platform/modules/listening"
	"github.com/example/ielts-platform/modules/points"
	"github.com/example/ielts-platform/modules/review"
	"github.com/example/ielts-platform/modules/sat"
	"github.com/example/ielts-platform/modules/tenant"
	"github.com/example/ielts-platform/modules/vocabulary"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var moduleNames = []string{"identity", "tenant", "assessment", "vocabulary", "listening", "review", "sat", "points", "analytics"}

func schemaDSN(raw, schema string) string {
	u, err := url.Parse(raw)
	if err != nil {
		panic(fmt.Sprintf("invalid DATABASE_URL: %v", err))
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	return u.String()
}

func requiredSigningSecret(key string) string {
	v := config.Required(key)
	if len(strings.TrimSpace(v)) < 32 {
		log.Fatalf("%s must be at least 32 characters", key)
	}
	return v
}

func firstExisting(candidates ...string) string {
	for _, p := range candidates {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return candidates[0]
}

func openModuleDBs(ctx context.Context, dsn string) (map[string]*pgxpool.Pool, error) {
	pools := make(map[string]*pgxpool.Pool, len(moduleNames))
	for _, name := range moduleNames {
		p, err := dbx.Open(ctx, schemaDSN(dsn, name))
		if err != nil {
			for _, opened := range pools {
				opened.Close()
			}
			return nil, fmt.Errorf("open %s schema: %w", name, err)
		}
		pools[name] = p
	}
	return pools, nil
}

func closePools(pools map[string]*pgxpool.Pool) {
	for _, p := range pools {
		p.Close()
	}
}

func waitForPostgres(ctx context.Context, dsn string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		conn, err := pgx.Connect(attemptCtx, dsn)
		if err == nil {
			err = conn.Ping(attemptCtx)
			_ = conn.Close(context.Background())
		}
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("postgres not ready after %s: %w", timeout, lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// One database, one process. Each logical module still gets its own schema.
	dsn := config.Required("DATABASE_URL")
	if err := waitForPostgres(ctx, dsn, 90*time.Second); err != nil {
		log.Fatal(err)
	}
	migrationsDir := firstExisting(
		config.String("MIGRATIONS_DIR", "backend/migrations"),
		"./backend/migrations",
		"./migrations",
	)
	if config.Bool("AUTO_MIGRATE", true) {
		mctx, mcancel := context.WithTimeout(ctx, 2*time.Minute)
		if err := migrate.ApplyAll(mctx, dsn, migrationsDir); err != nil {
			mcancel()
			log.Fatalf("database migration: %v", err)
		}
		mcancel()
		slog.Info("database schemas ready", "schemas", len(migrate.Schemas))
	}

	// Keep the monolith connection footprint bounded by default.
	if strings.TrimSpace(os.Getenv("DB_MAX_CONNS")) == "" {
		_ = os.Setenv("DB_MAX_CONNS", "4")
	}
	if strings.TrimSpace(os.Getenv("DB_MIN_CONNS")) == "" {
		_ = os.Setenv("DB_MIN_CONNS", "0")
	}
	pools, err := openModuleDBs(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer closePools(pools)

	englishDir := firstExisting(
		config.String("ENGLISH_BANK_DIR", "data/english-bank"),
		"./data/english-bank",
		"../data/english-bank",
	)
	englishBank, err := bank.LoadEnglish(englishDir)
	if err != nil {
		log.Fatalf("load English bank: %v", err)
	}

	satDir := firstExisting(
		config.String("SAT_BANK_DIR", "data/sat-math-bank"),
		"./data/sat-math-bank",
		"../data/sat-math-bank",
	)
	satBank, err := bank.LoadSAT(filepath.Join(satDir, "questions.csv"))
	if err != nil {
		log.Fatalf("load SAT bank: %v", err)
	}

	secret := requiredSigningSecret("INTERNAL_SIGNING_SECRET")
	authSecret := requiredSigningSecret("AUTH_JWT_SECRET")
	authIssuer := config.String("AUTH_JWT_ISSUER", "ielts-platform")
	authAudience := config.String("AUTH_JWT_AUDIENCE", "ielts-platform")
	accessTTL := time.Duration(config.Int("AUTH_ACCESS_TTL_MINUTES", 15)) * time.Minute
	refreshDays := config.Int("AUTH_REFRESH_TTL_DAYS", 30)
	if refreshDays < 1 || refreshDays > 365 {
		log.Fatal("AUTH_REFRESH_TTL_DAYS must be between 1 and 365")
	}
	refreshTTL := time.Duration(refreshDays) * 24 * time.Hour
	signer, err := auth.NewSigner(authSecret, authIssuer, authAudience, accessTTL)
	if err != nil {
		log.Fatal(err)
	}

	identitySvc := &identity.Service{
		DB:               pools["identity"],
		Signer:           signer,
		RefreshTTL:       refreshTTL,
		InternalSecret:   secret,
		MFAEncryptionKey: requiredSigningSecret("MFA_ENCRYPTION_KEY"),
	}
	identityRouter := identitySvc.Router()

	tenantSvc := &tenant.Service{
		DB:             pools["tenant"],
		InternalSecret: secret,
		Identity:       clientx.NewLocal(identityRouter, secret, "tenant"),
	}
	tenantRouter := tenantSvc.Router()

	pointsSvc := &points.Service{DB: pools["points"], InternalSecret: secret}
	pointsRouter := pointsSvc.Router()
	analyticsSvc := &analytics.Service{DB: pools["analytics"], InternalSecret: secret}
	analyticsRouter := analyticsSvc.Router()

	assessmentSvc := &assessment.Service{
		DB:             pools["assessment"],
		Bank:           englishBank,
		InternalSecret: secret,
		QuestionSecret: requiredSigningSecret("QUESTION_SHUFFLE_SECRET"),
		Tenant:         clientx.NewLocal(tenantRouter, secret, "assessment"),
		Identity:       clientx.NewLocal(identityRouter, secret, "assessment"),
		Points:         clientx.NewLocal(pointsRouter, secret, "assessment"),
		Analytics:      clientx.NewLocal(analyticsRouter, secret, "assessment"),
	}
	assessmentRouter := assessmentSvc.Router()

	vocabularySvc := &vocabulary.Service{
		DB:             pools["vocabulary"],
		InternalSecret: secret,
		DailyNew:       config.Int("VOCAB_DAILY_NEW", 10),
		DailyReview:    config.Int("VOCAB_DAILY_REVIEW", 10),
		Tenant:         clientx.NewLocal(tenantRouter, secret, "vocabulary"),
		Identity:       clientx.NewLocal(identityRouter, secret, "vocabulary"),
		Points:         clientx.NewLocal(pointsRouter, secret, "vocabulary"),
		Analytics:      clientx.NewLocal(analyticsRouter, secret, "vocabulary"),
	}
	vocabularyRouter := vocabularySvc.Router()

	listeningSvc := &listening.Service{
		DB:             pools["listening"],
		InternalSecret: secret,
		PlaybackSecret: requiredSigningSecret("PLAYBACK_SIGNING_SECRET"),
		StorageDir:     config.String("LISTENING_STORAGE_DIR", "./.runtime/data/listening"),
		MaxUpload:      int64(config.Int("LISTENING_MAX_UPLOAD_MB", 50)) << 20,
		Tenant:         clientx.NewLocal(tenantRouter, secret, "listening"),
		Points:         clientx.NewLocal(pointsRouter, secret, "listening"),
		Analytics:      clientx.NewLocal(analyticsRouter, secret, "listening"),
	}
	if err := os.MkdirAll(listeningSvc.StorageDir, 0o750); err != nil {
		log.Fatalf("listening storage: %v", err)
	}
	listeningRouter := listeningSvc.Router()

	reviewSvc := &review.Service{
		DB:             pools["review"],
		InternalSecret: secret,
		StorageDir:     config.String("REVIEW_STORAGE_DIR", "./.runtime/data/review"),
		MaxAudio:       int64(config.Int("REVIEW_MAX_AUDIO_MB", 20)) << 20,
		Assessment:     clientx.NewLocal(assessmentRouter, secret, "review"),
	}
	if err := os.MkdirAll(reviewSvc.StorageDir, 0o750); err != nil {
		log.Fatalf("review storage: %v", err)
	}
	reviewRouter := reviewSvc.Router()

	satSvc := &sat.Service{
		DB:             pools["sat"],
		Bank:           satBank,
		InternalSecret: secret,
		QuestionSecret: requiredSigningSecret("QUESTION_SHUFFLE_SECRET"),
		Tenant:         clientx.NewLocal(tenantRouter, secret, "sat"),
		Points:         clientx.NewLocal(pointsRouter, secret, "sat"),
		Analytics:      clientx.NewLocal(analyticsRouter, secret, "sat"),
	}
	satRouter := satSvc.Router()

	handlers := map[string]http.Handler{
		"identity":   identityRouter,
		"tenant":     tenantRouter,
		"assessment": assessmentRouter,
		"vocabulary": vocabularyRouter,
		"listening":  listeningRouter,
		"review":     reviewRouter,
		"sat":        satRouter,
		"points":     pointsRouter,
		"analytics":  analyticsRouter,
	}
	verifier, err := auth.NewVerifier(authSecret, authIssuer, authAudience)
	if err != nil {
		log.Fatal(err)
	}

	readyChecks := make(map[string]func(context.Context) error, len(pools))
	for name, pool := range pools {
		p := pool
		readyChecks[name] = func(ctx context.Context) error { return p.Ping(ctx) }
	}

	gatewaySvc := &gateway.Service{
		IdentityHandler:    identityRouter,
		Verifier:           verifier,
		Identity:           clientx.NewLocal(identityRouter, secret, "gateway"),
		InternalSecret:     secret,
		Handlers:           handlers,
		ReadyChecks:        readyChecks,
		AdminOrigins:       config.CSV("ADMIN_ORIGINS"),
		CenterOrigins:      config.CSV("CENTER_ORIGINS"),
		TeacherOrigins:     config.CSV("TEACHER_ORIGINS"),
		StudentOrigins:     config.CSV("STUDENT_ORIGINS"),
		RequireAdminAAL2:   config.Bool("REQUIRE_ADMIN_AAL2", true),
		RequireCenterAAL2:  config.Bool("REQUIRE_CENTER_AAL2", true),
		RequireTeacherAAL2: config.Bool("REQUIRE_TEACHER_AAL2", true),
		Limiter:            gateway.NewLimiter(config.Int("GATEWAY_RATE_LIMIT_PER_MINUTE", 600), time.Minute),
		AuthLimiter:        gateway.NewLimiter(config.Int("GATEWAY_AUTH_RATE_LIMIT_PER_MINUTE", 30), time.Minute),
	}

	// Railway injects PORT dynamically. Prefer it when present, while keeping
	// BACKEND_ADDR/GATEWAY_ADDR for local Docker and other environments.
	addr := config.String("BACKEND_ADDR", config.String("GATEWAY_ADDR", ":8080"))
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		addr = ":" + port
	}
	srv := webx.Server(addr, gatewaySvc.Router())
	srv.ReadTimeout = 60 * time.Second
	srv.WriteTimeout = 0

	go func() {
		slog.Info("IELTS platform monolith ready",
			"addr", srv.Addr,
			"modules", len(handlers),
			"english_questions", len(englishBank.Questions),
			"sat_questions", len(satBank.Questions),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}
