package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/infrastructure/security"
)

func TestVFSMutationsSyncMetadataAfterUnderlyingSuccess(t *testing.T) {
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 7})
	now := fixedMetadataVFSTime()
	nodeRepo := newFakeVFSNodeRepository()
	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSClock(func() time.Time { return now }))
	root, err := metadataSvc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	sourceID := uint(1)
	mountID := uint(10)
	docsNode := mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "docs",
		Path:      "/docs",
		Kind:      entity.VFSNodeKindMount,
		MountID:   &mountID,
		SourceID:  &sourceID,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &docsNode.ID,
		Name:      "hello.txt",
		Path:      "/docs/hello.txt",
		Kind:      entity.VFSNodeKindFile,
		MountID:   &mountID,
		SourceID:  &sourceID,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})

	operator := &vfsFileOperatorSpy{
		renameItem: &appdto.FileItem{
			Name:       "greeting.txt",
			Path:       "/greeting.txt",
			ParentPath: "/",
			SourceID:   sourceID,
			IsDir:      false,
		},
	}
	svc := NewVFSService(
		mountRegistryTestRepo{sources: []*entity.StorageSource{newTestLocalSource(t, sourceID, "文档库", "/docs")}},
		WithVFSFileOperator(operator),
		WithVFSMetadataServices(metadataSvc, nil),
	)

	created, err := svc.Mkdir(ctx, appdto.VFSMkdirRequest{ParentPath: "/docs", Name: "notes"})
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if created.Path != "/docs/notes" {
		t.Fatalf("Mkdir() metadata item path = %s, want /docs/notes", created.Path)
	}
	notes := mustFindMetadataNode(t, nodeRepo, "/docs/notes")
	if notes.SourceID == nil || *notes.SourceID != sourceID || notes.Kind != entity.VFSNodeKindDir {
		t.Fatalf("unexpected mkdir metadata node = %+v", notes)
	}

	oldPath, newPath, _, err := svc.Rename(ctx, appdto.VFSRenameRequest{Path: "/docs/hello.txt", NewName: "greeting.txt"})
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if oldPath != "/docs/hello.txt" || newPath != "/docs/greeting.txt" {
		t.Fatalf("Rename() paths old=%s new=%s", oldPath, newPath)
	}
	if _, err := nodeRepo.FindByPath(ctx, "/docs/hello.txt"); err == nil {
		t.Fatalf("old metadata path should be absent")
	}
	greeting := mustFindMetadataNode(t, nodeRepo, "/docs/greeting.txt")
	if greeting.Name != "greeting.txt" || greeting.UpdatedBy == nil || *greeting.UpdatedBy != 7 {
		t.Fatalf("unexpected renamed metadata node = %+v", greeting)
	}

	oldPath, newPath, err = svc.Move(ctx, appdto.VFSMoveCopyRequest{Path: "/docs/greeting.txt", TargetPath: "/docs/notes"})
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if oldPath != "/docs/greeting.txt" || newPath != "/docs/notes/greeting.txt" {
		t.Fatalf("Move() paths old=%s new=%s", oldPath, newPath)
	}
	if _, err := nodeRepo.FindByPath(ctx, "/docs/greeting.txt"); err == nil {
		t.Fatalf("moved source metadata path should be absent")
	}
	moved := mustFindMetadataNode(t, nodeRepo, "/docs/notes/greeting.txt")
	if moved.ParentID == nil || *moved.ParentID != notes.ID {
		t.Fatalf("unexpected moved metadata node = %+v notes=%+v", moved, notes)
	}

	deletedAt, err := svc.Delete(ctx, appdto.VFSDeleteRequest{Path: "/docs/notes/greeting.txt", DeleteMode: "permanent"})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if deletedAt.IsZero() {
		t.Fatalf("Delete() returned zero deleted_at")
	}
	if _, err := nodeRepo.FindByPath(ctx, "/docs/notes/greeting.txt"); err == nil {
		t.Fatalf("deleted metadata path should be absent")
	}
}

func TestVFSCopyRefreshesMetadataTargetParentAfterSuccess(t *testing.T) {
	ctx := context.Background()
	now := fixedMetadataVFSTime()
	nodeRepo := newFakeVFSNodeRepository()
	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSClock(func() time.Time { return now }))
	if _, err := metadataSvc.EnsureRoot(ctx); err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	refresh := &recordingMetadataRefresh{}
	sourceID := uint(1)
	svc := NewVFSService(
		mountRegistryTestRepo{sources: []*entity.StorageSource{newTestLocalSource(t, sourceID, "文档库", "/docs")}},
		WithVFSFileOperator(&vfsFileOperatorSpy{}),
		WithVFSMetadataServices(metadataSvc, refresh),
	)

	sourcePath, newPath, err := svc.Copy(ctx, appdto.VFSMoveCopyRequest{Path: "/docs/hello.txt", TargetPath: "/docs/notes"})
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	if sourcePath != "/docs/hello.txt" || newPath != "/docs/notes/hello.txt" {
		t.Fatalf("Copy() paths source=%s new=%s", sourcePath, newPath)
	}
	if !reflect.DeepEqual(refresh.paths, []string{"/docs/notes"}) {
		t.Fatalf("expected target parent refresh, got %v", refresh.paths)
	}
}

func TestVFSUnderlyingFailureDoesNotMutateMetadata(t *testing.T) {
	mutation := &recordingMetadataMutation{}
	svc := NewVFSService(
		mountRegistryTestRepo{sources: []*entity.StorageSource{newTestLocalSource(t, 1, "文档库", "/docs")}},
		WithVFSFileOperator(vfsFileOperatorFailing{err: ErrSourceReadOnly}),
		WithVFSMetadataMutationService(mutation),
	)

	_, err := svc.Mkdir(context.Background(), appdto.VFSMkdirRequest{ParentPath: "/docs", Name: "blocked"})
	if !errors.Is(err, ErrSourceReadOnly) {
		t.Fatalf("Mkdir() error = %v, want ErrSourceReadOnly", err)
	}
	if mutation.mkdirCalls != 0 {
		t.Fatalf("metadata mkdir should not run after underlying failure, calls=%d", mutation.mkdirCalls)
	}
}

func TestVFSMetadataMutationFailureIsMasked(t *testing.T) {
	rawErr := errors.New(`sql: duplicate key while touching D:\secret\provider-payload.json`)
	mutation := &recordingMetadataMutation{err: rawErr}
	svc := NewVFSService(
		mountRegistryTestRepo{sources: []*entity.StorageSource{newTestLocalSource(t, 1, "文档库", "/docs")}},
		WithVFSFileOperator(&vfsFileOperatorSpy{}),
		WithVFSMetadataMutationService(mutation),
	)

	_, err := svc.Mkdir(context.Background(), appdto.VFSMkdirRequest{ParentPath: "/docs", Name: "created-underlying"})
	if !errors.Is(err, ErrMetadataVFSMutationSyncFailed) {
		t.Fatalf("Mkdir() error = %v, want ErrMetadataVFSMutationSyncFailed", err)
	}
	if strings.Contains(err.Error(), "D:\\secret") || strings.Contains(err.Error(), "provider-payload") || strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("metadata sync error leaked raw details: %v", err)
	}
	if mutation.mkdirCalls != 1 {
		t.Fatalf("expected one metadata mkdir attempt, got %d", mutation.mkdirCalls)
	}
}

func TestVFSMetadataMutationFailureRecordsOperationJournal(t *testing.T) {
	now := fixedMetadataVFSTime()
	ctx := security.WithRequestAuth(context.Background(), security.RequestAuth{UserID: 42})
	rawErr := errors.New(`sql: duplicate key while touching D:\secret\provider-payload.json`)
	operationRepo := newFakeVFSOperationRepository()
	journal := NewVFSOperationJournalService(operationRepo, WithVFSOperationJournalClock(func() time.Time { return now }))
	svc := NewVFSService(
		mountRegistryTestRepo{sources: []*entity.StorageSource{newTestLocalSource(t, 1, "文档库", "/docs")}},
		WithVFSFileOperator(&vfsFileOperatorSpy{}),
		WithVFSMetadataMutationService(&recordingMetadataMutation{err: rawErr}),
		WithVFSOperationJournal(journal),
	)

	_, err := svc.Mkdir(ctx, appdto.VFSMkdirRequest{ParentPath: "/docs", Name: "created-underlying"})
	if !errors.Is(err, ErrMetadataVFSMutationSyncFailed) {
		t.Fatalf("Mkdir() error = %v, want ErrMetadataVFSMutationSyncFailed", err)
	}

	operations := operationRepo.all()
	if len(operations) != 1 {
		t.Fatalf("recorded operations = %#v, want 1", operations)
	}
	operation := operations[0]
	if operation.OperationType != entity.VFSOperationTypeMkdir || operation.Status != entity.VFSOperationStatusFailed {
		t.Fatalf("unexpected operation type/status = %#v", operation)
	}
	if operation.SourcePathSnapshot != "/docs" || operation.TargetPathSnapshot != "/docs/created-underlying" {
		t.Fatalf("unexpected path snapshots = %#v", operation)
	}
	if operation.SourceIDSnapshot == nil || *operation.SourceIDSnapshot != 1 || operation.DriverTypeSnapshot != "local" {
		t.Fatalf("unexpected source snapshot = %#v", operation)
	}
	if operation.CreatedBy == nil || *operation.CreatedBy != 42 {
		t.Fatalf("unexpected created_by = %#v", operation.CreatedBy)
	}
	if operation.ErrorCode != "METADATA_VFS_MUTATION_SYNC_FAILED" || operation.ErrorMessage != ErrMetadataVFSMutationSyncFailed.Error() {
		t.Fatalf("unexpected sanitized error fields = %#v", operation)
	}
	if strings.Contains(operation.ErrorMessage, "D:\\secret") || strings.Contains(operation.ErrorMessage, "provider-payload") || strings.Contains(operation.ErrorMessage, "duplicate key") {
		t.Fatalf("operation leaked raw metadata failure: %q", operation.ErrorMessage)
	}
	if operation.NextRetryAt == nil || !operation.NextRetryAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("NextRetryAt = %v, want %v", operation.NextRetryAt, now.Add(time.Minute))
	}
}

func TestVFSMetadataSyncFailureIsMaskedForEveryMutation(t *testing.T) {
	rawErr := errors.New(`provider payload contains D:\secret\object.json and sql: duplicate key`)

	cases := []struct {
		name string
		svc  func(t *testing.T) *VFSService
		run  func(*VFSService) error
	}{
		{
			name: "rename",
			svc: func(t *testing.T) *VFSService {
				t.Helper()
				return newVFSMetadataFailureMaskingTestService(t,
					WithVFSFileOperator(&vfsFileOperatorSpy{
						renameItem: &appdto.FileItem{
							Name:       "new.txt",
							Path:       "/new.txt",
							ParentPath: "/",
							SourceID:   1,
						},
					}),
					WithVFSMetadataMutationService(&recordingMetadataMutation{err: rawErr}),
				)
			},
			run: func(svc *VFSService) error {
				_, _, _, err := svc.Rename(context.Background(), appdto.VFSRenameRequest{Path: "/docs/old.txt", NewName: "new.txt"})
				return err
			},
		},
		{
			name: "move",
			svc: func(t *testing.T) *VFSService {
				t.Helper()
				return newVFSMetadataFailureMaskingTestService(t,
					WithVFSFileOperator(&vfsFileOperatorSpy{}),
					WithVFSMetadataMutationService(&recordingMetadataMutation{err: rawErr}),
				)
			},
			run: func(svc *VFSService) error {
				_, _, err := svc.Move(context.Background(), appdto.VFSMoveCopyRequest{Path: "/docs/old.txt", TargetPath: "/docs/target"})
				return err
			},
		},
		{
			name: "copy",
			svc: func(t *testing.T) *VFSService {
				t.Helper()
				return newVFSMetadataFailureMaskingTestService(t,
					WithVFSFileOperator(&vfsFileOperatorSpy{}),
					WithVFSMetadataServices(nil, &recordingMetadataRefresh{err: rawErr}),
				)
			},
			run: func(svc *VFSService) error {
				_, _, err := svc.Copy(context.Background(), appdto.VFSMoveCopyRequest{Path: "/docs/old.txt", TargetPath: "/docs/target"})
				return err
			},
		},
		{
			name: "delete",
			svc: func(t *testing.T) *VFSService {
				t.Helper()
				return newVFSMetadataFailureMaskingTestService(t,
					WithVFSFileOperator(&vfsFileOperatorSpy{}),
					WithVFSMetadataMutationService(&recordingMetadataMutation{err: rawErr}),
				)
			},
			run: func(svc *VFSService) error {
				_, err := svc.Delete(context.Background(), appdto.VFSDeleteRequest{Path: "/docs/old.txt", DeleteMode: "permanent"})
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run(tc.svc(t))
			if !errors.Is(err, ErrMetadataVFSMutationSyncFailed) {
				t.Fatalf("%s error = %v, want ErrMetadataVFSMutationSyncFailed", tc.name, err)
			}
			if strings.Contains(err.Error(), "D:\\secret") || strings.Contains(err.Error(), "provider payload") || strings.Contains(err.Error(), "duplicate key") {
				t.Fatalf("%s metadata sync error leaked raw details: %v", tc.name, err)
			}
		})
	}
}

func newVFSMetadataFailureMaskingTestService(t *testing.T, options ...VFSServiceOption) *VFSService {
	t.Helper()

	baseOptions := []VFSServiceOption{
		WithVFSFileOperator(&vfsFileOperatorSpy{}),
	}
	baseOptions = append(baseOptions, options...)
	return NewVFSService(
		mountRegistryTestRepo{sources: []*entity.StorageSource{newTestLocalSource(t, 1, "文档库", "/docs")}},
		baseOptions...,
	)
}

func mustFindMetadataNode(t *testing.T, repo *fakeVFSNodeRepository, pathValue string) *entity.VFSNode {
	t.Helper()

	node, err := repo.FindByPath(context.Background(), pathValue)
	if err != nil {
		t.Fatalf("FindByPath(%s) error = %v", pathValue, err)
	}
	return node
}

type recordingMetadataRefresh struct {
	paths []string
	err   error
}

func (r *recordingMetadataRefresh) RefreshPath(_ context.Context, targetPath string) (*MetadataVFSRefreshResult, error) {
	r.paths = append(r.paths, targetPath)
	if r.err != nil {
		return nil, r.err
	}
	return &MetadataVFSRefreshResult{Path: targetPath, SyncState: entity.VFSNodeSyncStateIndexed}, nil
}

type recordingMetadataMutation struct {
	mkdirCalls int
	err        error
}

func (m *recordingMetadataMutation) Mkdir(_ context.Context, _ MetadataVFSMkdirRequest) (*appdto.VFSItem, error) {
	m.mkdirCalls++
	if m.err != nil {
		return nil, m.err
	}
	return &appdto.VFSItem{Path: "/metadata-created"}, nil
}

func (m *recordingMetadataMutation) Rename(_ context.Context, _ MetadataVFSRenameRequest) (string, *appdto.VFSItem, error) {
	if m.err != nil {
		return "", nil, m.err
	}
	return "", &appdto.VFSItem{}, nil
}

func (m *recordingMetadataMutation) Move(_ context.Context, _ MetadataVFSMoveRequest) (string, *appdto.VFSItem, error) {
	if m.err != nil {
		return "", nil, m.err
	}
	return "", &appdto.VFSItem{}, nil
}

func (m *recordingMetadataMutation) Delete(context.Context, string) (time.Time, error) {
	if m.err != nil {
		return time.Time{}, m.err
	}
	return time.Now(), nil
}

type vfsFileOperatorFailing struct {
	err error
}

func (o vfsFileOperatorFailing) Mkdir(context.Context, appdto.MkdirRequest) (*appdto.FileItem, error) {
	return nil, o.err
}

func (o vfsFileOperatorFailing) Rename(context.Context, appdto.RenameRequest) (string, string, *appdto.FileItem, error) {
	return "", "", nil, o.err
}

func (o vfsFileOperatorFailing) Move(context.Context, appdto.MoveCopyRequest) (string, string, error) {
	return "", "", o.err
}

func (o vfsFileOperatorFailing) Copy(context.Context, appdto.MoveCopyRequest) (string, string, error) {
	return "", "", o.err
}

func (o vfsFileOperatorFailing) Delete(context.Context, appdto.DeleteFileRequest) (time.Time, error) {
	return time.Time{}, o.err
}
