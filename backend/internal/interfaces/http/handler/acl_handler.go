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

// ACLHandler 负责 ACL 管理接口。
type ACLHandler struct {
	service interface {
		List(ctx context.Context, query appdto.ACLRuleListQuery) (*appdto.ACLRuleListResponse, error)
		Create(ctx context.Context, req appdto.CreateACLRuleRequest) (*appdto.ACLRuleView, error)
		Update(ctx context.Context, id uint, req appdto.UpdateACLRuleRequest) (*appdto.ACLRuleView, error)
		Delete(ctx context.Context, id uint) error
	}
}

// NewACLHandler 创建 ACL handler。
func NewACLHandler(service interface {
	List(ctx context.Context, query appdto.ACLRuleListQuery) (*appdto.ACLRuleListResponse, error)
	Create(ctx context.Context, req appdto.CreateACLRuleRequest) (*appdto.ACLRuleView, error)
	Update(ctx context.Context, id uint, req appdto.UpdateACLRuleRequest) (*appdto.ACLRuleView, error)
	Delete(ctx context.Context, id uint) error
}) *ACLHandler {
	return &ACLHandler{service: service}
}

// List 返回 ACL 规则列表。
func (h *ACLHandler) List(c *gin.Context) {
	var query appdto.ACLRuleListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Create 创建 ACL 规则。
func (h *ACLHandler) Create(c *gin.Context) {
	var req appdto.CreateACLRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusCreated, "OK", "ok", gin.H{"rule": resp})
}

// Update 更新 ACL 规则。
func (h *ACLHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.UpdateACLRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Update(c.Request.Context(), uint(id), req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"rule": resp})
}

// Delete 删除 ACL 规则。
func (h *ACLHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if err := h.service.Delete(c.Request.Context(), uint(id)); err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.Empty(c, http.StatusOK)
}

func (h *ACLHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "ACL_RULE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrACLSubjectTypeInvalid):
		httpresp.Error(c, http.StatusBadRequest, "ACL_SUBJECT_TYPE_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrACLEffectInvalid):
		httpresp.Error(c, http.StatusBadRequest, "ACL_EFFECT_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrACLPermissionsInvalid):
		httpresp.Error(c, http.StatusBadRequest, "ACL_PERMISSIONS_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPathInvalid):
		httpresp.Error(c, http.StatusBadRequest, "PATH_INVALID", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}
