package identity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/ielts-platform/internal/auth"
	"github.com/example/ielts-platform/internal/authz"
	"github.com/example/ielts-platform/internal/passwordhash"
	"github.com/example/ielts-platform/internal/webx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB               *pgxpool.Pool
	Signer           *auth.Signer
	RefreshTTL       time.Duration
	InternalSecret   string
	MFAEncryptionKey string
}
type Profile struct {
	UserID         string  `json:"user_id"`
	OrganizationID *string `json:"organization_id"`
	Role           string  `json:"role"`
	Email          string  `json:"email"`
	FullName       string  `json:"full_name"`
	Status         string  `json:"status"`
	CurrentLevel   *string `json:"current_level"`
	Locale         string  `json:"locale"`
}

func (s *Service) Router() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		webx.JSON(w, 200, map[string]any{"status": "ok", "service": "identity"})
	})
	m.HandleFunc("GET /internal/resolve", webx.Handle(s.resolve))
	m.HandleFunc("POST /internal/auth/login", webx.Handle(s.internalLogin))
	m.HandleFunc("POST /internal/auth/refresh", webx.Handle(s.internalRefresh))
	m.HandleFunc("POST /internal/auth/logout", webx.Handle(s.internalLogout))
	m.HandleFunc("POST /internal/auth/mfa-verify", webx.Handle(s.internalMFAVerify))
	m.HandleFunc("POST /internal/users", webx.Handle(s.internalCreateUser))
	m.HandleFunc("GET /internal/students", webx.Handle(s.internalListStudents))
	m.HandleFunc("GET /internal/users", webx.Handle(s.internalListUsers))
	m.HandleFunc("PATCH /internal/managed-users/{id}", webx.Handle(s.internalUpdateManagedUser))
	m.HandleFunc("PATCH /internal/users/{id}", webx.Handle(s.internalUpdateStudent))
	m.HandleFunc("DELETE /internal/users/{id}", webx.Handle(s.internalDeleteUser))
	m.HandleFunc("PATCH /internal/users/{id}/level", webx.Handle(s.internalSetLevel))
	m.HandleFunc("GET /v1/me", webx.Handle(s.me))
	m.HandleFunc("PATCH /v1/me", webx.Handle(s.updateMe))
	m.HandleFunc("PATCH /v1/me/password", webx.Handle(s.changePassword))
	m.HandleFunc("GET /v1/mfa/status", webx.Handle(s.mfaStatus))
	m.HandleFunc("POST /v1/mfa/setup", webx.Handle(s.mfaSetup))
	m.HandleFunc("POST /v1/mfa/verify", webx.Handle(s.mfaSetupVerify))
	m.HandleFunc("POST /v1/mfa/disable", webx.Handle(s.mfaDisable))
	return webx.Security(m)
}
func (s *Service) actor(r *http.Request) (authz.Actor, error) {
	a, err := authz.Verify(r, s.InternalSecret)
	if err != nil {
		return a, webx.E(401, "internal_auth", "invalid gateway signature")
	}
	return a, nil
}
func (s *Service) serviceAuth(r *http.Request, allowed ...string) error {
	if err := authz.VerifyService(r, s.InternalSecret, allowed...); err != nil {
		return webx.E(401, "service_auth", "invalid service signature")
	}
	return nil
}
func (s *Service) get(ctx context.Context, id string) (Profile, error) {
	var p Profile
	var org *uuid.UUID
	err := s.DB.QueryRow(ctx, `SELECT user_id,organization_id,role,email,full_name,status,current_level,locale FROM profiles WHERE user_id=$1`, id).Scan(&p.UserID, &org, &p.Role, &p.Email, &p.FullName, &p.Status, &p.CurrentLevel, &p.Locale)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, webx.E(403, "profile_missing", "user profile is not provisioned")
	}
	if err != nil {
		return p, err
	}
	if org != nil {
		x := org.String()
		p.OrganizationID = &x
	}
	return p, nil
}
func (s *Service) resolve(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "gateway", "tenant", "assessment", "vocabulary", "listening", "sat"); err != nil {
		return err
	}
	id := r.URL.Query().Get("user_id")
	if _, err := uuid.Parse(id); err != nil {
		return webx.E(400, "user_id", "invalid user id")
	}
	p, err := s.get(r.Context(), id)
	if err != nil {
		return err
	}
	if p.Status != "active" {
		return webx.E(403, "profile_inactive", "profile is not active")
	}
	if rawVersion := strings.TrimSpace(r.URL.Query().Get("auth_version")); rawVersion != "" {
		version, err := strconv.Atoi(rawVersion)
		if err != nil || version < 1 {
			return webx.E(401, "session_invalid", "invalid session")
		}
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if _, err := uuid.Parse(sessionID); err != nil {
			return webx.E(401, "session_invalid", "invalid session")
		}
		var currentVersion int
		var validSession bool
		err = s.DB.QueryRow(r.Context(), `SELECT p.auth_version, EXISTS(SELECT 1 FROM auth_sessions s WHERE s.id=$2 AND s.user_id=p.user_id AND s.revoked_at IS NULL AND s.expires_at>now()) FROM profiles p WHERE p.user_id=$1`, id, sessionID).Scan(&currentVersion, &validSession)
		if err != nil || currentVersion != version || !validSession {
			return webx.E(401, "session_invalid", "session has expired or been revoked")
		}
	}
	webx.JSON(w, 200, p)
	return nil
}

type CreateUserRequest struct {
	OrganizationID *string `json:"organization_id"`
	Role           string  `json:"role"`
	Email          string  `json:"email"`
	Password       string  `json:"password"`
	FullName       string  `json:"full_name"`
	CurrentLevel   *string `json:"current_level"`
}

func validateCreate(x CreateUserRequest) error {
	if x.Role != "admin" && x.Role != "center" && x.Role != "teacher" && x.Role != "student" {
		return webx.E(400, "role", "invalid role")
	}
	if !strings.Contains(x.Email, "@") || len(x.Email) > 254 {
		return webx.E(400, "email", "valid email required")
	}
	if len(x.Password) < 10 || len(x.Password) > 128 {
		return webx.E(400, "password", "password must be 10-128 characters")
	}
	if strings.TrimSpace(x.FullName) == "" || len(x.FullName) > 120 {
		return webx.E(400, "full_name", "full name required")
	}
	if x.Role == "admin" && x.OrganizationID != nil {
		return webx.E(400, "organization", "platform admin cannot belong to a center")
	}
	if x.Role != "admin" && x.OrganizationID == nil {
		return webx.E(400, "organization", "organization required")
	}
	return nil
}
func (s *Service) createUser(ctx context.Context, x CreateUserRequest) (Profile, error) {
	if err := validateCreate(x); err != nil {
		return Profile{}, err
	}
	email := strings.ToLower(strings.TrimSpace(x.Email))
	passwordHash, err := passwordhash.Hash(x.Password)
	if err != nil {
		return Profile{}, webx.E(400, "password", err.Error())
	}
	userID := uuid.New()
	var org any = nil
	if x.OrganizationID != nil {
		oid, err := uuid.Parse(*x.OrganizationID)
		if err != nil {
			return Profile{}, webx.E(400, "organization", "invalid organization id")
		}
		org = oid
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO profiles(user_id,organization_id,role,email,full_name,current_level) VALUES($1,$2,$3,$4,$5,$6)`, userID, org, x.Role, email, strings.TrimSpace(x.FullName), x.CurrentLevel); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "profiles_email_lower_uq") || strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			return Profile{}, webx.E(409, "email_exists", "an account with this email already exists")
		}
		return Profile{}, fmt.Errorf("insert profile: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth_credentials(user_id,password_hash) VALUES($1,$2)`, userID, passwordHash); err != nil {
		return Profile{}, fmt.Errorf("insert credentials: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Profile{}, err
	}
	return s.get(ctx, userID.String())
}
func (s *Service) internalCreateUser(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "tenant"); err != nil {
		return err
	}
	var x CreateUserRequest
	if err := webx.Decode(r, &x, 1<<20); err != nil {
		return err
	}
	p, err := s.createUser(r.Context(), x)
	if err != nil {
		return err
	}
	webx.JSON(w, 201, p)
	return nil
}
func (s *Service) internalDeleteUser(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "tenant"); err != nil {
		return err
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		return webx.E(400, "id", "invalid user id")
	}
	ct, err := s.DB.Exec(r.Context(), `DELETE FROM profiles WHERE user_id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return webx.E(404, "user", "user not found")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Service) internalSetLevel(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "assessment"); err != nil {
		return err
	}
	var x struct {
		Level          string `json:"level"`
		OrganizationID string `json:"organization_id"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if x.Level != "A1" && x.Level != "A2" && x.Level != "B1" && x.Level != "B2" && x.Level != "C1" && x.Level != "C2" {
		return webx.E(400, "level", "invalid level")
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE profiles SET current_level=$3,updated_at=now() WHERE user_id=$1 AND organization_id=$2 AND role='student'`, r.PathValue("id"), x.OrganizationID, x.Level)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return webx.E(404, "student", "student not found")
	}
	webx.JSON(w, 200, map[string]any{"ok": true})
	return nil
}
func (s *Service) me(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	p, err := s.get(r.Context(), a.UserID)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, p)
	return nil
}
func (s *Service) updateMe(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	var x struct {
		FullName *string `json:"full_name"`
		Locale   *string `json:"locale"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if x.FullName != nil {
		v := strings.TrimSpace(*x.FullName)
		if v == "" || len(v) > 120 {
			return webx.E(400, "full_name", "invalid full name")
		}
		_, err = s.DB.Exec(r.Context(), `UPDATE profiles SET full_name=$2,updated_at=now() WHERE user_id=$1`, a.UserID, v)
		if err != nil {
			return err
		}
	}
	if x.Locale != nil {
		v := strings.TrimSpace(*x.Locale)
		if v != "uz" && v != "en" && v != "ru" {
			return webx.E(400, "locale", "unsupported locale")
		}
		_, err = s.DB.Exec(r.Context(), `UPDATE profiles SET locale=$2,updated_at=now() WHERE user_id=$1`, a.UserID, v)
		if err != nil {
			return err
		}
	}
	p, err := s.get(r.Context(), a.UserID)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, p)
	return nil
}
func (s *Service) internalListStudents(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "tenant"); err != nil {
		return err
	}
	orgID := strings.TrimSpace(r.URL.Query().Get("organization_id"))
	if _, err := uuid.Parse(orgID); err != nil {
		return webx.E(400, "organization", "valid organization_id required")
	}
	rows, err := s.DB.Query(r.Context(), `SELECT user_id,organization_id,role,email,full_name,status,current_level,locale FROM profiles WHERE organization_id=$1 AND role='student' ORDER BY full_name`, orgID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []Profile{}
	for rows.Next() {
		var p Profile
		var org uuid.UUID
		if err := rows.Scan(&p.UserID, &org, &p.Role, &p.Email, &p.FullName, &p.Status, &p.CurrentLevel, &p.Locale); err != nil {
			return err
		}
		o := org.String()
		p.OrganizationID = &o
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"items": out})
	return nil
}

func (s *Service) internalUpdateStudent(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "tenant"); err != nil {
		return err
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		return webx.E(400, "student", "invalid student id")
	}
	var x struct {
		OrganizationID string  `json:"organization_id"`
		Status         *string `json:"status"`
		CurrentLevel   *string `json:"current_level"`
		FullName       *string `json:"full_name"`
		NewPassword    *string `json:"new_password"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if _, err := uuid.Parse(x.OrganizationID); err != nil {
		return webx.E(400, "organization", "valid organization_id required")
	}
	p, err := s.get(r.Context(), id)
	if err != nil {
		return err
	}
	if p.Role != "student" || p.OrganizationID == nil || *p.OrganizationID != x.OrganizationID {
		return webx.E(404, "student", "student not found")
	}
	if x.Status != nil {
		if *x.Status != "active" && *x.Status != "suspended" && *x.Status != "archived" {
			return webx.E(400, "status", "invalid status")
		}
		if _, err = s.DB.Exec(r.Context(), `UPDATE profiles SET status=$2,auth_version=auth_version+1,updated_at=now() WHERE user_id=$1`, id, *x.Status); err != nil {
			return err
		}
		_, _ = s.DB.Exec(r.Context(), `UPDATE auth_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id)
	}
	if x.CurrentLevel != nil {
		lvl := *x.CurrentLevel
		if lvl != "A1" && lvl != "A2" && lvl != "B1" && lvl != "B2" && lvl != "C1" && lvl != "C2" {
			return webx.E(400, "level", "invalid level")
		}
		if _, err = s.DB.Exec(r.Context(), `UPDATE profiles SET current_level=$2,updated_at=now() WHERE user_id=$1`, id, lvl); err != nil {
			return err
		}
	}
	if x.FullName != nil {
		v := strings.TrimSpace(*x.FullName)
		if v == "" || len(v) > 120 {
			return webx.E(400, "full_name", "valid full name required")
		}
		if _, err = s.DB.Exec(r.Context(), `UPDATE profiles SET full_name=$2,updated_at=now() WHERE user_id=$1`, id, v); err != nil {
			return err
		}
	}
	if x.NewPassword != nil {
		if len(*x.NewPassword) < 10 || len(*x.NewPassword) > 128 {
			return webx.E(400, "new_password", "new password must be 10-128 characters")
		}
		hash, hashErr := passwordhash.Hash(*x.NewPassword)
		if hashErr != nil {
			return webx.E(400, "new_password", hashErr.Error())
		}
		tx, txErr := s.DB.Begin(r.Context())
		if txErr != nil {
			return txErr
		}
		defer tx.Rollback(r.Context())
		if _, txErr = tx.Exec(r.Context(), `UPDATE auth_credentials SET password_hash=$2,failed_attempts=0,locked_until=NULL,password_changed_at=now(),updated_at=now() WHERE user_id=$1`, id, hash); txErr != nil {
			return txErr
		}
		if _, txErr = tx.Exec(r.Context(), `UPDATE profiles SET auth_version=auth_version+1,updated_at=now() WHERE user_id=$1`, id); txErr != nil {
			return txErr
		}
		if _, txErr = tx.Exec(r.Context(), `UPDATE auth_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id); txErr != nil {
			return txErr
		}
		if txErr = tx.Commit(r.Context()); txErr != nil {
			return txErr
		}
	}
	p, err = s.get(r.Context(), id)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, p)
	return nil
}

func (s *Service) listStudents(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err = authz.Require(a, "center"); err != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	rows, err := s.DB.Query(r.Context(), `SELECT user_id,organization_id,role,email,full_name,status,current_level,locale FROM profiles WHERE organization_id=$1 AND role='student' ORDER BY full_name`, a.OrgID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []Profile{}
	for rows.Next() {
		var p Profile
		var org uuid.UUID
		if err := rows.Scan(&p.UserID, &org, &p.Role, &p.Email, &p.FullName, &p.Status, &p.CurrentLevel, &p.Locale); err != nil {
			return err
		}
		os := org.String()
		p.OrganizationID = &os
		out = append(out, p)
	}
	webx.JSON(w, 200, map[string]any{"items": out})
	return rows.Err()
}
func (s *Service) createStudent(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err = authz.Require(a, "center"); err != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	var x CreateUserRequest
	if err := webx.Decode(r, &x, 1<<20); err != nil {
		return err
	}
	x.Role = "student"
	x.OrganizationID = &a.OrgID
	p, err := s.createUser(r.Context(), x)
	if err != nil {
		return err
	}
	_, _ = s.DB.Exec(r.Context(), `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id) VALUES($1,$2,'student.created','student',$3)`, a.UserID, a.OrgID, p.UserID)
	webx.JSON(w, 201, p)
	return nil
}
func (s *Service) updateStudent(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err = authz.Require(a, "center"); err != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	id := r.PathValue("id")
	var x struct {
		Status       *string `json:"status"`
		CurrentLevel *string `json:"current_level"`
		FullName     *string `json:"full_name"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	p, err := s.get(r.Context(), id)
	if err != nil {
		return err
	}
	if p.OrganizationID == nil || *p.OrganizationID != a.OrgID || p.Role != "student" {
		return webx.E(404, "student", "student not found")
	}
	if x.Status != nil {
		if *x.Status != "active" && *x.Status != "suspended" && *x.Status != "archived" {
			return webx.E(400, "status", "invalid status")
		}
		_, err = s.DB.Exec(r.Context(), `UPDATE profiles SET status=$2,auth_version=auth_version+1,updated_at=now() WHERE user_id=$1`, id, *x.Status)
		if err != nil {
			return err
		}
		_, _ = s.DB.Exec(r.Context(), `UPDATE auth_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id)
	}
	if x.CurrentLevel != nil {
		lvl := *x.CurrentLevel
		if lvl != "A1" && lvl != "A2" && lvl != "B1" && lvl != "B2" && lvl != "C1" && lvl != "C2" {
			return webx.E(400, "level", "invalid level")
		}
		_, err = s.DB.Exec(r.Context(), `UPDATE profiles SET current_level=$2,updated_at=now() WHERE user_id=$1`, id, lvl)
		if err != nil {
			return err
		}
	}
	if x.FullName != nil {
		v := strings.TrimSpace(*x.FullName)
		if v == "" {
			return webx.E(400, "full_name", "full name required")
		}
		_, err = s.DB.Exec(r.Context(), `UPDATE profiles SET full_name=$2,updated_at=now() WHERE user_id=$1`, id, v)
		if err != nil {
			return err
		}
	}
	p, err = s.get(r.Context(), id)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, p)
	return nil
}
