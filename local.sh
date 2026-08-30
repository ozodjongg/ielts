#!/usr/bin/env bash
set -Eeuo pipefail

# IELTS Platform — Windows native local launcher
# Ishlatish: Git Bash ichida, shu faylni loyiha root papkasiga qo'ying.
# Talablar: Go 1.23+, Node.js 20.9+, npm, PostgreSQL (psql), Git Bash.
# Docker yoki tashqi auth SDK kerak emas. Rollar: admin, center, teacher, student.

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
cd "$SCRIPT_DIR"

RUN_DIR="$SCRIPT_DIR/.local-run"
BIN_DIR="$RUN_DIR/bin"
LOG_DIR="$RUN_DIR/logs"
PID_DIR="$RUN_DIR/pids"
mkdir -p "$BIN_DIR" "$LOG_DIR" "$PID_DIR"

# ------------------------------
# LOCAL SETTINGS
# Bular local test uchun. Production Railway/Vercel env'lariga ta'sir qilmaydi.
# DATABASE_URL env oldindan berilgan bo'lsa, quyidagi DB sozlamalari ishlatilmaydi.
# ------------------------------
DB_HOST="${LOCAL_DB_HOST:-127.0.0.1}"
DB_PORT="${LOCAL_DB_PORT:-5432}"
DB_USER="${LOCAL_DB_USER:-postgres}"
DB_NAME="${LOCAL_DB_NAME:-ielts_platform}"

export BACKEND_ADDR="${BACKEND_ADDR:-:8080}"
export NEXT_PUBLIC_API_URL="${NEXT_PUBLIC_API_URL:-http://localhost:8080}"
export ADMIN_ORIGINS="${ADMIN_ORIGINS:-http://localhost:3001}"
export CENTER_ORIGINS="${CENTER_ORIGINS:-http://localhost:3002}"
export STUDENT_ORIGINS="${STUDENT_ORIGINS:-http://localhost:3003}"
# Agar kelajak/current build'da apps/teacher-web mavjud bo'lsa, u 3004 da ishlaydi.
# Aks holda teacher center-web (3002) orqali kiradi.
if [[ -d "$SCRIPT_DIR/apps/teacher-web" ]]; then
  export TEACHER_ORIGINS="${TEACHER_ORIGINS:-http://localhost:3004}"
else
  export TEACHER_ORIGINS="${TEACHER_ORIGINS:-$CENTER_ORIGINS}"
fi
export AUTO_MIGRATE="${AUTO_MIGRATE:-true}"
export REQUIRE_ADMIN_AAL2="${REQUIRE_ADMIN_AAL2:-true}"
export REQUIRE_CENTER_AAL2="${REQUIRE_CENTER_AAL2:-true}"
export REQUIRE_TEACHER_AAL2="${REQUIRE_TEACHER_AAL2:-true}"

# TOTP/MFA faqat admin, center va teacher uchun. Ushbu privileged rollarning
# barcha mutating/sensitive amallari localda ham AAL2 talab qiladi; student MFA ishlatmaydi.
export AUTH_TOTP_ISSUER="${AUTH_TOTP_ISSUER:-IELTS}"
export AUTH_TOTP_DIGITS="${AUTH_TOTP_DIGITS:-6}"
export AUTH_TOTP_PERIOD_SECONDS="${AUTH_TOTP_PERIOD_SECONDS:-30}"
export AUTH_TOTP_WINDOW="${AUTH_TOTP_WINDOW:-1}"
export AUTH_TOTP_SETUP_TTL_MINUTES="${AUTH_TOTP_SETUP_TTL_MINUTES:-10}"
export AUTH_MFA_RECOVERY_CODES="${AUTH_MFA_RECOVERY_CODES:-10}"
export GATEWAY_RATE_LIMIT_PER_MINUTE="${GATEWAY_RATE_LIMIT_PER_MINUTE:-600}"
export GATEWAY_AUTH_RATE_LIMIT_PER_MINUTE="${GATEWAY_AUTH_RATE_LIMIT_PER_MINUTE:-30}"
export VOCAB_DAILY_NEW="${VOCAB_DAILY_NEW:-10}"
export VOCAB_DAILY_REVIEW="${VOCAB_DAILY_REVIEW:-10}"
export LISTENING_MAX_UPLOAD_MB="${LISTENING_MAX_UPLOAD_MB:-50}"
export REVIEW_MAX_AUDIO_MB="${REVIEW_MAX_AUDIO_MB:-20}"
export LISTENING_STORAGE_DIR="${LISTENING_STORAGE_DIR:-$SCRIPT_DIR/.runtime/data/listening}"
export REVIEW_STORAGE_DIR="${REVIEW_STORAGE_DIR:-$SCRIPT_DIR/.runtime/data/review}"
export DB_MAX_CONNS="${DB_MAX_CONNS:-4}"
export DB_MIN_CONNS="${DB_MIN_CONNS:-0}"

# Local secretlar birinchi ishga tushishda kriptografik random tarzda yaratiladi
# va .local-run/secrets.env ichida saqlanadi. Production Railway env'lari bundan
# mustaqil va har doim alohida kuchli secretlardan foydalanishi kerak.
LOCAL_SECRET_FILE="$RUN_DIR/secrets.env"
if [[ ! -f "$LOCAL_SECRET_FILE" ]]; then
  node - <<'NODE' >"$LOCAL_SECRET_FILE"
const { randomBytes } = require("crypto");
const secret = () => randomBytes(48).toString("hex");
console.log(`LOCAL_AUTH_JWT_SECRET=${secret()}`);
console.log(`LOCAL_INTERNAL_SIGNING_SECRET=${secret()}`);
console.log(`LOCAL_PLAYBACK_SIGNING_SECRET=${secret()}`);
console.log(`LOCAL_QUESTION_SHUFFLE_SECRET=${secret()}`);
// 32-byte key encoded as hex; TOTP secret encryption uchun.
console.log(`LOCAL_AUTH_TOTP_ENCRYPTION_KEY=${randomBytes(32).toString("hex")}`);
NODE
  chmod 600 "$LOCAL_SECRET_FILE" 2>/dev/null || true
fi
# shellcheck disable=SC1090
source "$LOCAL_SECRET_FILE"

export AUTH_JWT_SECRET="${AUTH_JWT_SECRET:-$LOCAL_AUTH_JWT_SECRET}"
export AUTH_JWT_ISSUER="${AUTH_JWT_ISSUER:-ielts-platform}"
export AUTH_JWT_AUDIENCE="${AUTH_JWT_AUDIENCE:-ielts-platform}"
export AUTH_ACCESS_TTL_MINUTES="${AUTH_ACCESS_TTL_MINUTES:-15}"
export AUTH_REFRESH_TTL_DAYS="${AUTH_REFRESH_TTL_DAYS:-30}"
export INTERNAL_SIGNING_SECRET="${INTERNAL_SIGNING_SECRET:-$LOCAL_INTERNAL_SIGNING_SECRET}"
export PLAYBACK_SIGNING_SECRET="${PLAYBACK_SIGNING_SECRET:-$LOCAL_PLAYBACK_SIGNING_SECRET}"
export QUESTION_SHUFFLE_SECRET="${QUESTION_SHUFFLE_SECRET:-$LOCAL_QUESTION_SHUFFLE_SECRET}"

# MFA/TOTP secret encryption.
# Bir xil persistent 32-byte keydan foydalanamiz.
export MFA_ENCRYPTION_KEY="${MFA_ENCRYPTION_KEY:-$LOCAL_AUTH_TOTP_ENCRYPTION_KEY}"

# Backward compatibility.
export AUTH_TOTP_ENCRYPTION_KEY="${AUTH_TOTP_ENCRYPTION_KEY:-$MFA_ENCRYPTION_KEY}"

# Next dev server telemetry'ni localda o'chiramiz.
export NEXT_TELEMETRY_DISABLED=1

PSQL_BIN=""
DB_PASSWORD="${LOCAL_DB_PASSWORD:-}"

say()  { printf '\033[1;36m%s\033[0m\n' "$*"; }
ok()   { printf '\033[1;32m[OK]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[!]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[X]\033[0m %s\n' "$*" >&2; exit 1; }

require_project() {
  [[ -f "$SCRIPT_DIR/backend/go.mod" ]] || fail "local.sh loyiha root papkasida turishi kerak (backend/go.mod topilmadi)."
  [[ -f "$SCRIPT_DIR/apps/admin-web/package.json" ]] || fail "apps/admin-web topilmadi."
  [[ -f "$SCRIPT_DIR/apps/center-web/package.json" ]] || fail "apps/center-web topilmadi."
  [[ -f "$SCRIPT_DIR/apps/student-web/package.json" ]] || fail "apps/student-web topilmadi."
  if [[ -d "$SCRIPT_DIR/apps/teacher-web" ]]; then
    [[ -f "$SCRIPT_DIR/apps/teacher-web/package.json" ]] || fail "apps/teacher-web mavjud, lekin package.json topilmadi."
  fi
}

version_checks() {
  command -v go >/dev/null 2>&1 || fail "Go topilmadi. Go 1.23+ PATH ichida bo'lishi kerak."
  command -v node >/dev/null 2>&1 || fail "Node.js topilmadi. Node 20.9+ PATH ichida bo'lishi kerak."
  command -v npm >/dev/null 2>&1 || fail "npm topilmadi."

  local go_v node_major node_minor
  go_v="$(go version | awk '{print $3}')"
  node_major="$(node -p 'process.versions.node.split(".")[0]')"
  node_minor="$(node -p 'process.versions.node.split(".")[1]')"
  say "Go:   $go_v"
  say "Node: $(node --version)"

  if (( node_major < 20 || (node_major == 20 && node_minor < 9) )); then
    fail "Next.js uchun Node.js 20.9+ kerak."
  fi
}

find_psql() {
  if command -v psql >/dev/null 2>&1; then
    PSQL_BIN="$(command -v psql)"
    return
  fi

  # Windows PostgreSQL standard install joylarini avtomatik qidirish.
  local candidate
  for candidate in "/c/Program Files/PostgreSQL/"*/bin/psql.exe "/c/Program Files (x86)/PostgreSQL/"*/bin/psql.exe; do
    if [[ -x "$candidate" ]]; then
      PSQL_BIN="$candidate"
    fi
  done

  [[ -n "$PSQL_BIN" ]] || fail "psql topilmadi. PostgreSQL o'rnating yoki PostgreSQL\\bin ni PATH'ga qo'shing."
}

urlencode() {
  node -e 'process.stdout.write(encodeURIComponent(process.argv[1]))' "$1"
}

prepare_database() {
  # Agar foydalanuvchi DATABASE_URL ni oldindan bergan bo'lsa, o'shani ishlatamiz.
  if [[ -n "${DATABASE_URL:-}" ]]; then
    ok "DATABASE_URL environment'dan olindi."
    return
  fi

  find_psql

  [[ "$DB_NAME" =~ ^[A-Za-z0-9_]+$ ]] || fail "LOCAL_DB_NAME faqat harf, raqam va underscore (_) dan iborat bo'lsin."
  [[ "$DB_USER" =~ ^[A-Za-z0-9_.-]+$ ]] || fail "LOCAL_DB_USER noto'g'ri formatda."

  if [[ -z "$DB_PASSWORD" ]]; then
    printf "PostgreSQL paroli (%s@%s:%s): " "$DB_USER" "$DB_HOST" "$DB_PORT"
    IFS= read -r -s DB_PASSWORD
    printf '\n'
  fi

  say "PostgreSQL tekshirilmoqda..."
  if ! PGPASSWORD="$DB_PASSWORD" "$PSQL_BIN" -X -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -tAc "SELECT 1" >/dev/null 2>&1; then
    fail "PostgreSQL'ga ulanib bo'lmadi. Server ishlayotganini va parolni tekshiring."
  fi

  local exists
  exists="$(PGPASSWORD="$DB_PASSWORD" "$PSQL_BIN" -X -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$DB_NAME'" 2>/dev/null | tr -d '[:space:]')"
  if [[ "$exists" != "1" ]]; then
    say "Database '$DB_NAME' yaratilmoqda..."
    PGPASSWORD="$DB_PASSWORD" "$PSQL_BIN" -X -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"$DB_NAME\"" >/dev/null
    ok "Database yaratildi: $DB_NAME"
  else
    ok "Database mavjud: $DB_NAME"
  fi

  local enc_user enc_pass enc_db
  enc_user="$(urlencode "$DB_USER")"
  enc_pass="$(urlencode "$DB_PASSWORD")"
  enc_db="$(urlencode "$DB_NAME")"
  export DATABASE_URL="postgresql://${enc_user}:${enc_pass}@${DB_HOST}:${DB_PORT}/${enc_db}?sslmode=disable"
}

pid_file() { printf '%s/%s.pid' "$PID_DIR" "$1"; }
log_file() { printf '%s/%s.log' "$LOG_DIR" "$1"; }

is_pid_running() {
  local pid="$1"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  if command -v tasklist.exe >/dev/null 2>&1; then
    MSYS2_ARG_CONV_EXCL='*' tasklist.exe /FI "PID eq $pid" /NH 2>/dev/null | tr -d '\r' | grep -Eq "[[:space:]]${pid}[[:space:]]"
  else
    kill -0 "$pid" >/dev/null 2>&1
  fi
}

service_running() {
  local file
  file="$(pid_file "$1")"
  [[ -f "$file" ]] || return 1
  local pid
  pid="$(cat "$file" 2>/dev/null || true)"
  is_pid_running "$pid"
}

check_free_port() {
  local port="$1" name="$2"
  if command -v netstat.exe >/dev/null 2>&1; then
    if netstat.exe -ano 2>/dev/null | tr -d '\r' | grep -E "[:.]${port}[[:space:]].*LISTENING" >/dev/null; then
      fail "Port $port band ($name). Avval 'bash local.sh stop' qiling yoki portni band qilgan processni yoping."
    fi
  fi
}

install_frontend_deps() {
  local app="$1"
  if [[ ! -d "$SCRIPT_DIR/apps/$app/node_modules/next" ]]; then
    say "$app dependencies o'rnatilmoqda..."
    (cd "$SCRIPT_DIR/apps/$app" && npm install --no-audit --no-fund)
    ok "$app dependencies tayyor."
  fi
}

build_backend() {
  say "Backend build qilinmoqda..."
  (cd "$SCRIPT_DIR/backend" && go build -o "$BIN_DIR/backend.exe" ./cmd/backend)
  ok "Backend build tayyor."
}

start_backend() {
  local log pidf
  log="$(log_file backend)"
  pidf="$(pid_file backend)"
  (
    cd "$SCRIPT_DIR"
    nohup "$BIN_DIR/backend.exe" >"$log" 2>&1 &
    echo $! >"$pidf"
  )
}

start_frontend() {
  local app="$1" port="$2" name="$3" log pidf
  log="$(log_file "$name")"
  pidf="$(pid_file "$name")"
  (
    cd "$SCRIPT_DIR/apps/$app"
    NEXT_PUBLIC_API_URL="$NEXT_PUBLIC_API_URL" nohup npm run dev -- -p "$port" >"$log" 2>&1 &
    echo $! >"$pidf"
  )
}

wait_url() {
  local name="$1" url="$2" attempts="${3:-45}"
  if ! command -v curl >/dev/null 2>&1; then
    warn "curl yo'q; $name readiness avtomatik tekshirilmadi."
    return 0
  fi
  local i
  for ((i=1; i<=attempts; i++)); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      ok "$name tayyor: $url"
      return 0
    fi
    sleep 1
  done
  warn "$name hali javob bermadi. Log: $(log_file "$name")"
  return 0
}

cmd_start() {
  require_project
  version_checks

  if service_running backend || service_running admin || service_running center || service_running student || service_running teacher; then
    warn "Ba'zi local processlar allaqachon ishlayapti."
    cmd_status
    printf "Avval restart qilaymi? [y/N]: "
    local ans
    IFS= read -r ans
    [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]] || exit 0
    cmd_stop
  fi

  check_free_port 8080 backend
  check_free_port 3001 admin
  check_free_port 3002 center
  check_free_port 3003 student
  if [[ -d "$SCRIPT_DIR/apps/teacher-web" ]]; then
    check_free_port 3004 teacher
  fi

  prepare_database
  build_backend
  install_frontend_deps admin-web
  install_frontend_deps center-web
  install_frontend_deps student-web
  if [[ -d "$SCRIPT_DIR/apps/teacher-web" ]]; then
    install_frontend_deps teacher-web
  fi

  # Eski loglarni tozalaymiz.
  : >"$(log_file backend)"
  : >"$(log_file admin)"
  : >"$(log_file center)"
  : >"$(log_file student)"
  if [[ -d "$SCRIPT_DIR/apps/teacher-web" ]]; then
    : >"$(log_file teacher)"
  fi

  say "Backend ishga tushmoqda..."
  start_backend
  wait_url backend "http://localhost:8080/ready" 60

  say "Frontendlar ishga tushmoqda..."
  start_frontend admin-web 3001 admin
  start_frontend center-web 3002 center
  start_frontend student-web 3003 student
  if [[ -d "$SCRIPT_DIR/apps/teacher-web" ]]; then
    start_frontend teacher-web 3004 teacher
  fi

  wait_url admin "http://localhost:3001" 60
  wait_url center "http://localhost:3002" 60
  wait_url student "http://localhost:3003" 60
  if [[ -d "$SCRIPT_DIR/apps/teacher-web" ]]; then
    wait_url teacher "http://localhost:3004" 60
  fi

  # Vocabulary tekshiruvi frontend startup'ni bloklamaydi.
  seed_vocab_if_empty

  printf '\n'
  ok "IELTS Platform localda ishga tushdi."
  printf '\n  Admin:          http://localhost:3001\n  Center:         http://localhost:3002\n'
  if [[ -d "$SCRIPT_DIR/apps/teacher-web" ]]; then
    printf '  Teacher:        http://localhost:3004\n'
  else
    printf '  Teacher:        http://localhost:3002  (center-web, teacher role)\n'
  fi
  printf '  Student:        http://localhost:3003\n  Backend:        http://localhost:8080\n  Ready:          http://localhost:8080/ready\n\n'
  printf "Birinchi admin yo'q bo'lsa: bash local.sh admin\n"
  printf "Privileged MFA: admin/center/teacher -> Security -> QR scan -> verify 6-digit code\n"
  printf "Loglar:                   bash local.sh logs backend\n"
  printf "To'xtatish:               bash local.sh stop\n"
}

kill_port() {
  local port="$1" label="$2"
  command -v netstat.exe >/dev/null 2>&1 || return 0
  command -v taskkill.exe >/dev/null 2>&1 || return 0
  local pids pid
  pids="$(netstat.exe -ano 2>/dev/null | tr -d '\r' | awk -v p=":${port}" '$2 ~ p"$" && $4 == "LISTENING" {print $5}' | sort -u)"
  [[ -n "$pids" ]] || return 0
  while IFS= read -r pid; do
    [[ "$pid" =~ ^[0-9]+$ ]] || continue
    MSYS2_ARG_CONV_EXCL='*' taskkill.exe /PID "$pid" /T /F >/dev/null 2>&1 || true
    ok "$label orphan process yopildi (PID=$pid, port=$port)."
  done <<< "$pids"
}

seed_vocab_if_empty() {
  find_psql

  local count vocab_dsn seed_log words_path synonyms_path
  seed_log="$LOG_DIR/vocabulary-seed.log"
  : >"$seed_log"

  say "Vocabulary corpus holati tekshirilmoqda..."

  # Startup hech qachon psql password prompt yoki DB lock sabab osilib qolmasin.
  # -w                    => interaktiv password prompt yo'q
  # PGCONNECT_TIMEOUT=5   => connection uchun max 5 soniya
  # statement_timeout     => query uchun max 5 soniya
  # MSYS2_ARG_CONV_EXCL   => Git Bash postgres:// URL'ni Windows pathga aylantirmaydi
  if ! count="$(
    PGCONNECT_TIMEOUT=5 \
    PGOPTIONS='-c statement_timeout=5000' \
    MSYS2_ARG_CONV_EXCL='*' \
    "$PSQL_BIN" -X -w -d "$DATABASE_URL" \
      -tAc "SELECT count(*) FROM vocabulary.lexemes WHERE active" \
      2>>"$seed_log" | tr -d '[:space:]'
  )"; then
    warn "Vocabulary holatini tekshirib bo'lmadi; frontendlar baribir ishlaydi."
    warn "Log: $seed_log"
    warn "Keyin alohida urinish: bash local.sh seed-vocab"
    return 0
  fi

  if [[ "$count" =~ ^[0-9]+$ ]] && (( count > 0 )); then
    ok "Vocabulary corpus mavjud: $count active lexeme."
    return 0
  fi

  words_path="$SCRIPT_DIR/data/vocabulary/demo_seed.csv"
  synonyms_path="$SCRIPT_DIR/data/vocabulary/demo_synonyms.csv"

  if [[ ! -f "$words_path" ]]; then
    warn "Vocabulary bo'sh, lekin demo seed topilmadi: $words_path"
    return 0
  fi

  # libpq/pgx `search_path`ni alohida URI parametri sifatida qabul qilmaydi.
  # Standard `options=-c search_path=...` connection parametri ishlatiladi.
  if ! vocab_dsn="$(
    node - "$DATABASE_URL" <<'NODE'
const raw = process.argv[2];
try {
  const u = new URL(raw);
  u.searchParams.set("options", "-c search_path=vocabulary,public");
  process.stdout.write(u.toString());
} catch (e) {
  console.error(String(e.message || e));
  process.exit(1);
}
NODE
  )"; then
    warn "Vocabulary DSN yaratib bo'lmadi; frontendlar baribir ishlaydi."
    return 0
  fi

  # Native Windows Go process uchun Git Bash /d/... pathlarini D:\... ga aylantiramiz.
  if command -v cygpath >/dev/null 2>&1; then
    words_path="$(cygpath -w "$words_path")"
    if [[ -f "$SCRIPT_DIR/data/vocabulary/demo_synonyms.csv" ]]; then
      synonyms_path="$(cygpath -w "$synonyms_path")"
    fi
  fi

  say "Vocabulary QA seed import qilinmoqda (A1-C2)..."

  if [[ -f "$SCRIPT_DIR/data/vocabulary/demo_synonyms.csv" ]]; then
    if ! (
      cd "$SCRIPT_DIR/backend"
      MSYS2_ARG_CONV_EXCL='*' go run ./cmd/vocab-import \
        -database "$vocab_dsn" \
        -words "$words_path" \
        -synonyms "$synonyms_path"
    ) >>"$seed_log" 2>&1; then
      warn "Vocabulary seed import xato bilan tugadi; frontendlar baribir ishlaydi."
      warn "Log: $seed_log"
      return 0
    fi
  else
    if ! (
      cd "$SCRIPT_DIR/backend"
      MSYS2_ARG_CONV_EXCL='*' go run ./cmd/vocab-import \
        -database "$vocab_dsn" \
        -words "$words_path"
    ) >>"$seed_log" 2>&1; then
      warn "Vocabulary seed import xato bilan tugadi; frontendlar baribir ishlaydi."
      warn "Log: $seed_log"
      return 0
    fi
  fi

  ok "Vocabulary QA seed tayyor."
}

kill_service() {
  local name="$1" file pid
  file="$(pid_file "$name")"
  [[ -f "$file" ]] || return 0
  pid="$(cat "$file" 2>/dev/null || true)"
  if is_pid_running "$pid"; then
    if command -v taskkill.exe >/dev/null 2>&1; then
      MSYS2_ARG_CONV_EXCL='*' taskkill.exe /PID "$pid" /T /F >/dev/null 2>&1 || true
    else
      kill "$pid" >/dev/null 2>&1 || true
    fi
    ok "$name to'xtatildi."
  fi
  rm -f "$file"
}

cmd_stop() {
  if [[ -d "$SCRIPT_DIR/apps/teacher-web" ]]; then
    kill_service teacher
  fi
  kill_service student
  kill_service center
  kill_service admin
  kill_service backend
  # npm/Next child processlar parent PID yopilgandan keyin portda qolib ketishi mumkin.
  if [[ -d "$SCRIPT_DIR/apps/teacher-web" ]]; then
    kill_port 3004 teacher
  fi
  kill_port 3003 student
  kill_port 3002 center
  kill_port 3001 admin
  kill_port 8080 backend
}

cmd_restart() {
  cmd_stop
  cmd_start
}

status_one() {
  local name="$1" url="$2" file pid="-"
  file="$(pid_file "$name")"
  [[ -f "$file" ]] && pid="$(cat "$file" 2>/dev/null || true)"
  if [[ "$pid" != "-" ]] && is_pid_running "$pid"; then
    printf '\033[1;32mRUNNING\033[0m  %-8s PID=%s  %s\n' "$name" "$pid" "$url"
  else
    printf '\033[1;31mSTOPPED\033[0m  %-8s %s\n' "$name" "$url"
  fi
}

cmd_status() {
  status_one backend "http://localhost:8080"
  status_one admin "http://localhost:3001"
  status_one center "http://localhost:3002"
  status_one student "http://localhost:3003"
  if [[ -d "$SCRIPT_DIR/apps/teacher-web" ]]; then
    status_one teacher "http://localhost:3004"
  else
    printf '[1;36mINFO[0m     teacher  center-web orqali: http://localhost:3002
'
  fi
}

cmd_logs() {
  local name="${1:-backend}"
  case "$name" in
    backend|admin|center|student|teacher) ;;
    *) fail "logs uchun: backend | admin | center | student | teacher" ;;
  esac
  local file
  file="$(log_file "$name")"
  [[ -f "$file" ]] || fail "Log hali yo'q: $file"
  say "$name logi (chiqish: Ctrl+C)"
  tail -n 100 -f "$file"
}

cmd_admin() {
  require_project
  version_checks
  prepare_database

  # Admin yaratishdan oldin migrationlarni kafolatlaymiz.
  say "Migrationlar tekshirilmoqda..."
  (cd "$SCRIPT_DIR/backend" && MIGRATIONS_DIR="$SCRIPT_DIR/backend/migrations" go run ./cmd/compact-migrate)

  local email name password password2
  printf "Admin email: "
  IFS= read -r email
  printf "Admin name [IELTS Platform Admin]: "
  IFS= read -r name
  name="${name:-IELTS Platform Admin}"
  printf "Admin password (10+ belgi): "
  IFS= read -r -s password
  printf '\nPassword qayta: '
  IFS= read -r -s password2
  printf '\n'
  [[ "$password" == "$password2" ]] || fail "Passwordlar bir xil emas."

  (cd "$SCRIPT_DIR/backend" && go run ./cmd/bootstrap-admin --email "$email" --password "$password" --name "$name")
}

cmd_reset_password() {
  require_project
  version_checks
  prepare_database
  local email password password2
  printf "Account email: "
  IFS= read -r email
  printf "Yangi password (10+ belgi): "
  IFS= read -r -s password
  printf '\nPassword qayta: '
  IFS= read -r -s password2
  printf '\n'
  [[ "$password" == "$password2" ]] || fail "Passwordlar bir xil emas."
  (cd "$SCRIPT_DIR/backend" && go run ./cmd/reset-password --email "$email" --password "$password")
}

cmd_seed_vocab() {
  require_project
  version_checks
  prepare_database
  say "Migrationlar tekshirilmoqda..."
  (cd "$SCRIPT_DIR/backend" && MIGRATIONS_DIR="$SCRIPT_DIR/backend/migrations" go run ./cmd/compact-migrate)
  seed_vocab_if_empty
}

cmd_seed_demo() {
  require_project
  version_checks
  prepare_database
  say "Migrationlar tekshirilmoqda..."
  (cd "$SCRIPT_DIR/backend" && MIGRATIONS_DIR="$SCRIPT_DIR/backend/migrations" go run ./cmd/compact-migrate)
  seed_vocab_if_empty
  say "Demo center, center admin, teacher, A1-C2 studentlar, group, vocabulary homework, assessment, SAT, listening, review, points va analytics data yaratilmoqda..."
  (cd "$SCRIPT_DIR/backend" && go run ./cmd/seed-demo --listening-storage "$LISTENING_STORAGE_DIR" "${@:1}")
  ok "Demo data tayyor. Loginlar yuqorida ko'rsatildi."
}

cmd_qa() {
  require_project
  version_checks
  command -v python >/dev/null 2>&1 || command -v python3 >/dev/null 2>&1 || fail "Python QA script uchun kerak."
  local py=python qa_script
  command -v python >/dev/null 2>&1 || py=python3
  if [[ -f "$SCRIPT_DIR/tools/qa_ielts.py" ]]; then
    qa_script="$SCRIPT_DIR/tools/qa_ielts.py"
  elif [[ -f "$SCRIPT_DIR/tools/qa_v5.py" ]]; then
    qa_script="$SCRIPT_DIR/tools/qa_v5.py"
    warn "Legacy tools/qa_v5.py ishlatilmoqda. Fayl nomini qa_ielts.py ga yangilash tavsiya etiladi."
  else
    fail "QA script topilmadi (tools/qa_ielts.py)."
  fi
  "$py" "$qa_script"
}

cmd_clean() {
  cmd_stop
  warn "Bu faqat build/log/node_modules cache'larini o'chiradi. PostgreSQL database O'CHIRILMAYDI."
  rm -rf "$RUN_DIR"
  rm -rf "$SCRIPT_DIR/apps/admin-web/.next" "$SCRIPT_DIR/apps/center-web/.next" "$SCRIPT_DIR/apps/student-web/.next"
  [[ ! -d "$SCRIPT_DIR/apps/teacher-web" ]] || rm -rf "$SCRIPT_DIR/apps/teacher-web/.next"
  ok "Local runtime cache tozalandi."
}

usage() {
  cat <<'TXT'
IELTS Platform — Windows local launcher

Git Bash ichida:
  bash local.sh start            Backend + IELTS frontendlarini ishga tushiradi
  bash local.sh stop             Hammasini to'xtatadi
  bash local.sh restart          Qayta ishga tushiradi
  bash local.sh status           Holatini ko'rsatadi
  bash local.sh logs backend     Backend logini ko'rsatadi
  bash local.sh logs admin       Admin frontend logi
  bash local.sh logs center      Center/Teacher frontend logi
  bash local.sh logs student     Student frontend logi
  bash local.sh logs teacher     Teacher frontend logi (agar apps/teacher-web mavjud bo'lsa)
  bash local.sh admin            Birinchi platform admin yaratadi
  bash local.sh reset-password   Local account passwordini reset qiladi
  bash local.sh seed-vocab       A1-C2 QA vocabulary seedini import qiladi
  bash local.sh seed-demo        To'liq demo/QA dataset yaratadi
  bash local.sh qa               Static/data/API contract QA ni ishga tushiradi
  bash local.sh clean            Runtime/.next cache'larni tozalaydi

Optional environment:
  LOCAL_DB_HOST=127.0.0.1
  LOCAL_DB_PORT=5432
  LOCAL_DB_USER=postgres
  LOCAL_DB_PASSWORD=your_password
  LOCAL_DB_NAME=ielts_platform

AAL2/TOTP optional overrides:
  REQUIRE_ADMIN_AAL2=true
  REQUIRE_CENTER_AAL2=true
  REQUIRE_TEACHER_AAL2=true
  AUTH_TOTP_ISSUER=IELTS

Teacher portal:
  - apps/teacher-web mavjud bo'lsa: http://localhost:3004
  - mavjud bo'lmasa: teacher center-web orqali http://localhost:3002 da ishlaydi

Agar DATABASE_URL oldindan set qilingan bo'lsa, script local PostgreSQL sozlamalarini chetlab o'tadi.
TXT
}

menu() {
  printf '\nIELTS Platform — Local\n'
  printf '1) Start\n2) Stop\n3) Restart\n4) Status\n5) Backend logs\n6) Create IELTS admin\n7) Reset password\n8) Seed vocabulary\n9) Seed full demo data\n10) QA\n11) Clean cache\n0) Exit\n\nTanlang: '
  local choice
  IFS= read -r choice
  case "$choice" in
    1) cmd_start ;;
    2) cmd_stop ;;
    3) cmd_restart ;;
    4) cmd_status ;;
    5) cmd_logs backend ;;
    6) cmd_admin ;;
    7) cmd_reset_password ;;
    8) cmd_seed_vocab ;;
    9) cmd_seed_demo ;;
    10) cmd_qa ;;
    11) cmd_clean ;;
    0) exit 0 ;;
    *) usage; exit 1 ;;
  esac
}

main() {
  local cmd="${1:-menu}"
  case "$cmd" in
    start) cmd_start ;;
    stop) cmd_stop ;;
    restart) cmd_restart ;;
    status) cmd_status ;;
    logs) cmd_logs "${2:-backend}" ;;
    admin) cmd_admin ;;
    reset-password) cmd_reset_password ;;
    seed-vocab) cmd_seed_vocab ;;
    seed-demo) shift; cmd_seed_demo "$@" ;;
    qa) cmd_qa ;;
    clean) cmd_clean ;;
    help|-h|--help) usage ;;
    menu) menu ;;
    *) usage; exit 1 ;;
  esac
}

main "$@"