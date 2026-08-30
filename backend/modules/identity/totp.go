package identity

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/example/ielts-platform/internal/webx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const recoveryAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func validMFARole(role string) bool {
	return role == "admin" || role == "center" || role == "teacher"
}

func requireMFARole(role string) error {
	if !validMFARole(role) {
		return webx.E(http.StatusForbidden, "mfa_not_available", "MFA is available only for admin, center and teacher accounts")
	}
	return nil
}

func (s *Service) mfaKey() []byte {
	h := sha256.Sum256([]byte(s.MFAEncryptionKey))
	return h[:]
}

func (s *Service) encryptMFASecret(secret string) ([]byte, error) {
	block, err := aes.NewCipher(s.mfaKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(secret), nil), nil
}

func (s *Service) decryptMFASecret(raw []byte) (string, error) {
	block, err := aes.NewCipher(s.mfaKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted MFA secret")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func newTOTPSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

func totpCode(secret string, unix int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	counter := uint64(unix / 30)
	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(msg)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binaryCode := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", binaryCode%1000000), nil
}

func verifyTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(strings.ReplaceAll(code, " ", ""))
	if len(code) != 6 {
		return false
	}
	for step := int64(-1); step <= 1; step++ {
		expected, err := totpCode(secret, now.Unix()+step*30)
		if err == nil && hmac.Equal([]byte(expected), []byte(code)) {
			return true
		}
	}
	return false
}

func newRecoveryCode() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, 12)
	for i := range out {
		out[i] = recoveryAlphabet[int(buf[i])%len(recoveryAlphabet)]
	}
	return string(out[:4]) + "-" + string(out[4:8]) + "-" + string(out[8:]), nil
}

func recoveryHash(userID, code string) []byte {
	norm := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	h := sha256.Sum256([]byte(userID + ":" + norm))
	return h[:]
}

func (s *Service) mfaStatus(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireMFARole(a.Role); err != nil {
		return err
	}
	var enabled bool
	var verified *time.Time
	err = s.DB.QueryRow(r.Context(), `SELECT enabled,verified_at FROM mfa_totp WHERE user_id=$1`, a.UserID).Scan(&enabled, &verified)
	if err == pgx.ErrNoRows {
		webx.JSON(w, 200, map[string]any{"enabled": false, "aal": a.AAL})
		return nil
	}
	if err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"enabled": enabled, "verified_at": verified, "aal": a.AAL})
	return nil
}

func (s *Service) mfaSetup(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireMFARole(a.Role); err != nil {
		return err
	}
	if strings.TrimSpace(s.MFAEncryptionKey) == "" {
		return webx.E(503, "mfa_unavailable", "MFA encryption key is not configured")
	}
	var alreadyEnabled bool
	err = s.DB.QueryRow(r.Context(), `SELECT enabled FROM mfa_totp WHERE user_id=$1`, a.UserID).Scan(&alreadyEnabled)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if alreadyEnabled {
		return webx.E(409, "mfa_already_enabled", "disable the current authenticator before starting a new setup")
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return err
	}
	ciphertext, err := s.encryptMFASecret(secret)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO mfa_totp(user_id,secret_ciphertext,enabled,verified_at,updated_at) VALUES($1,$2,false,NULL,now()) ON CONFLICT(user_id) DO UPDATE SET secret_ciphertext=excluded.secret_ciphertext,enabled=false,verified_at=NULL,updated_at=now()`, a.UserID, ciphertext)
	if err != nil {
		return err
	}
	p, err := s.get(r.Context(), a.UserID)
	if err != nil {
		return err
	}
	issuer := url.QueryEscape("IELTS Platform")
	label := url.QueryEscape("IELTS Platform:" + p.Email)
	uri := "otpauth://totp/" + label + "?secret=" + url.QueryEscape(secret) + "&issuer=" + issuer + "&algorithm=SHA1&digits=6&period=30"
	webx.JSON(w, 200, map[string]any{"secret": secret, "otpauth_uri": uri, "issuer": "IELTS Platform", "account": p.Email})
	return nil
}

func (s *Service) mfaSetupVerify(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireMFARole(a.Role); err != nil {
		return err
	}
	var x struct {
		Code string `json:"code"`
	}
	if err := webx.Decode(r, &x, 32<<10); err != nil {
		return err
	}
	var ciphertext []byte
	if err := s.DB.QueryRow(r.Context(), `SELECT secret_ciphertext FROM mfa_totp WHERE user_id=$1 AND enabled=false`, a.UserID).Scan(&ciphertext); err == pgx.ErrNoRows {
		return webx.E(409, "mfa_setup_missing", "start MFA setup first")
	} else if err != nil {
		return err
	}
	secret, err := s.decryptMFASecret(ciphertext)
	if err != nil {
		return err
	}
	if !verifyTOTP(secret, x.Code, time.Now().UTC()) {
		return webx.E(400, "invalid_totp", "verification code is invalid")
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `UPDATE mfa_totp SET enabled=true,verified_at=now(),updated_at=now() WHERE user_id=$1`, a.UserID); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM mfa_recovery_codes WHERE user_id=$1`, a.UserID); err != nil {
		return err
	}
	codes := make([]string, 10)
	for i := range codes {
		code, e := newRecoveryCode()
		if e != nil {
			return e
		}
		codes[i] = code
		if _, e = tx.Exec(r.Context(), `INSERT INTO mfa_recovery_codes(user_id,code_hash) VALUES($1,$2)`, a.UserID, recoveryHash(a.UserID, code)); e != nil {
			return e
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE auth_sessions SET aal='aal2',last_used_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, a.SessionID, a.UserID); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	p, err := s.get(r.Context(), a.UserID)
	if err != nil {
		return err
	}
	var authVersion int
	if err = s.DB.QueryRow(r.Context(), `SELECT auth_version FROM profiles WHERE user_id=$1`, a.UserID).Scan(&authVersion); err != nil {
		return err
	}
	access, exp, err := s.Signer.AccessToken(p.UserID, p.Email, a.SessionID, "aal2", authVersion)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"enabled": true, "aal": "aal2", "access_token": access, "expires_at": exp.UTC().Format(time.RFC3339), "recovery_codes": codes})
	return nil
}

func (s *Service) createMFAChallenge(ctx context.Context, userID, role string) (string, error) {
	if !validMFARole(role) {
		return "", webx.E(http.StatusForbidden, "mfa_not_available", "MFA is not available for this role")
	}
	id := uuid.NewString()
	_, err := s.DB.Exec(ctx, `INSERT INTO mfa_challenges(id,user_id,expected_role,expires_at) VALUES($1,$2,$3,now()+interval '5 minutes')`, id, userID, role)
	return id, err
}

func (s *Service) verifyRecoveryCode(ctx context.Context, userID, code string) (bool, error) {
	h := recoveryHash(userID, code)
	var id uuid.UUID
	err := s.DB.QueryRow(ctx, `SELECT id FROM mfa_recovery_codes WHERE user_id=$1 AND code_hash=$2 AND used_at IS NULL ORDER BY created_at LIMIT 1`, userID, h).Scan(&id)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	ct, err := s.DB.Exec(ctx, `UPDATE mfa_recovery_codes SET used_at=now() WHERE id=$1 AND used_at IS NULL`, id)
	return err == nil && ct.RowsAffected() == 1, err
}

func (s *Service) internalMFAVerify(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "gateway"); err != nil {
		return err
	}
	var x struct {
		ChallengeID  string `json:"challenge_id"`
		Code         string `json:"code"`
		ExpectedRole string `json:"expected_role"`
		UserAgent    string `json:"user_agent,omitempty"`
		IPAddress    string `json:"ip_address,omitempty"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if !validMFARole(x.ExpectedRole) {
		return webx.E(401, "invalid_mfa", "MFA challenge is invalid or expired")
	}
	if _, err := uuid.Parse(x.ChallengeID); err != nil {
		return webx.E(401, "invalid_mfa", "MFA challenge is invalid or expired")
	}
	var p Profile
	var org *uuid.UUID
	var ciphertext []byte
	var authVersion, attempts int
	err := s.DB.QueryRow(r.Context(), `SELECT p.user_id,p.organization_id,p.role,p.email,p.full_name,p.status,p.current_level,p.locale,p.auth_version,m.secret_ciphertext,c.attempts FROM mfa_challenges c JOIN profiles p ON p.user_id=c.user_id JOIN mfa_totp m ON m.user_id=p.user_id AND m.enabled=true WHERE c.id=$1 AND c.expected_role=$2 AND c.consumed_at IS NULL AND c.expires_at>now()`, x.ChallengeID, x.ExpectedRole).Scan(&p.UserID, &org, &p.Role, &p.Email, &p.FullName, &p.Status, &p.CurrentLevel, &p.Locale, &authVersion, &ciphertext, &attempts)
	if err == pgx.ErrNoRows {
		return webx.E(401, "invalid_mfa", "MFA challenge is invalid or expired")
	}
	if err != nil {
		return err
	}
	if p.Status != "active" {
		return webx.E(403, "account_inactive", "account is not active")
	}
	if attempts >= 5 {
		return webx.E(429, "mfa_locked", "too many invalid MFA attempts")
	}
	secret, err := s.decryptMFASecret(ciphertext)
	if err != nil {
		return err
	}
	valid := verifyTOTP(secret, x.Code, time.Now().UTC())
	if !valid {
		valid, err = s.verifyRecoveryCode(r.Context(), p.UserID, x.Code)
		if err != nil {
			return err
		}
	}
	if !valid {
		_, _ = s.DB.Exec(r.Context(), `UPDATE mfa_challenges SET attempts=attempts+1 WHERE id=$1`, x.ChallengeID)
		return webx.E(401, "invalid_mfa_code", "verification code is invalid")
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE mfa_challenges SET consumed_at=now() WHERE id=$1 AND consumed_at IS NULL`, x.ChallengeID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() != 1 {
		return webx.E(401, "invalid_mfa", "MFA challenge is invalid or expired")
	}
	if org != nil {
		o := org.String()
		p.OrganizationID = &o
	}
	sessionID := uuid.NewString()
	out, err := s.issueSession(r.Context(), p, authVersion, sessionID, "aal2", false, "", x.UserAgent, normalizeIP(x.IPAddress))
	if err != nil {
		return err
	}
	webx.JSON(w, 200, out)
	return nil
}

func (s *Service) mfaDisable(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireMFARole(a.Role); err != nil {
		return err
	}
	if a.AAL != "aal2" {
		return webx.E(403, "mfa_required", "AAL2 is required to disable MFA")
	}
	var x struct {
		Code string `json:"code"`
	}
	if err := webx.Decode(r, &x, 32<<10); err != nil {
		return err
	}
	var ciphertext []byte
	if err := s.DB.QueryRow(r.Context(), `SELECT secret_ciphertext FROM mfa_totp WHERE user_id=$1 AND enabled=true`, a.UserID).Scan(&ciphertext); err == pgx.ErrNoRows {
		return webx.E(404, "mfa_missing", "MFA is not enabled")
	} else if err != nil {
		return err
	}
	secret, err := s.decryptMFASecret(ciphertext)
	if err != nil {
		return err
	}
	if !verifyTOTP(secret, x.Code, time.Now().UTC()) {
		return webx.E(400, "invalid_totp", "verification code is invalid")
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	_, _ = tx.Exec(r.Context(), `DELETE FROM mfa_totp WHERE user_id=$1`, a.UserID)
	_, _ = tx.Exec(r.Context(), `DELETE FROM mfa_recovery_codes WHERE user_id=$1`, a.UserID)
	_, _ = tx.Exec(r.Context(), `UPDATE auth_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, a.UserID)
	_, _ = tx.Exec(r.Context(), `UPDATE profiles SET auth_version=auth_version+1,updated_at=now() WHERE user_id=$1`, a.UserID)
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"enabled": false, "reauthenticate": true})
	return nil
}
