# Security Model

## Authentication

Authentication is owned by the Go backend and PostgreSQL.

- Passwords use PBKDF2-HMAC-SHA256 with random per-user salt and 600,000 iterations.
- Access JWTs are short-lived and signed with `AUTH_JWT_SECRET`.
- Refresh tokens are opaque, rotated after use, and stored in PostgreSQL only as hashes.
- Every API request validates the JWT, active profile, `auth_version`, and non-revoked server-side session.
- Password/status changes revoke sessions immediately.
- Five invalid passwords trigger a 15-minute lock.
- Authentication has a dedicated per-IP rate limiter.

## TOTP / AAL2

First-party TOTP is implemented only for privileged `admin`, `center`, and `teacher` accounts. Student MFA state is removed by migration and student login never creates an MFA challenge.

Flow:

```text
privileged password login -> AAL1
Security setup -> locally rendered QR scan + code verification -> AAL2
future privileged login with MFA enabled -> MFA challenge -> TOTP/recovery code -> AAL2
student password login -> normal student session (no MFA flow)
```

`MFA_ENCRYPTION_KEY` protects TOTP secrets at rest with authenticated encryption. Recovery codes are displayed once and stored only as hashes. MFA challenges expire and limit invalid attempts.

Production defaults:

```env
REQUIRE_ADMIN_AAL2=true
REQUIRE_CENTER_AAL2=true
REQUIRE_TEACHER_AAL2=true
```

MFA setup/verification is intentionally reachable from an authenticated AAL1 session so a newly provisioned privileged user can bootstrap MFA. Existing MFA cannot be silently replaced; it must be explicitly disabled from AAL2 first.

## Authorization

Exact roles are `admin`, `center`, `teacher`, `student`. Gateway role matching and module-level checks are both used.

Notable least-privilege rules:

- only center can provision/manage teacher and student accounts;
- center owns teacher↔group assignment and may attach one or more teachers to each group;
- teacher may read only groups assigned through `tenant.group_teachers` and only students in those groups;
- vocabulary mutations and teacher homework endpoints require `teacher`, and student targets are revalidated against teacher-owned groups server-side;
- teacher English/SAT/Listening assignments are limited to teacher-owned groups or students in those groups; teachers cannot target `all`;
- student assignment endpoints are scoped to the current student and organization;
- internal actor headers include user, role, organization, AAL and session ID and are HMAC signed;
- frontend code receives no database password or server signing/encryption key.

## Web security

Each portal has an exact CORS origin allowlist and production security headers. Authenticated portals are intentionally `noindex`/`nofollow` and robots-disallowed.
