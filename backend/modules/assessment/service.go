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
	"os"
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
	DB                                  *pgxpool.Pool
	Bank                                *bank.EnglishBank
	Tenant, Identity, Points, Analytics *clientx.Client
	InternalSecret, QuestionSecret      string
	PlacementPaperPath                  string
	PlacementPaperManifestPath          string
	PlacementInvitationTTL              time.Duration
	PlacementSessionTTL                 time.Duration
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

var englishServices = map[string]bool{"placement": true, "level_upgrade": true, "progress": true, "grammar": true, "ielts_readiness": true, "weakness": true, "speaking": true, "writing": true, "mock": true, "final_exit": true}

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
	m.HandleFunc("GET /v1/pre-registration/placements", webx.Handle(s.listPreRegistrationPlacements))
	m.HandleFunc("POST /v1/pre-registration/placements", webx.Handle(s.createPreRegistrationPlacement))
	m.HandleFunc("GET /v1/pre-registration/placements/{id}", webx.Handle(s.getPreRegistrationPlacement))
	m.HandleFunc("POST /v1/pre-registration/placements/{id}/invitation", webx.Handle(s.reissuePreRegistrationInvitation))
	m.HandleFunc("POST /v1/pre-registration/placements/{id}/finish", webx.Handle(s.finishPreRegistrationPlacement))
	m.HandleFunc("POST /v1/pre-registration/placements/{id}/registered", webx.Handle(s.markPreRegistrationPlacementRegistered))
	m.HandleFunc("GET /v1/pre-registration/placement-paper", webx.Handle(s.downloadPlacementPaper))
	m.HandleFunc("POST /v1/public/placement/invitations/claim", webx.Handle(s.claimPreRegistrationInvitation))
	m.HandleFunc("GET /v1/public/placement/session", webx.Handle(s.getCandidatePlacementSession))
	m.HandleFunc("POST /v1/public/placement/session/answer", webx.Handle(s.saveCandidatePlacementAnswer))
	m.HandleFunc("POST /v1/public/placement/session/finish", webx.Handle(s.finishCandidatePlacement))
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
	if a.Role != "admin" && a.Role != "center" && a.Role != "teacher" && a.Role != "student" {
		return webx.E(403, "forbidden", "invalid role")
	}
	items := []map[string]any{{"code": "placement", "name": "Level Placement Test", "default_questions": 80, "mode": "auto"}, {"code": "level_upgrade", "name": "Level Upgrade Test", "default_questions": 40, "mode": "auto"}, {"code": "progress", "name": "Progress Test", "default_questions": 30, "mode": "auto"}, {"code": "grammar", "name": "Grammar Diagnostic", "default_questions": 40, "mode": "auto"}, {"code": "ielts_readiness", "name": "IELTS Readiness", "default_questions": 40, "mode": "auto"}, {"code": "weakness", "name": "Weakness Diagnostic", "default_questions": 30, "mode": "adaptive"}, {"code": "speaking", "name": "Speaking Assessment", "default_questions": 3, "mode": "manual"}, {"code": "writing", "name": "Writing Assessment", "default_questions": 2, "mode": "manual"}, {"code": "mock", "name": "IELTS-style Mock", "default_questions": 60, "mode": "hybrid"}, {"code": "final_exit", "name": "Final / Exit Assessment", "default_questions": 60, "mode": "auto"}}
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
	case "level_upgrade", "grammar", "ielts_readiness":
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
	if a.Role == "center" || a.Role == "teacher" {
		query := `SELECT id,organization_id,service_code,title,target_type,target_id,from_level,to_level,question_count,opens_at,due_at,status,created_by,created_at FROM assignments WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 500`
		args := []any{a.OrgID}
		if a.Role == "teacher" {
			query = `SELECT id,organization_id,service_code,title,target_type,target_id,from_level,to_level,question_count,opens_at,due_at,status,created_by,created_at FROM assignments WHERE organization_id=$1 AND created_by=$2 ORDER BY created_at DESC LIMIT 500`
			args = append(args, a.UserID)
		}
		rows, err := s.DB.Query(r.Context(), query, args...)
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
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if a.Role != "center" && a.Role != "teacher" {
		return webx.E(403, "forbidden", "center admin or teacher required")
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
	if a.Role != "center" && a.Role != "teacher" {
		return webx.E(403, "forbidden", "center admin or teacher required")
	}
	query := `UPDATE assignments SET status='closed' WHERE id=$1 AND organization_id=$2`
	args := []any{r.PathValue("id"), a.OrgID}
	if a.Role == "teacher" {
		query += ` AND created_by=$3`
		args = append(args, a.UserID)
	}
	ct, err := s.DB.Exec(r.Context(), query, args...)
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
	if a.Role != "student" && a.Role != "center" {
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
	if a.Role == "center" {
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
	resp := map[string]any{"attempt_id": x.ID, "status": x.Status, "service_code": x.ServiceCode, "answered": len(x.Plan), "total": len(x.Plan), "ready_to_finish": true}
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
	} else if a.Role == "center" {
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
	if a.Role == "center" {
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

type PreRegistrationPlacement struct {
	ID                      string            `json:"id"`
	OrganizationID          string            `json:"organization_id"`
	CreatedBy               string            `json:"created_by"`
	FullName                string            `json:"full_name"`
	ContactEmail            *string           `json:"contact_email,omitempty"`
	ContactPhone            *string           `json:"contact_phone,omitempty"`
	Mode                    string            `json:"mode"`
	BankVersion             string            `json:"bank_version"`
	Plan                    []PlanItem        `json:"plan,omitempty"`
	Answers                 map[string]string `json:"-"`
	Status                  string            `json:"status"`
	Score                   *float64          `json:"score,omitempty"`
	LevelResult             *string           `json:"level_result,omitempty"`
	RegisteredUserID        *string           `json:"registered_user_id,omitempty"`
	InvitationExpiresAt     *time.Time        `json:"invitation_expires_at,omitempty"`
	InvitationClaimedAt     *time.Time        `json:"invitation_claimed_at,omitempty"`
	CandidateSessionExpires *time.Time        `json:"candidate_session_expires_at,omitempty"`
	CandidateLastSeenAt     *time.Time        `json:"candidate_last_seen_at,omitempty"`
	StartedAt               time.Time         `json:"started_at"`
	CompletedAt             *time.Time        `json:"completed_at,omitempty"`
	RegisteredAt            *time.Time        `json:"registered_at,omitempty"`
}

type PlacementQuestionView struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Options     []string `json:"options"`
	Level       string   `json:"level"`
	SubjectCode string   `json:"subject_code"`
}

type CandidatePlacementQuestionView struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Options []string `json:"options"`
}

type placementResult struct {
	Score   float64               `json:"score"`
	Level   string                `json:"level"`
	ByLevel map[string]*breakdown `json:"by_level"`
	ByTopic map[string]*breakdown `json:"by_topic"`
	Clean   map[string]string     `json:"-"`
}

func opaquePlacementToken() string {
	// Two UUIDv4 values provide ~244 random bits while remaining URL/QR friendly.
	return uuid.NewString() + "." + uuid.NewString()
}

func placementTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum[:])
}

func (s *Service) invitationTTL() time.Duration {
	if s.PlacementInvitationTTL > 0 {
		return s.PlacementInvitationTTL
	}
	return 24 * time.Hour
}

func (s *Service) candidateSessionTTL() time.Duration {
	if s.PlacementSessionTTL > 0 {
		return s.PlacementSessionTTL
	}
	return 2 * time.Hour
}

func (s *Service) loadPreRegistrationPlacement(ctx context.Context, id string) (PreRegistrationPlacement, error) {
	var x PreRegistrationPlacement
	var placementID, orgID, createdBy uuid.UUID
	var registered *uuid.UUID
	var rawPlan, rawAnswers []byte
	err := s.DB.QueryRow(ctx, `SELECT id,organization_id,created_by,full_name,contact_email,contact_phone,mode,bank_version,question_plan,answers,status,score,level_result,registered_user_id,invitation_expires_at,invitation_claimed_at,candidate_session_expires_at,candidate_last_seen_at,started_at,completed_at,registered_at FROM pre_registration_placements WHERE id=$1`, id).
		Scan(&placementID, &orgID, &createdBy, &x.FullName, &x.ContactEmail, &x.ContactPhone, &x.Mode, &x.BankVersion, &rawPlan, &rawAnswers, &x.Status, &x.Score, &x.LevelResult, &registered, &x.InvitationExpiresAt, &x.InvitationClaimedAt, &x.CandidateSessionExpires, &x.CandidateLastSeenAt, &x.StartedAt, &x.CompletedAt, &x.RegisteredAt)
	if err != nil {
		return x, err
	}
	x.ID = placementID.String()
	x.OrganizationID = orgID.String()
	x.CreatedBy = createdBy.String()
	if registered != nil {
		v := registered.String()
		x.RegisteredUserID = &v
	}
	if err := json.Unmarshal(rawPlan, &x.Plan); err != nil {
		return x, err
	}
	x.Answers = map[string]string{}
	if len(rawAnswers) > 0 {
		if err := json.Unmarshal(rawAnswers, &x.Answers); err != nil {
			return x, err
		}
	}
	return x, nil
}

func (s *Service) placementQuestionViews(plan []PlanItem) ([]PlacementQuestionView, error) {
	items := make([]PlacementQuestionView, 0, len(plan))
	for _, p := range plan {
		q, ok := s.Bank.Questions[p.QuestionID]
		if !ok {
			return nil, fmt.Errorf("placement question %s missing from bank", p.QuestionID)
		}
		sub, ok := s.Bank.SubjectByUUID[q.SubjectUUID]
		if !ok {
			return nil, fmt.Errorf("placement subject %s missing from bank", q.SubjectUUID)
		}
		options := make([]string, len(p.DisplayOrder))
		for i, canon := range p.DisplayOrder {
			if canon < 0 || canon >= len(q.Options) {
				return nil, fmt.Errorf("placement option order is invalid for %s", q.UUID)
			}
			options[i] = q.Options[canon]
		}
		items = append(items, PlacementQuestionView{ID: q.UUID, Text: q.Text, Options: options, Level: sub.Level, SubjectCode: sub.ShortName})
	}
	return items, nil
}

func (s *Service) paperPlacementQuestions() ([]bank.Question, error) {
	path := strings.TrimSpace(s.PlacementPaperManifestPath)
	if path == "" {
		path = "data/placement/paper-v1.json"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read paper placement manifest: %w", err)
	}
	var manifest struct {
		BankVersion string   `json:"bank_version"`
		QuestionIDs []string `json:"question_ids"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("decode paper placement manifest: %w", err)
	}
	if manifest.BankVersion != "" && manifest.BankVersion != s.Bank.Version {
		return nil, fmt.Errorf("paper placement bank version %q does not match runtime bank %q", manifest.BankVersion, s.Bank.Version)
	}
	if len(manifest.QuestionIDs) == 0 {
		return nil, errors.New("paper placement manifest contains no questions")
	}
	items := make([]bank.Question, 0, len(manifest.QuestionIDs))
	seen := map[string]bool{}
	for _, id := range manifest.QuestionIDs {
		if seen[id] {
			return nil, fmt.Errorf("paper placement manifest contains duplicate question %s", id)
		}
		seen[id] = true
		q, ok := s.Bank.Questions[id]
		if !ok {
			return nil, fmt.Errorf("paper placement question %s missing from bank", id)
		}
		items = append(items, q)
	}
	return items, nil
}

func naturalOrder(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func (s *Service) listPreRegistrationPlacements(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,full_name,contact_email,contact_phone,mode,status,score,level_result,registered_user_id,invitation_expires_at,invitation_claimed_at,candidate_session_expires_at,candidate_last_seen_at,(SELECT COUNT(*)::int FROM jsonb_object_keys(COALESCE(answers, '{}'::jsonb))),jsonb_array_length(question_plan),started_at,completed_at,registered_at FROM pre_registration_placements WHERE organization_id=$1 ORDER BY started_at DESC LIMIT 100`, a.OrgID)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var fullName, mode, status string
		var email, phone, level *string
		var score *float64
		var registered *uuid.UUID
		var invitationExpires, invitationClaimed, sessionExpires, lastSeen *time.Time
		var answeredCount, questionCount int
		var started time.Time
		var completed, registeredAt *time.Time
		if err := rows.Scan(&id, &fullName, &email, &phone, &mode, &status, &score, &level, &registered, &invitationExpires, &invitationClaimed, &sessionExpires, &lastSeen, &answeredCount, &questionCount, &started, &completed, &registeredAt); err != nil {
			return err
		}
		item := map[string]any{"id": id.String(), "full_name": fullName, "contact_email": email, "contact_phone": phone, "mode": mode, "status": status, "score": score, "level_result": level, "invitation_expires_at": invitationExpires, "invitation_claimed_at": invitationClaimed, "candidate_session_expires_at": sessionExpires, "candidate_last_seen_at": lastSeen, "answered_count": answeredCount, "question_count": questionCount, "started_at": started, "completed_at": completed, "registered_at": registeredAt}
		if registered != nil {
			item["registered_user_id"] = registered.String()
		}
		items = append(items, item)
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}

func (s *Service) createPreRegistrationPlacement(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	var req struct {
		FullName      string `json:"full_name"`
		ContactEmail  string `json:"contact_email"`
		ContactPhone  string `json:"contact_phone"`
		Mode          string `json:"mode"`
		QuestionCount int    `json:"question_count"`
	}
	if err := webx.Decode(r, &req, 128<<10); err != nil {
		return err
	}
	req.FullName = strings.TrimSpace(req.FullName)
	req.ContactEmail = strings.ToLower(strings.TrimSpace(req.ContactEmail))
	req.ContactPhone = strings.TrimSpace(req.ContactPhone)
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.FullName == "" || len(req.FullName) > 120 {
		return webx.E(400, "full_name", "candidate full name is required")
	}
	if req.ContactEmail != "" && (!strings.Contains(req.ContactEmail, "@") || len(req.ContactEmail) > 254) {
		return webx.E(400, "contact_email", "valid contact email required")
	}
	if len(req.ContactPhone) > 40 {
		return webx.E(400, "contact_phone", "phone number is too long")
	}
	if req.Mode == "" {
		req.Mode = "digital"
	}
	if req.Mode != "digital" && req.Mode != "paper" {
		return webx.E(400, "mode", "mode must be digital or paper")
	}
	placementID := uuid.New()
	var questions []bank.Question
	if req.Mode == "paper" {
		questions, err = s.paperPlacementQuestions()
		if err != nil {
			return err
		}
	} else {
		count := req.QuestionCount
		if count == 0 {
			count = 40
		}
		if count < 20 || count > 60 {
			return webx.E(400, "question_count", "digital placement question count must be 20-60")
		}
		questions, err = s.buildQuestions(r.Context(), "placement", "", nil, nil, count, placementID.String(), a.OrgID, "")
		if err != nil {
			return err
		}
	}
	plan := make([]PlanItem, 0, len(questions))
	for _, q := range questions {
		sub, ok := s.Bank.SubjectByUUID[q.SubjectUUID]
		if !ok {
			return fmt.Errorf("placement subject %s missing", q.SubjectUUID)
		}
		order := naturalOrder(len(q.Options))
		if req.Mode == "digital" {
			order = shuffleOrder(s.QuestionSecret, placementID.String(), q.UUID, len(q.Options))
		}
		plan = append(plan, PlanItem{QuestionID: q.UUID, SubjectCode: sub.ShortName, Level: sub.Level, DisplayOrder: order, RushMultiplier: 1})
	}
	rawPlan, _ := json.Marshal(plan)
	var email, phone any
	if req.ContactEmail != "" {
		email = req.ContactEmail
	}
	if req.ContactPhone != "" {
		phone = req.ContactPhone
	}
	var invitationToken string
	var invitationHash any
	var invitationExpires any
	if req.Mode == "digital" {
		invitationToken = opaquePlacementToken()
		invitationHash = placementTokenHash(invitationToken)
		invitationExpires = time.Now().UTC().Add(s.invitationTTL())
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO pre_registration_placements(id,organization_id,created_by,full_name,contact_email,contact_phone,mode,bank_version,question_plan,invitation_token_hash,invitation_expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, placementID, a.OrgID, a.UserID, req.FullName, email, phone, req.Mode, s.Bank.Version, rawPlan, invitationHash, invitationExpires)
	if err != nil {
		return err
	}
	s.emit(r.Context(), a.OrgID, "", "placement", "placement.preregistration.started", map[string]any{"placement_id": placementID.String(), "mode": req.Mode, "question_count": len(plan)})
	return s.writePreRegistrationPlacementWithToken(w, r, a, placementID.String(), invitationToken)
}

func (s *Service) preRegistrationPayload(x PreRegistrationPlacement, includePaperQuestions bool) (map[string]any, error) {
	payload := map[string]any{
		"id": x.ID, "full_name": x.FullName, "contact_email": x.ContactEmail, "contact_phone": x.ContactPhone,
		"mode": x.Mode, "status": x.Status, "bank_version": x.BankVersion, "score": x.Score, "level_result": x.LevelResult,
		"registered_user_id": x.RegisteredUserID, "started_at": x.StartedAt, "completed_at": x.CompletedAt, "registered_at": x.RegisteredAt,
		"invitation_expires_at": x.InvitationExpiresAt, "invitation_claimed_at": x.InvitationClaimedAt,
		"candidate_session_expires_at": x.CandidateSessionExpires, "candidate_last_seen_at": x.CandidateLastSeenAt,
		"question_count": len(x.Plan), "answered_count": len(x.Answers),
	}
	if includePaperQuestions {
		questions, err := s.placementQuestionViews(x.Plan)
		if err != nil {
			return nil, err
		}
		payload["questions"] = questions
	}
	return payload, nil
}

func (s *Service) writePreRegistrationPlacementWithToken(w http.ResponseWriter, r *http.Request, a authz.Actor, id, invitationToken string) error {
	x, err := s.loadPreRegistrationPlacement(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && x.OrganizationID != a.OrgID) {
		return webx.E(404, "placement", "pre-registration placement not found")
	}
	if err != nil {
		return err
	}
	payload, err := s.preRegistrationPayload(x, x.Mode == "paper")
	if err != nil {
		return err
	}
	if invitationToken != "" {
		payload["invitation_token"] = invitationToken
	}
	webx.JSON(w, 200, payload)
	return nil
}

func (s *Service) writePreRegistrationPlacement(w http.ResponseWriter, r *http.Request, a authz.Actor, id string) error {
	return s.writePreRegistrationPlacementWithToken(w, r, a, id, "")
}

func (s *Service) getPreRegistrationPlacement(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	if _, err := uuid.Parse(r.PathValue("id")); err != nil {
		return webx.E(400, "placement", "invalid placement id")
	}
	return s.writePreRegistrationPlacement(w, r, a, r.PathValue("id"))
}

func (s *Service) reissuePreRegistrationInvitation(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		return webx.E(400, "placement", "invalid placement id")
	}
	token := opaquePlacementToken()
	hash := placementTokenHash(token)
	expires := time.Now().UTC().Add(s.invitationTTL())
	ct, err := s.DB.Exec(r.Context(), `UPDATE pre_registration_placements SET invitation_token_hash=$3,invitation_expires_at=$4,invitation_claimed_at=NULL,candidate_session_hash=NULL,candidate_session_expires_at=NULL,candidate_last_seen_at=NULL,answers='{}'::jsonb WHERE id=$1 AND organization_id=$2 AND mode='digital' AND status='in_progress' AND (candidate_session_hash IS NULL OR candidate_session_expires_at < now())`, id, a.OrgID, hash, expires)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return webx.E(409, "invitation_active", "active candidate session cannot be replaced; wait for it to expire or create a new placement")
	}
	s.emit(r.Context(), a.OrgID, "", "placement", "placement.invitation.reissued", map[string]any{"placement_id": id, "expires_at": expires})
	return s.writePreRegistrationPlacementWithToken(w, r, a, id, token)
}

func (s *Service) gradePreRegistrationPlacement(x PreRegistrationPlacement, answers map[string]string) (placementResult, error) {
	if len(answers) < len(x.Plan) {
		return placementResult{}, webx.E(409, "incomplete", "all placement questions must be answered")
	}
	clean := make(map[string]string, len(x.Plan))
	byLevel := map[string]*breakdown{}
	byTopic := map[string]*breakdown{}
	correctN := 0
	for _, p := range x.Plan {
		option := strings.ToUpper(strings.TrimSpace(answers[p.QuestionID]))
		if len(option) != 1 {
			return placementResult{}, webx.E(400, "answer", "each placement answer must be A-D")
		}
		idx := strings.Index("ABCD", option)
		if idx < 0 || idx >= len(p.DisplayOrder) {
			return placementResult{}, webx.E(400, "answer", "each placement answer must be A-D")
		}
		clean[p.QuestionID] = option
		q, ok := s.Bank.Questions[p.QuestionID]
		if !ok {
			return placementResult{}, fmt.Errorf("placement question %s missing from bank", p.QuestionID)
		}
		sub, ok := s.Bank.SubjectByUUID[q.SubjectUUID]
		if !ok {
			return placementResult{}, fmt.Errorf("placement subject %s missing from bank", q.SubjectUUID)
		}
		lb := byLevel[sub.Level]
		if lb == nil {
			lb = &breakdown{}
			byLevel[sub.Level] = lb
		}
		tb := byTopic[sub.ShortName]
		if tb == nil {
			tb = &breakdown{}
			byTopic[sub.ShortName] = tb
		}
		lb.Attempts++
		tb.Attempts++
		lb.MaxPoints += float64(sub.Point)
		tb.MaxPoints += float64(sub.Point)
		canon := p.DisplayOrder[idx]
		if canon == q.CorrectIndex {
			correctN++
			lb.Correct++
			tb.Correct++
			lb.Points += float64(sub.Point)
			tb.Points += float64(sub.Point)
		}
	}
	for _, m := range []map[string]*breakdown{byLevel, byTopic} {
		for _, b := range m {
			if b.Attempts > 0 {
				b.Accuracy = 100 * float64(b.Correct) / float64(b.Attempts)
			}
		}
	}
	score := 100 * float64(correctN) / float64(len(x.Plan))
	return placementResult{Score: score, Level: s.levelFrom(byLevel), ByLevel: byLevel, ByTopic: byTopic, Clean: clean}, nil
}

func (s *Service) finishPreRegistrationPlacement(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	if _, err := uuid.Parse(r.PathValue("id")); err != nil {
		return webx.E(400, "placement", "invalid placement id")
	}
	x, err := s.loadPreRegistrationPlacement(r.Context(), r.PathValue("id"))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && x.OrganizationID != a.OrgID) {
		return webx.E(404, "placement", "pre-registration placement not found")
	}
	if err != nil {
		return err
	}
	if x.Status != "in_progress" {
		return webx.E(409, "placement", "placement is already completed")
	}
	if x.Mode != "paper" {
		return webx.E(409, "candidate_required", "digital placement must be completed from the candidate invitation")
	}
	var req struct {
		Answers map[string]string `json:"answers"`
	}
	if err := webx.Decode(r, &req, 256<<10); err != nil {
		return err
	}
	result, err := s.gradePreRegistrationPlacement(x, req.Answers)
	if err != nil {
		return err
	}
	rawAnswers, _ := json.Marshal(result.Clean)
	ct, err := s.DB.Exec(r.Context(), `UPDATE pre_registration_placements SET answers=$3,status='completed',score=$4,level_result=$5,completed_at=now() WHERE id=$1 AND organization_id=$2 AND status='in_progress'`, x.ID, a.OrgID, rawAnswers, result.Score, result.Level)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return webx.E(409, "placement", "placement state changed; reload and try again")
	}
	s.emit(r.Context(), a.OrgID, "", "placement", "placement.preregistration.completed", map[string]any{"placement_id": x.ID, "score": result.Score, "level": result.Level, "mode": x.Mode})
	webx.JSON(w, 200, map[string]any{"id": x.ID, "status": "completed", "score": result.Score, "level": result.Level, "by_level": result.ByLevel, "by_topic": result.ByTopic})
	return nil
}

func (s *Service) markPreRegistrationPlacementRegistered(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	if _, err := uuid.Parse(r.PathValue("id")); err != nil {
		return webx.E(400, "placement", "invalid placement id")
	}
	x, err := s.loadPreRegistrationPlacement(r.Context(), r.PathValue("id"))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && x.OrganizationID != a.OrgID) {
		return webx.E(404, "placement", "pre-registration placement not found")
	}
	if err != nil {
		return err
	}
	if x.Status != "completed" || x.LevelResult == nil {
		return webx.E(409, "placement", "placement must be completed before registration")
	}
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := webx.Decode(r, &req, 64<<10); err != nil {
		return err
	}
	if _, err := uuid.Parse(req.UserID); err != nil {
		return webx.E(400, "user_id", "invalid registered student id")
	}
	var profile struct {
		OrganizationID *string `json:"organization_id"`
		Role           string  `json:"role"`
		CurrentLevel   *string `json:"current_level"`
	}
	if err := s.Identity.Do(r.Context(), "GET", "/internal/resolve?user_id="+req.UserID, nil, &profile); err != nil {
		return fmt.Errorf("validate registered student: %w", err)
	}
	if profile.Role != "student" || profile.OrganizationID == nil || *profile.OrganizationID != a.OrgID {
		return webx.E(400, "user_id", "registered account does not belong to this center")
	}
	if profile.CurrentLevel == nil || *profile.CurrentLevel != *x.LevelResult {
		return webx.E(409, "level", "registered account level must match placement result")
	}
	_, err = s.DB.Exec(r.Context(), `UPDATE pre_registration_placements SET status='registered',registered_user_id=$3,registered_at=now(),candidate_session_hash=NULL WHERE id=$1 AND organization_id=$2`, x.ID, a.OrgID, req.UserID)
	if err != nil {
		return err
	}
	s.emit(r.Context(), a.OrgID, req.UserID, "placement", "placement.preregistration.registered", map[string]any{"placement_id": x.ID, "level": *x.LevelResult})
	webx.JSON(w, 200, map[string]any{"ok": true, "placement_id": x.ID, "student_user_id": req.UserID, "level": *x.LevelResult})
	return nil
}

func candidateSessionToken(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Placement-Session"))
}

func (s *Service) loadCandidatePlacement(ctx context.Context, rawSession string) (PreRegistrationPlacement, error) {
	if len(rawSession) < 40 || len(rawSession) > 200 {
		return PreRegistrationPlacement{}, webx.E(401, "placement_session", "invalid or expired placement session")
	}
	hash := placementTokenHash(rawSession)
	var id uuid.UUID
	err := s.DB.QueryRow(ctx, `SELECT id FROM pre_registration_placements WHERE candidate_session_hash=$1 AND mode='digital' AND status='in_progress' AND candidate_session_expires_at > now()`, hash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return PreRegistrationPlacement{}, webx.E(401, "placement_session", "invalid or expired placement session")
	}
	if err != nil {
		return PreRegistrationPlacement{}, err
	}
	return s.loadPreRegistrationPlacement(ctx, id.String())
}

func (s *Service) candidatePlacementPayload(x PreRegistrationPlacement) (map[string]any, error) {
	views, err := s.placementQuestionViews(x.Plan)
	if err != nil {
		return nil, err
	}
	questions := make([]CandidatePlacementQuestionView, 0, len(views))
	for _, q := range views {
		questions = append(questions, CandidatePlacementQuestionView{ID: q.ID, Text: q.Text, Options: q.Options})
	}
	return map[string]any{
		"id":                 x.ID,
		"full_name":          x.FullName,
		"status":             x.Status,
		"question_count":     len(x.Plan),
		"answered_count":     len(x.Answers),
		"questions":          questions,
		"answers":            x.Answers,
		"session_expires_at": x.CandidateSessionExpires,
	}, nil
}

func (s *Service) claimPreRegistrationInvitation(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := webx.Decode(r, &req, 32<<10); err != nil {
		return err
	}
	req.Token = strings.TrimSpace(req.Token)
	if len(req.Token) < 40 || len(req.Token) > 200 {
		return webx.E(410, "invitation_unavailable", "invitation is expired, invalid, or already used")
	}
	invitationHash := placementTokenHash(req.Token)
	sessionToken := opaquePlacementToken()
	sessionHash := placementTokenHash(sessionToken)
	sessionExpires := time.Now().UTC().Add(s.candidateSessionTTL())
	var id uuid.UUID
	err := s.DB.QueryRow(r.Context(), `UPDATE pre_registration_placements SET invitation_token_hash=NULL,invitation_claimed_at=now(),candidate_session_hash=$2,candidate_session_expires_at=$3,candidate_last_seen_at=now() WHERE invitation_token_hash=$1 AND mode='digital' AND status='in_progress' AND invitation_expires_at > now() AND candidate_session_hash IS NULL RETURNING id`, invitationHash, sessionHash, sessionExpires).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(410, "invitation_unavailable", "invitation is expired, invalid, or already used")
	}
	if err != nil {
		return err
	}
	x, err := s.loadPreRegistrationPlacement(r.Context(), id.String())
	if err != nil {
		return err
	}
	payload, err := s.candidatePlacementPayload(x)
	if err != nil {
		return err
	}
	payload["session_token"] = sessionToken
	s.emit(r.Context(), x.OrganizationID, "", "placement", "placement.invitation.claimed", map[string]any{"placement_id": x.ID})
	webx.JSON(w, 200, payload)
	return nil
}

func (s *Service) getCandidatePlacementSession(w http.ResponseWriter, r *http.Request) error {
	x, err := s.loadCandidatePlacement(r.Context(), candidateSessionToken(r))
	if err != nil {
		return err
	}
	_, _ = s.DB.Exec(r.Context(), `UPDATE pre_registration_placements SET candidate_last_seen_at=now() WHERE id=$1`, x.ID)
	payload, err := s.candidatePlacementPayload(x)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, payload)
	return nil
}

func (s *Service) saveCandidatePlacementAnswer(w http.ResponseWriter, r *http.Request) error {
	rawSession := candidateSessionToken(r)
	x, err := s.loadCandidatePlacement(r.Context(), rawSession)
	if err != nil {
		return err
	}
	var req struct {
		QuestionID string `json:"question_id"`
		Answer     string `json:"answer"`
	}
	if err := webx.Decode(r, &req, 32<<10); err != nil {
		return err
	}
	req.QuestionID = strings.TrimSpace(req.QuestionID)
	req.Answer = strings.ToUpper(strings.TrimSpace(req.Answer))
	validQuestion := false
	optionCount := 0
	for _, p := range x.Plan {
		if p.QuestionID == req.QuestionID {
			validQuestion = true
			optionCount = len(p.DisplayOrder)
			break
		}
	}
	if !validQuestion {
		return webx.E(400, "question_id", "question does not belong to this placement")
	}
	idx := strings.Index("ABCD", req.Answer)
	if len(req.Answer) != 1 || idx < 0 || idx >= optionCount {
		return webx.E(400, "answer", "answer must be a valid A-D option")
	}
	hash := placementTokenHash(rawSession)
	ct, err := s.DB.Exec(r.Context(), `UPDATE pre_registration_placements SET answers=jsonb_set(answers,ARRAY[$3::text],to_jsonb($4::text),true),candidate_last_seen_at=now() WHERE id=$1 AND candidate_session_hash=$2 AND status='in_progress' AND candidate_session_expires_at > now()`, x.ID, hash, req.QuestionID, req.Answer)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return webx.E(401, "placement_session", "placement session expired")
	}
	webx.JSON(w, 200, map[string]any{"ok": true, "question_id": req.QuestionID, "answer": req.Answer})
	return nil
}

func (s *Service) finishCandidatePlacement(w http.ResponseWriter, r *http.Request) error {
	rawSession := candidateSessionToken(r)
	x, err := s.loadCandidatePlacement(r.Context(), rawSession)
	if err != nil {
		return err
	}
	result, err := s.gradePreRegistrationPlacement(x, x.Answers)
	if err != nil {
		return err
	}
	rawAnswers, _ := json.Marshal(result.Clean)
	hash := placementTokenHash(rawSession)
	ct, err := s.DB.Exec(r.Context(), `UPDATE pre_registration_placements SET answers=$3,status='completed',score=$4,level_result=$5,completed_at=now(),candidate_last_seen_at=now(),candidate_session_hash=NULL WHERE id=$1 AND candidate_session_hash=$2 AND status='in_progress' AND candidate_session_expires_at > now()`, x.ID, hash, rawAnswers, result.Score, result.Level)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return webx.E(409, "placement", "placement state changed; reload and try again")
	}
	s.emit(r.Context(), x.OrganizationID, "", "placement", "placement.preregistration.completed", map[string]any{"placement_id": x.ID, "score": result.Score, "level": result.Level, "mode": "digital"})
	webx.JSON(w, 200, map[string]any{"id": x.ID, "status": "completed", "score": result.Score, "level": result.Level})
	return nil
}

func (s *Service) downloadPlacementPaper(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "center") != nil {
		return webx.E(403, "forbidden", "center admin required")
	}
	path := strings.TrimSpace(s.PlacementPaperPath)
	if path == "" {
		path = "data/placement/placement-test-paper.docx"
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return webx.E(404, "placement_paper", "placement paper template is not installed")
		}
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", `attachment; filename="IELTS-placement-test-paper.docx"`)
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, "IELTS-placement-test-paper.docx", st.ModTime(), f)
	return nil
}
