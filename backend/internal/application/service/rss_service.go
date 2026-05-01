package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

const (
	RSSItemStatusNew            = "new"
	RSSItemStatusUnsupported    = "unsupported"
	RSSItemStatusIgnored        = "ignored"
	RSSItemStatusMatched        = "matched"
	RSSItemStatusEnqueued       = "enqueued"
	RSSItemStatusFailed         = "failed"
	RSSItemStatusRetryPending   = "retry_pending"
	RSSItemStatusCompleted      = "completed"
	RSSItemStatusNeedsAttention = "needs_attention"

	RSSSourceHealthOK          = "ok"
	RSSSourceHealthDegraded    = "degraded"
	RSSSourceHealthCircuitOpen = "circuit_open"

	RSSRefreshStatusSuccess = "success"
	RSSRefreshStatusFailed  = "failed"

	RSSRetryReasonDownloaderUnavailable = "downloader_unavailable"
	RSSRetryReasonTorrentFetchFailed    = "torrent_fetch_failed"
	RSSRetryReasonTaskFailed            = "task_failed"
	RSSRetryReasonStalled               = "stalled"
)

const defaultRSSRefreshIntervalSeconds = 1800

const (
	defaultRSSItemMaxRetryCount = 3
	defaultRSSRetryWorkerLimit  = 20
)

var (
	rssShortNumericKeywordPattern = regexp.MustCompile(`^[0-9]{1,2}$`)
	rssDigitRunPattern            = regexp.MustCompile(`[0-9]+`)
	rssDateLikePattern            = regexp.MustCompile(`[0-9]{4}[-/.年][0-9]{1,2}[-/.月][0-9]{1,2}(?:日)?`)
	rssMonthDayLikePattern        = regexp.MustCompile(`(?:^|[^0-9])[0-9]{1,2}[-/.月][0-9]{1,2}(?:日)?(?:$|[^0-9])`)
)

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

type rssTaskReader interface {
	FindByID(ctx context.Context, id uint) (*entity.DownloadTask, error)
}

// RSSService 负责 RSS 源、订阅、条目和下载入队流程。
type RSSService struct {
	rssRepo       domainrepo.RSSRepository
	sourceRepo    domainrepo.SourceRepository
	userRepo      domainrepo.UserRepository
	taskCreator   rssTaskCreator
	taskReader    rssTaskReader
	fetcher       RSSFetcher
	healthChecker QBitHealthChecker
	vfsResolver   interface {
		ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error)
	}
	aclAuthorizer    *ACLAuthorizer
	now              func() time.Time
	logger           *slog.Logger
	refreshLocks     sync.Map
	itemLocks        sync.Map
	retryWorkerLimit int
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

// WithRSSTaskRepository 注入任务读取器，用于 RSS item 与下载任务终态回写。
func WithRSSTaskRepository(repo rssTaskReader) RSSServiceOption {
	return func(s *RSSService) {
		s.taskReader = repo
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

// WithRSSRetryWorkerLimit 覆盖每轮自动重试处理上限，主要用于测试。
func WithRSSRetryWorkerLimit(limit int) RSSServiceOption {
	return func(s *RSSService) {
		if limit > 0 {
			s.retryWorkerLimit = limit
		}
	}
}

// NewRSSService 创建 RSS 服务。
func NewRSSService(rssRepo domainrepo.RSSRepository, sourceRepo domainrepo.SourceRepository, taskCreator rssTaskCreator, options ...RSSServiceOption) *RSSService {
	service := &RSSService{
		rssRepo:          rssRepo,
		sourceRepo:       sourceRepo,
		taskCreator:      taskCreator,
		now:              time.Now,
		logger:           newServiceLogger("service.rss"),
		retryWorkerLimit: defaultRSSRetryWorkerLimit,
	}
	if reader, ok := taskCreator.(rssTaskReader); ok {
		service.taskReader = reader
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
		HealthStatus:           RSSSourceHealthOK,
		NextRefreshAt:          &now,
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
		if source.IsEnabled && source.NextRefreshAt == nil {
			next := s.now()
			source.NextRefreshAt = &next
		}
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

// RefreshAllSources 手动刷新所有已启用 RSS 源，单源失败不影响其他源。
func (s *RSSService) RefreshAllSources(ctx context.Context) (*appdto.RSSRefreshAllResponse, error) {
	if _, err := currentRSSAuth(ctx); err != nil {
		return nil, err
	}
	enabled := true
	sources, err := s.rssRepo.ListSources(ctx, domainrepo.RSSSourceFilter{IncludeAll: true, Enabled: &enabled})
	if err != nil {
		return nil, err
	}
	resp := &appdto.RSSRefreshAllResponse{Items: make([]appdto.RSSRefreshAllItemView, 0, len(sources))}
	for _, source := range sources {
		stats, refreshErr := s.refreshSourceEntity(ctx, source)
		if refreshErr != nil {
			if errors.Is(refreshErr, ErrTaskInvalidState) {
				resp.Skipped++
				resp.Items = append(resp.Items, appdto.RSSRefreshAllItemView{SourceID: source.ID, Status: "skipped"})
				continue
			}
			message := refreshErr.Error()
			resp.Failed++
			resp.Items = append(resp.Items, appdto.RSSRefreshAllItemView{SourceID: source.ID, Status: RSSRefreshStatusFailed, Error: &message})
			s.logger.Warn("rss refresh-all source failed", slog.String("event", "rss.refresh_all.source_failed"), slog.Uint64("source_id", uint64(source.ID)), slog.Any("error", refreshErr))
			continue
		}
		resp.Refreshed++
		resp.Items = append(resp.Items, appdto.RSSRefreshAllItemView{SourceID: source.ID, Status: RSSRefreshStatusSuccess, Stats: stats})
	}
	return resp, nil
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
		if !sourceRefreshDue(source, now) {
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

// StartRetryWorker 启动无人值守自愈 worker：回写 task 终态并处理到期重试 item。
func (s *RSSService) StartRetryWorker(ctx context.Context, interval time.Duration) {
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
			_, _ = s.RunRetryCycle(ctx, s.retryWorkerLimit)
		}
	}
}

// RunRetryCycle 执行一轮 RSS item task 回写和自动重试，返回本轮处理数量。
func (s *RSSService) RunRetryCycle(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = s.retryWorkerLimit
	}
	processed := 0
	var joined error
	if err := s.reconcileTaskBacklinks(ctx, limit, &processed); err != nil {
		joined = errors.Join(joined, err)
	}
	if processed >= limit {
		return processed, joined
	}
	if err := s.retryDueItems(ctx, limit-processed, &processed); err != nil {
		joined = errors.Join(joined, err)
	}
	return processed, joined
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
		if s.itemHasActiveTask(ctx, item) || item.LinkType == RSSLinkTypeUnsupported {
			continue
		}
		if !rssItemAllowsAutomaticProcessing(item) {
			continue
		}
		s.processItem(ctx, item, []*entity.RSSSubscription{subscription}, &stats)
	}
	return stats.toDTO(), nil
}

// PreviewSubscription 用现有条目解释订阅规则会命中、缺失或排除哪些 item。
func (s *RSSService) PreviewSubscription(ctx context.Context, id uint) (*appdto.RSSSubscriptionPreviewResponse, error) {
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
	resp := &appdto.RSSSubscriptionPreviewResponse{
		SubscriptionID: subscription.ID,
		SourceID:       subscription.SourceID,
		Items:          make([]appdto.RSSSubscriptionPreviewItem, 0, len(items)),
	}
	for _, item := range items {
		preview := evaluateRSSSubscriptionPreview(subscription, item)
		resp.Items = append(resp.Items, preview)
		switch preview.Result {
		case "matched":
			resp.Matched++
		case "excluded":
			resp.Excluded++
		default:
			resp.Missing++
		}
	}
	return resp, nil
}

// ReprocessItem 对单个 item 重新执行匹配/处理。
func (s *RSSService) ReprocessItem(ctx context.Context, id uint) (*appdto.RSSItemView, error) {
	item, err := s.rssRepo.FindItemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, item.UserID); err != nil {
		return nil, err
	}
	if rssItemIsCompleted(item) {
		view := toRSSItemView(item)
		return &view, nil
	}
	if item.LinkType == RSSLinkTypeUnsupported {
		message := "download link unsupported"
		item.Status = RSSItemStatusNeedsAttention
		item.ErrorMessage = &message
		item.RetryReason = nil
		item.NextRetryAt = nil
		item.UpdatedAt = s.now()
		if err := s.rssRepo.UpdateItem(ctx, item); err != nil {
			return nil, err
		}
		view := toRSSItemView(item)
		return &view, nil
	}
	if s.itemHasActiveTask(ctx, item) {
		view := toRSSItemView(item)
		return &view, nil
	}
	enabled := true
	subscriptions, err := s.rssRepo.ListSubscriptions(ctx, domainrepo.RSSSubscriptionFilter{IncludeAll: true, SourceID: item.SourceID, Enabled: &enabled})
	if err != nil {
		return nil, err
	}
	stats := rssRefreshStats{SourceID: item.SourceID}
	s.processItem(ctx, item, subscriptions, &stats)
	if refreshed, err := s.rssRepo.FindItemByID(ctx, item.ID); err == nil {
		item = refreshed
	}
	view := toRSSItemView(item)
	return &view, nil
}

// RetryItem 手动重试单个 item，绕过 next_retry_at 但仍记录一次 attempt。
func (s *RSSService) RetryItem(ctx context.Context, id uint, req appdto.RSSManualDownloadRequest) (*appdto.RSSItemView, error) {
	item, err := s.rssRepo.FindItemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, item.UserID); err != nil {
		return nil, err
	}
	if s.itemHasActiveTask(ctx, item) {
		view := toRSSItemView(item)
		return &view, nil
	}
	if rssItemIsCompleted(item) {
		view := toRSSItemView(item)
		return &view, nil
	}
	subscription, err := s.subscriptionForItemRetry(ctx, item, req.SubscriptionID)
	if err != nil {
		return nil, err
	}
	if err := s.retryItemWithSubscription(ctx, item, subscription, true); err != nil {
		return nil, err
	}
	if refreshed, err := s.rssRepo.FindItemByID(ctx, item.ID); err == nil {
		item = refreshed
	}
	view := toRSSItemView(item)
	return &view, nil
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
	if s.itemHasActiveTask(ctx, item) {
		view := toRSSItemView(item)
		return &view, nil
	}
	if rssItemIsCompleted(item) {
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
	if refreshed, err := s.rssRepo.FindItemByID(ctx, item.ID); err == nil {
		item = refreshed
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
	unlock, locked := s.tryLockRSSSource(source.ID)
	if !locked {
		return nil, ErrTaskInvalidState
	}
	defer unlock()
	stats := rssRefreshStats{SourceID: source.ID}
	items, err := s.fetcher.Fetch(ctx, source.URL)
	now := s.now()
	source.LastRefreshedAt = &now
	source.UpdatedAt = now
	if err != nil {
		message := err.Error()
		source.LastError = &message
		stats.Failed = 1
		s.applyRSSSourceRefreshFailure(source, stats, now)
		_ = s.rssRepo.UpdateSource(ctx, source)
		return nil, err
	}
	source.LastError = nil
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
		if s.itemHasActiveTask(ctx, item) {
			continue
		}
		if !rssItemAllowsAutomaticProcessing(item) {
			continue
		}
		s.processItem(ctx, item, subscriptions, &stats)
	}
	s.applyRSSSourceRefreshSuccess(source, stats, now)
	if err := s.rssRepo.UpdateSource(ctx, source); err != nil {
		return nil, err
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
			UserID:        source.UserID,
			SourceID:      source.ID,
			Title:         strings.TrimSpace(fetched.Title),
			Link:          strings.TrimSpace(fetched.Link),
			PublishedAt:   fetched.PublishedAt,
			GUID:          strings.TrimSpace(fetched.GUID),
			DedupKey:      dedupKey,
			DownloadURL:   downloadURL,
			LinkType:      linkType,
			Status:        status,
			MaxRetryCount: defaultRSSItemMaxRetryCount,
			CreatedAt:     now,
			UpdatedAt:     now,
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
	ensureRSSItemRetryDefaults(existing)
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
	if item == nil || stats == nil || s.itemHasActiveTask(ctx, item) {
		return
	}
	if rssItemIsCompleted(item) {
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
			s.markItemRetryOrAttention(ctx, item, subscription.ID, err, s.now(), false)
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
	return s.enqueueItemWithAttempt(ctx, item, subscription, false)
}

func (s *RSSService) enqueueItemWithAttempt(ctx context.Context, item *entity.RSSItem, subscription *entity.RSSSubscription, countRetry bool) error {
	if item == nil || subscription == nil {
		return ErrPathInvalid
	}
	unlock, locked := s.tryLockRSSItem(item.ID)
	if !locked {
		return ErrTaskInvalidState
	}
	defer unlock()
	current, err := s.rssRepo.FindItemByID(ctx, item.ID)
	if err == nil {
		item = current
	} else if !errors.Is(err, domainrepo.ErrNotFound) {
		return err
	}
	if s.itemHasActiveTask(ctx, item) {
		return nil
	}
	if rssItemIsCompleted(item) {
		return nil
	}
	if !IsBTRSSDownloadLink(item.DownloadURL) {
		return ErrDownloadLinkUnsupported
	}
	if s.taskCreator == nil {
		return ErrSourceDriverUnsupported
	}
	now := s.now()
	ensureRSSItemRetryDefaults(item)
	if countRetry {
		item.RetryCount++
	}
	item.Status = RSSItemStatusMatched
	item.MatchedSubscriptionID = &subscription.ID
	item.ErrorMessage = nil
	item.LastAttemptAt = &now
	item.NextRetryAt = nil
	item.RetryReason = nil
	item.UpdatedAt = now
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
	item.NextRetryAt = nil
	item.RetryReason = nil
	item.UpdatedAt = s.now()
	if err := s.rssRepo.UpdateItem(ctx, item); err != nil {
		return err
	}
	return nil
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
	if auth, ok := security.RequestAuthFromContext(ctx); ok && auth.UserID == userID {
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

func (s rssRefreshStats) toView() appdto.RSSRefreshStatsView {
	return appdto.RSSRefreshStatsView{
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

func sourceRefreshDue(source *entity.RSSSource, now time.Time) bool {
	if source == nil || !source.IsEnabled {
		return false
	}
	if source.NextRefreshAt != nil {
		return !source.NextRefreshAt.After(now)
	}
	if source.LastRefreshedAt == nil {
		return true
	}
	interval := source.RefreshIntervalSeconds
	if interval <= 0 {
		interval = defaultRSSRefreshIntervalSeconds
	}
	return !source.LastRefreshedAt.Add(time.Duration(interval) * time.Second).After(now)
}

func (s *RSSService) applyRSSSourceRefreshSuccess(source *entity.RSSSource, stats rssRefreshStats, now time.Time) {
	source.HealthStatus = RSSSourceHealthOK
	source.ConsecutiveFailures = 0
	source.LastSuccessAt = &now
	source.LastRefreshStatus = RSSRefreshStatusSuccess
	source.LastRefreshStatsJSON = encodeRSSRefreshStats(stats)
	source.NextRefreshAt = ptrTime(now.Add(time.Duration(normalizeRSSRefreshInterval(source.RefreshIntervalSeconds)) * time.Second))
}

func (s *RSSService) applyRSSSourceRefreshFailure(source *entity.RSSSource, stats rssRefreshStats, now time.Time) {
	source.ConsecutiveFailures++
	source.LastRefreshStatus = RSSRefreshStatusFailed
	source.LastRefreshStatsJSON = encodeRSSRefreshStats(stats)
	source.HealthStatus = rssSourceHealthForFailures(source.ConsecutiveFailures)
	source.NextRefreshAt = ptrTime(now.Add(rssSourceFailureBackoff(source)))
}

func normalizeRSSRefreshInterval(interval int) int {
	if interval <= 0 {
		return defaultRSSRefreshIntervalSeconds
	}
	if interval < 60 {
		return 60
	}
	return interval
}

func rssSourceHealthForFailures(failures int) string {
	switch {
	case failures >= 5:
		return RSSSourceHealthCircuitOpen
	case failures >= 3:
		return RSSSourceHealthDegraded
	default:
		return RSSSourceHealthOK
	}
}

func rssSourceFailureBackoff(source *entity.RSSSource) time.Duration {
	interval := time.Duration(normalizeRSSRefreshInterval(source.RefreshIntervalSeconds)) * time.Second
	switch {
	case source.ConsecutiveFailures >= 6:
		return time.Hour
	case source.ConsecutiveFailures >= 5:
		return 30 * time.Minute
	case source.ConsecutiveFailures >= 3:
		if interval < 30*time.Minute {
			return 30 * time.Minute
		}
		return interval
	default:
		return interval
	}
}

func encodeRSSRefreshStats(stats rssRefreshStats) string {
	data, err := json.Marshal(stats.toView())
	if err != nil {
		return ""
	}
	return string(data)
}

func decodeRSSRefreshStats(raw string) *appdto.RSSRefreshStatsView {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var stats appdto.RSSRefreshStatsView
	if err := json.Unmarshal([]byte(raw), &stats); err != nil {
		return nil
	}
	return &stats
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func (s *RSSService) tryLockRSSSource(sourceID uint) (func(), bool) {
	value, _ := s.refreshLocks.LoadOrStore(sourceID, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	if !mutex.TryLock() {
		return func() {}, false
	}
	return mutex.Unlock, true
}

func (s *RSSService) tryLockRSSItem(itemID uint) (func(), bool) {
	value, _ := s.itemLocks.LoadOrStore(itemID, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	if !mutex.TryLock() {
		return func() {}, false
	}
	return mutex.Unlock, true
}

func (s *RSSService) itemHasActiveTask(ctx context.Context, item *entity.RSSItem) bool {
	if item == nil || item.TaskID == nil {
		return false
	}
	if s.taskReader == nil {
		return item.Status == RSSItemStatusEnqueued
	}
	task, err := s.taskReader.FindByID(ctx, *item.TaskID)
	if err != nil {
		return !errors.Is(err, domainrepo.ErrNotFound)
	}
	return !isTerminalTaskStatus(task.Status)
}

func (s *RSSService) reconcileTaskBacklinks(ctx context.Context, limit int, processed *int) error {
	if s.taskReader == nil || limit <= 0 {
		return nil
	}
	items, err := s.rssRepo.ListItems(ctx, domainrepo.RSSItemFilter{IncludeAll: true, Status: RSSItemStatusEnqueued})
	if err != nil {
		return err
	}
	var joined error
	now := s.now()
	for _, item := range items {
		if *processed >= limit {
			break
		}
		if item.TaskID == nil {
			continue
		}
		task, err := s.taskReader.FindByID(ctx, *item.TaskID)
		if err != nil {
			if errors.Is(err, domainrepo.ErrNotFound) {
				message := "download task not found"
				s.markItemRetryOrAttention(ctx, item, valueOrZero(item.MatchedSubscriptionID), errors.New(message), now, false)
				(*processed)++
				continue
			}
			joined = errors.Join(joined, err)
			continue
		}
		switch task.Status {
		case "completed":
			item.Status = RSSItemStatusCompleted
			item.ErrorMessage = nil
			item.RetryReason = nil
			item.NextRetryAt = nil
			item.UpdatedAt = now
			if err := s.rssRepo.UpdateItem(ctx, item); err != nil {
				joined = errors.Join(joined, err)
				continue
			}
			(*processed)++
		case "failed", "canceled":
			taskErr := errors.New(taskTerminalErrorMessage(task))
			s.markItemRetryOrAttention(ctx, item, valueOrZero(item.MatchedSubscriptionID), taskErr, now, false)
			(*processed)++
		}
	}
	return joined
}

func (s *RSSService) retryDueItems(ctx context.Context, limit int, processed *int) error {
	if limit <= 0 {
		return nil
	}
	items, err := s.rssRepo.ListItems(ctx, domainrepo.RSSItemFilter{IncludeAll: true, Status: RSSItemStatusRetryPending})
	if err != nil {
		return err
	}
	var joined error
	now := s.now()
	for _, item := range items {
		if *processed >= limit {
			break
		}
		if item.NextRetryAt != nil && item.NextRetryAt.After(now) {
			continue
		}
		if s.itemHasActiveTask(ctx, item) {
			continue
		}
		itemCtx := s.contextForRSSOwner(ctx, item.UserID)
		subscription, err := s.subscriptionForItemRetry(itemCtx, item, 0)
		if err != nil {
			message := err.Error()
			item.Status = RSSItemStatusNeedsAttention
			item.ErrorMessage = &message
			item.RetryReason = nil
			item.NextRetryAt = nil
			item.UpdatedAt = now
			if updateErr := s.rssRepo.UpdateItem(ctx, item); updateErr != nil {
				joined = errors.Join(joined, updateErr)
			}
			(*processed)++
			continue
		}
		if err := s.retryItemWithSubscription(itemCtx, item, subscription, false); err != nil {
			joined = errors.Join(joined, err)
		}
		(*processed)++
	}
	return joined
}

func (s *RSSService) subscriptionForItemRetry(ctx context.Context, item *entity.RSSItem, requestedID uint) (*entity.RSSSubscription, error) {
	if item == nil {
		return nil, ErrPathInvalid
	}
	if !IsBTRSSDownloadLink(item.DownloadURL) {
		return nil, ErrDownloadLinkUnsupported
	}
	subscriptionID := requestedID
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
	return subscription, nil
}

func (s *RSSService) retryItemWithSubscription(ctx context.Context, item *entity.RSSItem, subscription *entity.RSSSubscription, manual bool) error {
	if item == nil || subscription == nil {
		return ErrPathInvalid
	}
	if !manual && item.NextRetryAt != nil && item.NextRetryAt.After(s.now()) {
		return nil
	}
	err := s.enqueueItemWithAttempt(ctx, item, subscription, true)
	if err == nil {
		return nil
	}
	current, findErr := s.rssRepo.FindItemByID(ctx, item.ID)
	if findErr == nil {
		item = current
	}
	s.markItemRetryOrAttention(ctx, item, subscription.ID, err, s.now(), false)
	return nil
}

func (s *RSSService) markItemRetryOrAttention(ctx context.Context, item *entity.RSSItem, subscriptionID uint, err error, now time.Time, countAttempt bool) {
	if item == nil || err == nil {
		return
	}
	ensureRSSItemRetryDefaults(item)
	if countAttempt {
		item.RetryCount++
	}
	message := err.Error()
	item.ErrorMessage = &message
	if subscriptionID != 0 {
		item.MatchedSubscriptionID = &subscriptionID
	}
	reason, retryable := classifyRSSRetryError(err)
	if retryable && item.RetryCount < item.MaxRetryCount {
		item.Status = RSSItemStatusRetryPending
		item.RetryReason = &reason
		next := now.Add(rssItemRetryBackoff(item.RetryCount))
		item.NextRetryAt = &next
	} else {
		if retryable {
			item.Status = RSSItemStatusNeedsAttention
		} else {
			item.Status = RSSItemStatusNeedsAttention
		}
		item.RetryReason = &reason
		item.NextRetryAt = nil
	}
	item.UpdatedAt = now
	if updateErr := s.rssRepo.UpdateItem(ctx, item); updateErr != nil {
		s.logger.Warn("rss item retry state update failed", slog.String("event", "rss.item.retry_state_update_failed"), slog.Uint64("item_id", uint64(item.ID)), slog.Any("error", updateErr))
	}
}

func ensureRSSItemRetryDefaults(item *entity.RSSItem) {
	if item != nil && item.MaxRetryCount <= 0 {
		item.MaxRetryCount = defaultRSSItemMaxRetryCount
	}
}

func classifyRSSRetryError(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrDownloadLinkUnsupported),
		errors.Is(err, ErrPathInvalid),
		errors.Is(err, ErrNoBackingStorage),
		errors.Is(err, ErrNameConflict),
		errors.Is(err, ErrSourceReadOnly),
		errors.Is(err, ErrACLDenied),
		errors.Is(err, ErrPermissionDenied),
		errors.Is(err, ErrConfigInvalid),
		errors.Is(err, ErrRSSRegexInvalid):
		return "deterministic_error", false
	case errors.Is(err, ErrSourceDriverUnsupported):
		return RSSRetryReasonDownloaderUnavailable, true
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "permission denied") || strings.Contains(lower, "read-only") || strings.Contains(lower, "unsupported link") || strings.Contains(lower, "invalid path"):
		return "deterministic_error", false
	case strings.Contains(lower, "canceled") || strings.Contains(lower, "cancelled"):
		return RSSRetryReasonTaskFailed, false
	case strings.Contains(lower, "torrent"):
		return RSSRetryReasonTorrentFetchFailed, true
	case strings.Contains(lower, "unavailable") || strings.Contains(lower, "timeout") || strings.Contains(lower, "temporary") || strings.Contains(lower, "connection refused") || strings.Contains(lower, "network"):
		return RSSRetryReasonDownloaderUnavailable, true
	default:
		return RSSRetryReasonTaskFailed, true
	}
}

func rssItemRetryBackoff(retryCount int) time.Duration {
	switch {
	case retryCount <= 0:
		return 5 * time.Minute
	case retryCount == 1:
		return 30 * time.Minute
	default:
		return 2 * time.Hour
	}
}

func taskTerminalErrorMessage(task *entity.DownloadTask) string {
	if task == nil {
		return "download task failed"
	}
	if task.ErrorMessage != nil && strings.TrimSpace(*task.ErrorMessage) != "" {
		return strings.TrimSpace(*task.ErrorMessage)
	}
	if task.Status == "canceled" {
		return "download canceled"
	}
	return "download failed"
}

func valueOrZero(value *uint) uint {
	if value == nil {
		return 0
	}
	return *value
}

func evaluateRSSSubscriptionPreview(subscription *entity.RSSSubscription, item *entity.RSSItem) appdto.RSSSubscriptionPreviewItem {
	result := appdto.RSSSubscriptionPreviewItem{
		ItemID:        item.ID,
		Title:         item.Title,
		DownloadURL:   item.DownloadURL,
		CurrentStatus: item.Status,
		Matched:       []string{},
		Missing:       []string{},
		Excluded:      []string{},
		Result:        "matched",
	}
	title := strings.TrimSpace(item.Title)
	metadata := strings.TrimSpace(item.Link + " " + item.DownloadURL)
	for _, pattern := range trimStringList(subscription.MustContain) {
		matched, err := matchRSSPattern(title, metadata, pattern, subscription.UseRegex, subscription.CaseSensitive)
		if err == nil && matched {
			result.Matched = append(result.Matched, pattern)
		} else {
			result.Missing = append(result.Missing, pattern)
		}
	}
	for _, pattern := range trimStringList(subscription.MustNotContain) {
		matched, err := matchRSSPattern(title, metadata, pattern, subscription.UseRegex, subscription.CaseSensitive)
		if err == nil && matched {
			result.Excluded = append(result.Excluded, pattern)
		}
	}
	switch {
	case len(result.Excluded) > 0:
		result.Result = "excluded"
	case len(result.Missing) > 0:
		result.Result = "missing"
	default:
		result.Result = "matched"
	}
	return result
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
	title := strings.TrimSpace(item.Title)
	metadata := strings.TrimSpace(item.Link + " " + item.DownloadURL)
	if !matchAllRSSPatterns(title, metadata, subscription.MustContain, subscription.UseRegex, subscription.CaseSensitive) {
		return false
	}
	if matchAnyRSSPattern(title, metadata, subscription.MustNotContain, subscription.UseRegex, subscription.CaseSensitive) {
		return false
	}
	return true
}

func matchAllRSSPatterns(title string, metadata string, patterns []string, useRegex bool, caseSensitive bool) bool {
	for _, pattern := range trimStringList(patterns) {
		matched, err := matchRSSPattern(title, metadata, pattern, useRegex, caseSensitive)
		if err != nil || !matched {
			return false
		}
	}
	return true
}

func matchAnyRSSPattern(title string, metadata string, patterns []string, useRegex bool, caseSensitive bool) bool {
	for _, pattern := range trimStringList(patterns) {
		matched, err := matchRSSPattern(title, metadata, pattern, useRegex, caseSensitive)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func matchRSSPattern(title string, metadata string, pattern string, useRegex bool, caseSensitive bool) (bool, error) {
	if pattern == "" {
		return true, nil
	}
	text := strings.TrimSpace(title + " " + metadata)
	if useRegex {
		expr := pattern
		if !caseSensitive {
			expr = "(?i)" + pattern
		}
		return regexp.MatchString(expr, text)
	}
	if isShortNumericRSSKeyword(pattern) {
		return matchShortNumericRSSKeyword(title, pattern), nil
	}
	if !caseSensitive {
		title = strings.ToLower(title)
		metadata = strings.ToLower(metadata)
		pattern = strings.ToLower(pattern)
	}
	return strings.Contains(title, pattern) || strings.Contains(metadata, pattern), nil
}

func isShortNumericRSSKeyword(pattern string) bool {
	return rssShortNumericKeywordPattern.MatchString(strings.TrimSpace(pattern))
}

func matchShortNumericRSSKeyword(title string, pattern string) bool {
	want := normalizeRSSNumber(pattern)
	if want == "" {
		return false
	}
	if matchExplicitRSSEpisodeNumber(title, want) {
		return true
	}
	for _, loc := range rssDigitRunPattern.FindAllStringIndex(title, -1) {
		token := title[loc[0]:loc[1]]
		if len(token) > 3 || normalizeRSSNumber(token) != want {
			continue
		}
		if isRSSNumericRunMetadata(title, loc[0], loc[1]) {
			continue
		}
		return true
	}
	return false
}

func matchExplicitRSSEpisodeNumber(title string, normalizedNumber string) bool {
	escapedNumber := regexp.QuoteMeta(normalizedNumber)
	patterns := []string{
		`(?i)\bS[0-9]{1,2}E0*` + escapedNumber + `\b`,
		`(?i)\bEP(?:ISODE)?\.?\s*0*` + escapedNumber + `\b`,
		`(?i)\bE0*` + escapedNumber + `\b`,
		`第\s*0*` + escapedNumber + `\s*(?:集|话|話|回)`,
	}
	for _, pattern := range patterns {
		if regexp.MustCompile(pattern).MatchString(title) {
			return true
		}
	}
	return false
}

func isRSSNumericRunMetadata(title string, start int, end int) bool {
	if isRSSNumericRunInRegexp(title, start, end, rssDateLikePattern) ||
		isRSSNumericRunInRegexp(title, start, end, rssMonthDayLikePattern) {
		return true
	}
	if start > 0 && isASCIIAlphaNum(title[start-1]) {
		return true
	}
	if end < len(title) {
		next := title[end]
		if isASCIIAlphaNum(next) && !hasRSSVersionSuffix(title, end) {
			return true
		}
	}
	return false
}

func isRSSNumericRunInRegexp(title string, start int, end int, expr *regexp.Regexp) bool {
	for _, loc := range expr.FindAllStringIndex(title, -1) {
		if start >= loc[0] && end <= loc[1] {
			return true
		}
	}
	return false
}

func hasRSSVersionSuffix(title string, index int) bool {
	return index+1 < len(title) &&
		(title[index] == 'v' || title[index] == 'V') &&
		title[index+1] >= '0' && title[index+1] <= '9'
}

func normalizeRSSNumber(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimLeft(trimmed, "0")
	if trimmed == "" {
		return "0"
	}
	return trimmed
}

func isASCIIAlphaNum(value byte) bool {
	return (value >= '0' && value <= '9') ||
		(value >= 'a' && value <= 'z') ||
		(value >= 'A' && value <= 'Z')
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

func rssItemIsCompleted(item *entity.RSSItem) bool {
	return item != nil && item.Status == RSSItemStatusCompleted
}

func rssItemAllowsAutomaticProcessing(item *entity.RSSItem) bool {
	if item == nil {
		return false
	}
	switch item.Status {
	case "", RSSItemStatusNew, RSSItemStatusIgnored, RSSItemStatusMatched:
		return true
	default:
		return false
	}
}

func toRSSSourceView(source *entity.RSSSource) appdto.RSSSourceView {
	var lastRefreshedAt *string
	if source.LastRefreshedAt != nil {
		formatted := source.LastRefreshedAt.Format(time.RFC3339)
		lastRefreshedAt = &formatted
	}
	var lastSuccessAt *string
	if source.LastSuccessAt != nil {
		formatted := source.LastSuccessAt.Format(time.RFC3339)
		lastSuccessAt = &formatted
	}
	var nextRefreshAt *string
	if source.NextRefreshAt != nil {
		formatted := source.NextRefreshAt.Format(time.RFC3339)
		nextRefreshAt = &formatted
	}
	healthStatus := source.HealthStatus
	if healthStatus == "" {
		healthStatus = RSSSourceHealthOK
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
		HealthStatus:           healthStatus,
		ConsecutiveFailures:    source.ConsecutiveFailures,
		LastSuccessAt:          lastSuccessAt,
		NextRefreshAt:          nextRefreshAt,
		LastRefreshStatus:      source.LastRefreshStatus,
		LastRefreshStats:       decodeRSSRefreshStats(source.LastRefreshStatsJSON),
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
	var lastAttemptAt *string
	if item.LastAttemptAt != nil {
		formatted := item.LastAttemptAt.Format(time.RFC3339)
		lastAttemptAt = &formatted
	}
	var nextRetryAt *string
	if item.NextRetryAt != nil {
		formatted := item.NextRetryAt.Format(time.RFC3339)
		nextRetryAt = &formatted
	}
	maxRetryCount := item.MaxRetryCount
	if maxRetryCount <= 0 {
		maxRetryCount = defaultRSSItemMaxRetryCount
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
		RetryCount:            item.RetryCount,
		MaxRetryCount:         maxRetryCount,
		LastAttemptAt:         lastAttemptAt,
		NextRetryAt:           nextRetryAt,
		RetryReason:           item.RetryReason,
		CreatedAt:             item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:             item.UpdatedAt.Format(time.RFC3339),
	}
}
