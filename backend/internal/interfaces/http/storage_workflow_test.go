package http

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type sourceListData struct {
	Items []map[string]any `json:"items"`
	View  string           `json:"view"`
}

type sourceCreateData struct {
	Source map[string]any `json:"source"`
}

type sourceDetailData struct {
	Source        map[string]any `json:"source"`
	Config        map[string]any `json:"config"`
	SecretFields  map[string]any `json:"secret_fields"`
	LastCheckedAt string         `json:"last_checked_at"`
}

type sourceTestData struct {
	Reachable bool   `json:"reachable"`
	Status    string `json:"status"`
}

type uploadInitData struct {
	IsFastUpload bool `json:"is_fast_upload"`
	Upload       struct {
		UploadID                string `json:"upload_id"`
		SourceID                int    `json:"source_id"`
		Path                    string `json:"path"`
		Filename                string `json:"filename"`
		ChunkSize               int64  `json:"chunk_size"`
		TotalChunks             int    `json:"total_chunks"`
		Status                  string `json:"status"`
		TargetVirtualParentPath string `json:"target_virtual_parent_path"`
		ResolvedSourceID        int    `json:"resolved_source_id"`
		ResolvedInnerParentPath string `json:"resolved_inner_parent_path"`
	} `json:"upload"`
	Transport struct {
		Mode       string `json:"mode"`
		DriverType string `json:"driver_type"`
	} `json:"transport"`
	PartInstructions []struct {
		Index     int               `json:"index"`
		Method    string            `json:"method"`
		URL       string            `json:"url"`
		Headers   map[string]string `json:"headers"`
		ExpiresAt string            `json:"expires_at"`
		ByteRange struct {
			Start int64 `json:"start"`
			End   int64 `json:"end"`
		} `json:"byte_range"`
	} `json:"part_instructions"`
}

type uploadSessionListData struct {
	Items []map[string]any `json:"items"`
}

type uploadFinishData struct {
	Completed bool           `json:"completed"`
	UploadID  string         `json:"upload_id"`
	File      map[string]any `json:"file"`
}

type mkdirData struct {
	Created map[string]any `json:"created"`
}

type fileListData struct {
	Items           []map[string]any `json:"items"`
	CurrentPath     string           `json:"current_path"`
	CurrentSourceID int              `json:"current_source_id"`
}

type accessURLData struct {
	URL       string `json:"url"`
	Method    string `json:"method"`
	ExpiresAt string `json:"expires_at"`
}

type trashListData struct {
	Items []map[string]any `json:"items"`
}

type trashRestoreData struct {
	ID                  int    `json:"id"`
	Restored            bool   `json:"restored"`
	RestoredPath        string `json:"restored_path"`
	RestoredVirtualPath string `json:"restored_virtual_path"`
}

func TestS3SourceCreateDetailAndFileAccessLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/sources/test", map[string]any{
		"name":              "S3 媒体库",
		"driver_type":       "s3",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"root_path":         "/",
		"sort_order":        30,
		"config": map[string]any{
			"endpoint":         "https://s3.example.com",
			"region":           "us-east-1",
			"bucket":           "media",
			"base_prefix":      "library",
			"force_path_style": true,
		},
		"secret_patch": map[string]any{
			"access_key": "AKIA-TEST-1234",
			"secret_key": "secret-value",
		},
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 source test expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/sources", map[string]any{
		"name":              "S3 媒体库",
		"driver_type":       "s3",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"root_path":         "/",
		"sort_order":        30,
		"config": map[string]any{
			"endpoint":         "https://s3.example.com",
			"region":           "us-east-1",
			"bucket":           "media",
			"base_prefix":      "library",
			"force_path_style": true,
		},
		"secret_patch": map[string]any{
			"access_key": "AKIA-TEST-1234",
			"secret_key": "secret-value",
		},
	}, accessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("s3 source create expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeEnvelope[sourceCreateData](t, rec.Body.Bytes())
	sourceID := int(created.Source["id"].(float64))

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/sources/%d", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 source detail expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	detail := decodeEnvelope[sourceDetailData](t, rec.Body.Bytes())
	if detail.Config["bucket"] != "media" || detail.Config["base_prefix"] != "library" {
		t.Fatalf("unexpected s3 source config = %+v", detail.Config)
	}
	if detail.Config["access_key"] != "AKIA-TEST-1234" || detail.Config["secret_key"] != "secret-value" {
		t.Fatalf("expected super admin to see secret config, got config=%+v", detail.Config)
	}
	accessKeyMask := detail.SecretFields["access_key"].(map[string]any)
	if accessKeyMask["configured"] != true {
		t.Fatalf("expected access_key configured, got %+v", detail.SecretFields)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files?source_id=%d&path=/&page=1&page_size=200&sort_by=name&sort_order=asc", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 files list expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[fileListData](t, rec.Body.Bytes())
	if len(listed.Items) == 0 {
		t.Fatalf("expected s3 file list items, got %+v", listed)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/access-url", map[string]any{
		"source_id":   sourceID,
		"path":        "/movies/demo.mp4",
		"purpose":     "preview",
		"disposition": "inline",
		"expires_in":  300,
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 access-url expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	accessURL := decodeEnvelope[accessURLData](t, rec.Body.Bytes())
	if accessURL.Method != http.MethodGet || accessURL.URL == "" {
		t.Fatalf("unexpected s3 access-url = %+v", accessURL)
	}
}

func TestS3FileSearchLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, accessToken)

	rec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/search?source_id=%d&keyword=demo&path_prefix=/movies&page=1&page_size=50", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 files search expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	searched := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	items := searched["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 s3 search result, got %+v", searched)
	}
	first := items[0].(map[string]any)
	if first["path"] != "/movies/demo.mp4" {
		t.Fatalf("unexpected s3 search result = %+v", first)
	}
	if searched["current_source_id"].(float64) != float64(sourceID) {
		t.Fatalf("unexpected s3 search source id = %+v", searched)
	}
	if searched["path_prefix"] != "/movies" {
		t.Fatalf("unexpected s3 search path_prefix = %+v", searched)
	}
}

func TestS3DownloadRedirectLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, accessToken)

	rec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/download?source_id=%d&path=%s&disposition=inline", sourceID, url.QueryEscape("/movies/demo.mp4")), nil, accessToken)
	if rec.Code != http.StatusFound {
		t.Fatalf("s3 download expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatalf("expected redirect location, got headers=%v", rec.Header())
	}
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("url.Parse(location) error = %v", err)
	}
	if parsed.Host != "fake-s3.local" {
		t.Fatalf("unexpected redirect host = %s location=%s", parsed.Host, location)
	}
	if parsed.Query().Get("path") != "/movies/demo.mp4" {
		t.Fatalf("unexpected redirect path query = %s", location)
	}
	if parsed.Query().Get("disposition") != "inline" {
		t.Fatalf("unexpected redirect disposition query = %s", location)
	}
}

func TestS3AccessURLRedirectLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, accessToken)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/access-url", map[string]any{
		"source_id":   sourceID,
		"path":        "/movies/demo.mp4",
		"purpose":     "download",
		"disposition": "attachment",
		"expires_in":  300,
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 access-url expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	accessURL := decodeEnvelope[accessURLData](t, rec.Body.Bytes())
	if accessURL.Method != http.MethodGet {
		t.Fatalf("unexpected s3 access-url method = %+v", accessURL)
	}
	if got := accessURL.URL; len(got) < len("/api/v1/files/download?") || got[:len("/api/v1/files/download?")] != "/api/v1/files/download?" {
		t.Fatalf("expected backend download url, got %s", accessURL.URL)
	}

	redirectReq := newBinaryRequest(t, http.MethodGet, accessURL.URL, nil, "")
	redirectRec := httptestDo(engine, redirectReq)
	if redirectRec.Code != http.StatusFound {
		t.Fatalf("s3 access-url redirect expected 302, got %d body=%s", redirectRec.Code, redirectRec.Body.String())
	}
	location := redirectRec.Header().Get("Location")
	if location == "" {
		t.Fatalf("expected redirect location, got headers=%v", redirectRec.Header())
	}
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("url.Parse(location) error = %v", err)
	}
	if parsed.Host != "fake-s3.local" {
		t.Fatalf("unexpected redirect host = %s location=%s", parsed.Host, location)
	}
}

func TestS3UploadInitAndFinishLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, accessToken)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             "/uploads",
		"filename":         "archive.zip",
		"file_size":        11 * 1024 * 1024,
		"file_hash":        "md5-archive",
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 upload init expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	initPayload := decodeEnvelope[uploadInitData](t, rec.Body.Bytes())
	if initPayload.Transport.Mode != "direct_parts" || initPayload.Transport.DriverType != "s3" {
		t.Fatalf("unexpected s3 upload transport = %+v", initPayload.Transport)
	}
	if initPayload.Upload.UploadID == "" || len(initPayload.PartInstructions) == 0 {
		t.Fatalf("expected multipart instructions, got %+v", initPayload)
	}

	parts := make([]map[string]any, 0, len(initPayload.PartInstructions))
	for _, instruction := range initPayload.PartInstructions {
		parts = append(parts, map[string]any{
			"index": instruction.Index,
			"etag":  fmt.Sprintf("\"etag-part-%d\"", instruction.Index),
		})
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/upload/finish", map[string]any{
		"upload_id": initPayload.Upload.UploadID,
		"parts":     parts,
	}, accessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("s3 upload finish expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	finished := decodeEnvelope[uploadFinishData](t, rec.Body.Bytes())
	if !finished.Completed || finished.File["path"] != "/uploads/archive.zip" {
		t.Fatalf("unexpected s3 upload finish payload = %+v", finished)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files?source_id=%d&path=/uploads&page=1&page_size=200&sort_by=name&sort_order=asc", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 files list after upload expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[fileListData](t, rec.Body.Bytes())
	found := false
	for _, item := range listed.Items {
		if item["path"] == "/uploads/archive.zip" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected uploaded archive.zip in s3 list, got %+v", listed.Items)
	}
}

func TestS3PermanentDeleteLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, accessToken)

	rec := performRequest(t, engine, http.MethodDelete, "/api/v1/files", map[string]any{
		"source_id":   sourceID,
		"path":        "/movies/demo.mp4",
		"delete_mode": "permanent",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 delete expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/search?source_id=%d&keyword=demo&path_prefix=/movies&page=1&page_size=50", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 search after delete expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	searched := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	items := searched["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("expected deleted s3 object to disappear, got %+v", items)
	}
}

func TestS3TrashLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, accessToken)

	rec := performRequest(t, engine, http.MethodDelete, "/api/v1/files", map[string]any{
		"source_id":   sourceID,
		"path":        "/movies/demo.mp4",
		"delete_mode": "trash",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 trash delete expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	deleted := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if deleted["delete_mode"] != "trash" {
		t.Fatalf("unexpected s3 trash payload = %+v", deleted)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/search?source_id=%d&keyword=demo&path_prefix=/&page=1&page_size=50", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 search after trash expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	searched := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	items := searched["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("expected trashed s3 object to stay hidden from search, got %+v", items)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files?source_id=%d&path=/&page=1&page_size=200&sort_by=name&sort_order=asc", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 root list after trash expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[fileListData](t, rec.Body.Bytes())
	for _, item := range listed.Items {
		if item["name"] == ".trash" {
			t.Fatalf("expected .trash to stay hidden from list, got %+v", listed.Items)
		}
	}
}

func TestS3RenameMoveCopyLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, accessToken)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/rename", map[string]any{
		"source_id": sourceID,
		"path":      "/movies/demo.mp4",
		"new_name":  "demo-1080p.mp4",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 rename expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	renamed := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if renamed["old_path"] != "/movies/demo.mp4" || renamed["new_path"] != "/movies/demo-1080p.mp4" {
		t.Fatalf("unexpected s3 rename payload = %+v", renamed)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/move", map[string]any{
		"source_id":   sourceID,
		"path":        "/movies/demo-1080p.mp4",
		"target_path": "/covers",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 move expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	moved := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if moved["old_path"] != "/movies/demo-1080p.mp4" || moved["new_path"] != "/covers/demo-1080p.mp4" {
		t.Fatalf("unexpected s3 move payload = %+v", moved)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/copy", map[string]any{
		"source_id":   sourceID,
		"path":        "/covers/demo-1080p.mp4",
		"target_path": "/movies",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 copy expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	copied := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if copied["source_path"] != "/covers/demo-1080p.mp4" || copied["new_path"] != "/movies/demo-1080p.mp4" {
		t.Fatalf("unexpected s3 copy payload = %+v", copied)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/search?source_id=%d&keyword=demo-1080p&path_prefix=/movies&page=1&page_size=50", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 search after copy expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	searched := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	items := searched["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected renamed/copied result in movies, got %+v", searched)
	}
}

func TestS3DirectoryRenameMoveCopyLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, accessToken)

	uploadS3ObjectForTest(t, engine, accessToken, sourceID, "/albums/2026", "cover.jpg")
	uploadS3ObjectForTest(t, engine, accessToken, sourceID, "/albums/2026/raw", "frame.png")

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/rename", map[string]any{
		"source_id": sourceID,
		"path":      "/albums/2026",
		"new_name":  "2026-remastered",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 directory rename expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	renamed := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if renamed["old_path"] != "/albums/2026" || renamed["new_path"] != "/albums/2026-remastered" {
		t.Fatalf("unexpected s3 directory rename payload = %+v", renamed)
	}

	assertS3SearchPaths(t, engine, accessToken, sourceID, "/albums/2026-remastered", "frame", []string{
		"/albums/2026-remastered/raw/frame.png",
	})
	assertS3SearchPaths(t, engine, accessToken, sourceID, "/albums/2026", "frame", nil)

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "archive",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 mkdir archive expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/move", map[string]any{
		"source_id":   sourceID,
		"path":        "/albums/2026-remastered",
		"target_path": "/archive",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 directory move expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	moved := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if moved["old_path"] != "/albums/2026-remastered" || moved["new_path"] != "/archive/2026-remastered" {
		t.Fatalf("unexpected s3 directory move payload = %+v", moved)
	}

	assertS3SearchPaths(t, engine, accessToken, sourceID, "/archive/2026-remastered", "cover", []string{
		"/archive/2026-remastered/cover.jpg",
	})
	assertS3SearchPaths(t, engine, accessToken, sourceID, "/archive/2026-remastered", "frame", []string{
		"/archive/2026-remastered/raw/frame.png",
	})

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "backup",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 mkdir backup expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/copy", map[string]any{
		"source_id":   sourceID,
		"path":        "/archive/2026-remastered",
		"target_path": "/backup",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 directory copy expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	copied := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if copied["source_path"] != "/archive/2026-remastered" || copied["new_path"] != "/backup/2026-remastered" {
		t.Fatalf("unexpected s3 directory copy payload = %+v", copied)
	}

	assertS3SearchPaths(t, engine, accessToken, sourceID, "/backup/2026-remastered", "cover", []string{
		"/backup/2026-remastered/cover.jpg",
	})
	assertS3SearchPaths(t, engine, accessToken, sourceID, "/backup/2026-remastered", "frame", []string{
		"/backup/2026-remastered/raw/frame.png",
	})
}

func TestS3MkdirLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, accessToken)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "empty-folder",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 mkdir expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeEnvelope[mkdirData](t, rec.Body.Bytes())
	if created.Created["path"] != "/empty-folder" || created.Created["is_dir"] != true {
		t.Fatalf("unexpected s3 mkdir payload = %+v", created)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files?source_id=%d&path=/&page=1&page_size=200&sort_by=name&sort_order=asc", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 files list after mkdir expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[fileListData](t, rec.Body.Bytes())
	found := false
	for _, item := range listed.Items {
		if item["path"] == "/empty-folder" && item["is_dir"] == true {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected new empty folder in s3 root list, got %+v", listed.Items)
	}
}

func TestS3FileACLReadWriteFlow(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, adminToken)

	enableMultiUserForTest(t, engine, adminToken)
	userID, userToken := createNormalUserAndLoginForTest(t, engine, adminToken, "s3-user", "strong-password-456")

	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/movies", map[string]any{
		"read":   true,
		"write":  false,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/uploads", map[string]any{
		"read":   true,
		"write":  true,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/covers", map[string]any{
		"read":   true,
		"write":  true,
		"delete": true,
		"share":  false,
	}, "deny", 200, true)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "uploads",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir uploads expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files?source_id=%d&path=/&page=1&page_size=200&sort_by=name&sort_order=asc", sourceID), nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 acl root list expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[fileListData](t, rec.Body.Bytes())
	names := make([]string, 0, len(listed.Items))
	for _, item := range listed.Items {
		names = append(names, item["name"].(string))
	}
	if containsString(names, "covers") {
		t.Fatalf("expected s3 ACL filtered list to hide covers, got %v", names)
	}
	if !containsString(names, "movies") || !containsString(names, "uploads") {
		t.Fatalf("expected movies/uploads in s3 list, got %v", names)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/search?source_id=%d&keyword=poster&path_prefix=/&page=1&page_size=50", sourceID), nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 acl search expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	searched := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if items := searched["items"].([]any); len(items) != 0 {
		t.Fatalf("expected s3 ACL filtered search to hide poster, got %+v", items)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/access-url", map[string]any{
		"source_id":   sourceID,
		"path":        "/covers/poster.jpg",
		"purpose":     "preview",
		"disposition": "inline",
		"expires_in":  300,
	}, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("s3 denied access-url expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "ACL_DENIED")

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/download?source_id=%d&path=%s&disposition=inline", sourceID, url.QueryEscape("/covers/poster.jpg")), nil, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("s3 denied download expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "ACL_DENIED")

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             "/covers",
		"filename":         "blocked.zip",
		"file_size":        5 * 1024 * 1024,
		"file_hash":        "hash-blocked-s3",
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("s3 denied upload init expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "ACL_DENIED")

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             "/uploads",
		"filename":         "allowed.zip",
		"file_size":        5 * 1024 * 1024,
		"file_hash":        "hash-allowed-s3",
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 allowed upload init expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalFileACLReadFlow(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	enableMultiUserForTest(t, engine, adminToken)
	userID, userToken := createNormalUserAndLoginForTest(t, engine, adminToken, "alice", "strong-password-456")

	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/docs", map[string]any{
		"read":   true,
		"write":  false,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/workspace", map[string]any{
		"read":   true,
		"write":  true,
		"delete": true,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/private", map[string]any{
		"read":   true,
		"write":  false,
		"delete": false,
		"share":  false,
	}, "deny", 200, true)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "docs",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir docs expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "private",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir private expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "workspace",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir workspace expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/docs", "hello.txt", []byte("hello"))
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/private", "secret.txt", []byte("secret"))
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/workspace", "draft.txt", []byte("draft"))

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files?source_id=%d&path=/&page=1&page_size=200&sort_by=name&sort_order=asc", sourceID), nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("normal user root list expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[fileListData](t, rec.Body.Bytes())
	names := make([]string, 0, len(listed.Items))
	for _, item := range listed.Items {
		names = append(names, item["name"].(string))
	}
	if containsString(names, "private") {
		t.Fatalf("expected ACL filtered list to hide private, got %v", names)
	}
	if !containsString(names, "docs") || !containsString(names, "workspace") {
		t.Fatalf("expected docs/workspace to stay visible, got %v", names)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/search?source_id=%d&keyword=secret&path_prefix=/&page=1&page_size=50", sourceID), nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("normal user secret search expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	searched := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if items := searched["items"].([]any); len(items) != 0 {
		t.Fatalf("expected ACL filtered search to hide secret, got %+v", items)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/access-url", map[string]any{
		"source_id":   sourceID,
		"path":        "/private/secret.txt",
		"purpose":     "preview",
		"disposition": "inline",
		"expires_in":  300,
	}, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("normal user private access-url expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "ACL_DENIED")

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/download?source_id=%d&path=%s&disposition=inline", sourceID, url.QueryEscape("/private/secret.txt")), nil, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("normal user private download expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "ACL_DENIED")
}

func TestLocalFileACLWriteAndUploadFlow(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	enableMultiUserForTest(t, engine, adminToken)
	userID, userToken := createNormalUserAndLoginForTest(t, engine, adminToken, "bob", "strong-password-789")

	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/docs", map[string]any{
		"read":   true,
		"write":  false,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/workspace", map[string]any{
		"read":   true,
		"write":  true,
		"delete": true,
		"share":  false,
	}, "allow", 100, true)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "docs",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir docs expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "workspace",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir workspace expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/docs", "hello.txt", []byte("hello"))
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/workspace", "draft.txt", []byte("draft"))

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/docs",
		"name":        "blocked",
	}, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("normal user mkdir in docs expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "ACL_DENIED")

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/workspace",
		"name":        "allowed",
	}, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("normal user mkdir in workspace expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/rename", map[string]any{
		"source_id": sourceID,
		"path":      "/workspace/draft.txt",
		"new_name":  "draft-v2.txt",
	}, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("normal user rename workspace file expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/copy", map[string]any{
		"source_id":   sourceID,
		"path":        "/docs/hello.txt",
		"target_path": "/workspace",
	}, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("normal user copy docs->workspace expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodDelete, "/api/v1/files", map[string]any{
		"source_id": sourceID,
		"path":      "/workspace/hello.txt",
	}, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("normal user delete workspace file expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             "/docs",
		"filename":         "blocked.txt",
		"file_size":        5,
		"file_hash":        "hash-blocked",
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("normal user upload init in docs expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "ACL_DENIED")

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             "/workspace",
		"filename":         "allowed.txt",
		"file_size":        5,
		"file_hash":        "hash-allowed",
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("normal user upload init in workspace expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalTrashACLManagementFlow(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	enableMultiUserForTest(t, engine, adminToken)
	userID, userToken := createNormalUserAndLoginForTest(t, engine, adminToken, "trash-user", "strong-password-456")

	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/restore", map[string]any{
		"read":   false,
		"write":  true,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/purge", map[string]any{
		"read":   false,
		"write":  false,
		"delete": true,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/workspace", map[string]any{
		"read":   false,
		"write":  false,
		"delete": true,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/private", map[string]any{
		"read":   false,
		"write":  true,
		"delete": true,
		"share":  false,
	}, "deny", 200, true)

	for _, dir := range []string{"restore", "purge", "workspace", "private"} {
		rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
			"source_id":   sourceID,
			"parent_path": "/",
			"name":        dir,
		}, adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin mkdir %s expected 200, got %d body=%s", dir, rec.Code, rec.Body.String())
		}
	}

	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/restore", "keep.txt", []byte("restore"))
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/purge", "drop.txt", []byte("purge"))
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/workspace", "clear.txt", []byte("clear"))
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/private", "secret.txt", []byte("private"))

	for _, targetPath := range []string{"/restore/keep.txt", "/purge/drop.txt", "/workspace/clear.txt", "/private/secret.txt"} {
		rec := performRequest(t, engine, http.MethodDelete, "/api/v1/files", map[string]any{
			"source_id": sourceID,
			"path":      targetPath,
		}, adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin trash delete %s expected 200, got %d body=%s", targetPath, rec.Code, rec.Body.String())
		}
	}

	rec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/trash?source_id=%d&page=1&page_size=50", sourceID), nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("user trash list expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	userTrash := decodeEnvelope[trashListData](t, rec.Body.Bytes())
	if len(userTrash.Items) != 3 {
		t.Fatalf("expected 3 visible trash items, got %+v", userTrash.Items)
	}
	visible := map[string]int{}
	for _, item := range userTrash.Items {
		visible[item["original_path"].(string)] = int(item["id"].(float64))
	}
	if _, ok := visible["/private/secret.txt"]; ok {
		t.Fatalf("expected private trash item hidden, got %+v", userTrash.Items)
	}

	rec = performRequest(t, engine, http.MethodPost, fmt.Sprintf("/api/v1/trash/%d/restore", visible["/restore/keep.txt"]), nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore-visible trash expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/trash/%d", visible["/purge/drop.txt"]), nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete-visible trash expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/trash?source_id=%d", sourceID), nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear-visible trash expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	cleared := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if cleared["deleted_count"].(float64) != 1 {
		t.Fatalf("expected clear to delete 1 authorized item, got %+v", cleared)
	}

	adminTrashRec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/trash?source_id=%d&page=1&page_size=50", sourceID), nil, adminToken)
	if adminTrashRec.Code != http.StatusOK {
		t.Fatalf("admin trash list expected 200, got %d body=%s", adminTrashRec.Code, adminTrashRec.Body.String())
	}
	adminTrash := decodeEnvelope[trashListData](t, adminTrashRec.Body.Bytes())
	if len(adminTrash.Items) != 1 {
		t.Fatalf("expected only private trash item to remain, got %+v", adminTrash.Items)
	}
	privateTrashID := int(adminTrash.Items[0]["id"].(float64))

	rec = performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/trash/%d", privateTrashID), nil, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("delete-private trash expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "ACL_DENIED")
}

func TestLocalTrashLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, sourceID := bootstrapAdmin(t, engine)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "docs",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("mkdir docs expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	content := []byte("trash-local-file")
	uploadLocalObjectForTest(t, engine, accessToken, sourceID, "/docs", "hello.txt", content)

	rec = performRequest(t, engine, http.MethodDelete, "/api/v1/files", map[string]any{
		"source_id": sourceID,
		"path":      "/docs/hello.txt",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("local trash delete expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/trash?source_id=%d&page=1&page_size=50", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("local trash list expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[trashListData](t, rec.Body.Bytes())
	if len(listed.Items) != 1 {
		t.Fatalf("expected 1 local trash item, got %+v", listed.Items)
	}
	trashItem := listed.Items[0]
	if trashItem["original_path"] != "/docs/hello.txt" {
		t.Fatalf("unexpected local trash original_path = %+v", trashItem)
	}
	if trashItem["original_virtual_path"] != "/local/docs/hello.txt" {
		t.Fatalf("unexpected local trash original_virtual_path = %+v", trashItem)
	}
	if got := trashItem["trash_path"].(string); len(got) < len("/.trash/") || got[:len("/.trash/")] != "/.trash/" {
		t.Fatalf("unexpected local trash path = %+v", trashItem)
	}
	if trashItem["name"] != "hello.txt" {
		t.Fatalf("unexpected local trash item = %+v", trashItem)
	}

	assertSearchResultCount(t, engine, accessToken, sourceID, "/docs", "hello", 0)

	trashID := int(trashItem["id"].(float64))
	rec = performRequest(t, engine, http.MethodPost, fmt.Sprintf("/api/v1/trash/%d/restore", trashID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("local trash restore expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	restored := decodeEnvelope[trashRestoreData](t, rec.Body.Bytes())
	if !restored.Restored || restored.RestoredPath != "/docs/hello.txt" || restored.RestoredVirtualPath != "/local/docs/hello.txt" {
		t.Fatalf("unexpected local restore payload = %+v", restored)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/trash?source_id=%d&page=1&page_size=50", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("local trash list after restore expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed = decodeEnvelope[trashListData](t, rec.Body.Bytes())
	if len(listed.Items) != 0 {
		t.Fatalf("expected empty local trash after restore, got %+v", listed.Items)
	}

	assertSearchResultCount(t, engine, accessToken, sourceID, "/docs", "hello", 1)

	rec = performRequest(t, engine, http.MethodDelete, "/api/v1/files", map[string]any{
		"source_id": sourceID,
		"path":      "/docs/hello.txt",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("local trash delete second time expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/trash?source_id=%d&page=1&page_size=50", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("local trash list second time expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed = decodeEnvelope[trashListData](t, rec.Body.Bytes())
	if len(listed.Items) != 1 {
		t.Fatalf("expected 1 local trash item after second delete, got %+v", listed.Items)
	}

	trashID = int(listed.Items[0]["id"].(float64))
	rec = performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/trash/%d", trashID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("local trash delete one expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/trash?source_id=%d&page=1&page_size=50", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("local trash list after delete one expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed = decodeEnvelope[trashListData](t, rec.Body.Bytes())
	if len(listed.Items) != 0 {
		t.Fatalf("expected empty local trash after delete one, got %+v", listed.Items)
	}

	assertSearchResultCount(t, engine, accessToken, sourceID, "/docs", "hello", 0)
}

func TestS3TrashClearLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, accessToken)

	for _, targetPath := range []string{"/movies/demo.mp4", "/movies/trailer.mp4"} {
		rec := performRequest(t, engine, http.MethodDelete, "/api/v1/files", map[string]any{
			"source_id": sourceID,
			"path":      targetPath,
		}, accessToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("s3 trash delete %s expected 200, got %d body=%s", targetPath, rec.Code, rec.Body.String())
		}
	}

	rec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/trash?source_id=%d&page=1&page_size=50", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 trash list expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[trashListData](t, rec.Body.Bytes())
	if len(listed.Items) != 2 {
		t.Fatalf("expected 2 s3 trash items, got %+v", listed.Items)
	}

	rec = performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/trash?source_id=%d", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 trash clear expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/trash?source_id=%d&page=1&page_size=50", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 trash list after clear expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed = decodeEnvelope[trashListData](t, rec.Body.Bytes())
	if len(listed.Items) != 0 {
		t.Fatalf("expected empty s3 trash after clear, got %+v", listed.Items)
	}

	assertSearchResultCount(t, engine, accessToken, sourceID, "/movies", "demo", 0)
	assertSearchResultCount(t, engine, accessToken, sourceID, "/movies", "trailer", 0)
}

func TestSourceCRUDAndNavigationLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	rec := performRequest(t, engine, http.MethodGet, "/api/v1/sources?view=navigation", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("navigation sources expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	nav := decodeEnvelope[sourceListData](t, rec.Body.Bytes())
	if len(nav.Items) != 1 || nav.Items[0]["driver_type"] != "local" {
		t.Fatalf("unexpected navigation sources = %+v", nav)
	}
	if nav.Items[0]["mount_path"] != "/local" {
		t.Fatalf("expected default local mount_path /local, got %+v", nav.Items[0])
	}

	basePath := filepath.ToSlash(filepath.Join(t.TempDir(), "media-source"))
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(media source) error = %v", err)
	}
	rec = performRequest(t, engine, http.MethodPost, "/api/v1/sources/test", map[string]any{
		"name":              "媒体仓库",
		"driver_type":       "local",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"mount_path":        "/media",
		"root_path":         "/",
		"sort_order":        10,
		"config":            map[string]any{"base_path": basePath},
		"secret_patch":      map[string]any{},
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("source test expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	tested := decodeEnvelope[sourceTestData](t, rec.Body.Bytes())
	if !tested.Reachable || tested.Status != "online" {
		t.Fatalf("unexpected source test result = %+v", tested)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/sources", map[string]any{
		"name":              "媒体仓库",
		"driver_type":       "local",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"mount_path":        "/media",
		"root_path":         "/",
		"sort_order":        10,
		"config":            map[string]any{"base_path": basePath},
		"secret_patch":      map[string]any{},
	}, accessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("source create expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeEnvelope[sourceCreateData](t, rec.Body.Bytes())
	sourceID := int(created.Source["id"].(float64))
	if created.Source["mount_path"] != "/media" || created.Source["root_path"] != "/" {
		t.Fatalf("expected created source mount/root path, got %+v", created.Source)
	}

	photosBasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "photos-source"))
	if err := os.MkdirAll(photosBasePath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(photos source) error = %v", err)
	}
	rec = performRequest(t, engine, http.MethodPost, "/api/v1/sources", map[string]any{
		"name":              "照片仓库",
		"driver_type":       "local",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"mount_path":        "/photos",
		"root_path":         "/",
		"sort_order":        11,
		"config":            map[string]any{"base_path": photosBasePath},
		"secret_patch":      map[string]any{},
	}, accessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("second local source create expected 201 without raw unique slug error, got %d body=%s", rec.Code, rec.Body.String())
	}
	secondCreated := decodeEnvelope[sourceCreateData](t, rec.Body.Bytes())
	if secondCreated.Source["webdav_slug"] == created.Source["webdav_slug"] {
		t.Fatalf("expected unique webdav_slug for second local source, got first=%+v second=%+v", created.Source, secondCreated.Source)
	}

	conflictBasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "conflict-source"))
	if err := os.MkdirAll(conflictBasePath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(conflict source) error = %v", err)
	}
	rec = performRequest(t, engine, http.MethodPost, "/api/v1/sources", map[string]any{
		"name":              "重复挂载仓库",
		"driver_type":       "local",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"mount_path":        "/media",
		"root_path":         "/",
		"sort_order":        30,
		"config":            map[string]any{"base_path": conflictBasePath},
		"secret_patch":      map[string]any{},
	}, accessToken)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate mount path expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v1/sources?view=admin", nil, accessToken)
	adminList := decodeEnvelope[sourceListData](t, rec.Body.Bytes())
	if len(adminList.Items) < 2 {
		t.Fatalf("expected at least 2 sources, got %+v", adminList)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/sources/%d", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("source detail expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	detail := decodeEnvelope[sourceDetailData](t, rec.Body.Bytes())
	if detail.Config["base_path"] != basePath {
		t.Fatalf("unexpected source detail config = %+v", detail)
	}
	if detail.Source["mount_path"] != "/media" || detail.Source["root_path"] != "/" {
		t.Fatalf("expected detail source mount/root path, got %+v", detail.Source)
	}

	rec = performRequest(t, engine, http.MethodPut, fmt.Sprintf("/api/v1/sources/%d", sourceID), map[string]any{
		"name":              "媒体仓库-新",
		"is_enabled":        true,
		"is_webdav_exposed": true,
		"webdav_read_only":  false,
		"mount_path":        "/library",
		"root_path":         "/movies",
		"sort_order":        20,
		"config":            map[string]any{"base_path": basePath},
		"secret_patch":      map[string]any{},
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("source update expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/sources/%d", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("source detail after update expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	updatedDetail := decodeEnvelope[sourceDetailData](t, rec.Body.Bytes())
	if updatedDetail.Source["webdav_read_only"] != false || updatedDetail.Source["is_webdav_exposed"] != true {
		t.Fatalf("expected persisted webdav flags, got %+v", updatedDetail.Source)
	}
	if updatedDetail.Source["mount_path"] != "/library" || updatedDetail.Source["root_path"] != "/movies" {
		t.Fatalf("expected updated source mount/root path, got %+v", updatedDetail.Source)
	}

	rec = performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/sources/%d", sourceID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("source delete expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalSourceCreateInvalidPathReturnsClientError(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/sources", map[string]any{
		"name":              "前端本地盘",
		"driver_type":       "local",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"mount_path":        "/e2e-host-disk",
		"root_path":         "/mnt/e2e-host-disk",
		"sort_order":        10,
		"config":            map[string]any{},
		"secret_patch":      map[string]any{},
	}, accessToken)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid local source expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "PATH_INVALID")
}

func TestLocalSourceCreateRejectsMissingBasePath(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)
	missingBasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "not-exist-yunxia"))

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/sources", map[string]any{
		"name":              "不存在目录源",
		"driver_type":       "local",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"mount_path":        "/not-exist-yunxia",
		"root_path":         "/",
		"sort_order":        10,
		"config":            map[string]any{"base_path": missingBasePath},
		"secret_patch":      map[string]any{},
	}, accessToken)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing base_path expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "PATH_INVALID")
	if _, err := os.Stat(missingBasePath); !os.IsNotExist(err) {
		t.Fatalf("missing base_path should not be auto-created, stat error=%v", err)
	}
}

func TestLocalFileUploadAndDownloadLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, sourceID := bootstrapAdmin(t, engine)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "docs",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("mkdir expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	_ = decodeEnvelope[mkdirData](t, rec.Body.Bytes())

	content := []byte("hello yunxia")
	sum := md5.Sum(content)
	fileHash := hex.EncodeToString(sum[:])

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             "/docs",
		"filename":         "hello.txt",
		"file_size":        len(content),
		"file_hash":        fileHash,
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload init expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	init := decodeEnvelope[uploadInitData](t, rec.Body.Bytes())
	if init.Transport.Mode != "server_chunk" || init.Upload.UploadID == "" {
		t.Fatalf("unexpected upload init = %+v", init)
	}

	req := newBinaryRequest(t, http.MethodPut, fmt.Sprintf("/api/v1/upload/chunk?upload_id=%s&index=0", init.Upload.UploadID), content, accessToken)
	chunkRec := httptestDo(engine, req)
	if chunkRec.Code != http.StatusOK {
		t.Fatalf("upload chunk expected 200, got %d body=%s", chunkRec.Code, chunkRec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/upload/sessions?source_id=%d", sourceID), nil, accessToken)
	sessions := decodeEnvelope[uploadSessionListData](t, rec.Body.Bytes())
	if len(sessions.Items) == 0 {
		t.Fatalf("expected active upload session, got %+v", sessions)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/upload/finish", map[string]any{
		"upload_id": init.Upload.UploadID,
	}, accessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload finish expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	finished := decodeEnvelope[uploadFinishData](t, rec.Body.Bytes())
	if !finished.Completed || finished.File["path"] != "/docs/hello.txt" {
		t.Fatalf("unexpected upload finish payload = %+v", finished)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files?source_id=%d&path=/docs&page=1&page_size=200&sort_by=name&sort_order=asc", sourceID), nil, accessToken)
	listed := decodeEnvelope[fileListData](t, rec.Body.Bytes())
	if len(listed.Items) == 0 {
		t.Fatalf("expected file list items, got %+v", listed)
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/search?source_id=%d&keyword=hello", sourceID), nil, accessToken)
	searched := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if len(searched["items"].([]any)) == 0 {
		t.Fatalf("expected search results, got %+v", searched)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/rename", map[string]any{
		"source_id": sourceID,
		"path":      "/docs/hello.txt",
		"new_name":  "greeting.txt",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	_ = performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "archive",
	}, accessToken)

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/move", map[string]any{
		"source_id":   sourceID,
		"path":        "/docs/greeting.txt",
		"target_path": "/archive",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("move expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/copy", map[string]any{
		"source_id":   sourceID,
		"path":        "/archive/greeting.txt",
		"target_path": "/docs",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("copy expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/access-url", map[string]any{
		"source_id":   sourceID,
		"path":        "/archive/greeting.txt",
		"purpose":     "preview",
		"disposition": "inline",
		"expires_in":  300,
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("access-url expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	accessURL := decodeEnvelope[accessURLData](t, rec.Body.Bytes())

	parsed, err := url.Parse(accessURL.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	downloadReq := newBinaryRequest(t, http.MethodGet, parsed.RequestURI(), nil, "")
	downloadRec := httptestDo(engine, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("download expected 200, got %d body=%s", downloadRec.Code, downloadRec.Body.String())
	}
	if downloadRec.Body.String() != string(content) {
		t.Fatalf("unexpected download body = %q", downloadRec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodDelete, "/api/v1/files", map[string]any{
		"source_id":   sourceID,
		"path":        "/docs/greeting.txt",
		"delete_mode": "permanent",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUploadSessionListAndCancel(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, sourceID := bootstrapAdmin(t, engine)

	content := []byte("cancel me")
	sum := md5.Sum(content)
	fileHash := hex.EncodeToString(sum[:])

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             "/",
		"filename":         "cancel.txt",
		"file_size":        len(content),
		"file_hash":        fileHash,
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload init expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	init := decodeEnvelope[uploadInitData](t, rec.Body.Bytes())

	rec = performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/upload/sessions/%s", init.Upload.UploadID), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel upload expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/upload/sessions?source_id=%d", sourceID), nil, accessToken)
	sessions := decodeEnvelope[uploadSessionListData](t, rec.Body.Bytes())
	if len(sessions.Items) != 0 {
		t.Fatalf("expected no sessions after cancel, got %+v", sessions)
	}
}

func TestUploadFinishCancelPermissionBoundary(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	enableMultiUserForTest(t, engine, adminToken)
	ownerID, ownerToken := createNormalUserAndLoginForTest(t, engine, adminToken, "owner-user", "strong-password-456")
	peerID, peerToken := createNormalUserAndLoginForTest(t, engine, adminToken, "peer-user", "strong-password-789")

	createACLRuleForTest(t, engine, adminToken, sourceID, ownerID, "/workspace", map[string]any{
		"read":   true,
		"write":  true,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, peerID, "/workspace", map[string]any{
		"read":   true,
		"write":  true,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "workspace",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir workspace expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	cancelInitRec := performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             "/workspace",
		"filename":         "cancel.txt",
		"file_size":        6,
		"file_hash":        "hash-cancel-owner",
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, ownerToken)
	if cancelInitRec.Code != http.StatusOK {
		t.Fatalf("owner upload init(cancel) expected 200, got %d body=%s", cancelInitRec.Code, cancelInitRec.Body.String())
	}
	cancelInit := decodeEnvelope[uploadInitData](t, cancelInitRec.Body.Bytes())

	rec = performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/upload/sessions?source_id=%d", sourceID), nil, peerToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("peer upload sessions expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	peerSessions := decodeEnvelope[uploadSessionListData](t, rec.Body.Bytes())
	if len(peerSessions.Items) != 0 {
		t.Fatalf("expected peer not to see owner sessions, got %+v", peerSessions.Items)
	}

	rec = performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/upload/sessions/%s", cancelInit.Upload.UploadID), nil, peerToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("peer cancel owner upload expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "PERMISSION_DENIED")

	content := []byte("finish")
	finishInitRec := performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             "/workspace",
		"filename":         "finish.txt",
		"file_size":        len(content),
		"file_hash":        "",
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, ownerToken)
	if finishInitRec.Code != http.StatusOK {
		t.Fatalf("owner upload init(finish) expected 200, got %d body=%s", finishInitRec.Code, finishInitRec.Body.String())
	}
	finishInit := decodeEnvelope[uploadInitData](t, finishInitRec.Body.Bytes())

	chunkReq := newBinaryRequest(t, http.MethodPut, fmt.Sprintf("/api/v1/upload/chunk?upload_id=%s&index=0", finishInit.Upload.UploadID), content, ownerToken)
	chunkRec := httptestDo(engine, chunkReq)
	if chunkRec.Code != http.StatusOK {
		t.Fatalf("owner upload chunk expected 200, got %d body=%s", chunkRec.Code, chunkRec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/upload/finish", map[string]any{
		"upload_id": finishInit.Upload.UploadID,
	}, peerToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("peer finish owner upload expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "PERMISSION_DENIED")
}

func TestUploadChunkOwnerBoundary(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	enableMultiUserForTest(t, engine, adminToken)
	ownerID, ownerToken := createNormalUserAndLoginForTest(t, engine, adminToken, "chunk-owner", "strong-password-456")
	peerID, peerToken := createNormalUserAndLoginForTest(t, engine, adminToken, "chunk-peer", "strong-password-789")

	createACLRuleForTest(t, engine, adminToken, sourceID, ownerID, "/workspace", map[string]any{
		"read":   true,
		"write":  true,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, peerID, "/workspace", map[string]any{
		"read":   true,
		"write":  true,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "workspace",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir workspace expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	initRec := performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             "/workspace",
		"filename":         "chunk.txt",
		"file_size":        5,
		"file_hash":        "hash-chunk",
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, ownerToken)
	if initRec.Code != http.StatusOK {
		t.Fatalf("owner upload init expected 200, got %d body=%s", initRec.Code, initRec.Body.String())
	}
	initPayload := decodeEnvelope[uploadInitData](t, initRec.Body.Bytes())

	peerReq := newBinaryRequest(t, http.MethodPut, fmt.Sprintf("/api/v1/upload/chunk?upload_id=%s&index=0", initPayload.Upload.UploadID), []byte("hello"), peerToken)
	peerRec := httptestDo(engine, peerReq)
	if peerRec.Code != http.StatusForbidden {
		t.Fatalf("peer upload chunk expected 403, got %d body=%s", peerRec.Code, peerRec.Body.String())
	}
	assertFailureCode(t, peerRec.Body.Bytes(), "PERMISSION_DENIED")

	ownerReq := newBinaryRequest(t, http.MethodPut, fmt.Sprintf("/api/v1/upload/chunk?upload_id=%s&index=0", initPayload.Upload.UploadID), []byte("hello"), ownerToken)
	ownerRec := httptestDo(engine, ownerReq)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner upload chunk expected 200, got %d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
}

func newStorageTestRouter(t *testing.T) *gin.Engine {
	return newTestRouter(t)
}

func bootstrapAdmin(t *testing.T, engine *gin.Engine) (string, int) {
	t.Helper()

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/setup/init", map[string]any{
		"username": "admin",
		"password": "strong-password-123",
		"email":    "admin@example.com",
	}, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup init expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	initPayload := decodeEnvelope[setupInitData](t, rec.Body.Bytes())

	sourcesRec := performRequest(t, engine, http.MethodGet, "/api/v1/sources?view=navigation", nil, initPayload.Tokens.AccessToken)
	if sourcesRec.Code != http.StatusOK {
		t.Fatalf("navigation sources expected 200, got %d body=%s", sourcesRec.Code, sourcesRec.Body.String())
	}
	nav := decodeEnvelope[sourceListData](t, sourcesRec.Body.Bytes())
	sourceID := int(nav.Items[0]["id"].(float64))
	return initPayload.Tokens.AccessToken, sourceID
}

func enableMultiUserForTest(t *testing.T, engine *gin.Engine, accessToken string) {
	t.Helper()

	rec := performRequest(t, engine, http.MethodPut, "/api/v1/system/config", map[string]any{
		"site_name":          "云匣",
		"multi_user_enabled": true,
		"default_source_id":  nil,
		"max_upload_size":    int64(21474836480),
		"default_chunk_size": int64(5242880),
		"webdav_enabled":     true,
		"webdav_prefix":      "/dav",
		"theme":              "system",
		"language":           "zh-CN",
		"time_zone":          "Asia/Shanghai",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable multi user expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func createNormalUserAndLoginForTest(t *testing.T, engine *gin.Engine, adminToken, username, password string) (int, string) {
	return createUserAndLoginForTest(t, engine, adminToken, username, password, "user")
}

func createUserWithRoleAndLoginForTest(t *testing.T, engine *gin.Engine, adminToken, username, password, roleKey string) string {
	t.Helper()

	_, accessToken := createUserAndLoginForTest(t, engine, adminToken, username, password, roleKey)
	return accessToken
}

func createUserAndLoginForTest(t *testing.T, engine *gin.Engine, adminToken, username, password, roleKey string) (int, string) {
	t.Helper()

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/users", map[string]any{
		"username": username,
		"password": password,
		"email":    username + "@example.com",
		"role_key": roleKey,
	}, adminToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	userID := int(created["user"].(map[string]any)["id"].(float64))

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": username,
		"password": password,
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("normal user login expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	loginPayload := decodeEnvelope[tokenData](t, rec.Body.Bytes())
	return userID, loginPayload.Tokens.AccessToken
}

func createACLRuleForTest(t *testing.T, engine *gin.Engine, adminToken string, sourceID, userID int, rulePath string, permissions map[string]any, effect string, priority int, inheritToChildren bool) {
	t.Helper()

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/acl/rules", map[string]any{
		"source_id":           sourceID,
		"path":                rulePath,
		"subject_type":        "user",
		"subject_id":          userID,
		"effect":              effect,
		"priority":            priority,
		"permissions":         permissions,
		"inherit_to_children": inheritToChildren,
	}, adminToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create acl rule expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func createS3SourceForTest(t *testing.T, engine *gin.Engine, accessToken string) int {
	t.Helper()

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/sources", map[string]any{
		"name":              "S3 上传源",
		"driver_type":       "s3",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"root_path":         "/",
		"sort_order":        40,
		"config": map[string]any{
			"endpoint":         "https://s3.example.com",
			"region":           "us-east-1",
			"bucket":           "media",
			"base_prefix":      "library",
			"force_path_style": true,
		},
		"secret_patch": map[string]any{
			"access_key": "AKIA-UPLOAD-1234",
			"secret_key": "secret-upload",
		},
	}, accessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create s3 source expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	created := decodeEnvelope[sourceCreateData](t, rec.Body.Bytes())
	return int(created.Source["id"].(float64))
}

func uploadLocalObjectForTest(t *testing.T, engine *gin.Engine, accessToken string, sourceID int, filePath string, filename string, content []byte) {
	t.Helper()

	sum := md5.Sum(content)
	fileHash := hex.EncodeToString(sum[:])
	rec := performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             filePath,
		"filename":         filename,
		"file_size":        len(content),
		"file_hash":        fileHash,
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("local upload init expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	initPayload := decodeEnvelope[uploadInitData](t, rec.Body.Bytes())

	req := newBinaryRequest(t, http.MethodPut, fmt.Sprintf("/api/v1/upload/chunk?upload_id=%s&index=0", initPayload.Upload.UploadID), content, accessToken)
	chunkRec := httptestDo(engine, req)
	if chunkRec.Code != http.StatusOK {
		t.Fatalf("local upload chunk expected 200, got %d body=%s", chunkRec.Code, chunkRec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/upload/finish", map[string]any{
		"upload_id": initPayload.Upload.UploadID,
	}, accessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("local upload finish expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func uploadS3ObjectForTest(t *testing.T, engine *gin.Engine, accessToken string, sourceID int, filePath string, filename string) {
	t.Helper()

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"source_id":        sourceID,
		"path":             filePath,
		"filename":         filename,
		"file_size":        6 * 1024 * 1024,
		"file_hash":        fmt.Sprintf("hash-%s-%s", filePath, filename),
		"last_modified_at": time.Now().Format(time.RFC3339),
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("seed s3 upload init expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	initPayload := decodeEnvelope[uploadInitData](t, rec.Body.Bytes())
	if initPayload.Upload.UploadID == "" || len(initPayload.PartInstructions) == 0 {
		t.Fatalf("expected multipart instructions for seeded s3 upload, got %+v", initPayload)
	}

	parts := make([]map[string]any, 0, len(initPayload.PartInstructions))
	for _, instruction := range initPayload.PartInstructions {
		parts = append(parts, map[string]any{
			"index": instruction.Index,
			"etag":  fmt.Sprintf("\"etag-%s-%s-part-%d\"", strings.Trim(filePath, "/"), filename, instruction.Index),
		})
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/upload/finish", map[string]any{
		"upload_id": initPayload.Upload.UploadID,
		"parts":     parts,
	}, accessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed s3 upload finish expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func assertSearchResultCount(t *testing.T, engine *gin.Engine, accessToken string, sourceID int, pathPrefix string, keyword string, expectedCount int) {
	t.Helper()

	rec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/search?source_id=%d&keyword=%s&path_prefix=%s&page=1&page_size=50", sourceID, url.QueryEscape(keyword), url.QueryEscape(pathPrefix)), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("search expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	searched := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	items := searched["items"].([]any)
	if len(items) != expectedCount {
		t.Fatalf("expected %d search results, got %+v", expectedCount, items)
	}
}

func assertS3SearchPaths(t *testing.T, engine *gin.Engine, accessToken string, sourceID int, pathPrefix string, keyword string, expectedPaths []string) {
	t.Helper()

	rec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/files/search?source_id=%d&keyword=%s&path_prefix=%s&page=1&page_size=50", sourceID, url.QueryEscape(keyword), url.QueryEscape(pathPrefix)), nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 search expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	searched := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	items := searched["items"].([]any)
	if len(items) != len(expectedPaths) {
		t.Fatalf("expected %d search results, got %+v", len(expectedPaths), items)
	}

	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.(map[string]any)["path"].(string))
	}
	for index := range expectedPaths {
		if got[index] != expectedPaths[index] {
			t.Fatalf("expected search paths %v, got %v", expectedPaths, got)
		}
	}
}

func assertFailureCode(t *testing.T, body []byte, expectedCode string) {
	t.Helper()

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("json.Unmarshal(error envelope) error = %v body=%s", err, string(body))
	}
	if env.Success {
		t.Fatalf("expected failure envelope, got body=%s", string(body))
	}
	if env.Code != expectedCode {
		t.Fatalf("expected error code %s, got %s body=%s", expectedCode, env.Code, string(body))
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func newBinaryRequest(t *testing.T, method, path string, body []byte, accessToken string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	return req
}

func httptestDo(engine *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}
