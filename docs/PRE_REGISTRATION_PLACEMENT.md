# Pre-registration Placement Workflow

This workflow resolves the enrollment-level circular dependency: a candidate does **not** need a student account or a manually selected level before taking the placement test.

## Center flow

1. Open **Center → Placement** (`http://localhost:3002/placement`).
2. Enter the candidate's full name. Email and phone are optional at this stage.
3. Choose one mode:
   - **Digital / QR invitation** — the center creates a one-time QR/link and the candidate answers 40 questions on their own phone. The center device never needs to display the test questions.
   - **Paper / Word** — download the printer-ready Word test and give it to the candidate.
4. In digital mode, the candidate scans the QR code and completes the test on their phone; answers are saved server-side. In paper mode, center staff enter only the A/B/C/D answers from the printed answer sheet.
5. The assessment module calculates the score and CEFR level from the same English question bank.
6. Only after the result is available, create the student account. The placement level is sent as the account's initial `current_level` and the assessment service validates that it matches before linking the registration.
7. The student then signs in through the separate student portal.

## Security model

- Center management endpoints are center-only and pass through the role-aware gateway. The candidate has a deliberately narrow public placement API protected by a one-time invitation token, a short-lived candidate session, origin restrictions and an IP rate limiter.
- Student credentials are not created until the placement is completed.
- The student portal remains a separate login-only Next.js application.
- The placement record is scoped to the center organization.
- Raw invitation/session tokens are never stored in PostgreSQL; only SHA-256 hashes are stored.
- The QR link carries the invitation in a URL fragment (`#token=...`), which is not sent to the frontend server or Referer header.
- Claiming an invitation invalidates it so a second device cannot reuse the same QR/link.
- Privileged center mutations continue to use the platform's existing AAL2/TOTP policy.

## Paper test files

- Printable Word file: `data/placement/placement-test-paper.docx`
- Deterministic question manifest: `data/placement/paper-v1.json`

The manifest keeps the printed questions and the answer-entry screen aligned. Paper-mode answers use the natural A/B/C/D option order.

To regenerate the manifest after intentionally changing the English bank:

```bash
cd backend
go run ./cmd/placement-paper-manifest \
  -bank ../data/english-bank \
  -out ../data/placement/paper-v1.json
```

After regenerating the manifest, regenerate and visually verify the Word test before deployment so its question order matches the manifest.

## API routes

```text
GET  /api/center/assessment/pre-registration/placements
POST /api/center/assessment/pre-registration/placements
GET  /api/center/assessment/pre-registration/placements/{id}
POST /api/center/assessment/pre-registration/placements/{id}/invitation
POST /api/center/assessment/pre-registration/placements/{id}/finish
POST /api/center/assessment/pre-registration/placements/{id}/registered
GET  /api/center/assessment/pre-registration/placement-paper
```


Candidate public routes and detailed security notes are documented in `docs/PLACEMENT_INVITATION_SECURITY.md`.
