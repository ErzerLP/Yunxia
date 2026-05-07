package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

func TestVFSOperationJournalRecordFailureSanitizesAndSchedulesRetry(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 0, 0, 0, time.UTC)
	repo := newFakeVFSOperationRepository()
	service := NewVFSOperationJournalService(repo, WithVFSOperationJournalClock(func() time.Time { return now }))
	actorID := uint(7)
	sourceID := uint(10)
	rawErr := errors.New(`sql duplicate key while touching D:\secret\provider-payload.json`)

	operation, err := service.RecordFailure(context.Background(), VFSOperationRecordInput{
		OperationType:      entity.VFSOperationTypeMkdir,
		SourcePathSnapshot: "/docs",
		TargetPathSnapshot: "/docs/new",
		SourceIDSnapshot:   &sourceID,
		DriverTypeSnapshot: "local",
		ErrorCode:          "METADATA_VFS_MUTATION_SYNC_FAILED",
		Error:              rawErr,
		CreatedBy:          &actorID,
		Payload:            map[string]any{"phase": "metadata_sync"},
	})
	if err != nil {
		t.Fatalf("RecordFailure() error = %v", err)
	}
	if operation.Status != entity.VFSOperationStatusFailed || operation.RetryCount != 0 {
		t.Fatalf("unexpected failure operation status/retry = %#v", operation)
	}
	if operation.NextRetryAt == nil || !operation.NextRetryAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("NextRetryAt = %v, want %v", operation.NextRetryAt, now.Add(time.Minute))
	}
	if operation.ErrorCode != "METADATA_VFS_MUTATION_SYNC_FAILED" || operation.ErrorMessage != ErrMetadataVFSMutationSyncFailed.Error() {
		t.Fatalf("unexpected sanitized error fields = %#v", operation)
	}
	if strings.Contains(operation.ErrorMessage, "D:\\secret") || strings.Contains(operation.ErrorMessage, "provider-payload") || strings.Contains(operation.ErrorMessage, "duplicate key") {
		t.Fatalf("operation error_message leaked raw details: %q", operation.ErrorMessage)
	}
	if operation.PayloadJSON != `{"phase":"metadata_sync"}` {
		t.Fatalf("PayloadJSON = %q, want phase metadata_sync", operation.PayloadJSON)
	}
	if operation.CreatedBy == nil || *operation.CreatedBy != actorID || operation.SourceIDSnapshot == nil || *operation.SourceIDSnapshot != sourceID {
		t.Fatalf("unexpected snapshot fields = %#v", operation)
	}
}

func TestVFSOperationJournalPayloadSanitizesSensitiveDetails(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 30, 0, 0, time.UTC)
	repo := newFakeVFSOperationRepository()
	service := NewVFSOperationJournalService(repo, WithVFSOperationJournalClock(func() time.Time { return now }))

	operation, err := service.RecordPending(context.Background(), VFSOperationRecordInput{
		OperationType: entity.VFSOperationTypeRefresh,
		Payload: map[string]any{
			"phase":            "metadata_sync",
			"local_path":       `D:\secret\file.txt`,
			"provider_payload": map[string]any{"access_token": "raw-token"},
			"details":          []any{"safe", "/mnt/provider/raw-object.json"},
		},
	})
	if err != nil {
		t.Fatalf("RecordPending() error = %v", err)
	}
	if strings.Contains(operation.PayloadJSON, `D:\secret`) ||
		strings.Contains(operation.PayloadJSON, "/mnt/provider") ||
		strings.Contains(operation.PayloadJSON, "raw-token") ||
		strings.Contains(operation.PayloadJSON, "provider_payload") && strings.Contains(operation.PayloadJSON, "access_token") {
		t.Fatalf("PayloadJSON leaked sensitive details: %s", operation.PayloadJSON)
	}
	if !strings.Contains(operation.PayloadJSON, `"phase":"metadata_sync"`) || !strings.Contains(operation.PayloadJSON, vfsOperationRedactedValue) {
		t.Fatalf("PayloadJSON did not preserve safe fields and redactions: %s", operation.PayloadJSON)
	}

	rawOperation, err := service.RecordPending(context.Background(), VFSOperationRecordInput{
		OperationType: entity.VFSOperationTypeRefresh,
		PayloadJSON:   `{"phase":"metadata_sync","sql":"select * from vfs_nodes","path":"/mnt/provider/raw-object.json"}`,
	})
	if err != nil {
		t.Fatalf("RecordPending(raw) error = %v", err)
	}
	if strings.Contains(rawOperation.PayloadJSON, "select *") || strings.Contains(rawOperation.PayloadJSON, "/mnt/provider") {
		t.Fatalf("raw PayloadJSON leaked sensitive details: %s", rawOperation.PayloadJSON)
	}
}

func TestVFSOperationJournalAcquireMarkFailedAndSucceeded(t *testing.T) {
	now := time.Date(2026, 5, 7, 15, 0, 0, 0, time.UTC)
	repo := newFakeVFSOperationRepository()
	service := NewVFSOperationJournalService(
		repo,
		WithVFSOperationJournalClock(func() time.Time { return now }),
		WithVFSOperationJournalLockDuration(10*time.Minute),
	)

	pending, err := service.RecordPending(context.Background(), VFSOperationRecordInput{
		OperationType:      entity.VFSOperationTypeRefresh,
		TargetPathSnapshot: "/docs",
	})
	if err != nil {
		t.Fatalf("RecordPending() error = %v", err)
	}
	due, err := service.ListDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListDue() error = %v", err)
	}
	if len(due) != 1 || due[0].ID != pending.ID {
		t.Fatalf("unexpected due operations = %#v", due)
	}

	acquired, err := service.AcquireDue(context.Background(), "worker-a", 1)
	if err != nil {
		t.Fatalf("AcquireDue() error = %v", err)
	}
	if len(acquired) != 1 || acquired[0].ID != pending.ID || acquired[0].Status != entity.VFSOperationStatusRunning {
		t.Fatalf("unexpected acquired operations = %#v", acquired)
	}
	if acquired[0].LockedBy != "worker-a" || acquired[0].LockedUntil == nil || !acquired[0].LockedUntil.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("unexpected lock fields = %#v", acquired[0])
	}
	if due, err = service.ListDue(context.Background(), 10); err != nil {
		t.Fatalf("ListDue(after acquire) error = %v", err)
	} else if len(due) != 0 {
		t.Fatalf("locked running operation should not be due, got %#v", due)
	}

	failed, err := service.MarkFailed(context.Background(), pending.ID, errors.New(`/mnt/secret/provider payload`))
	if err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}
	if failed.Status != entity.VFSOperationStatusFailed || failed.RetryCount != 1 {
		t.Fatalf("unexpected failed operation = %#v", failed)
	}
	if failed.NextRetryAt == nil || !failed.NextRetryAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("MarkFailed next_retry_at = %v, want %v", failed.NextRetryAt, now.Add(time.Minute))
	}
	if failed.LockedBy != "" || failed.LockedUntil != nil {
		t.Fatalf("MarkFailed should clear lock fields: %#v", failed)
	}
	if failed.ErrorMessage != "vfs operation failed" || strings.Contains(failed.ErrorMessage, "/mnt/secret") {
		t.Fatalf("MarkFailed error_message not sanitized: %q", failed.ErrorMessage)
	}

	now = now.Add(time.Minute + time.Nanosecond)
	due, err = service.ListDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListDue(after backoff) error = %v", err)
	}
	if len(due) != 1 || due[0].ID != pending.ID {
		t.Fatalf("failed operation should become due after backoff, got %#v", due)
	}

	succeeded, err := service.MarkSucceeded(context.Background(), pending.ID)
	if err != nil {
		t.Fatalf("MarkSucceeded() error = %v", err)
	}
	if succeeded.Status != entity.VFSOperationStatusSucceeded || succeeded.ErrorCode != "" || succeeded.NextRetryAt != nil {
		t.Fatalf("unexpected succeeded operation = %#v", succeeded)
	}
}

func TestVFSOperationJournalProcessDueMarksProcessorResults(t *testing.T) {
	now := time.Date(2026, 5, 7, 16, 0, 0, 0, time.UTC)
	repo := newFakeVFSOperationRepository()
	service := NewVFSOperationJournalService(repo, WithVFSOperationJournalClock(func() time.Time { return now }))
	okOperation, err := service.RecordPending(context.Background(), VFSOperationRecordInput{
		OperationType:      entity.VFSOperationTypeRefresh,
		TargetPathSnapshot: "/ok",
	})
	if err != nil {
		t.Fatalf("RecordPending(ok) error = %v", err)
	}
	failedOperation, err := service.RecordPending(context.Background(), VFSOperationRecordInput{
		OperationType:      entity.VFSOperationTypeRefresh,
		TargetPathSnapshot: "/fail",
	})
	if err != nil {
		t.Fatalf("RecordPending(fail) error = %v", err)
	}

	processed, err := service.ProcessDue(context.Background(), "worker-a", 10, vfsOperationProcessorFunc(func(_ context.Context, operation *entity.VFSOperation) error {
		if operation.TargetPathSnapshot == "/fail" {
			return errors.New("provider raw payload should not be stored")
		}
		return nil
	}))
	if processed != 2 {
		t.Fatalf("ProcessDue processed = %d, want 2", processed)
	}
	if err == nil {
		t.Fatalf("ProcessDue() expected joined processor error")
	}
	if strings.Contains(err.Error(), "provider raw payload") {
		t.Fatalf("ProcessDue() returned raw processor error: %v", err)
	}
	okSaved, err := repo.FindByID(context.Background(), okOperation.ID)
	if err != nil {
		t.Fatalf("FindByID(ok) error = %v", err)
	}
	failSaved, err := repo.FindByID(context.Background(), failedOperation.ID)
	if err != nil {
		t.Fatalf("FindByID(fail) error = %v", err)
	}
	if okSaved.Status != entity.VFSOperationStatusSucceeded {
		t.Fatalf("ok operation status = %s, want succeeded", okSaved.Status)
	}
	if failSaved.Status != entity.VFSOperationStatusFailed || failSaved.RetryCount != 1 || failSaved.NextRetryAt == nil || !failSaved.NextRetryAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("failed operation retry fields = %#v", failSaved)
	}
	if strings.Contains(failSaved.ErrorMessage, "provider raw payload") {
		t.Fatalf("processor failure leaked raw error: %q", failSaved.ErrorMessage)
	}
}

type vfsOperationProcessorFunc func(context.Context, *entity.VFSOperation) error

func (f vfsOperationProcessorFunc) ProcessVFSOperation(ctx context.Context, operation *entity.VFSOperation) error {
	return f(ctx, operation)
}

type fakeVFSOperationRepository struct {
	nextID     uint
	operations map[uint]*entity.VFSOperation
}

func newFakeVFSOperationRepository() *fakeVFSOperationRepository {
	return &fakeVFSOperationRepository{
		nextID:     1,
		operations: make(map[uint]*entity.VFSOperation),
	}
}

func (r *fakeVFSOperationRepository) Create(_ context.Context, operation *entity.VFSOperation) error {
	if operation.ID == 0 {
		operation.ID = r.nextID
		r.nextID++
	}
	r.operations[operation.ID] = cloneVFSOperation(operation)
	*operation = *cloneVFSOperation(operation)
	return nil
}

func (r *fakeVFSOperationRepository) Update(_ context.Context, operation *entity.VFSOperation) error {
	if _, ok := r.operations[operation.ID]; !ok {
		return domainrepo.ErrNotFound
	}
	r.operations[operation.ID] = cloneVFSOperation(operation)
	return nil
}

func (r *fakeVFSOperationRepository) FindByID(_ context.Context, id uint) (*entity.VFSOperation, error) {
	operation, ok := r.operations[id]
	if !ok {
		return nil, domainrepo.ErrNotFound
	}
	return cloneVFSOperation(operation), nil
}

func (r *fakeVFSOperationRepository) ListDue(_ context.Context, filter domainrepo.VFSOperationDueFilter) ([]*entity.VFSOperation, error) {
	return r.filterDue(filter), nil
}

func (r *fakeVFSOperationRepository) AcquireDue(_ context.Context, filter domainrepo.VFSOperationDueFilter, lock domainrepo.VFSOperationLock) ([]*entity.VFSOperation, error) {
	due := r.filterDue(filter)
	acquired := make([]*entity.VFSOperation, 0, len(due))
	for _, operation := range due {
		current := cloneVFSOperation(operation)
		current.Status = entity.VFSOperationStatusRunning
		current.LockedBy = lock.WorkerID
		current.LockedUntil = &lock.LockedUntil
		r.operations[current.ID] = cloneVFSOperation(current)
		acquired = append(acquired, current)
	}
	return acquired, nil
}

func (r *fakeVFSOperationRepository) all() []*entity.VFSOperation {
	items := make([]*entity.VFSOperation, 0, len(r.operations))
	for _, operation := range r.operations {
		items = append(items, cloneVFSOperation(operation))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func (r *fakeVFSOperationRepository) filterDue(filter domainrepo.VFSOperationDueFilter) []*entity.VFSOperation {
	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = []string{
			entity.VFSOperationStatusPending,
			entity.VFSOperationStatusFailed,
			entity.VFSOperationStatusRunning,
		}
	}
	statusSet := map[string]struct{}{}
	for _, status := range statuses {
		statusSet[status] = struct{}{}
	}
	dueBefore := filter.DueBefore
	if dueBefore.IsZero() {
		dueBefore = time.Now().UTC()
	}
	items := make([]*entity.VFSOperation, 0)
	for _, operation := range r.operations {
		if _, ok := statusSet[operation.Status]; !ok {
			continue
		}
		if filter.OperationType != "" && operation.OperationType != filter.OperationType {
			continue
		}
		if operation.NextRetryAt != nil && operation.NextRetryAt.After(dueBefore) {
			continue
		}
		if !filter.IncludeLocked && operation.LockedUntil != nil && operation.LockedUntil.After(dueBefore) {
			continue
		}
		items = append(items, cloneVFSOperation(operation))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].NextRetryAt == nil && items[j].NextRetryAt != nil {
			return true
		}
		if items[i].NextRetryAt != nil && items[j].NextRetryAt == nil {
			return false
		}
		if items[i].NextRetryAt != nil && items[j].NextRetryAt != nil && !items[i].NextRetryAt.Equal(*items[j].NextRetryAt) {
			return items[i].NextRetryAt.Before(*items[j].NextRetryAt)
		}
		return items[i].ID < items[j].ID
	})
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func cloneVFSOperation(operation *entity.VFSOperation) *entity.VFSOperation {
	if operation == nil {
		return nil
	}
	clone := *operation
	return &clone
}
