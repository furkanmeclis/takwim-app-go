#!/usr/bin/env bash
# Backup of the production data (TEC-930, docs/OPERATIONS.md): a Postgres
# dump (custom format) and a copy of the object storage bucket, with a
# manifest (checksums, applied migration, row count of every table, object
# count and size, durations) the restore drill checks against.
#
#   scripts/backup.sh                       # compose.prod.yml + .env.server
#   ENV_FILE=/srv/takwim/.env.server BACKUP_DIR=/srv/backups scripts/backup.sh
#   BACKUP_MODE=direct BACKUP_STORAGE=off scripts/backup.sh   # host tools, DB only
#
# Settings (env file or environment):
#   BACKUP_DIR        where backups go (default ./backups); one folder per run
#   BACKUP_KEEP_DAYS  older run folders are removed (default 14; 0 = keep all)
#   BACKUP_STORAGE    on | off (default on)
#   BACKUP_MODE       compose | direct (see scripts/backup-lib.sh)
#
# The backup folder holds personal data: keep it on encrypted disk, outside
# the repository, and copy it off the server (docs/OPERATIONS.md).
set -euo pipefail
# shellcheck source=scripts/backup-lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/backup-lib.sh"

backup_init
umask 077
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
out="$BACKUP_DIR/$stamp"
mkdir -p "$out"
log "backup $stamp → $out (mode $BACKUP_MODE)"
started="$(now_s)"

# Database: the dump, then the facts the drill compares (taken right after
# the dump; writes in between show up as a row count difference).
t0="$(now_s)"
pg_run pg_dump -d "$DB_NAME" -Fc --no-password >"$out/postgres.dump"
db_seconds=$(($(now_s) - t0))
db_bytes="$(stat -c %s "$out/postgres.dump")"
[ "$db_bytes" -gt 0 ] || die "empty dump"
db_sha="$(sha256_of "$out/postgres.dump")"
schema_version="$(sql "$DB_NAME" "$SCHEMA_VERSION_SQL")"
row_counts="$(sql "$DB_NAME" "$ROW_COUNTS_SQL")"
server_version="$(sql "$DB_NAME" "SHOW server_version")"
log "database: $db_bytes bytes in ${db_seconds}s (migration $schema_version)"

# Object storage: a mirror of the bucket.
storage_json='null'
if [ "$BACKUP_STORAGE" = on ]; then
  mkdir -p "$out/storage"
  t0="$(now_s)"
  mc_run 'mc mirror --quiet --preserve "src/$S3_BUCKET" "$DEST/" >/dev/null' "$out/storage"
  st_seconds=$(($(now_s) - t0))
  read -r st_objects st_bytes < <(dir_stats "$out/storage")
  storage_json="{\"bucket\":\"$S3_BUCKET\",\"objects\":$st_objects,\"bytes\":$st_bytes,\"seconds\":$st_seconds}"
  log "storage: $st_objects objects, $st_bytes bytes in ${st_seconds}s"
fi

cat >"$out/manifest.json" <<EOF
{
  "format": 1,
  "created_at": "$stamp",
  "app": "${NAME_PREFIX:-}",
  "mode": "$BACKUP_MODE",
  "database": {
    "name": "$DB_NAME",
    "server_version": "$server_version",
    "file": "postgres.dump",
    "bytes": $db_bytes,
    "sha256": "$db_sha",
    "schema_version": "$schema_version",
    "seconds": $db_seconds,
    "row_counts": $row_counts
  },
  "storage": $storage_json,
  "seconds": $(($(now_s) - started))
}
EOF
log "manifest: $out/manifest.json"

# Retention: remove run folders older than BACKUP_KEEP_DAYS.
keep="${BACKUP_KEEP_DAYS:-14}"
if [ "$keep" -gt 0 ]; then
  find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name '20*Z' -mtime "+$keep" -print -exec rm -rf {} + |
    while read -r old; do log "removed old backup $old"; done
fi
echo "$out"
