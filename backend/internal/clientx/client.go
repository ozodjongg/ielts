package clientx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/example/assessment-platform-v5/internal/authz"
)

type Client struct {
	BaseURL, Secret, Service string
	HTTP                     *http.Client
	Handler                  http.Handler
}

// New creates the legacy HTTP client. It is still useful for tools or future
// external integrations, but the monolith uses NewLocal for zero-network
// internal module calls.
func New(base, secret, service string) *Client {
	return &Client{BaseURL: strings.TrimRight(base, "/"), Secret: secret, Service: service, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// NewLocal connects one backend module directly to another in the same process.
// Authentication signatures are preserved, so module trust rules remain active
// without running nine HTTP servers.
func NewLocal(handler http.Handler, secret, service string) *Client {
	return &Client{Secret: secret, Service: service, Handler: handler}
}

func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	base := c.BaseURL
	if c.Handler != nil {
		base = "http://monolith.internal"
	}
	if base == "" {
		return fmt.Errorf("clientx: no BaseURL or local Handler configured")
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	authz.AttachService(req.Header, c.Secret, c.Service, method, req.URL.RequestURI())

	var res *http.Response
	if c.Handler != nil {
		rr := httptest.NewRecorder()
		c.Handler.ServeHTTP(rr, req)
		res = rr.Result()
	} else {
		client := c.HTTP
		if client == nil {
			client = &http.Client{Timeout: 10 * time.Second}
		}
		res, err = client.Do(req)
		if err != nil {
			return err
		}
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("%s %s: %s: %s", method, path, res.Status, string(b))
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}
