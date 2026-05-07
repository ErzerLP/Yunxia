package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	appdto "yunxia/internal/application/dto"
	appsvc "yunxia/internal/application/service"
)

func TestVFSRefreshHandlerErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "file not found", err: appsvc.ErrFileNotFound, statusCode: http.StatusNotFound, code: "FILE_NOT_FOUND"},
		{name: "path invalid", err: appsvc.ErrPathInvalid, statusCode: http.StatusBadRequest, code: "PATH_INVALID"},
		{name: "acl denied", err: appsvc.ErrACLDenied, statusCode: http.StatusForbidden, code: "ACL_DENIED"},
		{name: "driver unsupported", err: appsvc.ErrSourceDriverUnsupported, statusCode: http.StatusUnprocessableEntity, code: "SOURCE_DRIVER_UNSUPPORTED"},
		{name: "provider unavailable", err: appsvc.ErrCloudProviderUnavailable, statusCode: http.StatusBadGateway, code: "CLOUD_PROVIDER_UNAVAILABLE"},
		{name: "sync conflict", err: appsvc.ErrVFSSyncConflict, statusCode: http.StatusConflict, code: "VFS_SYNC_CONFLICT"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			handler := &VFSHandler{vfsService: &vfsRefreshServiceStub{err: tc.err}}
			router.POST("/refresh", handler.Refresh)

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBufferString(`{"path":"/docs","mode":"sync"}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, req)

			if recorder.Code != tc.statusCode {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, tc.statusCode, recorder.Body.String())
			}
			var body struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Success || body.Code != tc.code {
				t.Fatalf("unexpected body = %+v raw=%s", body, recorder.Body.String())
			}
		})
	}
}

func TestVFSRefreshHandlerRejectsUnsupportedMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &VFSHandler{vfsService: &vfsRefreshServiceStub{}}
	router.POST("/refresh", handler.Refresh)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBufferString(`{"path":"/docs","mode":"async"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if handler.vfsService.(*vfsRefreshServiceStub).called {
		t.Fatalf("refresh service should not be called for unsupported mode")
	}
	var body struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Success || body.Code != "VALIDATION_ERROR" {
		t.Fatalf("unexpected body = %+v raw=%s", body, recorder.Body.String())
	}
}

func TestVFSRefreshHandlerDefaultsEmptyModeToSync(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	stub := &vfsRefreshServiceStub{}
	handler := &VFSHandler{vfsService: stub}
	router.POST("/refresh", handler.Refresh)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBufferString(`{"path":"/docs"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !stub.called || stub.req.Mode != "sync" {
		t.Fatalf("refresh service request = %+v called=%v, want mode sync", stub.req, stub.called)
	}
}

type vfsRefreshServiceStub struct {
	err    error
	called bool
	req    appdto.VFSRefreshRequest
}

func (s *vfsRefreshServiceStub) List(context.Context, string) (*appdto.VFSListResponse, error) {
	return nil, nil
}

func (s *vfsRefreshServiceStub) ResolvePath(context.Context, string) (appsvc.ResolvedPath, error) {
	return appsvc.ResolvedPath{}, nil
}

func (s *vfsRefreshServiceStub) Mkdir(context.Context, appdto.VFSMkdirRequest) (*appdto.VFSItem, error) {
	return nil, nil
}

func (s *vfsRefreshServiceStub) Rename(context.Context, appdto.VFSRenameRequest) (string, string, *appdto.VFSItem, error) {
	return "", "", nil, nil
}

func (s *vfsRefreshServiceStub) Move(context.Context, appdto.VFSMoveCopyRequest) (string, string, error) {
	return "", "", nil
}

func (s *vfsRefreshServiceStub) Copy(context.Context, appdto.VFSMoveCopyRequest) (string, string, error) {
	return "", "", nil
}

func (s *vfsRefreshServiceStub) Delete(context.Context, appdto.VFSDeleteRequest) (time.Time, error) {
	return time.Time{}, nil
}

func (s *vfsRefreshServiceStub) Search(context.Context, string, string) (*appdto.VFSSearchResponse, error) {
	return nil, nil
}

func (s *vfsRefreshServiceStub) Refresh(_ context.Context, req appdto.VFSRefreshRequest) (*appdto.VFSRefreshResponse, error) {
	s.called = true
	s.req = req
	if s.err != nil {
		return nil, s.err
	}
	return &appdto.VFSRefreshResponse{Path: "/docs", SyncState: "indexed"}, nil
}
