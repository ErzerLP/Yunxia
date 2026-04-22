package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appdto "yunxia/internal/application/dto"
	appsvc "yunxia/internal/application/service"
	domainrepo "yunxia/internal/domain/repository"
	httpresp "yunxia/internal/interfaces/http/response"
)

// UploadHandler 负责上传接口。
type UploadHandler struct {
	service interface {
		Init(ctx context.Context, userID uint, req appdto.UploadInitRequest) (*appdto.UploadInitResponse, error)
		UploadChunk(ctx context.Context, uploadID string, index int, data []byte) (*appdto.UploadChunkResponse, error)
		Finish(ctx context.Context, req appdto.UploadFinishRequest) (*appdto.UploadFinishResponse, error)
		ListSessions(ctx context.Context, userID uint, sourceID *uint, status string) (*appdto.UploadSessionListResponse, error)
		Cancel(ctx context.Context, uploadID string) error
	}
}

// NewUploadHandler 创建上传 handler。
func NewUploadHandler(service interface {
	Init(ctx context.Context, userID uint, req appdto.UploadInitRequest) (*appdto.UploadInitResponse, error)
	UploadChunk(ctx context.Context, uploadID string, index int, data []byte) (*appdto.UploadChunkResponse, error)
	Finish(ctx context.Context, req appdto.UploadFinishRequest) (*appdto.UploadFinishResponse, error)
	ListSessions(ctx context.Context, userID uint, sourceID *uint, status string) (*appdto.UploadSessionListResponse, error)
	Cancel(ctx context.Context, uploadID string) error
}) *UploadHandler {
	return &UploadHandler{service: service}
}

// Init 初始化上传。
func (h *UploadHandler) Init(c *gin.Context) {
	var req appdto.UploadInitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	userID := c.MustGet("user_id").(uint)
	resp, svcErr := h.service.Init(c.Request.Context(), userID, req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// UploadChunk 上传分片。
func (h *UploadHandler) UploadChunk(c *gin.Context) {
	uploadID := c.Query("upload_id")
	index, err := strconv.Atoi(c.Query("index"))
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid chunk index", nil)
		return
	}
	data, readErr := io.ReadAll(c.Request.Body)
	if readErr != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", readErr.Error(), nil)
		return
	}
	resp, svcErr := h.service.UploadChunk(c.Request.Context(), uploadID, index, data)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Finish 完成上传。
func (h *UploadHandler) Finish(c *gin.Context) {
	var req appdto.UploadFinishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Finish(c.Request.Context(), req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusCreated, "OK", "ok", resp)
}

// ListSessions 返回上传会话。
func (h *UploadHandler) ListSessions(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var sourceID *uint
	if value := c.Query("source_id"); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid source_id", nil)
			return
		}
		converted := uint(parsed)
		sourceID = &converted
	}
	resp, svcErr := h.service.ListSessions(c.Request.Context(), userID, sourceID, c.Query("status"))
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Cancel 取消上传会话。
func (h *UploadHandler) Cancel(c *gin.Context) {
	if svcErr := h.service.Cancel(c.Request.Context(), c.Param("upload_id")); svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{
		"upload_id": c.Param("upload_id"),
		"canceled":  true,
	})
}

func (h *UploadHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "SOURCE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrUploadSessionNotFound):
		httpresp.Error(c, http.StatusNotFound, "UPLOAD_SESSION_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrUploadChunkConflict):
		httpresp.Error(c, http.StatusConflict, "UPLOAD_CHUNK_CONFLICT", err.Error(), nil)
	case errors.Is(err, appsvc.ErrUploadFinishIncomplete):
		httpresp.Error(c, http.StatusConflict, "UPLOAD_FINISH_INCOMPLETE", err.Error(), nil)
	case errors.Is(err, appsvc.ErrUploadHashMismatch):
		httpresp.Error(c, http.StatusConflict, "UPLOAD_HASH_MISMATCH", err.Error(), nil)
	case errors.Is(err, appsvc.ErrUploadInvalidState):
		httpresp.Error(c, http.StatusConflict, "UPLOAD_INVALID_STATE", err.Error(), nil)
	case errors.Is(err, appsvc.ErrUploadTooLarge):
		httpresp.Error(c, http.StatusUnprocessableEntity, "UPLOAD_TOO_LARGE", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceDriverUnsupported):
		httpresp.Error(c, http.StatusUnprocessableEntity, "SOURCE_DRIVER_UNSUPPORTED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrNoBackingStorage):
		httpresp.Error(c, http.StatusConflict, "NO_BACKING_STORAGE", err.Error(), nil)
	case errors.Is(err, appsvc.ErrNameConflict):
		httpresp.Error(c, http.StatusConflict, "NAME_CONFLICT", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPathInvalid):
		httpresp.Error(c, http.StatusBadRequest, "PATH_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileAlreadyExists):
		httpresp.Error(c, http.StatusConflict, "FILE_ALREADY_EXISTS", err.Error(), nil)
	case errors.Is(err, appsvc.ErrACLDenied):
		httpresp.Error(c, http.StatusForbidden, "ACL_DENIED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPermissionDenied):
		httpresp.Error(c, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}
