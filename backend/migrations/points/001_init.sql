CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS question_stats (
 service_code text NOT NULL, question_id uuid NOT NULL, attempts bigint NOT NULL DEFAULT 0, correct bigint NOT NULL DEFAULT 0,
 smoothed_solve_rate numeric(7,6) NOT NULL DEFAULT 0.65, rush_multiplier numeric(6,3) NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(service_code,question_id)
);
CREATE TABLE IF NOT EXISTS point_ledger (
 id bigserial PRIMARY KEY, organization_id uuid NOT NULL, student_user_id uuid NOT NULL,
 service_code text NOT NULL, question_id uuid, event_key text NOT NULL UNIQUE, base_points numeric(8,2) NOT NULL,
 multiplier numeric(6,3) NOT NULL, awarded_points numeric(10,2) NOT NULL, reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS point_ledger_student_idx ON point_ledger(organization_id,student_user_id,created_at DESC);
