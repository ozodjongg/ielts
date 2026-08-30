package sat

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/example/ielts-platform/internal/authz"
	"github.com/example/ielts-platform/internal/bank"
	"github.com/example/ielts-platform/internal/clientx"
	"github.com/example/ielts-platform/internal/webx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB                             *pgxpool.Pool
	Bank                           *bank.SATBank
	Tenant, Points, Analytics      *clientx.Client
	InternalSecret, QuestionSecret string
}
type assignment struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	Title          string     `json:"title"`
	TargetType     string     `json:"target_type"`
	TargetID       *string    `json:"target_id,omitempty"`
	QuestionCount  int        `json:"question_count"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}
type planItem struct {
	QuestionID     string  `json:"question_id"`
	TopicCode      string  `json:"topic_code"`
	DisplayOrder   []int   `json:"display_order"`
	RushMultiplier float64 `json:"rush_multiplier"`
}

func (s *Service) Router() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		webx.JSON(w, 200, map[string]any{"status": "ok", "service": "sat", "bank_version": s.Bank.Version, "questions": len(s.Bank.Questions), "topics": len(s.Bank.Topics)})
	})
	m.HandleFunc("GET /v1/catalog", webx.Handle(s.catalog))
	m.HandleFunc("GET /v1/assignments", webx.Handle(s.assignments))
	m.HandleFunc("POST /v1/assignments", webx.Handle(s.createAssignment))
	m.HandleFunc("POST /v1/assignments/{id}/start", webx.Handle(s.start))
	m.HandleFunc("GET /v1/attempts/{id}", webx.Handle(s.current))
	m.HandleFunc("POST /v1/attempts/{id}/answer", webx.Handle(s.answer))
	m.HandleFunc("POST /v1/attempts/{id}/finish", webx.Handle(s.finish))
	m.HandleFunc("GET /v1/history", webx.Handle(s.history))
	return webx.Security(m)
}
func (s *Service) actor(r *http.Request) (authz.Actor, error) {
	a, e := authz.Verify(r, s.InternalSecret)
	if e != nil {
		return a, webx.E(401, "internal_auth", "invalid gateway signature")
	}
	return a, nil
}
func (s *Service) catalog(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" && a.Role != "center" && a.Role != "teacher" && a.Role != "admin" {
		return webx.E(403, "forbidden", "invalid role")
	}
	domains := map[string]int{}
	for _, q := range s.Bank.Questions {
		domains[q.Domain]++
	}
	webx.JSON(w, 200, map[string]any{"service": "sat_math", "name": "SAT Math Practice", "language": "English", "bank_version": s.Bank.Version, "question_count": len(s.Bank.Questions), "topic_count": len(s.Bank.Topics), "domains": domains, "notice": "Original SAT-style practice content; not official College Board material."})
	return nil
}
func scanAssignment(row pgx.Row) (assignment, error) {
	var x assignment
	var id, org uuid.UUID
	var target *uuid.UUID
	e := row.Scan(&id, &org, &x.Title, &x.TargetType, &target, &x.QuestionCount, &x.DueAt, &x.CreatedAt)
	x.ID = id.String()
	x.OrganizationID = org.String()
	if target != nil {
		v := target.String()
		x.TargetID = &v
	}
	return x, e
}
func (s *Service) assignments(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role == "center" || a.Role == "teacher" {
		query := `SELECT id,organization_id,title,target_type,target_id,question_count,due_at,created_at FROM sat_assignments WHERE organization_id=$1 ORDER BY created_at DESC`
		args := []any{a.OrgID}
		if a.Role == "teacher" {
			query = `SELECT id,organization_id,title,target_type,target_id,question_count,due_at,created_at FROM sat_assignments WHERE organization_id=$1 AND created_by=$2 ORDER BY created_at DESC`
			args = append(args, a.UserID)
		}
		rows, e := s.DB.Query(r.Context(), query, args...)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []assignment{}
		for rows.Next() {
			x, e := scanAssignment(rows)
			if e != nil {
				return e
			}
			items = append(items, x)
		}
		webx.JSON(w, 200, map[string]any{"items": items})
		return rows.Err()
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student or center admin required")
	}
	groups, e := s.groups(r, a)
	if e != nil {
		return e
	}
	rows, e := s.DB.Query(r.Context(), `SELECT id,organization_id,title,target_type,target_id,question_count,due_at,created_at FROM sat_assignments WHERE organization_id=$1 AND (due_at IS NULL OR due_at>=now()) ORDER BY created_at DESC`, a.OrgID)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []assignment{}
	for rows.Next() {
		x, e := scanAssignment(rows)
		if e != nil {
			return e
		}
		if applicable(a, x, groups) {
			items = append(items, x)
		}
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}
func applicable(a authz.Actor, x assignment, g map[string]bool) bool {
	return x.TargetType == "all" || (x.TargetType == "student" && x.TargetID != nil && *x.TargetID == a.UserID) || (x.TargetType == "group" && x.TargetID != nil && g[*x.TargetID])
}
func (s *Service) groups(r *http.Request, a authz.Actor) (map[string]bool, error) {
	var out struct {
		GroupIDs []string `json:"group_ids"`
	}
	if e := s.Tenant.Do(r.Context(), "GET", "/internal/student/"+a.UserID+"/groups?organization_id="+a.OrgID, nil, &out); e != nil {
		return nil, e
	}
	m := map[string]bool{}
	for _, g := range out.GroupIDs {
		m[g] = true
	}
	return m, nil
}
func (s *Service) validateAssignmentTarget(ctx context.Context, actor authz.Actor, targetType string, targetID *string) error {
	if targetType == "all" {
		return nil
	}
	if targetID == nil || strings.TrimSpace(*targetID) == "" {
		return webx.E(400, "target_id", "target id required")
	}
	if _, err := uuid.Parse(*targetID); err != nil {
		return webx.E(400, "target_id", "invalid target id")
	}
	var out struct {
		Valid bool `json:"valid"`
	}
	if err := s.Tenant.Do(ctx, "POST", "/internal/target/validate", map[string]any{
		"organization_id": actor.OrgID, "target_type": targetType, "target_id": *targetID,
		"actor_role": actor.Role, "actor_user_id": actor.UserID,
	}, &out); err != nil {
		return fmt.Errorf("validate assignment target: %w", err)
	}
	if !out.Valid {
		return webx.E(400, "target_id", "invalid assignment target")
	}
	return nil
}

func (s *Service) createAssignment(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "center" && a.Role != "teacher" {
		return webx.E(403, "forbidden", "center admin or teacher required")
	}
	var x struct {
		Title         string     `json:"title"`
		TargetType    string     `json:"target_type"`
		TargetID      *string    `json:"target_id"`
		QuestionCount int        `json:"question_count"`
		DueAt         *time.Time `json:"due_at"`
	}
	if e := webx.Decode(r, &x, 256<<10); e != nil {
		return e
	}
	if x.TargetType != "student" && x.TargetType != "group" && x.TargetType != "all" {
		return webx.E(400, "target_type", "student, group or all required")
	}
	if a.Role == "teacher" && x.TargetType == "all" {
		return webx.E(403, "target_forbidden", "teachers can assign only to their own groups or students in those groups")
	}
	if x.TargetType == "all" {
		x.TargetID = nil
	} else if x.TargetID == nil {
		return webx.E(400, "target_id", "target id required")
	}
	if err := s.validateAssignmentTarget(r.Context(), a, x.TargetType, x.TargetID); err != nil {
		return err
	}
	if x.QuestionCount == 0 {
		x.QuestionCount = 44
	}
	if x.QuestionCount < 10 || x.QuestionCount > 80 {
		return webx.E(400, "question_count", "question count must be 10-80")
	}
	if x.DueAt != nil && x.DueAt.Before(time.Now()) {
		return webx.E(400, "due_at", "due date must be in the future")
	}
	if strings.TrimSpace(x.Title) == "" {
		x.Title = "SAT Math Practice"
	}
	var id uuid.UUID
	e = s.DB.QueryRow(r.Context(), `INSERT INTO sat_assignments(organization_id,title,target_type,target_id,question_count,due_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, a.OrgID, x.Title, x.TargetType, x.TargetID, x.QuestionCount, x.DueAt, a.UserID).Scan(&id)
	if e != nil {
		return e
	}
	webx.JSON(w, 201, map[string]any{"id": id.String()})
	return nil
}
func seed(parts ...string) int64 {
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return int64(binary.LittleEndian.Uint64(h[:8]))
}
func (s *Service) buildPlan(student, asg string, count int) ([]planItem, error) {
	rng := rand.New(rand.NewSource(seed(s.QuestionSecret, student, asg, s.Bank.Version)))
	topics := append([]string(nil), s.Bank.Topics...)
	rng.Shuffle(len(topics), func(i, j int) { topics[i], topics[j] = topics[j], topics[i] })
	plan := make([]planItem, 0, count)
	round := 0
	for len(plan) < count {
		for _, t := range topics {
			qs := s.Bank.ByTopic[t]
			if len(qs) == 0 {
				continue
			}
			q := qs[(rng.Intn(len(qs))+round)%len(qs)]
			order := []int{0, 1, 2, 3}
			rr := rand.New(rand.NewSource(seed(s.QuestionSecret, student, asg, q.ID)))
			rr.Shuffle(4, func(i, j int) { order[i], order[j] = order[j], order[i] })
			plan = append(plan, planItem{QuestionID: q.ID, TopicCode: t, DisplayOrder: order, RushMultiplier: 1})
			if len(plan) >= count {
				break
			}
		}
		round++
	}
	ids := make([]string, len(plan))
	for i, p := range plan {
		ids[i] = p.QuestionID
	}
	var quotes struct {
		Items []struct {
			QuestionID string  `json:"question_id"`
			Multiplier float64 `json:"multiplier"`
		} `json:"items"`
	}
	if s.Points != nil {
		_ = s.Points.Do(context.Background(), "POST", "/internal/quote/batch", map[string]any{"service_code": "sat_math", "question_ids": ids}, &quotes)
		qm := map[string]float64{}
		for _, q := range quotes.Items {
			qm[q.QuestionID] = q.Multiplier
		}
		for i := range plan {
			if q := qm[plan[i].QuestionID]; q >= 1 {
				plan[i].RushMultiplier = q
			}
		}
	}
	return plan, nil
}
func satReservationKey(assignmentID, studentID string) string {
	return "sat:" + assignmentID + ":" + studentID
}

func (s *Service) start(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	x, e := scanAssignment(s.DB.QueryRow(r.Context(), `SELECT id,organization_id,title,target_type,target_id,question_count,due_at,created_at FROM sat_assignments WHERE id=$1 AND organization_id=$2`, r.PathValue("id"), a.OrgID))
	if errors.Is(e, pgx.ErrNoRows) {
		return webx.E(404, "assignment", "assignment not found")
	}
	if e != nil {
		return e
	}
	g, e := s.groups(r, a)
	if e != nil {
		return e
	}
	if !applicable(a, x, g) {
		return webx.E(403, "forbidden", "assignment not assigned to student")
	}
	if x.DueAt != nil && x.DueAt.Before(time.Now()) {
		return webx.E(409, "expired", "assignment expired")
	}
	var existing uuid.UUID
	e = s.DB.QueryRow(r.Context(), `SELECT id FROM sat_attempts WHERE assignment_id=$1 AND student_user_id=$2`, x.ID, a.UserID).Scan(&existing)
	if e == nil {
		webx.JSON(w, 200, map[string]any{"attempt_id": existing.String(), "resumed": true})
		return nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	var quota struct {
		Allowed bool   `json:"allowed"`
		Reason  string `json:"reason"`
	}
	if e = s.Tenant.Do(r.Context(), "POST", "/internal/usage/reserve", map[string]any{
		"organization_id": a.OrgID, "service_code": "sat_math", "amount": 1,
		"reservation_key": satReservationKey(x.ID, a.UserID), "hold_concurrency": true, "lease_minutes": 180,
	}, &quota); e != nil || !quota.Allowed {
		return webx.E(429, "quota", "SAT Math quota or concurrency limit reached")
	}
	reservationKey := satReservationKey(x.ID, a.UserID)
	created := false
	defer func() {
		if !created {
			_ = s.Tenant.Do(context.Background(), "POST", "/internal/usage/cancel", map[string]any{"organization_id": a.OrgID, "service_code": "sat_math", "reservation_key": reservationKey}, nil)
		}
	}()
	plan, e := s.buildPlan(a.UserID, x.ID, x.QuestionCount)
	if e != nil {
		return e
	}
	b, _ := json.Marshal(plan)
	var id uuid.UUID
	e = s.DB.QueryRow(r.Context(), `INSERT INTO sat_attempts(organization_id,assignment_id,student_user_id,bank_version,question_plan) VALUES($1,$2,$3,$4,$5) RETURNING id`, a.OrgID, x.ID, a.UserID, s.Bank.Version, b).Scan(&id)
	if e != nil {
		return e
	}
	created = true
	s.emit(r.Context(), a.OrgID, a.UserID, "sat.started", map[string]any{"attempt_id": id.String()})
	webx.JSON(w, 201, map[string]any{"attempt_id": id.String()})
	return nil
}
func (s *Service) loadAttempt(ctx context.Context, id, org, student string) (string, []planItem, error) {
	var status string
	var b []byte
	e := s.DB.QueryRow(ctx, `SELECT status,question_plan FROM sat_attempts WHERE id=$1 AND organization_id=$2 AND student_user_id=$3`, id, org, student).Scan(&status, &b)
	if errors.Is(e, pgx.ErrNoRows) {
		return "", nil, webx.E(404, "attempt", "attempt not found")
	}
	if e != nil {
		return "", nil, e
	}
	var plan []planItem
	if e = json.Unmarshal(b, &plan); e != nil {
		return "", nil, e
	}
	return status, plan, nil
}
func (s *Service) current(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	status, plan, e := s.loadAttempt(r.Context(), r.PathValue("id"), a.OrgID, a.UserID)
	if e != nil {
		return e
	}
	var answered int
	_ = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM sat_answers WHERE attempt_id=$1 AND answered_at IS NOT NULL`, r.PathValue("id")).Scan(&answered)
	if status != "in_progress" {
		var raw int
		var pct *float64
		var score *int
		_ = s.DB.QueryRow(r.Context(), `SELECT raw_correct,percent,estimated_sat_score FROM sat_attempts WHERE id=$1`, r.PathValue("id")).Scan(&raw, &pct, &score)
		webx.JSON(w, 200, map[string]any{"status": status, "answered": answered, "total": len(plan), "raw_correct": raw, "percent": pct, "estimated_sat_score": score})
		return nil
	}
	if answered >= len(plan) {
		webx.JSON(w, 200, map[string]any{"status": status, "answered": answered, "total": len(plan), "complete": true})
		return nil
	}
	p := plan[answered]
	q := s.Bank.Questions[p.QuestionID]
	opts := make([]string, 4)
	for i, idx := range p.DisplayOrder {
		opts[i] = q.Options[idx]
	}
	webx.JSON(w, 200, map[string]any{"status": status, "answered": answered, "total": len(plan), "question_ref": q.ID, "topic_code": q.TopicCode, "domain": q.Domain, "prompt": q.Prompt, "options": opts, "difficulty": q.Difficulty, "base_points": q.BasePoints, "rush_multiplier": p.RushMultiplier})
	return nil
}
func (s *Service) answer(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	var x struct {
		QuestionRef     string `json:"question_ref"`
		DisplayedOption int    `json:"displayed_option"`
		ResponseMS      *int   `json:"response_ms"`
	}
	if e := webx.Decode(r, &x, 64<<10); e != nil {
		return e
	}
	if x.DisplayedOption < 0 || x.DisplayedOption > 3 {
		return webx.E(400, "option", "displayed_option must be 0-3")
	}
	status, plan, e := s.loadAttempt(r.Context(), r.PathValue("id"), a.OrgID, a.UserID)
	if e != nil {
		return e
	}
	if status != "in_progress" {
		return webx.E(409, "attempt", "attempt is not active")
	}
	var answered int
	_ = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM sat_answers WHERE attempt_id=$1 AND answered_at IS NOT NULL`, r.PathValue("id")).Scan(&answered)
	if answered >= len(plan) {
		return webx.E(409, "complete", "all questions are already answered")
	}
	p := plan[answered]
	if p.QuestionID != x.QuestionRef {
		return webx.E(409, "stale_question", "question reference is stale")
	}
	q := s.Bank.Questions[p.QuestionID]
	canonical := p.DisplayOrder[x.DisplayedOption]
	correct := "ABCD"[canonical:canonical+1] == q.Correct
	_, e = s.DB.Exec(r.Context(), `INSERT INTO sat_answers(attempt_id,question_id,topic_code,selected_option,is_correct,base_points,rush_multiplier,response_ms,answered_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,now())`, r.PathValue("id"), q.ID, q.TopicCode, string("ABCD"[canonical]), correct, q.BasePoints, p.RushMultiplier, x.ResponseMS)
	if e != nil {
		return e
	}
	webx.JSON(w, 200, map[string]any{"accepted": true, "answered": answered + 1, "remaining": len(plan) - answered - 1})
	return nil
}
func (s *Service) finish(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	status, plan, e := s.loadAttempt(r.Context(), r.PathValue("id"), a.OrgID, a.UserID)
	if e != nil {
		return e
	}
	if status != "in_progress" {
		return webx.E(409, "attempt", "attempt already finished")
	}
	var assignmentID uuid.UUID
	if e = s.DB.QueryRow(r.Context(), `SELECT assignment_id FROM sat_attempts WHERE id=$1 AND organization_id=$2 AND student_user_id=$3`, r.PathValue("id"), a.OrgID, a.UserID).Scan(&assignmentID); e != nil {
		return e
	}
	rows, e := s.DB.Query(r.Context(), `SELECT question_id,topic_code,is_correct,base_points,rush_multiplier FROM sat_answers WHERE attempt_id=$1`, r.PathValue("id"))
	if e != nil {
		return e
	}
	defer rows.Close()
	type ans struct {
		QID, Topic string
		Correct    bool
		Base, Rush float64
	}
	as := []ans{}
	for rows.Next() {
		var x ans
		var id uuid.UUID
		if e := rows.Scan(&id, &x.Topic, &x.Correct, &x.Base, &x.Rush); e != nil {
			return e
		}
		x.QID = id.String()
		as = append(as, x)
	}
	if len(as) != len(plan) {
		return webx.E(409, "incomplete", "all SAT questions must be answered")
	}
	correct := 0
	topicN := map[string][2]int{}
	for _, x := range as {
		v := topicN[x.Topic]
		v[1]++
		if x.Correct {
			correct++
			v[0]++
		}
		topicN[x.Topic] = v
		if s.Points != nil {
			_ = s.Points.Do(r.Context(), "POST", "/internal/record", map[string]any{"organization_id": a.OrgID, "student_user_id": a.UserID, "service_code": "sat_math", "question_id": x.QID, "event_key": "sat:" + r.PathValue("id") + ":" + x.QID, "base_points": x.Base, "multiplier": x.Rush, "correct": x.Correct, "reason": "sat_answer"}, nil)
		}
	}
	pct := 100 * float64(correct) / float64(len(as))
	estimate := int(math.Round((200+600*pct/100)/10) * 10)
	if estimate < 200 {
		estimate = 200
	}
	if estimate > 800 {
		estimate = 800
	}
	_, e = s.DB.Exec(r.Context(), `UPDATE sat_attempts SET status='completed',raw_correct=$2,percent=$3,estimated_sat_score=$4,finished_at=now() WHERE id=$1`, r.PathValue("id"), correct, pct, estimate)
	if e != nil {
		return e
	}
	for topic, v := range topicN {
		mastery := float64(v[0]) / float64(v[1])
		_, _ = s.DB.Exec(r.Context(), `INSERT INTO sat_topic_mastery(organization_id,student_user_id,topic_code,attempts,correct,mastery) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(organization_id,student_user_id,topic_code) DO UPDATE SET attempts=sat_topic_mastery.attempts+$4,correct=sat_topic_mastery.correct+$5,mastery=(sat_topic_mastery.correct+$5)::numeric/(sat_topic_mastery.attempts+$4),updated_at=now()`, a.OrgID, a.UserID, topic, v[1], v[0], mastery)
	}
	_ = s.Tenant.Do(r.Context(), "POST", "/internal/usage/release", map[string]any{
		"organization_id": a.OrgID, "service_code": "sat_math",
		"reservation_key": satReservationKey(assignmentID.String(), a.UserID),
	}, nil)
	s.emit(r.Context(), a.OrgID, a.UserID, "sat.completed", map[string]any{"attempt_id": r.PathValue("id"), "percent": pct, "estimated_score": estimate})
	webx.JSON(w, 200, map[string]any{"status": "completed", "raw_correct": correct, "total": len(as), "percent": pct, "estimated_sat_score": estimate, "note": "Estimated practice score, not an official SAT score."})
	return nil
}
func (s *Service) history(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	student := a.UserID
	if a.Role == "center" {
		student = r.URL.Query().Get("student_user_id")
		if student == "" {
			return webx.E(400, "student", "student_user_id required")
		}
	} else if a.Role != "student" {
		return webx.E(403, "forbidden", "invalid role")
	}
	rows, e := s.DB.Query(r.Context(), `SELECT id,status,raw_correct,percent,estimated_sat_score,started_at,finished_at FROM sat_attempts WHERE organization_id=$1 AND student_user_id=$2 ORDER BY started_at DESC LIMIT 100`, a.OrgID, student)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var st string
		var raw int
		var pct *float64
		var score *int
		var start time.Time
		var fin *time.Time
		if e := rows.Scan(&id, &st, &raw, &pct, &score, &start, &fin); e != nil {
			return e
		}
		items = append(items, map[string]any{"id": id.String(), "status": st, "raw_correct": raw, "percent": pct, "estimated_sat_score": score, "started_at": start, "finished_at": fin})
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}
func (s *Service) emit(ctx context.Context, org, user, typ string, payload map[string]any) {
	if s.Analytics != nil {
		_ = s.Analytics.Do(ctx, "POST", "/internal/events", map[string]any{"organization_id": org, "student_user_id": user, "service_code": "sat_math", "event_type": typ, "payload": payload}, nil)
	}
}

var _ = sort.Strings
