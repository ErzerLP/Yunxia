package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

// S3Config 表示 S3 存储源配置。
type S3Config struct {
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	Bucket         string `json:"bucket"`
	BasePrefix     string `json:"base_prefix,omitempty"`
	ForcePathStyle bool   `json:"force_path_style"`
	AccessKey      string `json:"access_key,omitempty"`
	SecretKey      string `json:"secret_key,omitempty"`
}

// ParseS3ConfigJSON 从 JSON 中解析 S3 配置。
func ParseS3ConfigJSON(raw string) (S3Config, error) {
	var cfg S3Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return S3Config{}, err
	}
	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return S3Config{}, err
	}
	return cfg, nil
}

// BuildS3Config 根据公开配置和 secret patch 组装最终 S3 配置。
func BuildS3Config(config map[string]any, secretPatch map[string]any, existing *S3Config) (S3Config, error) {
	cfg := S3Config{}
	if existing != nil {
		cfg.AccessKey = existing.AccessKey
		cfg.SecretKey = existing.SecretKey
	}

	endpoint, err := readRequiredString(config, "endpoint")
	if err != nil {
		return S3Config{}, err
	}
	region, err := readRequiredString(config, "region")
	if err != nil {
		return S3Config{}, err
	}
	bucket, err := readRequiredString(config, "bucket")
	if err != nil {
		return S3Config{}, err
	}
	basePrefix, err := readOptionalString(config, "base_prefix")
	if err != nil {
		return S3Config{}, err
	}
	forcePathStyle, err := readOptionalBool(config, "force_path_style")
	if err != nil {
		return S3Config{}, err
	}

	cfg.Endpoint = endpoint
	cfg.Region = region
	cfg.Bucket = bucket
	cfg.BasePrefix = basePrefix
	cfg.ForcePathStyle = forcePathStyle

	if err := applySecretPatch(&cfg, secretPatch); err != nil {
		return S3Config{}, err
	}
	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return S3Config{}, err
	}
	return cfg, nil
}

// Marshal 将 S3 配置序列化为 JSON。
func (c S3Config) Marshal() (string, error) {
	c.normalize()
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PublicMap 返回可暴露给前端的公开配置。
func (c S3Config) PublicMap() map[string]any {
	c.normalize()
	return map[string]any{
		"endpoint":         c.Endpoint,
		"region":           c.Region,
		"bucket":           c.Bucket,
		"base_prefix":      c.BasePrefix,
		"force_path_style": c.ForcePathStyle,
	}
}

// Validate 校验 S3 配置必填字段。
func (c S3Config) Validate() error {
	switch {
	case strings.TrimSpace(c.Endpoint) == "":
		return errors.New("endpoint is required")
	case strings.TrimSpace(c.Region) == "":
		return errors.New("region is required")
	case strings.TrimSpace(c.Bucket) == "":
		return errors.New("bucket is required")
	case strings.TrimSpace(c.AccessKey) == "":
		return errors.New("access_key is required")
	case strings.TrimSpace(c.SecretKey) == "":
		return errors.New("secret_key is required")
	default:
		return nil
	}
}

func (c *S3Config) normalize() {
	c.Endpoint = strings.TrimSpace(c.Endpoint)
	c.Region = strings.TrimSpace(c.Region)
	c.Bucket = strings.TrimSpace(c.Bucket)
	c.AccessKey = strings.TrimSpace(c.AccessKey)
	c.SecretKey = strings.TrimSpace(c.SecretKey)
	c.BasePrefix = normalizeKeyPrefix(c.BasePrefix)
}

func applySecretPatch(cfg *S3Config, secretPatch map[string]any) error {
	for _, field := range []string{"access_key", "secret_key"} {
		value, exists := secretPatch[field]
		if !exists {
			continue
		}
		switch typed := value.(type) {
		case nil:
			setSecretValue(cfg, field, "")
		case string:
			setSecretValue(cfg, field, typed)
		default:
			return fmt.Errorf("%s must be string or null", field)
		}
	}
	return nil
}

func setSecretValue(cfg *S3Config, field string, value string) {
	switch field {
	case "access_key":
		cfg.AccessKey = value
	case "secret_key":
		cfg.SecretKey = value
	}
}

func readRequiredString(data map[string]any, key string) (string, error) {
	value, err := readOptionalString(data, key)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func readOptionalString(data map[string]any, key string) (string, error) {
	if data == nil {
		return "", nil
	}
	value, exists := data[key]
	if !exists || value == nil {
		return "", nil
	}
	typed, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be string", key)
	}
	return typed, nil
}

func readOptionalBool(data map[string]any, key string) (bool, error) {
	if data == nil {
		return false, nil
	}
	value, exists := data[key]
	if !exists || value == nil {
		return false, nil
	}
	typed, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be boolean", key)
	}
	return typed, nil
}

func normalizeKeyPrefix(value string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	cleaned := path.Clean("/" + trimmed)
	return strings.TrimPrefix(cleaned, "/")
}
