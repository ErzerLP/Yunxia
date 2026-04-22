package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

// FileService 负责文件管理与访问地址生成。
type FileService struct {
	sourceRepo       domainrepo.SourceRepository
	aclAuthorizer    *ACLAuthorizer
	fileAccessTokens interface {
		Issue(sourceID uint, path, purpose, disposition string, ttl time.Duration) (string, time.Time, error)
		Validate(raw string) (*security.FileAccessClaims, error)
	}
	authTokens interface {
		ValidateAccessToken(token string) (*security.Claims, error)
	}
	userRepo      domainrepo.UserRepository
	fileDrivers   map[string]FileDriver
	trashItemRepo domainrepo.TrashItemRepository
}

// NewFileService 创建文件服务。
func NewFileService(
	sourceRepo domainrepo.SourceRepository,
	fileAccessTokens interface {
		Issue(sourceID uint, path, purpose, disposition string, ttl time.Duration) (string, time.Time, error)
		Validate(raw string) (*security.FileAccessClaims, error)
	},
	authTokens interface {
		ValidateAccessToken(token string) (*security.Claims, error)
	},
	userRepo domainrepo.UserRepository,
	options ...FileServiceOption,
) *FileService {
	service := &FileService{
		sourceRepo:       sourceRepo,
		fileAccessTokens: fileAccessTokens,
		authTokens:       authTokens,
		userRepo:         userRepo,
		fileDrivers:      make(map[string]FileDriver),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// List 返回目录列表。
func (s *FileService) List(ctx context.Context, query appdto.FileListQuery) (*appdto.FileListResponse, int, int, int, int, error) {
	source, err := s.getSource(ctx, query.SourceID)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	if source.DriverType != "local" {
		return s.listWithDriver(ctx, source, query)
	}

	virtualPath, err := normalizeVirtualPath(query.Path)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	_, physicalPath, err := resolvePhysicalPath(source, virtualPath)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}

	entries, err := os.ReadDir(physicalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, 0, 0, 0, ErrFileNotFound
		}
		return nil, 0, 0, 0, 0, err
	}

	items := make([]appdto.FileItem, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".trash") || strings.HasPrefix(entry.Name(), ".system") {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, 0, 0, 0, 0, infoErr
		}
		itemPath := path.Join(virtualPath, entry.Name())
		if virtualPath == "/" {
			itemPath = "/" + entry.Name()
		}
		items = append(items, buildFileItem(source.ID, itemPath, info))
	}

	sortFileItems(items, query.SortBy, query.SortOrder)
	items, err = s.filterReadableFileItems(ctx, source.ID, items)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	pageItems, total, totalPages := paginateItems(items, query.Page, query.PageSize)

	return &appdto.FileListResponse{
		Items:           pageItems,
		CurrentPath:     virtualPath,
		CurrentSourceID: source.ID,
	}, pageValue(query.Page), pageSizeValue(query.PageSize), total, totalPages, nil
}

// Search 按文件名搜索。
func (s *FileService) Search(ctx context.Context, query appdto.FileSearchQuery) (*appdto.FileSearchResponse, int, int, int, int, error) {
	source, err := s.getSource(ctx, query.SourceID)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	if source.DriverType != "local" {
		return s.searchWithDriver(ctx, source, query)
	}

	pathPrefix := "/"
	if query.PathPrefix != "" {
		pathPrefix, err = normalizeVirtualPath(query.PathPrefix)
		if err != nil {
			return nil, 0, 0, 0, 0, err
		}
	}
	_, basePath, err := resolvePhysicalPath(source, pathPrefix)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}

	lowerKeyword := strings.ToLower(query.Keyword)
	items := make([]appdto.FileItem, 0)
	err = filepath.WalkDir(basePath, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == basePath {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".trash") || strings.HasPrefix(name, ".system") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.Contains(strings.ToLower(name), lowerKeyword) {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		relative, relErr := filepath.Rel(basePath, current)
		if relErr != nil {
			return relErr
		}
		virtualPath := path.Join(pathPrefix, filepath.ToSlash(relative))
		if pathPrefix == "/" {
			virtualPath = "/" + filepath.ToSlash(relative)
		}
		items = append(items, buildFileItem(source.ID, virtualPath, info))
		return nil
	})
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}

	sortFileItems(items, "name", "asc")
	items, err = s.filterReadableFileItems(ctx, source.ID, items)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	pageItems, total, totalPages := paginateItems(items, query.Page, query.PageSize)

	var prefixPtr *string
	if query.PathPrefix != "" {
		prefixPtr = &pathPrefix
	}

	return &appdto.FileSearchResponse{
		Items:           pageItems,
		Keyword:         query.Keyword,
		CurrentSourceID: source.ID,
		PathPrefix:      prefixPtr,
	}, pageValue(query.Page), pageSizeValue(query.PageSize), total, totalPages, nil
}

// Mkdir 创建目录。
func (s *FileService) Mkdir(ctx context.Context, req appdto.MkdirRequest) (*appdto.FileItem, error) {
	source, err := s.getSource(ctx, req.SourceID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizePath(ctx, source.ID, req.ParentPath, ACLActionWrite); err != nil {
		return nil, err
	}
	if source.DriverType != "local" {
		return s.mkdirWithDriver(ctx, source, req)
	}

	return s.mkdirLocal(source, req)
}

func (s *FileService) mkdirLocal(source *entity.StorageSource, req appdto.MkdirRequest) (*appdto.FileItem, error) {
	if err := validateFileName(req.Name); err != nil {
		return nil, err
	}
	parentPath, err := normalizeVirtualPath(req.ParentPath)
	if err != nil {
		return nil, err
	}
	_, parentPhysical, err := resolvePhysicalPath(source, parentPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(parentPhysical)
	if err != nil {
		return nil, ErrFileNotFound
	}
	if !info.IsDir() {
		return nil, ErrPathInvalid
	}

	targetVirtual := path.Join(parentPath, req.Name)
	if parentPath == "/" {
		targetVirtual = "/" + req.Name
	}
	_, targetPhysical, err := resolvePhysicalPath(source, targetVirtual)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(targetPhysical); statErr == nil {
		return nil, ErrFileAlreadyExists
	}
	if err := os.Mkdir(targetPhysical, 0o755); err != nil {
		return nil, err
	}
	targetInfo, _ := os.Stat(targetPhysical)
	item := buildFileItem(source.ID, targetVirtual, targetInfo)
	return &item, nil
}

func (s *FileService) mkdirWithDriver(ctx context.Context, source *entity.StorageSource, req appdto.MkdirRequest) (*appdto.FileItem, error) {
	driver, err := s.getFileDriver(source.DriverType)
	if err != nil {
		return nil, err
	}
	if err := validateFileName(req.Name); err != nil {
		return nil, err
	}
	parentPath, err := normalizeVirtualPath(req.ParentPath)
	if err != nil {
		return nil, err
	}
	entry, err := driver.Mkdir(ctx, source, parentPath, req.Name)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return nil, ErrFileNotFound
		case errors.Is(err, fs.ErrExist):
			return nil, ErrFileAlreadyExists
		case errors.Is(err, os.ErrInvalid):
			return nil, ErrPathInvalid
		default:
			return nil, err
		}
	}
	item := buildStorageEntryItem(source.ID, *entry)
	return &item, nil
}

// Rename 重命名文件或目录。
func (s *FileService) Rename(ctx context.Context, req appdto.RenameRequest) (string, string, *appdto.FileItem, error) {
	source, err := s.getSource(ctx, req.SourceID)
	if err != nil {
		return "", "", nil, err
	}
	if err := s.authorizePath(ctx, source.ID, req.Path, ACLActionWrite); err != nil {
		return "", "", nil, err
	}
	if source.DriverType != "local" {
		return s.renameWithDriver(ctx, source, req)
	}

	return s.renameLocal(source, req)
}

func (s *FileService) renameLocal(source *entity.StorageSource, req appdto.RenameRequest) (string, string, *appdto.FileItem, error) {
	if err := validateFileName(req.NewName); err != nil {
		return "", "", nil, err
	}
	virtualPath, err := normalizeVirtualPath(req.Path)
	if err != nil {
		return "", "", nil, err
	}
	_, physicalPath, err := resolvePhysicalPath(source, virtualPath)
	if err != nil {
		return "", "", nil, err
	}
	if _, statErr := os.Stat(physicalPath); statErr != nil {
		return "", "", nil, ErrFileNotFound
	}

	parentVirtual := path.Dir(virtualPath)
	if parentVirtual == "." {
		parentVirtual = "/"
	}
	newVirtual := path.Join(parentVirtual, req.NewName)
	if parentVirtual == "/" {
		newVirtual = "/" + req.NewName
	}
	_, newPhysical, err := resolvePhysicalPath(source, newVirtual)
	if err != nil {
		return "", "", nil, err
	}
	if _, statErr := os.Stat(newPhysical); statErr == nil {
		return "", "", nil, ErrFileAlreadyExists
	}
	if err := os.Rename(physicalPath, newPhysical); err != nil {
		return "", "", nil, err
	}

	info, _ := os.Stat(newPhysical)
	item := buildFileItem(source.ID, newVirtual, info)
	return virtualPath, newVirtual, &item, nil
}

// Move 移动文件或目录。
func (s *FileService) Move(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error) {
	source, err := s.getSource(ctx, req.SourceID)
	if err != nil {
		return "", "", err
	}
	if err := s.authorizePath(ctx, source.ID, req.Path, ACLActionWrite); err != nil {
		return "", "", err
	}
	if err := s.authorizePath(ctx, source.ID, req.TargetPath, ACLActionWrite); err != nil {
		return "", "", err
	}
	if source.DriverType != "local" {
		return s.moveWithDriver(ctx, source, req)
	}

	return s.moveLocal(source, req)
}

func (s *FileService) moveLocal(source *entity.StorageSource, req appdto.MoveCopyRequest) (string, string, error) {
	virtualPath, err := normalizeVirtualPath(req.Path)
	if err != nil {
		return "", "", err
	}
	targetPath, err := normalizeVirtualPath(req.TargetPath)
	if err != nil {
		return "", "", err
	}
	_, physicalPath, err := resolvePhysicalPath(source, virtualPath)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(physicalPath)
	if err != nil {
		return "", "", ErrFileNotFound
	}

	_, targetPhysicalDir, err := resolvePhysicalPath(source, targetPath)
	if err != nil {
		return "", "", err
	}
	targetInfo, err := os.Stat(targetPhysicalDir)
	if err != nil || !targetInfo.IsDir() {
		return "", "", ErrPathInvalid
	}

	newVirtual := path.Join(targetPath, info.Name())
	if targetPath == "/" {
		newVirtual = "/" + info.Name()
	}
	_, newPhysical, err := resolvePhysicalPath(source, newVirtual)
	if err != nil {
		return "", "", err
	}
	if _, statErr := os.Stat(newPhysical); statErr == nil {
		return "", "", ErrFileMoveConflict
	}
	if err := os.Rename(physicalPath, newPhysical); err != nil {
		return "", "", err
	}

	return virtualPath, newVirtual, nil
}

// Copy 复制文件或目录。
func (s *FileService) Copy(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error) {
	source, err := s.getSource(ctx, req.SourceID)
	if err != nil {
		return "", "", err
	}
	if err := s.authorizePath(ctx, source.ID, req.Path, ACLActionRead); err != nil {
		return "", "", err
	}
	if err := s.authorizePath(ctx, source.ID, req.TargetPath, ACLActionWrite); err != nil {
		return "", "", err
	}
	if source.DriverType != "local" {
		return s.copyWithDriver(ctx, source, req)
	}

	return s.copyLocal(source, req)
}

func (s *FileService) copyLocal(source *entity.StorageSource, req appdto.MoveCopyRequest) (string, string, error) {
	virtualPath, err := normalizeVirtualPath(req.Path)
	if err != nil {
		return "", "", err
	}
	targetPath, err := normalizeVirtualPath(req.TargetPath)
	if err != nil {
		return "", "", err
	}
	_, physicalPath, err := resolvePhysicalPath(source, virtualPath)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(physicalPath)
	if err != nil {
		return "", "", ErrFileNotFound
	}

	_, targetPhysicalDir, err := resolvePhysicalPath(source, targetPath)
	if err != nil {
		return "", "", err
	}
	targetInfo, err := os.Stat(targetPhysicalDir)
	if err != nil || !targetInfo.IsDir() {
		return "", "", ErrPathInvalid
	}

	newVirtual := path.Join(targetPath, info.Name())
	if targetPath == "/" {
		newVirtual = "/" + info.Name()
	}
	_, newPhysical, err := resolvePhysicalPath(source, newVirtual)
	if err != nil {
		return "", "", err
	}
	if _, statErr := os.Stat(newPhysical); statErr == nil {
		return "", "", ErrFileCopyConflict
	}

	if info.IsDir() {
		if err := copyDirectory(physicalPath, newPhysical); err != nil {
			return "", "", err
		}
	} else {
		if err := copyFile(physicalPath, newPhysical); err != nil {
			return "", "", err
		}
	}

	return virtualPath, newVirtual, nil
}

// Delete 删除文件或目录。
func (s *FileService) Delete(ctx context.Context, req appdto.DeleteFileRequest) (time.Time, error) {
	source, err := s.getSource(ctx, req.SourceID)
	if err != nil {
		return time.Time{}, err
	}
	if err := s.authorizePath(ctx, source.ID, req.Path, ACLActionDelete); err != nil {
		return time.Time{}, err
	}
	if source.DriverType != "local" {
		return s.deleteWithDriver(ctx, source, req)
	}

	return s.deleteLocal(ctx, source, req)
}

func (s *FileService) deleteLocal(ctx context.Context, source *entity.StorageSource, req appdto.DeleteFileRequest) (time.Time, error) {
	virtualPath, err := normalizeVirtualPath(req.Path)
	if err != nil {
		return time.Time{}, err
	}
	_, physicalPath, err := resolvePhysicalPath(source, virtualPath)
	if err != nil {
		return time.Time{}, err
	}
	info, statErr := os.Stat(physicalPath)
	if statErr != nil {
		return time.Time{}, ErrFileNotFound
	}

	if req.DeleteMode == "" {
		req.DeleteMode = "trash"
	}
	if req.DeleteMode == "permanent" {
		if err := os.RemoveAll(physicalPath); err != nil {
			return time.Time{}, err
		}
		return time.Now(), nil
	}

	deletedAt := time.Now()
	size, err := localEntrySize(physicalPath, info)
	if err != nil {
		return time.Time{}, err
	}
	_, trashVirtual := buildTrashPaths(virtualPath, deletedAt)
	_, trashPhysical, err := resolvePhysicalPath(source, trashVirtual)
	if err != nil {
		return time.Time{}, err
	}
	if err := os.MkdirAll(filepath.Dir(trashPhysical), 0o755); err != nil {
		return time.Time{}, err
	}
	if err := os.Rename(physicalPath, trashPhysical); err != nil {
		return time.Time{}, err
	}
	if err := s.recordTrashItem(ctx, source.ID, virtualPath, trashVirtual, info.Name(), info.IsDir(), size, deletedAt); err != nil {
		_ = os.MkdirAll(filepath.Dir(physicalPath), 0o755)
		_ = os.Rename(trashPhysical, physicalPath)
		return time.Time{}, err
	}
	return deletedAt, nil
}

func (s *FileService) renameWithDriver(ctx context.Context, source *entity.StorageSource, req appdto.RenameRequest) (string, string, *appdto.FileItem, error) {
	driver, err := s.getFileDriver(source.DriverType)
	if err != nil {
		return "", "", nil, err
	}
	if err := validateFileName(req.NewName); err != nil {
		return "", "", nil, err
	}
	virtualPath, err := normalizeVirtualPath(req.Path)
	if err != nil {
		return "", "", nil, err
	}
	entry, err := driver.Rename(ctx, source, virtualPath, req.NewName)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return "", "", nil, ErrFileNotFound
		case errors.Is(err, fs.ErrExist):
			return "", "", nil, ErrFileAlreadyExists
		case errors.Is(err, os.ErrInvalid):
			return "", "", nil, ErrPathInvalid
		default:
			return "", "", nil, err
		}
	}

	parentVirtual := path.Dir(virtualPath)
	if parentVirtual == "." {
		parentVirtual = "/"
	}
	newVirtual := path.Join(parentVirtual, req.NewName)
	if parentVirtual == "/" {
		newVirtual = "/" + req.NewName
	}
	item := buildStorageEntryItem(source.ID, *entry)
	return virtualPath, newVirtual, &item, nil
}

func (s *FileService) moveWithDriver(ctx context.Context, source *entity.StorageSource, req appdto.MoveCopyRequest) (string, string, error) {
	driver, err := s.getFileDriver(source.DriverType)
	if err != nil {
		return "", "", err
	}
	virtualPath, err := normalizeVirtualPath(req.Path)
	if err != nil {
		return "", "", err
	}
	targetPath, err := normalizeVirtualPath(req.TargetPath)
	if err != nil {
		return "", "", err
	}
	if err := driver.Move(ctx, source, virtualPath, targetPath); err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return "", "", ErrFileNotFound
		case errors.Is(err, fs.ErrExist):
			return "", "", ErrFileMoveConflict
		case errors.Is(err, os.ErrInvalid):
			return "", "", ErrPathInvalid
		default:
			return "", "", err
		}
	}

	newVirtual := path.Join(targetPath, path.Base(virtualPath))
	if targetPath == "/" {
		newVirtual = "/" + path.Base(virtualPath)
	}
	return virtualPath, newVirtual, nil
}

func (s *FileService) copyWithDriver(ctx context.Context, source *entity.StorageSource, req appdto.MoveCopyRequest) (string, string, error) {
	driver, err := s.getFileDriver(source.DriverType)
	if err != nil {
		return "", "", err
	}
	virtualPath, err := normalizeVirtualPath(req.Path)
	if err != nil {
		return "", "", err
	}
	targetPath, err := normalizeVirtualPath(req.TargetPath)
	if err != nil {
		return "", "", err
	}
	if err := driver.Copy(ctx, source, virtualPath, targetPath); err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return "", "", ErrFileNotFound
		case errors.Is(err, fs.ErrExist):
			return "", "", ErrFileCopyConflict
		case errors.Is(err, os.ErrInvalid):
			return "", "", ErrPathInvalid
		default:
			return "", "", err
		}
	}

	newVirtual := path.Join(targetPath, path.Base(virtualPath))
	if targetPath == "/" {
		newVirtual = "/" + path.Base(virtualPath)
	}
	return virtualPath, newVirtual, nil
}

// AccessURL 生成短时文件访问地址。
func (s *FileService) AccessURL(ctx context.Context, req appdto.AccessURLRequest) (*appdto.AccessURLResponse, error) {
	source, err := s.getSource(ctx, req.SourceID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizePath(ctx, source.ID, req.Path, ACLActionRead); err != nil {
		return nil, err
	}
	if source.DriverType != "local" {
		return s.accessURLWithDriver(ctx, source, req)
	}

	virtualPath, err := normalizeVirtualPath(req.Path)
	if err != nil {
		return nil, err
	}
	_, physicalPath, err := resolvePhysicalPath(source, virtualPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(physicalPath)
	if err != nil {
		return nil, ErrFileNotFound
	}
	if info.IsDir() {
		return nil, ErrFileIsDirectory
	}
	if req.ExpiresIn <= 0 {
		req.ExpiresIn = 300
	}

	token, expiresAt, err := s.fileAccessTokens.Issue(source.ID, virtualPath, req.Purpose, req.Disposition, time.Duration(req.ExpiresIn)*time.Second)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("source_id", fmt.Sprintf("%d", source.ID))
	params.Set("path", virtualPath)
	params.Set("disposition", req.Disposition)
	params.Set("access_token", token)

	return &appdto.AccessURLResponse{
		URL:       "/api/v1/files/download?" + params.Encode(),
		Method:    "GET",
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}, nil
}

// ResolveDownload 解析下载请求并返回文件。
func (s *FileService) ResolveDownload(ctx context.Context, sourceID uint, filePath string) (*os.File, os.FileInfo, string, error) {
	source, err := s.getSource(ctx, sourceID)
	if err != nil {
		return nil, nil, "", err
	}
	if err := s.authorizePath(ctx, source.ID, filePath, ACLActionRead); err != nil {
		return nil, nil, "", err
	}
	if source.DriverType != "local" {
		return nil, nil, "", ErrSourceDriverUnsupported
	}

	virtualPath, err := normalizeVirtualPath(filePath)
	if err != nil {
		return nil, nil, "", err
	}
	_, physicalPath, err := resolvePhysicalPath(source, virtualPath)
	if err != nil {
		return nil, nil, "", err
	}

	file, err := os.Open(physicalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, "", ErrFileNotFound
		}
		return nil, nil, "", err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, "", err
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, nil, "", ErrFileIsDirectory
	}

	mimeType := mimeFromName(info.Name())
	return file, info, mimeType, nil
}

// ResolveDownloadRedirect 返回非 local 驱动的下载跳转地址。
func (s *FileService) ResolveDownloadRedirect(ctx context.Context, sourceID uint, filePath, disposition string) (string, error) {
	source, err := s.getSource(ctx, sourceID)
	if err != nil {
		return "", err
	}
	if err := s.authorizePath(ctx, source.ID, filePath, ACLActionRead); err != nil {
		return "", err
	}
	if source.DriverType == "local" {
		return "", nil
	}

	driver, err := s.getFileDriver(source.DriverType)
	if err != nil {
		return "", err
	}
	virtualPath, err := normalizeVirtualPath(filePath)
	if err != nil {
		return "", err
	}
	if disposition == "" {
		disposition = "attachment"
	}

	redirectURL, _, err := driver.PresignDownload(ctx, source, virtualPath, disposition, 5*time.Minute)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrFileNotFound
		}
		return "", err
	}
	return redirectURL, nil
}

// ValidateFileAccessToken 校验短时文件访问令牌。
func (s *FileService) ValidateFileAccessToken(raw string) (*security.FileAccessClaims, error) {
	return s.fileAccessTokens.Validate(raw)
}

// AuthenticateBearerToken 校验下载请求携带的 Bearer token 并返回身份。
func (s *FileService) AuthenticateBearerToken(ctx context.Context, raw string) (*security.RequestAuth, error) {
	if raw == "" {
		return nil, ErrInvalidCredentials
	}
	token := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
	claims, err := s.authTokens.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if user.TokenVersion != claims.TokenVersion || user.IsLocked {
		return nil, ErrInvalidCredentials
	}
	return &security.RequestAuth{
		UserID: user.ID,
		Role:   user.Role,
	}, nil
}

func (s *FileService) getSource(ctx context.Context, sourceID uint) (*entity.StorageSource, error) {
	return s.sourceRepo.FindByID(ctx, sourceID)
}

func (s *FileService) getLocalSource(ctx context.Context, sourceID uint) (*entity.StorageSource, error) {
	return getLocalSourceByID(ctx, s.sourceRepo, sourceID)
}

func mimeFromName(name string) string {
	extension := strings.ToLower(filepath.Ext(name))
	mimeType := mime.TypeByExtension(extension)
	if mimeType == "" {
		return "application/octet-stream"
	}
	return mimeType
}

func pageValue(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

func pageSizeValue(pageSize int) int {
	if pageSize <= 0 {
		return 200
	}
	return pageSize
}

func readAllAndReset(body io.ReadCloser) ([]byte, error) {
	defer body.Close()
	return io.ReadAll(body)
}

func (s *FileService) listWithDriver(ctx context.Context, source *entity.StorageSource, query appdto.FileListQuery) (*appdto.FileListResponse, int, int, int, int, error) {
	driver, err := s.getFileDriver(source.DriverType)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}

	virtualPath, err := normalizeVirtualPath(query.Path)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	entries, err := driver.List(ctx, source, virtualPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, 0, 0, 0, ErrFileNotFound
		}
		return nil, 0, 0, 0, 0, err
	}

	items := make([]appdto.FileItem, 0, len(entries))
	for _, entry := range entries {
		if isHiddenStorageEntry(entry) {
			continue
		}
		items = append(items, buildStorageEntryItem(source.ID, entry))
	}
	sortFileItems(items, query.SortBy, query.SortOrder)
	items, err = s.filterReadableFileItems(ctx, source.ID, items)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	pageItems, total, totalPages := paginateItems(items, query.Page, query.PageSize)

	return &appdto.FileListResponse{
		Items:           pageItems,
		CurrentPath:     virtualPath,
		CurrentSourceID: source.ID,
	}, pageValue(query.Page), pageSizeValue(query.PageSize), total, totalPages, nil
}

func (s *FileService) searchWithDriver(ctx context.Context, source *entity.StorageSource, query appdto.FileSearchQuery) (*appdto.FileSearchResponse, int, int, int, int, error) {
	driver, err := s.getFileDriver(source.DriverType)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}

	pathPrefix := "/"
	if query.PathPrefix != "" {
		pathPrefix, err = normalizeVirtualPath(query.PathPrefix)
		if err != nil {
			return nil, 0, 0, 0, 0, err
		}
	}

	entries, err := driver.SearchByName(ctx, source, pathPrefix, query.Keyword)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}

	items := make([]appdto.FileItem, 0, len(entries))
	for _, entry := range entries {
		if isHiddenStorageEntry(entry) {
			continue
		}
		items = append(items, buildStorageEntryItem(source.ID, entry))
	}
	sortFileItems(items, "name", "asc")
	items, err = s.filterReadableFileItems(ctx, source.ID, items)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	pageItems, total, totalPages := paginateItems(items, query.Page, query.PageSize)

	var prefixPtr *string
	if query.PathPrefix != "" {
		prefixPtr = &pathPrefix
	}

	return &appdto.FileSearchResponse{
		Items:           pageItems,
		Keyword:         query.Keyword,
		CurrentSourceID: source.ID,
		PathPrefix:      prefixPtr,
	}, pageValue(query.Page), pageSizeValue(query.PageSize), total, totalPages, nil
}

func (s *FileService) accessURLWithDriver(ctx context.Context, source *entity.StorageSource, req appdto.AccessURLRequest) (*appdto.AccessURLResponse, error) {
	driver, err := s.getFileDriver(source.DriverType)
	if err != nil {
		return nil, err
	}
	virtualPath, err := normalizeVirtualPath(req.Path)
	if err != nil {
		return nil, err
	}
	if req.ExpiresIn <= 0 {
		req.ExpiresIn = 300
	}

	_, _, err = driver.PresignDownload(ctx, source, virtualPath, req.Disposition, time.Duration(req.ExpiresIn)*time.Second)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrFileNotFound
		}
		return nil, err
	}

	token, expiresAt, err := s.fileAccessTokens.Issue(source.ID, virtualPath, req.Purpose, req.Disposition, time.Duration(req.ExpiresIn)*time.Second)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("source_id", fmt.Sprintf("%d", source.ID))
	params.Set("path", virtualPath)
	params.Set("disposition", req.Disposition)
	params.Set("access_token", token)

	return &appdto.AccessURLResponse{
		URL:       "/api/v1/files/download?" + params.Encode(),
		Method:    "GET",
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}, nil
}

func (s *FileService) getFileDriver(driverType string) (FileDriver, error) {
	driver, exists := s.fileDrivers[driverType]
	if !exists {
		return nil, ErrSourceDriverUnsupported
	}
	return driver, nil
}

func (s *FileService) authorizePath(ctx context.Context, sourceID uint, pathValue string, action ACLAction) error {
	if s.aclAuthorizer == nil {
		return nil
	}
	return s.aclAuthorizer.AuthorizePath(ctx, sourceID, pathValue, action)
}

func (s *FileService) filterReadableFileItems(ctx context.Context, sourceID uint, items []appdto.FileItem) ([]appdto.FileItem, error) {
	if s.aclAuthorizer == nil {
		return items, nil
	}
	return s.aclAuthorizer.FilterFileItems(ctx, sourceID, items)
}

func (s *FileService) deleteWithDriver(ctx context.Context, source *entity.StorageSource, req appdto.DeleteFileRequest) (time.Time, error) {
	driver, err := s.getFileDriver(source.DriverType)
	if err != nil {
		return time.Time{}, err
	}
	virtualPath, err := normalizeVirtualPath(req.Path)
	if err != nil {
		return time.Time{}, err
	}
	if req.DeleteMode == "" {
		req.DeleteMode = "trash"
	}
	if req.DeleteMode == "permanent" {
		if err := driver.Delete(ctx, source, virtualPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return time.Time{}, ErrFileNotFound
			}
			return time.Time{}, err
		}
		return time.Now(), nil
	}
	if req.DeleteMode != "trash" {
		return time.Time{}, ErrSourceDriverUnsupported
	}

	entry, err := driver.Stat(ctx, source, virtualPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return time.Time{}, ErrFileNotFound
		}
		return time.Time{}, err
	}

	deletedAt := time.Now()
	trashParent, trashVirtual := buildTrashPaths(virtualPath, deletedAt)
	if err := ensureDriverPath(ctx, driver, source, trashParent); err != nil {
		return time.Time{}, err
	}
	if err := driver.Move(ctx, source, virtualPath, trashParent); err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return time.Time{}, ErrFileNotFound
		case errors.Is(err, fs.ErrExist):
			return time.Time{}, ErrFileAlreadyExists
		case errors.Is(err, os.ErrInvalid):
			return time.Time{}, ErrPathInvalid
		default:
			return time.Time{}, err
		}
	}

	if err := s.recordTrashItem(ctx, source.ID, virtualPath, trashVirtual, entry.Name, entry.IsDir, entry.Size, deletedAt); err != nil {
		originalParent := path.Dir(virtualPath)
		if originalParent == "." {
			originalParent = "/"
		}
		_ = ensureDriverPath(ctx, driver, source, originalParent)
		_ = driver.Move(ctx, source, trashVirtual, originalParent)
		return time.Time{}, err
	}

	return deletedAt, nil
}

func ensureDriverDirectory(ctx context.Context, driver FileDriver, source *entity.StorageSource, parentPath string, name string) error {
	_, err := driver.Mkdir(ctx, source, parentPath, name)
	if err == nil || errors.Is(err, fs.ErrExist) {
		return nil
	}
	if errors.Is(err, os.ErrInvalid) && name == ".trash" && parentPath == "/" {
		return nil
	}
	return err
}

func ensureDriverPath(ctx context.Context, driver FileDriver, source *entity.StorageSource, targetPath string) error {
	targetPath, err := normalizeVirtualPath(targetPath)
	if err != nil {
		return err
	}
	if targetPath == "/" {
		return nil
	}

	segments := strings.Split(strings.TrimPrefix(targetPath, "/"), "/")
	parentPath := "/"
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		if err := ensureDriverDirectory(ctx, driver, source, parentPath, segment); err != nil {
			return err
		}
		parentPath = path.Join(parentPath, segment)
		if parentPath == "." {
			parentPath = "/"
		}
	}
	return nil
}

func buildTrashPaths(virtualPath string, deletedAt time.Time) (string, string) {
	trimmed := strings.TrimPrefix(virtualPath, "/")
	parentRelative := path.Dir(trimmed)
	trashParent := path.Join("/.trash", deletedAt.Format("20060102-150405"))
	if parentRelative != "." && parentRelative != "/" {
		trashParent = path.Join(trashParent, parentRelative)
	}
	trashVirtual := path.Join(trashParent, path.Base(virtualPath))
	if trashParent == "/" {
		trashVirtual = "/" + path.Base(virtualPath)
	}
	return trashParent, trashVirtual
}

func localEntrySize(physicalPath string, info os.FileInfo) (int64, error) {
	if !info.IsDir() {
		return info.Size(), nil
	}

	var total int64
	err := filepath.WalkDir(physicalPath, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		fileInfo, err := d.Info()
		if err != nil {
			return err
		}
		total += fileInfo.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (s *FileService) recordTrashItem(
	ctx context.Context,
	sourceID uint,
	originalPath string,
	trashPath string,
	name string,
	isDir bool,
	size int64,
	deletedAt time.Time,
) error {
	if s.trashItemRepo == nil {
		return nil
	}

	return s.trashItemRepo.Create(ctx, &entity.TrashItem{
		SourceID:     sourceID,
		OriginalPath: originalPath,
		TrashPath:    trashPath,
		Name:         name,
		IsDir:        isDir,
		Size:         size,
		DeletedAt:    deletedAt,
		ExpiresAt:    deletedAt.Add(30 * 24 * time.Hour),
	})
}

func isHiddenStorageEntry(entry StorageEntry) bool {
	if strings.HasPrefix(entry.Name, ".trash") || strings.HasPrefix(entry.Name, ".system") {
		return true
	}
	return isHiddenVirtualPath(entry.Path)
}

func sourcePathExists(ctx context.Context, source *entity.StorageSource, virtualPath string, fileDrivers map[string]FileDriver) (bool, error) {
	switch source.DriverType {
	case "local":
		return localPathExists(source, virtualPath)
	default:
		driver, exists := fileDrivers[source.DriverType]
		if !exists {
			return false, ErrSourceDriverUnsupported
		}
		_, err := driver.Stat(ctx, source, virtualPath)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
}

func localPathExists(source *entity.StorageSource, virtualPath string) (bool, error) {
	_, physicalPath, err := resolvePhysicalPath(source, virtualPath)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(physicalPath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func isHiddenVirtualPath(virtualPath string) bool {
	trimmed := strings.Trim(strings.TrimSpace(virtualPath), "/")
	if trimmed == "" {
		return false
	}
	segments := strings.Split(trimmed, "/")
	for _, segment := range segments {
		if strings.HasPrefix(segment, ".trash") || strings.HasPrefix(segment, ".system") {
			return true
		}
	}
	return false
}
