package authz

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Actor struct{ UserID, Role, OrgID, Email, AAL, SessionID string }

const (
	HUser    = "X-IELTS-User-ID"
	HRole    = "X-IELTS-Role"
	HOrg     = "X-IELTS-Org-ID"
	HEmail   = "X-IELTS-Email"
	HAAL     = "X-IELTS-AAL"
	HSession = "X-IELTS-Session-ID"
	HTS      = "X-IELTS-Timestamp"
	HSig     = "X-IELTS-Signature"
)

func canonical(method, path, ts string, a Actor) string {
	return strings.Join([]string{method, path, ts, a.UserID, a.Role, a.OrgID, a.Email, a.AAL, a.SessionID}, "\n")
}
func Sign(secret, method, path, ts string, a Actor) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(canonical(method, path, ts, a)))
	return hex.EncodeToString(m.Sum(nil))
}
func Attach(h http.Header, secret, method, path string, a Actor) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h.Set(HUser, a.UserID)
	h.Set(HRole, a.Role)
	h.Set(HOrg, a.OrgID)
	h.Set(HEmail, a.Email)
	h.Set(HAAL, a.AAL)
	h.Set(HSession, a.SessionID)
	h.Set(HTS, ts)
	h.Set(HSig, Sign(secret, method, path, ts, a))
}
func Verify(r *http.Request, secret string) (Actor, error) {
	a := Actor{UserID: r.Header.Get(HUser), Role: r.Header.Get(HRole), OrgID: r.Header.Get(HOrg), Email: r.Header.Get(HEmail), AAL: r.Header.Get(HAAL), SessionID: r.Header.Get(HSession)}
	ts := r.Header.Get(HTS)
	sig := r.Header.Get(HSig)
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || time.Since(time.Unix(n, 0)) > 90*time.Second || time.Until(time.Unix(n, 0)) > 30*time.Second {
		return Actor{}, errors.New("stale internal identity")
	}
	want := Sign(secret, r.Method, r.URL.RequestURI(), ts, a)
	got, err := hex.DecodeString(sig)
	if err != nil {
		return Actor{}, errors.New("invalid internal signature")
	}
	wb, _ := hex.DecodeString(want)
	if !hmac.Equal(got, wb) {
		return Actor{}, errors.New("invalid internal signature")
	}
	if a.UserID == "" || a.Role == "" || a.SessionID == "" {
		return Actor{}, errors.New("missing internal identity")
	}
	return a, nil
}
func Require(a Actor, roles ...string) error {
	for _, r := range roles {
		if a.Role == r {
			return nil
		}
	}
	return fmt.Errorf("forbidden role %q", a.Role)
}
func InternalServiceSignature(secret, service, method, path, ts string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(strings.Join([]string{service, method, path, ts}, "\n")))
	return hex.EncodeToString(m.Sum(nil))
}
func AttachService(h http.Header, secret, service, method, path string) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h.Set("X-IELTS-Service", service)
	h.Set(HTS, ts)
	h.Set("X-IELTS-Service-Signature", InternalServiceSignature(secret, service, method, path, ts))
}
func VerifyService(r *http.Request, secret string, allowed ...string) error {
	svc := r.Header.Get("X-IELTS-Service")
	ok := false
	for _, a := range allowed {
		if svc == a {
			ok = true
		}
	}
	if !ok {
		return errors.New("service not allowed")
	}
	ts := r.Header.Get(HTS)
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || time.Since(time.Unix(n, 0)) > 90*time.Second || time.Until(time.Unix(n, 0)) > 30*time.Second {
		return errors.New("stale service signature")
	}
	want := InternalServiceSignature(secret, svc, r.Method, r.URL.RequestURI(), ts)
	a, b := hex.DecodeString(r.Header.Get("X-IELTS-Service-Signature"))
	if b != nil {
		return b
	}
	w, _ := hex.DecodeString(want)
	if !hmac.Equal(a, w) {
		return errors.New("bad service signature")
	}
	return nil
}
func StripInbound(h http.Header) {
	for _, k := range []string{HUser, HRole, HOrg, HEmail, HAAL, HSession, HTS, HSig, "X-IELTS-Service", "X-IELTS-Service-Signature"} {
		h.Del(k)
	}
}
