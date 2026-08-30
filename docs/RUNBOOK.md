# Operations Runbook

## Startup

Container startup performs migrations, idempotent vocabulary import/version validation, then starts the single backend. `/ready` checks module/database readiness.

## Authentication incidents

Inspect:

```text
identity.auth_login_audit
identity.auth_credentials
identity.auth_sessions
identity.mfa_totp
identity.mfa_challenges
```

Password recovery utility in the Railway image:

```bash
/app/reset-password --email "user@example.com" --password "New-Strong-Password"
```

Password resets revoke active sessions. If a user loses both authenticator and recovery codes, reset/administrative recovery should be handled through an authorized operational process; never edit `aal` directly in the database.

## CORS

Verify each Vercel domain exactly matches `ADMIN_ORIGINS`, `CENTER_ORIGINS`, `TEACHER_ORIGINS`, or `STUDENT_ORIGINS` as appropriate.

## Vocabulary

Teacher-created words and assignments are in `vocabulary.teacher_contributions`, `teacher_homework`, `teacher_homework_students`, and `student_extra_words`. Assigned words also enter `student_word_state`.

## Release validation

Run:

```bash
python3 tools/qa_ielts.py
```

A real dependency-connected environment should additionally run `go test ./...` and `pnpm typecheck && pnpm lint && pnpm build` in all four frontend apps.
