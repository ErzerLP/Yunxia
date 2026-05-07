package service

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"testing"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

func TestShareOpenUsesCurrentVFSNodePathAfterRename(t *testing.T) {
	env := newShareNodeFirstTestEnv(t)
	share := env.createFileShare(t, "/docs/hello.txt")

	if _, _, err := env.metadataSvc.Rename(env.actorCtx, MetadataVFSRenameRequest{
		Path:    "/local/docs/hello.txt",
		NewName: "renamed.txt",
	}); err != nil {
		t.Fatalf("metadata rename error = %v", err)
	}

	result, err := env.shareSvc.Open(context.Background(), share.Link[len("/s/"):], "", "", "inline", "name", "asc", 0, 0)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	assertShareRedirectPath(t, result.RedirectURL, "/local/docs/renamed.txt")
	if values, _ := url.Parse(result.RedirectURL); values.Query().Get("source_id") != "" {
		t.Fatalf("share redirect must use v2 virtual path token URL, got %s", result.RedirectURL)
	}
}

func TestShareOpenUsesCurrentVFSNodePathAfterMove(t *testing.T) {
	env := newShareNodeFirstTestEnv(t)
	share := env.createFileShare(t, "/docs/hello.txt")
	env.mustCreateNode(t, &entity.VFSNode{
		ParentID:  &env.mount.ID,
		Name:      "archive",
		Path:      "/local/archive",
		Kind:      entity.VFSNodeKindDir,
		SourceID:  &env.source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})

	if _, _, err := env.metadataSvc.Move(env.actorCtx, MetadataVFSMoveRequest{
		Path:             "/local/docs/hello.txt",
		TargetParentPath: "/local/archive",
	}); err != nil {
		t.Fatalf("metadata move error = %v", err)
	}

	result, err := env.shareSvc.Open(context.Background(), share.Link[len("/s/"):], "", "", "attachment", "name", "asc", 0, 0)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	assertShareRedirectPath(t, result.RedirectURL, "/local/archive/hello.txt")
}

func TestShareOpenDeletedVFSNodeReturnsFileNotFound(t *testing.T) {
	env := newShareNodeFirstTestEnv(t)
	share := env.createFileShare(t, "/docs/hello.txt")

	if _, err := env.metadataSvc.Delete(env.actorCtx, "/local/docs/hello.txt"); err != nil {
		t.Fatalf("metadata delete error = %v", err)
	}

	_, err := env.shareSvc.Open(context.Background(), share.Link[len("/s/"):], "", "", "attachment", "name", "asc", 0, 0)
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("Open() error = %v, want ErrFileNotFound", err)
	}
}

func TestShareOpenMissingVFSNodeReturnsFileNotFound(t *testing.T) {
	env := newShareNodeFirstTestEnv(t)
	share := env.createFileShare(t, "/docs/hello.txt")
	hello := *env.hello
	hello.SyncState = entity.VFSNodeSyncStateMissing
	if err := env.metadataSvc.nodeRepo.Update(context.Background(), &hello); err != nil {
		t.Fatalf("metadata update missing node error = %v", err)
	}

	_, err := env.shareSvc.Open(context.Background(), share.Link[len("/s/"):], "", "", "attachment", "name", "asc", 0, 0)
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("Open() error = %v, want ErrFileNotFound", err)
	}
}

func TestShareOpenMissingTargetSourceReturnsFileNotFound(t *testing.T) {
	env := newShareNodeFirstTestEnv(t)
	share := env.createFileShare(t, "/docs/hello.txt")
	delete(env.sourceRepo.sources, env.source.ID)

	_, err := env.shareSvc.Open(context.Background(), share.Link[len("/s/"):], "", "", "attachment", "name", "asc", 0, 0)
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("Open() error = %v, want ErrFileNotFound", err)
	}
}

func TestShareDirectoryOpenListsMetadataEntriesWithoutProviderDetails(t *testing.T) {
	env := newShareNodeFirstTestEnv(t)
	share := env.createDirectoryShare(t, "/docs")
	env.mustCreateNode(t, &entity.VFSNode{
		ParentID:         &env.docs.ID,
		Name:             "cloud-video.mp4",
		Path:             "/local/docs/cloud-video.mp4",
		Kind:             entity.VFSNodeKindFile,
		SourceID:         &env.source.ID,
		ProviderItemID:   stringPtrForShareTest("provider-file-1"),
		ProviderParentID: stringPtrForShareTest("provider-dir-1"),
		Size:             1024,
		MimeType:         "video/mp4",
		SyncState:        entity.VFSNodeSyncStateIndexed,
	})
	env.mustCreateNode(t, &entity.VFSNode{
		ParentID:  &env.docs.ID,
		Name:      "ghost.txt",
		Path:      "/local/docs/ghost.txt",
		Kind:      entity.VFSNodeKindFile,
		SourceID:  &env.source.ID,
		SyncState: entity.VFSNodeSyncStateMissing,
	})

	result, err := env.shareSvc.Open(context.Background(), share.Link[len("/s/"):], "", "", "inline", "name", "asc", 1, 20)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if result.Data == nil {
		t.Fatalf("expected directory data, got redirect %q", result.RedirectURL)
	}
	if result.Data.Share.TargetVirtualPath != "/local/docs" || result.Data.Share.ResolvedInnerPath != "/docs" {
		t.Fatalf("expected current node-first share target in response, got %+v", result.Data.Share)
	}
	if len(result.Data.Items) != 2 {
		t.Fatalf("expected metadata entries only, got %+v", result.Data.Items)
	}
	names := []string{result.Data.Items[0].Name, result.Data.Items[1].Name}
	sort.Strings(names)
	if names[0] != "cloud-video.mp4" || names[1] != "hello.txt" {
		t.Fatalf("unexpected public item names %v", names)
	}
	for _, item := range result.Data.Items {
		if item.Path == "/local/docs/cloud-video.mp4" || item.ParentPath == "/local/docs" {
			t.Fatalf("public share entry leaked absolute VFS path: %+v", item)
		}
	}
}

type shareNodeFirstTestEnv struct {
	actorCtx    context.Context
	source      entity.StorageSource
	mount       *entity.VFSNode
	docs        *entity.VFSNode
	hello       *entity.VFSNode
	sourceRepo  *shareNodeFirstSourceRepo
	shareSvc    *ShareService
	metadataSvc *MetadataVFSService
}

func newShareNodeFirstTestEnv(t *testing.T) *shareNodeFirstTestEnv {
	t.Helper()

	root := t.TempDir()
	configJSON, err := marshalLocalSourceConfig(root)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := entity.StorageSource{
		ID:         1,
		Name:       "local",
		DriverType: "local",
		IsEnabled:  true,
		MountPath:  "/local",
		RootPath:   "/",
		ConfigJSON: configJSON,
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll(docs) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile(hello.txt) error = %v", err)
	}

	nodeRepo := newFakeVFSNodeRepository()
	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSClock(fixedMetadataVFSTime))
	rootNode, err := metadataSvc.EnsureRoot(context.Background())
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	mount := mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &rootNode.ID,
		Name:      "local",
		Path:      "/local",
		Kind:      entity.VFSNodeKindMount,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})
	docs := mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &mount.ID,
		Name:      "docs",
		Path:      "/local/docs",
		Kind:      entity.VFSNodeKindDir,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
	})
	hello := mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &docs.ID,
		Name:      "hello.txt",
		Path:      "/local/docs/hello.txt",
		Kind:      entity.VFSNodeKindFile,
		SourceID:  &source.ID,
		Size:      5,
		MimeType:  "text/plain",
		SyncState: entity.VFSNodeSyncStateIndexed,
	})

	sourceRepo := &shareNodeFirstSourceRepo{sources: map[uint]*entity.StorageSource{source.ID: &source}}
	shareRepo := newShareNodeFirstRepo()
	shareSvc := NewShareService(
		shareRepo,
		sourceRepo,
		security.NewBcryptHasher(4),
		security.NewFileAccessTokenService("share-node-first-test"),
		WithShareMetadataVFS(metadataSvc, nil),
	)
	actorCtx := security.WithRequestAuth(context.Background(), security.RequestAuth{
		UserID:       1,
		Username:     "admin",
		RoleKey:      permission.RoleSuperAdmin,
		Status:       permission.StatusActive,
		Capabilities: permission.AllCapabilities(),
	})

	return &shareNodeFirstTestEnv{
		actorCtx:    actorCtx,
		source:      source,
		mount:       mount,
		docs:        docs,
		hello:       hello,
		sourceRepo:  sourceRepo,
		shareSvc:    shareSvc,
		metadataSvc: metadataSvc,
	}
}

func (e *shareNodeFirstTestEnv) createFileShare(t *testing.T, innerPath string) *appdto.ShareView {
	t.Helper()
	return e.createShare(t, innerPath)
}

func (e *shareNodeFirstTestEnv) createDirectoryShare(t *testing.T, innerPath string) *appdto.ShareView {
	t.Helper()
	return e.createShare(t, innerPath)
}

func (e *shareNodeFirstTestEnv) createShare(t *testing.T, innerPath string) *appdto.ShareView {
	t.Helper()
	view, err := e.shareSvc.Create(e.actorCtx, appdto.CreateShareRequest{
		SourceID:  e.source.ID,
		Path:      innerPath,
		ExpiresIn: 300,
	})
	if err != nil {
		t.Fatalf("Create(%s) error = %v", innerPath, err)
	}
	if view.TargetVFSNodeID == 0 {
		t.Fatalf("expected target_vfs_node_id in share view: %+v", view)
	}
	return view
}

func (e *shareNodeFirstTestEnv) mustCreateNode(t *testing.T, node *entity.VFSNode) *entity.VFSNode {
	t.Helper()
	if node.CreatedAt.IsZero() {
		now := fixedMetadataVFSTime()
		node.CreatedAt = now
		node.UpdatedAt = now
	}
	if err := e.metadataSvc.nodeRepo.Create(context.Background(), node); err != nil {
		t.Fatalf("Create node %s error = %v", node.Path, err)
	}
	return node
}

func assertShareRedirectPath(t *testing.T, rawURL string, expectedPath string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", rawURL, err)
	}
	if parsed.Path != "/api/v2/fs/download" {
		t.Fatalf("expected v2 download redirect, got %s", rawURL)
	}
	if got := parsed.Query().Get("path"); got != expectedPath {
		t.Fatalf("expected redirect path %s, got %s in %s", expectedPath, got, rawURL)
	}
	if parsed.Query().Get("access_token") == "" {
		t.Fatalf("expected share access_token in %s", rawURL)
	}
}

type shareNodeFirstRepo struct {
	nextID uint
	items  map[uint]*entity.ShareLink
	tokens map[string]uint
}

func newShareNodeFirstRepo() *shareNodeFirstRepo {
	return &shareNodeFirstRepo{
		nextID: 1,
		items:  make(map[uint]*entity.ShareLink),
		tokens: make(map[string]uint),
	}
}

func (r *shareNodeFirstRepo) Create(_ context.Context, share *entity.ShareLink) error {
	if share.ID == 0 {
		share.ID = r.nextID
		r.nextID++
	}
	r.items[share.ID] = cloneShareForNodeFirstTest(share)
	r.tokens[share.Token] = share.ID
	return nil
}

func (r *shareNodeFirstRepo) FindByID(_ context.Context, id uint) (*entity.ShareLink, error) {
	share, ok := r.items[id]
	if !ok {
		return nil, domainrepo.ErrNotFound
	}
	return cloneShareForNodeFirstTest(share), nil
}

func (r *shareNodeFirstRepo) FindByToken(_ context.Context, token string) (*entity.ShareLink, error) {
	id, ok := r.tokens[token]
	if !ok {
		return nil, domainrepo.ErrNotFound
	}
	return r.FindByID(context.Background(), id)
}

func (r *shareNodeFirstRepo) ListAll(context.Context) ([]*entity.ShareLink, error) {
	items := make([]*entity.ShareLink, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, cloneShareForNodeFirstTest(item))
	}
	return items, nil
}

func (r *shareNodeFirstRepo) ListByUser(_ context.Context, userID uint) ([]*entity.ShareLink, error) {
	items := make([]*entity.ShareLink, 0)
	for _, item := range r.items {
		if item.UserID == userID {
			items = append(items, cloneShareForNodeFirstTest(item))
		}
	}
	return items, nil
}

func (r *shareNodeFirstRepo) Update(_ context.Context, share *entity.ShareLink) error {
	if _, ok := r.items[share.ID]; !ok {
		return domainrepo.ErrNotFound
	}
	r.items[share.ID] = cloneShareForNodeFirstTest(share)
	r.tokens[share.Token] = share.ID
	return nil
}

func (r *shareNodeFirstRepo) Delete(_ context.Context, id uint) error {
	share, ok := r.items[id]
	if !ok {
		return domainrepo.ErrNotFound
	}
	delete(r.tokens, share.Token)
	delete(r.items, id)
	return nil
}

type shareNodeFirstSourceRepo struct {
	sources map[uint]*entity.StorageSource
}

func (r *shareNodeFirstSourceRepo) Create(_ context.Context, source *entity.StorageSource) error {
	if source.ID == 0 {
		source.ID = uint(len(r.sources) + 1)
	}
	copied := *source
	r.sources[source.ID] = &copied
	return nil
}

func (r *shareNodeFirstSourceRepo) Update(_ context.Context, source *entity.StorageSource) error {
	if _, ok := r.sources[source.ID]; !ok {
		return domainrepo.ErrNotFound
	}
	copied := *source
	r.sources[source.ID] = &copied
	return nil
}

func (r *shareNodeFirstSourceRepo) Delete(_ context.Context, id uint) error {
	if _, ok := r.sources[id]; !ok {
		return domainrepo.ErrNotFound
	}
	delete(r.sources, id)
	return nil
}

func (r *shareNodeFirstSourceRepo) FindByID(_ context.Context, id uint) (*entity.StorageSource, error) {
	source, ok := r.sources[id]
	if !ok {
		return nil, domainrepo.ErrNotFound
	}
	copied := *source
	return &copied, nil
}

func (r *shareNodeFirstSourceRepo) ListAll(context.Context) ([]*entity.StorageSource, error) {
	return r.list(), nil
}

func (r *shareNodeFirstSourceRepo) ListEnabled(context.Context) ([]*entity.StorageSource, error) {
	items := make([]*entity.StorageSource, 0)
	for _, source := range r.sources {
		if source.IsEnabled {
			copied := *source
			items = append(items, &copied)
		}
	}
	return items, nil
}

func (r *shareNodeFirstSourceRepo) FindByName(_ context.Context, name string) (*entity.StorageSource, error) {
	for _, source := range r.sources {
		if source.Name == name {
			copied := *source
			return &copied, nil
		}
	}
	return nil, domainrepo.ErrNotFound
}

func (r *shareNodeFirstSourceRepo) Count(context.Context) (int64, error) {
	return int64(len(r.sources)), nil
}

func (r *shareNodeFirstSourceRepo) list() []*entity.StorageSource {
	items := make([]*entity.StorageSource, 0, len(r.sources))
	for _, source := range r.sources {
		copied := *source
		items = append(items, &copied)
	}
	return items
}

func cloneShareForNodeFirstTest(share *entity.ShareLink) *entity.ShareLink {
	copied := *share
	if share.PasswordHash != nil {
		value := *share.PasswordHash
		copied.PasswordHash = &value
	}
	if share.ExpiresAt != nil {
		value := *share.ExpiresAt
		copied.ExpiresAt = &value
	}
	return &copied
}

func stringPtrForShareTest(value string) *string {
	return &value
}
