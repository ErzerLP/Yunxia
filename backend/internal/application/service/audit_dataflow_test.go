package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	appaudit "yunxia/internal/application/audit"
	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	gormrepo "yunxia/internal/infrastructure/persistence/gorm"
	"yunxia/internal/infrastructure/security"
)

func TestFileServiceRenameWritesSuccessAudit(t *testing.T) {
	deps := newAuditDataflowTestDeps(t)

	oldPath, newPath, _, err := deps.fileSvc.Rename(deps.actorCtx, appdto.RenameRequest{
		SourceID: deps.sourceID,
		Path:     "/docs/old.txt",
		NewName:  "new.txt",
	})
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if oldPath != "/docs/old.txt" || newPath != "/docs/new.txt" {
		t.Fatalf("unexpected rename result old=%s new=%s", oldPath, newPath)
	}

	log := deps.mustFindLastAudit(t)
	if log.ResourceType != "file" || log.Action != "rename" || log.Result != string(appaudit.ResultSuccess) {
		t.Fatalf("unexpected file audit log = %+v", log)
	}
}

func TestTaskServiceCancelWritesSuccessAudit(t *testing.T) {
	deps := newAuditDataflowTestDeps(t)

	created, err := deps.taskSvc.Create(deps.actorCtx, appdto.CreateTaskRequest{
		Type:     "url",
		URL:      "https://example.com/archive.zip",
		SourceID: deps.sourceID,
		SavePath: "/downloads",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := deps.taskSvc.Cancel(deps.actorCtx, created.ID, false); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	log := deps.mustFindLastAudit(t)
	if log.ResourceType != "task" || log.Action != "cancel" || log.Result != string(appaudit.ResultSuccess) {
		t.Fatalf("unexpected task audit log = %+v", log)
	}
}

func TestShareServiceCreateWritesSuccessAudit(t *testing.T) {
	deps := newAuditDataflowTestDeps(t)

	_, err := deps.shareSvc.Create(deps.actorCtx, appdto.CreateShareRequest{
		SourceID: deps.sourceID,
		Path:     "/docs/share.txt",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	log := deps.mustFindLastAudit(t)
	if log.ResourceType != "share" || log.Action != "create" || log.Result != string(appaudit.ResultSuccess) {
		t.Fatalf("unexpected share audit log = %+v", log)
	}
}

func TestTrashServiceRestoreWritesSuccessAudit(t *testing.T) {
	deps := newAuditDataflowTestDeps(t)

	deletedAt, err := deps.fileSvc.Delete(deps.actorCtx, appdto.DeleteFileRequest{
		SourceID:   deps.sourceID,
		Path:       "/docs/trash.txt",
		DeleteMode: "trash",
	})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	items, err := deps.trashRepo.ListBySourceID(context.Background(), deps.sourceID)
	if err != nil {
		t.Fatalf("trashRepo.ListBySourceID() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 trash item, got %d", len(items))
	}
	if _, err := deps.trashSvc.Restore(deps.actorCtx, items[0].ID); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if deletedAt.IsZero() {
		t.Fatalf("expected deletedAt to be set")
	}

	log := deps.mustFindLastAudit(t)
	if log.ResourceType != "file" || log.Action != "restore" || log.Result != string(appaudit.ResultSuccess) {
		t.Fatalf("unexpected restore audit log = %+v", log)
	}
}

func TestUploadServiceFinishWritesAudit(t *testing.T) {
	deps := newAuditDataflowTestDeps(t)

	initResp, err := deps.uploadSvc.Init(deps.actorCtx, deps.userID, appdto.UploadInitRequest{
		SourceID: deps.sourceID,
		Path:     "/docs",
		Filename: "hello.txt",
		FileSize: 5,
		FileHash: "5d41402abc4b2a76b9719d911017c592",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := deps.uploadSvc.UploadChunk(deps.actorCtx, initResp.Upload.UploadID, 0, []byte("hello")); err != nil {
		t.Fatalf("UploadChunk() error = %v", err)
	}
	if _, err := deps.uploadSvc.Finish(deps.actorCtx, appdto.UploadFinishRequest{UploadID: initResp.Upload.UploadID}); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}

	log := deps.mustFindLastAudit(t)
	if log.ResourceType != "file" || log.Action != "upload_finish" || log.Result != string(appaudit.ResultSuccess) {
		t.Fatalf("unexpected upload audit = %+v", log)
	}
}

type auditDataflowTestDeps struct {
	fileSvc   *FileService
	taskSvc   *TaskService
	shareSvc  *ShareService
	trashSvc  *TrashService
	uploadSvc *UploadService
	trashRepo *gormrepo.TrashItemRepository
	auditRepo *gormrepo.AuditLogRepository
	actorCtx  context.Context
	sourceID  uint
	userID    uint
}

func newAuditDataflowTestDeps(t *testing.T) *auditDataflowTestDeps {
	t.Helper()

	db, cleanup := openTestDB(t)
	t.Cleanup(cleanup)

	userRepo := gormrepo.NewUserRepository(db)
	refreshRepo := gormrepo.NewRefreshTokenRepository(db)
	configRepo := gormrepo.NewSystemConfigRepository(db)
	sourceRepo := gormrepo.NewSourceRepository(db)
	uploadRepo := gormrepo.NewUploadSessionRepository(db)
	taskRepo := gormrepo.NewTaskRepository(db)
	trashRepo := gormrepo.NewTrashItemRepository(db)
	shareRepo := gormrepo.NewShareRepository(db)
	auditRepo := gormrepo.NewAuditLogRepository(db)
	auditRecorder := appaudit.NewRecorder(auditRepo, nil)
	hasher := security.NewBcryptHasher(4)
	tokenSvc := security.NewJWTTokenService("test-secret", 15*time.Minute, 7*24*time.Hour)
	fileAccessSvc := security.NewFileAccessTokenService("test-secret")
	options := DefaultSystemOptions()
	root := t.TempDir()
	options.StorageDataDir = filepath.Join(root, "storage")
	options.TempDir = filepath.Join(root, "temp")

	setupSvc := NewSetupService(
		userRepo,
		refreshRepo,
		configRepo,
		sourceRepo,
		hasher,
		tokenSvc,
		options,
		WithSetupAuditRecorder(auditRecorder),
	)
	if _, err := setupSvc.Init(context.Background(), appdto.SetupInitRequest{
		Username: "admin",
		Password: "strong-password-123",
		Email:    "admin@example.com",
	}); err != nil {
		t.Fatalf("setup Init() error = %v", err)
	}

	source, err := sourceRepo.FindByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("sourceRepo.FindByID() error = %v", err)
	}
	_, docsDir, err := resolvePhysicalPath(source, "/docs")
	if err != nil {
		t.Fatalf("resolvePhysicalPath(/docs) error = %v", err)
	}
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(docsDir) error = %v", err)
	}
	for name, content := range map[string]string{
		"old.txt":   "old content",
		"share.txt": "share content",
		"trash.txt": "trash content",
	} {
		if err := os.WriteFile(filepath.Join(docsDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("os.WriteFile(%s) error = %v", name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(docsDir, "subdir"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(subdir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "storage", "default", "downloads"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(downloads) error = %v", err)
	}

	actorCtx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       1,
		Username:     "admin",
		RoleKey:      permission.RoleSuperAdmin,
		Status:       permission.StatusActive,
		Capabilities: permission.AllCapabilities(),
	})

	fileSvc := NewFileService(
		sourceRepo,
		fileAccessSvc,
		tokenSvc,
		userRepo,
		WithTrashItemRepository(trashRepo),
		WithFileAuditRecorder(auditRecorder),
	)
	taskSvc := NewTaskService(
		taskRepo,
		sourceRepo,
		taskServiceTestDownloader{},
		WithTaskAuditRecorder(auditRecorder),
	)
	shareSvc := NewShareService(
		shareRepo,
		sourceRepo,
		hasher,
		fileAccessSvc,
		WithShareAuditRecorder(auditRecorder),
	)
	trashSvc := NewTrashService(
		sourceRepo,
		trashRepo,
		WithTrashAuditRecorder(auditRecorder),
	)
	uploadSvc := NewUploadService(
		sourceRepo,
		uploadRepo,
		options,
		WithUploadAuditRecorder(auditRecorder),
	)

	return &auditDataflowTestDeps{
		fileSvc:   fileSvc,
		taskSvc:   taskSvc,
		shareSvc:  shareSvc,
		trashSvc:  trashSvc,
		uploadSvc: uploadSvc,
		trashRepo: trashRepo,
		auditRepo: auditRepo,
		actorCtx:  actorCtx,
		sourceID:  source.ID,
		userID:    1,
	}
}

func (d *auditDataflowTestDeps) mustFindLastAudit(t *testing.T) *entity.AuditLog {
	t.Helper()

	items, _, err := d.auditRepo.List(context.Background(), entity.AuditLogFilter{
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("auditRepo.List() error = %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected at least one audit log")
	}
	return items[0]
}
