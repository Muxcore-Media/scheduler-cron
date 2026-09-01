# Security Policy

## Reporting a Vulnerability

Please report security vulnerabilities to the MuxCore team by emailing
security@muxcore.io or opening a draft security advisory on Forgejo
(`git.zem.systems/muxcore/scheduler-cron`).

Do not open public issues for security vulnerabilities.

## Scope

This module is part of the MuxCore ecosystem. Security issues in the core
platform should be reported to the core repository.

## Scheduler-specific controls

### HTTP API authentication

By default the module binds to **loopback only** (`127.0.0.1:9200`). When
`SCHEDULER_HTTP_ADDR` listens on a non-loopback interface, **`SCHEDULER_API_TOKEN`
is required**. Mutating and read endpoints (`/schedule`, `/cancel/`, `/list`,
`/status/`) reject requests without a matching `X-Scheduler-Token` header or
`Authorization: Bearer <token>`.

### Webhook SSRF

Outbound webhooks (`meta.webhook_url` / `payload.webhook_url`) accept **`http` and
`https` only**. Hostnames resolving to loopback, link-local, RFC1918, or
unspecified addresses are **blocked** unless
`SCHEDULER_WEBHOOK_ALLOW_PRIVATE=1` (development/homelab override only).

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | ✅        |
