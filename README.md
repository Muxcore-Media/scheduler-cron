# Scheduler Cron

Cron-based task scheduler for periodic and recurring jobs in MuxCore.

Core has no built-in Scheduler — this module is the reference implementation
(on the default spool). Without it, there is no way to run tasks on a schedule;
all automation must be triggered manually or by external tooling.

## How It Works

```
Client schedules a task (HTTP or ModuleMesh Call):
  POST /schedule  { name, cron_expr, payload?, timeout?, once?, meta? }
  mesh Schedule   contracts.SchedulerTask JSON
        │
        ▼
scheduler-cron validates the cron expression and persists tasks when
SCHEDULER_STORE_PATH is set (JSON file)
        │
        ▼
At the scheduled time, the cron store fires the handler:
  publishes scheduler.task.* events on the core mesh
  optionally POSTs a webhook (meta.webhook_url or payload.webhook_url)
```

### Supported Cron Expressions

Standard 5-field cron expressions:

```
┌───────── minute (0-59)
│ ┌───────── hour (0-23)
│ │ ┌───────── day of month (1-31)
│ │ │ ┌───────── month (1-12)
│ │ │ │ ┌───────── day of week (0-6, 0=Sunday)
* * * * *
```

Also supports:
- `@every 5m` — run every 5 minutes
- `@daily` — run at midnight
- `@hourly` — run at the top of each hour
- `@once` — fire immediately once (or set `"once": true` on a real cron expr to fire at the next match then disarm)

## Configuration

### Environment

| Variable | Default | Description |
|----------|---------|-------------|
| `SCHEDULER_HTTP_ADDR` | `127.0.0.1:9200` | HTTP listen address for the schedule API |
| `SCHEDULER_API_TOKEN` | (empty) | Required when binding a non-loopback address; send as `X-Scheduler-Token` or `Authorization: Bearer` |
| `SCHEDULER_STORE_PATH` | (empty) | JSON file path for task persistence across restarts |
| `SCHEDULER_CATCH_UP` | `true` | Catch up missed fires when restoring from disk |
| `SCHEDULER_TZ` | `UTC` | IANA timezone for cron evaluation |
| `SCHEDULER_WEBHOOK_ALLOW_PRIVATE` | unset | Set to `1` to allow webhooks to loopback/private IPs (dev only) |
| `MUXCORE_GRPC_ADDR` | (SDK default) | Core gRPC address for sidecar registration and events |
| `MUXCORE_MODULE_ID` | `scheduler-cron` | Module identity when registering with core |
| `MUXCORE_INSECURE_DISABLE_TLS` | unset | Dev-only: disable TLS to core |

Task webhooks: set `meta.webhook_url` (or `payload.webhook_url`) on schedule; optional per-task `"timeout": "30s"`.

Events (via `contracts-media/events`): `scheduler.task.fired`, `scheduler.task.completed`, `scheduler.task.failed`, `scheduler.task.timeout`.

Integration: `go test -tags=integration ./test` (needs `MUXCORED_BIN` or sibling `../core`).

## HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/schedule` | Register a task (`name`, `cron_expr` required) |
| `DELETE` | `/cancel/{id}` | Cancel a task |
| `GET` | `/status/{id}` | Get task status |
| `GET` | `/list` | List tasks (`?name=` substring, `?status=` lifecycle filter) |
| `GET` | `/health` | Health check |
| `GET` | `/metrics` | Prometheus gauge `scheduler_tasks_total` + fired/completed/failed/timeout counters |

List/status responses include `timeout` as a duration string, object `payload` when JSON, `once`, `next_run`, and `last_fired_at`.

## ModuleMesh (`contracts.Scheduler`)

Peers with the `scheduler` capability can `Call` methods on this module's mesh service:

| Method | Payload | Response |
|--------|---------|----------|
| `Schedule` | `contracts.SchedulerTask` JSON | `{"task_id":"..."}` |
| `Cancel` | `{"task_id":"..."}` | `{"status":"cancelled"}` |
| `Status` | `{"task_id":"..."}` | `{"status":"..."}` |
| `List` | `contracts.SchedulerTaskFilter` JSON (optional) | `[]SchedulerTask` |

Settings: `Settings` / `UpdateSetting` (timezone, store path, catch-up).

## Implementation

- Registers with capabilities: `"scheduler"`, `"scheduler.cron"`, `"settings"`
- HTTP API mirrors `contracts.Scheduler` operations
- Uses `robfig/cron/v3` for cron expression parsing (including descriptors)
- Tasks persist to JSON when `SCHEDULER_STORE_PATH` is set
- Default HTTP listen address: `127.0.0.1:9200` (`_mvp/run-host.sh` uses `127.0.0.1:9204` when `MVP_ENABLE_SCHEDULER_CRON=1`)

Operator surface: [`muxcorectl-cli`](https://github.com/Muxcore-Media/muxcorectl-cli) `schedules list|status|add|cancel` (discovers this module's `HttpAddr`, or set `--scheduler-url` / `SCHEDULER_URL`).
