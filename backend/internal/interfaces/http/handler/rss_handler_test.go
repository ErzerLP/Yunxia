package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	appdto "yunxia/internal/application/dto"
	appsvc "yunxia/internal/application/service"
	domainrepo "yunxia/internal/domain/repository"
)

func TestRSSHandlerDownloadItemMapsDownloaderAuthFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewRSSHandler(&rssHandlerStub{
		downloadErr: fmt.Errorf("%w: qbittorrent /api/v2/torrents/add status 401: Unauthorized", appsvc.ErrDownloaderAuthFailed),
	})
	router.POST("/items/:id/download", handler.DownloadItem)

	req := httptest.NewRequest(http.MethodPost, "/items/1/download", strings.NewReader(`{"subscription_id":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"DOWNLOADER_AUTH_FAILED"`) {
		t.Fatalf("expected DOWNLOADER_AUTH_FAILED, got %s", body)
	}
	if strings.Contains(body, `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("downstream auth failure must not map to INTERNAL_ERROR: %s", body)
	}
}

type rssHandlerStub struct {
	downloadErr error
}

func (s *rssHandlerStub) ListSources(context.Context) (*appdto.RSSSourceListResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) CreateSource(context.Context, appdto.RSSSourceUpsertRequest) (*appdto.RSSSourceView, error) {
	return nil, nil
}
func (s *rssHandlerStub) GetSource(context.Context, uint) (*appdto.RSSSourceView, error) {
	return nil, nil
}
func (s *rssHandlerStub) UpdateSource(context.Context, uint, appdto.RSSSourceUpsertRequest) (*appdto.RSSSourceView, error) {
	return nil, nil
}
func (s *rssHandlerStub) DeleteSource(context.Context, uint) error { return nil }
func (s *rssHandlerStub) RefreshSource(context.Context, uint) (*appdto.RSSRefreshResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) RefreshAllSources(context.Context) (*appdto.RSSRefreshAllResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) ExportConfig(context.Context) (*appdto.RSSExportResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) ImportConfig(context.Context, appdto.RSSImportRequest) (*appdto.RSSImportResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) ListSubscriptions(context.Context, uint) (*appdto.RSSSubscriptionListResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) CreateSubscription(context.Context, appdto.RSSSubscriptionUpsertRequest) (*appdto.RSSSubscriptionView, error) {
	return nil, nil
}
func (s *rssHandlerStub) CloneSubscription(context.Context, uint, appdto.RSSSubscriptionCloneRequest) (*appdto.RSSSubscriptionView, error) {
	return nil, nil
}
func (s *rssHandlerStub) GetSubscription(context.Context, uint) (*appdto.RSSSubscriptionView, error) {
	return nil, nil
}
func (s *rssHandlerStub) UpdateSubscription(context.Context, uint, appdto.RSSSubscriptionUpsertRequest) (*appdto.RSSSubscriptionView, error) {
	return nil, nil
}
func (s *rssHandlerStub) DeleteSubscription(context.Context, uint) error { return nil }
func (s *rssHandlerStub) BatchUpdateSubscriptionState(context.Context, appdto.RSSSubscriptionBatchStateRequest) (*appdto.RSSSubscriptionBatchStateResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) RunSubscription(context.Context, uint) (*appdto.RSSRefreshResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) PreviewSubscription(context.Context, uint) (*appdto.RSSSubscriptionPreviewResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) PreviewSubscriptionRules(context.Context, appdto.RSSSubscriptionPreviewRequest) (*appdto.RSSSubscriptionPreviewResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) ListItems(context.Context, domainrepo.RSSItemFilter) (*appdto.RSSItemListResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) DownloadItem(context.Context, uint, appdto.RSSManualDownloadRequest) (*appdto.RSSItemView, error) {
	return nil, s.downloadErr
}
func (s *rssHandlerStub) ReprocessItem(context.Context, uint) (*appdto.RSSItemView, error) {
	return nil, nil
}
func (s *rssHandlerStub) RetryItem(context.Context, uint, appdto.RSSManualDownloadRequest) (*appdto.RSSItemView, error) {
	return nil, nil
}
func (s *rssHandlerStub) BatchIgnoreItems(context.Context, appdto.RSSItemBatchIgnoreRequest) (*appdto.RSSItemBatchActionResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) BatchRetryItems(context.Context, appdto.RSSItemBatchRetryRequest) (*appdto.RSSItemBatchActionResponse, error) {
	return nil, nil
}
func (s *rssHandlerStub) QBitHealth(context.Context) (*appdto.RSSQBitHealthResponse, error) {
	return nil, nil
}
