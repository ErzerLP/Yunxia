package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	appsvc "yunxia/internal/application/service"
)

// QBittorrentClient 实现 qBittorrent Web API 下载器。
type QBittorrentClient struct {
	apiURL   string
	username string
	password string
	client   *http.Client
}

// NewQBittorrentClient 创建 qBittorrent 客户端。
func NewQBittorrentClient(apiURL, username, password string) *QBittorrentClient {
	jar, _ := cookiejar.New(nil)
	return &QBittorrentClient{
		apiURL:   strings.TrimRight(apiURL, "/"),
		username: username,
		password: password,
		client:   &http.Client{Timeout: 30 * time.Second, Jar: jar},
	}
}

// Health 检查 qBittorrent Web API 是否可用。
func (c *QBittorrentClient) Health(ctx context.Context) error {
	if err := c.login(ctx); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/api/v2/app/version"), nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("qbittorrent health status %d", response.StatusCode)
	}
	return nil
}

// AddURI 添加 magnet 或 torrent URL。
func (c *QBittorrentClient) AddURI(ctx context.Context, uri string, dir string) (string, error) {
	tag := "yunxia-task-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	form := url.Values{}
	form.Set("urls", uri)
	if dir != "" {
		form.Set("savepath", dir)
	}
	form.Set("tags", tag)
	form.Set("paused", "false")
	if err := c.postForm(ctx, "/api/v2/torrents/add", form); err != nil {
		return "", err
	}
	return tag, nil
}

// TellStatus 查询 torrent 状态。
func (c *QBittorrentClient) TellStatus(ctx context.Context, externalID string) (*appsvc.DownloadStatus, error) {
	torrent, err := c.findTorrent(ctx, externalID)
	if err != nil {
		return nil, err
	}
	if torrent == nil {
		return &appsvc.DownloadStatus{Status: "canceled"}, nil
	}
	return mapQBitTorrentStatus(*torrent), nil
}

// Pause 暂停 torrent。
func (c *QBittorrentClient) Pause(ctx context.Context, externalID string) error {
	torrent, err := c.findTorrent(ctx, externalID)
	if err != nil || torrent == nil {
		return err
	}
	form := url.Values{"hashes": []string{torrent.Hash}}
	if err := c.postForm(ctx, "/api/v2/torrents/pause", form); err != nil {
		return c.postForm(ctx, "/api/v2/torrents/stop", form)
	}
	return nil
}

// Resume 恢复 torrent。
func (c *QBittorrentClient) Resume(ctx context.Context, externalID string) error {
	torrent, err := c.findTorrent(ctx, externalID)
	if err != nil || torrent == nil {
		return err
	}
	form := url.Values{"hashes": []string{torrent.Hash}}
	if err := c.postForm(ctx, "/api/v2/torrents/resume", form); err != nil {
		return c.postForm(ctx, "/api/v2/torrents/start", form)
	}
	return nil
}

// Remove 删除 torrent，默认不删除已下载文件，由 Yunxia 管理 staging 清理。
func (c *QBittorrentClient) Remove(ctx context.Context, externalID string) error {
	torrent, err := c.findTorrent(ctx, externalID)
	if err != nil || torrent == nil {
		return err
	}
	form := url.Values{}
	form.Set("hashes", torrent.Hash)
	form.Set("deleteFiles", "false")
	return c.postForm(ctx, "/api/v2/torrents/delete", form)
}

func (c *QBittorrentClient) findTorrent(ctx context.Context, externalID string) (*qbitTorrentInfo, error) {
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("tag", externalID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/api/v2/torrents/info")+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qbittorrent info status %d", response.StatusCode)
	}
	var torrents []qbitTorrentInfo
	if err := json.NewDecoder(response.Body).Decode(&torrents); err != nil {
		return nil, err
	}
	if len(torrents) == 0 {
		return nil, nil
	}
	return &torrents[0], nil
}

func (c *QBittorrentClient) login(ctx context.Context) error {
	if c.username == "" && c.password == "" {
		return nil
	}
	form := url.Values{}
	form.Set("username", c.username)
	form.Set("password", c.password)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/api/v2/auth/login"), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("qbittorrent login status %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(body)) != "Ok." {
		return fmt.Errorf("qbittorrent login failed")
	}
	return nil
}

func (c *QBittorrentClient) postForm(ctx context.Context, path string, form url.Values) error {
	if err := c.login(ctx); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("qbittorrent %s status %d", path, response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if strings.HasPrefix(strings.TrimSpace(string(body)), "Fails.") {
		return fmt.Errorf("qbittorrent %s failed", path)
	}
	return nil
}

func (c *QBittorrentClient) endpoint(path string) string {
	return c.apiURL + path
}

type qbitTorrentInfo struct {
	Hash       string  `json:"hash"`
	Name       string  `json:"name"`
	State      string  `json:"state"`
	Progress   float64 `json:"progress"`
	Downloaded int64   `json:"downloaded"`
	TotalSize  int64   `json:"total_size"`
	DLSpeed    int64   `json:"dlspeed"`
	ETA        int64   `json:"eta"`
	AmountLeft int64   `json:"amount_left"`
}

func mapQBitTorrentStatus(raw qbitTorrentInfo) *appsvc.DownloadStatus {
	status := mapQBitState(raw.State, raw.Progress, raw.AmountLeft)
	var totalBytes *int64
	if raw.TotalSize > 0 {
		totalBytes = &raw.TotalSize
	}
	var etaSeconds *int64
	if raw.ETA > 0 && raw.ETA < 8640000 {
		etaSeconds = &raw.ETA
	}
	return &appsvc.DownloadStatus{
		Status:         status,
		CompletedBytes: raw.Downloaded,
		TotalBytes:     totalBytes,
		DownloadSpeed:  raw.DLSpeed,
		ETASeconds:     etaSeconds,
		DisplayName:    raw.Name,
	}
}

func mapQBitState(state string, progress float64, amountLeft int64) string {
	if progress >= 1 && amountLeft == 0 {
		return "completed"
	}
	switch state {
	case "pausedDL", "pausedUP", "stoppedDL", "stoppedUP":
		return "paused"
	case "queuedDL", "queuedUP", "checkingDL", "checkingUP", "checkingResumeData", "metaDL":
		return "pending"
	case "downloading", "stalledDL", "forcedDL", "allocating", "uploading", "stalledUP", "forcedUP":
		return "running"
	case "error", "missingFiles", "unknown":
		return "failed"
	default:
		return "running"
	}
}
