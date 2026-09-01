# Digital Placement Invitation Security

Digital pre-registration placement is intentionally separated from authenticated student accounts. A candidate can take the test on their own phone without receiving center credentials or using a center computer.

## Flow

1. A center admin creates a **Digital / QR invitation** from Center → Placement.
2. The assessment service generates a high-entropy one-time invitation token. Only its SHA-256 hash is stored in PostgreSQL.
3. The center UI builds a QR/link in this form:

   `https://CENTER_HOST/placement/invite#token=...`

   The token is in the URL fragment, so browsers do not send it to the Next.js server or in the HTTP `Referer` header.
4. The candidate scans the QR code on their phone. The public page sends the token once in a HTTPS JSON request to `/public/placement/invitations/claim`.
5. On a successful claim the backend deletes the invitation hash and creates a separate short-lived candidate session token. A second device cannot claim the same invitation.
6. The candidate session is stored on that phone. Each answer is validated and persisted server-side, so refresh/reopen on the same device can resume the test while the session is valid.
7. Finishing the test invalidates the candidate session, computes the CEFR result and exposes the completed result to the center. The student account is still created only by the authenticated center flow.

## Defaults

- Invitation lifetime: `PLACEMENT_INVITATION_TTL_HOURS=24`
- Candidate test session: `PLACEMENT_SESSION_TTL_MINUTES=120`
- Public placement rate limit: `PLACEMENT_PUBLIC_RATE_LIMIT_PER_MINUTE=120`

These values can be changed in deployment environment variables.

## Public API surface

Only the following unauthenticated gateway routes exist, and all are rate-limited and restricted to the configured `CENTER_ORIGINS` browser origin:

```text
POST /public/placement/invitations/claim
GET  /public/placement/session
POST /public/placement/session/answer
POST /public/placement/session/finish
```

`GET/answer/finish` require `X-Placement-Session`. No answer key, correct option or privileged center data is returned through the public API.

## Center API additions

```text
POST /api/center/assessment/pre-registration/placements/{id}/invitation
```

This creates a new invitation when the previous invitation was not claimed or when the previous candidate session has expired. It refuses to replace an active candidate session.
