CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS assignments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, service_code text NOT NULL,
 title text NOT NULL, target_type text NOT NULL CHECK(target_type IN ('student','group','all')),
 target_id uuid, from_level text, to_level text, question_count integer, opens_at timestamptz NOT NULL DEFAULT now(),
 due_at timestamptz, status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','closed')), created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS assignments_org_idx ON assignments(organization_id,status,created_at DESC);
CREATE TABLE IF NOT EXISTS attempts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, assignment_id uuid REFERENCES assignments(id), student_user_id uuid NOT NULL,
 service_code text NOT NULL, bank_version text NOT NULL, status text NOT NULL DEFAULT 'in_progress' CHECK(status IN ('in_progress','pending_review','completed','expired')),
 from_level text, to_level text, question_plan jsonb NOT NULL DEFAULT '[]'::jsonb,
 auto_score numeric(8,2) NOT NULL DEFAULT 0, final_score numeric(8,2), level_result text, readiness numeric(8,2),
 started_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz, UNIQUE(assignment_id,student_user_id)
);
CREATE TABLE IF NOT EXISTS answers (
 id bigserial PRIMARY KEY, attempt_id uuid NOT NULL REFERENCES attempts(id) ON DELETE CASCADE, question_id uuid NOT NULL,
 subject_code text NOT NULL, displayed_options jsonb NOT NULL, selected_option text, is_correct boolean, base_points numeric(8,2) NOT NULL,
 rush_multiplier numeric(6,3) NOT NULL DEFAULT 1, response_ms integer, answered_at timestamptz, UNIQUE(attempt_id,question_id)
);
CREATE TABLE IF NOT EXISTS topic_mastery (
 organization_id uuid NOT NULL, student_user_id uuid NOT NULL, subject_code text NOT NULL, attempts integer NOT NULL DEFAULT 0,
 correct integer NOT NULL DEFAULT 0, mastery numeric(6,4) NOT NULL DEFAULT 0, updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(organization_id,student_user_id,subject_code)
);
CREATE TABLE IF NOT EXISTS anti_cheat_events (
 id bigserial PRIMARY KEY, attempt_id uuid NOT NULL, organization_id uuid NOT NULL, student_user_id uuid NOT NULL,
 event_type text NOT NULL, metadata jsonb NOT NULL DEFAULT '{}'::jsonb, created_at timestamptz NOT NULL DEFAULT now()
);
