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
	"yunxia/internal/infrastructure/persistence/gorm"
	"yunxia/internal/infrastructure/security"
)

func TestSetupServiceInitCreatesAdminAndStoresRefreshToken(t *testing.T) {
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
	if resp.User.Username != "admin" || resp.User.Role != "admin" {
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
	if !status.IsInitialized || status.SetupRequired || !status.HasAdmin {
		t.Fatalf("unexpected status = %+v", status)
	}
}

func TestAuthServiceLoginRefreshLogoutAndMe(t *testing.T) {
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
	if loginResp.User.Username != "admin" {
		t.Fatalf("unexpected login user = %+v", loginResp.User)
	}

	refreshResp, err := authSvc.Refresh(context.Background(), appdto.RefreshRequest{RefreshToken: loginResp.Tokens.RefreshToken})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if refreshResp.Tokens.RefreshToken == loginResp.Tokens.RefreshToken {
		t.Fatalf("expected rotated refresh token")
	}

	oldToken, err := refreshRepo.FindByTokenHash(context.Background(), hashToken(loginResp.Tokens.RefreshToken))
	if err != nil {
		t.Fatalf("FindByTokenHash(old) error = %v", err)
	}
	if oldToken.RevokedAt == nil {
		t.Fatalf("expected old refresh token to be revoked")
	}

	me, err := authSvc.Me(context.Background(), loginResp.User.ID)
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if me.Username != "admin" {
		t.Fatalf("unexpected me = %+v", me)
	}

	if err := authSvc.Logout(context.Background(), appdto.LogoutRequest{RefreshToken: refreshResp.Tokens.RefreshToken}); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	latest, err := refreshRepo.FindByTokenHash(context.Background(), hashToken(refreshResp.Tokens.RefreshToken))
	if err != nil {
		t.Fatalf("FindByTokenHash(latest) error = %v", err)
	}
	if latest.RevokedAt == nil {
		t.Fatalf("expected latest refresh token to be revoked")
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
		Role:         "user",
		IsLocked:     false,
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
		UserID: 42,
		Role:   "normal",
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

	created, err := svc.Create(context.Background(), appdto.CreateUserRequest{
		Username: "alice",
		Password: "strong-password-456",
		Email:    "alice@example.com",
		Role:     "normal",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Username != "alice" || created.Role != "normal" || created.Status != "active" {
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

	updated, err := svc.Update(context.Background(), created.ID, appdto.UpdateUserRequest{
		Email:  "alice+updated@example.com",
		Role:   "admin",
		Status: "locked",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Email != "alice+updated@example.com" || updated.Role != "admin" || updated.Status != "locked" {
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
		Role:         "user",
		IsLocked:     false,
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
		UserID: 7,
		Role:   "normal",
	})
	if err := authorizer.AuthorizePath(ctx, source.ID, "/docs/spec.md", ACLActionRead); err != nil {
		t.Fatalf("expected virtual_path rule to allow mounted path, got %v", err)
	}
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
