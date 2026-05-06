# RSS and qBittorrent Guidelines

> Backend implementation contracts for RSS feed parsing, subscription matching,
> and qBittorrent task ingestion.

## Scenario: RSS Feed Parsing and qBittorrent Torrent URL Ingestion

### 1. Scope / Trigger

- Trigger: RSS feed parsing, RSS subscription matching, BT/magnet task routing,
  qBittorrent Web API calls, or task status mapping changes.
- Reason: RSS feeds often put key fields in provider-specific extensions, and
  qBittorrent can accept `.torrent` URLs differently from uploaded torrent
  files. These differences must not leak as null timestamps, false matches, or
  silent canceled tasks.

### 2. Signatures

- Parser entry point:

```go
type RSSFetcher interface {
    Fetch(ctx context.Context, rawURL string) ([]RSSFetchedItem, error)
}
```

- Subscription matcher:

```go
func rssSubscriptionMatchesItem(subscription *entity.RSSSubscription, item *entity.RSSItem) bool
```

- Downloader routing:

```go
func ClassifyDownloadLink(rawLink string) string
func NewQBittorrentClient(apiURL, username, password string) *QBittorrentClient
func (c *QBittorrentClient) Health(ctx context.Context) error
func (c *QBittorrentClient) AddURI(ctx context.Context, uri string, dir string) (string, error)
func (c *QBittorrentClient) TellStatus(ctx context.Context, externalID string) (*service.DownloadStatus, error)
```

- qBittorrent deployment env:

```text
YUNXIA_QBITTORRENT_API_URL=http://qbittorrent:8080
YUNXIA_QBITTORRENT_USERNAME=
YUNXIA_QBITTORRENT_PASSWORD=
```

- RSS-created task naming snapshot:

```go
type CreateTaskRequest struct {
    TargetVirtualParentPath string `json:"target_virtual_parent_path,omitempty"`
    TargetFilename          string `json:"target_filename,omitempty"`
}

func (s *TaskService) importCompletedTask(ctx context.Context, task *entity.DownloadTask) error
```

### 3. Contracts

- RSS `published_at` must be populated when any supported feed field is
  parseable:
  - RSS item `pubDate`
  - Mikan torrent extension `torrent/pubDate`
  - RSS item `date`
  - Atom `published`, then `updated`
- Mikan torrent extension `torrent/link` must be preserved as a downloadable
  candidate alongside RSS `enclosure.url`; otherwise Mikan feeds with a detail
  page in item `link` and the `.torrent` URL in `torrent/link` become
  incorrectly `unsupported`.
- No-zone ISO feed timestamps such as `2026-04-25T18:39:13.708` are interpreted
  as `Asia/Shanghai` and serialized to RFC3339 in API DTOs.
- Non-regex keyword matching:
  - Ordinary text keywords may match title plus link/download metadata.
  - 1-2 digit pure numeric keywords are episode filters; match only against
    title episode tokens (`05`, `S01E05`, `EP05`, `第 05 集`) and must not match
    URL/hash/date/resolution metadata.
- Regex mode remains an explicit power-user mode and evaluates against the
  combined title + metadata text.
- `.torrent` URL ingestion must download the torrent file in backend, then POST
  multipart form field `torrents` to qBittorrent with `savepath`, `tags`, and
  `paused=false`.
- Project Docker Compose qBittorrent sidecar defaults to internal networking
  plus qBittorrent WebUI subnet auth whitelist. Backend default qBittorrent
  username/password must remain empty so the client skips
  `/api/v2/auth/login`. Do not add non-empty backend credential defaults unless
  the sidecar entrypoint also persists the same WebUI credentials and tests
  cover the full bootstrap path.
- The sidecar entrypoint must patch qBittorrent WebUI auth-bypass settings on
  every start, not only on first config creation, because named Docker volumes
  can preserve configs generated before the current entrypoint. Keep the
  backend Compose service, sidecar entrypoint, and config tests in sync for
  `AuthSubnetWhitelist`, `AuthSubnetWhitelistEnabled`, `LocalHostAuth`,
  `HostHeaderValidation`, `CSRFProtection`, and `SecureCookie`.
- When explicit qBittorrent credentials are configured, login uses
  `POST /api/v2/auth/login` with `application/x-www-form-urlencoded` fields
  named exactly `username` and `password`. Non-200 login responses must remain
  diagnostic, e.g. `qbittorrent login status 401`.
- qBittorrent Web API 401/403 responses from health, add, or status calls must
  wrap `ErrDownloaderAuthFailed` so handlers can return stable
  `DOWNLOADER_AUTH_FAILED` instead of `INTERNAL_ERROR`. Other qBittorrent
  transport/non-2xx availability failures should wrap `ErrDownloaderUnavailable`.
- qBittorrent missing-tag lookup immediately after add is not a user cancel.
  Keep the Yunxia task `pending` instead of mapping it to `canceled`.
- Terminal `failed` / `canceled` task statuses must carry an `error_message`.
- Unattended RSS source refresh must persist observable scheduling state:
  `health_status`, `consecutive_failures`, `last_success_at`,
  `next_refresh_at`, `last_refresh_status`, and `last_refresh_stats`.
  Consecutive source failures degrade/circuit-open the source and stretch the
  next probe; a successful refresh resets the source to `ok`.
- `POST /api/v1/rss/sources/refresh-all` must continue refreshing other enabled
  sources when one source fails, and must report per-source success/failure.
  Manual refresh-all is a force refresh for enabled sources; `skipped` is
  reserved for sources already locked by another refresh.
- RSS subscription preview must evaluate existing items and return explainable
  `matched`, `missing`, and `excluded` keyword lists. Do not return only a
  boolean match result.
- Temporary RSS subscription preview must accept only rule fields and
  `source_id`, validate the source owner and regexes, evaluate existing source
  items, return the same preview shape with `subscription_id=0`, and must not
  create or update a persisted subscription.
- RSS anime title parsing is lightweight and best-effort. When RSS items are
  created or updated from feeds, persist parsed metadata and expose it through
  the item DTO:
  `anime_title`, `season`, `episode`, `subtitle_group`, and `resolution`.
- RSS subscription directory templates are rendered only as safe relative
  subdirectories below `target_virtual_parent_path`. Supported placeholders
  are `{anime_title}`, `{season}`, `{episode}`, `{subtitle_group}`,
  `{resolution}`, and `{title}`. Unknown placeholders, absolute paths,
  backslashes, and `..` must fail validation with `PATH_INVALID`.
- RSS `filename_template` is rendered at RSS enqueue time into the created
  download task's `target_filename` snapshot. Downloaders still write into
  backend-visible staging; the task import pipeline may rename only when there
  is exactly one effective staged file. If the rendered filename has no clear
  extension (last suffix is alphanumeric and contains at least one letter),
  preserve the original staged file extension. Multi-file torrents /
  multi-file staging must keep original relative paths and must not rewrite
  torrent contents.
- RSS item unattended retry state must use clear statuses:
  `retry_pending`, `completed`, and `needs_attention`, plus retry fields
  `retry_count`, `max_retry_count`, `last_attempt_at`, `next_retry_at`, and
  `retry_reason`.
- Automatic retry only applies to transient errors such as downloader
  unavailable, torrent fetch timeout/network errors, and transient task
  failures. Deterministic errors such as unsupported links, invalid paths,
  ACL denial, read-only sources, and missing backing storage must move to
  `needs_attention`.
- Retry backoff is finite and defaults to 5m / 30m / 2h with a maximum of 3
  automatic retries. Manual retry may bypass `next_retry_at` but must still
  record an attempt.
- Normal source refresh and subscription run must not bypass item retry state:
  `retry_pending`, `needs_attention`, and `completed` items are left alone
  unless the user explicitly calls item `reprocess` / `retry`.
- An RSS item with an existing non-terminal task must not enqueue another task.
  Task `completed` must mark the item `completed`; task `failed` / `canceled`
  must write the error back to the item and classify it as `retry_pending` or
  `needs_attention`. User/task cancellation is not treated as transient and
  should move the item to `needs_attention` rather than silently re-queueing.
- Manual `POST /rss/items/:id/download` failures after the item reaches
  `matched` must not leave the item stuck at `matched`. Persist a visible
  failure state (currently `needs_attention`) with `error_message`,
  `retry_reason`, and `last_attempt_at`, and emit the existing
  `rss.item_needs_attention` notification on transition.
- Task terminal backlink must not depend only on periodic RSS workers. When
  `TaskService` changes an RSS-created task to `completed`, `failed`, or
  `canceled` through refresh/sync/cancel paths, it must notify the RSS backlink
  handler immediately so the item state is visible before the next scheduled
  refresh/retry cycle.
- RSS-created tasks should run under the RSS subscription/source owner context,
  even when refresh/retry is triggered by an administrator or background
  worker, so task ownership and VFS/ACL checks stay tied to the owner.
- RSS item batch actions must return per-item results and aggregate
  `succeeded` / `failed` counts. Item-level failures such as not found,
  permission denied, unsupported links, or invalid state must not fail the whole
  batch response.
- RSS subscription clone must authorize the original subscription owner, copy
  rule/template/target snapshot fields, allocate a new ID and timestamps, and
  leave the original subscription unchanged. Optional `name` and `is_enabled`
  only affect the clone; missing `name` should use a stable `Original Copy`
  style default.
- RSS subscription batch state changes must return per-subscription results and
  aggregate `succeeded` / `failed` counts. Each ID is fetched and authorized
  independently; successful items update `is_enabled` and `updated_at`, while
  item-level not-found or permission failures stay inside that result item.
- RSS import/export is configuration-only. Export must include
  `version`, `exported_at`, source `name/url/is_enabled/refresh_interval_seconds`,
  and subscription `source_url/name/is_enabled/must_contain/must_not_contain/use_regex/case_sensitive/target_virtual_parent_path/directory_template/filename_template`.
  It must not export items, tasks, refresh health, retry state, or other runtime
  fields.
- RSS import must return per-source and per-subscription item results with
  `action=create|reuse|skip|failed`, `success`, optional `id`, and optional
  `error_code/error_message`. Source import reuses an existing source by current
  owner + exact URL and must not overwrite that source. `dry_run` validates and
  previews all actions without persisting new sources or subscriptions.
- RSS import subscriptions resolve `source_url` (or legacy `source_ref`) through
  imported/reused sources and must revalidate `target_virtual_parent_path`
  writability with the same path normalization/write checks as subscription
  create/update. Item-level invalid source, regex, template, path, ACL, or
  read-only failures stay in the import response and do not fail the whole
  HTTP request.
- Batch ignore must refuse completed items and items with active non-terminal
  tasks using per-item `TASK_INVALID_STATE`; successful ignores set
  `status=ignored`, clear `error_message`, `retry_reason`, and `next_retry_at`,
  and avoid destructive task linkage cleanup.
- Batch retry must reuse the single-item manual retry semantics per item,
  including optional `subscription_id`, owner context, duplicate-active-task
  protection, retry attempt accounting, and per-item error codes.
- RSS source failure notifications are emitted only when health transitions into
  `degraded` or `circuit_open`; RSS item notifications are emitted when items
  transition into `needs_attention`; completed RSS task backlinks emit
  `rss.download_completed`.

### 4. Validation & Error Matrix

| Condition | Behavior |
|---|---|
| Mikan item lacks top-level `pubDate` but has `torrent/pubDate` | Parse and return non-null `published_at` |
| Mikan item has detail page in item `link` and `.torrent` in `torrent/link` | Resolve the `.torrent` link as the download URL |
| Feed timestamp is no-zone ISO | Parse in `Asia/Shanghai` |
| `must_contain=["05","1080p"]`, title episode is 05 | Match |
| `must_contain=["05","1080p"]`, title episode is 02 but URL/hash/date contains 05 | Do not match |
| `.torrent` URL fetch returns non-2xx | Return add error; do not create a false qBittorrent task |
| `.torrent` file exceeds `maxTorrentFileBytes` | Return add error |
| Compose sidecar uses default blank `YUNXIA_QBITTORRENT_USERNAME/PASSWORD` | Backend config resolves blank credentials and skips qBittorrent login |
| Existing qBittorrent config volume lacks WebUI auth whitelist | Entrypoint patches the config on startup before launching qBittorrent |
| Explicit qBittorrent credentials return login 401 | Health returns `status=unavailable` with diagnostic `error` containing `qbittorrent login status 401` |
| Empty credentials but `/api/v2/app/version` returns 401 | Health returns `status=unavailable` with diagnostic `error` containing `qbittorrent health status 401`; direct task/RSS enqueue maps to `DOWNLOADER_AUTH_FAILED`, not `INTERNAL_ERROR` |
| qBittorrent sidecar auth bootstrap changes | Update `docker-compose.backend.yml`, `backend/docker/qbittorrent.entrypoint.sh`, backend config defaults, API/deploy docs, and static consistency tests together |
| qBittorrent tag not visible immediately | Return `pending`, not `canceled` |
| qBittorrent state is `missingFiles` / `error` | Return `failed` with state in `error_message` |
| Source refresh fails repeatedly | Increment `consecutive_failures`, update `next_refresh_at`, enter `degraded` / `circuit_open` |
| Source refresh succeeds after failures | Clear `last_error`, reset failures, set `health_status=ok` |
| One source in refresh-all fails | Continue other sources and include per-source error |
| Enabled source has future `next_refresh_at` but refresh-all is called | Refresh it anyway |
| Preview sees must keyword missing | Return `result=missing` with missing keyword list |
| Preview sees must-not keyword matched | Return `result=excluded` with excluded keyword list |
| Temporary preview receives valid rules | Return `subscription_id=0`; do not persist a subscription |
| Temporary preview receives invalid regex | Return `RSS_REGEX_INVALID`; do not persist a subscription |
| RSS title contains common subtitle-group / SxxEyy / resolution tokens | Persist parsed anime metadata and return it in item DTO |
| Subscription `directory_template=""` | Preserve old behavior and enqueue into `target_virtual_parent_path` |
| Subscription `directory_template="{anime_title}/{season}"` | Enqueue into a safe child path below `target_virtual_parent_path` |
| Subscription template has unknown placeholder / absolute path / `..` / backslash | Reject with `PATH_INVALID` |
| `filename_template` is set and the completed task has one staged file | Render to task `target_filename`; import the file under that name, preserving the original extension when the template lacks a clear one |
| `filename_template` is set and the completed task has multiple staged files | Keep original staged relative paths; do not apply task `target_filename` |
| RSS item has active non-terminal task | Do not create another task during retry/reprocess |
| RSS item is `retry_pending` / `needs_attention` / `completed` during source refresh | Do not create another task |
| Admin refreshes another user's RSS source | Created download task uses the source/subscription owner's user context |
| Task completed for RSS item | Mark item `completed` |
| Task failed with transient error | Mark item `retry_pending` until max retries, then `needs_attention` |
| Task canceled for RSS item | Mark item `needs_attention` with the cancellation reason |
| Manual item download enqueue fails after matching | Mark item `needs_attention` with the downstream error message visible in `RSSItemView.error_message` |
| User cancels an RSS-linked task through task API | Immediately update the linked RSS item to `needs_attention`; do not wait for the next RSS worker tick |
| Deterministic item failure | Mark item `needs_attention`; do not auto retry |
| Subscription clone omits name/is_enabled | Create a new subscription named `Original Copy` (or simple numbered variant) and preserve the original enabled state |
| Subscription clone provides name/is_enabled | Apply the overrides only to the clone and do not mutate the source subscription |
| Batch subscription state mixes owned, missing, and unauthorized IDs | Return HTTP success with per-subscription success/failure, `RSS_SUBSCRIPTION_NOT_FOUND`, or `PERMISSION_DENIED` as applicable |
| RSS export has sources with runtime refresh state | Export only portable config fields; omit health, refresh, retry, items, and tasks |
| RSS import receives an existing owner+URL source | Return source item `action=reuse`; do not overwrite existing source fields |
| RSS import runs with `dry_run=true` | Validate and return would-create/reuse/fail results without creating sources or subscriptions |
| RSS import subscription target is invalid/unwritable | Return subscription item `action=failed` with `PATH_INVALID`, `NO_BACKING_STORAGE`, `SOURCE_READ_ONLY`, or `PERMISSION_DENIED`; continue other items |
| Batch ignore mixes mutable and invalid-state items | Return HTTP success with per-item success/failure and `TASK_INVALID_STATE` for completed/active-task items |
| Batch retry mixes retryable and unsupported items | Retry eligible items and return per-item `DOWNLOAD_LINK_UNSUPPORTED` for unsupported items |

### 5. Good/Base/Bad Cases

- Good: Mikan RSS item with `torrent/pubDate` and `torrent/link` or enclosure
  `.torrent` produces `published_at`, matches only the requested episode,
  uploads the torrent file to qBittorrent, and stays pending/running until
  qBittorrent reports progress.
- Base: magnet link uses qBittorrent multipart `urls` field and tag tracking.
- Base: Docker Compose qBittorrent sidecar uses auth whitelist and backend
  blank credentials, so health checks call `/api/v2/app/version` directly.
- Bad: treating `"05"` as a raw substring over title + URL + torrent hash,
  causing unrelated episodes to enqueue.
- Bad: submitting a `.torrent` URL as `urls` and assuming qBittorrent will
  always fetch it synchronously.
- Bad: setting backend default qBittorrent credentials to `admin/adminadmin`
  while Compose leaves `YUNXIA_QBITTORRENT_USERNAME/PASSWORD` empty; Viper treats
  empty env vars as unset and falls back to the non-empty defaults, causing an
  unintended login against the auth-whitelisted sidecar.
- Bad: mapping "tag not found" to `canceled` on the first status poll.

### 6. Tests Required

- RSS parser test with Mikan-style `torrent/pubDate` and no timezone.
- RSS parser test asserting Mikan `torrent/link` is surfaced as a download
  candidate.
- RSS matcher tests for short numeric episode filters:
  - positive `05`
  - negative `02` with URL/hash/date containing `05`
  - explicit `SxxEyy` / `EPyy` / `第 yy 集`
- qBittorrent client tests:
  - empty credentials skip login
  - explicit credentials POST `/api/v2/auth/login` with `username` and
    `password` form fields
  - login 401 is preserved as a diagnostic health error
  - health/app version 401 and torrents/add 401 wrap `ErrDownloaderAuthFailed`
    and map to stable API errors
  - magnet uses multipart form fields
  - `.torrent` URL is fetched then uploaded as multipart `torrents`
  - missing tag stays `pending`
  - failed qBittorrent state returns `error_message`
- Config/deploy tests:
  - backend default qBittorrent `api_url` matches Compose sidecar
  - backend default qBittorrent username/password are blank
  - Compose default `YUNXIA_QBITTORRENT_USERNAME/PASSWORD` stay blank
  - sidecar entrypoint keeps WebUI subnet auth whitelist enabled
  - sidecar entrypoint idempotently patches existing config volumes
- Task service test: terminal `failed` / `canceled` has `error_message`.
- RSS unattended tests:
  - source failure backoff and successful recovery
  - refresh-all continues after one source fails
  - subscription preview explains matched/missing/excluded
  - anime title parser extracts common Mikan/subtitle-group metadata
  - subscription template fields persist and are returned in DTOs
  - filename_template renders task target_filename during RSS enqueue
  - task import applies target_filename only for single-file staging and preserves multi-file paths
  - directory template rendering is path-safe and preserves old empty-template
    behavior
  - manual item retry and reprocess endpoints/service paths
  - manual item download failure persists `needs_attention` and
    `error_message`
  - automatic retry backoff and max-attempt `needs_attention`
  - task terminal status writes back to RSS item
  - canceling an RSS-linked task through TaskService immediately writes back the
    RSS item as `needs_attention`
  - active task prevents duplicate enqueue
  - temporary subscription preview does not persist a subscription
  - subscription clone preserves rules/templates/target snapshots without mutating the original
  - batch subscription state returns partial success/failure and updates only authorized items
  - export DTO/json omits runtime refresh, item, task, and retry fields
  - import `dry_run` does not persist sources or subscriptions
  - import reuses an existing same-owner URL source and creates subscriptions against it
  - import subscription invalid/unwritable target returns an item-level failure
  - batch ignore returns partial success/failure and preserves active/completed items
  - batch retry returns partial success/failure while reusing manual retry behavior
- Full gate: `go test -count=1 ./...`.

### 7. Wrong vs Correct

#### Wrong

```go
text := item.Title + " " + item.Link + " " + item.DownloadURL
return strings.Contains(text, "05")
```

This incorrectly matches episode 02 when a URL, date, or torrent hash contains
`05`.

#### Correct

```go
if isShortNumericRSSKeyword(pattern) {
    return matchShortNumericRSSKeyword(item.Title, pattern), nil
}
```

Short numeric keywords are episode filters and only inspect title episode
tokens.

#### Wrong

```go
form.Set("urls", torrentURL)
postForm("/api/v2/torrents/add", form)
```

This depends on qBittorrent asynchronously fetching a third-party `.torrent`
URL and can leave Yunxia unable to observe the torrent.

#### Correct

```go
torrentFile := fetchTorrentFile(ctx, torrentURL)
postMultipart("/api/v2/torrents/add", fields, []multipartUploadFile{torrentFile})
```

Yunxia fetches the torrent file first and uploads it directly to qBittorrent.

#### Wrong

```go
v.SetDefault("qbittorrent.username", "admin")
v.SetDefault("qbittorrent.password", "adminadmin")
```

This can make Docker Compose's intentionally blank
`YUNXIA_QBITTORRENT_USERNAME/PASSWORD` fall back to non-empty Viper defaults,
forcing an unnecessary login against the auth-whitelisted sidecar.

#### Correct

```go
v.SetDefault("qbittorrent.username", "")
v.SetDefault("qbittorrent.password", "")
```

Blank defaults match the project sidecar. Deployers who use an authenticated
external qBittorrent instance opt in with explicit environment variables.
