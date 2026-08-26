# Assessment Platform V5 — Production Release

Production-oriented learning/assessment platform with one Go backend, one PostgreSQL database split into module schemas, and three independent Next.js portals.

## Applications

- `apps/admin-web` — platform administration
- `apps/center-web` — Learning Center administration
- `apps/student-web` — student learning portal
- `backend` — Go modular monolith

## Main capabilities

- Center/student/group administration
- English assessments and level progression
- SAT Math
- Private listening audio and assignments
- Speaking/writing review workflow
- Points, analytics and service quotas
- English→Uzbek dictionary
- Search-triggered spaced vocabulary review
- Learning Center Vocabulary Manager for adding missing verified words
- Monochrome light/dark theme across all portals

## Local Windows / Git Bash

Requirements: Go 1.23+, Node 20.9+, PostgreSQL, npm.

```bash
bash local.sh start
```

Local URLs:

```text
Admin    http://localhost:3001
Center   http://localhost:3002
Student  http://localhost:3003
Backend  http://localhost:8080
Ready    http://localhost:8080/ready
```

Create the first local platform admin:

```bash
bash local.sh admin
```

## Vocabulary

The old noisy OPUS sentence corpus is not included in this production release.

Bundled:

- `data/vocabulary/demo_seed.csv` — 180 clean bootstrap lexemes
- `data/vocabulary/demo_synonyms.csv` — clean demo synonym pairs
- `tools/collect_vocabulary_stage1.py` — WordNet/CEFR clean collector
- `tools/collect_vocabulary_stage2_panlex.py` — optional PanLex enrichment stage

Preferred production file:

```text
data/vocabulary/generated/stage1_import_ready.csv
```

When present, Railway Docker startup imports it automatically once per dataset version. Otherwise Docker uses the clean 180-word bootstrap dataset.

## Learning Center Vocabulary Manager

Center route:

```text
/vocabulary-manager
```

A center admin can:

- check up to 200 English lexical entries against the global vocabulary table;
- add a missing word/short lexical phrase;
- bulk add up to 100 verified EN→UZ entries;
- view their center's contribution history.

Duplicate prevention and input validation are enforced by the backend.

## Production deployment

Recommended topology:

```text
Admin Vercel   ─┐
Center Vercel  ─┼──> Railway Go backend ──> Railway PostgreSQL
Student Vercel ─┘              │
                                └──> Railway /data volume for private audio
```

See [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) and [`PRODUCTION_CHECKLIST.md`](PRODUCTION_CHECKLIST.md).

## QA

Run:

```bash
python tools/qa_v5.py
```

The packaged release passed the repository's static/data/API-contract QA. Full dependency-backed `go test` and Next.js production builds must also be run in an environment with access to Go/npm registries (Railway/Vercel or a connected local machine).
