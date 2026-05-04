package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"yunxia/internal/domain/entity"
	domainstorage "yunxia/internal/domain/storage"
)

func TestPikPakDriverListStatDownloadSearchAndCapacity(t *testing.T) {
	client := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {
				{ID: "folder-movies", Name: "Movies", Kind: "drive#folder", ModifiedTime: "2026-05-04T10:00:00Z"},
				{ID: "file-readme", Name: "readme.txt", Kind: "drive#file", Size: "12", Hash: "hash-readme", ModifiedTime: "2026-05-04T11:00:00Z"},
			},
			"folder-movies": {
				{ID: "file-clip", Name: "clip.mp4", Kind: "drive#file", Size: "1048576", Hash: "hash-clip", ModifiedTime: "2026-05-04T12:00:00Z"},
			},
		},
		fileDetails: map[string]PikPakFile{
			"file-clip": {ID: "file-clip", Name: "clip.mp4", Kind: "drive#file", Size: "1048576", WebContentLink: "https://download.example/clip.mp4"},
		},
		about: &PikPakAbout{Quota: PikPakQuota{Usage: "100", Limit: "1000"}},
	}
	driver := NewPikPakDriver(WithPikPakAPIClient(client))
	source := newTestPikPakSource(t)

	if err := driver.Test(context.Background(), source); err != nil {
		t.Fatalf("Test() error = %v", err)
	}

	entries, err := driver.List(context.Background(), source, "/")
	if err != nil {
		t.Fatalf("List(/) error = %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	if !reflect.DeepEqual(names, []string{"Movies", "readme.txt"}) {
		t.Fatalf("unexpected root names = %v", names)
	}
	if entries[0].Path != "/Movies" || !entries[0].IsDir {
		t.Fatalf("unexpected folder entry = %+v", entries[0])
	}

	stat, err := driver.Stat(context.Background(), source, "/Movies/clip.mp4")
	if err != nil {
		t.Fatalf("Stat(clip) error = %v", err)
	}
	if stat.Name != "clip.mp4" || stat.Path != "/Movies/clip.mp4" || stat.Size != 1048576 || stat.IsDir {
		t.Fatalf("unexpected stat = %+v", stat)
	}

	rawURL, _, err := driver.PresignDownload(context.Background(), source, "/Movies/clip.mp4", "attachment", 0)
	if err != nil {
		t.Fatalf("PresignDownload() error = %v", err)
	}
	if rawURL != "https://download.example/clip.mp4" {
		t.Fatalf("unexpected download url = %s", rawURL)
	}

	search, err := driver.SearchByName(context.Background(), source, "/", "clip")
	if err != nil {
		t.Fatalf("SearchByName() error = %v", err)
	}
	if len(search) != 1 || search[0].Path != "/Movies/clip.mp4" {
		t.Fatalf("unexpected search result = %+v", search)
	}

	capacity, err := driver.Capacity(context.Background(), source)
	if err != nil {
		t.Fatalf("Capacity() error = %v", err)
	}
	if capacity.UsedBytes == nil || *capacity.UsedBytes != 100 || capacity.TotalBytes == nil || *capacity.TotalBytes != 1000 {
		t.Fatalf("unexpected capacity = %+v", capacity)
	}
}

func TestPikPakDriverWriteOperationsSuccess(t *testing.T) {
	client := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {
				{ID: "file-old", Name: "old.txt", Kind: "drive#file", Size: "10"},
				{ID: "file-move", Name: "move.txt", Kind: "drive#file", Size: "20"},
				{ID: "file-copy", Name: "copy.txt", Kind: "drive#file", Size: "30"},
				{ID: "file-trash", Name: "trash.txt", Kind: "drive#file", Size: "40"},
				{ID: "folder-target", Name: "Target", Kind: "drive#folder"},
			},
			"folder-target": {},
		},
	}
	driver := NewPikPakDriver(WithPikPakAPIClient(client))
	source := newTestPikPakSource(t)

	created, err := driver.Mkdir(context.Background(), source, "/", "new")
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if created.Path != "/new" || !created.IsDir {
		t.Fatalf("unexpected mkdir entry = %+v", created)
	}

	renamed, err := driver.Rename(context.Background(), source, "/old.txt", "renamed.txt")
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if renamed.Path != "/renamed.txt" || renamed.Name != "renamed.txt" {
		t.Fatalf("unexpected rename entry = %+v", renamed)
	}

	if err := driver.Move(context.Background(), source, "/move.txt", "/Target"); err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if len(client.moves) != 1 || client.moves[0].id != "file-move" || client.moves[0].targetParentID != "folder-target" {
		t.Fatalf("unexpected move calls = %+v", client.moves)
	}
	if err := driver.Copy(context.Background(), source, "/copy.txt", "/Target"); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	if len(client.copies) != 1 || client.copies[0].id != "file-copy" || client.copies[0].targetParentID != "folder-target" {
		t.Fatalf("unexpected copy calls = %+v", client.copies)
	}
	if err := driver.Delete(context.Background(), source, "/trash.txt"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !reflect.DeepEqual(client.trashedIDs, []string{"file-trash"}) {
		t.Fatalf("unexpected trashed ids = %+v", client.trashedIDs)
	}
}

func TestPikPakDriverWriteOperationsNameConflict(t *testing.T) {
	client := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {
				{ID: "file-old", Name: "old.txt", Kind: "drive#file"},
				{ID: "file-existing", Name: "existing.txt", Kind: "drive#file"},
				{ID: "folder-target", Name: "Target", Kind: "drive#folder"},
			},
			"folder-target": {
				{ID: "file-target-old", Name: "old.txt", Kind: "drive#file"},
			},
		},
	}
	driver := NewPikPakDriver(WithPikPakAPIClient(client))
	source := newTestPikPakSource(t)

	assertExist := func(t *testing.T, label string, err error) {
		t.Helper()
		if !errors.Is(err, fs.ErrExist) {
			t.Fatalf("%s expected fs.ErrExist, got %v", label, err)
		}
	}
	_, err := driver.Mkdir(context.Background(), source, "/", "existing.txt")
	assertExist(t, "Mkdir", err)
	_, err = driver.Rename(context.Background(), source, "/old.txt", "existing.txt")
	assertExist(t, "Rename", err)
	assertExist(t, "Move", driver.Move(context.Background(), source, "/old.txt", "/Target"))
	assertExist(t, "Copy", driver.Copy(context.Background(), source, "/old.txt", "/Target"))
}

func TestPikPakDriverWriteOperationsRootProtection(t *testing.T) {
	driver := NewPikPakDriver(WithPikPakAPIClient(&fakePikPakClient{}))
	source := newTestPikPakSource(t)

	if _, err := driver.Rename(context.Background(), source, "/", "root"); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Rename(root) expected os.ErrInvalid, got %v", err)
	}
	if err := driver.Move(context.Background(), source, "/", "/Target"); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Move(root) expected os.ErrInvalid, got %v", err)
	}
	if err := driver.Copy(context.Background(), source, "/", "/Target"); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Copy(root) expected os.ErrInvalid, got %v", err)
	}
	if err := driver.Delete(context.Background(), source, "/"); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Delete(root) expected os.ErrInvalid, got %v", err)
	}
}

func TestPikPakDriverWriteOperationsNotFound(t *testing.T) {
	client := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {
				{ID: "folder-target", Name: "Target", Kind: "drive#folder"},
			},
			"folder-target": {},
		},
	}
	driver := NewPikPakDriver(WithPikPakAPIClient(client))
	source := newTestPikPakSource(t)

	if _, err := driver.Mkdir(context.Background(), source, "/missing", "new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Mkdir(missing parent) expected os.ErrNotExist, got %v", err)
	}
	if _, err := driver.Rename(context.Background(), source, "/missing.txt", "new.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Rename(missing source) expected os.ErrNotExist, got %v", err)
	}
	if err := driver.Move(context.Background(), source, "/missing.txt", "/Target"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Move(missing source) expected os.ErrNotExist, got %v", err)
	}
	if err := driver.Copy(context.Background(), source, "/missing.txt", "/Target"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Copy(missing source) expected os.ErrNotExist, got %v", err)
	}
	if err := driver.Delete(context.Background(), source, "/missing.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Delete(missing source) expected os.ErrNotExist, got %v", err)
	}
}

func TestPikPakDriverWriteOperationsRejectFolderSelfOrDescendantTarget(t *testing.T) {
	client := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {
				{ID: "folder-src", Name: "Folder", Kind: "drive#folder"},
			},
			"folder-src": {
				{ID: "folder-child", Name: "Child", Kind: "drive#folder"},
			},
			"folder-child": {},
		},
	}
	driver := NewPikPakDriver(WithPikPakAPIClient(client))
	source := newTestPikPakSource(t)

	if err := driver.Move(context.Background(), source, "/Folder", "/Folder"); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Move(folder into itself) expected os.ErrInvalid, got %v", err)
	}
	if err := driver.Move(context.Background(), source, "/Folder", "/Folder/Child"); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Move(folder into descendant) expected os.ErrInvalid, got %v", err)
	}
	if err := driver.Copy(context.Background(), source, "/Folder", "/Folder/Child"); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Copy(folder into descendant) expected os.ErrInvalid, got %v", err)
	}
	if len(client.moves) != 0 || len(client.copies) != 0 {
		t.Fatalf("self/descendant folder operations should not call provider, moves=%+v copies=%+v", client.moves, client.copies)
	}
}

func TestPikPakHTTPErrorMappingFileNotFoundAndSanitizedProviderMessage(t *testing.T) {
	if err := mapPikPakHTTPError(http.StatusNotFound, []byte(`{"error_code":0}`)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected 404 to map to file not found, got %v", err)
	}
	if err := mapPikPakHTTPError(http.StatusConflict, []byte(`{"error_code":0}`)); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("expected 409 to map to name conflict, got %v", err)
	}
	if err := mapPikPakHTTPError(http.StatusOK, []byte(`{"error_code":"name_conflict"}`)); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("expected provider name_conflict to map to fs.ErrExist, got %v", err)
	}
	if err := mapPikPakHTTPError(http.StatusBadRequest, []byte(`{"error":"NAME_CONFLICT"}`)); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("expected provider error name_conflict to map to fs.ErrExist, got %v", err)
	}
	if err := mapPikPakHTTPError(http.StatusBadRequest, []byte(`{"error":"invalid_grant"}`)); !errors.Is(err, domainstorage.ErrCloudTokenInvalid) {
		t.Fatalf("expected provider invalid_grant to map to cloud token invalid, got %v", err)
	}

	err := mapPikPakHTTPError(http.StatusBadRequest, []byte(`{"error_code":"9999","error_description":"raw token secret should not leak"}`))
	if !errors.Is(err, domainstorage.ErrCloudProviderUnavailable) {
		t.Fatalf("expected unknown provider code to map to cloud unavailable, got %v", err)
	}
	if strings.Contains(err.Error(), "raw token secret") {
		t.Fatalf("provider details leaked in error message: %v", err)
	}
}

func TestPikPakHTTPClientWriteRequests(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("unexpected authorization header = %q", got)
		}
		seen = append(seen, r.Method+" "+r.URL.EscapedPath())
		var payload map[string]any
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode request body for %s %s error = %v", r.Method, r.URL.Path, err)
			}
		}
		switch r.Method + " " + r.URL.EscapedPath() {
		case "POST /drive/v1/files":
			if payload["kind"] != "drive#folder" || payload["parent_id"] != "root" || payload["name"] != "new" {
				t.Fatalf("unexpected create folder payload = %+v", payload)
			}
			_, _ = w.Write([]byte(`{"id":"folder-new","name":"new","kind":"drive#folder"}`))
		case "PATCH /drive/v1/files/file-1":
			if payload["name"] != "renamed.txt" {
				t.Fatalf("unexpected rename payload = %+v", payload)
			}
			_, _ = w.Write([]byte(`{"id":"file-1","name":"renamed.txt","kind":"drive#file"}`))
		case "POST /drive/v1/files:batchMove", "POST /drive/v1/files:batchCopy":
			if !reflect.DeepEqual(payload["ids"], []any{"file-1"}) {
				t.Fatalf("unexpected batch ids payload = %+v", payload)
			}
			to, ok := payload["to"].(map[string]any)
			if !ok || to["parent_id"] != "folder-target" {
				t.Fatalf("unexpected batch target payload = %+v", payload)
			}
			_, _ = w.Write([]byte(`{}`))
		case "POST /drive/v1/files:batchTrash":
			if !reflect.DeepEqual(payload["ids"], []any{"file-1"}) {
				t.Fatalf("unexpected batch trash payload = %+v", payload)
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	client := NewPikPakHTTPClient(
		WithPikPakHTTPClient(server.Client()),
		WithPikPakBaseURLs(server.URL, server.URL, server.URL),
	)
	session := PikPakSession{AccessToken: "access-token", CaptchaToken: "captcha", DeviceID: "device", UserAgent: "test-agent"}

	if _, err := client.CreateFolder(context.Background(), session, "root", "new"); err != nil {
		t.Fatalf("CreateFolder() error = %v", err)
	}
	if _, err := client.RenameFile(context.Background(), session, "file-1", "renamed.txt"); err != nil {
		t.Fatalf("RenameFile() error = %v", err)
	}
	if err := client.BatchMove(context.Background(), session, []string{"file-1"}, "folder-target"); err != nil {
		t.Fatalf("BatchMove() error = %v", err)
	}
	if err := client.BatchCopy(context.Background(), session, []string{"file-1"}, "folder-target"); err != nil {
		t.Fatalf("BatchCopy() error = %v", err)
	}
	if err := client.BatchTrash(context.Background(), session, []string{"file-1"}); err != nil {
		t.Fatalf("BatchTrash() error = %v", err)
	}

	expected := []string{
		"POST /drive/v1/files",
		"PATCH /drive/v1/files/file-1",
		"POST /drive/v1/files:batchMove",
		"POST /drive/v1/files:batchCopy",
		"POST /drive/v1/files:batchTrash",
	}
	if !reflect.DeepEqual(seen, expected) {
		t.Fatalf("unexpected requests = %v", seen)
	}
}

func newTestPikPakSource(t *testing.T) *entity.StorageSource {
	t.Helper()
	raw, err := (PikPakConfig{
		RootFolderID:     "root",
		Platform:         "web",
		DisableMediaLink: true,
		CacheTTLSeconds:  300,
		DownloadStrategy: "redirect",
		Username:         "user@example.com",
		Password:         "password",
		RefreshToken:     "refresh-0",
		DeviceID:         "device-0",
	}).Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return &entity.StorageSource{
		ID:         10,
		Name:       "PikPak",
		DriverType: PikPakDriverType,
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/pikpak",
		RootPath:   "/",
		ConfigJSON: raw,
	}
}

type fakePikPakClient struct {
	filesByParent map[string][]PikPakFile
	fileDetails   map[string]PikPakFile
	about         *PikPakAbout
	moves         []fakePikPakBatchCall
	copies        []fakePikPakBatchCall
	trashedIDs    []string
}

type fakePikPakBatchCall struct {
	id             string
	targetParentID string
}

func (c *fakePikPakClient) RefreshToken(context.Context, PikPakConfig) (*PikPakAuthToken, error) {
	return &PikPakAuthToken{AccessToken: "access-1", RefreshToken: "refresh-1", UserID: "user-id"}, nil
}

func (c *fakePikPakClient) Login(context.Context, PikPakConfig) (*PikPakAuthToken, error) {
	return &PikPakAuthToken{AccessToken: "access-login", RefreshToken: "refresh-login", UserID: "user-id"}, nil
}

func (c *fakePikPakClient) RefreshCaptcha(context.Context, PikPakConfig, string, string) (*PikPakCaptchaToken, error) {
	return &PikPakCaptchaToken{Token: "captcha-1", ExpiresIn: 300}, nil
}

func (c *fakePikPakClient) ListFiles(_ context.Context, _ PikPakSession, parentID string, _ string) (*PikPakListFilesResponse, error) {
	return &PikPakListFilesResponse{Files: c.filesByParent[parentID]}, nil
}

func (c *fakePikPakClient) GetFile(_ context.Context, _ PikPakSession, fileID string, _ string) (*PikPakFile, error) {
	file, ok := c.fileDetails[fileID]
	if !ok {
		return nil, errors.New("missing file detail")
	}
	return &file, nil
}

func (c *fakePikPakClient) CreateFolder(_ context.Context, _ PikPakSession, parentID string, name string) (*PikPakFile, error) {
	file := PikPakFile{ID: "folder-" + name, Name: name, Kind: "drive#folder", ModifiedTime: "2026-05-04T13:00:00Z"}
	c.filesByParent[parentID] = append(c.filesByParent[parentID], file)
	if c.filesByParent[file.ID] == nil {
		c.filesByParent[file.ID] = []PikPakFile{}
	}
	return &file, nil
}

func (c *fakePikPakClient) RenameFile(_ context.Context, _ PikPakSession, fileID string, name string) (*PikPakFile, error) {
	parentID, index, file, ok := c.findFile(fileID)
	if !ok {
		return nil, os.ErrNotExist
	}
	file.Name = name
	c.filesByParent[parentID][index] = file
	return &file, nil
}

func (c *fakePikPakClient) BatchMove(_ context.Context, _ PikPakSession, ids []string, targetParentID string) error {
	for _, id := range ids {
		parentID, index, file, ok := c.findFile(id)
		if !ok {
			return os.ErrNotExist
		}
		c.filesByParent[parentID] = append(c.filesByParent[parentID][:index], c.filesByParent[parentID][index+1:]...)
		c.filesByParent[targetParentID] = append(c.filesByParent[targetParentID], file)
		c.moves = append(c.moves, fakePikPakBatchCall{id: id, targetParentID: targetParentID})
	}
	return nil
}

func (c *fakePikPakClient) BatchCopy(_ context.Context, _ PikPakSession, ids []string, targetParentID string) error {
	for _, id := range ids {
		_, _, file, ok := c.findFile(id)
		if !ok {
			return os.ErrNotExist
		}
		copied := file
		copied.ID = file.ID + "-copy"
		c.filesByParent[targetParentID] = append(c.filesByParent[targetParentID], copied)
		c.copies = append(c.copies, fakePikPakBatchCall{id: id, targetParentID: targetParentID})
	}
	return nil
}

func (c *fakePikPakClient) BatchTrash(_ context.Context, _ PikPakSession, ids []string) error {
	for _, id := range ids {
		parentID, index, _, ok := c.findFile(id)
		if !ok {
			return os.ErrNotExist
		}
		c.filesByParent[parentID] = append(c.filesByParent[parentID][:index], c.filesByParent[parentID][index+1:]...)
		c.trashedIDs = append(c.trashedIDs, id)
	}
	return nil
}

func (c *fakePikPakClient) About(context.Context, PikPakSession) (*PikPakAbout, error) {
	if c.about == nil {
		return &PikPakAbout{}, nil
	}
	return c.about, nil
}

func (c *fakePikPakClient) findFile(fileID string) (string, int, PikPakFile, bool) {
	for parentID, files := range c.filesByParent {
		for index, file := range files {
			if file.ID == fileID {
				return parentID, index, file, true
			}
		}
	}
	return "", -1, PikPakFile{}, false
}
