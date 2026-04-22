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

// SetupHandler 负责初始化相关 HTTP 接口。
type SetupHandler struct {
	service interface {
		Status(ctx context.Context) (*appdto.SetupStatusResponse, error)
		Init(ctx context.Context, req appdto.SetupInitRequest) (*appdto.SetupInitResponse, error)
	}
}

// NewSetupHandler 创建初始化 handler。
func NewSetupHandler(service interface {
	Status(ctx context.Context) (*appdto.SetupStatusResponse, error)
	Init(ctx context.Context, req appdto.SetupInitRequest) (*appdto.SetupInitResponse, error)
}) *SetupHandler {
	return &SetupHandler{service: service}
}

// Status 返回初始化状态。
func (h *SetupHandler) Status(c *gin.Context) {
	resp, err := h.service.Status(c.Request.Context())
	if err != nil {
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Init 执行初始化。
func (h *SetupHandler) Init(c *gin.Context) {
	var req appdto.SetupInitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	resp, err := h.service.Init(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, appsvc.ErrSetupAlreadyCompleted) {
			httpresp.Error(c, http.StatusConflict, "SETUP_ALREADY_COMPLETED", err.Error(), nil)
			return
		}
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	httpresp.JSON(c, http.StatusCreated, "OK", "ok", resp)
}
