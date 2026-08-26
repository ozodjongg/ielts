#!/usr/bin/env bash
set -Eeuo pipefail

# Assessment Platform V5 - Vocabulary importer
# Windows Git Bash / Linux
#
# Examples:
#   bash vocab_import.sh import data/vocabulary/generated/stage1_import_ready.csv -
#   bash import-vocabulary.sh status
#   bash import-vocabulary.sh sample
#
# Local DB defaults are the same as local.sh:
#   host=127.0.0.1 port=5432 user=postgres db=assessment_v5
#
# Optional:
#   export LOCAL_DB_PASSWORD='your-postgres-password'
#   export LOCAL_DB_HOST=127.0.0.1
#   export LOCAL_DB_PORT=5432
#   export LOCAL_DB_USER=postgres
#   export LOCAL_DB_NAME=assessment_v5
#
# If DATABASE_URL is already a complete postgresql://... URL it is used.
# Invalid values such as DATABASE_URL=127.0.0.1 are ignored.

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
BACKEND="$ROOT/backend"
RUN_DIR="$ROOT/.local-run"
BIN_DIR="$RUN_DIR/bin"
LOG_DIR="$RUN_DIR/logs"

mkdir -p "$BIN_DIR" "$LOG_DIR"

if [[ "${OS:-}" == "Windows_NT" ]]; then
  IMPORTER="$BIN_DIR/vocab-import.exe"
else
  IMPORTER="$BIN_DIR/vocab-import"
fi

DB_HOST="${LOCAL_DB_HOST:-127.0.0.1}"
DB_PORT="${LOCAL_DB_PORT:-5432}"
DB_USER="${LOCAL_DB_USER:-postgres}"
DB_NAME="${LOCAL_DB_NAME:-assessment_v5}"
DB_PASSWORD="${LOCAL_DB_PASSWORD:-}"

PSQL_BIN=""
VOCAB_DSN=""

say()  { printf '\n\033[1;36m%s\033[0m\n' "$*"; }
ok()   { printf '\033[1;32m[OK]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[WARN]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[X]\033[0m %s\n' "$*" >&2; exit 1; }

find_psql() {
  if command -v psql >/dev/null 2>&1; then
    PSQL_BIN="$(command -v psql)"
    return
  fi

  local candidate
  for candidate in \
    "/c/Program Files/PostgreSQL/"*/bin/psql.exe \
    "/c/Program Files (x86)/PostgreSQL/"*/bin/psql.exe
  do
    if [[ -x "$candidate" ]]; then
      PSQL_BIN="$candidate"
    fi
  done

  [[ -n "$PSQL_BIN" ]] || fail \
    "psql topilmadi. PostgreSQL\\bin ni PATH ga qo'shing."
}

valid_database_url() {
  case "${1:-}" in
    postgresql://*|postgres://*) return 0 ;;
    *) return 1 ;;
  esac
}

make_vocab_dsn_from_url() {
  local raw="$1"
  VOCAB_DSN="$(
    node - "$raw" <<'NODE'
const raw = process.argv[2];
try {
  const u = new URL(raw);
  if (u.protocol !== "postgresql:" && u.protocol !== "postgres:") {
    throw new Error("DATABASE_URL postgresql:// yoki postgres:// bilan boshlanishi kerak");
  }
  // libpq/psql does NOT accept ?search_path=...
  // Use the standard `options` connection parameter instead.
  u.searchParams.set("options", "-c search_path=vocabulary,public");
  process.stdout.write(u.toString());
} catch (e) {
  console.error(String(e.message || e));
  process.exit(1);
}
NODE
  )" || fail "DATABASE_URL noto'g'ri formatda."
}

make_local_database_url() {
  if [[ -z "$DB_PASSWORD" ]]; then
    printf "PostgreSQL paroli (%s@%s:%s): " "$DB_USER" "$DB_HOST" "$DB_PORT"
    IFS= read -r -s DB_PASSWORD
    printf '\n'
  fi

  export PGPASSWORD="$DB_PASSWORD"

  say "PostgreSQL ulanishi tekshirilmoqda..."
  if ! PGCONNECT_TIMEOUT=8 \
      "$PSQL_BIN" -X -w \
      -h "$DB_HOST" \
      -p "$DB_PORT" \
      -U "$DB_USER" \
      -d "$DB_NAME" \
      -tAc "SELECT 1" >/dev/null 2>&1; then
    fail "PostgreSQL'ga ulanib bo'lmadi. Host/port/user/parol/database ni tekshiring."
  fi
  ok "PostgreSQL ulanishi ishlayapti."

  DATABASE_URL="$(
    DB_HOST="$DB_HOST" \
    DB_PORT="$DB_PORT" \
    DB_USER="$DB_USER" \
    DB_NAME="$DB_NAME" \
    DB_PASSWORD="$DB_PASSWORD" \
    node <<'NODE'
const u = new URL("postgresql://localhost");
u.hostname = process.env.DB_HOST;
u.port = process.env.DB_PORT;
u.username = process.env.DB_USER;
u.password = process.env.DB_PASSWORD;
u.pathname = "/" + process.env.DB_NAME;
u.searchParams.set("sslmode", "disable");
process.stdout.write(u.toString());
NODE
  )"

  export DATABASE_URL
  make_vocab_dsn_from_url "$DATABASE_URL"
}

prepare_database() {
  command -v node >/dev/null 2>&1 || fail "Node.js topilmadi."
  find_psql

  if [[ -n "${DATABASE_URL:-}" ]]; then
    if valid_database_url "$DATABASE_URL"; then
      ok "To'liq DATABASE_URL environment'dan olindi."
      make_vocab_dsn_from_url "$DATABASE_URL"
      return
    fi

    warn "DATABASE_URL noto'g'ri: '$DATABASE_URL'"
    warn "Bu to'liq PostgreSQL URL emas. Local DB sozlamalari ishlatiladi."
    unset DATABASE_URL
  fi

  make_local_database_url
}

psql_url() {
  PGCONNECT_TIMEOUT=10 \
  PGOPTIONS='-c statement_timeout=30000' \
  MSYS2_ARG_CONV_EXCL='*' \
  "$PSQL_BIN" -X -w -d "$DATABASE_URL" "$@"
}

check_project() {
  [[ -f "$BACKEND/go.mod" ]] || fail \
    "Script loyiha rootida turishi kerak. backend/go.mod topilmadi."
  [[ -d "$ROOT/data/vocabulary" ]] || warn \
    "data/vocabulary papkasi topilmadi; explicit CSV path berishingiz mumkin."
}

check_schema() {
  local exists
  exists="$(
    psql_url -tAc "SELECT to_regclass('vocabulary.lexemes') IS NOT NULL;" \
      | tr -d '[:space:]'
  )"

  [[ "$exists" == "t" ]] || fail \
    "vocabulary.lexemes jadvali topilmadi. Avval 'bash local.sh start' bilan migrationlarni ishga tushiring."
}

build_importer() {
  command -v go >/dev/null 2>&1 || fail "Go topilmadi."

  say "vocab-import build qilinmoqda..."
  (
    cd "$BACKEND"
    go build -o "$IMPORTER" ./cmd/vocab-import
  )
  ok "Importer tayyor: $IMPORTER"
}

find_default_words() {
  local f
  for f in \
    "$ROOT/data/vocabulary/generated/stage1_import_ready.csv" \
    "$ROOT/data/vocabulary/demo_seed.csv"
  do
    if [[ -f "$f" ]]; then
      printf '%s' "$f"
      return
    fi
  done
  return 1
}

find_default_synonyms() {
  local f
  for f in \
    "$ROOT/data/vocabulary/generated/stage1_synonyms_import_ready.csv" \
    "$ROOT/data/vocabulary/demo_synonyms.csv"
  do
    if [[ -f "$f" ]]; then
      printf '%s' "$f"
      return
    fi
  done
  return 1
}

show_status() {
  say "Vocabulary statistikasi"

  psql_url -P pager=off -c "
    SELECT
      count(*) FILTER (WHERE active) AS active_lexemes,
      count(*) AS total_lexemes,
      count(DISTINCT normalized_english) AS unique_english
    FROM vocabulary.lexemes;
  "

  psql_url -P pager=off -c "
    SELECT
      cefr,
      count(*) AS words
    FROM vocabulary.lexemes
    WHERE active
    GROUP BY cefr
    ORDER BY
      CASE cefr
        WHEN 'A1' THEN 1
        WHEN 'A2' THEN 2
        WHEN 'B1' THEN 3
        WHEN 'B2' THEN 4
        WHEN 'C1' THEN 5
        WHEN 'C2' THEN 6
        ELSE 99
      END;
  "

  psql_url -P pager=off -c "
    SELECT count(*) AS synonym_edges
    FROM vocabulary.synonym_edges;
  "
}

show_sample() {
  say "Vocabulary namunasi"

  psql_url -P pager=off -c "
    SELECT
      lemma_index,
      english,
      uzbek,
      part_of_speech,
      cefr,
      frequency_rank,
      synonym_group_id
    FROM vocabulary.lexemes
    WHERE active
    ORDER BY frequency_rank NULLS LAST, lemma_index
    LIMIT 25;
  "
}

import_data() {
  local words="${1:-}"
  local synonyms="${2:-}"
  local no_synonyms=0

  # Explicit "-" or "--no-synonyms" means: import words only.
  # This avoids accidentally auto-loading an older default synonyms.csv.
  if [[ "$synonyms" == "-" || "$synonyms" == "--no-synonyms" ]]; then
    synonyms=""
    no_synonyms=1
  fi

  if [[ -z "$words" ]]; then
    words="$(find_default_words || true)"
  fi

  [[ -n "$words" ]] || fail \
    "Words CSV topilmadi. Fayl pathini commandda bering."
  [[ -f "$words" ]] || fail "Words CSV mavjud emas: $words"

  if [[ -z "$synonyms" && "$no_synonyms" -eq 0 ]]; then
    synonyms="$(find_default_synonyms || true)"
  fi

  if [[ -n "$synonyms" && ! -f "$synonyms" ]]; then
    fail "Synonyms CSV mavjud emas: $synonyms"
  fi

  say "Import boshlanmoqda..."
  printf 'Words:    %s\n' "$words"
  if [[ -n "$synonyms" ]]; then
    printf 'Synonyms: %s\n' "$synonyms"
  else
    warn "Synonyms CSV topilmadi. Faqat lexemes import qilinadi."
  fi

  local log="$LOG_DIR/vocabulary-import-$(date +%Y%m%d-%H%M%S).log"

  # vocab-import.exe is a native Windows program when running under Git Bash.
  # Because MSYS2 path conversion is disabled to protect the PostgreSQL URI,
  # convert local file paths to native Windows paths explicitly.
  local words_arg="$words"
  local synonyms_arg="$synonyms"

  if [[ "${OS:-}" == "Windows_NT" ]] && command -v cygpath >/dev/null 2>&1; then
    words_arg="$(cygpath -w "$words")"
    if [[ -n "$synonyms" ]]; then
      synonyms_arg="$(cygpath -w "$synonyms")"
    fi
  fi

  local args=(
    -database "$VOCAB_DSN"
    -words "$words_arg"
  )

  if [[ -n "$synonyms_arg" ]]; then
    args+=(-synonyms "$synonyms_arg")
  fi

  set +e
  MSYS2_ARG_CONV_EXCL='*' \
    "$IMPORTER" "${args[@]}" 2>&1 | tee "$log"
  local rc=${PIPESTATUS[0]}
  set -e

  if (( rc != 0 )); then
    fail "Import xato bilan tugadi. Log: $log"
  fi

  ok "Vocabulary import tugadi."
  printf 'Log: %s\n' "$log"

  show_status
}

main() {
  check_project
  prepare_database
  check_schema

  local command="${1:-import}"
  shift || true

  case "$command" in
    import)
      build_importer
      import_data "${1:-}" "${2:-}"
      ;;
    status)
      show_status
      ;;
    sample)
      show_sample
      ;;
    *)
      cat <<'EOF'
Usage:
  bash import-vocabulary.sh import WORDS.csv [SYNONYMS.csv|-]
  bash import-vocabulary.sh status
  bash import-vocabulary.sh sample

Example (clean Stage 1):
  bash vocab_import.sh import \
    data/vocabulary/generated/stage1_import_ready.csv \
    data/vocabulary/generated/stage1_synonyms_import_ready.csv

Bootstrap demo only:
  bash vocab_import.sh import data/vocabulary/demo_seed.csv data/vocabulary/demo_synonyms.csv
EOF
      exit 2
      ;;
  esac
}

main "$@"