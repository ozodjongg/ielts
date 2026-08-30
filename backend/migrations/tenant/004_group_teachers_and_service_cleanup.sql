-- Real many-to-many teacher <-> group ownership and removal of the obsolete
-- vocabulary assessment service. This migration intentionally follows 001-003
-- so existing production migration checksums remain unchanged.
CREATE TABLE IF NOT EXISTS group_teachers (
  group_id uuid NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  organization_id uuid NOT NULL,
  teacher_user_id uuid NOT NULL,
  assigned_by_user_id uuid,
  assigned_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(group_id, teacher_user_id)
);
CREATE INDEX IF NOT EXISTS group_teachers_teacher_idx
  ON group_teachers(organization_id, teacher_user_id, group_id);

-- vocabulary_test was an assessment service, not the shared dictionary. It is
-- intentionally removed from the platform service catalog.
DELETE FROM usage_reservations WHERE service_code='vocabulary_test';
DELETE FROM usage_daily WHERE service_code='vocabulary_test';
DELETE FROM usage_monthly WHERE service_code='vocabulary_test';
DELETE FROM organization_service_limits WHERE service_code='vocabulary_test';
DELETE FROM service_catalog WHERE code='vocabulary_test';
