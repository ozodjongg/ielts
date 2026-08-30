package analytics

import (
	"github.com/example/ielts-platform/internal/authz"
	"github.com/example/ielts-platform/internal/webx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

type Service struct {
	DB             *pgxpool.Pool
	InternalSecret string
}
type Event struct {
	EventID        string         `json:"event_id"`
	OrganizationID *string        `json:"organization_id"`
	StudentUserID  *string        `json:"student_user_id"`
	EventType      string         `json:"event_type"`
	ServiceCode    *string        `json:"service_code"`
	OccurredAt     time.Time      `json:"occurred_at"`
	Payload        map[string]any `json:"payload"`
}

func (s *Service) Router() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		webx.JSON(w, 200, map[string]any{"status": "ok", "service": "analytics"})
	})
	m.HandleFunc("POST /internal/events", webx.Handle(s.ingest))
	m.HandleFunc("GET /v1/overview", webx.Handle(s.overview))
	m.HandleFunc("GET /v1/activity", webx.Handle(s.activity))
	return webx.Security(m)
}
func (s *Service) actor(r *http.Request) (authz.Actor, error) {
	a, err := authz.Verify(r, s.InternalSecret)
	if err != nil {
		return a, webx.E(401, "internal_auth", "invalid gateway signature")
	}
	return a, nil
}
func (s *Service) ingest(w http.ResponseWriter, r *http.Request) error {
	if err := authz.VerifyService(r, s.InternalSecret, "identity", "tenant", "assessment", "vocabulary", "listening", "review", "sat", "points"); err != nil {
		return webx.E(401, "service_auth", "invalid service signature")
	}
	var x Event
	if err := webx.Decode(r, &x, 1<<20); err != nil {
		return err
	}
	if x.EventID == "" {
		x.EventID = uuid.NewString()
	}
	if _, err := uuid.Parse(x.EventID); err != nil {
		return webx.E(400, "event_id", "invalid event id")
	}
	if x.OccurredAt.IsZero() {
		x.OccurredAt = time.Now().UTC()
	}
	if x.EventType == "" {
		return webx.E(400, "event_type", "event type required")
	}
	_, err := s.DB.Exec(r.Context(), `INSERT INTO events(event_id,organization_id,student_user_id,event_type,service_code,occurred_at,payload) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(event_id) DO NOTHING`, x.EventID, x.OrganizationID, x.StudentUserID, x.EventType, x.ServiceCode, x.OccurredAt, x.Payload)
	if err != nil {
		return err
	}
	webx.JSON(w, 202, map[string]any{"accepted": true})
	return nil
}
func (s *Service) overview(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	switch a.Role {
	case "admin":
		var centers, events, students int64
		_ = s.DB.QueryRow(r.Context(), `SELECT count(DISTINCT organization_id),count(*),count(DISTINCT student_user_id) FROM events WHERE occurred_at>=date_trunc('month',now())`).Scan(&centers, &events, &students)
		rows, err := s.DB.Query(r.Context(), `SELECT coalesce(service_code,'system'),count(*) FROM events WHERE occurred_at>=date_trunc('month',now()) GROUP BY 1 ORDER BY 2 DESC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		by := map[string]int64{}
		for rows.Next() {
			var k string
			var n int64
			if err := rows.Scan(&k, &n); err != nil {
				return err
			}
			by[k] = n
		}
		webx.JSON(w, 200, map[string]any{"active_centers": centers, "events_this_month": events, "active_students": students, "service_activity": by})
		return rows.Err()
	case "center":
		var attempts, completions, students int64
		_ = s.DB.QueryRow(r.Context(), `SELECT count(*) FILTER(WHERE event_type like '%.started'),count(*) FILTER(WHERE event_type like '%.completed'),count(DISTINCT student_user_id) FROM events WHERE organization_id=$1 AND occurred_at>=date_trunc('month',now())`, a.OrgID).Scan(&attempts, &completions, &students)
		webx.JSON(w, 200, map[string]any{"attempts_this_month": attempts, "completions_this_month": completions, "active_students": students})
		return nil
	case "student":
		var events int64
		_ = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM events WHERE organization_id=$1 AND student_user_id=$2 AND occurred_at>=date_trunc('month',now())`, a.OrgID, a.UserID).Scan(&events)
		rows, err := s.DB.Query(r.Context(), `SELECT coalesce(service_code,'system'),count(*) FROM events WHERE organization_id=$1 AND student_user_id=$2 AND occurred_at>=date_trunc('month',now()) GROUP BY 1`, a.OrgID, a.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		by := map[string]int64{}
		for rows.Next() {
			var k string
			var n int64
			if err := rows.Scan(&k, &n); err != nil {
				return err
			}
			by[k] = n
		}
		webx.JSON(w, 200, map[string]any{"activity_events": events, "by_service": by})
		return rows.Err()
	default:
		return webx.E(403, "forbidden", "invalid role")
	}
}
func (s *Service) activity(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	q := `SELECT event_id,organization_id,student_user_id,event_type,service_code,occurred_at,payload FROM events`
	args := []any{}
	if a.Role == "center" {
		q += ` WHERE organization_id=$1`
		args = append(args, a.OrgID)
	} else if a.Role == "student" {
		q += ` WHERE organization_id=$1 AND student_user_id=$2`
		args = append(args, a.OrgID, a.UserID)
	} else if a.Role != "admin" {
		return webx.E(403, "forbidden", "invalid role")
	}
	q += ` ORDER BY occurred_at DESC LIMIT 100`
	rows, err := s.DB.Query(r.Context(), q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var org, user *uuid.UUID
		var typ string
		var svc *string
		var at time.Time
		var payload map[string]any
		if err := rows.Scan(&id, &org, &user, &typ, &svc, &at, &payload); err != nil {
			return err
		}
		items = append(items, map[string]any{"event_id": id.String(), "organization_id": org, "student_user_id": user, "event_type": typ, "service_code": svc, "occurred_at": at, "payload": payload})
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}
