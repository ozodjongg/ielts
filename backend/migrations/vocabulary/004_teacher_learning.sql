CREATE TABLE IF NOT EXISTS teacher_contributions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL,
  teacher_user_id uuid NOT NULL,
  lexeme_index bigint NOT NULL,
  english text NOT NULL,
  normalized_english text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(organization_id,lexeme_index)
);
CREATE INDEX IF NOT EXISTS teacher_contributions_org_created_idx ON teacher_contributions(organization_id,created_at DESC);
CREATE INDEX IF NOT EXISTS teacher_contributions_teacher_idx ON teacher_contributions(teacher_user_id,created_at DESC);

CREATE TABLE IF NOT EXISTS teacher_homework (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL,
  teacher_user_id uuid NOT NULL,
  title text NOT NULL,
  instructions text NOT NULL DEFAULT '',
  due_at timestamptz,
  status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS teacher_homework_org_created_idx ON teacher_homework(organization_id,created_at DESC);

CREATE TABLE IF NOT EXISTS teacher_homework_words (
  homework_id uuid NOT NULL REFERENCES teacher_homework(id) ON DELETE CASCADE,
  lexeme_index bigint NOT NULL,
  position integer NOT NULL DEFAULT 0,
  PRIMARY KEY(homework_id,lexeme_index)
);

CREATE TABLE IF NOT EXISTS teacher_homework_students (
  homework_id uuid NOT NULL REFERENCES teacher_homework(id) ON DELETE CASCADE,
  organization_id uuid NOT NULL,
  student_user_id uuid NOT NULL,
  assigned_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  PRIMARY KEY(homework_id,student_user_id)
);
CREATE INDEX IF NOT EXISTS teacher_homework_students_student_idx ON teacher_homework_students(student_user_id,assigned_at DESC);

CREATE TABLE IF NOT EXISTS student_extra_words (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL,
  student_user_id uuid NOT NULL,
  lexeme_index bigint NOT NULL,
  assigned_by_teacher_user_id uuid NOT NULL,
  homework_id uuid REFERENCES teacher_homework(id) ON DELETE SET NULL,
  note text NOT NULL DEFAULT '',
  due_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(student_user_id,lexeme_index,assigned_by_teacher_user_id)
);
CREATE INDEX IF NOT EXISTS student_extra_words_student_idx ON student_extra_words(student_user_id,created_at DESC);
