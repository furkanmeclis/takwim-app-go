#!/usr/bin/env bash
# Restore drill (TEC-930, docs/OPERATIONS.md): restores a backup made by
# scripts/backup.sh into a scratch database and a scratch bucket next to the
# live ones, checks it against the manifest and measures how long each step
# takes. The live database and bucket are never written.
#
#   scripts/restore-drill.sh backups/20261024T030000Z
#   scripts/restore-drill.sh            # the newest backup in BACKUP_DIR
#
# Checks: dump checksum, pg_restore without errors, the same applied
# migration, the same row count in every table, the same object count and
# size in the bucket. The result goes to <backup>/drill-<time>.json and one
# line to BACKUP_DIR/drills.log; the exit code is 1 when a check failed.
#
# Settings: those of scripts/backup.sh, plus
#   DRILL_DB      scratch database (default <DB_NAME>_restore_drill)
#   DRILL_BUCKET  scratch bucket (default <S3_BUCKET>-restore-drill)
#   DRILL_KEEP=1  keep the scratch database and bucket for a look
set -euo pipefail
# shellcheck source=scripts/backup-lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/backup-lib.sh"

backup_init
src="${1:-}"
if [ -z "$src" ]; then
  src="$(find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name '20*Z' | sort | tail -n 1)"
  [ -n "$src" ] || die "no backup in $BACKUP_DIR"
fi
src="$(cd "$src" && pwd)"
manifest="$src/manifest.json"
[ -f "$manifest" ] || die "$manifest not found"

DRILL_DB="${DRILL_DB:-${DB_NAME}_restore_drill}"
[ "$DRILL_DB" != "$DB_NAME" ] || die "DRILL_DB must differ from the live database"
[[ "$DRILL_DB" =~ ^[a-z_][a-z0-9_]*$ ]] || die "DRILL_DB must be a plain lower-case name"
export DRILL_BUCKET="${DRILL_BUCKET:-${S3_BUCKET:-none}-restore-drill}"
[ "$DRILL_BUCKET" != "${S3_BUCKET:-}" ] || die "DRILL_BUCKET must differ from the live bucket"

# manifest_get KEY: a scalar of the manifest (flat lookup, our own format).
manifest_get() {
  sed -n "s/^ *\"$1\": *\"\{0,1\}\([^\",]*\)\"\{0,1\},\{0,1\}$/\1/p" "$manifest" | head -n 1
}

failures=()
fail() {
  failures+=("$1")
  log "FAIL: $1"
}
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
log "drill $stamp of $src → database $DRILL_DB"
started="$(now_s)"

# 1. The dump is the one that was written.
want_sha="$(manifest_get sha256)"
got_sha="$(sha256_of "$src/postgres.dump")"
[ "$want_sha" = "$got_sha" ] || fail "dump checksum differs ($got_sha, manifest $want_sha)"

# 2. Restore into the scratch database.
sql postgres "DROP DATABASE IF EXISTS \"$DRILL_DB\"" >/dev/null
sql postgres "CREATE DATABASE \"$DRILL_DB\"" >/dev/null
t0="$(now_s)"
restore_log="$src/drill-$stamp-pg_restore.log"
if ! pg_run pg_restore -d "$DRILL_DB" --no-owner --no-acl --exit-on-error <"$src/postgres.dump" 2>"$restore_log"; then
  fail "pg_restore failed (see $restore_log)"
fi
db_seconds=$(($(now_s) - t0))
[ -s "$restore_log" ] || rm -f "$restore_log"
log "database restored in ${db_seconds}s"

# 3. Same migration, same rows.
want_version="$(manifest_get schema_version)"
got_version="$(sql "$DRILL_DB" "$SCHEMA_VERSION_SQL")"
[ "$want_version" = "$got_version" ] || fail "migration $got_version, manifest $want_version"
got_counts="$(sql "$DRILL_DB" "$ROW_COUNTS_SQL")"
want_counts="$(sed -n 's/^ *"row_counts": *\(.*\)$/\1/p' "$manifest")"
# Compare table by table inside Postgres (no jq on the host needed).
diff_sql="SELECT COALESCE(string_agg(k || ': ' || COALESCE(w.value, '-') || ' → ' || COALESCE(g.value, '-'), ', ' ORDER BY k), '')
  FROM json_each_text('$want_counts'::json) w FULL JOIN json_each_text('$got_counts'::json) g USING (key)
  CROSS JOIN LATERAL (SELECT COALESCE(w.key, g.key) AS k) x
  WHERE w.value IS DISTINCT FROM g.value"
row_diff="$(sql "$DRILL_DB" "$diff_sql")"
tables="$(sql "$DRILL_DB" "SELECT COUNT(*) FROM json_object_keys('$got_counts'::json)")"
rows="$(sql "$DRILL_DB" "SELECT COALESCE(SUM(value::bigint), 0) FROM json_each_text('$got_counts'::json)")"
[ -z "$row_diff" ] || fail "row counts differ: $row_diff"
log "database checked: $tables tables, $rows rows, migration $got_version"

# 4. Object storage into the scratch bucket.
storage_json='null'
if [ "$BACKUP_STORAGE" = on ] && [ -d "$src/storage" ]; then
  read -r want_objects want_bytes < <(dir_stats "$src/storage")
  # The copy is complete: as many objects as the backup counted.
  manifest_objects="$(sed -n 's/.*"objects":\([0-9]*\).*/\1/p' "$manifest" | head -n 1)"
  [ "$manifest_objects" = "$want_objects" ] || fail "backup folder has $want_objects objects, manifest $manifest_objects"
  t0="$(now_s)"
  if ! mc_run 'mc rb --force "src/$DRILL_BUCKET" >/dev/null 2>&1 || true; mc mb "src/$DRILL_BUCKET" >/dev/null; mc mirror --quiet "$DEST/" "src/$DRILL_BUCKET" >/dev/null' "$src/storage"; then
    fail "storage restore failed"
  fi
  st_seconds=$(($(now_s) - t0))
  read -r got_objects got_bytes < <(mc_run 'mc ls --recursive --json "src/$DRILL_BUCKET" | grep -o "\"size\":[0-9]*" | awk -F: "{n++; s+=\$2} END {printf \"%d %d\\n\", n, s}"' "$src/storage")
  [ "$want_objects" = "$got_objects" ] || fail "objects $got_objects, backup $want_objects"
  [ "$want_bytes" = "$got_bytes" ] || fail "storage bytes $got_bytes, backup $want_bytes"
  storage_json="{\"bucket\":\"$DRILL_BUCKET\",\"objects\":$got_objects,\"bytes\":$got_bytes,\"seconds\":$st_seconds}"
  log "storage restored: $got_objects objects in ${st_seconds}s"
fi

# 5. Clean up unless asked to keep.
if [ "${DRILL_KEEP:-0}" != 1 ]; then
  sql postgres "DROP DATABASE IF EXISTS \"$DRILL_DB\"" >/dev/null
  if [ "$storage_json" != null ]; then
    mc_run 'mc rb --force "src/$DRILL_BUCKET" >/dev/null' "$src/storage" || log "could not remove $DRILL_BUCKET"
  fi
fi

total=$(($(now_s) - started))
result=passed
[ ${#failures[@]} -eq 0 ] || result=failed
failures_json="$(printf '%s\n' "${failures[@]:-}" | sed '/^$/d; s/\\/\\\\/g; s/"/\\"/g; s/.*/"&"/' | paste -sd, -)"
report="$src/drill-$stamp.json"
cat >"$report" <<EOF
{
  "drill_at": "$stamp",
  "backup": "$(basename "$src")",
  "backup_created_at": "$(manifest_get created_at)",
  "result": "$result",
  "failures": [${failures_json}],
  "database": {
    "name": "$DRILL_DB",
    "restore_seconds": $db_seconds,
    "schema_version": "$got_version",
    "tables": $tables,
    "rows": $rows
  },
  "storage": $storage_json,
  "seconds": $total
}
EOF
printf '%s %s backup=%s db=%ss storage=%s total=%ss tables=%s rows=%s\n' \
  "$stamp" "$result" "$(basename "$src")" "$db_seconds" "${st_seconds:-off}" "$total" "$tables" "$rows" >>"$BACKUP_DIR/drills.log"
log "drill $result in ${total}s → $report"
cat "$report"
[ "$result" = passed ]
