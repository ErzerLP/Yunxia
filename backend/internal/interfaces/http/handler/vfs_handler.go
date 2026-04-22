package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/gin-gonic/gin"

	appdto "yunxia/internal/application/dto"
	appsvc "yunxia/internal/application/service"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
	httpresp "yunxia/internal/interfaces/http/response"
)

// VFSHandler 负责统一虚拟目录树的只读接口。
type VFSHandler struct {
	vfsService interface {
		List(ctx context.Context, currentPath string) (*appdto.VFSListResponse, error)
		ResolvePath(ctx context.Context, virtualPath string) (appsvc.ResolvedPath, error)
	}
	fileService interface {
		Search(ctx context.Context, query appdto.FileSearchQuery) (*appdto.FileSearchResponse, int, int, int, int, error)
		AccessURL(ctx context.Context, req appdto.AccessURLRequest) (*appdto.AccessURLResponse, error)
		ResolveDownload(ctx context.Context, sourceID uint, filePath string) (*os.File, os.FileInfo, string, error)
		ResolveDownloadRedirect(ctx context.Context, sourceID uint, filePath, disposition string) (string, error)
		ValidateFileAccessToken(raw string) (*security.FileAccessClaims, error)
		AuthenticateBearerToken(ctx context.Context, raw string) (*security.RequestAuth, error)
	}
}

// NewVFSHandler 创建统一虚拟目录 handler。
func NewVFSHandler(
	vfsService interface {
		List(ctx context.Context, currentPath string) (*appdto.VFSListResponse, error)
		ResolvePath(ctx context.Context, virtualPath string) (appsvc.ResolvedPath, error)
	},
	fileService interface {
		Search(ctx context.Context, query appdto.FileSearchQuery) (*appdto.FileSearchResponse, int, int, int, int, error)
		AccessURL(ctx context.Context, req appdto.AccessURLRequest) (*appdto.AccessURLResponse, error)
		ResolveDownload(ctx context.Context, sourceID uint, filePath string) (*os.File, os.FileInfo, string, error)
		ResolveDownloadRedirect(ctx context.Context, sourceID uint, filePath, disposition string) (string, error)
		ValidateFileAccessToken(raw string) (*security.FileAccessClaims, error)
		AuthenticateBearerToken(ctx context.Context, raw string) (*security.RequestAuth, error)
	},
) *VFSHandler {
	return &VFSHandler{
		vfsService:  vfsService,
		fileService: fileService,
	}
}

// List 返回统一虚拟目录列表。
func (h *VFSHandler) List(c *gin.Context) {
	var query appdto.VFSListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if strings.TrimSpace(query.Path) == "" {
		query.Path = "/"
	}

	resp, err := h.vfsService.List(c.Request.Context(), query.Path)
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Search 返回统一虚拟目录搜索结果。
func (h *VFSHandler) Search(c *gin.Context) {
	var query appdto.VFSSearchQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if strings.TrimSpace(query.Path) == "" {
		query.Path = "/"
	}

	resolved, err := h.vfsService.ResolvePath(c.Request.Context(), query.Path)
	if err != nil {
		h.writeError(c, err)
		return
	}

	resp, _, _, _, _, err := h.fileService.Search(c.Request.Context(), appdto.FileSearchQuery{
		SourceID:   resolved.Source.ID,
		Keyword:    query.Keyword,
		PathPrefix: resolved.InnerPath,
		Page:       query.Page,
		PageSize:   query.PageSize,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}

	items := make([]appdto.VFSItem, 0, len(resp.Items))
	for _, item := range resp.Items {
		virtualPath := mergeMountAndInnerPathForHTTP(resolved.MatchedMountPath, item.Path)
		if virtualPath == "" {
			continue
		}
		item.Path = virtualPath
		item.ParentPath = mergeMountAndInnerPathForHTTP(resolved.MatchedMountPath, item.ParentPath)
		items = append(items, buildVFSItemFromFileItemForHTTP(item, false, false))
	}

	httpresp.JSON(c, http.StatusOK, "OK", "ok", &appdto.VFSSearchResponse{
		Items:      items,
		PathPrefix: query.Path,
		Keyword:    query.Keyword,
	})
}

// AccessURL 生成统一虚拟目录的短时访问地址。
func (h *VFSHandler) AccessURL(c *gin.Context) {
	var req appdto.VFSAccessURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	resolved, err := h.vfsService.ResolvePath(c.Request.Context(), req.Path)
	if err != nil {
		h.writeError(c, err)
		return
	}

	resp, err := h.fileService.AccessURL(c.Request.Context(), appdto.AccessURLRequest{
		SourceID:    resolved.Source.ID,
		Path:        resolved.InnerPath,
		Purpose:     req.Purpose,
		Disposition: req.Disposition,
		ExpiresIn:   req.ExpiresIn,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}

	resp.URL = rewriteVFSAccessURL(resp.URL, req.Path, req.Disposition)
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Download 下载统一虚拟目录中的文件或媒体流。
func (h *VFSHandler) Download(c *gin.Context) {
	virtualPath := c.Query("path")
	disposition := c.DefaultQuery("disposition", "attachment")

	resolved, err := h.vfsService.ResolvePath(c.Request.Context(), virtualPath)
	if err != nil {
		h.writeError(c, err)
		return
	}

	tempToken := c.Query("access_token")
	requestCtx := c.Request.Context()
	if tempToken != "" {
		claims, claimErr := h.fileService.ValidateFileAccessToken(tempToken)
		if claimErr != nil || claims.SourceID != resolved.Source.ID || claims.Path != resolved.InnerPath {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "invalid access token", nil)
			return
		}
	} else {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_MISSING", "missing bearer token", nil)
			return
		}
		auth, authErr := h.fileService.AuthenticateBearerToken(c.Request.Context(), authHeader)
		if authErr != nil {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "invalid access token", nil)
			return
		}
		requestCtx = security.WithRequestAuth(requestCtx, *auth)
	}

	redirectURL, err := h.fileService.ResolveDownloadRedirect(requestCtx, resolved.Source.ID, resolved.InnerPath, disposition)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if redirectURL != "" {
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	file, info, mimeType, err := h.fileService.ResolveDownload(requestCtx, resolved.Source.ID, resolved.InnerPath)
	if err != nil {
		h.writeError(c, err)
		return
	}
	defer file.Close()

	c.Header("Content-Type", mimeType)
	c.Header("Content-Disposition", disposition+`; filename="`+info.Name()+`"`)
	c.Header("Accept-Ranges", "bytes")
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
}

func rewriteVFSAccessURL(raw string, virtualPath string, disposition string) string {
	if !strings.HasPrefix(raw, "/api/v1/files/download?") {
		return raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	values := parsed.Query()
	params := url.Values{}
	params.Set("path", virtualPath)
	if disposition != "" {
		params.Set("disposition", disposition)
	}
	if accessToken := values.Get("access_token"); accessToken != "" {
		params.Set("access_token", accessToken)
	}

	return "/api/v2/fs/download?" + params.Encode()
}

func (h *VFSHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "SOURCE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceDriverUnsupported):
		httpresp.Error(c, http.StatusUnprocessableEntity, "SOURCE_DRIVER_UNSUPPORTED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPathInvalid):
		httpresp.Error(c, http.StatusBadRequest, "PATH_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileNotFound):
		httpresp.Error(c, http.StatusNotFound, "FILE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileIsDirectory):
		httpresp.Error(c, http.StatusUnprocessableEntity, "FILE_IS_DIRECTORY", err.Error(), nil)
	case errors.Is(err, appsvc.ErrACLDenied):
		httpresp.Error(c, http.StatusForbidden, "ACL_DENIED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrNoBackingStorage):
		httpresp.Error(c, http.StatusConflict, "NO_BACKING_STORAGE", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}

func mergeMountAndInnerPathForHTTP(mountPath string, innerPath string) string {
	if mountPath == "" {
		return ""
	}
	if strings.TrimSpace(innerPath) == "" || innerPath == "/" {
		return mountPath
	}
	if mountPath == "/" {
		return innerPath
	}
	return mountPath + innerPath
}

func buildVFSItemFromFileItemForHTTP(item appdto.FileItem, isVirtual bool, isMountPoint bool) appdto.VFSItem {
	entryKind := "file"
	if item.IsDir {
		entryKind = "directory"
	}

	return appdto.VFSItem{
		Name:         item.Name,
		Path:         item.Path,
		ParentPath:   item.ParentPath,
		SourceID:     &item.SourceID,
		EntryKind:    entryKind,
		IsVirtual:    isVirtual,
		IsMountPoint: isMountPoint,
		Size:         item.Size,
		MimeType:     item.MimeType,
		Extension:    item.Extension,
		ModifiedAt:   item.ModifiedAt,
		CreatedAt:    item.CreatedAt,
		Etag:         item.Etag,
		CanPreview:   item.CanPreview,
		CanDownload:  item.CanDownload,
		CanDelete:    item.CanDelete,
	}
}

func joinVirtualPathForHTTP(parentPath string, name string) string {
	if parentPath == "/" {
		return "/" + strings.TrimPrefix(name, "/")
	}
	return parentPath + "/" + strings.TrimPrefix(name, "/")
}

func parentVirtualPathForHTTP(pathValue string) string {
	parentPath := path.Dir(pathValue)
	if parentPath == "." {
		return "/"
	}
	return parentPath
}
