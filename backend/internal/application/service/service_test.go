package service

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
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

func TestSetupServiceInitCreatesSuperAdminAndStoresRefreshToken(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	userRepo := gorm.NewUserRepository(db)
	refreshRepo := gorm.NewRefreshTokenRepository(db)
	configRepo := gorm.NewSystemConfigRepository(db)
	sourceRepo := gorm.NewSourceRepository(db)
	hasher := security.NewBcryptHasher(4)
	tokenSvc := security.NewJWTTokenService("test-secret", 15*time.Minute, 7*24*time.Hour)
	options := DefaultSystemOptions()
	root := t.TempDir()
	options.StorageDataDir = filepath.Join(root, "storage")
	options.TempDir = filepath.Join(root, "temp")

	svc := NewSetupService(userRepo, refreshRepo, configRepo, sourceRepo, hasher, tokenSvc, options)

	resp, err := svc.Init(context.Background(), appdto.SetupInitRequest{
		Username: "admin",
		Password: "strong-password-123",
		Email:    "admin@example.com",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if resp.User.Username != "admin" || resp.User.RoleKey != "super_admin" || resp.User.Status != "active" {
		t.Fatalf("unexpected user = %+v", resp.User)
	}
	if resp.Tokens.AccessToken == "" || resp.Tokens.RefreshToken == "" {
		t.Fatalf("expected token pair, got %+v", resp.Tokens)
	}

	tokenHash := hashToken(resp.Tokens.RefreshToken)
	stored, err := refreshRepo.FindByTokenHash(context.Background(), tokenHash)
	if err != nil {
		t.Fatalf("FindByTokenHash() error = %v", err)
	}
	if stored.UserID != resp.User.ID {
		t.Fatalf("expected stored token user id %d, got %d", resp.User.ID, stored.UserID)
	}

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.IsInitialized || status.SetupRequired || !status.HasSuperAdmin {
		t.Fatalf("unexpected status = %+v", status)
	}
}

func TestAuthServiceMeReturnsCapabilities(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	userRepo := gorm.NewUserRepository(db)
	refreshRepo := gorm.NewRefreshTokenRepository(db)
	configRepo := gorm.NewSystemConfigRepository(db)
	sourceRepo := gorm.NewSourceRepository(db)
	hasher := security.NewBcryptHasher(4)
	tokenSvc := security.NewJWTTokenService("test-secret", 15*time.Minute, 7*24*time.Hour)
	options := DefaultSystemOptions()
	root := t.TempDir()
	options.StorageDataDir = filepath.Join(root, "storage")
	options.TempDir = filepath.Join(root, "temp")

	setupSvc := NewSetupService(userRepo, refreshRepo, configRepo, sourceRepo, hasher, tokenSvc, options)
	if _, err := setupSvc.Init(context.Background(), appdto.SetupInitRequest{
		Username: "admin",
		Password: "strong-password-123",
		Email:    "admin@example.com",
	}); err != nil {
		t.Fatalf("setup Init() error = %v", err)
	}

	authSvc := NewAuthService(userRepo, refreshRepo, hasher, tokenSvc)

	loginResp, err := authSvc.Login(context.Background(), appdto.LoginRequest{Username: "admin", Password: "strong-password-123"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	me, err := authSvc.Me(context.Background(), loginResp.User.ID)
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if me.User.RoleKey != "super_admin" || me.User.Status != "active" {
		t.Fatalf("unexpected me user = %+v", me.User)
	}
	if len(me.Capabilities) == 0 {
		t.Fatalf("expected capabilities, got empty list")
	}
}

func TestSystemServiceReturnsDefaultAndPersistsUpdate(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	configRepo := gorm.NewSystemConfigRepository(db)
	options := DefaultSystemOptions()
	root := t.TempDir()
	options.StorageDataDir = filepath.Join(root, "storage")
	options.TempDir = filepath.Join(root, "temp")
	svc := NewSystemService(configRepo, options)

	got, err := svc.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if got.SiteName != "云匣" || got.WebDAVPrefix != "/dav" {
		t.Fatalf("unexpected default config = %+v", got)
	}

	updated, err := svc.UpdateConfig(context.Background(), appdto.UpdateSystemConfigRequest{
		SiteName:         "云匣 Pro",
		MultiUserEnabled: true,
		MaxUploadSize:    20 * 1024 * 1024 * 1024,
		DefaultChunkSize: 5 * 1024 * 1024,
		WebDAVEnabled:    true,
		WebDAVPrefix:     "/dav",
		Theme:            "system",
		Language:         "zh-CN",
		TimeZone:         "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if updated.SiteName != "云匣 Pro" || !updated.MultiUserEnabled {
		t.Fatalf("unexpected updated config = %+v", updated)
	}
}

func TestSystemServiceGetStatsAggregatesLocalSourcesAndTasks(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	userRepo := gorm.NewUserRepository(db)
	refreshRepo := gorm.NewRefreshTokenRepository(db)
	configRepo := gorm.NewSystemConfigRepository(db)
	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)
	hasher := security.NewBcryptHasher(4)
	tokenSvc := security.NewJWTTokenService("test-secret", 15*time.Minute, 7*24*time.Hour)
	options := DefaultSystemOptions()
	root := t.TempDir()
	options.StorageDataDir = filepath.Join(root, "storage")
	options.TempDir = filepath.Join(root, "temp")

	setupSvc := NewSetupService(userRepo, refreshRepo, configRepo, sourceRepo, hasher, tokenSvc, options)
	if _, err := setupSvc.Init(context.Background(), appdto.SetupInitRequest{
		Username: "admin",
		Password: "strong-password-123",
		Email:    "admin@example.com",
	}); err != nil {
		t.Fatalf("setup Init() error = %v", err)
	}

	user := &entity.User{
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "hashed",
		RoleKey:      "user",
		Status:       "active",
		TokenVersion: 0,
	}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("userRepo.Create() error = %v", err)
	}

	sources, err := sourceRepo.ListAll(context.Background())
	if err != nil {
		t.Fatalf("sourceRepo.ListAll() error = %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 default source, got %d", len(sources))
	}
	defaultSource := sources[0]

	baseRoot, docsDir, err := resolvePhysicalPath(defaultSource, "/docs")
	if err != nil {
		t.Fatalf("resolvePhysicalPath(/docs) error = %v", err)
	}
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(docsDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(hello.txt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "readme.md"), []byte("read me"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(readme.md) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(baseRoot, ".trash", "20260421-120000"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(.trash) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseRoot, ".trash", "20260421-120000", "ghost.txt"), []byte("ghost"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(ghost.txt) error = %v", err)
	}

	archiveBase := filepath.Join(root, "archive-source")
	if err := os.MkdirAll(archiveBase, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(archiveBase) error = %v", err)
	}
	secondConfig, err := marshalLocalSourceConfig(archiveBase)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	secondSource := &entity.StorageSource{
		Name:            "归档库",
		DriverType:      "local",
		Status:          "online",
		IsEnabled:       true,
		IsWebDAVExposed: false,
		WebDAVReadOnly:  true,
		WebDAVSlug:      "archive",
		RootPath:        "/",
		SortOrder:       10,
		ConfigJSON:      secondConfig,
		LastCheckedAt:   timePointer(time.Now()),
	}
	if err := sourceRepo.Create(context.Background(), secondSource); err != nil {
		t.Fatalf("sourceRepo.Create(secondSource) error = %v", err)
	}
	_, mediaDir, err := resolvePhysicalPath(secondSource, "/media")
	if err != nil {
		t.Fatalf("resolvePhysicalPath(/media) error = %v", err)
	}
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(mediaDir) error = %v", err)
	}
	videoBytes := []byte("1234567890")
	if err := os.WriteFile(filepath.Join(mediaDir, "clip.mp4"), videoBytes, 0o644); err != nil {
		t.Fatalf("os.WriteFile(clip.mp4) error = %v", err)
	}

	now := time.Now()
	if err := taskRepo.Create(context.Background(), &entity.DownloadTask{
		Type:            "url",
		Status:          "running",
		SourceID:        defaultSource.ID,
		SavePath:        "/downloads",
		DisplayName:     "running-task",
		SourceURL:       "https://example.com/a",
		ExternalID:      "gid-running",
		Progress:        0.5,
		DownloadedBytes: 50,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("taskRepo.Create(running) error = %v", err)
	}
	if err := taskRepo.Create(context.Background(), &entity.DownloadTask{
		Type:            "url",
		Status:          "completed",
		SourceID:        defaultSource.ID,
		SavePath:        "/downloads",
		DisplayName:     "completed-task",
		SourceURL:       "https://example.com/b",
		ExternalID:      "gid-completed",
		Progress:        1,
		DownloadedBytes: 100,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("taskRepo.Create(completed) error = %v", err)
	}
	if err := taskRepo.Create(context.Background(), &entity.DownloadTask{
		Type:            "url",
		Status:          "paused",
		SourceID:        defaultSource.ID,
		SavePath:        "/downloads",
		DisplayName:     "paused-task",
		SourceURL:       "https://example.com/c",
		ExternalID:      "gid-paused",
		Progress:        0.25,
		DownloadedBytes: 25,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("taskRepo.Create(paused) error = %v", err)
	}

	svc := NewSystemService(
		configRepo,
		options,
		WithSystemStatsDependencies(userRepo, sourceRepo, taskRepo),
	)
	stats, err := svc.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	if stats.UsersTotal != 2 {
		t.Fatalf("expected users_total=2, got %+v", stats)
	}
	if stats.SourcesTotal != 2 {
		t.Fatalf("expected sources_total=2, got %+v", stats)
	}
	if stats.FilesTotal != 3 {
		t.Fatalf("expected files_total=3, got %+v", stats)
	}
	if stats.DownloadsRunning != 1 || stats.DownloadsCompleted != 1 {
		t.Fatalf("unexpected download stats = %+v", stats)
	}
	expectedSize := int64(len("hello") + len("read me") + len(videoBytes))
	if stats.StorageUsedBytes != expectedSize {
		t.Fatalf("expected storage_used_bytes=%d, got %+v", expectedSize, stats)
	}
}

func TestTaskServiceCreatePersistsOwnerID(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	configJSON, err := marshalLocalSourceConfig(t.TempDir())
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}

	source := &entity.StorageSource{
		Name:            "下载源",
		DriverType:      "local",
		Status:          "online",
		IsEnabled:       true,
		IsWebDAVExposed: false,
		WebDAVReadOnly:  true,
		WebDAVSlug:      "downloads",
		MountPath:       "/tasks",
		RootPath:        "/",
		SortOrder:       0,
		ConfigJSON:      configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	svc := NewTaskService(taskRepo, sourceRepo, taskServiceTestDownloader{})
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:  42,
		RoleKey: "user",
		Status:  "active",
	})

	resp, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:     "download",
		URL:      "https://example.com/archive.zip",
		SourceID: source.ID,
		SavePath: "/downloads",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if resp.SaveVirtualPath != "/tasks/downloads" || resp.ResolvedSourceID != source.ID || resp.ResolvedInnerSavePath != "/downloads" {
		t.Fatalf("expected task virtual snapshots to be persisted, got %+v", resp)
	}

	var storedUserID, storedResolvedSourceID uint
	var storedSaveVirtualPath, storedResolvedInnerSavePath string
	row := db.WithContext(context.Background()).
		Raw("select user_id, save_virtual_path, resolved_source_id, resolved_inner_save_path from download_task_models where id = ?", resp.ID).
		Row()
	if err := row.Scan(&storedUserID, &storedSaveVirtualPath, &storedResolvedSourceID, &storedResolvedInnerSavePath); err != nil {
		t.Fatalf("scan persisted task snapshot error = %v", err)
	}
	if storedUserID != 42 {
		t.Fatalf("expected stored task user_id=42, got %d", storedUserID)
	}
	if storedSaveVirtualPath != "/tasks/downloads" || storedResolvedSourceID != source.ID || storedResolvedInnerSavePath != "/downloads" {
		t.Fatalf(
			"expected persisted task snapshots, got save_virtual_path=%q resolved_source_id=%d resolved_inner_save_path=%q",
			storedSaveVirtualPath,
			storedResolvedSourceID,
			storedResolvedInnerSavePath,
		)
	}

	stored, err := taskRepo.FindByID(context.Background(), resp.ID)
	if err != nil {
		t.Fatalf("taskRepo.FindByID() error = %v", err)
	}
	userIDField := reflect.ValueOf(stored).Elem().FieldByName("UserID")
	if !userIDField.IsValid() {
		t.Fatalf("expected DownloadTask to expose UserID field")
	}
	if userIDField.Uint() != 42 {
		t.Fatalf("expected task entity user_id=42, got %d", userIDField.Uint())
	}
}

func TestTaskCreateDownloadsIntoStagingAndImportsCompletedLocalFile(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "本地下载目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	downloader := &completedWritingDownloader{
		filename: "archive.zip",
		content:  []byte("downloaded archive"),
	}
	stagingRoot := filepath.Join(t.TempDir(), "staging")
	svc := NewTaskService(taskRepo, sourceRepo, downloader, WithTaskStagingDir(stagingRoot))
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	created, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:     "download",
		URL:      "https://example.com/archive.zip",
		SourceID: source.ID,
		SavePath: "/downloads",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if downloader.addDir == "/downloads" || filepath.Dir(downloader.addDir) != stagingRoot {
		t.Fatalf("expected downloader dir under staging root %q, got %q", stagingRoot, downloader.addDir)
	}
	stored, err := taskRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("taskRepo.FindByID() error = %v", err)
	}
	if stored.StagingDir != downloader.addDir {
		t.Fatalf("expected stored staging dir %q, got %+v", downloader.addDir, stored)
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected imported task completed, got %+v", got)
	}
	targetPath := filepath.Join(basePath, "downloads", "archive.zip")
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("expected imported file at %s: %v", targetPath, err)
	}
	if string(content) != "downloaded archive" {
		t.Fatalf("unexpected imported content %q", content)
	}
	if _, err := os.Stat(downloader.addDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected staging dir removed after import, stat err=%v", err)
	}
}

func TestTaskCompletedImportIsIdempotentWhenStagingAlreadyCleaned(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "幂等导入目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	downloader := &completedWritingDownloader{
		filename: "archive.zip",
		content:  []byte("downloaded archive"),
	}
	stagingRoot := filepath.Join(t.TempDir(), "staging")
	svc := NewTaskService(taskRepo, sourceRepo, downloader, WithTaskStagingDir(stagingRoot))
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	created, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:     "download",
		URL:      "https://example.com/archive.zip",
		SourceID: source.ID,
		SavePath: "/downloads",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	storedBeforeImport, err := taskRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("taskRepo.FindByID(before) error = %v", err)
	}
	originalStagingDir := storedBeforeImport.StagingDir

	if _, err := svc.Get(ctx, created.ID); err != nil {
		t.Fatalf("Get() first import error = %v", err)
	}
	if _, err := os.Stat(originalStagingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected staging dir removed after first import, stat err=%v", err)
	}

	// 模拟并发刷新中的旧快照：目标文件已导入，但旧任务状态仍尝试再次导入已清理的 staging。
	staleTask, err := taskRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("taskRepo.FindByID(after) error = %v", err)
	}
	staleTask.Status = "running"
	staleTask.StagingDir = originalStagingDir
	staleTask.ErrorMessage = nil
	staleTask.FinishedAt = nil
	if err := taskRepo.Update(context.Background(), staleTask); err != nil {
		t.Fatalf("taskRepo.Update(stale) error = %v", err)
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() idempotent import error = %v", err)
	}
	if got.Status != "completed" || got.ErrorMessage != nil {
		t.Fatalf("expected idempotent refresh to keep completed without error, got %+v", got)
	}
	stored, err := taskRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("taskRepo.FindByID(final) error = %v", err)
	}
	if stored.StagingDir != "" {
		t.Fatalf("expected cleaned staging dir snapshot to be persisted, got %q", stored.StagingDir)
	}
}

func TestTaskCompletedImportUsesExistingLocalTargetWhenStagingEmpty(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)
	nodeRepo := gorm.NewVFSNodeRepository(db)
	objectRepo := gorm.NewStorageObjectRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "空 staging 幂等导入目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	filename := "WeChatWin_4.1.9.exe"
	targetDir := filepath.Join(basePath, "downloads")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(target) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, filename), []byte("installer"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	emptyStagingDir := filepath.Join(t.TempDir(), "staging", "task-empty")
	if err := os.MkdirAll(emptyStagingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(staging) error = %v", err)
	}

	now := time.Now()
	total := int64(len("installer"))
	task := &entity.DownloadTask{
		UserID:                  42,
		Type:                    "download",
		DownloaderType:          DownloaderTypeAria2,
		Status:                  "running",
		SourceID:                source.ID,
		SavePath:                "/downloads",
		TargetVirtualParentPath: "/local/downloads",
		SaveVirtualPath:         "/local/downloads",
		ResolvedSourceID:        source.ID,
		ResolvedInnerSavePath:   "/downloads",
		StagingDir:              emptyStagingDir,
		DisplayName:             filename,
		SourceURL:               "https://example.com/" + filename,
		ExternalID:              "gid-empty-staging",
		Progress:                100,
		DownloadedBytes:         total,
		TotalBytes:              &total,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatalf("taskRepo.Create() error = %v", err)
	}

	downloader := &completedWritingDownloader{filename: filename, content: []byte("installer")}
	committer := NewMetadataVFSCommitService(nodeRepo, objectRepo, WithMetadataVFSCommitTransactor(gorm.NewTransactor(db)))
	svc := NewTaskService(
		taskRepo,
		sourceRepo,
		downloader,
		WithTaskMetadataVFSCommitter(committer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	got, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" || got.ErrorMessage != nil || got.ResultVFSNodeID == 0 {
		t.Fatalf("expected completed task with result_vfs_node_id, got %+v", got)
	}
	if _, err := os.Stat(emptyStagingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected empty staging dir to be cleaned, stat err=%v", err)
	}
	node, err := nodeRepo.FindByPath(context.Background(), "/local/downloads/"+filename)
	if err != nil {
		t.Fatalf("nodeRepo.FindByPath(result) error = %v", err)
	}
	if got.ResultVFSNodeID != node.ID {
		t.Fatalf("expected result_vfs_node_id=%d, got %+v", node.ID, got)
	}
}

func TestTaskCompletedImportRecoversExistingLocalTargetNode(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)
	nodeRepo := gorm.NewVFSNodeRepository(db)
	objectRepo := gorm.NewStorageObjectRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "已有目标节点",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	filename := "download.txt"
	targetDir := filepath.Join(basePath, "downloads")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(target) error = %v", err)
	}
	targetFile := filepath.Join(targetDir, filename)
	if err := os.WriteFile(targetFile, []byte("already downloaded"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}

	committer := NewMetadataVFSCommitService(nodeRepo, objectRepo, WithMetadataVFSCommitTransactor(gorm.NewTransactor(db)))
	commitResult, err := committer.CommitFileObject(context.Background(), MetadataVFSFileObjectCommitRequest{
		Source:                  source,
		VirtualParentPath:       "/local/downloads",
		ResolvedInnerParentPath: "/downloads",
		Filename:                filename,
		ObjectPath:              "/downloads/" + filename,
		Size:                    int64(len("already downloaded")),
		MimeType:                "text/plain",
	})
	if err != nil {
		t.Fatalf("CommitFileObject(existing) error = %v", err)
	}

	stagingDir := filepath.Join(t.TempDir(), "staging", "task-existing")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(staging) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, filename), []byte("already downloaded"), 0o644); err != nil {
		t.Fatalf("WriteFile(staging) error = %v", err)
	}

	total := int64(len("already downloaded"))
	now := time.Now()
	task := &entity.DownloadTask{
		UserID:                  42,
		Type:                    "download",
		DownloaderType:          DownloaderTypeAria2,
		Status:                  "running",
		SourceID:                source.ID,
		SavePath:                "/downloads",
		TargetVirtualParentPath: "/local/downloads",
		SaveVirtualPath:         "/local/downloads",
		ResolvedSourceID:        source.ID,
		ResolvedInnerSavePath:   "/downloads",
		StagingDir:              stagingDir,
		DisplayName:             filename,
		SourceURL:               "https://example.com/" + filename,
		ExternalID:              "gid-existing-target",
		DownloadedBytes:         total,
		TotalBytes:              &total,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatalf("taskRepo.Create() error = %v", err)
	}

	metadataReader := NewMetadataVFSService(nodeRepo)
	svc := NewTaskService(
		taskRepo,
		sourceRepo,
		&completedWritingDownloader{filename: filename, content: []byte("already downloaded")},
		WithTaskMetadataVFSReader(metadataReader),
		WithTaskMetadataVFSCommitter(committer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	got, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" || got.ErrorMessage != nil || got.ResultVFSNodeID != commitResult.Node.ID {
		t.Fatalf("expected recovered completed task, got %+v existingNode=%+v", got, commitResult.Node)
	}
	if _, err := os.Stat(stagingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected staging dir cleaned, stat err=%v", err)
	}
}

func TestTaskTargetFilenameRenamesSingleStagedFile(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "文件名模板目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	downloader := &completedWritingDownloader{
		filename: "original.name.mkv",
		content:  []byte("video"),
	}
	svc := NewTaskService(taskRepo, sourceRepo, downloader, WithTaskStagingDir(filepath.Join(t.TempDir(), "staging")))
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	created, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:           "download",
		URL:            "https://example.com/original.name.mkv",
		SourceID:       source.ID,
		SavePath:       "/downloads",
		TargetFilename: "Example Show - S01E05 [1080p]",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.TargetFilename != "Example Show - S01E05 [1080p]" {
		t.Fatalf("created target filename = %q", created.TargetFilename)
	}
	stored, err := taskRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("taskRepo.FindByID() error = %v", err)
	}
	if stored.TargetFilename != created.TargetFilename {
		t.Fatalf("stored target filename = %q", stored.TargetFilename)
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected completed task, got %+v", got)
	}
	targetPath := filepath.Join(basePath, "downloads", "Example Show - S01E05 [1080p].mkv")
	if content, err := os.ReadFile(targetPath); err != nil || string(content) != "video" {
		t.Fatalf("expected renamed imported file at %s, content=%q err=%v", targetPath, content, err)
	}
	if _, err := os.Stat(filepath.Join(basePath, "downloads", "original.name.mkv")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected original filename not to be imported, stat err=%v", err)
	}
}

func TestTaskTargetFilenameIgnoredForMultiFileStaging(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "多文件模板目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	downloader := &completedWritingDownloader{
		files: map[string][]byte{
			"TorrentDir/episode.mkv": []byte("video"),
			"TorrentDir/poster.jpg":  []byte("poster"),
		},
	}
	svc := NewTaskService(taskRepo, sourceRepo, downloader, WithTaskStagingDir(filepath.Join(t.TempDir(), "staging")))
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	created, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:           "download",
		URL:            "https://example.com/multifile.zip",
		SourceID:       source.ID,
		SavePath:       "/downloads",
		TargetFilename: "Should Not Apply",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected completed task, got %+v", got)
	}
	if content, err := os.ReadFile(filepath.Join(basePath, "downloads", "TorrentDir", "episode.mkv")); err != nil || string(content) != "video" {
		t.Fatalf("expected original multi-file path, content=%q err=%v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(basePath, "downloads", "TorrentDir", "poster.jpg")); err != nil || string(content) != "poster" {
		t.Fatalf("expected original poster path, content=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(basePath, "downloads", "Should Not Apply.mkv")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected target filename not applied for multi-file task, stat err=%v", err)
	}
}

func TestTaskTargetFilenameExtensionHeuristic(t *testing.T) {
	if got := taskTargetFilenameWithOriginalExtension("Dr.STONE - S01.05", "downloaded.mkv"); got != "Dr.STONE - S01.05.mkv" {
		t.Fatalf("expected numeric suffix not to be treated as extension, got %q", got)
	}
	if got := taskTargetFilenameWithOriginalExtension("Example Show.mkv", "downloaded.mp4"); got != "Example Show.mkv" {
		t.Fatalf("expected explicit extension to be preserved, got %q", got)
	}
}

func TestTaskCompletedClearsRealtimeDownloadMetrics(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "完成态指标目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	eta := int64(1)
	downloader := &completedWritingDownloader{
		filename:      "done.bin",
		content:       []byte("done"),
		downloadSpeed: 10583607,
		etaSeconds:    &eta,
	}
	svc := NewTaskService(taskRepo, sourceRepo, downloader, WithTaskStagingDir(filepath.Join(t.TempDir(), "staging")))
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	created, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:     "download",
		URL:      "https://example.com/done.bin",
		SourceID: source.ID,
		SavePath: "/downloads",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected completed task, got %+v", got)
	}
	if got.SpeedBytes != 0 || got.ETASeconds != nil {
		t.Fatalf("expected completed task to clear speed/eta, got speed=%d eta=%v", got.SpeedBytes, got.ETASeconds)
	}
}

func TestTaskCompletedClearsStaleDownloaderErrorMessage(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "完成态错误清理目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	staleMessage := "file already exists"
	downloader := &completedWritingDownloader{
		filename:     "done-with-stale-error.bin",
		content:      []byte("done"),
		errorMessage: &staleMessage,
	}
	svc := NewTaskService(taskRepo, sourceRepo, downloader, WithTaskStagingDir(filepath.Join(t.TempDir(), "staging")))
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	created, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:     "download",
		URL:      "https://example.com/done-with-stale-error.bin",
		SourceID: source.ID,
		SavePath: "/downloads",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected completed task, got %+v", got)
	}
	if got.ErrorMessage != nil {
		t.Fatalf("expected completed task to clear stale error_message, got %q", *got.ErrorMessage)
	}
}

func TestTaskGetSanitizesPersistedCompletedRealtimeMetrics(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	configJSON, err := marshalLocalSourceConfig(t.TempDir())
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "完成态历史指标目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	eta := int64(1)
	now := time.Now()
	task := &entity.DownloadTask{
		UserID:          42,
		Type:            "download",
		Status:          "completed",
		SourceID:        source.ID,
		SavePath:        "/downloads",
		DisplayName:     "stale.bin",
		SourceURL:       "https://example.com/stale.bin",
		ExternalID:      "gid-stale",
		Progress:        100,
		DownloadedBytes: 100,
		SpeedBytes:      10583607,
		ETASeconds:      &eta,
		FinishedAt:      &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatalf("taskRepo.Create() error = %v", err)
	}

	svc := NewTaskService(taskRepo, sourceRepo, nil)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	got, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected completed task, got %+v", got)
	}
	if got.SpeedBytes != 0 || got.ETASeconds != nil {
		t.Fatalf("expected persisted completed task metrics sanitized, got speed=%d eta=%v", got.SpeedBytes, got.ETASeconds)
	}
}

func TestTaskGetSanitizesPersistedCompletedErrorMessage(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	configJSON, err := marshalLocalSourceConfig(t.TempDir())
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "完成态历史错误目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	staleMessage := "file already exists"
	now := time.Now()
	task := &entity.DownloadTask{
		UserID:       42,
		Type:         "download",
		Status:       "completed",
		SourceID:     source.ID,
		SavePath:     "/downloads",
		DisplayName:  "done.bin",
		SourceURL:    "https://example.com/done.bin",
		ExternalID:   "gid-completed-error",
		ErrorMessage: &staleMessage,
		FinishedAt:   &now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatalf("taskRepo.Create() error = %v", err)
	}

	svc := NewTaskService(taskRepo, sourceRepo, nil)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	got, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected completed task, got %+v", got)
	}
	if got.ErrorMessage != nil {
		t.Fatalf("expected persisted completed error_message sanitized, got %q", *got.ErrorMessage)
	}
}

func TestTaskCreateSupportsNonLocalTargetByImportDriver(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	source := &entity.StorageSource{
		Name:       "S3 下载目标",
		DriverType: "s3",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/remote",
		RootPath:   "/",
		ConfigJSON: "{}",
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	downloader := &completedWritingDownloader{
		filename: "episode01.mkv",
		content:  []byte("episode-content"),
	}
	importer := &recordingTaskImportDriver{}
	committer := &recordingMetadataVFSCommitter{}
	svc := NewTaskService(
		taskRepo,
		sourceRepo,
		downloader,
		WithTaskStagingDir(filepath.Join(t.TempDir(), "staging")),
		WithTaskImportDriver("s3", importer),
		WithTaskMetadataVFSCommitter(committer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	created, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:     "download",
		URL:      "https://example.com/episode01.mkv",
		SourceID: source.ID,
		SavePath: "/shows",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.Get(ctx, created.ID); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(importer.calls) != 1 {
		t.Fatalf("expected one import call, got %+v", importer.calls)
	}
	call := importer.calls[0]
	if call.targetPath != "/shows/episode01.mkv" {
		t.Fatalf("expected remote target /shows/episode01.mkv, got %+v", call)
	}
	if string(call.content) != "episode-content" {
		t.Fatalf("unexpected imported remote content %q", call.content)
	}
	if len(committer.calls) != 1 {
		t.Fatalf("expected one metadata commit after task import, got %+v", committer.calls)
	}
	commitCall := committer.calls[0]
	if commitCall.Source == nil ||
		commitCall.Source.ID != source.ID ||
		commitCall.Source.DriverType != source.DriverType ||
		commitCall.VirtualParentPath != "/remote/shows" ||
		commitCall.ResolvedInnerParentPath != "/shows" ||
		commitCall.Filename != "episode01.mkv" ||
		commitCall.ObjectPath != "/shows/episode01.mkv" ||
		commitCall.Size != int64(len("episode-content")) ||
		commitCall.ActorID == nil ||
		*commitCall.ActorID != 42 {
		t.Fatalf("unexpected task metadata commit call = %+v", commitCall)
	}
}

func TestTaskCreateAcceptsTargetVirtualParentPath(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "VFS 下载目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/media",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	downloader := &completedWritingDownloader{
		filename: "movie.mkv",
		content:  []byte("movie-content"),
	}
	vfsSvc := NewVFSService(sourceRepo)
	svc := NewTaskService(
		taskRepo,
		sourceRepo,
		downloader,
		WithTaskStagingDir(filepath.Join(t.TempDir(), "staging")),
		WithTaskVFSResolver(vfsSvc),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	created, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:                    "download",
		URL:                     "https://example.com/movie.mkv",
		TargetVirtualParentPath: "/media/downloads",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.SourceID != source.ID ||
		created.SavePath != "/downloads" ||
		created.TargetVirtualParentPath != "/media/downloads" ||
		created.SaveVirtualPath != "/media/downloads" ||
		created.ResolvedSourceID != source.ID ||
		created.ResolvedInnerSavePath != "/downloads" {
		t.Fatalf("unexpected resolved task view = %+v", created)
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected task completed, got %+v", got)
	}
	content, err := os.ReadFile(filepath.Join(basePath, "downloads", "movie.mkv"))
	if err != nil {
		t.Fatalf("expected imported file through vfs target: %v", err)
	}
	if string(content) != "movie-content" {
		t.Fatalf("unexpected imported content %q", content)
	}
}

func TestTaskImportCommitsMetadataVFSFileObject(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)
	nodeRepo := gorm.NewVFSNodeRepository(db)
	objectRepo := gorm.NewStorageObjectRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "metadata 下载目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/media",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	downloader := &completedWritingDownloader{
		filename: "movie.mkv",
		content:  []byte("movie-content"),
	}
	committer := NewMetadataVFSCommitService(nodeRepo, objectRepo, WithMetadataVFSCommitTransactor(gorm.NewTransactor(db)))
	svc := NewTaskService(
		taskRepo,
		sourceRepo,
		downloader,
		WithTaskStagingDir(filepath.Join(t.TempDir(), "staging")),
		WithTaskVFSResolver(NewVFSService(sourceRepo)),
		WithTaskMetadataVFSCommitter(committer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	created, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:                    "download",
		URL:                     "https://example.com/movie.mkv",
		TargetVirtualParentPath: "/media/downloads",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected completed task, got %+v", got)
	}

	node, err := nodeRepo.FindByPath(context.Background(), "/media/downloads/movie.mkv")
	if err != nil {
		t.Fatalf("nodeRepo.FindByPath(file) error = %v", err)
	}
	if node.Kind != entity.VFSNodeKindFile || node.ObjectID == nil || node.SourceID == nil || *node.SourceID != source.ID {
		t.Fatalf("unexpected metadata file node = %+v", node)
	}
	if got.ResultVFSNodeID != node.ID {
		t.Fatalf("expected task result_vfs_node_id=%d, got %+v", node.ID, got)
	}
	object, err := objectRepo.FindByID(context.Background(), *node.ObjectID)
	if err != nil {
		t.Fatalf("objectRepo.FindByID() error = %v", err)
	}
	if object.SourceID != source.ID || object.DriverType != "local" || object.LocatorType != "local_path" ||
		!strings.Contains(object.LocatorJSON, `"/downloads/movie.mkv"`) ||
		object.Status != entity.StorageObjectStatusAvailable ||
		object.Size != int64(len("movie-content")) {
		t.Fatalf("unexpected storage object = %+v", object)
	}
	parent, err := nodeRepo.FindByPath(context.Background(), "/media/downloads")
	if err != nil {
		t.Fatalf("nodeRepo.FindByPath(parent) error = %v", err)
	}
	if parent.Kind != entity.VFSNodeKindDir || parent.SourceID == nil || *parent.SourceID != source.ID {
		t.Fatalf("unexpected metadata parent node = %+v", parent)
	}
}

func TestTaskMetadataCommitFailureDoesNotCompleteTask(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)
	operationRepo := gorm.NewVFSOperationRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "metadata 失败目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/media",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	commitErr := errors.New(`metadata commit failed at D:\secret\yunxia\physical-path token=top-secret`)
	committer := &failingMetadataVFSCommitter{err: commitErr}
	journal := NewVFSOperationJournalService(operationRepo)
	svc := NewTaskService(
		taskRepo,
		sourceRepo,
		&completedWritingDownloader{filename: "broken.bin", content: []byte("broken")},
		WithTaskStagingDir(filepath.Join(t.TempDir(), "staging")),
		WithTaskVFSResolver(NewVFSService(sourceRepo)),
		WithTaskMetadataVFSCommitter(committer),
		WithTaskOperationJournal(journal),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	created, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:                    "download",
		URL:                     "https://example.com/broken.bin",
		TargetVirtualParentPath: "/media/downloads",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status == "completed" || got.ErrorMessage == nil || *got.ErrorMessage != ErrMetadataVFSCommitFailed.Error() {
		t.Fatalf("expected sanitized metadata failure to mark task failed, got %+v", got)
	}
	if strings.Contains(*got.ErrorMessage, "top-secret") || strings.Contains(*got.ErrorMessage, "physical-path") {
		t.Fatalf("metadata failure leaked unsafe details: %q", *got.ErrorMessage)
	}
	if committer.calls != 1 {
		t.Fatalf("expected one metadata commit attempt, got %d", committer.calls)
	}
	stored, err := taskRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("taskRepo.FindByID() error = %v", err)
	}
	if stored.Status == "completed" {
		t.Fatalf("task should not be persisted as completed after metadata failure: %+v", stored)
	}
	operations, err := operationRepo.ListDue(context.Background(), domainrepo.VFSOperationDueFilter{
		OperationType: entity.VFSOperationTypeTaskCommit,
		DueBefore:     time.Now().Add(time.Hour),
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("operationRepo.ListDue() error = %v", err)
	}
	if len(operations) != 1 {
		t.Fatalf("expected one task_commit journal operation, got %+v", operations)
	}
	operation := operations[0]
	if operation.Status != entity.VFSOperationStatusFailed ||
		operation.ErrorCode != "METADATA_VFS_COMMIT_FAILED" ||
		operation.ErrorMessage != ErrMetadataVFSCommitFailed.Error() ||
		strings.Contains(operation.PayloadJSON, "top-secret") ||
		strings.Contains(operation.PayloadJSON, "physical-path") {
		t.Fatalf("unexpected sanitized task_commit operation = %+v", operation)
	}
}

func TestMetadataCommitLocatorJSONIsStableAndRedacted(t *testing.T) {
	locatorType, locatorJSON, err := metadataCommitLocator(MetadataVFSFileObjectCommitRequest{
		LocatorType: "provider_file_id",
		LocatorJSON: `{
			"path": "/safe/object.bin",
			"physical_path": "D:\\secret\\object.bin",
			"access_token": "secret-token",
			"file_id": "file-1",
			"nested": {"password": "secret-password"}
		}`,
	}, "pikpak", "/fallback.bin")
	if err != nil {
		t.Fatalf("metadataCommitLocator(custom) error = %v", err)
	}
	if locatorType != "provider_file_id" {
		t.Fatalf("unexpected locator type %q", locatorType)
	}
	want := `{"access_token":"[redacted]","file_id":"file-1","nested":{"password":"[redacted]"},"path":"/safe/object.bin","physical_path":"[redacted]"}`
	if locatorJSON != want {
		t.Fatalf("unexpected canonical locator JSON:\n got %s\nwant %s", locatorJSON, want)
	}
	if strings.Contains(locatorJSON, "secret-token") ||
		strings.Contains(locatorJSON, "secret-password") ||
		strings.Contains(locatorJSON, `D:\secret`) {
		t.Fatalf("locator JSON leaked secret values: %s", locatorJSON)
	}

	defaultType, defaultJSON, err := metadataCommitLocator(MetadataVFSFileObjectCommitRequest{}, "s3", "/objects/movie.mkv")
	if err != nil {
		t.Fatalf("metadataCommitLocator(default) error = %v", err)
	}
	if defaultType != "driver_path" || defaultJSON != `{"path":"/objects/movie.mkv"}` {
		t.Fatalf("unexpected default locator type/json = %q %s", defaultType, defaultJSON)
	}
}

func TestTaskRefreshSetsTerminalErrorMessage(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		wantPrefix string
	}{
		{name: "failed", status: "failed", wantPrefix: "download failed"},
		{name: "canceled", status: "canceled", wantPrefix: "download canceled by downloader"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, cleanup := openTestDB(t)
			defer cleanup()

			sourceRepo := gorm.NewSourceRepository(db)
			taskRepo := gorm.NewTaskRepository(db)

			basePath := t.TempDir()
			configJSON, err := marshalLocalSourceConfig(basePath)
			if err != nil {
				t.Fatalf("marshalLocalSourceConfig() error = %v", err)
			}
			source := &entity.StorageSource{
				Name:       "下载目标",
				DriverType: "local",
				Status:     "online",
				IsEnabled:  true,
				MountPath:  "/local",
				RootPath:   "/",
				ConfigJSON: configJSON,
			}
			if err := sourceRepo.Create(context.Background(), source); err != nil {
				t.Fatalf("sourceRepo.Create() error = %v", err)
			}

			downloader := fixedStatusDownloader{status: tt.status}
			svc := NewTaskService(taskRepo, sourceRepo, downloader, WithTaskStagingDir(filepath.Join(t.TempDir(), "staging")))
			ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})
			created, err := svc.Create(ctx, appdto.CreateTaskRequest{
				Type:     "download",
				URL:      "https://example.com/archive.zip",
				SourceID: source.ID,
				SavePath: "/downloads",
			})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			got, err := svc.Get(ctx, created.ID)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if got.Status != tt.status {
				t.Fatalf("expected status %q, got %+v", tt.status, got)
			}
			if got.ErrorMessage == nil || !strings.HasPrefix(*got.ErrorMessage, tt.wantPrefix) {
				t.Fatalf("expected terminal error message %q, got %+v", tt.wantPrefix, got.ErrorMessage)
			}
			if got.FinishedAt == nil {
				t.Fatalf("expected terminal task to set finished_at")
			}
		})
	}
}

func TestTaskCancelSetsTerminalErrorMessage(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	taskRepo := gorm.NewTaskRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "下载目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	downloader := fixedStatusDownloader{status: "running"}
	svc := NewTaskService(taskRepo, sourceRepo, downloader, WithTaskStagingDir(filepath.Join(t.TempDir(), "staging")))
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})
	created, err := svc.Create(ctx, appdto.CreateTaskRequest{
		Type:     "download",
		URL:      "https://example.com/archive.zip",
		SourceID: source.ID,
		SavePath: "/downloads",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := svc.Cancel(ctx, created.ID, false); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Status != "canceled" {
		t.Fatalf("expected canceled, got %+v", got)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage != "download canceled by user" {
		t.Fatalf("expected user cancel error message, got %+v", got.ErrorMessage)
	}
}

func TestTaskCancelCompletedTaskIsIdempotentAndDoesNotCallDownloader(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	taskRepo := gorm.NewTaskRepository(db)
	now := time.Now()
	task := &entity.DownloadTask{
		UserID:     42,
		Type:       "download",
		Status:     "completed",
		SourceURL:  "https://example.com/done.bin",
		ExternalID: "gid-completed",
		CreatedAt:  now,
		UpdatedAt:  now,
		FinishedAt: &now,
	}
	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatalf("taskRepo.Create() error = %v", err)
	}

	downloader := &removeCountingDownloader{removeErr: errors.New("aria2 rpc status 400")}
	svc := NewTaskService(taskRepo, nil, downloader)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	if _, err := svc.Cancel(ctx, task.ID, false); err != nil {
		t.Fatalf("Cancel(completed) should be idempotent, got error = %v", err)
	}
	if downloader.removeCalls != 0 {
		t.Fatalf("completed cancel should not call downloader remove, got %d calls", downloader.removeCalls)
	}
	stored, err := taskRepo.FindByID(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if stored.Status != "completed" {
		t.Fatalf("completed task should remain completed, got %q", stored.Status)
	}
}

func TestTaskCancelUpdatesRSSItemBacklinkNeedsAttention(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	taskRepo := gorm.NewTaskRepository(db)
	rssRepo := gorm.NewRSSRepository(db)
	now := time.Date(2026, 5, 3, 11, 0, 0, 0, time.UTC)
	source := &entity.RSSSource{
		UserID:    42,
		Name:      "rss",
		URL:       "https://example/rss.xml",
		IsEnabled: true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := rssRepo.CreateSource(context.Background(), source); err != nil {
		t.Fatalf("CreateSource() error = %v", err)
	}
	subscription := &entity.RSSSubscription{
		UserID:                  42,
		SourceID:                source.ID,
		Name:                    "sub",
		IsEnabled:               true,
		TargetVirtualParentPath: "/anime",
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := rssRepo.CreateSubscription(context.Background(), subscription); err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}
	task := &entity.DownloadTask{
		UserID:         42,
		Type:           "download",
		DownloaderType: DownloaderTypeAria2,
		Status:         "running",
		SourceURL:      "magnet:?xt=urn:btih:cancel",
		ExternalID:     "gid-cancel",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatalf("taskRepo.Create() error = %v", err)
	}
	item := &entity.RSSItem{
		UserID:                42,
		SourceID:              source.ID,
		Title:                 "cancel",
		DownloadURL:           task.SourceURL,
		LinkType:              RSSLinkTypeMagnet,
		Status:                RSSItemStatusEnqueued,
		MatchedSubscriptionID: &subscription.ID,
		TaskID:                &task.ID,
		MaxRetryCount:         3,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := rssRepo.CreateItem(context.Background(), item); err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}

	taskSvc := NewTaskService(taskRepo, nil, taskServiceTestDownloader{})
	rssSvc := NewRSSService(rssRepo, nil, nil, WithRSSNow(func() time.Time { return now }))
	taskSvc.SetTerminalStatusObserver(rssSvc)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42, RoleKey: "user", Status: "active"})

	if _, err := taskSvc.Cancel(ctx, task.ID, false); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	storedTask, err := taskRepo.FindByID(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("FindByID() task error = %v", err)
	}
	if storedTask.Status != "canceled" {
		t.Fatalf("task status = %q", storedTask.Status)
	}
	storedItem, err := rssRepo.FindItemByID(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("FindItemByID() item error = %v", err)
	}
	if storedItem.Status != RSSItemStatusNeedsAttention || storedItem.NextRetryAt != nil {
		t.Fatalf("rss item after cancel = %#v", storedItem)
	}
	if storedItem.ErrorMessage == nil || *storedItem.ErrorMessage != "download canceled by user" {
		t.Fatalf("rss cancel error message = %#v", storedItem.ErrorMessage)
	}
	if storedItem.RetryReason == nil || *storedItem.RetryReason != RSSRetryReasonTaskFailed {
		t.Fatalf("rss retry reason = %#v", storedItem.RetryReason)
	}
}

func TestUserServiceManagementLifecycle(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	userRepo := gorm.NewUserRepository(db)
	refreshRepo := gorm.NewRefreshTokenRepository(db)
	configRepo := gorm.NewSystemConfigRepository(db)
	sourceRepo := gorm.NewSourceRepository(db)
	hasher := security.NewBcryptHasher(4)
	tokenSvc := security.NewJWTTokenService("test-secret", 15*time.Minute, 7*24*time.Hour)
	options := DefaultSystemOptions()
	root := t.TempDir()
	options.StorageDataDir = filepath.Join(root, "storage")
	options.TempDir = filepath.Join(root, "temp")

	setupSvc := NewSetupService(userRepo, refreshRepo, configRepo, sourceRepo, hasher, tokenSvc, options)
	if _, err := setupSvc.Init(context.Background(), appdto.SetupInitRequest{
		Username: "admin",
		Password: "strong-password-123",
		Email:    "admin@example.com",
	}); err != nil {
		t.Fatalf("setup Init() error = %v", err)
	}

	svc := NewUserService(userRepo, hasher)
	adminCtx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       1,
		RoleKey:      permission.RoleSuperAdmin,
		Status:       permission.StatusActive,
		Capabilities: permission.AllCapabilities(),
	})

	created, err := svc.Create(adminCtx, appdto.CreateUserRequest{
		Username: "alice",
		Password: "strong-password-456",
		Email:    "alice@example.com",
		RoleKey:  permission.RoleUser,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Username != "alice" || created.RoleKey != permission.RoleUser || created.Status != permission.StatusActive {
		t.Fatalf("unexpected created user = %+v", created)
	}

	listed, err := svc.List(context.Background(), appdto.UserListQuery{
		Page:     1,
		PageSize: 20,
		Keyword:  "ali",
		Status:   "active",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].Username != "alice" {
		t.Fatalf("unexpected listed users = %+v", listed.Items)
	}

	updated, err := svc.Update(adminCtx, created.ID, appdto.UpdateUserRequest{
		Email:   "alice+updated@example.com",
		RoleKey: permission.RoleAdmin,
		Status:  permission.StatusLocked,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Email != "alice+updated@example.com" || updated.RoleKey != permission.RoleAdmin || updated.Status != permission.StatusLocked {
		t.Fatalf("unexpected updated user = %+v", updated)
	}

	if err := svc.ResetPassword(context.Background(), created.ID, appdto.ResetUserPasswordRequest{
		NewPassword: "new-strong-password-789",
	}); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}

	stored, err := userRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("userRepo.FindByID() error = %v", err)
	}
	if !hasher.Compare(stored.PasswordHash, "new-strong-password-789") {
		t.Fatalf("expected password reset to update password hash")
	}

	beforeTokenVersion := stored.TokenVersion
	revoked, err := svc.RevokeTokens(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("RevokeTokens() error = %v", err)
	}
	if !revoked.Revoked || revoked.ID != created.ID {
		t.Fatalf("unexpected revoke response = %+v", revoked)
	}

	stored, err = userRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("userRepo.FindByID() after revoke error = %v", err)
	}
	if stored.TokenVersion != beforeTokenVersion+1 {
		t.Fatalf("expected token version increment, got before=%d after=%d", beforeTokenVersion, stored.TokenVersion)
	}
}

func TestCannotLockLastActiveSuperAdmin(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	userRepo := gorm.NewUserRepository(db)
	hasher := security.NewBcryptHasher(4)
	svc := NewUserService(userRepo, hasher)

	root := &entity.User{
		Username:     "root",
		PasswordHash: "hash",
		RoleKey:      permission.RoleSuperAdmin,
		Status:       permission.StatusActive,
	}
	if err := userRepo.Create(context.Background(), root); err != nil {
		t.Fatalf("Create(root) error = %v", err)
	}

	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       root.ID,
		RoleKey:      permission.RoleSuperAdmin,
		Status:       permission.StatusActive,
		Capabilities: permission.AllCapabilities(),
	})
	_, err := svc.Update(ctx, root.ID, appdto.UpdateUserRequest{
		Email:   "root@example.com",
		RoleKey: permission.RoleSuperAdmin,
		Status:  permission.StatusLocked,
	})
	if !errors.Is(err, ErrLastSuperAdminForbidden) {
		t.Fatalf("expected ErrLastSuperAdminForbidden, got %v", err)
	}
}

func TestSourceDetailMasksSecretsWithoutCapability(t *testing.T) {
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       2,
		RoleKey:      permission.RoleAdmin,
		Status:       permission.StatusActive,
		Capabilities: []string{permission.CapabilitySourceRead},
	})

	svc := newSourceServiceWithS3Fixture(t)
	resp, err := svc.Get(ctx, 1)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if resp.Config["secret_key"] != nil {
		t.Fatalf("expected secret_key to stay masked for admin")
	}
	if resp.SecretFields["secret_key"].Configured != true {
		t.Fatalf("expected secret field metadata to be present")
	}
}

func TestSourceDetailShowsSecretsForSuperAdmin(t *testing.T) {
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       1,
		RoleKey:      permission.RoleSuperAdmin,
		Status:       permission.StatusActive,
		Capabilities: permission.AllCapabilities(),
	})

	svc := newSourceServiceWithS3Fixture(t)
	resp, err := svc.Get(ctx, 1)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if resp.Config["secret_key"] == nil {
		t.Fatalf("expected super_admin to see secret_key")
	}
	if resp.Config["access_key"] != "AKIA-TEST-1234" || resp.Config["secret_key"] != "secret-value" {
		t.Fatalf("unexpected secret config = %+v", resp.Config)
	}
}

func TestSourceServiceS3CodecCreateTestDetailAndSecretRetention(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	configRepo := gorm.NewSystemConfigRepository(db)
	probe := &recordingSourceProbe{}
	svc := NewSourceService(sourceRepo, configRepo, WithSourceDriverProbe("s3", probe))
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       1,
		RoleKey:      permission.RoleSuperAdmin,
		Status:       permission.StatusActive,
		Capabilities: permission.AllCapabilities(),
	})

	req := appdto.SourceUpsertRequest{
		Name:       "s3 media",
		DriverType: "s3",
		IsEnabled:  true,
		RootPath:   "/",
		Config: map[string]any{
			"endpoint":         " https://s3.example.com ",
			"region":           "us-east-1",
			"bucket":           "media",
			"base_prefix":      "/library/",
			"force_path_style": true,
		},
		SecretPatch: map[string]any{
			"access_key": "AKIA-TEST-1234",
			"secret_key": "secret-value",
		},
	}
	testResp, err := svc.Test(ctx, req)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	if !testResp.Reachable || len(probe.sources) != 1 {
		t.Fatalf("unexpected test response/probe calls: resp=%+v calls=%d", testResp, len(probe.sources))
	}

	created, err := svc.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.DriverType != "s3" || created.MountPath != "/s3-media" {
		t.Fatalf("unexpected created source view = %+v", created)
	}

	stored, err := sourceRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	cfg, err := infraStorage.ParseS3ConfigJSON(stored.ConfigJSON)
	if err != nil {
		t.Fatalf("ParseS3ConfigJSON() error = %v", err)
	}
	if cfg.Endpoint != "https://s3.example.com" || cfg.BasePrefix != "library" || cfg.AccessKey != "AKIA-TEST-1234" || cfg.SecretKey != "secret-value" {
		t.Fatalf("unexpected stored s3 config = %+v", cfg)
	}

	limitedCtx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       2,
		RoleKey:      permission.RoleAdmin,
		Status:       permission.StatusActive,
		Capabilities: []string{permission.CapabilitySourceRead},
	})
	detail, err := svc.Get(limitedCtx, created.ID)
	if err != nil {
		t.Fatalf("Get(limited) error = %v", err)
	}
	if _, exists := detail.Config["secret_key"]; exists {
		t.Fatalf("expected secret_key to be hidden, config=%+v", detail.Config)
	}
	if !detail.SecretFields["access_key"].Configured || detail.SecretFields["access_key"].Masked != "AKIA****" {
		t.Fatalf("unexpected access key mask = %+v", detail.SecretFields["access_key"])
	}

	updated, err := svc.Update(ctx, created.ID, appdto.SourceUpsertRequest{
		Name:      "s3 media",
		IsEnabled: true,
		RootPath:  "/",
		Config: map[string]any{
			"endpoint":         "https://s3.example.com",
			"region":           "us-east-1",
			"bucket":           "media",
			"base_prefix":      "archive",
			"force_path_style": false,
		},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	updatedSource, err := sourceRepo.FindByID(context.Background(), updated.ID)
	if err != nil {
		t.Fatalf("FindByID(updated) error = %v", err)
	}
	updatedCfg, err := infraStorage.ParseS3ConfigJSON(updatedSource.ConfigJSON)
	if err != nil {
		t.Fatalf("ParseS3ConfigJSON(updated) error = %v", err)
	}
	if updatedCfg.BasePrefix != "archive" || updatedCfg.AccessKey != "AKIA-TEST-1234" || updatedCfg.SecretKey != "secret-value" {
		t.Fatalf("expected update to retain secrets while changing public config, got %+v", updatedCfg)
	}
}

func TestSourceServiceCreatePersistsExplicitWebDAVReadOnlyFalse(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	configRepo := gorm.NewSystemConfigRepository(db)
	svc := NewSourceService(sourceRepo, configRepo)
	basePath := t.TempDir()

	created, err := svc.Create(context.Background(), appdto.SourceUpsertRequest{
		Name:            "WebDAV 可写本地源",
		DriverType:      "local",
		IsEnabled:       true,
		IsWebDAVExposed: true,
		WebDAVReadOnly:  false,
		MountPath:       "/dav-writable",
		RootPath:        "/",
		Config: map[string]any{
			"base_path": basePath,
		},
		SecretPatch: map[string]any{},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !created.IsWebDAVExposed || created.WebDAVReadOnly {
		t.Fatalf("expected created view to preserve webdav_read_only=false, got %+v", created)
	}

	stored, err := sourceRepo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if !stored.IsWebDAVExposed || stored.WebDAVReadOnly {
		t.Fatalf("expected persisted source to preserve webdav_read_only=false, got %+v", stored)
	}
}

func TestSourceServiceTestReturnsCloudErrorWithoutConnectionWrapper(t *testing.T) {
	probe := &recordingSourceProbe{err: ErrCloudCaptchaRequired}
	svc := NewSourceService(
		nil,
		nil,
		WithSourceConfigCodec(fakeSourceConfigCodec{driverType: "fakecloud", slug: "cloud"}),
		WithSourceDriverProbe("fakecloud", probe),
	)

	_, err := svc.Test(context.Background(), appdto.SourceUpsertRequest{
		Name:       "captcha cloud",
		DriverType: "fakecloud",
		IsEnabled:  true,
		RootPath:   "/",
		Config:     map[string]any{},
	})
	if !errors.Is(err, ErrCloudCaptchaRequired) {
		t.Fatalf("expected cloud captcha error, got %v", err)
	}
	if errors.Is(err, ErrSourceConnectionFailed) {
		t.Fatalf("cloud provider error must not be wrapped as source connection failed: %v", err)
	}
}

func TestStorageDriverRegistryOptionsWireS3AndKeepStatsFallbackExplicit(t *testing.T) {
	importer := &recordingTaskImportDriver{}
	fileDriver := &storageFileDriverStub{}
	remoteIndexer := &fakeRemoteIndexer{}
	pikpakProbe := &recordingSourceProbe{}
	pikpakCapabilities := capabilityProviderStub{capabilities: StorageCapabilities{CanList: true, CanDownload: true}}
	registry := NewStorageDriverRegistry(
		DriverBundle{
			Type:         "pikpak",
			DisplayName:  "PikPak",
			Config:       fakeSourceConfigCodec{driverType: "pikpak", slug: "source-pikpak"},
			Probe:        pikpakProbe,
			File:         fileDriver,
			Import:       importer,
			Capabilities: pikpakCapabilities,
		},
		DriverBundle{
			Type:                   "s3",
			DisplayName:            "S3",
			File:                   fileDriver,
			Upload:                 &uploadDriverStub{},
			Import:                 importer,
			RecursiveStatsFallback: true,
		},
		DriverBundle{
			Type:    "indexed",
			File:    fileDriver,
			Indexer: remoteIndexer,
		},
	)

	sourceSvc := NewSourceService(nil, nil, registry.SourceServiceOptions()...)
	if _, exists := sourceSvc.configCodecs["pikpak"]; !exists {
		t.Fatalf("expected pikpak config codec to be registered for source service")
	}
	if _, exists := sourceSvc.driverProbes["pikpak"]; !exists {
		t.Fatalf("expected pikpak source probe to be registered for source service")
	}

	fileSvc := NewFileService(nil, nil, nil, nil, registry.FileServiceOptions()...)
	if _, exists := fileSvc.fileDrivers["s3"]; !exists {
		t.Fatalf("expected s3 file driver to be registered")
	}
	if _, exists := fileSvc.fileDrivers["pikpak"]; !exists {
		t.Fatalf("expected pikpak file driver to be registered for file/vfs services")
	}
	if _, exists := fileSvc.capabilityProviders["pikpak"]; !exists {
		t.Fatalf("expected pikpak capabilities to be registered for file service")
	}

	vfsSvc := NewVFSService(nil, registry.VFSServiceOptions()...)
	if _, exists := vfsSvc.fileDrivers["pikpak"]; !exists {
		t.Fatalf("expected pikpak file driver to be registered for vfs service")
	}
	if _, exists := vfsSvc.capabilityProviders["pikpak"]; !exists {
		t.Fatalf("expected pikpak capabilities to be registered for vfs service")
	}

	metadataSyncSvc := NewMetadataVFSSyncService(nil, nil, nil, registry.MetadataVFSSyncServiceOptions()...)
	if _, exists := metadataSyncSvc.indexers["pikpak"]; !exists {
		t.Fatalf("expected pikpak file driver to be registered for metadata vfs lazy sync")
	}
	if got := metadataSyncSvc.indexers["indexed"]; got != remoteIndexer {
		t.Fatalf("expected explicit metadata vfs indexer to take precedence over file driver bridge")
	}

	trashSvc := NewTrashService(nil, nil, registry.TrashServiceOptions()...)
	if _, exists := trashSvc.fileDrivers["pikpak"]; !exists {
		t.Fatalf("expected pikpak file driver to be registered for trash service")
	}

	uploadSvc := NewUploadService(nil, nil, DefaultSystemOptions(), registry.UploadServiceOptions()...)
	if _, exists := uploadSvc.uploadDrivers["s3"]; !exists {
		t.Fatalf("expected s3 direct upload driver to be registered")
	}
	if _, exists := uploadSvc.importDrivers["pikpak"]; !exists {
		t.Fatalf("expected pikpak import driver to be registered for server_chunk")
	}

	taskSvc := NewTaskService(nil, nil, nil, registry.TaskServiceOptions()...)
	if _, exists := taskSvc.importDrivers["pikpak"]; !exists {
		t.Fatalf("expected pikpak import driver to be registered for task import")
	}

	shareSvc := NewShareService(nil, nil, nil, nil, registry.ShareServiceOptions()...)
	if _, exists := shareSvc.fileDrivers["pikpak"]; !exists {
		t.Fatalf("expected pikpak file driver to be registered for share service")
	}

	systemSvc := NewSystemService(nil, DefaultSystemOptions(), registry.SystemServiceOptions()...)
	if _, exists := systemSvc.fileDrivers["s3"]; !exists {
		t.Fatalf("expected s3 recursive stats fallback to stay registered")
	}
	if _, exists := systemSvc.fileDrivers["pikpak"]; exists {
		t.Fatalf("did not expect pikpak file driver to be used for recursive system stats by default")
	}
}

func TestUploadServiceUsesImportDriverForServerChunkNonLocalDriver(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	uploadRepo := gorm.NewUploadSessionRepository(db)
	source := &entity.StorageSource{
		Name:       "第三方导入源",
		DriverType: "cloud-import",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/cloud",
		RootPath:   "/",
		ConfigJSON: "{}",
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	importer := &recordingTaskImportDriver{}
	committer := &recordingMetadataVFSCommitter{}
	options := DefaultSystemOptions()
	options.TempDir = filepath.Join(t.TempDir(), "upload-temp")
	options.DefaultChunkSize = 5
	options.MaxUploadSize = 1024
	svc := NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		WithUploadImportDriver("cloud-import", importer),
		WithUploadMetadataVFSCommitter(committer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 7, RoleKey: permission.RoleUser, Status: permission.StatusActive})

	initResp, err := svc.Init(ctx, 7, appdto.UploadInitRequest{
		SourceID: source.ID,
		Path:     "/uploads",
		Filename: "movie.mkv",
		FileSize: int64(len("hello-world")),
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if initResp.Transport == nil || initResp.Transport.Mode != "server_chunk" || initResp.Transport.DriverType != "cloud-import" {
		t.Fatalf("unexpected transport = %+v", initResp.Transport)
	}
	if len(initResp.PartInstructions) != 0 {
		t.Fatalf("server_chunk import should not return direct part instructions")
	}

	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 0, []byte("hello")); err != nil {
		t.Fatalf("UploadChunk(0) error = %v", err)
	}
	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 1, []byte("-worl")); err != nil {
		t.Fatalf("UploadChunk(1) error = %v", err)
	}
	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 2, []byte("d")); err != nil {
		t.Fatalf("UploadChunk(2) error = %v", err)
	}

	finishResp, err := svc.Finish(ctx, appdto.UploadFinishRequest{UploadID: initResp.Upload.UploadID})
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if finishResp.File.Path != "/uploads/movie.mkv" || finishResp.File.Size != int64(len("hello-world")) {
		t.Fatalf("unexpected finish file = %+v", finishResp.File)
	}
	if len(importer.calls) != 1 {
		t.Fatalf("expected one import call, got %+v", importer.calls)
	}
	call := importer.calls[0]
	if call.sourceID != source.ID || call.targetPath != "/uploads/movie.mkv" || string(call.content) != "hello-world" {
		t.Fatalf("unexpected import call = %+v", call)
	}
	if len(committer.calls) != 1 {
		t.Fatalf("expected one metadata commit after server_chunk import, got %+v", committer.calls)
	}
	commitCall := committer.calls[0]
	if commitCall.Source == nil ||
		commitCall.Source.ID != source.ID ||
		commitCall.Source.DriverType != source.DriverType ||
		commitCall.VirtualParentPath != "/cloud/uploads" ||
		commitCall.ResolvedInnerParentPath != "/uploads" ||
		commitCall.Filename != "movie.mkv" ||
		commitCall.ObjectPath != "/uploads/movie.mkv" ||
		commitCall.Size != int64(len("hello-world")) ||
		commitCall.ActorID == nil ||
		*commitCall.ActorID != 7 {
		t.Fatalf("unexpected metadata commit call = %+v", commitCall)
	}
	if _, err := uploadRepo.FindByID(context.Background(), initResp.Upload.UploadID); !errors.Is(err, domainrepo.ErrNotFound) {
		t.Fatalf("expected upload session to be deleted, err=%v", err)
	}
}

func TestUploadImportLocalFileCommitsMetadataVFSFileObject(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	source := &entity.StorageSource{
		Name:       "WebDAV metadata import",
		DriverType: "cloud-import",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/cloud",
		RootPath:   "/",
		ConfigJSON: "{}",
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	stagedPath := filepath.Join(t.TempDir(), "webdav-upload.txt")
	if err := os.WriteFile(stagedPath, []byte("webdav-content"), 0o644); err != nil {
		t.Fatalf("write staged file: %v", err)
	}

	importer := &recordingTaskImportDriver{}
	committer := &recordingMetadataVFSCommitter{}
	svc := NewUploadService(
		sourceRepo,
		nil,
		DefaultSystemOptions(),
		WithUploadImportDriver("cloud-import", importer),
		WithUploadMetadataVFSCommitter(committer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 7, RoleKey: permission.RoleUser, Status: permission.StatusActive})

	item, err := svc.ImportLocalFile(ctx, source.ID, "/uploads", "webdav-upload.txt", stagedPath)
	if err != nil {
		t.Fatalf("ImportLocalFile() error = %v", err)
	}
	if item.Path != "/uploads/webdav-upload.txt" || item.Size != int64(len("webdav-content")) {
		t.Fatalf("unexpected imported item = %+v", item)
	}
	if len(importer.calls) != 1 || importer.calls[0].targetPath != "/uploads/webdav-upload.txt" {
		t.Fatalf("expected one import driver call, got %+v", importer.calls)
	}
	if len(committer.calls) != 1 {
		t.Fatalf("expected one metadata commit, got %+v", committer.calls)
	}
	call := committer.calls[0]
	if call.Source == nil ||
		call.Source.ID != source.ID ||
		call.VirtualParentPath != "/cloud/uploads" ||
		call.ResolvedInnerParentPath != "/uploads" ||
		call.Filename != "webdav-upload.txt" ||
		call.ObjectPath != "/uploads/webdav-upload.txt" ||
		call.Size != int64(len("webdav-content")) ||
		call.ActorID == nil ||
		*call.ActorID != 7 {
		t.Fatalf("unexpected metadata commit call = %+v", call)
	}
}

func TestUploadFinishCommitsMetadataVFSFileObject(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	uploadRepo := gorm.NewUploadSessionRepository(db)
	nodeRepo := gorm.NewVFSNodeRepository(db)
	objectRepo := gorm.NewStorageObjectRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "metadata 上传目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/media",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	options := DefaultSystemOptions()
	options.TempDir = filepath.Join(t.TempDir(), "upload-temp")
	options.DefaultChunkSize = 5
	options.MaxUploadSize = 1024
	committer := NewMetadataVFSCommitService(nodeRepo, objectRepo, WithMetadataVFSCommitTransactor(gorm.NewTransactor(db)))
	svc := NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		WithUploadVFSResolver(NewVFSService(sourceRepo)),
		WithUploadMetadataVFSCommitter(committer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 7, RoleKey: permission.RoleUser, Status: permission.StatusActive})

	initResp, err := svc.Init(ctx, 7, appdto.UploadInitRequest{
		TargetVirtualParentPath: "/media/uploads",
		Filename:                "movie.txt",
		FileSize:                int64(len("hello-world")),
		FileHash:                "5eb63bbbe01eeed093cb22bb8f5acdc3",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 0, []byte("hello")); err != nil {
		t.Fatalf("UploadChunk(0) error = %v", err)
	}
	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 1, []byte("-worl")); err != nil {
		t.Fatalf("UploadChunk(1) error = %v", err)
	}
	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 2, []byte("d")); err != nil {
		t.Fatalf("UploadChunk(2) error = %v", err)
	}

	finishResp, err := svc.Finish(ctx, appdto.UploadFinishRequest{UploadID: initResp.Upload.UploadID})
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if !finishResp.Completed || finishResp.File.Path != "/uploads/movie.txt" {
		t.Fatalf("unexpected finish response = %+v", finishResp)
	}

	node, err := nodeRepo.FindByPath(context.Background(), "/media/uploads/movie.txt")
	if err != nil {
		t.Fatalf("nodeRepo.FindByPath(file) error = %v", err)
	}
	if node.Kind != entity.VFSNodeKindFile || node.ObjectID == nil || node.Checksum != "5eb63bbbe01eeed093cb22bb8f5acdc3" {
		t.Fatalf("unexpected metadata file node = %+v", node)
	}
	if finishResp.ResultVFSNodeID != node.ID {
		t.Fatalf("expected finish result_vfs_node_id=%d, got %+v", node.ID, finishResp)
	}
	object, err := objectRepo.FindByID(context.Background(), *node.ObjectID)
	if err != nil {
		t.Fatalf("objectRepo.FindByID() error = %v", err)
	}
	if object.SourceID != source.ID || object.DriverType != "local" || object.LocatorType != "local_path" ||
		!strings.Contains(object.LocatorJSON, `"/uploads/movie.txt"`) ||
		object.Status != entity.StorageObjectStatusAvailable ||
		object.Size != int64(len("hello-world")) {
		t.Fatalf("unexpected storage object = %+v", object)
	}
}

func TestUploadMetadataCommitFailureDoesNotReturnCompletedOrSaveResult(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	uploadRepo := gorm.NewUploadSessionRepository(db)
	operationRepo := gorm.NewVFSOperationRepository(db)

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "upload metadata failure",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/media",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	commitErr := errors.New(`metadata commit failed at /tmp/yunxia/private token=top-secret`)
	committer := &failingMetadataVFSCommitter{err: commitErr}
	journal := NewVFSOperationJournalService(operationRepo)
	options := DefaultSystemOptions()
	options.TempDir = filepath.Join(t.TempDir(), "upload-temp")
	options.DefaultChunkSize = 5
	options.MaxUploadSize = 1024
	svc := NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		WithUploadMetadataVFSCommitter(committer),
		WithUploadOperationJournal(journal),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 7, RoleKey: permission.RoleUser, Status: permission.StatusActive})

	initResp, err := svc.Init(ctx, 7, appdto.UploadInitRequest{
		SourceID: source.ID,
		Path:     "/uploads",
		Filename: "movie.txt",
		FileSize: int64(len("hello-world")),
		FileHash: "5eb63bbbe01eeed093cb22bb8f5acdc3",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 0, []byte("hello")); err != nil {
		t.Fatalf("UploadChunk(0) error = %v", err)
	}
	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 1, []byte("-worl")); err != nil {
		t.Fatalf("UploadChunk(1) error = %v", err)
	}
	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 2, []byte("d")); err != nil {
		t.Fatalf("UploadChunk(2) error = %v", err)
	}

	finishResp, err := svc.Finish(ctx, appdto.UploadFinishRequest{UploadID: initResp.Upload.UploadID})
	if !errors.Is(err, ErrMetadataVFSCommitFailed) {
		t.Fatalf("Finish() error = %v, want ErrMetadataVFSCommitFailed", err)
	}
	if finishResp != nil {
		t.Fatalf("metadata commit failure must not return completed response, got %+v", finishResp)
	}
	stored, err := uploadRepo.FindByID(context.Background(), initResp.Upload.UploadID)
	if err != nil {
		t.Fatalf("uploadRepo.FindByID() error = %v", err)
	}
	if stored.Status == "completed" || stored.ResultVFSNodeID != 0 {
		t.Fatalf("upload session should remain non-completed without result node, got %+v", stored)
	}
	operations, err := operationRepo.ListDue(context.Background(), domainrepo.VFSOperationDueFilter{
		OperationType: entity.VFSOperationTypeUploadCommit,
		DueBefore:     time.Now().Add(time.Hour),
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("operationRepo.ListDue() error = %v", err)
	}
	if len(operations) != 1 {
		t.Fatalf("expected one upload_commit journal operation, got %+v", operations)
	}
	operation := operations[0]
	if operation.Status != entity.VFSOperationStatusFailed ||
		operation.ErrorCode != "METADATA_VFS_COMMIT_FAILED" ||
		operation.ErrorMessage != ErrMetadataVFSCommitFailed.Error() ||
		strings.Contains(operation.PayloadJSON, "top-secret") ||
		strings.Contains(operation.PayloadJSON, "/tmp/yunxia/private") {
		t.Fatalf("unexpected sanitized upload_commit operation = %+v", operation)
	}
}

func TestUploadLocalFastUploadCommitsMetadataVFSFileObject(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	uploadRepo := gorm.NewUploadSessionRepository(db)

	basePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(basePath, "uploads"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(basePath, "uploads", "movie.txt"), []byte("hello-world"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "metadata 本地秒传目标",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/media",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	committer := &recordingMetadataVFSCommitter{}
	options := DefaultSystemOptions()
	options.TempDir = filepath.Join(t.TempDir(), "upload-temp")
	options.DefaultChunkSize = 5
	options.MaxUploadSize = 1024
	svc := NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		WithUploadMetadataVFSCommitter(committer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 7, RoleKey: permission.RoleUser, Status: permission.StatusActive})

	resp, err := svc.Init(ctx, 7, appdto.UploadInitRequest{
		SourceID: source.ID,
		Path:     "/uploads",
		Filename: "movie.txt",
		FileSize: int64(len("hello-world")),
		FileHash: "5eb63bbbe01eeed093cb22bb8f5acdc3",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if !resp.IsFastUpload || resp.File == nil {
		t.Fatalf("expected local fast-upload response, got %+v", resp)
	}
	if len(committer.calls) != 1 {
		t.Fatalf("expected one metadata commit after local fast-upload, got %+v", committer.calls)
	}
	call := committer.calls[0]
	if call.Source == nil ||
		call.Source.ID != source.ID ||
		call.Source.DriverType != source.DriverType ||
		call.VirtualParentPath != "/media/uploads" ||
		call.ResolvedInnerParentPath != "/uploads" ||
		call.Filename != "movie.txt" ||
		call.ObjectPath != "/uploads/movie.txt" ||
		call.Size != int64(len("hello-world")) ||
		call.Checksum != "5eb63bbbe01eeed093cb22bb8f5acdc3" ||
		call.ActorID == nil ||
		*call.ActorID != 7 {
		t.Fatalf("unexpected local fast-upload metadata commit call = %+v", call)
	}
}

func TestUploadServiceUsesDirectUploadDriverForNonLocalDriver(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	uploadRepo := gorm.NewUploadSessionRepository(db)
	source := &entity.StorageSource{
		Name:       "直传源",
		DriverType: "cloud-direct",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/direct",
		RootPath:   "/",
		ConfigJSON: "{}",
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	expiresAt := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	driver := &recordingUploadDriver{
		initPlan: &MultipartUploadPlan{
			State: MultipartUploadState{
				RemoteUploadID: "remote-upload-1",
				ObjectKey:      "object-key",
				VirtualPath:    "/uploads/movie.mkv",
				FileSize:       int64(len("hello-world")),
			},
			PartInstructions: []MultipartUploadPartInstruction{
				{
					Index:     0,
					Method:    "PUT",
					URL:       "https://upload.example.test/object-key",
					Headers:   map[string]string{"Content-Type": "video/x-matroska"},
					ByteStart: 0,
					ByteEnd:   int64(len("hello-world")) - 1,
					ExpiresAt: expiresAt,
				},
			},
		},
		completeEntry: &StorageEntry{
			Name:       "movie.mkv",
			Path:       "/uploads/movie.mkv",
			Size:       int64(len("hello-world")),
			ModifiedAt: expiresAt,
		},
	}
	options := DefaultSystemOptions()
	options.TempDir = filepath.Join(t.TempDir(), "upload-temp")
	options.DefaultChunkSize = 5
	options.MaxUploadSize = 1024
	svc := NewUploadService(sourceRepo, uploadRepo, options, WithUploadDriver("cloud-direct", driver))
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 7, RoleKey: permission.RoleUser, Status: permission.StatusActive})

	initResp, err := svc.Init(ctx, 7, appdto.UploadInitRequest{
		SourceID: source.ID,
		Path:     "/uploads",
		Filename: "movie.mkv",
		FileSize: int64(len("hello-world")),
		FileHash: "gcid:0123456789abcdef0123456789abcdef01234567",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if len(driver.initCalls) != 1 {
		t.Fatalf("expected one direct upload init call, got %+v", driver.initCalls)
	}
	initCall := driver.initCalls[0]
	if initCall.VirtualPath != "/uploads" || initCall.Filename != "movie.mkv" ||
		initCall.ContentHash != "gcid:0123456789abcdef0123456789abcdef01234567" ||
		initCall.FileSize != int64(len("hello-world")) {
		t.Fatalf("unexpected direct upload init request = %+v", initCall)
	}
	if initResp.Transport == nil || initResp.Transport.Mode != "direct_parts" || initResp.Transport.DriverType != "cloud-direct" {
		t.Fatalf("unexpected direct upload transport = %+v", initResp.Transport)
	}
	if initResp.Upload == nil || initResp.Upload.TotalChunks != 1 {
		t.Fatalf("expected one persisted direct upload session chunk, got %+v", initResp.Upload)
	}
	if len(initResp.PartInstructions) != 1 ||
		initResp.PartInstructions[0].Method != "PUT" ||
		initResp.PartInstructions[0].URL != "https://upload.example.test/object-key" {
		t.Fatalf("unexpected direct part instructions = %+v", initResp.PartInstructions)
	}

	finishResp, err := svc.Finish(ctx, appdto.UploadFinishRequest{
		UploadID: initResp.Upload.UploadID,
		Parts:    []appdto.UploadPartETag{{Index: 0, ETag: "etag-1"}},
	})
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if finishResp.File.Path != "/uploads/movie.mkv" || finishResp.File.Size != int64(len("hello-world")) {
		t.Fatalf("unexpected direct upload finish file = %+v", finishResp.File)
	}
	if len(driver.completeCalls) != 1 {
		t.Fatalf("expected one direct complete call, got %+v", driver.completeCalls)
	}
	completeCall := driver.completeCalls[0]
	if completeCall.state.RemoteUploadID != "remote-upload-1" ||
		completeCall.state.VirtualPath != "/uploads/movie.mkv" ||
		len(completeCall.parts) != 1 ||
		completeCall.parts[0].ETag != "etag-1" {
		t.Fatalf("unexpected direct complete call = %+v", completeCall)
	}
	if _, err := uploadRepo.FindByID(context.Background(), initResp.Upload.UploadID); !errors.Is(err, domainrepo.ErrNotFound) {
		t.Fatalf("expected upload session to be deleted after direct finish, err=%v", err)
	}
}

func TestUploadDirectFinishCommitsMetadataVFSFileObject(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	uploadRepo := gorm.NewUploadSessionRepository(db)
	source := &entity.StorageSource{
		Name:       "直传 metadata 源",
		DriverType: "cloud-direct-metadata",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/direct",
		RootPath:   "/",
		ConfigJSON: "{}",
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	expiresAt := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	driver := &recordingUploadDriver{
		initPlan: &MultipartUploadPlan{
			State: MultipartUploadState{
				RemoteUploadID: "remote-upload-metadata",
				ObjectKey:      "object-key",
				VirtualPath:    "/uploads/direct.bin",
				FileSize:       int64(len("hello-world")),
			},
			PartInstructions: []MultipartUploadPartInstruction{{
				Index:     0,
				Method:    "PUT",
				URL:       "https://upload.example.test/object-key",
				ByteStart: 0,
				ByteEnd:   int64(len("hello-world")) - 1,
				ExpiresAt: expiresAt,
			}},
		},
		completeEntry: &StorageEntry{
			Name:       "direct.bin",
			Path:       "/uploads/direct.bin",
			Size:       int64(len("hello-world")),
			ETag:       "etag-complete",
			ModifiedAt: expiresAt,
		},
	}
	committer := &recordingMetadataVFSCommitter{}
	options := DefaultSystemOptions()
	options.TempDir = filepath.Join(t.TempDir(), "upload-temp")
	options.DefaultChunkSize = 5
	options.MaxUploadSize = 1024
	svc := NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		WithUploadDriver("cloud-direct-metadata", driver),
		WithUploadMetadataVFSCommitter(committer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 7, RoleKey: permission.RoleUser, Status: permission.StatusActive})

	initResp, err := svc.Init(ctx, 7, appdto.UploadInitRequest{
		SourceID: source.ID,
		Path:     "/uploads",
		Filename: "direct.bin",
		FileSize: int64(len("hello-world")),
		FileHash: "driver-hash",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.Finish(ctx, appdto.UploadFinishRequest{
		UploadID: initResp.Upload.UploadID,
		Parts:    []appdto.UploadPartETag{{Index: 0, ETag: "etag-1"}},
	}); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if len(committer.calls) != 1 {
		t.Fatalf("expected one metadata commit after direct upload, got %+v", committer.calls)
	}
	call := committer.calls[0]
	if call.Source == nil ||
		call.Source.ID != source.ID ||
		call.Source.DriverType != source.DriverType ||
		call.VirtualParentPath != "/direct/uploads" ||
		call.ResolvedInnerParentPath != "/uploads" ||
		call.Filename != "direct.bin" ||
		call.ObjectPath != "/uploads/direct.bin" ||
		call.Size != int64(len("hello-world")) ||
		call.ETag != "etag-complete" ||
		call.Checksum != "driver-hash" ||
		call.ActorID == nil ||
		*call.ActorID != 7 {
		t.Fatalf("unexpected direct metadata commit call = %+v", call)
	}
}

func TestUploadServiceFallsBackToImportDriverWhenDirectUploadUnsupported(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	uploadRepo := gorm.NewUploadSessionRepository(db)
	source := &entity.StorageSource{
		Name:       "混合上传源",
		DriverType: "cloud-hybrid",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/hybrid",
		RootPath:   "/",
		ConfigJSON: "{}",
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	uploadDriver := &recordingUploadDriver{initErr: ErrSourceOperationUnsupported}
	importer := &recordingTaskImportDriver{}
	options := DefaultSystemOptions()
	options.TempDir = filepath.Join(t.TempDir(), "upload-temp")
	options.DefaultChunkSize = 5
	options.MaxUploadSize = 1024
	svc := NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		WithUploadDriver("cloud-hybrid", uploadDriver),
		WithUploadImportDriver("cloud-hybrid", importer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 7, RoleKey: permission.RoleUser, Status: permission.StatusActive})

	initResp, err := svc.Init(ctx, 7, appdto.UploadInitRequest{
		SourceID: source.ID,
		Path:     "/fallback",
		Filename: "fallback.txt",
		FileSize: int64(len("hello-world")),
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if len(uploadDriver.initCalls) != 1 {
		t.Fatalf("expected direct init to be attempted before fallback, got %+v", uploadDriver.initCalls)
	}
	if initResp.Transport == nil || initResp.Transport.Mode != "server_chunk" || initResp.Transport.DriverType != "cloud-hybrid" {
		t.Fatalf("unexpected fallback transport = %+v", initResp.Transport)
	}
	if len(initResp.PartInstructions) != 0 {
		t.Fatalf("fallback server_chunk should not return direct part instructions")
	}

	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 0, []byte("hello")); err != nil {
		t.Fatalf("UploadChunk(0) error = %v", err)
	}
	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 1, []byte("-worl")); err != nil {
		t.Fatalf("UploadChunk(1) error = %v", err)
	}
	if _, err := svc.UploadChunk(ctx, initResp.Upload.UploadID, 2, []byte("d")); err != nil {
		t.Fatalf("UploadChunk(2) error = %v", err)
	}

	finishResp, err := svc.Finish(ctx, appdto.UploadFinishRequest{UploadID: initResp.Upload.UploadID})
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if finishResp.File.Path != "/fallback/fallback.txt" || finishResp.File.Size != int64(len("hello-world")) {
		t.Fatalf("unexpected fallback finish file = %+v", finishResp.File)
	}
	if len(importer.calls) != 1 {
		t.Fatalf("expected one fallback import call, got %+v", importer.calls)
	}
	call := importer.calls[0]
	if call.sourceID != source.ID || call.targetPath != "/fallback/fallback.txt" || string(call.content) != "hello-world" {
		t.Fatalf("unexpected fallback import call = %+v", call)
	}
}

func TestUploadServiceReturnsFastUploadWhenDriverCompletesOnInit(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	uploadRepo := gorm.NewUploadSessionRepository(db)
	source := &entity.StorageSource{
		Name:       "秒传源",
		DriverType: "cloud-fast",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/fast",
		RootPath:   "/",
		ConfigJSON: "{}",
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	driver := &recordingUploadDriver{
		initPlan: &MultipartUploadPlan{
			CompletedEntry: &StorageEntry{
				Name:       "instant.bin",
				Path:       "/instant.bin",
				Size:       4,
				ModifiedAt: time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC),
			},
		},
	}
	options := DefaultSystemOptions()
	options.TempDir = filepath.Join(t.TempDir(), "upload-temp")
	options.MaxUploadSize = 1024
	svc := NewUploadService(sourceRepo, uploadRepo, options, WithUploadDriver("cloud-fast", driver))
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 7, RoleKey: permission.RoleUser, Status: permission.StatusActive})

	initResp, err := svc.Init(ctx, 7, appdto.UploadInitRequest{
		SourceID: source.ID,
		Path:     "/",
		Filename: "instant.bin",
		FileSize: 4,
		FileHash: "0123456789abcdef0123456789abcdef01234567",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if !initResp.IsFastUpload || initResp.Upload != nil || initResp.Transport != nil {
		t.Fatalf("expected fast upload response without persisted session, got %+v", initResp)
	}
	if initResp.File == nil || initResp.File.Path != "/instant.bin" || initResp.File.SourceID != source.ID {
		t.Fatalf("unexpected fast upload file = %+v", initResp.File)
	}
	if len(driver.completeCalls) != 0 {
		t.Fatalf("fast upload should not call CompleteMultipartUpload, got %+v", driver.completeCalls)
	}
}

func TestUploadFastUploadCommitsMetadataVFSFileObject(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	uploadRepo := gorm.NewUploadSessionRepository(db)
	source := &entity.StorageSource{
		Name:       "秒传 metadata 源",
		DriverType: "cloud-fast-metadata",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/fast",
		RootPath:   "/",
		ConfigJSON: "{}",
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	driver := &recordingUploadDriver{
		initPlan: &MultipartUploadPlan{
			CompletedEntry: &StorageEntry{
				Name:       "instant.bin",
				Path:       "/instant.bin",
				Size:       4,
				ETag:       "etag-instant",
				ModifiedAt: time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC),
			},
		},
	}
	committer := &recordingMetadataVFSCommitter{}
	options := DefaultSystemOptions()
	options.TempDir = filepath.Join(t.TempDir(), "upload-temp")
	options.MaxUploadSize = 1024
	svc := NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		WithUploadDriver("cloud-fast-metadata", driver),
		WithUploadMetadataVFSCommitter(committer),
	)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 7, RoleKey: permission.RoleUser, Status: permission.StatusActive})

	resp, err := svc.Init(ctx, 7, appdto.UploadInitRequest{
		SourceID: source.ID,
		Path:     "/",
		Filename: "instant.bin",
		FileSize: 4,
		FileHash: "0123456789abcdef0123456789abcdef01234567",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if !resp.IsFastUpload {
		t.Fatalf("expected fast upload response, got %+v", resp)
	}
	if len(committer.calls) != 1 {
		t.Fatalf("expected one metadata commit after fast upload, got %+v", committer.calls)
	}
	call := committer.calls[0]
	if call.Source == nil ||
		call.Source.ID != source.ID ||
		call.Source.DriverType != source.DriverType ||
		call.VirtualParentPath != "/fast" ||
		call.ResolvedInnerParentPath != "/" ||
		call.Filename != "instant.bin" ||
		call.ObjectPath != "/instant.bin" ||
		call.Size != 4 ||
		call.ETag != "etag-instant" ||
		call.Checksum != "0123456789abcdef0123456789abcdef01234567" ||
		call.ActorID == nil ||
		*call.ActorID != 7 {
		t.Fatalf("unexpected fast-upload metadata commit call = %+v", call)
	}
}

func TestSystemStatsUsesCapacityDriverBeforeRecursiveFileStats(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	source := &entity.StorageSource{
		Name:       "容量源",
		DriverType: "quota",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/quota",
		RootPath:   "/",
		ConfigJSON: "{}",
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	used := int64(12345)
	fileDriver := &storageFileDriverStub{listErr: errors.New("recursive stats should not be called")}
	svc := NewSystemService(
		gorm.NewSystemConfigRepository(db),
		DefaultSystemOptions(),
		WithSystemStatsDependencies(nil, sourceRepo, nil),
		WithSystemStatsCapacityDriver("quota", capacityDriverStub{used: &used}),
		WithSystemStatsFileDriver("quota", fileDriver),
	)
	stats, err := svc.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.StorageUsedBytes != used {
		t.Fatalf("expected capacity used bytes %d, got %+v", used, stats)
	}
	if fileDriver.listCalls != 0 {
		t.Fatalf("expected capacity driver to bypass recursive List, calls=%d", fileDriver.listCalls)
	}
}

func TestSystemStatsFallsBackToRecursiveFileStatsWhenCapacityUsageUnavailable(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	source := &entity.StorageSource{
		Name:       "容量缺失源",
		DriverType: "quota-missing",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/quota-missing",
		RootPath:   "/",
		ConfigJSON: "{}",
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	fileDriver := &storageFileDriverStub{entriesByPath: map[string][]StorageEntry{
		"/": {
			{Name: "movie.mkv", Path: "/movie.mkv", Size: 11},
			{Name: "nested", Path: "/nested", IsDir: true},
		},
		"/nested": {
			{Name: "episode.mkv", Path: "/nested/episode.mkv", Size: 22},
		},
	}}
	svc := NewSystemService(
		gorm.NewSystemConfigRepository(db),
		DefaultSystemOptions(),
		WithSystemStatsDependencies(nil, sourceRepo, nil),
		WithSystemStatsCapacityDriver("quota-missing", capacityDriverStub{}),
		WithSystemStatsFileDriver("quota-missing", fileDriver),
	)
	stats, err := svc.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.FilesTotal != 2 || stats.StorageUsedBytes != 33 {
		t.Fatalf("expected recursive fallback stats files=2 bytes=33, got %+v", stats)
	}
	if fileDriver.listCalls != 2 {
		t.Fatalf("expected recursive List fallback for root and child, calls=%d", fileDriver.listCalls)
	}
}

func TestACLServiceManagementLifecycle(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	userRepo := gorm.NewUserRepository(db)
	refreshRepo := gorm.NewRefreshTokenRepository(db)
	configRepo := gorm.NewSystemConfigRepository(db)
	sourceRepo := gorm.NewSourceRepository(db)
	hasher := security.NewBcryptHasher(4)
	tokenSvc := security.NewJWTTokenService("test-secret", 15*time.Minute, 7*24*time.Hour)
	options := DefaultSystemOptions()
	root := t.TempDir()
	options.StorageDataDir = filepath.Join(root, "storage")
	options.TempDir = filepath.Join(root, "temp")

	setupSvc := NewSetupService(userRepo, refreshRepo, configRepo, sourceRepo, hasher, tokenSvc, options)
	if _, err := setupSvc.Init(context.Background(), appdto.SetupInitRequest{
		Username: "admin",
		Password: "strong-password-123",
		Email:    "admin@example.com",
	}); err != nil {
		t.Fatalf("setup Init() error = %v", err)
	}

	normalUser := &entity.User{
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "hashed",
		RoleKey:      "user",
		Status:       "active",
		TokenVersion: 0,
	}
	if err := userRepo.Create(context.Background(), normalUser); err != nil {
		t.Fatalf("userRepo.Create(normalUser) error = %v", err)
	}

	sources, err := sourceRepo.ListAll(context.Background())
	if err != nil {
		t.Fatalf("sourceRepo.ListAll() error = %v", err)
	}
	if len(sources) == 0 {
		t.Fatalf("expected default source after setup")
	}
	sourceID := sources[0].ID
	expectedVirtualPath := mergeMountAndInnerPath(sources[0].MountPath, "/projects")

	aclRepo := gorm.NewACLRuleRepository(db)
	svc := NewACLService(sourceRepo, userRepo, aclRepo)

	created, err := svc.Create(context.Background(), appdto.CreateACLRuleRequest{
		SourceID:    sourceID,
		Path:        "/projects",
		SubjectType: "user",
		SubjectID:   normalUser.ID,
		Effect:      "allow",
		Priority:    100,
		Permissions: appdto.ACLPermissions{
			Read:   true,
			Write:  true,
			Delete: false,
			Share:  false,
		},
		InheritToChildren: true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Path != "/projects" || created.VirtualPath != expectedVirtualPath || created.SubjectType != "user" || !created.Permissions.Read || !created.Permissions.Write {
		t.Fatalf("unexpected created acl rule = %+v", created)
	}

	listed, err := svc.List(context.Background(), appdto.ACLRuleListQuery{
		SourceID: sourceID,
		Path:     "/projects",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != created.ID {
		t.Fatalf("unexpected listed acl rules = %+v", listed.Items)
	}
	if listed.Items[0].VirtualPath != expectedVirtualPath {
		t.Fatalf("expected listed acl virtual_path=%s, got %+v", expectedVirtualPath, listed.Items[0])
	}

	updated, err := svc.Update(context.Background(), created.ID, appdto.UpdateACLRuleRequest{
		Path:        "/projects",
		SubjectType: "user",
		SubjectID:   normalUser.ID,
		Effect:      "deny",
		Priority:    150,
		Permissions: appdto.ACLPermissions{
			Read:   true,
			Write:  false,
			Delete: false,
			Share:  false,
		},
		InheritToChildren: false,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Effect != "deny" || updated.Priority != 150 || updated.InheritToChildren || updated.VirtualPath != expectedVirtualPath {
		t.Fatalf("unexpected updated acl rule = %+v", updated)
	}

	if err := svc.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	listed, err = svc.List(context.Background(), appdto.ACLRuleListQuery{
		SourceID: sourceID,
		Path:     "/projects",
	})
	if err != nil {
		t.Fatalf("List() after delete error = %v", err)
	}
	if len(listed.Items) != 0 {
		t.Fatalf("expected empty acl rule list after delete, got %+v", listed.Items)
	}
}

func TestACLAuthorizerPrefersVirtualPathRule(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	configRepo := gorm.NewSystemConfigRepository(db)
	sourceRepo := gorm.NewSourceRepository(db)
	aclRepo := gorm.NewACLRuleRepository(db)

	configJSON, err := marshalLocalSourceConfig(t.TempDir())
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}

	source := &entity.StorageSource{
		Name:            "挂载文档",
		DriverType:      "local",
		Status:          "online",
		IsEnabled:       true,
		IsWebDAVExposed: false,
		WebDAVReadOnly:  true,
		WebDAVSlug:      "mounted-docs",
		MountPath:       "/mounted",
		RootPath:        "/",
		SortOrder:       0,
		ConfigJSON:      configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	if err := configRepo.Upsert(context.Background(), &entity.SystemConfig{
		SiteName:         "测试",
		MultiUserEnabled: true,
		MaxUploadSize:    1024,
		DefaultChunkSize: 256,
		WebDAVEnabled:    true,
		WebDAVPrefix:     "/dav",
		Theme:            "system",
		Language:         "zh-CN",
		TimeZone:         "Asia/Shanghai",
	}); err != nil {
		t.Fatalf("configRepo.Upsert() error = %v", err)
	}

	rule := &entity.ACLRule{
		SourceID:          source.ID,
		Path:              "/legacy-mismatch",
		VirtualPath:       "/mounted/docs",
		SubjectType:       "user",
		SubjectID:         7,
		Effect:            "allow",
		Priority:          100,
		Read:              true,
		InheritToChildren: true,
	}
	if err := aclRepo.Create(context.Background(), rule); err != nil {
		t.Fatalf("aclRepo.Create() error = %v", err)
	}

	authorizer := NewACLAuthorizer(configRepo, aclRepo, sourceRepo)
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       7,
		RoleKey:      permission.RoleUser,
		Status:       permission.StatusActive,
		Capabilities: []string{},
	})
	if err := authorizer.AuthorizePath(ctx, source.ID, "/docs/spec.md", ACLActionRead); err != nil {
		t.Fatalf("expected virtual_path rule to allow mounted path, got %v", err)
	}
}

func TestACLServiceCreateWithVFSNodeIDPersistsSnapshot(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	ctx := context.Background()
	sourceRepo := gorm.NewSourceRepository(db)
	userRepo := gorm.NewUserRepository(db)
	aclRepo := gorm.NewACLRuleRepository(db)
	nodeRepo := gorm.NewVFSNodeRepository(db)
	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSTransactor(gorm.NewTransactor(db)))

	source := createACLNodeFirstSourceForTest(t, sourceRepo, "/docs")
	user := createACLNodeFirstUserForTest(t, userRepo, "acl-node-snapshot")
	root, err := metadataSvc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	docs := createACLNodeFirstNodeForTest(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "docs",
		Path:      "/docs",
		Kind:      entity.VFSNodeKindMount,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})
	team := createACLNodeFirstNodeForTest(t, nodeRepo, &entity.VFSNode{
		ParentID:  &docs.ID,
		Name:      "team",
		Path:      "/docs/team",
		Kind:      entity.VFSNodeKindDir,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})

	svc := NewACLService(sourceRepo, userRepo, aclRepo, WithACLMetadataReader(metadataSvc))
	created, err := svc.Create(ctx, appdto.CreateACLRuleRequest{
		VFSNodeID:   &team.ID,
		SubjectType: "user",
		SubjectID:   user.ID,
		Effect:      "allow",
		Priority:    100,
		Permissions: appdto.ACLPermissions{Read: true},
	})
	if err != nil {
		t.Fatalf("Create(node-bound) error = %v", err)
	}
	if created.SourceID != source.ID || created.VFSNodeID == nil || *created.VFSNodeID != team.ID {
		t.Fatalf("expected source/node snapshot, got %+v", created)
	}
	if created.Path != "/team" || created.VirtualPath != "/docs/team" {
		t.Fatalf("expected inner/virtual snapshots, got %+v", created)
	}
	persisted, err := aclRepo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID(created) error = %v", err)
	}
	if persisted.VFSNodeID == nil || *persisted.VFSNodeID != team.ID {
		t.Fatalf("expected persisted vfs_node_id=%d, got %#v", team.ID, persisted)
	}

	compatCreated, err := svc.Create(ctx, appdto.CreateACLRuleRequest{
		SourceID:    source.ID,
		Path:        "/team",
		SubjectType: "user",
		SubjectID:   user.ID,
		Effect:      "allow",
		Priority:    90,
		Permissions: appdto.ACLPermissions{Read: true},
	})
	if err != nil {
		t.Fatalf("Create(source/path compat) error = %v", err)
	}
	if compatCreated.VFSNodeID == nil || *compatCreated.VFSNodeID != team.ID {
		t.Fatalf("expected source/path create to resolve vfs_node_id=%d, got %+v", team.ID, compatCreated)
	}
}

func TestACLAuthorizerNodeFirstFollowsNodeAndRecomputesInheritance(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	ctx := context.Background()
	configRepo := gorm.NewSystemConfigRepository(db)
	sourceRepo := gorm.NewSourceRepository(db)
	userRepo := gorm.NewUserRepository(db)
	aclRepo := gorm.NewACLRuleRepository(db)
	nodeRepo := gorm.NewVFSNodeRepository(db)
	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSTransactor(gorm.NewTransactor(db)))

	if err := configRepo.Upsert(ctx, aclNodeFirstSystemConfigForTest()); err != nil {
		t.Fatalf("configRepo.Upsert() error = %v", err)
	}
	source := createACLNodeFirstSourceForTest(t, sourceRepo, "/")
	user := createACLNodeFirstUserForTest(t, userRepo, "acl-node-reader")
	root, err := metadataSvc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	root.SourceID = &source.ID
	if err := nodeRepo.Update(ctx, root); err != nil {
		t.Fatalf("nodeRepo.Update(root source) error = %v", err)
	}
	team := createACLNodeFirstNodeForTest(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "team",
		Path:      "/team",
		Kind:      entity.VFSNodeKindDir,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})
	archive := createACLNodeFirstNodeForTest(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "archive",
		Path:      "/archive",
		Kind:      entity.VFSNodeKindDir,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})
	secret := createACLNodeFirstNodeForTest(t, nodeRepo, &entity.VFSNode{
		ParentID:  &team.ID,
		Name:      "secret.txt",
		Path:      "/team/secret.txt",
		Kind:      entity.VFSNodeKindFile,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})
	inherited := createACLNodeFirstNodeForTest(t, nodeRepo, &entity.VFSNode{
		ParentID:  &team.ID,
		Name:      "inherited.txt",
		Path:      "/team/inherited.txt",
		Kind:      entity.VFSNodeKindFile,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})

	createACLRuleEntityForTest(t, aclRepo, &entity.ACLRule{
		SourceID:          source.ID,
		VFSNodeID:         &team.ID,
		Path:              "/team",
		VirtualPath:       "/team",
		SubjectType:       "user",
		SubjectID:         user.ID,
		Effect:            "allow",
		Priority:          100,
		Read:              true,
		InheritToChildren: true,
	})
	createACLRuleEntityForTest(t, aclRepo, &entity.ACLRule{
		SourceID:    source.ID,
		VFSNodeID:   &secret.ID,
		Path:        "/team/secret.txt",
		VirtualPath: "/team/secret.txt",
		SubjectType: "user",
		SubjectID:   user.ID,
		Effect:      "allow",
		Priority:    100,
		Read:        true,
	})

	authorizer := NewACLAuthorizer(configRepo, aclRepo, sourceRepo, WithACLAuthorizerMetadataReader(metadataSvc))
	authCtx := security.WithRequestAuth(ctx, security.RequestAuth{
		UserID:  user.ID,
		RoleKey: permission.RoleUser,
		Status:  permission.StatusActive,
	})
	if err := authorizer.AuthorizePath(authCtx, source.ID, "/team/inherited.txt", ACLActionRead); err != nil {
		t.Fatalf("expected inherited node under /team to be readable, got %v", err)
	}
	if _, _, err := metadataSvc.Move(ctx, MetadataVFSMoveRequest{Path: inherited.Path, TargetParentPath: archive.Path}); err != nil {
		t.Fatalf("Move(inherited to archive) error = %v", err)
	}
	if err := authorizer.AuthorizePath(authCtx, source.ID, "/archive/inherited.txt", ACLActionRead); !errors.Is(err, ErrACLDenied) {
		t.Fatalf("expected moved child to lose inherited ACL, got %v", err)
	}

	if _, _, err := metadataSvc.Move(ctx, MetadataVFSMoveRequest{Path: secret.Path, TargetParentPath: archive.Path}); err != nil {
		t.Fatalf("Move(secret to archive) error = %v", err)
	}
	if err := authorizer.AuthorizePath(authCtx, source.ID, "/archive/secret.txt", ACLActionRead); err != nil {
		t.Fatalf("expected explicit node-bound ACL to follow moved node, got %v", err)
	}
	updatedSecret, err := metadataSvc.ResolveNode(ctx, "/archive/secret.txt")
	if err != nil {
		t.Fatalf("ResolveNode(moved secret) error = %v", err)
	}
	createACLRuleEntityForTest(t, aclRepo, &entity.ACLRule{
		SourceID:    source.ID,
		VFSNodeID:   &updatedSecret.ID,
		Path:        "/archive/secret.txt",
		VirtualPath: "/archive/secret.txt",
		SubjectType: "user",
		SubjectID:   user.ID,
		Effect:      "deny",
		Priority:    100,
		Read:        true,
	})
	if err := authorizer.AuthorizePath(authCtx, source.ID, "/archive/secret.txt", ACLActionRead); !errors.Is(err, ErrACLDenied) {
		t.Fatalf("expected same-priority deny to beat allow, got %v", err)
	}
}

func TestACLAuthorizerNodeBoundRuleDoesNotAllowOldSnapshotAfterRename(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	ctx := context.Background()
	configRepo := gorm.NewSystemConfigRepository(db)
	sourceRepo := gorm.NewSourceRepository(db)
	userRepo := gorm.NewUserRepository(db)
	aclRepo := gorm.NewACLRuleRepository(db)
	nodeRepo := gorm.NewVFSNodeRepository(db)
	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSTransactor(gorm.NewTransactor(db)))

	if err := configRepo.Upsert(ctx, aclNodeFirstSystemConfigForTest()); err != nil {
		t.Fatalf("configRepo.Upsert() error = %v", err)
	}
	source := createACLNodeFirstSourceForTest(t, sourceRepo, "/")
	user := createACLNodeFirstUserForTest(t, userRepo, "acl-node-rename-reader")
	root, err := metadataSvc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	root.SourceID = &source.ID
	if err := nodeRepo.Update(ctx, root); err != nil {
		t.Fatalf("nodeRepo.Update(root source) error = %v", err)
	}
	file := createACLNodeFirstNodeForTest(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "old.txt",
		Path:      "/old.txt",
		Kind:      entity.VFSNodeKindFile,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})
	createACLRuleEntityForTest(t, aclRepo, &entity.ACLRule{
		SourceID:    source.ID,
		VFSNodeID:   &file.ID,
		Path:        "/old.txt",
		VirtualPath: "/old.txt",
		SubjectType: "user",
		SubjectID:   user.ID,
		Effect:      "allow",
		Priority:    100,
		Read:        true,
	})

	authorizer := NewACLAuthorizer(configRepo, aclRepo, sourceRepo, WithACLAuthorizerMetadataReader(metadataSvc))
	authCtx := security.WithRequestAuth(ctx, security.RequestAuth{
		UserID:  user.ID,
		RoleKey: permission.RoleUser,
		Status:  permission.StatusActive,
	})
	if _, _, err := metadataSvc.Rename(ctx, MetadataVFSRenameRequest{Path: "/old.txt", NewName: "new.txt"}); err != nil {
		t.Fatalf("Rename(old.txt) error = %v", err)
	}
	if err := authorizer.AuthorizePath(authCtx, source.ID, "/new.txt", ACLActionRead); err != nil {
		t.Fatalf("expected node-bound ACL to follow renamed node, got %v", err)
	}
	if err := authorizer.AuthorizePath(authCtx, source.ID, "/old.txt", ACLActionRead); !errors.Is(err, ErrACLDenied) {
		t.Fatalf("expected node-bound ACL not to allow old path snapshot after rename, got %v", err)
	}
}

func TestACLAuthorizerPathFallbackRemainsSnapshotBound(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	ctx := context.Background()
	configRepo := gorm.NewSystemConfigRepository(db)
	sourceRepo := gorm.NewSourceRepository(db)
	userRepo := gorm.NewUserRepository(db)
	aclRepo := gorm.NewACLRuleRepository(db)
	nodeRepo := gorm.NewVFSNodeRepository(db)
	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSTransactor(gorm.NewTransactor(db)))

	if err := configRepo.Upsert(ctx, aclNodeFirstSystemConfigForTest()); err != nil {
		t.Fatalf("configRepo.Upsert() error = %v", err)
	}
	source := createACLNodeFirstSourceForTest(t, sourceRepo, "/")
	user := createACLNodeFirstUserForTest(t, userRepo, "acl-path-reader")
	root, err := metadataSvc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	root.SourceID = &source.ID
	if err := nodeRepo.Update(ctx, root); err != nil {
		t.Fatalf("nodeRepo.Update(root source) error = %v", err)
	}
	folder := createACLNodeFirstNodeForTest(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "path-only",
		Path:      "/path-only",
		Kind:      entity.VFSNodeKindDir,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})
	_ = createACLNodeFirstNodeForTest(t, nodeRepo, &entity.VFSNode{
		ParentID:  &folder.ID,
		Name:      "file.txt",
		Path:      "/path-only/file.txt",
		Kind:      entity.VFSNodeKindFile,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})
	createACLRuleEntityForTest(t, aclRepo, &entity.ACLRule{
		SourceID:          source.ID,
		Path:              "/path-only",
		VirtualPath:       "/path-only",
		SubjectType:       "user",
		SubjectID:         user.ID,
		Effect:            "allow",
		Priority:          100,
		Read:              true,
		InheritToChildren: true,
	})

	authorizer := NewACLAuthorizer(configRepo, aclRepo, sourceRepo, WithACLAuthorizerMetadataReader(metadataSvc))
	authCtx := security.WithRequestAuth(ctx, security.RequestAuth{
		UserID:  user.ID,
		RoleKey: permission.RoleUser,
		Status:  permission.StatusActive,
	})
	if err := authorizer.AuthorizePath(authCtx, source.ID, "/path-only/file.txt", ACLActionRead); err != nil {
		t.Fatalf("expected path fallback rule to allow original snapshot, got %v", err)
	}
	if _, _, err := metadataSvc.Rename(ctx, MetadataVFSRenameRequest{Path: "/path-only", NewName: "renamed"}); err != nil {
		t.Fatalf("Rename(path-only) error = %v", err)
	}
	if err := authorizer.AuthorizePath(authCtx, source.ID, "/renamed/file.txt", ACLActionRead); !errors.Is(err, ErrACLDenied) {
		t.Fatalf("expected path fallback not to follow renamed node, got %v", err)
	}
}

func createACLNodeFirstSourceForTest(t *testing.T, sourceRepo domainrepo.SourceRepository, mountPath string) *entity.StorageSource {
	t.Helper()
	configJSON, err := marshalLocalSourceConfig(t.TempDir())
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	slug := "acl-node" + strings.ReplaceAll(strings.Trim(mountPath, "/"), "/", "-")
	if slug == "acl-node" {
		slug = "acl-node-root"
	}
	source := &entity.StorageSource{
		Name:            slug,
		DriverType:      "local",
		Status:          "online",
		IsEnabled:       true,
		IsWebDAVExposed: false,
		WebDAVReadOnly:  true,
		WebDAVSlug:      slug,
		MountPath:       mountPath,
		RootPath:        "/",
		SortOrder:       0,
		ConfigJSON:      configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create(%s) error = %v", mountPath, err)
	}
	return source
}

func createACLNodeFirstUserForTest(t *testing.T, userRepo domainrepo.UserRepository, username string) *entity.User {
	t.Helper()
	user := &entity.User{
		Username:     username,
		Email:        username + "@example.com",
		PasswordHash: "hashed",
		RoleKey:      permission.RoleUser,
		Status:       permission.StatusActive,
		TokenVersion: 0,
	}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("userRepo.Create(%s) error = %v", username, err)
	}
	return user
}

func createACLNodeFirstNodeForTest(t *testing.T, nodeRepo domainrepo.VFSNodeRepository, node *entity.VFSNode) *entity.VFSNode {
	t.Helper()
	now := time.Now().UTC()
	if node.SyncState == "" {
		node.SyncState = entity.VFSNodeSyncStateIndexed
	}
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	}
	if node.UpdatedAt.IsZero() {
		node.UpdatedAt = now
	}
	if node.IndexedAt == nil {
		node.IndexedAt = &now
	}
	if err := nodeRepo.Create(context.Background(), node); err != nil {
		t.Fatalf("nodeRepo.Create(%s) error = %v", node.Path, err)
	}
	return node
}

func createACLRuleEntityForTest(t *testing.T, aclRepo domainrepo.ACLRuleRepository, rule *entity.ACLRule) *entity.ACLRule {
	t.Helper()
	now := time.Now().UTC()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	if rule.UpdatedAt.IsZero() {
		rule.UpdatedAt = now
	}
	if err := aclRepo.Create(context.Background(), rule); err != nil {
		t.Fatalf("aclRepo.Create(%s) error = %v", rule.VirtualPath, err)
	}
	return rule
}

func aclNodeFirstSystemConfigForTest() *entity.SystemConfig {
	return &entity.SystemConfig{
		SiteName:         "测试",
		MultiUserEnabled: true,
		MaxUploadSize:    1024,
		DefaultChunkSize: 256,
		WebDAVEnabled:    true,
		WebDAVPrefix:     "/dav",
		Theme:            "system",
		Language:         "zh-CN",
		TimeZone:         "Asia/Shanghai",
	}
}

func newSourceServiceWithS3Fixture(t *testing.T) *SourceService {
	t.Helper()

	db, cleanup := openTestDB(t)
	t.Cleanup(cleanup)

	sourceRepo := gorm.NewSourceRepository(db)
	configRepo := gorm.NewSystemConfigRepository(db)

	cfgJSON, err := (infraStorage.S3Config{
		Endpoint:       "https://s3.example.com",
		Region:         "us-east-1",
		Bucket:         "media",
		BasePrefix:     "library",
		ForcePathStyle: true,
		AccessKey:      "AKIA-TEST-1234",
		SecretKey:      "secret-value",
	}).Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	source := &entity.StorageSource{
		Name:            "S3 媒体库",
		DriverType:      "s3",
		Status:          "online",
		IsEnabled:       true,
		IsWebDAVExposed: false,
		WebDAVReadOnly:  true,
		WebDAVSlug:      "s3-media",
		RootPath:        "/",
		SortOrder:       1,
		ConfigJSON:      cfgJSON,
		LastCheckedAt:   timePointer(time.Now()),
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	return NewSourceService(sourceRepo, configRepo)
}

type taskServiceTestDownloader struct{}

func (taskServiceTestDownloader) AddURI(context.Context, string, string) (string, error) {
	return "gid-test-owner", nil
}

func (taskServiceTestDownloader) TellStatus(context.Context, string) (*DownloadStatus, error) {
	return &DownloadStatus{Status: "running"}, nil
}

func (taskServiceTestDownloader) Pause(context.Context, string) error {
	return nil
}

func (taskServiceTestDownloader) Resume(context.Context, string) error {
	return nil
}

func (taskServiceTestDownloader) Remove(context.Context, string) error {
	return nil
}

type completedWritingDownloader struct {
	filename      string
	content       []byte
	files         map[string][]byte
	downloadSpeed int64
	etaSeconds    *int64
	errorMessage  *string
	addDir        string
}

func (d *completedWritingDownloader) AddURI(_ context.Context, _ string, dir string) (string, error) {
	d.addDir = dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for name, content := range d.stagedFiles() {
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return "", err
		}
	}
	return "gid-completed", nil
}

func (d *completedWritingDownloader) TellStatus(context.Context, string) (*DownloadStatus, error) {
	var size int64
	for _, content := range d.stagedFiles() {
		size += int64(len(content))
	}
	return &DownloadStatus{
		Status:         "completed",
		CompletedBytes: size,
		TotalBytes:     &size,
		DownloadSpeed:  d.downloadSpeed,
		ETASeconds:     d.etaSeconds,
		ErrorMessage:   d.errorMessage,
		DisplayName:    d.filename,
	}, nil
}

func (d *completedWritingDownloader) stagedFiles() map[string][]byte {
	if len(d.files) > 0 {
		return d.files
	}
	return map[string][]byte{d.filename: d.content}
}

func (d *completedWritingDownloader) Pause(context.Context, string) error {
	return nil
}

func (d *completedWritingDownloader) Resume(context.Context, string) error {
	return nil
}

func (d *completedWritingDownloader) Remove(context.Context, string) error {
	return nil
}

type fixedStatusDownloader struct {
	status string
}

func (d fixedStatusDownloader) AddURI(context.Context, string, string) (string, error) {
	return "gid-fixed-status", nil
}

func (d fixedStatusDownloader) TellStatus(context.Context, string) (*DownloadStatus, error) {
	return &DownloadStatus{Status: d.status}, nil
}

func (d fixedStatusDownloader) Pause(context.Context, string) error {
	return nil
}

func (d fixedStatusDownloader) Resume(context.Context, string) error {
	return nil
}

func (d fixedStatusDownloader) Remove(context.Context, string) error {
	return nil
}

type removeCountingDownloader struct {
	removeCalls int
	removeErr   error
}

func (d *removeCountingDownloader) AddURI(context.Context, string, string) (string, error) {
	return "gid-remove-counting", nil
}

func (d *removeCountingDownloader) TellStatus(context.Context, string) (*DownloadStatus, error) {
	return &DownloadStatus{Status: "running"}, nil
}

func (d *removeCountingDownloader) Pause(context.Context, string) error {
	return nil
}

func (d *removeCountingDownloader) Resume(context.Context, string) error {
	return nil
}

func (d *removeCountingDownloader) Remove(context.Context, string) error {
	d.removeCalls++
	return d.removeErr
}

type recordingTaskImportDriver struct {
	calls []recordingTaskImportCall
}

type failingMetadataVFSCommitter struct {
	err   error
	calls int
}

func (c *failingMetadataVFSCommitter) CommitFileObject(context.Context, MetadataVFSFileObjectCommitRequest) (*MetadataVFSFileObjectCommitResult, error) {
	c.calls++
	return nil, c.err
}

type recordingMetadataVFSCommitter struct {
	calls []MetadataVFSFileObjectCommitRequest
	err   error
}

func (c *recordingMetadataVFSCommitter) CommitFileObject(_ context.Context, req MetadataVFSFileObjectCommitRequest) (*MetadataVFSFileObjectCommitResult, error) {
	c.calls = append(c.calls, req)
	if c.err != nil {
		return nil, c.err
	}
	return &MetadataVFSFileObjectCommitResult{
		Node: &entity.VFSNode{ID: uint(len(c.calls)), Path: joinVirtualPath(req.VirtualParentPath, req.Filename), Kind: entity.VFSNodeKindFile},
	}, nil
}

type fakeSourceConfigCodec struct {
	driverType string
	slug       string
}

func (c fakeSourceConfigCodec) DriverType() string {
	return c.driverType
}

func (c fakeSourceConfigCodec) DefaultMountSlug() string {
	return c.slug
}

func (fakeSourceConfigCodec) Build(_ map[string]any, _ map[string]any, _ string) (string, error) {
	return "{}", nil
}

func (fakeSourceConfigCodec) Public(string, bool) (map[string]any, map[string]appdto.SecretFieldMask, error) {
	return map[string]any{}, map[string]appdto.SecretFieldMask{}, nil
}

func (fakeSourceConfigCodec) AuditView(string) map[string]any {
	return map[string]any{}
}

type recordingSourceProbe struct {
	sources []*entity.StorageSource
	err     error
}

func (p *recordingSourceProbe) Test(_ context.Context, source *entity.StorageSource) error {
	if source != nil {
		copied := *source
		p.sources = append(p.sources, &copied)
	}
	return p.err
}

type recordingTaskImportCall struct {
	sourceID   uint
	targetPath string
	localPath  string
	content    []byte
}

func (d *recordingTaskImportDriver) ImportFile(_ context.Context, source *entity.StorageSource, targetPath string, localPath string) error {
	content, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	d.calls = append(d.calls, recordingTaskImportCall{
		sourceID:   source.ID,
		targetPath: targetPath,
		localPath:  localPath,
		content:    content,
	})
	return nil
}

type uploadDriverStub struct{}

func (*uploadDriverStub) InitMultipartUpload(context.Context, *entity.StorageSource, MultipartUploadRequest) (*MultipartUploadPlan, error) {
	return &MultipartUploadPlan{}, nil
}

func (*uploadDriverStub) CompleteMultipartUpload(context.Context, *entity.StorageSource, MultipartUploadState, []CompletedUploadPart) (*StorageEntry, error) {
	return &StorageEntry{Name: "uploaded.bin", Path: "/uploaded.bin", ModifiedAt: time.Now()}, nil
}

type recordingUploadCompleteCall struct {
	sourceID uint
	state    MultipartUploadState
	parts    []CompletedUploadPart
}

type recordingUploadDriver struct {
	initPlan      *MultipartUploadPlan
	initErr       error
	completeEntry *StorageEntry
	completeErr   error
	initCalls     []MultipartUploadRequest
	completeCalls []recordingUploadCompleteCall
}

func (d *recordingUploadDriver) InitMultipartUpload(_ context.Context, _ *entity.StorageSource, req MultipartUploadRequest) (*MultipartUploadPlan, error) {
	d.initCalls = append(d.initCalls, req)
	if d.initErr != nil {
		return nil, d.initErr
	}
	if d.initPlan != nil {
		return d.initPlan, nil
	}
	return &MultipartUploadPlan{}, nil
}

func (d *recordingUploadDriver) CompleteMultipartUpload(_ context.Context, source *entity.StorageSource, state MultipartUploadState, parts []CompletedUploadPart) (*StorageEntry, error) {
	copiedParts := append([]CompletedUploadPart(nil), parts...)
	d.completeCalls = append(d.completeCalls, recordingUploadCompleteCall{
		sourceID: source.ID,
		state:    state,
		parts:    copiedParts,
	})
	if d.completeErr != nil {
		return nil, d.completeErr
	}
	if d.completeEntry != nil {
		return d.completeEntry, nil
	}
	return &StorageEntry{Name: path.Base(state.VirtualPath), Path: state.VirtualPath, Size: state.FileSize, ModifiedAt: time.Now()}, nil
}

type capacityDriverStub struct {
	used  *int64
	total *int64
	err   error
}

func (d capacityDriverStub) Capacity(context.Context, *entity.StorageSource) (*CapacityInfo, error) {
	if d.err != nil {
		return nil, d.err
	}
	return &CapacityInfo{UsedBytes: d.used, TotalBytes: d.total}, nil
}

type capabilityProviderStub struct {
	capabilities StorageCapabilities
	err          error
}

func (p capabilityProviderStub) Capabilities(context.Context, *entity.StorageSource) (StorageCapabilities, error) {
	if p.err != nil {
		return StorageCapabilities{}, p.err
	}
	return p.capabilities, nil
}

type storageFileDriverStub struct {
	listCalls     int
	listErr       error
	entriesByPath map[string][]StorageEntry
}

func (d *storageFileDriverStub) List(_ context.Context, _ *entity.StorageSource, virtualPath string) ([]StorageEntry, error) {
	d.listCalls++
	if d.listErr != nil {
		return nil, d.listErr
	}
	return d.entriesByPath[virtualPath], nil
}

func (*storageFileDriverStub) SearchByName(context.Context, *entity.StorageSource, string, string) ([]StorageEntry, error) {
	return nil, nil
}

func (*storageFileDriverStub) Stat(context.Context, *entity.StorageSource, string) (*StorageEntry, error) {
	return &StorageEntry{}, nil
}

func (*storageFileDriverStub) Mkdir(context.Context, *entity.StorageSource, string, string) (*StorageEntry, error) {
	return &StorageEntry{}, nil
}

func (*storageFileDriverStub) Rename(context.Context, *entity.StorageSource, string, string) (*StorageEntry, error) {
	return &StorageEntry{}, nil
}

func (*storageFileDriverStub) Move(context.Context, *entity.StorageSource, string, string) error {
	return nil
}

func (*storageFileDriverStub) Copy(context.Context, *entity.StorageSource, string, string) error {
	return nil
}

func (*storageFileDriverStub) Delete(context.Context, *entity.StorageSource, string) error {
	return nil
}

func (*storageFileDriverStub) PresignDownload(context.Context, *entity.StorageSource, string, string, time.Duration) (string, time.Time, error) {
	return "", time.Time{}, nil
}

type mountRegistryTestRepo struct {
	sources []*entity.StorageSource
}

type vfsFileOperatorCall struct {
	Operation  string
	SourceID   uint
	ParentPath string
	Name       string
	Path       string
	NewName    string
	TargetPath string
	DeleteMode string
}

type vfsFileOperatorSpy struct {
	calls      []vfsFileOperatorCall
	mkdirItem  *appdto.FileItem
	renameItem *appdto.FileItem
}

func (r mountRegistryTestRepo) Create(context.Context, *entity.StorageSource) error {
	return nil
}

func (r mountRegistryTestRepo) Update(context.Context, *entity.StorageSource) error {
	return nil
}

func (r mountRegistryTestRepo) Delete(context.Context, uint) error {
	return nil
}

func (r mountRegistryTestRepo) FindByID(context.Context, uint) (*entity.StorageSource, error) {
	return nil, nil
}

func (r mountRegistryTestRepo) ListAll(context.Context) ([]*entity.StorageSource, error) {
	return r.sources, nil
}

func (r mountRegistryTestRepo) ListEnabled(context.Context) ([]*entity.StorageSource, error) {
	items := make([]*entity.StorageSource, 0, len(r.sources))
	for _, source := range r.sources {
		if source.IsEnabled {
			items = append(items, source)
		}
	}
	return items, nil
}

func (r mountRegistryTestRepo) FindByName(context.Context, string) (*entity.StorageSource, error) {
	return nil, nil
}

func (r mountRegistryTestRepo) Count(context.Context) (int64, error) {
	return int64(len(r.sources)), nil
}

func newTestLocalSource(t *testing.T, id uint, name, mountPath string) *entity.StorageSource {
	t.Helper()

	source, _ := newTestLocalSourceWithBase(t, id, name, mountPath)
	return source
}

func newTestLocalSourceWithBase(t *testing.T, id uint, name, mountPath string) (*entity.StorageSource, string) {
	t.Helper()

	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}

	return &entity.StorageSource{
		ID:         id,
		Name:       name,
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  mountPath,
		RootPath:   "/",
		ConfigJSON: configJSON,
	}, basePath
}

func collectVFSNames(items []appdto.VFSItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names
}

func mustFindVFSItem(t *testing.T, items []appdto.VFSItem, name string) appdto.VFSItem {
	t.Helper()

	for _, item := range items {
		if item.Name == name {
			return item
		}
	}

	t.Fatalf("expected vfs item %s in %+v", name, items)
	return appdto.VFSItem{}
}

func (s *vfsFileOperatorSpy) Mkdir(_ context.Context, req appdto.MkdirRequest) (*appdto.FileItem, error) {
	s.calls = append(s.calls, vfsFileOperatorCall{
		Operation:  "mkdir",
		SourceID:   req.SourceID,
		ParentPath: req.ParentPath,
		Name:       req.Name,
	})
	if s.mkdirItem != nil {
		return s.mkdirItem, nil
	}
	return &appdto.FileItem{
		Name:       req.Name,
		Path:       joinVirtualPath(req.ParentPath, req.Name),
		ParentPath: req.ParentPath,
		SourceID:   req.SourceID,
		IsDir:      true,
	}, nil
}

func (s *vfsFileOperatorSpy) Rename(_ context.Context, req appdto.RenameRequest) (string, string, *appdto.FileItem, error) {
	s.calls = append(s.calls, vfsFileOperatorCall{
		Operation: "rename",
		SourceID:  req.SourceID,
		Path:      req.Path,
		NewName:   req.NewName,
	})
	if s.renameItem != nil {
		parentPath := path.Dir(req.Path)
		if parentPath == "." {
			parentPath = "/"
		}
		return req.Path, joinVirtualPath(parentPath, req.NewName), s.renameItem, nil
	}
	return "", "", nil, nil
}

func (s *vfsFileOperatorSpy) Move(_ context.Context, req appdto.MoveCopyRequest) (string, string, error) {
	s.calls = append(s.calls, vfsFileOperatorCall{
		Operation:  "move",
		SourceID:   req.SourceID,
		Path:       req.Path,
		TargetPath: req.TargetPath,
	})
	return req.Path, joinVirtualPath(req.TargetPath, path.Base(req.Path)), nil
}

func (s *vfsFileOperatorSpy) Copy(_ context.Context, req appdto.MoveCopyRequest) (string, string, error) {
	s.calls = append(s.calls, vfsFileOperatorCall{
		Operation:  "copy",
		SourceID:   req.SourceID,
		Path:       req.Path,
		TargetPath: req.TargetPath,
	})
	return req.Path, joinVirtualPath(req.TargetPath, path.Base(req.Path)), nil
}

func (s *vfsFileOperatorSpy) Delete(_ context.Context, req appdto.DeleteFileRequest) (time.Time, error) {
	s.calls = append(s.calls, vfsFileOperatorCall{
		Operation:  "delete",
		SourceID:   req.SourceID,
		Path:       req.Path,
		DeleteMode: req.DeleteMode,
	})
	return time.Now(), nil
}

func TestNormalizeMountPath(t *testing.T) {
	got, err := normalizeMountPath("/docs//./team/../team/archive/")
	if err != nil {
		t.Fatalf("normalizeMountPath() error = %v", err)
	}
	if got != "/docs/team/archive" {
		t.Fatalf("expected normalized mount path /docs/team/archive, got %s", got)
	}

	if _, err := normalizeMountPath("docs/team"); !errors.Is(err, ErrPathInvalid) {
		t.Fatalf("expected ErrPathInvalid for relative mount path, got %v", err)
	}
}

func TestResolveVirtualPathByLongestPrefix(t *testing.T) {
	mounts := []MountEntry{
		{
			MountPath: "/docs",
			Source:    &entity.StorageSource{ID: 1, Name: "文档库"},
		},
		{
			MountPath: "/docs/team",
			Source:    &entity.StorageSource{ID: 2, Name: "团队文档"},
		},
		{
			MountPath: "/movies",
			Source:    &entity.StorageSource{ID: 3, Name: "影视库"},
		},
	}

	resolved, err := resolveVirtualPathByLongestPrefix("/docs/team/archive/2024/a.zip", mounts)
	if err != nil {
		t.Fatalf("resolveVirtualPathByLongestPrefix() error = %v", err)
	}
	if !resolved.IsRealMount || resolved.IsPureVirtual {
		t.Fatalf("expected real mount match, got %+v", resolved)
	}
	if resolved.MatchedMountPath != "/docs/team" {
		t.Fatalf("expected matched mount /docs/team, got %+v", resolved)
	}
	if resolved.InnerPath != "/archive/2024/a.zip" {
		t.Fatalf("expected inner path /archive/2024/a.zip, got %+v", resolved)
	}
	if resolved.Source == nil || resolved.Source.ID != 2 {
		t.Fatalf("expected matched source id 2, got %+v", resolved)
	}
}

func TestResolveVirtualPathFallsBackToPureVirtualParent(t *testing.T) {
	mounts := []MountEntry{
		{
			MountPath: "/movies/aliyun",
			Source:    &entity.StorageSource{ID: 1, Name: "阿里云影视"},
		},
		{
			MountPath: "/movies/local",
			Source:    &entity.StorageSource{ID: 2, Name: "本地影视"},
		},
	}

	resolved, err := resolveVirtualPathByLongestPrefix("/movies", mounts)
	if err != nil {
		t.Fatalf("resolveVirtualPathByLongestPrefix() error = %v", err)
	}
	if resolved.IsRealMount || !resolved.IsPureVirtual {
		t.Fatalf("expected pure virtual directory, got %+v", resolved)
	}
	if resolved.MatchedMountPath != "" || resolved.InnerPath != "" || resolved.Source != nil {
		t.Fatalf("expected pure virtual fallback without backing source, got %+v", resolved)
	}
}

func TestProjectVirtualChildrenForRoot(t *testing.T) {
	registry := NewMountRegistry(mountRegistryTestRepo{sources: []*entity.StorageSource{
		{ID: 1, Name: "影视库", MountPath: "/movies", IsEnabled: true},
		{ID: 2, Name: "团队文档", MountPath: "/docs/team", IsEnabled: true},
		{ID: 3, Name: "个人文档", MountPath: "/docs/personal", IsEnabled: true},
	}})

	children, err := registry.ProjectVirtualChildren(context.Background(), "/")
	if err != nil {
		t.Fatalf("ProjectVirtualChildren(/) error = %v", err)
	}

	expected := []string{"docs", "movies"}
	if !reflect.DeepEqual(children, expected) {
		t.Fatalf("expected root projected children %v, got %v", expected, children)
	}
}

func TestProjectVirtualChildrenForNestedPrefix(t *testing.T) {
	registry := NewMountRegistry(mountRegistryTestRepo{sources: []*entity.StorageSource{
		{ID: 1, Name: "影视库", MountPath: "/movies", IsEnabled: true},
		{ID: 2, Name: "团队文档", MountPath: "/docs/team", IsEnabled: true},
		{ID: 3, Name: "个人文档", MountPath: "/docs/personal", IsEnabled: true},
	}})

	children, err := registry.ProjectVirtualChildren(context.Background(), "/docs")
	if err != nil {
		t.Fatalf("ProjectVirtualChildren(/docs) error = %v", err)
	}

	expected := []string{"personal", "team"}
	if !reflect.DeepEqual(children, expected) {
		t.Fatalf("expected nested projected children %v, got %v", expected, children)
	}
}

func TestProjectVirtualChildrenDeduplicatesNames(t *testing.T) {
	registry := NewMountRegistry(mountRegistryTestRepo{sources: []*entity.StorageSource{
		{ID: 1, Name: "团队文档", MountPath: "/docs/team", IsEnabled: true},
		{ID: 2, Name: "团队归档", MountPath: "/docs/team/archive", IsEnabled: true},
		{ID: 3, Name: "团队报告", MountPath: "/docs/team/reports", IsEnabled: true},
		{ID: 4, Name: "个人文档", MountPath: "/docs/personal", IsEnabled: true},
	}})

	children, err := registry.ProjectVirtualChildren(context.Background(), "/docs")
	if err != nil {
		t.Fatalf("ProjectVirtualChildren(/docs) error = %v", err)
	}

	expected := []string{"personal", "team"}
	if !reflect.DeepEqual(children, expected) {
		t.Fatalf("expected deduplicated projected children %v, got %v", expected, children)
	}
}

func TestResolveWritableTargetAllowsMappedVirtualPath(t *testing.T) {
	svc := NewVFSService(mountRegistryTestRepo{sources: []*entity.StorageSource{
		newTestLocalSource(t, 1, "文档库", "/docs"),
		newTestLocalSource(t, 2, "团队文档", "/docs/team"),
	}})

	resolved, err := svc.ResolveWritableTarget(context.Background(), "/docs/team/report.txt")
	if err != nil {
		t.Fatalf("ResolveWritableTarget() error = %v", err)
	}
	if !resolved.IsRealMount || resolved.MatchedMountPath != "/docs/team" || resolved.InnerPath != "/report.txt" {
		t.Fatalf("expected writable target on nested mount, got %+v", resolved)
	}
}

func TestResolveWritableTargetRejectsPureVirtualParent(t *testing.T) {
	svc := NewVFSService(mountRegistryTestRepo{sources: []*entity.StorageSource{
		newTestLocalSource(t, 1, "团队文档", "/docs/team"),
		newTestLocalSource(t, 2, "个人文档", "/docs/personal"),
	}})

	_, err := svc.ResolveWritableTarget(context.Background(), "/docs/readme.md")
	if !errors.Is(err, ErrNoBackingStorage) {
		t.Fatalf("expected ErrNoBackingStorage, got %v", err)
	}
}

func TestResolveWritableTargetRejectsNameConflictWithMount(t *testing.T) {
	svc := NewVFSService(mountRegistryTestRepo{sources: []*entity.StorageSource{
		newTestLocalSource(t, 1, "文档库", "/docs"),
		newTestLocalSource(t, 2, "团队归档", "/docs/team/archive"),
	}})

	_, err := svc.ResolveWritableTarget(context.Background(), "/docs/team")
	if !errors.Is(err, ErrNameConflict) {
		t.Fatalf("expected ErrNameConflict, got %v", err)
	}
}

func TestVFSListRootReturnsProjectedMounts(t *testing.T) {
	svc := NewVFSService(mountRegistryTestRepo{sources: []*entity.StorageSource{
		newTestLocalSource(t, 1, "影视库", "/movies"),
		newTestLocalSource(t, 2, "团队文档", "/docs/team"),
		newTestLocalSource(t, 3, "个人文档", "/docs/personal"),
	}})

	listed, err := svc.List(context.Background(), "/")
	if err != nil {
		t.Fatalf("List(/) error = %v", err)
	}

	expected := []string{"docs", "movies"}
	if !reflect.DeepEqual(collectVFSNames(listed.Items), expected) {
		t.Fatalf("expected root vfs names %v, got %v", expected, collectVFSNames(listed.Items))
	}

	moviesItem := mustFindVFSItem(t, listed.Items, "movies")
	if !moviesItem.IsVirtual || !moviesItem.IsMountPoint {
		t.Fatalf("expected /movies to be a virtual mount point, got %+v", moviesItem)
	}
	docsItem := mustFindVFSItem(t, listed.Items, "docs")
	if !docsItem.IsVirtual || docsItem.IsMountPoint {
		t.Fatalf("expected /docs to be a pure virtual directory, got %+v", docsItem)
	}
}

func TestVFSListPureVirtualDirectoryReturnsOnlyProjectedChildren(t *testing.T) {
	svc := NewVFSService(mountRegistryTestRepo{sources: []*entity.StorageSource{
		newTestLocalSource(t, 1, "团队文档", "/docs/team"),
		newTestLocalSource(t, 2, "个人文档", "/docs/personal"),
	}})

	listed, err := svc.List(context.Background(), "/docs")
	if err != nil {
		t.Fatalf("List(/docs) error = %v", err)
	}

	expected := []string{"personal", "team"}
	if !reflect.DeepEqual(collectVFSNames(listed.Items), expected) {
		t.Fatalf("expected pure virtual children %v, got %v", expected, collectVFSNames(listed.Items))
	}
	for _, item := range listed.Items {
		if !item.IsVirtual || !item.IsMountPoint {
			t.Fatalf("expected projected child to be virtual mount point, got %+v", item)
		}
	}
}

func TestVFSListRealAndVirtualChildrenMergedWithMountPriority(t *testing.T) {
	docsSource, docsBasePath := newTestLocalSourceWithBase(t, 1, "文档库", "/docs")
	if err := os.MkdirAll(filepath.Join(docsBasePath, "team"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(team) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(docsBasePath, "notes"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(notes) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsBasePath, "readme.md"), []byte("readme"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(readme.md) error = %v", err)
	}

	svc := NewVFSService(mountRegistryTestRepo{sources: []*entity.StorageSource{
		docsSource,
		newTestLocalSource(t, 2, "团队挂载", "/docs/team"),
	}})

	listed, err := svc.List(context.Background(), "/docs")
	if err != nil {
		t.Fatalf("List(/docs) error = %v", err)
	}

	expected := []string{"notes", "readme.md", "team"}
	if !reflect.DeepEqual(collectVFSNames(listed.Items), expected) {
		t.Fatalf("expected merged vfs names %v, got %v", expected, collectVFSNames(listed.Items))
	}
	teamItem := mustFindVFSItem(t, listed.Items, "team")
	if !teamItem.IsVirtual || !teamItem.IsMountPoint {
		t.Fatalf("expected mount-backed team item to win merge, got %+v", teamItem)
	}
	if collectVFSNames(listed.Items)[2] != "team" {
		t.Fatalf("expected merged items to stay sorted, got %v", collectVFSNames(listed.Items))
	}
}

func TestVFSListLocalReadOnlyDirectoryDisablesDeleteCapability(t *testing.T) {
	source, basePath := newTestLocalSourceWithBase(t, 1, "只读文档", "/readonly")
	if err := os.WriteFile(filepath.Join(basePath, "locked.txt"), []byte("locked"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(locked.txt) error = %v", err)
	}

	svc := NewVFSService(
		mountRegistryTestRepo{sources: []*entity.StorageSource{source}},
		WithVFSLocalDirWritable(func(string) bool { return false }),
	)

	listed, err := svc.List(context.Background(), "/readonly")
	if err != nil {
		t.Fatalf("List(/readonly) error = %v", err)
	}

	item := mustFindVFSItem(t, listed.Items, "locked.txt")
	if item.CanDelete {
		t.Fatalf("expected read-only local directory to disable can_delete, got %+v", item)
	}
}

func TestFileMkdirReadOnlyLocalSourceReturnsSourceReadOnly(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	basePath := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "只读本地源",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/readonly",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	svc := NewFileService(
		sourceRepo,
		nil,
		nil,
		nil,
		WithFileLocalDirWritable(func(string) bool { return false }),
	)

	_, err = svc.Mkdir(context.Background(), appdto.MkdirRequest{
		SourceID:   source.ID,
		ParentPath: "/",
		Name:       "should-not-create",
	})
	if !errors.Is(err, ErrSourceReadOnly) {
		t.Fatalf("expected ErrSourceReadOnly, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(basePath, "should-not-create")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("read-only mkdir should not create directory, stat err=%v", statErr)
	}
}

func TestFileReadOnlyLocalWriteOperationsReturnSourceReadOnly(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	sourceRepo := gorm.NewSourceRepository(db)
	basePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(basePath, "locked.txt"), []byte("locked"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(locked.txt) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(basePath, "target"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(target) error = %v", err)
	}
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		Name:       "只读写操作本地源",
		DriverType: "local",
		Status:     "online",
		IsEnabled:  true,
		MountPath:  "/readonly",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := sourceRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("sourceRepo.Create() error = %v", err)
	}

	svc := NewFileService(
		sourceRepo,
		nil,
		nil,
		nil,
		WithFileLocalDirWritable(func(string) bool { return false }),
	)

	if _, _, err := svc.Move(context.Background(), appdto.MoveCopyRequest{
		SourceID:   source.ID,
		Path:       "/locked.txt",
		TargetPath: "/target",
	}); !errors.Is(err, ErrSourceReadOnly) {
		t.Fatalf("Move() expected ErrSourceReadOnly, got %v", err)
	}
	if _, _, err := svc.Copy(context.Background(), appdto.MoveCopyRequest{
		SourceID:   source.ID,
		Path:       "/locked.txt",
		TargetPath: "/target",
	}); !errors.Is(err, ErrSourceReadOnly) {
		t.Fatalf("Copy() expected ErrSourceReadOnly, got %v", err)
	}
	if _, err := svc.Delete(context.Background(), appdto.DeleteFileRequest{
		SourceID:   source.ID,
		Path:       "/locked.txt",
		DeleteMode: "permanent",
	}); !errors.Is(err, ErrSourceReadOnly) {
		t.Fatalf("Delete(permanent) expected ErrSourceReadOnly, got %v", err)
	}
	if _, err := svc.Delete(context.Background(), appdto.DeleteFileRequest{
		SourceID:   source.ID,
		Path:       "/locked.txt",
		DeleteMode: "trash",
	}); !errors.Is(err, ErrSourceReadOnly) {
		t.Fatalf("Delete(trash) expected ErrSourceReadOnly, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(basePath, "locked.txt")); err != nil {
		t.Fatalf("read-only operations should not remove source file, stat err=%v", err)
	}
}

func TestVFSMkdirOnMappedPath(t *testing.T) {
	operator := &vfsFileOperatorSpy{
		mkdirItem: &appdto.FileItem{
			Name:       "team",
			Path:       "/team",
			ParentPath: "/",
			SourceID:   1,
			IsDir:      true,
		},
	}
	svc := NewVFSService(
		mountRegistryTestRepo{sources: []*entity.StorageSource{
			newTestLocalSource(t, 1, "文档库", "/docs"),
		}},
		WithVFSFileOperator(operator),
	)

	item, err := svc.Mkdir(context.Background(), appdto.VFSMkdirRequest{
		ParentPath: "/docs",
		Name:       "team",
	})
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if len(operator.calls) != 1 {
		t.Fatalf("expected exactly one mkdir call, got %+v", operator.calls)
	}
	call := operator.calls[0]
	if call.Operation != "mkdir" || call.SourceID != 1 || call.ParentPath != "/" || call.Name != "team" {
		t.Fatalf("unexpected mkdir delegation = %+v", call)
	}
	if item.Path != "/docs/team" || item.ParentPath != "/docs" || item.EntryKind != "directory" {
		t.Fatalf("unexpected vfs mkdir item = %+v", item)
	}
}

func TestVFSMkdirRejectsPureVirtualParent(t *testing.T) {
	operator := &vfsFileOperatorSpy{}
	svc := NewVFSService(
		mountRegistryTestRepo{sources: []*entity.StorageSource{
			newTestLocalSource(t, 1, "团队文档", "/docs/team"),
			newTestLocalSource(t, 2, "个人文档", "/docs/personal"),
		}},
		WithVFSFileOperator(operator),
	)

	_, err := svc.Mkdir(context.Background(), appdto.VFSMkdirRequest{
		ParentPath: "/docs",
		Name:       "shared",
	})
	if !errors.Is(err, ErrNoBackingStorage) {
		t.Fatalf("expected ErrNoBackingStorage, got %v", err)
	}
	if len(operator.calls) != 0 {
		t.Fatalf("expected no delegated mkdir call, got %+v", operator.calls)
	}
}

func TestVFSRenameRejectsMountNameConflict(t *testing.T) {
	operator := &vfsFileOperatorSpy{}
	svc := NewVFSService(
		mountRegistryTestRepo{sources: []*entity.StorageSource{
			newTestLocalSource(t, 1, "文档库", "/docs"),
			newTestLocalSource(t, 2, "团队归档", "/docs/team/archive"),
		}},
		WithVFSFileOperator(operator),
	)

	_, _, _, err := svc.Rename(context.Background(), appdto.VFSRenameRequest{
		Path:    "/docs/readme.md",
		NewName: "team",
	})
	if !errors.Is(err, ErrNameConflict) {
		t.Fatalf("expected ErrNameConflict, got %v", err)
	}
	if len(operator.calls) != 0 {
		t.Fatalf("expected no delegated rename call, got %+v", operator.calls)
	}
}

func TestVFSMoveAcrossMountsFallsBackToCopyDelete(t *testing.T) {
	docsSource, docsBasePath := newTestLocalSourceWithBase(t, 1, "文档库", "/docs")
	archiveSource, archiveBasePath := newTestLocalSourceWithBase(t, 2, "归档库", "/archive")
	if err := os.WriteFile(filepath.Join(docsBasePath, "readme.md"), []byte("hello cross mount"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(readme.md) error = %v", err)
	}
	if err := os.MkdirAll(archiveBasePath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(archiveBasePath) error = %v", err)
	}

	svc := NewVFSService(mountRegistryTestRepo{sources: []*entity.StorageSource{
		docsSource,
		archiveSource,
	}})

	oldPath, newPath, err := svc.Move(context.Background(), appdto.VFSMoveCopyRequest{
		Path:       "/docs/readme.md",
		TargetPath: "/archive",
	})
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if oldPath != "/docs/readme.md" || newPath != "/archive/readme.md" {
		t.Fatalf("unexpected move paths old=%s new=%s", oldPath, newPath)
	}
	if _, err := os.Stat(filepath.Join(docsBasePath, "readme.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected source file removed after cross-mount move, got err=%v", err)
	}
	content, err := os.ReadFile(filepath.Join(archiveBasePath, "readme.md"))
	if err != nil {
		t.Fatalf("os.ReadFile(archive/readme.md) error = %v", err)
	}
	if string(content) != "hello cross mount" {
		t.Fatalf("unexpected copied content = %q", string(content))
	}
}
