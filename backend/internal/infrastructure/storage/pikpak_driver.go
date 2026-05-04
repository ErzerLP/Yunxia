package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"yunxia/internal/domain/entity"
	domainstorage "yunxia/internal/domain/storage"
)

const (
	defaultPikPakSearchMaxDepth   = 3
	defaultPikPakSearchMaxEntries = 500
)

// PikPakDriver 提供 PikPak 只读存储源能力。
type PikPakDriver struct {
	sessions         *PikPakSessionManager
	searchMaxDepth   int
	searchMaxEntries int
}

// PikPakDriverOption 定义 PikPak driver 可选配置。
type PikPakDriverOption func(*PikPakDriver)

// NewPikPakDriver 创建 PikPak driver。
func NewPikPakDriver(options ...PikPakDriverOption) *PikPakDriver {
	driver := &PikPakDriver{
		sessions:         NewPikPakSessionManager(nil),
		searchMaxDepth:   defaultPikPakSearchMaxDepth,
		searchMaxEntries: defaultPikPakSearchMaxEntries,
	}
	for _, option := range options {
		option(driver)
	}
	return driver
}

// WithPikPakAPIClient 注入 fake/client，避免测试访问真实 PikPak。
func WithPikPakAPIClient(client PikPakAPIClient) PikPakDriverOption {
	return func(d *PikPakDriver) {
		if client != nil {
			d.sessions = NewPikPakSessionManager(client)
		}
	}
}

// WithPikPakSessionManager 注入自定义 SessionManager。
func WithPikPakSessionManager(manager *PikPakSessionManager) PikPakDriverOption {
	return func(d *PikPakDriver) {
		if manager != nil {
			d.sessions = manager
		}
	}
}

// WithPikPakSearchLimits 覆盖递归搜索限制。
func WithPikPakSearchLimits(maxDepth int, maxEntries int) PikPakDriverOption {
	return func(d *PikPakDriver) {
		if maxDepth >= 0 {
			d.searchMaxDepth = maxDepth
		}
		if maxEntries > 0 {
			d.searchMaxEntries = maxEntries
		}
	}
}

// Test 做最小连通性检查：建立 session 后列根目录。
func (d *PikPakDriver) Test(ctx context.Context, source *entity.StorageSource) error {
	return d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		_, err := d.listFilesAll(ctx, session, cfg.RootFolderID)
		return err
	})
}

// List 列出指定目录。
func (d *PikPakDriver) List(ctx context.Context, source *entity.StorageSource, virtualPath string) ([]domainstorage.StorageEntry, error) {
	virtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return nil, err
	}

	var entries []domainstorage.StorageEntry
	err = d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		dir, resolveErr := d.resolvePathWithSession(ctx, session, cfg, virtualPath)
		if resolveErr != nil {
			return resolveErr
		}
		if !dir.isFolder() {
			return fs.ErrInvalid
		}
		files, listErr := d.listFilesAll(ctx, session, dir.ID)
		if listErr != nil {
			return listErr
		}
		entries = make([]domainstorage.StorageEntry, 0, len(files))
		for _, file := range files {
			entries = append(entries, pikPakFileToStorageEntry(file, joinVirtualPath(virtualPath, file.Name)))
		}
		return nil
	})
	return entries, err
}

// SearchByName 做受限递归搜索，避免前端搜索直接不可用。
func (d *PikPakDriver) SearchByName(ctx context.Context, source *entity.StorageSource, pathPrefix, keyword string) ([]domainstorage.StorageEntry, error) {
	pathPrefix, err := normalizeVirtualPath(pathPrefix)
	if err != nil {
		return nil, err
	}
	lowerKeyword := strings.ToLower(strings.TrimSpace(keyword))

	results := make([]domainstorage.StorageEntry, 0)
	err = d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		root, resolveErr := d.resolvePathWithSession(ctx, session, cfg, pathPrefix)
		if resolveErr != nil {
			return resolveErr
		}
		if !root.isFolder() {
			if lowerKeyword == "" || strings.Contains(strings.ToLower(root.Name), lowerKeyword) {
				results = append(results, pikPakFileToStorageEntry(root, pathPrefix))
			}
			return nil
		}
		return d.searchRecursive(ctx, session, root.ID, pathPrefix, lowerKeyword, d.searchMaxDepth, &results)
	})
	return results, err
}

// Stat 返回对象信息。根目录直接返回目录 entry；其他路径通过父目录 list 匹配。
func (d *PikPakDriver) Stat(ctx context.Context, source *entity.StorageSource, virtualPath string) (*domainstorage.StorageEntry, error) {
	virtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return nil, err
	}
	if virtualPath == "/" {
		return &domainstorage.StorageEntry{
			Name:       "/",
			Path:       "/",
			IsDir:      true,
			ModifiedAt: time.Now(),
		}, nil
	}

	var entry *domainstorage.StorageEntry
	err = d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		file, resolveErr := d.resolvePathWithSession(ctx, session, cfg, virtualPath)
		if resolveErr != nil {
			return resolveErr
		}
		storageEntry := pikPakFileToStorageEntry(file, virtualPath)
		entry = &storageEntry
		return nil
	})
	return entry, err
}

// PresignDownload 获取 PikPak 临时下载链接，FileService 会负责 302。
func (d *PikPakDriver) PresignDownload(ctx context.Context, source *entity.StorageSource, virtualPath, _ string, ttl time.Duration) (string, time.Time, error) {
	virtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return "", time.Time{}, err
	}

	var downloadURL string
	err = d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		file, resolveErr := d.resolvePathWithSession(ctx, session, cfg, virtualPath)
		if resolveErr != nil {
			return resolveErr
		}
		if file.isFolder() {
			return fs.ErrInvalid
		}
		usage := "FETCH"
		if !cfg.DisableMediaLink {
			usage = "CACHE"
		}
		detail, detailErr := d.sessions.client.GetFile(ctx, session, file.ID, usage)
		if detailErr != nil {
			return detailErr
		}
		downloadURL = pikPakDownloadURL(*detail, cfg.DisableMediaLink)
		if downloadURL == "" {
			return os.ErrNotExist
		}
		return nil
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return downloadURL, time.Now().Add(ttl), nil
}

// Capacity 查询 PikPak 容量详情。
func (d *PikPakDriver) Capacity(ctx context.Context, source *entity.StorageSource) (*domainstorage.CapacityInfo, error) {
	var info *domainstorage.CapacityInfo
	err := d.sessions.withSession(ctx, source, func(session PikPakSession, _ PikPakConfig) error {
		about, err := d.sessions.client.About(ctx, session)
		if err != nil {
			return err
		}
		used := parsePikPakInt64(about.Quota.Usage)
		total := parsePikPakInt64(about.Quota.Limit)
		info = &domainstorage.CapacityInfo{
			UsedBytes:  used,
			TotalBytes: total,
		}
		return nil
	})
	return info, err
}

// Capabilities 描述 PikPak 阶段 B 只读能力，供上层禁用写入口提示。
func (d *PikPakDriver) Capabilities(context.Context, *entity.StorageSource) (domainstorage.StorageCapabilities, error) {
	return domainstorage.StorageCapabilities{
		CanList:       true,
		CanSearch:     true,
		CanDownload:   true,
		CanCapacity:   true,
		CanMkdir:      false,
		CanRename:     false,
		CanMove:       false,
		CanCopy:       false,
		CanDelete:     false,
		CanImportFile: false,
	}, nil
}

// Mkdir 阶段 B 暂未实现，返回稳定 unsupported。
func (d *PikPakDriver) Mkdir(context.Context, *entity.StorageSource, string, string) (*domainstorage.StorageEntry, error) {
	return nil, domainstorage.ErrOperationUnsupported
}

// Rename 阶段 B 暂未实现，返回稳定 unsupported。
func (d *PikPakDriver) Rename(context.Context, *entity.StorageSource, string, string) (*domainstorage.StorageEntry, error) {
	return nil, domainstorage.ErrOperationUnsupported
}

// Move 阶段 B 暂未实现，返回稳定 unsupported。
func (d *PikPakDriver) Move(context.Context, *entity.StorageSource, string, string) error {
	return domainstorage.ErrOperationUnsupported
}

// Copy 阶段 B 暂未实现，返回稳定 unsupported。
func (d *PikPakDriver) Copy(context.Context, *entity.StorageSource, string, string) error {
	return domainstorage.ErrOperationUnsupported
}

// Delete 阶段 B 暂未实现，返回稳定 unsupported。
func (d *PikPakDriver) Delete(context.Context, *entity.StorageSource, string) error {
	return domainstorage.ErrOperationUnsupported
}

func (d *PikPakDriver) resolvePathWithSession(ctx context.Context, session PikPakSession, cfg PikPakConfig, virtualPath string) (PikPakFile, error) {
	virtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return PikPakFile{}, err
	}
	current := PikPakFile{
		ID:   cfg.RootFolderID,
		Name: "/",
		Kind: "drive#folder",
	}
	if virtualPath == "/" {
		return current, nil
	}

	segments := strings.Split(strings.TrimPrefix(virtualPath, "/"), "/")
	for index, segment := range segments {
		files, listErr := d.listFilesAll(ctx, session, current.ID)
		if listErr != nil {
			return PikPakFile{}, listErr
		}
		found := false
		for _, file := range files {
			if file.Name != segment {
				continue
			}
			current = file
			found = true
			break
		}
		if !found {
			return PikPakFile{}, os.ErrNotExist
		}
		if index < len(segments)-1 && !current.isFolder() {
			return PikPakFile{}, os.ErrNotExist
		}
	}
	return current, nil
}

func (d *PikPakDriver) listFilesAll(ctx context.Context, session PikPakSession, parentID string) ([]PikPakFile, error) {
	files := make([]PikPakFile, 0)
	pageToken := ""
	for {
		resp, err := d.sessions.client.ListFiles(ctx, session, parentID, pageToken)
		if err != nil {
			return nil, err
		}
		files = append(files, resp.Files...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return files, nil
}

func (d *PikPakDriver) searchRecursive(ctx context.Context, session PikPakSession, parentID string, parentPath string, lowerKeyword string, depth int, results *[]domainstorage.StorageEntry) error {
	if d.searchMaxEntries > 0 && len(*results) >= d.searchMaxEntries {
		return nil
	}
	files, err := d.listFilesAll(ctx, session, parentID)
	if err != nil {
		return err
	}
	for _, file := range files {
		if d.searchMaxEntries > 0 && len(*results) >= d.searchMaxEntries {
			return nil
		}
		filePath := joinVirtualPath(parentPath, file.Name)
		if lowerKeyword == "" || strings.Contains(strings.ToLower(file.Name), lowerKeyword) {
			*results = append(*results, pikPakFileToStorageEntry(file, filePath))
		}
		if file.isFolder() && depth > 0 {
			if err := d.searchRecursive(ctx, session, file.ID, filePath, lowerKeyword, depth-1, results); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return err
			}
		}
	}
	return nil
}

func pikPakFileToStorageEntry(file PikPakFile, virtualPath string) domainstorage.StorageEntry {
	modifiedAt := parsePikPakTime(file.ModifiedTime)
	if modifiedAt.IsZero() {
		modifiedAt = parsePikPakTime(file.CreatedTime)
	}
	if modifiedAt.IsZero() {
		modifiedAt = time.Unix(0, 0).UTC()
	}
	size := int64(0)
	if !file.isFolder() {
		size = parsePikPakSize(file.Size)
	}
	return domainstorage.StorageEntry{
		Name:       file.Name,
		Path:       virtualPath,
		IsDir:      file.isFolder(),
		Size:       size,
		ETag:       file.Hash,
		ModifiedAt: modifiedAt,
	}
}

func (f PikPakFile) isFolder() bool {
	return f.Kind == "drive#folder"
}

func parsePikPakSize(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func parsePikPakInt64(value string) *int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 0 {
		return nil
	}
	return &parsed
}

func parsePikPakTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func pikPakDownloadURL(file PikPakFile, disableMediaLink bool) string {
	if !disableMediaLink {
		for _, media := range file.Medias {
			if strings.TrimSpace(media.Link.URL) != "" {
				return media.Link.URL
			}
		}
	}
	return strings.TrimSpace(file.WebContentLink)
}
