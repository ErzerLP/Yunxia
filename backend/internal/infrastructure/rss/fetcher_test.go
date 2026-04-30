package rss

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetcherParsesMikanTorrentPubDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = writer.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>[Mikan] Example - 05 [1080p]</title>
      <link>https://mikanani.kas.pub/Home/Episode/123</link>
      <guid>mikan-episode-123</guid>
      <torrent xmlns="https://mikanani.kas.pub/0.1/">
        <link>https://mikanani.kas.pub/Download/123.torrent</link>
        <pubDate>2026-04-29T20:15:30.953</pubDate>
      </torrent>
    </item>
  </channel>
</rss>`))
	}))
	defer server.Close()

	items, err := NewFetcher().Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].PublishedAt == nil {
		t.Fatalf("expected Mikan torrent pubDate to be parsed")
	}
	if got := items[0].PublishedAt.Format("2006-01-02T15:04:05.000-07:00"); got != "2026-04-29T20:15:30.953+08:00" {
		t.Fatalf("unexpected published_at %s", got)
	}
	if len(items[0].Enclosures) != 1 || items[0].Enclosures[0] != "https://mikanani.kas.pub/Download/123.torrent" {
		t.Fatalf("expected Mikan torrent link to be exposed as downloadable enclosure, got %#v", items[0].Enclosures)
	}
}

func TestParseFeedTimeKeepsRFC1123ZSupport(t *testing.T) {
	got := parseFeedTime("Wed, 29 Apr 2026 12:15:30 +0000")
	if got == nil {
		t.Fatalf("expected RFC1123Z time to parse")
	}
	if got.UTC().Format("2006-01-02T15:04:05Z07:00") != "2026-04-29T12:15:30Z" {
		t.Fatalf("unexpected parsed time %s", got.UTC().Format("2006-01-02T15:04:05Z07:00"))
	}
}
