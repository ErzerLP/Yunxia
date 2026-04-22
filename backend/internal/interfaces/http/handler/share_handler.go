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

// ShareHandler 负责分享链接接口。
type ShareHandler struct {
	service interface {
		List(ctx context.Context) (*appdto.ShareListResponse, error)
		Get(ctx context.Context, id uint) (*appdto.ShareView, error)
		Create(ctx context.Context, req appdto.CreateShareRequest) (*appdto.ShareView, error)
		Update(ctx context.Context, id uint, req appdto.UpdateShareRequest) (*appdto.ShareView, error)
		Delete(ctx context.Context, id uint) (*appdto.DeleteShareResponse, error)
		Open(ctx context.Context, token, password, relativePath, disposition, sortBy, sortOrder string, page, pageSize int) (*appsvc.ShareOpenResult, error)
	}
}

// NewShareHandler 创建分享链接 handler。
func NewShareHandler(service interface {
	List(ctx context.Context) (*appdto.ShareListResponse, error)
	Get(ctx context.Context, id uint) (*appdto.ShareView, error)
	Create(ctx context.Context, req appdto.CreateShareRequest) (*appdto.ShareView, error)
	Update(ctx context.Context, id uint, req appdto.UpdateShareRequest) (*appdto.ShareView, error)
	Delete(ctx context.Context, id uint) (*appdto.DeleteShareResponse, error)
	Open(ctx context.Context, token, password, relativePath, disposition, sortBy, sortOrder string, page, pageSize int) (*appsvc.ShareOpenResult, error)
}) *ShareHandler {
	return &ShareHandler{service: service}
}

// List 返回当前用户的分享列表。
func (h *ShareHandler) List(c *gin.Context) {
	resp, err := h.service.List(c.Request.Context())
	if err != nil {
		h.writeManageError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Get 返回单个分享详情。
func (h *ShareHandler) Get(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Get(c.Request.Context(), id)
	if svcErr != nil {
		h.writeManageError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"share": resp})
}

// Create 创建分享链接。
func (h *ShareHandler) Create(c *gin.Context) {
	var req appdto.CreateShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		h.writeCreateError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusCreated, "OK", "ok", gin.H{"share": resp})
}

// Update 更新分享链接。
func (h *ShareHandler) Update(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.UpdateShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Update(c.Request.Context(), id, req)
	if svcErr != nil {
		h.writeManageError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"share": resp})
}

// Delete 删除分享链接。
func (h *ShareHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Delete(c.Request.Context(), uint(id))
	if svcErr != nil {
		h.writeManageError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Open 打开公开分享链接。
func (h *ShareHandler) Open(c *gin.Context) {
	token := c.Param("token")
	password := c.Query("password")
	if password == "" {
		password = c.GetHeader("X-Share-Password")
	}
	page, err := parseOptionalIntQuery(c, "page")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	pageSize, err := parseOptionalIntQuery(c, "page_size")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	result, err := h.service.Open(
		c.Request.Context(),
		token,
		password,
		c.Query("path"),
		c.DefaultQuery("disposition", "attachment"),
		c.DefaultQuery("sort_by", "name"),
		c.DefaultQuery("sort_order", "asc"),
		page,
		pageSize,
	)
	if err != nil {
		h.writeOpenError(c, err)
		return
	}
	if result.RedirectURL != "" {
		c.Redirect(http.StatusFound, result.RedirectURL)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", result.Data)
}

func parseOptionalIntQuery(c *gin.Context, name string) (int, error) {
	value := c.Query(name)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (h *ShareHandler) writeCreateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "SOURCE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPathInvalid):
		httpresp.Error(c, http.StatusBadRequest, "PATH_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileNotFound):
		httpresp.Error(c, http.StatusNotFound, "FILE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceDriverUnsupported):
		httpresp.Error(c, http.StatusUnprocessableEntity, "SOURCE_DRIVER_UNSUPPORTED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrACLDenied):
		httpresp.Error(c, http.StatusForbidden, "ACL_DENIED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPermissionDenied):
		httpresp.Error(c, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}

func (h *ShareHandler) writeManageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "SHARE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPermissionDenied):
		httpresp.Error(c, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}

func (h *ShareHandler) writeOpenError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "SHARE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrShareExpired):
		httpresp.Error(c, http.StatusGone, "SHARE_EXPIRED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSharePasswordRequired):
		httpresp.Error(c, http.StatusUnauthorized, "SHARE_PASSWORD_REQUIRED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSharePasswordInvalid):
		httpresp.Error(c, http.StatusUnauthorized, "SHARE_PASSWORD_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrFileNotFound):
		httpresp.Error(c, http.StatusNotFound, "FILE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPathInvalid):
		httpresp.Error(c, http.StatusBadRequest, "PATH_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceDriverUnsupported):
		httpresp.Error(c, http.StatusUnprocessableEntity, "SOURCE_DRIVER_UNSUPPORTED", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}
