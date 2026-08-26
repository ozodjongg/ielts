# Security Model

- Authentication is owned by the Go backend and PostgreSQL.
- Passwords are never stored in plaintext.
- PBKDF2-HMAC-SHA256 uses a random per-user salt and 600,000 iterations.
- Access JWTs use a dedicated `AUTH_JWT_SECRET`; this secret must never be shared with frontend deployments.
- Access tokens expire quickly (15 minutes by default).
- Refresh tokens are opaque random values and are rotated after each refresh.
- PostgreSQL stores only refresh-token hashes.
- Session revocation is checked on every API request.
- Suspending/archiving a user revokes active sessions.
- Password changes and administrative password resets revoke every active session for that account.
- Admin, Center and Student portals have separate exact CORS origin allowlists.
- Login endpoints are portal-role constrained.
- Five bad passwords lock an account for 15 minutes.
- Authentication requests have a separate per-IP rate limiter.
- Business modules trust only HMAC-signed internal actor/service headers.
- Frontends receive no database password or backend signing secret.

`REQUIRE_ADMIN_AAL2` and `REQUIRE_CENTER_AAL2` must remain `false` until a self-hosted MFA/TOTP flow is enabled. The current local-auth release issues AAL1 sessions.
