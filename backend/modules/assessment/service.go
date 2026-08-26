package assessment

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/example/assessment-platform-v5/internal/authz"
	"github.com/example/assessment-platform-v5/internal/bank"
	"github.com/example/assessment-platform-v5/internal/clientx"
	"github.com/example/assessment-platform-v5/internal/webx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB                                  *pgxpool.Pool
	Bank                                *bank.EnglishBank
	Tenant, Identity, Points, Analytics *clientx.Client
	InternalSecret, QuestionSecret      string
}
type Assignment struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	ServiceCode    string     `json:"service_code"`
	Title          string     `json:"title"`
	TargetType     string     `json:"target_type"`
	TargetID       *string    `json:"target_id,omitempty"`
	FromLevel      *string    `json:"from_level,omitempty"`
	ToLevel        *string    `json:"to_level,omitempty"`
	QuestionCount  *int       `json:"question_count,omitempty"`
	OpensAt        time.Time  `json:"opens_at"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	Status         string     `json:"status"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
}
type PlanItem struct {
	QuestionID     string  `json:"question_id"`
	SubjectCode    string  `json:"subject_code"`
	Level          string  `json:"level"`
	DisplayOrder   []int   `json:"display_order"`
	RushMultiplier float64 `json:"rush_multiplier"`
}
type ManualPrompt struct {
	PromptID   string `json:"prompt_id"`
	Component  string `json:"component"`
	Position   int    `json:"position"`
	PromptText string `json:"prompt_text"`
	Required   bool   `json:"required"`
	Submitted  bool   `json:"submitted"`
}
type Attempt struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	StudentUserID  string     `json:"student_user_id"`
	ServiceCode    string     `json:"service_code"`
	BankVersion    string     `json:"bank_version"`
	Status         string     `json:"status"`
	AssignmentID   *string    `json:"assignment_id,omitempty"`
	FromLevel      *string    `json:"from_level,omitempty"`
	ToLevel        *string    `json:"to_level,omitempty"`
	Plan           []PlanItem `json:"plan"`
	AutoScore      float64    `json:"auto_score"`
	FinalScore     *float64   `json:"final_score,omitempty"`
	LevelResult    *string    `json:"level_result,omitempty"`
	Readiness      *float64   `json:"readiness,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

var englishServices = map[string]bool{"placement": true, "vocabulary_test": true, "level_upgrade": true, "progress": true, "grammar": true, "ielts_readiness": true, "weakness": true, "speaking": true, "writing": true, "mock": true, "final_exit": true}

func (s *Service) Router() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		webx.JSON(w, 200, map[string]any{"status": "ok", "service": "assessment", "bank_version": s.Bank.Version, "questions": len(s.Bank.Questions)})
	})
	m.HandleFunc("GET /v1/catalog", webx.Handle(s.catalog))
	m.HandleFunc("GET /v1/assignments", webx.Handle(s.listAssignments))
	m.HandleFunc("POST /v1/assignments", webx.Handle(s.createAssignment))
	m.HandleFunc("DELETE /v1/assignments/{id}", webx.Handle(s.closeAssignment))
	m.HandleFunc("POST /v1/assignments/{id}/start", webx.Handle(s.start))
	m.HandleFunc("GET /v1/attempts/{id}", webx.Handle(s.current))
	m.HandleFunc("POST /v1/attempts/{id}/answer", webx.Handle(s.answer))
	m.HandleFunc("POST /v1/attempts/{id}/events", webx.Handle(s.event))
	m.HandleFunc("POST /v1/attempts/{id}/finish", webx.Handle(s.finish))
	m.HandleFunc("GET /v1/history", webx.Handle(s.history))
	m.HandleFunc("GET /v1/progress", webx.Handle(s.progress))
	m.HandleFunc("GET /internal/attempts/{id}", webx.Handle(s.internalAttempt))
	m.HandleFunc("GET /internal/attempts/{id}/manual-prompts", webx.Handle(s.internalManualPrompts))
	m.HandleFunc("POST /internal/attempts/{id}/manual-submissions", webx.Handle(s.internalRegisterManualSubmission))
	m.HandleFunc("PATCH /internal/attempts/{id}/review", webx.Handle(s.internalReview))
	return webx.Security(m)
}
func (s *Service) actor(r *http.Request) (authz.Actor, error) {
	a, err := authz.Verify(r, s.InternalSecret)
	if err != nil {
		return a, webx.E(401, "internal_auth", "invalid gateway signature")
	}
	return a, nil
}
func (s *Service) catalog(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if a.Role != "platform_admin" && a.Role != "center_admin" && a.Role != "student" {
		return webx.E(403, "forbidden", "invalid role")
	}
	items := []map[string]any{{"code": "placement", "name": "Level Placement Test", "default_questions": 80, "mode": "auto"}, {"code": "vocabulary_test", "name": "Vocabulary Assessment", "default_questions": 40, "mode": "auto"}, {"code": "level_upgrade", "name": "Level Upgrade Test", "default_questions": 40, "mode": "auto"}, {"code": "progress", "name": "Progress Test", "default_questions": 30, "mode": "auto"}, {"code": "grammar", "name": "Grammar Diagnostic", "default_questions": 40, "mode": "auto"}, {"code": "ielts_readiness", "name": "IELTS Readiness", "default_questions": 40, "mode": "auto"}, {"code": "weakness", "name": "Weakness Diagnostic", "default_questions": 30, "mode": "adaptive"}, {"code": "speaking", "name": "Speaking Assessment", "default_questions": 3, "mode": "manual"}, {"code": "writing", "name": "Writing Assessment", "default_questions": 2, "mode": "manual"}, {"code": "mock", "name": "IELTS-style Mock", "default_questions": 60, "mode": "hybrid"}, {"code": "final_exit", "name": "Final / Exit Assessment", "default_questions": 60, "mode": "auto"}}
	webx.JSON(w, 200, map[string]any{"items": items})
	return nil
}
func scanAssignment(row pgx.Row) (Assignment, error) {
	var a Assignment
	var id, org, by uuid.UUID
	var target *uuid.UUID
	err := row.Scan(&id, &org, &a.ServiceCode, &a.Title, &a.TargetType, &target, &a.FromLevel, &a.ToLevel, &a.QuestionCount, &a.OpensAt, &a.DueAt, &a.Status, &by, &a.CreatedAt)
	a.ID = id.String()
	a.OrganizationID = org.String()
	a.CreatedBy = by.String()
	if target != nil {
		x := target.String()
		a.TargetID = &x
	}
	return a, err
}
func defaultCount(code string) int {
	switch code {
	case "placement":
		return 80
	case "vocabulary_test", "level_upgrade", "grammar", "ielts_readiness":
		return 40
	case "progress", "weakness":
		return 30
	case "mock", "final_exit":
		return 60
	case "speaking":
		return 3
	case "writing":
		return 2
	}
	return 40
}
func validLevel(x string) bool {
	return x == "A1" || x == "A2" || x == "B1" || x == "B2" || x == "C1" || x == "C2"
}
func validUpgrade(from, to string) bool {
	switch from + ":" + to {
	case "A1:A2", "A2:B1", "B1:B2", "B2:C1":
		return true
	default:
		return false
	}
}
func (s *Service) listAssignments(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if a.Role == "center_admin" {
		rows, err := s.DB.Query(r.Context(), `SELECT id,organization_id,service_code,title,target_type,target_id,from_level,to_level,question_count,opens_at,due_at,status,created_by,created_at FROM assignments WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 500`, a.OrgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		items := []Assignment{}
		for rows.Next() {
			x, err := scanAssignment(rows)
			if err != nil {
				return err
			}
			items = append(items, x)
		}
		webx.JSON(w, 200, map[string]any{"items": items})
		return rows.Err()
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "center admin or student required")
	}
	groups, err := s.studentGroups(r.Context(), a)
	if err != nil {
		return err
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,organization_id,service_code,title,target_type,target_id,from_level,to_level,question_count,opens_at,due_at,status,created_by,created_at FROM assignments WHERE organization_id=$1 AND status='open' AND opens_at<=now() AND (due_at IS NULL OR due_at>=now()) ORDER BY created_at DESC`, a.OrgID)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []Assignment{}
	gm := map[string]bool{}
	for _, g := range groups {
		gm[g] = true
	}
	for rows.Next() {
		x, err := scanAssignment(rows)
		if err != nil {
			return err
		}
		ok := x.TargetType == "all" || (x.TargetType == "student" && x.TargetID != nil && *x.TargetID == a.UserID) || (x.TargetType == "group" && x.TargetID != nil && gm[*x.TargetID])
		if ok {
			items = append(items, x)
		}
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}
func (s *Service) validateAssignmentTarget(ctx context.Context, organizationID, targetType string, targetID *string) error {
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
		"organization_id": organizationID, "target_type": targetType, "target_id": *targetID,
	}, &out); err != nil {
		return fmt.Errorf("validate assignment target: %w", err)
	}
	if !out.Valid {
		return webx.E(400, "target_id", "invalid assignment target")
	}
	return nil
}

func (s *Service) createAssignment(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center_admin") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	var x struct {
		ServiceCode   string     `json:"service_code"`
		Title         string     `json:"title"`
		TargetType    string     `json:"target_type"`
		TargetID      *string    `json:"target_id"`
		FromLevel     *string    `json:"from_level"`
		ToLevel       *string    `json:"to_level"`
		QuestionCount *int       `json:"question_count"`
		DueAt         *time.Time `json:"due_at"`
	}
	if err := webx.Decode(r, &x, 1<<20); err != nil {
		return err
	}
	if !englishServices[x.ServiceCode] {
		return webx.E(400, "service", "unsupported English assessment service")
	}
	if x.TargetType != "student" && x.TargetType != "group" && x.TargetType != "all" {
		return webx.E(400, "target_type", "target must be student, group, or all")
	}
	if x.TargetType == "all" {
		x.TargetID = nil
	} else if x.TargetID == nil {
		return webx.E(400, "target_id", "target id required")
	}
	if err := s.validateAssignmentTarget(r.Context(), a.OrgID, x.TargetType, x.TargetID); err != nil {
		return err
	}
	if x.FromLevel != nil && !validLevel(*x.FromLevel) {
		return webx.E(400, "from_level", "invalid level")
	}
	if x.ToLevel != nil && !validLevel(*x.ToLevel) {
		return webx.E(400, "to_level", "invalid level")
	}
	if x.ServiceCode == "level_upgrade" {
		if x.FromLevel == nil || x.ToLevel == nil {
			return webx.E(400, "levels", "level upgrade requires from_level and to_level")
		}
		if !validUpgrade(*x.FromLevel, *x.ToLevel) {
			return webx.E(400, "levels", "supported upgrades are A1→A2, A2→B1, B1→B2 and B2→C1")
		}
	}
	cnt := defaultCount(x.ServiceCode)
	if x.QuestionCount != nil {
		cnt = *x.QuestionCount
	}
	if cnt < 1 || cnt > 80 {
		return webx.E(400, "question_count", "question count must be 1-80")
	}
	if x.ServiceCode == "speaking" && cnt > 5 {
		return webx.E(400, "question_count", "speaking supports up to 5 prompts")
	}
	if x.ServiceCode == "writing" && cnt > 3 {
		return webx.E(400, "question_count", "writing supports up to 3 prompts")
	}
	if x.DueAt != nil && x.DueAt.Before(time.Now()) {
		return webx.E(400, "due_at", "due date must be in the future")
	}
	title := strings.TrimSpace(x.Title)
	if title == "" {
		title = x.ServiceCode
	}
	var id uuid.UUID
	err = s.DB.QueryRow(r.Context(), `INSERT INTO assignments(organization_id,service_code,title,target_type,target_id,from_level,to_level,question_count,due_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, a.OrgID, x.ServiceCode, title, x.TargetType, x.TargetID, x.FromLevel, x.ToLevel, cnt, x.DueAt, a.UserID).Scan(&id)
	if err != nil {
		return err
	}
	s.emit(r.Context(), a.OrgID, "", x.ServiceCode, "assignment.created", map[string]any{"assignment_id": id.String(), "target_type": x.TargetType})
	webx.JSON(w, 201, map[string]any{"id": id.String()})
	return nil
}
func (s *Service) closeAssignment(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center_admin") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE assignments SET status='closed' WHERE id=$1 AND organization_id=$2`, r.PathValue("id"), a.OrgID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return webx.E(404, "assignment", "assignment not found")
	}
	w.WriteHeader(204)
	return nil
}
func (s *Service) studentGroups(ctx context.Context, a authz.Actor) ([]string, error) {
	var out struct {
		GroupIDs []string `json:"group_ids"`
	}
	if err := s.Tenant.Do(ctx, "GET", "/internal/student/"+a.UserID+"/groups?organization_id="+a.OrgID, nil, &out); err != nil {
		return nil, err
	}
	return out.GroupIDs, nil
}
func (s *Service) applicable(ctx context.Context, a authz.Actor, x Assignment) bool {
	if x.OrganizationID != a.OrgID || x.Status != "open" || time.Now().Before(x.OpensAt) || (x.DueAt != nil && time.Now().After(*x.DueAt)) {
		return false
	}
	if x.TargetType == "all" {
		return true
	}
	if x.TargetType == "student" {
		return x.TargetID != nil && *x.TargetID == a.UserID
	}
	if x.TargetType == "group" && x.TargetID != nil {
		gs, err := s.studentGroups(ctx, a)
		if err != nil {
			return false
		}
		for _, g := range gs {
			if g == *x.TargetID {
				return true
			}
		}
	}
	return false
}
func (s *Service) profile(ctx context.Context, user string) (struct {
	CurrentLevel *string `json:"current_level"`
}, error) {
	var p struct {
		CurrentLevel *string `json:"current_level"`
	}
	err := s.Identity.Do(ctx, "GET", "/internal/resolve?user_id="+user, nil, &p)
	return p, err
}
func nextLevel(l string) string {
	switch l {
	case "A1":
		return "A2"
	case "A2":
		return "B1"
	case "B1":
		return "B2"
	case "B2":
		return "C1"
	case "C1":
		return "C2"
	}
	return l
}
func lowerLevels(l string) []string {
	all := []string{"A1", "A2", "B1", "B2", "C1"}
	out := []string{}
	for _, x := range all {
		out = append(out, x)
		if x == l {
			break
		}
	}
	return out
}
func (s *Service) preferredWeak(ctx context.Context, org, user string) []string {
	rows, err := s.DB.Query(ctx, `SELECT subject_code FROM topic_mastery WHERE organization_id=$1 AND student_user_id=$2 ORDER BY mastery ASC, attempts DESC LIMIT 20`, org, user)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var x string
		if rows.Scan(&x) == nil {
			out = append(out, x)
		}
	}
	return out
}
func (s *Service) buildQuestions(ctx context.Context, code, current string, from, to *string, count int, seed, org, user string) ([]bank.Question, error) {
	levels := []string{}
	cats := []string{}
	preferred := []string{}
	switch code {
	case "placement":
		// Placement defaults to one question from each of the 80 subjects, but
		// custom counts must still be honored instead of silently returning 80.
		out := s.Bank.BuildBalanced([]string{"A1", "A2", "B1", "B2", "C1"}, nil, count, seed, nil)
		if len(out) < count {
			return nil, fmt.Errorf("question bank cannot build placement plan: got %d of %d", len(out), count)
		}
		return out, nil
	case "vocabulary_test":
		levels = []string{"A1", "A2", "B1", "B2", "C1"}
		cats = []string{"Vocabulary", "Use of English"}
	case "grammar":
		levels = []string{"A1", "A2", "B1", "B2", "C1"}
		cats = []string{"Grammar", "Advanced Grammar", "Use of English"}
	case "level_upgrade":
		if from == nil || to == nil {
			return nil, errors.New("level upgrade missing levels")
		}
		a := s.Bank.BuildBalanced([]string{*from}, nil, count/2, seed+":from", nil)
		b := s.Bank.BuildBalanced([]string{*to}, nil, count-len(a), seed+":to", nil)
		return append(a, b...), nil
	case "progress":
		if current == "" {
			current = "A1"
		}
		levels = []string{current}
		if count > 16 {
			levels = lowerLevels(current)
		}
	case "ielts_readiness":
		levels = []string{"B1", "B2", "C1"}
		cats = []string{"Grammar", "Advanced Grammar", "Vocabulary", "Use of English"}
	case "weakness":
		if current == "" {
			current = "A1"
		}
		levels = lowerLevels(current)
		preferred = s.preferredWeak(ctx, org, user)
	case "mock":
		levels = []string{"B1", "B2", "C1"}
	case "final_exit":
		target := current
		if to != nil {
			target = *to
		}
		if target == "" {
			target = "B1"
		}
		levels = lowerLevels(target)
	default:
		return nil, nil
	}
	out := s.Bank.BuildBalanced(levels, cats, count, seed, preferred)
	if len(out) < count {
		return nil, fmt.Errorf("question bank cannot build %s plan: got %d of %d", code, len(out), count)
	}
	return out, nil
}
func shuffleOrder(secret, attemptID, qid string, n int) []int {
	arr := make([]int, n)
	for i := range arr {
		arr[i] = i
	}
	h := sha256.Sum256([]byte(secret + ":" + attemptID + ":" + qid))
	r := rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(h[:8]))))
	r.Shuffle(n, func(i, j int) { arr[i], arr[j] = arr[j], arr[i] })
	return arr
}
func (s *Service) quote(ctx context.Context, code string, qs []bank.Question) map[string]float64 {
	ids := make([]string, 0, len(qs))
	for _, q := range qs {
		ids = append(ids, q.UUID)
	}
	var res struct {
		Items []struct {
			QuestionID string  `json:"question_id"`
			Multiplier float64 `json:"multiplier"`
		} `json:"items"`
	}
	if s.Points.Do(ctx, "POST", "/internal/quote/batch", map[string]any{"service_code": code, "question_ids": ids}, &res) != nil {
		return map[string]float64{}
	}
	m := map[string]float64{}
	for _, x := range res.Items {
		m[x.QuestionID] = x.Multiplier
	}
	return m
}
func (s *Service) reserve(ctx context.Context, org, code, key string) error {
	var out struct {
		Allowed   bool   `json:"allowed"`
		Reason    string `json:"reason"`
		Remaining int64  `json:"remaining"`
	}
	err := s.Tenant.Do(ctx, "POST", "/internal/usage/reserve", map[string]any{
		"organization_id": org, "service_code": code, "amount": 1,
		"reservation_key": key, "hold_concurrency": true, "lease_minutes": 180,
	}, &out)
	if err != nil {
		return webx.E(429, "quota", "service quota or concurrency limit reached")
	}
	if !out.Allowed {
		return webx.E(429, "quota", out.Reason)
	}
	return nil
}

func (s *Service) release(ctx context.Context, org, code, key string) {
	_ = s.Tenant.Do(ctx, "POST", "/internal/usage/release", map[string]any{
		"organization_id": org, "service_code": code, "reservation_key": key,
	}, nil)
}
func (s *Service) cancel(ctx context.Context, org, code, key string) {
	_ = s.Tenant.Do(ctx, "POST", "/internal/usage/cancel", map[string]any{
		"organization_id": org, "service_code": code, "reservation_key": key,
	}, nil)
}

func assessmentReservationKey(assignmentID, studentID string) string {
	return "assessment:" + assignmentID + ":" + studentID
}

var speakingPrompts = []string{"Introduce yourself and describe one goal you want to achieve this year.", "Describe a place in your city that you would recommend to a visitor.", "Do you think technology makes learning more effective? Explain your view.", "Describe a difficult decision you made and what you learned from it.", "Compare studying alone with studying in a group."}
var writingPrompts = []string{"Write 180-250 words about whether students learn better online or in a classroom. Give reasons and examples.", "Write 180-250 words describing a problem in your community and proposing a practical solution.", "Write 180-250 words discussing the advantages and disadvantages of using artificial intelligence in education."}

func manualPromptPlan(code string, count int) []ManualPrompt {
	items := []ManualPrompt{}
	if code == "speaking" {
		for i := 0; i < min(count, len(speakingPrompts)); i++ {
			items = append(items, ManualPrompt{PromptID: fmt.Sprintf("speaking-%d", i+1), Component: "speaking", Position: i + 1, PromptText: speakingPrompts[i], Required: true})
		}
	}
	if code == "writing" {
		for i := 0; i < min(count, len(writingPrompts)); i++ {
			items = append(items, ManualPrompt{PromptID: fmt.Sprintf("writing-%d", i+1), Component: "writing", Position: i + 1, PromptText: writingPrompts[i], Required: true})
		}
	}
	if code == "mock" {
		items = append(items,
			ManualPrompt{PromptID: "mock-speaking-1", Component: "speaking", Position: 1, PromptText: speakingPrompts[2], Required: true},
			ManualPrompt{PromptID: "mock-writing-1", Component: "writing", Position: 1, PromptText: writingPrompts[0], Required: true},
		)
	}
	return items
}

func (s *Service) manualPrompts(ctx context.Context, attemptID string) ([]ManualPrompt, error) {
	rows, err := s.DB.Query(ctx, `SELECT p.prompt_id,p.component,p.position,p.prompt_text,p.required,(r.submission_id IS NOT NULL)
		FROM manual_prompts p LEFT JOIN manual_submission_refs r ON r.attempt_id=p.attempt_id AND r.prompt_id=p.prompt_id
		WHERE p.attempt_id=$1 ORDER BY CASE p.component WHEN 'speaking' THEN 1 ELSE 2 END,p.position`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ManualPrompt{}
	for rows.Next() {
		var x ManualPrompt
		if err := rows.Scan(&x.PromptID, &x.Component, &x.Position, &x.PromptText, &x.Required, &x.Submitted); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

func (s *Service) requireManualSubmissions(ctx context.Context, attemptID string) error {
	var required, submitted int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FILTER(WHERE p.required),count(*) FILTER(WHERE p.required AND r.submission_id IS NOT NULL)
		FROM manual_prompts p LEFT JOIN manual_submission_refs r ON r.attempt_id=p.attempt_id AND r.prompt_id=p.prompt_id WHERE p.attempt_id=$1`, attemptID).Scan(&required, &submitted); err != nil {
		return err
	}
	if required > submitted {
		return webx.E(409, "manual_incomplete", fmt.Sprintf("%d required manual submission(s) are missing", required-submitted))
	}
	return nil
}

func (s *Service) start(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "student") != nil {
		return webx.E(403, "forbidden", "student required")
	}
	x, err := scanAssignment(s.DB.QueryRow(r.Context(), `SELECT id,organization_id,service_code,title,target_type,target_id,from_level,to_level,question_count,opens_at,due_at,status,created_by,created_at FROM assignments WHERE id=$1`, r.PathValue("id")))
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(404, "assignment", "assignment not found")
	}
	if err != nil {
		return err
	}
	if !s.applicable(r.Context(), a, x) {
		return webx.E(403, "assignment", "assignment is not available to this student")
	}
	var existing uuid.UUID
	err = s.DB.QueryRow(r.Context(), `SELECT id FROM attempts WHERE assignment_id=$1 AND student_user_id=$2`, x.ID, a.UserID).Scan(&existing)
	if err == nil {
		webx.JSON(w, 200, map[string]any{"attempt_id": existing.String(), "resumed": true})
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	reservationKey := assessmentReservationKey(x.ID, a.UserID)
	if err = s.reserve(r.Context(), a.OrgID, x.ServiceCode, reservationKey); err != nil {
		return err
	}
	provisioned := false
	defer func() {
		if !provisioned {
			s.cancel(context.Background(), a.OrgID, x.ServiceCode, reservationKey)
		}
	}()

	p, err := s.profile(r.Context(), a.UserID)
	if err != nil {
		return fmt.Errorf("load student level: %w", err)
	}
	current := "A1"
	if p.CurrentLevel != nil {
		current = *p.CurrentLevel
	}
	cnt := defaultCount(x.ServiceCode)
	if x.QuestionCount != nil {
		cnt = *x.QuestionCount
	}
	attemptID := uuid.New()
	plan := []PlanItem{}
	mode := "auto"
	if x.ServiceCode == "speaking" || x.ServiceCode == "writing" {
		mode = "manual"
	} else {
		qs, err := s.buildQuestions(r.Context(), x.ServiceCode, current, x.FromLevel, x.ToLevel, cnt, attemptID.String(), a.OrgID, a.UserID)
		if err != nil {
			return err
		}
		mult := s.quote(r.Context(), x.ServiceCode, qs)
		for _, q := range qs {
			sub := s.Bank.SubjectByUUID[q.SubjectUUID]
			m := mult[q.UUID]
			if m < 1 {
				m = 1
			}
			plan = append(plan, PlanItem{QuestionID: q.UUID, SubjectCode: sub.ShortName, Level: sub.Level, DisplayOrder: shuffleOrder(s.QuestionSecret, attemptID.String(), q.UUID, len(q.Options)), RushMultiplier: m})
		}
		if x.ServiceCode == "mock" {
			mode = "hybrid"
		}
	}
	manual := manualPromptPlan(x.ServiceCode, cnt)
	raw, _ := json.Marshal(plan)
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `INSERT INTO attempts(id,organization_id,assignment_id,student_user_id,service_code,bank_version,status,from_level,to_level,question_plan) VALUES($1,$2,$3,$4,$5,$6,'in_progress',$7,$8,$9)`, attemptID, a.OrgID, x.ID, a.UserID, x.ServiceCode, s.Bank.Version, x.FromLevel, x.ToLevel, raw)
	if err != nil {
		return err
	}
	for _, mp := range manual {
		if _, err = tx.Exec(r.Context(), `INSERT INTO manual_prompts(attempt_id,prompt_id,component,position,prompt_text,required) VALUES($1,$2,$3,$4,$5,$6)`, attemptID, mp.PromptID, mp.Component, mp.Position, mp.PromptText, mp.Required); err != nil {
			return err
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	provisioned = true
	s.emit(r.Context(), a.OrgID, a.UserID, x.ServiceCode, "assessment.started", map[string]any{"attempt_id": attemptID.String(), "assignment_id": x.ID})
	resp := map[string]any{"attempt_id": attemptID.String(), "mode": mode, "service_code": x.ServiceCode}
	if len(manual) > 0 {
		resp["manual_prompts"] = manual
	}
	webx.JSON(w, 201, resp)
	return nil
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func (s *Service) loadAttempt(ctx context.Context, id string) (Attempt, error) {
	var x Attempt
	var aid, org, user uuid.UUID
	var ass *uuid.UUID
	var raw []byte
	err := s.DB.QueryRow(ctx, `SELECT id,organization_id,assignment_id,student_user_id,service_code,bank_version,status,from_level,to_level,question_plan,auto_score,final_score,level_result,readiness,started_at,finished_at FROM attempts WHERE id=$1`, id).Scan(&aid, &org, &ass, &user, &x.ServiceCode, &x.BankVersion, &x.Status, &x.FromLevel, &x.ToLevel, &raw, &x.AutoScore, &x.FinalScore, &x.LevelResult, &x.Readiness, &x.StartedAt, &x.FinishedAt)
	if err != nil {
		return x, err
	}
	x.ID = aid.String()
	x.OrganizationID = org.String()
	x.StudentUserID = user.String()
	if ass != nil {
		v := ass.String()
		x.AssignmentID = &v
	}
	if err = json.Unmarshal(raw, &x.Plan); err != nil {
		return x, err
	}
	return x, nil
}
func (s *Service) ownership(a authz.Actor, x Attempt) error {
	if x.OrganizationID != a.OrgID {
		return webx.E(404, "attempt", "attempt not found")
	}
	if a.Role == "student" && x.StudentUserID != a.UserID {
		return webx.E(404, "attempt", "attempt not found")
	}
	if a.Role != "student" && a.Role != "center_admin" {
		return webx.E(403, "forbidden", "invalid role")
	}
	return nil
}
func (s *Service) current(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	x, err := s.loadAttempt(r.Context(), r.PathValue("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(404, "attempt", "attempt not found")
	}
	if err != nil {
		return err
	}
	if err = s.ownership(a, x); err != nil {
		return err
	}
	if a.Role == "center_admin" {
		webx.JSON(w, 200, x)
		return nil
	}
	if x.Status != "in_progress" {
		webx.JSON(w, 200, map[string]any{"attempt_id": x.ID, "status": x.Status, "service_code": x.ServiceCode})
		return nil
	}
	if x.ServiceCode == "speaking" || x.ServiceCode == "writing" {
		prompts, err := s.manualPrompts(r.Context(), x.ID)
		if err != nil {
			return err
		}
		webx.JSON(w, 200, map[string]any{"attempt_id": x.ID, "status": x.Status, "mode": "manual", "service_code": x.ServiceCode, "manual_prompts": prompts})
		return nil
	}
	answered := map[string]bool{}
	rows, err := s.DB.Query(r.Context(), `SELECT question_id FROM answers WHERE attempt_id=$1 AND answered_at IS NOT NULL`, x.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			answered[id.String()] = true
		}
	}
	rows.Close()
	for pos, p := range x.Plan {
		if answered[p.QuestionID] {
			continue
		}
		q, ok := s.Bank.Questions[p.QuestionID]
		if !ok {
			return errors.New("bank question missing")
		}
		opts := make([]string, len(p.DisplayOrder))
		for i, canon := range p.DisplayOrder {
			opts[i] = q.Options[canon]
		}
		webx.JSON(w, 200, map[string]any{"attempt_id": x.ID, "status": x.Status, "service_code": x.ServiceCode, "position": pos + 1, "answered": len(answered), "total": len(x.Plan), "question": map[string]any{"id": q.UUID, "text": q.Text, "options": opts, "rush_multiplier": p.RushMultiplier}})
		return nil
	}
	resp := map[string]any{"attempt_id": x.ID, "status": x.Status, "answered": len(x.Plan), "total": len(x.Plan), "ready_to_finish": true}
	if x.ServiceCode == "mock" {
		prompts, err := s.manualPrompts(r.Context(), x.ID)
		if err != nil {
			return err
		}
		resp["mode"] = "hybrid"
		resp["manual_prompts"] = prompts
	}
	webx.JSON(w, 200, resp)
	return nil
}
func (s *Service) answer(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "student") != nil {
		return webx.E(403, "forbidden", "student required")
	}
	x, err := s.loadAttempt(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if err = s.ownership(a, x); err != nil {
		return err
	}
	if x.Status != "in_progress" {
		return webx.E(409, "attempt", "attempt is not in progress")
	}
	var req struct {
		QuestionID string `json:"question_id"`
		Option     string `json:"option"`
		ResponseMS int    `json:"response_ms"`
	}
	if err := webx.Decode(r, &req, 64<<10); err != nil {
		return err
	}
	idx := strings.Index("ABCD", strings.ToUpper(req.Option))
	if idx < 0 {
		return webx.E(400, "option", "option must be A-D")
	}
	answered := map[string]bool{}
	rows, err := s.DB.Query(r.Context(), `SELECT question_id FROM answers WHERE attempt_id=$1 AND answered_at IS NOT NULL`, x.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			answered[id.String()] = true
		}
	}
	rows.Close()
	var pi *PlanItem
	expected := ""
	for i := range x.Plan {
		if !answered[x.Plan[i].QuestionID] {
			pi = &x.Plan[i]
			expected = x.Plan[i].QuestionID
			break
		}
	}
	if pi == nil {
		return webx.E(409, "complete", "all questions are already answered")
	}
	if req.QuestionID != expected {
		return webx.E(409, "stale_question", "question reference is stale")
	}
	q, ok := s.Bank.Questions[req.QuestionID]
	if !ok {
		return errors.New("question not found in bank")
	}
	if idx >= len(pi.DisplayOrder) {
		return webx.E(400, "option", "invalid option")
	}
	canon := pi.DisplayOrder[idx]
	correct := canon == q.CorrectIndex
	sub := s.Bank.SubjectByUUID[q.SubjectUUID]
	displayed := make([]string, len(pi.DisplayOrder))
	for i, c := range pi.DisplayOrder {
		displayed[i] = q.Options[c]
	}
	raw, _ := json.Marshal(displayed)
	ct, err := s.DB.Exec(r.Context(), `INSERT INTO answers(attempt_id,question_id,subject_code,displayed_options,selected_option,is_correct,base_points,rush_multiplier,response_ms,answered_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,now()) ON CONFLICT(attempt_id,question_id) DO NOTHING`, x.ID, q.UUID, sub.ShortName, raw, strings.ToUpper(req.Option), correct, sub.Point, pi.RushMultiplier, req.ResponseMS)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return webx.E(409, "stale_question", "question was already answered")
	}
	webx.JSON(w, 200, map[string]any{"accepted": true, "answered": len(answered) + 1, "remaining": len(x.Plan) - len(answered) - 1})
	return nil
}
func (s *Service) event(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "student") != nil {
		return webx.E(403, "forbidden", "student required")
	}
	x, err := s.loadAttempt(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if err = s.ownership(a, x); err != nil {
		return err
	}
	var req struct {
		Type     string         `json:"type"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := webx.Decode(r, &req, 64<<10); err != nil {
		return err
	}
	allowed := map[string]bool{"window_blur": true, "tab_hidden": true, "fullscreen_exit": true, "copy": true, "paste": true, "visibility_change": true}
	if !allowed[req.Type] {
		return webx.E(400, "event", "unsupported event type")
	}
	raw, _ := json.Marshal(req.Metadata)
	_, err = s.DB.Exec(r.Context(), `INSERT INTO anti_cheat_events(attempt_id,organization_id,student_user_id,event_type,metadata) VALUES($1,$2,$3,$4,$5)`, x.ID, a.OrgID, a.UserID, req.Type, raw)
	if err != nil {
		return err
	}
	webx.JSON(w, 202, map[string]any{"accepted": true})
	return nil
}

type breakdown struct {
	Attempts  int     `json:"attempts"`
	Correct   int     `json:"correct"`
	Accuracy  float64 `json:"accuracy"`
	Points    float64 `json:"points"`
	MaxPoints float64 `json:"max_points"`
}

func (s *Service) finish(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "student") != nil {
		return webx.E(403, "forbidden", "student required")
	}
	x, err := s.loadAttempt(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if err = s.ownership(a, x); err != nil {
		return err
	}
	if x.Status != "in_progress" {
		return webx.E(409, "attempt", "attempt already finished")
	}
	if x.ServiceCode == "speaking" || x.ServiceCode == "writing" {
		if err = s.requireManualSubmissions(r.Context(), x.ID); err != nil {
			return err
		}
		_, err = s.DB.Exec(r.Context(), `UPDATE attempts SET status='pending_review',finished_at=now() WHERE id=$1`, x.ID)
		if err != nil {
			return err
		}
		if x.AssignmentID != nil {
			s.release(r.Context(), a.OrgID, x.ServiceCode, assessmentReservationKey(*x.AssignmentID, a.UserID))
		}
		s.emit(r.Context(), a.OrgID, a.UserID, x.ServiceCode, "assessment.pending_review", map[string]any{"attempt_id": x.ID})
		webx.JSON(w, 200, map[string]any{"status": "pending_review", "attempt_id": x.ID})
		return nil
	}
	if x.ServiceCode == "mock" {
		if err = s.requireManualSubmissions(r.Context(), x.ID); err != nil {
			return err
		}
	}
	rows, err := s.DB.Query(r.Context(), `SELECT q.question_id,q.subject_code,q.is_correct,q.base_points,q.rush_multiplier FROM answers q WHERE q.attempt_id=$1`, x.ID)
	if err != nil {
		return err
	}
	type ar struct {
		QID, Subject string
		Correct      bool
		Base, Rush   float64
	}
	ans := []ar{}
	for rows.Next() {
		var v ar
		var qid uuid.UUID
		if err := rows.Scan(&qid, &v.Subject, &v.Correct, &v.Base, &v.Rush); err != nil {
			return err
		}
		v.QID = qid.String()
		ans = append(ans, v)
	}
	rows.Close()
	if len(ans) != len(x.Plan) {
		return webx.E(409, "incomplete", "all questions must be answered before finishing")
	}
	byLevel := map[string]*breakdown{}
	byTopic := map[string]*breakdown{}
	correctN := 0
	earned, maxp := 0.0, 0.0
	for _, v := range ans {
		sub, ok := s.Bank.SubjectByCode[v.Subject]
		if !ok {
			return fmt.Errorf("bank subject %q missing", v.Subject)
		}
		lb := byLevel[sub.Level]
		if lb == nil {
			lb = &breakdown{}
			byLevel[sub.Level] = lb
		}
		lb.Attempts++
		lb.MaxPoints += v.Base

		tb := byTopic[v.Subject]
		if tb == nil {
			tb = &breakdown{}
			byTopic[v.Subject] = tb
		}
		tb.Attempts++
		tb.MaxPoints += v.Base
		if v.Correct {
			lb.Correct++
			lb.Points += v.Base
			tb.Correct++
			tb.Points += v.Base
		}
		maxp += v.Base
		if v.Correct {
			correctN++
			earned += v.Base
		}
		_ = s.Points.Do(r.Context(), "POST", "/internal/record", map[string]any{"organization_id": a.OrgID, "student_user_id": a.UserID, "service_code": x.ServiceCode, "question_id": v.QID, "event_key": "assessment:" + x.ID + ":" + v.QID, "base_points": v.Base, "multiplier": v.Rush, "correct": v.Correct, "reason": "assessment_answer"}, nil)
	}
	for _, m := range []map[string]*breakdown{byLevel, byTopic} {
		for _, b := range m {
			if b.Attempts > 0 {
				b.Accuracy = 100 * float64(b.Correct) / float64(b.Attempts)
			}
		}
	}
	percent := 0.0
	if len(ans) > 0 {
		percent = 100 * float64(correctN) / float64(len(ans))
	}
	levelResult := s.levelFrom(byLevel)
	readiness := percent
	status := "completed"
	if x.ServiceCode == "level_upgrade" {
		fromAcc, toAcc := 0.0, 0.0
		if x.FromLevel != nil && byLevel[*x.FromLevel] != nil {
			fromAcc = byLevel[*x.FromLevel].Accuracy
		}
		if x.ToLevel != nil && byLevel[*x.ToLevel] != nil {
			toAcc = byLevel[*x.ToLevel].Accuracy
		}
		readiness = (fromAcc + toAcc) / 2
		if fromAcc >= 75 && toAcc >= 60 && x.ToLevel != nil {
			levelResult = *x.ToLevel
			_ = s.Identity.Do(r.Context(), "PATCH", "/internal/users/"+a.UserID+"/level", map[string]any{"organization_id": a.OrgID, "level": *x.ToLevel}, nil)
		} else if x.FromLevel != nil {
			levelResult = *x.FromLevel
		}
	}
	if x.ServiceCode == "placement" && levelResult != "" {
		_ = s.Identity.Do(r.Context(), "PATCH", "/internal/users/"+a.UserID+"/level", map[string]any{"organization_id": a.OrgID, "level": levelResult}, nil)
	}
	if x.ServiceCode == "ielts_readiness" {
		readiness = percent
	}
	if x.ServiceCode == "mock" {
		status = "pending_review"
	}
	final := percent
	_, err = s.DB.Exec(r.Context(), `UPDATE attempts SET status=$2,auto_score=$3,final_score=$3,level_result=$4,readiness=$5,finished_at=now() WHERE id=$1`, x.ID, status, final, levelResult, readiness)
	if err != nil {
		return err
	}
	for topic, b := range byTopic {
		_, _ = s.DB.Exec(r.Context(), `INSERT INTO topic_mastery(organization_id,student_user_id,subject_code,attempts,correct,mastery) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(organization_id,student_user_id,subject_code) DO UPDATE SET attempts=topic_mastery.attempts+$4,correct=topic_mastery.correct+$5,mastery=(topic_mastery.correct+$5)::numeric/(topic_mastery.attempts+$4),updated_at=now()`, a.OrgID, a.UserID, topic, b.Attempts, b.Correct, float64(b.Correct)/float64(b.Attempts))
	}
	if x.AssignmentID != nil {
		s.release(r.Context(), a.OrgID, x.ServiceCode, assessmentReservationKey(*x.AssignmentID, a.UserID))
	}
	s.emit(r.Context(), a.OrgID, a.UserID, x.ServiceCode, "assessment.completed", map[string]any{"attempt_id": x.ID, "score": percent, "level": levelResult, "status": status})
	webx.JSON(w, 200, map[string]any{"attempt_id": x.ID, "status": status, "score": percent, "level": levelResult, "readiness": readiness, "by_level": byLevel, "by_topic": byTopic, "base_points": earned, "max_base_points": maxp})
	return nil
}
func (s *Service) levelFrom(m map[string]*breakdown) string {
	levels := []string{"A1", "A2", "B1", "B2", "C1"}
	result := "A1"
	lowerOK := true
	for _, l := range levels {
		b := m[l]
		if b == nil {
			continue
		}
		if b.Accuracy >= 60 && lowerOK {
			result = l
		} else if b.Accuracy < 50 {
			lowerOK = false
		}
	}
	return result
}
func (s *Service) history(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	var student string
	if a.Role == "student" {
		student = a.UserID
	} else if a.Role == "center_admin" {
		student = r.URL.Query().Get("student_user_id")
		if student == "" {
			return webx.E(400, "student", "student_user_id required")
		}
	} else {
		return webx.E(403, "forbidden", "invalid role")
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,service_code,status,auto_score,final_score,level_result,readiness,started_at,finished_at FROM attempts WHERE organization_id=$1 AND student_user_id=$2 ORDER BY started_at DESC LIMIT 200`, a.OrgID, student)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var code, status string
		var auto float64
		var final *float64
		var level *string
		var readiness *float64
		var started time.Time
		var finished *time.Time
		if err := rows.Scan(&id, &code, &status, &auto, &final, &level, &readiness, &started, &finished); err != nil {
			return err
		}
		items = append(items, map[string]any{"id": id.String(), "service_code": code, "status": status, "auto_score": auto, "final_score": final, "level": level, "readiness": readiness, "started_at": started, "finished_at": finished})
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}
func (s *Service) progress(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	student := a.UserID
	if a.Role == "center_admin" {
		student = r.URL.Query().Get("student_user_id")
	} else if a.Role != "student" {
		return webx.E(403, "forbidden", "invalid role")
	}
	if student == "" {
		return webx.E(400, "student", "student required")
	}
	rows, err := s.DB.Query(r.Context(), `SELECT service_code,date_trunc('day',finished_at),avg(coalesce(final_score,auto_score)) FROM attempts WHERE organization_id=$1 AND student_user_id=$2 AND finished_at IS NOT NULL GROUP BY 1,2 ORDER BY 2`, a.OrgID, student)
	if err != nil {
		return err
	}
	items := []map[string]any{}
	for rows.Next() {
		var code string
		var day time.Time
		var avg float64
		if err := rows.Scan(&code, &day, &avg); err != nil {
			rows.Close()
			return err
		}
		items = append(items, map[string]any{"service_code": code, "day": day, "score": avg})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	masteryRows, err := s.DB.Query(r.Context(), `SELECT subject_code,attempts,correct,mastery,updated_at FROM topic_mastery WHERE organization_id=$1 AND student_user_id=$2 ORDER BY mastery ASC,attempts DESC,subject_code`, a.OrgID, student)
	if err != nil {
		return err
	}
	defer masteryRows.Close()
	mastery := []map[string]any{}
	for masteryRows.Next() {
		var subject string
		var attempts, correct int
		var score float64
		var updated time.Time
		if err := masteryRows.Scan(&subject, &attempts, &correct, &score, &updated); err != nil {
			return err
		}
		mastery = append(mastery, map[string]any{
			"subject_code": subject,
			"attempts":     attempts,
			"correct":      correct,
			"mastery":      score,
			"updated_at":   updated,
		})
	}
	if err := masteryRows.Err(); err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"items": items, "topic_mastery": mastery})
	return nil
}
func (s *Service) internalAttempt(w http.ResponseWriter, r *http.Request) error {
	if err := authz.VerifyService(r, s.InternalSecret, "review"); err != nil {
		return webx.E(401, "service_auth", "invalid service signature")
	}
	var id, org, student uuid.UUID
	var code, status string
	var auto float64
	err := s.DB.QueryRow(r.Context(), `SELECT id,organization_id,student_user_id,service_code,status,auto_score FROM attempts WHERE id=$1`, r.PathValue("id")).Scan(&id, &org, &student, &code, &status, &auto)
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(404, "attempt", "attempt not found")
	}
	if err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"id": id.String(), "organization_id": org.String(), "student_user_id": student.String(), "service_code": code, "status": status, "auto_score": auto})
	return nil
}
func (s *Service) internalManualPrompts(w http.ResponseWriter, r *http.Request) error {
	if err := authz.VerifyService(r, s.InternalSecret, "review"); err != nil {
		return webx.E(401, "service_auth", "invalid service signature")
	}
	items, err := s.manualPrompts(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return webx.E(404, "manual_prompts", "manual prompts not found")
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return nil
}

func (s *Service) internalRegisterManualSubmission(w http.ResponseWriter, r *http.Request) error {
	if err := authz.VerifyService(r, s.InternalSecret, "review"); err != nil {
		return webx.E(401, "service_auth", "invalid service signature")
	}
	var req struct {
		PromptID     string `json:"prompt_id"`
		SubmissionID string `json:"submission_id"`
	}
	if err := webx.Decode(r, &req, 64<<10); err != nil {
		return err
	}
	var status string
	if err := s.DB.QueryRow(r.Context(), `SELECT a.status FROM attempts a JOIN manual_prompts p ON p.attempt_id=a.id WHERE a.id=$1 AND p.prompt_id=$2`, r.PathValue("id"), req.PromptID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return webx.E(404, "prompt", "manual prompt not found")
	} else if err != nil {
		return err
	}
	if status != "in_progress" {
		return webx.E(409, "attempt", "attempt is not accepting submissions")
	}
	sid, err := uuid.Parse(req.SubmissionID)
	if err != nil {
		return webx.E(400, "submission_id", "invalid submission id")
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO manual_submission_refs(attempt_id,prompt_id,submission_id) VALUES($1,$2,$3)`, r.PathValue("id"), req.PromptID, sid)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return webx.E(409, "submission", "prompt already submitted")
		}
		return err
	}
	webx.JSON(w, 201, map[string]any{"registered": true})
	return nil
}

func (s *Service) internalReview(w http.ResponseWriter, r *http.Request) error {
	if err := authz.VerifyService(r, s.InternalSecret, "review"); err != nil {
		return webx.E(401, "service_auth", "invalid service signature")
	}
	var x struct {
		ManualScore float64 `json:"manual_score"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if x.ManualScore < 0 || x.ManualScore > 100 {
		return webx.E(400, "score", "score must be 0-100")
	}
	var code string
	var auto float64
	var orgID, studentID uuid.UUID
	err := s.DB.QueryRow(r.Context(), `SELECT service_code,auto_score,organization_id,student_user_id FROM attempts WHERE id=$1 AND status='pending_review' FOR UPDATE`, r.PathValue("id")).Scan(&code, &auto, &orgID, &studentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(409, "attempt", "attempt is not pending review")
	}
	if err != nil {
		return err
	}
	final := x.ManualScore
	if code == "mock" {
		final = 0.70*auto + 0.30*x.ManualScore
	}
	_, err = s.DB.Exec(r.Context(), `UPDATE attempts SET status='completed',final_score=$2,finished_at=coalesce(finished_at,now()) WHERE id=$1`, r.PathValue("id"), final)
	if err != nil {
		return err
	}
	s.emit(r.Context(), orgID.String(), studentID.String(), code, "assessment.review_completed", map[string]any{"attempt_id": r.PathValue("id"), "final_score": final, "manual_score": x.ManualScore, "auto_score": auto})
	webx.JSON(w, 200, map[string]any{"status": "completed", "final_score": final, "manual_score": x.ManualScore, "auto_score": auto})
	return nil
}

func (s *Service) emit(ctx context.Context, org, user, code, typ string, payload map[string]any) {
	if s.Analytics == nil {
		return
	}
	evt := map[string]any{"event_id": uuid.NewString(), "organization_id": org, "event_type": typ, "service_code": code, "occurred_at": time.Now().UTC(), "payload": payload}
	if user != "" {
		evt["student_user_id"] = user
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = s.Analytics.Do(c, "POST", "/internal/events", evt, nil)
}
