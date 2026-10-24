# Shared helpers of scripts/backup.sh and scripts/restore-drill.sh
# (docs/OPERATIONS.md). Sourced, not run.
#
# Two modes (BACKUP_MODE):
#   compose (default)  the production stack of compose.prod.yml: Postgres
#                      tools run inside the postgres container (same major
#                      version as the server), object storage through the
#                      minio/mc image of the minio-init service on the app
#                      network. Nothing is published on the host.
#   direct             host tools against DB_HOST / S3_ENDPOINT (pg_dump,
#                      pg_restore, psql and mc on PATH), e.g. a managed
#                      database or a local check.
#
# Settings come from the env file (default .env.server, KEY=VALUE lines,
# never sourced as shell) and the environment; the environment wins.

set -euo pipefail

BACKUP_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

log() { printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; }
die() {
  log "error: $*"
  exit 1
}

# load_env FILE: export KEY=VALUE lines that are not already set.
load_env() {
  local file="$1" line key value
  [ -f "$file" ] || return 0
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in \#* | '') continue ;; esac
    if [[ "$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
      key="${BASH_REMATCH[1]}"
      value="${BASH_REMATCH[2]}"
      value="${value%\"}"
      value="${value#\"}"
      if [ -z "${!key+x}" ]; then
        export "$key=$value"
      fi
    fi
  done <"$file"
}

backup_init() {
  ENV_FILE="${ENV_FILE:-$BACKUP_ROOT/.env.server}"
  load_env "$ENV_FILE"
  BACKUP_MODE="${BACKUP_MODE:-compose}"
  BACKUP_DIR="${BACKUP_DIR:-$BACKUP_ROOT/backups}"
  BACKUP_STORAGE="${BACKUP_STORAGE:-on}"
  : "${DB_USER:?DB_USER is required}"
  : "${DB_NAME:?DB_NAME is required}"
  case "$BACKUP_MODE" in
    compose)
      command -v docker >/dev/null || die "docker is required in compose mode"
      COMPOSE=(docker compose -f "$BACKUP_ROOT/compose.prod.yml")
      if [ -f "$ENV_FILE" ]; then
        COMPOSE+=(--env-file "$ENV_FILE")
      fi
      ;;
    direct)
      for tool in pg_dump pg_restore psql; do
        command -v "$tool" >/dev/null || die "$tool is required in direct mode"
      done
      export PGHOST="${DB_HOST:-127.0.0.1}" PGPORT="${DB_PORT:-5432}" PGUSER="$DB_USER"
      if [ -n "${DB_PASSWORD:-}" ]; then
        export PGPASSWORD="$DB_PASSWORD"
      fi
      export PGSSLMODE="${DB_SSLMODE:-prefer}"
      ;;
    *) die "BACKUP_MODE must be compose or direct" ;;
  esac
}

# pg_run TOOL ARGS...: a Postgres client tool, stdin / stdout passed through.
pg_run() {
  local tool="$1"
  shift
  if [ "$BACKUP_MODE" = compose ]; then
    "${COMPOSE[@]}" exec -T postgres "$tool" -U "$DB_USER" "$@"
  else
    "$tool" "$@"
  fi
}

# sql DB QUERY: one value (unaligned, tuples only).
sql() {
  pg_run psql -X -v ON_ERROR_STOP=1 -At -d "$1" -c "$2"
}

# Exact row count of every table of the public schema, as one JSON object.
ROW_COUNTS_SQL="SELECT COALESCE(json_object_agg(table_name, n ORDER BY table_name), '{}'::json) FROM (
  SELECT table_name, (xpath('/row/c/text()', query_to_xml(format('SELECT COUNT(*) AS c FROM %I.%I', table_schema, table_name), false, true, '')))[1]::text::bigint AS n
  FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE') t"

# Applied migration (golang-migrate), or "none" without the table.
SCHEMA_VERSION_SQL="SELECT CASE WHEN to_regclass('public.schema_migrations') IS NULL THEN 'none'
  ELSE (SELECT COALESCE(MAX(version)::text, 'none') || CASE WHEN bool_or(dirty) THEN '-dirty' ELSE '' END FROM schema_migrations) END"

# mc_run SCRIPT [MOUNT]: run an mc shell script with the alias "src" set to
# the object store. Inside the script $S3_BUCKET is the live bucket,
# $DRILL_BUCKET the scratch bucket of a drill and $DEST the MOUNT directory.
mc_run() {
  local script="$1" mount="${2:-}"
  : "${S3_BUCKET:?S3_BUCKET is required}"
  if [ "$BACKUP_MODE" = compose ]; then
    local args=(run --rm --no-deps -T --entrypoint /bin/sh -e DEST=/backup -e "DRILL_BUCKET=${DRILL_BUCKET:-}")
    if [ -n "$mount" ]; then
      args+=(-v "$mount:/backup")
    fi
    "${COMPOSE[@]}" "${args[@]}" minio-init -c \
      'set -eu; mc alias set src "http://minio.$APP_NETWORK:9000" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null; '"$script"
  else
    command -v mc >/dev/null || die "mc is required in direct mode (or BACKUP_STORAGE=off)"
    : "${S3_ENDPOINT:?S3_ENDPOINT is required}"
    local scheme="${S3_ENDPOINT%%://*}" host="${S3_ENDPOINT#*://}"
    MC_HOST_src="$scheme://$S3_ACCESS_KEY:$S3_SECRET_KEY@$host" S3_BUCKET="$S3_BUCKET" \
      DRILL_BUCKET="${DRILL_BUCKET:-}" DEST="$mount" sh -c "set -eu; $script"
  fi
}

sha256_of() {
  if command -v sha256sum >/dev/null; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# Count and total size of the files under a directory.
dir_stats() {
  local dir="$1"
  if [ ! -d "$dir" ]; then
    echo "0 0"
    return
  fi
  find "$dir" -type f -printf '%s\n' | awk '{n++; s+=$1} END {printf "%d %d\n", n, s}'
}

now_s() { date -u +%s; }
