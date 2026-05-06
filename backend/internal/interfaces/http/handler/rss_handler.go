package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appdto "yunxia/internal/application/dto"
	appsvc "yunxia/internal/application/service"
	domainrepo "yunxia/internal/domain/repository"
	httpresp "yunxia/internal/interfaces/http/response"
)

// RSSHandler 负责 RSS 番剧订阅下载接口。
type RSSHandler struct {
	service interface {
		ListSources(ctx context.Context) (*appdto.RSSSourceListResponse, error)
		CreateSource(ctx context.Context, req appdto.RSSSourceUpsertRequest) (*appdto.RSSSourceView, error)
		GetSource(ctx context.Context, id uint) (*appdto.RSSSourceView, error)
		UpdateSource(ctx context.Context, id uint, req appdto.RSSSourceUpsertRequest) (*appdto.RSSSourceView, error)
		DeleteSource(ctx context.Context, id uint) error
		RefreshSource(ctx context.Context, id uint) (*appdto.RSSRefreshResponse, error)
		RefreshAllSources(ctx context.Context) (*appdto.RSSRefreshAllResponse, error)
		ExportConfig(ctx context.Context) (*appdto.RSSExportResponse, error)
		ImportConfig(ctx context.Context, req appdto.RSSImportRequest) (*appdto.RSSImportResponse, error)

		ListSubscriptions(ctx context.Context, sourceID uint) (*appdto.RSSSubscriptionListResponse, error)
		CreateSubscription(ctx context.Context, req appdto.RSSSubscriptionUpsertRequest) (*appdto.RSSSubscriptionView, error)
		CloneSubscription(ctx context.Context, id uint, req appdto.RSSSubscriptionCloneRequest) (*appdto.RSSSubscriptionView, error)
		GetSubscription(ctx context.Context, id uint) (*appdto.RSSSubscriptionView, error)
		UpdateSubscription(ctx context.Context, id uint, req appdto.RSSSubscriptionUpsertRequest) (*appdto.RSSSubscriptionView, error)
		DeleteSubscription(ctx context.Context, id uint) error
		BatchUpdateSubscriptionState(ctx context.Context, req appdto.RSSSubscriptionBatchStateRequest) (*appdto.RSSSubscriptionBatchStateResponse, error)
		RunSubscription(ctx context.Context, id uint) (*appdto.RSSRefreshResponse, error)
		PreviewSubscription(ctx context.Context, id uint) (*appdto.RSSSubscriptionPreviewResponse, error)
		PreviewSubscriptionRules(ctx context.Context, req appdto.RSSSubscriptionPreviewRequest) (*appdto.RSSSubscriptionPreviewResponse, error)

		ListItems(ctx context.Context, filter domainrepo.RSSItemFilter) (*appdto.RSSItemListResponse, error)
		DownloadItem(ctx context.Context, id uint, req appdto.RSSManualDownloadRequest) (*appdto.RSSItemView, error)
		ReprocessItem(ctx context.Context, id uint) (*appdto.RSSItemView, error)
		RetryItem(ctx context.Context, id uint, req appdto.RSSManualDownloadRequest) (*appdto.RSSItemView, error)
		BatchIgnoreItems(ctx context.Context, req appdto.RSSItemBatchIgnoreRequest) (*appdto.RSSItemBatchActionResponse, error)
		BatchRetryItems(ctx context.Context, req appdto.RSSItemBatchRetryRequest) (*appdto.RSSItemBatchActionResponse, error)
		QBitHealth(ctx context.Context) (*appdto.RSSQBitHealthResponse, error)
	}
}

// NewRSSHandler 创建 RSS handler。
func NewRSSHandler(service interface {
	ListSources(ctx context.Context) (*appdto.RSSSourceListResponse, error)
	CreateSource(ctx context.Context, req appdto.RSSSourceUpsertRequest) (*appdto.RSSSourceView, error)
	GetSource(ctx context.Context, id uint) (*appdto.RSSSourceView, error)
	UpdateSource(ctx context.Context, id uint, req appdto.RSSSourceUpsertRequest) (*appdto.RSSSourceView, error)
	DeleteSource(ctx context.Context, id uint) error
	RefreshSource(ctx context.Context, id uint) (*appdto.RSSRefreshResponse, error)
	RefreshAllSources(ctx context.Context) (*appdto.RSSRefreshAllResponse, error)
	ExportConfig(ctx context.Context) (*appdto.RSSExportResponse, error)
	ImportConfig(ctx context.Context, req appdto.RSSImportRequest) (*appdto.RSSImportResponse, error)

	ListSubscriptions(ctx context.Context, sourceID uint) (*appdto.RSSSubscriptionListResponse, error)
	CreateSubscription(ctx context.Context, req appdto.RSSSubscriptionUpsertRequest) (*appdto.RSSSubscriptionView, error)
	CloneSubscription(ctx context.Context, id uint, req appdto.RSSSubscriptionCloneRequest) (*appdto.RSSSubscriptionView, error)
	GetSubscription(ctx context.Context, id uint) (*appdto.RSSSubscriptionView, error)
	UpdateSubscription(ctx context.Context, id uint, req appdto.RSSSubscriptionUpsertRequest) (*appdto.RSSSubscriptionView, error)
	DeleteSubscription(ctx context.Context, id uint) error
	BatchUpdateSubscriptionState(ctx context.Context, req appdto.RSSSubscriptionBatchStateRequest) (*appdto.RSSSubscriptionBatchStateResponse, error)
	RunSubscription(ctx context.Context, id uint) (*appdto.RSSRefreshResponse, error)
	PreviewSubscription(ctx context.Context, id uint) (*appdto.RSSSubscriptionPreviewResponse, error)
	PreviewSubscriptionRules(ctx context.Context, req appdto.RSSSubscriptionPreviewRequest) (*appdto.RSSSubscriptionPreviewResponse, error)

	ListItems(ctx context.Context, filter domainrepo.RSSItemFilter) (*appdto.RSSItemListResponse, error)
	DownloadItem(ctx context.Context, id uint, req appdto.RSSManualDownloadRequest) (*appdto.RSSItemView, error)
	ReprocessItem(ctx context.Context, id uint) (*appdto.RSSItemView, error)
	RetryItem(ctx context.Context, id uint, req appdto.RSSManualDownloadRequest) (*appdto.RSSItemView, error)
	BatchIgnoreItems(ctx context.Context, req appdto.RSSItemBatchIgnoreRequest) (*appdto.RSSItemBatchActionResponse, error)
	BatchRetryItems(ctx context.Context, req appdto.RSSItemBatchRetryRequest) (*appdto.RSSItemBatchActionResponse, error)
	QBitHealth(ctx context.Context) (*appdto.RSSQBitHealthResponse, error)
}) *RSSHandler {
	return &RSSHandler{service: service}
}

func (h *RSSHandler) ListSources(c *gin.Context) {
	resp, err := h.service.ListSources(c.Request.Context())
	if err != nil {
		h.writeError(c, err, "RSS_SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) CreateSource(c *gin.Context) {
	var req appdto.RSSSourceUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.CreateSource(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err, "RSS_SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusCreated, "OK", "ok", gin.H{"source": resp})
}

func (h *RSSHandler) GetSource(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.GetSource(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err, "RSS_SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) UpdateSource(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.RSSSourceUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.UpdateSource(c.Request.Context(), id, req)
	if err != nil {
		h.writeError(c, err, "RSS_SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"source": resp})
}

func (h *RSSHandler) DeleteSource(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if err := h.service.DeleteSource(c.Request.Context(), id); err != nil {
		h.writeError(c, err, "RSS_SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"deleted": true, "id": id})
}

func (h *RSSHandler) RefreshSource(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.RefreshSource(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err, "RSS_SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) RefreshAllSources(c *gin.Context) {
	resp, err := h.service.RefreshAllSources(c.Request.Context())
	if err != nil {
		h.writeError(c, err, "RSS_SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) ExportConfig(c *gin.Context) {
	resp, err := h.service.ExportConfig(c.Request.Context())
	if err != nil {
		h.writeError(c, err, "RSS_SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) ImportConfig(c *gin.Context) {
	var req appdto.RSSImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.ImportConfig(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err, "RSS_SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) ListSubscriptions(c *gin.Context) {
	sourceID, err := parseUintQuery(c, "source_id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.ListSubscriptions(c.Request.Context(), sourceID)
	if err != nil {
		h.writeError(c, err, "RSS_SUBSCRIPTION_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) CreateSubscription(c *gin.Context) {
	var req appdto.RSSSubscriptionUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.CreateSubscription(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err, "RSS_SUBSCRIPTION_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusCreated, "OK", "ok", gin.H{"subscription": resp})
}

func (h *RSSHandler) CloneSubscription(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.RSSSubscriptionCloneRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			return
		}
	}
	resp, err := h.service.CloneSubscription(c.Request.Context(), id, req)
	if err != nil {
		h.writeError(c, err, "RSS_SUBSCRIPTION_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusCreated, "OK", "ok", gin.H{"subscription": resp})
}

func (h *RSSHandler) GetSubscription(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.GetSubscription(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err, "RSS_SUBSCRIPTION_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) UpdateSubscription(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.RSSSubscriptionUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.UpdateSubscription(c.Request.Context(), id, req)
	if err != nil {
		h.writeError(c, err, "RSS_SUBSCRIPTION_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"subscription": resp})
}

func (h *RSSHandler) DeleteSubscription(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if err := h.service.DeleteSubscription(c.Request.Context(), id); err != nil {
		h.writeError(c, err, "RSS_SUBSCRIPTION_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", gin.H{"deleted": true, "id": id})
}

func (h *RSSHandler) BatchUpdateSubscriptionState(c *gin.Context) {
	var req appdto.RSSSubscriptionBatchStateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.BatchUpdateSubscriptionState(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err, "RSS_SUBSCRIPTION_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) RunSubscription(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.RunSubscription(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err, "RSS_SUBSCRIPTION_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) PreviewSubscription(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.PreviewSubscription(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err, "RSS_SUBSCRIPTION_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) PreviewSubscriptionRules(c *gin.Context) {
	var req appdto.RSSSubscriptionPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.PreviewSubscriptionRules(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err, "RSS_SOURCE_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) ListItems(c *gin.Context) {
	sourceID, err := parseUintQuery(c, "source_id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	subscriptionID, err := parseUintQuery(c, "subscription_id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.ListItems(c.Request.Context(), domainrepo.RSSItemFilter{
		SourceID:       sourceID,
		SubscriptionID: subscriptionID,
		Status:         c.Query("status"),
	})
	if err != nil {
		h.writeError(c, err, "RSS_ITEM_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) DownloadItem(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.RSSManualDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.DownloadItem(c.Request.Context(), id, req)
	if err != nil {
		h.writeError(c, err, "RSS_ITEM_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusAccepted, "OK", "ok", gin.H{"item": resp})
}

func (h *RSSHandler) ReprocessItem(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.ReprocessItem(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err, "RSS_ITEM_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusAccepted, "OK", "ok", gin.H{"item": resp})
}

func (h *RSSHandler) RetryItem(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	var req appdto.RSSManualDownloadRequest
	if c.Request.Body != nil && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			return
		}
	}
	resp, err := h.service.RetryItem(c.Request.Context(), id, req)
	if err != nil {
		h.writeError(c, err, "RSS_ITEM_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusAccepted, "OK", "ok", gin.H{"item": resp})
}

func (h *RSSHandler) BatchIgnoreItems(c *gin.Context) {
	var req appdto.RSSItemBatchIgnoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.BatchIgnoreItems(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err, "RSS_ITEM_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) BatchRetryItems(c *gin.Context) {
	var req appdto.RSSItemBatchRetryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	resp, err := h.service.BatchRetryItems(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err, "RSS_ITEM_NOT_FOUND")
		return
	}
	httpresp.JSON(c, http.StatusAccepted, "OK", "ok", resp)
}

func (h *RSSHandler) QBitHealth(c *gin.Context) {
	resp, err := h.service.QBitHealth(c.Request.Context())
	if err != nil {
		h.writeError(c, err, "RSS_QBITTORRENT_UNAVAILABLE")
		return
	}
	httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
}

func (h *RSSHandler) writeError(c *gin.Context, err error, notFoundCode string) {
	switch {
	case errors.Is(err, domainrepo.ErrNotFound):
		httpresp.Error(c, http.StatusNotFound, notFoundCode, err.Error(), nil)
	case errors.Is(err, appsvc.ErrConfigInvalid):
		httpresp.Error(c, http.StatusUnprocessableEntity, "CONFIG_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrPathInvalid):
		httpresp.Error(c, http.StatusBadRequest, "PATH_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrNoBackingStorage):
		httpresp.Error(c, http.StatusConflict, "NO_BACKING_STORAGE", err.Error(), nil)
	case errors.Is(err, appsvc.ErrNameConflict):
		httpresp.Error(c, http.StatusConflict, "NAME_CONFLICT", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceReadOnly):
		httpresp.Error(c, http.StatusForbidden, "SOURCE_READ_ONLY", err.Error(), nil)
	case errors.Is(err, appsvc.ErrACLDenied), errors.Is(err, appsvc.ErrPermissionDenied):
		httpresp.Error(c, http.StatusForbidden, "PERMISSION_DENIED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrDownloadLinkUnsupported):
		httpresp.Error(c, http.StatusUnprocessableEntity, "DOWNLOAD_LINK_UNSUPPORTED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrRSSRegexInvalid):
		httpresp.Error(c, http.StatusBadRequest, "RSS_REGEX_INVALID", err.Error(), nil)
	case errors.Is(err, appsvc.ErrSourceDriverUnsupported):
		httpresp.Error(c, http.StatusServiceUnavailable, "DOWNLOADER_UNAVAILABLE", err.Error(), nil)
	case errors.Is(err, appsvc.ErrDownloaderAuthFailed):
		httpresp.Error(c, http.StatusServiceUnavailable, "DOWNLOADER_AUTH_FAILED", err.Error(), nil)
	case errors.Is(err, appsvc.ErrDownloaderUnavailable):
		httpresp.Error(c, http.StatusServiceUnavailable, "DOWNLOADER_UNAVAILABLE", err.Error(), nil)
	case errors.Is(err, appsvc.ErrTaskInvalidState):
		httpresp.Error(c, http.StatusConflict, "TASK_INVALID_STATE", err.Error(), nil)
	default:
		httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}

func parseUintQuery(c *gin.Context, name string) (uint, error) {
	value := c.Query(name)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	return uint(parsed), err
}
