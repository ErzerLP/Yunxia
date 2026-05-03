package gorm

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// NotificationRepository 提供通知通道与事件 GORM 实现。
type NotificationRepository struct {
	db *gorm.DB
}

// NewNotificationRepository 创建通知仓储。
func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) CreateChannel(ctx context.Context, channel *entity.NotificationChannel) error {
	model, err := notificationChannelModelFromEntity(channel)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	*channel = *notificationChannelEntityFromModel(model)
	return nil
}

func (r *NotificationRepository) UpdateChannel(ctx context.Context, channel *entity.NotificationChannel) error {
	model, err := notificationChannelModelFromEntity(channel)
	if err != nil {
		return err
	}
	result := r.db.WithContext(ctx).
		Model(&NotificationChannelModel{}).
		Where("id = ?", channel.ID).
		Select("*").
		Omit("ID", "CreatedAt").
		Updates(model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

func (r *NotificationRepository) DeleteChannel(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&NotificationChannelModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

func (r *NotificationRepository) FindChannelByID(ctx context.Context, id uint) (*entity.NotificationChannel, error) {
	var model NotificationChannelModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return notificationChannelEntityFromModel(&model), nil
}

func (r *NotificationRepository) ListChannels(ctx context.Context, filter domainrepo.NotificationChannelFilter) ([]*entity.NotificationChannel, error) {
	query := r.db.WithContext(ctx).Model(&NotificationChannelModel{})
	if filter.Enabled != nil {
		query = query.Where("is_enabled = ?", *filter.Enabled)
	}
	var models []NotificationChannelModel
	if err := query.Order("created_at desc, id desc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*entity.NotificationChannel, 0, len(models))
	for i := range models {
		items = append(items, notificationChannelEntityFromModel(&models[i]))
	}
	return items, nil
}

func (r *NotificationRepository) CreateEvent(ctx context.Context, event *entity.NotificationEvent) error {
	model := notificationEventModelFromEntity(event)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	*event = *notificationEventEntityFromModel(model)
	return nil
}

func (r *NotificationRepository) UpdateEvent(ctx context.Context, event *entity.NotificationEvent) error {
	model := notificationEventModelFromEntity(event)
	result := r.db.WithContext(ctx).
		Model(&NotificationEventModel{}).
		Where("id = ?", event.ID).
		Select("*").
		Omit("ID", "CreatedAt").
		Updates(model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

func (r *NotificationRepository) FindEventByID(ctx context.Context, id uint) (*entity.NotificationEvent, error) {
	var model NotificationEventModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return notificationEventEntityFromModel(&model), nil
}

func (r *NotificationRepository) ListEvents(ctx context.Context, filter domainrepo.NotificationEventFilter) ([]*entity.NotificationEvent, error) {
	query := r.db.WithContext(ctx).Model(&NotificationEventModel{})
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.EventType != "" {
		query = query.Where("event_type = ?", filter.EventType)
	}
	if filter.DueBefore != nil {
		query = query.Where("next_attempt_at IS NULL OR next_attempt_at <= ?", *filter.DueBefore)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var models []NotificationEventModel
	if err := query.Order("created_at desc, id desc").Limit(limit).Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*entity.NotificationEvent, 0, len(models))
	for i := range models {
		items = append(items, notificationEventEntityFromModel(&models[i]))
	}
	return items, nil
}

func notificationChannelModelFromEntity(channel *entity.NotificationChannel) (*NotificationChannelModel, error) {
	eventTypesJSON, err := json.Marshal(channel.EventTypes)
	if err != nil {
		return nil, err
	}
	configJSON, err := json.Marshal(channel.Config)
	if err != nil {
		return nil, err
	}
	return &NotificationChannelModel{
		ID:             channel.ID,
		Name:           channel.Name,
		Type:           channel.Type,
		IsEnabled:      channel.IsEnabled,
		EventTypesJSON: string(eventTypesJSON),
		ConfigJSON:     string(configJSON),
		CreatedAt:      channel.CreatedAt,
		UpdatedAt:      channel.UpdatedAt,
	}, nil
}

func notificationChannelEntityFromModel(model *NotificationChannelModel) *entity.NotificationChannel {
	var eventTypes []string
	if model.EventTypesJSON != "" {
		_ = json.Unmarshal([]byte(model.EventTypesJSON), &eventTypes)
	}
	var config entity.NotificationChannelConfig
	if model.ConfigJSON != "" {
		_ = json.Unmarshal([]byte(model.ConfigJSON), &config)
	}
	return &entity.NotificationChannel{
		ID:         model.ID,
		Name:       model.Name,
		Type:       model.Type,
		IsEnabled:  model.IsEnabled,
		EventTypes: eventTypes,
		Config:     config,
		CreatedAt:  model.CreatedAt,
		UpdatedAt:  model.UpdatedAt,
	}
}

func notificationEventModelFromEntity(event *entity.NotificationEvent) *NotificationEventModel {
	return &NotificationEventModel{
		ID:            event.ID,
		UserID:        event.UserID,
		EventType:     event.EventType,
		Severity:      event.Severity,
		Title:         event.Title,
		Message:       event.Message,
		PayloadJSON:   event.PayloadJSON,
		Status:        event.Status,
		Attempts:      event.Attempts,
		MaxAttempts:   event.MaxAttempts,
		LastAttemptAt: event.LastAttemptAt,
		NextAttemptAt: event.NextAttemptAt,
		DeliveredAt:   event.DeliveredAt,
		LastError:     event.LastError,
		CreatedAt:     event.CreatedAt,
		UpdatedAt:     event.UpdatedAt,
	}
}

func notificationEventEntityFromModel(model *NotificationEventModel) *entity.NotificationEvent {
	return &entity.NotificationEvent{
		ID:            model.ID,
		UserID:        model.UserID,
		EventType:     model.EventType,
		Severity:      model.Severity,
		Title:         model.Title,
		Message:       model.Message,
		PayloadJSON:   model.PayloadJSON,
		Status:        model.Status,
		Attempts:      model.Attempts,
		MaxAttempts:   model.MaxAttempts,
		LastAttemptAt: model.LastAttemptAt,
		NextAttemptAt: model.NextAttemptAt,
		DeliveredAt:   model.DeliveredAt,
		LastError:     model.LastError,
		CreatedAt:     model.CreatedAt,
		UpdatedAt:     model.UpdatedAt,
	}
}
