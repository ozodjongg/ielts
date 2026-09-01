CREATE TABLE IF NOT EXISTS pre_registration_placements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL,
    created_by uuid NOT NULL,
    full_name text NOT NULL,
    contact_email text,
    contact_phone text,
    mode text NOT NULL CHECK (mode IN ('digital','paper')),
    bank_version text NOT NULL,
    question_plan jsonb NOT NULL DEFAULT '[]'::jsonb,
    answers jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress','completed','registered','expired')),
    score numeric(8,2),
    level_result text,
    registered_user_id uuid,
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    registered_at timestamptz
);

CREATE INDEX IF NOT EXISTS pre_registration_placements_org_idx
    ON pre_registration_placements(organization_id, started_at DESC);

CREATE INDEX IF NOT EXISTS pre_registration_placements_status_idx
    ON pre_registration_placements(organization_id, status, started_at DESC);
