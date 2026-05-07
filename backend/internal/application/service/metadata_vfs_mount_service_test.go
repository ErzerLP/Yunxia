package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
	gormrepo "yunxia/internal/infrastructure/persistence/gorm"
)

func TestMetadataVFSMountServiceSyncSourceMountCreatesControlPlane(t *testing.T) {
	ctx := context.Background()
	now := fixedMetadataVFSTime()
	nodeRepo := newFakeVFSNodeRepository()
	mountRepo := newFakeVFSMountRepository()
	sourceRepo := newFakeMetadataVFSSyncSourceRepository()
	svc := NewMetadataVFSMountService(
		nodeRepo,
		mountRepo,
		sourceRepo,
		WithMetadataVFSMountClock(func() time.Time { return now }),
	)
	source := &entity.StorageSource{
		ID:         10,
		Name:       "PikPak",
		DriverType: "pikpak",
		IsEnabled:  true,
		MountPath:  "/media/pikpak",
		RootPath:   "/",
		SortOrder:  7,
		ConfigJSON: `{"root_folder_id":"root","password":"secret","refresh_token":"refresh","base_prefix":"/library","base_path":"C:/secret/root"}`,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	result, err := svc.SyncSourceMount(ctx, source)
	if err != nil {
		t.Fatalf("SyncSourceMount() error = %v", err)
	}
	if result.MountPath != "/media/pikpak" || result.Node == nil || result.Mount == nil {
		t.Fatalf("unexpected sync result = %#v", result)
	}

	media := mustFindVFSNodeByPath(t, nodeRepo, "/media")
	if media.Kind != entity.VFSNodeKindVirtualDir || media.SourceID != nil {
		t.Fatalf("/media should be virtual_dir without source, got %#v", media)
	}
	mountNode := mustFindVFSNodeByPath(t, nodeRepo, "/media/pikpak")
	if mountNode.Kind != entity.VFSNodeKindMount || mountNode.SourceID == nil || *mountNode.SourceID != source.ID {
		t.Fatalf("mount node mismatch = %#v", mountNode)
	}
	if mountNode.MountID == nil {
		t.Fatalf("mount node should reference vfs_mount id: %#v", mountNode)
	}
	mount, err := mountRepo.FindByMountPath(ctx, "/media/pikpak")
	if err != nil {
		t.Fatalf("FindByMountPath() error = %v", err)
	}
	if mount.ID != *mountNode.MountID || mount.NodeID != mountNode.ID || mount.SourceID != source.ID || !mount.IsEnabled || mount.SortOrder != source.SortOrder {
		t.Fatalf("unexpected vfs_mount = %#v, node = %#v", mount, mountNode)
	}
	if strings.Contains(mount.RootLocatorJSON, "secret") || strings.Contains(mount.RootLocatorJSON, "refresh") || strings.Contains(mount.RootLocatorJSON, "C:/secret/root") {
		t.Fatalf("root locator leaked secret material: %s", mount.RootLocatorJSON)
	}
	var locator map[string]any
	if err := json.Unmarshal([]byte(mount.RootLocatorJSON), &locator); err != nil {
		t.Fatalf("root locator is not json: %v", err)
	}
	if locator["source_root_path"] != "/" || locator["mount_path"] != "/media/pikpak" || locator["driver_type"] != "pikpak" {
		t.Fatalf("unexpected root locator = %#v", locator)
	}
	configRoot, ok := locator["config_root"].(map[string]any)
	if !ok || configRoot["root_folder_id"] != "root" || configRoot["base_prefix"] != "/library" {
		t.Fatalf("unexpected root config snapshot = %#v", locator["config_root"])
	}
	if _, exists := configRoot["password"]; exists {
		t.Fatalf("secret key should not be present in config_root: %#v", configRoot)
	}
	if configRoot["base_path"] != "[redacted]" {
		t.Fatalf("physical base_path should be redacted in config_root: %#v", configRoot)
	}
}

func TestMetadataVFSMountServiceRenameDisablesOldMountAndDisabledSourceDisablesCurrent(t *testing.T) {
	ctx := context.Background()
	now := fixedMetadataVFSTime()
	nodeRepo := newFakeVFSNodeRepository()
	mountRepo := newFakeVFSMountRepository()
	sourceRepo := newFakeMetadataVFSSyncSourceRepository()
	clockNow := now
	svc := NewMetadataVFSMountService(
		nodeRepo,
		mountRepo,
		sourceRepo,
		WithMetadataVFSMountClock(func() time.Time { return clockNow }),
	)
	source := &entity.StorageSource{
		ID:         11,
		Name:       "Cloud",
		DriverType: "fakecloud",
		IsEnabled:  true,
		MountPath:  "/media/cloud",
		RootPath:   "/",
		ConfigJSON: "{}",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if _, err := svc.SyncSourceMount(ctx, source); err != nil {
		t.Fatalf("initial SyncSourceMount() error = %v", err)
	}
	oldMount, err := mountRepo.FindByMountPath(ctx, "/media/cloud")
	if err != nil {
		t.Fatalf("FindByMountPath(old) error = %v", err)
	}

	clockNow = now.Add(time.Minute)
	source.MountPath = "/media/cloud-renamed"
	if _, err := svc.SyncSourceMount(ctx, source); err != nil {
		t.Fatalf("rename SyncSourceMount() error = %v", err)
	}
	newMount, err := mountRepo.FindByMountPath(ctx, "/media/cloud-renamed")
	if err != nil {
		t.Fatalf("FindByMountPath(new) error = %v", err)
	}
	if !newMount.IsEnabled || newMount.SourceID != source.ID {
		t.Fatalf("new mount should be enabled for same source, got %#v", newMount)
	}
	oldMount, err = mountRepo.FindByID(ctx, oldMount.ID)
	if err != nil {
		t.Fatalf("FindByID(old) error = %v", err)
	}
	if oldMount.IsEnabled {
		t.Fatalf("old mount should be disabled after rename: %#v", oldMount)
	}
	oldNode, err := nodeRepo.FindByID(ctx, oldMount.NodeID)
	if err != nil {
		t.Fatalf("FindByID(old node) error = %v", err)
	}
	if oldNode.IsDeleted || oldNode.SyncState != entity.VFSNodeSyncStateStale {
		t.Fatalf("old mount node should be preserved and stale, got %#v", oldNode)
	}
	replacement := &entity.StorageSource{
		ID:         12,
		Name:       "Replacement",
		DriverType: "fakecloud",
		IsEnabled:  true,
		MountPath:  "/media/cloud",
		RootPath:   "/",
		ConfigJSON: "{}",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if _, err := svc.SyncSourceMount(ctx, replacement); err != nil {
		t.Fatalf("replacement should reuse disabled old mount path: %v", err)
	}
	reusedMount, err := mountRepo.FindByMountPath(ctx, "/media/cloud")
	if err != nil {
		t.Fatalf("FindByMountPath(reused) error = %v", err)
	}
	if !reusedMount.IsEnabled || reusedMount.SourceID != replacement.ID {
		t.Fatalf("disabled old mount should be reusable by replacement source, got %#v", reusedMount)
	}

	clockNow = now.Add(2 * time.Minute)
	source.IsEnabled = false
	if _, err := svc.SyncSourceMount(ctx, source); err != nil {
		t.Fatalf("disabled SyncSourceMount() error = %v", err)
	}
	disabledMount, err := mountRepo.FindByMountPath(ctx, "/media/cloud-renamed")
	if err != nil {
		t.Fatalf("FindByMountPath(disabled) error = %v", err)
	}
	if disabledMount.IsEnabled {
		t.Fatalf("current mount should follow disabled source: %#v", disabledMount)
	}
	currentNode := mustFindVFSNodeByPath(t, nodeRepo, "/media/cloud-renamed")
	if currentNode.IsDeleted {
		t.Fatalf("disabled source should preserve mount node: %#v", currentNode)
	}
}

func TestMetadataVFSMountServicePreservesRootMountWhenSyncingNestedMount(t *testing.T) {
	ctx := context.Background()
	now := fixedMetadataVFSTime()
	nodeRepo := newFakeVFSNodeRepository()
	mountRepo := newFakeVFSMountRepository()
	sourceRepo := newFakeMetadataVFSSyncSourceRepository()
	svc := NewMetadataVFSMountService(
		nodeRepo,
		mountRepo,
		sourceRepo,
		WithMetadataVFSMountClock(func() time.Time { return now }),
	)
	rootSource := &entity.StorageSource{
		ID:         21,
		Name:       "root",
		DriverType: "local",
		IsEnabled:  true,
		MountPath:  "/",
		RootPath:   "/",
		ConfigJSON: "{}",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	nestedSource := &entity.StorageSource{
		ID:         22,
		Name:       "nested",
		DriverType: "local",
		IsEnabled:  true,
		MountPath:  "/cloud",
		RootPath:   "/",
		ConfigJSON: "{}",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if _, err := svc.SyncSourceMount(ctx, rootSource); err != nil {
		t.Fatalf("SyncSourceMount(root) error = %v", err)
	}
	if _, err := svc.SyncSourceMount(ctx, nestedSource); err != nil {
		t.Fatalf("SyncSourceMount(nested) error = %v", err)
	}
	root := mustFindVFSNodeByPath(t, nodeRepo, "/")
	if root.SourceID == nil || *root.SourceID != rootSource.ID {
		t.Fatalf("root mount source should be preserved after nested sync: %#v", root)
	}
}

func TestMetadataVFSMountServiceSyncAllContinuesAfterBadSource(t *testing.T) {
	ctx := context.Background()
	now := fixedMetadataVFSTime()
	nodeRepo := newFakeVFSNodeRepository()
	mountRepo := newFakeVFSMountRepository()
	sourceRepo := newFakeMetadataVFSSyncSourceRepository(
		&entity.StorageSource{
			ID:         1,
			Name:       "ok",
			DriverType: "local",
			IsEnabled:  true,
			MountPath:  "/ok",
			RootPath:   "/",
			ConfigJSON: "{}",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		&entity.StorageSource{
			ID:         2,
			Name:       "bad",
			DriverType: "local",
			IsEnabled:  true,
			MountPath:  "relative",
			RootPath:   "/",
			ConfigJSON: "{}",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	)
	svc := NewMetadataVFSMountService(
		nodeRepo,
		mountRepo,
		sourceRepo,
		WithMetadataVFSMountClock(func() time.Time { return now }),
	)

	result, err := svc.SyncAllSourceMounts(ctx)
	if err != nil {
		t.Fatalf("SyncAllSourceMounts() error = %v", err)
	}
	if result.Synced != 1 || result.Failed != 1 || len(result.Errors) != 1 || result.Errors[0].SourceID != 2 {
		t.Fatalf("unexpected sync all result = %#v", result)
	}
	if _, err := mountRepo.FindByMountPath(ctx, "/ok"); err != nil {
		t.Fatalf("good source should still be synced: %v", err)
	}
}

func TestSourceServiceCreateUpdateInvokeMountSyncer(t *testing.T) {
	ctx := context.Background()
	sourceRepo := newFakeMetadataVFSSyncSourceRepository()
	syncer := &fakeSourceMountSyncer{}
	svc := NewSourceService(
		sourceRepo,
		nil,
		WithSourceConfigCodec(fakeSourceConfigCodec{driverType: "fakecloud", slug: "cloud"}),
		WithSourceDriverProbe("fakecloud", sourceServiceNoopProbe{}),
		WithSourceMountSyncer(syncer),
	)
	req := appdto.SourceUpsertRequest{
		Name:       "Cloud",
		DriverType: "fakecloud",
		IsEnabled:  true,
		MountPath:  "/media/cloud",
		RootPath:   "/",
		Config:     map[string]any{},
	}

	created, err := svc.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(syncer.synced) != 1 || syncer.synced[0].ID != created.ID || syncer.synced[0].MountPath != "/media/cloud" {
		t.Fatalf("create should sync source mount, calls = %#v", syncer.synced)
	}

	req.MountPath = "/media/cloud-renamed"
	updated, err := svc.Update(ctx, created.ID, req)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.MountPath != "/media/cloud-renamed" || len(syncer.synced) != 2 || syncer.synced[1].MountPath != "/media/cloud-renamed" {
		t.Fatalf("update should sync renamed mount, updated = %#v calls = %#v", updated, syncer.synced)
	}
}

func TestSourceServiceDeleteDisablesMetadataMount(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()
	sourceRepo := gormrepo.NewSourceRepository(db)
	configRepo := gormrepo.NewSystemConfigRepository(db)
	basePath := t.TempDir()
	createSvc := NewSourceService(sourceRepo, configRepo)
	created, err := createSvc.Create(ctx, appdto.SourceUpsertRequest{
		Name:       "Local",
		DriverType: "local",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		Config:     map[string]any{"base_path": basePath},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	syncer := &fakeSourceMountSyncer{}
	deleteSvc := NewSourceService(
		sourceRepo,
		configRepo,
		WithSourceMountSyncer(syncer),
		WithSourceTransactor(gormrepo.NewTransactor(db)),
	)
	if err := deleteSvc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(syncer.disabled) != 1 || syncer.disabled[0] != created.ID {
		t.Fatalf("delete should disable metadata mount, calls = %#v", syncer.disabled)
	}
	if _, err := sourceRepo.FindByID(ctx, created.ID); !errors.Is(err, domainrepo.ErrNotFound) {
		t.Fatalf("source should be deleted, FindByID error = %v", err)
	}
}

func TestSourceServiceCreateUpdateReturnStableMountSyncError(t *testing.T) {
	ctx := context.Background()
	sourceRepo := newFakeMetadataVFSSyncSourceRepository()
	syncer := &fakeSourceMountSyncer{err: ErrPathInvalid}
	svc := NewSourceService(
		sourceRepo,
		nil,
		WithSourceConfigCodec(fakeSourceConfigCodec{driverType: "fakecloud", slug: "cloud"}),
		WithSourceDriverProbe("fakecloud", sourceServiceNoopProbe{}),
		WithSourceMountSyncer(syncer),
	)
	req := appdto.SourceUpsertRequest{
		Name:       "Cloud",
		DriverType: "fakecloud",
		IsEnabled:  true,
		MountPath:  "/media/cloud",
		RootPath:   "/",
		Config:     map[string]any{},
	}
	if _, err := svc.Create(ctx, req); !errors.Is(err, ErrMetadataVFSMountSyncFailed) {
		t.Fatalf("Create() error = %v, want ErrMetadataVFSMountSyncFailed", err)
	}

	syncer.err = nil
	created, err := svc.Create(ctx, appdto.SourceUpsertRequest{
		Name:       "Cloud OK",
		DriverType: "fakecloud",
		IsEnabled:  true,
		MountPath:  "/media/cloud-ok",
		RootPath:   "/",
		Config:     map[string]any{},
	})
	if err != nil {
		t.Fatalf("Create(ok) error = %v", err)
	}
	syncer.err = ErrNameConflict
	if _, err := svc.Update(ctx, created.ID, appdto.SourceUpsertRequest{
		Name:      "Cloud OK",
		IsEnabled: true,
		MountPath: "/media/cloud-renamed",
		RootPath:  "/",
		Config:    map[string]any{},
	}); !errors.Is(err, ErrMetadataVFSMountSyncFailed) {
		t.Fatalf("Update() error = %v, want ErrMetadataVFSMountSyncFailed", err)
	}
}

func TestSourceServiceMountSyncFailureRollsBackWithTransactor(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()
	sourceRepo := gormrepo.NewSourceRepository(db)
	configRepo := gormrepo.NewSystemConfigRepository(db)
	syncer := &fakeSourceMountSyncer{err: ErrPathInvalid}
	basePath := t.TempDir()
	svc := NewSourceService(
		sourceRepo,
		configRepo,
		WithSourceMountSyncer(syncer),
		WithSourceTransactor(gormrepo.NewTransactor(db)),
	)
	req := appdto.SourceUpsertRequest{
		Name:       "Local",
		DriverType: "local",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		Config:     map[string]any{"base_path": basePath},
	}
	if _, err := svc.Create(ctx, req); !errors.Is(err, ErrMetadataVFSMountSyncFailed) {
		t.Fatalf("Create() error = %v, want ErrMetadataVFSMountSyncFailed", err)
	}
	if _, err := sourceRepo.FindByName(ctx, "Local"); !errors.Is(err, domainrepo.ErrNotFound) {
		t.Fatalf("source create should be rolled back, FindByName error = %v", err)
	}

	okSvc := NewSourceService(sourceRepo, configRepo)
	created, err := okSvc.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create(ok) error = %v", err)
	}
	if _, err := svc.Update(ctx, created.ID, appdto.SourceUpsertRequest{
		Name:      "Local",
		IsEnabled: true,
		MountPath: "/local-renamed",
		RootPath:  "/",
		Config:    map[string]any{"base_path": basePath},
	}); !errors.Is(err, ErrMetadataVFSMountSyncFailed) {
		t.Fatalf("Update() error = %v, want ErrMetadataVFSMountSyncFailed", err)
	}
	stored, err := sourceRepo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if stored.MountPath != "/local" {
		t.Fatalf("source update should be rolled back, mount_path = %q", stored.MountPath)
	}
}

func mustFindVFSNodeByPath(t *testing.T, repo *fakeVFSNodeRepository, nodePath string) *entity.VFSNode {
	t.Helper()
	node, err := repo.FindByPath(context.Background(), nodePath)
	if err != nil {
		t.Fatalf("FindByPath(%q) error = %v", nodePath, err)
	}
	return node
}

type fakeSourceMountSyncer struct {
	err      error
	synced   []*entity.StorageSource
	disabled []uint
}

func (s *fakeSourceMountSyncer) SyncSourceMount(_ context.Context, source *entity.StorageSource) (*MetadataVFSMountSyncResult, error) {
	s.synced = append(s.synced, cloneStorageSourceForSyncTest(source))
	if s.err != nil {
		return nil, s.err
	}
	return &MetadataVFSMountSyncResult{
		SourceID:  source.ID,
		MountPath: source.MountPath,
	}, nil
}

func (s *fakeSourceMountSyncer) DisableSourceMount(_ context.Context, sourceID uint) error {
	s.disabled = append(s.disabled, sourceID)
	if s.err != nil {
		return s.err
	}
	return nil
}

type sourceServiceNoopProbe struct{}

func (sourceServiceNoopProbe) Test(context.Context, *entity.StorageSource) error {
	return nil
}

type fakeVFSMountRepository struct {
	nextID uint
	mounts map[uint]*entity.VFSMount
}

func newFakeVFSMountRepository() *fakeVFSMountRepository {
	return &fakeVFSMountRepository{
		nextID: 1,
		mounts: make(map[uint]*entity.VFSMount),
	}
}

func (r *fakeVFSMountRepository) Create(_ context.Context, mount *entity.VFSMount) error {
	if err := r.ensureMountUnique(mount, 0); err != nil {
		return err
	}
	if mount.ID == 0 {
		mount.ID = r.nextID
		r.nextID++
	}
	r.mounts[mount.ID] = cloneVFSMountForTest(mount)
	*mount = *cloneVFSMountForTest(r.mounts[mount.ID])
	return nil
}

func (r *fakeVFSMountRepository) Update(_ context.Context, mount *entity.VFSMount) error {
	if _, exists := r.mounts[mount.ID]; !exists {
		return domainrepo.ErrNotFound
	}
	if err := r.ensureMountUnique(mount, mount.ID); err != nil {
		return err
	}
	r.mounts[mount.ID] = cloneVFSMountForTest(mount)
	*mount = *cloneVFSMountForTest(r.mounts[mount.ID])
	return nil
}

func (r *fakeVFSMountRepository) Delete(_ context.Context, id uint) error {
	if _, exists := r.mounts[id]; !exists {
		return domainrepo.ErrNotFound
	}
	delete(r.mounts, id)
	return nil
}

func (r *fakeVFSMountRepository) FindByID(_ context.Context, id uint) (*entity.VFSMount, error) {
	mount, exists := r.mounts[id]
	if !exists {
		return nil, domainrepo.ErrNotFound
	}
	return cloneVFSMountForTest(mount), nil
}

func (r *fakeVFSMountRepository) FindByNodeID(_ context.Context, nodeID uint) (*entity.VFSMount, error) {
	for _, mount := range r.mounts {
		if mount.NodeID == nodeID {
			return cloneVFSMountForTest(mount), nil
		}
	}
	return nil, domainrepo.ErrNotFound
}

func (r *fakeVFSMountRepository) FindByMountPath(_ context.Context, mountPath string) (*entity.VFSMount, error) {
	for _, mount := range r.mounts {
		if mount.MountPath == mountPath {
			return cloneVFSMountForTest(mount), nil
		}
	}
	return nil, domainrepo.ErrNotFound
}

func (r *fakeVFSMountRepository) List(_ context.Context, filter domainrepo.VFSMountListFilter) ([]*entity.VFSMount, error) {
	items := make([]*entity.VFSMount, 0)
	for _, mount := range r.mounts {
		if filter.SourceID != 0 && mount.SourceID != filter.SourceID {
			continue
		}
		if filter.Enabled != nil && mount.IsEnabled != *filter.Enabled {
			continue
		}
		if filter.Mode != "" && mount.Mode != filter.Mode {
			continue
		}
		if filter.PathPrefix != "" && !isSubPath(filter.PathPrefix, mount.MountPath) {
			continue
		}
		if !filter.IncludeHidden && mount.MountPath == "" {
			continue
		}
		items = append(items, cloneVFSMountForTest(mount))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SortOrder == items[j].SortOrder {
			return items[i].ID < items[j].ID
		}
		return items[i].SortOrder < items[j].SortOrder
	})
	return items, nil
}

func (r *fakeVFSMountRepository) UpsertByMountPath(ctx context.Context, mount *entity.VFSMount) error {
	for _, existing := range r.mounts {
		if existing.MountPath == mount.MountPath {
			mount.ID = existing.ID
			mount.CreatedAt = existing.CreatedAt
			return r.Update(ctx, mount)
		}
	}
	return r.Create(ctx, mount)
}

func (r *fakeVFSMountRepository) ensureMountUnique(mount *entity.VFSMount, excludeID uint) error {
	for _, existing := range r.mounts {
		if existing.ID == excludeID {
			continue
		}
		if existing.MountPath == mount.MountPath || existing.NodeID == mount.NodeID {
			return domainrepo.ErrConflict
		}
	}
	return nil
}

func cloneVFSMountForTest(mount *entity.VFSMount) *entity.VFSMount {
	if mount == nil {
		return nil
	}
	copied := *mount
	return &copied
}
