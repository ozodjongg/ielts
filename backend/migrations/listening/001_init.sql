CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS audio_assets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, title text NOT NULL,
 storage_key text NOT NULL UNIQUE, sha256 text NOT NULL, mime_type text NOT NULL,
 size_bytes bigint NOT NULL CHECK(size_bytes>0), duration_ms integer, level text,
 max_plays integer NOT NULL DEFAULT 2 CHECK(max_plays BETWEEN 1 AND 10), allow_seek boolean NOT NULL DEFAULT false,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')), created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS listening_sets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, audio_id uuid NOT NULL REFERENCES audio_assets(id),
 title text NOT NULL, level text, questions jsonb NOT NULL, answer_key jsonb NOT NULL, created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS listening_assignments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, set_id uuid NOT NULL REFERENCES listening_sets(id),
 target_type text NOT NULL CHECK(target_type IN ('student','group','all')), target_id uuid, due_at timestamptz, created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS listening_attempts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, assignment_id uuid NOT NULL REFERENCES listening_assignments(id),
 student_user_id uuid NOT NULL, play_count integer NOT NULL DEFAULT 0, status text NOT NULL DEFAULT 'in_progress', score numeric(6,2), answers jsonb NOT NULL DEFAULT '{}'::jsonb,
 started_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz, UNIQUE(assignment_id,student_user_id)
);
CREATE TABLE IF NOT EXISTS playback_events (
 id bigserial PRIMARY KEY, attempt_id uuid NOT NULL, audio_id uuid NOT NULL, event_type text NOT NULL, position_ms integer,
 ip_hash text, created_at timestamptz NOT NULL DEFAULT now()
);
