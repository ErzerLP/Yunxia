package gorm

import (
	"context"
	"testing"
	"time"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

func TestVFSOperationRepositoryDueAcquireAndStatusUpdates(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	assertJSONBColumn(t, db, &VFSOperationModel{}, "payload_json")

	ctx := context.Background()
	repo := NewVFSOperationRepository(db)
	now := time.Date(2026, 5, 7, 16, 0, 0, 0, time.UTC)
	sourceID := uint(10)

	pending := &entity.VFSOperation{
		OperationType:      entity.VFSOperationTypeMkdir,
		Status:             entity.VFSOperationStatusPending,
		SourcePathSnapshot: "/docs",
		TargetPathSnapshot: "/docs/new",
		SourceIDSnapshot:   &sourceID,
		DriverTypeSnapshot: "local",
		PayloadJSON:        "",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repo.Create(ctx, pending); err != nil {
		t.Fatalf("Create(pending) error = %v", err)
	}
	if pending.PayloadJSON != "{}" {
		t.Fatalf("PayloadJSON default = %q, want {}", pending.PayloadJSON)
	}

	dueAt := now.Add(-time.Minute)
	dueFailed := &entity.VFSOperation{
		OperationType:      entity.VFSOperationTypeRefresh,
		Status:             entity.VFSOperationStatusFailed,
		TargetPathSnapshot: "/docs",
		ErrorCode:          "VFS_OPERATION_FAILED",
		ErrorMessage:       "vfs operation failed",
		RetryCount:         1,
		NextRetryAt:        &dueAt,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repo.Create(ctx, dueFailed); err != nil {
		t.Fatalf("Create(due failed) error = %v", err)
	}

	futureRetryAt := now.Add(time.Hour)
	futureFailed := &entity.VFSOperation{
		OperationType: entity.VFSOperationTypeDelete,
		Status:        entity.VFSOperationStatusFailed,
		NextRetryAt:   &futureRetryAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.Create(ctx, futureFailed); err != nil {
		t.Fatalf("Create(future failed) error = %v", err)
	}

	lockedUntil := now.Add(time.Hour)
	lockedFailed := &entity.VFSOperation{
		OperationType: entity.VFSOperationTypeCopy,
		Status:        entity.VFSOperationStatusFailed,
		NextRetryAt:   &dueAt,
		LockedBy:      "worker-b",
		LockedUntil:   &lockedUntil,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.Create(ctx, lockedFailed); err != nil {
		t.Fatalf("Create(locked failed) error = %v", err)
	}

	expiredRunningLock := now.Add(-time.Minute)
	expiredRunning := &entity.VFSOperation{
		OperationType: entity.VFSOperationTypeRefresh,
		Status:        entity.VFSOperationStatusRunning,
		LockedBy:      "crashed-worker",
		LockedUntil:   &expiredRunningLock,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.Create(ctx, expiredRunning); err != nil {
		t.Fatalf("Create(expired running) error = %v", err)
	}

	due, err := repo.ListDue(ctx, domainrepo.VFSOperationDueFilter{DueBefore: now, Limit: 10})
	if err != nil {
		t.Fatalf("ListDue() error = %v", err)
	}
	if len(due) != 3 || due[0].ID != pending.ID || due[1].ID != expiredRunning.ID || due[2].ID != dueFailed.ID {
		t.Fatalf("unexpected due operations = %#v", due)
	}

	leaseUntil := now.Add(5 * time.Minute)
	acquired, err := repo.AcquireDue(ctx, domainrepo.VFSOperationDueFilter{DueBefore: now, Limit: 1}, domainrepo.VFSOperationLock{
		WorkerID:    "worker-a",
		LockedUntil: leaseUntil,
	})
	if err != nil {
		t.Fatalf("AcquireDue() error = %v", err)
	}
	if len(acquired) != 1 || acquired[0].ID != pending.ID {
		t.Fatalf("unexpected acquired operations = %#v", acquired)
	}
	if acquired[0].Status != entity.VFSOperationStatusRunning || acquired[0].LockedBy != "worker-a" || acquired[0].LockedUntil == nil || !acquired[0].LockedUntil.Equal(leaseUntil) {
		t.Fatalf("operation was not locked as running: %#v", acquired[0])
	}

	due, err = repo.ListDue(ctx, domainrepo.VFSOperationDueFilter{DueBefore: now, Limit: 10})
	if err != nil {
		t.Fatalf("ListDue(after acquire) error = %v", err)
	}
	if len(due) != 2 || due[0].ID != expiredRunning.ID || due[1].ID != dueFailed.ID {
		t.Fatalf("unexpected due operations after acquire = %#v", due)
	}

	acquired[0].Status = entity.VFSOperationStatusSucceeded
	acquired[0].ErrorCode = ""
	acquired[0].ErrorMessage = ""
	acquired[0].NextRetryAt = nil
	acquired[0].LockedBy = ""
	acquired[0].LockedUntil = nil
	acquired[0].UpdatedAt = now.Add(time.Minute)
	if err := repo.Update(ctx, acquired[0]); err != nil {
		t.Fatalf("Update(mark succeeded) error = %v", err)
	}
	succeeded, err := repo.FindByID(ctx, pending.ID)
	if err != nil {
		t.Fatalf("FindByID(succeeded) error = %v", err)
	}
	if succeeded.Status != entity.VFSOperationStatusSucceeded || succeeded.LockedUntil != nil {
		t.Fatalf("unexpected succeeded operation = %#v", succeeded)
	}

	nextRetryAt := now.Add(5 * time.Minute)
	dueFailed.RetryCount = 2
	dueFailed.NextRetryAt = &nextRetryAt
	dueFailed.ErrorCode = "METADATA_VFS_MUTATION_SYNC_FAILED"
	dueFailed.ErrorMessage = "metadata vfs mutation sync failed"
	dueFailed.UpdatedAt = now.Add(2 * time.Minute)
	if err := repo.Update(ctx, dueFailed); err != nil {
		t.Fatalf("Update(mark failed retry) error = %v", err)
	}
	failed, err := repo.FindByID(ctx, dueFailed.ID)
	if err != nil {
		t.Fatalf("FindByID(failed retry) error = %v", err)
	}
	if failed.Status != entity.VFSOperationStatusFailed || failed.RetryCount != 2 || failed.NextRetryAt == nil || !failed.NextRetryAt.Equal(nextRetryAt) {
		t.Fatalf("unexpected failed retry operation = %#v", failed)
	}
	if failed.ErrorCode != "METADATA_VFS_MUTATION_SYNC_FAILED" || failed.ErrorMessage != "metadata vfs mutation sync failed" {
		t.Fatalf("unexpected failed retry error fields = %#v", failed)
	}
}
