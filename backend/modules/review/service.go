package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	DB                         *pgxpool.Pool
	Assessment                 *clientx.Client
	InternalSecret, StorageDir string
	MaxAudio                   int64
}
type Submission struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	StudentUserID  string          `json:"student_user_id"`
	AttemptID      string          `json:"attempt_id,omitempty"`
	ServiceCode    string          `json:"service_code"`
	PromptID       string          `json:"prompt_id"`
	Status         string          `json:"status"`
	Text           *string         `json:"text_submission,omitempty"`
	HasAudio       bool            `json:"has_audio"`
	Rubric         json.RawMessage `json:"rubric,omitempty"`
	ReviewNotes    *string         `json:"review_notes,omitempty"`
	Score          *float64        `json:"score,omitempty"`
	SubmittedAt    time.Time       `json:"submitted_at"`
	ReviewedAt     *time.Time      `json:"reviewed_at,omitempty"`
}
type attempt struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	StudentUserID  string  `json:"student_user_id"`
	ServiceCode    string  `json:"service_code"`
	Status         string  `json:"status"`
	AutoScore      float64 `json:"auto_score"`
}

func (s *Service) Router() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		webx.JSON(w, 200, map[string]any{"status": "ok", "service": "review"})
	})
	m.HandleFunc("GET /v1/submissions", webx.Handle(s.list))
	m.HandleFunc("POST /v1/submissions/text", webx.Handle(s.submitText))
	m.HandleFunc("POST /v1/submissions/audio", webx.Handle(s.submitAudio))
	m.HandleFunc("GET /v1/submissions/{id}/audio", webx.Handle(s.audio))
	m.HandleFunc("PATCH /v1/submissions/{id}/review", webx.Handle(s.review))
	return webx.Security(m)
}
func (s *Service) actor(r *http.Request) (authz.Actor, error) {
	a, e := authz.Verify(r, s.InternalSecret)
	if e != nil {
		return a, webx.E(401, "internal_auth", "invalid gateway signature")
	}
	return a, nil
}
func (s *Service) attempt(ctx *http.Request, id string) (attempt, error) {
	var x attempt
	e := s.Assessment.Do(ctx.Context(), "GET", "/internal/attempts/"+id, nil, &x)
	return x, e
}
func validManual(code string) bool { return code == "speaking" || code == "writing" || code == "mock" }

type manualPrompt struct {
	PromptID  string `json:"prompt_id"`
	Component string `json:"component"`
	Required  bool   `json:"required"`
	Submitted bool   `json:"submitted"`
}

func (s *Service) prompt(ctx *http.Request, attemptID, promptID string) (manualPrompt, error) {
	var out struct {
		Items []manualPrompt `json:"items"`
	}
	if err := s.Assessment.Do(ctx.Context(), "GET", "/internal/attempts/"+attemptID+"/manual-prompts", nil, &out); err != nil {
		return manualPrompt{}, err
	}
	for _, p := range out.Items {
		if p.PromptID == promptID {
			return p, nil
		}
	}
	return manualPrompt{}, webx.E(404, "prompt", "manual prompt not found")
}
func duplicateErr(err error) bool {
	if err == nil {
		return false
	}
	x := strings.ToLower(err.Error())
	return strings.Contains(x, "duplicate key") || strings.Contains(x, "unique constraint")
}
func (s *Service) verify(a authz.Actor, r *http.Request, attemptID, code string) (attempt, error) {
	x, e := s.attempt(r, attemptID)
	if e != nil {
		return x, e
	}
	if x.OrganizationID != a.OrgID || x.StudentUserID != a.UserID || x.ServiceCode != code || !validManual(code) {
		return x, webx.E(403, "attempt", "attempt does not belong to this student/service")
	}
	if x.Status != "in_progress" && x.Status != "pending_review" {
		return x, webx.E(409, "attempt", "attempt cannot accept manual submissions")
	}
	return x, nil
}
func (s *Service) submitText(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	var x struct {
		AttemptID   string `json:"attempt_id"`
		ServiceCode string `json:"service_code"`
		PromptID    string `json:"prompt_id"`
		Text        string `json:"text"`
	}
	if e := webx.Decode(r, &x, 1<<20); e != nil {
		return e
	}
	x.Text = strings.TrimSpace(x.Text)
	if len(x.Text) < 20 || len(x.Text) > 20000 {
		return webx.E(400, "text", "submission must be 20-20000 characters")
	}
	if _, e := s.verify(a, r, x.AttemptID, x.ServiceCode); e != nil {
		return e
	}
	promptID := strings.TrimSpace(x.PromptID)
	mp, e := s.prompt(r, x.AttemptID, promptID)
	if e != nil {
		return e
	}
	if mp.Component != "writing" {
		return webx.E(400, "component", "this prompt requires an audio speaking submission")
	}
	if mp.Submitted {
		return webx.E(409, "submission", "prompt already submitted")
	}
	var id uuid.UUID
	e = s.DB.QueryRow(r.Context(), `INSERT INTO submissions(organization_id,student_user_id,attempt_id,service_code,prompt_id,text_submission) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, a.OrgID, a.UserID, x.AttemptID, x.ServiceCode, promptID, x.Text).Scan(&id)
	if e != nil {
		if duplicateErr(e) {
			return webx.E(409, "submission", "prompt already submitted")
		}
		return e
	}
	if e = s.Assessment.Do(r.Context(), "POST", "/internal/attempts/"+x.AttemptID+"/manual-submissions", map[string]any{"prompt_id": promptID, "submission_id": id.String()}, nil); e != nil {
		_, _ = s.DB.Exec(r.Context(), `DELETE FROM submissions WHERE id=$1`, id)
		return e
	}
	webx.JSON(w, 201, map[string]any{"id": id.String()})
	return nil
}
func allowedAudio(v string) bool {
	v = strings.ToLower(strings.Split(v, ";")[0])
	switch v {
	case "audio/webm", "audio/ogg", "audio/mpeg", "audio/mp4", "audio/wav", "audio/x-wav":
		return true
	}
	return false
}
func (s *Service) submitAudio(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	max := s.MaxAudio
	if max <= 0 {
		max = 20 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, max+(1<<20))
	if e := r.ParseMultipartForm(max); e != nil {
		return webx.E(400, "upload", "invalid audio upload")
	}
	attemptID := r.FormValue("attempt_id")
	code := r.FormValue("service_code")
	prompt := r.FormValue("prompt_id")
	if _, e := s.verify(a, r, attemptID, code); e != nil {
		return e
	}
	prompt = strings.TrimSpace(prompt)
	mp, e := s.prompt(r, attemptID, prompt)
	if e != nil {
		return e
	}
	if mp.Component != "speaking" {
		return webx.E(400, "component", "this prompt requires a text writing submission")
	}
	if mp.Submitted {
		return webx.E(409, "submission", "prompt already submitted")
	}
	f, h, e := r.FormFile("audio")
	if e != nil {
		return webx.E(400, "audio", "audio required")
	}
	defer f.Close()
	mime := h.Header.Get("Content-Type")
	if !allowedAudio(mime) {
		return webx.E(415, "mime", "unsupported speaking audio type")
	}
	if e = os.MkdirAll(s.StorageDir, 0700); e != nil {
		return e
	}
	key := uuid.NewString() + ".audio"
	tmp := filepath.Join(s.StorageDir, key+".part")
	out, e := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	hh := sha256.New()
	n, e := io.Copy(io.MultiWriter(out, hh), io.LimitReader(f, max+1))
	ce := out.Close()
	if e != nil || ce != nil || n > max {
		_ = os.Remove(tmp)
		return webx.E(400, "upload", "audio too large or invalid")
	}
	final := filepath.Join(s.StorageDir, key)
	if e = os.Rename(tmp, final); e != nil {
		return e
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(final)
		}
	}()
	var id uuid.UUID
	e = s.DB.QueryRow(r.Context(), `INSERT INTO submissions(organization_id,student_user_id,attempt_id,service_code,prompt_id,audio_storage_key,audio_sha256,audio_mime_type) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, a.OrgID, a.UserID, attemptID, code, prompt, key, hex.EncodeToString(hh.Sum(nil)), strings.ToLower(strings.Split(mime, ";")[0])).Scan(&id)
	if e != nil {
		if duplicateErr(e) {
			return webx.E(409, "submission", "prompt already submitted")
		}
		return e
	}
	if e = s.Assessment.Do(r.Context(), "POST", "/internal/attempts/"+attemptID+"/manual-submissions", map[string]any{"prompt_id": prompt, "submission_id": id.String()}, nil); e != nil {
		_, _ = s.DB.Exec(r.Context(), `DELETE FROM submissions WHERE id=$1`, id)
		return e
	}
	cleanup = false
	webx.JSON(w, 201, map[string]any{"id": id.String(), "bytes": n})
	return nil
}
func scan(row pgx.Row) (Submission, error) {
	var x Submission
	var id, org, student uuid.UUID
	var attempt *uuid.UUID
	var rubric []byte
	var audio *string
	e := row.Scan(&id, &org, &student, &attempt, &x.ServiceCode, &x.PromptID, &x.Text, &audio, &x.Status, &rubric, &x.ReviewNotes, &x.Score, &x.SubmittedAt, &x.ReviewedAt)
	x.ID = id.String()
	x.OrganizationID = org.String()
	x.StudentUserID = student.String()
	if attempt != nil {
		x.AttemptID = attempt.String()
	}
	x.HasAudio = audio != nil
	x.Rubric = json.RawMessage(rubric)
	return x, e
}
func (s *Service) list(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	var rows pgx.Rows
	if a.Role == "student" {
		rows, e = s.DB.Query(r.Context(), `SELECT id,organization_id,student_user_id,attempt_id,service_code,prompt_id,text_submission,audio_storage_key,status,coalesce(rubric,'{}'),review_notes,score,submitted_at,reviewed_at FROM submissions WHERE organization_id=$1 AND student_user_id=$2 ORDER BY submitted_at DESC`, a.OrgID, a.UserID)
	} else if a.Role == "center_admin" {
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "pending"
		}
		rows, e = s.DB.Query(r.Context(), `SELECT id,organization_id,student_user_id,attempt_id,service_code,prompt_id,text_submission,audio_storage_key,status,coalesce(rubric,'{}'),review_notes,score,submitted_at,reviewed_at FROM submissions WHERE organization_id=$1 AND status=$2 ORDER BY submitted_at`, a.OrgID, status)
	} else {
		return webx.E(403, "forbidden", "student or center admin required")
	}
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []Submission{}
	for rows.Next() {
		x, e := scan(rows)
		if e != nil {
			return e
		}
		items = append(items, x)
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}
func (s *Service) audio(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	var org, student uuid.UUID
	var key, mime *string
	e = s.DB.QueryRow(r.Context(), `SELECT organization_id,student_user_id,audio_storage_key,audio_mime_type FROM submissions WHERE id=$1`, r.PathValue("id")).Scan(&org, &student, &key, &mime)
	if errors.Is(e, pgx.ErrNoRows) {
		return webx.E(404, "submission", "submission not found")
	}
	if e != nil {
		return e
	}
	if org.String() != a.OrgID || (a.Role == "student" && student.String() != a.UserID) || (a.Role != "student" && a.Role != "center_admin") {
		return webx.E(403, "forbidden", "not allowed")
	}
	if key == nil {
		return webx.E(404, "audio", "no audio submission")
	}
	f, e := os.Open(filepath.Join(s.StorageDir, filepath.Base(*key)))
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	contentType := "application/octet-stream"
	if mime != nil && allowedAudio(*mime) {
		contentType = *mime
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, "speaking-response", st.ModTime(), f)
	return nil
}
func (s *Service) review(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "center_admin" {
		return webx.E(403, "forbidden", "center admin required")
	}
	var x struct {
		Score  float64        `json:"score"`
		Notes  string         `json:"notes"`
		Rubric map[string]any `json:"rubric"`
	}
	if e := webx.Decode(r, &x, 256<<10); e != nil {
		return e
	}
	if x.Score < 0 || x.Score > 100 {
		return webx.E(400, "score", "score must be 0-100")
	}
	var preAttemptID *uuid.UUID
	var preOrg uuid.UUID
	if err := s.DB.QueryRow(r.Context(), `SELECT attempt_id,organization_id FROM submissions WHERE id=$1`, r.PathValue("id")).Scan(&preAttemptID, &preOrg); errors.Is(err, pgx.ErrNoRows) {
		return webx.E(404, "submission", "submission not found")
	} else if err != nil {
		return err
	}
	if preOrg.String() != a.OrgID || preAttemptID == nil {
		return webx.E(403, "forbidden", "submission not available")
	}
	at, err := s.attempt(r, preAttemptID.String())
	if err != nil {
		return err
	}
	if at.OrganizationID != a.OrgID || at.Status != "pending_review" {
		return webx.E(409, "attempt", "student must finish the attempt before review")
	}
	rb, _ := json.Marshal(x.Rubric)
	var attemptID *uuid.UUID
	e = s.DB.QueryRow(r.Context(), `UPDATE submissions SET status='reviewed',score=$3,review_notes=$4,rubric=$5,reviewer_user_id=$6,reviewed_at=now() WHERE id=$1 AND organization_id=$2 AND status='pending' RETURNING attempt_id`, r.PathValue("id"), a.OrgID, x.Score, strings.TrimSpace(x.Notes), rb, a.UserID).Scan(&attemptID)
	if errors.Is(e, pgx.ErrNoRows) {
		return webx.E(409, "submission", "submission is not pending")
	}
	if e != nil {
		return e
	}
	if attemptID != nil {
		var pending int
		var avg float64
		e = s.DB.QueryRow(r.Context(), `SELECT count(*) FILTER(WHERE status='pending'),coalesce(avg(score) FILTER(WHERE status='reviewed'),0) FROM submissions WHERE attempt_id=$1`, *attemptID).Scan(&pending, &avg)
		if e != nil {
			return e
		}
		if pending == 0 {
			if e = s.Assessment.Do(r.Context(), "PATCH", "/internal/attempts/"+attemptID.String()+"/review", map[string]any{"manual_score": avg}, nil); e != nil {
				return e
			}
		}
	}
	webx.JSON(w, 200, map[string]any{"status": "reviewed"})
	return nil
}
