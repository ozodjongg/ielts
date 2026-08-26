ALTER TABLE submissions ADD COLUMN IF NOT EXISTS audio_mime_type text;
CREATE UNIQUE INDEX IF NOT EXISTS submissions_attempt_prompt_uidx ON submissions(attempt_id,prompt_id) WHERE attempt_id IS NOT NULL;
