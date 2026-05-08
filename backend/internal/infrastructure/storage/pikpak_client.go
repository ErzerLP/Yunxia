package storage

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
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
	RefreshCaptcha(ctx context.Context, cfg PikPakConfig, action string, userID string, accessToken string) (*PikPakCaptchaToken, error)
	ListFiles(ctx context.Context, session PikPakSession, parentID string, pageToken string) (*PikPakListFilesResponse, error)
	GetFile(ctx context.Context, session PikPakSession, fileID string, usage string) (*PikPakFile, error)
	CreateFolder(ctx context.Context, session PikPakSession, parentID string, name string) (*PikPakFile, error)
	CreateUploadFile(ctx context.Context, session PikPakSession, req PikPakCreateUploadFileRequest) (*PikPakCreateUploadFileResponse, error)
	CreateOfflineDownload(ctx context.Context, session PikPakSession, req PikPakCreateOfflineDownloadRequest) (*PikPakOfflineTask, error)
	GetOfflineDownloadTask(ctx context.Context, session PikPakSession, taskID string) (*PikPakOfflineTask, error)
	DeleteOfflineDownloadTasks(ctx context.Context, session PikPakSession, taskIDs []string, deleteFiles bool) error
	RenameFile(ctx context.Context, session PikPakSession, fileID string, name string) (*PikPakFile, error)
	BatchMove(ctx context.Context, session PikPakSession, ids []string, targetParentID string) error
	BatchCopy(ctx context.Context, session PikPakSession, ids []string, targetParentID string) error
	BatchTrash(ctx context.Context, session PikPakSession, ids []string) error
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
	ProxyURL     string
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

// PikPakCreateUploadFileRequest 表示创建 resumable 上传任务所需参数。
type PikPakCreateUploadFileRequest struct {
	ParentID string
	Name     string
	Size     int64
	Hash     string
}

// PikPakCreateUploadFileResponse 表示 PikPak 创建上传任务响应。
type PikPakCreateUploadFileResponse struct {
	UploadType string                 `json:"upload_type"`
	Resumable  *PikPakResumableUpload `json:"resumable"`
	File       *PikPakFile            `json:"file"`
}

// PikPakCreateOfflineDownloadRequest 表示 PikPak 原生离线下载创建参数。
type PikPakCreateOfflineDownloadRequest struct {
	ParentID string
	URL      string
	Name     string
}

// PikPakOfflineTask 表示 PikPak 离线下载任务。
type PikPakOfflineTask struct {
	ID                string      `json:"id"`
	Name              string      `json:"name"`
	Phase             string      `json:"phase"`
	Progress          float64     `json:"progress"`
	Message           string      `json:"message"`
	File              *PikPakFile `json:"file"`
	ReferenceResource *PikPakFile `json:"reference_resource"`
}

// PikPakResumableUpload 表示需要继续上传实体的 OSS 参数集合。
type PikPakResumableUpload struct {
	Kind     string                `json:"kind"`
	Provider string                `json:"provider"`
	Params   PikPakOSSUploadParams `json:"params"`
}

// PikPakOSSUploadParams 是 PikPak 返回的临时 OSS 上传凭证。
type PikPakOSSUploadParams struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	Bucket          string `json:"bucket"`
	Endpoint        string `json:"endpoint"`
	Expiration      string `json:"expiration"`
	Key             string `json:"key"`
	SecurityToken   string `json:"security_token"`
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
	httpClient       *http.Client
	httpClientCustom bool
	userBaseURL      string
	driveBaseURL     string
	aboutBaseURL     string
	defaultProxyURL  string
	proxyURLFromEnv  bool
	maxAttempts      int
	retryBaseDelay   time.Duration
	sleepBeforeRetry func(context.Context, time.Duration) error
}

// NewPikPakHTTPClient 创建 PikPak HTTP client。
func NewPikPakHTTPClient(options ...PikPakHTTPClientOption) *PikPakHTTPClient {
	client := &PikPakHTTPClient{
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
		userBaseURL:     DefaultPikPakUserBaseURL,
		driveBaseURL:    DefaultPikPakDriveBaseURL,
		aboutBaseURL:    DefaultPikPakAboutBaseURL,
		defaultProxyURL: strings.TrimSpace(os.Getenv("YUNXIA_PIKPAK_PROXY_URL")),
		proxyURLFromEnv: strings.TrimSpace(os.Getenv("YUNXIA_PIKPAK_PROXY_URL")) != "",
		maxAttempts:     3,
		retryBaseDelay:  500 * time.Millisecond,
		sleepBeforeRetry: func(ctx context.Context, delay time.Duration) error {
			if delay <= 0 {
				return nil
			}
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
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
			c.httpClientCustom = true
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

// WithPikPakRetryPolicy 覆盖 provider 临时错误重试策略。
func WithPikPakRetryPolicy(maxAttempts int, baseDelay time.Duration) PikPakHTTPClientOption {
	return func(c *PikPakHTTPClient) {
		if maxAttempts > 0 {
			c.maxAttempts = maxAttempts
		}
		if baseDelay >= 0 {
			c.retryBaseDelay = baseDelay
		}
	}
}

// WithPikPakRetrySleeper 注入重试等待函数，测试可用它避免真实 sleep。
func WithPikPakRetrySleeper(sleeper func(context.Context, time.Duration) error) PikPakHTTPClientOption {
	return func(c *PikPakHTTPClient) {
		if sleeper != nil {
			c.sleepBeforeRetry = sleeper
		}
	}
}

// WithPikPakProxyURL 设置 PikPak 专用默认代理，优先于标准 HTTP_PROXY 环境变量。
func WithPikPakProxyURL(proxyURL string) PikPakHTTPClientOption {
	return func(c *PikPakHTTPClient) {
		c.defaultProxyURL = strings.TrimSpace(proxyURL)
		c.proxyURLFromEnv = false
	}
}

// RefreshToken 使用 refresh token 换 access token。
func (c *PikPakHTTPClient) RefreshToken(ctx context.Context, cfg PikPakConfig) (*PikPakAuthToken, error) {
	platform, err := pikPakPlatform(cfg.Platform)
	if err != nil {
		return nil, err
	}
	userAgent := buildPikPakUserAgent(cfg, platform, "")
	payload := map[string]any{
		"client_id":     platform.ClientID,
		"client_secret": platform.ClientSecret,
		"grant_type":    "refresh_token",
		"refresh_token": cfg.RefreshToken,
	}
	var resp pikPakAuthResponse
	if err := c.doJSONWithProxy(ctx, http.MethodPost, c.userBaseURL+"/v1/auth/token", cfg.ProxyURL, userAgent, nil, map[string]string{"client_id": platform.ClientID}, payload, &resp); err != nil {
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
	userAgent := buildPikPakUserAgent(cfg, platform, "")
	payload := map[string]any{
		"captcha_token": cfg.CaptchaToken,
		"client_id":     platform.ClientID,
		"client_secret": platform.ClientSecret,
		"username":      cfg.Username,
		"password":      cfg.Password,
	}
	var resp pikPakAuthResponse
	if err := c.doJSONWithProxy(ctx, http.MethodPost, c.userBaseURL+"/v1/auth/signin", cfg.ProxyURL, userAgent, nil, map[string]string{"client_id": platform.ClientID}, payload, &resp); err != nil {
		return nil, err
	}
	return resp.toAuthToken(), nil
}

// RefreshCaptcha 初始化或刷新 captcha token。
func (c *PikPakHTTPClient) RefreshCaptcha(ctx context.Context, cfg PikPakConfig, action string, userID string, accessToken string) (*PikPakCaptchaToken, error) {
	platform, err := pikPakPlatform(cfg.Platform)
	if err != nil {
		return nil, err
	}
	userAgent := buildPikPakUserAgent(cfg, platform, cfg.captchaUserAgentID(userID))
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
	if err := c.doJSONWithProxy(ctx, http.MethodPost, c.userBaseURL+"/v1/shield/captcha/init", cfg.ProxyURL, userAgent, pikPakCaptchaHeaders(cfg, accessToken), map[string]string{"client_id": platform.ClientID}, payload, &resp); err != nil {
		return nil, err
	}
	if resp.URL != "" {
		return nil, &domainstorage.ProviderError{
			Kind:            domainstorage.ErrCloudCaptchaRequired,
			Message:         "cloud captcha required",
			ProviderCode:    "captcha_required",
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
		"page_token":     pageToken,
	}
	var resp PikPakListFilesResponse
	if err := c.doJSONWithProxy(ctx, http.MethodGet, c.driveBaseURL+"/drive/v1/files", session.ProxyURL, session.UserAgent, sessionHeaders(session), query, nil, &resp); err != nil {
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
	if err := c.doJSONWithProxy(ctx, http.MethodGet, c.driveBaseURL+"/drive/v1/files/"+url.PathEscape(fileID), session.ProxyURL, session.UserAgent, sessionHeaders(session), query, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateFolder 创建 PikPak 目录。
func (c *PikPakHTTPClient) CreateFolder(ctx context.Context, session PikPakSession, parentID string, name string) (*PikPakFile, error) {
	payload := map[string]any{
		"kind":      "drive#folder",
		"parent_id": parentID,
		"name":      name,
	}
	var resp PikPakFile
	if err := c.doJSONWithProxy(ctx, http.MethodPost, c.driveBaseURL+"/drive/v1/files", session.ProxyURL, session.UserAgent, sessionHeaders(session), nil, payload, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateUploadFile 创建 PikPak resumable 上传任务。
func (c *PikPakHTTPClient) CreateUploadFile(ctx context.Context, session PikPakSession, req PikPakCreateUploadFileRequest) (*PikPakCreateUploadFileResponse, error) {
	payload := map[string]any{
		"kind":        "drive#file",
		"name":        req.Name,
		"size":        req.Size,
		"hash":        req.Hash,
		"upload_type": "UPLOAD_TYPE_RESUMABLE",
		"objProvider": map[string]any{
			"provider": "UPLOAD_TYPE_UNKNOWN",
		},
		"parent_id":   req.ParentID,
		"folder_type": "NORMAL",
	}
	var resp PikPakCreateUploadFileResponse
	if err := c.doJSONWithProxy(ctx, http.MethodPost, c.driveBaseURL+"/drive/v1/files", session.ProxyURL, session.UserAgent, sessionHeaders(session), nil, payload, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateOfflineDownload 创建 PikPak provider 原生离线下载任务。
func (c *PikPakHTTPClient) CreateOfflineDownload(ctx context.Context, session PikPakSession, req PikPakCreateOfflineDownloadRequest) (*PikPakOfflineTask, error) {
	payload := map[string]any{
		"kind":        "drive#file",
		"name":        req.Name,
		"upload_type": "UPLOAD_TYPE_URL",
		"url": map[string]any{
			"url": req.URL,
		},
		"parent_id":   req.ParentID,
		"folder_type": "",
	}
	var resp struct {
		Task *PikPakOfflineTask `json:"task"`
	}
	if err := c.doJSONWithProxy(ctx, http.MethodPost, c.driveBaseURL+"/drive/v1/files", session.ProxyURL, session.UserAgent, sessionHeaders(session), nil, payload, &resp); err != nil {
		return nil, err
	}
	if resp.Task == nil {
		return nil, domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider task missing")
	}
	return resp.Task, nil
}

// GetOfflineDownloadTask 查询单个 PikPak provider 原生离线下载任务。
func (c *PikPakHTTPClient) GetOfflineDownloadTask(ctx context.Context, session PikPakSession, taskID string) (*PikPakOfflineTask, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, os.ErrNotExist
	}
	pageToken := ""
	for {
		query := map[string]string{
			"type":           "offline",
			"thumbnail_size": "SIZE_SMALL",
			"limit":          "10000",
			"with":           "reference_resource",
			"filters":        `{"phase":{"in":"PHASE_TYPE_RUNNING,PHASE_TYPE_ERROR,PHASE_TYPE_PENDING,PHASE_TYPE_COMPLETE"}}`,
		}
		if pageToken != "" {
			query["page_token"] = pageToken
		}
		var resp struct {
			Tasks         []PikPakOfflineTask `json:"tasks"`
			NextPageToken string              `json:"next_page_token"`
		}
		if err := c.doJSONWithProxy(ctx, http.MethodGet, c.driveBaseURL+"/drive/v1/tasks", session.ProxyURL, session.UserAgent, sessionHeaders(session), query, nil, &resp); err != nil {
			return nil, err
		}
		for _, task := range resp.Tasks {
			if task.ID == taskID {
				found := task
				return &found, nil
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return nil, os.ErrNotExist
}

// DeleteOfflineDownloadTasks 删除 PikPak provider 原生离线下载任务记录。
func (c *PikPakHTTPClient) DeleteOfflineDownloadTasks(ctx context.Context, session PikPakSession, taskIDs []string, deleteFiles bool) error {
	cleanIDs := make([]string, 0, len(taskIDs))
	for _, id := range taskIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			cleanIDs = append(cleanIDs, id)
		}
	}
	if len(cleanIDs) == 0 {
		return nil
	}
	query := map[string]string{
		"task_ids":     strings.Join(cleanIDs, ","),
		"delete_files": strconv.FormatBool(deleteFiles),
	}
	return c.doJSONWithProxy(ctx, http.MethodDelete, c.driveBaseURL+"/drive/v1/tasks", session.ProxyURL, session.UserAgent, sessionHeaders(session), query, nil, nil)
}

// RenameFile 修改 PikPak 对象名称。
func (c *PikPakHTTPClient) RenameFile(ctx context.Context, session PikPakSession, fileID string, name string) (*PikPakFile, error) {
	payload := map[string]any{
		"name": name,
	}
	var resp PikPakFile
	if err := c.doJSONWithProxy(ctx, http.MethodPatch, c.driveBaseURL+"/drive/v1/files/"+url.PathEscape(fileID), session.ProxyURL, session.UserAgent, sessionHeaders(session), nil, payload, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BatchMove 批量移动 PikPak 对象到目标目录。
func (c *PikPakHTTPClient) BatchMove(ctx context.Context, session PikPakSession, ids []string, targetParentID string) error {
	payload := map[string]any{
		"ids": ids,
		"to": map[string]any{
			"parent_id": targetParentID,
		},
	}
	return c.doJSONWithProxy(ctx, http.MethodPost, c.driveBaseURL+"/drive/v1/files:batchMove", session.ProxyURL, session.UserAgent, sessionHeaders(session), nil, payload, nil)
}

// BatchCopy 批量复制 PikPak 对象到目标目录。
func (c *PikPakHTTPClient) BatchCopy(ctx context.Context, session PikPakSession, ids []string, targetParentID string) error {
	payload := map[string]any{
		"ids": ids,
		"to": map[string]any{
			"parent_id": targetParentID,
		},
	}
	return c.doJSONWithProxy(ctx, http.MethodPost, c.driveBaseURL+"/drive/v1/files:batchCopy", session.ProxyURL, session.UserAgent, sessionHeaders(session), nil, payload, nil)
}

// BatchTrash 将 PikPak 对象移入 provider 回收站。
func (c *PikPakHTTPClient) BatchTrash(ctx context.Context, session PikPakSession, ids []string) error {
	payload := map[string]any{
		"ids": ids,
	}
	return c.doJSONWithProxy(ctx, http.MethodPost, c.driveBaseURL+"/drive/v1/files:batchTrash", session.ProxyURL, session.UserAgent, sessionHeaders(session), nil, payload, nil)
}

// About 查询容量。
func (c *PikPakHTTPClient) About(ctx context.Context, session PikPakSession) (*PikPakAbout, error) {
	var resp PikPakAbout
	if err := c.doJSONWithProxy(ctx, http.MethodGet, c.aboutBaseURL+"/drive/v1/about", session.ProxyURL, session.UserAgent, sessionHeaders(session), nil, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *PikPakHTTPClient) doJSON(ctx context.Context, method string, rawURL string, userAgent string, headers map[string]string, query map[string]string, payload any, out any) error {
	return c.doJSONWithProxy(ctx, method, rawURL, "", userAgent, headers, query, payload, out)
}

func (c *PikPakHTTPClient) doJSONWithProxy(ctx context.Context, method string, rawURL string, proxyURL string, userAgent string, headers map[string]string, query map[string]string, payload any, out any) error {
	if c.httpClient == nil {
		c.httpClient = http.DefaultClient
	}
	if c.maxAttempts <= 0 {
		c.maxAttempts = 1
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	httpClient, err := c.httpClientForProxy(proxyURL)
	if err != nil {
		return err
	}
	values := parsed.Query()
	for key, value := range query {
		values.Set(key, value)
	}
	parsed.RawQuery = values.Encode()

	var bodyData []byte
	if payload != nil {
		data, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return marshalErr
		}
		bodyData = data
	}

	var lastErr error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		var body io.Reader
		if bodyData != nil {
			body = bytes.NewReader(bodyData)
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

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%w: %v", domainstorage.ErrCloudProviderUnavailable, err)
			if !isRetryablePikPakTransportError(err) || attempt >= c.maxAttempts {
				return lastErr
			}
			if err := c.waitBeforePikPakRetry(ctx, attempt, 0, method, parsed, err); err != nil {
				return err
			}
			continue
		}

		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		retryAfter := parsePikPakRetryAfter(resp.Header.Get("Retry-After"))
		if retryablePikPakStatus(resp.StatusCode) && attempt < c.maxAttempts {
			if err := c.waitBeforePikPakRetry(ctx, attempt, retryAfter, method, parsed, nil); err != nil {
				return err
			}
			continue
		}
		if err := mapPikPakHTTPErrorForRequest(resp.StatusCode, data, parsed.String()); err != nil {
			if retryAfter > 0 {
				if providerErr, ok := err.(*domainstorage.ProviderError); ok && providerErr.RetryAfterSeconds == 0 {
					providerErr.RetryAfterSeconds = int(retryAfter.Seconds())
				}
			}
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
	if lastErr != nil {
		return lastErr
	}
	return domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider unavailable")
}

func (c *PikPakHTTPClient) httpClientForProxy(requestProxyURL string) (*http.Client, error) {
	effectiveProxyURL := c.effectiveProxyURL(requestProxyURL)
	if effectiveProxyURL == "" {
		return c.httpClient, nil
	}
	parsedProxyURL, err := url.Parse(effectiveProxyURL)
	if err != nil || parsedProxyURL.Scheme == "" || parsedProxyURL.Host == "" {
		return nil, fmt.Errorf("%w: proxy_url must be a valid URL", domainstorage.ErrConfigInvalid)
	}
	switch strings.ToLower(parsedProxyURL.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("%w: proxy_url scheme must be http or https", domainstorage.ErrConfigInvalid)
	}
	if parsedProxyURL.User != nil {
		return nil, fmt.Errorf("%w: proxy_url must not include credentials", domainstorage.ErrConfigInvalid)
	}
	if parsedProxyURL.RawQuery != "" || parsedProxyURL.Fragment != "" {
		return nil, fmt.Errorf("%w: proxy_url must not include query or fragment", domainstorage.ErrConfigInvalid)
	}

	baseTransport, _ := http.DefaultTransport.(*http.Transport)
	transport := baseTransport.Clone()
	if customTransport, ok := c.httpClient.Transport.(*http.Transport); ok && customTransport != nil {
		transport = customTransport.Clone()
	}
	transport.Proxy = http.ProxyURL(parsedProxyURL)
	return &http.Client{
		Transport:     transport,
		CheckRedirect: c.httpClient.CheckRedirect,
		Jar:           c.httpClient.Jar,
		Timeout:       c.httpClient.Timeout,
	}, nil
}

func (c *PikPakHTTPClient) effectiveProxyURL(requestProxyURL string) string {
	if value := strings.TrimSpace(requestProxyURL); value != "" {
		return value
	}
	if c.httpClientCustom && c.proxyURLFromEnv {
		return ""
	}
	return strings.TrimSpace(c.defaultProxyURL)
}

func (c *PikPakHTTPClient) waitBeforePikPakRetry(ctx context.Context, attempt int, retryAfter time.Duration, method string, parsed *url.URL, cause error) error {
	delay := retryAfter
	if delay <= 0 {
		delay = c.retryBaseDelay * time.Duration(1<<(attempt-1))
	}
	if delay < 0 {
		delay = 0
	}
	slog.Default().WarnContext(ctx, "pikpak provider request retry",
		slog.String("event", "pikpak.provider.retry"),
		slog.String("method", method),
		slog.String("host", parsed.Host),
		slog.String("path", parsed.EscapedPath()),
		slog.Int("attempt", attempt),
		slog.Int("max_attempts", c.maxAttempts),
		slog.Int64("delay_ms", delay.Milliseconds()),
		slog.String("cause", sanitizedPikPakRetryCause(cause)),
	)
	if c.sleepBeforeRetry == nil {
		return nil
	}
	return c.sleepBeforeRetry(ctx, delay)
}

func retryablePikPakStatus(status int) bool {
	return status == http.StatusTooManyRequests ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout ||
		status >= 500
}

func isRetryablePikPakTransportError(err error) bool {
	return err != nil
}

func parsePikPakRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

func sanitizedPikPakRetryCause(err error) string {
	if err == nil {
		return ""
	}
	return "transport_error"
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

func pikPakCaptchaHeaders(cfg PikPakConfig, accessToken string) map[string]string {
	headers := map[string]string{
		"X-Device-ID":     cfg.DeviceID,
		"X-Captcha-Token": cfg.CaptchaToken,
	}
	if strings.TrimSpace(accessToken) != "" {
		headers["Authorization"] = "Bearer " + strings.TrimSpace(accessToken)
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
	Details          any    `json:"details"`
	URL              string `json:"url"`
	VerificationURL  string `json:"verification_url"`
	CaptchaURL       string `json:"captcha_url"`
	RedirectURL      string `json:"redirect_url"`
}

func mapPikPakHTTPError(status int, body []byte) error {
	return mapPikPakHTTPErrorForRequest(status, body, "")
}

func mapPikPakHTTPErrorForRequest(status int, body []byte, rawURL string) error {
	var payload pikPakErrorPayload
	_ = json.Unmarshal(body, &payload)
	code := pikPakProviderErrorSignal(payload)
	verificationURL := pikPakPayloadVerificationURL(payload)
	authFlow := isPikPakAuthFlowURL(rawURL)
	rootList := isPikPakRootListURL(rawURL)
	if isPikPakRegionBlockedPayload(payload) {
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudRegionBlocked, Message: "cloud region blocked", ProviderCode: code}
	}
	if code != "" && code != "0" {
		return mapPikPakProviderCode(code, payload, authFlow || rootList)
	}
	if status >= http.StatusBadRequest && verificationURL != "" {
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudCaptchaRequired, Message: "cloud captcha required", VerificationURL: verificationURL}
	}
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusNotFound:
		if authFlow || rootList {
			return &domainstorage.ProviderError{
				Kind:         domainstorage.ErrCloudCaptchaRequired,
				Message:      "cloud captcha required",
				ProviderCode: "resource_not_found",
			}
		}
		return os.ErrNotExist
	case status == http.StatusConflict:
		return fs.ErrExist
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

func mapPikPakProviderCode(code string, payload pikPakErrorPayload, authFlow bool) error {
	message := sanitizedPikPakErrorMessage(payload)
	verificationURL := pikPakPayloadVerificationURL(payload)
	if isPikPakRegionBlockedPayload(payload) {
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudRegionBlocked, Message: "cloud region blocked", ProviderCode: code}
	}
	if authFlow && (verificationURL != "" || strings.Contains(strings.ToLower(message), "verify") || strings.Contains(strings.ToLower(message), "captcha")) {
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudCaptchaRequired, Message: "cloud captcha required", ProviderCode: code, VerificationURL: verificationURL}
	}
	switch code {
	case "404", "not_found", "file_not_found", "resource_not_found":
		if authFlow {
			return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudCaptchaRequired, Message: "cloud captcha required", ProviderCode: code, VerificationURL: verificationURL}
		}
		return os.ErrNotExist
	case "409", "name_conflict", "file_already_exists", "already_exists", "duplicate":
		return fs.ErrExist
	case "401", "403", "4126", "4122", "4121", "16", "invalid_grant", "invalid_token", "unauthorized", "forbidden":
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudTokenInvalid, Message: "cloud token invalid", ProviderCode: code}
	case "auth_failed", "invalid_credentials":
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudAuthFailed, Message: "cloud auth failed", ProviderCode: code}
	case "9", "captcha_expired", "captcha_token_expired":
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudCaptchaExpired, Message: "cloud captcha expired", ProviderCode: code}
	case "captcha_required", "verification_required":
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudCaptchaRequired, Message: "cloud captcha required", ProviderCode: code, VerificationURL: verificationURL}
	case "10", "rate_limited", "too_many_requests":
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudRateLimited, Message: "cloud rate limited", ProviderCode: code}
	default:
		if strings.Contains(strings.ToLower(message), "verify") {
			return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudCaptchaRequired, Message: "cloud captcha required", ProviderCode: code, VerificationURL: verificationURL}
		}
		return &domainstorage.ProviderError{Kind: domainstorage.ErrCloudProviderUnavailable, Message: "cloud provider request failed", ProviderCode: code}
	}
}

func isPikPakAuthFlowURL(rawURL string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	pathValue := strings.ToLower(parsed.Path)
	return strings.Contains(pathValue, "/v1/auth/") || strings.Contains(pathValue, "/v1/shield/")
}

func isPikPakRootListURL(rawURL string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	pathValue := strings.ToLower(parsed.Path)
	if !strings.Contains(pathValue, "/drive/v1/files") {
		return false
	}
	parentID := strings.TrimSpace(parsed.Query().Get("parent_id"))
	return parentID == "" || parentID == "root"
}

func pikPakProviderErrorSignal(payload pikPakErrorPayload) string {
	code := normalizePikPakErrorCode(payload.ErrorCode)
	if code != "" {
		return code
	}
	return normalizePikPakErrorCode(payload.Error)
}

func normalizePikPakErrorCode(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.ToLower(strings.TrimSpace(typed))
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	default:
		return strings.ToLower(strings.TrimSpace(fmt.Sprint(typed)))
	}
}

func sanitizedPikPakErrorMessage(payload pikPakErrorPayload) string {
	for _, value := range []string{payload.ErrorDescription, payload.Error, pikPakPayloadDetailsString(payload.Details)} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func pikPakPayloadVerificationURL(payload pikPakErrorPayload) string {
	for _, value := range []string{payload.URL, payload.VerificationURL, payload.CaptchaURL, payload.RedirectURL} {
		if verificationURL := sanitizePikPakVerificationURL(value); verificationURL != "" {
			return verificationURL
		}
	}
	return pikPakDetailsVerificationURL(payload.Details)
}

func pikPakDetailsVerificationURL(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return sanitizePikPakVerificationURL(typed)
	case map[string]any:
		for _, key := range []string{"url", "verification_url", "captcha_url", "redirect_url"} {
			if verificationURL := pikPakDetailsVerificationURL(typed[key]); verificationURL != "" {
				return verificationURL
			}
		}
		for _, nested := range typed {
			if verificationURL := pikPakDetailsVerificationURL(nested); verificationURL != "" {
				return verificationURL
			}
		}
	case []any:
		for _, nested := range typed {
			if verificationURL := pikPakDetailsVerificationURL(nested); verificationURL != "" {
				return verificationURL
			}
		}
	}
	return ""
}

func sanitizePikPakVerificationURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Host != "" {
			return value
		}
	}
	return ""
}

func isPikPakRegionBlockedPayload(payload pikPakErrorPayload) bool {
	signal := strings.ToLower(strings.Join([]string{
		normalizePikPakErrorCode(payload.ErrorCode),
		payload.Error,
		payload.ErrorDescription,
		pikPakPayloadDetailsString(payload.Details),
	}, " "))
	for _, marker := range []string{
		"accessprohibited",
		"prohibited:cn",
		"not available in your region",
		"mainland china",
	} {
		if strings.Contains(signal, marker) {
			return true
		}
	}
	return false
}

func pikPakPayloadDetailsString(details any) string {
	switch typed := details.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
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

func buildPikPakUserAgent(cfg PikPakConfig, platform pikPakPlatformInfo, userID string) string {
	if strings.EqualFold(cfg.Platform, "android") {
		return buildPikPakAndroidUserAgent(cfg.DeviceID, platform, userID)
	}
	return platform.UserAgent
}

func buildPikPakAndroidUserAgent(deviceID string, platform pikPakPlatformInfo, userID string) string {
	deviceSign := generatePikPakDeviceSign(deviceID, platform.PackageName)
	return fmt.Sprintf(
		"ANDROID-%s/%s protocolVersion/200 accesstype/ clientid/%s clientversion/%s action_type/ networktype/WIFI sessionid/ deviceid/%s providername/NONE devicesign/%s refresh_token/ sdkversion/%s datetime/%d usrno/%s appname/android-%s session_origin/ grant_type/ appid/ clientip/ devicename/Xiaomi_M2004j7ac osversion/13 platformversion/10 accessmode/ devicemodel/M2004J7AC ",
		platform.PackageName,
		platform.ClientVersion,
		platform.ClientID,
		platform.ClientVersion,
		deviceID,
		deviceSign,
		platform.SdkVersion,
		time.Now().UnixMilli(),
		userID,
		platform.PackageName,
	)
}

func generatePikPakDeviceSign(deviceID string, packageName string) string {
	signatureBase := deviceID + packageName + "1" + "appkey"
	sha1Sum := sha1.Sum([]byte(signatureBase))
	md5Sum := md5.Sum([]byte(hex.EncodeToString(sha1Sum[:])))
	return "div101." + deviceID + hex.EncodeToString(md5Sum[:])
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func isPikPakEmail(value string) bool {
	ok, _ := regexp.MatchString(`\w+([-+.]\w+)*@\w+([-.]\w+)*\.\w+([-.]\w+)*`, value)
	return ok
}
