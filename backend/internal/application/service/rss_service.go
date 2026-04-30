package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

const (
	RSSItemStatusNew         = "new"
	RSSItemStatusUnsupported = "unsupported"
	RSSItemStatusIgnored     = "ignored"
	RSSItemStatusMatched     = "matched"
	RSSItemStatusEnqueued    = "enqueued"
	RSSItemStatusFailed      = "failed"
)

const defaultRSSRefreshIntervalSeconds = 1800

// RSSFetchedItem 表示 RSS fetcher 解析出的原始条目。
type RSSFetchedItem struct {
	Title       string
	Link        string
	GUID        string
	PublishedAt *time.Time
	Enclosures  []string
}

// RSSFetcher 定义 RSS 抓取解析能力。
type RSSFetcher interface {
	Fetch(ctx context.Context, rawURL string) ([]RSSFetchedItem, error)
}

// QBitHealthChecker 定义 qBittorrent 健康检查能力。
type QBitHealthChecker interface {
	Health(ctx context.Context) error
}

type rssTaskCreator interface {
	Create(ctx context.Context, req appdto.CreateTaskRequest) (*appdto.DownloadTaskView, error)
}

// RSSService 负责 RSS 源、订阅、条目和下载入队流程。
type RSSService struct {
	rssRepo       domainrepo.RSSRepository
	sourceRepo    domainrepo.SourceRepository
	userRepo      domainrepo.UserRepository
	taskCreator   rssTaskCreator
	fetcher       RSSFetcher
	healthChecker QBitHealthChecker
	vfsResolver   interface {
		ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error)
	}
	aclAuthorizer *ACLAuthorizer
	now           func() time.Time
	logger        *slog.Logger
}

// RSSServiceOption 定义 RSSService 的可选配置。
type RSSServiceOption func(*RSSService)

// WithRSSFetcher 注入 RSS 抓取器。
func WithRSSFetcher(fetcher RSSFetcher) RSSServiceOption {
	return func(s *RSSService) {
		s.fetcher = fetcher
	}
}

// WithRSSVFSResolver 注入统一虚拟目录写入解析器。
func WithRSSVFSResolver(resolver interface {
	ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error)
}) RSSServiceOption {
	return func(s *RSSService) {
		s.vfsResolver = resolver
	}
}

// WithRSSACLAuthorizer 注入 ACL 判定器。
func WithRSSACLAuthorizer(authorizer *ACLAuthorizer) RSSServiceOption {
	return func(s *RSSService) {
		s.aclAuthorizer = authorizer
	}
}

// WithRSSUserRepository 注入用户仓储，用于后台刷新恢复订阅所有者身份。
func WithRSSUserRepository(repo domainrepo.UserRepository) RSSServiceOption {
	return func(s *RSSService) {
		s.userRepo = repo
	}
}

// WithRSSQBitHealthChecker 注入 qBittorrent 健康检查器。
func WithRSSQBitHealthChecker(checker QBitHealthChecker) RSSServiceOption {
	return func(s *RSSService) {
		s.healthChecker = checker
	}
}

// WithRSSNow 覆盖当前时间，主要用于测试。
func WithRSSNow(now func() time.Time) RSSServiceOption {
	return func(s *RSSService) {
		if now != nil {
			s.now = now
		}
	}
}

// NewRSSService 创建 RSS 服务。
func NewRSSService(rssRepo domainrepo.RSSRepository, sourceRepo domainrepo.SourceRepository, taskCreator rssTaskCreator, options ...RSSServiceOption) *RSSService {
	service := &RSSService{
		rssRepo:     rssRepo,
		sourceRepo:  sourceRepo,
		taskCreator: taskCreator,
		now:         time.Now,
		logger:      newServiceLogger("service.rss"),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// ListSources 返回 RSS 源列表。
func (s *RSSService) ListSources(ctx context.Context) (*appdto.RSSSourceListResponse, error) {
	auth, err := currentRSSAuth(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.rssRepo.ListSources(ctx, domainrepo.RSSSourceFilter{
		UserID:     auth.UserID,
		IncludeAll: permission.HasCapability(auth.Capabilities, permission.CapabilityRSSManage),
	})
	if err != nil {
		return nil, err
	}
	views := make([]appdto.RSSSourceView, 0, len(items))
	for _, item := range items {
		views = append(views, toRSSSourceView(item))
	}
	return &appdto.RSSSourceListResponse{Items: views}, nil
}

// CreateSource 创建 RSS 源。
func (s *RSSService) CreateSource(ctx context.Context, req appdto.RSSSourceUpsertRequest) (*appdto.RSSSourceView, error) {
	auth, err := currentRSSAuth(ctx)
	if err != nil {
		return nil, err
	}
	name, rawURL, interval, err := normalizeRSSSourceInput(req)
	if err != nil {
		return nil, err
	}
	enabled := true
	if req.IsEnabled != nil {
		enabled = *req.IsEnabled
	}
	now := s.now()
	source := &entity.RSSSource{
		UserID:                 auth.UserID,
		Name:                   name,
		URL:                    rawURL,
		IsEnabled:              enabled,
		RefreshIntervalSeconds: interval,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if err := s.rssRepo.CreateSource(ctx, source); err != nil {
		return nil, err
	}
	view := toRSSSourceView(source)
	return &view, nil
}

// GetSource 返回 RSS 源详情。
func (s *RSSService) GetSource(ctx context.Context, id uint) (*appdto.RSSSourceView, error) {
	source, err := s.rssRepo.FindSourceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, source.UserID); err != nil {
		return nil, err
	}
	view := toRSSSourceView(source)
	return &view, nil
}

// UpdateSource 更新 RSS 源。
func (s *RSSService) UpdateSource(ctx context.Context, id uint, req appdto.RSSSourceUpsertRequest) (*appdto.RSSSourceView, error) {
	source, err := s.rssRepo.FindSourceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, source.UserID); err != nil {
		return nil, err
	}
	name, rawURL, interval, err := normalizeRSSSourceInput(req)
	if err != nil {
		return nil, err
	}
	source.Name = name
	source.URL = rawURL
	if req.IsEnabled != nil {
		source.IsEnabled = *req.IsEnabled
	}
	source.RefreshIntervalSeconds = interval
	source.UpdatedAt = s.now()
	if err := s.rssRepo.UpdateSource(ctx, source); err != nil {
		return nil, err
	}
	view := toRSSSourceView(source)
	return &view, nil
}

// DeleteSource 删除 RSS 源及其订阅/条目。
func (s *RSSService) DeleteSource(ctx context.Context, id uint) error {
	source, err := s.rssRepo.FindSourceByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.authorizeRSSOwner(ctx, source.UserID); err != nil {
		return err
	}
	return s.rssRepo.DeleteSource(ctx, id)
}

// RefreshSource 手动刷新单个 RSS 源。
func (s *RSSService) RefreshSource(ctx context.Context, id uint) (*appdto.RSSRefreshResponse, error) {
	source, err := s.rssRepo.FindSourceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, source.UserID); err != nil {
		return nil, err
	}
	return s.refreshSourceEntity(ctx, source)
}

// RefreshDueSources 刷新到期的启用 RSS 源。
func (s *RSSService) RefreshDueSources(ctx context.Context) error {
	enabled := true
	sources, err := s.rssRepo.ListSources(ctx, domainrepo.RSSSourceFilter{IncludeAll: true, Enabled: &enabled})
	if err != nil {
		return err
	}
	var joined error
	now := s.now()
	for _, source := range sources {
		interval := source.RefreshIntervalSeconds
		if interval <= 0 {
			interval = defaultRSSRefreshIntervalSeconds
		}
		if source.LastRefreshedAt != nil && now.Sub(*source.LastRefreshedAt) < time.Duration(interval)*time.Second {
			continue
		}
		if _, refreshErr := s.refreshSourceEntity(ctx, source); refreshErr != nil {
			s.logger.Warn("rss refresh failed", slog.String("event", "rss.refresh.failed"), slog.Uint64("source_id", uint64(source.ID)), slog.Any("error", refreshErr))
			joined = errors.Join(joined, refreshErr)
		}
	}
	return joined
}

// StartRefreshWorker 启动 RSS 定时刷新 worker。
func (s *RSSService) StartRefreshWorker(ctx context.Context, interval time.Duration) {
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
			_ = s.RefreshDueSources(ctx)
		}
	}
}

// ListSubscriptions 返回订阅列表。
func (s *RSSService) ListSubscriptions(ctx context.Context, sourceID uint) (*appdto.RSSSubscriptionListResponse, error) {
	auth, err := currentRSSAuth(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.rssRepo.ListSubscriptions(ctx, domainrepo.RSSSubscriptionFilter{
		UserID:     auth.UserID,
		IncludeAll: permission.HasCapability(auth.Capabilities, permission.CapabilityRSSManage),
		SourceID:   sourceID,
	})
	if err != nil {
		return nil, err
	}
	views := make([]appdto.RSSSubscriptionView, 0, len(items))
	for _, item := range items {
		views = append(views, toRSSSubscriptionView(item))
	}
	return &appdto.RSSSubscriptionListResponse{Items: views}, nil
}

// CreateSubscription 创建订阅规则。
func (s *RSSService) CreateSubscription(ctx context.Context, req appdto.RSSSubscriptionUpsertRequest) (*appdto.RSSSubscriptionView, error) {
	auth, err := currentRSSAuth(ctx)
	if err != nil {
		return nil, err
	}
	source, err := s.rssRepo.FindSourceByID(ctx, req.SourceID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, source.UserID); err != nil {
		return nil, err
	}
	resolved, err := s.normalizeSubscriptionRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	now := s.now()
	subscription := &entity.RSSSubscription{
		UserID:                  auth.UserID,
		SourceID:                req.SourceID,
		Name:                    strings.TrimSpace(req.Name),
		IsEnabled:               boolDefault(req.IsEnabled, true),
		MustContain:             trimStringList(req.MustContain),
		MustNotContain:          trimStringList(req.MustNotContain),
		UseRegex:                req.UseRegex,
		CaseSensitive:           req.CaseSensitive,
		TargetVirtualParentPath: resolved.targetVirtualParentPath,
		ResolvedSourceID:        resolved.resolvedSourceID,
		ResolvedInnerParentPath: resolved.resolvedInnerParentPath,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := s.rssRepo.CreateSubscription(ctx, subscription); err != nil {
		return nil, err
	}
	view := toRSSSubscriptionView(subscription)
	return &view, nil
}

// GetSubscription 返回单个订阅。
func (s *RSSService) GetSubscription(ctx context.Context, id uint) (*appdto.RSSSubscriptionView, error) {
	subscription, err := s.rssRepo.FindSubscriptionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, subscription.UserID); err != nil {
		return nil, err
	}
	view := toRSSSubscriptionView(subscription)
	return &view, nil
}

// UpdateSubscription 更新订阅规则。
func (s *RSSService) UpdateSubscription(ctx context.Context, id uint, req appdto.RSSSubscriptionUpsertRequest) (*appdto.RSSSubscriptionView, error) {
	subscription, err := s.rssRepo.FindSubscriptionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, subscription.UserID); err != nil {
		return nil, err
	}
	source, err := s.rssRepo.FindSourceByID(ctx, req.SourceID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, source.UserID); err != nil {
		return nil, err
	}
	resolved, err := s.normalizeSubscriptionRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	subscription.SourceID = req.SourceID
	subscription.Name = strings.TrimSpace(req.Name)
	subscription.IsEnabled = boolDefault(req.IsEnabled, subscription.IsEnabled)
	subscription.MustContain = trimStringList(req.MustContain)
	subscription.MustNotContain = trimStringList(req.MustNotContain)
	subscription.UseRegex = req.UseRegex
	subscription.CaseSensitive = req.CaseSensitive
	subscription.TargetVirtualParentPath = resolved.targetVirtualParentPath
	subscription.ResolvedSourceID = resolved.resolvedSourceID
	subscription.ResolvedInnerParentPath = resolved.resolvedInnerParentPath
	subscription.UpdatedAt = s.now()
	if err := s.rssRepo.UpdateSubscription(ctx, subscription); err != nil {
		return nil, err
	}
	view := toRSSSubscriptionView(subscription)
	return &view, nil
}

// DeleteSubscription 删除订阅规则。
func (s *RSSService) DeleteSubscription(ctx context.Context, id uint) error {
	subscription, err := s.rssRepo.FindSubscriptionByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.authorizeRSSOwner(ctx, subscription.UserID); err != nil {
		return err
	}
	return s.rssRepo.DeleteSubscription(ctx, id)
}

// RunSubscription 对已入库条目手动执行订阅匹配和入队。
func (s *RSSService) RunSubscription(ctx context.Context, id uint) (*appdto.RSSRefreshResponse, error) {
	subscription, err := s.rssRepo.FindSubscriptionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, subscription.UserID); err != nil {
		return nil, err
	}
	items, err := s.rssRepo.ListItems(ctx, domainrepo.RSSItemFilter{IncludeAll: true, SourceID: subscription.SourceID})
	if err != nil {
		return nil, err
	}
	stats := rssRefreshStats{SourceID: subscription.SourceID}
	for _, item := range items {
		if isRSSItemAlreadyQueued(item) || item.LinkType == RSSLinkTypeUnsupported {
			continue
		}
		s.processItem(ctx, item, []*entity.RSSSubscription{subscription}, &stats)
	}
	return stats.toDTO(), nil
}

// ListItems 返回 RSS 条目列表。
func (s *RSSService) ListItems(ctx context.Context, filter domainrepo.RSSItemFilter) (*appdto.RSSItemListResponse, error) {
	auth, err := currentRSSAuth(ctx)
	if err != nil {
		return nil, err
	}
	filter.UserID = auth.UserID
	filter.IncludeAll = permission.HasCapability(auth.Capabilities, permission.CapabilityRSSManage)
	items, err := s.rssRepo.ListItems(ctx, filter)
	if err != nil {
		return nil, err
	}
	views := make([]appdto.RSSItemView, 0, len(items))
	for _, item := range items {
		views = append(views, toRSSItemView(item))
	}
	return &appdto.RSSItemListResponse{Items: views}, nil
}

// DownloadItem 手动将 RSS 条目入队。
func (s *RSSService) DownloadItem(ctx context.Context, id uint, req appdto.RSSManualDownloadRequest) (*appdto.RSSItemView, error) {
	item, err := s.rssRepo.FindItemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, item.UserID); err != nil {
		return nil, err
	}
	if isRSSItemAlreadyQueued(item) {
		view := toRSSItemView(item)
		return &view, nil
	}
	if !IsBTRSSDownloadLink(item.DownloadURL) {
		return nil, ErrDownloadLinkUnsupported
	}
	subscriptionID := req.SubscriptionID
	if subscriptionID == 0 && item.MatchedSubscriptionID != nil {
		subscriptionID = *item.MatchedSubscriptionID
	}
	if subscriptionID == 0 {
		return nil, ErrPathInvalid
	}
	subscription, err := s.rssRepo.FindSubscriptionByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if subscription.SourceID != item.SourceID {
		return nil, ErrPathInvalid
	}
	if err := s.authorizeRSSOwner(ctx, subscription.UserID); err != nil {
		return nil, err
	}
	if err := s.enqueueItem(ctx, item, subscription); err != nil {
		return nil, err
	}
	view := toRSSItemView(item)
	return &view, nil
}

// QBitHealth 返回 qBittorrent 健康状态。
func (s *RSSService) QBitHealth(ctx context.Context) (*appdto.RSSQBitHealthResponse, error) {
	if _, err := currentRSSAuth(ctx); err != nil {
		return nil, err
	}
	if s.healthChecker == nil {
		return &appdto.RSSQBitHealthResponse{Enabled: false, Status: "disabled"}, nil
	}
	if err := s.healthChecker.Health(ctx); err != nil {
		message := err.Error()
		return &appdto.RSSQBitHealthResponse{Enabled: true, Status: "unavailable", Error: &message}, nil
	}
	return &appdto.RSSQBitHealthResponse{Enabled: true, Status: "ok"}, nil
}

func (s *RSSService) refreshSourceEntity(ctx context.Context, source *entity.RSSSource) (*appdto.RSSRefreshResponse, error) {
	if s.fetcher == nil {
		return nil, ErrSourceDriverUnsupported
	}
	stats := rssRefreshStats{SourceID: source.ID}
	items, err := s.fetcher.Fetch(ctx, source.URL)
	now := s.now()
	source.LastRefreshedAt = &now
	source.UpdatedAt = now
	if err != nil {
		message := err.Error()
		source.LastError = &message
		_ = s.rssRepo.UpdateSource(ctx, source)
		return nil, err
	}
	source.LastError = nil
	if err := s.rssRepo.UpdateSource(ctx, source); err != nil {
		return nil, err
	}
	stats.Fetched = len(items)

	enabled := true
	subscriptions, err := s.rssRepo.ListSubscriptions(ctx, domainrepo.RSSSubscriptionFilter{IncludeAll: true, SourceID: source.ID, Enabled: &enabled})
	if err != nil {
		return nil, err
	}
	for _, fetched := range items {
		item, created, err := s.upsertFetchedItem(ctx, source, fetched)
		if err != nil {
			return nil, err
		}
		if created {
			stats.Created++
		} else {
			stats.Updated++
		}
		if item.LinkType == RSSLinkTypeUnsupported {
			stats.Unsupported++
			continue
		}
		if isRSSItemAlreadyQueued(item) {
			continue
		}
		s.processItem(ctx, item, subscriptions, &stats)
	}
	return stats.toDTO(), nil
}

func (s *RSSService) upsertFetchedItem(ctx context.Context, source *entity.RSSSource, fetched RSSFetchedItem) (*entity.RSSItem, bool, error) {
	downloadURL, linkType := resolveRSSDownloadLink(fetched)
	status := RSSItemStatusNew
	if linkType == RSSLinkTypeUnsupported {
		status = RSSItemStatusUnsupported
	}
	dedupKey := buildRSSDedupKey(source.ID, fetched)
	existing, err := s.rssRepo.FindItemByDedupKey(ctx, source.ID, dedupKey)
	if err != nil && !errors.Is(err, domainrepo.ErrNotFound) {
		return nil, false, err
	}
	now := s.now()
	if errors.Is(err, domainrepo.ErrNotFound) {
		item := &entity.RSSItem{
			UserID:      source.UserID,
			SourceID:    source.ID,
			Title:       strings.TrimSpace(fetched.Title),
			Link:        strings.TrimSpace(fetched.Link),
			PublishedAt: fetched.PublishedAt,
			GUID:        strings.TrimSpace(fetched.GUID),
			DedupKey:    dedupKey,
			DownloadURL: downloadURL,
			LinkType:    linkType,
			Status:      status,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.rssRepo.CreateItem(ctx, item); err != nil {
			return nil, false, err
		}
		return item, true, nil
	}

	existing.Title = strings.TrimSpace(fetched.Title)
	existing.Link = strings.TrimSpace(fetched.Link)
	existing.PublishedAt = fetched.PublishedAt
	existing.GUID = strings.TrimSpace(fetched.GUID)
	existing.DownloadURL = downloadURL
	existing.LinkType = linkType
	if existing.Status == RSSItemStatusNew || existing.Status == RSSItemStatusIgnored || existing.Status == RSSItemStatusUnsupported {
		existing.Status = status
	}
	existing.UpdatedAt = now
	if err := s.rssRepo.UpdateItem(ctx, existing); err != nil {
		return nil, false, err
	}
	return existing, false, nil
}

func (s *RSSService) processItem(ctx context.Context, item *entity.RSSItem, subscriptions []*entity.RSSSubscription, stats *rssRefreshStats) {
	if item == nil || stats == nil || isRSSItemAlreadyQueued(item) {
		return
	}
	matched := false
	for _, subscription := range subscriptions {
		if subscription == nil || !subscription.IsEnabled || subscription.SourceID != item.SourceID {
			continue
		}
		if !rssSubscriptionMatchesItem(subscription, item) {
			continue
		}
		matched = true
		stats.Matched++
		if err := s.enqueueItem(ctx, item, subscription); err != nil {
			message := err.Error()
			item.Status = RSSItemStatusFailed
			item.ErrorMessage = &message
			item.MatchedSubscriptionID = &subscription.ID
			item.UpdatedAt = s.now()
			_ = s.rssRepo.UpdateItem(ctx, item)
			stats.Failed++
		} else {
			stats.Enqueued++
		}
		return
	}
	if !matched && item.Status != RSSItemStatusIgnored {
		item.Status = RSSItemStatusIgnored
		item.ErrorMessage = nil
		item.UpdatedAt = s.now()
		_ = s.rssRepo.UpdateItem(ctx, item)
	}
}

func (s *RSSService) enqueueItem(ctx context.Context, item *entity.RSSItem, subscription *entity.RSSSubscription) error {
	if item == nil || subscription == nil {
		return ErrPathInvalid
	}
	if !IsBTRSSDownloadLink(item.DownloadURL) {
		return ErrDownloadLinkUnsupported
	}
	if s.taskCreator == nil {
		return ErrSourceDriverUnsupported
	}
	item.Status = RSSItemStatusMatched
	item.MatchedSubscriptionID = &subscription.ID
	item.ErrorMessage = nil
	item.UpdatedAt = s.now()
	if err := s.rssRepo.UpdateItem(ctx, item); err != nil {
		return err
	}
	taskCtx := s.contextForRSSOwner(ctx, subscription.UserID)
	task, err := s.taskCreator.Create(taskCtx, appdto.CreateTaskRequest{
		Type:                    "download",
		URL:                     item.DownloadURL,
		TargetVirtualParentPath: subscription.TargetVirtualParentPath,
	})
	if err != nil {
		return err
	}
	item.Status = RSSItemStatusEnqueued
	item.TaskID = &task.ID
	item.ErrorMessage = nil
	item.UpdatedAt = s.now()
	return s.rssRepo.UpdateItem(ctx, item)
}

type resolvedRSSSubscriptionTarget struct {
	targetVirtualParentPath string
	resolvedSourceID        uint
	resolvedInnerParentPath string
}

func (s *RSSService) normalizeSubscriptionRequest(ctx context.Context, req appdto.RSSSubscriptionUpsertRequest) (resolvedRSSSubscriptionTarget, error) {
	if strings.TrimSpace(req.Name) == "" {
		return resolvedRSSSubscriptionTarget{}, ErrConfigInvalid
	}
	if err := validateRSSRulePatterns(req.MustContain, req.MustNotContain, req.UseRegex); err != nil {
		return resolvedRSSSubscriptionTarget{}, err
	}
	return s.validateWritableTarget(ctx, req.TargetVirtualParentPath)
}

func (s *RSSService) validateWritableTarget(ctx context.Context, targetVirtualParentPath string) (resolvedRSSSubscriptionTarget, error) {
	virtualParentPath, err := normalizeVirtualPath(strings.TrimSpace(targetVirtualParentPath))
	if err != nil {
		return resolvedRSSSubscriptionTarget{}, err
	}
	if s.vfsResolver == nil {
		return resolvedRSSSubscriptionTarget{}, ErrSourceDriverUnsupported
	}
	probeName := ".yunxia-rss-probe"
	resolved, err := s.vfsResolver.ResolveWritableTarget(ctx, joinVirtualPath(virtualParentPath, probeName))
	if err != nil {
		return resolvedRSSSubscriptionTarget{}, err
	}
	if resolved.Source == nil {
		return resolvedRSSSubscriptionTarget{}, ErrNoBackingStorage
	}
	innerParentPath, _, err := splitParentName(resolved.InnerPath)
	if err != nil {
		return resolvedRSSSubscriptionTarget{}, err
	}
	if s.aclAuthorizer != nil {
		if err := s.aclAuthorizer.AuthorizePath(ctx, resolved.Source.ID, innerParentPath, ACLActionWrite); err != nil {
			return resolvedRSSSubscriptionTarget{}, err
		}
	}
	if resolved.Source.DriverType == "local" {
		if err := ensureLocalTargetWritable(resolved.Source, innerParentPath); err != nil {
			return resolvedRSSSubscriptionTarget{}, err
		}
	}
	return resolvedRSSSubscriptionTarget{
		targetVirtualParentPath: virtualParentPath,
		resolvedSourceID:        resolved.Source.ID,
		resolvedInnerParentPath: innerParentPath,
	}, nil
}

func ensureLocalTargetWritable(source *entity.StorageSource, innerParentPath string) error {
	_, physicalPath, err := resolvePhysicalPath(source, innerParentPath)
	if err != nil {
		return err
	}
	probePath := physicalPath
	for {
		info, statErr := os.Stat(probePath)
		if statErr == nil {
			if !info.IsDir() {
				return ErrPathInvalid
			}
			if !probeLocalDirectoryWritable(probePath) {
				return ErrSourceReadOnly
			}
			return nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return normalizeLocalWriteError(statErr)
		}
		parent := filepath.Dir(probePath)
		if parent == probePath || parent == "." {
			return ErrPathInvalid
		}
		probePath = parent
	}
}

func (s *RSSService) contextForRSSOwner(ctx context.Context, userID uint) context.Context {
	if _, ok := security.RequestAuthFromContext(ctx); ok {
		return ctx
	}
	auth := security.RequestAuth{UserID: userID}
	if s.userRepo != nil && userID != 0 {
		if user, err := s.userRepo.FindByID(ctx, userID); err == nil {
			auth.Username = user.Username
			auth.RoleKey = user.RoleKey
			auth.Status = user.Status
			if capabilities, capErr := permission.ResolveCapabilities(user.RoleKey); capErr == nil {
				auth.Capabilities = capabilities
			}
		}
	}
	return security.WithRequestAuth(ctx, auth)
}

func (s *RSSService) authorizeRSSOwner(ctx context.Context, ownerID uint) error {
	auth, err := currentRSSAuth(ctx)
	if err != nil {
		return err
	}
	if auth.UserID == ownerID || permission.HasCapability(auth.Capabilities, permission.CapabilityRSSManage) {
		return nil
	}
	return ErrPermissionDenied
}

func currentRSSAuth(ctx context.Context) (security.RequestAuth, error) {
	auth, ok := security.RequestAuthFromContext(ctx)
	if !ok {
		return security.RequestAuth{}, ErrPermissionDenied
	}
	return auth, nil
}

type rssRefreshStats struct {
	SourceID    uint
	Fetched     int
	Created     int
	Updated     int
	Matched     int
	Enqueued    int
	Unsupported int
	Failed      int
}

func (s rssRefreshStats) toDTO() *appdto.RSSRefreshResponse {
	return &appdto.RSSRefreshResponse{
		SourceID:    s.SourceID,
		Fetched:     s.Fetched,
		Created:     s.Created,
		Updated:     s.Updated,
		Matched:     s.Matched,
		Enqueued:    s.Enqueued,
		Unsupported: s.Unsupported,
		Failed:      s.Failed,
	}
}

func normalizeRSSSourceInput(req appdto.RSSSourceUpsertRequest) (string, string, int, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return "", "", 0, ErrConfigInvalid
	}
	rawURL := strings.TrimSpace(req.URL)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", 0, ErrConfigInvalid
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", 0, ErrConfigInvalid
	}
	interval := req.RefreshIntervalSeconds
	if interval <= 0 {
		interval = defaultRSSRefreshIntervalSeconds
	}
	if interval < 60 {
		interval = 60
	}
	return name, rawURL, interval, nil
}

func resolveRSSDownloadLink(item RSSFetchedItem) (string, string) {
	candidates := make([]string, 0, 2+len(item.Enclosures))
	candidates = append(candidates, item.Link)
	candidates = append(candidates, item.Enclosures...)
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		linkType := ClassifyDownloadLink(trimmed)
		if linkType == RSSLinkTypeMagnet || linkType == RSSLinkTypeTorrent {
			return trimmed, linkType
		}
	}
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed != "" {
			return trimmed, RSSLinkTypeUnsupported
		}
	}
	return "", RSSLinkTypeUnsupported
}

func buildRSSDedupKey(sourceID uint, item RSSFetchedItem) string {
	guid := strings.TrimSpace(item.GUID)
	if guid != "" {
		return "guid:" + guid
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", sourceID, strings.TrimSpace(item.Link), strings.TrimSpace(item.Title))))
	return "hash:" + hex.EncodeToString(hash[:])
}

func rssSubscriptionMatchesItem(subscription *entity.RSSSubscription, item *entity.RSSItem) bool {
	if subscription == nil || item == nil {
		return false
	}
	text := strings.TrimSpace(item.Title + " " + item.Link + " " + item.DownloadURL)
	if !matchAllRSSPatterns(text, subscription.MustContain, subscription.UseRegex, subscription.CaseSensitive) {
		return false
	}
	if matchAnyRSSPattern(text, subscription.MustNotContain, subscription.UseRegex, subscription.CaseSensitive) {
		return false
	}
	return true
}

func matchAllRSSPatterns(text string, patterns []string, useRegex bool, caseSensitive bool) bool {
	for _, pattern := range trimStringList(patterns) {
		matched, err := matchRSSPattern(text, pattern, useRegex, caseSensitive)
		if err != nil || !matched {
			return false
		}
	}
	return true
}

func matchAnyRSSPattern(text string, patterns []string, useRegex bool, caseSensitive bool) bool {
	for _, pattern := range trimStringList(patterns) {
		matched, err := matchRSSPattern(text, pattern, useRegex, caseSensitive)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func matchRSSPattern(text string, pattern string, useRegex bool, caseSensitive bool) (bool, error) {
	if pattern == "" {
		return true, nil
	}
	if useRegex {
		expr := pattern
		if !caseSensitive {
			expr = "(?i)" + pattern
		}
		return regexp.MatchString(expr, text)
	}
	if !caseSensitive {
		text = strings.ToLower(text)
		pattern = strings.ToLower(pattern)
	}
	return strings.Contains(text, pattern), nil
}

func validateRSSRulePatterns(mustContain []string, mustNotContain []string, useRegex bool) error {
	if !useRegex {
		return nil
	}
	for _, pattern := range append(trimStringList(mustContain), trimStringList(mustNotContain)...) {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("%w: %s", ErrRSSRegexInvalid, err.Error())
		}
	}
	return nil
}

func trimStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func isRSSItemAlreadyQueued(item *entity.RSSItem) bool {
	return item != nil && (item.Status == RSSItemStatusEnqueued || item.TaskID != nil)
}

func toRSSSourceView(source *entity.RSSSource) appdto.RSSSourceView {
	var lastRefreshedAt *string
	if source.LastRefreshedAt != nil {
		formatted := source.LastRefreshedAt.Format(time.RFC3339)
		lastRefreshedAt = &formatted
	}
	return appdto.RSSSourceView{
		ID:                     source.ID,
		UserID:                 source.UserID,
		Name:                   source.Name,
		URL:                    source.URL,
		IsEnabled:              source.IsEnabled,
		RefreshIntervalSeconds: source.RefreshIntervalSeconds,
		LastRefreshedAt:        lastRefreshedAt,
		LastError:              source.LastError,
		CreatedAt:              source.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              source.UpdatedAt.Format(time.RFC3339),
	}
}

func toRSSSubscriptionView(subscription *entity.RSSSubscription) appdto.RSSSubscriptionView {
	return appdto.RSSSubscriptionView{
		ID:                      subscription.ID,
		UserID:                  subscription.UserID,
		SourceID:                subscription.SourceID,
		Name:                    subscription.Name,
		IsEnabled:               subscription.IsEnabled,
		MustContain:             append([]string{}, subscription.MustContain...),
		MustNotContain:          append([]string{}, subscription.MustNotContain...),
		UseRegex:                subscription.UseRegex,
		CaseSensitive:           subscription.CaseSensitive,
		TargetVirtualParentPath: subscription.TargetVirtualParentPath,
		ResolvedSourceID:        subscription.ResolvedSourceID,
		ResolvedInnerParentPath: subscription.ResolvedInnerParentPath,
		CreatedAt:               subscription.CreatedAt.Format(time.RFC3339),
		UpdatedAt:               subscription.UpdatedAt.Format(time.RFC3339),
	}
}

func toRSSItemView(item *entity.RSSItem) appdto.RSSItemView {
	var publishedAt *string
	if item.PublishedAt != nil {
		formatted := item.PublishedAt.Format(time.RFC3339)
		publishedAt = &formatted
	}
	return appdto.RSSItemView{
		ID:                    item.ID,
		UserID:                item.UserID,
		SourceID:              item.SourceID,
		Title:                 item.Title,
		Link:                  item.Link,
		PublishedAt:           publishedAt,
		GUID:                  item.GUID,
		DownloadURL:           item.DownloadURL,
		LinkType:              item.LinkType,
		Status:                item.Status,
		MatchedSubscriptionID: item.MatchedSubscriptionID,
		TaskID:                item.TaskID,
		ErrorMessage:          item.ErrorMessage,
		CreatedAt:             item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:             item.UpdatedAt.Format(time.RFC3339),
	}
}
