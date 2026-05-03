package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/observability/logging"
	"yunxia/internal/infrastructure/security"
)

const (
	NotificationChannelTypeWebhook = "webhook"

	NotificationEventRSSSourceFailure      = "rss.source_failure"
	NotificationEventRSSItemNeedsAttention = "rss.item_needs_attention"
	NotificationEventRSSDownloadCompleted  = "rss.download_completed"
	NotificationEventTest                  = "notification.test"

	NotificationSeverityInfo    = "info"
	NotificationSeverityWarning = "warning"
	NotificationSeverityError   = "error"

	NotificationStatusPending      = "pending"
	NotificationStatusDelivered    = "delivered"
	NotificationStatusRetryPending = "retry_pending"
	NotificationStatusFailed       = "failed"
	NotificationStatusSkipped      = "skipped"

	defaultNotificationMaxAttempts = 3
)

var (
	// ErrNotificationChannelUnsupported 表示通知通道类型不支持。
	ErrNotificationChannelUnsupported = errors.New("notification channel unsupported")
	// ErrNotificationDeliveryFailed 表示通知投递失败。
	ErrNotificationDeliveryFailed = errors.New("notification delivery failed")
)

var supportedNotificationEvents = map[string]struct{}{
	NotificationEventRSSSourceFailure:      {},
	NotificationEventRSSItemNeedsAttention: {},
	NotificationEventRSSDownloadCompleted:  {},
}

// NotificationWebhookEndpoint 表示一次 webhook 投递的目标信息。
type NotificationWebhookEndpoint struct {
	URL     string
	Secret  string
	Timeout time.Duration
}

// NotificationWebhookPayload 表示 webhook 投递载荷。
type NotificationWebhookPayload struct {
	EventID   uint           `json:"event_id"`
	EventType string         `json:"event_type"`
	Severity  string         `json:"severity"`
	Title     string         `json:"title"`
	Message   string         `json:"message"`
	Payload   map[string]any `json:"payload"`
	CreatedAt string         `json:"created_at"`
}

// NotificationWebhookSender 定义 webhook 投递所需的最小能力。
type NotificationWebhookSender interface {
	Send(ctx context.Context, endpoint NotificationWebhookEndpoint, payload NotificationWebhookPayload) error
}

// NotificationEventInput 表示业务模块提交的通知事件。
type NotificationEventInput struct {
	UserID    uint
	EventType string
	Severity  string
	Title     string
	Message   string
	Payload   map[string]any
}

// NotificationService 负责通知通道、事件和 webhook 投递编排。
type NotificationService struct {
	repo          domainrepo.NotificationRepository
	webhookSender NotificationWebhookSender
	logger        *slog.Logger
	now           func() time.Time
}

// NotificationServiceOption 定义通知服务可选能力。
type NotificationServiceOption func(*NotificationService)

// WithNotificationWebhookSender 注入 webhook 投递器。
func WithNotificationWebhookSender(sender NotificationWebhookSender) NotificationServiceOption {
	return func(s *NotificationService) {
		s.webhookSender = sender
	}
}

// WithNotificationLogger 注入通知服务 logger。
func WithNotificationLogger(logger *slog.Logger) NotificationServiceOption {
	return func(s *NotificationService) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// WithNotificationNow 注入时间源，便于测试。
func WithNotificationNow(now func() time.Time) NotificationServiceOption {
	return func(s *NotificationService) {
		if now != nil {
			s.now = now
		}
	}
}

// NewNotificationService 创建通知服务。
func NewNotificationService(repo domainrepo.NotificationRepository, options ...NotificationServiceOption) *NotificationService {
	service := &NotificationService{
		repo:   repo,
		logger: logging.Component(slog.Default(), "service.notification"),
		now:    time.Now,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// ListChannels 返回通知通道列表。
func (s *NotificationService) ListChannels(ctx context.Context) (*appdto.NotificationChannelListResponse, error) {
	if _, err := currentNotificationAuth(ctx); err != nil {
		return nil, err
	}
	channels, err := s.repo.ListChannels(ctx, domainrepo.NotificationChannelFilter{})
	if err != nil {
		return nil, err
	}
	views := make([]appdto.NotificationChannelView, 0, len(channels))
	for _, channel := range channels {
		views = append(views, toNotificationChannelView(channel))
	}
	return &appdto.NotificationChannelListResponse{Items: views}, nil
}

// CreateChannel 创建通知通道。
func (s *NotificationService) CreateChannel(ctx context.Context, req appdto.NotificationChannelUpsertRequest) (*appdto.NotificationChannelView, error) {
	if _, err := currentNotificationAuth(ctx); err != nil {
		return nil, err
	}
	name, channelType, eventTypes, config, err := normalizeNotificationChannelInput(req, nil)
	if err != nil {
		return nil, err
	}
	now := s.now()
	channel := &entity.NotificationChannel{
		Name:       name,
		Type:       channelType,
		IsEnabled:  boolDefault(req.IsEnabled, true),
		EventTypes: eventTypes,
		Config:     config,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repo.CreateChannel(ctx, channel); err != nil {
		return nil, err
	}
	view := toNotificationChannelView(channel)
	return &view, nil
}

// UpdateChannel 更新通知通道。
func (s *NotificationService) UpdateChannel(ctx context.Context, id uint, req appdto.NotificationChannelUpsertRequest) (*appdto.NotificationChannelView, error) {
	if _, err := currentNotificationAuth(ctx); err != nil {
		return nil, err
	}
	channel, err := s.repo.FindChannelByID(ctx, id)
	if err != nil {
		return nil, err
	}
	name, channelType, eventTypes, config, err := normalizeNotificationChannelInput(req, &channel.Config)
	if err != nil {
		return nil, err
	}
	channel.Name = name
	channel.Type = channelType
	if req.IsEnabled != nil {
		channel.IsEnabled = *req.IsEnabled
	}
	channel.EventTypes = eventTypes
	channel.Config = config
	channel.UpdatedAt = s.now()
	if err := s.repo.UpdateChannel(ctx, channel); err != nil {
		return nil, err
	}
	view := toNotificationChannelView(channel)
	return &view, nil
}

// DeleteChannel 删除通知通道。
func (s *NotificationService) DeleteChannel(ctx context.Context, id uint) error {
	if _, err := currentNotificationAuth(ctx); err != nil {
		return err
	}
	return s.repo.DeleteChannel(ctx, id)
}

// TestChannel 立即向指定通道发送测试通知，不落事件表。
func (s *NotificationService) TestChannel(ctx context.Context, id uint) (*appdto.NotificationTestResponse, error) {
	if _, err := currentNotificationAuth(ctx); err != nil {
		return nil, err
	}
	channel, err := s.repo.FindChannelByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if channel.Type != NotificationChannelTypeWebhook || s.webhookSender == nil {
		return nil, ErrNotificationChannelUnsupported
	}
	payload := NotificationWebhookPayload{
		EventType: NotificationEventTest,
		Severity:  NotificationSeverityInfo,
		Title:     "Yunxia notification test",
		Message:   "This is a test notification from Yunxia.",
		Payload: map[string]any{
			"channel_id": channel.ID,
			"channel":    channel.Name,
		},
		CreatedAt: s.now().Format(time.RFC3339),
	}
	if err := s.webhookSender.Send(ctx, notificationWebhookEndpoint(channel), payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotificationDeliveryFailed, err)
	}
	return &appdto.NotificationTestResponse{OK: true}, nil
}

// ListEvents 返回通知事件列表。
func (s *NotificationService) ListEvents(ctx context.Context, filter domainrepo.NotificationEventFilter) (*appdto.NotificationEventListResponse, error) {
	if _, err := currentNotificationAuth(ctx); err != nil {
		return nil, err
	}
	if filter.Status != "" && !isValidNotificationStatus(filter.Status) {
		return nil, ErrConfigInvalid
	}
	if filter.EventType != "" {
		if _, ok := supportedNotificationEvents[filter.EventType]; !ok {
			return nil, ErrConfigInvalid
		}
	}
	events, err := s.repo.ListEvents(ctx, filter)
	if err != nil {
		return nil, err
	}
	views := make([]appdto.NotificationEventView, 0, len(events))
	for _, event := range events {
		views = append(views, toNotificationEventView(event))
	}
	return &appdto.NotificationEventListResponse{Items: views}, nil
}

// RetryEvent 手动重试一个失败或待重试通知事件。
func (s *NotificationService) RetryEvent(ctx context.Context, id uint) (*appdto.NotificationEventView, error) {
	if _, err := currentNotificationAuth(ctx); err != nil {
		return nil, err
	}
	event, err := s.repo.FindEventByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if event.Status == NotificationStatusDelivered || event.Status == NotificationStatusSkipped {
		return nil, ErrTaskInvalidState
	}
	if err := s.dispatchEvent(ctx, event, true); err != nil {
		return nil, err
	}
	view := toNotificationEventView(event)
	return &view, nil
}

// Notify 持久化并尝试投递业务通知事件。
func (s *NotificationService) Notify(ctx context.Context, input NotificationEventInput) (*entity.NotificationEvent, error) {
	if s == nil || s.repo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	if _, ok := supportedNotificationEvents[input.EventType]; !ok {
		return nil, ErrConfigInvalid
	}
	now := s.now()
	payloadJSON, err := marshalNotificationPayload(input.Payload)
	if err != nil {
		return nil, err
	}
	event := &entity.NotificationEvent{
		UserID:      input.UserID,
		EventType:   input.EventType,
		Severity:    normalizeNotificationSeverity(input.Severity),
		Title:       strings.TrimSpace(input.Title),
		Message:     strings.TrimSpace(input.Message),
		PayloadJSON: payloadJSON,
		Status:      NotificationStatusPending,
		MaxAttempts: defaultNotificationMaxAttempts,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if event.Title == "" || event.Message == "" {
		return nil, ErrConfigInvalid
	}
	if err := s.repo.CreateEvent(ctx, event); err != nil {
		return nil, err
	}
	if err := s.dispatchEvent(ctx, event, false); err != nil {
		s.logger.Warn("notification dispatch failed", slog.String("event", "notification.dispatch.failed"), slog.Uint64("notification_event_id", uint64(event.ID)), slog.Any("error", err))
	}
	return event, nil
}

// ProcessDueEvents 处理待重试通知事件。
func (s *NotificationService) ProcessDueEvents(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 20
	}
	now := s.now()
	statuses := []string{NotificationStatusPending, NotificationStatusRetryPending}
	processed := 0
	var joined error
	for _, status := range statuses {
		if processed >= limit {
			break
		}
		events, err := s.repo.ListEvents(ctx, domainrepo.NotificationEventFilter{Status: status, DueBefore: &now, Limit: limit - processed})
		if err != nil {
			return processed, err
		}
		for _, event := range events {
			if processed >= limit {
				break
			}
			if err := s.dispatchEvent(ctx, event, false); err != nil {
				joined = errors.Join(joined, err)
			}
			processed++
		}
	}
	return processed, joined
}

// StartRetryWorker 定时处理通知重试。
func (s *NotificationService) StartRetryWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.ProcessDueEvents(ctx, 20); err != nil {
				s.logger.Warn("notification retry worker failed", slog.String("event", "notification.retry.failed"), slog.Any("error", err))
			}
		}
	}
}

func (s *NotificationService) dispatchEvent(ctx context.Context, event *entity.NotificationEvent, manual bool) error {
	if event == nil {
		return ErrConfigInvalid
	}
	enabled := true
	channels, err := s.repo.ListChannels(ctx, domainrepo.NotificationChannelFilter{Enabled: &enabled})
	if err != nil {
		return err
	}
	matched := make([]*entity.NotificationChannel, 0, len(channels))
	for _, channel := range channels {
		if notificationChannelMatchesEvent(channel, event.EventType) {
			matched = append(matched, channel)
		}
	}
	now := s.now()
	if len(matched) == 0 {
		event.Status = NotificationStatusSkipped
		event.UpdatedAt = now
		event.NextAttemptAt = nil
		return s.repo.UpdateEvent(ctx, event)
	}
	payload := notificationWebhookPayloadFromEvent(event)
	event.Attempts++
	event.LastAttemptAt = &now
	var joined error
	for _, channel := range matched {
		if channel.Type != NotificationChannelTypeWebhook || s.webhookSender == nil {
			joined = errors.Join(joined, ErrNotificationChannelUnsupported)
			continue
		}
		if sendErr := s.webhookSender.Send(ctx, notificationWebhookEndpoint(channel), payload); sendErr != nil {
			joined = errors.Join(joined, sendErr)
		}
	}
	if joined == nil {
		event.Status = NotificationStatusDelivered
		event.DeliveredAt = &now
		event.NextAttemptAt = nil
		event.LastError = nil
	} else {
		message := joined.Error()
		event.LastError = &message
		if event.Attempts >= event.MaxAttempts && !manual {
			event.Status = NotificationStatusFailed
			event.NextAttemptAt = nil
		} else {
			event.Status = NotificationStatusRetryPending
			next := now.Add(notificationRetryBackoff(event.Attempts))
			event.NextAttemptAt = &next
		}
	}
	event.UpdatedAt = now
	if err := s.repo.UpdateEvent(ctx, event); err != nil {
		return err
	}
	if joined != nil {
		return fmt.Errorf("%w: %v", ErrNotificationDeliveryFailed, joined)
	}
	return nil
}

func currentNotificationAuth(ctx context.Context) (security.RequestAuth, error) {
	auth, ok := security.RequestAuthFromContext(ctx)
	if !ok {
		return security.RequestAuth{}, ErrPermissionDenied
	}
	if auth.Status != "" && auth.Status != permission.StatusActive {
		return security.RequestAuth{}, ErrPermissionDenied
	}
	return auth, nil
}

func normalizeNotificationChannelInput(req appdto.NotificationChannelUpsertRequest, existing *entity.NotificationChannelConfig) (string, string, []string, entity.NotificationChannelConfig, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return "", "", nil, entity.NotificationChannelConfig{}, ErrConfigInvalid
	}
	channelType := strings.TrimSpace(req.Type)
	if channelType != NotificationChannelTypeWebhook {
		return "", "", nil, entity.NotificationChannelConfig{}, ErrNotificationChannelUnsupported
	}
	eventTypes, err := normalizeNotificationEventTypes(req.EventTypes)
	if err != nil {
		return "", "", nil, entity.NotificationChannelConfig{}, err
	}
	config := entity.NotificationChannelConfig{}
	if existing != nil {
		config = *existing
	}
	webhookURL := strings.TrimSpace(req.Config.URL)
	if webhookURL == "" && existing != nil {
		webhookURL = existing.WebhookURL
	}
	if err := validateNotificationWebhookURL(webhookURL); err != nil {
		return "", "", nil, entity.NotificationChannelConfig{}, err
	}
	config.WebhookURL = webhookURL
	if req.Config.Secret != nil {
		config.Secret = strings.TrimSpace(*req.Config.Secret)
	}
	return name, channelType, eventTypes, config, nil
}

func normalizeNotificationEventTypes(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := supportedNotificationEvents[trimmed]; !ok {
			return nil, ErrConfigInvalid
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out, nil
}

func validateNotificationWebhookURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ErrConfigInvalid
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrConfigInvalid
	}
	return nil
}

func notificationChannelMatchesEvent(channel *entity.NotificationChannel, eventType string) bool {
	if channel == nil || !channel.IsEnabled {
		return false
	}
	if len(channel.EventTypes) == 0 {
		return true
	}
	for _, item := range channel.EventTypes {
		if item == eventType {
			return true
		}
	}
	return false
}

func notificationWebhookEndpoint(channel *entity.NotificationChannel) NotificationWebhookEndpoint {
	return NotificationWebhookEndpoint{
		URL:     channel.Config.WebhookURL,
		Secret:  channel.Config.Secret,
		Timeout: 10 * time.Second,
	}
}

func notificationWebhookPayloadFromEvent(event *entity.NotificationEvent) NotificationWebhookPayload {
	return NotificationWebhookPayload{
		EventID:   event.ID,
		EventType: event.EventType,
		Severity:  event.Severity,
		Title:     event.Title,
		Message:   event.Message,
		Payload:   decodeNotificationPayload(event.PayloadJSON),
		CreatedAt: event.CreatedAt.Format(time.RFC3339),
	}
}

func marshalNotificationPayload(payload map[string]any) (string, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeNotificationPayload(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func notificationRetryBackoff(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return 5 * time.Minute
	case attempts == 2:
		return 30 * time.Minute
	default:
		return 2 * time.Hour
	}
}

func normalizeNotificationSeverity(value string) string {
	switch strings.TrimSpace(value) {
	case NotificationSeverityInfo, NotificationSeverityWarning, NotificationSeverityError:
		return strings.TrimSpace(value)
	default:
		return NotificationSeverityInfo
	}
}

func isValidNotificationStatus(status string) bool {
	switch status {
	case NotificationStatusPending, NotificationStatusDelivered, NotificationStatusRetryPending, NotificationStatusFailed, NotificationStatusSkipped:
		return true
	default:
		return false
	}
}

func formatNotificationTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(time.RFC3339)
	return &formatted
}

func toNotificationChannelView(channel *entity.NotificationChannel) appdto.NotificationChannelView {
	return appdto.NotificationChannelView{
		ID:         channel.ID,
		Name:       channel.Name,
		Type:       channel.Type,
		IsEnabled:  channel.IsEnabled,
		EventTypes: append([]string{}, channel.EventTypes...),
		Config: appdto.NotificationChannelConfigView{
			URL:              channel.Config.WebhookURL,
			SecretConfigured: strings.TrimSpace(channel.Config.Secret) != "",
		},
		CreatedAt: channel.CreatedAt.Format(time.RFC3339),
		UpdatedAt: channel.UpdatedAt.Format(time.RFC3339),
	}
}

func toNotificationEventView(event *entity.NotificationEvent) appdto.NotificationEventView {
	return appdto.NotificationEventView{
		ID:            event.ID,
		UserID:        event.UserID,
		EventType:     event.EventType,
		Severity:      event.Severity,
		Title:         event.Title,
		Message:       event.Message,
		Payload:       decodeNotificationPayload(event.PayloadJSON),
		Status:        event.Status,
		Attempts:      event.Attempts,
		MaxAttempts:   event.MaxAttempts,
		LastAttemptAt: formatNotificationTime(event.LastAttemptAt),
		NextAttemptAt: formatNotificationTime(event.NextAttemptAt),
		DeliveredAt:   formatNotificationTime(event.DeliveredAt),
		LastError:     event.LastError,
		CreatedAt:     event.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     event.UpdatedAt.Format(time.RFC3339),
	}
}
