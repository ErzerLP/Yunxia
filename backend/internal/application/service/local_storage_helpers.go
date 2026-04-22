package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

type localSourceConfig struct {
	BasePath string `json:"base_path"`
}

func parseLocalSourceConfig(source *entity.StorageSource) (localSourceConfig, error) {
	var cfg localSourceConfig
	if err := json.Unmarshal([]byte(source.ConfigJSON), &cfg); err != nil {
		return localSourceConfig{}, err
	}
	if cfg.BasePath == "" {
		return localSourceConfig{}, ErrPathInvalid
	}

	return cfg, nil
}

func marshalLocalSourceConfig(basePath string) (string, error) {
	data, err := json.Marshal(localSourceConfig{BasePath: filepath.ToSlash(filepath.Clean(basePath))})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func normalizeVirtualPath(input string) (string, error) {
	if input == "" {
		input = "/"
	}
	if !strings.HasPrefix(input, "/") {
		return "", ErrPathInvalid
	}
	cleaned := path.Clean(input)
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	if strings.Contains(cleaned, "..") {
		return "", ErrPathInvalid
	}
	return cleaned, nil
}

func validateFileName(name string) error {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || name == "." || name == ".." {
		return ErrFileNameInvalid
	}
	return nil
}

func resolvePhysicalPath(source *entity.StorageSource, virtualPath string) (string, string, error) {
	cfg, err := parseLocalSourceConfig(source)
	if err != nil {
		return "", "", err
	}

	normalizedRoot, err := normalizeVirtualPath(source.RootPath)
	if err != nil {
		return "", "", err
	}
	normalizedPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return "", "", err
	}

	baseRoot := filepath.Join(filepath.Clean(cfg.BasePath), filepath.FromSlash(strings.TrimPrefix(normalizedRoot, "/")))
	target := filepath.Join(baseRoot, filepath.FromSlash(strings.TrimPrefix(normalizedPath, "/")))

	cleanBase := filepath.Clean(baseRoot)
	cleanTarget := filepath.Clean(target)
	if !strings.HasPrefix(cleanTarget, cleanBase) {
		return "", "", ErrPathInvalid
	}

	return cleanBase, cleanTarget, nil
}

func buildFileItem(sourceID uint, virtualPath string, entry fs.FileInfo) appdto.FileItem {
	parent := path.Dir(virtualPath)
	if parent == "." {
		parent = "/"
	}

	item := appdto.FileItem{
		Name:        entry.Name(),
		Path:        virtualPath,
		ParentPath:  parent,
		SourceID:    sourceID,
		IsDir:       entry.IsDir(),
		ModifiedAt:  entry.ModTime().Format(time.RFC3339),
		CreatedAt:   entry.ModTime().Format(time.RFC3339),
		CanDelete:   true,
		CanDownload: !entry.IsDir(),
	}
	if entry.IsDir() {
		item.Size = 0
		item.MimeType = "inode/directory"
		item.Extension = ""
		item.Etag = ""
		item.CanPreview = false
		return item
	}

	item.Size = entry.Size()
	item.Extension = strings.ToLower(filepath.Ext(entry.Name()))
	item.MimeType = mime.TypeByExtension(item.Extension)
	if item.MimeType == "" {
		item.MimeType = "application/octet-stream"
	}
	item.Etag = buildEtag(entry)
	item.CanPreview = canPreviewMIME(item.MimeType)
	return item
}

func buildStorageEntryItem(sourceID uint, entry StorageEntry) appdto.FileItem {
	parent := path.Dir(entry.Path)
	if parent == "." {
		parent = "/"
	}
	modifiedAt := entry.ModifiedAt
	if modifiedAt.IsZero() {
		modifiedAt = time.Unix(0, 0).UTC()
	}

	item := appdto.FileItem{
		Name:        entry.Name,
		Path:        entry.Path,
		ParentPath:  parent,
		SourceID:    sourceID,
		IsDir:       entry.IsDir,
		ModifiedAt:  modifiedAt.Format(time.RFC3339),
		CreatedAt:   modifiedAt.Format(time.RFC3339),
		CanDelete:   true,
		CanDownload: !entry.IsDir,
		Etag:        entry.ETag,
	}
	if entry.IsDir {
		item.Size = 0
		item.MimeType = "inode/directory"
		item.Extension = ""
		item.CanPreview = false
		return item
	}

	item.Size = entry.Size
	item.Extension = strings.ToLower(filepath.Ext(entry.Name))
	item.MimeType = mime.TypeByExtension(item.Extension)
	if item.MimeType == "" {
		item.MimeType = "application/octet-stream"
	}
	item.CanPreview = canPreviewMIME(item.MimeType)
	return item
}

func buildVFSItemFromLocal(sourceID uint, virtualPath string, entry fs.FileInfo) appdto.VFSItem {
	fileItem := buildFileItem(sourceID, virtualPath, entry)
	return buildVFSItemFromFileItem(fileItem, false, false)
}

func buildVFSItemFromStorageEntry(sourceID uint, entry StorageEntry) appdto.VFSItem {
	fileItem := buildStorageEntryItem(sourceID, entry)
	return buildVFSItemFromFileItem(fileItem, false, false)
}

func buildVirtualDirItem(pathValue string, isMountPoint bool) appdto.VFSItem {
	parentPath := path.Dir(pathValue)
	if parentPath == "." {
		parentPath = "/"
	}

	return appdto.VFSItem{
		Name:         path.Base(pathValue),
		Path:         pathValue,
		ParentPath:   parentPath,
		EntryKind:    string(VirtualEntryKindDirectory),
		IsVirtual:    true,
		IsMountPoint: isMountPoint,
		Size:         0,
		MimeType:     "inode/directory",
		Extension:    "",
		CanPreview:   false,
		CanDownload:  false,
		CanDelete:    false,
	}
}

func buildVFSItemFromFileItem(fileItem appdto.FileItem, isVirtual bool, isMountPoint bool) appdto.VFSItem {
	entryKind := string(VirtualEntryKindFile)
	if fileItem.IsDir {
		entryKind = string(VirtualEntryKindDirectory)
	}

	return appdto.VFSItem{
		Name:         fileItem.Name,
		Path:         fileItem.Path,
		ParentPath:   fileItem.ParentPath,
		SourceID:     &fileItem.SourceID,
		EntryKind:    entryKind,
		IsVirtual:    isVirtual,
		IsMountPoint: isMountPoint,
		Size:         fileItem.Size,
		MimeType:     fileItem.MimeType,
		Extension:    fileItem.Extension,
		ModifiedAt:   fileItem.ModifiedAt,
		CreatedAt:    fileItem.CreatedAt,
		Etag:         fileItem.Etag,
		CanPreview:   fileItem.CanPreview,
		CanDownload:  fileItem.CanDownload,
		CanDelete:    fileItem.CanDelete,
	}
}

func buildEtag(info fs.FileInfo) string {
	return "mtime:" + info.ModTime().UTC().Format(time.RFC3339Nano)
}

func canPreviewMIME(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/") ||
		strings.HasPrefix(mimeType, "video/") ||
		strings.HasPrefix(mimeType, "audio/") ||
		strings.HasPrefix(mimeType, "text/") ||
		mimeType == "application/pdf" ||
		strings.HasSuffix(mimeType, "json")
}

func sortFileItems(items []appdto.FileItem, sortBy, sortOrder string) {
	desc := strings.EqualFold(sortOrder, "desc")
	if sortBy == "" {
		sortBy = "modified_at"
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}

		var less bool
		switch sortBy {
		case "size":
			less = items[i].Size < items[j].Size
		case "name":
			less = strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		default:
			less = items[i].ModifiedAt < items[j].ModifiedAt
		}
		if desc {
			return !less
		}
		return less
	})
}

func paginateItems[T any](items []T, page, pageSize int) ([]T, int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 200
	}

	total := len(items)
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	start := (page - 1) * pageSize
	if start >= total {
		return []T{}, total, totalPages
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	return items[start:end], total, totalPages
}

func hashReaderMD5(reader io.Reader) (string, error) {
	hasher := md5.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func hashFileMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	return hashReaderMD5(file)
}

func copyFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	info, err := src.Stat()
	if err == nil {
		_ = os.Chtimes(dstPath, info.ModTime(), info.ModTime())
	}

	return nil
}

func copyDirectory(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(current string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(srcDir, current)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, relative)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(current, target)
	})
}

func computeUsedBytes(root string) *int64 {
	var total int64
	_ = filepath.WalkDir(root, func(current string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		total += info.Size()
		return nil
	})

	return &total
}

func generateSlug(name string, fallback string) string {
	value := strings.TrimSpace(strings.ToLower(name))
	if value == "" {
		return fallback
	}

	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
			lastDash = false
		case !lastDash:
			builder.WriteRune('-')
			lastDash = true
		}
	}

	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return fallback
	}
	return slug
}

func getLocalSourceByID(ctx context.Context, repo domainrepo.SourceRepository, sourceID uint) (*entity.StorageSource, error) {
	source, err := repo.FindByID(ctx, sourceID)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, err
	}
	if source.DriverType != "local" {
		return nil, ErrSourceDriverUnsupported
	}
	return source, nil
}
