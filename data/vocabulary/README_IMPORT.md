# Assessment Platform V5 — 100K Vocabulary Data

Files:
- `vocabulary_100k.csv` — exactly 100,000 importer-compatible entries
- `synonyms_100k.csv` — conservative synonym edges derived only from identical Uzbek translations
- `SOURCE_NOTICES.md` — provenance/licensing notes
- `QA_REPORT.json` — generation statistics

## Import

Use your existing V5 importer:

```bash
./.local-run/bin/vocab-import.exe \
  -database "$VOCABULARY_DATABASE_URL" \
  -words "vocabulary_100k.csv" \
  -synonyms "synonyms_100k.csv"
```

Or from `backend/`:

```bash
go run ./cmd/vocab-import \
  -database "$VOCABULARY_DATABASE_URL" \
  -words "../data/vocabulary/vocabulary_100k.csv" \
  -synonyms "../data/vocabulary/synonyms_100k.csv"
```

## Counts

- Rows: 100,000
- Unique normalized English entries: 100,000
- Word entries: 332
- Phrase entries: 7,809
- Longer expressions: 91,859
- Synonym groups: 47
- Synonym edges: 94

CEFR:
- A1: 10,000
- A2: 15,000
- B1: 20,000
- B2: 25,000
- C1: 20,000
- C2: 10,000

## CEFR warning

The CEFR labels are **heuristic**. They are based on English corpus frequency, rarity, token
length and expression complexity. They are intended to make the V5 daily-learning pipeline
usable across A1–C2 immediately; they should not be presented as Cambridge/CEFR-certified labels.
