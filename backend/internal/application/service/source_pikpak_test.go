package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
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
	if len(vfsList.Items) != 1 || vfsList.Items[0].CanDelete || !vfsList.Items[0].CanDownload {
		t.Fatalf("expected PikPak VFS item to be downloadable but read-only, got %+v", vfsList.Items)
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

type sourceServiceFakePikPakClient struct {
	filesByParent map[string][]infraStorage.PikPakFile
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

func (c *sourceServiceFakePikPakClient) About(context.Context, infraStorage.PikPakSession) (*infraStorage.PikPakAbout, error) {
	return &infraStorage.PikPakAbout{}, nil
}

var _ infraStorage.PikPakAPIClient = (*sourceServiceFakePikPakClient)(nil)
