package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
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
	failedMessage := "temporary network error"
	failedTaskID := tasks.addTask("failed", &failedMessage)
	source := repo.mustCreateSource(&entity.RSSSource{UserID: 1, Name: "s", URL: "https://example/rss.xml", IsEnabled: true, CreatedAt: now, UpdatedAt: now})
	sub := repo.mustCreateSubscription(&entity.RSSSubscription{UserID: 1, SourceID: source.ID, Name: "sub", IsEnabled: true, TargetVirtualParentPath: "/anime", CreatedAt: now, UpdatedAt: now})
	completed := repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "done", DownloadURL: "magnet:?xt=urn:btih:done", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusEnqueued, MatchedSubscriptionID: &sub.ID, TaskID: &completedTaskID, MaxRetryCount: 3, CreatedAt: now, UpdatedAt: now})
	failed := repo.mustCreateItem(&entity.RSSItem{UserID: 1, SourceID: source.ID, Title: "fail", DownloadURL: "magnet:?xt=urn:btih:fail", LinkType: RSSLinkTypeMagnet, Status: RSSItemStatusEnqueued, MatchedSubscriptionID: &sub.ID, TaskID: &failedTaskID, MaxRetryCount: 3, CreatedAt: now, UpdatedAt: now})
	svc := NewRSSService(repo, nil, tasks, WithRSSTaskRepository(tasks), WithRSSNow(func() time.Time { return now }))

	if _, err := svc.RunRetryCycle(context.Background(), 10); err != nil {
		t.Fatalf("RunRetryCycle failed: %v", err)
	}
	completed, _ = repo.FindItemByID(context.Background(), completed.ID)
	failed, _ = repo.FindItemByID(context.Background(), failed.ID)
	if completed.Status != RSSItemStatusCompleted {
		t.Fatalf("completed backlink status = %q", completed.Status)
	}
	if failed.Status != RSSItemStatusRetryPending || failed.RetryReason == nil || *failed.RetryReason != RSSRetryReasonDownloaderUnavailable {
		t.Fatalf("failed backlink item = %#v", failed)
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
	nextID           uint
	createErr        error
	createCalls      int
	lastCreateUserID uint
	tasks            map[uint]*entity.DownloadTask
}

func newFakeRSSTasks() *fakeRSSTasks {
	return &fakeRSSTasks{nextID: 1, tasks: map[uint]*entity.DownloadTask{}}
}

func (f *fakeRSSTasks) Create(ctx context.Context, req appdto.CreateTaskRequest) (*appdto.DownloadTaskView, error) {
	f.createCalls++
	if auth, ok := security.RequestAuthFromContext(ctx); ok {
		f.lastCreateUserID = auth.UserID
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	id := f.nextID
	f.nextID++
	f.tasks[id] = &entity.DownloadTask{ID: id, Status: "pending", SourceURL: req.URL}
	return &appdto.DownloadTaskView{ID: id, Status: "pending", SourceURL: req.URL}, nil
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

func rssTestAuthContext() context.Context {
	return security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       1,
		Username:     "admin",
		RoleKey:      "super_admin",
		Capabilities: []string{"rss.read", "rss.manage"},
	})
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}
