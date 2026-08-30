package vocabulary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/example/ielts-platform/internal/authz"
	"github.com/example/ielts-platform/internal/clientx"
	"github.com/example/ielts-platform/internal/webx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB                                  *pgxpool.Pool
	Tenant, Identity, Points, Analytics *clientx.Client
	InternalSecret                      string
	DailyNew                            int
	DailyReview                         int
}

type lexeme struct {
	Index         int64           `json:"index"`
	English       string          `json:"english"`
	Uzbek         json.RawMessage `json:"uzbek"`
	POS           *string         `json:"part_of_speech"`
	CEFR          string          `json:"cefr"`
	LevelSource   string          `json:"level_source"`
	FrequencyRank *int            `json:"frequency_rank"`
	SynonymGroup  *int64          `json:"synonym_group_id"`
	SourceName    string          `json:"source_name"`
	SourceLicense string          `json:"source_license"`
}

type profile struct {
	UserID         string  `json:"user_id"`
	OrganizationID *string `json:"organization_id"`
	Role           string  `json:"role"`
	CurrentLevel   *string `json:"current_level"`
}

type reviewState struct {
	LexemeIndex     int64      `json:"lexeme_index"`
	SearchCount     int        `json:"search_count"`
	ReviewCount     int        `json:"review_count"`
	CorrectCount    int        `json:"correct_count"`
	IncorrectCount  int        `json:"incorrect_count"`
	IntervalMinutes int        `json:"interval_minutes"`
	Mastery         float64    `json:"mastery"`
	NextReviewAt    time.Time  `json:"next_review_at"`
	LastReviewAt    *time.Time `json:"last_review_at,omitempty"`
	Status          string     `json:"status"`
}

type dueReviewItem struct {
	Word            lexeme     `json:"word"`
	SearchCount     int        `json:"search_count"`
	ReviewCount     int        `json:"review_count"`
	CorrectCount    int        `json:"correct_count"`
	IncorrectCount  int        `json:"incorrect_count"`
	IntervalMinutes int        `json:"interval_minutes"`
	Mastery         float64    `json:"mastery"`
	NextReviewAt    time.Time  `json:"next_review_at"`
	LastReviewAt    *time.Time `json:"last_review_at,omitempty"`
	Status          string     `json:"status"`
}

func (s *Service) Router() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		var n int64
		_ = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM lexemes WHERE active`).Scan(&n)
		webx.JSON(w, 200, map[string]any{"status": "ok", "service": "vocabulary", "lexemes": n, "target_capacity": 100000})
	})
	m.HandleFunc("GET /v1/search", webx.Handle(s.search))
	m.HandleFunc("GET /v1/words/{index}", webx.Handle(s.word))
	m.HandleFunc("GET /v1/words/{index}/synonyms", webx.Handle(s.synonyms))
	m.HandleFunc("POST /v1/words/{index}/seen", webx.Handle(s.seen))
	m.HandleFunc("GET /v1/review/due", webx.Handle(s.reviewDue))
	m.HandleFunc("POST /v1/review/{index}/grade", webx.Handle(s.reviewGrade))
	m.HandleFunc("GET /v1/daily", webx.Handle(s.daily))
	m.HandleFunc("POST /v1/daily/{sessionID}/grade", webx.Handle(s.grade))
	m.HandleFunc("GET /v1/stats", webx.Handle(s.stats))
	m.HandleFunc("POST /v1/teacher/words/check", webx.Handle(s.centerCheckWords))
	m.HandleFunc("POST /v1/teacher/words", webx.Handle(s.centerAddWord))
	m.HandleFunc("POST /v1/teacher/words/batch", webx.Handle(s.centerAddWordsBatch))
	m.HandleFunc("GET /v1/teacher/contributions", webx.Handle(s.centerContributions))
	m.HandleFunc("POST /v1/teacher/students/{studentID}/words", webx.Handle(s.teacherAssignWords))
	m.HandleFunc("GET /v1/teacher/students/{studentID}/words", webx.Handle(s.teacherStudentWords))
	m.HandleFunc("POST /v1/teacher/homework", webx.Handle(s.teacherCreateHomework))
	m.HandleFunc("GET /v1/teacher/homework", webx.Handle(s.teacherHomework))
	m.HandleFunc("GET /v1/assigned", webx.Handle(s.studentAssigned))
	m.HandleFunc("POST /v1/assigned/homework/{id}/complete", webx.Handle(s.studentCompleteHomework))
	m.HandleFunc("GET /internal/corpus/stats", webx.Handle(s.internalStats))
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
func validLevel(x string) bool {
	switch x {
	case "A1", "A2", "B1", "B2", "C1", "C2":
		return true
	}
	return false
}

func scanLexeme(row pgx.Row) (lexeme, error) {
	var x lexeme
	var uz []byte
	err := row.Scan(&x.Index, &x.English, &uz, &x.POS, &x.CEFR, &x.LevelSource, &x.FrequencyRank, &x.SynonymGroup, &x.SourceName, &x.SourceLicense)
	x.Uzbek = json.RawMessage(append([]byte(nil), uz...))
	return x, err
}

func (s *Service) search(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if a.Role != "student" && a.Role != "center" && a.Role != "teacher" && a.Role != "admin" {
		return webx.E(403, "forbidden", "invalid role")
	}
	q := normalizeEnglish(r.URL.Query().Get("q"))
	level := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("level")))
	if level != "" && !validLevel(level) {
		return webx.E(400, "level", "invalid CEFR level")
	}
	limit := 20
	if v, _ := strconv.Atoi(r.URL.Query().Get("limit")); v > 0 && v <= 100 {
		limit = v
	}
	var rows pgx.Rows
	if q == "" {
		if level == "" {
			return webx.E(400, "q", "q or level required")
		}
		rows, err = s.DB.Query(r.Context(), `SELECT lemma_index,english,uzbek,part_of_speech,cefr,level_source,frequency_rank,synonym_group_id,source_name,source_license FROM lexemes WHERE active AND cefr=$1 ORDER BY frequency_rank NULLS LAST,lemma_index LIMIT $2`, level, limit)
	} else if level == "" {
		rows, err = s.DB.Query(r.Context(), `SELECT lemma_index,english,uzbek,part_of_speech,cefr,level_source,frequency_rank,synonym_group_id,source_name,source_license FROM lexemes WHERE active AND (normalized_english LIKE $1 OR public.similarity(normalized_english,$2)>0.25) ORDER BY CASE WHEN normalized_english=$2 THEN 0 ELSE 1 END, public.similarity(normalized_english,$2) DESC,frequency_rank NULLS LAST LIMIT $3`, q+"%", q, limit)
	} else {
		rows, err = s.DB.Query(r.Context(), `SELECT lemma_index,english,uzbek,part_of_speech,cefr,level_source,frequency_rank,synonym_group_id,source_name,source_license FROM lexemes WHERE active AND cefr=$1 AND (normalized_english LIKE $2 OR public.similarity(normalized_english,$3)>0.25) ORDER BY public.similarity(normalized_english,$3) DESC,frequency_rank NULLS LAST LIMIT $4`, level, q+"%", q, limit)
	}
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []lexeme{}
	for rows.Next() {
		var x lexeme
		var uz []byte
		if err := rows.Scan(&x.Index, &x.English, &uz, &x.POS, &x.CEFR, &x.LevelSource, &x.FrequencyRank, &x.SynonymGroup, &x.SourceName, &x.SourceLicense); err != nil {
			return err
		}
		x.Uzbek = json.RawMessage(uz)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var enrolled *reviewState
	if a.Role == "student" && q != "" {
		for _, item := range out {
			if normalizeEnglish(item.English) == q {
				state, seenErr := s.recordSeen(r.Context(), a, item.Index, "dictionary_search")
				if seenErr != nil {
					return seenErr
				}
				enrolled = &state
				break
			}
		}
	}
	webx.JSON(w, 200, map[string]any{"items": out, "review_enrolled": enrolled})
	return nil
}
func (s *Service) word(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.actor(r); err != nil {
		return err
	}
	idx, err := strconv.ParseInt(r.PathValue("index"), 10, 64)
	if err != nil {
		return webx.E(400, "index", "invalid word index")
	}
	x, err := scanLexeme(s.DB.QueryRow(r.Context(), `SELECT lemma_index,english,uzbek,part_of_speech,cefr,level_source,frequency_rank,synonym_group_id,source_name,source_license FROM lexemes WHERE lemma_index=$1 AND active`, idx))
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(404, "word", "word not found")
	}
	if err != nil {
		return err
	}
	webx.JSON(w, 200, x)
	return nil
}
func (s *Service) synonyms(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.actor(r); err != nil {
		return err
	}
	idx, err := strconv.ParseInt(r.PathValue("index"), 10, 64)
	if err != nil {
		return webx.E(400, "index", "invalid word index")
	}
	rows, err := s.DB.Query(r.Context(), `WITH wanted AS (SELECT synonym_group_id FROM lexemes WHERE lemma_index=$1) SELECT l.lemma_index,l.english,l.uzbek,l.part_of_speech,l.cefr,l.level_source,l.frequency_rank,l.synonym_group_id,l.source_name,l.source_license,coalesce(e.weight,0.75) FROM lexemes l LEFT JOIN synonym_edges e ON e.lexeme_index=$1 AND e.synonym_lexeme_index=l.lemma_index,wanted w WHERE l.active AND l.lemma_index<>$1 AND ((w.synonym_group_id IS NOT NULL AND l.synonym_group_id=w.synonym_group_id) OR e.synonym_lexeme_index IS NOT NULL) ORDER BY coalesce(e.weight,0.75) DESC,l.frequency_rank NULLS LAST LIMIT 50`, idx)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var x lexeme
		var uz []byte
		var weight float64
		if err := rows.Scan(&x.Index, &x.English, &uz, &x.POS, &x.CEFR, &x.LevelSource, &x.FrequencyRank, &x.SynonymGroup, &x.SourceName, &x.SourceLicense, &weight); err != nil {
			return err
		}
		x.Uzbek = json.RawMessage(uz)
		out = append(out, map[string]any{"word": x, "weight": weight})
	}
	webx.JSON(w, 200, map[string]any{"items": out})
	return rows.Err()
}

func normalizeEnglish(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

type centerWordInput struct {
	English   string   `json:"english"`
	Uzbek     []string `json:"uzbek"`
	CEFR      string   `json:"cefr"`
	POS       string   `json:"part_of_speech"`
	SourceRef string   `json:"source_ref,omitempty"`
}

type centerWordResult struct {
	English string  `json:"english"`
	Exists  bool    `json:"exists"`
	Added   bool    `json:"added"`
	Word    *lexeme `json:"word,omitempty"`
	Error   string  `json:"error,omitempty"`
}

func validCenterPOS(value string) bool {
	switch value {
	case "", "noun", "verb", "adjective", "adverb", "pronoun", "determiner", "preposition", "conjunction", "interjection", "numeral", "modal", "phrase", "phrasal_verb":
		return true
	default:
		return false
	}
}

func validEnglishLexical(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 80 || len(strings.Fields(value)) > 4 {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) {
			if r > unicode.MaxASCII {
				return false
			}
			continue
		}
		switch r {
		case ' ', '\'', '-', '’':
			continue
		default:
			return false
		}
	}
	return true
}

func validUzbekLexical(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 120 || len(strings.Fields(value)) > 6 {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) {
			if !unicode.In(r, unicode.Latin) {
				return false
			}
			continue
		}
		switch r {
		case ' ', '\'', '-', '’', '‘', 'ʻ', 'ʼ', '`', '´':
			continue
		default:
			return false
		}
	}
	return true
}

func normalizeCenterWordInput(in centerWordInput) (centerWordInput, error) {
	in.English = normalizeEnglish(in.English)
	in.CEFR = strings.ToUpper(strings.TrimSpace(in.CEFR))
	in.POS = strings.ToLower(strings.TrimSpace(in.POS))
	in.SourceRef = strings.TrimSpace(in.SourceRef)
	if !validEnglishLexical(in.English) {
		return in, webx.E(400, "english", "English must be a word or short lexical phrase (max 4 words, letters/apostrophe/hyphen only)")
	}
	if !validLevel(in.CEFR) {
		return in, webx.E(400, "cefr", "CEFR must be A1, A2, B1, B2, C1, or C2")
	}
	if !validCenterPOS(in.POS) {
		return in, webx.E(400, "part_of_speech", "unsupported part_of_speech")
	}
	seen := map[string]bool{}
	clean := make([]string, 0, len(in.Uzbek))
	for _, item := range in.Uzbek {
		item = strings.Join(strings.Fields(strings.TrimSpace(item)), " ")
		if !validUzbekLexical(item) {
			return in, webx.E(400, "uzbek", "Uzbek translations must be short Latin-script lexical terms")
		}
		key := strings.ToLower(item)
		if !seen[key] {
			seen[key] = true
			clean = append(clean, item)
		}
	}
	if len(clean) == 0 || len(clean) > 12 {
		return in, webx.E(400, "uzbek", "provide 1-12 Uzbek translations")
	}
	in.Uzbek = clean
	return in, nil
}

func (s *Service) lookupLexemeByNormalized(ctx context.Context, q pgx.Row) (*lexeme, error) {
	x, err := scanLexeme(q)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &x, nil
}

func (s *Service) centerCheckWords(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireTeacherActor(a); err != nil {
		return err
	}
	var input struct {
		Words []string `json:"words"`
	}
	if err := webx.Decode(r, &input, 128<<10); err != nil {
		return err
	}
	if len(input.Words) == 0 || len(input.Words) > 200 {
		return webx.E(400, "words", "provide 1-200 English words")
	}
	results := make([]centerWordResult, 0, len(input.Words))
	seen := map[string]bool{}
	for _, raw := range input.Words {
		n := normalizeEnglish(raw)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		res := centerWordResult{English: n}
		if !validEnglishLexical(n) {
			res.Error = "invalid lexical form"
			results = append(results, res)
			continue
		}
		item, qerr := s.lookupLexemeByNormalized(r.Context(), s.DB.QueryRow(r.Context(), `SELECT lemma_index,english,uzbek,part_of_speech,cefr,level_source,frequency_rank,synonym_group_id,source_name,source_license FROM lexemes WHERE normalized_english=$1 AND active ORDER BY lemma_index LIMIT 1`, n))
		if qerr != nil {
			return qerr
		}
		if item != nil {
			res.Exists = true
			res.Word = item
		}
		results = append(results, res)
	}
	webx.JSON(w, 200, map[string]any{"items": results})
	return nil
}

func (s *Service) insertCenterWord(ctx context.Context, a authz.Actor, raw centerWordInput) (centerWordResult, error) {
	in, err := normalizeCenterWordInput(raw)
	if err != nil {
		return centerWordResult{English: normalizeEnglish(raw.English)}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return centerWordResult{}, err
	}
	defer tx.Rollback(ctx)

	// Serializes center contributions for the same normalized word and prevents
	// duplicate center-created lexemes under concurrent requests.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, in.English); err != nil {
		return centerWordResult{}, err
	}
	existing, err := s.lookupLexemeByNormalized(ctx, tx.QueryRow(ctx, `SELECT lemma_index,english,uzbek,part_of_speech,cefr,level_source,frequency_rank,synonym_group_id,source_name,source_license FROM lexemes WHERE normalized_english=$1 AND active ORDER BY lemma_index LIMIT 1`, in.English))
	if err != nil {
		return centerWordResult{}, err
	}
	if existing != nil {
		if err := tx.Commit(ctx); err != nil {
			return centerWordResult{}, err
		}
		return centerWordResult{English: in.English, Exists: true, Word: existing}, nil
	}

	uzJSON, err := json.Marshal(in.Uzbek)
	if err != nil {
		return centerWordResult{}, err
	}
	sourceRef := in.SourceRef
	if sourceRef == "" {
		sourceRef = "teacher:" + a.OrgID + ":user:" + a.UserID
	}
	var idx int64
	err = tx.QueryRow(ctx, `
		INSERT INTO lexemes(english,normalized_english,uzbek,part_of_speech,cefr,level_source,source_name,source_license,source_ref,active)
		VALUES($1,$2,$3::jsonb,nullif($4,''),$5,'teacher','teacher-contribution','Teacher-provided educational content; verify redistribution rights',$6,true)
		RETURNING lemma_index`, in.English, in.English, string(uzJSON), in.POS, in.CEFR, sourceRef).Scan(&idx)
	if err != nil {
		return centerWordResult{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO teacher_contributions(organization_id,teacher_user_id,lexeme_index,english,normalized_english) VALUES($1,$2,$3,$4,$5) ON CONFLICT(organization_id,lexeme_index) DO NOTHING`, a.OrgID, a.UserID, idx, in.English, in.English); err != nil {
		return centerWordResult{}, err
	}
	item, err := s.lookupLexemeByNormalized(ctx, tx.QueryRow(ctx, `SELECT lemma_index,english,uzbek,part_of_speech,cefr,level_source,frequency_rank,synonym_group_id,source_name,source_license FROM lexemes WHERE lemma_index=$1`, idx))
	if err != nil {
		return centerWordResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return centerWordResult{}, err
	}
	return centerWordResult{English: in.English, Added: true, Word: item}, nil
}

func (s *Service) centerAddWord(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireTeacherActor(a); err != nil {
		return err
	}
	var input centerWordInput
	if err := webx.Decode(r, &input, 64<<10); err != nil {
		return err
	}
	result, err := s.insertCenterWord(r.Context(), a, input)
	if err != nil {
		return err
	}
	status := http.StatusCreated
	if result.Exists {
		status = http.StatusOK
	}
	webx.JSON(w, status, result)
	return nil
}

func (s *Service) centerAddWordsBatch(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireTeacherActor(a); err != nil {
		return err
	}
	var input struct {
		Items []centerWordInput `json:"items"`
	}
	if err := webx.Decode(r, &input, 512<<10); err != nil {
		return err
	}
	if len(input.Items) == 0 || len(input.Items) > 100 {
		return webx.E(400, "items", "provide 1-100 vocabulary items")
	}
	results := make([]centerWordResult, 0, len(input.Items))
	added, existing, failed := 0, 0, 0
	for _, item := range input.Items {
		result, itemErr := s.insertCenterWord(r.Context(), a, item)
		if itemErr != nil {
			failed++
			message := "word could not be added"
			var clientErr *webx.Error
			if errors.As(itemErr, &clientErr) {
				message = clientErr.Message
			}
			result = centerWordResult{English: normalizeEnglish(item.English), Error: message}
		} else if result.Added {
			added++
		} else if result.Exists {
			existing++
		}
		results = append(results, result)
	}
	webx.JSON(w, 200, map[string]any{"items": results, "added": added, "existing": existing, "failed": failed})
	return nil
}

func (s *Service) centerContributions(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if err := requireTeacherActor(a); err != nil {
		return err
	}
	limit := 100
	if v, _ := strconv.Atoi(r.URL.Query().Get("limit")); v > 0 && v <= 200 {
		limit = v
	}
	rows, err := s.DB.Query(r.Context(), `
		SELECT c.created_at,l.lemma_index,l.english,l.uzbek,l.part_of_speech,l.cefr,l.level_source,
		       l.frequency_rank,l.synonym_group_id,l.source_name,l.source_license
		FROM teacher_contributions c
		JOIN lexemes l ON l.lemma_index=c.lexeme_index
		WHERE c.organization_id=$1
		ORDER BY c.created_at DESC
		LIMIT $2`, a.OrgID, limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var created time.Time
		var x lexeme
		var uz []byte
		if err := rows.Scan(&created, &x.Index, &x.English, &uz, &x.POS, &x.CEFR, &x.LevelSource, &x.FrequencyRank, &x.SynonymGroup, &x.SourceName, &x.SourceLicense); err != nil {
			return err
		}
		x.Uzbek = json.RawMessage(uz)
		out = append(out, map[string]any{"created_at": created, "word": x})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"items": out})
	return nil
}

func (s *Service) recordSeen(ctx context.Context, a authz.Actor, idx int64, source string) (reviewState, error) {
	var state reviewState
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lexemes WHERE lemma_index=$1 AND active)`, idx).Scan(&exists); err != nil {
		return state, err
	}
	if !exists {
		return state, webx.E(404, "word", "word not found")
	}
	if source == "" {
		source = "dictionary"
	}
	err := s.DB.QueryRow(ctx, `
		INSERT INTO student_word_state(
			organization_id,student_user_id,lexeme_index,due_at,
			first_seen_at,last_seen_at,search_count,interval_minutes,next_review_at,discovery_source,status
		) VALUES($1,$2,$3,current_date,now(),now(),1,90,now()+interval '90 minutes',$4,'learning')
		ON CONFLICT(student_user_id,lexeme_index) DO UPDATE SET
			first_seen_at=coalesce(student_word_state.first_seen_at,now()),
			last_seen_at=now(),
			search_count=student_word_state.search_count+1,
			discovery_source=coalesce(student_word_state.discovery_source,excluded.discovery_source),
			updated_at=now()
		RETURNING lexeme_index,search_count,review_count,correct_count,incorrect_count,
			interval_minutes,mastery,next_review_at,last_review_at,status`,
		a.OrgID, a.UserID, idx, source).Scan(
		&state.LexemeIndex, &state.SearchCount, &state.ReviewCount, &state.CorrectCount,
		&state.IncorrectCount, &state.IntervalMinutes, &state.Mastery, &state.NextReviewAt,
		&state.LastReviewAt, &state.Status,
	)
	if err != nil {
		return state, err
	}
	s.emit(ctx, a.OrgID, a.UserID, "vocabulary.word.seen", map[string]any{
		"lexeme_index": idx, "source": source, "search_count": state.SearchCount,
	})
	return state, nil
}

func (s *Service) seen(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "student") != nil {
		return webx.E(403, "forbidden", "student required")
	}
	idx, err := strconv.ParseInt(r.PathValue("index"), 10, 64)
	if err != nil || idx <= 0 {
		return webx.E(400, "index", "invalid word index")
	}
	state, err := s.recordSeen(r.Context(), a, idx, "dictionary_open")
	if err != nil {
		return err
	}
	webx.JSON(w, 200, state)
	return nil
}

func (s *Service) reviewDue(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "student") != nil {
		return webx.E(403, "forbidden", "student required")
	}
	limit := 20
	if v, parseErr := strconv.Atoi(r.URL.Query().Get("limit")); parseErr == nil && v > 0 && v <= 100 {
		limit = v
	}
	rows, err := s.DB.Query(r.Context(), `
		SELECT l.lemma_index,l.english,l.uzbek,l.part_of_speech,l.cefr,l.level_source,
			l.frequency_rank,l.synonym_group_id,l.source_name,l.source_license,
			sws.search_count,sws.review_count,sws.correct_count,sws.incorrect_count,
			sws.interval_minutes,sws.mastery,sws.next_review_at,sws.last_review_at,sws.status
		FROM student_word_state sws
		JOIN lexemes l ON l.lemma_index=sws.lexeme_index
		WHERE sws.student_user_id=$1 AND sws.status<>'suspended' AND sws.search_count>0
			AND sws.next_review_at<=now() AND l.active
		ORDER BY sws.next_review_at ASC,sws.mastery ASC,l.frequency_rank NULLS LAST
		LIMIT $2`, a.UserID, limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []dueReviewItem{}
	for rows.Next() {
		var item dueReviewItem
		var uz []byte
		if err := rows.Scan(
			&item.Word.Index, &item.Word.English, &uz, &item.Word.POS, &item.Word.CEFR,
			&item.Word.LevelSource, &item.Word.FrequencyRank, &item.Word.SynonymGroup,
			&item.Word.SourceName, &item.Word.SourceLicense, &item.SearchCount, &item.ReviewCount,
			&item.CorrectCount, &item.IncorrectCount, &item.IntervalMinutes, &item.Mastery,
			&item.NextReviewAt, &item.LastReviewAt, &item.Status,
		); err != nil {
			return err
		}
		item.Word.Uzbek = json.RawMessage(uz)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var totalDue int
	var nextAt *time.Time
	if err := s.DB.QueryRow(r.Context(), `
		SELECT count(*) FILTER(WHERE search_count>0 AND next_review_at<=now() AND status<>'suspended'),
			min(next_review_at) FILTER(WHERE search_count>0 AND next_review_at>now() AND status<>'suspended')
		FROM student_word_state WHERE student_user_id=$1`, a.UserID).Scan(&totalDue, &nextAt); err != nil {
		return err
	}
	webx.JSON(w, 200, map[string]any{"items": items, "due_count": totalDue, "next_review_at": nextAt})
	return nil
}

func nextSearchReviewInterval(rating string, currentMinutes, reviewCount, searchCount int) int {
	minutes := currentMinutes
	if reviewCount == 0 {
		switch rating {
		case "again":
			minutes = 10
		case "hard":
			minutes = 60
		case "good":
			minutes = 1440
		case "easy":
			minutes = 4320
		}
	} else {
		if currentMinutes <= 0 {
			currentMinutes = 90
		}
		switch rating {
		case "again":
			minutes = 60
		case "hard":
			minutes = int(math.Round(float64(currentMinutes) * 1.5))
			if minutes < 180 {
				minutes = 180
			}
		case "good":
			minutes = int(math.Round(float64(currentMinutes) * 2.5))
			if minutes < 1440 {
				minutes = 1440
			}
		case "easy":
			minutes = int(math.Round(float64(currentMinutes) * 4.0))
			if minutes < 4320 {
				minutes = 4320
			}
		}
	}
	if rating != "again" {
		if searchCount >= 5 {
			minutes = int(math.Round(float64(minutes) * 0.70))
		} else if searchCount >= 3 {
			minutes = int(math.Round(float64(minutes) * 0.85))
		}
	}
	if minutes < 10 {
		minutes = 10
	}
	const maxMinutes = 180 * 24 * 60
	if minutes > maxMinutes {
		minutes = maxMinutes
	}
	return minutes
}

func (s *Service) reviewGrade(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "student") != nil {
		return webx.E(403, "forbidden", "student required")
	}
	idx, err := strconv.ParseInt(r.PathValue("index"), 10, 64)
	if err != nil || idx <= 0 {
		return webx.E(400, "index", "invalid word index")
	}
	var input struct {
		Rating string `json:"rating"`
	}
	if err := webx.Decode(r, &input, 32<<10); err != nil {
		return err
	}
	input.Rating = strings.ToLower(strings.TrimSpace(input.Rating))
	if input.Rating != "again" && input.Rating != "hard" && input.Rating != "good" && input.Rating != "easy" {
		return webx.E(400, "rating", "rating must be again, hard, good, or easy")
	}

	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())

	var currentMinutes, reviewCount, searchCount, repetitions int
	var mastery, ease float64
	var status string
	err = tx.QueryRow(r.Context(), `
		SELECT interval_minutes,review_count,search_count,repetitions,mastery,ease,status
		FROM student_word_state
		WHERE student_user_id=$1 AND lexeme_index=$2
		FOR UPDATE`, a.UserID, idx).Scan(&currentMinutes, &reviewCount, &searchCount, &repetitions, &mastery, &ease, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(404, "review", "word is not enrolled for review")
	}
	if err != nil {
		return err
	}
	if status == "suspended" {
		return webx.E(409, "review", "word review is suspended")
	}

	minutes := nextSearchReviewInterval(input.Rating, currentMinutes, reviewCount, searchCount)
	score := 0.0
	correct := input.Rating != "again"
	switch input.Rating {
	case "again":
		repetitions = 0
		ease = math.Max(1.3, ease-0.20)
	case "hard":
		score = 0.45
		repetitions++
		ease = math.Max(1.3, ease-0.10)
	case "good":
		score = 0.78
		repetitions++
	case "easy":
		score = 1.0
		repetitions++
		ease = math.Min(3.0, ease+0.10)
	}
	mastery = 0.75*mastery + 0.25*score
	nextAt := time.Now().UTC().Add(time.Duration(minutes) * time.Minute)
	newReviewCount := reviewCount + 1
	newStatus := "learning"
	if mastery >= 0.90 && newReviewCount >= 5 {
		newStatus = "mastered"
	}

	gradeValue := map[string]int{"again": 0, "hard": 3, "good": 4, "easy": 5}[input.Rating]
	commandTag, err := tx.Exec(r.Context(), `
		UPDATE student_word_state SET
			organization_id=$3::uuid,
			repetitions=$4::int,
			ease=$5::numeric,
			mastery=$6::numeric,
			last_grade=$7::int,
			review_count=review_count+1,
			correct_count=correct_count+CASE WHEN $8::boolean THEN 1 ELSE 0 END,
			incorrect_count=incorrect_count+CASE WHEN $8::boolean THEN 0 ELSE 1 END,
			interval_minutes=$9::int,
			next_review_at=$10::timestamptz,
			due_at=($10::timestamptz)::date,
			last_review_at=now(),
			status=$11::text,
			updated_at=now()
		WHERE student_user_id=$1::uuid AND lexeme_index=$2::bigint`,
		a.UserID, idx, a.OrgID, repetitions, ease, mastery,
		gradeValue, correct, minutes, nextAt, newStatus)
	if err != nil {
		return fmt.Errorf("review grade state update: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return webx.E(404, "review", "word is not enrolled for review")
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}

	qid := vocabularyQuestionUUID(idx)
	mult := 1.0
	if s.Points != nil {
		var quote struct {
			Items []struct {
				QuestionID string  `json:"question_id"`
				Multiplier float64 `json:"multiplier"`
			} `json:"items"`
		}
		if err := s.Points.Do(r.Context(), "POST", "/internal/quote/batch", map[string]any{"service_code": "daily_vocabulary", "question_ids": []string{qid}}, &quote); err == nil && len(quote.Items) > 0 && quote.Items[0].Multiplier >= 1 {
			mult = quote.Items[0].Multiplier
		}
		_ = s.Points.Do(r.Context(), "POST", "/internal/record", map[string]any{
			"organization_id": a.OrgID, "student_user_id": a.UserID, "service_code": "daily_vocabulary",
			"question_id": qid, "event_key": fmt.Sprintf("search-review:%s:%d:%d", a.UserID, idx, newReviewCount),
			"base_points": 1.0, "multiplier": mult, "correct": correct, "reason": "dictionary_spaced_recall",
		}, nil)
	}
	s.emit(r.Context(), a.OrgID, a.UserID, "vocabulary.review.graded", map[string]any{
		"lexeme_index": idx, "rating": input.Rating, "interval_minutes": minutes, "mastery": mastery,
	})
	webx.JSON(w, 200, map[string]any{
		"ok": true, "rating": input.Rating, "next_review_at": nextAt, "interval_minutes": minutes,
		"mastery": mastery, "status": newStatus, "review_count": newReviewCount, "rush_multiplier": mult,
	})
	return nil
}

func (s *Service) currentProfile(ctx context.Context, a authz.Actor) (profile, error) {
	var p profile
	if err := s.Identity.Do(ctx, "GET", "/internal/resolve?user_id="+url.QueryEscape(a.UserID), nil, &p); err != nil {
		return p, err
	}
	return p, nil
}
func (s *Service) reserve(ctx context.Context, org string, amount int, key string) error {
	var q struct {
		Allowed   bool   `json:"allowed"`
		Reason    string `json:"reason"`
		Remaining int64  `json:"remaining"`
	}
	err := s.Tenant.Do(ctx, "POST", "/internal/usage/reserve", map[string]any{
		"organization_id": org, "service_code": "daily_vocabulary", "amount": amount,
		"reservation_key": key, "hold_concurrency": false, "lease_minutes": 10,
	}, &q)
	if err != nil {
		return webx.E(429, "quota", "daily vocabulary quota exceeded")
	}
	if !q.Allowed {
		return webx.E(429, "quota", q.Reason)
	}
	return nil
}

func (s *Service) cancel(ctx context.Context, org, key string) {
	_ = s.Tenant.Do(ctx, "POST", "/internal/usage/cancel", map[string]any{"organization_id": org, "service_code": "daily_vocabulary", "reservation_key": key}, nil)
}

func (s *Service) organizationDay(ctx context.Context, org string) (string, error) {
	var x struct {
		Timezone string `json:"timezone"`
	}
	if err := s.Tenant.Do(ctx, "GET", "/internal/organization/"+url.QueryEscape(org), nil, &x); err != nil {
		return "", err
	}
	loc, err := time.LoadLocation(x.Timezone)
	if err != nil {
		return "", fmt.Errorf("invalid organization timezone %q: %w", x.Timezone, err)
	}
	return time.Now().In(loc).Format("2006-01-02"), nil
}

type dailyItem struct {
	Position int     `json:"position"`
	Word     lexeme  `json:"word"`
	IsReview bool    `json:"is_review"`
	Mastery  float64 `json:"mastery"`
}

func (s *Service) loadSession(ctx context.Context, id uuid.UUID, student string) ([]dailyItem, error) {
	rows, err := s.DB.Query(ctx, `SELECT di.position,l.lemma_index,l.english,l.uzbek,l.part_of_speech,l.cefr,l.level_source,l.frequency_rank,l.synonym_group_id,l.source_name,l.source_license,coalesce(sws.mastery,0),coalesce(sws.repetitions,0)>0 FROM daily_items di JOIN daily_sessions ds ON ds.id=di.session_id JOIN lexemes l ON l.lemma_index=di.lexeme_index LEFT JOIN student_word_state sws ON sws.student_user_id=ds.student_user_id AND sws.lexeme_index=l.lemma_index WHERE di.session_id=$1 AND ds.student_user_id=$2 ORDER BY di.position`, id, student)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []dailyItem{}
	for rows.Next() {
		var it dailyItem
		var uz []byte
		if err := rows.Scan(&it.Position, &it.Word.Index, &it.Word.English, &uz, &it.Word.POS, &it.Word.CEFR, &it.Word.LevelSource, &it.Word.FrequencyRank, &it.Word.SynonymGroup, &it.Word.SourceName, &it.Word.SourceLicense, &it.Mastery, &it.IsReview); err != nil {
			return nil, err
		}
		it.Word.Uzbek = json.RawMessage(uz)
		out = append(out, it)
	}
	return out, rows.Err()
}
func (s *Service) daily(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "student") != nil {
		return webx.E(403, "forbidden", "student required")
	}
	p, err := s.currentProfile(r.Context(), a)
	if err != nil {
		return err
	}
	level := "A1"
	if p.CurrentLevel != nil && validLevel(*p.CurrentLevel) {
		level = *p.CurrentLevel
	}
	day, err := s.organizationDay(r.Context(), a.OrgID)
	if err != nil {
		return err
	}
	var sid uuid.UUID
	var completed *time.Time
	var newN, reviewN int
	err = s.DB.QueryRow(r.Context(), `SELECT id,new_count,review_count,completed_at FROM daily_sessions WHERE student_user_id=$1 AND day=$2::date`, a.UserID, day).Scan(&sid, &newN, &reviewN, &completed)
	if err == nil {
		items, e := s.loadSession(r.Context(), sid, a.UserID)
		if e != nil {
			return e
		}
		webx.JSON(w, 200, map[string]any{"session_id": sid, "level": level, "new_count": newN, "review_count": reviewN, "completed_at": completed, "items": items})
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	reviewLimit := s.DailyReview
	if reviewLimit <= 0 {
		reviewLimit = 10
	}
	newLimit := s.DailyNew
	if newLimit <= 0 {
		newLimit = 10
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	reviewRows, err := tx.Query(r.Context(), `SELECT sws.lexeme_index FROM student_word_state sws JOIN lexemes l ON l.lemma_index=sws.lexeme_index WHERE sws.student_user_id=$1 AND sws.status<>'suspended' AND sws.next_review_at<=now() AND l.active ORDER BY sws.next_review_at,sws.mastery LIMIT $2`, a.UserID, reviewLimit)
	if err != nil {
		return err
	}
	ids := []int64{}
	for reviewRows.Next() {
		var i int64
		if err := reviewRows.Scan(&i); err != nil {
			return err
		}
		ids = append(ids, i)
	}
	reviewRows.Close()
	reviews := len(ids)
	needed := newLimit
	rows, err := tx.Query(r.Context(), `SELECT l.lemma_index FROM lexemes l WHERE l.active AND l.cefr=$1 AND NOT EXISTS(SELECT 1 FROM student_word_state s WHERE s.student_user_id=$2 AND s.lexeme_index=l.lemma_index) ORDER BY l.frequency_rank NULLS LAST,l.lemma_index LIMIT $3`, level, a.UserID, needed)
	if err != nil {
		return err
	}
	for rows.Next() {
		var i int64
		if err := rows.Scan(&i); err != nil {
			return err
		}
		ids = append(ids, i)
	}
	rows.Close()
	if len(ids) == 0 {
		return webx.E(503, "corpus_empty", "no vocabulary entries are loaded for this level")
	}
	reservationKey := "daily-vocabulary:" + a.UserID + ":" + day
	if err := s.reserve(r.Context(), a.OrgID, len(ids), reservationKey); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			s.cancel(context.Background(), a.OrgID, reservationKey)
		}
	}()
	err = tx.QueryRow(r.Context(), `INSERT INTO daily_sessions(organization_id,student_user_id,level,day,new_count,review_count) VALUES($1,$2,$3,$4::date,$5,$6) RETURNING id`, a.OrgID, a.UserID, level, day, len(ids)-reviews, reviews).Scan(&sid)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.Exec(r.Context(), `INSERT INTO daily_items(session_id,position,lexeme_index) VALUES($1,$2,$3)`, sid, i+1, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	committed = true
	items, err := s.loadSession(r.Context(), sid, a.UserID)
	if err != nil {
		return err
	}
	s.emit(r.Context(), a.OrgID, a.UserID, "vocabulary.daily.created", map[string]any{"session_id": sid.String(), "level": level, "count": len(items)})
	webx.JSON(w, 201, map[string]any{"session_id": sid, "level": level, "new_count": len(ids) - reviews, "review_count": reviews, "items": items})
	return nil
}
func vocabularyQuestionUUID(index int64) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("ielts-platform:vocabulary:%d", index))).String()
}

func (s *Service) grade(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "student") != nil {
		return webx.E(403, "forbidden", "student required")
	}
	sid, err := uuid.Parse(r.PathValue("sessionID"))
	if err != nil {
		return webx.E(400, "session", "invalid session")
	}
	var x struct {
		Position int `json:"position"`
		Grade    int `json:"grade"`
	}
	if err := webx.Decode(r, &x, 64<<10); err != nil {
		return err
	}
	if x.Position < 1 || x.Position > 100 || x.Grade < 0 || x.Grade > 5 {
		return webx.E(400, "grade", "position 1-100 and grade 0-5 required")
	}
	var idx int64
	err = s.DB.QueryRow(r.Context(), `SELECT di.lexeme_index FROM daily_items di JOIN daily_sessions ds ON ds.id=di.session_id WHERE di.session_id=$1 AND di.position=$2 AND ds.student_user_id=$3`, sid, x.Position, a.UserID).Scan(&idx)
	if errors.Is(err, pgx.ErrNoRows) {
		return webx.E(404, "item", "daily item not found")
	}
	if err != nil {
		return err
	}
	day, err := s.organizationDay(r.Context(), a.OrgID)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var reps, interval int
	ease := 2.5
	mastery := 0.0
	err = tx.QueryRow(r.Context(), `SELECT repetitions,interval_days,ease,mastery FROM student_word_state WHERE student_user_id=$1 AND lexeme_index=$2 FOR UPDATE`, a.UserID, idx).Scan(&reps, &interval, &ease, &mastery)
	if errors.Is(err, pgx.ErrNoRows) {
		reps = 0
		interval = 0
		ease = 2.5
		mastery = 0
	} else if err != nil {
		return err
	}
	if x.Grade < 3 {
		reps = 0
		interval = 1
	} else {
		reps++
		if reps == 1 {
			interval = 1
		} else if reps == 2 {
			interval = 6
		} else {
			interval = int(math.Round(float64(interval) * ease))
			if interval < 1 {
				interval = 1
			}
		}
	}
	q := float64(5 - x.Grade)
	ease = ease + (0.1 - q*(0.08+q*0.02))
	if ease < 1.3 {
		ease = 1.3
	}
	mastery = 0.75*mastery + 0.25*(float64(x.Grade)/5)
	_, err = tx.Exec(r.Context(), `INSERT INTO student_word_state(organization_id,student_user_id,lexeme_index,repetitions,interval_days,ease,mastery,due_at,last_grade,review_count,correct_count,incorrect_count,interval_minutes,next_review_at,last_review_at,status) VALUES($1,$2,$3,$4,$5,$6,$7,$9::date+$5::int,$8,1,CASE WHEN $8>=3 THEN 1 ELSE 0 END,CASE WHEN $8>=3 THEN 0 ELSE 1 END,$5*1440,($9::date+$5::int)::timestamptz,now(),'learning') ON CONFLICT(student_user_id,lexeme_index) DO UPDATE SET repetitions=$4,interval_days=$5,ease=$6,mastery=$7,due_at=$9::date+$5::int,last_grade=$8,review_count=student_word_state.review_count+1,correct_count=student_word_state.correct_count+CASE WHEN $8>=3 THEN 1 ELSE 0 END,incorrect_count=student_word_state.incorrect_count+CASE WHEN $8>=3 THEN 0 ELSE 1 END,interval_minutes=$5*1440,next_review_at=($9::date+$5::int)::timestamptz,last_review_at=now(),status=CASE WHEN $7>=0.9 AND student_word_state.review_count+1>=5 THEN 'mastered' ELSE 'learning' END,updated_at=now()`, a.OrgID, a.UserID, idx, reps, interval, ease, mastery, x.Grade, day)
	if err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), `UPDATE daily_items SET grade=$3,answered_at=now() WHERE session_id=$1 AND position=$2`, sid, x.Position, x.Grade)
	if err != nil {
		return err
	}
	var remaining int
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM daily_items WHERE session_id=$1 AND grade IS NULL`, sid).Scan(&remaining); err != nil {
		return err
	}
	if remaining == 0 {
		_, err = tx.Exec(r.Context(), `UPDATE daily_sessions SET completed_at=coalesce(completed_at,now()) WHERE id=$1 AND student_user_id=$2`, sid, a.UserID)
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	qid := vocabularyQuestionUUID(idx)
	mult := 1.0
	if s.Points != nil {
		var quote struct {
			Items []struct {
				QuestionID string  `json:"question_id"`
				Multiplier float64 `json:"multiplier"`
			} `json:"items"`
		}
		if err := s.Points.Do(r.Context(), "POST", "/internal/quote/batch", map[string]any{"service_code": "daily_vocabulary", "question_ids": []string{qid}}, &quote); err == nil && len(quote.Items) > 0 && quote.Items[0].Multiplier >= 1 {
			mult = quote.Items[0].Multiplier
		}
		_ = s.Points.Do(r.Context(), "POST", "/internal/record", map[string]any{"organization_id": a.OrgID, "student_user_id": a.UserID, "service_code": "daily_vocabulary", "question_id": qid, "event_key": fmt.Sprintf("daily-vocabulary:%s:%d", sid.String(), x.Position), "base_points": 1.0, "multiplier": mult, "correct": x.Grade >= 3, "reason": "daily_vocabulary_recall"}, nil)
	}
	webx.JSON(w, 200, map[string]any{"ok": true, "remaining": remaining, "next_due_days": interval, "mastery": mastery, "rush_multiplier": mult})
	return nil
}

func (s *Service) stats(w http.ResponseWriter, r *http.Request) error {
	a, err := s.actor(r)
	if err != nil {
		return err
	}
	if authz.Require(a, "student") != nil {
		return webx.E(403, "forbidden", "student required")
	}
	day, err := s.organizationDay(r.Context(), a.OrgID)
	if err != nil {
		return err
	}
	var learned, dueAll, dueSearch, searchedWords, totalSearches, reviews, mastered int
	var mastery float64
	var nextReview *time.Time
	err = s.DB.QueryRow(r.Context(), `SELECT count(*),coalesce(avg(mastery),0),count(*) FILTER(WHERE next_review_at<=now() AND status<>'suspended'),count(*) FILTER(WHERE search_count>0 AND next_review_at<=now() AND status<>'suspended'),count(*) FILTER(WHERE search_count>0),coalesce(sum(search_count),0),coalesce(sum(review_count),0),count(*) FILTER(WHERE status='mastered'),min(next_review_at) FILTER(WHERE search_count>0 AND next_review_at>now() AND status<>'suspended') FROM student_word_state WHERE student_user_id=$1`, a.UserID).Scan(&learned, &mastery, &dueAll, &dueSearch, &searchedWords, &totalSearches, &reviews, &mastered, &nextReview)
	if err != nil {
		return err
	}
	var streak int
	_ = s.DB.QueryRow(r.Context(), `WITH d AS (SELECT day,row_number() over(order by day desc) rn FROM daily_sessions WHERE student_user_id=$1 AND completed_at IS NOT NULL),x AS(SELECT count(*) n FROM d WHERE day=$2::date-(rn-1)::int) SELECT coalesce(max(n),0) FROM x`, a.UserID, day).Scan(&streak)
	webx.JSON(w, 200, map[string]any{"learned": learned, "average_mastery": mastery, "due_today": dueAll, "due_now": dueSearch, "searched_words": searchedWords, "total_searches": totalSearches, "reviews": reviews, "mastered": mastered, "next_review_at": nextReview, "streak_days": streak})
	return nil
}

func (s *Service) internalStats(w http.ResponseWriter, r *http.Request) error {
	if err := s.serviceAuth(r, "gateway", "analytics"); err != nil {
		return err
	}
	rows, err := s.DB.Query(r.Context(), `SELECT cefr,count(*) FROM lexemes WHERE active GROUP BY cefr ORDER BY cefr`)
	if err != nil {
		return err
	}
	defer rows.Close()
	by := map[string]int64{}
	var total int64
	for rows.Next() {
		var l string
		var n int64
		if err := rows.Scan(&l, &n); err != nil {
			return err
		}
		by[l] = n
		total += n
	}
	webx.JSON(w, 200, map[string]any{"total": total, "by_level": by, "target": 100000, "coverage_percent": 100 * float64(total) / 100000.0})
	return rows.Err()
}

func (s *Service) emit(ctx context.Context, org, user, typ string, payload map[string]any) {
	if s.Analytics == nil {
		return
	}
	_ = s.Analytics.Do(ctx, "POST", "/internal/events", map[string]any{
		"organization_id": org,
		"student_user_id": user,
		"service_code":    "vocabulary",
		"event_type":      typ,
		"payload":         payload,
	}, nil)
}
