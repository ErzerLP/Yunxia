package gorm

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"yunxia/internal/domain/entity"
)

func TestUserRepositoryPersistsRoleKeyAndStatus(t *testing.T) {
	db, cleanup := testDB(t, filepath.Join(t.TempDir(), "repo.db"))
	defer cleanup()

	repo := NewUserRepository(db)
	user := &entity.User{
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "hash",
		RoleKey:      "operator",
		Status:       "active",
		TokenVersion: 2,
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.FindByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.RoleKey != "operator" || got.Status != "active" {
		t.Fatalf("unexpected user = %+v", got)
	}
}

func TestSystemConfigRepositoryUpsertAndGet(t *testing.T) {
	db, cleanup := testDB(t, filepath.Join(t.TempDir(), "cfg.db"))
	defer cleanup()

	repo := NewSystemConfigRepository(db)
	cfg := &entity.SystemConfig{SiteName: "云匣", MultiUserEnabled: true, DefaultChunkSize: 5 * 1024 * 1024, MaxUploadSize: 10 * 1024 * 1024 * 1024, WebDAVEnabled: true, WebDAVPrefix: "/dav", Theme: "system", Language: "zh-CN", TimeZone: "Asia/Shanghai", UpdatedAt: time.Now()}
	if err := repo.Upsert(context.Background(), cfg); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	got, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.SiteName != "云匣" || !got.MultiUserEnabled {
		t.Fatalf("unexpected config = %+v", got)
	}
}

func TestRefreshTokenRepositoryCreateFindAndRevoke(t *testing.T) {
	db, cleanup := testDB(t, filepath.Join(t.TempDir(), "token.db"))
	defer cleanup()

	repo := NewRefreshTokenRepository(db)
	token := &entity.RefreshToken{UserID: 7, TokenHash: "hash-value", ExpiresAt: time.Now().Add(time.Hour)}
	if err := repo.Create(context.Background(), token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.FindByTokenHash(context.Background(), "hash-value")
	if err != nil {
		t.Fatalf("FindByTokenHash() error = %v", err)
	}
	if got.UserID != 7 {
		t.Fatalf("unexpected token = %+v", got)
	}

	if err := repo.RevokeByTokenHash(context.Background(), "hash-value"); err != nil {
		t.Fatalf("RevokeByTokenHash() error = %v", err)
	}

	revoked, err := repo.FindByTokenHash(context.Background(), "hash-value")
	if err != nil {
		t.Fatalf("FindByTokenHash() error after revoke = %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatalf("expected token to be revoked")
	}
}
