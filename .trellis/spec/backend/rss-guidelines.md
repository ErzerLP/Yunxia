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
