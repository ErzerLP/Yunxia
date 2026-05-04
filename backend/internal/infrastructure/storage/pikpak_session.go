package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"yunxia/internal/domain/entity"
	domainstorage "yunxia/internal/domain/storage"
)

const pikPakDriveListCaptchaAction = "GET:/drive/v1/files"

type pikPakSessionState struct {
	AccessToken  string
	RefreshToken string
	CaptchaToken string
	DeviceID     string
	UserID       string
}

// PikPakSessionManager 维护 source 级别的 PikPak 运行态 token/captcha。
type PikPakSessionManager struct {
	client PikPakAPIClient
	mu     sync.Mutex
	states map[string]*pikPakSessionState
}

// NewPikPakSessionManager 创建 SessionManager。
func NewPikPakSessionManager(client PikPakAPIClient) *PikPakSessionManager {
	if client == nil {
		client = NewPikPakHTTPClient()
	}
	return &PikPakSessionManager{
		client: client,
		states: make(map[string]*pikPakSessionState),
	}
}

func (m *PikPakSessionManager) withSession(ctx context.Context, source *entity.StorageSource, fn func(PikPakSession, PikPakConfig) error) error {
	session, cfg, err := m.ensureSession(ctx, source)
	if err != nil {
		return err
	}
	if err := fn(session, cfg); err != nil {
		switch {
		case errors.Is(err, domainstorage.ErrCloudTokenInvalid):
			session, cfg, err = m.refreshAccess(ctx, source)
			if err != nil {
				return err
			}
			return fn(session, cfg)
		case errors.Is(err, domainstorage.ErrCloudCaptchaExpired):
			session, cfg, err = m.refreshDriveCaptcha(ctx, source)
			if err != nil {
				return err
			}
			return fn(session, cfg)
		default:
			return err
		}
	}
	return nil
}

func (m *PikPakSessionManager) ensureSession(ctx context.Context, source *entity.StorageSource) (PikPakSession, PikPakConfig, error) {
	cfg, err := parsePikPakSourceConfig(source)
	if err != nil {
		return PikPakSession{}, PikPakConfig{}, err
	}
	key := pikPakSessionKey(source)

	m.mu.Lock()
	state := m.states[key]
	if state == nil {
		state = stateFromPikPakConfig(cfg)
		m.states[key] = state
	}
	if state.AccessToken != "" {
		session := pikPakSessionFromState(cfg, state)
		m.mu.Unlock()
		m.writeBackRuntimeConfig(source, cfg, state)
		return session, cfg, nil
	}
	m.mu.Unlock()

	if err := m.authenticate(ctx, source, cfg, state); err != nil {
		return PikPakSession{}, PikPakConfig{}, err
	}

	m.mu.Lock()
	session := pikPakSessionFromState(cfg, state)
	m.mu.Unlock()
	return session, cfg, nil
}

func (m *PikPakSessionManager) refreshAccess(ctx context.Context, source *entity.StorageSource) (PikPakSession, PikPakConfig, error) {
	cfg, err := parsePikPakSourceConfig(source)
	if err != nil {
		return PikPakSession{}, PikPakConfig{}, err
	}
	key := pikPakSessionKey(source)
	m.mu.Lock()
	state := m.states[key]
	if state == nil {
		state = stateFromPikPakConfig(cfg)
		m.states[key] = state
	}
	state.AccessToken = ""
	m.mu.Unlock()

	if err := m.authenticate(ctx, source, cfg, state); err != nil {
		return PikPakSession{}, PikPakConfig{}, err
	}
	m.mu.Lock()
	session := pikPakSessionFromState(cfg, state)
	m.mu.Unlock()
	return session, cfg, nil
}

func (m *PikPakSessionManager) refreshDriveCaptcha(ctx context.Context, source *entity.StorageSource) (PikPakSession, PikPakConfig, error) {
	cfg, err := parsePikPakSourceConfig(source)
	if err != nil {
		return PikPakSession{}, PikPakConfig{}, err
	}
	key := pikPakSessionKey(source)
	m.mu.Lock()
	state := m.states[key]
	if state == nil {
		state = stateFromPikPakConfig(cfg)
		m.states[key] = state
	}
	cfg.RefreshToken = state.RefreshToken
	cfg.CaptchaToken = state.CaptchaToken
	cfg.DeviceID = state.DeviceID
	userID := state.UserID
	m.mu.Unlock()

	if userID == "" {
		return m.refreshAccess(ctx, source)
	}
	captcha, err := m.client.RefreshCaptcha(ctx, cfg, pikPakDriveListCaptchaAction, userID)
	if err != nil {
		return PikPakSession{}, PikPakConfig{}, err
	}
	m.mu.Lock()
	state.CaptchaToken = captcha.Token
	session := pikPakSessionFromState(cfg, state)
	m.mu.Unlock()
	m.writeBackRuntimeConfig(source, cfg, state)
	return session, cfg, nil
}

func (m *PikPakSessionManager) authenticate(ctx context.Context, source *entity.StorageSource, cfg PikPakConfig, state *pikPakSessionState) error {
	if state == nil {
		return domainstorage.ErrCloudTokenInvalid
	}
	cfg.RefreshToken = state.RefreshToken
	cfg.CaptchaToken = state.CaptchaToken
	cfg.DeviceID = state.DeviceID
	if cfg.DeviceID == "" && cfg.Username != "" && cfg.Password != "" {
		cfg.DeviceID = GeneratePikPakDeviceID(cfg.Username, cfg.Password)
		state.DeviceID = cfg.DeviceID
	}

	if cfg.RefreshToken != "" {
		token, err := m.client.RefreshToken(ctx, cfg)
		if err == nil {
			m.applyAuthToken(state, token)
			if err := m.refreshDriveCaptchaAfterAuth(ctx, cfg, state); err != nil {
				return err
			}
			m.writeBackRuntimeConfig(source, cfg, state)
			return nil
		}
		if !errors.Is(err, domainstorage.ErrCloudTokenInvalid) && !errors.Is(err, domainstorage.ErrCloudAuthFailed) {
			return err
		}
	}

	if cfg.Username == "" || cfg.Password == "" {
		return domainstorage.NewProviderError(domainstorage.ErrCloudTokenInvalid, "cloud token invalid")
	}
	if cfg.CaptchaToken == "" {
		captcha, err := m.client.RefreshCaptcha(ctx, cfg, "POST:/v1/auth/signin", "")
		if err != nil {
			return err
		}
		state.CaptchaToken = captcha.Token
		cfg.CaptchaToken = captcha.Token
	}
	token, err := m.client.Login(ctx, cfg)
	if err != nil {
		if errors.Is(err, domainstorage.ErrCloudTokenInvalid) {
			return domainstorage.NewProviderError(domainstorage.ErrCloudAuthFailed, "cloud auth failed")
		}
		return err
	}
	m.applyAuthToken(state, token)
	if err := m.refreshDriveCaptchaAfterAuth(ctx, cfg, state); err != nil {
		return err
	}
	m.writeBackRuntimeConfig(source, cfg, state)
	return nil
}

func (m *PikPakSessionManager) refreshDriveCaptchaAfterAuth(ctx context.Context, cfg PikPakConfig, state *pikPakSessionState) error {
	if state.UserID == "" {
		return nil
	}
	cfg.RefreshToken = state.RefreshToken
	cfg.CaptchaToken = state.CaptchaToken
	cfg.DeviceID = state.DeviceID
	captcha, err := m.client.RefreshCaptcha(ctx, cfg, pikPakDriveListCaptchaAction, state.UserID)
	if err != nil {
		return err
	}
	state.CaptchaToken = captcha.Token
	return nil
}

func (m *PikPakSessionManager) applyAuthToken(state *pikPakSessionState, token *PikPakAuthToken) {
	if token == nil {
		return
	}
	state.AccessToken = token.AccessToken
	if token.RefreshToken != "" {
		state.RefreshToken = token.RefreshToken
	}
	if token.UserID != "" {
		state.UserID = token.UserID
	}
}

func (m *PikPakSessionManager) writeBackRuntimeConfig(source *entity.StorageSource, cfg PikPakConfig, state *pikPakSessionState) {
	if source == nil || state == nil {
		return
	}
	cfg.RefreshToken = state.RefreshToken
	cfg.CaptchaToken = state.CaptchaToken
	cfg.DeviceID = state.DeviceID
	raw, err := cfg.Marshal()
	if err != nil {
		return
	}
	// TODO: 后续阶段通过注入最小 repository 接口持久化 refresh_token/captcha/device_id。
	// 阶段 B 先更新当前 source 实例，create/update 会随实体保存，运行态请求使用内存 session。
	source.ConfigJSON = raw
}

func parsePikPakSourceConfig(source *entity.StorageSource) (PikPakConfig, error) {
	if source == nil {
		return PikPakConfig{}, fmt.Errorf("%w: source is required", domainstorage.ErrConfigInvalid)
	}
	if source.RootPath != "" && source.RootPath != "/" {
		return PikPakConfig{}, fmt.Errorf("%w: pikpak root_path must be /", domainstorage.ErrConfigInvalid)
	}
	return ParsePikPakConfigJSON(source.ConfigJSON)
}

func stateFromPikPakConfig(cfg PikPakConfig) *pikPakSessionState {
	return &pikPakSessionState{
		RefreshToken: cfg.RefreshToken,
		CaptchaToken: cfg.CaptchaToken,
		DeviceID:     cfg.DeviceID,
	}
}

func pikPakSessionFromState(cfg PikPakConfig, state *pikPakSessionState) PikPakSession {
	platform, _ := pikPakPlatform(cfg.Platform)
	return PikPakSession{
		AccessToken:  state.AccessToken,
		CaptchaToken: state.CaptchaToken,
		DeviceID:     state.DeviceID,
		UserID:       state.UserID,
		UserAgent:    platform.UserAgent,
		Platform:     cfg.Platform,
	}
}

func pikPakSessionKey(source *entity.StorageSource) string {
	if source != nil && source.ID != 0 {
		return fmt.Sprintf("source:%d", source.ID)
	}
	raw := ""
	if source != nil {
		raw = source.DriverType + ":" + source.Name + ":" + source.ConfigJSON
	}
	sum := sha256.Sum256([]byte(raw))
	return "ephemeral:" + hex.EncodeToString(sum[:])
}
