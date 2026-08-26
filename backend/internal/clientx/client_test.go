package clientx

import (
	"context"
	"net/http"
	"testing"

	"github.com/example/assessment-platform-v5/internal/authz"
)

func TestLocalClientPreservesServiceAuthentication(t *testing.T) {
	const secret = "test-secret-with-enough-entropy-for-unit-test"
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := authz.VerifyService(r, secret, "assessment"); err != nil {
			t.Fatalf("service signature rejected: %v", err)
		}
		if r.URL.Path != "/internal/ping" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	var out struct {
		OK bool `json:"ok"`
	}
	c := NewLocal(h, secret, "assessment")
	if err := c.Do(context.Background(), http.MethodGet, "/internal/ping", nil, &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatal("expected ok=true")
	}
}
