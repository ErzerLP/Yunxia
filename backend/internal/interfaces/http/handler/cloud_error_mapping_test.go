package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	appsvc "yunxia/internal/application/service"
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

func TestWebDAVCloudRegionBlockedStatusMapping(t *testing.T) {
	if got := webDAVStatusFromError(appsvc.ErrCloudRegionBlocked); got != http.StatusUnavailableForLegalReasons {
		t.Fatalf("webDAVStatusFromError(region blocked) = %d, want %d", got, http.StatusUnavailableForLegalReasons)
	}
}
