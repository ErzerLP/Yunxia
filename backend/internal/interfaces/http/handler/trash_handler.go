package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appdto "yunxia/internal/application/dto"
	appsvc "yunxia/internal/application/service"
	domainrepo "yunxia/internal/domain/repository"
	httpresp "yunxia/internal/interfaces/http/response"
)

// TrashHandler 负责回收站接口。
type TrashHandler struct {
	service interface {
		List(ctx context.Context, query appdto.TrashListQuery) (*appdto.TrashListResponse, error)
		Restore(ctx context.Context, id uint) (*appdto.TrashRestoreResponse, error)
		Delete(ctx context.Context, id uint) (*appdto.TrashDeleteResponse, error)
		ClearSource(ctx context.Context, sourceID uint) (*appdto.TrashDeleteResponse, error)
	}
}

// NewTrashHandler 创建回收站 handler。
func NewTrashHandler(service interface {
	List(ctx context.Context, query appdto.TrashListQuery) (*appdto.TrashListResponse, error)
	Restore(ctx context.Context, id uint) (*appdto.TrashRestoreResponse, error)
	Delete(ctx context.Context, id uint) (*appdto.TrashDeleteResponse, error)
	ClearSource(ctx context.Context, sourceID uint) (*appdto.TrashDeleteResponse, error)
}) *TrashHandler {
	return &TrashHandler{service: service}
}

// List 返回回收站列表。
func (h *TrashHandler) List(c *gin.Context) {
	var query appdto.TrashListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	resp, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		h.writeError(c, err, "SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Restore 恢复回收站项。
func (h *TrashHandler) Restore(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	resp, svcErr := h.service.Restore(c.Request.Context(), uint(id))
	if svcErr != nil {
		h.writeError(c, svcErr, "TRASH_ITEM_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Delete 永久删除回收站项。
func (h *TrashHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	resp, svcErr := h.service.Delete(c.Request.Context(), uint(id))
	if svcErr != nil {
		h.writeError(c, svcErr, "TRASH_ITEM_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Clear 清空指定 source 的回收站。
func (h *TrashHandler) Clear(c *gin.Context) {
	sourceIDValue, err := strconv.ParseUint(c.Query("source_id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid source_id", nil)
		return
	}

	resp, svcErr := h.service.ClearSource(c.Request.Context(), uint(sourceIDValue))
	if svcErr != nil {
		h.writeError(c, svcErr, "SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *TrashHandler) writeError(c *gin.Context, err error, notFoundCode string) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, notFoundCode, err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileAlreadyExists), errors.Is(err, appsvc.ErrFileMoveConflict):
		httpresp.Error(c, http.StatusConflict, "TRASH_RESTORE_CONFLICT", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileNotFound):
		httpresp.Error(c, http.StatusNotFound, "FILE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPathInvalid):
		httpresp.Error(c, http.StatusBadRequest, "PATH_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceDriverUnsupported):
		httpresp.Error(c, http.StatusUnprocessableEntity, "SOURCE_DRIVER_UNSUPPORTED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceOperationUnsupported):
		httpresp.Error(c, http.StatusUnprocessableEntity, "SOURCE_OPERATION_UNSUPPORTED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrACLDenied):
		httpresp.Error(c, http.StatusForbidden, "ACL_DENIED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrCloudAuthFailed):
		httpresp.Error(c, http.StatusUnprocessableEntity, "CLOUD_AUTH_FAILED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrCloudTokenInvalid):
		httpresp.Error(c, http.StatusUnprocessableEntity, "CLOUD_TOKEN_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrCloudCaptchaRequired), errors.Is(err, appsvc.ErrCloudCaptchaExpired):
		httpresp.Error(c, http.StatusUnprocessableEntity, "CLOUD_CAPTCHA_REQUIRED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrCloudRateLimited):
		httpresp.Error(c, http.StatusTooManyRequests, "CLOUD_RATE_LIMITED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrCloudRegionBlocked):
		httpresp.Error(c, http.StatusUnavailableForLegalReasons, "CLOUD_REGION_BLOCKED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrCloudProviderUnavailable):
		httpresp.Error(c, http.StatusBadGateway, "CLOUD_PROVIDER_UNAVAILABLE", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}
