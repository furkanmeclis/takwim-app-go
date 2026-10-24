# Operations

Backups, the restore drill, error tracking, log monitoring and the failed work lists (TEC-930, acceptance A22: operation history, failed tasks, provider errors and a restore attempt can be evaluated).

| What                | Where                                                                                       |
| ------------------- | ------------------------------------------------------------------------------------------- |
| Backup              | `scripts/backup.sh` (`make backup`)                                                         |
| Restore drill       | `scripts/restore-drill.sh` (`make restore-drill`)                                           |
| Error tracking      | Sentry protocol, [DEVELOPMENT.md](./DEVELOPMENT.md#error-tracking-optional-sentry-protocol) |
| Logs                | Platform → System settings → Logs (`/platform/settings/logs`)                               |
| Operation history   | Platform → System settings → Activity (`/platform/settings/activity`)                       |
| Health, failed work | Platform → System settings → Jobs and errors (`/platform/settings/operations`)              |
| API                 | `/v1/platform/operations/*` (`backend/internal/modules/operations`, `openapi.yaml`)         |

## 1. Backups

`scripts/backup.sh` writes one folder per run under `BACKUP_DIR` (default `./backups`, ignored by git):

```text
backups/20261024T030000Z/
  postgres.dump      pg_dump custom format (-Fc) of DB_NAME
  storage/           a mirror of the S3_BUCKET bucket (mc mirror)
  manifest.json      checksum, applied migration, row count of every table,
                     object count and size, durations
```

```sh
make backup                                   # compose.prod.yml + .env.server
ENV_FILE=/srv/takwim/.env.server BACKUP_DIR=/srv/backups scripts/backup.sh
BACKUP_MODE=direct BACKUP_STORAGE=off scripts/backup.sh   # host pg tools, database only
```

| Setting            | Default       | Meaning                                                                                                                                                     |
| ------------------ | ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ENV_FILE`         | `.env.server` | `KEY=VALUE` file read for `DB_*`, `S3_*`, `NAME_PREFIX` (never sourced as shell)                                                                            |
| `BACKUP_MODE`      | `compose`     | `compose`: tools run in the stack's `postgres` and `minio-init` (mc) containers; `direct`: host `pg_dump` / `psql` / `mc` against `DB_HOST` / `S3_ENDPOINT` |
| `BACKUP_DIR`       | `./backups`   | Target folder                                                                                                                                               |
| `BACKUP_KEEP_DAYS` | `14`          | Run folders older than this are removed (`0` keeps all)                                                                                                     |
| `BACKUP_STORAGE`   | `on`          | `off` skips the bucket                                                                                                                                      |

In `compose` mode the dump runs inside the `postgres` container, so client and server have the same major version (18), and nothing is published on the host. The bucket is mirrored by the `minio/mc` image of `minio-init` on the app network.

What is not backed up, on purpose: Redis (queue state, rate limits, caches, step-up sessions: jobs are recorded in Postgres and the minute sweeps pick lost work up again), Meilisearch (rebuilt with `make search-reindex`), Centrifugo (stateless).

**The backup folder holds personal data** (customers, phone numbers, files). Keep it on an encrypted disk with `0700` permissions (the script sets `umask 077`), never inside the repository, and copy it off the server. The copy to another location and the schedule belong to the hosting setup (see §6).

Schedule (host cron, example):

```cron
15 3 * * * cd /srv/takwim && ENV_FILE=/srv/takwim/.env.server BACKUP_DIR=/srv/backups scripts/backup.sh >>/var/log/takwim-backup.log 2>&1
45 3 * * 0 cd /srv/takwim && ENV_FILE=/srv/takwim/.env.server BACKUP_DIR=/srv/backups scripts/restore-drill.sh >>/var/log/takwim-backup.log 2>&1
```

## 2. Restore drill

`scripts/restore-drill.sh [backup folder]` (default: the newest) restores a backup next to the live data and checks it; the live database and bucket are never written:

1. the dump's SHA-256 equals the manifest;
2. `pg_restore --no-owner --no-acl --exit-on-error` into `DRILL_DB` (default `<DB_NAME>_restore_drill`, recreated);
3. the same applied migration (`schema_migrations`) and the same row count in **every** table of the public schema;
4. the bucket copy goes into `DRILL_BUCKET` (default `<S3_BUCKET>-restore-drill`, recreated); object count and total size equal the backup;
5. scratch database and bucket are removed (`DRILL_KEEP=1` keeps them for a look).

Each step is timed. The result is written to `<backup>/drill-<time>.json` and one line is appended to `BACKUP_DIR/drills.log`:

```text
20261024T091634Z passed backup=20261024T091633Z db=6s storage=off total=6s tables=110 rows=200523
```

The exit code is `1` when a check failed, so a cron job or CI step can alert on it. A failed drill means the backup is not usable: investigate before the next deploy.

### Measured

Local measurement on the development machine (PostgreSQL 16, local MinIO, `BACKUP_MODE=direct`), migration 73:

| Data                                                             | Backup | Restore drill (database / storage / total) |
| ---------------------------------------------------------------- | ------ | ------------------------------------------ |
| Empty schema (110 tables, 510 rows)                              | < 1 s  | 2 s / – / 3 s                              |
| 200 523 rows (99 MB database, 7.7 MB dump) + 301 objects (14 MB) | 1 s    | 4 s / 1 s / 5 s                            |

The production drill (the pilot server, `compose` mode) is part of the hosting setup (§6); its `drills.log` lines are the numbers to quote for the pilot.

## 3. Restoring for real

Only after a decision to roll the live data back; it replaces what is there.

```sh
# 1. Stop writers.
docker compose -f compose.prod.yml --env-file .env.server stop backend worker
# 2. Database: drop and restore (the dump carries the schema and the migration table).
docker compose -f compose.prod.yml --env-file .env.server exec -T postgres \
  pg_restore -U "$DB_USER" -d "$DB_NAME" --clean --if-exists --no-owner --exit-on-error < backups/<run>/postgres.dump
# 3. Files: copy the bucket back (add --remove to drop objects newer than the backup).
docker compose -f compose.prod.yml --env-file .env.server run --rm --no-deps -T \
  -v "$PWD/backups/<run>/storage:/backup" --entrypoint /bin/sh minio-init -c \
  'mc alias set src "http://minio.$APP_NETWORK:9000" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" && mc mirror --overwrite /backup/ "src/$S3_BUCKET"'
# 4. Start again and rebuild the search index.
docker compose -f compose.prod.yml --env-file .env.server up -d backend worker
make search-reindex
```

Run the drill on the same backup first: it shows how long step 2 and 3 will take. Restored objects get their content type from the file name (the API derives media types from names too).

## 4. Error tracking and logs

- Error tracking: set `BACKEND_SENTRY_DSN` (API and worker) and `FRONTEND_SENTRY_DSN` (Next.js); what is reported and what is never sent is in [DEVELOPMENT.md](./DEVELOPMENT.md#error-tracking-optional-sentry-protocol). Failed queue tasks are reported with their `task_type`. The Jobs and errors page shows whether the API and the web app report (`error_tracking`).
- Logs: `WARN` / `ERROR` lines of the API and the worker are stored and searchable on the Logs page (purge rules there). `stdout` JSON logs stay available to the host's log collector.
- Operation history: every platform and business action is in the activity log, including the actions of this page (`operations.queue_task_run`, `operations.queue_task_deleted`, `operations.integration_job_retried`).

## 5. Jobs and errors page

`/platform/settings/operations` (read: `platform.settings.read`, actions: `platform.settings.write`; both held by the super admin role). The system health page (`/platform/settings`) shows the same summary in its "Background work" card.

| Tab             | Source                                                                                                                     | Action                                                                                                                                                                                                                          |
| --------------- | -------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Overview        | Asynq queues (pending, active, retry, archived, today's processed / failed), running workers, counts below, error tracking | –                                                                                                                                                                                                                               |
| Failed tasks    | Asynq tasks out of attempts (`archived`) or waiting for the next attempt (`retry`), every queue                            | Run now, delete. Jobs are idempotent: a task run again decides itself whether work is left                                                                                                                                      |
| Provider errors | `integration_jobs` of every business: `failed`, `needs_verification`, `blocked` (filter for others)                        | Open (provider calls), retry: CheckStatus first, resubmit only work the provider confirms was not done (same rule as the business's own button, [integration-jobs.md](./design/integration-jobs.md#6-staff-retry-yeniden-dene)) |
| Notice errors   | Failed `customer_notification_deliveries` of every business (event, channel, provider, error)                              | None: a channel is tried at most once per notice ([customer-notifications.md](./design/customer-notifications.md))                                                                                                              |

The lists show the business's name and never customer data (no recipient, no message text, no payload). The summary `status` is `attention` when no worker runs, the queue cannot be read, a task is out of attempts, an integration job needs a person, a notice failed in the last 24 hours or an outbox event failed.

## 6. Open (hosting, TEC-886)

These need the real server and accounts and are tracked as a follow-up issue:

- run `scripts/backup.sh` and `scripts/restore-drill.sh` on the pilot server in `compose` mode (scheduled), and record the measured times;
- an off-server copy of the backup folder (encrypted, another provider or region) and its retention;
- alerting on a failed drill / backup (cron mail or the monitoring of the host) and on `status: attention`;
- the Sentry-protocol endpoint and the two DSNs.
