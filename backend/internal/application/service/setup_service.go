package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	appaudit "yunxia/internal/application/audit"
	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

// SystemOptions 表示系统默认配置。
type SystemOptions struct {
	SiteName         string
	MultiUserEnabled bool
	DefaultSourceID  *uint
	MaxUploadSize    int64
	DefaultChunkSize int64
	WebDAVEnabled    bool
	WebDAVPrefix     string
	Theme            string
	Language         string
	TimeZone         string
	StorageDataDir   string
	TempDir          string
}

var (
	// ErrSetupAlreadyCompleted 表示系统已完成初始化。
	ErrSetupAlreadyCompleted = errors.New("setup already completed")
	// ErrInvalidCredentials 表示用户名或密码错误。
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrRefreshTokenInvalid 表示刷新令牌无效。
	ErrRefreshTokenInvalid = errors.New("refresh token invalid")
	// ErrAccountLocked 表示账号已锁定。
	ErrAccountLocked = errors.New("account locked")
)

// DefaultSystemOptions 返回默认系统配置。
func DefaultSystemOptions() SystemOptions {
	return SystemOptions{
		SiteName:         "云匣",
		MultiUserEnabled: false,
		DefaultSourceID:  nil,
		MaxUploadSize:    10 * 1024 * 1024 * 1024,
		DefaultChunkSize: 5 * 1024 * 1024,
		WebDAVEnabled:    true,
		WebDAVPrefix:     "/dav",
		Theme:            "system",
		Language:         "zh-CN",
		TimeZone:         "Asia/Shanghai",
		StorageDataDir:   "./data/storage",
		TempDir:          "./data/temp",
	}
}

type passwordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) bool
}

type tokenService interface {
	IssueAccessToken(userID uint, roleKey string, tokenVersion int) (string, error)
	IssueRefreshToken(userID uint, roleKey string, tokenVersion int) (string, error)
	ValidateRefreshToken(token string) (*security.Claims, error)
	AccessTokenTTL() time.Duration
	RefreshTokenTTL() time.Duration
}

// SetupService 负责系统初始化。
type SetupService struct {
	userRepo         domainrepo.UserRepository
	refreshTokenRepo domainrepo.RefreshTokenRepository
	systemConfigRepo domainrepo.SystemConfigRepository
	sourceRepo       domainrepo.SourceRepository
	hasher           passwordHasher
	tokens           tokenService
	options          SystemOptions
	logger           *slog.Logger
	auditRecorder    *appaudit.Recorder
}

// NewSetupService 创建初始化服务。
func NewSetupService(
	userRepo domainrepo.UserRepository,
	refreshTokenRepo domainrepo.RefreshTokenRepository,
	systemConfigRepo domainrepo.SystemConfigRepository,
	sourceRepo domainrepo.SourceRepository,
	hasher passwordHasher,
	tokens tokenService,
	options SystemOptions,
	serviceOptions ...SetupServiceOption,
) *SetupService {
	service := &SetupService{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		systemConfigRepo: systemConfigRepo,
		sourceRepo:       sourceRepo,
		hasher:           hasher,
		tokens:           tokens,
		options:          options,
		logger:           newServiceLogger("service.setup"),
	}
	for _, option := range serviceOptions {
		option(service)
	}
	return service
}

// Status 返回初始化状态。
func (s *SetupService) Status(ctx context.Context) (*appdto.SetupStatusResponse, error) {
	count, err := s.userRepo.Count(ctx)
	if err != nil {
		return nil, err
	}

	hasSuperAdmin := count > 0
	return &appdto.SetupStatusResponse{
		IsInitialized: hasSuperAdmin,
		SetupRequired: !hasSuperAdmin,
		HasSuperAdmin: hasSuperAdmin,
	}, nil
}

// Init 创建首个管理员并返回登录态。
func (s *SetupService) Init(ctx context.Context, req appdto.SetupInitRequest) (*appdto.SetupInitResponse, error) {
	count, err := s.userRepo.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrSetupAlreadyCompleted
	}

	passwordHash, err := s.hasher.Hash(req.Password)
	if err != nil {
		return nil, err
	}

	user := &entity.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		RoleKey:      permission.RoleSuperAdmin,
		Status:       permission.StatusActive,
		TokenVersion: 0,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	defaultSource, err := ensureDefaultLocalSource(ctx, s.sourceRepo, s.options)
	if err != nil {
		return nil, err
	}

	cfg := defaultSystemConfigEntity(s.options)
	cfg.DefaultSourceID = &defaultSource.ID
	if err := s.systemConfigRepo.Upsert(ctx, cfg); err != nil {
		return nil, err
	}

	tokenPair, err := issueAndStoreTokenPair(ctx, user, s.refreshTokenRepo, s.tokens)
	if err != nil {
		return nil, err
	}

	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "setup",
		Action:       "init",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(user.ID),
		After: map[string]any{
			"user": map[string]any{
				"id":       user.ID,
				"username": user.Username,
				"email":    user.Email,
				"role_key": user.RoleKey,
				"status":   user.Status,
			},
			"default_source_id": defaultSource.ID,
		},
	})

	return &appdto.SetupInitResponse{
		User:   toUserSummary(user),
		Tokens: tokenPair,
	}, nil
}

// AuthService 负责登录、刷新、登出和获取当前用户。
type AuthService struct {
	userRepo         domainrepo.UserRepository
	refreshTokenRepo domainrepo.RefreshTokenRepository
	hasher           passwordHasher
	tokens           tokenService
}

// NewAuthService 创建认证服务。
func NewAuthService(
	userRepo domainrepo.UserRepository,
	refreshTokenRepo domainrepo.RefreshTokenRepository,
	hasher passwordHasher,
	tokens tokenService,
) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		hasher:           hasher,
		tokens:           tokens,
	}
}

// Login 使用用户名密码登录。
func (s *AuthService) Login(ctx context.Context, req appdto.LoginRequest) (*appdto.LoginResponse, error) {
	user, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if user.Status == permission.StatusLocked {
		return nil, ErrAccountLocked
	}
	if !s.hasher.Compare(user.PasswordHash, req.Password) {
		return nil, ErrInvalidCredentials
	}

	tokenPair, err := issueAndStoreTokenPair(ctx, user, s.refreshTokenRepo, s.tokens)
	if err != nil {
		return nil, err
	}

	return &appdto.LoginResponse{
		User:   toUserSummary(user),
		Tokens: tokenPair,
	}, nil
}

// Refresh 使用 refresh token 轮换访问令牌。
func (s *AuthService) Refresh(ctx context.Context, req appdto.RefreshRequest) (*appdto.RefreshResponse, error) {
	claims, err := s.tokens.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, ErrRefreshTokenInvalid
	}

	token, err := s.refreshTokenRepo.FindByTokenHash(ctx, hashToken(req.RefreshToken))
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return nil, ErrRefreshTokenInvalid
		}
		return nil, err
	}
	if token.RevokedAt != nil || time.Now().After(token.ExpiresAt) {
		return nil, ErrRefreshTokenInvalid
	}

	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if user.Status == permission.StatusLocked || user.TokenVersion != claims.TokenVersion {
		return nil, ErrRefreshTokenInvalid
	}

	if err := s.refreshTokenRepo.RevokeByTokenHash(ctx, hashToken(req.RefreshToken)); err != nil {
		return nil, err
	}

	tokenPair, err := issueAndStoreTokenPair(ctx, user, s.refreshTokenRepo, s.tokens)
	if err != nil {
		return nil, err
	}

	return &appdto.RefreshResponse{Tokens: tokenPair}, nil
}

// Logout 撤销当前刷新令牌。
func (s *AuthService) Logout(ctx context.Context, req appdto.LogoutRequest) error {
	return s.refreshTokenRepo.RevokeByTokenHash(ctx, hashToken(req.RefreshToken))
}

// Me 获取当前用户摘要。
func (s *AuthService) Me(ctx context.Context, userID uint) (*appdto.CurrentUserResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	capabilities, err := permission.ResolveCapabilities(user.RoleKey)
	if err != nil {
		return nil, err
	}
	return &appdto.CurrentUserResponse{
		User:         toUserSummary(user),
		Capabilities: capabilities,
	}, nil
}

// SystemService 负责系统配置读取和更新。
type SystemService struct {
	systemConfigRepo domainrepo.SystemConfigRepository
	options          SystemOptions
	statsUserRepo    systemStatsUserRepository
	statsSourceRepo  systemStatsSourceRepository
	statsTaskRepo    systemStatsTaskRepository
	fileDrivers      map[string]FileDriver
	logger           *slog.Logger
	auditRecorder    *appaudit.Recorder
}

// NewSystemService 创建系统服务。
func NewSystemService(systemConfigRepo domainrepo.SystemConfigRepository, options SystemOptions, serviceOptions ...SystemServiceOption) *SystemService {
	service := &SystemService{
		systemConfigRepo: systemConfigRepo,
		options:          options,
		logger:           newServiceLogger("service.system"),
	}
	for _, option := range serviceOptions {
		option(service)
	}
	return service
}

// GetConfig 获取当前系统配置，不存在时返回默认值。
func (s *SystemService) GetConfig(ctx context.Context) (*appdto.SystemConfigPublic, error) {
	cfg, err := s.systemConfigRepo.Get(ctx)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			defaultCfg := defaultSystemConfigEntity(s.options)
			dto := toSystemConfigPublic(defaultCfg)
			return &dto, nil
		}
		return nil, err
	}

	dto := toSystemConfigPublic(cfg)
	return &dto, nil
}

// UpdateConfig 更新系统配置。
func (s *SystemService) UpdateConfig(ctx context.Context, req appdto.UpdateSystemConfigRequest) (*appdto.SystemConfigPublic, error) {
	current, err := s.systemConfigRepo.Get(ctx)
	if err != nil && !errors.Is(err, domainrepo.ErrNotFound) {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "system_config",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
		})
		return nil, err
	}
	if errors.Is(err, domainrepo.ErrNotFound) {
		current = defaultSystemConfigEntity(s.options)
	}
	before := systemConfigAuditView(current)

	current.SiteName = req.SiteName
	current.MultiUserEnabled = req.MultiUserEnabled
	current.DefaultSourceID = req.DefaultSourceID
	current.MaxUploadSize = req.MaxUploadSize
	current.DefaultChunkSize = req.DefaultChunkSize
	current.WebDAVEnabled = req.WebDAVEnabled
	current.WebDAVPrefix = req.WebDAVPrefix
	current.Theme = req.Theme
	current.Language = req.Language
	current.TimeZone = req.TimeZone

	if err := s.systemConfigRepo.Upsert(ctx, current); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "system_config",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			Before:       before,
		})
		return nil, err
	}

	after := systemConfigAuditView(current)
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "system_config",
		Action:       "update",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(current.ID),
		Before:       before,
		After:        after,
	})

	dto := toSystemConfigPublic(current)
	return &dto, nil
}

func defaultSystemConfigEntity(options SystemOptions) *entity.SystemConfig {
	return &entity.SystemConfig{
		ID:               1,
		SiteName:         options.SiteName,
		MultiUserEnabled: options.MultiUserEnabled,
		DefaultSourceID:  options.DefaultSourceID,
		MaxUploadSize:    options.MaxUploadSize,
		DefaultChunkSize: options.DefaultChunkSize,
		WebDAVEnabled:    options.WebDAVEnabled,
		WebDAVPrefix:     options.WebDAVPrefix,
		Theme:            options.Theme,
		Language:         options.Language,
		TimeZone:         options.TimeZone,
	}
}

func issueAndStoreTokenPair(
	ctx context.Context,
	user *entity.User,
	refreshRepo domainrepo.RefreshTokenRepository,
	tokens tokenService,
) (appdto.TokenPair, error) {
	accessToken, err := tokens.IssueAccessToken(user.ID, user.RoleKey, user.TokenVersion)
	if err != nil {
		return appdto.TokenPair{}, err
	}
	refreshToken, err := tokens.IssueRefreshToken(user.ID, user.RoleKey, user.TokenVersion)
	if err != nil {
		return appdto.TokenPair{}, err
	}

	if err := refreshRepo.Create(ctx, &entity.RefreshToken{
		UserID:    user.ID,
		TokenHash: hashToken(refreshToken),
		ExpiresAt: time.Now().Add(tokens.RefreshTokenTTL()),
	}); err != nil {
		return appdto.TokenPair{}, err
	}

	return appdto.TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresIn:        int(tokens.AccessTokenTTL().Seconds()),
		RefreshExpiresIn: int(tokens.RefreshTokenTTL().Seconds()),
		TokenType:        "Bearer",
	}, nil
}

func toUserSummary(user *entity.User) appdto.UserSummary {
	return appdto.UserSummary{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		RoleKey:   user.RoleKey,
		Status:    user.Status,
		CreatedAt: user.CreatedAt.Format(time.RFC3339),
	}
}

func toSystemConfigPublic(cfg *entity.SystemConfig) appdto.SystemConfigPublic {
	return appdto.SystemConfigPublic{
		SiteName:         cfg.SiteName,
		MultiUserEnabled: cfg.MultiUserEnabled,
		DefaultSourceID:  cfg.DefaultSourceID,
		MaxUploadSize:    cfg.MaxUploadSize,
		DefaultChunkSize: cfg.DefaultChunkSize,
		WebDAVEnabled:    cfg.WebDAVEnabled,
		WebDAVPrefix:     cfg.WebDAVPrefix,
		Theme:            cfg.Theme,
		Language:         cfg.Language,
		TimeZone:         cfg.TimeZone,
	}
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
