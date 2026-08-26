CREATE TABLE IF NOT EXISTS manual_prompts (
  attempt_id uuid NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
  prompt_id text NOT NULL,
  component text NOT NULL CHECK(component IN ('speaking','writing')),
  position integer NOT NULL CHECK(position>0),
  prompt_text text NOT NULL,
  required boolean NOT NULL DEFAULT true,
  PRIMARY KEY(attempt_id,prompt_id),
  UNIQUE(attempt_id,component,position)
);

CREATE TABLE IF NOT EXISTS manual_submission_refs (
  attempt_id uuid NOT NULL,
  prompt_id text NOT NULL,
  submission_id uuid NOT NULL UNIQUE,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(attempt_id,prompt_id),
  FOREIGN KEY(attempt_id,prompt_id) REFERENCES manual_prompts(attempt_id,prompt_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS manual_submission_refs_attempt_idx ON manual_submission_refs(attempt_id);
