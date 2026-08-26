# Production Checklist

## Release gate

- [x] Repository static/data/API-contract QA returns `ok: true` in the packaged release.
- [ ] Run `go test ./...` on a machine/CI with access to `proxy.golang.org`.
- [ ] In each frontend root run `corepack enable && pnpm install --frozen-lockfile && pnpm typecheck && pnpm lint && pnpm build` with npm registry access.
- [ ] Confirm no `.env`, database password or signing secret is committed.
- [ ] If using a generated production vocabulary file, sample/review it and preserve source/license metadata.
- [x] No noisy OPUS sentence corpus is included in the production release.

## Railway

- [ ] PostgreSQL service created.
- [ ] Backend builds from repository-root `Dockerfile`.
- [ ] Persistent volume mounted at `/data`.
- [ ] `DATABASE_URL=${{Postgres.DATABASE_URL}}`.
- [ ] `AUTO_MIGRATE=false` because the Docker entrypoint runs migrations first.
- [ ] `VOCAB_AUTO_IMPORT=true` or intentionally disabled.
- [ ] Four unrelated signing secrets generated, each >=32 characters.
- [ ] Healthcheck path `/ready`.
- [ ] Backend public domain generated.

## Vercel

- [ ] Admin root: `apps/admin-web`.
- [ ] Center root: `apps/center-web`.
- [ ] Student root: `apps/student-web`.
- [ ] Each has `NEXT_PUBLIC_API_URL=https://<railway-backend>`.
- [ ] Final Vercel URLs copied into matching Railway CORS origins.

## Functional smoke test

- [ ] Platform admin login and center creation.
- [ ] Center admin login and student/group creation.
- [ ] Center Vocabulary Manager can check/add a missing word and does not duplicate an existing word.
- [ ] Student login.
- [ ] Dictionary search enrolls spaced review.
- [ ] Again/Hard/Good/Easy review flow works.
- [ ] Daily vocabulary grading works.
- [ ] English assessment start/answer/finish works.
- [ ] Listening upload/playback works and survives backend redeploy.
- [ ] Review, SAT, Points and Analytics smoke tests pass.
- [ ] Backend redeploy skips applied migrations and the already-imported vocabulary dataset.

## Operations

- [ ] Railway PostgreSQL backup/restore policy configured.
- [ ] `/data` volume backup plan configured if private audio must be retained.
- [ ] Production logs reviewed for errors after first deploy.
- [ ] Keep `REQUIRE_ADMIN_AAL2=false` and `REQUIRE_CENTER_AAL2=false` until a real AAL2/TOTP flow is implemented.
