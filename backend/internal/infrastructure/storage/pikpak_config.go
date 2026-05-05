package storage

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"

	domainstorage "yunxia/internal/domain/storage"
)

const (
	// PikPakDriverType 是 Yunxia 内部稳定 driver_type。
	PikPakDriverType = "pikpak"

	defaultPikPakPlatform         = "web"
	defaultPikPakCacheTTLSeconds  = 300
	defaultPikPakDownloadStrategy = "redirect"
)

// PikPakConfig 表示 PikPak 存储源配置。敏感字段持久化在同一 JSON 中，
// 对外展示必须通过 PublicMap/secret mask 过滤。
type PikPakConfig struct {
	RootFolderID     string `json:"root_folder_id"`
	Platform         string `json:"platform"`
	DisableMediaLink bool   `json:"disable_media_link"`
	CacheTTLSeconds  int    `json:"cache_ttl_seconds"`
	DownloadStrategy string `json:"download_strategy"`
	ProxyURL         string `json:"proxy_url,omitempty"`

	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	CaptchaToken string `json:"captcha_token,omitempty"`
	DeviceID     string `json:"device_id,omitempty"`

	// captchaUserAgentUserID 仅用于兼容 Android 首次登录后 captcha 的 UA 时序，不持久化。
	captchaUserAgentUserID *string
}

// SecretMask 描述敏感字段的脱敏状态。
type SecretMask struct {
	Configured bool
	Masked     string
}

// ParsePikPakConfigJSON 从 JSON 中解析并校验 PikPak 配置。
func ParsePikPakConfigJSON(raw string) (PikPakConfig, error) {
	var cfg PikPakConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return PikPakConfig{}, fmt.Errorf("%w: %v", domainstorage.ErrConfigInvalid, err)
	}
	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return PikPakConfig{}, err
	}
	return cfg, nil
}

// BuildPikPakConfig 根据公开配置和 secret patch 组装最终 PikPak 配置。
func BuildPikPakConfig(config map[string]any, secretPatch map[string]any, existing *PikPakConfig) (PikPakConfig, error) {
	cfg := PikPakConfig{
		Platform:         defaultPikPakPlatform,
		DisableMediaLink: true,
		CacheTTLSeconds:  defaultPikPakCacheTTLSeconds,
		DownloadStrategy: defaultPikPakDownloadStrategy,
	}
	if existing != nil {
		cfg.Username = existing.Username
		cfg.Password = existing.Password
		cfg.RefreshToken = existing.RefreshToken
		cfg.CaptchaToken = existing.CaptchaToken
		cfg.DeviceID = existing.DeviceID
		cfg.ProxyURL = existing.ProxyURL
	}

	var err error
	if cfg.RootFolderID, err = readOptionalString(config, "root_folder_id"); err != nil {
		return PikPakConfig{}, wrapPikPakConfigError(err)
	}
	if cfg.Platform, err = readOptionalStringDefault(config, "platform", cfg.Platform); err != nil {
		return PikPakConfig{}, wrapPikPakConfigError(err)
	}
	if cfg.DisableMediaLink, err = readOptionalBoolDefault(config, "disable_media_link", cfg.DisableMediaLink); err != nil {
		return PikPakConfig{}, wrapPikPakConfigError(err)
	}
	if cfg.CacheTTLSeconds, err = readOptionalIntDefault(config, "cache_ttl_seconds", cfg.CacheTTLSeconds); err != nil {
		return PikPakConfig{}, wrapPikPakConfigError(err)
	}
	if cfg.DownloadStrategy, err = readOptionalStringDefault(config, "download_strategy", cfg.DownloadStrategy); err != nil {
		return PikPakConfig{}, wrapPikPakConfigError(err)
	}
	if config != nil {
		if _, exists := config["proxy_url"]; exists {
			if cfg.ProxyURL, err = readOptionalString(config, "proxy_url"); err != nil {
				return PikPakConfig{}, wrapPikPakConfigError(err)
			}
		}
	}
	if err := applyPikPakSecretPatch(&cfg, secretPatch); err != nil {
		return PikPakConfig{}, wrapPikPakConfigError(err)
	}
	cfg.normalize()
	if cfg.DeviceID == "" {
		cfg.DeviceID = GeneratePikPakDeviceID(cfg.Username, cfg.Password)
	}
	if err := cfg.Validate(); err != nil {
		return PikPakConfig{}, err
	}
	return cfg, nil
}

// Marshal 将 PikPak 配置序列化为 JSON。
func (c PikPakConfig) Marshal() (string, error) {
	c.normalize()
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PublicMap 返回可暴露给前端的非敏感配置。
func (c PikPakConfig) PublicMap() map[string]any {
	c.normalize()
	return map[string]any{
		"root_folder_id":     c.RootFolderID,
		"platform":           c.Platform,
		"disable_media_link": c.DisableMediaLink,
		"cache_ttl_seconds":  c.CacheTTLSeconds,
		"download_strategy":  c.DownloadStrategy,
		"proxy_url":          c.ProxyURL,
	}
}

// SecretMasks 返回敏感字段脱敏状态。
func (c PikPakConfig) SecretMasks() map[string]SecretMask {
	c.normalize()
	return map[string]SecretMask{
		"username": {
			Configured: c.Username != "",
			Masked:     maskPikPakUsername(c.Username),
		},
		"password": {
			Configured: c.Password != "",
			Masked:     maskFixedSecret(c.Password),
		},
		"refresh_token": {
			Configured: c.RefreshToken != "",
			Masked:     maskFixedSecret(c.RefreshToken),
		},
		"captcha_token": {
			Configured: c.CaptchaToken != "",
			Masked:     maskFixedSecret(c.CaptchaToken),
		},
		"device_id": {
			Configured: c.DeviceID != "",
			Masked:     maskFixedSecret(c.DeviceID),
		},
	}
}

// Validate 校验 PikPak 配置。
func (c PikPakConfig) Validate() error {
	switch c.Platform {
	case "web", "android", "pc":
	default:
		return fmt.Errorf("%w: platform must be web, android or pc", domainstorage.ErrConfigInvalid)
	}
	if c.CacheTTLSeconds <= 0 || c.CacheTTLSeconds > 86400 {
		return fmt.Errorf("%w: cache_ttl_seconds must be between 1 and 86400", domainstorage.ErrConfigInvalid)
	}
	if c.DownloadStrategy != defaultPikPakDownloadStrategy {
		return fmt.Errorf("%w: download_strategy must be redirect", domainstorage.ErrConfigInvalid)
	}
	if err := validatePikPakProxyURL(c.ProxyURL); err != nil {
		return err
	}
	if strings.TrimSpace(c.RefreshToken) == "" && (strings.TrimSpace(c.Username) == "" || strings.TrimSpace(c.Password) == "") {
		return fmt.Errorf("%w: username/password or refresh_token is required", domainstorage.ErrConfigInvalid)
	}
	return nil
}

func (c *PikPakConfig) normalize() {
	c.RootFolderID = strings.TrimSpace(c.RootFolderID)
	c.Platform = strings.ToLower(strings.TrimSpace(c.Platform))
	if c.Platform == "" {
		c.Platform = defaultPikPakPlatform
	}
	c.DownloadStrategy = strings.ToLower(strings.TrimSpace(c.DownloadStrategy))
	if c.DownloadStrategy == "" {
		c.DownloadStrategy = defaultPikPakDownloadStrategy
	}
	if c.CacheTTLSeconds <= 0 {
		c.CacheTTLSeconds = defaultPikPakCacheTTLSeconds
	}
	c.Username = strings.TrimSpace(c.Username)
	c.RefreshToken = strings.TrimSpace(c.RefreshToken)
	c.CaptchaToken = strings.TrimSpace(c.CaptchaToken)
	c.DeviceID = strings.TrimSpace(c.DeviceID)
	c.ProxyURL = strings.TrimSpace(c.ProxyURL)
}

func (c PikPakConfig) providerRootID() string {
	rootID := strings.TrimSpace(c.RootFolderID)
	if rootID == "" {
		return "root"
	}
	return rootID
}

func (c PikPakConfig) captchaUserAgentID(userID string) string {
	if c.captchaUserAgentUserID != nil {
		return *c.captchaUserAgentUserID
	}
	return userID
}

func withPikPakCaptchaUserAgentUserID(cfg PikPakConfig, userID string) PikPakConfig {
	cfg.captchaUserAgentUserID = &userID
	return cfg
}

func validatePikPakProxyURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: proxy_url must be a valid URL", domainstorage.ErrConfigInvalid)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("%w: proxy_url scheme must be http or https", domainstorage.ErrConfigInvalid)
	}
	if parsed.User != nil {
		return fmt.Errorf("%w: proxy_url must not include credentials", domainstorage.ErrConfigInvalid)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: proxy_url must not include query or fragment", domainstorage.ErrConfigInvalid)
	}
	return nil
}

func applyPikPakSecretPatch(cfg *PikPakConfig, secretPatch map[string]any) error {
	for _, field := range []string{"username", "password", "refresh_token", "captcha_token", "device_id"} {
		value, exists := secretPatch[field]
		if !exists {
			continue
		}
		switch typed := value.(type) {
		case nil:
			setPikPakSecretValue(cfg, field, "")
		case string:
			setPikPakSecretValue(cfg, field, typed)
		default:
			return fmt.Errorf("%s must be string or null", field)
		}
	}
	return nil
}

func setPikPakSecretValue(cfg *PikPakConfig, field string, value string) {
	switch field {
	case "username":
		cfg.Username = value
	case "password":
		cfg.Password = value
	case "refresh_token":
		cfg.RefreshToken = value
	case "captcha_token":
		cfg.CaptchaToken = value
	case "device_id":
		cfg.DeviceID = value
	}
}

func readOptionalStringDefault(data map[string]any, key string, fallback string) (string, error) {
	value, err := readOptionalString(data, key)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	return value, nil
}

func readOptionalBoolDefault(data map[string]any, key string, fallback bool) (bool, error) {
	if data == nil {
		return fallback, nil
	}
	value, exists := data[key]
	if !exists || value == nil {
		return fallback, nil
	}
	typed, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be boolean", key)
	}
	return typed, nil
}

func readOptionalIntDefault(data map[string]any, key string, fallback int) (int, error) {
	if data == nil {
		return fallback, nil
	}
	value, exists := data[key]
	if !exists || value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		if typed != math.Trunc(typed) {
			return 0, fmt.Errorf("%s must be integer", key)
		}
		return int(typed), nil
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, fmt.Errorf("%s must be integer", key)
		}
		return int(parsed), nil
	default:
		return 0, fmt.Errorf("%s must be integer", key)
	}
}

func wrapPikPakConfigError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", domainstorage.ErrConfigInvalid, err)
}

// GeneratePikPakDeviceID 生成稳定 device_id，兼容 OpenList 的最小策略。
func GeneratePikPakDeviceID(username string, password string) string {
	sum := md5.Sum([]byte(strings.TrimSpace(username) + password))
	return hex.EncodeToString(sum[:])
}

func maskPikPakUsername(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return string(runes[:1]) + "****"
	}
	return string(runes[:4]) + "****"
}

func maskFixedSecret(value string) string {
	if value == "" {
		return ""
	}
	return "******"
}
