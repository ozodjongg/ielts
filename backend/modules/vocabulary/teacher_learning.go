package vocabulary

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/example/ielts-platform/internal/authz"
	"github.com/example/ielts-platform/internal/webx"
	"github.com/google/uuid"
)

func requireTeacherActor(a authz.Actor) error {
	if a.Role != "teacher" || strings.TrimSpace(a.OrgID) == "" {
		return webx.E(403, "forbidden", "teacher required")
	}
	return nil
}

func (s *Service) validateStudentForTeacher(r *http.Request, a authz.Actor, studentID string) error {
	if _, err := uuid.Parse(studentID); err != nil {
		return webx.E(400, "student", "invalid student id")
	}
	var out struct {
		Valid bool `json:"valid"`
	}
	if err := s.Tenant.Do(r.Context(), "POST", "/internal/target/validate", map[string]any{
		"organization_id": a.OrgID,
		"target_type":     "student",
		"target_id":       studentID,
		"actor_role":      "teacher",
		"actor_user_id":   a.UserID,
	}, &out); err != nil || !out.Valid {
		return webx.E(404, "student", "student is not in one of this teacher's groups")
	}
	return nil
}

func (s *Service) teacherAssignWords(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireTeacherActor(a); err != nil {
		return err
	}
	studentID := r.PathValue("studentID")
	if err := s.validateStudentForTeacher(r, a, studentID); err != nil {
		return err
	}
	var x struct {
		LexemeIndexes []int64    `json:"lexeme_indexes"`
		Note          string     `json:"note"`
		DueAt         *time.Time `json:"due_at"`
	}
	if err := webx.Decode(r, &x, 128<<10); err != nil {
		return err
	}
	if len(x.LexemeIndexes) == 0 || len(x.LexemeIndexes) > 200 {
		return webx.E(400, "lexeme_indexes", "provide 1-200 words")
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	added := 0
	for _, idx := range x.LexemeIndexes {
		var exists bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM lexemes WHERE lemma_index=$1 AND active)`, idx).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return webx.E(400, "word", "one or more vocabulary words do not exist")
		}
		ct, e := tx.Exec(r.Context(), `INSERT INTO student_extra_words(organization_id,student_user_id,lexeme_index,assigned_by_teacher_user_id,note,due_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(student_user_id,lexeme_index,assigned_by_teacher_user_id) DO UPDATE SET note=excluded.note,due_at=excluded.due_at,created_at=now()`, a.OrgID, studentID, idx, a.UserID, strings.TrimSpace(x.Note), x.DueAt)
		if e != nil {
			return e
		}
		if ct.RowsAffected() > 0 {
			added++
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO student_word_state(organization_id,student_user_id,lexeme_index,due_at,first_seen_at,last_seen_at,search_count,interval_minutes,next_review_at,discovery_source,status) VALUES($1,$2,$3,current_date,now(),now(),0,0,now(),'teacher_assignment','learning') ON CONFLICT(student_user_id,lexeme_index) DO UPDATE SET status='learning',next_review_at=LEAST(student_word_state.next_review_at,now()),updated_at=now()`, a.OrgID, studentID, idx)
		if e != nil {
			return e
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"ok": true, "assigned": added, "student_user_id": studentID})
	return nil
}

func (s *Service) teacherStudentWords(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireTeacherActor(a); err != nil {
		return err
	}
	studentID := r.PathValue("studentID")
	if err := s.validateStudentForTeacher(r, a, studentID); err != nil {
		return err
	}
	rows, err := s.DB.Query(r.Context(), `SELECT x.id,x.created_at,x.note,x.due_at,l.lemma_index,l.english,l.uzbek,l.part_of_speech,l.cefr,l.level_source,l.frequency_rank,l.synonym_group_id,l.source_name,l.source_license FROM student_extra_words x JOIN lexemes l ON l.lemma_index=x.lexeme_index WHERE x.organization_id=$1 AND x.student_user_id=$2 ORDER BY x.created_at DESC LIMIT 500`, a.OrgID, studentID)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var created time.Time
		var note string
		var due *time.Time
		var lx lexeme
		var uz []byte
		if err := rows.Scan(&id, &created, &note, &due, &lx.Index, &lx.English, &uz, &lx.POS, &lx.CEFR, &lx.LevelSource, &lx.FrequencyRank, &lx.SynonymGroup, &lx.SourceName, &lx.SourceLicense); err != nil {
			return err
		}
		lx.Uzbek = json.RawMessage(uz)
		items = append(items, map[string]any{"id": id.String(), "created_at": created, "note": note, "due_at": due, "word": lx})
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}

func (s *Service) teacherCreateHomework(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireTeacherActor(a); err != nil {
		return err
	}
	var x struct {
		Title          string     `json:"title"`
		Instructions   string     `json:"instructions"`
		DueAt          *time.Time `json:"due_at"`
		LexemeIndexes  []int64    `json:"lexeme_indexes"`
		StudentUserIDs []string   `json:"student_user_ids"`
	}
	if err := webx.Decode(r, &x, 512<<10); err != nil {
		return err
	}
	x.Title = strings.TrimSpace(x.Title)
	x.Instructions = strings.TrimSpace(x.Instructions)
	if x.Title == "" || len(x.Title) > 160 {
		return webx.E(400, "title", "homework title required")
	}
	if len(x.LexemeIndexes) == 0 || len(x.LexemeIndexes) > 200 {
		return webx.E(400, "words", "provide 1-200 vocabulary words")
	}
	if len(x.StudentUserIDs) == 0 || len(x.StudentUserIDs) > 500 {
		return webx.E(400, "students", "provide 1-500 students")
	}
	seenStudents := map[string]bool{}
	students := make([]string, 0, len(x.StudentUserIDs))
	for _, id := range x.StudentUserIDs {
		if seenStudents[id] {
			continue
		}
		if err := s.validateStudentForTeacher(r, a, id); err != nil {
			return err
		}
		seenStudents[id] = true
		students = append(students, id)
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var homeworkID uuid.UUID
	if err = tx.QueryRow(r.Context(), `INSERT INTO teacher_homework(organization_id,teacher_user_id,title,instructions,due_at) VALUES($1,$2,$3,$4,$5) RETURNING id`, a.OrgID, a.UserID, x.Title, x.Instructions, x.DueAt).Scan(&homeworkID); err != nil {
		return err
	}
	seenWords := map[int64]bool{}
	words := make([]int64, 0, len(x.LexemeIndexes))
	for pos, idx := range x.LexemeIndexes {
		if seenWords[idx] {
			continue
		}
		var exists bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM lexemes WHERE lemma_index=$1 AND active)`, idx).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return webx.E(400, "word", "one or more vocabulary words do not exist")
		}
		seenWords[idx] = true
		words = append(words, idx)
		if _, err = tx.Exec(r.Context(), `INSERT INTO teacher_homework_words(homework_id,lexeme_index,position) VALUES($1,$2,$3)`, homeworkID, idx, pos); err != nil {
			return err
		}
	}
	for _, studentID := range students {
		if _, err = tx.Exec(r.Context(), `INSERT INTO teacher_homework_students(homework_id,organization_id,student_user_id) VALUES($1,$2,$3)`, homeworkID, a.OrgID, studentID); err != nil {
			return err
		}
		for _, idx := range words {
			_, err = tx.Exec(r.Context(), `INSERT INTO student_extra_words(organization_id,student_user_id,lexeme_index,assigned_by_teacher_user_id,homework_id,note,due_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(student_user_id,lexeme_index,assigned_by_teacher_user_id) DO UPDATE SET homework_id=excluded.homework_id,note=excluded.note,due_at=excluded.due_at,created_at=now()`, a.OrgID, studentID, idx, a.UserID, homeworkID, x.Title, x.DueAt)
			if err != nil {
				return err
			}
			_, err = tx.Exec(r.Context(), `INSERT INTO student_word_state(organization_id,student_user_id,lexeme_index,due_at,first_seen_at,last_seen_at,search_count,interval_minutes,next_review_at,discovery_source,status) VALUES($1,$2,$3,current_date,now(),now(),0,0,now(),'teacher_homework','learning') ON CONFLICT(student_user_id,lexeme_index) DO UPDATE SET status='learning',next_review_at=LEAST(student_word_state.next_review_at,now()),updated_at=now()`, a.OrgID, studentID, idx)
			if err != nil {
				return err
			}
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	webx.JSON(w, 201, map[string]any{"id": homeworkID.String(), "title": x.Title, "student_count": len(students), "word_count": len(words), "due_at": x.DueAt})
	return nil
}

func (s *Service) teacherHomework(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireTeacherActor(a); err != nil {
		return err
	}
	rows, err := s.DB.Query(r.Context(), `SELECT h.id,h.title,h.instructions,h.due_at,h.status,h.created_at,(SELECT count(*) FROM teacher_homework_words w WHERE w.homework_id=h.id),(SELECT count(*) FROM teacher_homework_students st WHERE st.homework_id=h.id),(SELECT count(*) FROM teacher_homework_students st WHERE st.homework_id=h.id AND st.completed_at IS NOT NULL) FROM teacher_homework h WHERE h.organization_id=$1 AND h.teacher_user_id=$2 ORDER BY h.created_at DESC LIMIT 200`, a.OrgID, a.UserID)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var title, instructions, status string
		var due *time.Time
		var created time.Time
		var wc, sc, completed int
		if err := rows.Scan(&id, &title, &instructions, &due, &status, &created, &wc, &sc, &completed); err != nil {
			return err
		}
		items = append(items, map[string]any{"id": id.String(), "title": title, "instructions": instructions, "due_at": due, "status": status, "created_at": created, "word_count": wc, "student_count": sc, "completed_count": completed})
	}
	webx.JSON(w, 200, map[string]any{"items": items})
	return rows.Err()
}

func (s *Service) studentAssigned(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	rows, err := s.DB.Query(r.Context(), `SELECT x.id,x.created_at,x.note,x.due_at,x.homework_id,l.lemma_index,l.english,l.uzbek,l.part_of_speech,l.cefr,l.level_source,l.frequency_rank,l.synonym_group_id,l.source_name,l.source_license FROM student_extra_words x JOIN lexemes l ON l.lemma_index=x.lexeme_index WHERE x.organization_id=$1 AND x.student_user_id=$2 ORDER BY coalesce(x.due_at,'infinity'::timestamptz),x.created_at DESC LIMIT 500`, a.OrgID, a.UserID)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var created time.Time
		var note string
		var due *time.Time
		var homework *uuid.UUID
		var lx lexeme
		var uz []byte
		if err := rows.Scan(&id, &created, &note, &due, &homework, &lx.Index, &lx.English, &uz, &lx.POS, &lx.CEFR, &lx.LevelSource, &lx.FrequencyRank, &lx.SynonymGroup, &lx.SourceName, &lx.SourceLicense); err != nil {
			return err
		}
		lx.Uzbek = json.RawMessage(uz)
		var hw any = nil
		if homework != nil {
			hw = homework.String()
		}
		items = append(items, map[string]any{"id": id.String(), "created_at": created, "note": note, "due_at": due, "homework_id": hw, "word": lx})
	}
	hwRows, err := s.DB.Query(r.Context(), `SELECT h.id,h.title,h.instructions,h.due_at,h.created_at,st.completed_at FROM teacher_homework_students st JOIN teacher_homework h ON h.id=st.homework_id WHERE st.organization_id=$1 AND st.student_user_id=$2 AND h.status='active' ORDER BY coalesce(h.due_at,'infinity'::timestamptz),h.created_at DESC LIMIT 100`, a.OrgID, a.UserID)
	if err != nil {
		return err
	}
	defer hwRows.Close()
	homework := []map[string]any{}
	for hwRows.Next() {
		var id uuid.UUID
		var title, instructions string
		var due, completed *time.Time
		var created time.Time
		if err := hwRows.Scan(&id, &title, &instructions, &due, &created, &completed); err != nil {
			return err
		}
		homework = append(homework, map[string]any{"id": id.String(), "title": title, "instructions": instructions, "due_at": due, "created_at": created, "completed_at": completed})
	}
	webx.JSON(w, 200, map[string]any{"words": items, "homework": homework})
	return nil
}

func (s *Service) studentCompleteHomework(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if a.Role != "student" {
		return webx.E(403, "forbidden", "student required")
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		return webx.E(400, "homework", "invalid homework id")
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE teacher_homework_students st SET completed_at=coalesce(st.completed_at,now()) FROM teacher_homework h WHERE st.homework_id=h.id AND st.homework_id=$1 AND st.organization_id=$2 AND st.student_user_id=$3 AND h.status='active'`, id, a.OrgID, a.UserID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() != 1 {
		return webx.E(404, "homework", "homework assignment not found")
	}
	var completed time.Time
	if err := s.DB.QueryRow(r.Context(), `SELECT completed_at FROM teacher_homework_students WHERE homework_id=$1 AND organization_id=$2 AND student_user_id=$3`, id, a.OrgID, a.UserID).Scan(&completed); err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"ok": true, "homework_id": id, "completed_at": completed})
	return nil
}
