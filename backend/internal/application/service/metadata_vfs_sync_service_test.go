package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

func TestMetadataVFSSyncRefreshCreatesNodes(t *testing.T) {
	ctx := context.Background()
	env := newMetadataVFSSyncTestEnv(t)
	env.indexer.entries = []RemoteEntry{
		{Name: "Anime", IsDir: true, ProviderItemID: "dir-anime", ProviderParentID: "root"},
		{Name: "episode-01.mkv", IsDir: false, Size: 1024, ETag: "etag-v1", ProviderItemID: "file-1", ProviderParentID: "root", LocatorType: "provider_file_id", LocatorJSON: `{"file_id":"file-1"}`},
	}

	result, err := env.syncSvc.RefreshPath(ctx, "/cloud")
	if err != nil {
		t.Fatalf("RefreshPath() error = %v", err)
	}
	if result.Seen != 2 || result.Indexed != 2 || result.Updated != 0 || result.SyncState != entity.VFSNodeSyncStateIndexed {
		t.Fatalf("unexpected refresh result = %#v", result)
	}

	dir, err := env.nodeRepo.FindByPath(ctx, "/cloud/Anime")
	if err != nil {
		t.Fatalf("FindByPath(dir) error = %v", err)
	}
	if dir.Kind != entity.VFSNodeKindDir || dir.ProviderItemID == nil || *dir.ProviderItemID != "dir-anime" {
		t.Fatalf("unexpected indexed dir = %#v", dir)
	}
	file, err := env.nodeRepo.FindByPath(ctx, "/cloud/episode-01.mkv")
	if err != nil {
		t.Fatalf("FindByPath(file) error = %v", err)
	}
	if file.Kind != entity.VFSNodeKindFile || file.ObjectID == nil || file.Size != 1024 || file.ETag != "etag-v1" {
		t.Fatalf("unexpected indexed file = %#v", file)
	}
}

func TestMetadataVFSSyncSecondRefreshUpdatesMetadataAndObject(t *testing.T) {
	ctx := context.Background()
	env := newMetadataVFSSyncTestEnv(t)
	env.indexer.entries = []RemoteEntry{
		{Name: "episode-01.mkv", IsDir: false, Size: 1024, ETag: "etag-v1", ProviderItemID: "file-1", LocatorType: "provider_file_id", LocatorJSON: `{"file_id":"file-1"}`},
	}
	if _, err := env.syncSvc.RefreshPath(ctx, "/cloud"); err != nil {
		t.Fatalf("first RefreshPath() error = %v", err)
	}
	firstFile, err := env.nodeRepo.FindByPath(ctx, "/cloud/episode-01.mkv")
	if err != nil {
		t.Fatalf("FindByPath(first file) error = %v", err)
	}
	if firstFile.ObjectID == nil {
		t.Fatalf("expected first file to bind object: %#v", firstFile)
	}

	env.indexer.entries = []RemoteEntry{
		{Name: "episode-01.mkv", IsDir: false, Size: 2048, ETag: "etag-v2", ProviderItemID: "file-1", LocatorType: "provider_file_id", LocatorJSON: `{"file_id":"file-1"}`},
	}
	result, err := env.syncSvc.RefreshPath(ctx, "/cloud")
	if err != nil {
		t.Fatalf("second RefreshPath() error = %v", err)
	}
	if result.Indexed != 0 || result.Updated != 1 || result.Missing != 0 {
		t.Fatalf("unexpected second refresh result = %#v", result)
	}

	file, err := env.nodeRepo.FindByPath(ctx, "/cloud/episode-01.mkv")
	if err != nil {
		t.Fatalf("FindByPath(updated file) error = %v", err)
	}
	if file.ObjectID == nil || *file.ObjectID != *firstFile.ObjectID || file.Size != 2048 || file.ETag != "etag-v2" {
		t.Fatalf("file metadata/object binding not updated in place: before=%#v after=%#v", firstFile, file)
	}
	object, err := env.objectRepo.FindByID(ctx, *file.ObjectID)
	if err != nil {
		t.Fatalf("FindByID(object) error = %v", err)
	}
	if object.Size != 2048 || object.ETag != "etag-v2" || object.Status != entity.StorageObjectStatusAvailable {
		t.Fatalf("object was not updated = %#v", object)
	}
}

func TestMetadataVFSSyncRemoteDeleteMarksMissing(t *testing.T) {
	ctx := context.Background()
	env := newMetadataVFSSyncTestEnv(t)
	env.indexer.entries = []RemoteEntry{
		{Name: "episode-01.mkv", IsDir: false, Size: 1024, ETag: "etag-v1", ProviderItemID: "file-1", LocatorType: "provider_file_id", LocatorJSON: `{"file_id":"file-1"}`},
	}
	if _, err := env.syncSvc.RefreshPath(ctx, "/cloud"); err != nil {
		t.Fatalf("first RefreshPath() error = %v", err)
	}

	env.indexer.entries = []RemoteEntry{}
	result, err := env.syncSvc.RefreshPath(ctx, "/cloud")
	if err != nil {
		t.Fatalf("second RefreshPath(empty) error = %v", err)
	}
	if result.Missing != 1 || result.Indexed != 0 {
		t.Fatalf("unexpected missing result = %#v", result)
	}
	file, err := env.nodeRepo.FindByPath(ctx, "/cloud/episode-01.mkv")
	if err != nil {
		t.Fatalf("missing node should remain queryable: %v", err)
	}
	if file.SyncState != entity.VFSNodeSyncStateMissing {
		t.Fatalf("file sync state = %q, want missing", file.SyncState)
	}
	if file.ObjectID == nil {
		t.Fatalf("missing file lost object binding: %#v", file)
	}
	object, err := env.objectRepo.FindByID(ctx, *file.ObjectID)
	if err != nil {
		t.Fatalf("FindByID(object) error = %v", err)
	}
	if object.Status != entity.StorageObjectStatusMissing {
		t.Fatalf("object status = %q, want missing", object.Status)
	}
}

func TestMetadataVFSSyncPreservesControlChildren(t *testing.T) {
	ctx := context.Background()
	env := newMetadataVFSSyncTestEnv(t)
	now := fixedMetadataVFSTime()
	nestedSourceID := uint(99)
	mountID := uint(100)
	control := mustCreateMetadataVFSNode(t, env.nodeRepo, &entity.VFSNode{
		ParentID:  &env.mountNode.ID,
		Name:      "nested",
		Path:      "/cloud/nested",
		Kind:      entity.VFSNodeKindMount,
		MountID:   &mountID,
		SourceID:  &nestedSourceID,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	env.indexer.entries = []RemoteEntry{
		{Name: "nested", IsDir: true, ProviderItemID: "real-dir-with-same-name"},
		{Name: "readme.txt", IsDir: false, Size: 10, ProviderItemID: "file-readme"},
	}

	result, err := env.syncSvc.RefreshPath(ctx, "/cloud")
	if err != nil {
		t.Fatalf("RefreshPath() error = %v", err)
	}
	if result.Indexed != 1 || result.Missing != 0 {
		t.Fatalf("unexpected refresh result = %#v", result)
	}
	stored, err := env.nodeRepo.FindByID(ctx, control.ID)
	if err != nil {
		t.Fatalf("FindByID(control) error = %v", err)
	}
	if stored.Kind != entity.VFSNodeKindMount || stored.SyncState != entity.VFSNodeSyncStateIndexed || stored.SourceID == nil || *stored.SourceID != nestedSourceID {
		t.Fatalf("control mount child should not be overwritten or marked missing: %#v", stored)
	}
}

func TestMetadataVFSSyncDriverFailureKeepsExistingChildren(t *testing.T) {
	ctx := context.Background()
	env := newMetadataVFSSyncTestEnv(t)
	env.indexer.entries = []RemoteEntry{
		{Name: "episode-01.mkv", IsDir: false, Size: 1024, ETag: "etag-v1", ProviderItemID: "file-1", LocatorType: "provider_file_id", LocatorJSON: `{"file_id":"file-1"}`},
	}
	if _, err := env.syncSvc.RefreshPath(ctx, "/cloud"); err != nil {
		t.Fatalf("first RefreshPath() error = %v", err)
	}

	env.indexer.err = errors.New("provider is down")
	result, err := env.syncSvc.RefreshPath(ctx, "/cloud")
	if !errors.Is(err, ErrCloudProviderUnavailable) {
		t.Fatalf("RefreshPath(provider failure) error = %v, want ErrCloudProviderUnavailable", err)
	}
	if result.Errors != 1 || result.Missing != 0 || result.SyncState != entity.VFSNodeSyncStateError {
		t.Fatalf("unexpected failure result = %#v", result)
	}
	file, err := env.nodeRepo.FindByPath(ctx, "/cloud/episode-01.mkv")
	if err != nil {
		t.Fatalf("existing child should remain after provider failure: %v", err)
	}
	if file.SyncState != entity.VFSNodeSyncStateIndexed {
		t.Fatalf("existing child sync state = %q, want indexed", file.SyncState)
	}
	mount, err := env.nodeRepo.FindByPath(ctx, "/cloud")
	if err != nil {
		t.Fatalf("FindByPath(mount) error = %v", err)
	}
	if mount.SyncState != entity.VFSNodeSyncStateError {
		t.Fatalf("target sync state = %q, want error", mount.SyncState)
	}
}

func TestMetadataVFSSyncDuplicateNamesReturnConflict(t *testing.T) {
	ctx := context.Background()
	env := newMetadataVFSSyncTestEnv(t)
	env.indexer.entries = []RemoteEntry{
		{Name: "duplicate.mkv", IsDir: false, Size: 100, ProviderItemID: "file-a", LocatorType: "provider_file_id", LocatorJSON: `{"file_id":"file-a"}`},
		{Name: "duplicate.mkv", IsDir: false, Size: 200, ProviderItemID: "file-b", LocatorType: "provider_file_id", LocatorJSON: `{"file_id":"file-b"}`},
	}

	result, err := env.syncSvc.RefreshPath(ctx, "/cloud")
	if !errors.Is(err, ErrVFSSyncConflict) {
		t.Fatalf("RefreshPath(duplicate) error = %v, want ErrVFSSyncConflict", err)
	}
	if result.Conflicts != 1 || result.SyncState != entity.VFSNodeSyncStateConflict || result.Error != ErrVFSSyncConflict.Error() {
		t.Fatalf("unexpected conflict result = %#v", result)
	}
	child, err := env.nodeRepo.FindByPath(ctx, "/cloud/duplicate.mkv")
	if err != nil {
		t.Fatalf("conflict child should be recorded: %v", err)
	}
	if child.SyncState != entity.VFSNodeSyncStateConflict {
		t.Fatalf("child sync state = %q, want conflict", child.SyncState)
	}
}

func TestMetadataVFSSyncFileObjectBinding(t *testing.T) {
	ctx := context.Background()
	env := newMetadataVFSSyncTestEnv(t)
	env.indexer.entries = []RemoteEntry{
		{Name: "movie.mp4", IsDir: false, Size: 4096, ETag: "etag-movie", Checksum: "sha256:movie", MimeType: "video/mp4", ProviderItemID: "file-movie", ProviderParentID: "root", LocatorType: "provider_file_id", LocatorJSON: `{"file_id":"file-movie","parent_id":"root"}`},
	}

	if _, err := env.syncSvc.RefreshPath(ctx, "/cloud"); err != nil {
		t.Fatalf("RefreshPath() error = %v", err)
	}
	file, err := env.nodeRepo.FindByPath(ctx, "/cloud/movie.mp4")
	if err != nil {
		t.Fatalf("FindByPath(file) error = %v", err)
	}
	if file.ObjectID == nil {
		t.Fatalf("file node did not bind storage object: %#v", file)
	}
	object, err := env.objectRepo.FindByID(ctx, *file.ObjectID)
	if err != nil {
		t.Fatalf("FindByID(object) error = %v", err)
	}
	if object.SourceID != env.source.ID || object.DriverType != "fakecloud" || object.LocatorType != "provider_file_id" || object.LocatorJSON != `{"file_id":"file-movie","parent_id":"root"}` {
		t.Fatalf("unexpected object locator = %#v", object)
	}
	if object.Size != file.Size || object.ETag != file.ETag || object.Checksum != file.Checksum || object.MimeType != "video/mp4" {
		t.Fatalf("object metadata does not match node: node=%#v object=%#v", file, object)
	}
}

func TestMetadataVFSSyncFileDriverBridgeIndexesStorageEntries(t *testing.T) {
	ctx := context.Background()
	env := newMetadataVFSSyncTestEnv(t)
	env.source.DriverType = "legacy"
	env.sourceRepo.sources[env.source.ID] = cloneStorageSourceForSyncTest(env.source)

	driver := &storageFileDriverStub{
		entriesByPath: map[string][]StorageEntry{
			"/": {
				{Name: "bridge.txt", Path: "/bridge.txt", IsDir: false, Size: 12, ETag: "etag-bridge"},
			},
		},
	}
	env.syncSvc = NewMetadataVFSSyncService(
		env.nodeRepo,
		env.objectRepo,
		env.sourceRepo,
		WithMetadataVFSSyncFileDriver("legacy", driver),
		WithMetadataVFSSyncClock(func() time.Time { return fixedMetadataVFSTime().Add(time.Minute) }),
	)

	result, err := env.syncSvc.RefreshPath(ctx, "/cloud")
	if err != nil {
		t.Fatalf("RefreshPath(file driver bridge) error = %v", err)
	}
	if driver.listCalls != 1 || result.Indexed != 1 || result.SyncState != entity.VFSNodeSyncStateIndexed {
		t.Fatalf("unexpected bridge result/list calls: calls=%d result=%#v", driver.listCalls, result)
	}
	file, err := env.nodeRepo.FindByPath(ctx, "/cloud/bridge.txt")
	if err != nil {
		t.Fatalf("FindByPath(bridge file) error = %v", err)
	}
	if file.ObjectID == nil || file.Size != 12 || file.ETag != "etag-bridge" {
		t.Fatalf("bridge did not create file/object binding: %#v", file)
	}
	object, err := env.objectRepo.FindByID(ctx, *file.ObjectID)
	if err != nil {
		t.Fatalf("FindByID(bridge object) error = %v", err)
	}
	if object.LocatorType != "provider_path" || object.LocatorJSON != `{"path":"/bridge.txt"}` {
		t.Fatalf("unexpected bridge object locator = %#v", object)
	}
}

func TestMetadataVFSSyncDefaultLocalIndexerIndexesFilesystem(t *testing.T) {
	ctx := context.Background()
	now := fixedMetadataVFSTime()
	nodeRepo := newFakeVFSNodeRepository()
	objectRepo := newFakeStorageObjectRepository()
	basePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(basePath, "visible.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(basePath, ".trash"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.trash) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(basePath, ".trash", "hidden.txt"), []byte("hidden"), 0o644); err != nil {
		t.Fatalf("WriteFile(hidden) error = %v", err)
	}
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	sourceID := uint(31)
	source := &entity.StorageSource{
		ID:         sourceID,
		Name:       "local",
		DriverType: "local",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	sourceRepo := newFakeMetadataVFSSyncSourceRepository(source)
	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSClock(func() time.Time { return now }))
	root, err := metadataSvc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	mountID := uint(32)
	mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "local",
		Path:      "/local",
		Kind:      entity.VFSNodeKindMount,
		MountID:   &mountID,
		SourceID:  &sourceID,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	syncSvc := NewMetadataVFSSyncService(nodeRepo, objectRepo, sourceRepo)

	result, err := syncSvc.RefreshPath(ctx, "/local")
	if err != nil {
		t.Fatalf("RefreshPath(local) error = %v", err)
	}
	if result.Indexed != 1 {
		t.Fatalf("unexpected local refresh result = %#v", result)
	}
	file, err := nodeRepo.FindByPath(ctx, "/local/visible.txt")
	if err != nil {
		t.Fatalf("FindByPath(visible) error = %v", err)
	}
	if file.ObjectID == nil || file.Size != 5 || file.MimeType != "text/plain; charset=utf-8" {
		t.Fatalf("local file metadata mismatch: %#v", file)
	}
	if _, err := nodeRepo.FindByPath(ctx, "/local/.trash"); !errors.Is(err, domainrepo.ErrNotFound) {
		t.Fatalf("hidden .trash should not be indexed, err = %v", err)
	}
}

type metadataVFSSyncTestEnv struct {
	nodeRepo   *fakeVFSNodeRepository
	objectRepo *fakeStorageObjectRepository
	sourceRepo *fakeMetadataVFSSyncSourceRepository
	indexer    *fakeRemoteIndexer
	syncSvc    *MetadataVFSSyncService
	source     *entity.StorageSource
	mountNode  *entity.VFSNode
}

func newMetadataVFSSyncTestEnv(t *testing.T) *metadataVFSSyncTestEnv {
	t.Helper()

	ctx := context.Background()
	now := fixedMetadataVFSTime()
	nodeRepo := newFakeVFSNodeRepository()
	objectRepo := newFakeStorageObjectRepository()
	sourceID := uint(10)
	mountID := uint(20)
	source := &entity.StorageSource{
		ID:         sourceID,
		Name:       "fake cloud",
		DriverType: "fakecloud",
		IsEnabled:  true,
		MountPath:  "/cloud",
		RootPath:   "/",
		ConfigJSON: "{}",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	sourceRepo := newFakeMetadataVFSSyncSourceRepository(source)
	indexer := &fakeRemoteIndexer{}

	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSClock(func() time.Time { return now }))
	root, err := metadataSvc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	mountNode := mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "cloud",
		Path:      "/cloud",
		Kind:      entity.VFSNodeKindMount,
		MountID:   &mountID,
		SourceID:  &sourceID,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})

	syncSvc := NewMetadataVFSSyncService(
		nodeRepo,
		objectRepo,
		sourceRepo,
		WithMetadataVFSSyncIndexer("fakecloud", indexer),
		WithMetadataVFSSyncClock(func() time.Time { return now.Add(time.Minute) }),
	)
	return &metadataVFSSyncTestEnv{
		nodeRepo:   nodeRepo,
		objectRepo: objectRepo,
		sourceRepo: sourceRepo,
		indexer:    indexer,
		syncSvc:    syncSvc,
		source:     source,
		mountNode:  mountNode,
	}
}

type fakeRemoteIndexer struct {
	entries  []RemoteEntry
	err      error
	requests []RemoteListRequest
}

func (i *fakeRemoteIndexer) ListRemoteChildren(_ context.Context, _ *entity.StorageSource, req RemoteListRequest) ([]RemoteEntry, error) {
	i.requests = append(i.requests, req)
	if i.err != nil {
		return nil, i.err
	}
	items := make([]RemoteEntry, len(i.entries))
	copy(items, i.entries)
	return items, nil
}

type fakeMetadataVFSSyncSourceRepository struct {
	sources map[uint]*entity.StorageSource
}

func newFakeMetadataVFSSyncSourceRepository(sources ...*entity.StorageSource) *fakeMetadataVFSSyncSourceRepository {
	repo := &fakeMetadataVFSSyncSourceRepository{sources: make(map[uint]*entity.StorageSource)}
	for _, source := range sources {
		repo.sources[source.ID] = cloneStorageSourceForSyncTest(source)
	}
	return repo
}

func (r *fakeMetadataVFSSyncSourceRepository) Create(_ context.Context, source *entity.StorageSource) error {
	if source.ID == 0 {
		source.ID = uint(len(r.sources) + 1)
	}
	r.sources[source.ID] = cloneStorageSourceForSyncTest(source)
	return nil
}

func (r *fakeMetadataVFSSyncSourceRepository) Update(_ context.Context, source *entity.StorageSource) error {
	if _, exists := r.sources[source.ID]; !exists {
		return domainrepo.ErrNotFound
	}
	r.sources[source.ID] = cloneStorageSourceForSyncTest(source)
	return nil
}

func (r *fakeMetadataVFSSyncSourceRepository) Delete(_ context.Context, id uint) error {
	if _, exists := r.sources[id]; !exists {
		return domainrepo.ErrNotFound
	}
	delete(r.sources, id)
	return nil
}

func (r *fakeMetadataVFSSyncSourceRepository) FindByID(_ context.Context, id uint) (*entity.StorageSource, error) {
	source, exists := r.sources[id]
	if !exists {
		return nil, domainrepo.ErrNotFound
	}
	return cloneStorageSourceForSyncTest(source), nil
}

func (r *fakeMetadataVFSSyncSourceRepository) ListAll(context.Context) ([]*entity.StorageSource, error) {
	return r.list(false), nil
}

func (r *fakeMetadataVFSSyncSourceRepository) ListEnabled(context.Context) ([]*entity.StorageSource, error) {
	return r.list(true), nil
}

func (r *fakeMetadataVFSSyncSourceRepository) FindByName(_ context.Context, name string) (*entity.StorageSource, error) {
	for _, source := range r.sources {
		if source.Name == name {
			return cloneStorageSourceForSyncTest(source), nil
		}
	}
	return nil, domainrepo.ErrNotFound
}

func (r *fakeMetadataVFSSyncSourceRepository) Count(context.Context) (int64, error) {
	return int64(len(r.sources)), nil
}

func (r *fakeMetadataVFSSyncSourceRepository) list(enabledOnly bool) []*entity.StorageSource {
	items := make([]*entity.StorageSource, 0, len(r.sources))
	for _, source := range r.sources {
		if enabledOnly && !source.IsEnabled {
			continue
		}
		items = append(items, cloneStorageSourceForSyncTest(source))
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func cloneStorageSourceForSyncTest(source *entity.StorageSource) *entity.StorageSource {
	if source == nil {
		return nil
	}
	copied := *source
	if source.LastCheckedAt != nil {
		lastCheckedAt := *source.LastCheckedAt
		copied.LastCheckedAt = &lastCheckedAt
	}
	return &copied
}

type fakeStorageObjectRepository struct {
	nextID  uint
	objects map[uint]*entity.StorageObject
}

func newFakeStorageObjectRepository() *fakeStorageObjectRepository {
	return &fakeStorageObjectRepository{
		nextID:  1,
		objects: make(map[uint]*entity.StorageObject),
	}
}

func (r *fakeStorageObjectRepository) Create(_ context.Context, object *entity.StorageObject) error {
	if object.ID == 0 {
		object.ID = r.nextID
		r.nextID++
	}
	r.objects[object.ID] = cloneStorageObjectForSyncTest(object)
	*object = *cloneStorageObjectForSyncTest(r.objects[object.ID])
	return nil
}

func (r *fakeStorageObjectRepository) Update(_ context.Context, object *entity.StorageObject) error {
	if _, exists := r.objects[object.ID]; !exists {
		return domainrepo.ErrNotFound
	}
	r.objects[object.ID] = cloneStorageObjectForSyncTest(object)
	*object = *cloneStorageObjectForSyncTest(r.objects[object.ID])
	return nil
}

func (r *fakeStorageObjectRepository) Delete(_ context.Context, id uint) error {
	if _, exists := r.objects[id]; !exists {
		return domainrepo.ErrNotFound
	}
	delete(r.objects, id)
	return nil
}

func (r *fakeStorageObjectRepository) FindByID(_ context.Context, id uint) (*entity.StorageObject, error) {
	object, exists := r.objects[id]
	if !exists {
		return nil, domainrepo.ErrNotFound
	}
	return cloneStorageObjectForSyncTest(object), nil
}

func (r *fakeStorageObjectRepository) FindByLocator(_ context.Context, sourceID uint, driverType string, locatorType string, locatorJSON string) (*entity.StorageObject, error) {
	for _, object := range r.objects {
		if object.SourceID == sourceID && object.DriverType == driverType && object.LocatorType == locatorType && object.LocatorJSON == locatorJSON {
			return cloneStorageObjectForSyncTest(object), nil
		}
	}
	return nil, domainrepo.ErrNotFound
}

func (r *fakeStorageObjectRepository) List(_ context.Context, filter domainrepo.StorageObjectListFilter) ([]*entity.StorageObject, error) {
	items := make([]*entity.StorageObject, 0, len(r.objects))
	for _, object := range r.objects {
		if filter.SourceID != 0 && object.SourceID != filter.SourceID {
			continue
		}
		if filter.DriverType != "" && object.DriverType != filter.DriverType {
			continue
		}
		if filter.LocatorType != "" && object.LocatorType != filter.LocatorType {
			continue
		}
		if filter.Status != "" && object.Status != filter.Status {
			continue
		}
		items = append(items, cloneStorageObjectForSyncTest(object))
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

func (r *fakeStorageObjectRepository) UpsertByLocator(ctx context.Context, object *entity.StorageObject) error {
	existing, err := r.FindByLocator(ctx, object.SourceID, object.DriverType, object.LocatorType, object.LocatorJSON)
	if err == nil {
		object.ID = existing.ID
		object.CreatedAt = existing.CreatedAt
		return r.Update(ctx, object)
	}
	if !errors.Is(err, domainrepo.ErrNotFound) {
		return err
	}
	return r.Create(ctx, object)
}

func cloneStorageObjectForSyncTest(object *entity.StorageObject) *entity.StorageObject {
	if object == nil {
		return nil
	}
	copied := *object
	return &copied
}
