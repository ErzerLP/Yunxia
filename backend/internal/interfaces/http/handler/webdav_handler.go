package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/webdav"

	appsvc "yunxia/internal/application/service"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

type passwordComparer interface {
	Compare(hash, password string) bool
}

type localWebDAVConfig struct {
	BasePath string `json:"base_path"`
}

// WebDAVHandler 负责 local 存储源的 WebDAV 暴露。
type WebDAVHandler struct {
	prefix           string
	sourceRepo       domainrepo.SourceRepository
	systemConfigRepo domainrepo.SystemConfigRepository
	userRepo         domainrepo.UserRepository
	aclAuthorizer    *appsvc.ACLAuthorizer
	hasher           passwordComparer
	lockSystem       webdav.LockSystem
}

// NewWebDAVHandler 创建 WebDAV handler。
func NewWebDAVHandler(
	prefix string,
	sourceRepo domainrepo.SourceRepository,
	systemConfigRepo domainrepo.SystemConfigRepository,
	userRepo domainrepo.UserRepository,
	aclAuthorizer *appsvc.ACLAuthorizer,
	hasher passwordComparer,
) *WebDAVHandler {
	return &WebDAVHandler{
		prefix:           prefix,
		sourceRepo:       sourceRepo,
		systemConfigRepo: systemConfigRepo,
		userRepo:         userRepo,
		aclAuthorizer:    aclAuthorizer,
		hasher:           hasher,
		lockSystem:       webdav.NewMemLS(),
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
	if !source.IsEnabled || !source.IsWebDAVExposed || source.DriverType != "local" {
		http.NotFound(c.Writer, c.Request)
		return
	}

	user, authErr := h.authenticate(c.Request)
	if authErr != nil {
		challengeWebDAV(c.Writer)
		return
	}

	rootDir, err := resolveLocalWebDAVRoot(source)
	if err != nil {
		http.Error(c.Writer, "internal error", http.StatusInternalServerError)
		return
	}

	webdavPath, err := normalizeWebDAVRequestPath(c.Param("filepath"))
	if err != nil {
		http.Error(c.Writer, "bad request", http.StatusBadRequest)
		return
	}

	requestCtx := security.WithRequestAuth(c.Request.Context(), security.RequestAuth{
		UserID: user.ID,
		Role:   user.Role,
	})

	req := cloneRequest(c.Request.WithContext(requestCtx))
	req.URL.Path = webdavPath
	req.URL.RawPath = webdavPath
	rewriteWebDAVDestination(req, h.prefix, source.WebDAVSlug)
	if err := h.authorizeRequest(req.Context(), source.ID, req.Method, webdavPath, req.Header.Get("Destination")); err != nil {
		if errors.Is(err, appsvc.ErrACLDenied) {
			http.Error(c.Writer, "forbidden", http.StatusForbidden)
			return
		}
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
	if user.IsLocked || !h.hasher.Compare(user.PasswordHash, password) {
		return nil, domainrepo.ErrNotFound
	}
	return user, nil
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
