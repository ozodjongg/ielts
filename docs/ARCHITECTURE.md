# Architecture

```text
Admin / Center / Teacher / Student (Vercel)
                     |
                     | HTTPS + Bearer JWT
                     v
             Go gateway (Railway)
                     |
   JWT + server session + role + AAL + CORS
                     |
       HMAC-signed internal actor context
                     v
  identity | tenant | assessment | vocabulary
  listening | review | sat | points | analytics
                     |
                     v
          one Railway PostgreSQL database
          with schema-separated ownership
```

There is one backend process and one HTTP port. Module-to-module calls remain in-process through local handlers; there are no internal microservice ports.

## Role model

- `admin`: platform scope, no center membership.
- `center`: one organization, center administration.
- `teacher`: one organization, teaching/vocabulary scope.
- `student`: one organization, learner scope.

## Identity trust chain

Gateway validates access token, server session, profile status, role and organization. It then signs an internal actor containing user ID, role, organization ID, email, AAL and session ID. Business modules reject unsigned/spoofed browser actor headers.

## Data ownership

Nine logical schemas remain: `identity`, `tenant`, `assessment`, `vocabulary`, `listening`, `review`, `sat`, `points`, `analytics`. This keeps code/data boundaries understandable without deployment complexity from nine services.
