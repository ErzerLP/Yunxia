package handler

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/webdav"

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

type localWebDAVConfig struct {
	BasePath string `json:"base_path"`
}

type webDAVFileService interface {
	List(ctx context.Context, query appdto.FileListQuery) (*appdto.FileListResponse, int, int, int, int, error)
	Stat(ctx context.Context, sourceID uint, filePath string) (*appdto.FileItem, error)
	Mkdir(ctx context.Context, req appdto.MkdirRequest) (*appdto.FileItem, error)
	Rename(ctx context.Context, req appdto.RenameRequest) (string, string, *appdto.FileItem, error)
	Move(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error)
	Copy(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error)
	Delete(ctx context.Context, req appdto.DeleteFileRequest) (time.Time, error)
	ResolveDownloadRedirect(ctx context.Context, sourceID uint, filePath, disposition string) (string, error)
}

type webDAVUploadService interface {
	ImportLocalFile(ctx context.Context, sourceID uint, parentPath string, filename string, localPath string) (*appdto.FileItem, error)
}

// WebDAVHandler 负责存储源的 WebDAV 暴露。
type WebDAVHandler struct {
	prefix           string
	sourceRepo       domainrepo.SourceRepository
	systemConfigRepo domainrepo.SystemConfigRepository
	userRepo         domainrepo.UserRepository
	aclAuthorizer    *appsvc.ACLAuthorizer
	fileService      webDAVFileService
	uploadService    webDAVUploadService
	hasher           passwordComparer
	lockSystem       webdav.LockSystem
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
		fileService:      fileService,
		uploadService:    uploadService,
		hasher:           hasher,
		lockSystem:       webdav.NewMemLS(),
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
	if err := h.authorizeRequest(req.Context(), source.ID, req.Method, webdavPath, req.Header.Get("Destination")); err != nil {
		if errors.Is(err, appsvc.ErrACLDenied) {
			http.Error(c.Writer, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(c.Writer, "internal error", http.StatusInternalServerError)
		return
	}

	if source.DriverType != "local" {
		h.serveDriverWebDAV(c, req, source, webdavPath)
		return
	}

	rootDir, err := resolveLocalWebDAVRoot(source)
	if err != nil {
		http.Error(c.Writer, "internal error", http.StatusInternalServerError)
		return
	}

	var fileSystem webdav.FileSystem = webdav.Dir(rootDir)
	if source.WebDAVReadOnly {
		fileSystem = readOnlyWebDAVFileSystem{delegate: fileSystem}
	}

	(&webdav.Handler{
		FileSystem: fileSystem,
		LockSystem: h.lockSystem,
	}).ServeHTTP(c.Writer, req)
}

func (h *WebDAVHandler) serveDriverWebDAV(c *gin.Context, req *http.Request, source *entity.StorageSource, webdavPath string) {
	if h.fileService == nil {
		http.Error(c.Writer, "webdav storage driver unavailable", http.StatusNotImplemented)
		return
	}

	switch strings.ToUpper(req.Method) {
	case http.MethodOptions:
		writeWebDAVOptions(c.Writer)
	case "PROPFIND":
		h.handleDriverPropfind(c.Writer, req, source, webdavPath)
	case http.MethodHead:
		h.handleDriverDownload(c.Writer, req, source, webdavPath, true)
	case http.MethodGet:
		h.handleDriverDownload(c.Writer, req, source, webdavPath, false)
	case "MKCOL":
		if !h.ensureWebDAVWritable(c.Writer, source) {
			return
		}
		h.handleDriverMKCOL(c.Writer, req, source, webdavPath)
	case http.MethodPut:
		if !h.ensureWebDAVWritable(c.Writer, source) {
			return
		}
		h.handleDriverPUT(c.Writer, req, source, webdavPath)
	case http.MethodDelete:
		if !h.ensureWebDAVWritable(c.Writer, source) {
			return
		}
		h.handleDriverDELETE(c.Writer, req, source, webdavPath)
	case "COPY":
		if !h.ensureWebDAVWritable(c.Writer, source) {
			return
		}
		h.handleDriverCopyMove(c.Writer, req, source, webdavPath, false)
	case "MOVE":
		if !h.ensureWebDAVWritable(c.Writer, source) {
			return
		}
		h.handleDriverCopyMove(c.Writer, req, source, webdavPath, true)
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

func (h *WebDAVHandler) handleDriverPropfind(writer http.ResponseWriter, req *http.Request, source *entity.StorageSource, webdavPath string) {
	item, err := h.driverWebDAVItem(req.Context(), source, webdavPath)
	if err != nil {
		http.Error(writer, "not found", webDAVStatusFromError(err))
		return
	}

	items := []appdto.FileItem{*item}
	depth := strings.TrimSpace(req.Header.Get("Depth"))
	if item.IsDir && depth != "0" {
		resp, _, _, _, _, err := h.fileService.List(req.Context(), appdto.FileListQuery{
			SourceID:  source.ID,
			Path:      webdavPath,
			Page:      1,
			PageSize:  10000,
			SortBy:    "name",
			SortOrder: "asc",
		})
		if err != nil {
			http.Error(writer, "not found", webDAVStatusFromError(err))
			return
		}
		items = append(items, resp.Items...)
	}
	writeWebDAVMultiStatus(writer, h.prefix, source.WebDAVSlug, items)
}

func (h *WebDAVHandler) driverWebDAVItem(ctx context.Context, source *entity.StorageSource, webdavPath string) (*appdto.FileItem, error) {
	if webdavPath == "/" {
		return &appdto.FileItem{
			Name:       source.WebDAVSlug,
			Path:       "/",
			SourceID:   source.ID,
			IsDir:      true,
			ModifiedAt: time.Now().UTC().Format(time.RFC3339),
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		}, nil
	}
	return h.fileService.Stat(ctx, source.ID, webdavPath)
}

func (h *WebDAVHandler) handleDriverDownload(writer http.ResponseWriter, req *http.Request, source *entity.StorageSource, webdavPath string, headOnly bool) {
	redirectURL, err := h.fileService.ResolveDownloadRedirect(req.Context(), source.ID, webdavPath, "inline")
	if err != nil {
		http.Error(writer, "not found", webDAVStatusFromError(err))
		return
	}
	if redirectURL == "" {
		http.Error(writer, "download unavailable", http.StatusNotImplemented)
		return
	}
	writer.Header().Set("Location", redirectURL)
	if headOnly {
		writer.WriteHeader(http.StatusFound)
		return
	}
	http.Redirect(writer, req, redirectURL, http.StatusFound)
}

func (h *WebDAVHandler) handleDriverMKCOL(writer http.ResponseWriter, req *http.Request, source *entity.StorageSource, webdavPath string) {
	parentPath, name, err := splitWebDAVTarget(webdavPath)
	if err != nil {
		http.Error(writer, "bad request", http.StatusBadRequest)
		return
	}
	if _, err := h.fileService.Mkdir(req.Context(), appdto.MkdirRequest{
		SourceID:   source.ID,
		ParentPath: parentPath,
		Name:       name,
	}); err != nil {
		http.Error(writer, "webdav mkdir failed", webDAVStatusFromError(err))
		return
	}
	writer.WriteHeader(http.StatusCreated)
}

func (h *WebDAVHandler) handleDriverPUT(writer http.ResponseWriter, req *http.Request, source *entity.StorageSource, webdavPath string) {
	if h.uploadService == nil {
		http.Error(writer, "webdav upload unavailable", http.StatusNotImplemented)
		return
	}
	parentPath, name, err := splitWebDAVTarget(webdavPath)
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

func (h *WebDAVHandler) handleDriverDELETE(writer http.ResponseWriter, req *http.Request, source *entity.StorageSource, webdavPath string) {
	if _, err := h.fileService.Delete(req.Context(), appdto.DeleteFileRequest{
		SourceID:   source.ID,
		Path:       webdavPath,
		DeleteMode: "trash",
	}); err != nil {
		http.Error(writer, "webdav delete failed", webDAVStatusFromError(err))
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *WebDAVHandler) handleDriverCopyMove(writer http.ResponseWriter, req *http.Request, source *entity.StorageSource, webdavPath string, move bool) {
	destinationPath, err := h.normalizedDriverDestinationPath(req, source)
	if err != nil {
		http.Error(writer, "bad destination", http.StatusBadRequest)
		return
	}
	if webdavPath == "/" || destinationPath == "/" {
		http.Error(writer, "bad request", http.StatusBadRequest)
		return
	}
	destinationParent := path.Dir(destinationPath)
	if destinationParent == "." {
		destinationParent = "/"
	}
	destinationName := path.Base(destinationPath)
	sourceName := path.Base(webdavPath)

	var newPath string
	if move {
		_, movedPath, err := h.fileService.Move(req.Context(), appdto.MoveCopyRequest{
			SourceID:   source.ID,
			Path:       webdavPath,
			TargetPath: destinationParent,
		})
		if err != nil {
			http.Error(writer, "webdav move failed", webDAVStatusFromError(err))
			return
		}
		newPath = movedPath
	} else {
		_, copiedPath, err := h.fileService.Copy(req.Context(), appdto.MoveCopyRequest{
			SourceID:   source.ID,
			Path:       webdavPath,
			TargetPath: destinationParent,
		})
		if err != nil {
			http.Error(writer, "webdav copy failed", webDAVStatusFromError(err))
			return
		}
		newPath = copiedPath
	}

	if destinationName != sourceName {
		_, renamedPath, _, err := h.fileService.Rename(req.Context(), appdto.RenameRequest{
			SourceID: source.ID,
			Path:     newPath,
			NewName:  destinationName,
		})
		if err != nil {
			http.Error(writer, "webdav rename failed", webDAVStatusFromError(err))
			return
		}
		newPath = renamedPath
	}
	if newPath != destinationPath {
		writer.Header().Set("Content-Location", h.webDAVExternalHref(source.WebDAVSlug, newPath))
	}
	if move {
		writer.WriteHeader(http.StatusCreated)
		return
	}
	writer.WriteHeader(http.StatusCreated)
}

func (h *WebDAVHandler) normalizedDriverDestinationPath(req *http.Request, source *entity.StorageSource) (string, error) {
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

func (h *WebDAVHandler) authorizeRequest(ctx context.Context, sourceID uint, method string, requestPath string, destination string) error {
	if h.aclAuthorizer == nil {
		return nil
	}
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, "PROPFIND":
		return h.aclAuthorizer.AuthorizePath(ctx, sourceID, requestPath, appsvc.ACLActionRead)
	case http.MethodPut, "MKCOL":
		return h.aclAuthorizer.AuthorizePath(ctx, sourceID, requestPath, appsvc.ACLActionWrite)
	case http.MethodDelete:
		return h.aclAuthorizer.AuthorizePath(ctx, sourceID, requestPath, appsvc.ACLActionDelete)
	case "COPY":
		if err := h.aclAuthorizer.AuthorizePath(ctx, sourceID, requestPath, appsvc.ACLActionRead); err != nil {
			return err
		}
		if destination == "" {
			return nil
		}
		return h.aclAuthorizer.AuthorizePath(ctx, sourceID, destination, appsvc.ACLActionWrite)
	case "MOVE":
		if err := h.aclAuthorizer.AuthorizePath(ctx, sourceID, requestPath, appsvc.ACLActionWrite); err != nil {
			return err
		}
		if destination == "" {
			return nil
		}
		return h.aclAuthorizer.AuthorizePath(ctx, sourceID, destination, appsvc.ACLActionWrite)
	default:
		return nil
	}
}

type readOnlyWebDAVFileSystem struct {
	delegate webdav.FileSystem
}

func (fs readOnlyWebDAVFileSystem) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return os.ErrPermission
}

func (fs readOnlyWebDAVFileSystem) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	writeFlags := os.O_WRONLY | os.O_RDWR | os.O_APPEND | os.O_CREATE | os.O_TRUNC
	if flag&writeFlags != 0 {
		return nil, os.ErrPermission
	}
	return fs.delegate.OpenFile(ctx, name, flag, perm)
}

func (fs readOnlyWebDAVFileSystem) RemoveAll(ctx context.Context, name string) error {
	return os.ErrPermission
}

func (fs readOnlyWebDAVFileSystem) Rename(ctx context.Context, oldName string, newName string) error {
	return os.ErrPermission
}

func (fs readOnlyWebDAVFileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	return fs.delegate.Stat(ctx, name)
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

func resolveLocalWebDAVRoot(source *entity.StorageSource) (string, error) {
	var cfg localWebDAVConfig
	if err := json.Unmarshal([]byte(source.ConfigJSON), &cfg); err != nil {
		return "", err
	}
	if cfg.BasePath == "" {
		return "", domainrepo.ErrNotFound
	}

	rootPath, err := normalizeWebDAVRequestPath(source.RootPath)
	if err != nil {
		return "", err
	}

	baseDir := filepath.Clean(cfg.BasePath)
	rootDir := filepath.Join(baseDir, filepath.FromSlash(strings.TrimPrefix(rootPath, "/")))
	if err := ensureSubPath(baseDir, rootDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return "", err
	}
	return rootDir, nil
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

func ensureSubPath(baseDir, target string) error {
	rel, err := filepath.Rel(filepath.Clean(baseDir), filepath.Clean(target))
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return os.ErrPermission
	}
	return nil
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
