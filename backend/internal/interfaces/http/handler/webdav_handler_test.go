package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
)

func TestWebDAVNonLocalSourceUsesServiceBackedWorkflow(t *testing.T) {
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
	fileSvc := &webDAVHandlerFakeFileService{
		itemsByPath: map[string][]appdto.FileItem{
			"/": {
				{Name: "remote.txt", Path: "/remote.txt", SourceID: source.ID, IsDir: false, Size: 11, ModifiedAt: "2026-05-05T08:00:00Z"},
			},
		},
		statByPath: map[string]appdto.FileItem{
			"/remote.txt": {Name: "remote.txt", Path: "/remote.txt", SourceID: source.ID, IsDir: false, Size: 11, ModifiedAt: "2026-05-05T08:00:00Z"},
		},
		redirectURL: "https://provider.example/remote.txt",
	}
	uploadSvc := &webDAVHandlerFakeUploadService{}
	engine := newWebDAVHandlerTestEngine(source, fileSvc, uploadSvc)

	propfind := performWebDAVHandlerRequest(engine, "PROPFIND", "/dav/cloud", nil, nil)
	if propfind.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND expected 207, got %d body=%s", propfind.Code, propfind.Body.String())
	}
	if !strings.Contains(propfind.Body.String(), "/dav/cloud/remote.txt") {
		t.Fatalf("PROPFIND response should include listed child, body=%s", propfind.Body.String())
	}
	if len(fileSvc.listCalls) != 1 || fileSvc.listCalls[0] != "/" {
		t.Fatalf("expected list through file service for non-local PROPFIND, got %+v", fileSvc.listCalls)
	}

	mkcol := performWebDAVHandlerRequest(engine, "MKCOL", "/dav/cloud/newdir", nil, nil)
	if mkcol.Code != http.StatusCreated {
		t.Fatalf("MKCOL expected 201, got %d body=%s", mkcol.Code, mkcol.Body.String())
	}
	if len(fileSvc.mkdirCalls) != 1 || fileSvc.mkdirCalls[0].ParentPath != "/" || fileSvc.mkdirCalls[0].Name != "newdir" {
		t.Fatalf("unexpected mkdir calls = %+v", fileSvc.mkdirCalls)
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
	if get.Code != http.StatusFound || get.Header().Get("Location") != "https://provider.example/remote.txt" {
		t.Fatalf("GET expected redirect, code=%d location=%q body=%s", get.Code, get.Header().Get("Location"), get.Body.String())
	}

	copyReqHeaders := map[string]string{"Destination": "/dav/cloud/copies/remote.txt"}
	copyResp := performWebDAVHandlerRequest(engine, "COPY", "/dav/cloud/remote.txt", nil, copyReqHeaders)
	if copyResp.Code != http.StatusCreated {
		t.Fatalf("COPY expected 201, got %d body=%s", copyResp.Code, copyResp.Body.String())
	}
	if len(fileSvc.copyCalls) != 1 || fileSvc.copyCalls[0].TargetPath != "/copies" {
		t.Fatalf("unexpected copy calls = %+v", fileSvc.copyCalls)
	}

	moveReqHeaders := map[string]string{"Destination": "/dav/cloud/moved/remote.txt"}
	moveResp := performWebDAVHandlerRequest(engine, "MOVE", "/dav/cloud/remote.txt", nil, moveReqHeaders)
	if moveResp.Code != http.StatusCreated {
		t.Fatalf("MOVE expected 201, got %d body=%s", moveResp.Code, moveResp.Body.String())
	}
	if len(fileSvc.moveCalls) != 1 || fileSvc.moveCalls[0].TargetPath != "/moved" {
		t.Fatalf("unexpected move calls = %+v", fileSvc.moveCalls)
	}

	deleteResp := performWebDAVHandlerRequest(engine, http.MethodDelete, "/dav/cloud/remote.txt", nil, nil)
	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("DELETE expected 204, got %d body=%s", deleteResp.Code, deleteResp.Body.String())
	}
	if len(fileSvc.deleteCalls) != 1 || fileSvc.deleteCalls[0].Path != "/remote.txt" {
		t.Fatalf("unexpected delete calls = %+v", fileSvc.deleteCalls)
	}
}

func TestWebDAVNonLocalReadOnlySourceRejectsWrites(t *testing.T) {
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
	}
	fileSvc := &webDAVHandlerFakeFileService{}
	engine := newWebDAVHandlerTestEngine(source, fileSvc, &webDAVHandlerFakeUploadService{})

	rec := performWebDAVHandlerRequest(engine, "MKCOL", "/dav/readonly/newdir", nil, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only MKCOL expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(fileSvc.mkdirCalls) != 0 {
		t.Fatalf("read-only source should not call file service, got %+v", fileSvc.mkdirCalls)
	}
}

func newWebDAVHandlerTestEngine(source *entity.StorageSource, fileSvc webDAVFileService, uploadSvc webDAVUploadService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := NewWebDAVHandler(
		"/dav",
		&webDAVHandlerSourceRepo{source: source},
		&webDAVHandlerSystemRepo{cfg: &entity.SystemConfig{WebDAVEnabled: true, WebDAVPrefix: "/dav"}},
		&webDAVHandlerUserRepo{user: &entity.User{ID: 1, Username: "admin", PasswordHash: "hash", RoleKey: permission.RoleSuperAdmin, Status: permission.StatusActive}},
		nil,
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

type webDAVHandlerFakeFileService struct {
	itemsByPath map[string][]appdto.FileItem
	statByPath  map[string]appdto.FileItem
	redirectURL string

	listCalls   []string
	mkdirCalls  []appdto.MkdirRequest
	copyCalls   []appdto.MoveCopyRequest
	moveCalls   []appdto.MoveCopyRequest
	renameCalls []appdto.RenameRequest
	deleteCalls []appdto.DeleteFileRequest
}

func (s *webDAVHandlerFakeFileService) List(_ context.Context, query appdto.FileListQuery) (*appdto.FileListResponse, int, int, int, int, error) {
	s.listCalls = append(s.listCalls, query.Path)
	return &appdto.FileListResponse{Items: s.itemsByPath[query.Path], CurrentPath: query.Path, CurrentSourceID: query.SourceID}, 1, 10000, len(s.itemsByPath[query.Path]), 1, nil
}

func (s *webDAVHandlerFakeFileService) Stat(_ context.Context, sourceID uint, filePath string) (*appdto.FileItem, error) {
	if item, ok := s.statByPath[filePath]; ok {
		return &item, nil
	}
	for parent, items := range s.itemsByPath {
		_ = parent
		for _, item := range items {
			if item.Path == filePath {
				found := item
				return &found, nil
			}
		}
	}
	return nil, errors.New("not found")
}

func (s *webDAVHandlerFakeFileService) Mkdir(_ context.Context, req appdto.MkdirRequest) (*appdto.FileItem, error) {
	s.mkdirCalls = append(s.mkdirCalls, req)
	return &appdto.FileItem{Name: req.Name, Path: joinWebDAVTestPath(req.ParentPath, req.Name), SourceID: req.SourceID, IsDir: true}, nil
}

func (s *webDAVHandlerFakeFileService) Rename(_ context.Context, req appdto.RenameRequest) (string, string, *appdto.FileItem, error) {
	s.renameCalls = append(s.renameCalls, req)
	parent := req.Path[:strings.LastIndex(req.Path, "/")]
	if parent == "" {
		parent = "/"
	}
	newPath := joinWebDAVTestPath(parent, req.NewName)
	return req.Path, newPath, &appdto.FileItem{Name: req.NewName, Path: newPath, SourceID: req.SourceID}, nil
}

func (s *webDAVHandlerFakeFileService) Move(_ context.Context, req appdto.MoveCopyRequest) (string, string, error) {
	s.moveCalls = append(s.moveCalls, req)
	return req.Path, joinWebDAVTestPath(req.TargetPath, webDAVTestBase(req.Path)), nil
}

func (s *webDAVHandlerFakeFileService) Copy(_ context.Context, req appdto.MoveCopyRequest) (string, string, error) {
	s.copyCalls = append(s.copyCalls, req)
	return req.Path, joinWebDAVTestPath(req.TargetPath, webDAVTestBase(req.Path)), nil
}

func (s *webDAVHandlerFakeFileService) Delete(_ context.Context, req appdto.DeleteFileRequest) (time.Time, error) {
	s.deleteCalls = append(s.deleteCalls, req)
	return time.Now(), nil
}

func (s *webDAVHandlerFakeFileService) ResolveDownloadRedirect(context.Context, uint, string, string) (string, error) {
	return s.redirectURL, nil
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
