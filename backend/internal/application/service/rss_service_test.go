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

func TestRSSSubscriptionShortNumericKeywordMatchesEpisodeInTitleOnly(t *testing.T) {
	subscription := &entity.RSSSubscription{
		MustContain:   []string{"05", "1080p"},
		CaseSensitive: false,
	}

	if !rssSubscriptionMatchesItem(subscription, &entity.RSSItem{
		Title:       "[SubsPlease] Example Show - 05 [1080p]",
		Link:        "https://mikanani.kas.pub/Home/Episode/2",
		DownloadURL: "https://mikanani.kas.pub/Download/abcdef.torrent",
	}) {
		t.Fatalf("expected short numeric keyword to match episode number in title")
	}

	if rssSubscriptionMatchesItem(subscription, &entity.RSSItem{
		Title:       "[SubsPlease] Example Show - 02 [1080p]",
		Link:        "https://mikanani.kas.pub/Home/Episode/2?published=2026-04-25",
		DownloadURL: "https://mikanani.kas.pub/Download/abcdef05abcdef.torrent",
	}) {
		t.Fatalf("short numeric keyword should not match URL/hash metadata")
	}
}

func TestRSSSubscriptionShortNumericKeywordIgnoresDatesAndResolution(t *testing.T) {
	subscription := &entity.RSSSubscription{
		MustContain:   []string{"05", "1080p"},
		CaseSensitive: false,
	}

	if rssSubscriptionMatchesItem(subscription, &entity.RSSItem{
		Title:       "[2026.05.02] Example Show - 02 [1080p]",
		DownloadURL: "magnet:?xt=urn:btih:abcdef",
	}) {
		t.Fatalf("short numeric keyword should not match date metadata")
	}

	if !rssSubscriptionMatchesItem(subscription, &entity.RSSItem{
		Title:       "[SubsPlease] Example Show S01E05 [1080p]",
		DownloadURL: "magnet:?xt=urn:btih:abcdef",
	}) {
		t.Fatalf("expected short numeric keyword to match explicit SxxExx episode marker")
	}
}

func TestRSSSubscriptionShortNumericKeywordMatchesExplicitEpisodeForms(t *testing.T) {
	subscription := &entity.RSSSubscription{
		MustContain:   []string{"05"},
		CaseSensitive: false,
	}
	titles := []string{
		"[SubsPlease] Example Show S01E05 [1080p]",
		"[SubsPlease] Example Show EP05 [1080p]",
		"[SubsPlease] Example Show Episode 05 [1080p]",
		"[字幕组] 示例番 第 05 集 [1080p]",
		"[字幕组] 示例番 第05话 [1080p]",
	}
	for _, title := range titles {
		t.Run(title, func(t *testing.T) {
			if !rssSubscriptionMatchesItem(subscription, &entity.RSSItem{Title: title}) {
				t.Fatalf("expected %q to match episode 05", title)
			}
		})
	}
}

func TestRSSSubscriptionShortNumericKeywordIgnoresTitleHash(t *testing.T) {
	subscription := &entity.RSSSubscription{
		MustContain:   []string{"05", "1080p"},
		CaseSensitive: false,
	}
	if rssSubscriptionMatchesItem(subscription, &entity.RSSItem{
		Title:       "[ABC05DEF] Example Show - 02 [1080p]",
		DownloadURL: "magnet:?xt=urn:btih:abcdef",
	}) {
		t.Fatalf("short numeric keyword should not match hash-like title token")
	}
}

func TestRSSSubscriptionNormalTextKeywordCanStillMatchMetadata(t *testing.T) {
	subscription := &entity.RSSSubscription{
		MustContain:   []string{"mikanani"},
		CaseSensitive: false,
	}

	if !rssSubscriptionMatchesItem(subscription, &entity.RSSItem{
		Title:       "Example Show - 02",
		DownloadURL: "https://mikanani.kas.pub/Download/abcdef.torrent",
	}) {
		t.Fatalf("expected ordinary text keyword to remain usable against metadata")
	}
}

func TestRSSSubscriptionRegexModeCanStillMatchMetadata(t *testing.T) {
	subscription := &entity.RSSSubscription{
		MustContain: []string{"abcdef05"},
		UseRegex:    true,
	}
	if !rssSubscriptionMatchesItem(subscription, &entity.RSSItem{
		Title:       "Example Show - 02",
		DownloadURL: "https://mikanani.kas.pub/Download/abcdef05abcdef.torrent",
	}) {
		t.Fatalf("expected regex mode to keep matching combined metadata text")
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
