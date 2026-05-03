# Notification Guidelines

> Backend contracts for webhook notification channels, event persistence, and retry behavior.

## Scenario: Webhook Notifications for RSS Automation

### 1. Scope / Trigger

- Trigger: notification channels, notification event persistence, webhook delivery, retry workers, or RSS alert event emission.
- Goal: important unattended RSS states must be observable even when a webhook endpoint is temporarily unavailable.

### 2. Signatures

- Service entry point:

```go
type NotificationWebhookSender interface {
    Send(ctx context.Context, endpoint NotificationWebhookEndpoint, payload NotificationWebhookPayload) error
}

func (s *NotificationService) Notify(ctx context.Context, input NotificationEventInput) (*entity.NotificationEvent, error)
```

- RSS integration:

```go
type rssNotifier interface {
    Notify(ctx context.Context, input NotificationEventInput) (*entity.NotificationEvent, error)
}
```

### 3. Contracts

- Notification events are persisted before delivery is attempted.
- Channel type is currently `webhook`; future Telegram / WeCom integrations must reuse the same channel/event model instead of adding one-off RSS-only config.
- Supported event types are:
  - `rss.source_failure`
  - `rss.item_needs_attention`
  - `rss.download_completed`
- Channel `event_types=[]` means receive every supported event.
- Channel list/detail responses must never return secret plaintext. Return `secret_configured` only.
- Webhook delivery uses `POST application/json`; when a secret exists, add `X-Yunxia-Timestamp` and `X-Yunxia-Signature: sha256=<hmac>`.
- Event status values are fixed:
  - `pending`
  - `delivered`
  - `retry_pending`
  - `failed`
  - `skipped`
- Delivery failure must set `retry_pending` while attempts remain, with `attempts`, `last_attempt_at`, `next_attempt_at`, and `last_error` populated.
- Manual retry must not allow already `delivered` or `skipped` events and should return `TASK_INVALID_STATE` for those states.
- RSS source failure notifications should be noise-controlled: emit when health transitions into `degraded` or `circuit_open`, not on every failed refresh.
- RSS item `needs_attention` notification should be emitted when the item transitions into `needs_attention`, not on every later list/update.
- RSS download completed notification should be emitted when the RSS item backlink reconciler marks the item `completed` from a terminal completed download task.

### 4. Validation & Error Matrix

| Condition | Behavior |
|---|---|
| Webhook URL is missing or not http/https | `CONFIG_INVALID` |
| Channel type is not `webhook` | `NOTIFICATION_CHANNEL_UNSUPPORTED` |
| Unknown event type in channel filter or query | `CONFIG_INVALID` |
| Webhook endpoint returns non-2xx during test/retry | `NOTIFICATION_DELIVERY_FAILED` |
| Event has no matching enabled channel | Persist event as `skipped` |
| Event delivery fails with attempts remaining | Persist `retry_pending` and `next_attempt_at` |
| Event delivery fails at max attempts | Persist `failed` |
| Manual retry delivered/skipped event | `TASK_INVALID_STATE` |

### 5. Tests Required

- Service test: channel response hides secret and preserves `secret_configured`.
- Service test: failed webhook dispatch becomes `retry_pending` and manual retry can mark `delivered`.
- Service test: event type filter skips unmatched channels.
- Repository test: channel config/event JSON and due retry filters persist correctly.
- RSS integration tests when changing event trigger conditions.
- Full gate: `go test -count=1 ./...`.
