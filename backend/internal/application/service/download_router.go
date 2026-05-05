package service

import (
	"net/url"
	"path"
	"strings"
)

const (
	// DownloaderTypeAria2 表示 HTTP/HTTPS 下载继续使用 Aria2。
	DownloaderTypeAria2 = "aria2"
	// DownloaderTypeQBittorrent 表示 BT/magnet 下载使用 qBittorrent。
	DownloaderTypeQBittorrent = "qbittorrent"
	// DownloaderTypePikPakNative 表示目标 PikPak source 使用 provider 原生离线下载。
	DownloaderTypePikPakNative = "pikpak_native"
)

const (
	RSSLinkTypeMagnet      = "magnet"
	RSSLinkTypeTorrent     = "torrent"
	RSSLinkTypeHTTP        = "http"
	RSSLinkTypeUnsupported = "unsupported"
)

// DownloaderRouter 根据 URL 类型选择具体下载器。
type DownloaderRouter struct {
	downloaders map[string]Downloader
}

// NewDownloaderRouter 创建下载器路由器。
func NewDownloaderRouter(defaultAria2 Downloader) *DownloaderRouter {
	router := &DownloaderRouter{downloaders: make(map[string]Downloader)}
	if defaultAria2 != nil {
		router.Register(DownloaderTypeAria2, defaultAria2)
	}
	return router
}

// Register 注册下载器。
func (r *DownloaderRouter) Register(downloaderType string, downloader Downloader) {
	if r == nil || strings.TrimSpace(downloaderType) == "" || downloader == nil {
		return
	}
	r.downloaders[downloaderType] = downloader
}

// Select 根据 URI 选择下载器类型和实例。
func (r *DownloaderRouter) Select(rawURI string) (string, Downloader, error) {
	switch ClassifyDownloadLink(rawURI) {
	case RSSLinkTypeHTTP:
		downloader, err := r.Get(DownloaderTypeAria2)
		return DownloaderTypeAria2, downloader, err
	case RSSLinkTypeMagnet, RSSLinkTypeTorrent:
		downloader, err := r.Get(DownloaderTypeQBittorrent)
		return DownloaderTypeQBittorrent, downloader, err
	default:
		return "", nil, ErrDownloadLinkUnsupported
	}
}

// Get 按类型返回下载器。
func (r *DownloaderRouter) Get(downloaderType string) (Downloader, error) {
	if r == nil || r.downloaders == nil {
		return nil, ErrSourceDriverUnsupported
	}
	downloader, ok := r.downloaders[downloaderType]
	if !ok || downloader == nil {
		return nil, ErrSourceDriverUnsupported
	}
	return downloader, nil
}

// ClassifyDownloadLink 判断下载链接类型。
func ClassifyDownloadLink(rawLink string) string {
	trimmed := strings.TrimSpace(rawLink)
	if trimmed == "" {
		return RSSLinkTypeUnsupported
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "magnet:?") {
		return RSSLinkTypeMagnet
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" {
		return RSSLinkTypeUnsupported
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return RSSLinkTypeUnsupported
	}
	if strings.EqualFold(path.Ext(parsed.Path), ".torrent") || strings.HasSuffix(strings.ToLower(parsed.Path), ".torrent") {
		return RSSLinkTypeTorrent
	}
	return RSSLinkTypeHTTP
}

// IsBTRSSDownloadLink 判断 RSS MVP 是否支持自动入队该链接。
func IsBTRSSDownloadLink(rawLink string) bool {
	switch ClassifyDownloadLink(rawLink) {
	case RSSLinkTypeMagnet, RSSLinkTypeTorrent:
		return true
	default:
		return false
	}
}
