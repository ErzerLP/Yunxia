package http

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWebDAVWriteMethodsCreateAuditLogs(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)
	enableWebDAVForTest(t, engine, adminToken, sourceID)

	mkcolRec := performBasicRequest(engine, "MKCOL", "/dav/local/audit-docs", nil, "admin", "strong-password-123")
	if mkcolRec.Code != http.StatusCreated {
		t.Fatalf("MKCOL expected 201, got %d body=%s", mkcolRec.Code, mkcolRec.Body.String())
	}

	putRec := performBasicRequest(engine, http.MethodPut, "/dav/local/audit-docs/hello.txt", []byte("dav hello"), "admin", "strong-password-123")
	if putRec.Code != http.StatusCreated && putRec.Code != http.StatusNoContent {
		t.Fatalf("PUT expected 201/204, got %d body=%s", putRec.Code, putRec.Body.String())
	}

	copyRec := performWebDAVRequest(engine, "COPY", "/dav/local/audit-docs/hello.txt", nil, "admin", "strong-password-123", map[string]string{
		"Destination": "/dav/local/audit-docs/hello-copy.txt",
		"Overwrite":   "T",
	})
	if copyRec.Code != http.StatusCreated && copyRec.Code != http.StatusNoContent {
		t.Fatalf("COPY expected 201/204, got %d body=%s", copyRec.Code, copyRec.Body.String())
	}

	moveRec := performWebDAVRequest(engine, "MOVE", "/dav/local/audit-docs/hello-copy.txt", nil, "admin", "strong-password-123", map[string]string{
		"Destination": "/dav/local/audit-docs/hello-moved.txt",
		"Overwrite":   "T",
	})
	if moveRec.Code != http.StatusCreated && moveRec.Code != http.StatusNoContent {
		t.Fatalf("MOVE expected 201/204, got %d body=%s", moveRec.Code, moveRec.Body.String())
	}

	deleteRec := performBasicRequest(engine, http.MethodDelete, "/dav/local/audit-docs/hello-moved.txt", nil, "admin", "strong-password-123")
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE expected 204, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	assertAuditLogExists(t, engine, map[string]any{
		"entrypoint":     "webdav",
		"actor_username": "admin",
		"method":         "MKCOL",
		"path":           "/dav/local/audit-docs",
		"resource_type":  "file",
		"action":         "mkcol",
		"result":         "success",
		"source_id":      sourceID,
		"virtual_path":   "/local/audit-docs",
	})
	assertAuditLogExists(t, engine, map[string]any{
		"entrypoint":     "webdav",
		"actor_username": "admin",
		"method":         "PUT",
		"path":           "/dav/local/audit-docs/hello.txt",
		"resource_type":  "file",
		"action":         "put",
		"result":         "success",
		"source_id":      sourceID,
		"virtual_path":   "/local/audit-docs/hello.txt",
	})
	assertAuditLogExists(t, engine, map[string]any{
		"entrypoint":     "webdav",
		"actor_username": "admin",
		"method":         "COPY",
		"path":           "/dav/local/audit-docs/hello.txt",
		"resource_type":  "file",
		"action":         "copy",
		"result":         "success",
		"source_id":      sourceID,
		"virtual_path":   "/local/audit-docs/hello-copy.txt",
	})
	assertAuditLogExists(t, engine, map[string]any{
		"entrypoint":     "webdav",
		"actor_username": "admin",
		"method":         "MOVE",
		"path":           "/dav/local/audit-docs/hello-copy.txt",
		"resource_type":  "file",
		"action":         "move",
		"result":         "success",
		"source_id":      sourceID,
		"virtual_path":   "/local/audit-docs/hello-moved.txt",
	})
	assertAuditLogExists(t, engine, map[string]any{
		"entrypoint":     "webdav",
		"actor_username": "admin",
		"method":         "DELETE",
		"path":           "/dav/local/audit-docs/hello-moved.txt",
		"resource_type":  "file",
		"action":         "delete",
		"result":         "success",
		"source_id":      sourceID,
		"virtual_path":   "/local/audit-docs/hello-moved.txt",
	})
}

func TestAuditWriteFailureDoesNotBreakBusinessSuccess(t *testing.T) {
	engine := newStorageTestRouterWithFailingAuditRepo(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)
	enableWebDAVForTest(t, engine, adminToken, sourceID)
	resetRuntimeLogBuffers(t, engine)

	rec := performBasicRequest(engine, "MKCOL", "/dav/local/failing-audit", nil, "admin", "strong-password-123")
	if rec.Code != http.StatusCreated {
		t.Fatalf("MKCOL expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	assertRuntimeLogContains(t, engine, `"event":"audit.write.failed"`)
	assertRuntimeLogContains(t, engine, `"action":"mkcol"`)
}

func enableWebDAVForTest(t *testing.T, engine *gin.Engine, accessToken string, sourceID int) {
	t.Helper()

	detailRec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/sources/%d", sourceID), nil, accessToken)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("source detail expected 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	detail := decodeEnvelope[sourceDetailData](t, detailRec.Body.Bytes())

	updateRec := performRequest(t, engine, http.MethodPut, fmt.Sprintf("/api/v1/sources/%d", sourceID), map[string]any{
		"name":              detail.Source["name"],
		"is_enabled":        true,
		"is_webdav_exposed": true,
		"webdav_read_only":  false,
		"root_path":         detail.Source["root_path"],
		"sort_order":        0,
		"config":            detail.Config,
		"secret_patch":      map[string]any{},
	}, accessToken)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("source update expected 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
}

func performWebDAVRequest(engine *gin.Engine, method string, requestPath string, body []byte, username string, password string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, requestPath, bytes.NewReader(body))
	req.Header.Set("Authorization", basicAuthHeader(username, password))
	req.Header.Set("X-Forwarded-Proto", "https")
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}
