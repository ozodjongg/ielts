CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS profiles (
  user_id uuid PRIMARY KEY,
  organization_id uuid,
  role text NOT NULL CHECK (role IN ('admin','center','teacher','student')),
  email text NOT NULL,
  full_name text NOT NULL,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','archived')),
  current_level text CHECK (current_level IS NULL OR current_level IN ('A1','A2','B1','B2','C1','C2')),
  locale text NOT NULL DEFAULT 'uz',
  auth_version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS profiles_email_lower_uq ON profiles(lower(email));
CREATE INDEX IF NOT EXISTS profiles_org_role_idx ON profiles(organization_id, role, status);
CREATE TABLE IF NOT EXISTS audit_log (
  id bigserial PRIMARY KEY, actor_user_id uuid, organization_id uuid, action text NOT NULL,
  target_type text, target_id text, metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS identity_audit_org_time_idx ON audit_log(organization_id, created_at DESC);
