package tenant

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/example/assessment-platform-v5/internal/authz"
	"github.com/example/assessment-platform-v5/internal/clientx"
	"github.com/example/assessment-platform-v5/internal/webx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB             *pgxpool.Pool
	Identity       *clientx.Client
	InternalSecret string
}
type Center struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Slug               string    `json:"slug"`
	Status             string    `json:"status"`
	SubscriptionStatus string    `json:"subscription_status"`
	TrialEndsAt        time.Time `json:"trial_ends_at"`
	Timezone           string    `json:"timezone"`
	ActiveStudentLimit int       `json:"active_student_limit"`
	CreatedAt          time.Time `json:"created_at"`
}
type Limit struct {
	ServiceCode      string `json:"service_code"`
	Name             string `json:"name"`
	Category         string `json:"category"`
	Unit             string `json:"unit"`
	Enabled          bool   `json:"enabled"`
	MonthlyLimit     int64  `json:"monthly_limit"`
	DailyLimit       *int64 `json:"daily_limit"`
	ConcurrencyLimit int    `json:"concurrency_limit"`
	Used             int64  `json:"used"`
	Remaining        int64  `json:"remaining"`
}
type Group struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Level       *string `json:"level"`
	TeacherName *string `json:"teacher_name"`
	Status      string  `json:"status"`
	MemberCount int     `json:"member_count"`
}
type Student struct {
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
		webx.JSON(w, 200, map[string]any{"status": "ok", "service": "tenant"})
	})
	m.HandleFunc("GET /v1/centers", webx.Handle(s.listCenters))
	m.HandleFunc("POST /v1/centers", webx.Handle(s.createCenter))
	m.HandleFunc("PATCH /v1/centers/{id}", webx.Handle(s.updateCenter))
	m.HandleFunc("GET /v1/services", webx.Handle(s.services))
	m.HandleFunc("GET /v1/centers/{id}/services", webx.Handle(s.centerServices))
	m.HandleFunc("PATCH /v1/centers/{id}/services/{code}", webx.Handle(s.updateLimit))
	m.HandleFunc("GET /v1/students", webx.Handle(s.students))
	m.HandleFunc("POST /v1/students", webx.Handle(s.createStudent))
	m.HandleFunc("PATCH /v1/students/{id}", webx.Handle(s.updateStudent))
	m.HandleFunc("GET /v1/groups", webx.Handle(s.groups))
	m.HandleFunc("POST /v1/groups", webx.Handle(s.createGroup))
	m.HandleFunc("DELETE /v1/groups/{id}", webx.Handle(s.archiveGroup))
	m.HandleFunc("GET /v1/groups/{id}/students", webx.Handle(s.groupStudents))
	m.HandleFunc("POST /v1/groups/{id}/students", webx.Handle(s.addGroupStudent))
	m.HandleFunc("DELETE /v1/groups/{id}/students/{studentID}", webx.Handle(s.removeGroupStudent))
	m.HandleFunc("GET /internal/student/{id}/groups", webx.Handle(s.internalStudentGroups))
	m.HandleFunc("POST /internal/target/validate", webx.Handle(s.internalValidateTarget))
	m.HandleFunc("POST /internal/usage/reserve", webx.Handle(s.reserve))
	m.HandleFunc("POST /internal/usage/release", webx.Handle(s.releaseUsage))
	m.HandleFunc("POST /internal/usage/cancel", webx.Handle(s.cancelUsage))
	m.HandleFunc("GET /internal/organization/{id}", webx.Handle(s.internalOrganization))
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
func scanCenter(row pgx.Row) (Center, error) {
	var c Center
	var id uuid.UUID
	err := row.Scan(&id, &c.Name, &c.Slug, &c.Status, &c.SubscriptionStatus, &c.TrialEndsAt, &c.Timezone, &c.ActiveStudentLimit, &c.CreatedAt)
	c.ID = id.String()
	return c, err
}
func (s *Service) listCenters(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "platform_admin") != nil {
		return webx.E(403, "forbidden", "platform admin required")
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,name,slug,status,subscription_status,trial_ends_at,timezone,active_student_limit,created_at FROM organizations ORDER BY created_at DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []Center{}
	for rows.Next() {
		c, err := scanCenter(rows)
		if err != nil {
			return err
		}
		out = append(out, c)
	}
	webx.JSON(w, 200, map[string]any{"items": out})
	return rows.Err()
}

var slugRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type CreateCenter struct {
	Name               string           `json:"name"`
	Slug               string           `json:"slug"`
	AdminName          string           `json:"admin_name"`
	AdminEmail         string           `json:"admin_email"`
	AdminPassword      string           `json:"admin_password"`
	ActiveStudentLimit int              `json:"active_student_limit"`
	ServiceLimits      map[string]int64 `json:"service_limits"`
}

func (s *Service) createCenter(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "platform_admin") != nil {
		return webx.E(403, "forbidden", "platform admin required")
	}
	var x CreateCenter
	if err := webx.Decode(r, &x, 1<<20); err != nil {
		return err
	}
	x.Name = strings.TrimSpace(x.Name)
	x.Slug = strings.ToLower(strings.TrimSpace(x.Slug))
	if x.Name == "" || len(x.Name) > 160 {
		return webx.E(400, "name", "valid center name required")
	}
	if !slugRE.MatchString(x.Slug) || len(x.Slug) > 80 {
		return webx.E(400, "slug", "slug must contain lowercase letters, numbers and hyphens")
	}
	if x.ActiveStudentLimit <= 0 {
		x.ActiveStudentLimit = 100
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var orgID uuid.UUID
	err = tx.QueryRow(r.Context(), `INSERT INTO organizations(name,slug,status,active_student_limit) VALUES($1,$2,'provisioning',$3) RETURNING id`, x.Name, x.Slug, x.ActiveStudentLimit).Scan(&orgID)
	if err != nil {
		return fmt.Errorf("create organization: %w", err)
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO organization_service_limits(organization_id,service_code,monthly_limit,daily_limit) SELECT $1,code,default_monthly_limit,default_daily_limit FROM service_catalog WHERE enabled=true ON CONFLICT DO NOTHING`, orgID)
	if err != nil {
		return err
	}
	for code, lim := range x.ServiceLimits {
		if lim < 0 {
			return webx.E(400, "limit", "service limit cannot be negative")
		}
		ct, err := tx.Exec(r.Context(), `UPDATE organization_service_limits SET monthly_limit=$3,updated_at=now() WHERE organization_id=$1 AND service_code=$2`, orgID, code, lim)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return webx.E(400, "service", "unknown service code "+code)
		}
	}
	req := map[string]any{"organization_id": orgID.String(), "role": "center_admin", "email": x.AdminEmail, "password": x.AdminPassword, "full_name": x.AdminName}
	var created struct {
		UserID string `json:"user_id"`
	}
	if err = s.Identity.Do(r.Context(), "POST", "/internal/users", req, &created); err != nil {
		return fmt.Errorf("provision center administrator: %w", err)
	}
	cleanupUser := true
	defer func() {
		if cleanupUser && created.UserID != "" {
			c, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			_ = s.Identity.Do(c, "DELETE", "/internal/users/"+created.UserID, nil, nil)
		}
	}()
	if _, err = tx.Exec(r.Context(), `UPDATE organizations SET status='active',updated_at=now() WHERE id=$1`, orgID); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return fmt.Errorf("commit center provisioning: %w", err)
	}
	cleanupUser = false
	c, err := scanCenter(s.DB.QueryRow(r.Context(), `SELECT id,name,slug,status,subscription_status,trial_ends_at,timezone,active_student_limit,created_at FROM organizations WHERE id=$1`, orgID))
	if err != nil {
		return err
	}
	webx.JSON(w, 201, map[string]any{"center": c, "admin_user_id": created.UserID})
	return nil
}
func (s *Service) updateCenter(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "platform_admin") != nil {
		return webx.E(403, "forbidden", "platform admin required")
	}
	id := r.PathValue("id")
	var x struct {
		Status             *string `json:"status"`
		SubscriptionStatus *string `json:"subscription_status"`
		ActiveStudentLimit *int    `json:"active_student_limit"`
		Timezone           *string `json:"timezone"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if x.Status != nil {
		if *x.Status != "active" && *x.Status != "suspended" && *x.Status != "archived" {
			return webx.E(400, "status", "invalid status")
		}
		if _, err = s.DB.Exec(r.Context(), `UPDATE organizations SET status=$2,updated_at=now() WHERE id=$1`, id, *x.Status); err != nil {
			return err
		}
	}
	if x.SubscriptionStatus != nil {
		if *x.SubscriptionStatus != "trialing" && *x.SubscriptionStatus != "active" && *x.SubscriptionStatus != "past_due" && *x.SubscriptionStatus != "cancelled" {
			return webx.E(400, "subscription", "invalid subscription status")
		}
		if _, err = s.DB.Exec(r.Context(), `UPDATE organizations SET subscription_status=$2,updated_at=now() WHERE id=$1`, id, *x.SubscriptionStatus); err != nil {
			return err
		}
	}
	if x.ActiveStudentLimit != nil {
		if *x.ActiveStudentLimit < 0 {
			return webx.E(400, "limit", "invalid student limit")
		}
		if _, err = s.DB.Exec(r.Context(), `UPDATE organizations SET active_student_limit=$2,updated_at=now() WHERE id=$1`, id, *x.ActiveStudentLimit); err != nil {
			return err
		}
	}
	if x.Timezone != nil {
		if len(*x.Timezone) > 64 {
			return webx.E(400, "timezone", "invalid timezone")
		}
		if _, err = s.DB.Exec(r.Context(), `UPDATE organizations SET timezone=$2,updated_at=now() WHERE id=$1`, id, *x.Timezone); err != nil {
			return err
		}
	}
	c, err := scanCenter(s.DB.QueryRow(r.Context(), `SELECT id,name,slug,status,subscription_status,trial_ends_at,timezone,active_student_limit,created_at FROM organizations WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(404, "center", "center not found")
	}
	if err != nil {
		return err
	}
	webx.JSON(w, 200, c)
	return nil
}
func (s *Service) limits(ctx context.Context, org string) ([]Limit, error) {
	rows, err := s.DB.Query(ctx, `SELECT c.code,c.name,c.category,c.unit,coalesce(l.enabled,true),coalesce(l.monthly_limit,c.default_monthly_limit),coalesce(l.daily_limit,c.default_daily_limit),coalesce(l.concurrency_limit,10),coalesce(u.used,0) FROM service_catalog c LEFT JOIN organization_service_limits l ON l.service_code=c.code AND l.organization_id=$1 LEFT JOIN usage_monthly u ON u.service_code=c.code AND u.organization_id=$1 AND u.period=date_trunc('month',now())::date WHERE c.enabled=true ORDER BY c.category,c.name`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Limit{}
	for rows.Next() {
		var x Limit
		if err := rows.Scan(&x.ServiceCode, &x.Name, &x.Category, &x.Unit, &x.Enabled, &x.MonthlyLimit, &x.DailyLimit, &x.ConcurrencyLimit, &x.Used); err != nil {
			return nil, err
		}
		x.Remaining = x.MonthlyLimit - x.Used
		if x.Remaining < 0 {
			x.Remaining = 0
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Service) services(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if a.Role == "platform_admin" {
		rows, err := s.DB.Query(r.Context(), `SELECT code,name,unit,category,default_monthly_limit,default_daily_limit,description FROM service_catalog WHERE enabled=true ORDER BY category,name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var code, name, unit, cat, desc string
			var ml int64
			var dl *int64
			if err := rows.Scan(&code, &name, &unit, &cat, &ml, &dl, &desc); err != nil {
				return err
			}
			items = append(items, map[string]any{"service_code": code, "name": name, "unit": unit, "category": cat, "default_monthly_limit": ml, "default_daily_limit": dl, "description": desc})
		}
		webx.JSON(w, 200, map[string]any{"items": items})
		return rows.Err()
	}
	if a.Role != "center_admin" && a.Role != "student" {
		return webx.E(403, "forbidden", "unsupported role")
	}
	ls, err := s.limits(r.Context(), a.OrgID)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"items": ls})
	return nil
}
func (s *Service) centerServices(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "platform_admin") != nil {
		return webx.E(403, "forbidden", "platform admin required")
	}
	org := r.PathValue("id")
	if _, err := uuid.Parse(org); err != nil {
		return webx.E(400, "center", "invalid center id")
	}
	var exists bool
	if err := s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM organizations WHERE id=$1)`, org).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return webx.E(404, "center", "center not found")
	}
	ls, err := s.limits(r.Context(), org)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"items": ls})
	return nil
}
func (s *Service) updateLimit(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "platform_admin") != nil {
		return webx.E(403, "forbidden", "platform admin required")
	}
	org := r.PathValue("id")
	code := r.PathValue("code")
	var x struct {
		Enabled          *bool  `json:"enabled"`
		MonthlyLimit     *int64 `json:"monthly_limit"`
		DailyLimit       *int64 `json:"daily_limit"`
		ClearDailyLimit  bool   `json:"clear_daily_limit"`
		ConcurrencyLimit *int   `json:"concurrency_limit"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO organization_service_limits(organization_id,service_code,monthly_limit,daily_limit) SELECT $1,c.code,c.default_monthly_limit,c.default_daily_limit FROM service_catalog c WHERE c.code=$2 ON CONFLICT DO NOTHING`, org, code)
	if err != nil {
		return err
	}
	if x.Enabled != nil {
		_, err = s.DB.Exec(r.Context(), `UPDATE organization_service_limits SET enabled=$3,updated_at=now() WHERE organization_id=$1 AND service_code=$2`, org, code, *x.Enabled)
		if err != nil {
			return err
		}
	}
	if x.MonthlyLimit != nil {
		if *x.MonthlyLimit < 0 {
			return webx.E(400, "limit", "monthly limit cannot be negative")
		}
		_, err = s.DB.Exec(r.Context(), `UPDATE organization_service_limits SET monthly_limit=$3,updated_at=now() WHERE organization_id=$1 AND service_code=$2`, org, code, *x.MonthlyLimit)
		if err != nil {
			return err
		}
	}
	if x.ClearDailyLimit {
		_, err = s.DB.Exec(r.Context(), `UPDATE organization_service_limits SET daily_limit=NULL,updated_at=now() WHERE organization_id=$1 AND service_code=$2`, org, code)
		if err != nil {
			return err
		}
	} else if x.DailyLimit != nil {
		if *x.DailyLimit < 0 {
			return webx.E(400, "limit", "daily limit cannot be negative")
		}
		_, err = s.DB.Exec(r.Context(), `UPDATE organization_service_limits SET daily_limit=$3,updated_at=now() WHERE organization_id=$1 AND service_code=$2`, org, code, *x.DailyLimit)
		if err != nil {
			return err
		}
	}
	if x.ConcurrencyLimit != nil {
		if *x.ConcurrencyLimit < 1 {
			return webx.E(400, "limit", "concurrency limit must be positive")
		}
		_, err = s.DB.Exec(r.Context(), `UPDATE organization_service_limits SET concurrency_limit=$3,updated_at=now() WHERE organization_id=$1 AND service_code=$2`, org, code, *x.ConcurrencyLimit)
		if err != nil {
			return err
		}
	}
	ls, err := s.limits(r.Context(), org)
	if err != nil {
		return err
	}
	for _, v := range ls {
		if v.ServiceCode == code {
			webx.JSON(w, 200, v)
			return nil
		}
	}
	return webx.E(404, "service", "service not found")
}
func (s *Service) students(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center_admin") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	var out struct {
		Items []Student `json:"items"`
	}
	if err := s.Identity.Do(r.Context(), "GET", "/internal/students?organization_id="+a.OrgID, nil, &out); err != nil {
		return fmt.Errorf("list center students: %w", err)
	}
	webx.JSON(w, 200, out)
	return nil
}

func (s *Service) createStudent(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center_admin") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	var x struct {
		Email        string  `json:"email"`
		Password     string  `json:"password"`
		FullName     string  `json:"full_name"`
		CurrentLevel *string `json:"current_level"`
	}
	if err := webx.Decode(r, &x, 1<<20); err != nil {
		return err
	}
	// Tenant DB owns the commercial center limit. Identity remains the source of
	// truth for user/profile state, so count active students there before provisioning.
	var center Center
	center, err = scanCenter(s.DB.QueryRow(r.Context(), `SELECT id,name,slug,status,subscription_status,trial_ends_at,timezone,active_student_limit,created_at FROM organizations WHERE id=$1`, a.OrgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(404, "center", "learning center not found")
	}
	if err != nil {
		return err
	}
	if center.Status != "active" {
		return webx.E(403, "center_inactive", "learning center is not active")
	}
	if center.SubscriptionStatus != "active" && center.SubscriptionStatus != "trialing" {
		return webx.E(402, "subscription_inactive", "learning center subscription is inactive")
	}
	if center.SubscriptionStatus == "trialing" && time.Now().After(center.TrialEndsAt) {
		return webx.E(402, "trial_expired", "learning center trial has expired")
	}
	var current struct {
		Items []Student `json:"items"`
	}
	if err := s.Identity.Do(r.Context(), "GET", "/internal/students?organization_id="+a.OrgID, nil, &current); err != nil {
		return fmt.Errorf("count active students: %w", err)
	}
	active := 0
	for _, st := range current.Items {
		if st.Status == "active" {
			active++
		}
	}
	if active >= center.ActiveStudentLimit {
		return webx.E(429, "student_limit", "active student limit reached for this learning center")
	}
	req := map[string]any{
		"organization_id": a.OrgID,
		"role":            "student",
		"email":           x.Email,
		"password":        x.Password,
		"full_name":       x.FullName,
		"current_level":   x.CurrentLevel,
	}
	var created Student
	if err := s.Identity.Do(r.Context(), "POST", "/internal/users", req, &created); err != nil {
		return fmt.Errorf("create student account: %w", err)
	}
	webx.JSON(w, 201, created)
	return nil
}

func (s *Service) updateStudent(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center_admin") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	if _, err := uuid.Parse(r.PathValue("id")); err != nil {
		return webx.E(400, "student", "invalid student id")
	}
	var x struct {
		Status       *string `json:"status"`
		CurrentLevel *string `json:"current_level"`
		FullName     *string `json:"full_name"`
		NewPassword  *string `json:"new_password"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	payload := map[string]any{"organization_id": a.OrgID}
	if x.Status != nil {
		payload["status"] = *x.Status
	}
	if x.CurrentLevel != nil {
		payload["current_level"] = *x.CurrentLevel
	}
	if x.FullName != nil {
		payload["full_name"] = *x.FullName
	}
	if x.NewPassword != nil {
		payload["new_password"] = *x.NewPassword
	}
	var updated Student
	if err := s.Identity.Do(r.Context(), "PATCH", "/internal/users/"+r.PathValue("id"), payload, &updated); err != nil {
		return fmt.Errorf("update student account: %w", err)
	}
	webx.JSON(w, 200, updated)
	return nil
}

func (s *Service) groups(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if a.Role != "center_admin" && a.Role != "student" {
		return webx.E(403, "forbidden", "center or student required")
	}
	if a.Role == "student" {
		rows, err := s.DB.Query(r.Context(), `SELECT g.id,g.name,g.level,g.teacher_name,g.status,(SELECT count(*) FROM group_members m WHERE m.group_id=g.id) FROM groups g JOIN group_members gm ON gm.group_id=g.id WHERE gm.organization_id=$1 AND gm.student_user_id=$2 AND g.status='active' ORDER BY g.name`, a.OrgID, a.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		out, err := scanGroups(rows)
		if err != nil {
			return err
		}
		webx.JSON(w, 200, map[string]any{"items": out})
		return nil
	}
	rows, err := s.DB.Query(r.Context(), `SELECT g.id,g.name,g.level,g.teacher_name,g.status,(SELECT count(*) FROM group_members m WHERE m.group_id=g.id) FROM groups g WHERE g.organization_id=$1 AND g.status='active' ORDER BY g.name`, a.OrgID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out, err := scanGroups(rows)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"items": out})
	return nil
}

type rowScanner interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanGroups(rows rowScanner) ([]Group, error) {
	out := []Group{}
	for rows.Next() {
		var g Group
		var id uuid.UUID
		if err := rows.Scan(&id, &g.Name, &g.Level, &g.TeacherName, &g.Status, &g.MemberCount); err != nil {
			return nil, err
		}
		g.ID = id.String()
		out = append(out, g)
	}
	return out, rows.Err()
}
func (s *Service) createGroup(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center_admin") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	var x struct {
		Name        string  `json:"name"`
		Level       *string `json:"level"`
		TeacherName *string `json:"teacher_name"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	x.Name = strings.TrimSpace(x.Name)
	if x.Name == "" || len(x.Name) > 120 {
		return webx.E(400, "name", "group name required")
	}
	if x.Level != nil {
		level := strings.ToUpper(strings.TrimSpace(*x.Level))
		if level == "" {
			x.Level = nil
		} else {
			switch level {
			case "A1", "A2", "B1", "B2", "C1", "C2":
				x.Level = &level
			default:
				return webx.E(400, "level", "invalid CEFR level")
			}
		}
	}
	var id uuid.UUID
	err = s.DB.QueryRow(r.Context(), `INSERT INTO groups(organization_id,name,level,teacher_name) VALUES($1,$2,$3,$4) RETURNING id`, a.OrgID, x.Name, x.Level, x.TeacherName).Scan(&id)
	if err != nil {
		return err
	}
	webx.JSON(w, 201, map[string]any{"id": id.String(), "name": x.Name})
	return nil
}
func (s *Service) archiveGroup(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center_admin") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE groups SET status='archived' WHERE id=$1 AND organization_id=$2`, r.PathValue("id"), a.OrgID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return webx.E(404, "group", "group not found")
	}
	w.WriteHeader(204)
	return nil
}
func (s *Service) groupStudents(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center_admin") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	rows, err := s.DB.Query(r.Context(), `SELECT m.student_user_id,m.joined_at FROM group_members m JOIN groups g ON g.id=m.group_id WHERE m.group_id=$1 AND m.organization_id=$2 AND g.organization_id=$2 ORDER BY m.joined_at`, r.PathValue("id"), a.OrgID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type member struct {
		ID       string
		JoinedAt time.Time
	}
	members := []member{}
	for rows.Next() {
		var id uuid.UUID
		var joined time.Time
		if err := rows.Scan(&id, &joined); err != nil {
			return err
		}
		members = append(members, member{ID: id.String(), JoinedAt: joined})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var students struct {
		Items []Student `json:"items"`
	}
	if err := s.Identity.Do(r.Context(), "GET", "/internal/students?organization_id="+a.OrgID, nil, &students); err != nil {
		return fmt.Errorf("resolve group students: %w", err)
	}
	byID := make(map[string]Student, len(students.Items))
	for _, st := range students.Items {
		byID[st.UserID] = st
	}
	items := make([]map[string]any, 0, len(members))
	for _, m := range members {
		st, ok := byID[m.ID]
		item := map[string]any{"student_user_id": m.ID, "joined_at": m.JoinedAt}
		if ok {
			item["user_id"] = st.UserID
			item["full_name"] = st.FullName
			item["email"] = st.Email
			item["status"] = st.Status
			item["current_level"] = st.CurrentLevel
		}
		items = append(items, item)
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return nil
}
func (s *Service) validateStudent(ctx context.Context, id, org string) error {
	var p struct {
		UserID         string  `json:"user_id"`
		OrganizationID *string `json:"organization_id"`
		Role           string  `json:"role"`
		Status         string  `json:"status"`
	}
	if err := s.Identity.Do(ctx, "GET", "/internal/resolve?user_id="+id, nil, &p); err != nil {
		return fmt.Errorf("resolve student: %w", err)
	}
	if p.Role != "student" || p.OrganizationID == nil || *p.OrganizationID != org || p.Status != "active" {
		return webx.E(400, "student", "active student does not belong to this center")
	}
	return nil
}
func (s *Service) addGroupStudent(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center_admin") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	var x struct {
		StudentUserID string `json:"student_user_id"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if err = s.validateStudent(r.Context(), x.StudentUserID, a.OrgID); err != nil {
		return err
	}
	var exists bool
	if err = s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM groups WHERE id=$1 AND organization_id=$2 AND status='active')`, r.PathValue("id"), a.OrgID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return webx.E(404, "group", "group not found")
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO group_members(group_id,organization_id,student_user_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, r.PathValue("id"), a.OrgID, x.StudentUserID)
	if err != nil {
		return err
	}
	webx.JSON(w, 201, map[string]any{"ok": true})
	return nil
}
func (s *Service) removeGroupStudent(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center_admin") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	_, err = s.DB.Exec(r.Context(), `DELETE FROM group_members WHERE group_id=$1 AND organization_id=$2 AND student_user_id=$3`, r.PathValue("id"), a.OrgID, r.PathValue("studentID"))
	if err != nil {
		return err
	}
	w.WriteHeader(204)
	return nil
}
func (s *Service) internalValidateTarget(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "assessment", "sat", "listening"); err != nil {
		return err
	}
	var x struct {
		OrganizationID string `json:"organization_id"`
		TargetType     string `json:"target_type"`
		TargetID       string `json:"target_id"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if _, err := uuid.Parse(x.OrganizationID); err != nil {
		return webx.E(400, "organization_id", "invalid organization id")
	}
	if x.TargetType == "all" {
		webx.JSON(w, 200, map[string]any{"valid": true})
		return nil
	}
	if x.TargetType != "student" && x.TargetType != "group" {
		return webx.E(400, "target_type", "target must be student, group, or all")
	}
	if _, err := uuid.Parse(x.TargetID); err != nil {
		return webx.E(400, "target_id", "invalid target id")
	}
	if x.TargetType == "group" {
		var ok bool
		if err := s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM groups WHERE id=$1 AND organization_id=$2 AND status='active')`, x.TargetID, x.OrganizationID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return webx.E(400, "target_id", "active group does not belong to this center")
		}
		webx.JSON(w, 200, map[string]any{"valid": true})
		return nil
	}
	if err := s.validateStudent(r.Context(), x.TargetID, x.OrganizationID); err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"valid": true})
	return nil
}

func (s *Service) internalStudentGroups(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "assessment", "sat", "listening"); err != nil {
		return err
	}
	org := r.URL.Query().Get("organization_id")
	rows, err := s.DB.Query(r.Context(), `SELECT group_id FROM group_members WHERE organization_id=$1 AND student_user_id=$2`, org, r.PathValue("id"))
	if err != nil {
		return err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id.String())
	}
	webx.JSON(w, 200, map[string]any{"group_ids": ids})
	return rows.Err()
}
func (s *Service) reserve(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "assessment", "vocabulary", "listening", "sat"); err != nil {
		return err
	}
	var x struct {
		OrganizationID  string `json:"organization_id"`
		ServiceCode     string `json:"service_code"`
		Amount          int    `json:"amount"`
		ReservationKey  string `json:"reservation_key"`
		HoldConcurrency bool   `json:"hold_concurrency"`
		LeaseMinutes    int    `json:"lease_minutes"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if x.Amount == 0 {
		x.Amount = 1
	}
	if x.LeaseMinutes == 0 {
		x.LeaseMinutes = 180
	}
	var allowed bool
	var used, remaining int64
	var lim int
	var reason string
	var reservationID *uuid.UUID
	err := s.DB.QueryRow(r.Context(), `SELECT allowed,used,monthly_limit,remaining,reason,reservation_id FROM reserve_service_usage($1,$2,$3,nullif($4,''),$5,$6)`, x.OrganizationID, x.ServiceCode, x.Amount, x.ReservationKey, x.HoldConcurrency, x.LeaseMinutes).Scan(&allowed, &used, &lim, &remaining, &reason, &reservationID)
	if err != nil {
		return err
	}
	status := 200
	if !allowed {
		status = 429
	}
	var rid any
	if reservationID != nil {
		rid = reservationID.String()
	}
	webx.JSON(w, status, map[string]any{"allowed": allowed, "used": used, "monthly_limit": lim, "remaining": remaining, "reason": reason, "reservation_id": rid})
	return nil
}

func (s *Service) releaseUsage(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "assessment", "vocabulary", "listening", "sat"); err != nil {
		return err
	}
	var x struct {
		OrganizationID string `json:"organization_id"`
		ServiceCode    string `json:"service_code"`
		ReservationKey string `json:"reservation_key"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if strings.TrimSpace(x.ReservationKey) == "" {
		return webx.E(400, "reservation_key", "reservation key required")
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE usage_reservations SET released_at=coalesce(released_at,now()) WHERE organization_id=$1 AND service_code=$2 AND reservation_key=$3`, x.OrganizationID, x.ServiceCode, x.ReservationKey)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"released": ct.RowsAffected() > 0})
	return nil
}

func (s *Service) cancelUsage(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "assessment", "vocabulary", "listening", "sat"); err != nil {
		return err
	}
	var x struct {
		OrganizationID string `json:"organization_id"`
		ServiceCode    string `json:"service_code"`
		ReservationKey string `json:"reservation_key"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if strings.TrimSpace(x.ReservationKey) == "" {
		return webx.E(400, "reservation_key", "reservation key required")
	}
	var cancelled bool
	if err := s.DB.QueryRow(r.Context(), `SELECT cancel_service_usage($1,$2,$3)`, x.OrganizationID, x.ServiceCode, x.ReservationKey).Scan(&cancelled); err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"cancelled": cancelled})
	return nil
}

func (s *Service) internalOrganization(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "identity", "assessment", "vocabulary", "listening", "sat", "analytics"); err != nil {
		return err
	}
	c, err := scanCenter(s.DB.QueryRow(r.Context(), `SELECT id,name,slug,status,subscription_status,trial_ends_at,timezone,active_student_limit,created_at FROM organizations WHERE id=$1`, r.PathValue("id")))
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(404, "center", "center not found")
	}
	if err != nil {
		return err
	}
	webx.JSON(w, 200, c)
	return nil
}
