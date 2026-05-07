package service

import (
	"context"
	"os"
	"path"
	"strings"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

type legacyFileVFSService interface {
	List(ctx context.Context, currentPath string) (*appdto.VFSListResponse, error)
	Search(ctx context.Context, pathPrefix string, keyword string) (*appdto.VFSSearchResponse, error)
	Mkdir(ctx context.Context, req appdto.VFSMkdirRequest) (*appdto.VFSItem, error)
	Rename(ctx context.Context, req appdto.VFSRenameRequest) (string, string, *appdto.VFSItem, error)
	Move(ctx context.Context, req appdto.VFSMoveCopyRequest) (string, string, error)
	Copy(ctx context.Context, req appdto.VFSMoveCopyRequest) (string, string, error)
	Delete(ctx context.Context, req appdto.VFSDeleteRequest) (time.Time, error)
}

type legacyFileBackend interface {
	AccessURL(ctx context.Context, req appdto.AccessURLRequest) (*appdto.AccessURLResponse, error)
	ResolveDownload(ctx context.Context, sourceID uint, filePath string) (*os.File, os.FileInfo, string, error)
	ResolveDownloadRedirect(ctx context.Context, sourceID uint, filePath, disposition string) (string, error)
	ValidateFileAccessToken(raw string) (*security.FileAccessClaims, error)
	AuthenticateBearerToken(ctx context.Context, raw string) (*security.RequestAuth, error)
}

// LegacyFileFacade 保留 /api/v1/files 的 source_id+path 外观，内部读写统一转 VFS。
type LegacyFileFacade struct {
	sourceRepo domainrepo.SourceRepository
	vfs        legacyFileVFSService
	file       legacyFileBackend
}

// NewLegacyFileFacade 创建旧文件 API 兼容门面。
func NewLegacyFileFacade(sourceRepo domainrepo.SourceRepository, vfs legacyFileVFSService, file legacyFileBackend) *LegacyFileFacade {
	return &LegacyFileFacade{
		sourceRepo: sourceRepo,
		vfs:        vfs,
		file:       file,
	}
}

// List 返回文件列表，响应仍保持 source 内路径。
func (s *LegacyFileFacade) List(ctx context.Context, query appdto.FileListQuery) (*appdto.FileListResponse, int, int, int, int, error) {
	source, virtualPath, innerPath, err := s.resolveLegacyPath(ctx, query.SourceID, query.Path)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	resp, err := s.vfs.List(ctx, virtualPath)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	items := s.fileItemsFromVFS(source, resp.Items)
	sortFileItems(items, query.SortBy, query.SortOrder)
	pageItems, total, totalPages := paginateItems(items, query.Page, query.PageSize)
	return &appdto.FileListResponse{
		Items:           pageItems,
		CurrentPath:     innerPath,
		CurrentSourceID: source.ID,
	}, pageValue(query.Page), pageSizeValue(query.PageSize), total, totalPages, nil
}

// Search 返回文件搜索结果，响应仍保持 source 内路径。
func (s *LegacyFileFacade) Search(ctx context.Context, query appdto.FileSearchQuery) (*appdto.FileSearchResponse, int, int, int, int, error) {
	pathPrefix := query.PathPrefix
	if strings.TrimSpace(pathPrefix) == "" {
		pathPrefix = "/"
	}
	source, virtualPrefix, innerPrefix, err := s.resolveLegacyPath(ctx, query.SourceID, pathPrefix)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	resp, err := s.vfs.Search(ctx, virtualPrefix, query.Keyword)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	items := s.fileItemsFromVFS(source, resp.Items)
	sortFileItems(items, "name", "asc")
	pageItems, total, totalPages := paginateItems(items, query.Page, query.PageSize)

	var prefixPtr *string
	if strings.TrimSpace(query.PathPrefix) != "" {
		prefixPtr = &innerPrefix
	}
	return &appdto.FileSearchResponse{
		Items:           pageItems,
		Keyword:         query.Keyword,
		CurrentSourceID: source.ID,
		PathPrefix:      prefixPtr,
	}, pageValue(query.Page), pageSizeValue(query.PageSize), total, totalPages, nil
}

// Mkdir 在 VFS 中创建目录。
func (s *LegacyFileFacade) Mkdir(ctx context.Context, req appdto.MkdirRequest) (*appdto.FileItem, error) {
	source, parentVirtualPath, _, err := s.resolveLegacyPath(ctx, req.SourceID, req.ParentPath)
	if err != nil {
		return nil, err
	}
	item, err := s.vfs.Mkdir(ctx, appdto.VFSMkdirRequest{ParentPath: parentVirtualPath, Name: req.Name})
	if err != nil {
		return nil, err
	}
	return s.fileItemFromVFS(source, *item)
}

// Rename 在 VFS 中重命名节点。
func (s *LegacyFileFacade) Rename(ctx context.Context, req appdto.RenameRequest) (string, string, *appdto.FileItem, error) {
	source, virtualPath, innerPath, err := s.resolveLegacyPath(ctx, req.SourceID, req.Path)
	if err != nil {
		return "", "", nil, err
	}
	_, newVirtualPath, item, err := s.vfs.Rename(ctx, appdto.VFSRenameRequest{Path: virtualPath, NewName: req.NewName})
	if err != nil {
		return "", "", nil, err
	}
	newInnerPath, ok := legacyInnerPathForSource(source, newVirtualPath)
	if !ok {
		return "", "", nil, ErrPathInvalid
	}
	fileItem, err := s.fileItemFromVFS(source, *item)
	if err != nil {
		return "", "", nil, err
	}
	return innerPath, newInnerPath, fileItem, nil
}

// Move 在 VFS 中移动节点。
func (s *LegacyFileFacade) Move(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error) {
	source, virtualPath, innerPath, err := s.resolveLegacyPath(ctx, req.SourceID, req.Path)
	if err != nil {
		return "", "", err
	}
	_, targetVirtualPath, _, err := s.resolveLegacyPath(ctx, req.SourceID, req.TargetPath)
	if err != nil {
		return "", "", err
	}
	_, newVirtualPath, err := s.vfs.Move(ctx, appdto.VFSMoveCopyRequest{Path: virtualPath, TargetPath: targetVirtualPath})
	if err != nil {
		return "", "", err
	}
	newInnerPath, ok := legacyInnerPathForSource(source, newVirtualPath)
	if !ok {
		return "", "", ErrPathInvalid
	}
	return innerPath, newInnerPath, nil
}

// Copy 在 VFS 中复制节点。
func (s *LegacyFileFacade) Copy(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error) {
	source, virtualPath, innerPath, err := s.resolveLegacyPath(ctx, req.SourceID, req.Path)
	if err != nil {
		return "", "", err
	}
	_, targetVirtualPath, _, err := s.resolveLegacyPath(ctx, req.SourceID, req.TargetPath)
	if err != nil {
		return "", "", err
	}
	_, newVirtualPath, err := s.vfs.Copy(ctx, appdto.VFSMoveCopyRequest{Path: virtualPath, TargetPath: targetVirtualPath})
	if err != nil {
		return "", "", err
	}
	newInnerPath, ok := legacyInnerPathForSource(source, newVirtualPath)
	if !ok {
		return "", "", ErrPathInvalid
	}
	return innerPath, newInnerPath, nil
}

// Delete 在 VFS 中删除节点。
func (s *LegacyFileFacade) Delete(ctx context.Context, req appdto.DeleteFileRequest) (time.Time, error) {
	_, virtualPath, _, err := s.resolveLegacyPath(ctx, req.SourceID, req.Path)
	if err != nil {
		return time.Time{}, err
	}
	return s.vfs.Delete(ctx, appdto.VFSDeleteRequest{Path: virtualPath, DeleteMode: req.DeleteMode})
}

// AccessURL 生成短时文件访问地址。
func (s *LegacyFileFacade) AccessURL(ctx context.Context, req appdto.AccessURLRequest) (*appdto.AccessURLResponse, error) {
	return s.file.AccessURL(ctx, req)
}

// ResolveDownload 解析下载请求并返回文件。
func (s *LegacyFileFacade) ResolveDownload(ctx context.Context, sourceID uint, filePath string) (*os.File, os.FileInfo, string, error) {
	return s.file.ResolveDownload(ctx, sourceID, filePath)
}

// ResolveDownloadRedirect 返回非 local 驱动的下载跳转地址。
func (s *LegacyFileFacade) ResolveDownloadRedirect(ctx context.Context, sourceID uint, filePath, disposition string) (string, error) {
	return s.file.ResolveDownloadRedirect(ctx, sourceID, filePath, disposition)
}

// ValidateFileAccessToken 校验短时文件访问令牌。
func (s *LegacyFileFacade) ValidateFileAccessToken(raw string) (*security.FileAccessClaims, error) {
	return s.file.ValidateFileAccessToken(raw)
}

// AuthenticateBearerToken 校验下载请求携带的 Bearer token 并返回身份。
func (s *LegacyFileFacade) AuthenticateBearerToken(ctx context.Context, raw string) (*security.RequestAuth, error) {
	return s.file.AuthenticateBearerToken(ctx, raw)
}

func (s *LegacyFileFacade) resolveLegacyPath(ctx context.Context, sourceID uint, innerPath string) (*entity.StorageSource, string, string, error) {
	source, err := s.sourceRepo.FindByID(ctx, sourceID)
	if err != nil {
		return nil, "", "", err
	}
	normalizedInnerPath, err := normalizeVirtualPath(innerPath)
	if err != nil {
		return nil, "", "", err
	}
	virtualPath := mergeMountAndInnerPath(source.MountPath, normalizedInnerPath)
	if virtualPath == "" {
		return nil, "", "", ErrPathInvalid
	}
	return source, virtualPath, normalizedInnerPath, nil
}

func (s *LegacyFileFacade) fileItemsFromVFS(source *entity.StorageSource, items []appdto.VFSItem) []appdto.FileItem {
	fileItems := make([]appdto.FileItem, 0, len(items))
	for _, item := range items {
		fileItem, err := s.fileItemFromVFS(source, item)
		if err == nil {
			fileItems = append(fileItems, *fileItem)
		}
	}
	return fileItems
}

func (s *LegacyFileFacade) fileItemFromVFS(source *entity.StorageSource, item appdto.VFSItem) (*appdto.FileItem, error) {
	if source == nil {
		return nil, ErrPathInvalid
	}
	if item.SourceID == nil || *item.SourceID != source.ID {
		return nil, ErrPathInvalid
	}
	innerPath, ok := legacyInnerPathForSource(source, item.Path)
	if !ok {
		return nil, ErrPathInvalid
	}
	parentPath := path.Dir(innerPath)
	if parentPath == "." {
		parentPath = "/"
	}
	isDir := item.EntryKind != string(VirtualEntryKindFile)
	return &appdto.FileItem{
		Name:         item.Name,
		Path:         innerPath,
		ParentPath:   parentPath,
		SourceID:     source.ID,
		IsDir:        isDir,
		Size:         item.Size,
		MimeType:     item.MimeType,
		Extension:    item.Extension,
		Etag:         item.Etag,
		ModifiedAt:   item.ModifiedAt,
		CreatedAt:    item.CreatedAt,
		CanPreview:   item.CanPreview,
		CanDownload:  item.CanDownload && !isDir,
		CanDelete:    item.CanDelete,
		ThumbnailURL: nil,
	}, nil
}

func legacyInnerPathForSource(source *entity.StorageSource, virtualPath string) (string, bool) {
	if source == nil {
		return "", false
	}
	normalizedMountPath, err := normalizeMountPath(source.MountPath)
	if err != nil {
		return "", false
	}
	normalizedVirtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return "", false
	}
	if !isSubPath(normalizedMountPath, normalizedVirtualPath) {
		return "", false
	}
	if normalizedMountPath == "/" {
		return normalizedVirtualPath, true
	}
	innerPath := strings.TrimPrefix(normalizedVirtualPath, normalizedMountPath)
	if innerPath == "" {
		return "/", true
	}
	if !strings.HasPrefix(innerPath, "/") {
		innerPath = "/" + innerPath
	}
	return innerPath, true
}
