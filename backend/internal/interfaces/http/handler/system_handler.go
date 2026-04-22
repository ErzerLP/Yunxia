package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	appdto "yunxia/internal/application/dto"
	httpresp "yunxia/internal/interfaces/http/response"
)

// SystemHandler 负责系统接口。
type SystemHandler struct {
	service interface {
		GetConfig(ctx context.Context) (*appdto.SystemConfigPublic, error)
		GetStats(ctx context.Context) (*appdto.SystemStatsResponse, error)
		UpdateConfig(ctx context.Context, req appdto.UpdateSystemConfigRequest) (*appdto.SystemConfigPublic, error)
	}
	version   string
	commit    string
	buildTime string
	goVersion string
}

// NewSystemHandler 创建系统 handler。
func NewSystemHandler(
	service interface {
		GetConfig(ctx context.Context) (*appdto.SystemConfigPublic, error)
		GetStats(ctx context.Context) (*appdto.SystemStatsResponse, error)
		UpdateConfig(ctx context.Context, req appdto.UpdateSystemConfigRequest) (*appdto.SystemConfigPublic, error)
	},
	version string,
	commit string,
	buildTime string,
	goVersion string,
) *SystemHandler {
	return &SystemHandler{
		service:   service,
		version:   version,
		commit:    commit,
		buildTime: buildTime,
		goVersion: goVersion,
	}
}

// Health 返回健康状态。
func (h *SystemHandler) Health(c *gin.Context) {
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{
		"status":  "ok",
		"service": "yunxia",
		"version": h.version,
	})
}

// Version 返回版本信息。
func (h *SystemHandler) Version(c *gin.Context) {
	var commit *string
	if h.commit != "" {
		commit = &h.commit
	}
	var buildTime *string
	if h.buildTime != "" {
		buildTime = &h.buildTime
	}

	httpresp.JSON(c, http.StatusOK, "OK", "ok", appdto.VersionResponse{
		Service:    "yunxia",
		Version:    h.version,
		Commit:     commit,
		BuildTime:  buildTime,
		GoVersion:  h.goVersion,
		APIVersion: "v1",
	})
}

// GetConfig 返回系统配置。
func (h *SystemHandler) GetConfig(c *gin.Context) {
	resp, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Stats 返回系统统计信息。
func (h *SystemHandler) Stats(c *gin.Context) {
	resp, err := h.service.GetStats(c.Request.Context())
	if err != nil {
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// UpdateConfig 更新系统配置。
func (h *SystemHandler) UpdateConfig(c *gin.Context) {
	var req appdto.UpdateSystemConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	resp, err := h.service.UpdateConfig(c.Request.Context(), req)
	if err != nil {
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}
