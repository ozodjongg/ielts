package points

import (
	"math"
	"net/http"
	"time"

	"github.com/example/assessment-platform-v5/internal/authz"
	"github.com/example/assessment-platform-v5/internal/webx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB             *pgxpool.Pool
	InternalSecret string
}

func (s *Service) Router() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		webx.JSON(w, 200, map[string]any{"status": "ok", "service": "points"})
	})
	m.HandleFunc("GET /v1/me", webx.Handle(s.me))
	m.HandleFunc("GET /v1/leaderboard", webx.Handle(s.leaderboard))
	m.HandleFunc("POST /internal/quote/batch", webx.Handle(s.quoteBatch))
	m.HandleFunc("POST /internal/record", webx.Handle(s.record))
	return webx.Security(m)
}
func (s *Service) actor(r *http.Request) (authz.Actor, error) {
	a, err := authz.Verify(r, s.InternalSecret)
	if err != nil {
		return a, webx.E(401, "internal_auth", "invalid gateway signature")
	}
	return a, nil
}
func (s *Service) service(r *http.Request) error {
	if err := authz.VerifyService(r, s.InternalSecret, "assessment", "sat", "listening", "vocabulary"); err != nil {
		return webx.E(401, "service_auth", "invalid service signature")
	}
	return nil
}
func rush(attempts int64, rate float64) float64 {
	if attempts < 20 {
		return 1
	}
	m := 1 + 1.5*math.Max(0, 0.65-rate)/0.65
	if m > 2.5 {
		m = 2.5
	}
	return math.Round(m*1000) / 1000
}

type Quote struct {
	QuestionID string  `json:"question_id"`
	Multiplier float64 `json:"multiplier"`
	Attempts   int64   `json:"attempts"`
	SolveRate  float64 `json:"solve_rate"`
}

func (s *Service) quoteBatch(w http.ResponseWriter, r *http.Request) error {
	if err := s.service(r); err != nil {
		return err
	}
	var x struct {
		ServiceCode string   `json:"service_code"`
		QuestionIDs []string `json:"question_ids"`
	}
	if err := webx.Decode(r, &x, 1<<20); err != nil {
		return err
	}
	if len(x.QuestionIDs) > 100 {
		return webx.E(400, "question_ids", "maximum 100 questions per quote")
	}
	out := make([]Quote, 0, len(x.QuestionIDs))
	for _, id := range x.QuestionIDs {
		if _, err := uuid.Parse(id); err != nil {
			return webx.E(400, "question_id", "invalid question id")
		}
		var attempts, correct int64
		err := s.DB.QueryRow(r.Context(), `SELECT attempts,correct FROM question_stats WHERE service_code=$1 AND question_id=$2`, x.ServiceCode, id).Scan(&attempts, &correct)
		if err != nil {
			attempts = 0
			correct = 0
		}
		rate := (float64(correct) + 13) / (float64(attempts) + 20)
		out = append(out, Quote{QuestionID: id, Multiplier: rush(attempts, rate), Attempts: attempts, SolveRate: math.Round(rate*10000) / 10000})
	}
	webx.JSON(w, 200, map[string]any{"items": out})
	return nil
}

type Record struct {
	OrganizationID string  `json:"organization_id"`
	StudentUserID  string  `json:"student_user_id"`
	ServiceCode    string  `json:"service_code"`
	QuestionID     *string `json:"question_id"`
	EventKey       string  `json:"event_key"`
	BasePoints     float64 `json:"base_points"`
	Multiplier     float64 `json:"multiplier"`
	Correct        bool    `json:"correct"`
	Reason         string  `json:"reason"`
}

func (s *Service) record(w http.ResponseWriter, r *http.Request) error {
	if err := s.service(r); err != nil {
		return err
	}
	var x Record
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if _, err := uuid.Parse(x.OrganizationID); err != nil {
		return webx.E(400, "organization", "invalid organization")
	}
	if _, err := uuid.Parse(x.StudentUserID); err != nil {
		return webx.E(400, "student", "invalid student")
	}
	if x.EventKey == "" || len(x.EventKey) > 200 {
		return webx.E(400, "event_key", "invalid event key")
	}
	if x.BasePoints < 0 || x.BasePoints > 1000 {
		return webx.E(400, "base_points", "invalid base points")
	}
	if x.Multiplier < 1 || x.Multiplier > 2.5 {
		return webx.E(400, "multiplier", "invalid multiplier")
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	awarded := x.BasePoints * x.Multiplier
	if !x.Correct {
		awarded = 0
	}
	var qid any
	if x.QuestionID != nil {
		if _, err := uuid.Parse(*x.QuestionID); err != nil {
			return webx.E(400, "question_id", "invalid question id")
		}
		qid = *x.QuestionID
		inc := 0
		if x.Correct {
			inc = 1
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO question_stats(service_code,question_id,attempts,correct) VALUES($1,$2,1,$3) ON CONFLICT(service_code,question_id) DO UPDATE SET attempts=question_stats.attempts+1,correct=question_stats.correct+$3,smoothed_solve_rate=(question_stats.correct+$3+13.0)/(question_stats.attempts+1+20.0),rush_multiplier=LEAST(2.5,GREATEST(1.0,1+1.5*GREATEST(0,0.65-((question_stats.correct+$3+13.0)/(question_stats.attempts+1+20.0)))/0.65)),updated_at=now()`, x.ServiceCode, *x.QuestionID, inc)
		if err != nil {
			return err
		}
	}
	ct, err := tx.Exec(r.Context(), `INSERT INTO point_ledger(organization_id,student_user_id,service_code,question_id,event_key,base_points,multiplier,awarded_points,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(event_key) DO NOTHING`, x.OrganizationID, x.StudentUserID, x.ServiceCode, qid, x.EventKey, x.BasePoints, x.Multiplier, awarded, x.Reason)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"awarded_points": awarded, "created": ct.RowsAffected() > 0})
	return nil
}
func (s *Service) me(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "student") != nil {
		return webx.E(403, "forbidden", "student required")
	}
	var total float64
	_ = s.DB.QueryRow(r.Context(), `SELECT coalesce(sum(awarded_points),0) FROM point_ledger WHERE organization_id=$1 AND student_user_id=$2`, a.OrgID, a.UserID).Scan(&total)
	rows, err := s.DB.Query(r.Context(), `SELECT service_code,coalesce(sum(awarded_points),0) FROM point_ledger WHERE organization_id=$1 AND student_user_id=$2 GROUP BY service_code ORDER BY 2 DESC`, a.OrgID, a.UserID)
	if err != nil {
		return err
	}
	defer rows.Close()
	by := map[string]float64{}
	for rows.Next() {
		var code string
		var p float64
		if err := rows.Scan(&code, &p); err != nil {
			return err
		}
		by[code] = p
	}
	webx.JSON(w, 200, map[string]any{"total_points": total, "by_service": by})
	return rows.Err()
}
func (s *Service) leaderboard(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if a.Role != "center_admin" && a.Role != "student" {
		return webx.E(403, "forbidden", "center membership required")
	}
	rows, err := s.DB.Query(r.Context(), `SELECT student_user_id,coalesce(sum(awarded_points),0) pts,max(created_at) last_at FROM point_ledger WHERE organization_id=$1 GROUP BY student_user_id ORDER BY pts DESC,last_at DESC LIMIT 50`, a.OrgID)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	rank := 0
	for rows.Next() {
		rank++
		var id uuid.UUID
		var pts float64
		var last time.Time
		if err := rows.Scan(&id, &pts, &last); err != nil {
			return err
		}
		items = append(items, map[string]any{"rank": rank, "student_user_id": id.String(), "points": pts, "last_activity": last})
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}
