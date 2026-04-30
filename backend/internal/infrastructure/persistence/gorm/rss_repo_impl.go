package gorm

import (
	"context"
	"encoding/json"
	"errors"

	gormpkg "gorm.io/gorm"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// RSSRepository 提供 RSS 源、订阅和条目仓储实现。
type RSSRepository struct {
	db *gormpkg.DB
}

// NewRSSRepository 创建 RSS 仓储。
func NewRSSRepository(db *gormpkg.DB) *RSSRepository {
	return &RSSRepository{db: db}
}

func (r *RSSRepository) CreateSource(ctx context.Context, source *entity.RSSSource) error {
	model := rssSourceModelFromEntity(source)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	*source = *rssSourceEntityFromModel(model)
	return nil
}

func (r *RSSRepository) UpdateSource(ctx context.Context, source *entity.RSSSource) error {
	model := rssSourceModelFromEntity(source)
	result := r.db.WithContext(ctx).
		Model(&RSSSourceModel{}).
		Where("id = ?", source.ID).
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

func (r *RSSRepository) DeleteSource(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gormpkg.DB) error {
		result := tx.Delete(&RSSSourceModel{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return domainrepo.ErrNotFound
		}
		if err := tx.Where("source_id = ?", id).Delete(&RSSSubscriptionModel{}).Error; err != nil {
			return err
		}
		return tx.Where("source_id = ?", id).Delete(&RSSItemModel{}).Error
	})
}

func (r *RSSRepository) FindSourceByID(ctx context.Context, id uint) (*entity.RSSSource, error) {
	var model RSSSourceModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return rssSourceEntityFromModel(&model), nil
}

func (r *RSSRepository) ListSources(ctx context.Context, filter domainrepo.RSSSourceFilter) ([]*entity.RSSSource, error) {
	query := r.db.WithContext(ctx).Model(&RSSSourceModel{})
	if !filter.IncludeAll {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.Enabled != nil {
		query = query.Where("is_enabled = ?", *filter.Enabled)
	}
	var models []RSSSourceModel
	if err := query.Order("created_at desc, id desc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*entity.RSSSource, 0, len(models))
	for i := range models {
		items = append(items, rssSourceEntityFromModel(&models[i]))
	}
	return items, nil
}

func (r *RSSRepository) CreateSubscription(ctx context.Context, subscription *entity.RSSSubscription) error {
	model, err := rssSubscriptionModelFromEntity(subscription)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	*subscription = *rssSubscriptionEntityFromModel(model)
	return nil
}

func (r *RSSRepository) UpdateSubscription(ctx context.Context, subscription *entity.RSSSubscription) error {
	model, err := rssSubscriptionModelFromEntity(subscription)
	if err != nil {
		return err
	}
	result := r.db.WithContext(ctx).
		Model(&RSSSubscriptionModel{}).
		Where("id = ?", subscription.ID).
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

func (r *RSSRepository) DeleteSubscription(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&RSSSubscriptionModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

func (r *RSSRepository) FindSubscriptionByID(ctx context.Context, id uint) (*entity.RSSSubscription, error) {
	var model RSSSubscriptionModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return rssSubscriptionEntityFromModel(&model), nil
}

func (r *RSSRepository) ListSubscriptions(ctx context.Context, filter domainrepo.RSSSubscriptionFilter) ([]*entity.RSSSubscription, error) {
	query := r.db.WithContext(ctx).Model(&RSSSubscriptionModel{})
	if !filter.IncludeAll {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.SourceID != 0 {
		query = query.Where("source_id = ?", filter.SourceID)
	}
	if filter.Enabled != nil {
		query = query.Where("is_enabled = ?", *filter.Enabled)
	}
	var models []RSSSubscriptionModel
	if err := query.Order("created_at desc, id desc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*entity.RSSSubscription, 0, len(models))
	for i := range models {
		items = append(items, rssSubscriptionEntityFromModel(&models[i]))
	}
	return items, nil
}

func (r *RSSRepository) CreateItem(ctx context.Context, item *entity.RSSItem) error {
	model := rssItemModelFromEntity(item)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	*item = *rssItemEntityFromModel(model)
	return nil
}

func (r *RSSRepository) UpdateItem(ctx context.Context, item *entity.RSSItem) error {
	model := rssItemModelFromEntity(item)
	result := r.db.WithContext(ctx).
		Model(&RSSItemModel{}).
		Where("id = ?", item.ID).
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

func (r *RSSRepository) FindItemByID(ctx context.Context, id uint) (*entity.RSSItem, error) {
	var model RSSItemModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return rssItemEntityFromModel(&model), nil
}

func (r *RSSRepository) FindItemByDedupKey(ctx context.Context, sourceID uint, dedupKey string) (*entity.RSSItem, error) {
	var model RSSItemModel
	if err := r.db.WithContext(ctx).Where("source_id = ? AND dedup_key = ?", sourceID, dedupKey).First(&model).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return rssItemEntityFromModel(&model), nil
}

func (r *RSSRepository) ListItems(ctx context.Context, filter domainrepo.RSSItemFilter) ([]*entity.RSSItem, error) {
	query := r.db.WithContext(ctx).Model(&RSSItemModel{})
	if !filter.IncludeAll {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.SourceID != 0 {
		query = query.Where("source_id = ?", filter.SourceID)
	}
	if filter.SubscriptionID != 0 {
		query = query.Where("matched_subscription_id = ?", filter.SubscriptionID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	var models []RSSItemModel
	if err := query.Order("published_at desc, created_at desc, id desc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*entity.RSSItem, 0, len(models))
	for i := range models {
		items = append(items, rssItemEntityFromModel(&models[i]))
	}
	return items, nil
}

func rssSourceModelFromEntity(source *entity.RSSSource) *RSSSourceModel {
	return &RSSSourceModel{
		ID:                     source.ID,
		UserID:                 source.UserID,
		Name:                   source.Name,
		URL:                    source.URL,
		IsEnabled:              source.IsEnabled,
		RefreshIntervalSeconds: source.RefreshIntervalSeconds,
		LastRefreshedAt:        source.LastRefreshedAt,
		LastError:              source.LastError,
		CreatedAt:              source.CreatedAt,
		UpdatedAt:              source.UpdatedAt,
	}
}

func rssSourceEntityFromModel(model *RSSSourceModel) *entity.RSSSource {
	return &entity.RSSSource{
		ID:                     model.ID,
		UserID:                 model.UserID,
		Name:                   model.Name,
		URL:                    model.URL,
		IsEnabled:              model.IsEnabled,
		RefreshIntervalSeconds: model.RefreshIntervalSeconds,
		LastRefreshedAt:        model.LastRefreshedAt,
		LastError:              model.LastError,
		CreatedAt:              model.CreatedAt,
		UpdatedAt:              model.UpdatedAt,
	}
}

func rssSubscriptionModelFromEntity(subscription *entity.RSSSubscription) (*RSSSubscriptionModel, error) {
	mustContain, err := marshalRSSStringList(subscription.MustContain)
	if err != nil {
		return nil, err
	}
	mustNotContain, err := marshalRSSStringList(subscription.MustNotContain)
	if err != nil {
		return nil, err
	}
	return &RSSSubscriptionModel{
		ID:                      subscription.ID,
		UserID:                  subscription.UserID,
		SourceID:                subscription.SourceID,
		Name:                    subscription.Name,
		IsEnabled:               subscription.IsEnabled,
		MustContainJSON:         mustContain,
		MustNotContainJSON:      mustNotContain,
		UseRegex:                subscription.UseRegex,
		CaseSensitive:           subscription.CaseSensitive,
		TargetVirtualParentPath: subscription.TargetVirtualParentPath,
		ResolvedSourceID:        subscription.ResolvedSourceID,
		ResolvedInnerParentPath: subscription.ResolvedInnerParentPath,
		CreatedAt:               subscription.CreatedAt,
		UpdatedAt:               subscription.UpdatedAt,
	}, nil
}

func rssSubscriptionEntityFromModel(model *RSSSubscriptionModel) *entity.RSSSubscription {
	return &entity.RSSSubscription{
		ID:                      model.ID,
		UserID:                  model.UserID,
		SourceID:                model.SourceID,
		Name:                    model.Name,
		IsEnabled:               model.IsEnabled,
		MustContain:             unmarshalRSSStringList(model.MustContainJSON),
		MustNotContain:          unmarshalRSSStringList(model.MustNotContainJSON),
		UseRegex:                model.UseRegex,
		CaseSensitive:           model.CaseSensitive,
		TargetVirtualParentPath: model.TargetVirtualParentPath,
		ResolvedSourceID:        model.ResolvedSourceID,
		ResolvedInnerParentPath: model.ResolvedInnerParentPath,
		CreatedAt:               model.CreatedAt,
		UpdatedAt:               model.UpdatedAt,
	}
}

func rssItemModelFromEntity(item *entity.RSSItem) *RSSItemModel {
	return &RSSItemModel{
		ID:                    item.ID,
		UserID:                item.UserID,
		SourceID:              item.SourceID,
		Title:                 item.Title,
		Link:                  item.Link,
		PublishedAt:           item.PublishedAt,
		GUID:                  item.GUID,
		DedupKey:              item.DedupKey,
		DownloadURL:           item.DownloadURL,
		LinkType:              item.LinkType,
		Status:                item.Status,
		MatchedSubscriptionID: item.MatchedSubscriptionID,
		TaskID:                item.TaskID,
		ErrorMessage:          item.ErrorMessage,
		CreatedAt:             item.CreatedAt,
		UpdatedAt:             item.UpdatedAt,
	}
}

func rssItemEntityFromModel(model *RSSItemModel) *entity.RSSItem {
	return &entity.RSSItem{
		ID:                    model.ID,
		UserID:                model.UserID,
		SourceID:              model.SourceID,
		Title:                 model.Title,
		Link:                  model.Link,
		PublishedAt:           model.PublishedAt,
		GUID:                  model.GUID,
		DedupKey:              model.DedupKey,
		DownloadURL:           model.DownloadURL,
		LinkType:              model.LinkType,
		Status:                model.Status,
		MatchedSubscriptionID: model.MatchedSubscriptionID,
		TaskID:                model.TaskID,
		ErrorMessage:          model.ErrorMessage,
		CreatedAt:             model.CreatedAt,
		UpdatedAt:             model.UpdatedAt,
	}
}

func marshalRSSStringList(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalRSSStringList(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	if values == nil {
		return []string{}
	}
	return values
}

func normalizeGormNotFound(err error) error {
	if errors.Is(err, gormpkg.ErrRecordNotFound) {
		return domainrepo.ErrNotFound
	}
	return err
}
