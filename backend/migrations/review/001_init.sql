CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS submissions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, student_user_id uuid NOT NULL,
 attempt_id uuid, service_code text NOT NULL CHECK(service_code IN ('speaking','writing','mock')),
 prompt_id text NOT NULL, text_submission text, audio_storage_key text, audio_sha256 text,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','reviewed','returned')),
 rubric jsonb, reviewer_user_id uuid, review_notes text, score numeric(6,2), submitted_at timestamptz NOT NULL DEFAULT now(), reviewed_at timestamptz
);
CREATE INDEX IF NOT EXISTS review_queue_idx ON submissions(organization_id,status,submitted_at);
