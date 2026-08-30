-- IELTS Platform roles + first-party TOTP MFA/AAL2.
ALTER TABLE profiles DROP CONSTRAINT IF EXISTS profiles_role_check;
UPDATE profiles SET role='admin' WHERE role='platform_admin';
UPDATE profiles SET role='center' WHERE role='center_admin';
ALTER TABLE profiles ADD CONSTRAINT profiles_role_check CHECK (role IN ('admin','center','teacher','student'));

CREATE TABLE IF NOT EXISTS mfa_totp (
  user_id uuid PRIMARY KEY REFERENCES profiles(user_id) ON DELETE CASCADE,
  secret_ciphertext bytea NOT NULL,
  enabled boolean NOT NULL DEFAULT false,
  verified_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mfa_recovery_codes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  code_hash bytea NOT NULL,
  used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mfa_recovery_codes_user_idx ON mfa_recovery_codes(user_id) WHERE used_at IS NULL;

CREATE TABLE IF NOT EXISTS mfa_challenges (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
  expected_role text NOT NULL CHECK (expected_role IN ('admin','center','teacher','student')),
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mfa_challenges_user_idx ON mfa_challenges(user_id, expires_at DESC) WHERE consumed_at IS NULL;
