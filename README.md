# IELTS Platform

Production-oriented IELTS learning-center platform with four role-specific web portals and one modular Go backend.

## Production topology

```text
Admin Web   ─ Vercel ─┐
Center Web  ─ Vercel ─┤
Teacher Web ─ Vercel ─┼── HTTPS ──> Go modular monolith ─ Railway
Student Web ─ Vercel ─┘                         │
                                                 ├─ PostgreSQL ─ Railway
                                                 └─ private media volume /data
```

The backend is **one process and one public API**. Business boundaries remain separated as Go modules and PostgreSQL schemas; there are no nine service processes or internal service ports.

## Roles

| Role | Main responsibilities |
|---|---|
| `admin` | platform/center administration, service limits, global analytics |
| `center` | center users, teachers, students, groups, assessments, listening, reviews |
| `teacher` | own groups/students, vocabulary contribution/homework, English/SAT/Listening service assignment |
| `student` | learning, assessments, SAT, vocabulary/SRS, teacher assignments, submissions, progress |

Exact portal role matching is enforced by the gateway. A user cannot use another role's portal with the same token.

## Authentication and MFA

Authentication is fully self-hosted in Go + PostgreSQL; there is no Supabase dependency.

- PBKDF2-HMAC-SHA256 password hashes with per-user random salt and 600,000 iterations.
- 15-minute access JWTs by default.
- Opaque refresh tokens stored only as hashes and rotated on refresh.
- Server-side sessions enable immediate revocation.
- Five failed password attempts lock the account for 15 minutes.
- First-party RFC-compatible TOTP setup/verify/login challenge.
- One-time recovery codes are stored only as hashes.
- Admin, center and teacher mutations require AAL2 by default.
- TOTP/MFA is available only to `admin`, `center`, and `teacher`; students use password/session authentication without MFA.

## Teacher learning workflow

Vocabulary mutation is teacher-only. Teachers can:

1. see only groups assigned to them by the center;
2. see only students who belong to those groups;
3. search/contribute shared vocabulary and add extra words for those students;
4. create vocabulary homework only for students in their assigned groups;
5. assign English, SAT Math and existing Listening services to their own groups/students;
6. see their own assignment/homework activity.

Assigned words are inserted into the student's spaced-review state immediately. Students see them in **Assigned** and can mark homework complete.

## Frontend

There are four independent Next.js 16 applications:

```text
apps/admin-web
apps/center-web
apps/teacher-web
apps/student-web
```

Each app has:

- IELTS branding;
- responsive desktop/tablet/mobile layouts;
- mobile bottom navigation and touch-friendly controls;
- 12 themes: Pearl, Midnight, Ocean, Emerald, Violet, Rose, Amber, Sky, Indigo, Mint, Slate and Sunset;
- TOTP/AAL2 Security screen for admin, center and teacher portals only;
- private-portal metadata and `noindex` policy.

Private authenticated dashboards intentionally do not target public search indexing. This is safer than exposing administrative/login surfaces to search engines; metadata, PWA manifests and responsive viewport configuration are still complete.

## Backend modules

`identity`, `tenant`, `assessment`, `vocabulary`, `listening`, `review`, `sat`, `points`, and `analytics` run in-process inside one Go server. Their PostgreSQL data remains schema-separated.

## Local Docker start

```bash
cp .env.example .env
# replace all CHANGE_ME values
docker compose up -d --build
```

Local URLs:

```text
Admin:   http://localhost:3001
Center:  http://localhost:3002
Student: http://localhost:3003
Teacher: http://localhost:3004
Backend: http://localhost:8080
Ready:   http://localhost:8080/ready
```

Create the first admin after backend readiness:

```bash
docker compose --profile tools run --rm bootstrap-admin \
  --email "admin@example.com" \
  --password "Use-A-Unique-Strong-Password" \
  --name "Platform Administrator"
```

The first privileged login is AAL1 until TOTP is enrolled. Admin/center/teacher open **Security**, scan the local QR code with an authenticator and verify a code; the session is upgraded to AAL2. Student accounts never enter this MFA flow.

## Deployment

See [`START_HERE.md`](START_HERE.md) and [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md).
