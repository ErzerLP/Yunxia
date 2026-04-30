# Logging Guidelines

> How logging is done in this project.

## Overview

The backend uses Go `log/slog` through `backend/internal/infrastructure/observability/logging/`. `cmd/server/main.go` creates the root logger and installs it with `slog.SetDefault(rootLogger)`.

Runtime config comes from `backend/internal/infrastructure/config/config.go`:

| Config field | Env key | Default |
|---|---|---|
| `logging.level` | `YUNXIA_LOGGING_LEVEL` | `info` |
| `logging.format` | `YUNXIA_LOGGING_FORMAT` | `json` |
| `logging.add_source` | `YUNXIA_LOGGING_ADD_SOURCE` | `false` |
| `logging.access_log_enabled` | `YUNXIA_LOGGING_ACCESS_LOG_ENABLED` | `true` |

## Logger Contract

Create child loggers with `logging.Component`:

```go
rootLogger := appLog.NewRootLogger(...)
auditRecorder := appaudit.NewRecorder(auditRepo, appLog.Component(rootLogger, "audit.recorder"))
logger := logging.Component(slog.Default(), "service.source")
```

Root logger includes `service`, `env`, `version`, and `commit`. Warn/error go to stderr; lower levels go to stdout.

## Request Logging

HTTP request logging is centralized in `backend/internal/interfaces/middleware/access_log.go`.

AccessLog middleware:

- Adds request-scoped logger to `context.Context`
- Adds audit request context (`request_id`, entry point, client IP, user agent, method, path)
- Logs completion event `http.request.completed`
- Maps status to level: `>=500` error, `>=400` warn, otherwise info
- Skips `/api/v1/health`

Required fields include:

```text
request_id, entrypoint, method, path, route, client_ip, user_agent,
status, latency_ms, response_bytes, error_code
```

`RequestID()` reads `X-Request-Id` or creates a UUID, sets Gin key `request_id`, and returns header `X-Request-Id`.

## Audit Logging

Audit is persisted separately via `application/audit.Recorder`.

Contract:

- Services call `recordServiceAudit(ctx, logger, recorder, appaudit.Event{...})`.
- Audit writes are best-effort: `appaudit.RecordBestEffort` logs `audit.write.failed` but must not fail business operations.
- Never include secrets in audit snapshots. Existing source audit view records endpoint/region/bucket/base_prefix/force_path_style, not secret values.

## Log Levels

| Level | Use for |
|---|---|
| debug | Local investigation / high-volume development detail |
| info | Lifecycle and successful request completion |
| warn | Client errors, denied requests, recoverable failures |
| error | Panic recovery, audit write failures, unexpected dependency failures |

## What NOT to Log

Do not log JWT tokens, passwords, password hashes, S3/access secrets, `secret_patch` values, file contents, large bodies, or raw user-controlled payloads without need.

## Wrong vs Correct

### Wrong

```go
log.Printf("token refresh failed: %s", refreshToken)
```

### Correct

```go
logger.Warn("refresh failed",
    slog.String("event", "auth.refresh.failed"),
    slog.Any("error", err),
)
```
