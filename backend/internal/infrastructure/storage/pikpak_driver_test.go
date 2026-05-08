package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestPikPakDriverPersistsRuntimeSessionConfig(t *testing.T) {
	client := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {},
		},
	}
	var persisted []string
	driver := NewPikPakDriver(
		WithPikPakAPIClient(client),
		WithPikPakRuntimeConfigWriter(func(_ context.Context, source *entity.StorageSource, configJSON string) error {
			if source == nil || source.ID != 10 {
				t.Fatalf("unexpected source passed to runtime writer = %+v", source)
			}
			persisted = append(persisted, configJSON)
			return nil
		}),
	)
	source := newTestPikPakSource(t)

	if err := driver.Test(context.Background(), source); err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	if len(persisted) == 0 {
		t.Fatalf("expected runtime config writer to be called")
	}
	cfg, err := ParsePikPakConfigJSON(persisted[len(persisted)-1])
	if err != nil {
		t.Fatalf("ParsePikPakConfigJSON(persisted) error = %v", err)
	}
	if cfg.RefreshToken != "refresh-1" || cfg.CaptchaToken != "captcha-1" || cfg.DeviceID != "device-0" {
		t.Fatalf("unexpected persisted runtime config = %+v", cfg)
	}
	sourceCfg, err := ParsePikPakConfigJSON(source.ConfigJSON)
	if err != nil {
		t.Fatalf("ParsePikPakConfigJSON(source) error = %v", err)
	}
	if sourceCfg.RefreshToken != cfg.RefreshToken || sourceCfg.CaptchaToken != cfg.CaptchaToken || sourceCfg.DeviceID != cfg.DeviceID {
		t.Fatalf("source ConfigJSON should mirror persisted runtime config, source=%+v persisted=%+v", sourceCfg, cfg)
	}
}

func TestPikPakDriverTestUsesProviderCompatibleAuthContext(t *testing.T) {
	const (
		username = "user@example.com"
		password = " pass-with-edge-space "
	)
	deviceID := GeneratePikPakDeviceID(username, password)
	var seenActions []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAuthContextFailure := func(message string) {
			t.Errorf("%s", message)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error_code":"invalid_token"}`))
		}

		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/shield/captcha/init":
			if got := r.Header.Get("X-Device-ID"); got != deviceID {
				writeAuthContextFailure("captcha init missing provider-compatible X-Device-ID")
				return
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("Decode captcha payload error = %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			action, _ := payload["action"].(string)
			seenActions = append(seenActions, action)
			if payload["device_id"] != deviceID {
				writeAuthContextFailure("captcha init payload used unexpected device_id")
				return
			}
			if action == pikPakDriveListCaptchaAction {
				if got := r.Header.Get("Authorization"); got != "Bearer access-1" {
					writeAuthContextFailure("post-login captcha init missing Authorization")
					return
				}
				_, _ = w.Write([]byte(`{"captcha_token":"captcha-drive","expires_in":300}`))
				return
			}
			if got := r.Header.Get("Authorization"); got != "" {
				writeAuthContextFailure("pre-login captcha init should not send Authorization")
				return
			}
			_, _ = w.Write([]byte(`{"captcha_token":"captcha-login","expires_in":300}`))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/auth/signin":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("Decode login payload error = %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if payload["username"] != username || payload["password"] != password {
				writeAuthContextFailure("login payload did not preserve exact credentials")
				return
			}
			if payload["captcha_token"] != "captcha-login" {
				writeAuthContextFailure("login payload used unexpected captcha token")
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"access-1","refresh_token":"refresh-1","sub":"user-id"}`))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/drive/v1/files":
			if got := r.Header.Get("Authorization"); got != "Bearer access-1" {
				writeAuthContextFailure("drive list missing Authorization")
				return
			}
			if got := r.Header.Get("X-Device-ID"); got != deviceID {
				writeAuthContextFailure("drive list missing provider-compatible X-Device-ID")
				return
			}
			if got := r.Header.Get("X-Captcha-Token"); got != "captcha-drive" {
				writeAuthContextFailure("drive list used unexpected captcha token")
				return
			}
			if got := r.URL.Query().Get("parent_id"); got != "root" {
				t.Errorf("unexpected parent_id = %q", got)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"files":[]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := PikPakConfig{
		RootFolderID:     "root",
		Platform:         "web",
		DisableMediaLink: true,
		CacheTTLSeconds:  300,
		DownloadStrategy: "redirect",
		Username:         username,
		Password:         password,
	}
	raw, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	source := &entity.StorageSource{
		ID:         11,
		Name:       "PikPak",
		DriverType: PikPakDriverType,
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/pikpak",
		RootPath:   "/",
		ConfigJSON: raw,
	}
	client := NewPikPakHTTPClient(
		WithPikPakHTTPClient(server.Client()),
		WithPikPakBaseURLs(server.URL, server.URL, server.URL),
		WithPikPakRetryPolicy(1, 0),
	)
	driver := NewPikPakDriver(WithPikPakAPIClient(client))

	if err := driver.Test(context.Background(), source); err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	expectedActions := []string{"POST:/v1/auth/signin", pikPakDriveListCaptchaAction}
	if !reflect.DeepEqual(seenActions, expectedActions) {
		t.Fatalf("unexpected captcha actions = %v", seenActions)
	}
}

func TestPikPakDriverTestMatchesOpenListRootAndAndroidCaptchaFlow(t *testing.T) {
	const (
		username = "user@example.com"
		password = "password-value"
	)
	deviceID := GeneratePikPakDeviceID(username, password)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeFailure := func(message string) {
			t.Errorf("%s", message)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error_code":"invalid_token"}`))
		}

		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/shield/captcha/init":
			if got := r.Header.Get("X-Device-ID"); got != deviceID {
				writeFailure("captcha init missing Android X-Device-ID")
				return
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("Decode captcha payload error = %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			action, _ := payload["action"].(string)
			switch action {
			case "POST:/v1/auth/signin":
				if !strings.Contains(r.Header.Get("User-Agent"), "usrno/ appname/android-com.pikcloud.pikpak") {
					writeFailure("pre-login Android captcha user-agent should not include user id")
					return
				}
				_, _ = w.Write([]byte(`{"captcha_token":"captcha-login","expires_in":300}`))
			case pikPakDriveListCaptchaAction:
				if got := r.Header.Get("Authorization"); got != "Bearer access-1" {
					writeFailure("post-login captcha init missing Authorization")
					return
				}
				if !strings.Contains(r.Header.Get("User-Agent"), "usrno/ appname/android-com.pikcloud.pikpak") {
					writeFailure("initial post-login Android captcha should still use pre-login user-agent")
					return
				}
				meta, ok := payload["meta"].(map[string]any)
				if !ok || meta["user_id"] != "user-id" || meta["captcha_sign"] == "" {
					writeFailure("post-login captcha meta should include provider user_id and captcha_sign")
					return
				}
				_, _ = w.Write([]byte(`{"captcha_token":"captcha-drive","expires_in":300}`))
			default:
				writeFailure("unexpected captcha action " + action)
			}
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/v1/auth/signin":
			_, _ = w.Write([]byte(`{"access_token":"access-1","refresh_token":"refresh-1","sub":"user-id"}`))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/drive/v1/files":
			if got := r.URL.Query().Get("parent_id"); got != "root" {
				t.Errorf("empty root_folder_id should be sent to provider as parent_id=root, got %q", got)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if _, exists := r.URL.Query()["page_token"]; !exists {
				t.Errorf("first drive list request should include an explicit empty page_token")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if got := r.Header.Get("X-Captcha-Token"); got != "captcha-drive" {
				writeFailure("drive list used unexpected captcha token")
				return
			}
			if !strings.Contains(r.Header.Get("User-Agent"), "usrno/user-id appname/android-com.pikcloud.pikpak") {
				writeFailure("drive list Android user-agent should include user id after post-login captcha")
				return
			}
			_, _ = w.Write([]byte(`{"files":[]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := PikPakConfig{
		Platform:         "android",
		DisableMediaLink: true,
		CacheTTLSeconds:  300,
		DownloadStrategy: "redirect",
		Username:         username,
		Password:         password,
	}
	raw, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	source := &entity.StorageSource{
		ID:         12,
		Name:       "PikPak",
		DriverType: PikPakDriverType,
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/pikpak",
		RootPath:   "/",
		ConfigJSON: raw,
	}
	client := NewPikPakHTTPClient(
		WithPikPakHTTPClient(server.Client()),
		WithPikPakBaseURLs(server.URL, server.URL, server.URL),
		WithPikPakRetryPolicy(1, 0),
	)
	driver := NewPikPakDriver(WithPikPakAPIClient(client))

	if err := driver.Test(context.Background(), source); err != nil {
		t.Fatalf("Test() error = %v", err)
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

func TestPikPakDriverImportFileFastUploadSuccess(t *testing.T) {
	localPath := writeTempPikPakUploadFile(t, "fast-content")
	client := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {
				{ID: "folder-uploads", Name: "Uploads", Kind: "drive#folder"},
			},
			"folder-uploads": {},
		},
		createUploadResponse: &PikPakCreateUploadFileResponse{
			File: &PikPakFile{ID: "file-fast", Name: "fast.txt", Kind: "drive#file", Size: "12", Hash: "FAKE-GCID"},
		},
	}
	hasher := &fakePikPakUploadHasher{hash: "fake-gcid"}
	uploader := &fakePikPakOSSUploader{}
	driver := NewPikPakDriver(
		WithPikPakAPIClient(client),
		WithPikPakUploadHashCalculator(hasher),
		WithPikPakOSSUploader(uploader),
	)

	if err := driver.ImportFile(context.Background(), newTestPikPakSource(t), "/Uploads/fast.txt", localPath); err != nil {
		t.Fatalf("ImportFile(fast) error = %v", err)
	}
	if len(client.createUploadCalls) != 1 {
		t.Fatalf("expected one create upload call, got %+v", client.createUploadCalls)
	}
	call := client.createUploadCalls[0]
	if call.ParentID != "folder-uploads" || call.Name != "fast.txt" || call.Size != int64(len("fast-content")) || call.Hash != "FAKE-GCID" {
		t.Fatalf("unexpected create upload call = %+v", call)
	}
	if len(uploader.calls) != 0 {
		t.Fatalf("fast upload should not call OSS uploader, got %+v", uploader.calls)
	}
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("ImportFile must not remove staging source, stat err=%v", err)
	}
}

func TestPikPakDriverImportFileOSSPutObjectSuccess(t *testing.T) {
	localPath := writeTempPikPakUploadFile(t, "oss-content")
	client := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {
				{ID: "folder-uploads", Name: "Uploads", Kind: "drive#folder"},
			},
			"folder-uploads": {},
		},
		createUploadResponse: &PikPakCreateUploadFileResponse{
			Resumable: &PikPakResumableUpload{
				Provider: "UPLOAD_TYPE_UNKNOWN",
				Params: PikPakOSSUploadParams{
					AccessKeyID:     "ak",
					AccessKeySecret: "sk",
					Bucket:          "bucket",
					Endpoint:        "https://oss.example",
					Key:             "object-key",
					SecurityToken:   "token",
				},
			},
			File: &PikPakFile{ID: "file-oss", Name: "oss.txt", Kind: "drive#file", Size: "11"},
		},
	}
	uploader := &fakePikPakOSSUploader{}
	driver := NewPikPakDriver(
		WithPikPakAPIClient(client),
		WithPikPakUploadHashCalculator(&fakePikPakUploadHasher{hash: "fake-gcid"}),
		WithPikPakOSSUploader(uploader),
	)

	if err := driver.ImportFile(context.Background(), newTestPikPakSource(t), "/Uploads/oss.txt", localPath); err != nil {
		t.Fatalf("ImportFile(oss) error = %v", err)
	}
	if len(uploader.calls) != 1 {
		t.Fatalf("expected one OSS upload call, got %+v", uploader.calls)
	}
	call := uploader.calls[0]
	if call.params.Bucket != "bucket" || call.params.Key != "object-key" || string(call.content) != "oss-content" {
		t.Fatalf("unexpected OSS upload call = %+v", call)
	}
	if call.contentType != "text/plain; charset=utf-8" {
		t.Fatalf("expected text/plain content type, got %q", call.contentType)
	}
}

func TestPikPakDriverImportFileConflictRecursiveParentAndProviderError(t *testing.T) {
	localPath := writeTempPikPakUploadFile(t, "content")
	source := newTestPikPakSource(t)

	conflictClient := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {
				{ID: "file-existing", Name: "existing.txt", Kind: "drive#file"},
			},
		},
	}
	conflictDriver := NewPikPakDriver(
		WithPikPakAPIClient(conflictClient),
		WithPikPakUploadHashCalculator(&fakePikPakUploadHasher{hash: "fake-gcid"}),
		WithPikPakOSSUploader(&fakePikPakOSSUploader{}),
	)
	if err := conflictDriver.ImportFile(context.Background(), source, "/existing.txt", localPath); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("expected fs.ErrExist for same-name conflict, got %v", err)
	}
	if len(conflictClient.createUploadCalls) != 0 {
		t.Fatalf("conflict should not create upload task, got %+v", conflictClient.createUploadCalls)
	}

	recursiveParentClient := &fakePikPakClient{filesByParent: map[string][]PikPakFile{"root": {}}}
	recursiveParentDriver := NewPikPakDriver(
		WithPikPakAPIClient(recursiveParentClient),
		WithPikPakUploadHashCalculator(&fakePikPakUploadHasher{hash: "fake-gcid"}),
		WithPikPakOSSUploader(&fakePikPakOSSUploader{}),
	)
	if err := recursiveParentDriver.ImportFile(context.Background(), source, "/Missing/Nested/file.txt", localPath); err != nil {
		t.Fatalf("expected recursive parent creation, got %v", err)
	}
	if len(recursiveParentClient.createUploadCalls) != 1 ||
		recursiveParentClient.createUploadCalls[0].ParentID != "folder-Nested" ||
		recursiveParentClient.createUploadCalls[0].Name != "file.txt" {
		t.Fatalf("unexpected recursive parent upload calls = %+v", recursiveParentClient.createUploadCalls)
	}

	fileParentDriver := NewPikPakDriver(
		WithPikPakAPIClient(&fakePikPakClient{filesByParent: map[string][]PikPakFile{
			"root": {{ID: "file-parent", Name: "NotAFolder", Kind: "drive#file"}},
		}}),
		WithPikPakUploadHashCalculator(&fakePikPakUploadHasher{hash: "fake-gcid"}),
		WithPikPakOSSUploader(&fakePikPakOSSUploader{}),
	)
	if err := fileParentDriver.ImportFile(context.Background(), source, "/NotAFolder/file.txt", localPath); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected os.ErrInvalid for non-folder parent segment, got %v", err)
	}

	providerErr := domainstorage.NewProviderError(domainstorage.ErrCloudProviderUnavailable, "cloud provider unavailable")
	providerClient := &fakePikPakClient{
		filesByParent:   map[string][]PikPakFile{"root": {}},
		createUploadErr: providerErr,
	}
	providerDriver := NewPikPakDriver(
		WithPikPakAPIClient(providerClient),
		WithPikPakUploadHashCalculator(&fakePikPakUploadHasher{hash: "fake-gcid"}),
		WithPikPakOSSUploader(&fakePikPakOSSUploader{}),
	)
	if err := providerDriver.ImportFile(context.Background(), source, "/provider.txt", localPath); !errors.Is(err, domainstorage.ErrCloudProviderUnavailable) {
		t.Fatalf("expected provider unavailable mapping, got %v", err)
	}
}

func TestPikPakDriverCapabilitiesExposeDirectAndServerUpload(t *testing.T) {
	caps, err := NewPikPakDriver(WithPikPakAPIClient(&fakePikPakClient{})).Capabilities(context.Background(), newTestPikPakSource(t))
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if !caps.CanImportFile || !caps.CanServerUpload || !caps.CanNativeDownload || !caps.CanDirectUpload {
		t.Fatalf("expected stage E native download plus direct/server upload, got %+v", caps)
	}
}

func TestPikPakDriverDirectUploadPlanAndComplete(t *testing.T) {
	expiresAt := "2026-05-05T10:30:00Z"
	client := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {
				{ID: "folder-uploads", Name: "Uploads", Kind: "drive#folder"},
			},
			"folder-uploads": {},
		},
		createUploadResponse: &PikPakCreateUploadFileResponse{
			Resumable: &PikPakResumableUpload{Params: PikPakOSSUploadParams{
				AccessKeyID:     "ak",
				AccessKeySecret: "secret",
				Bucket:          "bucket",
				Endpoint:        "https://oss.example.com",
				Expiration:      expiresAt,
				Key:             "objects/movie.mkv",
				SecurityToken:   "token",
			}},
		},
	}
	driver := NewPikPakDriver(
		WithPikPakAPIClient(client),
		WithPikPakOSSInstructionNow(func() time.Time {
			return time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
		}),
	)
	source := newTestPikPakSource(t)

	plan, err := driver.InitMultipartUpload(context.Background(), source, domainstorage.MultipartUploadRequest{
		VirtualPath: "/Uploads",
		Filename:    "movie.mkv",
		ContentType: "video/x-matroska",
		ContentHash: "gcid:0123456789abcdef0123456789abcdef01234567",
		FileSize:    1234,
	})
	if err != nil {
		t.Fatalf("InitMultipartUpload() error = %v", err)
	}
	if len(client.createUploadCalls) != 1 {
		t.Fatalf("expected one create upload call, got %+v", client.createUploadCalls)
	}
	call := client.createUploadCalls[0]
	if call.ParentID != "folder-uploads" || call.Name != "movie.mkv" || call.Hash != "0123456789ABCDEF0123456789ABCDEF01234567" || call.Size != 1234 {
		t.Fatalf("unexpected create upload call = %+v", call)
	}
	if plan.CompletedEntry != nil || len(plan.PartInstructions) != 1 {
		t.Fatalf("expected one direct OSS instruction, got %+v", plan)
	}
	instruction := plan.PartInstructions[0]
	if instruction.Method != http.MethodPut || instruction.URL != "https://bucket.oss.example.com/objects/movie.mkv" {
		t.Fatalf("unexpected direct instruction = %+v", instruction)
	}
	if instruction.ByteStart != 0 || instruction.ByteEnd != 1233 || instruction.ExpiresAt.Format(time.RFC3339) != expiresAt {
		t.Fatalf("unexpected direct instruction range/expiration = %+v", instruction)
	}
	if instruction.Headers["Content-Type"] != "video/x-matroska" || instruction.Headers["X-Oss-Security-Token"] != "token" || !strings.HasPrefix(instruction.Headers["Authorization"], "OSS ak:") {
		t.Fatalf("unexpected direct headers = %+v", instruction.Headers)
	}

	entry, err := driver.CompleteMultipartUpload(context.Background(), source, plan.State, []domainstorage.CompletedUploadPart{{Index: 0, ETag: "etag"}})
	if err != nil {
		t.Fatalf("CompleteMultipartUpload() error = %v", err)
	}
	if entry.Path != "/Uploads/movie.mkv" || entry.Name != "movie.mkv" || entry.Size != 1234 {
		t.Fatalf("unexpected completed entry = %+v", entry)
	}
}

func TestPikPakDriverDirectUploadWithoutGCIDReturnsUnsupported(t *testing.T) {
	driver := NewPikPakDriver(WithPikPakAPIClient(&fakePikPakClient{}))
	_, err := driver.InitMultipartUpload(context.Background(), newTestPikPakSource(t), domainstorage.MultipartUploadRequest{
		VirtualPath: "/",
		Filename:    "movie.mkv",
		FileSize:    1234,
	})
	if !errors.Is(err, domainstorage.ErrOperationUnsupported) {
		t.Fatalf("expected direct upload without GCID to be unsupported, got %v", err)
	}
}

func TestPikPakDriverNativeDownloadCreateStatusAndCancel(t *testing.T) {
	client := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {
				{ID: "folder-downloads", Name: "Downloads", Kind: "drive#folder"},
			},
			"folder-downloads": {},
		},
		offlineTasks: map[string]PikPakOfflineTask{
			"offline-1": {
				ID:       "offline-1",
				Name:     "episode.mkv",
				Phase:    "PHASE_TYPE_COMPLETE",
				Progress: 1,
			},
		},
	}
	driver := NewPikPakDriver(WithPikPakAPIClient(client))
	source := newTestPikPakSource(t)

	task, err := driver.CreateNativeDownload(context.Background(), source, domainstorage.NativeDownloadRequest{
		URL:            "magnet:?xt=urn:btih:abcdef",
		TargetDirPath:  "/Downloads",
		TargetFilename: "episode.mkv",
	})
	if err != nil {
		t.Fatalf("CreateNativeDownload() error = %v", err)
	}
	if task.ExternalID != "offline-1" || task.DisplayName != "episode.mkv" || task.ProgressPercent == nil || *task.ProgressPercent != 0 {
		t.Fatalf("unexpected native task = %+v", task)
	}
	if len(client.createOfflineCalls) != 1 {
		t.Fatalf("expected one native create call, got %+v", client.createOfflineCalls)
	}
	call := client.createOfflineCalls[0]
	if call.ParentID != "folder-downloads" || call.URL != "magnet:?xt=urn:btih:abcdef" || call.Name != "episode.mkv" {
		t.Fatalf("unexpected native create call = %+v", call)
	}
	unnamedTask, err := driver.CreateNativeDownload(context.Background(), source, domainstorage.NativeDownloadRequest{
		URL:           "https://example.com/movie.mkv",
		TargetDirPath: "/Downloads",
	})
	if err != nil {
		t.Fatalf("CreateNativeDownload(unnamed) error = %v", err)
	}
	if unnamedTask.DisplayName != "" {
		t.Fatalf("native task should not synthesize display name from magnet URL, got %+v", unnamedTask)
	}
	client.offlineTasks[unnamedTask.ExternalID] = PikPakOfflineTask{
		ID:       unnamedTask.ExternalID,
		Name:     "https://example.com/movie.mkv",
		Phase:    "PHASE_TYPE_COMPLETE",
		Progress: 1,
	}
	unnamedStatus, err := driver.GetNativeDownloadStatus(context.Background(), source, unnamedTask.ExternalID)
	if err != nil {
		t.Fatalf("GetNativeDownloadStatus(unnamed) error = %v", err)
	}
	if unnamedStatus.Status != "completed" || unnamedStatus.DisplayName != "" {
		t.Fatalf("native completed status without provider filename should keep empty display name, got %+v", unnamedStatus)
	}
	client.offlineTasks[task.ExternalID] = PikPakOfflineTask{
		ID:       task.ExternalID,
		Name:     "episode.mkv",
		Phase:    "PHASE_TYPE_COMPLETE",
		Progress: 1,
	}

	status, err := driver.GetNativeDownloadStatus(context.Background(), source, "offline-1")
	if err != nil {
		t.Fatalf("GetNativeDownloadStatus() error = %v", err)
	}
	if status.Status != "completed" || status.ProgressPercent == nil || *status.ProgressPercent != 100 {
		t.Fatalf("unexpected native status = %+v", status)
	}
	client.offlineTasks[task.ExternalID] = PikPakOfflineTask{
		ID:       task.ExternalID,
		Name:     "episode.mkv",
		Phase:    "PHASE_TYPE_ERROR",
		Message:  `provider payload token=secret /tmp/raw.json`,
		Progress: 0.5,
	}
	status, err = driver.GetNativeDownloadStatus(context.Background(), source, "offline-1")
	if err != nil {
		t.Fatalf("GetNativeDownloadStatus(error) error = %v", err)
	}
	if status.Status != "failed" ||
		status.ErrorMessage == nil ||
		*status.ErrorMessage != "cloud provider task failed" ||
		strings.Contains(*status.ErrorMessage, "secret") ||
		strings.Contains(*status.ErrorMessage, "payload") {
		t.Fatalf("native status should sanitize provider task message, got %+v", status)
	}

	if err := driver.CancelNativeDownload(context.Background(), source, "offline-1", true); err != nil {
		t.Fatalf("CancelNativeDownload() error = %v", err)
	}
	if !reflect.DeepEqual(client.deletedOfflineTasks, []string{"offline-1"}) || !reflect.DeepEqual(client.deleteOfflineFiles, []bool{true}) {
		t.Fatalf("unexpected native cancel calls = ids=%v delete=%v", client.deletedOfflineTasks, client.deleteOfflineFiles)
	}
	if err := driver.PauseNativeDownload(context.Background(), source, "offline-1"); !errors.Is(err, domainstorage.ErrOperationUnsupported) {
		t.Fatalf("expected pause unsupported, got %v", err)
	}
}

func TestPikPakDriverPathCacheAvoidsRepeatedResolveAndInvalidatesOnWrite(t *testing.T) {
	client := &fakePikPakClient{
		filesByParent: map[string][]PikPakFile{
			"root": {
				{ID: "folder-a", Name: "A", Kind: "drive#folder"},
			},
			"folder-a": {
				{ID: "file-old", Name: "old.txt", Kind: "drive#file", Size: "3"},
			},
		},
	}
	driver := NewPikPakDriver(WithPikPakAPIClient(client))
	source := newTestPikPakSource(t)

	if _, err := driver.Stat(context.Background(), source, "/A/old.txt"); err != nil {
		t.Fatalf("Stat(first) error = %v", err)
	}
	firstResolveCalls := len(client.listCalls)
	if firstResolveCalls != 2 {
		t.Fatalf("expected first resolve to list root and /A, got calls=%v", client.listCalls)
	}
	if _, err := driver.Stat(context.Background(), source, "/A/old.txt"); err != nil {
		t.Fatalf("Stat(cached) error = %v", err)
	}
	if len(client.listCalls) != firstResolveCalls {
		t.Fatalf("expected cached Stat to avoid extra ListFiles, calls=%v", client.listCalls)
	}

	if _, err := driver.Rename(context.Background(), source, "/A/old.txt", "renamed.txt"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	afterRenameCalls := len(client.listCalls)
	if _, err := driver.Stat(context.Background(), source, "/A/renamed.txt"); err != nil {
		t.Fatalf("Stat(after rename) error = %v", err)
	}
	if len(client.listCalls) <= afterRenameCalls {
		t.Fatalf("expected write invalidation to force provider list after rename, calls=%v", client.listCalls)
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
	authFlowErr := mapPikPakHTTPErrorForRequest(http.StatusNotFound, []byte(`{"error_code":"resource_not_found","error_description":"resource not found"}`), DefaultPikPakUserBaseURL+"/v1/shield/captcha/init")
	if !errors.Is(authFlowErr, domainstorage.ErrCloudCaptchaRequired) {
		t.Fatalf("expected auth-flow provider 404 to map to captcha required, got %v", authFlowErr)
	}
	var authFlowProviderErr *domainstorage.ProviderError
	if !errors.As(authFlowErr, &authFlowProviderErr) || authFlowProviderErr.ProviderCode != "resource_not_found" {
		t.Fatalf("expected auth-flow provider code preserved, got %+v / %v", authFlowProviderErr, authFlowErr)
	}
	authFlowErrWithURL := mapPikPakHTTPErrorForRequest(http.StatusNotFound, []byte(`{"error_code":"resource_not_found","verification_url":"https://verify.example/resource"}`), DefaultPikPakUserBaseURL+"/v1/shield/captcha/init")
	authFlowProviderErr = nil
	if !errors.As(authFlowErrWithURL, &authFlowProviderErr) || authFlowProviderErr.ProviderCode != "resource_not_found" || authFlowProviderErr.VerificationURL != "https://verify.example/resource" {
		t.Fatalf("expected auth-flow resource_not_found verification URL/code preserved, got %+v / %v", authFlowProviderErr, authFlowErrWithURL)
	}
	rootListErr := mapPikPakHTTPErrorForRequest(http.StatusNotFound, []byte(`{"error_code":"resource_not_found","error_description":"resource not found"}`), DefaultPikPakDriveBaseURL+"/drive/v1/files?parent_id=root")
	if !errors.Is(rootListErr, domainstorage.ErrCloudCaptchaRequired) {
		t.Fatalf("expected root list provider 404 to map to captcha required, got %v", rootListErr)
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
	regionErr := mapPikPakHTTPError(http.StatusBadRequest, []byte(`{"error":"invalid_grant","error_code":4126,"error_description":"AccessProhibited","details":{"reason":"PROHIBITED:CN","message":"Sorry, PikPak is not available in your region (Mainland China)"}}`))
	if !errors.Is(regionErr, domainstorage.ErrCloudRegionBlocked) {
		t.Fatalf("expected provider AccessProhibited to map to cloud region blocked, got %v", regionErr)
	}
	if strings.Contains(regionErr.Error(), "Mainland China") || strings.Contains(regionErr.Error(), "PROHIBITED") {
		t.Fatalf("region blocked provider details leaked in error message: %v", regionErr)
	}
	regionErr = mapPikPakHTTPError(http.StatusForbidden, []byte(`{"error_description":"AccessProhibited","details":"Sorry, PikPak is not available in your region"}`))
	if !errors.Is(regionErr, domainstorage.ErrCloudRegionBlocked) {
		t.Fatalf("expected region-block payload without explicit error_code to map to cloud region blocked, got %v", regionErr)
	}

	captchaErr := mapPikPakHTTPError(http.StatusBadRequest, []byte(`{"error_code":"captcha_required","url":"https://verify.example/captcha"}`))
	var providerErr *domainstorage.ProviderError
	if !errors.Is(captchaErr, domainstorage.ErrCloudCaptchaRequired) || !errors.As(captchaErr, &providerErr) {
		t.Fatalf("expected captcha payload to map to provider captcha error, got %v", captchaErr)
	}
	if providerErr.VerificationURL != "https://verify.example/captcha" || providerErr.ProviderCode != "captcha_required" {
		t.Fatalf("expected captcha verification url/code to be preserved, got %+v", providerErr)
	}

	captchaErr = mapPikPakHTTPError(http.StatusBadRequest, []byte(`{"error":"verification_required","details":{"verification_url":"https://verify.example/nested"}}`))
	providerErr = nil
	if !errors.As(captchaErr, &providerErr) || providerErr.VerificationURL != "https://verify.example/nested" {
		t.Fatalf("expected nested captcha verification url to be preserved, got %v / %+v", captchaErr, providerErr)
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
			if uploadType, _ := payload["upload_type"].(string); uploadType == "UPLOAD_TYPE_RESUMABLE" {
				if payload["kind"] != "drive#file" || payload["parent_id"] != "root" || payload["name"] != "upload.bin" || payload["hash"] != "FAKE-GCID" || payload["size"] != float64(123) {
					t.Fatalf("unexpected create upload payload = %+v", payload)
				}
				objProvider, ok := payload["objProvider"].(map[string]any)
				if !ok || objProvider["provider"] != "UPLOAD_TYPE_UNKNOWN" {
					t.Fatalf("unexpected upload provider payload = %+v", payload)
				}
				_, _ = w.Write([]byte(`{"upload_type":"UPLOAD_TYPE_RESUMABLE","resumable":{"provider":"oss","params":{"access_key_id":"ak","access_key_secret":"sk","bucket":"bucket","endpoint":"https://oss.example","key":"object.bin"}}}`))
				return
			}
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
	if upload, err := client.CreateUploadFile(context.Background(), session, PikPakCreateUploadFileRequest{
		ParentID: "root",
		Name:     "upload.bin",
		Size:     123,
		Hash:     "FAKE-GCID",
	}); err != nil {
		t.Fatalf("CreateUploadFile() error = %v", err)
	} else if upload == nil || upload.Resumable == nil || upload.Resumable.Params.Bucket != "bucket" {
		t.Fatalf("unexpected CreateUploadFile response = %+v", upload)
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

func TestPikPakHTTPClientRetriesTransientWriteRequest(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() attempt %d error = %v", attempts, err)
		}
		if payload["name"] != "retry-folder" {
			t.Fatalf("unexpected retry payload = %+v", payload)
		}
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error_code":"provider_busy"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"folder-retry","name":"retry-folder","kind":"drive#folder"}`))
	}))
	defer server.Close()

	var delays []time.Duration
	client := NewPikPakHTTPClient(
		WithPikPakHTTPClient(server.Client()),
		WithPikPakBaseURLs(server.URL, server.URL, server.URL),
		WithPikPakRetryPolicy(3, time.Second),
		WithPikPakRetrySleeper(func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		}),
	)

	file, err := client.CreateFolder(context.Background(), PikPakSession{AccessToken: "access-token"}, "root", "retry-folder")
	if err != nil {
		t.Fatalf("CreateFolder() error = %v", err)
	}
	if file.ID != "folder-retry" || attempts != 3 {
		t.Fatalf("expected retry success on third attempt, file=%+v attempts=%d", file, attempts)
	}
	if !reflect.DeepEqual(delays, []time.Duration{time.Second, 2 * time.Second}) {
		t.Fatalf("unexpected retry delays = %v", delays)
	}
}

func TestPikPakHTTPClientDoesNotRetryTokenInvalid(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error_code":"invalid_token"}`))
	}))
	defer server.Close()

	client := NewPikPakHTTPClient(
		WithPikPakHTTPClient(server.Client()),
		WithPikPakBaseURLs(server.URL, server.URL, server.URL),
		WithPikPakRetryPolicy(3, 0),
		WithPikPakRetrySleeper(func(context.Context, time.Duration) error {
			t.Fatalf("token invalid must not retry")
			return nil
		}),
	)
	_, err := client.About(context.Background(), PikPakSession{AccessToken: "expired"})
	if !errors.Is(err, domainstorage.ErrCloudTokenInvalid) {
		t.Fatalf("expected token invalid, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected one unauthorized attempt, got %d", attempts)
	}
}

func TestPikPakHTTPClientRateLimitKeepsRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error_code":"too_many_requests"}`))
	}))
	defer server.Close()

	client := NewPikPakHTTPClient(
		WithPikPakHTTPClient(server.Client()),
		WithPikPakBaseURLs(server.URL, server.URL, server.URL),
		WithPikPakRetryPolicy(1, 0),
	)
	_, err := client.About(context.Background(), PikPakSession{AccessToken: "access-token"})
	if !errors.Is(err, domainstorage.ErrCloudRateLimited) {
		t.Fatalf("expected rate limited, got %v", err)
	}
	var providerErr *domainstorage.ProviderError
	if !errors.As(err, &providerErr) || providerErr.RetryAfterSeconds != 7 {
		t.Fatalf("expected retry_after_seconds=7, got %#v", err)
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
	filesByParent        map[string][]PikPakFile
	fileDetails          map[string]PikPakFile
	about                *PikPakAbout
	createUploadResponse *PikPakCreateUploadFileResponse
	createUploadErr      error
	createUploadCalls    []PikPakCreateUploadFileRequest
	offlineTasks         map[string]PikPakOfflineTask
	createOfflineCalls   []PikPakCreateOfflineDownloadRequest
	deletedOfflineTasks  []string
	deleteOfflineFiles   []bool
	listCalls            []string
	moves                []fakePikPakBatchCall
	copies               []fakePikPakBatchCall
	trashedIDs           []string
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

func (c *fakePikPakClient) RefreshCaptcha(context.Context, PikPakConfig, string, string, string) (*PikPakCaptchaToken, error) {
	return &PikPakCaptchaToken{Token: "captcha-1", ExpiresIn: 300}, nil
}

func (c *fakePikPakClient) ListFiles(_ context.Context, _ PikPakSession, parentID string, _ string) (*PikPakListFilesResponse, error) {
	c.listCalls = append(c.listCalls, parentID)
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

func (c *fakePikPakClient) CreateUploadFile(_ context.Context, _ PikPakSession, req PikPakCreateUploadFileRequest) (*PikPakCreateUploadFileResponse, error) {
	c.createUploadCalls = append(c.createUploadCalls, req)
	if c.createUploadErr != nil {
		return nil, c.createUploadErr
	}
	if c.createUploadResponse != nil {
		if c.createUploadResponse.File != nil {
			c.filesByParent[req.ParentID] = append(c.filesByParent[req.ParentID], *c.createUploadResponse.File)
		}
		return c.createUploadResponse, nil
	}
	file := PikPakFile{ID: "file-" + req.Name, Name: req.Name, Kind: "drive#file", Size: strconv.FormatInt(req.Size, 10), Hash: req.Hash}
	c.filesByParent[req.ParentID] = append(c.filesByParent[req.ParentID], file)
	return &PikPakCreateUploadFileResponse{File: &file}, nil
}

func (c *fakePikPakClient) CreateOfflineDownload(_ context.Context, _ PikPakSession, req PikPakCreateOfflineDownloadRequest) (*PikPakOfflineTask, error) {
	c.createOfflineCalls = append(c.createOfflineCalls, req)
	if c.offlineTasks == nil {
		c.offlineTasks = make(map[string]PikPakOfflineTask)
	}
	id := "offline-" + strconv.Itoa(len(c.createOfflineCalls))
	task := PikPakOfflineTask{
		ID:       id,
		Name:     req.Name,
		Phase:    "PHASE_TYPE_PENDING",
		Progress: 0,
	}
	c.offlineTasks[id] = task
	return &task, nil
}

func (c *fakePikPakClient) GetOfflineDownloadTask(_ context.Context, _ PikPakSession, taskID string) (*PikPakOfflineTask, error) {
	task, ok := c.offlineTasks[taskID]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &task, nil
}

func (c *fakePikPakClient) DeleteOfflineDownloadTasks(_ context.Context, _ PikPakSession, taskIDs []string, deleteFiles bool) error {
	for _, id := range taskIDs {
		c.deletedOfflineTasks = append(c.deletedOfflineTasks, id)
		c.deleteOfflineFiles = append(c.deleteOfflineFiles, deleteFiles)
	}
	return nil
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

type fakePikPakUploadHasher struct {
	hash string
	err  error
}

func (h *fakePikPakUploadHasher) HashFile(context.Context, string) (string, error) {
	if h.err != nil {
		return "", h.err
	}
	return h.hash, nil
}

type fakePikPakOSSUploader struct {
	calls []fakePikPakOSSUploadCall
	err   error
}

type fakePikPakOSSUploadCall struct {
	params      PikPakOSSUploadParams
	localPath   string
	contentType string
	content     []byte
}

func (u *fakePikPakOSSUploader) PutObject(_ context.Context, params PikPakOSSUploadParams, localPath string, contentType string) error {
	if u.err != nil {
		return u.err
	}
	content, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	u.calls = append(u.calls, fakePikPakOSSUploadCall{
		params:      params,
		localPath:   localPath,
		contentType: contentType,
		content:     content,
	})
	return nil
}

func writeTempPikPakUploadFile(t *testing.T, content string) string {
	t.Helper()
	localPath := path.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(localPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return localPath
}
