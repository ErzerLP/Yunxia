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
	"yunxia/internal/infrastructure/security"
	httpresp "yunxia/internal/interfaces/http/response"
)

// VFSTagHandler 负责 VFS 标签管理与节点绑定接口。
type VFSTagHandler struct {
	service interface {
		ListTags(ctx context.Context, ownerUserID uint) (*appdto.VFSTagListResponse, error)
		CreateTag(ctx context.Context, ownerUserID uint, req appdto.VFSTagUpsertRequest) (*appdto.VFSTagView, error)
		UpdateTag(ctx context.Context, ownerUserID uint, tagID uint, req appdto.VFSTagUpsertRequest) (*appdto.VFSTagView, error)
		DeleteTag(ctx context.Context, ownerUserID uint, tagID uint) error
		ListNodeTags(ctx context.Context, ownerUserID uint, virtualPath string) (*appdto.VFSNodeTagListResponse, error)
		AttachTag(ctx context.Context, ownerUserID uint, req appdto.VFSNodeTagRequest) (*appdto.VFSNodeTagListResponse, error)
		DetachTag(ctx context.Context, ownerUserID uint, req appdto.VFSNodeTagRequest) (*appdto.VFSNodeTagListResponse, error)
	}
}

// NewVFSTagHandler 创建 VFS 标签 handler。
func NewVFSTagHandler(service interface {
	ListTags(ctx context.Context, ownerUserID uint) (*appdto.VFSTagListResponse, error)
	CreateTag(ctx context.Context, ownerUserID uint, req appdto.VFSTagUpsertRequest) (*appdto.VFSTagView, error)
	UpdateTag(ctx context.Context, ownerUserID uint, tagID uint, req appdto.VFSTagUpsertRequest) (*appdto.VFSTagView, error)
	DeleteTag(ctx context.Context, ownerUserID uint, tagID uint) error
	ListNodeTags(ctx context.Context, ownerUserID uint, virtualPath string) (*appdto.VFSNodeTagListResponse, error)
	AttachTag(ctx context.Context, ownerUserID uint, req appdto.VFSNodeTagRequest) (*appdto.VFSNodeTagListResponse, error)
	DetachTag(ctx context.Context, ownerUserID uint, req appdto.VFSNodeTagRequest) (*appdto.VFSNodeTagListResponse, error)
}) *VFSTagHandler {
	return &VFSTagHandler{service: service}
}

// ListTags 列出当前用户标签。
func (h *VFSTagHandler) ListTags(c *gin.Context) {
	ownerID, ok := requestUserID(c)
	if !ok {
		httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "missing auth context", nil)
		return
	}
	resp, err := h.service.ListTags(c.Request.Context(), ownerID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// CreateTag 创建当前用户标签。
func (h *VFSTagHandler) CreateTag(c *gin.Context) {
	ownerID, ok := requestUserID(c)
	if !ok {
		httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "missing auth context", nil)
		return
	}
	var req appdto.VFSTagUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	tag, err := h.service.CreateTag(c.Request.Context(), ownerID, req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusCreated, "OK", "ok", gin.H{"tag": tag})
}

// UpdateTag 更新当前用户标签。
func (h *VFSTagHandler) UpdateTag(c *gin.Context) {
	ownerID, ok := requestUserID(c)
	if !ok {
		httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "missing auth context", nil)
		return
	}
	tagID, err := parseTagIDParam(c)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.VFSTagUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	tag, err := h.service.UpdateTag(c.Request.Context(), ownerID, tagID, req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"tag": tag})
}

// DeleteTag 删除当前用户标签。
func (h *VFSTagHandler) DeleteTag(c *gin.Context) {
	ownerID, ok := requestUserID(c)
	if !ok {
		httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "missing auth context", nil)
		return
	}
	tagID, err := parseTagIDParam(c)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if err := h.service.DeleteTag(c.Request.Context(), ownerID, tagID); err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"deleted": true, "id": tagID})
}

// ListNodeTags 列出指定 VFS 节点标签。
func (h *VFSTagHandler) ListNodeTags(c *gin.Context) {
	ownerID, ok := requestUserID(c)
	if !ok {
		httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "missing auth context", nil)
		return
	}
	resp, err := h.service.ListNodeTags(c.Request.Context(), ownerID, c.Query("path"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// AttachTag 绑定标签到 VFS 节点。
func (h *VFSTagHandler) AttachTag(c *gin.Context) {
	ownerID, ok := requestUserID(c)
	if !ok {
		httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "missing auth context", nil)
		return
	}
	var req appdto.VFSNodeTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.AttachTag(c.Request.Context(), ownerID, req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// DetachTag 解除 VFS 节点标签绑定。
func (h *VFSTagHandler) DetachTag(c *gin.Context) {
	ownerID, ok := requestUserID(c)
	if !ok {
		httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "missing auth context", nil)
		return
	}
	var req appdto.VFSNodeTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.DetachTag(c.Request.Context(), ownerID, req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *VFSTagHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appsvc.ErrFileNotFound):
		httpresp.Error(c, http.StatusNotFound, "FILE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "TAG_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrTagBindingNotFound):
		httpresp.Error(c, http.StatusNotFound, "TAG_BINDING_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrTagInvalid), errors.Is(err, appsvc.ErrPathInvalid):
		httpresp.Error(c, http.StatusBadRequest, "TAG_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrTagForbidden), errors.Is(err, appsvc.ErrPermissionDenied):
		httpresp.Error(c, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceDriverUnsupported):
		httpresp.Error(c, http.StatusUnprocessableEntity, "SOURCE_DRIVER_UNSUPPORTED", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}

func requestUserID(c *gin.Context) (uint, bool) {
	auth, ok := security.RequestAuthFromContext(c.Request.Context())
	if !ok || auth.UserID == 0 {
		return 0, false
	}
	return auth.UserID, true
}

func parseTagIDParam(c *gin.Context) (uint, error) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	return uint(value), err
}
