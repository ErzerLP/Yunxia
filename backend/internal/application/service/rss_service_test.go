package service

import (
	"testing"

	"yunxia/internal/domain/entity"
)

func TestClassifyDownloadLinkRoutesBTToQBittorrentTypes(t *testing.T) {
	tests := []struct {
		name string
		link string
		want string
	}{
		{name: "magnet", link: "magnet:?xt=urn:btih:abcdef", want: RSSLinkTypeMagnet},
		{name: "torrent", link: "https://example.com/show.torrent?passkey=abc", want: RSSLinkTypeTorrent},
		{name: "http", link: "https://example.com/archive.zip", want: RSSLinkTypeHTTP},
		{name: "unsupported", link: "ftp://example.com/show.torrent", want: RSSLinkTypeUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyDownloadLink(tt.link); got != tt.want {
				t.Fatalf("ClassifyDownloadLink() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveRSSDownloadLinkPrefersBTLinks(t *testing.T) {
	gotURL, gotType := resolveRSSDownloadLink(RSSFetchedItem{
		Title: "Example",
		Link:  "https://example.com/detail/1",
		Enclosures: []string{
			"https://example.com/files/show.torrent",
			"https://example.com/files/show.mp4",
		},
	})
	if gotURL != "https://example.com/files/show.torrent" || gotType != RSSLinkTypeTorrent {
		t.Fatalf("resolveRSSDownloadLink() = (%q,%q)", gotURL, gotType)
	}
}

func TestRSSSubscriptionMatchesItemSupportsRules(t *testing.T) {
	subscription := &entity.RSSSubscription{
		MustContain:    []string{"[1080p]", "frieren"},
		MustNotContain: []string{"CHT"},
		CaseSensitive:  false,
	}
	matched := rssSubscriptionMatchesItem(subscription, &entity.RSSItem{
		Title:       "[1080P] Sousou no Frieren - 01 [CHS]",
		DownloadURL: "magnet:?xt=urn:btih:abcdef",
	})
	if !matched {
		t.Fatalf("expected item to match subscription")
	}

	subscription.MustNotContain = []string{"CHS"}
	matched = rssSubscriptionMatchesItem(subscription, &entity.RSSItem{
		Title:       "[1080P] Sousou no Frieren - 01 [CHS]",
		DownloadURL: "magnet:?xt=urn:btih:abcdef",
	})
	if matched {
		t.Fatalf("expected must_not_contain to reject item")
	}
}

func TestValidateRSSRulePatternsRejectsInvalidRegex(t *testing.T) {
	if err := validateRSSRulePatterns([]string{"["}, nil, true); err == nil {
		t.Fatalf("expected invalid regex error")
	}
	if err := validateRSSRulePatterns([]string{"["}, nil, false); err != nil {
		t.Fatalf("non-regex rule should not compile regexp, got %v", err)
	}
}
