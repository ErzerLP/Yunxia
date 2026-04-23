package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
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

	var storedUserID uint
	row := db.WithContext(context.Background()).Raw("select user_id from download_task_models where id = ?", resp.ID).Row()
	if err := row.Scan(&storedUserID); err != nil {
		t.Fatalf("scan persisted task user_id error = %v", err)
	}
	if storedUserID != 42 {
		t.Fatalf("expected stored task user_id=42, got %d", storedUserID)
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
	if created.Path != "/projects" || created.SubjectType != "user" || !created.Permissions.Read || !created.Permissions.Write {
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
	if updated.Effect != "deny" || updated.Priority != 150 || updated.InheritToChildren {
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
