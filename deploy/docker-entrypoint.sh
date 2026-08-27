#!/usr/bin/env sh
set -eu

APP_DIR="${APP_DIR:-/app}"
DATASET_VERSION="${VOCAB_DATASET_VERSION:-stage1-clean-v1}"
WORDS_FILE="${VOCAB_WORDS_FILE:-$APP_DIR/data/vocabulary/generated/stage1_import_ready.csv}"
SYNONYMS_FILE="${VOCAB_SYNONYMS_FILE:-}"
AUTO_IMPORT="${VOCAB_AUTO_IMPORT:-true}"
IMPORT_STALE_SECONDS="${VOCAB_IMPORT_STALE_SECONDS:-600}"
IMPORT_WAIT_SECONDS="${VOCAB_IMPORT_WAIT_SECONDS:-90}"
OWNER_TOKEN="$(hostname)-$$-$(date +%s)"

: "${DATABASE_URL:?DATABASE_URL is required}"

mkdir -p /data/private/listening /data/private/review
chown -R app:app /data

echo "[startup] applying database migrations..."
su-exec app env MIGRATIONS_DIR="${MIGRATIONS_DIR:-$APP_DIR/backend-migrations}" /app/compact-migrate

if [ "$AUTO_IMPORT" = "true" ]; then
  if [ ! -f "$WORDS_FILE" ]; then
    FALLBACK_WORDS="$APP_DIR/data/vocabulary/demo_seed.csv"
    FALLBACK_SYNONYMS="$APP_DIR/data/vocabulary/demo_synonyms.csv"
    if [ -f "$FALLBACK_WORDS" ]; then
      echo "[startup] clean Stage 1 dataset not found; using clean bootstrap vocabulary."
      WORDS_FILE="$FALLBACK_WORDS"
      SYNONYMS_FILE="$FALLBACK_SYNONYMS"
      DATASET_VERSION="demo-clean-v1"
    else
      echo "[startup] vocabulary auto-import skipped: no clean dataset found."
      WORDS_FILE=""
    fi
  fi

  if [ -n "$WORDS_FILE" ]; then
    WORDS_HASH="$(sha256sum "$WORDS_FILE" | awk '{print $1}')"
    SYNONYMS_HASH=""
    if [ -n "$SYNONYMS_FILE" ] && [ -f "$SYNONYMS_FILE" ]; then
      SYNONYMS_HASH="$(sha256sum "$SYNONYMS_FILE" | awk '{print $1}')"
    fi

    psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
CREATE SCHEMA IF NOT EXISTS vocabulary;
CREATE TABLE IF NOT EXISTS vocabulary.dataset_versions (
    version text PRIMARY KEY,
    status text NOT NULL DEFAULT 'complete',
    words_sha256 text,
    synonyms_sha256 text,
    owner_token text,
    started_at timestamptz NOT NULL DEFAULT now(),
    imported_at timestamptz,
    row_count integer NOT NULL DEFAULT 0
);
ALTER TABLE vocabulary.dataset_versions ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'complete';
ALTER TABLE vocabulary.dataset_versions ADD COLUMN IF NOT EXISTS words_sha256 text;
ALTER TABLE vocabulary.dataset_versions ADD COLUMN IF NOT EXISTS synonyms_sha256 text;
ALTER TABLE vocabulary.dataset_versions ADD COLUMN IF NOT EXISTS owner_token text;
ALTER TABLE vocabulary.dataset_versions ADD COLUMN IF NOT EXISTS started_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE vocabulary.dataset_versions ADD COLUMN IF NOT EXISTS imported_at timestamptz;
ALTER TABLE vocabulary.dataset_versions ADD COLUMN IF NOT EXISTS row_count integer NOT NULL DEFAULT 0;
SQL

    # Existing complete versions are immutable by checksum. An importing row can
    # be left behind when Railway terminates a container during the import. We
    # wait for a live import briefly and automatically recover only truly stale
    # claims. Most importantly, a live claim never causes a crash/restart loop.
    WAITED=0
    while [ -n "$WORDS_FILE" ]; do
      STATE="$(psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -v version="$DATASET_VERSION" -At <<'SQL'
SELECT status || '|' || coalesce(words_sha256,'') || '|' || coalesce(synonyms_sha256,'') || '|' ||
       greatest(0, extract(epoch FROM (now()-started_at))::bigint)::text
FROM vocabulary.dataset_versions
WHERE version=:'version';
SQL
)"

      if [ -z "$STATE" ]; then
        break
      fi

      STATUS="$(printf '%s' "$STATE" | cut -d'|' -f1)"
      OLD_WORDS_HASH="$(printf '%s' "$STATE" | cut -d'|' -f2)"
      OLD_SYNONYMS_HASH="$(printf '%s' "$STATE" | cut -d'|' -f3)"
      AGE_SECONDS="$(printf '%s' "$STATE" | cut -d'|' -f4)"

      if [ "$STATUS" = "complete" ]; then
        if { [ -z "$OLD_WORDS_HASH" ] || [ "$OLD_WORDS_HASH" = "$WORDS_HASH" ]; } \
          && { [ -z "$OLD_SYNONYMS_HASH" ] || [ "$OLD_SYNONYMS_HASH" = "$SYNONYMS_HASH" ]; }; then
          echo "[startup] vocabulary $DATASET_VERSION already imported; skip."
          WORDS_FILE=""
          break
        fi
        echo "[startup] ERROR: VOCAB_DATASET_VERSION=$DATASET_VERSION already exists with a different file checksum." >&2
        echo "[startup] Use a new VOCAB_DATASET_VERSION for changed vocabulary data." >&2
        exit 1
      fi

      if [ "$STATUS" = "importing" ]; then
        case "$AGE_SECONDS" in
          ''|*[!0-9]*) AGE_SECONDS=0 ;;
        esac

        if [ "$AGE_SECONDS" -ge "$IMPORT_STALE_SECONDS" ]; then
          echo "[startup] stale vocabulary import claim detected (${AGE_SECONDS}s old); recovering it."
          DELETED="$(psql "$DATABASE_URL" -v ON_ERROR_STOP=1 \
            -v version="$DATASET_VERSION" -v stale="$IMPORT_STALE_SECONDS" -At <<'SQL'
DELETE FROM vocabulary.dataset_versions
WHERE version=:'version'
  AND status='importing'
  AND started_at < now() - make_interval(secs => :'stale'::integer)
RETURNING 'deleted';
SQL
)"
          if [ "$DELETED" = "deleted" ]; then
            break
          fi
          continue
        fi

        if [ "$WAITED" -lt "$IMPORT_WAIT_SECONDS" ]; then
          echo "[startup] vocabulary $DATASET_VERSION is currently importing (${AGE_SECONDS}s old); waiting..."
          sleep 5
          WAITED=$((WAITED + 5))
          continue
        fi

        echo "[startup] WARNING: vocabulary $DATASET_VERSION is still importing after ${IMPORT_WAIT_SECONDS}s." >&2
        echo "[startup] Starting backend without taking over the live claim; another deployment may finish it." >&2
        WORDS_FILE=""
        break
      fi

      echo "[startup] ERROR: unknown vocabulary dataset status '$STATUS' for $DATASET_VERSION." >&2
      exit 1
    done

    if [ -n "$WORDS_FILE" ]; then
      CLAIM="$(psql "$DATABASE_URL" -v ON_ERROR_STOP=1 \
        -v version="$DATASET_VERSION" -v words_hash="$WORDS_HASH" -v synonyms_hash="$SYNONYMS_HASH" \
        -v owner="$OWNER_TOKEN" -At <<'SQL'
INSERT INTO vocabulary.dataset_versions(version,status,words_sha256,synonyms_sha256,owner_token,started_at,row_count)
VALUES(:'version','importing',:'words_hash',nullif(:'synonyms_hash',''),:'owner',now(),0)
ON CONFLICT(version) DO NOTHING
RETURNING 'claimed';
SQL
)"

      if [ "$CLAIM" != "claimed" ]; then
        echo "[startup] vocabulary $DATASET_VERSION was claimed concurrently; starting backend and leaving import to the owner."
        WORDS_FILE=""
      fi
    fi

    if [ -n "$WORDS_FILE" ]; then
      echo "[startup] importing clean vocabulary: $WORDS_FILE"
      if [ -n "$SYNONYMS_FILE" ] && [ -f "$SYNONYMS_FILE" ]; then
        if ! su-exec app /app/vocab-import -database "$DATABASE_URL" -words "$WORDS_FILE" -synonyms "$SYNONYMS_FILE"; then
          psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -v version="$DATASET_VERSION" -v owner="$OWNER_TOKEN" \
            -c "DELETE FROM vocabulary.dataset_versions WHERE version=:'version' AND status='importing' AND owner_token=:'owner'" >/dev/null || true
          exit 1
        fi
      else
        if ! su-exec app /app/vocab-import -database "$DATABASE_URL" -words "$WORDS_FILE"; then
          psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -v version="$DATASET_VERSION" -v owner="$OWNER_TOKEN" \
            -c "DELETE FROM vocabulary.dataset_versions WHERE version=:'version' AND status='importing' AND owner_token=:'owner'" >/dev/null || true
          exit 1
        fi
      fi

      ROW_COUNT="$(psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -tAc "SELECT count(*) FROM vocabulary.lexemes WHERE active=true" | tr -d '[:space:]')"
      UPDATED="$(psql "$DATABASE_URL" -v ON_ERROR_STOP=1 \
        -v version="$DATASET_VERSION" -v rows="${ROW_COUNT:-0}" \
        -v words_hash="$WORDS_HASH" -v synonyms_hash="$SYNONYMS_HASH" -v owner="$OWNER_TOKEN" -At <<'SQL'
UPDATE vocabulary.dataset_versions
SET status='complete', imported_at=now(), row_count=:'rows'::integer,
    words_sha256=:'words_hash', synonyms_sha256=nullif(:'synonyms_hash',''), owner_token=NULL
WHERE version=:'version' AND status='importing' AND owner_token=:'owner'
RETURNING 'complete';
SQL
)"
      if [ "$UPDATED" != "complete" ]; then
        echo "[startup] ERROR: vocabulary import finished but ownership marker changed unexpectedly." >&2
        exit 1
      fi
      echo "[startup] vocabulary import complete; active lexemes=${ROW_COUNT:-0}"
    fi
  fi
else
  echo "[startup] vocabulary auto-import disabled."
fi

echo "[startup] starting backend..."
exec su-exec app /app/backend