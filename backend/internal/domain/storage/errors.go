package storage

import "errors"

var (
	// ErrConfigInvalid 表示 driver 配置不合法。
	ErrConfigInvalid = errors.New("config invalid")
	// ErrOperationUnsupported 表示当前 driver 暂不支持该操作。
	ErrOperationUnsupported = errors.New("source operation unsupported")
	// ErrCloudAuthFailed 表示第三方账号密码或授权失败。
	ErrCloudAuthFailed = errors.New("cloud auth failed")
	// ErrCloudTokenInvalid 表示 refresh/access token 无效且无法恢复。
	ErrCloudTokenInvalid = errors.New("cloud token invalid")
	// ErrCloudCaptchaRequired 表示 provider 要求人工验证。
	ErrCloudCaptchaRequired = errors.New("cloud captcha required")
	// ErrCloudCaptchaExpired 表示 captcha token 过期，可刷新后重试。
	ErrCloudCaptchaExpired = errors.New("cloud captcha expired")
	// ErrCloudRateLimited 表示 provider 限流。
	ErrCloudRateLimited = errors.New("cloud rate limited")
	// ErrCloudRegionBlocked 表示 provider 因部署区域或网络出口限制拒绝访问。
	ErrCloudRegionBlocked = errors.New("cloud region blocked")
	// ErrCloudProviderUnavailable 表示 provider 临时不可用。
	ErrCloudProviderUnavailable = errors.New("cloud provider unavailable")
)

// ProviderError 包装第三方 provider 错误，保留稳定 sentinel 与脱敏诊断信息。
type ProviderError struct {
	Kind              error
	Message           string
	ProviderCode      string
	VerificationURL   string
	RetryAfterSeconds int
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Kind != nil {
		return e.Kind.Error()
	}
	return "cloud provider error"
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Kind
}

// NewProviderError 创建一个脱敏 provider 错误。
func NewProviderError(kind error, message string) *ProviderError {
	return &ProviderError{Kind: kind, Message: message}
}
