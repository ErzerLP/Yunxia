package http

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

type shareListData struct {
	Items []map[string]any `json:"items"`
}

type shareCreateData struct {
	Share map[string]any `json:"share"`
}

type shareOpenData struct {
	Share       map[string]any   `json:"share"`
	CurrentPath string           `json:"current_path"`
	CurrentDir  map[string]any   `json:"current_dir"`
	Breadcrumbs []map[string]any `json:"breadcrumbs"`
	Pagination  map[string]any   `json:"pagination"`
	Items       []map[string]any `json:"items"`
}

func TestShareFileLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/docs", "hello.txt", []byte("hello share"))

	createRec := performRequest(t, engine, http.MethodPost, "/api/v1/shares", map[string]any{
		"source_id":  sourceID,
		"path":       "/docs/hello.txt",
		"expires_in": 300,
	}, adminToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create share expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	created := decodeEnvelope[shareCreateData](t, createRec.Body.Bytes())
	shareID := int(created.Share["id"].(float64))
	link := created.Share["link"].(string)
	if link == "" {
		t.Fatalf("expected share link, got %+v", created.Share)
	}

	listRec := performRequest(t, engine, http.MethodGet, "/api/v1/shares", nil, adminToken)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list shares expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	listed := decodeEnvelope[shareListData](t, listRec.Body.Bytes())
	if len(listed.Items) != 1 {
		t.Fatalf("expected one share item, got %+v", listed.Items)
	}
	if int(listed.Items[0]["id"].(float64)) != shareID {
		t.Fatalf("expected listed share id=%d, got %+v", shareID, listed.Items)
	}

	publicRec := performRequest(t, engine, http.MethodGet, link, nil, "")
	if publicRec.Code != http.StatusFound {
		t.Fatalf("public share open expected 302, got %d body=%s", publicRec.Code, publicRec.Body.String())
	}
	location := publicRec.Header().Get("Location")
	if location == "" {
		t.Fatalf("expected redirect location, got headers=%v", publicRec.Header())
	}

	downloadRec := performRequest(t, engine, http.MethodGet, location, nil, "")
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("shared file download expected 200, got %d body=%s", downloadRec.Code, downloadRec.Body.String())
	}
	if downloadRec.Body.String() != "hello share" {
		t.Fatalf("unexpected shared file body=%q", downloadRec.Body.String())
	}

	deleteRec := performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/shares/%d", shareID), nil, adminToken)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete share expected 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	missingRec := performRequest(t, engine, http.MethodGet, link, nil, "")
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("deleted share open expected 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}
	assertFailureCode(t, missingRec.Body.Bytes(), "SHARE_NOT_FOUND")
}

func TestShareGetAndUpdateLifecycle(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/docs", "draft.txt", []byte("draft share"))

	createRec := performRequest(t, engine, http.MethodPost, "/api/v1/shares", map[string]any{
		"source_id":  sourceID,
		"path":       "/docs/draft.txt",
		"expires_in": 300,
	}, adminToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create share expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	created := decodeEnvelope[shareCreateData](t, createRec.Body.Bytes())
	shareID := int(created.Share["id"].(float64))
	link := created.Share["link"].(string)

	getRec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/shares/%d", shareID), nil, adminToken)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get share expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	got := decodeEnvelope[shareCreateData](t, getRec.Body.Bytes())
	if int(got.Share["id"].(float64)) != shareID || got.Share["path"] != "/docs/draft.txt" {
		t.Fatalf("unexpected share detail %+v", got.Share)
	}
	if got.Share["has_password"] != false {
		t.Fatalf("expected initial share without password, got %+v", got.Share)
	}

	updateRec := performRequest(t, engine, http.MethodPut, fmt.Sprintf("/api/v1/shares/%d", shareID), map[string]any{
		"expires_in": 900,
		"password":   "updated-pass-123",
	}, adminToken)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update share expected 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	updated := decodeEnvelope[shareCreateData](t, updateRec.Body.Bytes())
	if updated.Share["has_password"] != true {
		t.Fatalf("expected password-protected share after update, got %+v", updated.Share)
	}
	if updated.Share["expires_at"] == nil {
		t.Fatalf("expected expires_at after update, got %+v", updated.Share)
	}

	requiredRec := performRequest(t, engine, http.MethodGet, link, nil, "")
	if requiredRec.Code != http.StatusUnauthorized {
		t.Fatalf("updated password share without password expected 401, got %d body=%s", requiredRec.Code, requiredRec.Body.String())
	}
	assertFailureCode(t, requiredRec.Body.Bytes(), "SHARE_PASSWORD_REQUIRED")

	validRec := performRequest(t, engine, http.MethodGet, link+"?password=updated-pass-123", nil, "")
	if validRec.Code != http.StatusFound {
		t.Fatalf("updated password share with password expected 302, got %d body=%s", validRec.Code, validRec.Body.String())
	}

	clearRec := performRequest(t, engine, http.MethodPut, fmt.Sprintf("/api/v1/shares/%d", shareID), map[string]any{
		"expires_in": 0,
		"password":   "",
	}, adminToken)
	if clearRec.Code != http.StatusOK {
		t.Fatalf("clear share protection expected 200, got %d body=%s", clearRec.Code, clearRec.Body.String())
	}
	cleared := decodeEnvelope[shareCreateData](t, clearRec.Body.Bytes())
	if cleared.Share["has_password"] != false {
		t.Fatalf("expected cleared share password, got %+v", cleared.Share)
	}
	if cleared.Share["expires_at"] != nil {
		t.Fatalf("expected cleared share expires_at=nil, got %+v", cleared.Share)
	}

	publicRec := performRequest(t, engine, http.MethodGet, link, nil, "")
	if publicRec.Code != http.StatusFound {
		t.Fatalf("cleared share open expected 302, got %d body=%s", publicRec.Code, publicRec.Body.String())
	}
}

func TestShareOwnerBoundaryAndACL(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	enableMultiUserForTest(t, engine, adminToken)
	ownerID, ownerToken := createNormalUserAndLoginForTest(t, engine, adminToken, "share-owner", "strong-password-456")
	peerID, peerToken := createNormalUserAndLoginForTest(t, engine, adminToken, "share-peer", "strong-password-789")

	createACLRuleForTest(t, engine, adminToken, sourceID, ownerID, "/shared", map[string]any{
		"read":   true,
		"write":  false,
		"delete": false,
		"share":  true,
	}, "allow", 100, true)
	createACLRuleForTest(t, engine, adminToken, sourceID, peerID, "/shared", map[string]any{
		"read":   true,
		"write":  false,
		"delete": false,
		"share":  false,
	}, "allow", 100, true)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "shared",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir shared expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/shared", "report.txt", []byte("shared report"))

	ownerCreateRec := performRequest(t, engine, http.MethodPost, "/api/v1/shares", map[string]any{
		"source_id":  sourceID,
		"path":       "/shared/report.txt",
		"expires_in": 300,
	}, ownerToken)
	if ownerCreateRec.Code != http.StatusCreated {
		t.Fatalf("owner create share expected 201, got %d body=%s", ownerCreateRec.Code, ownerCreateRec.Body.String())
	}
	ownerCreated := decodeEnvelope[shareCreateData](t, ownerCreateRec.Body.Bytes())
	shareID := int(ownerCreated.Share["id"].(float64))

	peerCreateRec := performRequest(t, engine, http.MethodPost, "/api/v1/shares", map[string]any{
		"source_id":  sourceID,
		"path":       "/shared/report.txt",
		"expires_in": 300,
	}, peerToken)
	if peerCreateRec.Code != http.StatusForbidden {
		t.Fatalf("peer create share expected 403, got %d body=%s", peerCreateRec.Code, peerCreateRec.Body.String())
	}
	assertFailureCode(t, peerCreateRec.Body.Bytes(), "ACL_DENIED")

	peerListRec := performRequest(t, engine, http.MethodGet, "/api/v1/shares", nil, peerToken)
	if peerListRec.Code != http.StatusOK {
		t.Fatalf("peer list shares expected 200, got %d body=%s", peerListRec.Code, peerListRec.Body.String())
	}
	peerList := decodeEnvelope[shareListData](t, peerListRec.Body.Bytes())
	if len(peerList.Items) != 0 {
		t.Fatalf("expected peer to see no owner shares, got %+v", peerList.Items)
	}

	peerDeleteRec := performRequest(t, engine, http.MethodDelete, fmt.Sprintf("/api/v1/shares/%d", shareID), nil, peerToken)
	if peerDeleteRec.Code != http.StatusForbidden {
		t.Fatalf("peer delete owner share expected 403, got %d body=%s", peerDeleteRec.Code, peerDeleteRec.Body.String())
	}
	assertFailureCode(t, peerDeleteRec.Body.Bytes(), "PERMISSION_DENIED")
}

func TestShareGetAndUpdateOwnerBoundary(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	enableMultiUserForTest(t, engine, adminToken)
	ownerID, ownerToken := createNormalUserAndLoginForTest(t, engine, adminToken, "share-owner-2", "strong-password-456")
	_, peerToken := createNormalUserAndLoginForTest(t, engine, adminToken, "share-peer-2", "strong-password-789")

	createACLRuleForTest(t, engine, adminToken, sourceID, ownerID, "/shared", map[string]any{
		"read":   true,
		"write":  false,
		"delete": false,
		"share":  true,
	}, "allow", 100, true)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "shared",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin mkdir shared expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/shared", "private.txt", []byte("private share"))

	createRec := performRequest(t, engine, http.MethodPost, "/api/v1/shares", map[string]any{
		"source_id":  sourceID,
		"path":       "/shared/private.txt",
		"expires_in": 300,
	}, ownerToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("owner create share expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	created := decodeEnvelope[shareCreateData](t, createRec.Body.Bytes())
	shareID := int(created.Share["id"].(float64))

	getRec := performRequest(t, engine, http.MethodGet, fmt.Sprintf("/api/v1/shares/%d", shareID), nil, peerToken)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("peer get owner share expected 403, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	assertFailureCode(t, getRec.Body.Bytes(), "PERMISSION_DENIED")

	updateRec := performRequest(t, engine, http.MethodPut, fmt.Sprintf("/api/v1/shares/%d", shareID), map[string]any{
		"password": "hijack",
	}, peerToken)
	if updateRec.Code != http.StatusForbidden {
		t.Fatalf("peer update owner share expected 403, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	assertFailureCode(t, updateRec.Body.Bytes(), "PERMISSION_DENIED")
}

func TestSharePasswordProtectedAccess(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/secure", "secret.txt", []byte("top-secret"))

	createRec := performRequest(t, engine, http.MethodPost, "/api/v1/shares", map[string]any{
		"source_id":  sourceID,
		"path":       "/secure/secret.txt",
		"expires_in": 300,
		"password":   "share-pass-123",
	}, adminToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("password share create expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	created := decodeEnvelope[shareCreateData](t, createRec.Body.Bytes())
	link := created.Share["link"].(string)

	requiredRec := performRequest(t, engine, http.MethodGet, link, nil, "")
	if requiredRec.Code != http.StatusUnauthorized {
		t.Fatalf("share without password expected 401, got %d body=%s", requiredRec.Code, requiredRec.Body.String())
	}
	assertFailureCode(t, requiredRec.Body.Bytes(), "SHARE_PASSWORD_REQUIRED")

	invalidRec := performRequest(t, engine, http.MethodGet, link+"?password=wrong-pass", nil, "")
	if invalidRec.Code != http.StatusUnauthorized {
		t.Fatalf("share with wrong password expected 401, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
	assertFailureCode(t, invalidRec.Body.Bytes(), "SHARE_PASSWORD_INVALID")

	validRec := performRequest(t, engine, http.MethodGet, link+"?password=share-pass-123", nil, "")
	if validRec.Code != http.StatusFound {
		t.Fatalf("share with correct password expected 302, got %d body=%s", validRec.Code, validRec.Body.String())
	}
}

func TestShareExpiredAccess(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/expired", "old.txt", []byte("expired"))

	createRec := performRequest(t, engine, http.MethodPost, "/api/v1/shares", map[string]any{
		"source_id":  sourceID,
		"path":       "/expired/old.txt",
		"expires_in": 1,
	}, adminToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expired share create expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	created := decodeEnvelope[shareCreateData](t, createRec.Body.Bytes())
	link := created.Share["link"].(string)

	time.Sleep(1500 * time.Millisecond)

	expiredRec := performRequest(t, engine, http.MethodGet, link, nil, "")
	if expiredRec.Code != http.StatusGone {
		t.Fatalf("expired share open expected 410, got %d body=%s", expiredRec.Code, expiredRec.Body.String())
	}
	assertFailureCode(t, expiredRec.Body.Bytes(), "SHARE_EXPIRED")
}

func TestShareDirectoryBrowseAndDownload(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "albums",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("mkdir albums expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	rec = performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/albums",
		"name":        "2026",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("mkdir albums/2026 expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/albums", "cover.jpg", []byte("root-cover"))
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/albums", "a.txt", []byte("a"))
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/albums", "b.txt", []byte("b"))
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/albums/2026", "photo.txt", []byte("nested-photo"))

	createRec := performRequest(t, engine, http.MethodPost, "/api/v1/shares", map[string]any{
		"source_id":  sourceID,
		"path":       "/albums",
		"expires_in": 300,
	}, adminToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("directory share create expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	created := decodeEnvelope[shareCreateData](t, createRec.Body.Bytes())
	link := created.Share["link"].(string)
	if created.Share["is_dir"] != true {
		t.Fatalf("expected directory share, got %+v", created.Share)
	}

	rootRec := performRequest(t, engine, http.MethodGet, link+"?page=1&page_size=2", nil, "")
	if rootRec.Code != http.StatusOK {
		t.Fatalf("directory share root expected 200, got %d body=%s", rootRec.Code, rootRec.Body.String())
	}
	rootData := decodeEnvelope[shareOpenData](t, rootRec.Body.Bytes())
	if rootData.CurrentPath != "/" {
		t.Fatalf("expected current_path=/, got %+v", rootData)
	}
	if rootData.CurrentDir["name"] != "albums" || rootData.CurrentDir["path"] != "/" || rootData.CurrentDir["is_root"] != true {
		t.Fatalf("unexpected current_dir %+v", rootData.CurrentDir)
	}
	if len(rootData.Breadcrumbs) != 1 || rootData.Breadcrumbs[0]["name"] != "albums" || rootData.Breadcrumbs[0]["path"] != "/" {
		t.Fatalf("unexpected breadcrumbs %+v", rootData.Breadcrumbs)
	}
	if int(rootData.Pagination["page"].(float64)) != 1 ||
		int(rootData.Pagination["page_size"].(float64)) != 2 ||
		int(rootData.Pagination["total"].(float64)) != 4 ||
		int(rootData.Pagination["total_pages"].(float64)) != 2 {
		t.Fatalf("unexpected pagination %+v", rootData.Pagination)
	}
	if len(rootData.Items) != 2 {
		t.Fatalf("expected 2 root items, got %+v", rootData.Items)
	}
	for _, item := range rootData.Items {
		if item["is_dir"] == false && item["preview_type"] == nil {
			t.Fatalf("expected preview_type for file items, got %+v", rootData.Items)
		}
	}

	nestedRec := performRequest(t, engine, http.MethodGet, link+"?path=/2026&page=1&page_size=10", nil, "")
	if nestedRec.Code != http.StatusOK {
		t.Fatalf("directory share nested expected 200, got %d body=%s", nestedRec.Code, nestedRec.Body.String())
	}
	nestedData := decodeEnvelope[shareOpenData](t, nestedRec.Body.Bytes())
	if nestedData.CurrentPath != "/2026" {
		t.Fatalf("expected current_path=/2026, got %+v", nestedData)
	}
	if nestedData.CurrentDir["name"] != "2026" || nestedData.CurrentDir["path"] != "/2026" || nestedData.CurrentDir["parent_path"] != "/" {
		t.Fatalf("unexpected nested current_dir %+v", nestedData.CurrentDir)
	}
	if len(nestedData.Breadcrumbs) != 2 || nestedData.Breadcrumbs[0]["path"] != "/" || nestedData.Breadcrumbs[1]["path"] != "/2026" {
		t.Fatalf("unexpected nested breadcrumbs %+v", nestedData.Breadcrumbs)
	}
	if int(nestedData.Pagination["page"].(float64)) != 1 ||
		int(nestedData.Pagination["page_size"].(float64)) != 10 ||
		int(nestedData.Pagination["total"].(float64)) != 1 ||
		int(nestedData.Pagination["total_pages"].(float64)) != 1 {
		t.Fatalf("unexpected nested pagination %+v", nestedData.Pagination)
	}
	if len(nestedData.Items) != 1 || nestedData.Items[0]["path"] != "/2026/photo.txt" {
		t.Fatalf("unexpected nested directory items %+v", nestedData.Items)
	}
	if nestedData.Items[0]["preview_type"] != "text" {
		t.Fatalf("expected nested text preview_type, got %+v", nestedData.Items[0])
	}

	fileRec := performRequest(t, engine, http.MethodGet, link+"?path=/2026/photo.txt", nil, "")
	if fileRec.Code != http.StatusFound {
		t.Fatalf("directory shared file open expected 302, got %d body=%s", fileRec.Code, fileRec.Body.String())
	}
	location := fileRec.Header().Get("Location")
	if location == "" {
		t.Fatalf("expected redirect location, got headers=%v", fileRec.Header())
	}
	downloadRec := performRequest(t, engine, http.MethodGet, location, nil, "")
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("directory shared file download expected 200, got %d body=%s", downloadRec.Code, downloadRec.Body.String())
	}
	if downloadRec.Body.String() != "nested-photo" {
		t.Fatalf("unexpected directory shared download body=%q", downloadRec.Body.String())
	}

	sortedRec := performRequest(t, engine, http.MethodGet, link+"?sort_by=name&sort_order=desc&page=1&page_size=10", nil, "")
	if sortedRec.Code != http.StatusOK {
		t.Fatalf("directory share sorted expected 200, got %d body=%s", sortedRec.Code, sortedRec.Body.String())
	}
	sortedData := decodeEnvelope[shareOpenData](t, sortedRec.Body.Bytes())
	expectedSortedPaths := []string{"/2026", "/cover.jpg", "/b.txt", "/a.txt"}
	if len(sortedData.Items) != len(expectedSortedPaths) {
		t.Fatalf("expected %d sorted items, got %+v", len(expectedSortedPaths), sortedData.Items)
	}
	for i, expected := range expectedSortedPaths {
		if sortedData.Items[i]["path"] != expected {
			t.Fatalf("expected sorted paths %v, got %+v", expectedSortedPaths, sortedData.Items)
		}
	}
	if sortedData.Items[0]["preview_type"] != "directory" || sortedData.Items[1]["preview_type"] != "image" {
		t.Fatalf("unexpected sorted preview types %+v", sortedData.Items)
	}
}

func TestShareDirectoryPathBoundary(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, sourceID := bootstrapAdmin(t, engine)

	rec := performRequest(t, engine, http.MethodPost, "/api/v1/files/mkdir", map[string]any{
		"source_id":   sourceID,
		"parent_path": "/",
		"name":        "public",
	}, adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("mkdir public expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/public", "readme.txt", []byte("public-readme"))
	uploadLocalObjectForTest(t, engine, adminToken, sourceID, "/", "secret.txt", []byte("secret-outside"))

	createRec := performRequest(t, engine, http.MethodPost, "/api/v1/shares", map[string]any{
		"source_id":  sourceID,
		"path":       "/public",
		"expires_in": 300,
	}, adminToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("public dir share create expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	created := decodeEnvelope[shareCreateData](t, createRec.Body.Bytes())
	link := created.Share["link"].(string)

	outsideRec := performRequest(t, engine, http.MethodGet, link+"?path=/../secret.txt", nil, "")
	if outsideRec.Code != http.StatusBadRequest {
		t.Fatalf("share escape path expected 400, got %d body=%s", outsideRec.Code, outsideRec.Body.String())
	}
	assertFailureCode(t, outsideRec.Body.Bytes(), "PATH_INVALID")
}

func TestS3ShareDirectoryBrowseAndRedirect(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, adminToken)

	createRec := performRequest(t, engine, http.MethodPost, "/api/v1/shares", map[string]any{
		"source_id":  sourceID,
		"path":       "/movies",
		"expires_in": 300,
	}, adminToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("s3 directory share create expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	created := decodeEnvelope[shareCreateData](t, createRec.Body.Bytes())
	link := created.Share["link"].(string)
	if created.Share["is_dir"] != true {
		t.Fatalf("expected s3 directory share, got %+v", created.Share)
	}

	rootRec := performRequest(t, engine, http.MethodGet, link+"?page=1&page_size=1", nil, "")
	if rootRec.Code != http.StatusOK {
		t.Fatalf("s3 directory share root expected 200, got %d body=%s", rootRec.Code, rootRec.Body.String())
	}
	rootData := decodeEnvelope[shareOpenData](t, rootRec.Body.Bytes())
	if rootData.CurrentPath != "/" {
		t.Fatalf("expected s3 current_path=/, got %+v", rootData)
	}
	if rootData.CurrentDir["name"] != "movies" || rootData.CurrentDir["path"] != "/" {
		t.Fatalf("unexpected s3 current_dir %+v", rootData.CurrentDir)
	}
	if len(rootData.Breadcrumbs) != 1 || rootData.Breadcrumbs[0]["name"] != "movies" {
		t.Fatalf("unexpected s3 breadcrumbs %+v", rootData.Breadcrumbs)
	}
	if int(rootData.Pagination["page"].(float64)) != 1 ||
		int(rootData.Pagination["page_size"].(float64)) != 1 ||
		int(rootData.Pagination["total"].(float64)) != 2 ||
		int(rootData.Pagination["total_pages"].(float64)) != 2 {
		t.Fatalf("unexpected s3 pagination %+v", rootData.Pagination)
	}
	if len(rootData.Items) != 1 {
		t.Fatalf("expected 1 paged s3 root item, got %+v", rootData.Items)
	}
	if rootData.Items[0]["path"] != "/demo.mp4" {
		t.Fatalf("unexpected s3 shared root items %+v", rootData.Items)
	}
	if rootData.Items[0]["preview_type"] != "video" {
		t.Fatalf("expected s3 video preview_type, got %+v", rootData.Items[0])
	}

	fileRec := performRequest(t, engine, http.MethodGet, link+"?path=/demo.mp4&disposition=inline", nil, "")
	if fileRec.Code != http.StatusFound {
		t.Fatalf("s3 shared file open expected 302, got %d body=%s", fileRec.Code, fileRec.Body.String())
	}
	location := fileRec.Header().Get("Location")
	if location == "" {
		t.Fatalf("expected s3 redirect location, got headers=%v", fileRec.Header())
	}
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("url.Parse(location) error = %v", err)
	}
	if parsed.Query().Get("path") != "/movies/demo.mp4" {
		t.Fatalf("unexpected s3 shared redirect path = %s", location)
	}
	if parsed.Query().Get("disposition") != "inline" {
		t.Fatalf("unexpected s3 shared redirect disposition = %s", location)
	}

	sortedRec := performRequest(t, engine, http.MethodGet, link+"?sort_by=name&sort_order=desc&page=1&page_size=10", nil, "")
	if sortedRec.Code != http.StatusOK {
		t.Fatalf("s3 directory share sorted expected 200, got %d body=%s", sortedRec.Code, sortedRec.Body.String())
	}
	sortedData := decodeEnvelope[shareOpenData](t, sortedRec.Body.Bytes())
	expectedSortedPaths := []string{"/trailer.mp4", "/demo.mp4"}
	if len(sortedData.Items) != len(expectedSortedPaths) {
		t.Fatalf("expected %d sorted s3 items, got %+v", len(expectedSortedPaths), sortedData.Items)
	}
	for i, expected := range expectedSortedPaths {
		if sortedData.Items[i]["path"] != expected {
			t.Fatalf("expected sorted s3 paths %v, got %+v", expectedSortedPaths, sortedData.Items)
		}
	}
	if sortedData.Items[0]["preview_type"] != "video" || sortedData.Items[1]["preview_type"] != "video" {
		t.Fatalf("unexpected sorted s3 preview types %+v", sortedData.Items)
	}
}

func TestS3ShareDirectoryPathBoundary(t *testing.T) {
	engine := newStorageTestRouter(t)
	adminToken, _ := bootstrapAdmin(t, engine)
	sourceID := createS3SourceForTest(t, engine, adminToken)

	createRec := performRequest(t, engine, http.MethodPost, "/api/v1/shares", map[string]any{
		"source_id":  sourceID,
		"path":       "/movies",
		"expires_in": 300,
	}, adminToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("s3 public dir share create expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	created := decodeEnvelope[shareCreateData](t, createRec.Body.Bytes())
	link := created.Share["link"].(string)

	outsideRec := performRequest(t, engine, http.MethodGet, link+"?path=/../covers/poster.jpg", nil, "")
	if outsideRec.Code != http.StatusBadRequest {
		t.Fatalf("s3 share escape path expected 400, got %d body=%s", outsideRec.Code, outsideRec.Body.String())
	}
	assertFailureCode(t, outsideRec.Body.Bytes(), "PATH_INVALID")
}
