package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	appdto "yunxia/internal/application/dto"
	appsvc "yunxia/internal/application/service"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
)

func TestWebDAVLocalSourceUsesMetadataVFSNotPhysicalDir(t *testing.T) {
	physicalRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(physicalRoot, "physical-secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write physical fixture: %v", err)
	}
	source := &entity.StorageSource{
		ID:              9,
		Name:            "Local",
		DriverType:      "local",
		Status:          "online",
		IsEnabled:       true,
		IsWebDAVExposed: true,
		WebDAVReadOnly:  false,
		WebDAVSlug:      "local",
		MountPath:       "/local",
		RootPath:        "/",
		ConfigJSON:      `{"base_path":"` + strings.ReplaceAll(physicalRoot, `\`, `/`) + `"}`,
	}
	vfsSvc := &webDAVHandlerFakeVFSService{
		source: source,
		itemsByPath: map[string][]appdto.VFSItem{
			"/local": {
				vfsTestItem(source.ID, "metadata-only.txt", "/local/metadata-only.txt", false),
			},
		},
	}
	engine := newWebDAVHandlerTestEngine(source, vfsSvc, &webDAVHandlerFakeFileService{}, &webDAVHandlerFakeUploadService{})

	propfind := performWebDAVHandlerRequest(engine, "PROPFIND", "/dav/local", nil, nil)
	if propfind.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND expected 207, got %d body=%s", propfind.Code, propfind.Body.String())
	}
	body := propfind.Body.String()
	if !strings.Contains(body, "/dav/local/metadata-only.txt") {
		t.Fatalf("PROPFIND response should include metadata child, body=%s", body)
	}
	if strings.Contains(body, "physical-secret.txt") {
		t.Fatalf("PROPFIND leaked physical webdav.Dir child, body=%s", body)
	}
	if len(vfsSvc.listCalls) != 1 || vfsSvc.listCalls[0] != "/local" {
		t.Fatalf("expected PROPFIND to list metadata VFS mount path, got %+v", vfsSvc.listCalls)
	}
}

func TestWebDAVSourceUsesVFSBackedWorkflow(t *testing.T) {
	source := &entity.StorageSource{
		ID:              7,
		Name:            "Cloud",
		DriverType:      "cloud",
		Status:          "online",
		IsEnabled:       true,
		IsWebDAVExposed: true,
		WebDAVReadOnly:  false,
		WebDAVSlug:      "cloud",
		MountPath:       "/cloud",
		RootPath:        "/",
	}
	vfsSvc := &webDAVHandlerFakeVFSService{
		source: source,
		itemsByPath: map[string][]appdto.VFSItem{
			"/cloud": {
				vfsTestItem(source.ID, "remote.txt", "/cloud/remote.txt", false),
			},
		},
	}
	fileSvc := &webDAVHandlerFakeFileService{}
	uploadSvc := &webDAVHandlerFakeUploadService{}
	engine := newWebDAVHandlerTestEngine(source, vfsSvc, fileSvc, uploadSvc)

	depth0 := performWebDAVHandlerRequest(engine, "PROPFIND", "/dav/cloud", nil, map[string]string{"Depth": "0"})
	if depth0.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND Depth 0 expected 207, got %d body=%s", depth0.Code, depth0.Body.String())
	}
	if strings.Contains(depth0.Body.String(), "/dav/cloud/remote.txt") || len(vfsSvc.listCalls) != 0 {
		t.Fatalf("PROPFIND Depth 0 should not list children, calls=%+v body=%s", vfsSvc.listCalls, depth0.Body.String())
	}

	propfind := performWebDAVHandlerRequest(engine, "PROPFIND", "/dav/cloud", nil, nil)
	if propfind.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND expected 207, got %d body=%s", propfind.Code, propfind.Body.String())
	}
	if !strings.Contains(propfind.Body.String(), "/dav/cloud/remote.txt") {
		t.Fatalf("PROPFIND response should include listed child, body=%s", propfind.Body.String())
	}
	if len(vfsSvc.listCalls) != 1 || vfsSvc.listCalls[0] != "/cloud" {
		t.Fatalf("expected PROPFIND through VFS service, got %+v", vfsSvc.listCalls)
	}

	mkcol := performWebDAVHandlerRequest(engine, "MKCOL", "/dav/cloud/newdir", nil, nil)
	if mkcol.Code != http.StatusCreated {
		t.Fatalf("MKCOL expected 201, got %d body=%s", mkcol.Code, mkcol.Body.String())
	}
	if len(vfsSvc.mkdirCalls) != 1 || vfsSvc.mkdirCalls[0].ParentPath != "/cloud" || vfsSvc.mkdirCalls[0].Name != "newdir" {
		t.Fatalf("unexpected mkdir calls = %+v", vfsSvc.mkdirCalls)
	}
	afterMKCOL := performWebDAVHandlerRequest(engine, "PROPFIND", "/dav/cloud", nil, nil)
	if !strings.Contains(afterMKCOL.Body.String(), "/dav/cloud/newdir") {
		t.Fatalf("metadata list should reflect MKCOL immediately, body=%s", afterMKCOL.Body.String())
	}

	put := performWebDAVHandlerRequest(engine, http.MethodPut, "/dav/cloud/newdir/hello.txt", []byte("dav hello"), nil)
	if put.Code != http.StatusCreated {
		t.Fatalf("PUT expected 201, got %d body=%s", put.Code, put.Body.String())
	}
	if len(uploadSvc.importCalls) != 1 ||
		uploadSvc.importCalls[0].parentPath != "/newdir" ||
		uploadSvc.importCalls[0].filename != "hello.txt" ||
		uploadSvc.importCalls[0].content != "dav hello" {
		t.Fatalf("unexpected import calls = %+v", uploadSvc.importCalls)
	}

	get := performWebDAVHandlerRequest(engine, http.MethodGet, "/dav/cloud/remote.txt", nil, nil)
	getLocation := get.Header().Get("Location")
	if get.Code != http.StatusFound || !strings.HasPrefix(getLocation, "/api/v2/fs/download?") ||
		!strings.Contains(getLocation, "path=%2Fcloud%2Fremote.txt") ||
		!strings.Contains(getLocation, "disposition=inline") ||
		!strings.Contains(getLocation, "access_token=webdav-token") {
		t.Fatalf("GET expected redirect, code=%d location=%q body=%s", get.Code, get.Header().Get("Location"), get.Body.String())
	}
	head := performWebDAVHandlerRequest(engine, http.MethodHead, "/dav/cloud/remote.txt", nil, nil)
	if head.Code != http.StatusFound || !strings.HasPrefix(head.Header().Get("Location"), "/api/v2/fs/download?") {
		t.Fatalf("HEAD expected redirect, code=%d location=%q", head.Code, head.Header().Get("Location"))
	}
	if head.Body.Len() != 0 {
		t.Fatalf("HEAD must not write response body, got %q", head.Body.String())
	}
	if len(fileSvc.accessCalls) != 2 {
		t.Fatalf("GET/HEAD should request two short links, got %+v", fileSvc.accessCalls)
	}
	for _, call := range fileSvc.accessCalls {
		if call.SourceID != source.ID || call.Path != "/remote.txt" || call.Disposition != "inline" || call.ExpiresIn != 300 || call.Purpose != "download" {
			t.Fatalf("unexpected access url request = %+v", call)
		}
	}

	copyReqHeaders := map[string]string{"Destination": "/dav/cloud/copies/remote.txt"}
	copyResp := performWebDAVHandlerRequest(engine, "COPY", "/dav/cloud/remote.txt", nil, copyReqHeaders)
	if copyResp.Code != http.StatusCreated {
		t.Fatalf("COPY expected 201, got %d body=%s", copyResp.Code, copyResp.Body.String())
	}
	if len(vfsSvc.copyCalls) != 1 || vfsSvc.copyCalls[0].TargetPath != "/cloud/copies" {
		t.Fatalf("unexpected copy calls = %+v", vfsSvc.copyCalls)
	}

	moveReqHeaders := map[string]string{"Destination": "/dav/cloud/moved/remote.txt"}
	moveResp := performWebDAVHandlerRequest(engine, "MOVE", "/dav/cloud/remote.txt", nil, moveReqHeaders)
	if moveResp.Code != http.StatusCreated {
		t.Fatalf("MOVE expected 201, got %d body=%s", moveResp.Code, moveResp.Body.String())
	}
	if len(vfsSvc.moveCalls) != 1 || vfsSvc.moveCalls[0].TargetPath != "/cloud/moved" {
		t.Fatalf("unexpected move calls = %+v", vfsSvc.moveCalls)
	}

	deleteResp := performWebDAVHandlerRequest(engine, http.MethodDelete, "/dav/cloud/remote.txt", nil, nil)
	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("DELETE expected 204, got %d body=%s", deleteResp.Code, deleteResp.Body.String())
	}
	if len(vfsSvc.deleteCalls) != 1 || vfsSvc.deleteCalls[0].Path != "/cloud/remote.txt" {
		t.Fatalf("unexpected delete calls = %+v", vfsSvc.deleteCalls)
	}
}

func TestWebDAVPropfindUsesFilteredVFSListWithoutLeakingUnauthorizedNames(t *testing.T) {
	source := &entity.StorageSource{
		ID:              10,
		Name:            "Team",
		DriverType:      "local",
		Status:          "online",
		IsEnabled:       true,
		IsWebDAVExposed: true,
		WebDAVSlug:      "team",
		MountPath:       "/team",
		RootPath:        "/",
		ConfigJSON:      `{"base_path":"/sandbox/team"}`,
	}
	vfsSvc := &webDAVHandlerFakeVFSService{
		source: source,
		itemsByPath: map[string][]appdto.VFSItem{
			"/team": {
				vfsTestItem(source.ID, "allowed", "/team/allowed", true),
			},
		},
	}
	engine := newWebDAVHandlerTestEngine(source, vfsSvc, &webDAVHandlerFakeFileService{}, &webDAVHandlerFakeUploadService{})

	rec := performWebDAVHandlerRequest(engine, "PROPFIND", "/dav/team", nil, nil)
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND expected 207, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/dav/team/allowed/") && !strings.Contains(body, "/dav/team/allowed") {
		t.Fatalf("PROPFIND should include VFS-authorized child, body=%s", body)
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "private") {
		t.Fatalf("PROPFIND leaked unauthorized child name, body=%s", body)
	}
}

func TestWebDAVPropfindRootRequiresVisibleSourceACL(t *testing.T) {
	source := &entity.StorageSource{
		ID:              11,
		Name:            "Private",
		DriverType:      "local",
		Status:          "online",
		IsEnabled:       true,
		IsWebDAVExposed: true,
		WebDAVSlug:      "private",
		MountPath:       "/private",
		RootPath:        "/",
		ConfigJSON:      `{"base_path":"/sandbox/private"}`,
	}
	sourceRepo := &webDAVHandlerSourceRepo{source: source}
	systemRepo := &webDAVHandlerSystemRepo{cfg: &entity.SystemConfig{
		MultiUserEnabled: true,
		WebDAVEnabled:    true,
		WebDAVPrefix:     "/dav",
	}}
	userRepo := &webDAVHandlerUserRepo{user: &entity.User{
		ID:           2,
		Username:     "admin",
		PasswordHash: "hash",
		RoleKey:      permission.RoleUser,
		Status:       permission.StatusActive,
	}}
	aclAuthorizer := appsvc.NewACLAuthorizer(systemRepo, &webDAVHandlerACLRuleRepo{}, sourceRepo)
	vfsSvc := &webDAVHandlerFakeVFSService{source: source}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := NewWebDAVHandler(
		"/dav",
		sourceRepo,
		systemRepo,
		userRepo,
		aclAuthorizer,
		vfsSvc,
		&webDAVHandlerFakeFileService{},
		&webDAVHandlerFakeUploadService{},
		webDAVHandlerPasswordComparer{},
		nil,
		nil,
	)
	engine.Handle("PROPFIND", "/dav/:slug", handler.Serve)
	engine.Handle("PROPFIND", "/dav/:slug/*filepath", handler.Serve)

	rec := performWebDAVHandlerRequest(engine, "PROPFIND", "/dav/private", nil, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthorized source root PROPFIND expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(vfsSvc.listCalls) != 0 {
		t.Fatalf("unauthorized root should not list VFS children, got %+v", vfsSvc.listCalls)
	}
}

func TestWebDAVReadOnlySourceRejectsWritesWithoutPhysicalPathLeak(t *testing.T) {
	source := &entity.StorageSource{
		ID:              8,
		Name:            "ReadOnlyCloud",
		DriverType:      "cloud",
		Status:          "online",
		IsEnabled:       true,
		IsWebDAVExposed: true,
		WebDAVReadOnly:  true,
		WebDAVSlug:      "readonly",
		MountPath:       "/readonly",
		RootPath:        "/",
		ConfigJSON:      `{"base_path":"/container/secret/path"}`,
	}
	fileSvc := &webDAVHandlerFakeFileService{}
	vfsSvc := &webDAVHandlerFakeVFSService{source: source}
	engine := newWebDAVHandlerTestEngine(source, vfsSvc, fileSvc, &webDAVHandlerFakeUploadService{})

	rec := performWebDAVHandlerRequest(engine, "MKCOL", "/dav/readonly/newdir", nil, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only MKCOL expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "/container/secret/path") {
		t.Fatalf("read-only response leaked physical path: %s", rec.Body.String())
	}
	if len(vfsSvc.mkdirCalls) != 0 {
		t.Fatalf("read-only source should not call VFS mutation, got %+v", vfsSvc.mkdirCalls)
	}
}

func newWebDAVHandlerTestEngine(source *entity.StorageSource, vfsSvc webDAVVFSService, fileSvc webDAVFileService, uploadSvc webDAVUploadService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := NewWebDAVHandler(
		"/dav",
		&webDAVHandlerSourceRepo{source: source},
		&webDAVHandlerSystemRepo{cfg: &entity.SystemConfig{WebDAVEnabled: true, WebDAVPrefix: "/dav"}},
		&webDAVHandlerUserRepo{user: &entity.User{ID: 1, Username: "admin", PasswordHash: "hash", RoleKey: permission.RoleSuperAdmin, Status: permission.StatusActive}},
		nil,
		vfsSvc,
		fileSvc,
		uploadSvc,
		webDAVHandlerPasswordComparer{},
		nil,
		nil,
	)
	methods := []string{http.MethodOptions, http.MethodHead, http.MethodGet, http.MethodPut, http.MethodDelete, "PROPFIND", "MKCOL", "COPY", "MOVE"}
	for _, method := range methods {
		engine.Handle(method, "/dav/:slug", handler.Serve)
		engine.Handle(method, "/dav/:slug/*filepath", handler.Serve)
	}
	return engine
}

func performWebDAVHandlerRequest(engine *gin.Engine, method string, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("X-Forwarded-Proto", "https")
	req.SetBasicAuth("admin", "password")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

type webDAVHandlerPasswordComparer struct{}

func (webDAVHandlerPasswordComparer) Compare(string, string) bool {
	return true
}

type webDAVHandlerSourceRepo struct {
	source *entity.StorageSource
}

func (r *webDAVHandlerSourceRepo) Create(context.Context, *entity.StorageSource) error { return nil }
func (r *webDAVHandlerSourceRepo) Update(context.Context, *entity.StorageSource) error { return nil }
func (r *webDAVHandlerSourceRepo) Delete(context.Context, uint) error                  { return nil }
func (r *webDAVHandlerSourceRepo) FindByID(_ context.Context, id uint) (*entity.StorageSource, error) {
	if r.source != nil && r.source.ID == id {
		return r.source, nil
	}
	return nil, domainrepo.ErrNotFound
}
func (r *webDAVHandlerSourceRepo) ListAll(context.Context) ([]*entity.StorageSource, error) {
	if r.source == nil {
		return nil, nil
	}
	return []*entity.StorageSource{r.source}, nil
}
func (r *webDAVHandlerSourceRepo) ListEnabled(context.Context) ([]*entity.StorageSource, error) {
	return r.ListAll(context.Background())
}
func (r *webDAVHandlerSourceRepo) FindByName(context.Context, string) (*entity.StorageSource, error) {
	return nil, domainrepo.ErrNotFound
}
func (r *webDAVHandlerSourceRepo) Count(context.Context) (int64, error) { return 0, nil }

type webDAVHandlerSystemRepo struct {
	cfg *entity.SystemConfig
}

func (r *webDAVHandlerSystemRepo) Get(context.Context) (*entity.SystemConfig, error) {
	if r.cfg == nil {
		return nil, domainrepo.ErrNotFound
	}
	return r.cfg, nil
}

func (r *webDAVHandlerSystemRepo) Upsert(_ context.Context, cfg *entity.SystemConfig) error {
	r.cfg = cfg
	return nil
}

type webDAVHandlerUserRepo struct {
	user *entity.User
}

func (r *webDAVHandlerUserRepo) Create(context.Context, *entity.User) error { return nil }
func (r *webDAVHandlerUserRepo) FindByID(_ context.Context, id uint) (*entity.User, error) {
	if r.user != nil && r.user.ID == id {
		return r.user, nil
	}
	return nil, domainrepo.ErrNotFound
}
func (r *webDAVHandlerUserRepo) FindByUsername(_ context.Context, username string) (*entity.User, error) {
	if r.user != nil && r.user.Username == username {
		return r.user, nil
	}
	return nil, domainrepo.ErrNotFound
}
func (r *webDAVHandlerUserRepo) List(context.Context, domainrepo.UserListFilter) ([]*entity.User, error) {
	if r.user == nil {
		return nil, nil
	}
	return []*entity.User{r.user}, nil
}
func (r *webDAVHandlerUserRepo) Count(context.Context) (int64, error) { return 0, nil }
func (r *webDAVHandlerUserRepo) Update(context.Context, *entity.User) error {
	return nil
}
func (r *webDAVHandlerUserRepo) UpdateTokenVersion(context.Context, uint, int) error {
	return nil
}

type webDAVHandlerACLRuleRepo struct {
	rules []*entity.ACLRule
}

func (r *webDAVHandlerACLRuleRepo) Create(_ context.Context, rule *entity.ACLRule) error {
	r.rules = append(r.rules, rule)
	return nil
}

func (r *webDAVHandlerACLRuleRepo) FindByID(_ context.Context, id uint) (*entity.ACLRule, error) {
	for _, rule := range r.rules {
		if rule.ID == id {
			return rule, nil
		}
	}
	return nil, domainrepo.ErrNotFound
}

func (r *webDAVHandlerACLRuleRepo) List(_ context.Context, filter domainrepo.ACLRuleFilter) ([]*entity.ACLRule, error) {
	items := make([]*entity.ACLRule, 0, len(r.rules))
	for _, rule := range r.rules {
		if filter.AnySource || rule.SourceID == filter.SourceID || (filter.IncludeGlobal && rule.SourceID == 0) {
			items = append(items, rule)
		}
	}
	return items, nil
}

func (r *webDAVHandlerACLRuleRepo) Update(_ context.Context, rule *entity.ACLRule) error {
	for index, current := range r.rules {
		if current.ID == rule.ID {
			r.rules[index] = rule
			return nil
		}
	}
	return domainrepo.ErrNotFound
}

func (r *webDAVHandlerACLRuleRepo) Delete(_ context.Context, id uint) error {
	for index, rule := range r.rules {
		if rule.ID == id {
			r.rules = append(r.rules[:index], r.rules[index+1:]...)
			return nil
		}
	}
	return domainrepo.ErrNotFound
}

type webDAVHandlerFakeFileService struct {
	accessCalls []appdto.AccessURLRequest
}

func (s *webDAVHandlerFakeFileService) AccessURL(_ context.Context, req appdto.AccessURLRequest) (*appdto.AccessURLResponse, error) {
	s.accessCalls = append(s.accessCalls, req)
	values := url.Values{}
	values.Set("source_id", "7")
	values.Set("path", req.Path)
	values.Set("disposition", req.Disposition)
	values.Set("access_token", "webdav-token")
	return &appdto.AccessURLResponse{URL: "/api/v1/files/download?" + values.Encode(), Method: "GET", ExpiresAt: "2026-05-05T08:05:00Z"}, nil
}

type webDAVHandlerFakeUploadService struct {
	importCalls []webDAVHandlerImportCall
}

type webDAVHandlerImportCall struct {
	parentPath string
	filename   string
	content    string
}

func (s *webDAVHandlerFakeUploadService) ImportLocalFile(_ context.Context, sourceID uint, parentPath string, filename string, localPath string) (*appdto.FileItem, error) {
	content, err := os.ReadFile(localPath)
	if err != nil {
		return nil, err
	}
	s.importCalls = append(s.importCalls, webDAVHandlerImportCall{parentPath: parentPath, filename: filename, content: string(content)})
	return &appdto.FileItem{Name: filename, Path: joinWebDAVTestPath(parentPath, filename), SourceID: sourceID, Size: int64(len(content))}, nil
}

func joinWebDAVTestPath(parent string, name string) string {
	parent = strings.TrimRight(parent, "/")
	if parent == "" {
		return "/" + name
	}
	return parent + "/" + name
}

func webDAVTestBase(value string) string {
	idx := strings.LastIndex(value, "/")
	if idx < 0 {
		return value
	}
	return value[idx+1:]
}

type webDAVHandlerFakeVFSService struct {
	source      *entity.StorageSource
	itemsByPath map[string][]appdto.VFSItem

	listCalls   []string
	mkdirCalls  []appdto.VFSMkdirRequest
	copyCalls   []appdto.VFSMoveCopyRequest
	moveCalls   []appdto.VFSMoveCopyRequest
	renameCalls []appdto.VFSRenameRequest
	deleteCalls []appdto.VFSDeleteRequest
}

func (s *webDAVHandlerFakeVFSService) List(_ context.Context, currentPath string) (*appdto.VFSListResponse, error) {
	s.listCalls = append(s.listCalls, currentPath)
	if s.itemsByPath == nil {
		s.itemsByPath = make(map[string][]appdto.VFSItem)
	}
	return &appdto.VFSListResponse{Items: append([]appdto.VFSItem(nil), s.itemsByPath[currentPath]...), CurrentPath: currentPath}, nil
}

func (s *webDAVHandlerFakeVFSService) ResolvePath(_ context.Context, virtualPath string) (appsvc.ResolvedPath, error) {
	if s.source == nil {
		return appsvc.ResolvedPath{}, errors.New("source missing")
	}
	inner := strings.TrimPrefix(virtualPath, strings.TrimRight(s.source.MountPath, "/"))
	if inner == "" {
		inner = "/"
	}
	if !strings.HasPrefix(inner, "/") {
		inner = "/" + inner
	}
	return appsvc.ResolvedPath{
		Source:           s.source,
		MatchedMountPath: s.source.MountPath,
		VirtualPath:      virtualPath,
		InnerPath:        inner,
		IsRealMount:      true,
	}, nil
}

func (s *webDAVHandlerFakeVFSService) Mkdir(_ context.Context, req appdto.VFSMkdirRequest) (*appdto.VFSItem, error) {
	s.mkdirCalls = append(s.mkdirCalls, req)
	created := vfsTestItem(s.source.ID, req.Name, joinWebDAVTestPath(req.ParentPath, req.Name), true)
	s.appendItem(req.ParentPath, created)
	return &created, nil
}

func (s *webDAVHandlerFakeVFSService) Rename(_ context.Context, req appdto.VFSRenameRequest) (string, string, *appdto.VFSItem, error) {
	s.renameCalls = append(s.renameCalls, req)
	parentPath := parentVirtualPathForHTTP(req.Path)
	newPath := joinWebDAVTestPath(parentPath, req.NewName)
	renamed := vfsTestItem(s.source.ID, req.NewName, newPath, false)
	s.removeItem(parentPath, req.Path)
	s.appendItem(parentPath, renamed)
	return req.Path, newPath, &renamed, nil
}

func (s *webDAVHandlerFakeVFSService) Move(_ context.Context, req appdto.VFSMoveCopyRequest) (string, string, error) {
	s.moveCalls = append(s.moveCalls, req)
	oldParent := parentVirtualPathForHTTP(req.Path)
	newPath := joinWebDAVTestPath(req.TargetPath, webDAVTestBase(req.Path))
	moved := vfsTestItem(s.source.ID, webDAVTestBase(req.Path), newPath, false)
	s.removeItem(oldParent, req.Path)
	s.appendItem(req.TargetPath, moved)
	return req.Path, newPath, nil
}

func (s *webDAVHandlerFakeVFSService) Copy(_ context.Context, req appdto.VFSMoveCopyRequest) (string, string, error) {
	s.copyCalls = append(s.copyCalls, req)
	newPath := joinWebDAVTestPath(req.TargetPath, webDAVTestBase(req.Path))
	copied := vfsTestItem(s.source.ID, webDAVTestBase(req.Path), newPath, false)
	s.appendItem(req.TargetPath, copied)
	return req.Path, newPath, nil
}

func (s *webDAVHandlerFakeVFSService) Delete(_ context.Context, req appdto.VFSDeleteRequest) (time.Time, error) {
	s.deleteCalls = append(s.deleteCalls, req)
	s.removeItem(parentVirtualPathForHTTP(req.Path), req.Path)
	return time.Now(), nil
}

func (s *webDAVHandlerFakeVFSService) appendItem(parentPath string, item appdto.VFSItem) {
	if s.itemsByPath == nil {
		s.itemsByPath = make(map[string][]appdto.VFSItem)
	}
	s.itemsByPath[parentPath] = append(s.itemsByPath[parentPath], item)
}

func (s *webDAVHandlerFakeVFSService) removeItem(parentPath string, targetPath string) {
	items := s.itemsByPath[parentPath]
	filtered := items[:0]
	for _, item := range items {
		if item.Path != targetPath {
			filtered = append(filtered, item)
		}
	}
	s.itemsByPath[parentPath] = filtered
}

func vfsTestItem(sourceID uint, name string, itemPath string, isDir bool) appdto.VFSItem {
	entryKind := "file"
	if isDir {
		entryKind = "directory"
	}
	return appdto.VFSItem{
		Name:        name,
		Path:        itemPath,
		ParentPath:  parentVirtualPathForHTTP(itemPath),
		SourceID:    &sourceID,
		EntryKind:   entryKind,
		Size:        11,
		ModifiedAt:  "2026-05-05T08:00:00Z",
		CreatedAt:   "2026-05-05T08:00:00Z",
		CanDownload: !isDir,
		CanDelete:   true,
	}
}
