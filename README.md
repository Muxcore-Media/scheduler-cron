# Scheduler Cron

Cron-based task scheduler for periodic and recurring jobs in MuxCore.

Core has no built-in Scheduler — this module is the reference implementation
(on the default spool). Without it, there is no way to run tasks on a schedule;
all automation must be triggered manually or by external tooling.

## How It Works

```
Client registers a cron task via HTTP:
  POST /schedule  { name, cron_expr, payload?, timeout?, meta? }
        │
        ▼
scheduler-cron validates the expression and stores the schedule in-memory
        │
        ▼
At the scheduled time, the cron store fires the task handler
(currently logs only; see ROADMAP for event publishing)
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

## Configuration

### Environment

| Variable | Default | Description |
|----------|---------|-------------|
| `SCHEDULER_HTTP_ADDR` | `:9200` | HTTP listen address for the schedule API |
| `MUXCORE_GRPC_ADDR` | (SDK default) | Core gRPC address for sidecar registration |
| `MUXCORE_MODULE_ID` | `scheduler-cron` | Module identity when registering with core |
| `MUXCORE_INSECURE_DISABLE_TLS` | unset | Dev-only: disable TLS to core |

Timezone: set `SCHEDULER_TZ` (IANA name; default UTC). One-shot: `"once": true` or `cron_expr: "@once"`. Task webhook timeout: `"timeout": "30s"`. Events: `scheduler.task.fired|completed|failed|timeout`. Persistence: set `SCHEDULER_STORE_PATH` (JSON). Missed fires are caught up on restore. Still open: muxcored integration test.

## HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/schedule` | Register a task (`name`, `cron_expr` required) |
| `DELETE` | `/cancel/{id}` | Cancel a task |
| `GET` | `/status/{id}` | Get task status |
| `GET` | `/list` | List tasks (`?name=` substring filter) |
| `GET` | `/health` | Health check |
| `GET` | `/metrics` | Prometheus gauge `scheduler_tasks_total` |

## Implementation

- Registers with capabilities: `"scheduler"`, `"scheduler.cron"`
- HTTP API mirrors `contracts.Scheduler` operations (Schedule, Cancel, Status, List)
- Uses `robfig/cron/v3` for cron expression parsing (including descriptors)
- Tasks are stored in-memory
- Default HTTP listen address: `:9200`
