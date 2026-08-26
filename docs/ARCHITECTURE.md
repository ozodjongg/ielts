# Architecture

## Request path

```text
Vercel portal
   |
   | 1. POST /auth/<portal>/login for authentication
   | 2. Bearer access JWT for API requests
   v
Go gateway (same monolith process)
   |
   | verify JWT signature / issuer / audience / expiry
   | verify session id + auth_version in PostgreSQL
   | resolve active profile and exact portal role
   | attach HMAC-signed internal actor identity
   v
In-process module router
   |
   +-- identity
   +-- tenant
   +-- assessment
   +-- vocabulary
   +-- listening
   +-- review
   +-- sat
   +-- points
   +-- analytics
   |
   v
One PostgreSQL database with schema-separated module ownership
```

There are no internal HTTP service ports. Module-to-module calls stay inside one Go process through local handlers while preserving signed internal trust boundaries.

## Authentication data

The `identity` schema owns:

- `profiles`
- `auth_credentials`
- `auth_sessions`
- `auth_login_audit`
- `audit_log`

Access JWTs are stateless cryptographic credentials, but access is also checked against a server-side session row on every gateway request. This permits immediate revocation without waiting for JWT expiration.
