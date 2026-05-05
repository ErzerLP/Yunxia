package storage

import (
	"context"
	"errors"
	"io/fs"
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
	sessions         *PikPakSessionManager
	searchMaxDepth   int
	searchMaxEntries int
	uploadHasher     PikPakUploadHashCalculator
	ossUploader      PikPakOSSUploader
}

// PikPakDriverOption 定义 PikPak driver 可选配置。
type PikPakDriverOption func(*PikPakDriver)

// NewPikPakDriver 创建 PikPak driver。
func NewPikPakDriver(options ...PikPakDriverOption) *PikPakDriver {
	driver := &PikPakDriver{
		sessions:         NewPikPakSessionManager(nil),
		searchMaxDepth:   defaultPikPakSearchMaxDepth,
		searchMaxEntries: defaultPikPakSearchMaxEntries,
		uploadHasher:     PikPakGCIDUploadHashCalculator{},
		ossUploader:      NewPikPakHTTPOSSUploader(),
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

// Capabilities 描述 PikPak 阶段 D 文件写入与后端 staging 上传导入能力。
func (d *PikPakDriver) Capabilities(context.Context, *entity.StorageSource) (domainstorage.StorageCapabilities, error) {
	return domainstorage.StorageCapabilities{
		CanList:          true,
		CanSearch:        true,
		CanDownload:      true,
		CanCapacity:      true,
		CanMkdir:         true,
		CanRename:        true,
		CanMove:          true,
		CanCopy:          true,
		CanDelete:        true,
		CanProviderTrash: true,
		CanImportFile:    true,
		CanServerUpload:  true,
		CanDirectUpload:  false,
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
		parent, resolveErr := d.ensureFolderPathWithSession(ctx, session, cfg, parentPath)
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

func (d *PikPakDriver) ensureFolderPathWithSession(ctx context.Context, session PikPakSession, cfg PikPakConfig, virtualPath string) (PikPakFile, error) {
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
	for _, segment := range segments {
		if err := validatePikPakFileName(segment); err != nil {
			return PikPakFile{}, err
		}
		files, listErr := d.listFilesAll(ctx, session, current.ID)
		if listErr != nil {
			return PikPakFile{}, listErr
		}
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
		parent, resolveErr := d.resolvePathWithSession(ctx, session, cfg, parentPath)
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
		file, resolveErr := d.resolvePathWithSession(ctx, session, cfg, virtualPath)
		if resolveErr != nil {
			return resolveErr
		}
		parent, parentErr := d.resolvePathWithSession(ctx, session, cfg, parentPath)
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
		file, resolveErr := d.resolvePathWithSession(ctx, session, cfg, virtualPath)
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
		file, resolveErr := d.resolvePathWithSession(ctx, session, cfg, virtualPath)
		if resolveErr != nil {
			return resolveErr
		}
		target, targetErr := d.resolvePathWithSession(ctx, session, cfg, targetPath)
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
