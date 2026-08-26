# Backend Runbook

## Startup sequence

1. Read `DATABASE_URL` and required signing secrets.
2. Wait for PostgreSQL readiness.
3. Apply migrations when `AUTO_MIGRATE=true`.
4. Open bounded schema-specific connection pools.
5. Load English and SAT question banks.
6. Initialize JWT/session/authentication components.
7. Initialize all business modules in-process.
8. Start the single HTTP server on the configured/Railway `PORT`.
9. `/ready` verifies database/module readiness.

## Authentication operations

- Failed logins: inspect `identity.auth_login_audit`.
- Active/revoked sessions: inspect `identity.auth_sessions`.
- Account lock state: inspect `identity.auth_credentials.locked_until`.
- Password reset utility: `/app/reset-password`.
- User status/password changes revoke existing sessions.

## Local Windows operations

From Git Bash:

```bash
bash local.sh start
bash local.sh status
bash local.sh logs backend
bash local.sh restart
bash local.sh stop
```

Populate local QA data only when needed:

```bash
bash local.sh seed-vocab
bash local.sh seed-demo
```

Run static/data/API QA:

```bash
bash local.sh qa
```

## Incident triage

### Backend unavailable

1. Check Railway deployment state/logs.
2. Check `/health`.
3. Check `/ready`.
4. Verify `DATABASE_URL` and PostgreSQL availability.
5. Verify required signing secrets are present.

### Browser CORS failure

1. Confirm the frontend is using the expected backend URL.
2. Confirm the exact Vercel origin is present in the matching backend origin variable.
3. Redeploy backend after origin changes.
4. Test `/ready` with the browser origin header.

### Login failures

1. Confirm the user is using the correct portal for their role.
2. Inspect failed-login audit records and lock status.
3. Confirm the profile is active and attached to the expected organization.
4. Use the reset utility only when normal recovery is unavailable.

### Vocabulary unavailable

1. Confirm the vocabulary schema/migrations exist.
2. Confirm `vocabulary.lexemes` contains active rows for the student's CEFR level.
3. Use `seed-vocab` only for local QA; import a verified corpus for production.

### Listening/review media unavailable

1. Confirm `/data` (production) or local storage directories are writable.
2. Confirm storage keys in PostgreSQL match actual files.
3. Confirm playback/signing secrets are stable across backend restarts/deploys.
4. Check volume capacity and persistence.

## Release validation

Use `QA_REPORT.md`, `API_CONTRACT_REPORT.md` and `INTERNAL_API_CONTRACT_REPORT.md` as release artifacts. A full dependency build must still run in an environment where dependencies are available before declaring a deployment fully compiled and smoke-tested.
