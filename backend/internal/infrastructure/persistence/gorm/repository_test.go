package gorm

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

func TestUserRepositoryPersistsRoleKeyAndStatus(t *testing.T) {
	db, cleanup := testDB(t, filepath.Join(t.TempDir(), "repo.db"))
	defer cleanup()

	repo := NewUserRepository(db)
	user := &entity.User{
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "hash",
		RoleKey:      "operator",
		Status:       "active",
		TokenVersion: 2,
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.FindByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.RoleKey != "operator" || got.Status != "active" {
		t.Fatalf("unexpected user = %+v", got)
	}
}

func TestSystemConfigRepositoryUpsertAndGet(t *testing.T) {
	db, cleanup := testDB(t, filepath.Join(t.TempDir(), "cfg.db"))
	defer cleanup()

	repo := NewSystemConfigRepository(db)
	cfg := &entity.SystemConfig{SiteName: "云匣", MultiUserEnabled: true, DefaultChunkSize: 5 * 1024 * 1024, MaxUploadSize: 10 * 1024 * 1024 * 1024, WebDAVEnabled: true, WebDAVPrefix: "/dav", Theme: "system", Language: "zh-CN", TimeZone: "Asia/Shanghai", UpdatedAt: time.Now()}
	if err := repo.Upsert(context.Background(), cfg); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	got, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.SiteName != "云匣" || !got.MultiUserEnabled {
		t.Fatalf("unexpected config = %+v", got)
	}
}

func TestRefreshTokenRepositoryCreateFindAndRevoke(t *testing.T) {
	db, cleanup := testDB(t, filepath.Join(t.TempDir(), "token.db"))
	defer cleanup()

	repo := NewRefreshTokenRepository(db)
	token := &entity.RefreshToken{UserID: 7, TokenHash: "hash-value", ExpiresAt: time.Now().Add(time.Hour)}
	if err := repo.Create(context.Background(), token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.FindByTokenHash(context.Background(), "hash-value")
	if err != nil {
		t.Fatalf("FindByTokenHash() error = %v", err)
	}
	if got.UserID != 7 {
		t.Fatalf("unexpected token = %+v", got)
	}

	if err := repo.RevokeByTokenHash(context.Background(), "hash-value"); err != nil {
		t.Fatalf("RevokeByTokenHash() error = %v", err)
	}

	revoked, err := repo.FindByTokenHash(context.Background(), "hash-value")
	if err != nil {
		t.Fatalf("FindByTokenHash() error after revoke = %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatalf("expected token to be revoked")
	}
}

func TestRSSRepositoryPersistsTemplatesAndParsedMetadata(t *testing.T) {
	db, cleanup := testDB(t, filepath.Join(t.TempDir(), "rss.db"))
	defer cleanup()

	repo := NewRSSRepository(db)
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	subscription := &entity.RSSSubscription{
		UserID:                  1,
		SourceID:                2,
		Name:                    "anime",
		IsEnabled:               true,
		MustContain:             []string{"Example"},
		TargetVirtualParentPath: "/anime",
		DirectoryTemplate:       "{anime_title}/{season}",
		FilenameTemplate:        "{anime_title} - {episode} [{resolution}]",
		ResolvedSourceID:        7,
		ResolvedInnerParentPath: "/anime",
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := repo.CreateSubscription(context.Background(), subscription); err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	gotSub, err := repo.FindSubscriptionByID(context.Background(), subscription.ID)
	if err != nil {
		t.Fatalf("FindSubscriptionByID() error = %v", err)
	}
	if gotSub.DirectoryTemplate != subscription.DirectoryTemplate || gotSub.FilenameTemplate != subscription.FilenameTemplate {
		t.Fatalf("templates not persisted: %#v", gotSub)
	}
	if gotSub.IsEnabled != subscription.IsEnabled {
		t.Fatalf("is_enabled not persisted: got %v want %v", gotSub.IsEnabled, subscription.IsEnabled)
	}

	item := &entity.RSSItem{
		UserID:      1,
		SourceID:    2,
		Title:       "[SubsPlease] Example Show S01E05 [1080p]",
		Link:        "magnet:?xt=urn:btih:parsed",
		GUID:        "parsed",
		DedupKey:    "guid:parsed",
		DownloadURL: "magnet:?xt=urn:btih:parsed",
		LinkType:    "magnet",
		Parsed: entity.RSSAnimeParsed{
			AnimeTitle:    "Example Show",
			Season:        "S01",
			Episode:       "05",
			SubtitleGroup: "SubsPlease",
			Resolution:    "1080p",
		},
		Status:        "new",
		MaxRetryCount: 3,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.CreateItem(context.Background(), item); err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	gotItem, err := repo.FindItemByDedupKey(context.Background(), item.SourceID, item.DedupKey)
	if err != nil {
		t.Fatalf("FindItemByDedupKey() error = %v", err)
	}
	if gotItem.Parsed.AnimeTitle != "Example Show" || gotItem.Parsed.Season != "S01" || gotItem.Parsed.Episode != "05" || gotItem.Parsed.SubtitleGroup != "SubsPlease" || gotItem.Parsed.Resolution != "1080p" {
		t.Fatalf("parsed metadata not persisted: %#v", gotItem.Parsed)
	}
}

func TestRSSRepositoryCreateSubscriptionPersistsExplicitDisabled(t *testing.T) {
	db, cleanup := testDB(t, filepath.Join(t.TempDir(), "rss-disabled.db"))
	defer cleanup()

	repo := NewRSSRepository(db)
	now := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	subscription := &entity.RSSSubscription{
		UserID:                  1,
		SourceID:                2,
		Name:                    "disabled",
		IsEnabled:               false,
		TargetVirtualParentPath: "/anime",
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := repo.CreateSubscription(context.Background(), subscription); err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	if subscription.IsEnabled {
		t.Fatalf("created subscription returned enabled: %#v", subscription)
	}
	got, err := repo.FindSubscriptionByID(context.Background(), subscription.ID)
	if err != nil {
		t.Fatalf("FindSubscriptionByID() error = %v", err)
	}
	if got.IsEnabled {
		t.Fatalf("stored subscription enabled despite explicit false: %#v", got)
	}
}

func TestNotificationRepositoryPersistsChannelAndEvent(t *testing.T) {
	db, cleanup := testDB(t, filepath.Join(t.TempDir(), "notification.db"))
	defer cleanup()

	repo := NewNotificationRepository(db)
	now := time.Date(2026, 5, 2, 16, 0, 0, 0, time.UTC)
	channel := &entity.NotificationChannel{
		Name:       "ops webhook",
		Type:       "webhook",
		IsEnabled:  true,
		EventTypes: []string{"rss.source_failure"},
		Config: entity.NotificationChannelConfig{
			WebhookURL: "https://example.com/hook",
			Secret:     "secret",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateChannel(context.Background(), channel); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	gotChannel, err := repo.FindChannelByID(context.Background(), channel.ID)
	if err != nil {
		t.Fatalf("FindChannelByID() error = %v", err)
	}
	if gotChannel.Config.WebhookURL != channel.Config.WebhookURL || gotChannel.Config.Secret != "secret" || len(gotChannel.EventTypes) != 1 {
		t.Fatalf("unexpected channel = %#v", gotChannel)
	}

	next := now.Add(5 * time.Minute)
	lastErr := "temporary webhook outage"
	event := &entity.NotificationEvent{
		UserID:        1,
		EventType:     "rss.item_needs_attention",
		Severity:      "error",
		Title:         "needs attention",
		Message:       "download failed",
		PayloadJSON:   `{"item_id":7}`,
		Status:        "retry_pending",
		Attempts:      1,
		MaxAttempts:   3,
		NextAttemptAt: &next,
		LastError:     &lastErr,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.CreateEvent(context.Background(), event); err != nil {
		t.Fatalf("CreateEvent() error = %v", err)
	}
	gotEvent, err := repo.FindEventByID(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("FindEventByID() error = %v", err)
	}
	if gotEvent.EventType != event.EventType || gotEvent.Status != event.Status || gotEvent.NextAttemptAt == nil || gotEvent.LastError == nil {
		t.Fatalf("unexpected event = %#v", gotEvent)
	}
	due, err := repo.ListEvents(context.Background(), domainrepo.NotificationEventFilter{Status: "retry_pending", DueBefore: ptrTimeForNotificationTest(now.Add(10 * time.Minute)), Limit: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(due) != 1 || due[0].ID != event.ID {
		t.Fatalf("unexpected due events = %#v", due)
	}
}

func ptrTimeForNotificationTest(value time.Time) *time.Time {
	return &value
}
