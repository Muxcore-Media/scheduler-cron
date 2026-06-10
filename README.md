# Scheduler Cron

Cron-based task scheduler for periodic and recurring jobs in MuxCore.

Without this module, there is no way to run tasks on a schedule. All automation
must be triggered manually or by external tooling.

## How It Works

```
Module registers cron task:
  Schedule(ctx, task{SchedulerTask{
    Name:     "daily-library-scan",
    CronExpr: "0 3 * * *",
    Payload:  json(`{"type": "library.scan"}`),
  }})
        │
        ▼
scheduler-cron parses expression and stores the schedule
        │
        ▼
At the scheduled time, the module publishes an event:
  scheduler.task.started → executor module picks up the task
  scheduler.task.completed → published on success
  scheduler.task.failed → published on error
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

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--timezone` | `UTC` | Timezone for cron evaluation |
| `--missed-startup` | `false` | Catch up on missed schedules after restart |

## Implementation

- Registers with capability: `"scheduler"`
- Implements `contracts.Scheduler` (Schedule, Cancel, Status, List)
- Uses `robfig/cron` for cron expression parsing
- Tasks are stored in-memory
- Published events: `scheduler.task.started`, `.completed`, `.failed`, `.cancelled`
- Timeout enforcement per task (from `SchedulerTask.Timeout`)
