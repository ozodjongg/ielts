package bank

import (
	"encoding/csv"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

type SATQuestion struct {
	ID, TopicCode, Domain, Prompt        string
	EquivalentNo, Difficulty, BasePoints int
	Options                              [4]string
	Correct                              string
	Explanation                          string
}
type SATBank struct {
	Version   string
	Questions map[string]SATQuestion
	ByTopic   map[string][]SATQuestion
	Topics    []string
}

func LoadSAT(path string) (*SATBank, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	hrow, err := r.Read()
	if err != nil {
		return nil, err
	}
	h := map[string]int{}
	for i, x := range hrow {
		h[strings.TrimPrefix(x, "\ufeff")] = i
	}
	g := func(row []string, k string) string { return row[h[k]] }
	b := &SATBank{Version: "sat-math-v5.0.0", Questions: map[string]SATQuestion{}, ByTopic: map[string][]SATQuestion{}}
	seen := map[string]bool{}
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		eq, _ := strconv.Atoi(g(row, "equivalent_no"))
		dif, _ := strconv.Atoi(g(row, "difficulty"))
		pts, _ := strconv.Atoi(g(row, "base_points"))
		q := SATQuestion{ID: g(row, "id"), TopicCode: g(row, "topic_code"), Domain: g(row, "domain"), Prompt: g(row, "prompt"), EquivalentNo: eq, Difficulty: dif, BasePoints: pts, Options: [4]string{g(row, "option_a"), g(row, "option_b"), g(row, "option_c"), g(row, "option_d")}, Correct: g(row, "correct_option"), Explanation: g(row, "explanation")}
		b.Questions[q.ID] = q
		b.ByTopic[q.TopicCode] = append(b.ByTopic[q.TopicCode], q)
		if !seen[q.TopicCode] {
			seen[q.TopicCode] = true
			b.Topics = append(b.Topics, q.TopicCode)
		}
	}
	return b, nil
}
