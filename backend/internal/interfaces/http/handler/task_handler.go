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

// TaskHandler 负责离线下载任务接口。
type TaskHandler struct {
	service interface {
		List(ctx context.Context) (*appdto.TaskListResponse, error)
		Create(ctx context.Context, req appdto.CreateTaskRequest) (*appdto.DownloadTaskView, error)
		Get(ctx context.Context, id uint) (*appdto.DownloadTaskView, error)
		Cancel(ctx context.Context, id uint, deleteFile bool) (*appdto.CancelTaskResponse, error)
		Pause(ctx context.Context, id uint) (*appdto.TaskActionResponse, error)
		Resume(ctx context.Context, id uint) (*appdto.TaskActionResponse, error)
	}
}

// NewTaskHandler 创建任务 handler。
func NewTaskHandler(service interface {
	List(ctx context.Context) (*appdto.TaskListResponse, error)
	Create(ctx context.Context, req appdto.CreateTaskRequest) (*appdto.DownloadTaskView, error)
	Get(ctx context.Context, id uint) (*appdto.DownloadTaskView, error)
	Cancel(ctx context.Context, id uint, deleteFile bool) (*appdto.CancelTaskResponse, error)
	Pause(ctx context.Context, id uint) (*appdto.TaskActionResponse, error)
	Resume(ctx context.Context, id uint) (*appdto.TaskActionResponse, error)
}) *TaskHandler {
	return &TaskHandler{service: service}
}

// List 返回任务列表。
func (h *TaskHandler) List(c *gin.Context) {
	resp, err := h.service.List(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Create 创建任务。
func (h *TaskHandler) Create(c *gin.Context) {
	var req appdto.CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	httpresp.JSON(c, http.StatusAccepted, "OK", "ok", gin.H{"task": resp})
}

// Get 返回单个任务。
func (h *TaskHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Get(c.Request.Context(), uint(id))
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Cancel 取消任务。
func (h *TaskHandler) Cancel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	deleteFile := c.DefaultQuery("delete_file", "false") == "true"
	resp, svcErr := h.service.Cancel(c.Request.Context(), uint(id), deleteFile)
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Pause 暂停任务。
func (h *TaskHandler) Pause(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Pause(c.Request.Context(), uint(id))
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

// Resume 恢复任务。
func (h *TaskHandler) Resume(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, svcErr := h.service.Resume(c.Request.Context(), uint(id))
	if svcErr != nil {
		h.writeError(c, svcErr)
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *TaskHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, "TASK_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPathInvalid):
		httpresp.Error(c, http.StatusBadRequest, "PATH_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrNoBackingStorage):
		httpresp.Error(c, http.StatusConflict, "NO_BACKING_STORAGE", err.Error(), nil)
	case errors.Is(err, appsvc.ErrTaskInvalidState):
		httpresp.Error(c, http.StatusConflict, "TASK_INVALID_STATE", err.Error(), nil)
	case errors.Is(err, appsvc.ErrACLDenied), errors.Is(err, appsvc.ErrPermissionDenied):
		httpresp.Error(c, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceDriverUnsupported):
		httpresp.Error(c, http.StatusServiceUnavailable, "DOWNLOADER_UNAVAILABLE", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}
