# Deployment — Railway backend/database + four Vercel portals

## Railway

Create:

1. PostgreSQL service.
2. Backend service from repository root using `/Dockerfile`.
3. Persistent volume mounted at `/data` for listening/review media.

Use `deploy/railway/backend.json` as Railway config and copy variables from `deploy/railway-backend.env.example`.

Required independent secrets:

```text
AUTH_JWT_SECRET
INTERNAL_SIGNING_SECRET
PLAYBACK_SIGNING_SECRET
QUESTION_SHUFFLE_SECRET
MFA_ENCRYPTION_KEY
```

Required origins after Vercel domains exist:

```text
ADMIN_ORIGINS=https://...
CENTER_ORIGINS=https://...
TEACHER_ORIGINS=https://...
STUDENT_ORIGINS=https://...
```

The container entrypoint applies checksum-protected migrations, performs idempotent vocabulary import/version checks, then starts the one Go backend. Verify `/health` and `/ready`.

## Vercel

Create four Vercel projects from the same repository:

```text
Admin:   apps/admin-web
Center:  apps/center-web
Teacher: apps/teacher-web
Student: apps/student-web
```

Each requires:

```env
NEXT_PUBLIC_API_URL=https://YOUR-RAILWAY-BACKEND
```

No Supabase keys or database credentials belong in Vercel.

## Bootstrap admin

From a Railway backend shell:

```bash
/app/bootstrap-admin --email "admin@example.com" --password "Strong-Unique-Password" --name "Platform Administrator"
```

Log in, go to Security and enroll TOTP before performing privileged write actions.

## Smoke test

1. `/ready` -> 200.
2. Admin login -> TOTP setup -> AAL2.
3. Admin creates center.
4. Center login -> TOTP -> create teacher and student.
5. Teacher login -> TOTP -> add a new vocabulary entry.
6. Teacher assigns extra word to one student.
7. Teacher creates vocabulary homework for multiple students.
8. Student sees Assigned items, SRS queue, and marks homework complete.
9. Test English assessment, listening, SAT, review, points and analytics.
10. Redeploy backend and verify migration/data version idempotency and media volume persistence.
