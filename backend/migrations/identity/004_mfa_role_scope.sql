-- MFA is available only to privileged operational roles. Student accounts do
-- not participate in TOTP/AAL2 and any legacy student MFA state is removed.
DELETE FROM mfa_challenges c
USING profiles p
WHERE p.user_id=c.user_id AND p.role='student';
DELETE FROM mfa_recovery_codes r
USING profiles p
WHERE p.user_id=r.user_id AND p.role='student';
DELETE FROM mfa_totp m
USING profiles p
WHERE p.user_id=m.user_id AND p.role='student';

ALTER TABLE mfa_challenges DROP CONSTRAINT IF EXISTS mfa_challenges_expected_role_check;
ALTER TABLE mfa_challenges ADD CONSTRAINT mfa_challenges_expected_role_check
  CHECK (expected_role IN ('admin','center','teacher'));
