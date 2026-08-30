# Teacher Vocabulary & Homework

Vocabulary mutation moved from Center to the dedicated Teacher role.

Teacher portal routes:

```text
/vocabulary-manager
/students
/homework
```

Backend-enforced teacher endpoints:

```text
POST /v1/teacher/words/check
POST /v1/teacher/words
POST /v1/teacher/words/batch
GET  /v1/teacher/contributions
POST /v1/teacher/students/{studentID}/words
GET  /v1/teacher/students/{studentID}/words
POST /v1/teacher/homework
GET  /v1/teacher/homework
```

Student endpoints:

```text
GET  /v1/assigned
POST /v1/assigned/homework/{id}/complete
```

Center users can manage teachers and students but cannot call vocabulary mutation handlers. Teacher assignments are organization-scoped and immediately enroll assigned words into the student's spaced-review state.
