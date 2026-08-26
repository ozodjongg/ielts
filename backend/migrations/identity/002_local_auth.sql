CREATE TABLE IF NOT EXISTS auth_credentials (
  user_id uuid PRIMARY KEY REFERENCES profiles(user_id) ON DELETE CASCADE,
  password_hash text NOT NULL,
  failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
  locked_until timestamptz,
  password_changed_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auth_sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  refresh_token_hash bytea NOT NULL,
  aal text NOT NULL DEFAULT 'aal1' CHECK (aal IN ('aal1','aal2')),
  user_agent text,
  ip_address inet,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS auth_sessions_refresh_hash_uq ON auth_sessions(refresh_token_hash);
CREATE INDEX IF NOT EXISTS auth_sessions_user_active_idx ON auth_sessions(user_id, expires_at DESC) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS auth_login_audit (
  id bigserial PRIMARY KEY,
  email text NOT NULL,
  expected_role text,
  user_id uuid,
  success boolean NOT NULL,
  ip_address inet,
  user_agent text,
  reason text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS auth_login_audit_email_time_idx ON auth_login_audit(lower(email), created_at DESC);
