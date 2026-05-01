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
func (c *QBittorrentClient) AddURI(ctx context.Context, uri string, dir string) (string, error)
func (c *QBittorrentClient) TellStatus(ctx context.Context, externalID string) (*service.DownloadStatus, error)
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
- RSS-created tasks should run under the RSS subscription/source owner context,
  even when refresh/retry is triggered by an administrator or background
  worker, so task ownership and VFS/ACL checks stay tied to the owner.

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
| qBittorrent tag not visible immediately | Return `pending`, not `canceled` |
| qBittorrent state is `missingFiles` / `error` | Return `failed` with state in `error_message` |
| Source refresh fails repeatedly | Increment `consecutive_failures`, update `next_refresh_at`, enter `degraded` / `circuit_open` |
| Source refresh succeeds after failures | Clear `last_error`, reset failures, set `health_status=ok` |
| One source in refresh-all fails | Continue other sources and include per-source error |
| Enabled source has future `next_refresh_at` but refresh-all is called | Refresh it anyway |
| Preview sees must keyword missing | Return `result=missing` with missing keyword list |
| Preview sees must-not keyword matched | Return `result=excluded` with excluded keyword list |
| RSS item has active non-terminal task | Do not create another task during retry/reprocess |
| RSS item is `retry_pending` / `needs_attention` / `completed` during source refresh | Do not create another task |
| Admin refreshes another user's RSS source | Created download task uses the source/subscription owner's user context |
| Task completed for RSS item | Mark item `completed` |
| Task failed with transient error | Mark item `retry_pending` until max retries, then `needs_attention` |
| Task canceled for RSS item | Mark item `needs_attention` with the cancellation reason |
| Deterministic item failure | Mark item `needs_attention`; do not auto retry |

### 5. Good/Base/Bad Cases

- Good: Mikan RSS item with `torrent/pubDate` and `torrent/link` or enclosure
  `.torrent` produces `published_at`, matches only the requested episode,
  uploads the torrent file to qBittorrent, and stays pending/running until
  qBittorrent reports progress.
- Base: magnet link uses qBittorrent multipart `urls` field and tag tracking.
- Bad: treating `"05"` as a raw substring over title + URL + torrent hash,
  causing unrelated episodes to enqueue.
- Bad: submitting a `.torrent` URL as `urls` and assuming qBittorrent will
  always fetch it synchronously.
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
  - magnet uses multipart form fields
  - `.torrent` URL is fetched then uploaded as multipart `torrents`
  - missing tag stays `pending`
  - failed qBittorrent state returns `error_message`
- Task service test: terminal `failed` / `canceled` has `error_message`.
- RSS unattended tests:
  - source failure backoff and successful recovery
  - refresh-all continues after one source fails
  - subscription preview explains matched/missing/excluded
  - manual item retry and reprocess endpoints/service paths
  - automatic retry backoff and max-attempt `needs_attention`
  - task terminal status writes back to RSS item
  - active task prevents duplicate enqueue
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
