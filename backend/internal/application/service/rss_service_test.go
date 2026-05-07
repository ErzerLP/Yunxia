package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
	gormrepo "yunxia/internal/infrastructure/persistence/gorm"
	"yunxia/internal/infrastructure/security"
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

func TestParseRSSAnimeTitleExtractsCommonMikanFields(t *testing.T) {
	tests := []struct {
		name          string
		title         string
		animeTitle    string
		season        string
		episode       string
		subtitleGroup string
		resolution    string
	}{
		{
			name:          "dash episode",
			title:         "[SubsPlease] Sousou no Frieren - 05 [1080p]",
			animeTitle:    "Sousou no Frieren",
			episode:       "05",
			subtitleGroup: "SubsPlease",
			resolution:    "1080p",
		},
		{
			name:          "sxxeyy",
			title:         "[ANi] Summer Pockets S02E03 [1080P][Baha][WEB-DL][CHT]",
			animeTitle:    "Summer Pockets",
			season:        "S02",
			episode:       "03",
			subtitleGroup: "ANi",
			resolution:    "1080p",
		},
		{
			name:          "mikan bracket title",
			title:         "【喵萌奶茶屋】★04月新番★[夏日口袋/Summer Pockets][04][1080p][简日双语]",
			animeTitle:    "夏日口袋/Summer Pockets",
			episode:       "04",
			subtitleGroup: "喵萌奶茶屋",
			resolution:    "1080p",
		},
		{
			name:          "season word",
			title:         "[Nekomoe kissaten&LoliHouse] Shoushimin Series 2nd Season - 03 [WebRip 1080p HEVC-10bit AAC][CHS]",
			animeTitle:    "Shoushimin Series 2nd Season",
			season:        "S02",
			episode:       "03",
			subtitleGroup: "Nekomoe kissaten&LoliHouse",
			resolution:    "1080p",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRSSAnimeTitle(tt.title)
			if got.AnimeTitle != tt.animeTitle || got.Season != tt.season || got.Episode != tt.episode || got.SubtitleGroup != tt.subtitleGroup || got.Resolution != tt.resolution {
				t.Fatalf("parseRSSAnimeTitle() = %#v", got)
			}
		})
	}
}

func TestRSSDirectoryTemplateRenderingIsSafe(t *testing.T) {
	item := &entity.RSSItem{Title: "[SubsPlease] Example/Show S01E05 [1080p]"}
	item.Parsed = parseRSSAnimeTitle(item.Title)
	got, err := renderRSSTargetVirtualParentPath(&entity.RSSSubscription{
		TargetVirtualParentPath: "/anime",
		DirectoryTemplate:       "{anime_title}/{season}/{episode}",
	}, item)
	if err != nil {
		t.Fatalf("renderRSSTargetVirtualParentPath() error = %v", err)
	}
	if got != "/anime/Example Show/S01/05" {
		t.Fatalf("target path = %q", got)
	}

	sanitized, err := renderRSSDirectoryTemplate("{title}", &entity.RSSItem{Title: "A/../B\\C"})
	if err != nil {
		t.Fatalf("renderRSSDirectoryTemplate() should sanitize placeholder values: %v", err)
	}
	if strings.Contains(sanitized, "..") || strings.ContainsAny(sanitized, `/\`) {
		t.Fatalf("sanitized path is unsafe: %q", sanitized)
	}

	for _, template := range []string{
		"../{anime_title}",
		"/abs/{anime_title}",
		"bad\\{title}",
		"{bad1}",
		"{anime-title}",
		"{AnimeTitle}",
		"{unknown_placeholder}",
		"C:/anime",
		"D:/anime",
	} {
		t.Run(template, func(t *testing.T) {
			if err := validateRSSDirectoryTemplate(template); !errors.Is(err, ErrPathInvalid) {
				t.Fatalf("expected ErrPathInvalid for %q, got %v", template, err)
			}
		})
	}

	for _, template := range []string{
		"{anime_title}-{bad}",
		"{bad1}",
		"{anime-title}",
		"{AnimeTitle}",
		"C:/anime",
		"D:/anime",
		"C:anime",
	} {
		t.Run("filename_"+template, func(t *testing.T) {
			if err := validateRSSFilenameTemplate(template); !errors.Is(err, ErrPathInvalid) {
				t.Fatalf("expected ErrPathInvalid for %q, got %v", template, err)
			}
		})
	}
}

func TestRSSSubscriptionTemplateFieldsPersistInResponses(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	svc := NewRSSService(repo, nil, nil, WithRSSVFSResolver(fakeRSSVFSResolver{}), WithRSSNow(func() time.Time { return now }))
	ctx := rssTestAuthContext()

	created, err := svc.CreateSubscription(ctx, appdto.RSSSubscriptionUpsertRequest{
		SourceID:                source.ID,
		Name:                    "anime",
		IsEnabled:               boolPtr(true),
		MustContain:             []string{"Example"},
		TargetVirtualParentPath: "/anime",
		DirectoryTemplate:       "{anime_title}/{season}",
		FilenameTemplate:        "{anime_title} - {episode} [{resolution}]",
	})
	if err != nil {
		t.Fatalf("CreateSubscription failed: %v", err)
	}
	if created.DirectoryTemplate != "{anime_title}/{season}" || created.FilenameTemplate != "{anime_title} - {episode} [{resolution}]" {
		t.Fatalf("created templates = %#v", created)
	}

	detail, err := svc.GetSubscription(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSubscription failed: %v", err)
	}
	list, err := svc.ListSubscriptions(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListSubscriptions failed: %v", err)
	}
	if detail.DirectoryTemplate != created.DirectoryTemplate || len(list.Items) != 1 || list.Items[0].FilenameTemplate != created.FilenameTemplate {
		t.Fatalf("templates not returned in detail/list: detail=%#v list=%#v", detail, list.Items)
	}

	updated, err := svc.UpdateSubscription(ctx, created.ID, appdto.RSSSubscriptionUpsertRequest{
		SourceID:                source.ID,
		Name:                    "anime updated",
		IsEnabled:               boolPtr(true),
		TargetVirtualParentPath: "/anime",
		DirectoryTemplate:       "{subtitle_group}/{anime_title}",
		FilenameTemplate:        "{title}",
	})
	if err != nil {
		t.Fatalf("UpdateSubscription failed: %v", err)
	}
	if updated.DirectoryTemplate != "{subtitle_group}/{anime_title}" || updated.FilenameTemplate != "{title}" {
		t.Fatalf("updated templates = %#v", updated)
	}
}

func TestRSSExportConfigExcludesRuntimeFields(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	lastRefreshed := now.Add(-time.Hour)
	lastError := "temporary error"
	source := repo.mustCreateSource(&entity.RSSSource{
		UserID:                 1,
		Name:                   "mikan",
		URL:                    "https://mikan.example/rss.xml",
		IsEnabled:              true,
		RefreshIntervalSeconds: 600,
		LastRefreshedAt:        &lastRefreshed,
		LastError:              &lastError,
		HealthStatus:           RSSSourceHealthDegraded,
		ConsecutiveFailures:    2,
		LastRefreshStatus:      RSSRefreshStatusFailed,
		LastRefreshStatsJSON:   `{"failed":1}`,
		CreatedAt:              now.Add(-2 * time.Hour),
		UpdatedAt:              now.Add(-time.Hour),
	})
	repo.mustCreateSubscription(&entity.RSSSubscription{
		UserID:                  1,
		SourceID:                source.ID,
		Name:                    "Frieren",
		IsEnabled:               true,
		MustContain:             []string{"Frieren", "1080p"},
		MustNotContain:          []string{"CHT"},
		TargetVirtualParentPath: "/anime",
		DirectoryTemplate:       "{anime_title}/{season}",
		FilenameTemplate:        "{anime_title} - {episode}",
		ResolvedSourceID:        7,
		ResolvedInnerParentPath: "/anime",
		CreatedAt:               now.Add(-2 * time.Hour),
		UpdatedAt:               now.Add(-time.Hour),
	})
	svc := NewRSSService(repo, nil, nil, WithRSSNow(func() time.Time { return now }))

	resp, err := svc.ExportConfig(rssTestAuthContext())
	if err != nil {
		t.Fatalf("ExportConfig failed: %v", err)
	}
	if resp.Version != rssExportVersion || resp.ExportedAt != now.Format(time.RFC3339) {
		t.Fatalf("export metadata = %#v", resp)
	}
	if len(resp.Sources) != 1 || len(resp.Subscriptions) != 1 {
		t.Fatalf("export counts = sources %d subscriptions %d", len(resp.Sources), len(resp.Subscriptions))
	}
	if resp.Sources[0].Name != "mikan" || resp.Subscriptions[0].SourceURL != source.URL {
		t.Fatalf("exported config = %#v", resp)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal export failed: %v", err)
	}
	for _, forbidden := range []string{"last_error", "health_status", "next_refresh_at", "last_refresh_stats", "task", "retry", "resolved_source_id"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("export contains runtime field %q: %s", forbidden, raw)
		}
	}
}

func TestRSSImportConfigDryRunDoesNotPersist(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 5, 2, 12, 30, 0, 0, time.UTC)
	svc := NewRSSService(repo, nil, nil, WithRSSVFSResolver(fakeRSSVFSResolver{}), WithRSSNow(func() time.Time { return now }))

	resp, err := svc.ImportConfig(rssTestAuthContext(), appdto.RSSImportRequest{
		DryRun: true,
		Sources: []appdto.RSSImportSource{{
			Name:                   "mikan",
			URL:                    "https://mikan.example/rss.xml",
			IsEnabled:              boolPtr(true),
			RefreshIntervalSeconds: 600,
		}},
		Subscriptions: []appdto.RSSImportSubscription{{
			SourceURL:               "https://mikan.example/rss.xml",
			Name:                    "Frieren",
			IsEnabled:               boolPtr(true),
			MustContain:             []string{"Frieren"},
			TargetVirtualParentPath: "/anime",
			DirectoryTemplate:       "{anime_title}",
		}},
	})
	if err != nil {
		t.Fatalf("ImportConfig dry-run failed: %v", err)
	}
	if !resp.DryRun || resp.Sources.Created != 1 || resp.Subscriptions.Created != 1 || resp.Sources.Failed != 0 || resp.Subscriptions.Failed != 0 {
		t.Fatalf("dry-run response = %#v", resp)
	}
	if len(repo.sources) != 0 || len(repo.subscriptions) != 0 {
		t.Fatalf("dry-run persisted data: sources=%d subscriptions=%d", len(repo.sources), len(repo.subscriptions))
	}
}

func TestRSSImportConfigReusesSameURLSourceAndCreatesSubscription(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 5, 2, 13, 0, 0, 0, time.UTC)
	existing := repo.mustCreateSource(&entity.RSSSource{
		UserID:                 1,
		Name:                   "existing",
		URL:                    "https://mikan.example/rss.xml",
		IsEnabled:              true,
		RefreshIntervalSeconds: 300,
		CreatedAt:              now.Add(-time.Hour),
		UpdatedAt:              now.Add(-time.Hour),
	})
	svc := NewRSSService(repo, nil, nil, WithRSSVFSResolver(fakeRSSVFSResolver{}), WithRSSNow(func() time.Time { return now }))

	resp, err := svc.ImportConfig(rssTestAuthContext(), appdto.RSSImportRequest{
		Sources: []appdto.RSSImportSource{{
			Name:                   "incoming",
			URL:                    existing.URL,
			IsEnabled:              boolPtr(false),
			RefreshIntervalSeconds: 900,
		}},
		Subscriptions: []appdto.RSSImportSubscription{{
			SourceURL:               existing.URL,
			Name:                    "Frieren",
			IsEnabled:               boolPtr(false),
			MustContain:             []string{"Frieren", "1080p"},
			MustNotContain:          []string{"CHT"},
			TargetVirtualParentPath: "/anime",
			DirectoryTemplate:       "{anime_title}/{season}",
			FilenameTemplate:        "{anime_title} - {episode}",
		}},
	})
	if err != nil {
		t.Fatalf("ImportConfig failed: %v", err)
	}
	if resp.Sources.Reused != 1 || resp.Sources.Created != 0 || resp.Subscriptions.Created != 1 || resp.Subscriptions.Failed != 0 {
		t.Fatalf("import response = %#v", resp)
	}
	storedSource, _ := repo.FindSourceByID(context.Background(), existing.ID)
	if storedSource.Name != "existing" || !storedSource.IsEnabled || storedSource.RefreshIntervalSeconds != 300 {
		t.Fatalf("existing source was overwritten: %#v", storedSource)
	}
	if len(repo.subscriptions) != 1 {
		t.Fatalf("subscription count = %d", len(repo.subscriptions))
	}
	var storedSub *entity.RSSSubscription
	for _, subscription := range repo.subscriptions {
		storedSub = subscription
	}
	if storedSub.SourceID != existing.ID || storedSub.UserID != 1 || storedSub.Name != "Frieren" || storedSub.IsEnabled {
		t.Fatalf("created subscription = %#v", storedSub)
	}
	if storedSub.TargetVirtualParentPath != "/anime" || storedSub.ResolvedSourceID != 7 || storedSub.DirectoryTemplate != "{anime_title}/{season}" {
		t.Fatalf("created subscription target/template = %#v", storedSub)
	}
}

func TestRSSImportConfigInvalidTargetFailsSubscriptionItem(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 5, 2, 13, 30, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{
		UserID:                 1,
		Name:                   "mikan",
		URL:                    "https://mikan.example/rss.xml",
		IsEnabled:              true,
		RefreshIntervalSeconds: 300,
		CreatedAt:              now,
		UpdatedAt:              now,
	})
	svc := NewRSSService(repo, nil, nil, WithRSSVFSResolver(fakeRSSFailingVFSResolver{err: ErrPathInvalid}), WithRSSNow(func() time.Time { return now }))

	resp, err := svc.ImportConfig(rssTestAuthContext(), appdto.RSSImportRequest{
		DryRun: true,
		Subscriptions: []appdto.RSSImportSubscription{{
			SourceURL:               source.URL,
			Name:                    "bad target",
			IsEnabled:               boolPtr(true),
			TargetVirtualParentPath: "../escape",
		}},
	})
	if err != nil {
		t.Fatalf("ImportConfig should return partial response, got error: %v", err)
	}
	if resp.Subscriptions.Failed != 1 || len(resp.Subscriptions.Items) != 1 {
		t.Fatalf("invalid target response = %#v", resp)
	}
	item := resp.Subscriptions.Items[0]
	if item.Success || item.ErrorCode == nil || *item.ErrorCode != "PATH_INVALID" {
		t.Fatalf("invalid target item = %#v", item)
	}
	if len(repo.subscriptions) != 0 {
		t.Fatalf("invalid subscription persisted: %d", len(repo.subscriptions))
	}
}

func TestRSSCloneSubscriptionCopiesFieldsAndSupportsOverrides(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 5, 2, 10, 30, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	original := repo.mustCreateSubscription(&entity.RSSSubscription{
		UserID:                  1,
		SourceID:                source.ID,
		Name:                    "Original",
		IsEnabled:               true,
		MustContain:             []string{"Show", "1080p"},
		MustNotContain:          []string{"CHT"},
		UseRegex:                true,
		CaseSensitive:           true,
		TargetVirtualParentPath: "/anime",
		DirectoryTemplate:       "{anime_title}/{season}",
		FilenameTemplate:        "{anime_title} - {episode}",
		ResolvedSourceID:        7,
		ResolvedInnerParentPath: "/anime",
		CreatedAt:               now.Add(-time.Hour),
		UpdatedAt:               now.Add(-time.Hour),
	})
	svc := NewRSSService(repo, nil, nil, WithRSSNow(func() time.Time { return now }))

	clone, err := svc.CloneSubscription(rssTestAuthContext(), original.ID, appdto.RSSSubscriptionCloneRequest{})
	if err != nil {
		t.Fatalf("CloneSubscription default failed: %v", err)
	}
	if clone.ID == original.ID || clone.Name != "Original Copy" || !clone.IsEnabled {
		t.Fatalf("default clone identity/state = %#v", clone)
	}
	if clone.SourceID != original.SourceID ||
		strings.Join(clone.MustContain, ",") != "Show,1080p" ||
		strings.Join(clone.MustNotContain, ",") != "CHT" ||
		!clone.UseRegex || !clone.CaseSensitive ||
		clone.TargetVirtualParentPath != original.TargetVirtualParentPath ||
		clone.DirectoryTemplate != original.DirectoryTemplate ||
		clone.FilenameTemplate != original.FilenameTemplate ||
		clone.ResolvedSourceID != original.ResolvedSourceID ||
		clone.ResolvedInnerParentPath != original.ResolvedInnerParentPath {
		t.Fatalf("clone did not preserve fields: %#v", clone)
	}
	if clone.CreatedAt != now.Format(time.RFC3339) || clone.UpdatedAt != now.Format(time.RFC3339) {
		t.Fatalf("clone timestamps = created %q updated %q", clone.CreatedAt, clone.UpdatedAt)
	}
	storedOriginal, err := repo.FindSubscriptionByID(context.Background(), original.ID)
	if err != nil {
		t.Fatalf("FindSubscriptionByID original failed: %v", err)
	}
	if storedOriginal.Name != "Original" || !storedOriginal.IsEnabled || !storedOriginal.CreatedAt.Equal(now.Add(-time.Hour)) {
		t.Fatalf("original subscription mutated: %#v", storedOriginal)
	}

	override, err := svc.CloneSubscription(rssTestAuthContext(), original.ID, appdto.RSSSubscriptionCloneRequest{
		Name:      "Paused Copy",
		IsEnabled: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("CloneSubscription override failed: %v", err)
	}
	if override.Name != "Paused Copy" || override.IsEnabled {
		t.Fatalf("override clone state = %#v", override)
	}
	if len(repo.subscriptions) != 3 {
		t.Fatalf("subscription count = %d, want 3", len(repo.subscriptions))
	}
}

func TestRSSCloneSubscriptionPersistsExplicitDisabledWithGORM(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	repo := gormrepo.NewRSSRepository(db)
	now := time.Date(2026, 5, 3, 10, 30, 0, 0, time.UTC)
	source := &entity.RSSSource{
		UserID:    1,
		Name:      "s",
		URL:       "https://example/rss.xml",
		IsEnabled: true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateSource(context.Background(), source); err != nil {
		t.Fatalf("CreateSource() error = %v", err)
	}
	original := &entity.RSSSubscription{
		UserID:                  1,
		SourceID:                source.ID,
		Name:                    "Original",
		IsEnabled:               true,
		TargetVirtualParentPath: "/anime",
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := repo.CreateSubscription(context.Background(), original); err != nil {
		t.Fatalf("CreateSubscription() original error = %v", err)
	}
	svc := NewRSSService(repo, nil, nil, WithRSSNow(func() time.Time { return now }))

	clone, err := svc.CloneSubscription(rssTestAuthContext(), original.ID, appdto.RSSSubscriptionCloneRequest{
		Name:      "Paused Copy",
		IsEnabled: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("CloneSubscription() error = %v", err)
	}
	if clone.IsEnabled {
		t.Fatalf("clone response enabled despite explicit false: %#v", clone)
	}
	stored, err := repo.FindSubscriptionByID(context.Background(), clone.ID)
	if err != nil {
		t.Fatalf("FindSubscriptionByID() clone error = %v", err)
	}
	if stored.IsEnabled {
		t.Fatalf("stored clone enabled despite explicit false: %#v", stored)
	}
}

func TestRSSBatchUpdateSubscriptionStateReturnsPartialResults(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 5, 2, 11, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "own", URL: "https://own/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	otherSource := repo.mustCreateSource(&entity.RSSSource{UserID: 2, Name: "other", URL: "https://other/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	own := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "own", IsEnabled: true, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)})
	other := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 2, SourceID: otherSource.ID, Name: "other", IsEnabled: true, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)})
	svc := NewRSSService(repo, nil, nil, WithRSSNow(func() time.Time { return now }))

	resp, err := svc.BatchUpdateSubscriptionState(rssUserContext(1, "rss.read"), appdto.RSSSubscriptionBatchStateRequest{
		SubscriptionIDs: []uint{own.ID, other.ID, 999},
		IsEnabled:       boolPtr(false),
	})
	if err != nil {
		t.Fatalf("BatchUpdateSubscriptionState failed: %v", err)
	}
	if resp.Succeeded != 1 || resp.Failed != 2 || len(resp.Items) != 3 {
		t.Fatalf("batch response = %#v", resp)
	}
	if !resp.Items[0].Success || resp.Items[0].Subscription == nil || resp.Items[0].Subscription.IsEnabled {
		t.Fatalf("success result = %#v", resp.Items[0])
	}
	if resp.Items[1].Success || resp.Items[1].ErrorCode == nil || *resp.Items[1].ErrorCode != "PERMISSION_DENIED" {
		t.Fatalf("permission failure result = %#v", resp.Items[1])
	}
	if resp.Items[2].Success || resp.Items[2].ErrorCode == nil || *resp.Items[2].ErrorCode != "RSS_SUBSCRIPTION_NOT_FOUND" {
		t.Fatalf("not found result = %#v", resp.Items[2])
	}
	storedOwn, _ := repo.FindSubscriptionByID(context.Background(), own.ID)
	if storedOwn.IsEnabled || !storedOwn.UpdatedAt.Equal(now) {
		t.Fatalf("own subscription state = %#v", storedOwn)
	}
	storedOther, _ := repo.FindSubscriptionByID(context.Background(), other.ID)
	if !storedOther.IsEnabled || !storedOther.UpdatedAt.Equal(now.Add(-time.Hour)) {
		t.Fatalf("other subscription should be unchanged: %#v", storedOther)
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

func TestRSSSourceFailureBackoffAndRecovery(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{
		UserID:                 1,
		Name:                   "bad",
		URL:                    "https://example.com/rss.xml",
		IsEnabled:              true,
		RefreshIntervalSeconds: 60,
		HealthStatus:           RSSSourceHealthOK,
		NextRefreshAt:          &now,
		CreatedAt:              now,
		UpdatedAt:              now,
	})
	fetcher := &fakeRSSFetcher{errByURL: map[string]error{source.URL: errors.New("temporary network timeout")}}
	svc := NewRSSService(repo, nil, nil, WithRSSFetcher(fetcher), WithRSSNow(func() time.Time { return now }))
	ctx := rssTestAuthContext()

	for i := 0; i < 5; i++ {
		if _, err := svc.RefreshSource(ctx, source.ID); err == nil {
			t.Fatalf("refresh %d expected error", i+1)
		}
		now = now.Add(time.Minute)
	}
	got, _ := repo.FindSourceByID(ctx, source.ID)
	if got.ConsecutiveFailures != 5 {
		t.Fatalf("consecutive failures = %d, want 5", got.ConsecutiveFailures)
	}
	if got.HealthStatus != RSSSourceHealthCircuitOpen {
		t.Fatalf("health = %q, want circuit_open", got.HealthStatus)
	}
	if got.NextRefreshAt == nil || got.NextRefreshAt.Sub(now.Add(-time.Minute)) != 30*time.Minute {
		t.Fatalf("next refresh = %v, want 30m backoff from last attempt", got.NextRefreshAt)
	}

	delete(fetcher.errByURL, source.URL)
	if _, err := svc.RefreshSource(ctx, source.ID); err != nil {
		t.Fatalf("refresh after recovery failed: %v", err)
	}
	got, _ = repo.FindSourceByID(ctx, source.ID)
	if got.ConsecutiveFailures != 0 || got.HealthStatus != RSSSourceHealthOK || got.LastSuccessAt == nil {
		t.Fatalf("source did not recover: failures=%d health=%q last_success=%v", got.ConsecutiveFailures, got.HealthStatus, got.LastSuccessAt)
	}
	if got.LastRefreshStatus != RSSRefreshStatusSuccess {
		t.Fatalf("last_refresh_status = %q", got.LastRefreshStatus)
	}
}

func TestRSSRefreshAllContinuesAfterSingleSourceFailure(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 4, 30, 11, 0, 0, 0, time.UTC)
	bad := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "bad", URL: "https://bad/rss.xml", IsEnabled: true, RefreshIntervalSeconds: 60, NextRefreshAt: &now, CreatedAt: now, UpdatedAt: now})
	good := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "good", URL: "https://good/rss.xml", IsEnabled: true, RefreshIntervalSeconds: 60, NextRefreshAt: &now, CreatedAt: now, UpdatedAt: now})
	fetcher := &fakeRSSFetcher{
		itemsByURL: map[string][]RSSFetchedItem{good.URL: {{Title: "Show 01", Link: "magnet:?xt=urn:btih:good"}}},
		errByURL:   map[string]error{bad.URL: errors.New("source temporarily unavailable")},
	}
	svc := NewRSSService(repo, nil, nil, WithRSSFetcher(fetcher), WithRSSNow(func() time.Time { return now }))

	resp, err := svc.RefreshAllSources(rssTestAuthContext())
	if err != nil {
		t.Fatalf("RefreshAllSources failed: %v", err)
	}
	if resp.Refreshed != 1 || resp.Failed != 1 {
		t.Fatalf("refresh-all counts = refreshed %d failed %d", resp.Refreshed, resp.Failed)
	}
	badAfter, _ := repo.FindSourceByID(context.Background(), bad.ID)
	goodAfter, _ := repo.FindSourceByID(context.Background(), good.ID)
	if badAfter.LastRefreshStatus != RSSRefreshStatusFailed || goodAfter.LastRefreshStatus != RSSRefreshStatusSuccess {
		t.Fatalf("source statuses = bad %q good %q", badAfter.LastRefreshStatus, goodAfter.LastRefreshStatus)
	}
}

func TestRSSRefreshAllForcesEnabledSourcesEvenWhenNotDue(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 4, 30, 11, 30, 0, 0, time.UTC)
	next := now.Add(time.Hour)
	source := repo.mustCreateSource(&entity.RSSSource{
		UserID:                 1,
		Name:                   "not due",
		URL:                    "https://not-due/rss.xml",
		IsEnabled:              true,
		RefreshIntervalSeconds: 3600,
		NextRefreshAt:          &next,
		CreatedAt:              now,
		UpdatedAt:              now,
	})
	fetcher := &fakeRSSFetcher{itemsByURL: map[string][]RSSFetchedItem{source.URL: {{Title: "Show 01", Link: "magnet:?xt=urn:btih:notdue"}}}}
	svc := NewRSSService(repo, nil, nil, WithRSSFetcher(fetcher), WithRSSNow(func() time.Time { return now }))

	resp, err := svc.RefreshAllSources(rssTestAuthContext())
	if err != nil {
		t.Fatalf("RefreshAllSources failed: %v", err)
	}
	if resp.Refreshed != 1 || resp.Skipped != 0 || resp.Failed != 0 {
		t.Fatalf("refresh-all counts = refreshed %d skipped %d failed %d", resp.Refreshed, resp.Skipped, resp.Failed)
	}
	got, _ := repo.FindSourceByID(context.Background(), source.ID)
	if got.LastRefreshStatus != RSSRefreshStatusSuccess || got.LastRefreshedAt == nil || !got.LastRefreshedAt.Equal(now) {
		t.Fatalf("source was not force refreshed: %#v", got)
	}
}

func TestRSSSubscriptionPreviewExplainsMatchedMissingExcluded(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	sub := repo.mustCreateSubscription(&entity.RSSSubscription{
		UserID:         1,
		SourceID:       source.ID,
		Name:           "sub",
		IsEnabled:      true,
		MustContain:    []string{"1080p", "05"},
		MustNotContain: []string{"CHT"},
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "Show - 05 [1080p] [CHS]", DownloadURL: "magnet:?xt=urn:btih:1", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusNew, CreatedAt: now, UpdatedAt: now})
	repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "Show - 04 [1080p] [CHS]", DownloadURL: "magnet:?xt=urn:btih:2", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusNew, CreatedAt: now, UpdatedAt: now})
	repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "Show - 05 [1080p] [CHT]", DownloadURL: "magnet:?xt=urn:btih:3", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusNew, CreatedAt: now, UpdatedAt: now})
	svc := NewRSSService(repo, nil, nil)

	resp, err := svc.PreviewSubscription(rssTestAuthContext(), sub.ID)
	if err != nil {
		t.Fatalf("PreviewSubscription failed: %v", err)
	}
	if resp.Matched != 1 || resp.Missing != 1 || resp.Excluded != 1 {
		t.Fatalf("preview counts = matched %d missing %d excluded %d", resp.Matched, resp.Missing, resp.Excluded)
	}
	if resp.Items[1].Result != "missing" || !containsString(resp.Items[1].Missing, "05") {
		t.Fatalf("missing explanation = %#v", resp.Items[1])
	}
	if resp.Items[2].Result != "excluded" || !containsString(resp.Items[2].Excluded, "CHT") {
		t.Fatalf("excluded explanation = %#v", resp.Items[2])
	}
}

func TestRSSTemporarySubscriptionPreviewDoesNotPersist(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "Show - 05 [1080p] [CHS]", DownloadURL: "magnet:?xt=urn:btih:tmp1", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusNew, CreatedAt: now, UpdatedAt: now})
	repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "Show - 04 [1080p] [CHS]", DownloadURL: "magnet:?xt=urn:btih:tmp2", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusNew, CreatedAt: now, UpdatedAt: now})
	repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "Show - 05 [1080p] [CHT]", DownloadURL: "magnet:?xt=urn:btih:tmp3", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusNew, CreatedAt: now, UpdatedAt: now})
	svc := NewRSSService(repo, nil, nil)

	resp, err := svc.PreviewSubscriptionRules(rssTestAuthContext(), appdto.RSSSubscriptionPreviewRequest{
		SourceID:       source.ID,
		MustContain:    []string{"1080p", "05"},
		MustNotContain: []string{"CHT"},
	})
	if err != nil {
		t.Fatalf("PreviewSubscriptionRules failed: %v", err)
	}
	if resp.SubscriptionID != 0 || resp.SourceID != source.ID {
		t.Fatalf("temporary preview identity = subscription %d source %d", resp.SubscriptionID, resp.SourceID)
	}
	if resp.Matched != 1 || resp.Missing != 1 || resp.Excluded != 1 {
		t.Fatalf("preview counts = matched %d missing %d excluded %d", resp.Matched, resp.Missing, resp.Excluded)
	}
	if len(repo.subscriptions) != 0 {
		t.Fatalf("temporary preview persisted subscriptions: %d", len(repo.subscriptions))
	}
	_, err = svc.PreviewSubscriptionRules(rssTestAuthContext(), appdto.RSSSubscriptionPreviewRequest{
		SourceID:    source.ID,
		MustContain: []string{"["},
		UseRegex:    true,
	})
	if !errors.Is(err, ErrRSSRegexInvalid) {
		t.Fatalf("invalid regex error = %v", err)
	}
}

func TestRSSBatchIgnoreReturnsPartialResults(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	now := time.Date(2026, 5, 2, 9, 30, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	oldError := "old error"
	oldReason := RSSRetryReasonDownloaderUnavailable
	nextRetryAt := now.Add(time.Hour)
	ok := repo.mustCreateItem(&entity.RSSItem{
		UserID:       1,
		SourceID:     source.ID,
		Title:        "ok",
		Status:       RSSItemStatusNeedsAttention,
		ErrorMessage: &oldError,
		RetryReason:  &oldReason,
		NextRetryAt:  &nextRetryAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	completed := repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "done", Status: RSSItemStatusCompleted, CreatedAt: now, UpdatedAt: now})
	activeTaskID := tasks.addTask("running", nil)
	active := repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "active", Status: RSSItemStatusEnqueued, TaskID: &activeTaskID, CreatedAt: now, UpdatedAt: now})
	svc := NewRSSService(repo, nil, nil, WithRSSTaskRepository(tasks), WithRSSNow(func() time.Time { return now }))

	resp, err := svc.BatchIgnoreItems(rssTestAuthContext(), appdto.RSSItemBatchIgnoreRequest{ItemIDs: []uint{ok.ID, completed.ID, active.ID}})
	if err != nil {
		t.Fatalf("BatchIgnoreItems failed: %v", err)
	}
	if resp.Succeeded != 1 || resp.Failed != 2 || len(resp.Items) != 3 {
		t.Fatalf("batch ignore response = %#v", resp)
	}
	stored, err := repo.FindItemByID(context.Background(), ok.ID)
	if err != nil {
		t.Fatalf("FindItemByID ok failed: %v", err)
	}
	if stored.Status != RSSItemStatusIgnored || stored.ErrorMessage != nil || stored.RetryReason != nil || stored.NextRetryAt != nil || !stored.UpdatedAt.Equal(now) {
		t.Fatalf("ignored item state = %#v", stored)
	}
	for _, result := range resp.Items[1:] {
		if result.Success || result.ErrorCode == nil || *result.ErrorCode != "TASK_INVALID_STATE" {
			t.Fatalf("expected task invalid failure, got %#v", result)
		}
	}
}

func TestRSSBatchRetryReturnsPartialResults(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	sub := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "sub", IsEnabled: true, MustContain: []string{"Show"}, TargetVirtualParentPath: "/anime", CreatedAt: now, UpdatedAt: now})
	retryable := repo.mustCreateItem(&entity.RSSItem{
		UserID:                1,
		SourceID:              source.ID,
		Title:                 "Show - 01",
		DownloadURL:           "magnet:?xt=urn:btih:batchretry",
		LinkType:              RSSLinkTypeMagnet,
		Status:                RSSItemStatusFailed,
		MatchedSubscriptionID: &sub.ID,
		MaxRetryCount:         3,
		CreatedAt:             now,
		UpdatedAt:             now,
	})
	unsupported := repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "Show - 02", DownloadURL: "https://example.invalid/file.zip", LinkType: RSSLinkTypeUnsupported, Status: RSSItemStatusNeedsAttention, CreatedAt: now, UpdatedAt: now})
	svc := NewRSSService(repo, nil, tasks, WithRSSTaskRepository(tasks), WithRSSNow(func() time.Time { return now }))

	resp, err := svc.BatchRetryItems(rssTestAuthContext(), appdto.RSSItemBatchRetryRequest{ItemIDs: []uint{retryable.ID, unsupported.ID}})
	if err != nil {
		t.Fatalf("BatchRetryItems failed: %v", err)
	}
	if resp.Succeeded != 1 || resp.Failed != 1 || len(resp.Items) != 2 {
		t.Fatalf("batch retry response = %#v", resp)
	}
	if !resp.Items[0].Success || resp.Items[0].Item == nil || resp.Items[0].Item.Status != RSSItemStatusEnqueued || resp.Items[0].Item.RetryCount != 1 {
		t.Fatalf("retry success result = %#v", resp.Items[0])
	}
	if resp.Items[1].Success || resp.Items[1].ErrorCode == nil || *resp.Items[1].ErrorCode != "DOWNLOAD_LINK_UNSUPPORTED" {
		t.Fatalf("retry failure result = %#v", resp.Items[1])
	}
}

func TestRSSManualRetryAndReprocess(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	now := time.Date(2026, 4, 30, 13, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	sub := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "sub", IsEnabled: true, MustContain: []string{"Show"}, TargetVirtualParentPath: "/anime", CreatedAt: now, UpdatedAt: now})
	failed := repo.mustCreateItem(&entity.RSSItem{
		UserID:                1,
		SourceID:              source.ID,
		Title:                 "Show - 01",
		DownloadURL:           "magnet:?xt=urn:btih:retry",
		LinkType:              RSSLinkTypeMagnet,
		Status:                RSSItemStatusFailed,
		MatchedSubscriptionID: &sub.ID,
		MaxRetryCount:         3,
		CreatedAt:             now,
		UpdatedAt:             now,
	})
	ignored := repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "Show - 02", DownloadURL: "magnet:?xt=urn:btih:reprocess", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusIgnored, MaxRetryCount: 3, CreatedAt: now, UpdatedAt: now})
	svc := NewRSSService(repo, nil, tasks, WithRSSTaskRepository(tasks), WithRSSNow(func() time.Time { return now }))
	ctx := rssTestAuthContext()

	retried, err := svc.RetryItem(ctx, failed.ID, appdto.RSSManualDownloadRequest{})
	if err != nil {
		t.Fatalf("RetryItem failed: %v", err)
	}
	if retried.Status != RSSItemStatusEnqueued || retried.TaskID == nil || retried.RetryCount != 1 {
		t.Fatalf("retried item = %#v", retried)
	}
	reprocessed, err := svc.ReprocessItem(ctx, ignored.ID)
	if err != nil {
		t.Fatalf("ReprocessItem failed: %v", err)
	}
	if reprocessed.Status != RSSItemStatusEnqueued || reprocessed.TaskID == nil {
		t.Fatalf("reprocessed item = %#v", reprocessed)
	}
}

func TestRSSAdminTriggeredEnqueueUsesSubscriptionOwnerContext(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	now := time.Date(2026, 4, 30, 13, 30, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, NextRefreshAt: &now, CreatedAt: now, UpdatedAt: now})
	repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "sub", IsEnabled: true, MustContain: []string{"Show"}, TargetVirtualParentPath: "/anime", CreatedAt: now, UpdatedAt: now})
	fetcher := &fakeRSSFetcher{itemsByURL: map[string][]RSSFetchedItem{source.URL: {{Title: "Show - 01", Link: "magnet:?xt=urn:btih:owner"}}}}
	svc := NewRSSService(repo, nil, tasks, WithRSSTaskRepository(tasks), WithRSSFetcher(fetcher), WithRSSNow(func() time.Time { return now }))
	adminCtx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       99,
		Username:     "admin",
		RoleKey:      "admin",
		Capabilities: []string{"rss.read", "rss.manage"},
	})

	if _, err := svc.RefreshSource(adminCtx, source.ID); err != nil {
		t.Fatalf("RefreshSource failed: %v", err)
	}
	if tasks.lastCreateUserID != source.UserID {
		t.Fatalf("task create user_id = %d, want source owner %d", tasks.lastCreateUserID, source.UserID)
	}
}

func TestRSSRefreshPersistsParsedMetadataAndDTO(t *testing.T) {
	repo := newFakeRSSRepo()
	now := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, NextRefreshAt: &now, CreatedAt: now, UpdatedAt: now})
	fetcher := &fakeRSSFetcher{itemsByURL: map[string][]RSSFetchedItem{source.URL: {
		{Title: "[SubsPlease] Example Show S01E05 [1080p]", Link: "magnet:?xt=urn:btih:parsed", GUID: "parsed"},
	}}}
	svc := NewRSSService(repo, nil, nil, WithRSSFetcher(fetcher), WithRSSNow(func() time.Time { return now }))

	if _, err := svc.RefreshSource(rssTestAuthContext(), source.ID); err != nil {
		t.Fatalf("RefreshSource failed: %v", err)
	}
	list, err := svc.ListItems(rssTestAuthContext(), domainrepo.RSSItemFilter{SourceID: source.ID})
	if err != nil {
		t.Fatalf("ListItems failed: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("items len = %d", len(list.Items))
	}
	parsed := list.Items[0].Parsed
	if parsed.AnimeTitle != "Example Show" || parsed.Season != "S01" || parsed.Episode != "05" || parsed.SubtitleGroup != "SubsPlease" || parsed.Resolution != "1080p" {
		t.Fatalf("parsed DTO = %#v", parsed)
	}
	stored, err := repo.FindItemByID(context.Background(), list.Items[0].ID)
	if err != nil {
		t.Fatalf("FindItemByID failed: %v", err)
	}
	if stored.Parsed.AnimeTitle != "Example Show" || stored.Parsed.Episode != "05" {
		t.Fatalf("stored parsed = %#v", stored.Parsed)
	}
}

func TestRSSEnqueueDirectoryTemplateTargetPath(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	now := time.Date(2026, 5, 2, 11, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	baseSub := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "base", IsEnabled: true, TargetVirtualParentPath: "/anime", CreatedAt: now, UpdatedAt: now})
	baseItem := repo.mustCreateItem(&entity.RSSItem{
		UserID:      1,
		SourceID:    source.ID,
		Title:       "[SubsPlease] Base Show - 01 [1080p]",
		DownloadURL: "magnet:?xt=urn:btih:base",
		LinkType:    RSSLinkTypeMagnet,
		Status:      RSSItemStatusNew,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	templateSub := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "template", IsEnabled: true, TargetVirtualParentPath: "/anime", DirectoryTemplate: "{anime_title}/{season}", CreatedAt: now, UpdatedAt: now})
	templateItem := repo.mustCreateItem(&entity.RSSItem{
		UserID:      1,
		SourceID:    source.ID,
		Title:       "[SubsPlease] Example/Show S01E05 [1080p]",
		DownloadURL: "magnet:?xt=urn:btih:template",
		LinkType:    RSSLinkTypeMagnet,
		Status:      RSSItemStatusNew,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	svc := NewRSSService(repo, nil, tasks, WithRSSNow(func() time.Time { return now }))
	ctx := rssTestAuthContext()

	if _, err := svc.DownloadItem(ctx, baseItem.ID, appdto.RSSManualDownloadRequest{SubscriptionID: baseSub.ID}); err != nil {
		t.Fatalf("DownloadItem base failed: %v", err)
	}
	if tasks.lastCreateRequest.TargetVirtualParentPath != "/anime" {
		t.Fatalf("base target path = %q", tasks.lastCreateRequest.TargetVirtualParentPath)
	}
	if _, err := svc.DownloadItem(ctx, templateItem.ID, appdto.RSSManualDownloadRequest{SubscriptionID: templateSub.ID}); err != nil {
		t.Fatalf("DownloadItem template failed: %v", err)
	}
	if tasks.lastCreateRequest.TargetVirtualParentPath != "/anime/Example Show/S01" {
		t.Fatalf("template target path = %q", tasks.lastCreateRequest.TargetVirtualParentPath)
	}
}

func TestRSSEnqueueFilenameTemplateTargetFilename(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	now := time.Date(2026, 5, 2, 11, 30, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	sub := repo.mustCreateSubscription(&entity.RSSSubscription{
		UserID:                  1,
		SourceID:                source.ID,
		Name:                    "template",
		IsEnabled:               true,
		TargetVirtualParentPath: "/anime",
		FilenameTemplate:        "{anime_title} - {season}E{episode} [{resolution}]",
		CreatedAt:               now,
		UpdatedAt:               now,
	})
	item := repo.mustCreateItem(&entity.RSSItem{
		UserID:      1,
		SourceID:    source.ID,
		Title:       "[SubsPlease] Example/Show S01E05 [1080p]",
		DownloadURL: "magnet:?xt=urn:btih:filename",
		LinkType:    RSSLinkTypeMagnet,
		Status:      RSSItemStatusNew,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	svc := NewRSSService(repo, nil, tasks, WithRSSNow(func() time.Time { return now }))

	if _, err := svc.DownloadItem(rssTestAuthContext(), item.ID, appdto.RSSManualDownloadRequest{SubscriptionID: sub.ID}); err != nil {
		t.Fatalf("DownloadItem failed: %v", err)
	}
	if tasks.lastCreateRequest.TargetFilename != "Example Show - S01E05 [1080p]" {
		t.Fatalf("target filename = %q", tasks.lastCreateRequest.TargetFilename)
	}
}

func TestRSSManualDownloadFailureMarksItemNeedsAttention(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	tasks.createErr = fmt.Errorf("%w: qbittorrent /api/v2/torrents/add status 401: Unauthorized", ErrDownloaderAuthFailed)
	notifier := &fakeRSSNotifier{}
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	sub := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "sub", IsEnabled: true, TargetVirtualParentPath: "/anime", CreatedAt: now, UpdatedAt: now})
	item := repo.mustCreateItem(&entity.RSSItem{
		UserID:      1,
		SourceID:    source.ID,
		Title:       "Show - 01",
		DownloadURL: "magnet:?xt=urn:btih:manual401",
		LinkType:    RSSLinkTypeMagnet,
		Status:      RSSItemStatusMatched,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	svc := NewRSSService(repo, nil, tasks, WithRSSNotifier(notifier), WithRSSNow(func() time.Time { return now }))

	_, err := svc.DownloadItem(rssTestAuthContext(), item.ID, appdto.RSSManualDownloadRequest{SubscriptionID: sub.ID})
	if err == nil {
		t.Fatalf("expected manual download failure")
	}
	if !errors.Is(err, ErrDownloaderAuthFailed) {
		t.Fatalf("expected ErrDownloaderAuthFailed, got %v", err)
	}

	stored, err := repo.FindItemByID(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("FindItemByID() error = %v", err)
	}
	if stored.Status != RSSItemStatusNeedsAttention {
		t.Fatalf("expected needs_attention after manual failure, got %q", stored.Status)
	}
	if stored.ErrorMessage == nil || !strings.Contains(*stored.ErrorMessage, "torrents/add status 401") {
		t.Fatalf("expected visible qBittorrent error, got %#v", stored.ErrorMessage)
	}
	if stored.RetryReason == nil || *stored.RetryReason != RSSRetryReasonDownloaderUnavailable {
		t.Fatalf("expected downloader retry reason, got %#v", stored.RetryReason)
	}
	if stored.NextRetryAt != nil {
		t.Fatalf("manual failure should not schedule automatic retry, got %v", stored.NextRetryAt)
	}
	if stored.LastAttemptAt == nil || !stored.LastAttemptAt.Equal(now) {
		t.Fatalf("last_attempt_at = %#v", stored.LastAttemptAt)
	}
	if stored.MatchedSubscriptionID == nil || *stored.MatchedSubscriptionID != sub.ID {
		t.Fatalf("matched subscription = %#v", stored.MatchedSubscriptionID)
	}
	if stored.TaskID != nil {
		t.Fatalf("failed manual enqueue should not attach task, got %#v", stored.TaskID)
	}
	if len(notifier.events) != 1 {
		t.Fatalf("expected one needs-attention notification, got %d", len(notifier.events))
	}
	if event := notifier.events[0]; event.EventType != NotificationEventRSSItemNeedsAttention || event.UserID != item.UserID {
		t.Fatalf("unexpected notification event: %+v", event)
	}

	listResp, err := svc.ListItems(rssTestAuthContext(), domainrepo.RSSItemFilter{Status: RSSItemStatusNeedsAttention})
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(listResp.Items) != 1 || listResp.Items[0].ErrorMessage == nil || !strings.Contains(*listResp.Items[0].ErrorMessage, "torrents/add status 401") {
		t.Fatalf("list response should expose item error, got %#v", listResp.Items)
	}
}

func TestRSSRetryWorkerBackoffAndMaxAttention(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	tasks.createErr = errors.New("qbittorrent unavailable")
	now := time.Date(2026, 4, 30, 14, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	sub := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "sub", IsEnabled: true, TargetVirtualParentPath: "/anime", CreatedAt: now, UpdatedAt: now})
	item := repo.mustCreateItem(&entity.RSSItem{
		UserID:                1,
		SourceID:              source.ID,
		Title:                 "Show - 01",
		DownloadURL:           "magnet:?xt=urn:btih:retry",
		LinkType:              RSSLinkTypeMagnet,
		Status:                RSSItemStatusRetryPending,
		MatchedSubscriptionID: &sub.ID,
		MaxRetryCount:         3,
		NextRetryAt:           &now,
		CreatedAt:             now,
		UpdatedAt:             now,
	})
	svc := NewRSSService(repo, nil, tasks, WithRSSTaskRepository(tasks), WithRSSNow(func() time.Time { return now }))

	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := svc.RunRetryCycle(context.Background(), 10); err != nil {
			t.Fatalf("retry cycle %d failed: %v", attempt, err)
		}
		item, _ = repo.FindItemByID(context.Background(), item.ID)
		if attempt < 3 {
			if item.Status != RSSItemStatusRetryPending || item.RetryCount != attempt || item.NextRetryAt == nil {
				t.Fatalf("attempt %d item = %#v", attempt, item)
			}
			now = *item.NextRetryAt
		}
	}
	item, _ = repo.FindItemByID(context.Background(), item.ID)
	if item.Status != RSSItemStatusNeedsAttention || item.RetryCount != 3 || item.NextRetryAt != nil {
		t.Fatalf("item after max retry = %#v", item)
	}
}

func TestRSSTaskBacklinkUpdatesItemTerminalStates(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	now := time.Date(2026, 4, 30, 15, 0, 0, 0, time.UTC)
	completedTaskID := tasks.addTask("completed", nil)
	tasks.tasks[completedTaskID].ResultVFSNodeID = 99
	completedWithoutNodeTaskID := tasks.addTask("completed", nil)
	failedMessage := "temporary network error"
	failedTaskID := tasks.addTask("failed", &failedMessage)
	metadataFailedMessage := ErrMetadataVFSCommitFailed.Error()
	metadataFailedTaskID := tasks.addTask("failed", &metadataFailedMessage)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	sub := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "sub", IsEnabled: true, TargetVirtualParentPath: "/anime", CreatedAt: now, UpdatedAt: now})
	completed := repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "done", DownloadURL: "magnet:?xt=urn:btih:done", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusEnqueued, MatchedSubscriptionID: &sub.ID, TaskID: &completedTaskID, MaxRetryCount: 3, CreatedAt: now, UpdatedAt: now})
	completedWithoutNode := repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "done-no-node", DownloadURL: "magnet:?xt=urn:btih:done2", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusEnqueued, MatchedSubscriptionID: &sub.ID, TaskID: &completedWithoutNodeTaskID, MaxRetryCount: 3, CreatedAt: now, UpdatedAt: now})
	failed := repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "fail", DownloadURL: "magnet:?xt=urn:btih:fail", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusEnqueued, MatchedSubscriptionID: &sub.ID, TaskID: &failedTaskID, MaxRetryCount: 3, CreatedAt: now, UpdatedAt: now})
	metadataFailed := repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "metadata-fail", DownloadURL: "magnet:?xt=urn:btih:metafail", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusEnqueued, MatchedSubscriptionID: &sub.ID, TaskID: &metadataFailedTaskID, MaxRetryCount: 3, CreatedAt: now, UpdatedAt: now})
	svc := NewRSSService(repo, nil, tasks, WithRSSTaskRepository(tasks), WithRSSNow(func() time.Time { return now }))

	if _, err := svc.RunRetryCycle(context.Background(), 10); err != nil {
		t.Fatalf("RunRetryCycle failed: %v", err)
	}
	completed, _ = repo.FindItemByID(context.Background(), completed.ID)
	completedWithoutNode, _ = repo.FindItemByID(context.Background(), completedWithoutNode.ID)
	failed, _ = repo.FindItemByID(context.Background(), failed.ID)
	metadataFailed, _ = repo.FindItemByID(context.Background(), metadataFailed.ID)
	if completed.Status != RSSItemStatusCompleted {
		t.Fatalf("completed backlink status = %q", completed.Status)
	}
	if completed.ResultVFSNodeID != 99 {
		t.Fatalf("expected completed item result_vfs_node_id=99, got %#v", completed)
	}
	if completedWithoutNode.Status != RSSItemStatusNeedsAttention ||
		completedWithoutNode.ErrorMessage == nil ||
		*completedWithoutNode.ErrorMessage != ErrMetadataVFSCommitFailed.Error() {
		t.Fatalf("completed task without result node should need attention, got %#v", completedWithoutNode)
	}
	if failed.Status != RSSItemStatusRetryPending || failed.RetryReason == nil || *failed.RetryReason != RSSRetryReasonDownloaderUnavailable {
		t.Fatalf("failed backlink item = %#v", failed)
	}
	if metadataFailed.Status != RSSItemStatusNeedsAttention ||
		metadataFailed.ErrorMessage == nil ||
		*metadataFailed.ErrorMessage != ErrMetadataVFSCommitFailed.Error() ||
		metadataFailed.NextRetryAt != nil {
		t.Fatalf("metadata failed backlink item = %#v", metadataFailed)
	}
}

func TestRSSRetrySkipsActiveTaskToAvoidDuplicate(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	now := time.Date(2026, 4, 30, 16, 0, 0, 0, time.UTC)
	activeTaskID := tasks.addTask("running", nil)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	sub := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "sub", IsEnabled: true, TargetVirtualParentPath: "/anime", CreatedAt: now, UpdatedAt: now})
	item := repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "Show", DownloadURL: "magnet:?xt=urn:btih:active", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusRetryPending, MatchedSubscriptionID: &sub.ID, TaskID: &activeTaskID, MaxRetryCount: 3, NextRetryAt: &now, CreatedAt: now, UpdatedAt: now})
	svc := NewRSSService(repo, nil, tasks, WithRSSTaskRepository(tasks), WithRSSNow(func() time.Time { return now }))

	if _, err := svc.RunRetryCycle(context.Background(), 10); err != nil {
		t.Fatalf("RunRetryCycle failed: %v", err)
	}
	itemAfter, _ := repo.FindItemByID(context.Background(), item.ID)
	if tasks.createCalls != 0 {
		t.Fatalf("created duplicate tasks: %d", tasks.createCalls)
	}
	if itemAfter.TaskID == nil || *itemAfter.TaskID != activeTaskID || itemAfter.Status != RSSItemStatusRetryPending {
		t.Fatalf("item changed unexpectedly: %#v", itemAfter)
	}
}

func TestRSSRefreshDoesNotRequeueCompletedOrRetryPendingItems(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	now := time.Date(2026, 4, 30, 17, 0, 0, 0, time.UTC)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, NextRefreshAt: &now, CreatedAt: now, UpdatedAt: now})
	sub := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "sub", IsEnabled: true, MustContain: []string{"Show"}, TargetVirtualParentPath: "/anime", CreatedAt: now, UpdatedAt: now})
	completedTaskID := tasks.addTask("completed", nil)
	retryAt := now.Add(30 * time.Minute)
	completed := repo.mustCreateItem(&entity.RSSItem{
		UserID:                1,
		SourceID:              source.ID,
		Title:                 "Show - 01",
		Link:                  "magnet:?xt=urn:btih:done",
		GUID:                  "done",
		DedupKey:              "guid:done",
		DownloadURL:           "magnet:?xt=urn:btih:done",
		LinkType:              RSSLinkTypeMagnet,
		Status:                RSSItemStatusCompleted,
		MatchedSubscriptionID: &sub.ID,
		TaskID:                &completedTaskID,
		MaxRetryCount:         3,
		CreatedAt:             now,
		UpdatedAt:             now,
	})
	retryPending := repo.mustCreateItem(&entity.RSSItem{
		UserID:                1,
		SourceID:              source.ID,
		Title:                 "Show - 02",
		Link:                  "magnet:?xt=urn:btih:retry",
		GUID:                  "retry",
		DedupKey:              "guid:retry",
		DownloadURL:           "magnet:?xt=urn:btih:retry",
		LinkType:              RSSLinkTypeMagnet,
		Status:                RSSItemStatusRetryPending,
		MatchedSubscriptionID: &sub.ID,
		MaxRetryCount:         3,
		NextRetryAt:           &retryAt,
		CreatedAt:             now,
		UpdatedAt:             now,
	})
	fetcher := &fakeRSSFetcher{itemsByURL: map[string][]RSSFetchedItem{source.URL: {
		{Title: completed.Title, Link: completed.Link, GUID: completed.GUID},
		{Title: retryPending.Title, Link: retryPending.Link, GUID: retryPending.GUID},
	}}}
	svc := NewRSSService(repo, nil, tasks, WithRSSTaskRepository(tasks), WithRSSFetcher(fetcher), WithRSSNow(func() time.Time { return now }))

	if _, err := svc.RefreshSource(rssTestAuthContext(), source.ID); err != nil {
		t.Fatalf("RefreshSource failed: %v", err)
	}
	completedAfter, _ := repo.FindItemByID(context.Background(), completed.ID)
	retryAfter, _ := repo.FindItemByID(context.Background(), retryPending.ID)
	if tasks.createCalls != 0 {
		t.Fatalf("created duplicate tasks: %d", tasks.createCalls)
	}
	if completedAfter.Status != RSSItemStatusCompleted || completedAfter.TaskID == nil || *completedAfter.TaskID != completedTaskID {
		t.Fatalf("completed item changed unexpectedly: %#v", completedAfter)
	}
	if retryAfter.Status != RSSItemStatusRetryPending || retryAfter.NextRetryAt == nil || !retryAfter.NextRetryAt.Equal(retryAt) {
		t.Fatalf("retry pending item changed unexpectedly: %#v", retryAfter)
	}
}

func TestRSSTaskCanceledMovesItemToNeedsAttention(t *testing.T) {
	repo := newFakeRSSRepo()
	tasks := newFakeRSSTasks()
	now := time.Date(2026, 4, 30, 18, 0, 0, 0, time.UTC)
	canceledTaskID := tasks.addTask("canceled", nil)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	sub := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "sub", IsEnabled: true, TargetVirtualParentPath: "/anime", CreatedAt: now, UpdatedAt: now})
	item := repo.mustCreateItem(&entity.RSSItem{
		UserID:                1,
		SourceID:              source.ID,
		Title:                 "cancel",
		DownloadURL:           "magnet:?xt=urn:btih:cancel",
		LinkType:              RSSLinkTypeMagnet,
		Status:                RSSItemStatusEnqueued,
		MatchedSubscriptionID: &sub.ID,
		TaskID:                &canceledTaskID,
		MaxRetryCount:         3,
		CreatedAt:             now,
		UpdatedAt:             now,
	})
	svc := NewRSSService(repo, nil, tasks, WithRSSTaskRepository(tasks), WithRSSNow(func() time.Time { return now }))

	if _, err := svc.RunRetryCycle(context.Background(), 10); err != nil {
		t.Fatalf("RunRetryCycle failed: %v", err)
	}
	item, _ = repo.FindItemByID(context.Background(), item.ID)
	if item.Status != RSSItemStatusNeedsAttention || item.NextRetryAt != nil {
		t.Fatalf("canceled item should need attention without auto retry: %#v", item)
	}
	if item.ErrorMessage == nil || *item.ErrorMessage != "download canceled" {
		t.Fatalf("canceled error message = %#v", item.ErrorMessage)
	}
}

type fakeRSSFetcher struct {
	itemsByURL map[string][]RSSFetchedItem
	errByURL   map[string]error
}

func (f *fakeRSSFetcher) Fetch(ctx context.Context, rawURL string) ([]RSSFetchedItem, error) {
	if err := f.errByURL[rawURL]; err != nil {
		return nil, err
	}
	return append([]RSSFetchedItem{}, f.itemsByURL[rawURL]...), nil
}

type fakeRSSTasks struct {
	nextID            uint
	createErr         error
	createCalls       int
	lastCreateUserID  uint
	lastCreateRequest appdto.CreateTaskRequest
	tasks             map[uint]*entity.DownloadTask
}

func newFakeRSSTasks() *fakeRSSTasks {
	return &fakeRSSTasks{nextID: 1, tasks: map[uint]*entity.DownloadTask{}}
}

func (f *fakeRSSTasks) Create(ctx context.Context, req appdto.CreateTaskRequest) (*appdto.DownloadTaskView, error) {
	f.createCalls++
	f.lastCreateRequest = req
	if auth, ok := security.RequestAuthFromContext(ctx); ok {
		f.lastCreateUserID = auth.UserID
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	id := f.nextID
	f.nextID++
	f.tasks[id] = &entity.DownloadTask{ID: id, Status: "pending", SourceURL: req.URL, TargetFilename: req.TargetFilename}
	return &appdto.DownloadTaskView{ID: id, Status: "pending", SourceURL: req.URL, TargetFilename: req.TargetFilename}, nil
}

func (f *fakeRSSTasks) FindByID(ctx context.Context, id uint) (*entity.DownloadTask, error) {
	task, ok := f.tasks[id]
	if !ok {
		return nil, domainrepo.ErrNotFound
	}
	copied := *task
	return &copied, nil
}

func (f *fakeRSSTasks) addTask(status string, message *string) uint {
	id := f.nextID
	f.nextID++
	f.tasks[id] = &entity.DownloadTask{ID: id, Status: status, ErrorMessage: message}
	return id
}

type fakeRSSNotifier struct {
	events []NotificationEventInput
}

func (f *fakeRSSNotifier) Notify(ctx context.Context, input NotificationEventInput) (*entity.NotificationEvent, error) {
	f.events = append(f.events, input)
	return &entity.NotificationEvent{UserID: input.UserID, EventType: input.EventType}, nil
}

type fakeRSSRepo struct {
	nextSourceID       uint
	nextSubscriptionID uint
	nextItemID         uint
	sources            map[uint]*entity.RSSSource
	subscriptions      map[uint]*entity.RSSSubscription
	items              map[uint]*entity.RSSItem
}

func newFakeRSSRepo() *fakeRSSRepo {
	return &fakeRSSRepo{
		nextSourceID:       1,
		nextSubscriptionID: 1,
		nextItemID:         1,
		sources:            map[uint]*entity.RSSSource{},
		subscriptions:      map[uint]*entity.RSSSubscription{},
		items:              map[uint]*entity.RSSItem{},
	}
}

func (r *fakeRSSRepo) mustCreateSource(source *entity.RSSSource) *entity.RSSSource {
	if err := r.CreateSource(context.Background(), source); err != nil {
		panic(err)
	}
	return source
}

func (r *fakeRSSRepo) CreateSource(ctx context.Context, source *entity.RSSSource) error {
	if source.ID == 0 {
		source.ID = r.nextSourceID
		r.nextSourceID++
	}
	copied := *source
	r.sources[source.ID] = &copied
	return nil
}

func (r *fakeRSSRepo) UpdateSource(ctx context.Context, source *entity.RSSSource) error {
	if _, ok := r.sources[source.ID]; !ok {
		return domainrepo.ErrNotFound
	}
	copied := *source
	r.sources[source.ID] = &copied
	return nil
}

func (r *fakeRSSRepo) DeleteSource(ctx context.Context, id uint) error {
	if _, ok := r.sources[id]; !ok {
		return domainrepo.ErrNotFound
	}
	delete(r.sources, id)
	return nil
}

func (r *fakeRSSRepo) FindSourceByID(ctx context.Context, id uint) (*entity.RSSSource, error) {
	source, ok := r.sources[id]
	if !ok {
		return nil, domainrepo.ErrNotFound
	}
	copied := *source
	return &copied, nil
}

func (r *fakeRSSRepo) ListSources(ctx context.Context, filter domainrepo.RSSSourceFilter) ([]*entity.RSSSource, error) {
	out := []*entity.RSSSource{}
	for _, source := range r.sources {
		if !filter.IncludeAll && filter.UserID != source.UserID {
			continue
		}
		if filter.Enabled != nil && *filter.Enabled != source.IsEnabled {
			continue
		}
		copied := *source
		out = append(out, &copied)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *fakeRSSRepo) mustCreateSubscription(subscription *entity.RSSSubscription) *entity.RSSSubscription {
	if err := r.CreateSubscription(context.Background(), subscription); err != nil {
		panic(err)
	}
	return subscription
}

func (r *fakeRSSRepo) CreateSubscription(ctx context.Context, subscription *entity.RSSSubscription) error {
	if subscription.ID == 0 {
		subscription.ID = r.nextSubscriptionID
		r.nextSubscriptionID++
	}
	copied := *subscription
	r.subscriptions[subscription.ID] = &copied
	return nil
}

func (r *fakeRSSRepo) UpdateSubscription(ctx context.Context, subscription *entity.RSSSubscription) error {
	if _, ok := r.subscriptions[subscription.ID]; !ok {
		return domainrepo.ErrNotFound
	}
	copied := *subscription
	r.subscriptions[subscription.ID] = &copied
	return nil
}

func (r *fakeRSSRepo) DeleteSubscription(ctx context.Context, id uint) error {
	if _, ok := r.subscriptions[id]; !ok {
		return domainrepo.ErrNotFound
	}
	delete(r.subscriptions, id)
	return nil
}

func (r *fakeRSSRepo) FindSubscriptionByID(ctx context.Context, id uint) (*entity.RSSSubscription, error) {
	subscription, ok := r.subscriptions[id]
	if !ok {
		return nil, domainrepo.ErrNotFound
	}
	copied := *subscription
	return &copied, nil
}

func (r *fakeRSSRepo) ListSubscriptions(ctx context.Context, filter domainrepo.RSSSubscriptionFilter) ([]*entity.RSSSubscription, error) {
	out := []*entity.RSSSubscription{}
	for _, subscription := range r.subscriptions {
		if !filter.IncludeAll && filter.UserID != subscription.UserID {
			continue
		}
		if filter.SourceID != 0 && filter.SourceID != subscription.SourceID {
			continue
		}
		if filter.Enabled != nil && *filter.Enabled != subscription.IsEnabled {
			continue
		}
		copied := *subscription
		out = append(out, &copied)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *fakeRSSRepo) mustCreateItem(item *entity.RSSItem) *entity.RSSItem {
	if err := r.CreateItem(context.Background(), item); err != nil {
		panic(err)
	}
	return item
}

func (r *fakeRSSRepo) CreateItem(ctx context.Context, item *entity.RSSItem) error {
	if item.ID == 0 {
		item.ID = r.nextItemID
		r.nextItemID++
	}
	if item.DedupKey == "" {
		item.DedupKey = fmt.Sprintf("test:%d", item.ID)
	}
	copied := *item
	r.items[item.ID] = &copied
	return nil
}

func (r *fakeRSSRepo) UpdateItem(ctx context.Context, item *entity.RSSItem) error {
	if _, ok := r.items[item.ID]; !ok {
		return domainrepo.ErrNotFound
	}
	copied := *item
	r.items[item.ID] = &copied
	return nil
}

func (r *fakeRSSRepo) FindItemByID(ctx context.Context, id uint) (*entity.RSSItem, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, domainrepo.ErrNotFound
	}
	copied := *item
	return &copied, nil
}

func (r *fakeRSSRepo) FindItemByDedupKey(ctx context.Context, sourceID uint, dedupKey string) (*entity.RSSItem, error) {
	for _, item := range r.items {
		if item.SourceID == sourceID && item.DedupKey == dedupKey {
			copied := *item
			return &copied, nil
		}
	}
	return nil, domainrepo.ErrNotFound
}

func (r *fakeRSSRepo) ListItems(ctx context.Context, filter domainrepo.RSSItemFilter) ([]*entity.RSSItem, error) {
	out := []*entity.RSSItem{}
	for _, item := range r.items {
		if !filter.IncludeAll && filter.UserID != item.UserID {
			continue
		}
		if filter.SourceID != 0 && filter.SourceID != item.SourceID {
			continue
		}
		if filter.SubscriptionID != 0 && (item.MatchedSubscriptionID == nil || *item.MatchedSubscriptionID != filter.SubscriptionID) {
			continue
		}
		if filter.TaskID != 0 && (item.TaskID == nil || *item.TaskID != filter.TaskID) {
			continue
		}
		if filter.Status != "" && filter.Status != item.Status {
			continue
		}
		copied := *item
		out = append(out, &copied)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

type fakeRSSVFSResolver struct{}

func (fakeRSSVFSResolver) ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error) {
	return ResolvedPath{
		VirtualPath: virtualPath,
		InnerPath:   virtualPath,
		Source:      &entity.StorageSource{ID: 7, DriverType: "s3"},
		IsRealMount: true,
	}, nil
}

type fakeRSSFailingVFSResolver struct {
	err error
}

func (f fakeRSSFailingVFSResolver) ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error) {
	if f.err != nil {
		return ResolvedPath{}, f.err
	}
	return ResolvedPath{}, ErrPathInvalid
}

func rssTestAuthContext() context.Context {
	return security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       1,
		Username:     "admin",
		RoleKey:      "super_admin",
		Capabilities: []string{"rss.read", "rss.manage"},
	})
}

func rssUserContext(userID uint, capabilities ...string) context.Context {
	return security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       userID,
		Username:     fmt.Sprintf("user-%d", userID),
		Capabilities: capabilities,
	})
}

func boolPtr(value bool) *bool {
	return &value
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}
