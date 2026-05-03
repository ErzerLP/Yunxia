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
	"yunxia/internal/interfaces/http/response"
)

// NotificationHandler 负责通知接口。
type NotificationHandler struct {
	service interface {
		ListChannels(ctx context.Context) (*appdto.NotificationChannelListResponse, error)
		CreateChannel(ctx context.Context, req appdto.NotificationChannelUpsertRequest) (*appdto.NotificationChannelView, error)
		UpdateChannel(ctx context.Context, id uint, req appdto.NotificationChannelUpsertRequest) (*appdto.NotificationChannelView, error)
		DeleteChannel(ctx context.Context, id uint) error
		TestChannel(ctx context.Context, id uint) (*appdto.NotificationTestResponse, error)
		ListEvents(ctx context.Context, filter domainrepo.NotificationEventFilter) (*appdto.NotificationEventListResponse, error)
		RetryEvent(ctx context.Context, id uint) (*appdto.NotificationEventView, error)
	}
}

// NewNotificationHandler 创建通知 handler。
func NewNotificationHandler(service interface {
	ListChannels(ctx context.Context) (*appdto.NotificationChannelListResponse, error)
	CreateChannel(ctx context.Context, req appdto.NotificationChannelUpsertRequest) (*appdto.NotificationChannelView, error)
	UpdateChannel(ctx context.Context, id uint, req appdto.NotificationChannelUpsertRequest) (*appdto.NotificationChannelView, error)
	DeleteChannel(ctx context.Context, id uint) error
	TestChannel(ctx context.Context, id uint) (*appdto.NotificationTestResponse, error)
	ListEvents(ctx context.Context, filter domainrepo.NotificationEventFilter) (*appdto.NotificationEventListResponse, error)
	RetryEvent(ctx context.Context, id uint) (*appdto.NotificationEventView, error)
}) *NotificationHandler {
	return &NotificationHandler{service: service}
}

// ListChannels 返回通知通道列表。
func (h *NotificationHandler) ListChannels(c *gin.Context) {
	resp, err := h.service.ListChannels(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// CreateChannel 创建通知通道。
func (h *NotificationHandler) CreateChannel(c *gin.Context) {
	var req appdto.NotificationChannelUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.CreateChannel(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, "OK", "ok", gin.H{"channel": resp})
}

// UpdateChannel 更新通知通道。
func (h *NotificationHandler) UpdateChannel(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.NotificationChannelUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.UpdateChannel(c.Request.Context(), id, req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "OK", "ok", gin.H{"channel": resp})
}

// DeleteChannel 删除通知通道。
func (h *NotificationHandler) DeleteChannel(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if err := h.service.DeleteChannel(c.Request.Context(), id); err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "OK", "ok", gin.H{"deleted": true, "id": id})
}

// TestChannel 发送测试通知。
func (h *NotificationHandler) TestChannel(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.TestChannel(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// ListEvents 返回通知事件列表。
func (h *NotificationHandler) ListEvents(c *gin.Context) {
	limit, err := parseOptionalNotificationIntQuery(c, "limit")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.ListEvents(c.Request.Context(), domainrepo.NotificationEventFilter{
		Status:    c.Query("status"),
		EventType: c.Query("event_type"),
		Limit:     limit,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// RetryEvent 手动重试通知事件。
func (h *NotificationHandler) RetryEvent(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.RetryEvent(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusAccepted, "OK", "ok", gin.H{"event": resp})
}

func (h *NotificationHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		response.Error(c, http.StatusNotFound, "NOTIFICATION_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPermissionDenied):
		response.Error(c, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrConfigInvalid):
		response.Error(c, http.StatusUnprocessableEntity, "CONFIG_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrNotificationChannelUnsupported):
		response.Error(c, http.StatusUnprocessableEntity, "NOTIFICATION_CHANNEL_UNSUPPORTED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrNotificationDeliveryFailed):
		response.Error(c, http.StatusBadGateway, "NOTIFICATION_DELIVERY_FAILED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrTaskInvalidState):
		response.Error(c, http.StatusConflict, "TASK_INVALID_STATE", err.Error(), nil)
	default:
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}

func parseOptionalNotificationIntQuery(c *gin.Context, name string) (int, error) {
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
