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
	"unicode"

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
	rssExportVersion            = 1
)

const (
	rssImportActionCreate = "create"
	rssImportActionReuse  = "reuse"
	rssImportActionSkip   = "skip"
	rssImportActionFailed = "failed"
)

var (
	rssShortNumericKeywordPattern = regexp.MustCompile(`^[0-9]{1,2}$`)
	rssDigitRunPattern            = regexp.MustCompile(`[0-9]+`)
	rssDateLikePattern            = regexp.MustCompile(`[0-9]{4}[-/.年][0-9]{1,2}[-/.月][0-9]{1,2}(?:日)?`)
	rssMonthDayLikePattern        = regexp.MustCompile(`(?:^|[^0-9])[0-9]{1,2}[-/.月][0-9]{1,2}(?:日)?(?:$|[^0-9])`)
	rssTemplatePlaceholderPattern = regexp.MustCompile(`\{([a-z_]+)\}`)
	rssAllowedTemplateTokens      = map[string]struct{}{
		"anime_title":    {},
		"season":         {},
		"episode":        {},
		"subtitle_group": {},
		"resolution":     {},
		"title":          {},
	}
	rssResolutionPattern     = regexp.MustCompile(`(?i)(?:2160|1440|1080|720|480)p|[48]k`)
	rssSxxEyyPattern         = regexp.MustCompile(`(?i)\bS([0-9]{1,2})\s*E([0-9]{1,4}(?:\.[0-9]+)?)(?:v[0-9]+)?\b`)
	rssSeasonWordPattern     = regexp.MustCompile(`(?i)\b(?:Season\s*([0-9]{1,2})|([0-9]{1,2})(?:st|nd|rd|th)\s+Season)\b`)
	rssChineseSeasonPattern  = regexp.MustCompile(`第\s*([0-9]{1,2})\s*季`)
	rssChineseEpisodePattern = regexp.MustCompile(`第\s*([0-9]{1,4}(?:\.[0-9]+)?)\s*(?:集|话|話|回)`)
	rssEpisodeWordPattern    = regexp.MustCompile(`(?i)\b(?:EP|Episode)\.?\s*([0-9]{1,4}(?:\.[0-9]+)?)(?:v[0-9]+)?\b`)
	rssDashEpisodePattern    = regexp.MustCompile(`(?i)(?:^|\s)[-–—]\s*([0-9]{1,4}(?:\.[0-9]+)?)(?:v[0-9]+)?(?:\b|[^0-9])`)
	rssBracketTokenPattern   = regexp.MustCompile(`[\[【]([^\]】]+)[\]】]`)
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

type rssNotifier interface {
	Notify(ctx context.Context, input NotificationEventInput) (*entity.NotificationEvent, error)
}

// RSSService 负责 RSS 源、订阅、条目和下载入队流程。
type RSSService struct {
	rssRepo       domainrepo.RSSRepository
	sourceRepo    domainrepo.SourceRepository
	userRepo      domainrepo.UserRepository
	taskCreator   rssTaskCreator
	taskReader    rssTaskReader
	notifier      rssNotifier
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

// WithRSSNotifier 注入 RSS 通知器。
func WithRSSNotifier(notifier rssNotifier) RSSServiceOption {
	return func(s *RSSService) {
		s.notifier = notifier
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

// ExportConfig 导出当前身份可管理范围内的 RSS 源和订阅配置，不包含运行时状态。
func (s *RSSService) ExportConfig(ctx context.Context) (*appdto.RSSExportResponse, error) {
	auth, err := currentRSSAuth(ctx)
	if err != nil {
		return nil, err
	}
	includeAll := permission.HasCapability(auth.Capabilities, permission.CapabilityRSSManage)
	sources, err := s.rssRepo.ListSources(ctx, domainrepo.RSSSourceFilter{
		UserID:     auth.UserID,
		IncludeAll: includeAll,
	})
	if err != nil {
		return nil, err
	}
	subscriptions, err := s.rssRepo.ListSubscriptions(ctx, domainrepo.RSSSubscriptionFilter{
		UserID:     auth.UserID,
		IncludeAll: includeAll,
	})
	if err != nil {
		return nil, err
	}
	resp := &appdto.RSSExportResponse{
		Version:       rssExportVersion,
		ExportedAt:    s.now().Format(time.RFC3339),
		Sources:       make([]appdto.RSSExportSource, 0, len(sources)),
		Subscriptions: make([]appdto.RSSExportSubscription, 0, len(subscriptions)),
	}
	sourceURLByID := make(map[uint]string, len(sources))
	for _, source := range sources {
		sourceURLByID[source.ID] = source.URL
		resp.Sources = append(resp.Sources, appdto.RSSExportSource{
			Name:                   source.Name,
			URL:                    source.URL,
			IsEnabled:              source.IsEnabled,
			RefreshIntervalSeconds: source.RefreshIntervalSeconds,
		})
	}
	for _, subscription := range subscriptions {
		sourceURL := sourceURLByID[subscription.SourceID]
		if sourceURL == "" {
			continue
		}
		resp.Subscriptions = append(resp.Subscriptions, appdto.RSSExportSubscription{
			SourceURL:               sourceURL,
			Name:                    subscription.Name,
			IsEnabled:               subscription.IsEnabled,
			MustContain:             append([]string{}, subscription.MustContain...),
			MustNotContain:          append([]string{}, subscription.MustNotContain...),
			UseRegex:                subscription.UseRegex,
			CaseSensitive:           subscription.CaseSensitive,
			TargetVirtualParentPath: subscription.TargetVirtualParentPath,
			DirectoryTemplate:       subscription.DirectoryTemplate,
			FilenameTemplate:        subscription.FilenameTemplate,
		})
	}
	return resp, nil
}

// ImportConfig 导入 RSS 源和订阅配置；逐项返回结果，单项失败不影响后续条目。
func (s *RSSService) ImportConfig(ctx context.Context, req appdto.RSSImportRequest) (*appdto.RSSImportResponse, error) {
	auth, err := currentRSSAuth(ctx)
	if err != nil {
		return nil, err
	}
	resp := &appdto.RSSImportResponse{
		DryRun: req.DryRun,
		Sources: appdto.RSSImportSectionResult{
			Items: make([]appdto.RSSImportItemResult, 0, len(req.Sources)),
		},
		Subscriptions: appdto.RSSImportSectionResult{
			Items: make([]appdto.RSSImportItemResult, 0, len(req.Subscriptions)),
		},
	}
	existingSources, err := s.rssRepo.ListSources(ctx, domainrepo.RSSSourceFilter{UserID: auth.UserID})
	if err != nil {
		return nil, err
	}
	sourceByURL := make(map[string]*entity.RSSSource, len(existingSources)+len(req.Sources))
	for _, source := range existingSources {
		sourceByURL[strings.TrimSpace(source.URL)] = source
	}
	now := s.now()
	for index, sourceReq := range req.Sources {
		result := appdto.RSSImportItemResult{
			Index:     index,
			SourceURL: strings.TrimSpace(sourceReq.URL),
			Name:      strings.TrimSpace(sourceReq.Name),
		}
		name, rawURL, interval, normalizeErr := normalizeRSSSourceInput(appdto.RSSSourceUpsertRequest{
			Name:                   sourceReq.Name,
			URL:                    sourceReq.URL,
			IsEnabled:              sourceReq.IsEnabled,
			RefreshIntervalSeconds: sourceReq.RefreshIntervalSeconds,
		})
		if normalizeErr != nil {
			fillRSSImportError(&result, normalizeErr, "RSS_SOURCE_NOT_FOUND")
			appendRSSImportResult(&resp.Sources, result)
			continue
		}
		result.SourceURL = rawURL
		result.Name = name
		if existing, ok := sourceByURL[rawURL]; ok {
			result.Action = rssImportActionReuse
			result.Success = true
			result.ID = existing.ID
			appendRSSImportResult(&resp.Sources, result)
			continue
		}
		enabled := boolDefault(sourceReq.IsEnabled, true)
		if req.DryRun {
			result.Action = rssImportActionCreate
			result.Success = true
			sourceByURL[rawURL] = &entity.RSSSource{
				UserID:                 auth.UserID,
				Name:                   name,
				URL:                    rawURL,
				IsEnabled:              enabled,
				RefreshIntervalSeconds: interval,
			}
			appendRSSImportResult(&resp.Sources, result)
			continue
		}
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
			fillRSSImportError(&result, err, "RSS_SOURCE_NOT_FOUND")
			appendRSSImportResult(&resp.Sources, result)
			continue
		}
		sourceByURL[rawURL] = source
		result.Action = rssImportActionCreate
		result.Success = true
		result.ID = source.ID
		appendRSSImportResult(&resp.Sources, result)
	}
	for index, subscriptionReq := range req.Subscriptions {
		sourceURL := firstNonEmpty(subscriptionReq.SourceURL, subscriptionReq.SourceRef)
		result := appdto.RSSImportItemResult{
			Index:     index,
			SourceURL: sourceURL,
			Name:      strings.TrimSpace(subscriptionReq.Name),
		}
		source, ok := sourceByURL[sourceURL]
		if !ok {
			fillRSSImportError(&result, domainrepo.ErrNotFound, "RSS_SOURCE_NOT_FOUND")
			appendRSSImportResult(&resp.Subscriptions, result)
			continue
		}
		upsertReq := appdto.RSSSubscriptionUpsertRequest{
			SourceID:                source.ID,
			Name:                    subscriptionReq.Name,
			IsEnabled:               subscriptionReq.IsEnabled,
			MustContain:             subscriptionReq.MustContain,
			MustNotContain:          subscriptionReq.MustNotContain,
			UseRegex:                subscriptionReq.UseRegex,
			CaseSensitive:           subscriptionReq.CaseSensitive,
			TargetVirtualParentPath: subscriptionReq.TargetVirtualParentPath,
			DirectoryTemplate:       subscriptionReq.DirectoryTemplate,
			FilenameTemplate:        subscriptionReq.FilenameTemplate,
		}
		resolved, normalizeErr := s.normalizeSubscriptionRequest(ctx, upsertReq)
		if normalizeErr != nil {
			fillRSSImportError(&result, normalizeErr, "RSS_SUBSCRIPTION_NOT_FOUND")
			appendRSSImportResult(&resp.Subscriptions, result)
			continue
		}
		if req.DryRun {
			result.Action = rssImportActionCreate
			result.Success = true
			appendRSSImportResult(&resp.Subscriptions, result)
			continue
		}
		subscription := &entity.RSSSubscription{
			UserID:                  auth.UserID,
			SourceID:                source.ID,
			Name:                    strings.TrimSpace(subscriptionReq.Name),
			IsEnabled:               boolDefault(subscriptionReq.IsEnabled, true),
			MustContain:             trimStringList(subscriptionReq.MustContain),
			MustNotContain:          trimStringList(subscriptionReq.MustNotContain),
			UseRegex:                subscriptionReq.UseRegex,
			CaseSensitive:           subscriptionReq.CaseSensitive,
			TargetVirtualParentPath: resolved.targetVirtualParentPath,
			DirectoryTemplate:       resolved.directoryTemplate,
			FilenameTemplate:        resolved.filenameTemplate,
			ResolvedSourceID:        resolved.resolvedSourceID,
			ResolvedInnerParentPath: resolved.resolvedInnerParentPath,
			CreatedAt:               now,
			UpdatedAt:               now,
		}
		if err := s.rssRepo.CreateSubscription(ctx, subscription); err != nil {
			fillRSSImportError(&result, err, "RSS_SUBSCRIPTION_NOT_FOUND")
			appendRSSImportResult(&resp.Subscriptions, result)
			continue
		}
		result.Action = rssImportActionCreate
		result.Success = true
		result.ID = subscription.ID
		appendRSSImportResult(&resp.Subscriptions, result)
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

// OnTaskTerminalStatus 立即同步关联 RSS item 的下载任务终态。
func (s *RSSService) OnTaskTerminalStatus(ctx context.Context, task *entity.DownloadTask) error {
	if task == nil || !isTerminalTaskStatus(task.Status) {
		return nil
	}
	return s.reconcileSingleTaskBacklink(ctx, task)
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
		DirectoryTemplate:       resolved.directoryTemplate,
		FilenameTemplate:        resolved.filenameTemplate,
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

// CloneSubscription 复制一条订阅规则，不重新解析目标路径快照。
func (s *RSSService) CloneSubscription(ctx context.Context, id uint, req appdto.RSSSubscriptionCloneRequest) (*appdto.RSSSubscriptionView, error) {
	original, err := s.rssRepo.FindSubscriptionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, original.UserID); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = s.defaultRSSSubscriptionCloneName(ctx, original)
	}
	isEnabled := original.IsEnabled
	if req.IsEnabled != nil {
		isEnabled = *req.IsEnabled
	}

	now := s.now()
	clone := &entity.RSSSubscription{
		UserID:                  original.UserID,
		SourceID:                original.SourceID,
		Name:                    name,
		IsEnabled:               isEnabled,
		MustContain:             append([]string{}, original.MustContain...),
		MustNotContain:          append([]string{}, original.MustNotContain...),
		UseRegex:                original.UseRegex,
		CaseSensitive:           original.CaseSensitive,
		TargetVirtualParentPath: original.TargetVirtualParentPath,
		DirectoryTemplate:       original.DirectoryTemplate,
		FilenameTemplate:        original.FilenameTemplate,
		ResolvedSourceID:        original.ResolvedSourceID,
		ResolvedInnerParentPath: original.ResolvedInnerParentPath,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := s.rssRepo.CreateSubscription(ctx, clone); err != nil {
		return nil, err
	}
	view := toRSSSubscriptionView(clone)
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
	subscription.DirectoryTemplate = resolved.directoryTemplate
	subscription.FilenameTemplate = resolved.filenameTemplate
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

// BatchUpdateSubscriptionState 批量启用或禁用订阅；单项失败不影响其他 subscription。
func (s *RSSService) BatchUpdateSubscriptionState(ctx context.Context, req appdto.RSSSubscriptionBatchStateRequest) (*appdto.RSSSubscriptionBatchStateResponse, error) {
	if _, err := currentRSSAuth(ctx); err != nil {
		return nil, err
	}
	if len(req.SubscriptionIDs) == 0 || req.IsEnabled == nil {
		return nil, ErrConfigInvalid
	}
	resp := &appdto.RSSSubscriptionBatchStateResponse{
		Items: make([]appdto.RSSSubscriptionBatchStateResult, 0, len(req.SubscriptionIDs)),
	}
	now := s.now()
	for _, subscriptionID := range req.SubscriptionIDs {
		result := appdto.RSSSubscriptionBatchStateResult{SubscriptionID: subscriptionID}
		subscription, err := s.rssRepo.FindSubscriptionByID(ctx, subscriptionID)
		if err == nil {
			err = s.authorizeRSSOwner(ctx, subscription.UserID)
		}
		if err == nil {
			subscription.IsEnabled = *req.IsEnabled
			subscription.UpdatedAt = now
			err = s.rssRepo.UpdateSubscription(ctx, subscription)
		}
		if err != nil {
			fillRSSSubscriptionBatchError(&result, err)
			resp.Failed++
			resp.Items = append(resp.Items, result)
			continue
		}
		view := toRSSSubscriptionView(subscription)
		result.Success = true
		result.Subscription = &view
		resp.Succeeded++
		resp.Items = append(resp.Items, result)
	}
	return resp, nil
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
	return s.previewRSSSubscription(ctx, subscription)
}

// PreviewSubscriptionRules 用不落库的临时订阅规则预览已有条目命中情况。
func (s *RSSService) PreviewSubscriptionRules(ctx context.Context, req appdto.RSSSubscriptionPreviewRequest) (*appdto.RSSSubscriptionPreviewResponse, error) {
	source, err := s.rssRepo.FindSourceByID(ctx, req.SourceID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, source.UserID); err != nil {
		return nil, err
	}
	if err := validateRSSRulePatterns(req.MustContain, req.MustNotContain, req.UseRegex); err != nil {
		return nil, err
	}
	subscription := &entity.RSSSubscription{
		SourceID:       source.ID,
		UserID:         source.UserID,
		IsEnabled:      true,
		MustContain:    trimStringList(req.MustContain),
		MustNotContain: trimStringList(req.MustNotContain),
		UseRegex:       req.UseRegex,
		CaseSensitive:  req.CaseSensitive,
	}
	return s.previewRSSSubscription(ctx, subscription)
}

func (s *RSSService) previewRSSSubscription(ctx context.Context, subscription *entity.RSSSubscription) (*appdto.RSSSubscriptionPreviewResponse, error) {
	if subscription == nil {
		return nil, ErrPathInvalid
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

// BatchIgnoreItems 批量忽略 RSS 条目；单项失败不影响其他 item。
func (s *RSSService) BatchIgnoreItems(ctx context.Context, req appdto.RSSItemBatchIgnoreRequest) (*appdto.RSSItemBatchActionResponse, error) {
	if _, err := currentRSSAuth(ctx); err != nil {
		return nil, err
	}
	if len(req.ItemIDs) == 0 {
		return nil, ErrConfigInvalid
	}
	resp := &appdto.RSSItemBatchActionResponse{
		Items: make([]appdto.RSSItemBatchActionResult, 0, len(req.ItemIDs)),
	}
	for _, itemID := range req.ItemIDs {
		result := appdto.RSSItemBatchActionResult{ItemID: itemID}
		item, err := s.ignoreRSSItem(ctx, itemID)
		if err != nil {
			fillRSSItemBatchError(&result, err)
			resp.Failed++
			resp.Items = append(resp.Items, result)
			continue
		}
		view := toRSSItemView(item)
		result.Success = true
		result.Item = &view
		resp.Succeeded++
		resp.Items = append(resp.Items, result)
	}
	return resp, nil
}

// BatchRetryItems 批量执行手动 retry；复用单条 RetryItem 语义并返回逐项结果。
func (s *RSSService) BatchRetryItems(ctx context.Context, req appdto.RSSItemBatchRetryRequest) (*appdto.RSSItemBatchActionResponse, error) {
	if _, err := currentRSSAuth(ctx); err != nil {
		return nil, err
	}
	if len(req.ItemIDs) == 0 {
		return nil, ErrConfigInvalid
	}
	resp := &appdto.RSSItemBatchActionResponse{
		Items: make([]appdto.RSSItemBatchActionResult, 0, len(req.ItemIDs)),
	}
	retryReq := appdto.RSSManualDownloadRequest{SubscriptionID: req.SubscriptionID}
	for _, itemID := range req.ItemIDs {
		result := appdto.RSSItemBatchActionResult{ItemID: itemID}
		item, err := s.RetryItem(ctx, itemID, retryReq)
		if err != nil {
			fillRSSItemBatchError(&result, err)
			resp.Failed++
			resp.Items = append(resp.Items, result)
			continue
		}
		result.Success = true
		result.Item = item
		resp.Succeeded++
		resp.Items = append(resp.Items, result)
	}
	return resp, nil
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
	previousHealth := normalizeRSSSourceHealth(source.HealthStatus)
	source.LastRefreshedAt = &now
	source.UpdatedAt = now
	if err != nil {
		message := err.Error()
		source.LastError = &message
		stats.Failed = 1
		s.applyRSSSourceRefreshFailure(source, stats, now)
		_ = s.rssRepo.UpdateSource(ctx, source)
		s.notifyRSSSourceFailure(ctx, source, err, previousHealth)
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
	parsed := parseRSSAnimeTitle(fetched.Title)
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
			Parsed:        parsed,
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
	existing.Parsed = parsed
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
	targetVirtualParentPath, err := renderRSSTargetVirtualParentPath(subscription, item)
	if err != nil {
		return err
	}
	targetFilename, err := renderRSSFilenameTemplate(subscription, item)
	if err != nil {
		return err
	}
	taskCtx := s.contextForRSSOwner(ctx, subscription.UserID)
	task, err := s.taskCreator.Create(taskCtx, appdto.CreateTaskRequest{
		Type:                    "download",
		URL:                     item.DownloadURL,
		TargetVirtualParentPath: targetVirtualParentPath,
		TargetFilename:          targetFilename,
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
	directoryTemplate       string
	filenameTemplate        string
	resolvedSourceID        uint
	resolvedInnerParentPath string
}

func (s *RSSService) defaultRSSSubscriptionCloneName(ctx context.Context, original *entity.RSSSubscription) string {
	base := strings.TrimSpace(original.Name)
	if base == "" {
		base = "Subscription"
	}
	candidate := base + " Copy"
	existing, err := s.rssRepo.ListSubscriptions(ctx, domainrepo.RSSSubscriptionFilter{
		IncludeAll: true,
		SourceID:   original.SourceID,
	})
	if err != nil {
		return candidate
	}
	used := map[string]struct{}{}
	for _, subscription := range existing {
		if subscription.UserID != original.UserID {
			continue
		}
		used[strings.TrimSpace(subscription.Name)] = struct{}{}
	}
	if _, ok := used[candidate]; !ok {
		return candidate
	}
	for index := 2; ; index++ {
		next := fmt.Sprintf("%s Copy %d", base, index)
		if _, ok := used[next]; !ok {
			return next
		}
	}
}

func (s *RSSService) normalizeSubscriptionRequest(ctx context.Context, req appdto.RSSSubscriptionUpsertRequest) (resolvedRSSSubscriptionTarget, error) {
	if strings.TrimSpace(req.Name) == "" {
		return resolvedRSSSubscriptionTarget{}, ErrConfigInvalid
	}
	if err := validateRSSRulePatterns(req.MustContain, req.MustNotContain, req.UseRegex); err != nil {
		return resolvedRSSSubscriptionTarget{}, err
	}
	directoryTemplate := strings.TrimSpace(req.DirectoryTemplate)
	if err := validateRSSDirectoryTemplate(directoryTemplate); err != nil {
		return resolvedRSSSubscriptionTarget{}, err
	}
	filenameTemplate := strings.TrimSpace(req.FilenameTemplate)
	if err := validateRSSFilenameTemplate(filenameTemplate); err != nil {
		return resolvedRSSSubscriptionTarget{}, err
	}
	resolved, err := s.validateWritableTarget(ctx, req.TargetVirtualParentPath)
	if err != nil {
		return resolvedRSSSubscriptionTarget{}, err
	}
	resolved.directoryTemplate = directoryTemplate
	resolved.filenameTemplate = filenameTemplate
	return resolved, nil
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

func normalizeRSSSourceHealth(health string) string {
	if strings.TrimSpace(health) == "" {
		return RSSSourceHealthOK
	}
	return strings.TrimSpace(health)
}

func (s *RSSService) notifyRSSSourceFailure(ctx context.Context, source *entity.RSSSource, refreshErr error, previousHealth string) {
	if source == nil || refreshErr == nil {
		return
	}
	currentHealth := normalizeRSSSourceHealth(source.HealthStatus)
	if currentHealth != RSSSourceHealthDegraded && currentHealth != RSSSourceHealthCircuitOpen {
		return
	}
	if previousHealth == currentHealth {
		return
	}
	s.emitRSSNotification(ctx, NotificationEventInput{
		UserID:    source.UserID,
		EventType: NotificationEventRSSSourceFailure,
		Severity:  NotificationSeverityWarning,
		Title:     "RSS source refresh failures",
		Message:   fmt.Sprintf("RSS source %s entered %s after %d consecutive failures", source.Name, currentHealth, source.ConsecutiveFailures),
		Payload: map[string]any{
			"source_id":            source.ID,
			"user_id":              source.UserID,
			"name":                 source.Name,
			"url":                  source.URL,
			"health_status":        currentHealth,
			"consecutive_failures": source.ConsecutiveFailures,
			"error":                refreshErr.Error(),
		},
	})
}

func (s *RSSService) notifyRSSItemNeedsAttention(ctx context.Context, item *entity.RSSItem, itemErr error) {
	if item == nil {
		return
	}
	message := "RSS item needs attention"
	if itemErr != nil {
		message = itemErr.Error()
	} else if item.ErrorMessage != nil && strings.TrimSpace(*item.ErrorMessage) != "" {
		message = strings.TrimSpace(*item.ErrorMessage)
	}
	s.emitRSSNotification(ctx, NotificationEventInput{
		UserID:    item.UserID,
		EventType: NotificationEventRSSItemNeedsAttention,
		Severity:  NotificationSeverityError,
		Title:     "RSS item needs attention",
		Message:   message,
		Payload: map[string]any{
			"item_id":                 item.ID,
			"user_id":                 item.UserID,
			"source_id":               item.SourceID,
			"title":                   item.Title,
			"status":                  item.Status,
			"matched_subscription_id": item.MatchedSubscriptionID,
			"task_id":                 item.TaskID,
			"retry_count":             item.RetryCount,
			"max_retry_count":         item.MaxRetryCount,
			"retry_reason":            item.RetryReason,
			"error":                   message,
		},
	})
}

func (s *RSSService) notifyRSSDownloadCompleted(ctx context.Context, item *entity.RSSItem, task *entity.DownloadTask) {
	if item == nil {
		return
	}
	payload := map[string]any{
		"item_id":                 item.ID,
		"user_id":                 item.UserID,
		"source_id":               item.SourceID,
		"title":                   item.Title,
		"matched_subscription_id": item.MatchedSubscriptionID,
		"task_id":                 item.TaskID,
	}
	if task != nil {
		payload["download_task_id"] = task.ID
		payload["display_name"] = task.DisplayName
		payload["target_virtual_parent_path"] = task.TargetVirtualParentPath
		payload["save_virtual_path"] = task.SaveVirtualPath
		payload["downloader_type"] = task.DownloaderType
	}
	s.emitRSSNotification(ctx, NotificationEventInput{
		UserID:    item.UserID,
		EventType: NotificationEventRSSDownloadCompleted,
		Severity:  NotificationSeverityInfo,
		Title:     "RSS download completed",
		Message:   fmt.Sprintf("RSS download completed: %s", item.Title),
		Payload:   payload,
	})
}

func (s *RSSService) emitRSSNotification(ctx context.Context, input NotificationEventInput) {
	if s.notifier == nil {
		return
	}
	if _, err := s.notifier.Notify(ctx, input); err != nil {
		s.logger.Warn("rss notification failed", slog.String("event", "rss.notification.failed"), slog.String("notification_event_type", input.EventType), slog.Any("error", err))
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

func (s *RSSService) ignoreRSSItem(ctx context.Context, itemID uint) (*entity.RSSItem, error) {
	item, err := s.rssRepo.FindItemByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeRSSOwner(ctx, item.UserID); err != nil {
		return nil, err
	}
	unlock, locked := s.tryLockRSSItem(item.ID)
	if !locked {
		return nil, ErrTaskInvalidState
	}
	defer unlock()
	current, err := s.rssRepo.FindItemByID(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	item = current
	if err := s.authorizeRSSOwner(ctx, item.UserID); err != nil {
		return nil, err
	}
	if s.itemHasActiveTask(ctx, item) || rssItemIsCompleted(item) {
		return nil, ErrTaskInvalidState
	}
	item.Status = RSSItemStatusIgnored
	item.ErrorMessage = nil
	item.RetryReason = nil
	item.NextRetryAt = nil
	item.UpdatedAt = s.now()
	if err := s.rssRepo.UpdateItem(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
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
		if err := s.applyRSSItemTaskBacklink(ctx, item, task, now); err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		if isTerminalTaskStatus(task.Status) {
			(*processed)++
		}
	}
	return joined
}

func (s *RSSService) reconcileSingleTaskBacklink(ctx context.Context, task *entity.DownloadTask) error {
	if task == nil || !isTerminalTaskStatus(task.Status) {
		return nil
	}
	items, err := s.rssRepo.ListItems(ctx, domainrepo.RSSItemFilter{
		IncludeAll: true,
		TaskID:     task.ID,
		Status:     RSSItemStatusEnqueued,
	})
	if err != nil {
		return err
	}
	var joined error
	now := s.now()
	for _, item := range items {
		if err := s.applyRSSItemTaskBacklink(ctx, item, task, now); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

func (s *RSSService) applyRSSItemTaskBacklink(ctx context.Context, item *entity.RSSItem, task *entity.DownloadTask, now time.Time) error {
	if item == nil || task == nil {
		return nil
	}
	switch task.Status {
	case "completed":
		item.Status = RSSItemStatusCompleted
		item.ErrorMessage = nil
		item.RetryReason = nil
		item.NextRetryAt = nil
		item.UpdatedAt = now
		if err := s.rssRepo.UpdateItem(ctx, item); err != nil {
			return err
		}
		s.notifyRSSDownloadCompleted(ctx, item, task)
	case "failed", "canceled":
		taskErr := errors.New(taskTerminalErrorMessage(task))
		s.markItemRetryOrAttention(ctx, item, valueOrZero(item.MatchedSubscriptionID), taskErr, now, false)
	}
	return nil
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
			} else {
				s.notifyRSSItemNeedsAttention(ctx, item, err)
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
	previousStatus := item.Status
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
		return
	}
	if item.Status == RSSItemStatusNeedsAttention && previousStatus != RSSItemStatusNeedsAttention {
		s.notifyRSSItemNeedsAttention(ctx, item, err)
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

func appendRSSImportResult(section *appdto.RSSImportSectionResult, result appdto.RSSImportItemResult) {
	if section == nil {
		return
	}
	if result.Action == "" {
		if result.Success {
			result.Action = rssImportActionSkip
		} else {
			result.Action = rssImportActionFailed
		}
	}
	section.Items = append(section.Items, result)
	switch result.Action {
	case rssImportActionCreate:
		if result.Success {
			section.Created++
		} else {
			section.Failed++
		}
	case rssImportActionReuse:
		if result.Success {
			section.Reused++
		} else {
			section.Failed++
		}
	case rssImportActionSkip:
		if result.Success {
			section.Skipped++
		} else {
			section.Failed++
		}
	default:
		section.Failed++
	}
}

func fillRSSImportError(result *appdto.RSSImportItemResult, err error, notFoundCode string) {
	if result == nil || err == nil {
		return
	}
	code := rssImportErrorCode(err, notFoundCode)
	message := err.Error()
	result.Action = rssImportActionFailed
	result.Success = false
	result.ErrorCode = &code
	result.ErrorMessage = &message
}

func rssImportErrorCode(err error, notFoundCode string) string {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		return notFoundCode
	case errors.Is(err, ErrConfigInvalid):
		return "CONFIG_INVALID"
	case errors.Is(err, ErrPathInvalid):
		return "PATH_INVALID"
	case errors.Is(err, ErrNoBackingStorage):
		return "NO_BACKING_STORAGE"
	case errors.Is(err, ErrNameConflict):
		return "NAME_CONFLICT"
	case errors.Is(err, ErrSourceReadOnly):
		return "SOURCE_READ_ONLY"
	case errors.Is(err, ErrACLDenied), errors.Is(err, ErrPermissionDenied):
		return "PERMISSION_DENIED"
	case errors.Is(err, ErrRSSRegexInvalid):
		return "RSS_REGEX_INVALID"
	case errors.Is(err, ErrSourceDriverUnsupported):
		return "DOWNLOADER_UNAVAILABLE"
	default:
		return "INTERNAL_ERROR"
	}
}

func fillRSSSubscriptionBatchError(result *appdto.RSSSubscriptionBatchStateResult, err error) {
	if result == nil || err == nil {
		return
	}
	code := rssSubscriptionBatchErrorCode(err)
	message := err.Error()
	result.Success = false
	result.ErrorCode = &code
	result.ErrorMessage = &message
}

func rssSubscriptionBatchErrorCode(err error) string {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		return "RSS_SUBSCRIPTION_NOT_FOUND"
	case errors.Is(err, ErrConfigInvalid):
		return "CONFIG_INVALID"
	case errors.Is(err, ErrPathInvalid):
		return "PATH_INVALID"
	case errors.Is(err, ErrNoBackingStorage):
		return "NO_BACKING_STORAGE"
	case errors.Is(err, ErrNameConflict):
		return "NAME_CONFLICT"
	case errors.Is(err, ErrSourceReadOnly):
		return "SOURCE_READ_ONLY"
	case errors.Is(err, ErrACLDenied), errors.Is(err, ErrPermissionDenied):
		return "PERMISSION_DENIED"
	case errors.Is(err, ErrRSSRegexInvalid):
		return "RSS_REGEX_INVALID"
	case errors.Is(err, ErrSourceDriverUnsupported):
		return "DOWNLOADER_UNAVAILABLE"
	default:
		return "INTERNAL_ERROR"
	}
}

func fillRSSItemBatchError(result *appdto.RSSItemBatchActionResult, err error) {
	if result == nil || err == nil {
		return
	}
	code := rssItemBatchErrorCode(err)
	message := err.Error()
	result.Success = false
	result.ErrorCode = &code
	result.ErrorMessage = &message
}

func rssItemBatchErrorCode(err error) string {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		return "RSS_ITEM_NOT_FOUND"
	case errors.Is(err, ErrConfigInvalid):
		return "CONFIG_INVALID"
	case errors.Is(err, ErrPathInvalid):
		return "PATH_INVALID"
	case errors.Is(err, ErrNoBackingStorage):
		return "NO_BACKING_STORAGE"
	case errors.Is(err, ErrNameConflict):
		return "NAME_CONFLICT"
	case errors.Is(err, ErrSourceReadOnly):
		return "SOURCE_READ_ONLY"
	case errors.Is(err, ErrACLDenied), errors.Is(err, ErrPermissionDenied):
		return "PERMISSION_DENIED"
	case errors.Is(err, ErrDownloadLinkUnsupported):
		return "DOWNLOAD_LINK_UNSUPPORTED"
	case errors.Is(err, ErrRSSRegexInvalid):
		return "RSS_REGEX_INVALID"
	case errors.Is(err, ErrSourceDriverUnsupported):
		return "DOWNLOADER_UNAVAILABLE"
	case errors.Is(err, ErrTaskInvalidState):
		return "TASK_INVALID_STATE"
	default:
		return "INTERNAL_ERROR"
	}
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

func parseRSSAnimeTitle(title string) entity.RSSAnimeParsed {
	original := strings.TrimSpace(title)
	withoutGroup, subtitleGroup := stripRSSSubtitleGroup(original)
	working := stripRSSReleaseDecorations(withoutGroup)
	season, episode := parseRSSSeasonEpisode(original)
	animeTitle := cleanupRSSAnimeTitle(extractRSSAnimeTitle(working))
	if animeTitle == "" {
		animeTitle = cleanupRSSAnimeTitle(working)
	}
	if animeTitle == "" {
		animeTitle = original
	}
	return entity.RSSAnimeParsed{
		AnimeTitle:    animeTitle,
		Season:        season,
		Episode:       episode,
		SubtitleGroup: subtitleGroup,
		Resolution:    normalizeRSSResolution(rssResolutionPattern.FindString(original)),
	}
}

func stripRSSSubtitleGroup(title string) (string, string) {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return "", ""
	}
	open, close := "", ""
	switch {
	case strings.HasPrefix(trimmed, "["):
		open, close = "[", "]"
	case strings.HasPrefix(trimmed, "【"):
		open, close = "【", "】"
	default:
		return trimmed, ""
	}
	end := strings.Index(trimmed[len(open):], close)
	if end < 0 {
		return trimmed, ""
	}
	end += len(open)
	group := strings.TrimSpace(trimmed[len(open):end])
	if group == "" || isRSSMetadataToken(group) {
		return trimmed, ""
	}
	rest := strings.TrimSpace(trimmed[end+len(close):])
	return rest, group
}

func stripRSSReleaseDecorations(title string) string {
	out := strings.TrimSpace(title)
	for strings.HasPrefix(out, "★") {
		next := strings.Index(out[len("★"):], "★")
		if next < 0 {
			break
		}
		next += len("★")
		token := out[:next+len("★")]
		if !strings.Contains(token, "新番") && !strings.Contains(token, "月") {
			break
		}
		out = strings.TrimSpace(out[next+len("★"):])
	}
	return out
}

func parseRSSSeasonEpisode(title string) (string, string) {
	if matches := rssSxxEyyPattern.FindStringSubmatch(title); len(matches) >= 3 {
		return formatRSSSeason(matches[1]), normalizeRSSEpisodeValue(matches[2])
	}
	season := ""
	if matches := rssSeasonWordPattern.FindStringSubmatch(title); len(matches) >= 3 {
		if matches[1] != "" {
			season = formatRSSSeason(matches[1])
		} else {
			season = formatRSSSeason(matches[2])
		}
	}
	if season == "" {
		if matches := rssChineseSeasonPattern.FindStringSubmatch(title); len(matches) >= 2 {
			season = formatRSSSeason(matches[1])
		}
	}
	for _, expr := range []*regexp.Regexp{rssChineseEpisodePattern, rssEpisodeWordPattern, rssDashEpisodePattern} {
		if matches := expr.FindStringSubmatch(title); len(matches) >= 2 && isRSSEpisodeToken(matches[1]) {
			return season, normalizeRSSEpisodeValue(matches[1])
		}
	}
	for _, matches := range rssBracketTokenPattern.FindAllStringSubmatch(title, -1) {
		if len(matches) >= 2 && isRSSEpisodeToken(matches[1]) {
			return season, normalizeRSSEpisodeValue(matches[1])
		}
	}
	return season, ""
}

func extractRSSAnimeTitle(title string) string {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return ""
	}
	for _, expr := range []*regexp.Regexp{rssSxxEyyPattern, rssDashEpisodePattern, rssChineseEpisodePattern, rssEpisodeWordPattern} {
		if loc := expr.FindStringIndex(trimmed); loc != nil && loc[0] > 0 {
			return trimmed[:loc[0]]
		}
	}
	matches := rssBracketTokenPattern.FindAllStringSubmatchIndex(trimmed, -1)
	for i, match := range matches {
		if len(match) < 4 {
			continue
		}
		token := trimmed[match[2]:match[3]]
		if !isRSSEpisodeToken(token) {
			continue
		}
		if i > 0 {
			previous := matches[i-1]
			previousToken := strings.TrimSpace(trimmed[previous[2]:previous[3]])
			if previousToken != "" && !isRSSMetadataToken(previousToken) {
				return previousToken
			}
		}
		if match[0] > 0 {
			return trimmed[:match[0]]
		}
	}
	return trimRSSMetadataSuffix(trimmed)
}

func cleanupRSSAnimeTitle(title string) string {
	out := trimRSSMetadataSuffix(strings.TrimSpace(title))
	out = strings.Trim(out, " \t\r\n-–—_")
	out = strings.TrimSpace(out)
	if strings.HasPrefix(out, "[") && strings.HasSuffix(out, "]") && len(out) > 2 {
		out = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(out, "["), "]"))
	}
	if strings.HasPrefix(out, "【") && strings.HasSuffix(out, "】") && len(out) > len("【】") {
		out = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(out, "【"), "】"))
	}
	return out
}

func trimRSSMetadataSuffix(title string) string {
	out := strings.TrimSpace(title)
	for {
		changed := false
		if strings.HasSuffix(out, ")") {
			if start := strings.LastIndex(out, "("); start >= 0 {
				token := strings.TrimSpace(out[start+1 : len(out)-1])
				if isRSSMetadataToken(token) || isRSSEpisodeToken(token) {
					out = strings.TrimSpace(out[:start])
					changed = true
				}
			}
		}
		matches := rssBracketTokenPattern.FindAllStringSubmatchIndex(out, -1)
		if len(matches) > 0 {
			last := matches[len(matches)-1]
			if strings.TrimSpace(out[last[1]:]) == "" {
				token := strings.TrimSpace(out[last[2]:last[3]])
				if isRSSMetadataToken(token) || isRSSEpisodeToken(token) {
					out = strings.TrimSpace(out[:last[0]])
					changed = true
				}
			}
		}
		if !changed {
			return out
		}
	}
}

func isRSSMetadataToken(token string) bool {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	if normalizeRSSResolution(trimmed) != "" {
		return true
	}
	metadataHints := []string{
		"web-dl", "webrip", "baha", "aac", "avc", "hevc", "x264", "x265",
		"h264", "h265", "mp4", "mkv", "chs", "cht", "gb", "big5", "繁", "简",
		"字幕", "内嵌", "內嵌", "招募", "sc", "tc",
	}
	for _, hint := range metadataHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func isRSSEpisodeToken(token string) bool {
	trimmed := normalizeRSSEpisodeValue(token)
	if trimmed == "" {
		return false
	}
	if !regexp.MustCompile(`^[0-9]{1,4}(?:\.[0-9]+)?$`).MatchString(trimmed) {
		return false
	}
	integerPart := strings.SplitN(trimmed, ".", 2)[0]
	if len(integerPart) == 4 && integerPart >= "1900" && integerPart <= "2100" {
		return false
	}
	if normalizeRSSResolution(trimmed+"p") != "" && (trimmed == "480" || trimmed == "720" || trimmed == "1080" || trimmed == "1440" || trimmed == "2160") {
		return false
	}
	return true
}

func normalizeRSSEpisodeValue(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = regexp.MustCompile(`(?i)v[0-9]+$`).ReplaceAllString(trimmed, "")
	return strings.TrimSpace(trimmed)
}

func formatRSSSeason(value string) string {
	normalized := normalizeRSSNumber(value)
	if normalized == "" {
		return ""
	}
	if len(normalized) == 1 {
		return "S0" + normalized
	}
	return "S" + normalized
}

func normalizeRSSResolution(value string) string {
	match := rssResolutionPattern.FindString(strings.TrimSpace(value))
	if match == "" {
		return ""
	}
	lower := strings.ToLower(match)
	if strings.HasSuffix(lower, "p") {
		return strings.TrimSuffix(lower, "p") + "p"
	}
	return strings.ToUpper(lower)
}

func validateRSSDirectoryTemplate(template string) error {
	trimmed := strings.TrimSpace(template)
	if trimmed == "" {
		return nil
	}
	if strings.Contains(trimmed, "\\") || strings.HasPrefix(trimmed, "/") || strings.Contains(trimmed, "..") || hasRSSWindowsDrivePrefix(trimmed) {
		return ErrPathInvalid
	}
	if err := validateRSSTemplatePlaceholders(trimmed); err != nil {
		return err
	}
	_, err := renderRSSDirectoryTemplate(trimmed, &entity.RSSItem{})
	return err
}

func validateRSSFilenameTemplate(template string) error {
	trimmed := strings.TrimSpace(template)
	if trimmed == "" {
		return nil
	}
	if strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "/") || strings.Contains(trimmed, "..") || hasRSSWindowsDrivePrefix(trimmed) {
		return ErrPathInvalid
	}
	if err := validateRSSTemplatePlaceholders(trimmed); err != nil {
		return err
	}
	rendered := renderRSSTemplate(trimmed, &entity.RSSItem{})
	if strings.TrimSpace(rendered) == "" {
		return nil
	}
	segment := sanitizeRSSPathSegment(rendered)
	if segment == "" || strings.Contains(segment, "..") {
		return ErrPathInvalid
	}
	return nil
}

func validateRSSTemplatePlaceholders(template string) error {
	for i := 0; i < len(template); i++ {
		switch template[i] {
		case '{':
			end := strings.IndexByte(template[i+1:], '}')
			if end < 0 {
				return ErrPathInvalid
			}
			token := template[i+1 : i+1+end]
			if token == "" || strings.ContainsAny(token, "{}") {
				return ErrPathInvalid
			}
			if _, ok := rssAllowedTemplateTokens[token]; !ok {
				return ErrPathInvalid
			}
			i += end + 1
		case '}':
			return ErrPathInvalid
		}
	}
	return nil
}

func hasRSSWindowsDrivePrefix(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 2 || trimmed[1] != ':' {
		return false
	}
	ch := trimmed[0]
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func renderRSSTargetVirtualParentPath(subscription *entity.RSSSubscription, item *entity.RSSItem) (string, error) {
	if subscription == nil {
		return "", ErrPathInvalid
	}
	base, err := normalizeVirtualPath(strings.TrimSpace(subscription.TargetVirtualParentPath))
	if err != nil {
		return "", err
	}
	relativeDir, err := renderRSSDirectoryTemplate(subscription.DirectoryTemplate, item)
	if err != nil {
		return "", err
	}
	if relativeDir == "" {
		return base, nil
	}
	target := joinVirtualPath(base, relativeDir)
	normalized, err := normalizeVirtualPath(target)
	if err != nil {
		return "", err
	}
	if !isSubPath(base, normalized) {
		return "", ErrPathInvalid
	}
	return normalized, nil
}

func renderRSSDirectoryTemplate(template string, item *entity.RSSItem) (string, error) {
	trimmed := strings.TrimSpace(template)
	if trimmed == "" {
		return "", nil
	}
	if strings.Contains(trimmed, "\\") || strings.HasPrefix(trimmed, "/") || strings.Contains(trimmed, "..") || hasRSSWindowsDrivePrefix(trimmed) {
		return "", ErrPathInvalid
	}
	if err := validateRSSTemplatePlaceholders(trimmed); err != nil {
		return "", err
	}
	rendered := renderRSSTemplate(trimmed, item)
	return sanitizeRSSRelativePath(rendered)
}

func renderRSSFilenameTemplate(subscription *entity.RSSSubscription, item *entity.RSSItem) (string, error) {
	if subscription == nil {
		return "", ErrPathInvalid
	}
	trimmed := strings.TrimSpace(subscription.FilenameTemplate)
	if trimmed == "" {
		return "", nil
	}
	if err := validateRSSFilenameTemplate(trimmed); err != nil {
		return "", err
	}
	rendered := renderRSSTemplate(trimmed, item)
	targetFilename, err := normalizeTaskTargetFilename(rendered)
	if err != nil {
		return "", ErrPathInvalid
	}
	return targetFilename, nil
}

func renderRSSTemplate(template string, item *entity.RSSItem) string {
	if item == nil {
		item = &entity.RSSItem{}
	}
	parsed := item.Parsed
	if parsed.AnimeTitle == "" && parsed.Season == "" && parsed.Episode == "" && parsed.SubtitleGroup == "" && parsed.Resolution == "" && strings.TrimSpace(item.Title) != "" {
		parsed = parseRSSAnimeTitle(item.Title)
	}
	values := map[string]string{
		"anime_title":    firstNonEmpty(parsed.AnimeTitle, item.Title),
		"season":         parsed.Season,
		"episode":        parsed.Episode,
		"subtitle_group": parsed.SubtitleGroup,
		"resolution":     parsed.Resolution,
		"title":          item.Title,
	}
	return rssTemplatePlaceholderPattern.ReplaceAllStringFunc(template, func(placeholder string) string {
		matches := rssTemplatePlaceholderPattern.FindStringSubmatch(placeholder)
		if len(matches) != 2 {
			return ""
		}
		return sanitizeRSSPathSegment(values[matches[1]])
	})
}

func sanitizeRSSRelativePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "..") {
		return "", ErrPathInvalid
	}
	segments := strings.Split(trimmed, "/")
	out := make([]string, 0, len(segments))
	for _, segment := range segments {
		cleaned := sanitizeRSSPathSegment(segment)
		if cleaned == "" {
			continue
		}
		if cleaned == "." || cleaned == ".." || strings.Contains(cleaned, "..") {
			return "", ErrPathInvalid
		}
		out = append(out, cleaned)
	}
	return strings.Join(out, "/"), nil
}

func sanitizeRSSPathSegment(value string) string {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r == '/' || r == '\\' || r == 0 || unicode.IsControl(r):
			builder.WriteRune(' ')
		case strings.ContainsRune(`<>:"|?*`, r):
			builder.WriteRune(' ')
		default:
			builder.WriteRune(r)
		}
	}
	collapsed := strings.Join(strings.Fields(builder.String()), " ")
	collapsed = strings.Trim(collapsed, " .")
	for strings.Contains(collapsed, "..") {
		collapsed = strings.ReplaceAll(collapsed, "..", ".")
	}
	collapsed = strings.Trim(collapsed, " .")
	if collapsed == "." || collapsed == ".." {
		return ""
	}
	return collapsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
		DirectoryTemplate:       subscription.DirectoryTemplate,
		FilenameTemplate:        subscription.FilenameTemplate,
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
		ID:          item.ID,
		UserID:      item.UserID,
		SourceID:    item.SourceID,
		Title:       item.Title,
		Link:        item.Link,
		PublishedAt: publishedAt,
		GUID:        item.GUID,
		DownloadURL: item.DownloadURL,
		LinkType:    item.LinkType,
		Parsed: appdto.RSSAnimeParsedView{
			AnimeTitle:    item.Parsed.AnimeTitle,
			Season:        item.Parsed.Season,
			Episode:       item.Parsed.Episode,
			SubtitleGroup: item.Parsed.SubtitleGroup,
			Resolution:    item.Parsed.Resolution,
		},
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
