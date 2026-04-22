package http

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

type taskListData struct {
	Items []map[string]any `json:"items"`
}

type taskCreateData struct {
	Task map[string]any `json:"task"`
}

func TestTaskLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, sourceID := bootstrapAdmin(t, engine)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/tasks", map[string]any{
		"type":      "download",
		"url":       "https://example.com/archive.zip",
		"source_id": sourceID,
		"save_path": "/downloads",
	}, accessToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create task expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeEnvelope[taskCreateData](t, rec.Body.Bytes())
	taskID := int(created.Task["id"].(float64))
	if created.Task["save_virtual_path"] != "/local/downloads" ||
		int(created.Task["resolved_source_id"].(float64)) != sourceID ||
		created.Task["resolved_inner_save_path"] != "/downloads" {
		t.Fatalf("expected task virtual snapshots, got %+v", created.Task)
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/tasks", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("list tasks expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[taskListData](t, rec.Body.Bytes())
	if len(listed.Items) == 0 {
		t.Fatalf("expected at least one task, got %+v", listed)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%d", taskID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("get task expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	got := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if got["save_virtual_path"] != "/local/downloads" ||
		int(got["resolved_source_id"].(float64)) != sourceID ||
		got["resolved_inner_save_path"] != "/downloads" {
		t.Fatalf("expected task detail virtual snapshots, got %+v", got)
	}

	rec = performRequest(t, engine, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/pause", taskID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("pause task expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/resume", taskID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("resume task expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%d?delete_file=false", taskID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete task expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTaskSavePathACLFlow(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	enableMultiUserForTest(t, engine, adminToken)
	userID, userToken := createNormalUserAndLoginForTest(t, engine, adminToken, "task-user", "strong-password-456")

	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/downloads", map[string]any{
		"read":   true,
		"write":  true,
		"delete": true,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/private-downloads", map[string]any{
		"read":   true,
		"write":  true,
		"delete": true,
		"share":  false,
	}, "deny", 200, true)

	allowedRec := performRequest(t, engine, http.MethodPost, "/api/v1/tasks", map[string]any{
		"type":      "download",
		"url":       "https://example.com/allowed.zip",
		"source_id": sourceID,
		"save_path": "/downloads",
	}, userToken)
	if allowedRec.Code != http.StatusAccepted {
		t.Fatalf("allowed task create expected 202, got %d body=%s", allowedRec.Code, allowedRec.Body.String())
	}
	allowedTask := decodeEnvelope[taskCreateData](t, allowedRec.Body.Bytes())
	allowedTaskID := int(allowedTask.Task["id"].(float64))

	blockedRec := performRequest(t, engine, http.MethodPost, "/api/v1/tasks", map[string]any{
		"type":      "download",
		"url":       "https://example.com/blocked.zip",
		"source_id": sourceID,
		"save_path": "/private-downloads",
	}, userToken)
	if blockedRec.Code != http.StatusForbidden {
		t.Fatalf("blocked task create expected 403, got %d body=%s", blockedRec.Code, blockedRec.Body.String())
	}
	assertFailureCode(t, blockedRec.Body.Bytes(), "PERMISSION_DENIED")

	adminPrivateRec := performRequest(t, engine, http.MethodPost, "/api/v1/tasks", map[string]any{
		"type":      "download",
		"url":       "https://example.com/private-admin.zip",
		"source_id": sourceID,
		"save_path": "/private-downloads",
	}, adminToken)
	if adminPrivateRec.Code != http.StatusAccepted {
		t.Fatalf("admin private task create expected 202, got %d body=%s", adminPrivateRec.Code, adminPrivateRec.Body.String())
	}
	adminPrivateTask := decodeEnvelope[taskCreateData](t, adminPrivateRec.Body.Bytes())
	adminPrivateTaskID := int(adminPrivateTask.Task["id"].(float64))

	listRec := performRequest(t, engine, http.MethodGet, "/api/v1/tasks", nil, userToken)
	if listRec.Code != http.StatusOK {
		t.Fatalf("task list expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	listed := decodeEnvelope[taskListData](t, listRec.Body.Bytes())
	if len(listed.Items) != 1 {
		t.Fatalf("expected only one visible task, got %+v", listed.Items)
	}
	if int(listed.Items[0]["id"].(float64)) != allowedTaskID {
		t.Fatalf("expected visible task id=%d, got %+v", allowedTaskID, listed.Items)
	}

	getBlockedRec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%d", adminPrivateTaskID), nil, userToken)
	if getBlockedRec.Code != http.StatusForbidden {
		t.Fatalf("blocked task get expected 403, got %d body=%s", getBlockedRec.Code, getBlockedRec.Body.String())
	}
	assertFailureCode(t, getBlockedRec.Body.Bytes(), "PERMISSION_DENIED")

	pauseBlockedRec := performRequest(t, engine, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/pause", adminPrivateTaskID), nil, userToken)
	if pauseBlockedRec.Code != http.StatusForbidden {
		t.Fatalf("blocked task pause expected 403, got %d body=%s", pauseBlockedRec.Code, pauseBlockedRec.Body.String())
	}
	assertFailureCode(t, pauseBlockedRec.Body.Bytes(), "PERMISSION_DENIED")
}

func TestTaskOwnerIsolationFlow(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	enableMultiUserForTest(t, engine, adminToken)
	userID, userToken := createNormalUserAndLoginForTest(t, engine, adminToken, "task-owner", "strong-password-456")

	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/downloads", map[string]any{
		"read":   true,
		"write":  true,
		"delete": true,
		"share":  false,
	}, "allow", 100, true)

	adminRec := performRequest(t, engine, http.MethodPost, "/api/v1/tasks", map[string]any{
		"type":      "download",
		"url":       "https://example.com/admin-owned.zip",
		"source_id": sourceID,
		"save_path": "/downloads",
	}, adminToken)
	if adminRec.Code != http.StatusAccepted {
		t.Fatalf("admin task create expected 202, got %d body=%s", adminRec.Code, adminRec.Body.String())
	}
	adminTask := decodeEnvelope[taskCreateData](t, adminRec.Body.Bytes())
	adminTaskID := int(adminTask.Task["id"].(float64))

	userRec := performRequest(t, engine, http.MethodPost, "/api/v1/tasks", map[string]any{
		"type":      "download",
		"url":       "https://example.com/user-owned.zip",
		"source_id": sourceID,
		"save_path": "/downloads",
	}, userToken)
	if userRec.Code != http.StatusAccepted {
		t.Fatalf("user task create expected 202, got %d body=%s", userRec.Code, userRec.Body.String())
	}
	userTask := decodeEnvelope[taskCreateData](t, userRec.Body.Bytes())
	userTaskID := int(userTask.Task["id"].(float64))

	listRec := performRequest(t, engine, http.MethodGet, "/api/v1/tasks", nil, userToken)
	if listRec.Code != http.StatusOK {
		t.Fatalf("user task list expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	userList := decodeEnvelope[taskListData](t, listRec.Body.Bytes())
	if len(userList.Items) != 1 {
		t.Fatalf("expected user to see only own task, got %+v", userList.Items)
	}
	if int(userList.Items[0]["id"].(float64)) != userTaskID {
		t.Fatalf("expected visible task id=%d, got %+v", userTaskID, userList.Items)
	}

	adminListRec := performRequest(t, engine, http.MethodGet, "/api/v1/tasks", nil, adminToken)
	if adminListRec.Code != http.StatusOK {
		t.Fatalf("admin task list expected 200, got %d body=%s", adminListRec.Code, adminListRec.Body.String())
	}
	adminList := decodeEnvelope[taskListData](t, adminListRec.Body.Bytes())
	if len(adminList.Items) != 2 {
		t.Fatalf("expected admin to see all tasks, got %+v", adminList.Items)
	}

	getBlockedRec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%d", adminTaskID), nil, userToken)
	if getBlockedRec.Code != http.StatusForbidden {
		t.Fatalf("user get admin task expected 403, got %d body=%s", getBlockedRec.Code, getBlockedRec.Body.String())
	}
	assertFailureCode(t, getBlockedRec.Body.Bytes(), "PERMISSION_DENIED")

	pauseBlockedRec := performRequest(t, engine, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/pause", adminTaskID), nil, userToken)
	if pauseBlockedRec.Code != http.StatusForbidden {
		t.Fatalf("user pause admin task expected 403, got %d body=%s", pauseBlockedRec.Code, pauseBlockedRec.Body.String())
	}
	assertFailureCode(t, pauseBlockedRec.Body.Bytes(), "PERMISSION_DENIED")
}

func TestNavigationSourcesACLVisibility(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, defaultSourceID := bootstrapAdmin(t, engine)

	enableMultiUserForTest(t, engine, adminToken)
	userID, userToken := createNormalUserAndLoginForTest(t, engine, adminToken, "nav-user", "strong-password-456")

	basePath := filepath.ToSlash(filepath.Join(t.TempDir(), "library-source"))
	createRec := performRequest(t, engine, http.MethodPost, "/api/v1/sources", map[string]any{
		"name":              "媒体仓库",
		"driver_type":       "local",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"root_path":         "/",
		"sort_order":        10,
		"config":            map[string]any{"base_path": basePath},
		"secret_patch":      map[string]any{},
	}, adminToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create secondary source expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	created := decodeEnvelope[sourceCreateData](t, createRec.Body.Bytes())
	secondSourceID := int(created.Source["id"].(float64))

	createACLRuleForTest(t, engine, adminToken, secondSourceID, userID, "/media", map[string]any{
		"read":   true,
		"write":  false,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)

	navRec := performRequest(t, engine, http.MethodGet, "/api/v1/sources?view=navigation", nil, userToken)
	if navRec.Code != http.StatusOK {
		t.Fatalf("navigation sources expected 200, got %d body=%s", navRec.Code, navRec.Body.String())
	}
	nav := decodeEnvelope[sourceListData](t, navRec.Body.Bytes())
	if len(nav.Items) != 1 {
		t.Fatalf("expected only one visible navigation source, got %+v", nav.Items)
	}
	gotID := int(nav.Items[0]["id"].(float64))
	if gotID != secondSourceID {
		t.Fatalf("expected visible source=%d and hidden default=%d, got %+v", secondSourceID, defaultSourceID, nav.Items)
	}
}

func TestWebDAVLocalSourceWorkflow(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, sourceID := bootstrapAdmin(t, engine)

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

	mkcolRec := performBasicRequest(engine, "MKCOL", "/dav/local/library", nil, "admin", "strong-password-123")
	if mkcolRec.Code != http.StatusCreated && mkcolRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("MKCOL expected 201 or 405-compatible, got %d body=%s", mkcolRec.Code, mkcolRec.Body.String())
	}

	putRec := performBasicRequest(engine, http.MethodPut, "/dav/local/library/hello.txt", []byte("dav hello"), "admin", "strong-password-123")
	if putRec.Code != http.StatusCreated && putRec.Code != http.StatusNoContent {
		t.Fatalf("PUT dav expected 201/204, got %d body=%s", putRec.Code, putRec.Body.String())
	}

	getRec := performBasicRequest(engine, http.MethodGet, "/dav/local/library/hello.txt", nil, "admin", "strong-password-123")
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET dav expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if getRec.Body.String() != "dav hello" {
		t.Fatalf("unexpected dav body = %q", getRec.Body.String())
	}

	propfindReq := httptest.NewRequest("PROPFIND", "/dav/local/library", nil)
	propfindReq.Header.Set("Depth", "1")
	propfindReq.Header.Set("Authorization", basicAuthHeader("admin", "strong-password-123"))
	propfindReq.Header.Set("X-Forwarded-Proto", "https")
	propfindRec := httptest.NewRecorder()
	engine.ServeHTTP(propfindRec, propfindReq)
	if propfindRec.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND expected 207, got %d body=%s", propfindRec.Code, propfindRec.Body.String())
	}
}

func TestWebDAVRejectsPlainHTTP(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, sourceID := bootstrapAdmin(t, engine)

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

	req := httptest.NewRequest(http.MethodGet, "/dav/local", nil)
	req.Header.Set("Authorization", basicAuthHeader("admin", "strong-password-123"))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("plain http webdav expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWebDAVACLForNormalUser(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	enableMultiUserForTest(t, engine, adminToken)
	userID, _ := createNormalUserAndLoginForTest(t, engine, adminToken, "dav-user", "strong-password-456")

	detailRec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/sources/%d", sourceID), nil, adminToken)
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
	}, adminToken)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("source update expected 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/library", map[string]any{
		"read":   true,
		"write":  true,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/private", map[string]any{
		"read":   true,
		"write":  true,
		"delete": true,
		"share":  false,
	}, "deny", 200, true)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "library",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir library expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "private",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir private expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/private", "secret.txt", []byte("secret"))

	putRec := performBasicRequest(engine, http.MethodPut, "/dav/local/library/hello.txt", []byte("dav hello"), "dav-user", "strong-password-456")
	if putRec.Code != http.StatusCreated && putRec.Code != http.StatusNoContent {
		t.Fatalf("normal user dav PUT to library expected 201/204, got %d body=%s", putRec.Code, putRec.Body.String())
	}

	getRec := performBasicRequest(engine, http.MethodGet, "/dav/local/library/hello.txt", nil, "dav-user", "strong-password-456")
	if getRec.Code != http.StatusOK {
		t.Fatalf("normal user dav GET library expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	blockedRec := performBasicRequest(engine, http.MethodGet, "/dav/local/private/secret.txt", nil, "dav-user", "strong-password-456")
	if blockedRec.Code != http.StatusForbidden {
		t.Fatalf("normal user dav GET private expected 403, got %d body=%s", blockedRec.Code, blockedRec.Body.String())
	}
}

func performBasicRequest(engine *gin.Engine, method, path string, body []byte, username, password string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", basicAuthHeader(username, password))
	req.Header.Set("X-Forwarded-Proto", "https")
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func basicAuthHeader(username, password string) string {
	token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return "Basic " + token
}
