CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE TABLE IF NOT EXISTS lexemes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), lemma_index bigserial UNIQUE, english text NOT NULL,
 normalized_english text NOT NULL, uzbek jsonb NOT NULL, part_of_speech text,
 cefr text NOT NULL CHECK(cefr IN ('A1','A2','B1','B2','C1','C2')), level_source text NOT NULL DEFAULT 'source',
 frequency_rank integer, synonym_group_id bigint, source_name text NOT NULL, source_license text NOT NULL,
 source_ref text, active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS lexeme_source_unique ON lexemes(normalized_english,source_name,coalesce(part_of_speech,''));
CREATE INDEX IF NOT EXISTS lexeme_level_rank_idx ON lexemes(cefr,frequency_rank NULLS LAST,lemma_index);
CREATE INDEX IF NOT EXISTS lexeme_synonym_idx ON lexemes(synonym_group_id) WHERE synonym_group_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS lexeme_trgm_idx ON lexemes USING gin(normalized_english gin_trgm_ops);
CREATE TABLE IF NOT EXISTS synonym_edges (
 lexeme_index bigint NOT NULL, synonym_lexeme_index bigint NOT NULL, weight numeric(5,4) NOT NULL DEFAULT 1,
 source text NOT NULL, PRIMARY KEY(lexeme_index,synonym_lexeme_index)
);
CREATE INDEX IF NOT EXISTS synonym_reverse_idx ON synonym_edges(synonym_lexeme_index);
CREATE TABLE IF NOT EXISTS student_word_state (
 organization_id uuid NOT NULL, student_user_id uuid NOT NULL, lexeme_index bigint NOT NULL,
 repetitions integer NOT NULL DEFAULT 0, interval_days integer NOT NULL DEFAULT 0, ease numeric(5,2) NOT NULL DEFAULT 2.5,
 mastery numeric(5,4) NOT NULL DEFAULT 0, due_at date NOT NULL DEFAULT current_date, last_grade integer,
 updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(student_user_id,lexeme_index)
);
CREATE INDEX IF NOT EXISTS word_due_idx ON student_word_state(student_user_id,due_at);
CREATE TABLE IF NOT EXISTS daily_sessions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, student_user_id uuid NOT NULL,
 level text NOT NULL, day date NOT NULL DEFAULT current_date, new_count integer NOT NULL, review_count integer NOT NULL,
 completed_at timestamptz, UNIQUE(student_user_id,day)
);
CREATE TABLE IF NOT EXISTS daily_items (
 session_id uuid NOT NULL REFERENCES daily_sessions(id) ON DELETE CASCADE, position integer NOT NULL, lexeme_index bigint NOT NULL,
 grade integer, answered_at timestamptz, PRIMARY KEY(session_id,position), UNIQUE(session_id,lexeme_index)
);
