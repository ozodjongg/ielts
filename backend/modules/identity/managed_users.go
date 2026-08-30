package identity

import (
	"net/http"
	"strings"

	"github.com/example/ielts-platform/internal/passwordhash"
	"github.com/example/ielts-platform/internal/webx"
	"github.com/google/uuid"
)

func (s *Service) internalListUsers(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "tenant"); err != nil {
		return err
	}
	orgID := strings.TrimSpace(r.URL.Query().Get("organization_id"))
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	if _, err := uuid.Parse(orgID); err != nil {
		return webx.E(400, "organization", "valid organization_id required")
	}
	if role != "teacher" && role != "student" {
		return webx.E(400, "role", "role must be teacher or student")
	}
	rows, err := s.DB.Query(r.Context(), `SELECT user_id,organization_id,role,email,full_name,status,current_level,locale FROM profiles WHERE organization_id=$1 AND role=$2 ORDER BY full_name`, orgID, role)
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

func (s *Service) internalUpdateManagedUser(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "tenant"); err != nil {
		return err
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		return webx.E(400, "user", "invalid user id")
	}
	var x struct {
		OrganizationID string  `json:"organization_id"`
		ExpectedRole   string  `json:"expected_role"`
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
	if x.ExpectedRole != "teacher" && x.ExpectedRole != "student" {
		return webx.E(400, "role", "managed role must be teacher or student")
	}
	p, err := s.get(r.Context(), id)
	if err != nil {
		return err
	}
	if p.OrganizationID == nil || *p.OrganizationID != x.OrganizationID || p.Role != x.ExpectedRole {
		return webx.E(404, "user", "managed user not found")
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
		if x.ExpectedRole != "student" {
			return webx.E(400, "level", "current level is only valid for students")
		}
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
		hash, e := passwordhash.Hash(*x.NewPassword)
		if e != nil {
			return webx.E(400, "new_password", e.Error())
		}
		tx, e := s.DB.Begin(r.Context())
		if e != nil {
			return e
		}
		defer tx.Rollback(r.Context())
		if _, e = tx.Exec(r.Context(), `UPDATE auth_credentials SET password_hash=$2,failed_attempts=0,locked_until=NULL,password_changed_at=now(),updated_at=now() WHERE user_id=$1`, id, hash); e != nil {
			return e
		}
		if _, e = tx.Exec(r.Context(), `UPDATE profiles SET auth_version=auth_version+1,updated_at=now() WHERE user_id=$1`, id); e != nil {
			return e
		}
		if _, e = tx.Exec(r.Context(), `UPDATE auth_sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id); e != nil {
			return e
		}
		if e = tx.Commit(r.Context()); e != nil {
			return e
		}
	}
	p, err = s.get(r.Context(), id)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, p)
	return nil
}
