package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	appsvc "yunxia/internal/application/service"
	domainrepo "yunxia/internal/domain/repository"
	domainstorage "yunxia/internal/domain/storage"
)

func TestCloudRegionBlockedErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name  string
		write func(*gin.Context)
	}{
		{
			name: "source",
			write: func(c *gin.Context) {
				(&SourceHandler{}).writeError(c, appsvc.ErrCloudRegionBlocked)
			},
		},
		{
			name: "file",
			write: func(c *gin.Context) {
				(&FileHandler{}).writeError(c, appsvc.ErrCloudRegionBlocked)
			},
		},
		{
			name: "task",
			write: func(c *gin.Context) {
				(&TaskHandler{}).writeError(c, appsvc.ErrCloudRegionBlocked)
			},
		},
		{
			name: "trash",
			write: func(c *gin.Context) {
				(&TrashHandler{}).writeError(c, appsvc.ErrCloudRegionBlocked, "TRASH_NOT_FOUND")
			},
		},
		{
			name: "upload",
			write: func(c *gin.Context) {
				(&UploadHandler{}).writeError(c, appsvc.ErrCloudRegionBlocked)
			},
		},
		{
			name: "vfs",
			write: func(c *gin.Context) {
				(&VFSHandler{}).writeError(c, appsvc.ErrCloudRegionBlocked)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)

			tc.write(ctx)

			if recorder.Code != http.StatusUnavailableForLegalReasons {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnavailableForLegalReasons)
			}
			var body struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Success || body.Code != "CLOUD_REGION_BLOCKED" {
				t.Fatalf("unexpected body = %+v", body)
			}
		})
	}
}

func TestDownloaderAuthFailedErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	downstreamErr := fmt.Errorf("%w: qbittorrent /api/v2/torrents/add status 401: Unauthorized", appsvc.ErrDownloaderAuthFailed)

	cases := []struct {
		name  string
		write func(*gin.Context)
	}{
		{
			name: "rss",
			write: func(c *gin.Context) {
				(&RSSHandler{}).writeError(c, downstreamErr, "RSS_ITEM_NOT_FOUND")
			},
		},
		{
			name: "task",
			write: func(c *gin.Context) {
				(&TaskHandler{}).writeError(c, downstreamErr)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)

			tc.write(ctx)

			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
			}
			var body struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Success || body.Code != "DOWNLOADER_AUTH_FAILED" {
				t.Fatalf("unexpected body = %+v raw=%s", body, recorder.Body.String())
			}
		})
	}
}

func TestVFSMetadataMutationSyncFailureMappingIsStable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	(&VFSHandler{}).writeError(ctx, fmt.Errorf("%w: sql row failed at D:\\secret\\object.json", appsvc.ErrMetadataVFSMutationSyncFailed))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	var body struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Success || body.Code != "METADATA_VFS_MUTATION_SYNC_FAILED" {
		t.Fatalf("unexpected body = %+v raw=%s", body, recorder.Body.String())
	}
	if body.Message != appsvc.ErrMetadataVFSMutationSyncFailed.Error() {
		t.Fatalf("metadata mutation message leaked details: %q", body.Message)
	}
}

func TestVFSSyncConflictErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	(&VFSHandler{}).writeError(ctx, appsvc.ErrVFSSyncConflict)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	var body struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Success || body.Code != "VFS_SYNC_CONFLICT" {
		t.Fatalf("unexpected body = %+v raw=%s", body, recorder.Body.String())
	}
}

func TestCloudCaptchaRequiredIncludesVerificationURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	providerErr := &domainstorage.ProviderError{
		Kind:            appsvc.ErrCloudCaptchaRequired,
		Message:         "cloud captcha required",
		ProviderCode:    "captcha_required",
		VerificationURL: "https://verify.example/captcha",
	}
	(&SourceHandler{}).writeError(ctx, fmt.Errorf("%w: %w", appsvc.ErrSourceConnectionFailed, providerErr))

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	var body struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Error   struct {
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Success || body.Code != "CLOUD_CAPTCHA_REQUIRED" {
		t.Fatalf("unexpected body = %+v", body)
	}
	if body.Error.Details["verification_url"] != "https://verify.example/captcha" || body.Error.Details["requires_manual_verification"] != true || body.Error.Details["provider_code"] != "captcha_required" {
		t.Fatalf("captcha details not exposed correctly: %+v", body.Error.Details)
	}
}

func TestSourceHandlerCloudCaptchaResourceNotFoundDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	providerErr := &domainstorage.ProviderError{
		Kind:            appsvc.ErrCloudCaptchaRequired,
		Message:         "cloud captcha required",
		ProviderCode:    "resource_not_found",
		VerificationURL: "https://verify.example/resource",
	}
	(&SourceHandler{}).writeError(ctx, providerErr)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	var body struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Error   struct {
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Success || body.Code != "CLOUD_CAPTCHA_REQUIRED" {
		t.Fatalf("unexpected body = %+v", body)
	}
	if body.Error.Details["verification_url"] != "https://verify.example/resource" || body.Error.Details["provider_code"] != "resource_not_found" {
		t.Fatalf("resource_not_found captcha details not exposed correctly: %+v", body.Error.Details)
	}
}

func TestSourceConnectionErrorDoesNotBecomeSourceNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	(&SourceHandler{}).writeError(ctx, fmt.Errorf("%w: %w", appsvc.ErrSourceConnectionFailed, domainrepo.ErrNotFound))

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	var body struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Success || body.Code != "SOURCE_CONNECTION_FAILED" {
		t.Fatalf("unexpected body = %+v raw=%s", body, recorder.Body.String())
	}
}

func TestWebDAVCloudRegionBlockedStatusMapping(t *testing.T) {
	if got := webDAVStatusFromError(appsvc.ErrCloudRegionBlocked); got != http.StatusUnavailableForLegalReasons {
		t.Fatalf("webDAVStatusFromError(region blocked) = %d, want %d", got, http.StatusUnavailableForLegalReasons)
	}
}
