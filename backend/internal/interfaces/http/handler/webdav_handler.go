package handler

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	appaudit "yunxia/internal/application/audit"
	appdto "yunxia/internal/application/dto"
	appsvc "yunxia/internal/application/service"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/observability/logging"
	"yunxia/internal/infrastructure/security"
)

type passwordComparer interface {
	Compare(hash, password string) bool
}

type webDAVFileService interface {
	AccessURL(ctx context.Context, req appdto.AccessURLRequest) (*appdto.AccessURLResponse, error)
	AccessURLByVFSObject(ctx context.Context, virtualPath string, req appdto.AccessURLRequest) (*appdto.AccessURLResponse, bool, error)
}

type webDAVUploadService interface {
	ImportLocalFile(ctx context.Context, sourceID uint, parentPath string, filename string, localPath string) (*appdto.FileItem, error)
}

type webDAVVFSService interface {
	List(ctx context.Context, currentPath string) (*appdto.VFSListResponse, error)
	ResolvePath(ctx context.Context, virtualPath string) (appsvc.ResolvedPath, error)
	Mkdir(ctx context.Context, req appdto.VFSMkdirRequest) (*appdto.VFSItem, error)
	Rename(ctx context.Context, req appdto.VFSRenameRequest) (string, string, *appdto.VFSItem, error)
	Move(ctx context.Context, req appdto.VFSMoveCopyRequest) (string, string, error)
	Copy(ctx context.Context, req appdto.VFSMoveCopyRequest) (string, string, error)
	Delete(ctx context.Context, req appdto.VFSDeleteRequest) (time.Time, error)
}

// WebDAVHandler 负责存储源的 WebDAV 暴露。
type WebDAVHandler struct {
	prefix           string
	sourceRepo       domainrepo.SourceRepository
	systemConfigRepo domainrepo.SystemConfigRepository
	userRepo         domainrepo.UserRepository
	aclAuthorizer    *appsvc.ACLAuthorizer
	vfsService       webDAVVFSService
	fileService      webDAVFileService
	uploadService    webDAVUploadService
	hasher           passwordComparer
	auditRecorder    *appaudit.Recorder
	logger           *slog.Logger
}

// NewWebDAVHandler 创建 WebDAV handler。
func NewWebDAVHandler(
	prefix string,
	sourceRepo domainrepo.SourceRepository,
	systemConfigRepo domainrepo.SystemConfigRepository,
	userRepo domainrepo.UserRepository,
	aclAuthorizer *appsvc.ACLAuthorizer,
	vfsService webDAVVFSService,
	fileService webDAVFileService,
	uploadService webDAVUploadService,
	hasher passwordComparer,
	auditRecorder *appaudit.Recorder,
	logger *slog.Logger,
) *WebDAVHandler {
	if logger == nil {
		logger = logging.Component(slog.Default(), "http.webdav")
	}
	return &WebDAVHandler{
		prefix:           prefix,
		sourceRepo:       sourceRepo,
		systemConfigRepo: systemConfigRepo,
		userRepo:         userRepo,
		aclAuthorizer:    aclAuthorizer,
		vfsService:       vfsService,
		fileService:      fileService,
		uploadService:    uploadService,
		hasher:           hasher,
		auditRecorder:    auditRecorder,
		logger:           logger,
	}
}

// Serve 处理 WebDAV 请求。
func (h *WebDAVHandler) Serve(c *gin.Context) {
	if !isSecureWebDAVRequest(c.Request) {
		http.Error(c.Writer, "webdav requires https", http.StatusForbidden)
		return
	}

	cfg, err := h.systemConfigRepo.Get(c.Request.Context())
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			http.NotFound(c.Writer, c.Request)
			return
		}
		http.Error(c.Writer, "internal error", http.StatusInternalServerError)
		return
	}
	if !cfg.WebDAVEnabled {
		http.NotFound(c.Writer, c.Request)
		return
	}

	source, err := h.findSourceBySlug(c.Request.Context(), c.Param("slug"))
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			http.NotFound(c.Writer, c.Request)
			return
		}
		http.Error(c.Writer, "internal error", http.StatusInternalServerError)
		return
	}
	if !source.IsEnabled || !source.IsWebDAVExposed {
		http.NotFound(c.Writer, c.Request)
		return
	}

	user, authErr := h.authenticate(c.Request)
	if authErr != nil {
		challengeWebDAV(c.Writer)
		return
	}

	webdavPath, err := normalizeWebDAVRequestPath(c.Param("filepath"))
	if err != nil {
		http.Error(c.Writer, "bad request", http.StatusBadRequest)
		return
	}

	requestCtx := security.WithRequestAuth(c.Request.Context(), security.RequestAuth{
		UserID:       user.ID,
		Username:     user.Username,
		RoleKey:      user.RoleKey,
		Status:       user.Status,
		Capabilities: capabilitiesForRole(user.RoleKey),
	})
	c.Request = c.Request.WithContext(requestCtx)

	req := cloneRequest(c.Request)
	req.URL.Path = webdavPath
	req.URL.RawPath = webdavPath
	rewriteWebDAVDestination(req, h.prefix, source.WebDAVSlug)
	if action, ok := webDAVAuditAction(req.Method); ok {
		destinationPath, _ := normalizeWebDAVDestinationPath(req.Header.Get("Destination"))
		defer h.recordWriteAudit(c, source, action, webdavPath, destinationPath)
	}

	vfsPath, err := webDAVVFSPath(source, webdavPath)
	if err != nil {
		http.Error(c.Writer, "bad request", http.StatusBadRequest)
		return
	}

	h.serveVFSWebDAV(c, req, source, webdavPath, vfsPath)
}

func (h *WebDAVHandler) serveVFSWebDAV(c *gin.Context, req *http.Request, source *entity.StorageSource, webdavPath string, vfsPath string) {
	if h.vfsService == nil {
		http.Error(c.Writer, "webdav vfs unavailable", http.StatusNotImplemented)
		return
	}

	switch strings.ToUpper(req.Method) {
	case http.MethodOptions:
		writeWebDAVOptions(c.Writer)
	case "PROPFIND":
		h.handleVFSPropfind(c.Writer, req, source, webdavPath, vfsPath)
	case http.MethodHead:
		h.handleVFSDownload(c.Writer, req, vfsPath, true)
	case http.MethodGet:
		h.handleVFSDownload(c.Writer, req, vfsPath, false)
	case "MKCOL":
		if !h.ensureWebDAVWritable(c.Writer, source) {
			return
		}
		h.handleVFSMKCOL(c.Writer, req, vfsPath)
	case http.MethodPut:
		if !h.ensureWebDAVWritable(c.Writer, source) {
			return
		}
		h.handleVFSPUT(c.Writer, req, source, vfsPath)
	case http.MethodDelete:
		if !h.ensureWebDAVWritable(c.Writer, source) {
			return
		}
		h.handleVFSDELETE(c.Writer, req, vfsPath)
	case "COPY":
		if !h.ensureWebDAVWritable(c.Writer, source) {
			return
		}
		h.handleVFSCopyMove(c.Writer, req, source, vfsPath, false)
	case "MOVE":
		if !h.ensureWebDAVWritable(c.Writer, source) {
			return
		}
		h.handleVFSCopyMove(c.Writer, req, source, vfsPath, true)
	default:
		http.Error(c.Writer, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *WebDAVHandler) ensureWebDAVWritable(writer http.ResponseWriter, source *entity.StorageSource) bool {
	if source != nil && source.WebDAVReadOnly {
		http.Error(writer, "webdav source is read only", http.StatusForbidden)
		return false
	}
	return true
}

func (h *WebDAVHandler) handleVFSPropfind(writer http.ResponseWriter, req *http.Request, source *entity.StorageSource, webdavPath string, vfsPath string) {
	item, err := h.vfsWebDAVItem(req.Context(), source, webdavPath, vfsPath)
	if err != nil {
		http.Error(writer, "not found", webDAVStatusFromError(err))
		return
	}

	items := []appdto.FileItem{*item}
	depth := strings.TrimSpace(req.Header.Get("Depth"))
	if item.IsDir && depth != "0" {
		resp, err := h.vfsService.List(req.Context(), vfsPath)
		if err != nil {
			http.Error(writer, "not found", webDAVStatusFromError(err))
			return
		}
		for _, child := range resp.Items {
			childItem, ok := webDAVFileItemFromVFSItem(source, child)
			if ok {
				items = append(items, childItem)
			}
		}
	}
	writeWebDAVMultiStatus(writer, h.prefix, source.WebDAVSlug, items)
}

func (h *WebDAVHandler) vfsWebDAVItem(ctx context.Context, source *entity.StorageSource, webdavPath string, vfsPath string) (*appdto.FileItem, error) {
	if webdavPath == "/" {
		if err := h.ensureWebDAVSourceVisible(ctx, source); err != nil {
			return nil, err
		}
		return &appdto.FileItem{
			Name:       source.WebDAVSlug,
			Path:       "/",
			SourceID:   source.ID,
			IsDir:      true,
			ModifiedAt: time.Now().UTC().Format(time.RFC3339),
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	parentPath := parentVirtualPathForHTTP(vfsPath)
	resp, err := h.vfsService.List(ctx, parentPath)
	if err != nil {
		return nil, err
	}
	for _, item := range resp.Items {
		if item.Path != vfsPath {
			continue
		}
		fileItem, ok := webDAVFileItemFromVFSItem(source, item)
		if !ok {
			return nil, appsvc.ErrFileNotFound
		}
		return &fileItem, nil
	}
	return nil, appsvc.ErrFileNotFound
}

func (h *WebDAVHandler) ensureWebDAVSourceVisible(ctx context.Context, source *entity.StorageSource) error {
	if h == nil || h.aclAuthorizer == nil || source == nil {
		return nil
	}
	visible, err := h.aclAuthorizer.CanSeeSource(ctx, source.ID)
	if err != nil {
		return err
	}
	if !visible {
		return appsvc.ErrACLDenied
	}
	return nil
}

func (h *WebDAVHandler) handleVFSDownload(writer http.ResponseWriter, req *http.Request, vfsPath string, headOnly bool) {
	if h.fileService == nil {
		http.Error(writer, "download unavailable", http.StatusNotImplemented)
		return
	}
	resolved, err := h.vfsService.ResolvePath(req.Context(), vfsPath)
	if err != nil {
		http.Error(writer, "not found", webDAVStatusFromError(err))
		return
	}
	accessReq := appdto.AccessURLRequest{
		SourceID:    resolved.Source.ID,
		Path:        resolved.InnerPath,
		Purpose:     "download",
		Disposition: "inline",
		ExpiresIn:   300,
	}
	accessURL, matched, err := h.fileService.AccessURLByVFSObject(req.Context(), vfsPath, accessReq)
	if err != nil {
		http.Error(writer, "not found", webDAVStatusFromError(err))
		return
	}
	if !matched {
		accessURL, err = h.fileService.AccessURL(req.Context(), accessReq)
		if err != nil {
			http.Error(writer, "not found", webDAVStatusFromError(err))
			return
		}
	}
	if accessURL == nil || accessURL.URL == "" {
		http.Error(writer, "download unavailable", http.StatusNotImplemented)
		return
	}
	redirectURL := rewriteVFSAccessURL(accessURL.URL, vfsPath, "inline")
	writer.Header().Set("Location", redirectURL)
	if headOnly {
		writer.WriteHeader(http.StatusFound)
		return
	}
	http.Redirect(writer, req, redirectURL, http.StatusFound)
}

func (h *WebDAVHandler) handleVFSMKCOL(writer http.ResponseWriter, req *http.Request, vfsPath string) {
	parentPath, name, err := splitWebDAVTarget(vfsPath)
	if err != nil {
		http.Error(writer, "bad request", http.StatusBadRequest)
		return
	}
	if _, err := h.vfsService.Mkdir(req.Context(), appdto.VFSMkdirRequest{
		ParentPath: parentPath,
		Name:       name,
	}); err != nil {
		http.Error(writer, "webdav mkdir failed", webDAVStatusFromError(err))
		return
	}
	writer.WriteHeader(http.StatusCreated)
}

func (h *WebDAVHandler) handleVFSPUT(writer http.ResponseWriter, req *http.Request, source *entity.StorageSource, vfsPath string) {
	if h.uploadService == nil {
		http.Error(writer, "webdav upload unavailable", http.StatusNotImplemented)
		return
	}
	resolved, err := h.vfsService.ResolvePath(req.Context(), vfsPath)
	if err != nil {
		http.Error(writer, "not found", webDAVStatusFromError(err))
		return
	}
	parentPath, name, err := splitWebDAVTarget(resolved.InnerPath)
	if err != nil {
		http.Error(writer, "bad request", http.StatusBadRequest)
		return
	}
	tmp, err := os.CreateTemp("", "yunxia-webdav-upload-*")
	if err != nil {
		http.Error(writer, "internal error", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := io.Copy(tmp, req.Body); err != nil {
		_ = tmp.Close()
		http.Error(writer, "internal error", http.StatusInternalServerError)
		return
	}
	if err := tmp.Close(); err != nil {
		http.Error(writer, "internal error", http.StatusInternalServerError)
		return
	}
	if _, err := h.uploadService.ImportLocalFile(req.Context(), source.ID, parentPath, name, tmpPath); err != nil {
		http.Error(writer, "webdav put failed", webDAVStatusFromError(err))
		return
	}
	writer.WriteHeader(http.StatusCreated)
}

func (h *WebDAVHandler) handleVFSDELETE(writer http.ResponseWriter, req *http.Request, vfsPath string) {
	if _, err := h.vfsService.Delete(req.Context(), appdto.VFSDeleteRequest{
		Path:       vfsPath,
		DeleteMode: "trash",
	}); err != nil {
		http.Error(writer, "webdav delete failed", webDAVStatusFromError(err))
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *WebDAVHandler) handleVFSCopyMove(writer http.ResponseWriter, req *http.Request, source *entity.StorageSource, vfsPath string, move bool) {
	destinationInnerPath, err := h.normalizedWebDAVDestinationPath(req)
	if err != nil {
		http.Error(writer, "bad destination", http.StatusBadRequest)
		return
	}
	destinationPath, err := webDAVVFSPath(source, destinationInnerPath)
	if err != nil {
		http.Error(writer, "bad destination", http.StatusBadRequest)
		return
	}
	sourceRootPath, err := webDAVVFSPath(source, "/")
	if err != nil {
		http.Error(writer, "bad request", http.StatusBadRequest)
		return
	}
	if vfsPath == sourceRootPath || destinationPath == sourceRootPath {
		http.Error(writer, "bad request", http.StatusBadRequest)
		return
	}
	destinationParent := path.Dir(destinationPath)
	if destinationParent == "." {
		destinationParent = "/"
	}
	destinationName := path.Base(destinationPath)
	sourceName := path.Base(vfsPath)

	var newPath string
	if move {
		_, movedPath, err := h.vfsService.Move(req.Context(), appdto.VFSMoveCopyRequest{
			Path:       vfsPath,
			TargetPath: destinationParent,
		})
		if err != nil {
			http.Error(writer, "webdav move failed", webDAVStatusFromError(err))
			return
		}
		newPath = movedPath
	} else {
		_, copiedPath, err := h.vfsService.Copy(req.Context(), appdto.VFSMoveCopyRequest{
			Path:       vfsPath,
			TargetPath: destinationParent,
		})
		if err != nil {
			http.Error(writer, "webdav copy failed", webDAVStatusFromError(err))
			return
		}
		newPath = copiedPath
	}

	if destinationName != sourceName {
		_, renamedPath, _, err := h.vfsService.Rename(req.Context(), appdto.VFSRenameRequest{
			Path:    newPath,
			NewName: destinationName,
		})
		if err != nil {
			http.Error(writer, "webdav rename failed", webDAVStatusFromError(err))
			return
		}
		newPath = renamedPath
	}
	if newPath != destinationPath {
		if innerPath, ok := webDAVInnerPathFromVFSPath(source.MountPath, newPath); ok {
			writer.Header().Set("Content-Location", h.webDAVExternalHref(source.WebDAVSlug, innerPath))
		}
	}
	if move {
		writer.WriteHeader(http.StatusCreated)
		return
	}
	writer.WriteHeader(http.StatusCreated)
}

func (h *WebDAVHandler) normalizedWebDAVDestinationPath(req *http.Request) (string, error) {
	raw := strings.TrimSpace(req.Header.Get("Destination"))
	if raw == "" {
		return "", os.ErrInvalid
	}
	destinationPath, err := normalizeWebDAVDestinationPath(raw)
	if err != nil {
		return "", err
	}
	externalPrefix := strings.TrimRight(h.prefix, "/")
	if externalPrefix == "" {
		externalPrefix = "/dav"
	}
	if destinationPath == externalPrefix || strings.HasPrefix(destinationPath, externalPrefix+"/") {
		return "", os.ErrInvalid
	}
	return destinationPath, nil
}

func (h *WebDAVHandler) recordWriteAudit(c *gin.Context, source *entity.StorageSource, action string, requestPath string, destinationPath string) {
	if h == nil || h.auditRecorder == nil || source == nil {
		return
	}

	status := c.Writer.Status()
	result, errorCode := classifyWebDAVAuditResult(status)
	sourceID := source.ID
	sourceVirtualPath := mergeMountAndInnerPathForHTTP(source.MountPath, requestPath)
	targetPath := requestPath
	if (action == "copy" || action == "move") && destinationPath != "" {
		targetPath = destinationPath
	}
	targetVirtualPath := mergeMountAndInnerPathForHTTP(source.MountPath, targetPath)

	event := appaudit.Event{
		ResourceType: "file",
		Action:       action,
		Result:       result,
		ErrorCode:    errorCode,
		SourceID:     &sourceID,
		VirtualPath:  targetVirtualPath,
		Detail: map[string]any{
			"status":              status,
			"request_path":        requestPath,
			"target_virtual_path": targetVirtualPath,
		},
	}
	switch action {
	case "mkcol", "put":
		event.After = map[string]any{
			"virtual_path": targetVirtualPath,
		}
	case "copy", "move":
		event.Before = map[string]any{
			"virtual_path": sourceVirtualPath,
		}
		event.After = map[string]any{
			"virtual_path": targetVirtualPath,
		}
		if destinationPath != "" {
			event.Detail["destination_path"] = destinationPath
		}
	case "delete":
		event.Before = map[string]any{
			"virtual_path": sourceVirtualPath,
		}
	}

	appaudit.RecordBestEffort(c.Request.Context(), h.auditRecorder, h.logger, event)
}

func (h *WebDAVHandler) findSourceBySlug(ctx context.Context, slug string) (*entity.StorageSource, error) {
	items, err := h.sourceRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.WebDAVSlug == slug {
			return item, nil
		}
	}
	return nil, domainrepo.ErrNotFound
}

func (h *WebDAVHandler) authenticate(req *http.Request) (*entity.User, error) {
	username, password, ok := req.BasicAuth()
	if !ok || username == "" {
		return nil, domainrepo.ErrNotFound
	}

	user, err := h.userRepo.FindByUsername(req.Context(), username)
	if err != nil {
		return nil, err
	}
	if user.Status == permission.StatusLocked || !h.hasher.Compare(user.PasswordHash, password) {
		return nil, domainrepo.ErrNotFound
	}
	return user, nil
}

func capabilitiesForRole(roleKey string) []string {
	capabilities, err := permission.ResolveCapabilities(roleKey)
	if err != nil {
		return nil
	}
	return capabilities
}

func cloneRequest(req *http.Request) *http.Request {
	cloned := req.Clone(req.Context())
	if req.URL != nil {
		urlCopy := *req.URL
		cloned.URL = &urlCopy
	}
	return cloned
}

func splitWebDAVTarget(webdavPath string) (string, string, error) {
	webdavPath, err := normalizeWebDAVRequestPath(webdavPath)
	if err != nil {
		return "", "", err
	}
	if webdavPath == "/" {
		return "", "", os.ErrInvalid
	}
	parentPath := path.Dir(webdavPath)
	if parentPath == "." {
		parentPath = "/"
	}
	name := path.Base(webdavPath)
	if name == "." || name == "/" || strings.TrimSpace(name) == "" {
		return "", "", os.ErrInvalid
	}
	return parentPath, name, nil
}

func webDAVVFSPath(source *entity.StorageSource, webdavPath string) (string, error) {
	if source == nil || strings.TrimSpace(source.MountPath) == "" {
		return "", appsvc.ErrPathInvalid
	}
	innerPath, err := normalizeWebDAVRequestPath(webdavPath)
	if err != nil {
		return "", err
	}
	virtualPath := mergeMountAndInnerPathForHTTP(source.MountPath, innerPath)
	if virtualPath == "" {
		return "", appsvc.ErrPathInvalid
	}
	return normalizeWebDAVRequestPath(virtualPath)
}

func webDAVFileItemFromVFSItem(source *entity.StorageSource, item appdto.VFSItem) (appdto.FileItem, bool) {
	if source == nil {
		return appdto.FileItem{}, false
	}
	innerPath, ok := webDAVInnerPathFromVFSPath(source.MountPath, item.Path)
	if !ok {
		return appdto.FileItem{}, false
	}
	sourceID := source.ID
	if item.SourceID != nil {
		sourceID = *item.SourceID
	}
	isDir := item.EntryKind == "directory" || item.IsMountPoint
	name := item.Name
	if innerPath == "/" && name == "" {
		name = source.WebDAVSlug
	}
	return appdto.FileItem{
		Name:        name,
		Path:        innerPath,
		ParentPath:  parentVirtualPathForHTTP(innerPath),
		SourceID:    sourceID,
		IsDir:       isDir,
		Size:        item.Size,
		MimeType:    item.MimeType,
		Extension:   item.Extension,
		Etag:        item.Etag,
		ModifiedAt:  item.ModifiedAt,
		CreatedAt:   item.CreatedAt,
		CanPreview:  item.CanPreview,
		CanDownload: item.CanDownload,
		CanDelete:   item.CanDelete,
	}, true
}

func webDAVInnerPathFromVFSPath(mountPath string, virtualPath string) (string, bool) {
	mountPath, err := normalizeWebDAVRequestPath(mountPath)
	if err != nil {
		return "", false
	}
	virtualPath, err = normalizeWebDAVRequestPath(virtualPath)
	if err != nil {
		return "", false
	}
	if mountPath == "/" {
		return virtualPath, true
	}
	if virtualPath == mountPath {
		return "/", true
	}
	prefix := strings.TrimRight(mountPath, "/") + "/"
	if !strings.HasPrefix(virtualPath, prefix) {
		return "", false
	}
	innerPath := strings.TrimPrefix(virtualPath, strings.TrimRight(mountPath, "/"))
	if innerPath == "" {
		innerPath = "/"
	}
	return innerPath, true
}

func writeWebDAVOptions(writer http.ResponseWriter) {
	writer.Header().Set("Allow", "OPTIONS, HEAD, GET, PUT, DELETE, PROPFIND, MKCOL, COPY, MOVE")
	writer.Header().Set("DAV", "1, 2")
	writer.Header().Set("MS-Author-Via", "DAV")
	writer.WriteHeader(http.StatusOK)
}

func writeWebDAVMultiStatus(writer http.ResponseWriter, prefix string, slug string, items []appdto.FileItem) {
	writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
	writer.WriteHeader(http.StatusMultiStatus)
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	builder.WriteString(`<D:multistatus xmlns:D="DAV:">`)
	for _, item := range items {
		href := webDAVExternalHref(prefix, slug, item.Path)
		builder.WriteString(`<D:response>`)
		builder.WriteString(`<D:href>`)
		writeXMLEscaped(&builder, href)
		builder.WriteString(`</D:href>`)
		builder.WriteString(`<D:propstat><D:prop>`)
		builder.WriteString(`<D:displayname>`)
		writeXMLEscaped(&builder, item.Name)
		builder.WriteString(`</D:displayname>`)
		builder.WriteString(`<D:resourcetype>`)
		if item.IsDir {
			builder.WriteString(`<D:collection/>`)
		}
		builder.WriteString(`</D:resourcetype>`)
		if !item.IsDir {
			builder.WriteString(`<D:getcontentlength>`)
			builder.WriteString(fmt.Sprintf("%d", item.Size))
			builder.WriteString(`</D:getcontentlength>`)
		}
		builder.WriteString(`<D:getlastmodified>`)
		writeXMLEscaped(&builder, webDAVHTTPTime(item.ModifiedAt))
		builder.WriteString(`</D:getlastmodified>`)
		builder.WriteString(`<D:status>HTTP/1.1 200 OK</D:status>`)
		builder.WriteString(`</D:prop></D:propstat>`)
		builder.WriteString(`</D:response>`)
	}
	builder.WriteString(`</D:multistatus>`)
	_, _ = writer.Write([]byte(builder.String()))
}

func (h *WebDAVHandler) webDAVExternalHref(slug string, innerPath string) string {
	return webDAVExternalHref(h.prefix, slug, innerPath)
}

func webDAVExternalHref(prefix string, slug string, innerPath string) string {
	if innerPath == "" {
		innerPath = "/"
	}
	if !strings.HasPrefix(innerPath, "/") {
		innerPath = "/" + innerPath
	}
	base := strings.TrimRight(prefix, "/")
	if base == "" {
		base = "/dav"
	}
	rawPath := base + "/" + slug
	if innerPath != "/" {
		rawPath += innerPath
	}
	escaped := (&url.URL{Path: rawPath}).EscapedPath()
	if innerPath == "/" && !strings.HasSuffix(escaped, "/") {
		escaped += "/"
	}
	return escaped
}

func writeXMLEscaped(builder *strings.Builder, value string) {
	_ = xml.EscapeText(builder, []byte(value))
}

func webDAVHTTPTime(value string) string {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC().Format(http.TimeFormat)
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC().Format(http.TimeFormat)
	}
	return time.Now().UTC().Format(http.TimeFormat)
}

func webDAVStatusFromError(err error) int {
	switch {
	case errors.Is(err, appsvc.ErrACLDenied), errors.Is(err, appsvc.ErrSourceReadOnly):
		return http.StatusForbidden
	case errors.Is(err, appsvc.ErrFileNotFound), errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, appsvc.ErrFileAlreadyExists), errors.Is(err, appsvc.ErrNameConflict):
		return http.StatusConflict
	case errors.Is(err, appsvc.ErrPathInvalid), errors.Is(err, appsvc.ErrFileNameInvalid), errors.Is(err, os.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, appsvc.ErrFileIsDirectory):
		return http.StatusConflict
	case errors.Is(err, appsvc.ErrSourceOperationUnsupported), errors.Is(err, appsvc.ErrSourceDriverUnsupported):
		return http.StatusUnprocessableEntity
	case errors.Is(err, appsvc.ErrMetadataVFSCommitFailed), errors.Is(err, appsvc.ErrMetadataVFSMutationSyncFailed):
		return http.StatusInternalServerError
	case errors.Is(err, appsvc.ErrCloudAuthFailed), errors.Is(err, appsvc.ErrCloudTokenInvalid),
		errors.Is(err, appsvc.ErrCloudCaptchaRequired), errors.Is(err, appsvc.ErrCloudCaptchaExpired):
		return http.StatusUnprocessableEntity
	case errors.Is(err, appsvc.ErrCloudRateLimited):
		return http.StatusTooManyRequests
	case errors.Is(err, appsvc.ErrCloudRegionBlocked):
		return http.StatusUnavailableForLegalReasons
	case errors.Is(err, appsvc.ErrCloudProviderUnavailable):
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

func challengeWebDAV(writer http.ResponseWriter) {
	writer.Header().Set("WWW-Authenticate", `Basic realm="Yunxia WebDAV", charset="UTF-8"`)
	http.Error(writer, "unauthorized", http.StatusUnauthorized)
}

func isSecureWebDAVRequest(req *http.Request) bool {
	if req.TLS != nil {
		return true
	}
	return strings.EqualFold(req.Header.Get("X-Forwarded-Proto"), "https")
}

func normalizeWebDAVRequestPath(raw string) (string, error) {
	if raw == "" {
		return "/", nil
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	cleaned := path.Clean(raw)
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	if strings.Contains(cleaned, "..") {
		return "", os.ErrPermission
	}
	return cleaned, nil
}

func rewriteWebDAVDestination(req *http.Request, prefix, slug string) {
	raw := req.Header.Get("Destination")
	if raw == "" {
		return
	}

	parsed, err := url.Parse(raw)
	if err == nil && parsed.Path != "" {
		if rewritten := stripWebDAVExternalPrefix(parsed.Path, prefix, slug); rewritten != "" {
			parsed.Path = rewritten
			parsed.RawPath = rewritten
			req.Header.Set("Destination", parsed.String())
			return
		}
	}

	if rewritten := stripWebDAVExternalPrefix(raw, prefix, slug); rewritten != "" {
		req.Header.Set("Destination", rewritten)
	}
}

func stripWebDAVExternalPrefix(rawPath, prefix, slug string) string {
	externalPrefix := strings.TrimRight(prefix, "/") + "/" + slug
	switch {
	case rawPath == externalPrefix:
		return "/"
	case strings.HasPrefix(rawPath, externalPrefix+"/"):
		return strings.TrimPrefix(rawPath, externalPrefix)
	default:
		return ""
	}
}

func webDAVAuditAction(method string) (string, bool) {
	switch strings.ToUpper(method) {
	case http.MethodPut:
		return "put", true
	case "MKCOL":
		return "mkcol", true
	case http.MethodDelete:
		return "delete", true
	case "COPY":
		return "copy", true
	case "MOVE":
		return "move", true
	default:
		return "", false
	}
}

func classifyWebDAVAuditResult(status int) (appaudit.Result, string) {
	switch {
	case status >= http.StatusOK && status < http.StatusBadRequest:
		return appaudit.ResultSuccess, ""
	case status >= http.StatusBadRequest && status < http.StatusInternalServerError:
		return appaudit.ResultDenied, fmt.Sprintf("HTTP_%d", status)
	default:
		return appaudit.ResultFailed, fmt.Sprintf("HTTP_%d", status)
	}
}

func normalizeWebDAVDestinationPath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Path != "" {
		return normalizeWebDAVRequestPath(parsed.Path)
	}
	return normalizeWebDAVRequestPath(raw)
}
