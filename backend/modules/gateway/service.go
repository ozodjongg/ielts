package gateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/example/ielts-platform/internal/auth"
	"github.com/example/ielts-platform/internal/authz"
	"github.com/example/ielts-platform/internal/clientx"
	"github.com/example/ielts-platform/internal/webx"
)

type Profile struct {
	UserID         string  `json:"user_id"`
	OrganizationID *string `json:"organization_id"`
	Role           string  `json:"role"`
	Email          string  `json:"email"`
	FullName       string  `json:"full_name"`
	Status         string  `json:"status"`
	CurrentLevel   *string `json:"current_level"`
}
type Service struct {
	IdentityHandler                                             http.Handler
	Verifier                                                    *auth.Verifier
	Identity                                                    *clientx.Client
	InternalSecret                                              string
	Handlers                                                    map[string]http.Handler
	ReadyChecks                                                 map[string]func(context.Context) error
	AdminOrigins, CenterOrigins, TeacherOrigins, StudentOrigins []string
	RequireAdminAAL2, RequireCenterAAL2, RequireTeacherAAL2     bool
	Limiter                                                     *Limiter
	AuthLimiter                                                 *Limiter
	PublicPlacementLimiter                                      *Limiter
}

func (s *Service) Router() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		webx.JSON(w, 200, map[string]any{"status": "ok", "service": "backend", "mode": "modular_monolith", "version": "ielts-1"})
	})
	m.HandleFunc("GET /ready", s.ready)
	m.HandleFunc("POST /auth/{portal}/{action}", s.authEndpoint)
	m.Handle("/api/", http.HandlerFunc(s.proxy))
	m.Handle("/public/placement/", http.HandlerFunc(s.publicPlacement))
	return s.cors(webx.Security(m))
}
func (s *Service) ready(w http.ResponseWriter, r *http.Request) {
	type state struct {
		OK        bool   `json:"ok"`
		LatencyMS int64  `json:"latency_ms"`
		Error     string `json:"error,omitempty"`
	}
	states := map[string]state{}
	allOK := true
	for name, h := range s.Handlers {
		start := time.Now()
		st := state{OK: h != nil}
		if st.OK {
			if check := s.ReadyChecks[name]; check != nil {
				ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
				err := check(ctx)
				cancel()
				if err != nil {
					st.OK = false
					st.Error = "database unavailable"
				}
			}
		}
		st.LatencyMS = time.Since(start).Milliseconds()
		if !st.OK {
			allOK = false
		}
		states[name] = st
	}
	statusCode := http.StatusOK
	statusText := "ready"
	if !allOK {
		statusCode = http.StatusServiceUnavailable
		statusText = "not_ready"
	}
	webx.JSON(w, statusCode, map[string]any{
		"status":     statusText,
		"mode":       "modular_monolith",
		"modules":    states,
		"checked_at": time.Now().UTC(),
	})
}
func (s *Service) proxy(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = fmt.Sprintf("ielts-%d", time.Now().UnixNano())
	}
	w.Header().Set("X-Request-ID", requestID)
	role, service, downPath, ok := parsePath(r.URL.Path)
	if !ok {
		webx.JSON(w, 404, map[string]any{"error": "not_found", "request_id": requestID})
		return
	}
	if !allowedForRequest(role, service, downPath, r.Method) {
		webx.JSON(w, 404, map[string]any{"error": "not_found", "request_id": requestID})
		return
	}
	if !s.originAllowed(role, r.Header.Get("Origin")) {
		webx.JSON(w, 403, map[string]any{"error": "origin_forbidden", "message": "origin is not allowed for this portal", "request_id": requestID})
		return
	}
	raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if raw == "" || raw == r.Header.Get("Authorization") {
		webx.JSON(w, 401, map[string]any{"error": "unauthorized", "message": "Bearer token required", "request_id": requestID})
		return
	}
	p, e := s.Verifier.Verify(r.Context(), raw)
	if e != nil {
		webx.JSON(w, 401, map[string]any{"error": "unauthorized", "message": "invalid access token", "request_id": requestID})
		return
	}
	var prof Profile
	if e = s.Identity.Do(r.Context(), "GET", "/internal/resolve?user_id="+url.QueryEscape(p.UserID)+"&auth_version="+url.QueryEscape(fmt.Sprint(p.AuthVersion))+"&session_id="+url.QueryEscape(p.SessionID), nil, &prof); e != nil {
		slog.Warn("profile resolve denied", "request_id", requestID, "user", p.UserID, "error", e)
		webx.JSON(w, 403, map[string]any{"error": "forbidden", "message": "account is not provisioned or active", "request_id": requestID})
		return
	}
	if prof.Role != role || prof.Status != "active" {
		webx.JSON(w, 403, map[string]any{"error": "role_forbidden", "message": "this account cannot use this portal", "request_id": requestID})
		return
	}
	org := ""
	if prof.OrganizationID != nil {
		org = *prof.OrganizationID
	}
	if role != "admin" && org == "" {
		webx.JSON(w, 403, map[string]any{"error": "tenant_missing", "message": "account is not attached to an active learning center", "request_id": requestID})
		return
	}
	mutating := r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodDelete || r.Method == http.MethodPut
	mfaBootstrap := service == "identity" && strings.HasPrefix(downPath, "/v1/mfa/")
	if mutating && !mfaBootstrap && ((role == "admin" && s.RequireAdminAAL2) || (role == "center" && s.RequireCenterAAL2) || (role == "teacher" && s.RequireTeacherAAL2)) && p.AAL != "aal2" {
		webx.JSON(w, 403, map[string]any{"error": "mfa_required", "message": "AAL2/MFA is required for this sensitive action", "request_id": requestID})
		return
	}
	if s.Limiter != nil && !s.Limiter.Allow(p.UserID+":"+service) {
		w.Header().Set("Retry-After", "60")
		webx.JSON(w, 429, map[string]any{"error": "rate_limited", "message": "too many requests", "request_id": requestID})
		return
	}
	actor := authz.Actor{UserID: prof.UserID, Role: prof.Role, OrgID: org, Email: prof.Email, AAL: p.AAL, SessionID: p.SessionID}
	h := s.Handlers[service]
	if h == nil {
		webx.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "module_unavailable", "request_id": requestID})
		return
	}
	req := r.Clone(r.Context())
	u := *r.URL
	u.Path = downPath
	u.RawPath = ""
	req.URL = &u
	req.RequestURI = u.RequestURI()
	req.Header = make(http.Header)
	copyRequestHeaders(req.Header, r.Header)
	req.Header.Del("Authorization")
	req.Header.Set("X-Request-ID", requestID)
	authz.Attach(req.Header, s.InternalSecret, req.Method, req.URL.RequestURI(), actor)
	h.ServeHTTP(w, req)
}

func (s *Service) publicPlacement(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = fmt.Sprintf("ielts-%d", time.Now().UnixNano())
	}
	w.Header().Set("X-Request-ID", requestID)
	if !s.originAllowed("center", r.Header.Get("Origin")) {
		webx.JSON(w, http.StatusForbidden, map[string]any{"error": "origin_forbidden", "message": "origin is not allowed for placement", "request_id": requestID})
		return
	}
	if s.PublicPlacementLimiter != nil && !s.PublicPlacementLimiter.Allow(requestIP(r)) {
		w.Header().Set("Retry-After", "60")
		webx.JSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "message": "too many placement requests", "request_id": requestID})
		return
	}

	suffix := strings.TrimPrefix(r.URL.Path, "/public/placement/")
	allowed := (r.Method == http.MethodPost && suffix == "invitations/claim") ||
		(r.Method == http.MethodGet && suffix == "session") ||
		(r.Method == http.MethodPost && suffix == "session/answer") ||
		(r.Method == http.MethodPost && suffix == "session/finish")
	if !allowed {
		webx.JSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "request_id": requestID})
		return
	}
	h := s.Handlers["assessment"]
	if h == nil {
		webx.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "module_unavailable", "request_id": requestID})
		return
	}
	req := r.Clone(r.Context())
	u := *r.URL
	u.Path = "/v1/public/placement/" + suffix
	u.RawPath = ""
	req.URL = &u
	req.RequestURI = u.RequestURI()
	req.Header = make(http.Header)
	copyRequestHeaders(req.Header, r.Header)
	req.Header.Del("Authorization")
	req.Header.Set("X-Request-ID", requestID)
	h.ServeHTTP(w, req)
}

func portalRole(portal string) (string, bool) {
	switch portal {
	case "admin":
		return "admin", true
	case "center":
		return "center", true
	case "teacher":
		return "teacher", true
	case "student":
		return "student", true
	default:
		return "", false
	}
}

func requestIP(r *http.Request) string {
	if x := strings.TrimSpace(r.Header.Get("X-Real-IP")); x != "" {
		return x
	}
	if raw := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); raw != "" {
		// Proxies append hops to X-Forwarded-For; the first value is the
		// original client address when Railway/Vercel proxy headers are trusted.
		parts := strings.Split(raw, ",")
		if x := strings.TrimSpace(parts[0]); x != "" {
			return x
		}
	}
	return r.RemoteAddr
}

func (s *Service) authEndpoint(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = fmt.Sprintf("ielts-%d", time.Now().UnixNano())
	}
	w.Header().Set("X-Request-ID", requestID)
	portal := r.PathValue("portal")
	action := r.PathValue("action")
	role, ok := portalRole(portal)
	if !ok || (action != "login" && action != "refresh" && action != "logout" && action != "mfa-verify") {
		webx.JSON(w, http.StatusNotFound, map[string]any{"error": "not_found", "request_id": requestID})
		return
	}
	if !s.originAllowed(role, r.Header.Get("Origin")) {
		webx.JSON(w, http.StatusForbidden, map[string]any{"error": "origin_forbidden", "message": "origin is not allowed for this portal", "request_id": requestID})
		return
	}
	if s.IdentityHandler == nil {
		webx.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "identity_unavailable", "request_id": requestID})
		return
	}
	if s.AuthLimiter != nil && action != "logout" && !s.AuthLimiter.Allow(requestIP(r)+":"+portal) {
		w.Header().Set("Retry-After", "60")
		webx.JSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "message": "too many authentication attempts", "request_id": requestID})
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&payload); err != nil {
		webx.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "message": "invalid JSON body", "request_id": requestID})
		return
	}
	payload["expected_role"] = role
	payload["user_agent"] = r.UserAgent()
	payload["ip_address"] = requestIP(r)
	body, err := json.Marshal(payload)
	if err != nil {
		webx.JSON(w, http.StatusInternalServerError, map[string]any{"error": "internal", "request_id": requestID})
		return
	}
	path := "/internal/auth/" + action
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "http://monolith.internal"+path, strings.NewReader(string(body)))
	if err != nil {
		webx.JSON(w, http.StatusInternalServerError, map[string]any{"error": "internal", "request_id": requestID})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	authz.AttachService(req.Header, s.InternalSecret, "gateway", http.MethodPost, path)
	rr := httptest.NewRecorder()
	s.IdentityHandler.ServeHTTP(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	for k, vals := range res.Header {
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Connection") {
			continue
		}
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}

func parsePath(path string) (role, service, down string, ok bool) {
	p := strings.Split(strings.TrimPrefix(path, "/api/"), "/")
	if len(p) < 2 {
		return "", "", "", false
	}
	switch p[0] {
	case "admin":
		role = "admin"
	case "center":
		role = "center"
	case "teacher":
		role = "teacher"
	case "student":
		role = "student"
	default:
		return "", "", "", false
	}
	service = p[1]
	if service == "" || service == "undefined" || service == "null" {
		return "", "", "", false
	}
	for _, segment := range p[2:] {
		if segment == "" || strings.EqualFold(segment, "undefined") || strings.EqualFold(segment, "null") {
			return "", "", "", false
		}
	}
	down = "/v1"
	if len(p) > 2 {
		down += "/" + strings.Join(p[2:], "/")
	}
	return role, service, down, true
}
func allowedForRequest(role, service, downPath, method string) bool {
	var list string
	switch role {
	case "admin":
		list = "identity tenant analytics vocabulary points assessment sat"
	case "center":
		list = "identity tenant assessment listening review sat analytics points vocabulary"
	case "teacher":
		list = "identity tenant vocabulary assessment listening sat"
	case "student":
		list = "identity assessment vocabulary listening review sat points analytics"
		// Students may read only their organization's effective service availability.
		// The rest of Tenant API remains unreachable from the student portal even
		// before tenant-side RBAC is evaluated.
		if service == "tenant" {
			return method == http.MethodGet && downPath == "/v1/services"
		}
	}
	for _, x := range strings.Fields(list) {
		if x == service {
			return true
		}
	}
	return false
}
func copyRequestHeaders(dst, src http.Header) {
	for k, vs := range src {
		lk := strings.ToLower(k)
		if lk == "connection" || lk == "keep-alive" || lk == "proxy-authenticate" || lk == "proxy-authorization" || lk == "te" || lk == "trailers" || lk == "transfer-encoding" || lk == "upgrade" || strings.HasPrefix(lk, "x-ielts-") {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
func (s *Service) origins(role string) []string {
	switch role {
	case "admin":
		return s.AdminOrigins
	case "center":
		return s.CenterOrigins
	case "teacher":
		return s.TeacherOrigins
	case "student":
		return s.StudentOrigins
	}
	return nil
}
func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func (s *Service) originAllowed(role, origin string) bool {
	if origin == "" {
		return true
	}
	for _, o := range s.origins(role) {
		if secureEqual(strings.TrimRight(o, "/"), strings.TrimRight(origin, "/")) {
			return true
		}
	}
	return false
}
func (s *Service) originAllowedAny(origin string) bool {
	if origin == "" {
		return true
	}
	return s.originAllowed("admin", origin) ||
		s.originAllowed("center", origin) ||
		s.originAllowed("teacher", origin) ||
		s.originAllowed("student", origin)
}

func (s *Service) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		path := r.URL.Path
		role := ""
		switch {
		case strings.HasPrefix(path, "/api/admin/"), strings.HasPrefix(path, "/auth/admin/"):
			role = "admin"
		case strings.HasPrefix(path, "/api/center/"), strings.HasPrefix(path, "/auth/center/"):
			role = "center"
		case strings.HasPrefix(path, "/api/teacher/"), strings.HasPrefix(path, "/auth/teacher/"):
			role = "teacher"
		case strings.HasPrefix(path, "/api/student/"), strings.HasPrefix(path, "/auth/student/"):
			role = "student"
		case strings.HasPrefix(path, "/public/placement/"):
			role = "center"
		}

		diagnostic := path == "/health" || path == "/ready"
		allowed := origin == ""
		if origin != "" {
			if diagnostic {
				allowed = s.originAllowedAny(origin)
			} else if role != "" {
				allowed = s.originAllowed(role, origin)
			}
		}

		if origin != "" && allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, Range, X-Playback-Token, X-Placement-Session")
			w.Header().Set("Access-Control-Allow-Methods", "GET,HEAD,POST,PATCH,PUT,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		if r.Method == http.MethodOptions {
			if !allowed || origin == "" {
				w.WriteHeader(http.StatusForbidden)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

type bucket struct {
	start time.Time
	count int
}
type Limiter struct {
	mu     sync.Mutex
	Window time.Duration
	Max    int
	items  map[string]bucket
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{Max: max, Window: window, items: map[string]bucket{}}
}
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b := l.items[key]
	if b.start.IsZero() || now.Sub(b.start) >= l.Window {
		b = bucket{start: now}
	}
	if b.count >= l.Max {
		l.items[key] = b
		return false
	}
	b.count++
	l.items[key] = b
	if len(l.items) > 100000 {
		for k, v := range l.items {
			if now.Sub(v.start) > 2*l.Window {
				delete(l.items, k)
			}
		}
	}
	return true
}

var _ = context.Background
var _ = json.Valid
