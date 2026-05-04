package storage

import (
	"context"
	"errors"
	"net/http"
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

func TestPikPakDriverWriteOperationsUnsupported(t *testing.T) {
	driver := NewPikPakDriver(WithPikPakAPIClient(&fakePikPakClient{}))
	source := newTestPikPakSource(t)

	if _, err := driver.Mkdir(context.Background(), source, "/", "new"); !errors.Is(err, domainstorage.ErrOperationUnsupported) {
		t.Fatalf("Mkdir() expected unsupported, got %v", err)
	}
	if _, err := driver.Rename(context.Background(), source, "/a.txt", "b.txt"); !errors.Is(err, domainstorage.ErrOperationUnsupported) {
		t.Fatalf("Rename() expected unsupported, got %v", err)
	}
	if err := driver.Move(context.Background(), source, "/a.txt", "/b"); !errors.Is(err, domainstorage.ErrOperationUnsupported) {
		t.Fatalf("Move() expected unsupported, got %v", err)
	}
	if err := driver.Copy(context.Background(), source, "/a.txt", "/b"); !errors.Is(err, domainstorage.ErrOperationUnsupported) {
		t.Fatalf("Copy() expected unsupported, got %v", err)
	}
	if err := driver.Delete(context.Background(), source, "/a.txt"); !errors.Is(err, domainstorage.ErrOperationUnsupported) {
		t.Fatalf("Delete() expected unsupported, got %v", err)
	}
}

func TestPikPakHTTPErrorMappingFileNotFoundAndSanitizedProviderMessage(t *testing.T) {
	if err := mapPikPakHTTPError(http.StatusNotFound, []byte(`{"error_code":0}`)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected 404 to map to file not found, got %v", err)
	}

	err := mapPikPakHTTPError(http.StatusBadRequest, []byte(`{"error_code":"9999","error_description":"raw token secret should not leak"}`))
	if !errors.Is(err, domainstorage.ErrCloudProviderUnavailable) {
		t.Fatalf("expected unknown provider code to map to cloud unavailable, got %v", err)
	}
	if strings.Contains(err.Error(), "raw token secret") {
		t.Fatalf("provider details leaked in error message: %v", err)
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

func (c *fakePikPakClient) About(context.Context, PikPakSession) (*PikPakAbout, error) {
	if c.about == nil {
		return &PikPakAbout{}, nil
	}
	return c.about, nil
}
