package gorm

import (
	"context"
	"testing"
	"time"

	"yunxia/internal/domain/entity"
)

func TestAuditLogRepositoryCreateAndList(t *testing.T) {
	db, cleanup := testDB(t, "file::memory:?cache=shared")
	defer cleanup()

	repo := NewAuditLogRepository(db)
	log := &entity.AuditLog{
		OccurredAt:   time.Now(),
		RequestID:    "req-1",
		EntryPoint:   "rest_v1",
		ActorUserID:  ptrUint(1),
		ActorRoleKey: "super_admin",
		ResourceType: "user",
		Action:       "create",
		Result:       "success",
		AfterJSON:    `{"username":"alice"}`,
	}

	if err := repo.Create(context.Background(), log); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	items, total, err := repo.List(context.Background(), entity.AuditLogFilter{
		ResourceType: "user",
		Action:       "create",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected one audit log, got total=%d len=%d", total, len(items))
	}
}

func ptrUint(v uint) *uint {
	return &v
}
