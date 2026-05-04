package service

import (
	"encoding/json"
	"fmt"

	appdto "yunxia/internal/application/dto"
	infraStorage "yunxia/internal/infrastructure/storage"
)

// SourceConfigCodec 负责存储源配置的持久化、展示和审计视图转换。
type SourceConfigCodec interface {
	DriverType() string
	DefaultMountSlug() string
	Build(config map[string]any, secretPatch map[string]any, existingRaw string) (string, error)
	Public(rawConfigJSON string, canReadSecret bool) (map[string]any, map[string]appdto.SecretFieldMask, error)
	AuditView(rawConfigJSON string) map[string]any
}

type localSourceConfigCodec struct{}

// NewLocalSourceConfigCodec 创建 local 存储源配置 codec。
func NewLocalSourceConfigCodec() SourceConfigCodec {
	return localSourceConfigCodec{}
}

func (localSourceConfigCodec) DriverType() string {
	return "local"
}

func (localSourceConfigCodec) DefaultMountSlug() string {
	return "source-local"
}

func (localSourceConfigCodec) Build(config map[string]any, _ map[string]any, _ string) (string, error) {
	cfg, err := parseLocalConfigMap(config)
	if err != nil {
		return "", err
	}
	if err := validateLocalBasePath(cfg.BasePath); err != nil {
		return "", err
	}
	return marshalLocalSourceConfig(cfg.BasePath)
}

func (localSourceConfigCodec) Public(rawConfigJSON string, _ bool) (map[string]any, map[string]appdto.SecretFieldMask, error) {
	config := map[string]any{}
	if err := json.Unmarshal([]byte(rawConfigJSON), &config); err != nil {
		return nil, nil, err
	}
	return config, map[string]appdto.SecretFieldMask{}, nil
}

func (localSourceConfigCodec) AuditView(rawConfigJSON string) map[string]any {
	var config localSourceConfig
	if err := json.Unmarshal([]byte(rawConfigJSON), &config); err != nil {
		return map[string]any{}
	}
	return map[string]any{"base_path": config.BasePath}
}

type s3SourceConfigCodec struct{}

// NewS3SourceConfigCodec 创建 S3 存储源配置 codec。
func NewS3SourceConfigCodec() SourceConfigCodec {
	return s3SourceConfigCodec{}
}

func (s3SourceConfigCodec) DriverType() string {
	return "s3"
}

func (s3SourceConfigCodec) DefaultMountSlug() string {
	return "source-s3"
}

func (s3SourceConfigCodec) Build(config map[string]any, secretPatch map[string]any, existingRaw string) (string, error) {
	var existingCfg *infraStorage.S3Config
	if existingRaw != "" {
		parsed, err := infraStorage.ParseS3ConfigJSON(existingRaw)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrConfigInvalid, err)
		}
		existingCfg = &parsed
	}

	cfg, err := infraStorage.BuildS3Config(config, secretPatch, existingCfg)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	}
	configJSON, err := cfg.Marshal()
	if err != nil {
		return "", err
	}
	return configJSON, nil
}

func (s3SourceConfigCodec) Public(rawConfigJSON string, canReadSecret bool) (map[string]any, map[string]appdto.SecretFieldMask, error) {
	cfg, err := infraStorage.ParseS3ConfigJSON(rawConfigJSON)
	if err != nil {
		return nil, nil, err
	}
	config := cfg.PublicMap()
	if canReadSecret {
		config["access_key"] = cfg.AccessKey
		config["secret_key"] = cfg.SecretKey
	}
	return config, buildS3SecretMasks(cfg), nil
}

func (s3SourceConfigCodec) AuditView(rawConfigJSON string) map[string]any {
	cfg, err := infraStorage.ParseS3ConfigJSON(rawConfigJSON)
	if err != nil {
		return map[string]any{}
	}
	return cfg.PublicMap()
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

type pikPakSourceConfigCodec struct{}

// NewPikPakSourceConfigCodec 创建 PikPak 存储源配置 codec。
func NewPikPakSourceConfigCodec() SourceConfigCodec {
	return pikPakSourceConfigCodec{}
}

func (pikPakSourceConfigCodec) DriverType() string {
	return infraStorage.PikPakDriverType
}

func (pikPakSourceConfigCodec) DefaultMountSlug() string {
	return "source-pikpak"
}

func (pikPakSourceConfigCodec) Build(config map[string]any, secretPatch map[string]any, existingRaw string) (string, error) {
	var existingCfg *infraStorage.PikPakConfig
	if existingRaw != "" {
		parsed, err := infraStorage.ParsePikPakConfigJSON(existingRaw)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrConfigInvalid, err)
		}
		existingCfg = &parsed
	}

	cfg, err := infraStorage.BuildPikPakConfig(config, secretPatch, existingCfg)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	}
	configJSON, err := cfg.Marshal()
	if err != nil {
		return "", err
	}
	return configJSON, nil
}

func (pikPakSourceConfigCodec) Public(rawConfigJSON string, canReadSecret bool) (map[string]any, map[string]appdto.SecretFieldMask, error) {
	cfg, err := infraStorage.ParsePikPakConfigJSON(rawConfigJSON)
	if err != nil {
		return nil, nil, err
	}
	config := cfg.PublicMap()
	if canReadSecret {
		config["username"] = cfg.Username
		config["password"] = cfg.Password
		config["refresh_token"] = cfg.RefreshToken
		config["captcha_token"] = cfg.CaptchaToken
		config["device_id"] = cfg.DeviceID
	}
	return config, buildPikPakSecretMasks(cfg), nil
}

func (pikPakSourceConfigCodec) AuditView(rawConfigJSON string) map[string]any {
	cfg, err := infraStorage.ParsePikPakConfigJSON(rawConfigJSON)
	if err != nil {
		return map[string]any{}
	}
	view := cfg.PublicMap()
	for key, mask := range cfg.SecretMasks() {
		view[key+"_configured"] = mask.Configured
	}
	return view
}

func buildPikPakSecretMasks(cfg infraStorage.PikPakConfig) map[string]appdto.SecretFieldMask {
	infraMasks := cfg.SecretMasks()
	masks := make(map[string]appdto.SecretFieldMask, len(infraMasks))
	for key, mask := range infraMasks {
		masks[key] = appdto.SecretFieldMask{
			Configured: mask.Configured,
			Masked:     mask.Masked,
		}
	}
	return masks
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
