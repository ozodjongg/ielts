CREATE TABLE IF NOT EXISTS center_contributions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL,
  user_id uuid NOT NULL,
  lexeme_index bigint NOT NULL REFERENCES lexemes(lemma_index) ON DELETE CASCADE,
  english text NOT NULL,
  normalized_english text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (organization_id, lexeme_index)
);

CREATE INDEX IF NOT EXISTS center_contributions_org_created_idx
  ON center_contributions(organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS center_contributions_normalized_idx
  ON center_contributions(normalized_english);
