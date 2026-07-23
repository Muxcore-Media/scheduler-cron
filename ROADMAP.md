# scheduler-cron — Remaining Work

### Events & Observability
- [ ] Publish events on task lifecycle (`scheduler.task.*`)
- [ ] Task timeout enforcement

### Persistence & Advanced
- [ ] Optional persistent schedule store via DatabaseProvider
- [ ] Missed schedule catch-up on restart
- [ ] Configurable timezone (currently UTC only)
- [ ] One-shot tasks (non-recurring)
- [ ] Integration test with running muxcored
