package service

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/persistence/gorm"
	"yunxia/internal/infrastructure/security"
	infraStorage "yunxia/internal/infrastructure/storage"
)

func TestSourceServicePikPakCreateTestDetailSecretRetentionAndVFSList(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	configRepo := gorm.NewSystemConfigRepository(db)
	pikpakClient := &sourceServiceFakePikPakClient{
		filesByParent: map[string][]infraStorage.PikPakFile{
			"root": {
				{ID: "file-remote", Name: "remote.txt", Kind: "drive#file", Size: "12", ModifiedTime: "2026-05-04T10:00:00Z"},
			},
		},
	}
	pikpakDriver := infraStorage.NewPikPakDriver(infraStorage.WithPikPakAPIClient(pikpakClient))
	registry := NewStorageDriverRegistry(DriverBundle{
		Type:         infraStorage.PikPakDriverType,
		DisplayName:  "PikPak",
		Config:       NewPikPakSourceConfigCodec(),
		Probe:        pikpakDriver,
		File:         pikpakDriver,
		Capacity:     pikpakDriver,
		Capabilities: pikpakDriver,
	})
	sourceSvc := NewSourceService(sourceRepo, configRepo, registry.SourceServiceOptions()...)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       1,
		RoleKey:      permission.RoleSuperAdmin,
		Status:       permission.StatusActive,
		Capabilities: permission.AllCapabilities(),
	})

	req := appdto.SourceUpsertRequest{
		Name:            "PikPak 媒体库",
		DriverType:      infraStorage.PikPakDriverType,
		IsEnabled:       true,
		IsWebDAVExposed: false,
		WebDAVReadOnly:  true,
		MountPath:       "/pikpak",
		RootPath:        "/",
		SortOrder:       20,
		Config: map[string]any{
			"root_folder_id":     "root",
			"platform":           "web",
			"disable_media_link": true,
			"cache_ttl_seconds":  300,
			"download_strategy":  "redirect",
		},
		SecretPatch: map[string]any{
			"username":      "user@example.com",
			"password":      "password-value",
			"refresh_token": "refresh-0",
		},
	}

	testResp, err := sourceSvc.Test(ctx, req)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	if !testResp.Reachable {
		t.Fatalf("expected reachable test response")
	}

	created, err := sourceSvc.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.DriverType != infraStorage.PikPakDriverType || created.MountPath != "/pikpak" {
		t.Fatalf("unexpected created source = %+v", created)
	}

	stored, err := sourceRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	cfg, err := infraStorage.ParsePikPakConfigJSON(stored.ConfigJSON)
	if err != nil {
		t.Fatalf("ParsePikPakConfigJSON() error = %v", err)
	}
	if cfg.RefreshToken != "refresh-1" || cfg.CaptchaToken != "captcha-1" || cfg.DeviceID == "" {
		t.Fatalf("expected probe/session to write runtime config into created entity, got %+v", cfg)
	}

	limitedCtx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       2,
		RoleKey:      permission.RoleAdmin,
		Status:       permission.StatusActive,
		Capabilities: []string{permission.CapabilitySourceRead},
	})
	detail, err := sourceSvc.Get(limitedCtx, created.ID)
	if err != nil {
		t.Fatalf("Get(limited) error = %v", err)
	}
	if _, exists := detail.Config["password"]; exists {
		t.Fatalf("expected password to be hidden in public config: %+v", detail.Config)
	}
	if !detail.SecretFields["username"].Configured || detail.SecretFields["username"].Masked != "user****" {
		t.Fatalf("unexpected username mask = %+v", detail.SecretFields["username"])
	}
	secretDetail, err := sourceSvc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get(secret) error = %v", err)
	}
	if secretDetail.Config["password"] != "password-value" || secretDetail.Config["refresh_token"] != "refresh-1" {
		t.Fatalf("expected source.secret.read to expose plaintext secrets, got %+v", secretDetail.Config)
	}

	_, err = sourceSvc.Update(ctx, created.ID, appdto.SourceUpsertRequest{
		Name:            "PikPak 媒体库",
		IsEnabled:       true,
		IsWebDAVExposed: false,
		WebDAVReadOnly:  true,
		MountPath:       "/pikpak",
		RootPath:        "/",
		SortOrder:       21,
		Config: map[string]any{
			"root_folder_id":     "root",
			"platform":           "pc",
			"disable_media_link": true,
			"cache_ttl_seconds":  600,
			"download_strategy":  "redirect",
		},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	updated, err := sourceRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("FindByID(updated) error = %v", err)
	}
	updatedCfg, err := infraStorage.ParsePikPakConfigJSON(updated.ConfigJSON)
	if err != nil {
		t.Fatalf("ParsePikPakConfigJSON(updated) error = %v", err)
	}
	if updatedCfg.Username != "user@example.com" || updatedCfg.Password != "password-value" || updatedCfg.RefreshToken == "" {
		t.Fatalf("expected update to retain secrets, got %+v", updatedCfg)
	}
	if updatedCfg.Platform != "pc" || updatedCfg.CacheTTLSeconds != 600 {
		t.Fatalf("expected public config update, got %+v", updatedCfg)
	}

	vfsSvc := NewVFSService(sourceRepo, registry.VFSServiceOptions()...)
	vfsList, err := vfsSvc.List(context.Background(), "/pikpak")
	if err != nil {
		t.Fatalf("VFS List(/pikpak) error = %v", err)
	}
	if got := collectVFSNames(vfsList.Items); !reflect.DeepEqual(got, []string{"remote.txt"}) {
		t.Fatalf("expected remote file through VFS, got %v", got)
	}
	if len(vfsList.Items) != 1 || !vfsList.Items[0].CanDelete || !vfsList.Items[0].CanDownload {
		t.Fatalf("expected PikPak VFS item to expose stage C delete/download capabilities, got %+v", vfsList.Items)
	}
}

func TestPikPakSourceRootPathMustStaySlash(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	configRepo := gorm.NewSystemConfigRepository(db)
	pikpakDriver := infraStorage.NewPikPakDriver(infraStorage.WithPikPakAPIClient(&sourceServiceFakePikPakClient{}))
	sourceSvc := NewSourceService(
		sourceRepo,
		configRepo,
		WithSourceConfigCodec(NewPikPakSourceConfigCodec()),
		WithSourceDriverProbe(infraStorage.PikPakDriverType, pikpakDriver),
	)
	_, err := sourceSvc.Test(context.Background(), appdto.SourceUpsertRequest{
		Name:       "bad pikpak",
		DriverType: infraStorage.PikPakDriverType,
		RootPath:   "/remote",
		Config: map[string]any{
			"root_folder_id": "root",
		},
		SecretPatch: map[string]any{
			"refresh_token": "refresh-0",
		},
	})
	if !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("expected ErrConfigInvalid for non-root root_path, got %v", err)
	}
}

func TestTaskCreateToPikPakWithoutImportDriverReturnsOperationUnsupported(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)
	raw, err := (infraStorage.PikPakConfig{
		RootFolderID:     "root",
		Platform:         "web",
		DisableMediaLink: true,
		CacheTTLSeconds:  300,
		DownloadStrategy: "redirect",
		RefreshToken:     "refresh-0",
		DeviceID:         "device-0",
	}).Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "PikPak",
		DriverType: infraStorage.PikPakDriverType,
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/pikpak",
		RootPath:   "/",
		ConfigJSON: raw,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("Create(source) error = %v", err)
	}

	taskSvc := NewTaskService(taskRepo, sourceRepo, nil)
	_, err = taskSvc.Create(context.Background(), appdto.CreateTaskRequest{
		URL:      "https://example.com/movie.mkv",
		SourceID: source.ID,
		SavePath: "/",
	})
	if !errors.Is(err, ErrSourceOperationUnsupported) {
		t.Fatalf("expected SOURCE_OPERATION_UNSUPPORTED for PikPak task target, got %v", err)
	}
}

func TestUploadInitToPikPakWithoutImportDriverReturnsOperationUnsupported(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	uploadRepo := gorm.NewUploadSessionRepository(db)
	raw, err := (infraStorage.PikPakConfig{
		RootFolderID:     "root",
		Platform:         "web",
		DisableMediaLink: true,
		CacheTTLSeconds:  300,
		DownloadStrategy: "redirect",
		RefreshToken:     "refresh-0",
		DeviceID:         "device-0",
	}).Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "PikPak",
		DriverType: infraStorage.PikPakDriverType,
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/pikpak",
		RootPath:   "/",
		ConfigJSON: raw,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("Create(source) error = %v", err)
	}

	uploadSvc := NewUploadService(sourceRepo, uploadRepo, DefaultSystemOptions())
	_, err = uploadSvc.Init(context.Background(), 1, appdto.UploadInitRequest{
		SourceID: source.ID,
		Path:     "/",
		Filename: "new.txt",
		FileSize: 1,
		FileHash: "hash",
	})
	if !errors.Is(err, ErrSourceOperationUnsupported) {
		t.Fatalf("expected SOURCE_OPERATION_UNSUPPORTED for PikPak upload target, got %v", err)
	}
}

func TestPikPakStageDUploadUsesServerChunkImport(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	uploadRepo := gorm.NewUploadSessionRepository(db)
	raw, err := (infraStorage.PikPakConfig{
		RootFolderID:     "root",
		Platform:         "web",
		DisableMediaLink: true,
		CacheTTLSeconds:  300,
		DownloadStrategy: "redirect",
		RefreshToken:     "refresh-0",
		DeviceID:         "device-0",
	}).Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "PikPak",
		DriverType: infraStorage.PikPakDriverType,
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/pikpak",
		RootPath:   "/",
		ConfigJSON: raw,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("Create(source) error = %v", err)
	}

	importer := &recordingTaskImportDriver{}
	options := DefaultSystemOptions()
	options.TempDir = filepath.Join(t.TempDir(), "upload-temp")
	options.DefaultChunkSize = 4
	options.MaxUploadSize = 1024
	uploadSvc := NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		WithUploadImportDriver(infraStorage.PikPakDriverType, importer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       7,
		RoleKey:      permission.RoleUser,
		Status:       permission.StatusActive,
		Capabilities: []string{},
	})

	initResp, err := uploadSvc.Init(ctx, 7, appdto.UploadInitRequest{
		SourceID: source.ID,
		Path:     "/Anime",
		Filename: "episode.mkv",
		FileSize: int64(len("video-body")),
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if initResp.Transport == nil ||
		initResp.Transport.Mode != "server_chunk" ||
		initResp.Transport.DriverType != infraStorage.PikPakDriverType ||
		len(initResp.PartInstructions) != 0 {
		t.Fatalf("expected PikPak server_chunk transport without direct parts, got %+v", initResp)
	}

	chunks := [][]byte{[]byte("vide"), []byte("o-bo"), []byte("dy")}
	for i, chunk := range chunks {
		if _, err := uploadSvc.UploadChunk(ctx, initResp.Upload.UploadID, i, chunk); err != nil {
			t.Fatalf("UploadChunk(%d) error = %v", i, err)
		}
	}

	finishResp, err := uploadSvc.Finish(ctx, appdto.UploadFinishRequest{UploadID: initResp.Upload.UploadID})
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if finishResp.File.Path != "/Anime/episode.mkv" || finishResp.File.Size != int64(len("video-body")) {
		t.Fatalf("unexpected finish file = %+v", finishResp.File)
	}
	if len(importer.calls) != 1 {
		t.Fatalf("expected one PikPak import call, got %+v", importer.calls)
	}
	call := importer.calls[0]
	if call.sourceID != source.ID || call.targetPath != "/Anime/episode.mkv" || string(call.content) != "video-body" {
		t.Fatalf("unexpected PikPak upload import call = %+v", call)
	}
	if _, err := uploadRepo.FindByID(context.Background(), initResp.Upload.UploadID); !errors.Is(err, domainrepo.ErrNotFound) {
		t.Fatalf("expected upload session to be deleted after finish, err=%v", err)
	}
}

func TestPikPakStageDTaskDownloadImportsAndCleansStaging(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)
	raw, err := (infraStorage.PikPakConfig{
		RootFolderID:     "root",
		Platform:         "web",
		DisableMediaLink: true,
		CacheTTLSeconds:  300,
		DownloadStrategy: "redirect",
		RefreshToken:     "refresh-0",
		DeviceID:         "device-0",
	}).Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "PikPak",
		DriverType: infraStorage.PikPakDriverType,
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/pikpak",
		RootPath:   "/",
		ConfigJSON: raw,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("Create(source) error = %v", err)
	}

	downloader := &completedWritingDownloader{
		filename: "movie.mkv",
		content:  []byte("downloaded-video"),
	}
	importer := &recordingTaskImportDriver{}
	stagingRoot := filepath.Join(t.TempDir(), "task-staging")
	taskSvc := NewTaskService(
		taskRepo,
		sourceRepo,
		downloader,
		WithTaskStagingDir(stagingRoot),
		WithTaskImportDriver(infraStorage.PikPakDriverType, importer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       42,
		RoleKey:      permission.RoleUser,
		Status:       permission.StatusActive,
		Capabilities: []string{},
	})

	created, err := taskSvc.Create(ctx, appdto.CreateTaskRequest{
		Type:     "download",
		URL:      "https://example.com/movie.mkv",
		SourceID: source.ID,
		SavePath: "/Downloads",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	storedBefore, err := taskRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("FindByID(before) error = %v", err)
	}
	if storedBefore.StagingDir == "" || filepath.Dir(storedBefore.StagingDir) != stagingRoot {
		t.Fatalf("expected PikPak task staging under %q, got %q", stagingRoot, storedBefore.StagingDir)
	}

	got, err := taskSvc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" || got.ErrorMessage != nil {
		t.Fatalf("expected completed PikPak task without error, got %+v", got)
	}
	if len(importer.calls) != 1 {
		t.Fatalf("expected one PikPak task import call, got %+v", importer.calls)
	}
	call := importer.calls[0]
	if call.sourceID != source.ID || call.targetPath != "/Downloads/movie.mkv" || string(call.content) != "downloaded-video" {
		t.Fatalf("unexpected PikPak task import call = %+v", call)
	}
	if _, err := os.Stat(storedBefore.StagingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected staging dir removed after PikPak import, stat err=%v", err)
	}
	storedAfter, err := taskRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("FindByID(after) error = %v", err)
	}
	if storedAfter.StagingDir != "" {
		t.Fatalf("expected persisted staging dir to be cleared, got %q", storedAfter.StagingDir)
	}
}

func TestPikPakStageCFileAndVFSWritesUseDriverCapabilities(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	raw, err := (infraStorage.PikPakConfig{
		RootFolderID:     "root",
		Platform:         "web",
		DisableMediaLink: true,
		CacheTTLSeconds:  300,
		DownloadStrategy: "redirect",
		RefreshToken:     "refresh-0",
		DeviceID:         "device-0",
	}).Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "PikPak",
		DriverType: infraStorage.PikPakDriverType,
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/pikpak",
		RootPath:   "/",
		ConfigJSON: raw,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("Create(source) error = %v", err)
	}

	driver := &pikPakStageCFileDriver{
		entriesByPath: map[string][]StorageEntry{
			"/": {
				{Name: "remote.txt", Path: "/remote.txt", IsDir: false},
			},
		},
	}
	capabilities := capabilityProviderStub{capabilities: StorageCapabilities{
		CanList:          true,
		CanSearch:        true,
		CanDownload:      true,
		CanMkdir:         true,
		CanRename:        true,
		CanMove:          true,
		CanCopy:          true,
		CanDelete:        true,
		CanProviderTrash: true,
	}}
	fileSvc := NewFileService(
		sourceRepo,
		nil,
		nil,
		nil,
		WithFileDriver(infraStorage.PikPakDriverType, driver),
		WithFileCapabilityProvider(infraStorage.PikPakDriverType, capabilities),
	)

	if _, err := fileSvc.Mkdir(context.Background(), appdto.MkdirRequest{SourceID: source.ID, ParentPath: "/", Name: "new"}); err != nil {
		t.Fatalf("FileService.Mkdir() error = %v", err)
	}
	if _, _, _, err := fileSvc.Rename(context.Background(), appdto.RenameRequest{SourceID: source.ID, Path: "/old.txt", NewName: "renamed.txt"}); err != nil {
		t.Fatalf("FileService.Rename() error = %v", err)
	}
	if _, _, err := fileSvc.Move(context.Background(), appdto.MoveCopyRequest{SourceID: source.ID, Path: "/move.txt", TargetPath: "/Target"}); err != nil {
		t.Fatalf("FileService.Move() error = %v", err)
	}
	if _, _, err := fileSvc.Copy(context.Background(), appdto.MoveCopyRequest{SourceID: source.ID, Path: "/copy.txt", TargetPath: "/Target"}); err != nil {
		t.Fatalf("FileService.Copy() error = %v", err)
	}
	if _, err := fileSvc.Delete(context.Background(), appdto.DeleteFileRequest{SourceID: source.ID, Path: "/trash.txt"}); err != nil {
		t.Fatalf("FileService.Delete() error = %v", err)
	}
	if _, err := fileSvc.Delete(context.Background(), appdto.DeleteFileRequest{SourceID: source.ID, Path: "/trash.txt", DeleteMode: "permanent"}); !errors.Is(err, ErrSourceOperationUnsupported) {
		t.Fatalf("FileService.Delete(permanent) expected unsupported for provider trash driver, got %v", err)
	}
	wantFileCalls := []pikPakStageCFileCall{
		{operation: "mkdir", parentPath: "/", name: "new"},
		{operation: "rename", path: "/old.txt", name: "renamed.txt"},
		{operation: "move", path: "/move.txt", targetPath: "/Target"},
		{operation: "copy", path: "/copy.txt", targetPath: "/Target"},
		{operation: "delete", path: "/trash.txt"},
	}
	if !reflect.DeepEqual(driver.calls, wantFileCalls) {
		t.Fatalf("unexpected FileService driver calls = %+v", driver.calls)
	}

	fileList, _, _, _, _, err := fileSvc.List(context.Background(), appdto.FileListQuery{SourceID: source.ID, Path: "/"})
	if err != nil {
		t.Fatalf("FileService.List() error = %v", err)
	}
	if len(fileList.Items) != 1 || !fileList.Items[0].CanDelete {
		t.Fatalf("expected FileService list can_delete from PikPak capability, got %+v", fileList.Items)
	}

	driver.calls = nil
	vfsSvc := NewVFSService(
		sourceRepo,
		WithVFSFileDriver(infraStorage.PikPakDriverType, driver),
		WithVFSCapabilityProvider(infraStorage.PikPakDriverType, capabilities),
		WithVFSFileOperator(fileSvc),
	)
	vfsList, err := vfsSvc.List(context.Background(), "/pikpak")
	if err != nil {
		t.Fatalf("VFSService.List() error = %v", err)
	}
	if len(vfsList.Items) != 1 || vfsList.Items[0].Path != "/pikpak/remote.txt" || !vfsList.Items[0].CanDelete {
		t.Fatalf("expected VFS can_delete from PikPak capability, got %+v", vfsList.Items)
	}
	if _, err := vfsSvc.Mkdir(context.Background(), appdto.VFSMkdirRequest{ParentPath: "/pikpak", Name: "vfs-new"}); err != nil {
		t.Fatalf("VFSService.Mkdir() error = %v", err)
	}
	if _, _, _, err := vfsSvc.Rename(context.Background(), appdto.VFSRenameRequest{Path: "/pikpak/old.txt", NewName: "vfs-renamed.txt"}); err != nil {
		t.Fatalf("VFSService.Rename() error = %v", err)
	}
	if _, _, err := vfsSvc.Move(context.Background(), appdto.VFSMoveCopyRequest{Path: "/pikpak/move.txt", TargetPath: "/pikpak/Target"}); err != nil {
		t.Fatalf("VFSService.Move() error = %v", err)
	}
	if _, _, err := vfsSvc.Copy(context.Background(), appdto.VFSMoveCopyRequest{Path: "/pikpak/copy.txt", TargetPath: "/pikpak/Target"}); err != nil {
		t.Fatalf("VFSService.Copy() error = %v", err)
	}
	if _, err := vfsSvc.Delete(context.Background(), appdto.VFSDeleteRequest{Path: "/pikpak/trash.txt"}); err != nil {
		t.Fatalf("VFSService.Delete() error = %v", err)
	}
	wantVFSCalls := []pikPakStageCFileCall{
		{operation: "list", path: "/"},
		{operation: "mkdir", parentPath: "/", name: "vfs-new"},
		{operation: "rename", path: "/old.txt", name: "vfs-renamed.txt"},
		{operation: "move", path: "/move.txt", targetPath: "/Target"},
		{operation: "copy", path: "/copy.txt", targetPath: "/Target"},
		{operation: "delete", path: "/trash.txt"},
	}
	if !reflect.DeepEqual(driver.calls, wantVFSCalls) {
		t.Fatalf("unexpected VFSService driver calls = %+v", driver.calls)
	}
}

type sourceServiceFakePikPakClient struct {
	filesByParent map[string][]infraStorage.PikPakFile
}

type pikPakStageCFileCall struct {
	operation  string
	parentPath string
	name       string
	path       string
	targetPath string
}

type pikPakStageCFileDriver struct {
	entriesByPath map[string][]StorageEntry
	calls         []pikPakStageCFileCall
}

func (d *pikPakStageCFileDriver) List(_ context.Context, _ *entity.StorageSource, virtualPath string) ([]StorageEntry, error) {
	d.calls = append(d.calls, pikPakStageCFileCall{operation: "list", path: virtualPath})
	return d.entriesByPath[virtualPath], nil
}

func (d *pikPakStageCFileDriver) SearchByName(context.Context, *entity.StorageSource, string, string) ([]StorageEntry, error) {
	return nil, nil
}

func (d *pikPakStageCFileDriver) Stat(_ context.Context, _ *entity.StorageSource, virtualPath string) (*StorageEntry, error) {
	for _, entries := range d.entriesByPath {
		for _, entry := range entries {
			if entry.Path == virtualPath {
				found := entry
				return &found, nil
			}
		}
	}
	return nil, os.ErrNotExist
}

func (d *pikPakStageCFileDriver) Mkdir(_ context.Context, _ *entity.StorageSource, parentPath string, name string) (*StorageEntry, error) {
	d.calls = append(d.calls, pikPakStageCFileCall{operation: "mkdir", parentPath: parentPath, name: name})
	return &StorageEntry{Name: name, Path: joinVirtualPath(parentPath, name), IsDir: true}, nil
}

func (d *pikPakStageCFileDriver) Rename(_ context.Context, _ *entity.StorageSource, virtualPath string, newName string) (*StorageEntry, error) {
	d.calls = append(d.calls, pikPakStageCFileCall{operation: "rename", path: virtualPath, name: newName})
	parentPath := path.Dir(virtualPath)
	if parentPath == "." {
		parentPath = "/"
	}
	return &StorageEntry{Name: newName, Path: joinVirtualPath(parentPath, newName), IsDir: false}, nil
}

func (d *pikPakStageCFileDriver) Move(_ context.Context, _ *entity.StorageSource, virtualPath string, targetPath string) error {
	d.calls = append(d.calls, pikPakStageCFileCall{operation: "move", path: virtualPath, targetPath: targetPath})
	return nil
}

func (d *pikPakStageCFileDriver) Copy(_ context.Context, _ *entity.StorageSource, virtualPath string, targetPath string) error {
	d.calls = append(d.calls, pikPakStageCFileCall{operation: "copy", path: virtualPath, targetPath: targetPath})
	return nil
}

func (d *pikPakStageCFileDriver) Delete(_ context.Context, _ *entity.StorageSource, virtualPath string) error {
	d.calls = append(d.calls, pikPakStageCFileCall{operation: "delete", path: virtualPath})
	return nil
}

func (d *pikPakStageCFileDriver) PresignDownload(context.Context, *entity.StorageSource, string, string, time.Duration) (string, time.Time, error) {
	return "", time.Time{}, nil
}

func (c *sourceServiceFakePikPakClient) RefreshToken(context.Context, infraStorage.PikPakConfig) (*infraStorage.PikPakAuthToken, error) {
	return &infraStorage.PikPakAuthToken{AccessToken: "access-1", RefreshToken: "refresh-1", UserID: "user-id"}, nil
}

func (c *sourceServiceFakePikPakClient) Login(context.Context, infraStorage.PikPakConfig) (*infraStorage.PikPakAuthToken, error) {
	return &infraStorage.PikPakAuthToken{AccessToken: "access-login", RefreshToken: "refresh-login", UserID: "user-id"}, nil
}

func (c *sourceServiceFakePikPakClient) RefreshCaptcha(context.Context, infraStorage.PikPakConfig, string, string) (*infraStorage.PikPakCaptchaToken, error) {
	return &infraStorage.PikPakCaptchaToken{Token: "captcha-1", ExpiresIn: 300}, nil
}

func (c *sourceServiceFakePikPakClient) ListFiles(_ context.Context, _ infraStorage.PikPakSession, parentID string, _ string) (*infraStorage.PikPakListFilesResponse, error) {
	return &infraStorage.PikPakListFilesResponse{Files: c.filesByParent[parentID]}, nil
}

func (c *sourceServiceFakePikPakClient) GetFile(context.Context, infraStorage.PikPakSession, string, string) (*infraStorage.PikPakFile, error) {
	return nil, errors.New("not used")
}

func (c *sourceServiceFakePikPakClient) CreateFolder(context.Context, infraStorage.PikPakSession, string, string) (*infraStorage.PikPakFile, error) {
	return nil, errors.New("not used")
}

func (c *sourceServiceFakePikPakClient) CreateUploadFile(context.Context, infraStorage.PikPakSession, infraStorage.PikPakCreateUploadFileRequest) (*infraStorage.PikPakCreateUploadFileResponse, error) {
	return nil, errors.New("not used")
}

func (c *sourceServiceFakePikPakClient) RenameFile(context.Context, infraStorage.PikPakSession, string, string) (*infraStorage.PikPakFile, error) {
	return nil, errors.New("not used")
}

func (c *sourceServiceFakePikPakClient) BatchMove(context.Context, infraStorage.PikPakSession, []string, string) error {
	return errors.New("not used")
}

func (c *sourceServiceFakePikPakClient) BatchCopy(context.Context, infraStorage.PikPakSession, []string, string) error {
	return errors.New("not used")
}

func (c *sourceServiceFakePikPakClient) BatchTrash(context.Context, infraStorage.PikPakSession, []string) error {
	return errors.New("not used")
}

func (c *sourceServiceFakePikPakClient) About(context.Context, infraStorage.PikPakSession) (*infraStorage.PikPakAbout, error) {
	return &infraStorage.PikPakAbout{}, nil
}

var _ infraStorage.PikPakAPIClient = (*sourceServiceFakePikPakClient)(nil)
