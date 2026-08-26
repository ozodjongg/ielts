package listening

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	DB                                         *pgxpool.Pool
	Tenant, Points, Analytics                  *clientx.Client
	InternalSecret, PlaybackSecret, StorageDir string
	MaxUpload                                  int64
}
type Audio struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Title          string    `json:"title"`
	MimeType       string    `json:"mime_type"`
	Level          string    `json:"level"`
	Status         string    `json:"status"`
	SizeBytes      int64     `json:"size_bytes"`
	DurationMS     *int      `json:"duration_ms,omitempty"`
	MaxPlays       int       `json:"max_plays"`
	AllowSeek      bool      `json:"allow_seek"`
	CreatedAt      time.Time `json:"created_at"`
}
type Set struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	AudioID        string          `json:"audio_id"`
	Title          string          `json:"title"`
	Level          *string         `json:"level,omitempty"`
	Questions      json.RawMessage `json:"questions"`
	CreatedAt      time.Time       `json:"created_at"`
}
type Assignment struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	SetID          string     `json:"set_id"`
	TargetType     string     `json:"target_type"`
	TargetID       *string    `json:"target_id,omitempty"`
	DueAt          *time.Time `json:"due_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (s *Service) Router() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		webx.JSON(w, 200, map[string]any{"status": "ok", "service": "listening"})
	})
	m.HandleFunc("GET /v1/audio", webx.Handle(s.listAudio))
	m.HandleFunc("POST /v1/audio", webx.Handle(s.upload))
	m.HandleFunc("GET /v1/sets", webx.Handle(s.listSets))
	m.HandleFunc("POST /v1/sets", webx.Handle(s.createSet))
	m.HandleFunc("GET /v1/assignments", webx.Handle(s.assignments))
	m.HandleFunc("POST /v1/assignments", webx.Handle(s.createAssignment))
	m.HandleFunc("POST /v1/assignments/{id}/start", webx.Handle(s.start))
	m.HandleFunc("GET /v1/attempts/{id}", webx.Handle(s.current))
	m.HandleFunc("POST /v1/attempts/{id}/play-token", webx.Handle(s.playToken))
	m.HandleFunc("GET /v1/audio/{id}/stream", webx.Handle(s.stream))
	m.HandleFunc("POST /v1/attempts/{id}/events", webx.Handle(s.event))
	m.HandleFunc("POST /v1/attempts/{id}/finish", webx.Handle(s.finish))
	return webx.Security(m)
}
func (s *Service) actor(r *http.Request) (authz.Actor, error) {
	a, e := authz.Verify(r, s.InternalSecret)
	if e != nil {
		return a, webx.E(401, "internal_auth", "invalid gateway signature")
	}
	return a, nil
}
func scanAudio(row pgx.Row) (Audio, string, error) {
	var a Audio
	var id, org uuid.UUID
	var storage, sha string
	err := row.Scan(&id, &org, &a.Title, &storage, &sha, &a.MimeType, &a.SizeBytes, &a.DurationMS, &a.Level, &a.MaxPlays, &a.AllowSeek, &a.Status, &a.CreatedAt)
	a.ID = id.String()
	a.OrganizationID = org.String()
	return a, storage, err
}
func (s *Service) listAudio(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "center_admin" {
		return webx.E(403, "forbidden", "center admin required")
	}
	rows, e := s.DB.Query(r.Context(), `SELECT id,organization_id,title,storage_key,sha256,mime_type,size_bytes,duration_ms,coalesce(level,''),max_plays,allow_seek,status,created_at FROM audio_assets WHERE organization_id=$1 ORDER BY created_at DESC`, a.OrgID)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []Audio{}
	for rows.Next() {
		x, _, e := scanAudio(rows)
		if e != nil {
			return e
		}
		items = append(items, x)
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}
func allowedMIME(v string) bool {
	v = strings.ToLower(strings.TrimSpace(strings.Split(v, ";")[0]))
	switch v {
	case "audio/mpeg", "audio/mp3", "audio/wav", "audio/x-wav", "audio/ogg", "audio/mp4", "audio/x-m4a":
		return true
	}
	return false
}
func extFor(h *multipart.FileHeader) string {
	ext := strings.ToLower(filepath.Ext(h.Filename))
	switch ext {
	case ".mp3", ".wav", ".ogg", ".m4a", ".mp4":
		return ext
	}
	return ".bin"
}
func (s *Service) upload(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "center_admin" {
		return webx.E(403, "forbidden", "center admin required")
	}
	max := s.MaxUpload
	if max <= 0 {
		max = 50 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, max+1<<20)
	if e := r.ParseMultipartForm(max); e != nil {
		return webx.E(400, "upload", "invalid or oversized multipart upload")
	}
	f, h, e := r.FormFile("audio")
	if e != nil {
		return webx.E(400, "audio", "audio file required")
	}
	defer f.Close()
	mime := h.Header.Get("Content-Type")
	if !allowedMIME(mime) {
		return webx.E(415, "mime", "MP3, WAV, OGG or M4A audio required")
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" || len(title) > 180 {
		return webx.E(400, "title", "title required")
	}
	level := strings.ToUpper(strings.TrimSpace(r.FormValue("level")))
	if level != "" && !validLevel(level) {
		return webx.E(400, "level", "invalid level")
	}
	maxPlays := 2
	if v, _ := strconv.Atoi(r.FormValue("max_plays")); v >= 1 && v <= 10 {
		maxPlays = v
	}
	allowSeek := strings.EqualFold(r.FormValue("allow_seek"), "true")
	dur := 0
	if v, _ := strconv.Atoi(r.FormValue("duration_ms")); v > 0 {
		dur = v
	}
	if e := os.MkdirAll(s.StorageDir, 0700); e != nil {
		return e
	}
	key := uuid.NewString() + extFor(h)
	tmp := filepath.Join(s.StorageDir, key+".part")
	out, e := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(out, hash), io.LimitReader(f, max+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || n > max {
		_ = os.Remove(tmp)
		return webx.E(400, "upload", "audio upload failed or exceeds limit")
	}
	final := filepath.Join(s.StorageDir, key)
	if e = os.Rename(tmp, final); e != nil {
		_ = os.Remove(tmp)
		return e
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(final)
		}
	}()
	var id uuid.UUID
	var duration any = nil
	if dur > 0 {
		duration = dur
	}
	e = s.DB.QueryRow(r.Context(), `INSERT INTO audio_assets(organization_id,title,storage_key,sha256,mime_type,size_bytes,duration_ms,level,max_plays,allow_seek,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,nullif($8,''),$9,$10,$11) RETURNING id`, a.OrgID, title, key, hex.EncodeToString(hash.Sum(nil)), mime, n, duration, level, maxPlays, allowSeek, a.UserID).Scan(&id)
	if e != nil {
		return e
	}
	cleanup = false
	s.emit(r.Context(), a.OrgID, a.UserID, "listening.audio_uploaded", map[string]any{"audio_id": id.String(), "bytes": n})
	webx.JSON(w, 201, map[string]any{"id": id.String(), "title": title, "size_bytes": n, "max_plays": maxPlays})
	return nil
}
func validLevel(x string) bool {
	switch x {
	case "A1", "A2", "B1", "B2", "C1", "C2":
		return true
	}
	return false
}
func (s *Service) listSets(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "center_admin" {
		return webx.E(403, "forbidden", "center admin required")
	}
	rows, e := s.DB.Query(r.Context(), `SELECT id,organization_id,audio_id,title,level,questions,created_at FROM listening_sets WHERE organization_id=$1 ORDER BY created_at DESC`, a.OrgID)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []Set{}
	for rows.Next() {
		var x Set
		var id, org, aud uuid.UUID
		var q []byte
		if e := rows.Scan(&id, &org, &aud, &x.Title, &x.Level, &q, &x.CreatedAt); e != nil {
			return e
		}
		x.ID = id.String()
		x.OrganizationID = org.String()
		x.AudioID = aud.String()
		x.Questions = json.RawMessage(q)
		items = append(items, x)
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}

type Question struct {
	ID         string   `json:"id"`
	Prompt     string   `json:"prompt"`
	Options    []string `json:"options"`
	BasePoints float64  `json:"base_points,omitempty"`
}

func validateQuestions(q []Question, key map[string]string) error {
	if len(q) < 1 || len(q) > 50 {
		return webx.E(400, "questions", "1-50 questions required")
	}
	seen := map[string]bool{}
	for _, x := range q {
		if strings.TrimSpace(x.ID) == "" || seen[x.ID] || strings.TrimSpace(x.Prompt) == "" || len(x.Options) < 2 || len(x.Options) > 6 {
			return webx.E(400, "questions", "each question needs unique id, prompt and 2-6 options")
		}
		seen[x.ID] = true
		if x.BasePoints < 0 || x.BasePoints > 20 {
			return webx.E(400, "base_points", "listening base_points must be 0-20")
		}
		answer, ok := key[x.ID]
		if !ok {
			return webx.E(400, "answer_key", "answer key missing question "+x.ID)
		}
		found := false
		for _, option := range x.Options {
			if strings.TrimSpace(option) == strings.TrimSpace(answer) {
				found = true
				break
			}
		}
		if !found {
			return webx.E(400, "answer_key", "answer for question "+x.ID+" must exactly match one option")
		}
	}
	return nil
}
func (s *Service) createSet(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "center_admin" {
		return webx.E(403, "forbidden", "center admin required")
	}
	var x struct {
		AudioID   string            `json:"audio_id"`
		Title     string            `json:"title"`
		Level     string            `json:"level"`
		Questions []Question        `json:"questions"`
		AnswerKey map[string]string `json:"answer_key"`
	}
	if e := webx.Decode(r, &x, 2<<20); e != nil {
		return e
	}
	if e := validateQuestions(x.Questions, x.AnswerKey); e != nil {
		return e
	}
	if lv := strings.ToUpper(strings.TrimSpace(x.Level)); lv != "" && lv != "A1" && lv != "A2" && lv != "B1" && lv != "B2" && lv != "C1" && lv != "C2" {
		return webx.E(400, "level", "invalid CEFR level")
	}
	var exists bool
	e = s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM audio_assets WHERE id=$1 AND organization_id=$2 AND status='active')`, x.AudioID, a.OrgID).Scan(&exists)
	if e != nil {
		return e
	}
	if !exists {
		return webx.E(404, "audio", "audio not found")
	}
	for i := range x.Questions {
		if x.Questions[i].BasePoints == 0 {
			x.Questions[i].BasePoints = 2
		}
	}
	qb, _ := json.Marshal(x.Questions)
	kb, _ := json.Marshal(x.AnswerKey)
	var id uuid.UUID
	e = s.DB.QueryRow(r.Context(), `INSERT INTO listening_sets(organization_id,audio_id,title,level,questions,answer_key,created_by) VALUES($1,$2,$3,nullif($4,''),$5,$6,$7) RETURNING id`, a.OrgID, x.AudioID, strings.TrimSpace(x.Title), strings.ToUpper(strings.TrimSpace(x.Level)), qb, kb, a.UserID).Scan(&id)
	if e != nil {
		return e
	}
	webx.JSON(w, 201, map[string]any{"id": id.String()})
	return nil
}
func (s *Service) assignments(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role == "center_admin" {
		rows, e := s.DB.Query(r.Context(), `SELECT id,organization_id,set_id,target_type,target_id,due_at,created_at FROM listening_assignments WHERE organization_id=$1 ORDER BY created_at DESC`, a.OrgID)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []Assignment{}
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
		return webx.E(403, "forbidden", "invalid role")
	}
	groups, e := s.studentGroups(r, a)
	if e != nil {
		return e
	}
	rows, e := s.DB.Query(r.Context(), `SELECT id,organization_id,set_id,target_type,target_id,due_at,created_at FROM listening_assignments WHERE organization_id=$1 AND (due_at IS NULL OR due_at>=now()) ORDER BY created_at DESC`, a.OrgID)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []Assignment{}
	for rows.Next() {
		x, e := scanAssignment(rows)
		if e != nil {
			return e
		}
		if x.TargetType == "all" || (x.TargetType == "student" && x.TargetID != nil && *x.TargetID == a.UserID) || (x.TargetType == "group" && x.TargetID != nil && groups[*x.TargetID]) {
			items = append(items, x)
		}
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}
func scanAssignment(row pgx.Row) (Assignment, error) {
	var x Assignment
	var id, org, set uuid.UUID
	var target *uuid.UUID
	e := row.Scan(&id, &org, &set, &x.TargetType, &target, &x.DueAt, &x.CreatedAt)
	x.ID = id.String()
	x.OrganizationID = org.String()
	x.SetID = set.String()
	if target != nil {
		v := target.String()
		x.TargetID = &v
	}
	return x, e
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
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "center_admin" {
		return webx.E(403, "forbidden", "center admin required")
	}
	var x struct {
		SetID      string     `json:"set_id"`
		TargetType string     `json:"target_type"`
		TargetID   *string    `json:"target_id"`
		DueAt      *time.Time `json:"due_at"`
	}
	if e := webx.Decode(r, &x, 256<<10); e != nil {
		return e
	}
	if x.TargetType != "student" && x.TargetType != "group" && x.TargetType != "all" {
		return webx.E(400, "target_type", "student, group or all required")
	}
	if x.TargetType == "all" {
		x.TargetID = nil
	} else if x.TargetID == nil {
		return webx.E(400, "target_id", "target id required")
	}
	if err := s.validateAssignmentTarget(r.Context(), a.OrgID, x.TargetType, x.TargetID); err != nil {
		return err
	}
	if x.DueAt != nil && x.DueAt.Before(time.Now()) {
		return webx.E(400, "due_at", "due date must be in the future")
	}
	var id uuid.UUID
	e = s.DB.QueryRow(r.Context(), `INSERT INTO listening_assignments(organization_id,set_id,target_type,target_id,due_at,created_by) SELECT $1,id,$3,$4,$5,$6 FROM listening_sets WHERE id=$2 AND organization_id=$1 RETURNING id`, a.OrgID, x.SetID, x.TargetType, x.TargetID, x.DueAt, a.UserID).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return webx.E(404, "set", "listening set not found")
	}
	if e != nil {
		return e
	}
	webx.JSON(w, 201, map[string]any{"id": id.String()})
	return nil
}
func (s *Service) studentGroups(r *http.Request, a authz.Actor) (map[string]bool, error) {
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
func listeningReservationKey(assignmentID, studentID string) string {
	return "listening:" + assignmentID + ":" + studentID
}

func (s *Service) start(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	x, e := scanAssignment(s.DB.QueryRow(r.Context(), `SELECT id,organization_id,set_id,target_type,target_id,due_at,created_at FROM listening_assignments WHERE id=$1 AND organization_id=$2`, r.PathValue("id"), a.OrgID))
	if errors.Is(e, pgx.ErrNoRows) {
		return webx.E(404, "assignment", "assignment not found")
	}
	if e != nil {
		return e
	}
	groups, e := s.studentGroups(r, a)
	if e != nil {
		return e
	}
	ok := x.TargetType == "all" || (x.TargetType == "student" && x.TargetID != nil && *x.TargetID == a.UserID) || (x.TargetType == "group" && x.TargetID != nil && groups[*x.TargetID])
	if !ok {
		return webx.E(403, "forbidden", "assignment not assigned to this student")
	}
	if x.DueAt != nil && x.DueAt.Before(time.Now()) {
		return webx.E(409, "expired", "assignment expired")
	}
	var existing uuid.UUID
	e = s.DB.QueryRow(r.Context(), `SELECT id FROM listening_attempts WHERE assignment_id=$1 AND student_user_id=$2`, x.ID, a.UserID).Scan(&existing)
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
	reservationKey := listeningReservationKey(x.ID, a.UserID)
	if e = s.Tenant.Do(r.Context(), "POST", "/internal/usage/reserve", map[string]any{
		"organization_id": a.OrgID, "service_code": "listening", "amount": 1,
		"reservation_key": reservationKey, "hold_concurrency": true, "lease_minutes": 180,
	}, &quota); e != nil || !quota.Allowed {
		return webx.E(429, "quota", "listening quota or concurrency limit reached")
	}
	created := false
	defer func() {
		if !created {
			_ = s.Tenant.Do(context.Background(), "POST", "/internal/usage/cancel", map[string]any{"organization_id": a.OrgID, "service_code": "listening", "reservation_key": reservationKey}, nil)
		}
	}()
	var id uuid.UUID
	e = s.DB.QueryRow(r.Context(), `INSERT INTO listening_attempts(organization_id,assignment_id,student_user_id) VALUES($1,$2,$3) RETURNING id`, a.OrgID, x.ID, a.UserID).Scan(&id)
	if e != nil {
		return e
	}
	created = true
	webx.JSON(w, 201, map[string]any{"attempt_id": id.String()})
	return nil
}
func listeningQuestionUUID(setID, questionID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("assessment-platform-v5:listening:"+setID+":"+questionID)).String()
}

func (s *Service) publicQuestionsWithRush(ctx context.Context, setID string, raw []byte) ([]map[string]any, error) {
	var questions []Question
	if err := json.Unmarshal(raw, &questions); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(questions))
	for _, q := range questions {
		ids = append(ids, listeningQuestionUUID(setID, q.ID))
	}
	mult := map[string]float64{}
	if s.Points != nil && len(ids) > 0 {
		var out struct {
			Items []struct {
				QuestionID string  `json:"question_id"`
				Multiplier float64 `json:"multiplier"`
			} `json:"items"`
		}
		if err := s.Points.Do(ctx, "POST", "/internal/quote/batch", map[string]any{"service_code": "listening", "question_ids": ids}, &out); err == nil {
			for _, x := range out.Items {
				mult[x.QuestionID] = x.Multiplier
			}
		}
	}
	items := make([]map[string]any, 0, len(questions))
	for _, q := range questions {
		qid := listeningQuestionUUID(setID, q.ID)
		m := mult[qid]
		if m < 1 {
			m = 1
		}
		base := q.BasePoints
		if base <= 0 {
			base = 2
		}
		items = append(items, map[string]any{"id": q.ID, "prompt": q.Prompt, "options": q.Options, "base_points": base, "rush_multiplier": m})
	}
	return items, nil
}

func (s *Service) current(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	var attemptID, assignmentID, setID, audioID uuid.UUID
	var status, title, mime string
	var play, maxPlays int
	var allow bool
	var q []byte
	e = s.DB.QueryRow(r.Context(), `SELECT la.id,la.assignment_id,ls.id,aa.id,la.status,ls.title,aa.mime_type,la.play_count,aa.max_plays,aa.allow_seek,ls.questions FROM listening_attempts la JOIN listening_assignments asg ON asg.id=la.assignment_id JOIN listening_sets ls ON ls.id=asg.set_id JOIN audio_assets aa ON aa.id=ls.audio_id WHERE la.id=$1 AND la.organization_id=$2 AND la.student_user_id=$3`, r.PathValue("id"), a.OrgID, a.UserID).Scan(&attemptID, &assignmentID, &setID, &audioID, &status, &title, &mime, &play, &maxPlays, &allow, &q)
	if errors.Is(e, pgx.ErrNoRows) {
		return webx.E(404, "attempt", "attempt not found")
	}
	if e != nil {
		return e
	}
	questions, err := s.publicQuestionsWithRush(r.Context(), setID.String(), q)
	if err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"id": attemptID.String(), "status": status, "title": title, "audio_id": audioID.String(), "mime_type": mime, "play_count": play, "max_plays": maxPlays, "allow_seek": allow, "questions": questions})
	return nil
}
func (s *Service) signToken(grant, attempt, audio, user string, exp int64) string {
	msg := strings.Join([]string{grant, attempt, audio, user, strconv.FormatInt(exp, 10)}, "\n")
	h := hmac.New(sha256.New, []byte(s.PlaybackSecret))
	h.Write([]byte(msg))
	return grant + "." + strconv.FormatInt(exp, 10) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (s *Service) verifyToken(tok, attempt, audio, user string) (string, bool) {
	p := strings.Split(tok, ".")
	if len(p) != 3 {
		return "", false
	}
	if _, e := uuid.Parse(p[0]); e != nil {
		return "", false
	}
	exp, e := strconv.ParseInt(p[1], 10, 64)
	if e != nil || time.Now().Unix() > exp || exp-time.Now().Unix() > 180 {
		return "", false
	}
	want := s.signToken(p[0], attempt, audio, user, exp)
	return p[0], hmac.Equal([]byte(tok), []byte(want))
}
func (s *Service) playToken(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var audio uuid.UUID
	var max, play int
	e = tx.QueryRow(r.Context(), `
		UPDATE listening_attempts la SET play_count=la.play_count+1
		FROM listening_assignments x, listening_sets ls, audio_assets aa
		WHERE la.id=$1 AND la.organization_id=$2 AND la.student_user_id=$3
		  AND la.status='in_progress' AND x.id=la.assignment_id AND ls.id=x.set_id AND aa.id=ls.audio_id
		  AND la.play_count < aa.max_plays
		RETURNING ls.audio_id,aa.max_plays,la.play_count`, r.PathValue("id"), a.OrgID, a.UserID).Scan(&audio, &max, &play)
	if errors.Is(e, pgx.ErrNoRows) {
		var exists bool
		if ee := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM listening_attempts WHERE id=$1 AND organization_id=$2 AND student_user_id=$3)`, r.PathValue("id"), a.OrgID, a.UserID).Scan(&exists); ee != nil {
			return ee
		}
		if !exists {
			return webx.E(404, "attempt", "attempt not found")
		}
		return webx.E(429, "play_limit", "audio play limit reached or attempt is not active")
	}
	if e != nil {
		return e
	}
	grant := uuid.New()
	expTime := time.Now().Add(2 * time.Minute)
	_, e = tx.Exec(r.Context(), `INSERT INTO playback_grants(id,attempt_id,audio_id,student_user_id,expires_at) VALUES($1,$2,$3,$4,$5)`, grant, r.PathValue("id"), audio, a.UserID, expTime)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	tok := s.signToken(grant.String(), r.PathValue("id"), audio.String(), a.UserID, expTime.Unix())
	webx.JSON(w, 200, map[string]any{"audio_id": audio.String(), "token": tok, "expires_at": expTime, "plays_remaining": max - play})
	return nil
}
func (s *Service) stream(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	attempt := r.URL.Query().Get("attempt_id")
	audio := r.PathValue("id")
	token := r.Header.Get("X-Playback-Token")
	grant, ok := s.verifyToken(token, attempt, audio, a.UserID)
	if !ok {
		return webx.E(401, "playback_token", "invalid or expired playback token")
	}
	ct, e := s.DB.Exec(r.Context(), `UPDATE playback_grants SET consumed_at=now() WHERE id=$1 AND attempt_id=$2 AND audio_id=$3 AND student_user_id=$4 AND consumed_at IS NULL AND expires_at>now()`, grant, attempt, audio, a.UserID)
	if e != nil {
		return e
	}
	if ct.RowsAffected() != 1 {
		return webx.E(401, "playback_token", "playback token was already used or expired")
	}
	var key, mime, title string
	var allow bool
	e = s.DB.QueryRow(r.Context(), `SELECT aa.storage_key,aa.mime_type,aa.title,aa.allow_seek FROM listening_attempts la JOIN listening_assignments x ON x.id=la.assignment_id JOIN listening_sets ls ON ls.id=x.set_id JOIN audio_assets aa ON aa.id=ls.audio_id WHERE la.id=$1 AND la.student_user_id=$2 AND la.organization_id=$3 AND aa.id=$4`, attempt, a.UserID, a.OrgID, audio).Scan(&key, &mime, &title, &allow)
	if errors.Is(e, pgx.ErrNoRows) {
		return webx.E(404, "audio", "audio not found")
	}
	if e != nil {
		return e
	}
	if !allow && r.Header.Get("Range") != "" {
		return webx.E(416, "seek_disabled", "seeking is disabled for this listening attempt")
	}
	f, e := os.Open(filepath.Join(s.StorageDir, filepath.Base(key)))
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", strings.ReplaceAll(title, "\"", "")))
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("Accept-Ranges", map[bool]string{true: "bytes", false: "none"}[allow])
	http.ServeContent(w, r, title, st.ModTime(), f)
	return nil
}
func (s *Service) event(w http.ResponseWriter, r *http.Request) error {
	a, e := s.actor(r)
	if e != nil {
		return e
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	var x struct {
		EventType  string `json:"event_type"`
		PositionMS *int   `json:"position_ms"`
	}
	if e := webx.Decode(r, &x, 64<<10); e != nil {
		return e
	}
	switch x.EventType {
	case "play", "pause", "ended", "focus_lost", "seek_attempt":
	default:
		return webx.E(400, "event", "invalid event")
	}
	var audio uuid.UUID
	e = s.DB.QueryRow(r.Context(), `SELECT ls.audio_id FROM listening_attempts la JOIN listening_assignments x ON x.id=la.assignment_id JOIN listening_sets ls ON ls.id=x.set_id WHERE la.id=$1 AND la.student_user_id=$2 AND la.organization_id=$3`, r.PathValue("id"), a.UserID, a.OrgID).Scan(&audio)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(r.Context(), `INSERT INTO playback_events(attempt_id,audio_id,event_type,position_ms) VALUES($1,$2,$3,$4)`, r.PathValue("id"), audio, x.EventType, x.PositionMS)
	if e != nil {
		return e
	}
	w.WriteHeader(204)
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
	var x struct {
		Answers map[string]string `json:"answers"`
	}
	if e := webx.Decode(r, &x, 1<<20); e != nil {
		return e
	}
	var keyBytes []byte
	var qBytes []byte
	var status string
	var assignmentID, setID uuid.UUID
	e = s.DB.QueryRow(r.Context(), `SELECT ls.answer_key,ls.questions,la.status,la.assignment_id,ls.id FROM listening_attempts la JOIN listening_assignments asg ON asg.id=la.assignment_id JOIN listening_sets ls ON ls.id=asg.set_id WHERE la.id=$1 AND la.organization_id=$2 AND la.student_user_id=$3`, r.PathValue("id"), a.OrgID, a.UserID).Scan(&keyBytes, &qBytes, &status, &assignmentID, &setID)
	if errors.Is(e, pgx.ErrNoRows) {
		return webx.E(404, "attempt", "attempt not found")
	}
	if e != nil {
		return e
	}
	if status != "in_progress" {
		return webx.E(409, "attempt", "attempt already finished")
	}
	key := map[string]string{}
	_ = json.Unmarshal(keyBytes, &key)
	if len(x.Answers) != len(key) {
		return webx.E(409, "incomplete", "all listening questions must be answered")
	}
	var questions []Question
	if err := json.Unmarshal(qBytes, &questions); err != nil {
		return err
	}
	byID := map[string]Question{}
	questionUUIDs := make([]string, 0, len(questions))
	for _, q := range questions {
		byID[q.ID] = q
		questionUUIDs = append(questionUUIDs, listeningQuestionUUID(setID.String(), q.ID))
	}
	mults := map[string]float64{}
	if s.Points != nil {
		var quotes struct {
			Items []struct {
				QuestionID string  `json:"question_id"`
				Multiplier float64 `json:"multiplier"`
			} `json:"items"`
		}
		if err := s.Points.Do(r.Context(), "POST", "/internal/quote/batch", map[string]any{"service_code": "listening", "question_ids": questionUUIDs}, &quotes); err == nil {
			for _, q := range quotes.Items {
				mults[q.QuestionID] = q.Multiplier
			}
		}
	}
	correct := 0
	reward := 0.0
	for id, want := range key {
		ok := strings.EqualFold(strings.TrimSpace(x.Answers[id]), strings.TrimSpace(want))
		if ok {
			correct++
		}
		q := byID[id]
		base := q.BasePoints
		if base <= 0 {
			base = 2
		}
		qid := listeningQuestionUUID(setID.String(), id)
		mult := mults[qid]
		if mult < 1 {
			mult = 1
		}
		if ok {
			reward += base * mult
		}
		if s.Points != nil {
			_ = s.Points.Do(r.Context(), "POST", "/internal/record", map[string]any{"organization_id": a.OrgID, "student_user_id": a.UserID, "service_code": "listening", "question_id": qid, "event_key": "listening:" + r.PathValue("id") + ":" + qid, "base_points": base, "multiplier": mult, "correct": ok, "reason": "listening_answer"}, nil)
		}
	}
	score := 100 * float64(correct) / float64(len(key))
	ab, _ := json.Marshal(x.Answers)
	_, e = s.DB.Exec(r.Context(), `UPDATE listening_attempts SET answers=$2,score=$3,status='completed',finished_at=now() WHERE id=$1`, r.PathValue("id"), ab, score)
	if e != nil {
		return e
	}
	_ = s.Tenant.Do(r.Context(), "POST", "/internal/usage/release", map[string]any{
		"organization_id": a.OrgID, "service_code": "listening",
		"reservation_key": listeningReservationKey(assignmentID.String(), a.UserID),
	}, nil)
	s.emit(r.Context(), a.OrgID, a.UserID, "listening.completed", map[string]any{"attempt_id": r.PathValue("id"), "score": score})
	webx.JSON(w, 200, map[string]any{"status": "completed", "score": score, "correct": correct, "total": len(key), "rush_points": reward})
	return nil
}
func (s *Service) emit(ctx context.Context, org, user, typ string, payload map[string]any) {
	if s.Analytics != nil {
		_ = s.Analytics.Do(ctx, "POST", "/internal/events", map[string]any{"organization_id": org, "student_user_id": user, "service_code": "listening", "event_type": typ, "payload": payload}, nil)
	}
}
