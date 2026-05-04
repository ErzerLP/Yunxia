package storage

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	domainstorage "yunxia/internal/domain/storage"
)

const (
	DefaultPikPakUserBaseURL  = "https://user.mypikpak.net"
	DefaultPikPakDriveBaseURL = "https://api-drive.mypikpak.net"
	DefaultPikPakAboutBaseURL = "https://api-drive.mypikpak.com"
)

var (
	pikPakAndroidAlgorithms = []string{
		"SOP04dGzk0TNO7t7t9ekDbAmx+eq0OI1ovEx",
		"nVBjhYiND4hZ2NCGyV5beamIr7k6ifAsAbl",
		"Ddjpt5B/Cit6EDq2a6cXgxY9lkEIOw4yC1GDF28KrA",
		"VVCogcmSNIVvgV6U+AochorydiSymi68YVNGiz",
		"u5ujk5sM62gpJOsB/1Gu/zsfgfZO",
		"dXYIiBOAHZgzSruaQ2Nhrqc2im",
		"z5jUTBSIpBN9g4qSJGlidNAutX6",
		"KJE2oveZ34du/g1tiimm",
	}
	pikPakWebAlgorithms = []string{
		"C9qPpZLN8ucRTaTiUMWYS9cQvWOE",
		"+r6CQVxjzJV6LCV",
		"F",
		"pFJRC",
		"9WXYIDGrwTCz2OiVlgZa90qpECPD6olt",
		"/750aCr4lm/Sly/c",
		"RB+DT/gZCrbV",
		"",
		"CyLsf7hdkIRxRm215hl",
		"7xHvLi2tOYP0Y92b",
		"ZGTXXxu8E/MIWaEDB+Sm/",
		"1UI3",
		"E7fP5Pfijd+7K+t6Tg/NhuLq0eEUVChpJSkrKxpO",
		"ihtqpG6FMt65+Xk+tWUH2",
		"NhXXU9rg4XXdzo7u5o",
	}
	pikPakPCAlgorithms = []string{
		"KHBJ07an7ROXDoK7Db",
		"G6n399rSWkl7WcQmw5rpQInurc1DkLmLJqE",
		"JZD1A3M4x+jBFN62hkr7VDhkkZxb9g3rWqRZqFAAb",
		"fQnw/AmSlbbI91Ik15gpddGgyU7U",
		"/Dv9JdPYSj3sHiWjouR95NTQff",
		"yGx2zuTjbWENZqecNI+edrQgqmZKP",
		"ljrbSzdHLwbqcRn",
		"lSHAsqCkGDGxQqqwrVu",
		"TsWXI81fD1",
		"vk7hBjawK/rOSrSWajtbMk95nfgf3",
	}
)

// PikPakAPIClient 抽象 PikPak 原始 API，便于测试注入 fake client。
type PikPakAPIClient interface {
	RefreshToken(ctx context.Context, cfg PikPakConfig) (*PikPakAuthToken, error)
	Login(ctx context.Context, cfg PikPakConfig) (*PikPakAuthToken, error)
	RefreshCaptcha(ctx context.Context, cfg PikPakConfig, action string, userID string) (*PikPakCaptchaToken, error)
	ListFiles(ctx context.Context, session PikPakSession, parentID string, pageToken string) (*PikPakListFilesResponse, error)
	GetFile(ctx context.Context, session PikPakSession, fileID string, usage string) (*PikPakFile, error)
	About(ctx context.Context, session PikPakSession) (*PikPakAbout, error)
}

// PikPakSession 是一次 provider 请求所需的运行态身份。
type PikPakSession struct {
	AccessToken  string
	CaptchaToken string
	DeviceID     string
	UserID       string
	UserAgent    string
	Platform     string
}

// PikPakAuthToken 表示登录/刷新 token 响应。
type PikPakAuthToken struct {
	AccessToken  string
	RefreshToken string
	UserID       string
}

// PikPakCaptchaToken 表示 captcha 初始化响应。
type PikPakCaptchaToken struct {
	Token           string
	ExpiresIn       int
	VerificationURL string
}

// PikPakListFilesResponse 表示目录分页响应。
type PikPakListFilesResponse struct {
	Files         []PikPakFile `json:"files"`
	NextPageToken string       `json:"next_page_token"`
}

// PikPakFile 表示 PikPak 文件对象的最小字段集合。
type PikPakFile struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Kind           string        `json:"kind"`
	Size           string        `json:"size"`
	Hash           string        `json:"hash"`
	CreatedTime    string        `json:"created_time"`
	ModifiedTime   string        `json:"modified_time"`
	ThumbnailLink  string        `json:"thumbnail_link"`
	WebContentLink string        `json:"web_content_link"`
	Medias         []PikPakMedia `json:"medias"`
}

// PikPakMedia 表示视频媒体链接。
type PikPakMedia struct {
	Link PikPakMediaLink `json:"link"`
}

// PikPakMediaLink 表示媒体 URL。
type PikPakMediaLink struct {
	URL string `json:"url"`
}

// PikPakAbout 表示容量详情响应。
type PikPakAbout struct {
	Quota PikPakQuota `json:"quota"`
}

// PikPakQuota 表示 provider quota。
type PikPakQuota struct {
	Limit string `json:"limit"`
	Usage string `json:"usage"`
}

type pikPakPlatformInfo struct {
	ClientID      string
	ClientSecret  string
	ClientVersion string
	PackageName   string
	SdkVersion    string
	UserAgent     string
	Algorithms    []string
}

// PikPakHTTPClientOption 定义 HTTP client 的可选配置。
type PikPakHTTPClientOption func(*PikPakHTTPClient)

// PikPakHTTPClient 是 PikPak 原始 HTTP API 的最小实现。
type PikPakHTTPClient struct {
	httpClient   *http.Client
	userBaseURL  string
	driveBaseURL string
	aboutBaseURL string
}

// NewPikPakHTTPClient 创建 PikPak HTTP client。
func NewPikPakHTTPClient(options ...PikPakHTTPClientOption) *PikPakHTTPClient {
	client := &PikPakHTTPClient{
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
		userBaseURL:  DefaultPikPakUserBaseURL,
		driveBaseURL: DefaultPikPakDriveBaseURL,
		aboutBaseURL: DefaultPikPakAboutBaseURL,
	}
	for _, option := range options {
		option(client)
	}
	return client
}

// WithPikPakHTTPClient 注入自定义 http.Client。
func WithPikPakHTTPClient(httpClient *http.Client) PikPakHTTPClientOption {
	return func(c *PikPakHTTPClient) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithPikPakBaseURLs 覆盖 PikPak base URL，主要用于 httptest。
func WithPikPakBaseURLs(userBaseURL string, driveBaseURL string, aboutBaseURL string) PikPakHTTPClientOption {
	return func(c *PikPakHTTPClient) {
		if strings.TrimSpace(userBaseURL) != "" {
			c.userBaseURL = strings.TrimRight(strings.TrimSpace(userBaseURL), "/")
		}
		if strings.TrimSpace(driveBaseURL) != "" {
			c.driveBaseURL = strings.TrimRight(strings.TrimSpace(driveBaseURL), "/")
		}
		if strings.TrimSpace(aboutBaseURL) != "" {
			c.aboutBaseURL = strings.TrimRight(strings.TrimSpace(aboutBaseURL), "/")
		}
	}
}

// RefreshToken 使用 refresh token 换 access token。
func (c *PikPakHTTPClient) RefreshToken(ctx context.Context, cfg PikPakConfig) (*PikPakAuthToken, error) {
	platform, err := pikPakPlatform(cfg.Platform)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"client_id":     platform.ClientID,
		"client_secret": platform.ClientSecret,
		"grant_type":    "refresh_token",
		"refresh_token": cfg.RefreshToken,
	}
	var resp pikPakAuthResponse
	if err := c.doJSON(ctx, http.MethodPost, c.userBaseURL+"/v1/auth/token", platform.UserAgent, nil, map[string]string{"client_id": platform.ClientID}, payload, &resp); err != nil {
		return nil, err
	}
	return resp.toAuthToken(), nil
}

// Login 使用用户名密码登录。
func (c *PikPakHTTPClient) Login(ctx context.Context, cfg PikPakConfig) (*PikPakAuthToken, error) {
	platform, err := pikPakPlatform(cfg.Platform)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"captcha_token": cfg.CaptchaToken,
		"client_id":     platform.ClientID,
		"client_secret": platform.ClientSecret,
		"username":      cfg.Username,
		"password":      cfg.Password,
	}
	var resp pikPakAuthResponse
	if err := c.doJSON(ctx, http.MethodPost, c.userBaseURL+"/v1/auth/signin", platform.UserAgent, nil, map[string]string{"client_id": platform.ClientID}, payload, &resp); err != nil {
		return nil, err
	}
	return resp.toAuthToken(), nil
}

// RefreshCaptcha 初始化或刷新 captcha token。
func (c *PikPakHTTPClient) RefreshCaptcha(ctx context.Context, cfg PikPakConfig, action string, userID string) (*PikPakCaptchaToken, error) {
	platform, err := pikPakPlatform(cfg.Platform)
	if err != nil {
		return nil, err
	}
	meta := buildPikPakCaptchaMeta(cfg, platform, userID)
	payload := map[string]any{
		"action":        action,
		"captcha_token": cfg.CaptchaToken,
		"client_id":     platform.ClientID,
		"device_id":     cfg.DeviceID,
		"meta":          meta,
		"redirect_uri":  "xlaccsdk01://xbase.cloud/callback?state=harbor",
	}
	var resp pikPakCaptchaResponse
	if err := c.doJSON(ctx, http.MethodPost, c.userBaseURL+"/v1/shield/captcha/init", platform.UserAgent, nil, map[string]string{"client_id": platform.ClientID}, payload, &resp); err != nil {
		return nil, err
	}
	if resp.URL != "" {
		return nil, &domainstorage.ProviderError{
			Kind:            domainstorage.ErrCloudCaptchaRequired,
			Message:         "cloud captcha required",
			VerificationURL: resp.URL,
		}
	}
	return &PikPakCaptchaToken{Token: resp.CaptchaToken, ExpiresIn: resp.ExpiresIn}, nil
}

// ListFiles 分页列出目录。
func (c *PikPakHTTPClient) ListFiles(ctx context.Context, session PikPakSession, parentID string, pageToken string) (*PikPakListFilesResponse, error) {
	query := map[string]string{
		"parent_id":      parentID,
		"thumbnail_size": "SIZE_LARGE",
		"with_audit":     "true",
		"limit":          "100",
		"filters":        `{"phase":{"eq":"PHASE_TYPE_COMPLETE"},"trashed":{"eq":false}}`,
	}
	if pageToken != "" {
		query["page_token"] = pageToken
	}
	var resp PikPakListFilesResponse
	if err := c.doJSON(ctx, http.MethodGet, c.driveBaseURL+"/drive/v1/files", session.UserAgent, sessionHeaders(session), query, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetFile 获取文件详情和临时下载链接。
func (c *PikPakHTTPClient) GetFile(ctx context.Context, session PikPakSession, fileID string, usage string) (*PikPakFile, error) {
	if usage == "" {
		usage = "FETCH"
	}
	query := map[string]string{
		"_magic":         "2021",
		"usage":          usage,
		"thumbnail_size": "SIZE_LARGE",
	}
	var resp PikPakFile
	if err := c.doJSON(ctx, http.MethodGet, c.driveBaseURL+"/drive/v1/files/"+url.PathEscape(fileID), session.UserAgent, sessionHeaders(session), query, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// About 查询容量。
func (c *PikPakHTTPClient) About(ctx context.Context, session PikPakSession) (*PikPakAbout, error) {
	var resp PikPakAbout
	if err := c.doJSON(ctx, http.MethodGet, c.aboutBaseURL+"/drive/v1/about", session.UserAgent, sessionHeaders(session), nil, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *PikPakHTTPClient) doJSON(ctx context.Context, method string, rawURL string, userAgent string, headers map[string]string, query map[string]string, payload any, out any) error {
	if c.httpClient == nil {
		c.httpClient = http.DefaultClient
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	values := parsed.Query()
	for key, value := range query {
		values.Set(key, value)
	}
	parsed.RawQuery = values.Encode()

	var body io.Reader
	if payload != nil {
		data, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return marshalErr
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	for key, value := range headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", domainstorage.ErrCloudProviderUnavailable, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := mapPikPakHTTPError(resp.StatusCode, data); err != nil {
		return err
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: invalid provider response", domainstorage.ErrCloudProviderUnavailable)
	}
	return nil
}

func sessionHeaders(session PikPakSession) map[string]string {
	headers := map[string]string{
		"X-Device-ID":     session.DeviceID,
		"X-Captcha-Token": session.CaptchaToken,
	}
	if session.AccessToken != "" {
		headers["Authorization"] = "Bearer " + session.AccessToken
	}
	return headers
}

type pikPakAuthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Sub          string `json:"sub"`
}

func (r pikPakAuthResponse) toAuthToken() *PikPakAuthToken {
	return &PikPakAuthToken{
		AccessToken:  r.AccessToken,
		RefreshToken: r.RefreshToken,
		UserID:       r.Sub,
	}
}

type pikPakCaptchaResponse struct {
	CaptchaToken string `json:"captcha_token"`
	ExpiresIn    int    `json:"expires_in"`
	URL          string `json:"url"`
}

type pikPakErrorPayload struct {
	ErrorCode        any    `json:"error_code"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Details          string `json:"details"`
}

func mapPikPakHTTPError(status int, body []byte) error {
	var payload pikPakErrorPayload
	_ = json.Unmarshal(body, &payload)
	code := normalizePikPakErrorCode(payload.ErrorCode)
	if code != "" && code != "0" {
		return mapPikPakProviderCode(code, payload)
	}
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusNotFound:
		return os.ErrNotExist
	case status == http.StatusTooManyRequests:
		return domainstorage.NewProviderError(domainstorage.ErrCloudRateLimited, "cloud rate limited")
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return domainstorage.NewProviderError(domainstorage.ErrCloudTokenInvalid, "cloud token invalid")
	case status >= 500:
		return domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider unavailable")
	default:
		return domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider request failed")
	}
}

func mapPikPakProviderCode(code string, payload pikPakErrorPayload) error {
	message := sanitizedPikPakErrorMessage(payload)
	switch code {
	case "404", "not_found", "file_not_found", "resource_not_found":
		return os.ErrNotExist
	case "4126", "4122", "4121", "16":
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudTokenInvalid, Message: "cloud token invalid", ProviderCode: code}
	case "9":
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudCaptchaExpired, Message: "cloud captcha expired", ProviderCode: code}
	case "10":
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudRateLimited, Message: "cloud rate limited", ProviderCode: code}
	default:
		if strings.Contains(strings.ToLower(message), "verify") {
			return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudCaptchaRequired, Message: "cloud captcha required", ProviderCode: code}
		}
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudProviderUnavailable, Message: "cloud provider request failed", ProviderCode: code}
	}
}

func normalizePikPakErrorCode(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func sanitizedPikPakErrorMessage(payload pikPakErrorPayload) string {
	for _, value := range []string{payload.ErrorDescription, payload.Error, payload.Details} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func pikPakPlatform(platform string) (pikPakPlatformInfo, error) {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "", "web":
		return pikPakPlatformInfo{
			ClientID:      "YUMx5nI8ZU8Ap8pm",
			ClientSecret:  "dbw2OtmVEeuUvIptb1Coyg",
			ClientVersion: "2.0.0",
			PackageName:   "mypikpak.com",
			SdkVersion:    "8.0.3",
			UserAgent:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36",
			Algorithms:    pikPakWebAlgorithms,
		}, nil
	case "android":
		return pikPakPlatformInfo{
			ClientID:      "YNxT9w7GMdWvEOKa",
			ClientSecret:  "dbw2OtmVEeuUvIptb1Coyg",
			ClientVersion: "1.53.2",
			PackageName:   "com.pikcloud.pikpak",
			SdkVersion:    "2.0.6.206003",
			UserAgent:     "ANDROID-com.pikcloud.pikpak/1.53.2 protocolVersion/200 networktype/WIFI",
			Algorithms:    pikPakAndroidAlgorithms,
		}, nil
	case "pc":
		return pikPakPlatformInfo{
			ClientID:      "YvtoWO6GNHiuCl7x",
			ClientSecret:  "1NIH5R1IEe2pAxZE3hv3uA",
			ClientVersion: "undefined",
			PackageName:   "mypikpak.com",
			SdkVersion:    "8.0.3",
			UserAgent:     "MainWindow Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) PikPak/2.6.11.4955 Chrome/100.0.4896.160 Electron/18.3.15 Safari/537.36",
			Algorithms:    pikPakPCAlgorithms,
		}, nil
	default:
		return pikPakPlatformInfo{}, fmt.Errorf("%w: platform must be web, android or pc", domainstorage.ErrConfigInvalid)
	}
}

func buildPikPakCaptchaMeta(cfg PikPakConfig, platform pikPakPlatformInfo, userID string) map[string]string {
	if userID != "" {
		timestamp, sign := buildPikPakCaptchaSign(platform, cfg.DeviceID)
		return map[string]string{
			"client_version": platform.ClientVersion,
			"package_name":   platform.PackageName,
			"user_id":        userID,
			"timestamp":      timestamp,
			"captcha_sign":   sign,
		}
	}
	meta := make(map[string]string)
	switch {
	case isPikPakEmail(cfg.Username):
		meta["email"] = cfg.Username
	case len(cfg.Username) >= 11 && len(cfg.Username) <= 18:
		meta["phone_number"] = cfg.Username
	default:
		meta["username"] = cfg.Username
	}
	return meta
}

func buildPikPakCaptchaSign(platform pikPakPlatformInfo, deviceID string) (string, string) {
	timestamp := fmt.Sprint(time.Now().UnixMilli())
	value := platform.ClientID + platform.ClientVersion + platform.PackageName + deviceID + timestamp
	for _, algorithm := range platform.Algorithms {
		value = md5Hex(value + algorithm)
	}
	return timestamp, "1." + value
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func isPikPakEmail(value string) bool {
	ok, _ := regexp.MatchString(`\w+([-+.]\w+)*@\w+([-.]\w+)*\.\w+([-.]\w+)*`, value)
	return ok
}
