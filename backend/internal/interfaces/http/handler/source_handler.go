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

// SourceHandler 负责存储源接口。
type SourceHandler struct {
	service interface {
		List(ctx context.Context, view string) (*appdto.SourceListResponse, error)
		Get(ctx context.Context, id uint) (*appdto.SourceDetailResponse, error)
		Test(ctx context.Context, req appdto.SourceUpsertRequest) (*appdto.SourceTestResponse, error)
		Retest(ctx context.Context, id uint) (*appdto.SourceTestResponse, error)
		Create(ctx context.Context, req appdto.SourceUpsertRequest) (*appdto.StorageSourceView, error)
		Update(ctx context.Context, id uint, req appdto.SourceUpsertRequest) (*appdto.StorageSourceView, error)
		Delete(ctx context.Context, id uint) error
	}
}

// NewSourceHandler 创建存储源 handler。
func NewSourceHandler(service interface {
	List(ctx context.Context, view string) (*appdto.SourceListResponse, error)
	Get(ctx context.Context, id uint) (*appdto.SourceDetailResponse, error)
	Test(ctx context.Context, req appdto.SourceUpsertRequest) (*appdto.SourceTestResponse, error)
	Retest(ctx context.Context, id uint) (*appdto.SourceTestResponse, error)
	Create(ctx context.Context, req appdto.SourceUpsertRequest) (*appdto.StorageSourceView, error)
	Update(ctx context.Context, id uint, req appdto.SourceUpsertRequest) (*appdto.StorageSourceView, error)
	Delete(ctx context.Context, id uint) error
}) *SourceHandler {
	return &SourceHandler{service: service}
}

// List 返回存储源列表。
func (h *SourceHandler) List(c *gin.Context) {
	view := c.DefaultQuery("view", "navigation")
	if view == "admin" && c.GetString("user_role") != "admin" {
		httpresp.Error(c, http.StatusForbidden, "ROLE_FORBIDDEN", "admin role required", nil)
		return
	}
	resp, err := h.service.List(c.Request.Context(), view)
	if err != nil {
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Get 返回单个存储源详情。
func (h *SourceHandler) Get(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Get(c.Request.Context(), id)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Test 测试存储源配置。
func (h *SourceHandler) Test(c *gin.Context) {
	var req appdto.SourceUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Test(c.Request.Context(), req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Retest 重新测试已保存存储源。
func (h *SourceHandler) Retest(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Retest(c.Request.Context(), id)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Create 创建存储源。
func (h *SourceHandler) Create(c *gin.Context) {
	var req appdto.SourceUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Create(c.Request.Context(), req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusCreated, "OK", "ok", gin.H{"source": resp})
}

// Update 更新存储源。
func (h *SourceHandler) Update(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.SourceUpsertRequest
	if bindErr := c.ShouldBindJSON(&req); bindErr != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", bindErr.Error(), nil)
		return
	}
	resp, svcErr := h.service.Update(c.Request.Context(), id, req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"source": resp})
}

// Delete 删除存储源。
func (h *SourceHandler) Delete(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if svcErr := h.service.Delete(c.Request.Context(), id); svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"deleted": true, "id": id})
}

func (h *SourceHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "SOURCE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceDriverUnsupported):
		httpresp.Error(c, http.StatusUnprocessableEntity, "SOURCE_DRIVER_UNSUPPORTED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrConfigInvalid):
		httpresp.Error(c, http.StatusUnprocessableEntity, "CONFIG_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceConnectionFailed):
		httpresp.Error(c, http.StatusUnprocessableEntity, "SOURCE_CONNECTION_FAILED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceNameConflict):
		httpresp.Error(c, http.StatusConflict, "SOURCE_NAME_CONFLICT", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceInUse):
		httpresp.Error(c, http.StatusConflict, "SOURCE_IN_USE", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}

func parseUintParam(c *gin.Context, name string) (uint, error) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	return uint(value), err
}
