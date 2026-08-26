package bank

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Subject struct {
	UUID, ID, Name, ShortName, Category, Level string
	Difficulty, Point                          int
}
type Question struct {
	UUID, SubjectUUID, SubjectID string
	EquivalentNo                 int
	Text, Template               string
	Options                      []string
	CorrectIndex                 int
}
type EnglishBank struct {
	Version       string
	Subjects      []Subject
	Questions     map[string]Question
	BySubject     map[string][]Question
	SubjectByUUID map[string]Subject
	SubjectByCode map[string]Subject
}

var nonWord = regexp.MustCompile(`[^a-z0-9']+`)

func isASCIIWordRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

func Normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "’", "'")

	// Keep apostrophes only when they are inside a word. This preserves
	// contractions such as "don't" while treating quote marks around an
	// answer option as punctuation. The question-bank model contains both.
	runes := []rune(s)
	for i, r := range runes {
		if r != '\'' {
			continue
		}
		prevWord := i > 0 && isASCIIWordRune(runes[i-1])
		nextWord := i+1 < len(runes) && isASCIIWordRune(runes[i+1])
		if !prevWord || !nextWord {
			runes[i] = ' '
		}
	}

	s = nonWord.ReplaceAllString(string(runes), " ")
	return strings.Join(strings.Fields(s), " ")
}

func fill(tmpl, opt string) string {
	blankCount := strings.Count(tmpl, "___")
	if blankCount == 0 {
		return strings.TrimSpace(tmpl + " " + opt)
	}

	// Options for multi-blank questions are stored as slash-separated pieces,
	// for example: "Have / been" for "___ you ever ___ to Japan?".
	if blankCount > 1 {
		parts := strings.Split(opt, "/")
		if len(parts) == blankCount {
			out := tmpl
			for _, part := range parts {
				out = strings.Replace(out, "___", strings.TrimSpace(part), 1)
			}
			return out
		}
	}

	return strings.Replace(tmpl, "___", strings.TrimSpace(opt), 1)
}
func LoadEnglish(dir string) (*EnglishBank, error) {
	subjects, err := loadSubjects(filepath.Join(dir, "subjects.csv"))
	if err != nil {
		return nil, err
	}
	models, err := loadModels(filepath.Join(dir, "model.csv"))
	if err != nil {
		return nil, err
	}
	questions, by, err := loadQuestions(filepath.Join(dir, "questions.csv"), models)
	if err != nil {
		return nil, err
	}
	b := &EnglishBank{Version: "english-v5-from-v4-bank", Subjects: subjects, Questions: questions, BySubject: by, SubjectByUUID: map[string]Subject{}, SubjectByCode: map[string]Subject{}}
	for _, s := range subjects {
		b.SubjectByUUID[s.UUID] = s
		b.SubjectByCode[s.ShortName] = s
	}
	return b, nil
}
func reader(path string) (*csv.Reader, *os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	return r, f, nil
}
func header(r *csv.Reader) (map[string]int, error) {
	h, err := r.Read()
	if err != nil {
		return nil, err
	}
	m := map[string]int{}
	for i, x := range h {
		m[strings.TrimPrefix(x, "\ufeff")] = i
	}
	return m, nil
}
func get(row []string, h map[string]int, k string) string {
	i, ok := h[k]
	if !ok || i >= len(row) {
		return ""
	}
	return row[i]
}
func loadSubjects(path string) ([]Subject, error) {
	r, f, err := reader(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h, err := header(r)
	if err != nil {
		return nil, err
	}
	out := []Subject{}
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		d, _ := strconv.Atoi(get(row, h, "difficulty"))
		p, _ := strconv.Atoi(get(row, h, "point"))
		out = append(out, Subject{UUID: get(row, h, "subject_uuid"), ID: get(row, h, "subject_id"), Name: get(row, h, "name"), ShortName: get(row, h, "short_name"), Category: get(row, h, "category"), Level: get(row, h, "level"), Difficulty: d, Point: p})
	}
	return out, nil
}
func loadModels(path string) (map[string]map[string]struct{}, error) {
	r, f, err := reader(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h, err := header(r)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]struct{}{}
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		qid := get(row, h, "question_uuid")
		n := get(row, h, "normalized_text")
		if n == "" {
			n = Normalize(get(row, h, "accepted_text"))
		}
		if out[qid] == nil {
			out[qid] = map[string]struct{}{}
		}
		out[qid][Normalize(n)] = struct{}{}
	}
	return out, nil
}
func loadQuestions(path string, models map[string]map[string]struct{}) (map[string]Question, map[string][]Question, error) {
	r, f, err := reader(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	h, err := header(r)
	if err != nil {
		return nil, nil, err
	}
	all := map[string]Question{}
	by := map[string][]Question{}
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		qid := get(row, h, "question_uuid")
		opts := []string{}
		for _, k := range []string{"option_a", "option_b", "option_c", "option_d"} {
			if x := strings.TrimSpace(get(row, h, k)); x != "" {
				opts = append(opts, x)
			}
		}
		correct := -1
		for i, o := range opts {
			if _, ok := models[qid][Normalize(fill(get(row, h, "answer_template"), o))]; ok {
				if correct != -1 {
					return nil, nil, fmt.Errorf("multiple correct options for %s", qid)
				}
				correct = i
			}
		}
		if correct < 0 {
			return nil, nil, fmt.Errorf("cannot resolve correct option for %s", qid)
		}
		eq, _ := strconv.Atoi(get(row, h, "equivalent_no"))
		q := Question{UUID: qid, SubjectUUID: get(row, h, "subject_uuid"), SubjectID: get(row, h, "subject_id"), EquivalentNo: eq, Text: get(row, h, "question_text"), Template: get(row, h, "answer_template"), Options: opts, CorrectIndex: correct}
		all[qid] = q
		by[q.SubjectUUID] = append(by[q.SubjectUUID], q)
	}
	for k := range by {
		sort.Slice(by[k], func(i, j int) bool { return by[k][i].EquivalentNo < by[k][j].EquivalentNo })
	}
	return all, by, nil
}
func seeded(seed string) *rand.Rand {
	h := sha256.Sum256([]byte(seed))
	return rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(h[:8]))))
}
func (b *EnglishBank) Pick(subject Subject, n int, seed string) []Question {
	qs := append([]Question(nil), b.BySubject[subject.UUID]...)
	r := seeded(seed + subject.UUID)
	r.Shuffle(len(qs), func(i, j int) { qs[i], qs[j] = qs[j], qs[i] })
	if n > len(qs) {
		n = len(qs)
	}
	return qs[:n]
}
func (b *EnglishBank) SubjectsMatching(levels, categories []string) []Subject {
	lm := map[string]bool{}
	cm := map[string]bool{}
	for _, x := range levels {
		lm[x] = true
	}
	for _, x := range categories {
		cm[x] = true
	}
	out := []Subject{}
	for _, s := range b.Subjects {
		if len(lm) > 0 && !lm[s.Level] {
			continue
		}
		if len(cm) > 0 && !cm[s.Category] {
			continue
		}
		out = append(out, s)
	}
	return out
}
func (b *EnglishBank) BuildBalanced(levels, categories []string, count int, seed string, preferred []string) []Question {
	subs := b.SubjectsMatching(levels, categories)
	if len(preferred) > 0 {
		pm := map[string]bool{}
		for _, x := range preferred {
			pm[x] = true
		}
		sort.SliceStable(subs, func(i, j int) bool { return pm[subs[i].ShortName] && !pm[subs[j].ShortName] })
	}
	if len(subs) == 0 {
		return nil
	}
	r := seeded(seed)
	r.Shuffle(len(subs), func(i, j int) { subs[i], subs[j] = subs[j], subs[i] })
	out := []Question{}
	round := 0
	for len(out) < count && round < 100 {
		for _, s := range subs {
			if len(out) >= count {
				break
			}
			qs := b.Pick(s, 100, fmt.Sprintf("%s:%d", seed, round))
			if round < len(qs) {
				out = append(out, qs[round])
			}
		}
		round++
	}
	return out
}
