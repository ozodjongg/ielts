package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/ielts-platform/internal/bank"
)

type manifest struct {
	Version     string   `json:"version"`
	BankVersion string   `json:"bank_version"`
	Seed        string   `json:"seed"`
	QuestionIDs []string `json:"question_ids"`
}

func main() {
	bankDir := flag.String("bank", "../data/english-bank", "English bank directory")
	output := flag.String("out", "../data/placement/paper-v1.json", "Manifest output path")
	count := flag.Int("count", 40, "Number of questions")
	seed := flag.String("seed", "placement-paper-v1", "Deterministic selection seed")
	flag.Parse()

	b, err := bank.LoadEnglish(*bankDir)
	if err != nil {
		panic(fmt.Errorf("load English bank: %w", err))
	}
	questions := b.BuildBalanced([]string{"A1", "A2", "B1", "B2", "C1"}, nil, *count, *seed, nil)
	if len(questions) != *count {
		panic(fmt.Errorf("requested %d questions, bank returned %d", *count, len(questions)))
	}
	ids := make([]string, 0, len(questions))
	for _, q := range questions {
		ids = append(ids, q.UUID)
	}
	out := manifest{Version: "placement-paper-v1", BankVersion: b.Version, Seed: *seed, QuestionIDs: ids}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(*output, append(raw, '\n'), 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s (%d questions)\n", *output, len(ids))
}
