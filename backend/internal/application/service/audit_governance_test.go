package service

import (
	"context"
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

func TestUserServiceCreateWritesAuditLog(t *testing.T) {
	deps := newAuditGovernanceTestDeps(t)

	_, err := deps.userSvc.Create(deps.adminCtx, appdto.CreateUserRequest{
		Username: "alice",
		Password: "strong-password-123",
		Email:    "alice@example.com",
		RoleKey:  permission.RoleUser,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	log := deps.mustFindLastAudit(t)
	if log.ResourceType != "user" || log.Action != "create" || log.Result != string(appaudit.ResultSuccess) {
		t.Fatalf("unexpected audit log = %+v", log)
	}
}

func TestUserServiceUpdateWritesDeniedAuditWhenLastSuperAdminWouldBeLocked(t *testing.T) {
	deps := newAuditGovernanceTestDeps(t)

	_, err := deps.userSvc.Update(deps.adminCtx, deps.superAdminID, appdto.UpdateUserRequest{
		Email:   "admin@example.com",
		RoleKey: permission.RoleAdmin,
		Status:  permission.StatusLocked,
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	log := deps.mustFindLastAudit(t)
	if log.Action != "update" || log.Result != string(appaudit.ResultDenied) || log.ErrorCode != "LAST_SUPER_ADMIN_FORBIDDEN" {
		t.Fatalf("unexpected denied audit = %+v", log)
	}
}

type auditGovernanceTestDeps struct {
	userSvc      *UserService
	auditRepo    *gormrepo.AuditLogRepository
	adminCtx     context.Context
	superAdminID uint
}

func newAuditGovernanceTestDeps(t *testing.T) *auditGovernanceTestDeps {
	t.Helper()

	db, cleanup := openTestDB(t)
	t.Cleanup(cleanup)

	userRepo := gormrepo.NewUserRepository(db)
	refreshRepo := gormrepo.NewRefreshTokenRepository(db)
	configRepo := gormrepo.NewSystemConfigRepository(db)
	sourceRepo := gormrepo.NewSourceRepository(db)
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

	auditRepo := gormrepo.NewAuditLogRepository(db)
	auditRecorder := appaudit.NewRecorder(auditRepo, nil)
	userSvc := NewUserService(userRepo, hasher, WithUserAuditRecorder(auditRecorder))
	adminCtx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       1,
		Username:     "admin",
		RoleKey:      permission.RoleSuperAdmin,
		Status:       permission.StatusActive,
		Capabilities: permission.AllCapabilities(),
	})

	return &auditGovernanceTestDeps{
		userSvc:      userSvc,
		auditRepo:    auditRepo,
		adminCtx:     adminCtx,
		superAdminID: 1,
	}
}

func (d *auditGovernanceTestDeps) mustFindLastAudit(t *testing.T) *entity.AuditLog {
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
