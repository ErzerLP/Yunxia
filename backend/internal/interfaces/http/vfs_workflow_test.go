package http

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

type vfsListData struct {
	Items       []map[string]any `json:"items"`
	CurrentPath string           `json:"current_path"`
}

type vfsTagCreateData struct {
	Tag map[string]any `json:"tag"`
}

type vfsNodeTagsData struct {
	Path string           `json:"path"`
	Tags []map[string]any `json:"tags"`
}

func TestVFSListNestedMounts(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	docsSourceID := createLocalSourceWithMountForTest(t, engine, accessToken, "docs-root", "/docs")
	uploadLocalObjectForTest(t, engine, accessToken, docsSourceID, "/", "readme.md", []byte("hello docs"))

	teamSourceID := createLocalSourceWithMountForTest(t, engine, accessToken, "docs-team", "/docs/team")
	uploadLocalObjectForTest(t, engine, accessToken, teamSourceID, "/", "spec.md", []byte("team spec"))

	rec := performRequest(t, engine, http.MethodGet, "/api/v2/fs/list?path=/docs", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs list expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	listed := decodeEnvelope[vfsListData](t, rec.Body.Bytes())
	if listed.CurrentPath != "/docs" {
		t.Fatalf("expected current path /docs, got %+v", listed)
	}

	names := make([]string, 0, len(listed.Items))
	var teamItem map[string]any
	for _, item := range listed.Items {
		names = append(names, item["name"].(string))
		if item["name"] == "team" {
			teamItem = item
		}
	}

	if !containsString(names, "readme.md") || !containsString(names, "team") {
		t.Fatalf("expected merged docs items, got %v", names)
	}
	if teamItem == nil || teamItem["is_virtual"] != true || teamItem["is_mount_point"] != true {
		t.Fatalf("expected team projected as mount point, got %+v", teamItem)
	}
}

func TestVFSDownloadLocalByVirtualPath(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	sourceID := createLocalSourceWithMountForTest(t, engine, accessToken, "docs-root", "/docs")
	content := []byte("hello via vfs")
	uploadLocalObjectForTest(t, engine, accessToken, sourceID, "/", "hello.txt", content)

	rec := performRequest(t, engine, http.MethodGet, "/api/v2/fs/download?path=%2Fdocs%2Fhello.txt&disposition=inline", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs local download expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(content) {
		t.Fatalf("unexpected vfs local download body = %q", rec.Body.String())
	}
}

func TestVFSDownloadS3ByVirtualPathRedirect(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	_ = createS3SourceWithMountForTest(t, engine, accessToken, "S3 媒体库", "/media")

	rec := performRequest(t, engine, http.MethodGet, "/api/v2/fs/download?path=%2Fmedia%2Fmovies%2Fdemo.mp4&disposition=inline", nil, accessToken)
	if rec.Code != http.StatusFound {
		t.Fatalf("vfs s3 download expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}

	location := rec.Header().Get("Location")
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
}

func TestVFSAccessURLByVirtualPath(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	sourceID := createLocalSourceWithMountForTest(t, engine, accessToken, "docs-root", "/docs")
	content := []byte("hello access url")
	uploadLocalObjectForTest(t, engine, accessToken, sourceID, "/", "hello.txt", content)

	rec := performRequest(t, engine, http.MethodPost, "/api/v2/fs/access-url", map[string]any{
		"path":        "/docs/hello.txt",
		"purpose":     "download",
		"disposition": "inline",
		"expires_in":  300,
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs access-url expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	accessURL := decodeEnvelope[accessURLData](t, rec.Body.Bytes())
	if accessURL.Method != http.MethodGet {
		t.Fatalf("expected GET access-url method, got %+v", accessURL)
	}
	if len(accessURL.URL) < len("/api/v2/fs/download?") || accessURL.URL[:len("/api/v2/fs/download?")] != "/api/v2/fs/download?" {
		t.Fatalf("expected v2 download url, got %s", accessURL.URL)
	}

	downloadRec := performRequest(t, engine, http.MethodGet, accessURL.URL, nil, "")
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("vfs access-url download expected 200, got %d body=%s", downloadRec.Code, downloadRec.Body.String())
	}
	if downloadRec.Body.String() != string(content) {
		t.Fatalf("unexpected vfs access-url body = %q", downloadRec.Body.String())
	}
}

func TestVFSSearchUsesMetadataIndex(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	sourceID := createLocalSourceWithMountForTest(t, engine, accessToken, "docs-search", "/docs-search")
	uploadLocalObjectForTest(t, engine, accessToken, sourceID, "/", "metadata-search-hit.txt", []byte("hello metadata search"))

	rec := performRequest(t, engine, http.MethodGet, "/api/v2/fs/search?path=/docs-search&keyword=metadata-search", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs search expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[vfsListData](t, rec.Body.Bytes())
	names := collectMapNames(listed.Items)
	if !containsString(names, "metadata-search-hit.txt") {
		t.Fatalf("expected metadata-backed search hit, got %+v", listed.Items)
	}
}

func TestVFSTagLifecycleOnMetadataNode(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	sourceID := createLocalSourceWithMountForTest(t, engine, accessToken, "tag-root", "/tag-root")
	uploadLocalObjectForTest(t, engine, accessToken, sourceID, "/", "tag-me.txt", []byte("tag me"))
	rec := performRequest(t, engine, http.MethodGet, "/api/v2/fs/list?path=/tag-root", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs list before tag expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v1/tags", map[string]any{
		"name":  "番剧",
		"color": "#66ccff",
	}, accessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tag expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeEnvelope[vfsTagCreateData](t, rec.Body.Bytes())
	tagID := uint(created.Tag["id"].(float64))

	rec = performRequest(t, engine, http.MethodPost, "/api/v2/fs/tags/attach", map[string]any{
		"path":   "/tag-root/tag-me.txt",
		"tag_id": tagID,
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("attach tag expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	attached := decodeEnvelope[vfsNodeTagsData](t, rec.Body.Bytes())
	if len(attached.Tags) != 1 || attached.Tags[0]["name"] != "番剧" {
		t.Fatalf("unexpected attached tags = %+v", attached)
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v2/fs/tags?path=/tag-root/tag-me.txt", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("list node tags expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[vfsNodeTagsData](t, rec.Body.Bytes())
	if len(listed.Tags) != 1 || listed.Tags[0]["id"].(float64) != float64(tagID) {
		t.Fatalf("unexpected listed tags = %+v", listed)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v2/fs/tags/detach", map[string]any{
		"path":   "/tag-root/tag-me.txt",
		"tag_id": tagID,
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("detach tag expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	detached := decodeEnvelope[vfsNodeTagsData](t, rec.Body.Bytes())
	if len(detached.Tags) != 0 {
		t.Fatalf("expected tags detached, got %+v", detached)
	}
}

func TestVFSUploadInitToMappedPath(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	docsSourceID := createLocalSourceWithMountForTest(t, engine, accessToken, "docs-root", "/docs")

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"target_virtual_parent_path": "/docs",
		"filename":                   "brief.txt",
		"file_size":                  5,
		"file_hash":                  "5d41402abc4b2a76b9719d911017c592",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs upload init expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	initPayload := decodeEnvelope[uploadInitData](t, rec.Body.Bytes())
	if initPayload.Upload.SourceID != docsSourceID {
		t.Fatalf("expected resolved source %d, got %+v", docsSourceID, initPayload.Upload)
	}
	if initPayload.Upload.Path != "/" {
		t.Fatalf("expected resolved inner parent path /, got %+v", initPayload.Upload)
	}
	if initPayload.Upload.TargetVirtualParentPath != "/docs" {
		t.Fatalf("expected target virtual parent path /docs, got %+v", initPayload.Upload)
	}
	if initPayload.Upload.TargetVFSParentNodeID <= 0 {
		t.Fatalf("expected target_vfs_parent_node_id to be persisted, got %+v", initPayload.Upload)
	}
	if initPayload.Upload.ResolvedSourceID != docsSourceID || initPayload.Upload.ResolvedInnerParentPath != "/" {
		t.Fatalf("expected resolved snapshot to be persisted, got %+v", initPayload.Upload)
	}
}

func TestVFSUploadInitRejectsPureVirtualParent(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	_ = createLocalSourceWithMountForTest(t, engine, accessToken, "docs-team", "/docs/team")
	_ = createLocalSourceWithMountForTest(t, engine, accessToken, "docs-personal", "/docs/personal")

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/upload/init", map[string]any{
		"target_virtual_parent_path": "/docs",
		"filename":                   "brief.txt",
		"file_size":                  5,
		"file_hash":                  "5d41402abc4b2a76b9719d911017c592",
	}, accessToken)
	if rec.Code != http.StatusConflict {
		t.Fatalf("vfs pure virtual upload init expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "NO_BACKING_STORAGE")
}

func TestVFSMkdirMoveCopyDeleteLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	docsSourceID := createLocalSourceWithMountForTest(t, engine, accessToken, "docs-root", "/docs")
	_ = createLocalSourceWithMountForTest(t, engine, accessToken, "archive-root", "/archive")
	uploadLocalObjectForTest(t, engine, accessToken, docsSourceID, "/", "hello.txt", []byte("hello vfs write"))

	rec := performRequest(t, engine, http.MethodPost, "/api/v2/fs/mkdir", map[string]any{
		"parent_path": "/docs",
		"name":        "notes",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs mkdir expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = performRequest(t, engine, http.MethodGet, "/api/v2/fs/list?path=/docs", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs list docs after mkdir expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	docsListed := decodeEnvelope[vfsListData](t, rec.Body.Bytes())
	if !containsString(collectMapNames(docsListed.Items), "notes") {
		t.Fatalf("expected /docs to contain notes after mkdir, got %+v", docsListed.Items)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v2/fs/rename", map[string]any{
		"path":     "/docs/hello.txt",
		"new_name": "greeting.txt",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs rename expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = performRequest(t, engine, http.MethodGet, "/api/v2/fs/list?path=/docs", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs list docs after rename expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	docsListed = decodeEnvelope[vfsListData](t, rec.Body.Bytes())
	docsNames := collectMapNames(docsListed.Items)
	if !containsString(docsNames, "greeting.txt") || containsString(docsNames, "hello.txt") {
		t.Fatalf("expected /docs to contain greeting.txt only after rename, got %+v", docsListed.Items)
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v2/fs/move", map[string]any{
		"path":        "/docs/greeting.txt",
		"target_path": "/archive",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs move expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodPost, "/api/v2/fs/copy", map[string]any{
		"path":        "/archive/greeting.txt",
		"target_path": "/docs/notes",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs copy expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = performRequest(t, engine, http.MethodGet, "/api/v2/fs/list?path=/docs/notes", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs list notes after copy expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	notesListed := decodeEnvelope[vfsListData](t, rec.Body.Bytes())
	if !containsString(collectMapNames(notesListed.Items), "greeting.txt") {
		t.Fatalf("expected copied greeting.txt visible in /docs/notes, got %+v", notesListed.Items)
	}

	rec = performRequest(t, engine, http.MethodDelete, "/api/v2/fs", map[string]any{
		"path":        "/docs/notes/greeting.txt",
		"delete_mode": "permanent",
	}, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs delete expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v2/fs/list?path=/archive", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs list archive expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	archiveListed := decodeEnvelope[vfsListData](t, rec.Body.Bytes())
	if !containsString(collectMapNames(archiveListed.Items), "greeting.txt") {
		t.Fatalf("expected archive to contain greeting.txt, got %+v", archiveListed.Items)
	}

	rec = performRequest(t, engine, http.MethodGet, "/api/v2/fs/list?path=/docs/notes", nil, accessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs list notes expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	notesListed = decodeEnvelope[vfsListData](t, rec.Body.Bytes())
	if containsString(collectMapNames(notesListed.Items), "greeting.txt") {
		t.Fatalf("expected deleted greeting.txt absent from /docs/notes, got %+v", notesListed.Items)
	}
}

func TestVFSMkdirDeniedWhenUserOnlyHasReadACL(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, _ := bootstrapAdmin(t, engine)

	sourceID := createLocalSourceWithMountForTest(t, engine, adminToken, "e2e-root", "/")

	rec := performRequest(t, engine, http.MethodPost, "/api/v2/fs/mkdir", map[string]any{
		"parent_path": "/",
		"name":        "e2e-folder",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	userID, userToken := createNormalUserAndLoginForTest(t, engine, adminToken, "e2e-reader", "strong-password-123")
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/e2e-folder", map[string]any{
		"read":   true,
		"write":  false,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)

	rec = performRequest(t, engine, http.MethodPost, "/api/v2/fs/mkdir", map[string]any{
		"parent_path": "/e2e-folder",
		"name":        "should-not-create",
	}, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only user mkdir expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "ACL_DENIED")
}

func TestVFSListFiltersUnauthorizedMountedChildren(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, _ := bootstrapAdmin(t, engine)

	sourceID := createLocalSourceWithMountForTest(t, engine, adminToken, "acl-list-root", "/acl-list-local")
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/", "visible-check-20260426.txt", []byte("leak"))

	rec := performRequest(t, engine, http.MethodPost, "/api/v2/fs/mkdir", map[string]any{
		"parent_path": "/acl-list-local",
		"name":        "mount-check-folder",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin vfs mkdir expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	userID, userToken := createNormalUserAndLoginForTest(t, engine, adminToken, "vfs-list-reader", "strong-password-123")
	createACLRuleForTest(t, engine, adminToken, sourceID, userID, "/mount-check-folder", map[string]any{
		"read":   true,
		"write":  false,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)

	rec = performRequest(t, engine, http.MethodGet, "/api/v2/fs/list?path=/acl-list-local", nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs list expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[vfsListData](t, rec.Body.Bytes())
	names := collectMapNames(listed.Items)
	if !containsString(names, "mount-check-folder") {
		t.Fatalf("expected authorized folder visible, got %+v", listed.Items)
	}
	if containsString(names, "visible-check-20260426.txt") {
		t.Fatalf("expected unauthorized file hidden from vfs list, got %+v", listed.Items)
	}
	for _, item := range listed.Items {
		if item["name"] != "mount-check-folder" {
			continue
		}
		if item["can_delete"] == true || item["can_download"] == true {
			t.Fatalf("expected read-only directory capabilities constrained, got %+v", item)
		}
	}
}

func TestVFSRootListHidesUnauthorizedVirtualMountParents(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, _ := bootstrapAdmin(t, engine)

	visibleSourceID := createLocalSourceWithMountForTest(t, engine, adminToken, "visible-nested", "/visible-parent/allowed")
	_ = createLocalSourceWithMountForTest(t, engine, adminToken, "hidden-nested", "/hidden-parent/secret")

	userID, userToken := createNormalUserAndLoginForTest(t, engine, adminToken, "vfs-root-reader", "strong-password-123")
	createACLRuleForTest(t, engine, adminToken, visibleSourceID, userID, "/", map[string]any{
		"read":   true,
		"write":  false,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)

	rec := performRequest(t, engine, http.MethodGet, "/api/v2/fs/list?path=/", nil, userToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("vfs root list expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	listed := decodeEnvelope[vfsListData](t, rec.Body.Bytes())
	names := collectMapNames(listed.Items)
	if !containsString(names, "visible-parent") {
		t.Fatalf("expected authorized virtual parent visible, got %+v", listed.Items)
	}
	if containsString(names, "hidden-parent") {
		t.Fatalf("expected unauthorized virtual parent hidden, got %+v", listed.Items)
	}
}

func TestVFSMkdirRejectsPureVirtualParent(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	_ = createLocalSourceWithMountForTest(t, engine, accessToken, "docs-team", "/docs/team")
	_ = createLocalSourceWithMountForTest(t, engine, accessToken, "docs-personal", "/docs/personal")

	rec := performRequest(t, engine, http.MethodPost, "/api/v2/fs/mkdir", map[string]any{
		"parent_path": "/docs",
		"name":        "shared",
	}, accessToken)
	if rec.Code != http.StatusConflict {
		t.Fatalf("vfs pure virtual mkdir expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "NO_BACKING_STORAGE")
}

func TestVFSRenameRejectsMountNameConflict(t *testing.T) {
	engine := newStorageTestRouter(t)
	accessToken, _ := bootstrapAdmin(t, engine)

	docsSourceID := createLocalSourceWithMountForTest(t, engine, accessToken, "docs-root", "/docs")
	_ = createLocalSourceWithMountForTest(t, engine, accessToken, "docs-team-archive", "/docs/team/archive")
	uploadLocalObjectForTest(t, engine, accessToken, docsSourceID, "/", "readme.md", []byte("hello"))

	rec := performRequest(t, engine, http.MethodPost, "/api/v2/fs/rename", map[string]any{
		"path":     "/docs/readme.md",
		"new_name": "team",
	}, accessToken)
	if rec.Code != http.StatusConflict {
		t.Fatalf("vfs rename mount conflict expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertFailureCode(t, rec.Body.Bytes(), "NAME_CONFLICT")
}

func createLocalSourceWithMountForTest(t *testing.T, engine *gin.Engine, accessToken string, name string, mountPath string) int {
	t.Helper()

	basePath := filepath.ToSlash(filepath.Join(t.TempDir(), name))
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(local source %s) error = %v", name, err)
	}
	rec := performRequest(t, engine, http.MethodPost, "/api/v1/sources", map[string]any{
		"name":              name,
		"driver_type":       "local",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"mount_path":        mountPath,
		"root_path":         "/",
		"sort_order":        20,
		"config":            map[string]any{"base_path": basePath},
		"secret_patch":      map[string]any{},
	}, accessToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create local source expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	created := decodeEnvelope[sourceCreateData](t, rec.Body.Bytes())
	return int(created.Source["id"].(float64))
}

func createS3SourceWithMountForTest(t *testing.T, engine *gin.Engine, accessToken string, name string, mountPath string) int {
	t.Helper()

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/sources", map[string]any{
		"name":              name,
		"driver_type":       "s3",
		"is_enabled":        true,
		"is_webdav_exposed": false,
		"webdav_read_only":  true,
		"mount_path":        mountPath,
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

func collectMapNames(items []map[string]any) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item["name"].(string))
	}
	return names
}
