package service

import (
	"context"
	"errors"
	"testing"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

func TestNotificationChannelConfigHidesSecret(t *testing.T) {
	repo := newFakeNotificationRepo()
	svc := NewNotificationService(repo, WithNotificationWebhookSender(&fakeNotificationSender{}), WithNotificationNow(fixedNotificationNow))
	secret := "webhook-secret"
	channel, err := svc.CreateChannel(notificationTestAuthContext(), appdto.NotificationChannelUpsertRequest{
		Name:       "ops webhook",
		Type:       NotificationChannelTypeWebhook,
		IsEnabled:  boolPtr(true),
		EventTypes: []string{NotificationEventRSSSourceFailure},
		Config: appdto.NotificationWebhookConfigRequest{
			URL:    "https://example.com/yunxia",
			Secret: &secret,
		},
	})
	if err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	if channel.Config.URL != "https://example.com/yunxia" || !channel.Config.SecretConfigured {
		t.Fatalf("unexpected channel view = %#v", channel)
	}
	list, err := svc.ListChannels(notificationTestAuthContext())
	if err != nil {
		t.Fatalf("ListChannels() error = %v", err)
	}
	if len(list.Items) != 1 || !list.Items[0].Config.SecretConfigured || len(list.Items[0].EventTypes) != 1 {
		t.Fatalf("unexpected channel list = %#v", list)
	}
}

func TestNotificationDispatchFailureCanRetry(t *testing.T) {
	repo := newFakeNotificationRepo()
	sender := &fakeNotificationSender{err: errors.New("temporary webhook outage")}
	svc := NewNotificationService(repo, WithNotificationWebhookSender(sender), WithNotificationNow(fixedNotificationNow))
	if _, err := svc.CreateChannel(notificationTestAuthContext(), appdto.NotificationChannelUpsertRequest{
		Name:      "ops webhook",
		Type:      NotificationChannelTypeWebhook,
		IsEnabled: boolPtr(true),
		Config:    appdto.NotificationWebhookConfigRequest{URL: "https://example.com/yunxia"},
	}); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}

	event, err := svc.Notify(context.Background(), NotificationEventInput{
		UserID:    1,
		EventType: NotificationEventRSSItemNeedsAttention,
		Severity:  NotificationSeverityError,
		Title:     "needs attention",
		Message:   "download failed",
		Payload:   map[string]any{"item_id": uint(7)},
	})
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	stored, _ := repo.FindEventByID(context.Background(), event.ID)
	if stored.Status != NotificationStatusRetryPending || stored.Attempts != 1 || stored.NextAttemptAt == nil || stored.LastError == nil {
		t.Fatalf("event after failed dispatch = %#v", stored)
	}
	if sender.calls != 1 {
		t.Fatalf("sender calls after notify = %d", sender.calls)
	}

	sender.err = nil
	view, err := svc.RetryEvent(notificationTestAuthContext(), event.ID)
	if err != nil {
		t.Fatalf("RetryEvent() error = %v", err)
	}
	if view.Status != NotificationStatusDelivered || view.Attempts != 2 || view.DeliveredAt == nil || view.LastError != nil {
		t.Fatalf("event after retry = %#v", view)
	}
	if sender.calls != 2 {
		t.Fatalf("sender calls after retry = %d", sender.calls)
	}
}

func TestNotificationEventTypeFilterSkipsUnmatchedChannels(t *testing.T) {
	repo := newFakeNotificationRepo()
	sender := &fakeNotificationSender{}
	svc := NewNotificationService(repo, WithNotificationWebhookSender(sender), WithNotificationNow(fixedNotificationNow))
	if _, err := svc.CreateChannel(notificationTestAuthContext(), appdto.NotificationChannelUpsertRequest{
		Name:       "source only",
		Type:       NotificationChannelTypeWebhook,
		IsEnabled:  boolPtr(true),
		EventTypes: []string{NotificationEventRSSSourceFailure},
		Config:     appdto.NotificationWebhookConfigRequest{URL: "https://example.com/source"},
	}); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	event, err := svc.Notify(context.Background(), NotificationEventInput{
		EventType: NotificationEventRSSDownloadCompleted,
		Severity:  NotificationSeverityInfo,
		Title:     "done",
		Message:   "download completed",
	})
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if event.Status != NotificationStatusSkipped || sender.calls != 0 {
		t.Fatalf("expected skipped event and no send, event=%#v calls=%d", event, sender.calls)
	}
}

func fixedNotificationNow() time.Time {
	return time.Date(2026, 5, 2, 16, 0, 0, 0, time.UTC)
}

func notificationTestAuthContext() context.Context {
	return security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 1, RoleKey: "admin", Status: "active"})
}

type fakeNotificationSender struct {
	err   error
	calls int
	last  NotificationWebhookPayload
}

func (s *fakeNotificationSender) Send(_ context.Context, _ NotificationWebhookEndpoint, payload NotificationWebhookPayload) error {
	s.calls++
	s.last = payload
	return s.err
}

type fakeNotificationRepo struct {
	nextChannelID uint
	nextEventID   uint
	channels      map[uint]*entity.NotificationChannel
	events        map[uint]*entity.NotificationEvent
}

func newFakeNotificationRepo() *fakeNotificationRepo {
	return &fakeNotificationRepo{nextChannelID: 1, nextEventID: 1, channels: map[uint]*entity.NotificationChannel{}, events: map[uint]*entity.NotificationEvent{}}
}

func (r *fakeNotificationRepo) CreateChannel(_ context.Context, channel *entity.NotificationChannel) error {
	clone := cloneNotificationChannel(channel)
	clone.ID = r.nextChannelID
	r.nextChannelID++
	r.channels[clone.ID] = clone
	*channel = *cloneNotificationChannel(clone)
	return nil
}

func (r *fakeNotificationRepo) UpdateChannel(_ context.Context, channel *entity.NotificationChannel) error {
	if _, ok := r.channels[channel.ID]; !ok {
		return domainrepo.ErrNotFound
	}
	r.channels[channel.ID] = cloneNotificationChannel(channel)
	return nil
}

func (r *fakeNotificationRepo) DeleteChannel(_ context.Context, id uint) error {
	if _, ok := r.channels[id]; !ok {
		return domainrepo.ErrNotFound
	}
	delete(r.channels, id)
	return nil
}

func (r *fakeNotificationRepo) FindChannelByID(_ context.Context, id uint) (*entity.NotificationChannel, error) {
	channel, ok := r.channels[id]
	if !ok {
		return nil, domainrepo.ErrNotFound
	}
	return cloneNotificationChannel(channel), nil
}

func (r *fakeNotificationRepo) ListChannels(_ context.Context, filter domainrepo.NotificationChannelFilter) ([]*entity.NotificationChannel, error) {
	items := make([]*entity.NotificationChannel, 0, len(r.channels))
	for _, channel := range r.channels {
		if filter.Enabled != nil && channel.IsEnabled != *filter.Enabled {
			continue
		}
		items = append(items, cloneNotificationChannel(channel))
	}
	return items, nil
}

func (r *fakeNotificationRepo) CreateEvent(_ context.Context, event *entity.NotificationEvent) error {
	clone := cloneNotificationEvent(event)
	clone.ID = r.nextEventID
	r.nextEventID++
	r.events[clone.ID] = clone
	*event = *cloneNotificationEvent(clone)
	return nil
}

func (r *fakeNotificationRepo) UpdateEvent(_ context.Context, event *entity.NotificationEvent) error {
	if _, ok := r.events[event.ID]; !ok {
		return domainrepo.ErrNotFound
	}
	r.events[event.ID] = cloneNotificationEvent(event)
	return nil
}

func (r *fakeNotificationRepo) FindEventByID(_ context.Context, id uint) (*entity.NotificationEvent, error) {
	event, ok := r.events[id]
	if !ok {
		return nil, domainrepo.ErrNotFound
	}
	return cloneNotificationEvent(event), nil
}

func (r *fakeNotificationRepo) ListEvents(_ context.Context, filter domainrepo.NotificationEventFilter) ([]*entity.NotificationEvent, error) {
	items := make([]*entity.NotificationEvent, 0, len(r.events))
	for _, event := range r.events {
		if filter.Status != "" && event.Status != filter.Status {
			continue
		}
		if filter.EventType != "" && event.EventType != filter.EventType {
			continue
		}
		if filter.DueBefore != nil && event.NextAttemptAt != nil && event.NextAttemptAt.After(*filter.DueBefore) {
			continue
		}
		items = append(items, cloneNotificationEvent(event))
		if filter.Limit > 0 && len(items) >= filter.Limit {
			break
		}
	}
	return items, nil
}

func cloneNotificationChannel(channel *entity.NotificationChannel) *entity.NotificationChannel {
	if channel == nil {
		return nil
	}
	clone := *channel
	clone.EventTypes = append([]string{}, channel.EventTypes...)
	return &clone
}

func cloneNotificationEvent(event *entity.NotificationEvent) *entity.NotificationEvent {
	if event == nil {
		return nil
	}
	clone := *event
	return &clone
}
