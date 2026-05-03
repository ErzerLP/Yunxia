package repository

import (
	"context"
	"time"

	"yunxia/internal/domain/entity"
)

// NotificationChannelFilter 定义通知通道列表筛选。
type NotificationChannelFilter struct {
	Enabled *bool
}

// NotificationEventFilter 定义通知事件列表筛选。
type NotificationEventFilter struct {
	Status    string
	EventType string
	DueBefore *time.Time
	Limit     int
}

// NotificationRepository 定义通知通道与事件持久化能力。
type NotificationRepository interface {
	CreateChannel(ctx context.Context, channel *entity.NotificationChannel) error
	UpdateChannel(ctx context.Context, channel *entity.NotificationChannel) error
	DeleteChannel(ctx context.Context, id uint) error
	FindChannelByID(ctx context.Context, id uint) (*entity.NotificationChannel, error)
	ListChannels(ctx context.Context, filter NotificationChannelFilter) ([]*entity.NotificationChannel, error)

	CreateEvent(ctx context.Context, event *entity.NotificationEvent) error
	UpdateEvent(ctx context.Context, event *entity.NotificationEvent) error
	FindEventByID(ctx context.Context, id uint) (*entity.NotificationEvent, error)
	ListEvents(ctx context.Context, filter NotificationEventFilter) ([]*entity.NotificationEvent, error)
}
