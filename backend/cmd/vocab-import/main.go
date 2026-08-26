package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type wordRow struct {
	English        string
	Uzbek          []string
	PartOfSpeech   *string
	CEFR           string
	FrequencyRank  *int
	SynonymGroupID *int64
	SourceName     string
	SourceLicense  string
	SourceRef      *string
}

type synonymRow struct {
	English string
	Synonym string
	Weight  float64
	Source  string
}

func main() {
	var wordsPath, synonymsPath, dsn string
	flag.StringVar(&wordsPath, "words", "", "CSV containing vocabulary rows (required)")
	flag.StringVar(&synonymsPath, "synonyms", "", "optional synonym edge CSV")
	flag.StringVar(&dsn, "database", os.Getenv("VOCABULARY_DATABASE_URL"), "PostgreSQL DSN; defaults to VOCABULARY_DATABASE_URL")
	flag.Parse()
	if wordsPath == "" || dsn == "" {
		flag.Usage()
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	must(err)
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = "vocabulary,public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	must(err)
	defer pool.Close()
	must(pool.Ping(ctx))

	words, err := readWords(wordsPath)
	must(err)
	if len(words) == 0 {
		log.Fatal("words CSV has no data rows")
	}
	inserted, updated, err := importWords(ctx, pool, words)
	must(err)
	log.Printf("vocabulary import complete: rows=%d inserted=%d updated=%d", len(words), inserted, updated)

	if synonymsPath != "" {
		edges, err := readSynonyms(synonymsPath)
		must(err)
		resolved, unresolved, err := importSynonyms(ctx, pool, edges)
		must(err)
		log.Printf("synonym import complete: rows=%d resolved=%d unresolved=%d", len(edges), resolved, unresolved)
		if unresolved > 0 {
			log.Printf("warning: %d synonym rows referenced words absent from lexemes", unresolved)
		}
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func headerIndex(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[normalize(h)] = i
	}
	return m
}

func value(row []string, idx map[string]int, key string) string {
	i, ok := idx[key]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func requireHeaders(idx map[string]int, keys ...string) error {
	for _, k := range keys {
		if _, ok := idx[k]; !ok {
			return fmt.Errorf("missing required CSV column %q", k)
		}
	}
	return nil
}

func readWords(path string) ([]wordRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := headerIndex(header)
	if err := requireHeaders(idx, "english", "uzbek", "cefr", "source_name", "source_license"); err != nil {
		return nil, err
	}
	validLevel := map[string]bool{"A1": true, "A2": true, "B1": true, "B2": true, "C1": true, "C2": true}
	var out []wordRow
	for line := 2; ; line++ {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		english := strings.TrimSpace(value(row, idx, "english"))
		if english == "" {
			return nil, fmt.Errorf("line %d: english is required", line)
		}
		parts := strings.Split(value(row, idx, "uzbek"), "|")
		uz := make([]string, 0, len(parts))
		seen := map[string]bool{}
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" && !seen[p] {
				seen[p] = true
				uz = append(uz, p)
			}
		}
		if len(uz) == 0 {
			return nil, fmt.Errorf("line %d: at least one Uzbek translation is required", line)
		}
		level := strings.ToUpper(value(row, idx, "cefr"))
		if !validLevel[level] {
			return nil, fmt.Errorf("line %d: invalid CEFR %q", line, level)
		}
		source := value(row, idx, "source_name")
		license := value(row, idx, "source_license")
		if source == "" || license == "" {
			return nil, fmt.Errorf("line %d: source_name and source_license are mandatory", line)
		}
		var pos, ref *string
		if v := value(row, idx, "part_of_speech"); v != "" {
			pos = &v
		}
		if v := value(row, idx, "source_ref"); v != "" {
			ref = &v
		}
		var rank *int
		if v := value(row, idx, "frequency_rank"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("line %d: invalid frequency_rank", line)
			}
			rank = &n
		}
		var group *int64
		if v := value(row, idx, "synonym_group_id"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("line %d: invalid synonym_group_id", line)
			}
			group = &n
		}
		out = append(out, wordRow{English: english, Uzbek: uz, PartOfSpeech: pos, CEFR: level, FrequencyRank: rank, SynonymGroupID: group, SourceName: source, SourceLicense: license, SourceRef: ref})
	}
	return out, nil
}

func importWords(ctx context.Context, pool *pgxpool.Pool, rows []wordRow) (inserted, updated int64, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE vocab_import_stage (
		english text NOT NULL, normalized_english text NOT NULL, uzbek jsonb NOT NULL,
		part_of_speech text, cefr text NOT NULL, frequency_rank integer, synonym_group_id bigint,
		source_name text NOT NULL, source_license text NOT NULL, source_ref text
	) ON COMMIT DROP`)
	if err != nil {
		return 0, 0, err
	}
	copyRows := make([][]any, 0, len(rows))
	for _, x := range rows {
		jb, _ := json.Marshal(x.Uzbek)
		copyRows = append(copyRows, []any{x.English, normalize(x.English), jb, x.PartOfSpeech, x.CEFR, x.FrequencyRank, x.SynonymGroupID, x.SourceName, x.SourceLicense, x.SourceRef})
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"vocab_import_stage"}, []string{"english", "normalized_english", "uzbek", "part_of_speech", "cefr", "frequency_rank", "synonym_group_id", "source_name", "source_license", "source_ref"}, pgx.CopyFromRows(copyRows))
	if err != nil {
		return 0, 0, err
	}
	// Count rows that already exist before the UPSERT for an auditable import summary.
	err = tx.QueryRow(ctx, `SELECT count(*) FROM vocab_import_stage s JOIN lexemes l
		ON l.normalized_english=s.normalized_english AND l.source_name=s.source_name
		AND coalesce(l.part_of_speech,'')=coalesce(s.part_of_speech,'')`).Scan(&updated)
	if err != nil {
		return 0, 0, err
	}
	inserted = int64(len(rows)) - updated
	_, err = tx.Exec(ctx, `INSERT INTO lexemes(
		english, normalized_english, uzbek, part_of_speech, cefr, level_source,
		frequency_rank, synonym_group_id, source_name, source_license, source_ref, active)
	SELECT english, normalized_english, uzbek, part_of_speech, cefr, 'import',
		frequency_rank, synonym_group_id, source_name, source_license, source_ref, true
	FROM vocab_import_stage
	ON CONFLICT (normalized_english, source_name, (coalesce(part_of_speech,''))) DO UPDATE SET
		english=excluded.english, uzbek=excluded.uzbek, cefr=excluded.cefr,
		frequency_rank=excluded.frequency_rank, synonym_group_id=excluded.synonym_group_id,
		source_license=excluded.source_license, source_ref=excluded.source_ref, active=true`)
	if err != nil {
		return 0, 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return inserted, updated, nil
}

func readSynonyms(path string) ([]synonymRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := headerIndex(header)
	if err := requireHeaders(idx, "english", "synonym", "source"); err != nil {
		return nil, err
	}
	var out []synonymRow
	for line := 2; ; line++ {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		a, b, source := normalize(value(row, idx, "english")), normalize(value(row, idx, "synonym")), value(row, idx, "source")
		if a == "" || b == "" || a == b || source == "" {
			return nil, fmt.Errorf("line %d: english, synonym, distinct values, and source are required", line)
		}
		weight := 1.0
		if v := value(row, idx, "weight"); v != "" {
			weight, err = strconv.ParseFloat(v, 64)
			if err != nil || weight <= 0 || weight > 1 {
				return nil, fmt.Errorf("line %d: weight must be >0 and <=1", line)
			}
		}
		out = append(out, synonymRow{English: a, Synonym: b, Weight: weight, Source: source})
	}
	return out, nil
}

func importSynonyms(ctx context.Context, pool *pgxpool.Pool, rows []synonymRow) (resolved, unresolved int64, err error) {
	if len(rows) == 0 {
		return 0, 0, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE synonym_import_stage (
		english_norm text NOT NULL, synonym_norm text NOT NULL, weight numeric(5,4) NOT NULL, source text NOT NULL
	) ON COMMIT DROP`)
	if err != nil {
		return 0, 0, err
	}
	copyRows := make([][]any, 0, len(rows))
	for _, x := range rows {
		copyRows = append(copyRows, []any{x.English, x.Synonym, x.Weight, x.Source})
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"synonym_import_stage"}, []string{"english_norm", "synonym_norm", "weight", "source"}, pgx.CopyFromRows(copyRows))
	if err != nil {
		return 0, 0, err
	}
	// Resolve a canonical active lexeme for each English headword. Frequency rank
	// wins; lemma_index is the stable tie breaker.
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE synonym_resolved AS
	SELECT s.*,
	       (SELECT l.lemma_index FROM lexemes l WHERE l.active AND l.normalized_english=s.english_norm ORDER BY l.frequency_rank NULLS LAST,l.lemma_index LIMIT 1) a,
	       (SELECT l.lemma_index FROM lexemes l WHERE l.active AND l.normalized_english=s.synonym_norm ORDER BY l.frequency_rank NULLS LAST,l.lemma_index LIMIT 1) b
	FROM synonym_import_stage s`)
	if err != nil {
		return 0, 0, err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE a IS NOT NULL AND b IS NOT NULL), count(*) FILTER (WHERE a IS NULL OR b IS NULL) FROM synonym_resolved`).Scan(&resolved, &unresolved); err != nil {
		return 0, 0, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO synonym_edges(lexeme_index,synonym_lexeme_index,weight,source)
	SELECT a,b,weight,source FROM synonym_resolved WHERE a IS NOT NULL AND b IS NOT NULL AND a<>b
	UNION ALL
	SELECT b,a,weight,source FROM synonym_resolved WHERE a IS NOT NULL AND b IS NOT NULL AND a<>b
	ON CONFLICT(lexeme_index,synonym_lexeme_index) DO UPDATE SET weight=greatest(synonym_edges.weight,excluded.weight),source=excluded.source`)
	if err != nil {
		return 0, 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return resolved, unresolved, nil
}
