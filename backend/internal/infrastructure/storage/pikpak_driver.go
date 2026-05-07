package storage

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
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

// PikPakDriver 提供 PikPak 存储源文件能力。
type PikPakDriver struct {
	sessions          *PikPakSessionManager
	searchMaxDepth    int
	searchMaxEntries  int
	uploadHasher      PikPakUploadHashCalculator
	ossUploader       PikPakOSSUploader
	ossInstructionNow func() time.Time
	pathCache         *PikPakPathCache
}

// PikPakDriverOption 定义 PikPak driver 可选配置。
type PikPakDriverOption func(*PikPakDriver)

// NewPikPakDriver 创建 PikPak driver。
func NewPikPakDriver(options ...PikPakDriverOption) *PikPakDriver {
	driver := &PikPakDriver{
		sessions:          NewPikPakSessionManager(nil),
		searchMaxDepth:    defaultPikPakSearchMaxDepth,
		searchMaxEntries:  defaultPikPakSearchMaxEntries,
		uploadHasher:      PikPakGCIDUploadHashCalculator{},
		ossUploader:       NewPikPakHTTPOSSUploader(),
		ossInstructionNow: time.Now,
		pathCache:         NewPikPakPathCache(),
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
			var writer func(context.Context, *entity.StorageSource, string) error
			if d.sessions != nil {
				writer = d.sessions.runtimeConfigWriter
			}
			d.sessions = NewPikPakSessionManager(client, WithPikPakSessionRuntimeConfigWriter(writer))
		}
	}
}

// WithPikPakSessionManager 注入自定义 SessionManager。
func WithPikPakSessionManager(manager *PikPakSessionManager) PikPakDriverOption {
	return func(d *PikPakDriver) {
		if manager != nil {
			if d.sessions != nil && manager.runtimeConfigWriter == nil {
				manager.runtimeConfigWriter = d.sessions.runtimeConfigWriter
			}
			d.sessions = manager
		}
	}
}

// WithPikPakRuntimeConfigWriter 注入运行态 refresh_token/captcha/device_id 持久化回写函数。
func WithPikPakRuntimeConfigWriter(writer func(context.Context, *entity.StorageSource, string) error) PikPakDriverOption {
	return func(d *PikPakDriver) {
		if d.sessions == nil {
			d.sessions = NewPikPakSessionManager(nil)
		}
		d.sessions.runtimeConfigWriter = writer
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

// WithPikPakUploadHashCalculator 注入 PikPak 上传 hash 计算器。
func WithPikPakUploadHashCalculator(calculator PikPakUploadHashCalculator) PikPakDriverOption {
	return func(d *PikPakDriver) {
		if calculator != nil {
			d.uploadHasher = calculator
		}
	}
}

// WithPikPakOSSUploader 注入 OSS uploader，测试应使用 fake 避免真实网络。
func WithPikPakOSSUploader(uploader PikPakOSSUploader) PikPakDriverOption {
	return func(d *PikPakDriver) {
		if uploader != nil {
			d.ossUploader = uploader
		}
	}
}

// WithPikPakOSSInstructionNow 覆盖直传 OSS 签名时间，便于测试。
func WithPikPakOSSInstructionNow(now func() time.Time) PikPakDriverOption {
	return func(d *PikPakDriver) {
		if now != nil {
			d.ossInstructionNow = now
		}
	}
}

// WithPikPakPathCache 注入路径缓存；传 nil 表示沿用默认缓存。
func WithPikPakPathCache(cache *PikPakPathCache) PikPakDriverOption {
	return func(d *PikPakDriver) {
		if cache != nil {
			d.pathCache = cache
		}
	}
}

// Test 做最小连通性检查：建立 session 后列根目录。
func (d *PikPakDriver) Test(ctx context.Context, source *entity.StorageSource) error {
	return d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		_, err := d.listFilesAll(ctx, session, cfg.providerRootID())
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
		dir, resolveErr := d.resolvePathWithSession(ctx, source.ID, session, cfg, virtualPath)
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
		d.cacheChildren(source.ID, cfg, virtualPath, files)
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
		root, resolveErr := d.resolvePathWithSession(ctx, source.ID, session, cfg, pathPrefix)
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
		file, resolveErr := d.resolvePathWithSession(ctx, source.ID, session, cfg, virtualPath)
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
		file, resolveErr := d.resolvePathWithSession(ctx, source.ID, session, cfg, virtualPath)
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

// Capabilities 描述 PikPak 文件、上传、原生离线下载等 provider 能力。
func (d *PikPakDriver) Capabilities(context.Context, *entity.StorageSource) (domainstorage.StorageCapabilities, error) {
	return domainstorage.StorageCapabilities{
		CanList:           true,
		CanSearch:         true,
		CanDownload:       true,
		CanCapacity:       true,
		CanMkdir:          true,
		CanRename:         true,
		CanMove:           true,
		CanCopy:           true,
		CanDelete:         true,
		CanProviderTrash:  true,
		CanImportFile:     true,
		CanServerUpload:   true,
		CanDirectUpload:   true,
		CanNativeDownload: true,
	}, nil
}

// InitMultipartUpload 为已提供 GCID 的浏览器直传创建 PikPak OSS 上传计划。
func (d *PikPakDriver) InitMultipartUpload(ctx context.Context, source *entity.StorageSource, req domainstorage.MultipartUploadRequest) (*domainstorage.MultipartUploadPlan, error) {
	targetDirPath, err := normalizeVirtualPath(req.VirtualPath)
	if err != nil {
		return nil, err
	}
	if err := validatePikPakFileName(req.Filename); err != nil {
		return nil, err
	}
	uploadHash := normalizePikPakDirectUploadHash(req.ContentHash)
	if uploadHash == "" {
		return nil, domainstorage.ErrOperationUnsupported
	}
	if req.FileSize <= 0 {
		return nil, os.ErrInvalid
	}
	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = contentTypeForPikPakImport(req.Filename)
	}

	var plan *domainstorage.MultipartUploadPlan
	err = d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		parent, resolveErr := d.ensureFolderPathWithSession(ctx, source.ID, session, cfg, targetDirPath)
		if resolveErr != nil {
			return resolveErr
		}
		if err := d.ensureNoChildWithName(ctx, session, parent.ID, req.Filename); err != nil {
			return err
		}
		upload, createErr := d.sessions.client.CreateUploadFile(ctx, session, PikPakCreateUploadFileRequest{
			ParentID: parent.ID,
			Name:     req.Filename,
			Size:     req.FileSize,
			Hash:     uploadHash,
		})
		if createErr != nil {
			return createErr
		}
		targetPath := joinVirtualPath(targetDirPath, req.Filename)
		if upload == nil || upload.Resumable == nil {
			plan = &domainstorage.MultipartUploadPlan{
				CompletedEntry: &domainstorage.StorageEntry{
					Name:       req.Filename,
					Path:       targetPath,
					IsDir:      false,
					Size:       req.FileSize,
					ModifiedAt: time.Now(),
				},
			}
			d.invalidatePikPakPathCache(source.ID, cfg)
			return nil
		}
		instruction, instructionErr := d.pikPakDirectUploadInstruction(upload.Resumable.Params, contentType, req.FileSize)
		if instructionErr != nil {
			return instructionErr
		}
		plan = &domainstorage.MultipartUploadPlan{
			State: domainstorage.MultipartUploadState{
				RemoteUploadID: "pikpak_oss",
				ObjectKey:      upload.Resumable.Params.Key,
				VirtualPath:    targetPath,
				FileSize:       req.FileSize,
			},
			PartInstructions: []domainstorage.MultipartUploadPartInstruction{instruction},
		}
		return nil
	})
	return plan, err
}

// CompleteMultipartUpload 完成 PikPak 浏览器直传。PikPak OSS 上传无需额外合并请求。
func (d *PikPakDriver) CompleteMultipartUpload(ctx context.Context, source *entity.StorageSource, state domainstorage.MultipartUploadState, parts []domainstorage.CompletedUploadPart) (*domainstorage.StorageEntry, error) {
	if strings.TrimSpace(state.VirtualPath) == "" || state.FileSize <= 0 || len(parts) == 0 {
		return nil, os.ErrInvalid
	}
	virtualPath, err := normalizeVirtualPath(state.VirtualPath)
	if err != nil {
		return nil, err
	}
	err = d.sessions.withSession(ctx, source, func(_ PikPakSession, cfg PikPakConfig) error {
		d.invalidatePikPakPathCache(source.ID, cfg)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &domainstorage.StorageEntry{
		Name:       path.Base(virtualPath),
		Path:       virtualPath,
		IsDir:      false,
		Size:       state.FileSize,
		ModifiedAt: time.Now(),
	}, nil
}

// ImportFile 将后端本地 staging 文件导入 PikPak 目标路径。
func (d *PikPakDriver) ImportFile(ctx context.Context, source *entity.StorageSource, targetPath string, localPath string) error {
	targetPath, err := normalizeVirtualPath(targetPath)
	if err != nil {
		return err
	}
	if targetPath == "/" || strings.TrimSpace(localPath) == "" {
		return os.ErrInvalid
	}
	name := path.Base(targetPath)
	if err := validatePikPakFileName(name); err != nil {
		return err
	}
	parentPath := path.Dir(targetPath)
	if parentPath == "." {
		parentPath = "/"
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.ErrInvalid
	}

	hasher := d.uploadHasher
	if hasher == nil {
		hasher = PikPakGCIDUploadHashCalculator{}
	}
	return d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		defer d.invalidatePikPakPathCache(source.ID, cfg)
		parent, resolveErr := d.ensureFolderPathWithSession(ctx, source.ID, session, cfg, parentPath)
		if resolveErr != nil {
			return resolveErr
		}
		if err := d.ensureNoChildWithName(ctx, session, parent.ID, name); err != nil {
			return err
		}
		uploadHash, hashErr := hasher.HashFile(ctx, localPath)
		if hashErr != nil {
			return hashErr
		}
		uploadHash = strings.ToUpper(strings.TrimSpace(uploadHash))
		if uploadHash == "" {
			return domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider upload hash invalid")
		}
		upload, createErr := d.sessions.client.CreateUploadFile(ctx, session, PikPakCreateUploadFileRequest{
			ParentID: parent.ID,
			Name:     name,
			Size:     info.Size(),
			Hash:     uploadHash,
		})
		if createErr != nil {
			return createErr
		}
		if upload == nil || upload.Resumable == nil {
			return nil
		}
		if d.ossUploader == nil {
			return domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider upload unavailable")
		}
		return d.ossUploader.PutObject(ctx, upload.Resumable.Params, localPath, contentTypeForPikPakImport(targetPath))
	})
}

// CreateNativeDownload 直接在 PikPak 目标目录创建 provider 原生离线下载任务。
func (d *PikPakDriver) CreateNativeDownload(ctx context.Context, source *entity.StorageSource, req domainstorage.NativeDownloadRequest) (*domainstorage.NativeDownloadTask, error) {
	targetDirPath, err := normalizeVirtualPath(req.TargetDirPath)
	if err != nil {
		return nil, err
	}
	rawURL := strings.TrimSpace(req.URL)
	if rawURL == "" {
		return nil, domainstorage.ErrOperationUnsupported
	}
	targetFilename := strings.TrimSpace(req.TargetFilename)
	if targetFilename != "" {
		if err := validatePikPakFileName(targetFilename); err != nil {
			return nil, err
		}
	}

	var result *domainstorage.NativeDownloadTask
	err = d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		defer d.invalidatePikPakPathCache(source.ID, cfg)
		parent, resolveErr := d.ensureFolderPathWithSession(ctx, source.ID, session, cfg, targetDirPath)
		if resolveErr != nil {
			return resolveErr
		}
		if targetFilename != "" {
			if err := d.ensureNoChildWithName(ctx, session, parent.ID, targetFilename); err != nil {
				return err
			}
		}
		task, createErr := d.sessions.client.CreateOfflineDownload(ctx, session, PikPakCreateOfflineDownloadRequest{
			ParentID: parent.ID,
			URL:      rawURL,
			Name:     targetFilename,
		})
		if createErr != nil {
			return createErr
		}
		if task == nil || strings.TrimSpace(task.ID) == "" {
			return domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider task missing")
		}
		progress := pikPakOfflineProgressPercent(task.Progress)
		result = &domainstorage.NativeDownloadTask{
			ExternalID:      strings.TrimSpace(task.ID),
			DisplayName:     pikPakOfflineTaskDisplayName(task, ""),
			ProgressPercent: progress,
		}
		return nil
	})
	return result, err
}

// GetNativeDownloadStatus 查询 PikPak provider 原生离线下载任务状态。
func (d *PikPakDriver) GetNativeDownloadStatus(ctx context.Context, source *entity.StorageSource, externalID string) (*domainstorage.NativeDownloadStatus, error) {
	var status *domainstorage.NativeDownloadStatus
	err := d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		task, err := d.sessions.client.GetOfflineDownloadTask(ctx, session, externalID)
		if err != nil {
			return err
		}
		status = pikPakOfflineTaskToNativeStatus(task)
		if status.Status == "completed" {
			d.invalidatePikPakPathCache(source.ID, cfg)
		}
		return nil
	})
	return status, err
}

// CancelNativeDownload 删除 PikPak provider 离线任务记录。
func (d *PikPakDriver) CancelNativeDownload(ctx context.Context, source *entity.StorageSource, externalID string, deleteFiles bool) error {
	return d.sessions.withSession(ctx, source, func(session PikPakSession, _ PikPakConfig) error {
		return d.sessions.client.DeleteOfflineDownloadTasks(ctx, session, []string{externalID}, deleteFiles)
	})
}

// PauseNativeDownload 当前 PikPak provider 原生任务暂不暴露暂停能力。
func (d *PikPakDriver) PauseNativeDownload(context.Context, *entity.StorageSource, string) error {
	return domainstorage.ErrOperationUnsupported
}

// ResumeNativeDownload 当前 PikPak provider 原生任务暂不暴露恢复能力。
func (d *PikPakDriver) ResumeNativeDownload(context.Context, *entity.StorageSource, string) error {
	return domainstorage.ErrOperationUnsupported
}

func (d *PikPakDriver) ensureFolderPathWithSession(ctx context.Context, sourceID uint, session PikPakSession, cfg PikPakConfig, virtualPath string) (PikPakFile, error) {
	virtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return PikPakFile{}, err
	}
	if cached, ok := d.cachedPath(sourceID, cfg, virtualPath); ok {
		if !cached.isFolder() {
			return PikPakFile{}, os.ErrInvalid
		}
		return cached, nil
	}
	current := PikPakFile{
		ID:   cfg.providerRootID(),
		Name: "/",
		Kind: "drive#folder",
	}
	d.cachePath(sourceID, cfg, "/", current)
	if virtualPath == "/" {
		return current, nil
	}

	segments := strings.Split(strings.TrimPrefix(virtualPath, "/"), "/")
	parentPath := "/"
	for _, segment := range segments {
		if err := validatePikPakFileName(segment); err != nil {
			return PikPakFile{}, err
		}
		currentPath := joinVirtualPath(parentPath, segment)
		if cached, ok := d.cachedPath(sourceID, cfg, currentPath); ok {
			if !cached.isFolder() {
				return PikPakFile{}, os.ErrInvalid
			}
			current = cached
			parentPath = currentPath
			continue
		}
		files, listErr := d.listFilesAll(ctx, session, current.ID)
		if listErr != nil {
			return PikPakFile{}, listErr
		}
		d.cacheChildren(sourceID, cfg, parentPath, files)
		var found *PikPakFile
		for _, file := range files {
			if file.Name == segment {
				copied := file
				found = &copied
				break
			}
		}
		if found != nil {
			if !found.isFolder() {
				return PikPakFile{}, os.ErrInvalid
			}
			current = *found
			parentPath = currentPath
			continue
		}
		created, createErr := d.sessions.client.CreateFolder(ctx, session, current.ID, segment)
		if createErr != nil {
			return PikPakFile{}, createErr
		}
		if created == nil {
			created = &PikPakFile{Name: segment, Kind: "drive#folder"}
		}
		if created.ID == "" {
			return PikPakFile{}, domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider folder id missing")
		}
		if created.Name == "" {
			created.Name = segment
		}
		if created.Kind == "" {
			created.Kind = "drive#folder"
		}
		if !created.isFolder() {
			return PikPakFile{}, os.ErrInvalid
		}
		current = *created
		d.cachePath(sourceID, cfg, currentPath, current)
		parentPath = currentPath
	}
	return current, nil
}

// Mkdir 在 PikPak 中创建目录。
func (d *PikPakDriver) Mkdir(ctx context.Context, source *entity.StorageSource, parentPath string, name string) (*domainstorage.StorageEntry, error) {
	parentPath, err := normalizeVirtualPath(parentPath)
	if err != nil {
		return nil, err
	}
	if err := validatePikPakFileName(name); err != nil {
		return nil, err
	}

	var entry *domainstorage.StorageEntry
	err = d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		defer d.invalidatePikPakPathCache(source.ID, cfg)
		parent, resolveErr := d.resolvePathWithSession(ctx, source.ID, session, cfg, parentPath)
		if resolveErr != nil {
			return resolveErr
		}
		if !parent.isFolder() {
			return os.ErrInvalid
		}
		if err := d.ensureNoChildWithName(ctx, session, parent.ID, name); err != nil {
			return err
		}
		created, createErr := d.sessions.client.CreateFolder(ctx, session, parent.ID, name)
		if createErr != nil {
			return createErr
		}
		if created == nil {
			created = &PikPakFile{Name: name, Kind: "drive#folder"}
		}
		if created.Name == "" {
			created.Name = name
		}
		if created.Kind == "" {
			created.Kind = "drive#folder"
		}
		storageEntry := pikPakFileToStorageEntry(*created, joinVirtualPath(parentPath, name))
		entry = &storageEntry
		return nil
	})
	return entry, err
}

// Rename 重命名 PikPak 对象，根目录不可重命名。
func (d *PikPakDriver) Rename(ctx context.Context, source *entity.StorageSource, virtualPath string, newName string) (*domainstorage.StorageEntry, error) {
	virtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return nil, err
	}
	if virtualPath == "/" {
		return nil, os.ErrInvalid
	}
	if err := validatePikPakFileName(newName); err != nil {
		return nil, err
	}

	parentPath := path.Dir(virtualPath)
	if parentPath == "." {
		parentPath = "/"
	}
	newPath := joinVirtualPath(parentPath, newName)
	var entry *domainstorage.StorageEntry
	err = d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		defer d.invalidatePikPakPathCache(source.ID, cfg)
		file, resolveErr := d.resolvePathWithSession(ctx, source.ID, session, cfg, virtualPath)
		if resolveErr != nil {
			return resolveErr
		}
		parent, parentErr := d.resolvePathWithSession(ctx, source.ID, session, cfg, parentPath)
		if parentErr != nil {
			return parentErr
		}
		if !parent.isFolder() {
			return os.ErrInvalid
		}
		if err := d.ensureNoChildWithName(ctx, session, parent.ID, newName); err != nil {
			return err
		}
		renamed, renameErr := d.sessions.client.RenameFile(ctx, session, file.ID, newName)
		if renameErr != nil {
			return renameErr
		}
		if renamed == nil {
			renamed = &file
		}
		renamed.Name = newName
		if renamed.Kind == "" {
			renamed.Kind = file.Kind
		}
		storageEntry := pikPakFileToStorageEntry(*renamed, newPath)
		entry = &storageEntry
		return nil
	})
	return entry, err
}

// Move 将 PikPak 对象移动到目标目录，根目录不可移动。
func (d *PikPakDriver) Move(ctx context.Context, source *entity.StorageSource, virtualPath string, targetPath string) error {
	return d.moveOrCopy(ctx, source, virtualPath, targetPath, true)
}

// Copy 将 PikPak 对象复制到目标目录，根目录不可复制。
func (d *PikPakDriver) Copy(ctx context.Context, source *entity.StorageSource, virtualPath string, targetPath string) error {
	return d.moveOrCopy(ctx, source, virtualPath, targetPath, false)
}

// Delete 将 PikPak 对象移入 provider 回收站，根目录不可删除。
func (d *PikPakDriver) Delete(ctx context.Context, source *entity.StorageSource, virtualPath string) error {
	virtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return err
	}
	if virtualPath == "/" {
		return os.ErrInvalid
	}
	return d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		defer d.invalidatePikPakPathCache(source.ID, cfg)
		file, resolveErr := d.resolvePathWithSession(ctx, source.ID, session, cfg, virtualPath)
		if resolveErr != nil {
			return resolveErr
		}
		return d.sessions.client.BatchTrash(ctx, session, []string{file.ID})
	})
}

func (d *PikPakDriver) moveOrCopy(ctx context.Context, source *entity.StorageSource, virtualPath string, targetPath string, removeSource bool) error {
	virtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return err
	}
	targetPath, err = normalizeVirtualPath(targetPath)
	if err != nil {
		return err
	}
	if virtualPath == "/" {
		return os.ErrInvalid
	}
	return d.sessions.withSession(ctx, source, func(session PikPakSession, cfg PikPakConfig) error {
		defer d.invalidatePikPakPathCache(source.ID, cfg)
		file, resolveErr := d.resolvePathWithSession(ctx, source.ID, session, cfg, virtualPath)
		if resolveErr != nil {
			return resolveErr
		}
		target, targetErr := d.resolvePathWithSession(ctx, source.ID, session, cfg, targetPath)
		if targetErr != nil {
			return targetErr
		}
		if !target.isFolder() {
			return os.ErrInvalid
		}
		if file.isFolder() && isPikPakSelfOrDescendantTarget(virtualPath, targetPath) {
			return os.ErrInvalid
		}
		if err := d.ensureNoChildWithName(ctx, session, target.ID, file.Name); err != nil {
			return err
		}
		if removeSource {
			return d.sessions.client.BatchMove(ctx, session, []string{file.ID}, target.ID)
		}
		return d.sessions.client.BatchCopy(ctx, session, []string{file.ID}, target.ID)
	})
}

func (d *PikPakDriver) ensureNoChildWithName(ctx context.Context, session PikPakSession, parentID string, name string) error {
	files, err := d.listFilesAll(ctx, session, parentID)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.Name == name {
			return fs.ErrExist
		}
	}
	return nil
}

func isPikPakSelfOrDescendantTarget(sourcePath string, targetPath string) bool {
	if sourcePath == "/" {
		return true
	}
	return targetPath == sourcePath || strings.HasPrefix(targetPath, sourcePath+"/")
}

func validatePikPakFileName(name string) error {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || name == "." || name == ".." {
		return os.ErrInvalid
	}
	return nil
}

func (d *PikPakDriver) resolvePathWithSession(ctx context.Context, sourceID uint, session PikPakSession, cfg PikPakConfig, virtualPath string) (PikPakFile, error) {
	virtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return PikPakFile{}, err
	}
	if cached, ok := d.cachedPath(sourceID, cfg, virtualPath); ok {
		return cached, nil
	}
	current := PikPakFile{
		ID:   cfg.providerRootID(),
		Name: "/",
		Kind: "drive#folder",
	}
	d.cachePath(sourceID, cfg, "/", current)
	if virtualPath == "/" {
		return current, nil
	}

	segments := strings.Split(strings.TrimPrefix(virtualPath, "/"), "/")
	parentPath := "/"
	for index, segment := range segments {
		currentPath := joinVirtualPath(parentPath, segment)
		if cached, ok := d.cachedPath(sourceID, cfg, currentPath); ok {
			current = cached
			if index < len(segments)-1 && !current.isFolder() {
				return PikPakFile{}, os.ErrNotExist
			}
			parentPath = currentPath
			continue
		}
		files, listErr := d.listFilesAll(ctx, session, current.ID)
		if listErr != nil {
			return PikPakFile{}, listErr
		}
		d.cacheChildren(sourceID, cfg, parentPath, files)
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
		parentPath = currentPath
	}
	return current, nil
}

func (d *PikPakDriver) cachedPath(sourceID uint, cfg PikPakConfig, virtualPath string) (PikPakFile, bool) {
	if d == nil || d.pathCache == nil {
		return PikPakFile{}, false
	}
	return d.pathCache.get(sourceID, cfg.providerRootID(), virtualPath)
}

func (d *PikPakDriver) cachePath(sourceID uint, cfg PikPakConfig, virtualPath string, file PikPakFile) {
	if d == nil || d.pathCache == nil {
		return
	}
	d.pathCache.set(sourceID, cfg.providerRootID(), virtualPath, file, time.Duration(cfg.CacheTTLSeconds)*time.Second)
}

func (d *PikPakDriver) cacheChildren(sourceID uint, cfg PikPakConfig, parentPath string, files []PikPakFile) {
	for _, file := range files {
		if file.Name == "" {
			continue
		}
		d.cachePath(sourceID, cfg, joinVirtualPath(parentPath, file.Name), file)
	}
}

func (d *PikPakDriver) invalidatePikPakPathCache(sourceID uint, cfg PikPakConfig) {
	if d == nil || d.pathCache == nil {
		return
	}
	d.pathCache.clearSource(sourceID, cfg.providerRootID())
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

func (d *PikPakDriver) pikPakDirectUploadInstruction(params PikPakOSSUploadParams, contentType string, fileSize int64) (domainstorage.MultipartUploadPartInstruction, error) {
	if err := validatePikPakOSSParams(params); err != nil {
		return domainstorage.MultipartUploadPartInstruction{}, err
	}
	now := time.Now
	if d != nil && d.ossInstructionNow != nil {
		now = d.ossInstructionNow
	}
	signedAt := now().UTC()
	objectURL, canonicalResource, err := buildPikPakOSSObjectURL(params)
	if err != nil {
		return domainstorage.MultipartUploadPartInstruction{}, err
	}
	headers := http.Header{}
	headers.Set("Content-Type", contentType)
	headers.Set("Date", signedAt.Format(http.TimeFormat))
	if params.SecurityToken != "" {
		headers.Set("X-OSS-Security-Token", params.SecurityToken)
	}
	headers.Set("Authorization", buildPikPakOSSPutAuthorization(params, headers, canonicalResource))
	return domainstorage.MultipartUploadPartInstruction{
		Index:     0,
		Method:    http.MethodPut,
		URL:       objectURL,
		Headers:   flattenPikPakUploadHeaders(headers),
		ByteStart: 0,
		ByteEnd:   fileSize - 1,
		ExpiresAt: pikPakOSSInstructionExpiresAt(params.Expiration, signedAt),
	}, nil
}

func flattenPikPakUploadHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

func pikPakOSSInstructionExpiresAt(raw string, fallbackFrom time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			return parsed
		}
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return parsed
		}
	}
	return fallbackFrom.Add(15 * time.Minute)
}

func normalizePikPakDirectUploadHash(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "GCID:")
	if len(value) != 40 {
		return ""
	}
	for _, r := range value {
		if r >= '0' && r <= '9' || r >= 'A' && r <= 'F' {
			continue
		}
		return ""
	}
	return value
}

func pikPakOfflineTaskToNativeStatus(task *PikPakOfflineTask) *domainstorage.NativeDownloadStatus {
	if task == nil {
		return &domainstorage.NativeDownloadStatus{Status: "pending"}
	}
	progress := pikPakOfflineProgressPercent(task.Progress)
	status := &domainstorage.NativeDownloadStatus{
		Status:          mapPikPakOfflinePhase(task.Phase),
		ProgressPercent: progress,
		DisplayName:     pikPakOfflineTaskDisplayName(task, ""),
		ErrorMessage:    pikPakOfflineTaskSafeErrorMessage(task),
	}
	if status.Status == "completed" {
		completed := float64(100)
		status.ProgressPercent = &completed
	}
	return status
}

func pikPakOfflineTaskSafeErrorMessage(task *PikPakOfflineTask) *string {
	if task == nil || strings.TrimSpace(task.Message) == "" {
		return nil
	}
	message := "cloud provider task failed"
	return &message
}

func mapPikPakOfflinePhase(phase string) string {
	switch strings.ToUpper(strings.TrimSpace(phase)) {
	case "PHASE_TYPE_PENDING":
		return "pending"
	case "PHASE_TYPE_RUNNING":
		return "running"
	case "PHASE_TYPE_COMPLETE":
		return "completed"
	case "PHASE_TYPE_ERROR":
		return "failed"
	case "PHASE_TYPE_CANCEL", "PHASE_TYPE_CANCELED", "PHASE_TYPE_CANCELLED":
		return "canceled"
	default:
		return "running"
	}
}

func pikPakOfflineProgressPercent(progress float64) *float64 {
	if progress < 0 {
		progress = 0
	}
	if progress <= 1 {
		progress *= 100
	}
	if progress > 100 {
		progress = 100
	}
	return &progress
}

func pikPakOfflineTaskDisplayName(task *PikPakOfflineTask, fallbackURL string) string {
	if task == nil {
		return pikPakOfflineFallbackURLFileName(fallbackURL)
	}
	for _, candidate := range []string{
		task.Name,
		pikPakOfflineFileName(task.File),
		pikPakOfflineFileName(task.ReferenceResource),
	} {
		if filename := pikPakOfflineSafeDisplayName(candidate); filename != "" {
			return filename
		}
	}
	return pikPakOfflineFallbackURLFileName(fallbackURL)
}

func pikPakOfflineFallbackURLFileName(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return ""
	}
	return pikPakOfflineSafeDisplayName(path.Base(parsed.Path))
}

func pikPakOfflineSafeDisplayName(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || candidate == "." || candidate == "/" {
		return ""
	}
	lower := strings.ToLower(candidate)
	if strings.Contains(candidate, "://") || strings.HasPrefix(lower, "magnet:") {
		return ""
	}
	if validatePikPakFileName(candidate) != nil {
		return ""
	}
	return candidate
}

func pikPakOfflineFileName(file *PikPakFile) string {
	if file == nil {
		return ""
	}
	return file.Name
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
