package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	appsvc "yunxia/internal/application/service"
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

func TestWebDAVCloudRegionBlockedStatusMapping(t *testing.T) {
	if got := webDAVStatusFromError(appsvc.ErrCloudRegionBlocked); got != http.StatusUnavailableForLegalReasons {
		t.Fatalf("webDAVStatusFromError(region blocked) = %d, want %d", got, http.StatusUnavailableForLegalReasons)
	}
}
