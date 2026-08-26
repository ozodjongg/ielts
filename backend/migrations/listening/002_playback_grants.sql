CREATE TABLE IF NOT EXISTS playback_grants (
  id uuid PRIMARY KEY,
  attempt_id uuid NOT NULL REFERENCES listening_attempts(id) ON DELETE CASCADE,
  audio_id uuid NOT NULL REFERENCES audio_assets(id) ON DELETE CASCADE,
  student_user_id uuid NOT NULL,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS playback_grants_attempt_idx ON playback_grants(attempt_id,expires_at);
