package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	appaudit "yunxia/internal/application/audit"
	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

// SourceService 负责存储源管理。
type SourceService struct {
	sourceRepo       domainrepo.SourceRepository
	systemConfigRepo domainrepo.SystemConfigRepository
	aclAuthorizer    *ACLAuthorizer
	configCodecs     map[string]SourceConfigCodec
	driverProbes     map[string]SourceDriverProbe
	mountSyncer      MetadataSourceMountSyncer
	transactor       domainrepo.Transactor
	logger           *slog.Logger
	auditRecorder    *appaudit.Recorder
}

// NewSourceService 创建存储源服务。
func NewSourceService(sourceRepo domainrepo.SourceRepository, systemConfigRepo domainrepo.SystemConfigRepository, options ...SourceServiceOption) *SourceService {
	service := &SourceService{
		sourceRepo:       sourceRepo,
		systemConfigRepo: systemConfigRepo,
		configCodecs:     make(map[string]SourceConfigCodec),
		driverProbes:     make(map[string]SourceDriverProbe),
		logger:           newServiceLogger("service.source"),
	}
	service.registerDefaultConfigCodecs()
	for _, option := range options {
		option(service)
	}
	return service
}

// List 返回存储源列表。
func (s *SourceService) List(ctx context.Context, view string) (*appdto.SourceListResponse, error) {
	if view == "" {
		view = "navigation"
	}

	var (
		sources []*entity.StorageSource
		err     error
	)
	if view == "admin" {
		sources, err = s.sourceRepo.ListAll(ctx)
	} else {
		sources, err = s.sourceRepo.ListEnabled(ctx)
	}
	if err != nil {
		return nil, err
	}

	items := make([]appdto.StorageSourceView, 0, len(sources))
	for _, source := range sources {
		if view != "admin" && s.aclAuthorizer != nil {
			visible, visErr := s.aclAuthorizer.CanSeeSource(ctx, source.ID)
			if visErr != nil {
				return nil, visErr
			}
			if !visible {
				continue
			}
		}
		items = append(items, s.toSourceView(source))
	}

	return &appdto.SourceListResponse{
		Items: items,
		View:  view,
	}, nil
}

// Get 返回单个存储源详情。
func (s *SourceService) Get(ctx context.Context, id uint) (*appdto.SourceDetailResponse, error) {
	source, err := s.sourceRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	config, secretFields, err := s.sourceDetailConfig(ctx, source)
	if err != nil {
		return nil, err
	}

	var lastCheckedAt *string
	if source.LastCheckedAt != nil {
		formatted := source.LastCheckedAt.Format(time.RFC3339)
		lastCheckedAt = &formatted
	}

	return &appdto.SourceDetailResponse{
		Source:        s.toSourceView(source),
		Config:        config,
		SecretFields:  secretFields,
		LastCheckedAt: lastCheckedAt,
	}, nil
}

// Test 测试存储源配置。
func (s *SourceService) Test(ctx context.Context, req appdto.SourceUpsertRequest) (*appdto.SourceTestResponse, error) {
	start := time.Now()
	source, err := s.buildSourceEntity(req, nil)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "test",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
		})
		return nil, err
	}
	if err := s.validateSource(ctx, source); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "test",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
			Detail:       sourceTestDetail(source, time.Since(start).Milliseconds(), false),
		})
		return nil, err
	}

	checkedAt := time.Now()
	resp := &appdto.SourceTestResponse{
		Reachable: true,
		Status:    "online",
		LatencyMS: time.Since(start).Milliseconds(),
		CheckedAt: checkedAt.Format(time.RFC3339),
		Warnings:  []string{},
	}
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "storage_source",
		Action:       "test",
		Result:       appaudit.ResultSuccess,
		Detail:       sourceTestDetail(source, resp.LatencyMS, true),
	})
	return resp, nil
}

// Retest 重新测试已保存存储源。
func (s *SourceService) Retest(ctx context.Context, id uint) (*appdto.SourceTestResponse, error) {
	source, err := s.sourceRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "test",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return nil, err
	}
	if err := s.validateSource(ctx, source); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "test",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
			ResourceID:   encodeUintID(id),
			Detail:       sourceTestDetail(source, 0, false),
		})
		return nil, err
	}

	checkedAt := time.Now()
	resp := &appdto.SourceTestResponse{
		Reachable: true,
		Status:    "online",
		LatencyMS: 0,
		CheckedAt: checkedAt.Format(time.RFC3339),
		Warnings:  []string{},
	}
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "storage_source",
		Action:       "test",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		Detail:       sourceTestDetail(source, 0, true),
	})
	return resp, nil
}

// Create 创建存储源。
func (s *SourceService) Create(ctx context.Context, req appdto.SourceUpsertRequest) (*appdto.StorageSourceView, error) {
	if _, err := s.sourceRepo.FindByName(ctx, req.Name); err == nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_NAME_CONFLICT",
		})
		return nil, ErrSourceNameConflict
	} else if !errors.Is(err, domainrepo.ErrNotFound) {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
		})
		return nil, err
	}

	source, err := s.buildSourceEntity(req, nil)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
		})
		return nil, err
	}
	if err := s.ensureMountPathAvailable(ctx, source.MountPath, 0); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
		})
		return nil, err
	}
	if err := s.assignUniqueWebDAVSlug(ctx, source, 0); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
		})
		return nil, err
	}
	if err := s.validateSource(ctx, source); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
		})
		return nil, err
	}
	if err := s.withinSourceTx(ctx, func(txCtx context.Context) error {
		if err := s.sourceRepo.Create(txCtx, source); err != nil {
			return err
		}
		return s.syncSourceMountAfterMutation(txCtx, source)
	}); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
		})
		return nil, err
	}

	view := s.toSourceView(source)
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "storage_source",
		Action:       "create",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(source.ID),
		SourceID:     &source.ID,
		VirtualPath:  source.MountPath,
		After:        s.sourceAuditView(source),
	})
	return &view, nil
}

// Update 更新存储源。
func (s *SourceService) Update(ctx context.Context, id uint, req appdto.SourceUpsertRequest) (*appdto.StorageSourceView, error) {
	existing, err := s.sourceRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return nil, err
	}
	before := s.sourceAuditView(existing)
	req.DriverType = existing.DriverType

	source, err := s.buildSourceEntity(req, existing)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}
	if err := s.ensureMountPathAvailable(ctx, source.MountPath, existing.ID); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}
	if err := s.validateSource(ctx, source); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}
	source.ID = existing.ID
	source.CreatedAt = existing.CreatedAt
	source.WebDAVSlug = existing.WebDAVSlug

	if err := s.withinSourceTx(ctx, func(txCtx context.Context) error {
		if err := s.sourceRepo.Update(txCtx, source); err != nil {
			return err
		}
		return s.syncSourceMountAfterMutation(txCtx, source)
	}); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}

	view := s.toSourceView(source)
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "storage_source",
		Action:       "update",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		SourceID:     &source.ID,
		VirtualPath:  source.MountPath,
		Before:       before,
		After:        s.sourceAuditView(source),
	})
	return &view, nil
}

// Delete 删除存储源。
func (s *SourceService) Delete(ctx context.Context, id uint) error {
	source, err := s.sourceRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "delete",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return err
	}
	before := s.sourceAuditView(source)

	cfg, err := s.systemConfigRepo.Get(ctx)
	if err == nil && cfg.DefaultSourceID != nil && *cfg.DefaultSourceID == id {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "delete",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_IN_USE",
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return ErrSourceInUse
	}
	if err != nil && !errors.Is(err, domainrepo.ErrNotFound) {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "delete",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return err
	}

	if err := s.withinSourceTx(ctx, func(txCtx context.Context) error {
		if s.mountSyncer != nil {
			if err := s.mountSyncer.DisableSourceMount(txCtx, id); err != nil {
				return fmt.Errorf("%w: %w", ErrMetadataVFSMountSyncFailed, err)
			}
		}
		return s.sourceRepo.Delete(txCtx, id)
	}); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "storage_source",
			Action:       "delete",
			Result:       appaudit.ResultFailed,
			ErrorCode:    sourceErrorCode(err),
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return err
	}
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "storage_source",
		Action:       "delete",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		SourceID:     &source.ID,
		VirtualPath:  source.MountPath,
		Before:       before,
	})
	return nil
}

func (s *SourceService) buildSourceEntity(req appdto.SourceUpsertRequest, existing *entity.StorageSource) (*entity.StorageSource, error) {
	now := time.Now()
	rootPath, err := normalizeVirtualPath(req.RootPath)
	if err != nil {
		return nil, err
	}
	codec, err := s.sourceConfigCodec(req.DriverType)
	if err != nil {
		return nil, err
	}
	mountPath, err := s.resolveSourceMountPath(req, existing)
	if err != nil {
		return nil, err
	}

	source := &entity.StorageSource{
		Name:            req.Name,
		DriverType:      req.DriverType,
		Status:          "online",
		IsEnabled:       req.IsEnabled,
		IsWebDAVExposed: req.IsWebDAVExposed,
		WebDAVReadOnly:  req.WebDAVReadOnly,
		MountPath:       mountPath,
		RootPath:        rootPath,
		SortOrder:       req.SortOrder,
		LastCheckedAt:   timePointer(now),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if existing != nil {
		source.CreatedAt = existing.CreatedAt
	}

	existingRaw := ""
	if existing != nil {
		existingRaw = existing.ConfigJSON
	}
	configJSON, err := codec.Build(req.Config, req.SecretPatch, existingRaw)
	if err != nil {
		return nil, err
	}
	source.ConfigJSON = configJSON
	if existing != nil {
		source.WebDAVSlug = existing.WebDAVSlug
	} else {
		source.WebDAVSlug = generateSlug(req.Name, codec.DefaultMountSlug())
	}

	return source, nil
}

func (s *SourceService) sourceDetailConfig(ctx context.Context, source *entity.StorageSource) (map[string]any, map[string]appdto.SecretFieldMask, error) {
	codec, err := s.sourceConfigCodec(source.DriverType)
	if err != nil {
		return nil, nil, ErrSourceDriverUnsupported
	}
	canReadSecret := false
	if auth, ok := security.RequestAuthFromContext(ctx); ok {
		canReadSecret = permission.HasCapability(auth.Capabilities, permission.CapabilitySourceSecretRead)
	}
	return codec.Public(source.ConfigJSON, canReadSecret)
}

func (s *SourceService) toSourceView(source *entity.StorageSource) appdto.StorageSourceView {
	createdAt := source.CreatedAt.Format(time.RFC3339)
	updatedAt := source.UpdatedAt.Format(time.RFC3339)

	var usedBytes *int64
	if source.DriverType == "local" {
		cfg, err := parseLocalSourceConfig(source)
		if err == nil {
			usedBytes = computeUsedBytes(cfg.BasePath)
		}
	}

	return appdto.StorageSourceView{
		ID:              source.ID,
		Name:            source.Name,
		DriverType:      source.DriverType,
		Status:          source.Status,
		IsEnabled:       source.IsEnabled,
		IsWebDAVExposed: source.IsWebDAVExposed,
		WebDAVReadOnly:  source.WebDAVReadOnly,
		WebDAVSlug:      source.WebDAVSlug,
		MountPath:       source.MountPath,
		RootPath:        source.RootPath,
		UsedBytes:       usedBytes,
		TotalBytes:      nil,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}
}

func parseLocalConfigMap(config map[string]any) (localSourceConfig, error) {
	basePath, _ := config["base_path"].(string)
	if basePath == "" {
		return localSourceConfig{}, ErrPathInvalid
	}

	return localSourceConfig{BasePath: filepath.ToSlash(filepath.Clean(basePath))}, nil
}

func validateLocalBasePath(basePath string) error {
	info, err := os.Stat(basePath)
	if err != nil {
		return ErrPathInvalid
	}
	if !info.IsDir() {
		return ErrPathInvalid
	}

	return nil
}

func (s *SourceService) registerDefaultConfigCodecs() {
	for _, codec := range []SourceConfigCodec{
		NewLocalSourceConfigCodec(),
		NewS3SourceConfigCodec(),
		NewPikPakSourceConfigCodec(),
	} {
		s.configCodecs[codec.DriverType()] = codec
	}
}

func (s *SourceService) sourceConfigCodec(driverType string) (SourceConfigCodec, error) {
	codec, exists := s.configCodecs[driverType]
	if !exists || codec == nil {
		return nil, ErrSourceDriverUnsupported
	}
	return codec, nil
}

func (s *SourceService) sourceAuditView(source *entity.StorageSource) map[string]any {
	view := sourceAuditView(source)
	if source == nil {
		return view
	}
	if codec, err := s.sourceConfigCodec(source.DriverType); err == nil {
		view["config"] = codec.AuditView(source.ConfigJSON)
	}
	return view
}

func ensureDefaultLocalSource(ctx context.Context, repo domainrepo.SourceRepository, options SystemOptions) (*entity.StorageSource, error) {
	enabled, err := repo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if len(enabled) > 0 {
		return enabled[0], nil
	}

	basePath := filepath.Join(options.StorageDataDir, "default")
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, err
	}
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		return nil, err
	}

	source := &entity.StorageSource{
		Name:            "本地存储",
		DriverType:      "local",
		Status:          "online",
		IsEnabled:       true,
		IsWebDAVExposed: false,
		WebDAVReadOnly:  true,
		WebDAVSlug:      "local",
		MountPath:       "/local",
		RootPath:        "/",
		SortOrder:       0,
		ConfigJSON:      configJSON,
		LastCheckedAt:   timePointer(time.Now()),
	}
	if err := repo.Create(ctx, source); err != nil {
		return nil, err
	}

	return source, nil
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func (s *SourceService) resolveSourceMountPath(req appdto.SourceUpsertRequest, existing *entity.StorageSource) (string, error) {
	mountPath := req.MountPath
	switch {
	case mountPath != "":
	case existing != nil && existing.MountPath != "":
		mountPath = existing.MountPath
	case existing != nil && existing.WebDAVSlug != "":
		mountPath = "/" + existing.WebDAVSlug
	default:
		fallback := generateSlug(req.Name, s.defaultSourceMountSlug(req.DriverType))
		mountPath = "/" + fallback
	}

	return normalizeVirtualPath(mountPath)
}

func (s *SourceService) defaultSourceMountSlug(driverType string) string {
	codec, err := s.sourceConfigCodec(driverType)
	if err != nil {
		return "source-local"
	}
	return codec.DefaultMountSlug()
}

func (s *SourceService) ensureMountPathAvailable(ctx context.Context, mountPath string, excludeID uint) error {
	registry := NewMountRegistry(s.sourceRepo)
	conflict, err := registry.HasMountPathConflict(ctx, mountPath, excludeID)
	if err != nil {
		return err
	}
	if conflict {
		return ErrSourceMountPathConflict
	}

	return nil
}

func (s *SourceService) assignUniqueWebDAVSlug(ctx context.Context, source *entity.StorageSource, excludeID uint) error {
	sources, err := s.sourceRepo.ListAll(ctx)
	if err != nil {
		return err
	}

	used := make(map[string]struct{}, len(sources))
	for _, existing := range sources {
		if existing.ID == excludeID || existing.WebDAVSlug == "" {
			continue
		}
		used[existing.WebDAVSlug] = struct{}{}
	}

	base := source.WebDAVSlug
	if base == "" {
		base = s.defaultSourceMountSlug(source.DriverType)
	}
	if _, exists := used[base]; !exists {
		source.WebDAVSlug = base
		return nil
	}

	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if _, exists := used[candidate]; !exists {
			source.WebDAVSlug = candidate
			return nil
		}
	}
}

func (s *SourceService) validateSource(ctx context.Context, source *entity.StorageSource) error {
	if source.DriverType == "local" {
		return nil
	}
	probe, exists := s.driverProbes[source.DriverType]
	if !exists {
		return ErrSourceDriverUnsupported
	}
	if err := probe.Test(ctx, source); err != nil {
		return fmt.Errorf("%w: %w", ErrSourceConnectionFailed, err)
	}
	return nil
}

func (s *SourceService) syncSourceMountAfterMutation(ctx context.Context, source *entity.StorageSource) error {
	if s.mountSyncer == nil {
		return nil
	}
	if _, err := s.mountSyncer.SyncSourceMount(ctx, source); err != nil {
		return fmt.Errorf("%w: %w", ErrMetadataVFSMountSyncFailed, err)
	}
	return nil
}

func (s *SourceService) withinSourceTx(ctx context.Context, fn func(context.Context) error) error {
	if s.transactor == nil {
		return fn(ctx)
	}
	return s.transactor.WithinTx(ctx, fn)
}
