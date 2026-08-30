# Backend Modules

IELTS uses one Go backend process with nine logical modules.

| Module | Responsibility | Schema |
|---|---|---|
| Identity | four roles, credentials, sessions, TOTP/AAL2, audit | `identity` |
| Tenant | centers, teachers/students, groups, quotas | `tenant` |
| Assessment | English assignments/attempts/mastery | `assessment` |
| Vocabulary | dictionary, SRS, teacher words/homework | `vocabulary` |
| Listening | audio, sets, assignments, secure playback | `listening` |
| Review | speaking/writing submissions and review | `review` |
| SAT | SAT assignments/attempts/mastery | `sat` |
| Points | reward/multiplier ledger | `points` |
| Analytics | cross-module analytics | `analytics` |

Public gateway prefixes:

```text
/api/admin/{module}/...
/api/center/{module}/...
/api/teacher/{module}/...
/api/student/{module}/...
```


## Teacher-targeted services

The gateway allows teachers to use `assessment`, `sat`, and `listening` in addition to tenant/vocabulary reads. Assignment creation passes `actor_role` and `actor_user_id` into tenant target validation. A teacher may target only a group linked in `group_teachers` or a student who is a member of at least one such active group. The `all` target remains center-only.

The obsolete `vocabulary_test` assessment service is removed from runtime catalogs/limits by tenant migration `004_group_teachers_and_service_cleanup.sql`; the shared dictionary/vocabulary learning features remain unchanged.
