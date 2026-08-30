package webx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }
func E(status int, code, message string) error {
	return &Error{Status: status, Code: code, Message: message}
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func Decode(r *http.Request, dst any, max int64) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return E(http.StatusUnsupportedMediaType, "content_type", "application/json required")
	}
	limited := io.LimitReader(r.Body, max+1)
	dec := json.NewDecoder(limited)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return E(http.StatusBadRequest, "invalid_json", "invalid JSON body")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return E(http.StatusBadRequest, "invalid_json", "only one JSON document is allowed")
	}
	return nil
}
func Handle(fn func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if errors.As(err, &e) {
		JSON(w, e.Status, map[string]any{"error": e.Code, "message": e.Message, "request_id": RequestID(r)})
		return
	}

	// Convert common PostgreSQL constraint/input failures into stable client
	// responses. Raw SQL errors must never leak to browser clients, and malformed
	// UUIDs such as an accidental "undefined" path must not become HTTP 500.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "22P02", "22007", "22008", "22023":
			JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_input", "message": "request contains an invalid identifier or value", "request_id": RequestID(r)})
			return
		case "23505":
			JSON(w, http.StatusConflict, map[string]any{"error": "conflict", "message": "resource already exists", "request_id": RequestID(r)})
			return
		case "23503":
			JSON(w, http.StatusConflict, map[string]any{"error": "reference_conflict", "message": "referenced resource does not exist or is still in use", "request_id": RequestID(r)})
			return
		case "23514", "23502":
			JSON(w, http.StatusBadRequest, map[string]any{"error": "validation", "message": "request violates a data constraint", "request_id": RequestID(r)})
			return
		}
	}

	slog.Error("request failed", "request_id", RequestID(r), "method", r.Method, "path", r.URL.Path, "error", err)
	JSON(w, http.StatusInternalServerError, map[string]any{"error": "internal", "message": "internal server error", "request_id": RequestID(r)})
}

type ctxKey string

const requestIDKey ctxKey = "request_id"

func RequestID(r *http.Request) string {
	if v, ok := r.Context().Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func withRequestID(r *http.Request) (*http.Request, string) {
	id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if id == "" || len(id) > 128 {
		id = fmt.Sprintf("ielts-%d", time.Now().UnixNano())
	}
	ctx := context.WithValue(r.Context(), requestIDKey, id)
	return r.WithContext(ctx), id
}

func Security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, requestID := withRequestID(r)
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), payment=(), usb=()")
		w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func Server(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 75 * time.Second, MaxHeaderBytes: 1 << 20}
}
