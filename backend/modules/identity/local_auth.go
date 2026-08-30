package identity

import (
	"context"
	"crypto/sha256"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/example/ielts-platform/internal/auth"
	"github.com/example/ielts-platform/internal/passwordhash"
	"github.com/example/ielts-platform/internal/webx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const dummyPasswordHash = "pbkdf2_sha256$600000$VjVMb2NhbEF1dGhEdW1teSE$usi/qeeqkZrUm3/sadYOqPKgKaBaMKhga0ifNnMNA+M"

type AuthRequest struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	ExpectedRole string `json:"expected_role"`
	UserAgent    string `json:"user_agent,omitempty"`
	IPAddress    string `json:"ip_address,omitempty"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	ExpectedRole string `json:"expected_role"`
	UserAgent    string `json:"user_agent,omitempty"`
	IPAddress    string `json:"ip_address,omitempty"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
	ExpectedRole string `json:"expected_role,omitempty"`
	UserAgent    string `json:"user_agent,omitempty"`
	IPAddress    string `json:"ip_address,omitempty"`
}

type AuthSession struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    int64   `json:"expires_in"`
	ExpiresAt    string  `json:"expires_at"`
	Profile      Profile `json:"profile"`
}

func validRole(role string) bool {
	return role == "admin" || role == "center" || role == "teacher" || role == "student"
}

func normalizeIP(v string) any {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if host, _, err := net.SplitHostPort(v); err == nil {
		v = host
	}
	if ip := net.ParseIP(v); ip != nil {
		return ip.String()
	}
	return nil
}

func refreshHash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func newRefreshToken(sessionID string) (string, error) {
	random, err := auth.NewOpaqueToken(32)
	if err != nil {
		return "", err
	}
	return sessionID + "." + random, nil
}

func parseRefreshToken(token string) (string, bool) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 || len(parts[1]) < 32 {
		return "", false
	}
	if _, err := uuid.Parse(parts[0]); err != nil {
		return "", false
	}
	return parts[0], true
}

func (s *Service) issueSession(ctx context.Context, p Profile, authVersion int, sessionID, aal string, rotateExisting bool, previousRefresh, userAgent string, ip any) (AuthSession, error) {
	if s.Signer == nil {
		return AuthSession{}, errors.New("auth signer is not configured")
	}
	refreshTTL := s.RefreshTTL
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	refresh, err := newRefreshToken(sessionID)
	if err != nil {
		return AuthSession{}, err
	}
	expires := time.Now().UTC().Add(refreshTTL)
	hash := refreshHash(refresh)
	if rotateExisting {
		ct, err := s.DB.Exec(ctx, `UPDATE auth_sessions SET refresh_token_hash=$2,last_used_at=now(),user_agent=$3,ip_address=$4 WHERE id=$1 AND refresh_token_hash=$5 AND revoked_at IS NULL AND expires_at>now()`, sessionID, hash, nullableTrim(userAgent), ip, refreshHash(previousRefresh))
		if err != nil {
			return AuthSession{}, err
		}
		if ct.RowsAffected() == 0 {
			return AuthSession{}, webx.E(401, "invalid_refresh", "refresh token is invalid or expired")
		}
	} else {
		_, err = s.DB.Exec(ctx, `INSERT INTO auth_sessions(id,user_id,refresh_token_hash,aal,user_agent,ip_address,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, sessionID, p.UserID, hash, aal, nullableTrim(userAgent), ip, expires)
		if err != nil {
			return AuthSession{}, err
		}
	}
	access, accessExp, err := s.Signer.AccessToken(p.UserID, p.Email, sessionID, aal, authVersion)
	if err != nil {
		return AuthSession{}, err
	}
	return AuthSession{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(time.Until(accessExp).Seconds()),
		ExpiresAt:    accessExp.UTC().Format(time.RFC3339),
		Profile:      p,
	}, nil
}

func nullableTrim(v string) any {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if len(v) > 512 {
		v = v[:512]
	}
	return v
}

func (s *Service) auditLogin(ctx context.Context, x AuthRequest, userID any, success bool, reason string) {
	_, _ = s.DB.Exec(ctx, `INSERT INTO auth_login_audit(email,expected_role,user_id,success,ip_address,user_agent,reason) VALUES($1,$2,$3,$4,$5,$6,$7)`, strings.ToLower(strings.TrimSpace(x.Email)), x.ExpectedRole, userID, success, normalizeIP(x.IPAddress), nullableTrim(x.UserAgent), reason)
}

func (s *Service) internalLogin(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "gateway"); err != nil {
		return err
	}
	var x AuthRequest
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	x.Email = strings.ToLower(strings.TrimSpace(x.Email))
	if !strings.Contains(x.Email, "@") || len(x.Email) > 254 || len(x.Password) > 128 || !validRole(x.ExpectedRole) {
		return webx.E(401, "invalid_credentials", "email or password is incorrect")
	}
	var p Profile
	var org *uuid.UUID
	var passwordHash string
	var authVersion, failed int
	var lockedUntil *time.Time
	err := s.DB.QueryRow(r.Context(), `SELECT p.user_id,p.organization_id,p.role,p.email,p.full_name,p.status,p.current_level,p.locale,p.auth_version,c.password_hash,c.failed_attempts,c.locked_until FROM profiles p JOIN auth_credentials c ON c.user_id=p.user_id WHERE lower(p.email)=lower($1) AND p.role=$2`, x.Email, x.ExpectedRole).Scan(&p.UserID, &org, &p.Role, &p.Email, &p.FullName, &p.Status, &p.CurrentLevel, &p.Locale, &authVersion, &passwordHash, &failed, &lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		// Keep unknown-email timing closer to a real password check to reduce account enumeration signals.
		_ = passwordhash.Verify(dummyPasswordHash, x.Password)
		s.auditLogin(r.Context(), x, nil, false, "invalid_credentials")
		return webx.E(401, "invalid_credentials", "email or password is incorrect")
	}
	if err != nil {
		return err
	}
	if org != nil {
		o := org.String()
		p.OrganizationID = &o
	}
	uid, _ := uuid.Parse(p.UserID)
	if p.Status != "active" {
		s.auditLogin(r.Context(), x, uid, false, "account_inactive")
		return webx.E(403, "account_inactive", "account is not active")
	}
	if lockedUntil != nil && lockedUntil.After(time.Now()) {
		s.auditLogin(r.Context(), x, uid, false, "account_locked")
		return webx.E(429, "account_locked", "too many failed login attempts; try again later")
	}
	if !passwordhash.Verify(passwordHash, x.Password) {
		failed++
		if failed >= 5 {
			_, _ = s.DB.Exec(r.Context(), `UPDATE auth_credentials SET failed_attempts=0,locked_until=now()+interval '15 minutes',updated_at=now() WHERE user_id=$1`, p.UserID)
		} else {
			_, _ = s.DB.Exec(r.Context(), `UPDATE auth_credentials SET failed_attempts=$2,locked_until=NULL,updated_at=now() WHERE user_id=$1`, p.UserID, failed)
		}
		s.auditLogin(r.Context(), x, uid, false, "invalid_credentials")
		return webx.E(401, "invalid_credentials", "email or password is incorrect")
	}
	_, _ = s.DB.Exec(r.Context(), `UPDATE auth_credentials SET failed_attempts=0,locked_until=NULL,updated_at=now() WHERE user_id=$1`, p.UserID)
	_, _ = s.DB.Exec(r.Context(), `DELETE FROM auth_sessions WHERE user_id=$1 AND (expires_at<=now() OR revoked_at IS NOT NULL AND revoked_at<now()-interval '7 days')`, p.UserID)
	var mfaEnabled bool
	if validMFARole(p.Role) {
		err = s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM mfa_totp WHERE user_id=$1 AND enabled=true)`, p.UserID).Scan(&mfaEnabled)
		if err != nil {
			return err
		}
	}
	if mfaEnabled {
		challengeID, challengeErr := s.createMFAChallenge(r.Context(), p.UserID, p.Role)
		if challengeErr != nil {
			return challengeErr
		}
		s.auditLogin(r.Context(), x, uid, false, "mfa_required")
		webx.JSON(w, http.StatusAccepted, map[string]any{"mfa_required": true, "challenge_id": challengeID, "expires_in": 300, "profile": p})
		return nil
	}
	sessionID := uuid.NewString()
	out, err := s.issueSession(r.Context(), p, authVersion, sessionID, "aal1", false, "", x.UserAgent, normalizeIP(x.IPAddress))
	if err != nil {
		return err
	}
	s.auditLogin(r.Context(), x, uid, true, "ok")
	webx.JSON(w, http.StatusOK, out)
	return nil
}

func (s *Service) internalRefresh(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "gateway"); err != nil {
		return err
	}
	var x RefreshRequest
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if !validRole(x.ExpectedRole) {
		return webx.E(401, "invalid_refresh", "refresh token is invalid or expired")
	}
	sessionID, ok := parseRefreshToken(x.RefreshToken)
	if !ok {
		return webx.E(401, "invalid_refresh", "refresh token is invalid or expired")
	}
	var p Profile
	var org *uuid.UUID
	var authVersion int
	var aal string
	err := s.DB.QueryRow(r.Context(), `SELECT p.user_id,p.organization_id,p.role,p.email,p.full_name,p.status,p.current_level,p.locale,p.auth_version,s.aal FROM auth_sessions s JOIN profiles p ON p.user_id=s.user_id WHERE s.id=$1 AND s.refresh_token_hash=$2 AND s.revoked_at IS NULL AND s.expires_at>now() AND p.role=$3`, sessionID, refreshHash(x.RefreshToken), x.ExpectedRole).Scan(&p.UserID, &org, &p.Role, &p.Email, &p.FullName, &p.Status, &p.CurrentLevel, &p.Locale, &authVersion, &aal)
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(401, "invalid_refresh", "refresh token is invalid or expired")
	}
	if err != nil {
		return err
	}
	if p.Status != "active" {
		_, _ = s.DB.Exec(r.Context(), `UPDATE auth_sessions SET revoked_at=now() WHERE id=$1`, sessionID)
		return webx.E(403, "account_inactive", "account is not active")
	}
	if org != nil {
		o := org.String()
		p.OrganizationID = &o
	}
	out, err := s.issueSession(r.Context(), p, authVersion, sessionID, aal, true, x.RefreshToken, x.UserAgent, normalizeIP(x.IPAddress))
	if err != nil {
		return err
	}
	webx.JSON(w, http.StatusOK, out)
	return nil
}

func (s *Service) internalLogout(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "gateway"); err != nil {
		return err
	}
	var x LogoutRequest
	if err := webx.Decode(r, &x, 32<<10); err != nil {
		return err
	}
	if sessionID, ok := parseRefreshToken(x.RefreshToken); ok {
		_, _ = s.DB.Exec(r.Context(), `UPDATE auth_sessions SET revoked_at=now() WHERE id=$1 AND refresh_token_hash=$2 AND revoked_at IS NULL`, sessionID, refreshHash(x.RefreshToken))
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Service) changePassword(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	var x struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := webx.Decode(r, &x, 32<<10); err != nil {
		return err
	}
	if len(x.NewPassword) < 10 || len(x.NewPassword) > 128 {
		return webx.E(400, "new_password", "new password must be 10-128 characters")
	}
	if len(x.CurrentPassword) == 0 || len(x.CurrentPassword) > 128 {
		return webx.E(400, "current_password", "current password is required")
	}
	var currentHash string
	if err := s.DB.QueryRow(r.Context(), `SELECT password_hash FROM auth_credentials WHERE user_id=$1`, a.UserID).Scan(&currentHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return webx.E(403, "credentials_missing", "credentials are not provisioned")
		}
		return err
	}
	if !passwordhash.Verify(currentHash, x.CurrentPassword) {
		return webx.E(401, "current_password", "current password is incorrect")
	}
	if passwordhash.Verify(currentHash, x.NewPassword) {
		return webx.E(400, "new_password", "new password must be different from the current password")
	}
	newHash, err := passwordhash.Hash(x.NewPassword)
	if err != nil {
		return webx.E(400, "new_password", err.Error())
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `UPDATE auth_credentials SET password_hash=$2,failed_attempts=0,locked_until=NULL,password_changed_at=now(),updated_at=now() WHERE user_id=$1`, a.UserID, newHash); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `UPDATE profiles SET auth_version=auth_version+1,updated_at=now() WHERE user_id=$1`, a.UserID); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `UPDATE auth_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, a.UserID); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id) VALUES($1,NULLIF($2,'')::uuid,'password.changed','user',$1::text)`, a.UserID, a.OrgID); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	webx.JSON(w, http.StatusOK, map[string]any{"ok": true, "reauth_required": true})
	return nil
}
