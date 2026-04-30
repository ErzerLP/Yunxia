package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// RSSSourceFilter 定义 RSS 源列表筛选。
type RSSSourceFilter struct {
	UserID     uint
	IncludeAll bool
	Enabled    *bool
}

// RSSSubscriptionFilter 定义 RSS 订阅列表筛选。
type RSSSubscriptionFilter struct {
	UserID     uint
	IncludeAll bool
	SourceID   uint
	Enabled    *bool
}

// RSSItemFilter 定义 RSS 条目列表筛选。
type RSSItemFilter struct {
	UserID         uint
	IncludeAll     bool
	SourceID       uint
	SubscriptionID uint
	Status         string
}

// RSSRepository 定义 RSS 源、订阅和条目仓储能力。
type RSSRepository interface {
	CreateSource(ctx context.Context, source *entity.RSSSource) error
	UpdateSource(ctx context.Context, source *entity.RSSSource) error
	DeleteSource(ctx context.Context, id uint) error
	FindSourceByID(ctx context.Context, id uint) (*entity.RSSSource, error)
	ListSources(ctx context.Context, filter RSSSourceFilter) ([]*entity.RSSSource, error)

	CreateSubscription(ctx context.Context, subscription *entity.RSSSubscription) error
	UpdateSubscription(ctx context.Context, subscription *entity.RSSSubscription) error
	DeleteSubscription(ctx context.Context, id uint) error
	FindSubscriptionByID(ctx context.Context, id uint) (*entity.RSSSubscription, error)
	ListSubscriptions(ctx context.Context, filter RSSSubscriptionFilter) ([]*entity.RSSSubscription, error)

	CreateItem(ctx context.Context, item *entity.RSSItem) error
	UpdateItem(ctx context.Context, item *entity.RSSItem) error
	FindItemByID(ctx context.Context, id uint) (*entity.RSSItem, error)
	FindItemByDedupKey(ctx context.Context, sourceID uint, dedupKey string) (*entity.RSSItem, error)
	ListItems(ctx context.Context, filter RSSItemFilter) ([]*entity.RSSItem, error)
}
