package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
	infraStorage "yunxia/internal/infrastructure/storage"
)

// SourceService 负责存储源管理。
type SourceService struct {
	sourceRepo       domainrepo.SourceRepository
	systemConfigRepo domainrepo.SystemConfigRepository
	aclAuthorizer    *ACLAuthorizer
	driverProbes     map[string]SourceDriverProbe
}

// NewSourceService 创建存储源服务。
func NewSourceService(sourceRepo domainrepo.SourceRepository, systemConfigRepo domainrepo.SystemConfigRepository, options ...SourceServiceOption) *SourceService {
	service := &SourceService{
		sourceRepo:       sourceRepo,
		systemConfigRepo: systemConfigRepo,
		driverProbes:     make(map[string]SourceDriverProbe),
	}
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

	config, secretFields, err := s.sourceDetailConfig(source)
	if err != nil {
		return nil, err
	}
	if auth, ok := security.RequestAuthFromContext(ctx); ok &&
		permission.HasCapability(auth.Capabilities, permission.CapabilitySourceSecretRead) &&
		source.DriverType == "s3" {
		s3cfg, err := infraStorage.ParseS3ConfigJSON(source.ConfigJSON)
		if err != nil {
			return nil, err
		}
		config["access_key"] = s3cfg.AccessKey
		config["secret_key"] = s3cfg.SecretKey
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
		return nil, err
	}
	if err := s.validateSource(ctx, source); err != nil {
		return nil, err
	}

	checkedAt := time.Now()
	return &appdto.SourceTestResponse{
		Reachable: true,
		Status:    "online",
		LatencyMS: time.Since(start).Milliseconds(),
		CheckedAt: checkedAt.Format(time.RFC3339),
		Warnings:  []string{},
	}, nil
}

// Retest 重新测试已保存存储源。
func (s *SourceService) Retest(ctx context.Context, id uint) (*appdto.SourceTestResponse, error) {
	source, err := s.sourceRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.validateSource(ctx, source); err != nil {
		return nil, err
	}

	checkedAt := time.Now()
	return &appdto.SourceTestResponse{
		Reachable: true,
		Status:    "online",
		LatencyMS: 0,
		CheckedAt: checkedAt.Format(time.RFC3339),
		Warnings:  []string{},
	}, nil
}

// Create 创建存储源。
func (s *SourceService) Create(ctx context.Context, req appdto.SourceUpsertRequest) (*appdto.StorageSourceView, error) {
	if _, err := s.sourceRepo.FindByName(ctx, req.Name); err == nil {
		return nil, ErrSourceNameConflict
	} else if !errors.Is(err, domainrepo.ErrNotFound) {
		return nil, err
	}

	source, err := s.buildSourceEntity(req, nil)
	if err != nil {
		return nil, err
	}
	if err := s.validateSource(ctx, source); err != nil {
		return nil, err
	}
	if err := s.sourceRepo.Create(ctx, source); err != nil {
		return nil, err
	}

	view := s.toSourceView(source)
	return &view, nil
}

// Update 更新存储源。
func (s *SourceService) Update(ctx context.Context, id uint, req appdto.SourceUpsertRequest) (*appdto.StorageSourceView, error) {
	existing, err := s.sourceRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	req.DriverType = existing.DriverType

	source, err := s.buildSourceEntity(req, existing)
	if err != nil {
		return nil, err
	}
	if err := s.validateSource(ctx, source); err != nil {
		return nil, err
	}
	source.ID = existing.ID
	source.CreatedAt = existing.CreatedAt
	source.WebDAVSlug = existing.WebDAVSlug

	if err := s.sourceRepo.Update(ctx, source); err != nil {
		return nil, err
	}

	view := s.toSourceView(source)
	return &view, nil
}

// Delete 删除存储源。
func (s *SourceService) Delete(ctx context.Context, id uint) error {
	cfg, err := s.systemConfigRepo.Get(ctx)
	if err == nil && cfg.DefaultSourceID != nil && *cfg.DefaultSourceID == id {
		return ErrSourceInUse
	}
	if err != nil && !errors.Is(err, domainrepo.ErrNotFound) {
		return err
	}

	return s.sourceRepo.Delete(ctx, id)
}

func (s *SourceService) buildSourceEntity(req appdto.SourceUpsertRequest, existing *entity.StorageSource) (*entity.StorageSource, error) {
	now := time.Now()
	source := &entity.StorageSource{
		Name:            req.Name,
		DriverType:      req.DriverType,
		Status:          "online",
		IsEnabled:       req.IsEnabled,
		IsWebDAVExposed: req.IsWebDAVExposed,
		WebDAVReadOnly:  req.WebDAVReadOnly,
		RootPath:        req.RootPath,
		SortOrder:       req.SortOrder,
		LastCheckedAt:   timePointer(now),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if existing != nil {
		source.CreatedAt = existing.CreatedAt
	}

	switch req.DriverType {
	case "local":
		cfg, err := parseLocalConfigMap(req.Config)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(cfg.BasePath, 0o755); err != nil {
			return nil, err
		}
		configJSON, err := marshalLocalSourceConfig(cfg.BasePath)
		if err != nil {
			return nil, err
		}
		source.ConfigJSON = configJSON
		if existing != nil {
			source.WebDAVSlug = existing.WebDAVSlug
		} else {
			source.WebDAVSlug = generateSlug(req.Name, "source-local")
		}
	case "s3":
		var existingCfg *infraStorage.S3Config
		if existing != nil && existing.ConfigJSON != "" {
			parsed, parseErr := infraStorage.ParseS3ConfigJSON(existing.ConfigJSON)
			if parseErr != nil {
				return nil, fmt.Errorf("%w: %v", ErrConfigInvalid, parseErr)
			}
			existingCfg = &parsed
		}

		cfg, err := infraStorage.BuildS3Config(req.Config, req.SecretPatch, existingCfg)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrConfigInvalid, err)
		}
		configJSON, err := cfg.Marshal()
		if err != nil {
			return nil, err
		}
		source.ConfigJSON = configJSON
		if existing != nil {
			source.WebDAVSlug = existing.WebDAVSlug
		} else {
			source.WebDAVSlug = generateSlug(req.Name, "source-s3")
		}
	default:
		return nil, ErrSourceDriverUnsupported
	}

	return source, nil
}

func (s *SourceService) sourceDetailConfig(source *entity.StorageSource) (map[string]any, map[string]appdto.SecretFieldMask, error) {
	switch source.DriverType {
	case "local":
		config := map[string]any{}
		if err := json.Unmarshal([]byte(source.ConfigJSON), &config); err != nil {
			return nil, nil, err
		}
		return config, map[string]appdto.SecretFieldMask{}, nil
	case "s3":
		cfg, err := infraStorage.ParseS3ConfigJSON(source.ConfigJSON)
		if err != nil {
			return nil, nil, err
		}
		return cfg.PublicMap(), buildS3SecretMasks(cfg), nil
	default:
		return nil, nil, ErrSourceDriverUnsupported
	}
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

func (s *SourceService) validateSource(ctx context.Context, source *entity.StorageSource) error {
	if source.DriverType == "local" {
		return nil
	}
	probe, exists := s.driverProbes[source.DriverType]
	if !exists {
		return ErrSourceDriverUnsupported
	}
	if err := probe.Test(ctx, source); err != nil {
		return fmt.Errorf("%w: %v", ErrSourceConnectionFailed, err)
	}
	return nil
}

func buildS3SecretMasks(cfg infraStorage.S3Config) map[string]appdto.SecretFieldMask {
	return map[string]appdto.SecretFieldMask{
		"access_key": {
			Configured: cfg.AccessKey != "",
			Masked:     maskAccessKey(cfg.AccessKey),
		},
		"secret_key": {
			Configured: cfg.SecretKey != "",
			Masked:     maskSecretValue(cfg.SecretKey),
		},
	}
}

func maskAccessKey(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	keep := 4
	if len(runes) < keep {
		keep = len(runes)
	}
	return string(runes[:keep]) + "****"
}

func maskSecretValue(value string) string {
	if value == "" {
		return ""
	}
	return "******"
}
