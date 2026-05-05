package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	domainstorage "yunxia/internal/domain/storage"
)

const (
	pikPakGCIDMinBlockSize = 0x40000
	pikPakGCIDMaxBlockSize = 0x200000
	pikPakGCIDMaxBlocks    = 0x200
)

// PikPakUploadHashCalculator 计算 PikPak 创建上传任务需要的 hash。
type PikPakUploadHashCalculator interface {
	HashFile(ctx context.Context, localPath string) (string, error)
}

// PikPakGCIDUploadHashCalculator 计算 PikPak resumable 上传所需 GCID。
type PikPakGCIDUploadHashCalculator struct{}

// PikPakSHA1UploadHashCalculator 保留旧名兼容，实际计算 PikPak GCID。
type PikPakSHA1UploadHashCalculator = PikPakGCIDUploadHashCalculator

// HashFile 返回文件内容 GCID 的大写十六进制字符串。
func (PikPakGCIDUploadHashCalculator) HashFile(ctx context.Context, localPath string) (string, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	return calculatePikPakGCID(ctx, file, info.Size())
}

// PikPakOSSUploader 抽象 OSS 实体上传，测试必须注入 fake，避免访问真实 OSS。
type PikPakOSSUploader interface {
	PutObject(ctx context.Context, params PikPakOSSUploadParams, localPath string, contentType string) error
}

// PikPakHTTPOSSUploaderOption 定义默认 OSS uploader 的可选配置。
type PikPakHTTPOSSUploaderOption func(*PikPakHTTPOSSUploader)

// PikPakHTTPOSSUploader 使用 Aliyun OSS V1 签名执行单文件 PutObject。
type PikPakHTTPOSSUploader struct {
	httpClient *http.Client
	now        func() time.Time
	userAgent  string
}

// NewPikPakHTTPOSSUploader 创建默认 OSS PutObject uploader。
func NewPikPakHTTPOSSUploader(options ...PikPakHTTPOSSUploaderOption) *PikPakHTTPOSSUploader {
	uploader := &PikPakHTTPOSSUploader{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		now:        time.Now,
		userAgent:  "aliyun-sdk-android/2.9.13(Linux/Android 14;Yunxia)",
	}
	for _, option := range options {
		option(uploader)
	}
	return uploader
}

// WithPikPakOSSHTTPClient 注入自定义 HTTP client，主要用于 httptest。
func WithPikPakOSSHTTPClient(httpClient *http.Client) PikPakHTTPOSSUploaderOption {
	return func(u *PikPakHTTPOSSUploader) {
		if httpClient != nil {
			u.httpClient = httpClient
		}
	}
}

// WithPikPakOSSNow 覆盖时间源，便于签名测试。
func WithPikPakOSSNow(now func() time.Time) PikPakHTTPOSSUploaderOption {
	return func(u *PikPakHTTPOSSUploader) {
		if now != nil {
			u.now = now
		}
	}
}

// PutObject 将本地 staging 文件上传到 PikPak 返回的 OSS 对象地址。
func (u *PikPakHTTPOSSUploader) PutObject(ctx context.Context, params PikPakOSSUploadParams, localPath string, contentType string) error {
	if u.httpClient == nil {
		u.httpClient = http.DefaultClient
	}
	if u.now == nil {
		u.now = time.Now
	}
	if err := validatePikPakOSSParams(params); err != nil {
		return err
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}

	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	objectURL, canonicalResource, err := buildPikPakOSSObjectURL(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, objectURL, file)
	if err != nil {
		return err
	}
	req.ContentLength = info.Size()
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Date", u.now().UTC().Format(http.TimeFormat))
	if u.userAgent != "" {
		req.Header.Set("User-Agent", u.userAgent)
	}
	if params.SecurityToken != "" {
		req.Header.Set("X-OSS-Security-Token", params.SecurityToken)
	}
	req.Header.Set("Authorization", buildPikPakOSSPutAuthorization(params, req.Header, canonicalResource))

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", domainstorage.ErrCloudProviderUnavailable, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return domainstorage.NewProviderError(domainstorage.ErrCloudTokenInvalid, "cloud token invalid")
	case http.StatusTooManyRequests:
		return domainstorage.NewProviderError(domainstorage.ErrCloudRateLimited, "cloud rate limited")
	default:
		return domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider upload failed")
	}
}

func validatePikPakOSSParams(params PikPakOSSUploadParams) error {
	if strings.TrimSpace(params.Endpoint) == "" ||
		strings.TrimSpace(params.Bucket) == "" ||
		strings.TrimSpace(params.Key) == "" ||
		strings.TrimSpace(params.AccessKeyID) == "" ||
		strings.TrimSpace(params.AccessKeySecret) == "" {
		return domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider upload params invalid")
	}
	return nil
}

func calculatePikPakGCID(ctx context.Context, reader io.Reader, size int64) (string, error) {
	blockSize := pikPakGCIDBlockSize(size)
	buffer := make([]byte, blockSize)
	totalHash := sha1.New()
	blockHash := sha1.New()

	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := io.ReadFull(reader, buffer)
		if err == io.EOF && n == 0 {
			break
		}
		if err != nil && err != io.ErrUnexpectedEOF {
			return "", err
		}
		blockHash.Reset()
		if _, writeErr := blockHash.Write(buffer[:n]); writeErr != nil {
			return "", writeErr
		}
		if _, writeErr := totalHash.Write(blockHash.Sum(nil)); writeErr != nil {
			return "", writeErr
		}
		if err == io.ErrUnexpectedEOF {
			break
		}
	}

	return strings.ToUpper(hex.EncodeToString(totalHash.Sum(nil))), nil
}

func pikPakGCIDBlockSize(size int64) int {
	blockSize := int64(pikPakGCIDMinBlockSize)
	for size/blockSize > pikPakGCIDMaxBlocks && blockSize < pikPakGCIDMaxBlockSize {
		blockSize <<= 1
	}
	return int(blockSize)
}

func buildPikPakOSSObjectURL(params PikPakOSSUploadParams) (string, string, error) {
	endpoint := strings.TrimSpace(params.Endpoint)
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", "", domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider upload endpoint invalid")
	}
	bucket := strings.TrimSpace(params.Bucket)
	key := strings.TrimLeft(strings.TrimSpace(params.Key), "/")
	canonicalResource := "/" + bucket + "/" + key
	if shouldUsePikPakOSSPathStyle(parsed.Hostname()) {
		parsed.Path = joinURLPath(parsed.Path, bucket, key)
	} else {
		if !strings.HasPrefix(parsed.Host, bucket+".") {
			parsed.Host = bucket + "." + parsed.Host
		}
		parsed.Path = joinURLPath(parsed.Path, key)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), canonicalResource, nil
}

func shouldUsePikPakOSSPathStyle(host string) bool {
	if host == "localhost" {
		return true
	}
	return net.ParseIP(host) != nil
}

func joinURLPath(base string, parts ...string) string {
	items := make([]string, 0, len(parts)+1)
	if strings.TrimSpace(base) != "" && base != "/" {
		items = append(items, strings.Trim(base, "/"))
	}
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			items = append(items, part)
		}
	}
	if len(items) == 0 {
		return "/"
	}
	return "/" + strings.Join(items, "/")
}

func buildPikPakOSSPutAuthorization(params PikPakOSSUploadParams, headers http.Header, canonicalResource string) string {
	ossHeaders := canonicalizedOSSHeaders(headers)
	stringToSign := strings.Join([]string{
		http.MethodPut,
		"",
		headers.Get("Content-Type"),
		headers.Get("Date"),
	}, "\n") + "\n" + ossHeaders + canonicalResource
	mac := hmac.New(sha1.New, []byte(params.AccessKeySecret))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return "OSS " + params.AccessKeyID + ":" + signature
}

func canonicalizedOSSHeaders(headers http.Header) string {
	keys := make([]string, 0)
	values := make(map[string][]string)
	for key, headerValues := range headers {
		lower := strings.ToLower(strings.TrimSpace(key))
		if !strings.HasPrefix(lower, "x-oss-") {
			continue
		}
		keys = append(keys, lower)
		values[lower] = headerValues
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		cleaned := make([]string, 0, len(values[key]))
		for _, value := range values[key] {
			cleaned = append(cleaned, strings.TrimSpace(value))
		}
		builder.WriteString(key)
		builder.WriteString(":")
		builder.WriteString(strings.Join(cleaned, ","))
		builder.WriteString("\n")
	}
	return builder.String()
}

func contentTypeForPikPakImport(targetPath string) string {
	contentType := mime.TypeByExtension(strings.ToLower(path.Ext(targetPath)))
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}
