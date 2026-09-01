ALTER TABLE pre_registration_placements
    ADD COLUMN IF NOT EXISTS invitation_token_hash text,
    ADD COLUMN IF NOT EXISTS invitation_expires_at timestamptz,
    ADD COLUMN IF NOT EXISTS invitation_claimed_at timestamptz,
    ADD COLUMN IF NOT EXISTS candidate_session_hash text,
    ADD COLUMN IF NOT EXISTS candidate_session_expires_at timestamptz,
    ADD COLUMN IF NOT EXISTS candidate_last_seen_at timestamptz;

CREATE UNIQUE INDEX IF NOT EXISTS pre_registration_placements_invitation_token_hash_uidx
    ON pre_registration_placements(invitation_token_hash)
    WHERE invitation_token_hash IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS pre_registration_placements_candidate_session_hash_uidx
    ON pre_registration_placements(candidate_session_hash)
    WHERE candidate_session_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS pre_registration_placements_invitation_expiry_idx
    ON pre_registration_placements(invitation_expires_at)
    WHERE mode = 'digital' AND status = 'in_progress';
