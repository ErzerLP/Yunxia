package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appaudit "yunxia/internal/application/audit"
	appdto "yunxia/internal/application/dto"
	domainrepo "yunxia/internal/domain/repository"
	httpresp "yunxia/internal/interfaces/http/response"
)

// AuditHandler 负责审计查询接口。
type AuditHandler struct {
	service interface {
		List(ctx context.Context, query appdto.AuditLogListQuery) (*appdto.AuditLogListResponse, error)
		Get(ctx context.Context, id uint) (*appdto.AuditLogDetailResponse, error)
	}
}

// NewAuditHandler 创建审计 handler。
func NewAuditHandler(service interface {
	List(ctx context.Context, query appdto.AuditLogListQuery) (*appdto.AuditLogListResponse, error)
	Get(ctx context.Context, id uint) (*appdto.AuditLogDetailResponse, error)
}) *AuditHandler {
	return &AuditHandler{service: service}
}

// List 返回审计列表。
func (h *AuditHandler) List(c *gin.Context) {
	var query appdto.AuditLogListQuery
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

// Get 返回单条审计详情。
func (h *AuditHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	resp, err := h.service.Get(c.Request.Context(), uint(id))
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *AuditHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appaudit.ErrInvalidTimeFilter):
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "AUDIT_LOG_NOT_FOUND", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}
