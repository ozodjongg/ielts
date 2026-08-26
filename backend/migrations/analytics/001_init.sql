CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS events (
 id bigserial PRIMARY KEY, event_id uuid NOT NULL UNIQUE, organization_id uuid, student_user_id uuid,
 event_type text NOT NULL, service_code text, occurred_at timestamptz NOT NULL, payload jsonb NOT NULL DEFAULT '{}'::jsonb,
 received_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS events_org_time_idx ON events(organization_id,occurred_at DESC);
CREATE INDEX IF NOT EXISTS events_student_time_idx ON events(student_user_id,occurred_at DESC);
CREATE TABLE IF NOT EXISTS daily_center_metrics (
 organization_id uuid NOT NULL, day date NOT NULL, service_code text NOT NULL, attempts integer NOT NULL DEFAULT 0,
 completions integer NOT NULL DEFAULT 0, avg_score numeric(8,2), points_awarded numeric(12,2) NOT NULL DEFAULT 0,
 PRIMARY KEY(organization_id,day,service_code)
);
