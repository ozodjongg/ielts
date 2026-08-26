# Backend Modules

V5 no longer runs nine backend services. These are **logical modules inside one Go backend process**.

| Module | Responsibility | PostgreSQL schema |
|---|---|---|
| Identity | profiles, roles, levels, identity audit | `identity` |
| Tenant | centers, groups, quotas, usage reservations | `tenant` |
| Assessment | English assignments, attempts, answers, mastery | `assessment` |
| Vocabulary | lexemes, synonyms, SRS, daily sessions | `vocabulary` |
| Listening | audio metadata, sets, attempts, playback security | `listening` |
| Review | speaking/writing submissions and human review | `review` |
| SAT | SAT assignments, attempts, answers, mastery | `sat` |
| Points | Rush multipliers and reward ledger | `points` |
| Analytics | cross-module analytics events and aggregates | `analytics` |

The public API remains compatible with the previous gateway URL shape:

```text
/api/admin/{module}/...
/api/center/{module}/...
/api/student/{module}/...
```

No module exposes its own port.
