package handler

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	appdto "yunxia/internal/application/dto"
	appsvc "yunxia/internal/application/service"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
	httpresp "yunxia/internal/interfaces/http/response"
)

// FileHandler 负责文件管理接口。
type FileHandler struct {
	service interface {
		List(ctx context.Context, query appdto.FileListQuery) (*appdto.FileListResponse, int, int, int, int, error)
		Search(ctx context.Context, query appdto.FileSearchQuery) (*appdto.FileSearchResponse, int, int, int, int, error)
		Mkdir(ctx context.Context, req appdto.MkdirRequest) (*appdto.FileItem, error)
		Rename(ctx context.Context, req appdto.RenameRequest) (string, string, *appdto.FileItem, error)
		Move(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error)
		Copy(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error)
		Delete(ctx context.Context, req appdto.DeleteFileRequest) (time.Time, error)
		AccessURL(ctx context.Context, req appdto.AccessURLRequest) (*appdto.AccessURLResponse, error)
		ResolveDownload(ctx context.Context, sourceID uint, filePath string) (*os.File, os.FileInfo, string, error)
		ResolveDownloadRedirect(ctx context.Context, sourceID uint, filePath, disposition string) (string, error)
		ValidateFileAccessToken(raw string) (*security.FileAccessClaims, error)
		AuthenticateBearerToken(ctx context.Context, raw string) (*security.RequestAuth, error)
	}
}

// NewFileHandler 创建文件 handler。
func NewFileHandler(service interface {
	List(ctx context.Context, query appdto.FileListQuery) (*appdto.FileListResponse, int, int, int, int, error)
	Search(ctx context.Context, query appdto.FileSearchQuery) (*appdto.FileSearchResponse, int, int, int, int, error)
	Mkdir(ctx context.Context, req appdto.MkdirRequest) (*appdto.FileItem, error)
	Rename(ctx context.Context, req appdto.RenameRequest) (string, string, *appdto.FileItem, error)
	Move(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error)
	Copy(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error)
	Delete(ctx context.Context, req appdto.DeleteFileRequest) (time.Time, error)
	AccessURL(ctx context.Context, req appdto.AccessURLRequest) (*appdto.AccessURLResponse, error)
	ResolveDownload(ctx context.Context, sourceID uint, filePath string) (*os.File, os.FileInfo, string, error)
	ResolveDownloadRedirect(ctx context.Context, sourceID uint, filePath, disposition string) (string, error)
	ValidateFileAccessToken(raw string) (*security.FileAccessClaims, error)
	AuthenticateBearerToken(ctx context.Context, raw string) (*security.RequestAuth, error)
}) *FileHandler {
	return &FileHandler{service: service}
}

// List 返回文件列表。
func (h *FileHandler) List(c *gin.Context) {
	var query appdto.FileListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, _, _, _, _, svcErr := h.service.List(c.Request.Context(), query)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Search 返回搜索结果。
func (h *FileHandler) Search(c *gin.Context) {
	var query appdto.FileSearchQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, _, _, _, _, svcErr := h.service.Search(c.Request.Context(), query)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Mkdir 创建目录。
func (h *FileHandler) Mkdir(c *gin.Context) {
	var req appdto.MkdirRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	item, svcErr := h.service.Mkdir(c.Request.Context(), req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"created": item})
}

// Rename 重命名资源。
func (h *FileHandler) Rename(c *gin.Context) {
	var req appdto.RenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	oldPath, newPath, item, svcErr := h.service.Rename(c.Request.Context(), req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{
		"old_path": oldPath,
		"new_path": newPath,
		"file":     item,
	})
}

// Move 移动资源。
func (h *FileHandler) Move(c *gin.Context) {
	var req appdto.MoveCopyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	oldPath, newPath, svcErr := h.service.Move(c.Request.Context(), req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{
		"old_path": oldPath,
		"new_path": newPath,
		"moved":    true,
	})
}

// Copy 复制资源。
func (h *FileHandler) Copy(c *gin.Context) {
	var req appdto.MoveCopyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	sourcePath, newPath, svcErr := h.service.Copy(c.Request.Context(), req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{
		"source_path": sourcePath,
		"new_path":    newPath,
		"copied":      true,
	})
}

// Delete 删除资源。
func (h *FileHandler) Delete(c *gin.Context) {
	var req appdto.DeleteFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	deletedAt, svcErr := h.service.Delete(c.Request.Context(), req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	deleteMode := req.DeleteMode
	if deleteMode == "" {
		deleteMode = "trash"
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{
		"deleted":     true,
		"delete_mode": deleteMode,
		"path":        req.Path,
		"deleted_at":  deletedAt.Format(time.RFC3339),
	})
}

// AccessURL 生成访问地址。
func (h *FileHandler) AccessURL(c *gin.Context) {
	var req appdto.AccessURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.AccessURL(c.Request.Context(), req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Download 下载文件或媒体流。
func (h *FileHandler) Download(c *gin.Context) {
	sourceIDValue, err := strconv.ParseUint(c.Query("source_id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid source_id", nil)
		return
	}
	filePath := c.Query("path")
	disposition := c.DefaultQuery("disposition", "attachment")

	tempToken := c.Query("access_token")
	requestCtx := c.Request.Context()
	if tempToken != "" {
		claims, claimErr := h.service.ValidateFileAccessToken(tempToken)
		if claimErr != nil || claims.SourceID != uint(sourceIDValue) || claims.Path != filePath {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "invalid access token", nil)
			return
		}
	} else {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_MISSING", "missing bearer token", nil)
			return
		}
		auth, authErr := h.service.AuthenticateBearerToken(c.Request.Context(), authHeader)
		if authErr != nil {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "invalid access token", nil)
			return
		}
		requestCtx = security.WithRequestAuth(requestCtx, *auth)
	}

	redirectURL, svcErr := h.service.ResolveDownloadRedirect(requestCtx, uint(sourceIDValue), filePath, disposition)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	if redirectURL != "" {
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	file, info, mimeType, svcErr := h.service.ResolveDownload(requestCtx, uint(sourceIDValue), filePath)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	defer file.Close()

	c.Header("Content-Type", mimeType)
	c.Header("Content-Disposition", disposition+`; filename="`+info.Name()+`"`)
	c.Header("Accept-Ranges", "bytes")
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
}

func (h *FileHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "SOURCE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceDriverUnsupported):
		httpresp.Error(c, http.StatusUnprocessableEntity, "SOURCE_DRIVER_UNSUPPORTED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPathInvalid):
		httpresp.Error(c, http.StatusBadRequest, "PATH_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileNotFound):
		httpresp.Error(c, http.StatusNotFound, "FILE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileAlreadyExists):
		httpresp.Error(c, http.StatusConflict, "FILE_ALREADY_EXISTS", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileNameInvalid):
		httpresp.Error(c, http.StatusUnprocessableEntity, "FILE_NAME_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileMoveConflict):
		httpresp.Error(c, http.StatusConflict, "FILE_MOVE_CONFLICT", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileCopyConflict):
		httpresp.Error(c, http.StatusConflict, "FILE_COPY_CONFLICT", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileIsDirectory):
		httpresp.Error(c, http.StatusUnprocessableEntity, "FILE_IS_DIRECTORY", err.Error(), nil)
	case errors.Is(err, appsvc.ErrACLDenied):
		httpresp.Error(c, http.StatusForbidden, "ACL_DENIED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceReadOnly):
		httpresp.Error(c, http.StatusForbidden, "SOURCE_READ_ONLY", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}
