-- Search-triggered vocabulary spaced repetition.
-- Extends the existing student_word_state table without breaking daily vocabulary.

ALTER TABLE student_word_state
  ADD COLUMN IF NOT EXISTS first_seen_at timestamptz,
  ADD COLUMN IF NOT EXISTS last_seen_at timestamptz,
  ADD COLUMN IF NOT EXISTS search_count integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS review_count integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS correct_count integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS incorrect_count integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS interval_minutes integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_review_at timestamptz,
  ADD COLUMN IF NOT EXISTS last_review_at timestamptz,
  ADD COLUMN IF NOT EXISTS discovery_source text,
  ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'learning';

UPDATE student_word_state
SET next_review_at = due_at::timestamptz
WHERE next_review_at IS NULL;

UPDATE student_word_state
SET interval_minutes = GREATEST(interval_days, 0) * 1440
WHERE interval_minutes = 0 AND interval_days > 0;

ALTER TABLE student_word_state
  ALTER COLUMN next_review_at SET DEFAULT (now() + interval '90 minutes'),
  ALTER COLUMN next_review_at SET NOT NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'student_word_state_search_count_check'
      AND conrelid = 'student_word_state'::regclass
  ) THEN
    ALTER TABLE student_word_state
      ADD CONSTRAINT student_word_state_search_count_check CHECK (search_count >= 0);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'student_word_state_review_count_check'
      AND conrelid = 'student_word_state'::regclass
  ) THEN
    ALTER TABLE student_word_state
      ADD CONSTRAINT student_word_state_review_count_check CHECK (review_count >= 0);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'student_word_state_status_check'
      AND conrelid = 'student_word_state'::regclass
  ) THEN
    ALTER TABLE student_word_state
      ADD CONSTRAINT student_word_state_status_check
      CHECK (status IN ('learning','mastered','suspended'));
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS word_next_review_idx
  ON student_word_state(student_user_id, next_review_at)
  WHERE status <> 'suspended';

CREATE INDEX IF NOT EXISTS word_last_seen_idx
  ON student_word_state(student_user_id, last_seen_at DESC)
  WHERE last_seen_at IS NOT NULL;
