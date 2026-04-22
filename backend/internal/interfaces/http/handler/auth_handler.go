package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	appdto "yunxia/internal/application/dto"
	appsvc "yunxia/internal/application/service"
	httpresp "yunxia/internal/interfaces/http/response"
)

// AuthHandler 负责认证相关接口。
type AuthHandler struct {
	service interface {
		Login(ctx context.Context, req appdto.LoginRequest) (*appdto.LoginResponse, error)
		Refresh(ctx context.Context, req appdto.RefreshRequest) (*appdto.RefreshResponse, error)
		Logout(ctx context.Context, req appdto.LogoutRequest) error
		Me(ctx context.Context, userID uint) (*appdto.UserSummary, error)
	}
}

// NewAuthHandler 创建认证 handler。
func NewAuthHandler(service interface {
	Login(ctx context.Context, req appdto.LoginRequest) (*appdto.LoginResponse, error)
	Refresh(ctx context.Context, req appdto.RefreshRequest) (*appdto.RefreshResponse, error)
	Logout(ctx context.Context, req appdto.LogoutRequest) error
	Me(ctx context.Context, userID uint) (*appdto.UserSummary, error)
}) *AuthHandler {
	return &AuthHandler{service: service}
}

// Login 用户名密码登录。
func (h *AuthHandler) Login(c *gin.Context) {
	var req appdto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	resp, err := h.service.Login(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, appsvc.ErrInvalidCredentials):
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "invalid credentials", nil)
		case errors.Is(err, appsvc.ErrAccountLocked):
			httpresp.Error(c, http.StatusForbidden, "AUTH_ACCOUNT_LOCKED", "account locked", nil)
		default:
			httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		}
		return
	}

	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Refresh 刷新访问令牌。
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req appdto.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	resp, err := h.service.Refresh(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, appsvc.ErrRefreshTokenInvalid) {
			httpresp.Error(c, http.StatusUnauthorized, "AUTH_REFRESH_TOKEN_INVALID", "refresh token invalid", nil)
			return
		}
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Logout 撤销 refresh token。
func (h *AuthHandler) Logout(c *gin.Context) {
	var req appdto.LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	if err := h.service.Logout(c.Request.Context(), req); err != nil {
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	httpresp.Empty(c, http.StatusOK)
}

// Me 返回当前用户。
func (h *AuthHandler) Me(c *gin.Context) {
	value, exists := c.Get("user_id")
	if !exists {
		httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "missing user context", nil)
		return
	}

	userID, ok := value.(uint)
	if !ok {
		httpresp.Error(c, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "invalid user context", nil)
		return
	}

	resp, err := h.service.Me(c.Request.Context(), userID)
	if err != nil {
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}
