# Deployment — Railway backend/database + Vercel portals

## 1. Topology

```text
Vercel Admin   ─┐
Vercel Center  ─┼── HTTPS ──> Railway backend ──> Railway PostgreSQL
Vercel Student ─┘                    │
                                     └── Railway Volume mounted at /data
```

The backend is one Go process. PostgreSQL uses nine logical schemas: identity, tenant, assessment, vocabulary, listening, review, sat, points and analytics.

## 2. Push this release to GitHub

Do not commit `.env`, passwords, private keys or production signing secrets.

If you already generated the clean Stage 1 vocabulary on your local machine, include:

```text
data/vocabulary/generated/stage1_import_ready.csv
```

Optionally include a synonym file only when every referenced word exists in the selected lexeme dataset.

If no clean generated file is committed, the backend Docker image starts with the clean 180-word bootstrap corpus; it never auto-imports the removed OPUS sentence corpus.

## 3. Railway PostgreSQL

Create a PostgreSQL service in the same Railway project.

## 4. Railway backend

Create a service from the same GitHub repository.

```text
Root directory: repository root / blank
Dockerfile: /Dockerfile
Healthcheck: /ready
```

Do not set the root directory to `/backend`, because the root Docker build also needs migrations and data banks.

Attach a persistent Railway Volume:

```text
Mount path: /data
```

Private listening/review files are stored below `/data/private/...`.

### Required Railway variables

Start from `deploy/railway-backend.env.example`.

Minimum production set:

```env
DATABASE_URL=${{Postgres.DATABASE_URL}}
AUTO_MIGRATE=false
VOCAB_AUTO_IMPORT=true

AUTH_JWT_SECRET=<random 32+ chars>
INTERNAL_SIGNING_SECRET=<different random 32+ chars>
PLAYBACK_SIGNING_SECRET=<different random 32+ chars>
QUESTION_SHUFFLE_SECRET=<different random 32+ chars>

ADMIN_ORIGINS=https://YOUR-ADMIN.vercel.app
CENTER_ORIGINS=https://YOUR-CENTER.vercel.app
STUDENT_ORIGINS=https://YOUR-STUDENT.vercel.app
```

Generate independent secrets locally:

```bash
node - <<'NODE'
const { randomBytes } = require("crypto");
for (const name of [
  "AUTH_JWT_SECRET",
  "INTERNAL_SIGNING_SECRET",
  "PLAYBACK_SIGNING_SECRET",
  "QUESTION_SHUFFLE_SECRET",
]) console.log(`${name}=${randomBytes(48).toString("hex")}`);
NODE
```

The backend refuses signing secrets shorter than 32 characters.

## 5. Startup behavior

`deploy/docker-entrypoint.sh` performs:

```text
1. compact-migrate
2. clean vocabulary dataset check/import
3. backend start
```

### Migration safety

Applied migrations are recorded in:

```text
public.app_schema_migrations
```

Each migration stores a SHA-256 checksum. A PostgreSQL advisory lock serializes concurrent migration startup. If an already-applied migration file changes, startup fails with a migration-drift error instead of silently rewriting production history.

After a migration has shipped, create a new numbered migration; do not edit the old file.

### Vocabulary auto-import safety

Vocabulary dataset state is recorded in:

```text
vocabulary.dataset_versions
```

The entrypoint stores the word/synonym SHA-256 hashes. A unique dataset-version claim prevents two startup instances from importing the same version concurrently. Reusing an existing version with different file content fails fast; change `VOCAB_DATASET_VERSION` when shipping a new dataset.

Defaults:

```env
VOCAB_DATASET_VERSION=stage1-clean-v1
VOCAB_WORDS_FILE=/app/data/vocabulary/generated/stage1_import_ready.csv
```

If that file is absent, Docker falls back to `demo_seed.csv` as `demo-clean-v1`.

For a later clean dataset:

```env
VOCAB_DATASET_VERSION=stage2-clean-v1
VOCAB_WORDS_FILE=/app/data/vocabulary/generated/stage2_merged_import_ready.csv
```

## 6. Generate backend domain

Generate a Railway public domain and verify:

```text
GET https://YOUR-BACKEND/health
GET https://YOUR-BACKEND/ready
```

`/ready` must return HTTP 200.

## 7. Vercel — three independent projects

Import the same GitHub repository three times.

### Admin

```text
Root Directory: apps/admin-web
NEXT_PUBLIC_API_URL=https://YOUR-RAILWAY-BACKEND
```

### Learning Center

```text
Root Directory: apps/center-web
NEXT_PUBLIC_API_URL=https://YOUR-RAILWAY-BACKEND
```

### Student

```text
Root Directory: apps/student-web
NEXT_PUBLIC_API_URL=https://YOUR-RAILWAY-BACKEND
```

Each frontend contains a checked-in `pnpm-lock.yaml` and `packageManager` pin for reproducible Vercel installs.

After Vercel assigns final domains, copy the exact domains into Railway `ADMIN_ORIGINS`, `CENTER_ORIGINS`, and `STUDENT_ORIGINS`, then redeploy the backend.

## 8. First production admin

Open a Railway backend shell and run once:

```bash
/app/bootstrap-admin \
  --email "admin@example.com" \
  --password "Use-A-Unique-Strong-Password" \
  --name "Platform Admin"
```

Do not run `seed-demo` in production.

## 9. Smoke-test order

1. `/ready` returns 200.
2. Platform admin login.
3. Create a Learning Center.
4. Center admin login; create student/group.
5. Vocabulary Manager: check one missing word, add it, verify a second add is treated as existing.
6. Student dictionary search and spaced-review enrollment.
7. Daily vocabulary grade.
8. English assessment lifecycle.
9. Listening upload/playback and `/data` persistence across redeploy.
10. SAT, review, points and analytics.
11. Redeploy backend; verify migrations and the same vocabulary version are skipped.
