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

// UserHandler 负责管理员用户管理接口。
type UserHandler struct {
	service interface {
		List(ctx context.Context, query appdto.UserListQuery) (*appdto.UserListResponse, error)
		Create(ctx context.Context, req appdto.CreateUserRequest) (*appdto.UserAdminView, error)
		Update(ctx context.Context, id uint, req appdto.UpdateUserRequest) (*appdto.UserAdminView, error)
		ResetPassword(ctx context.Context, id uint, req appdto.ResetUserPasswordRequest) error
		RevokeTokens(ctx context.Context, id uint) (*appdto.RevokeUserTokensResponse, error)
	}
}

// NewUserHandler 创建用户管理 handler。
func NewUserHandler(service interface {
	List(ctx context.Context, query appdto.UserListQuery) (*appdto.UserListResponse, error)
	Create(ctx context.Context, req appdto.CreateUserRequest) (*appdto.UserAdminView, error)
	Update(ctx context.Context, id uint, req appdto.UpdateUserRequest) (*appdto.UserAdminView, error)
	ResetPassword(ctx context.Context, id uint, req appdto.ResetUserPasswordRequest) error
	RevokeTokens(ctx context.Context, id uint) (*appdto.RevokeUserTokensResponse, error)
}) *UserHandler {
	return &UserHandler{service: service}
}

// List 返回用户列表。
func (h *UserHandler) List(c *gin.Context) {
	var query appdto.UserListQuery
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

// Create 创建用户。
func (h *UserHandler) Create(c *gin.Context) {
	var req appdto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusCreated, "OK", "ok", gin.H{"user": resp})
}

// Update 更新用户。
func (h *UserHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Update(c.Request.Context(), uint(id), req)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"user": resp})
}

// ResetPassword 重置用户密码。
func (h *UserHandler) ResetPassword(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.ResetUserPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if err := h.service.ResetPassword(c.Request.Context(), uint(id), req); err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.Empty(c, http.StatusOK)
}

// RevokeTokens 撤销用户令牌。
func (h *UserHandler) RevokeTokens(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.RevokeTokens(c.Request.Context(), uint(id))
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *UserHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "USER_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrUserNameConflict):
		httpresp.Error(c, http.StatusConflict, "USER_NAME_CONFLICT", err.Error(), nil)
	case errors.Is(err, appsvc.ErrUserRoleInvalid):
		httpresp.Error(c, http.StatusBadRequest, "USER_ROLE_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrUserStatusInvalid):
		httpresp.Error(c, http.StatusBadRequest, "USER_STATUS_INVALID", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}
