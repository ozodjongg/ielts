CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS sat_assignments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, title text NOT NULL,
 target_type text NOT NULL CHECK(target_type IN ('student','group','all')), target_id uuid, question_count integer NOT NULL DEFAULT 44 CHECK(question_count BETWEEN 10 AND 80),
 due_at timestamptz, created_by uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS sat_attempts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, assignment_id uuid NOT NULL REFERENCES sat_assignments(id), student_user_id uuid NOT NULL,
 bank_version text NOT NULL, question_plan jsonb NOT NULL, status text NOT NULL DEFAULT 'in_progress', raw_correct integer NOT NULL DEFAULT 0,
 percent numeric(6,2), estimated_sat_score integer, started_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz, UNIQUE(assignment_id,student_user_id)
);
CREATE TABLE IF NOT EXISTS sat_answers (
 id bigserial PRIMARY KEY, attempt_id uuid NOT NULL REFERENCES sat_attempts(id) ON DELETE CASCADE, question_id uuid NOT NULL,
 topic_code text NOT NULL, selected_option text, is_correct boolean, base_points numeric(6,2) NOT NULL,
 rush_multiplier numeric(6,3) NOT NULL DEFAULT 1, response_ms integer, answered_at timestamptz, UNIQUE(attempt_id,question_id)
);
CREATE TABLE IF NOT EXISTS sat_topic_mastery (
 organization_id uuid NOT NULL, student_user_id uuid NOT NULL, topic_code text NOT NULL, attempts integer NOT NULL DEFAULT 0,
 correct integer NOT NULL DEFAULT 0, mastery numeric(6,4) NOT NULL DEFAULT 0, updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(organization_id,student_user_id,topic_code)
);
