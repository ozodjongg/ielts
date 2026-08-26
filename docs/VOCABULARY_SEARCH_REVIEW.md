# Search-triggered vocabulary review

Student dictionary searches now feed a spaced-repetition queue.

## Flow

1. An exact dictionary search automatically enrolls the matched lexeme.
2. Opening a search result also records that lexeme as seen.
3. A newly discovered word is first due after 90 minutes.
4. `/api/student/vocabulary/review/due` returns due words.
5. Ratings `again`, `hard`, `good`, `easy` adapt the next interval.
6. Repeated searches shorten future intervals because they signal difficulty.
7. Points and analytics events are recorded after a review grade.

The existing daily vocabulary flow remains supported and shares the same `student_word_state` table.

## Initial schedule

- Again: 10 minutes
- Hard: 60 minutes
- Good: 1 day
- Easy: 3 days

Later reviews grow adaptively and are capped at 180 days.
