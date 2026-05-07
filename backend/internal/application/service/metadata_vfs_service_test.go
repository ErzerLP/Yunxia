package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

func TestMetadataVFSServiceEnsureRootListAndMkdir(t *testing.T) {
	ctx := context.Background()
	repo := newFakeVFSNodeRepository()
	now := fixedMetadataVFSTime()
	svc := NewMetadataVFSService(repo, WithMetadataVFSClock(func() time.Time { return now }))

	root, err := svc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	if root.Path != "/" || root.Kind != entity.VFSNodeKindRoot || root.IsDeleted {
		t.Fatalf("unexpected root = %#v", root)
	}

	virtualDir, err := svc.Mkdir(ctx, MetadataVFSMkdirRequest{ParentPath: "/", Name: "library"})
	if err != nil {
		t.Fatalf("Mkdir(virtual dir) error = %v", err)
	}
	if !virtualDir.IsVirtual || virtualDir.EntryKind != string(VirtualEntryKindDirectory) || virtualDir.IsMountPoint {
		t.Fatalf("unexpected virtual dir item = %#v", virtualDir)
	}

	sourceID := uint(10)
	mountID := uint(20)
	mountNode := &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "pikpak",
		Path:      "/pikpak",
		Kind:      entity.VFSNodeKindMount,
		MountID:   &mountID,
		SourceID:  &sourceID,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.Create(ctx, mountNode); err != nil {
		t.Fatalf("Create(mount node) error = %v", err)
	}

	listed, err := svc.ListChildren(ctx, "/")
	if err != nil {
		t.Fatalf("ListChildren(/) error = %v", err)
	}
	if got, want := collectMetadataVFSNames(listed.Items), []string{"library", "pikpak"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ListChildren(/) names = %#v, want %#v", got, want)
	}
	mountItem := findMetadataVFSItem(t, listed.Items, "pikpak")
	if !mountItem.IsMountPoint || !mountItem.IsVirtual || mountItem.SourceID == nil || *mountItem.SourceID != sourceID || mountItem.CanDelete {
		t.Fatalf("unexpected mount item = %#v", mountItem)
	}

	child, err := svc.Mkdir(ctx, MetadataVFSMkdirRequest{ParentPath: "/pikpak", Name: "anime"})
	if err != nil {
		t.Fatalf("Mkdir(source-backed dir) error = %v", err)
	}
	if child.IsVirtual || child.SourceID == nil || *child.SourceID != sourceID {
		t.Fatalf("unexpected source-backed child item = %#v", child)
	}
	childNode, err := repo.FindByPath(ctx, "/pikpak/anime")
	if err != nil {
		t.Fatalf("FindByPath(child) error = %v", err)
	}
	if childNode.Kind != entity.VFSNodeKindDir || childNode.MountID == nil || *childNode.MountID != mountID {
		t.Fatalf("source-backed child did not inherit metadata context: %#v", childNode)
	}
}

func TestMetadataVFSMkdirConflict(t *testing.T) {
	ctx := context.Background()
	repo := newFakeVFSNodeRepository()
	svc := NewMetadataVFSService(repo, WithMetadataVFSClock(func() time.Time { return fixedMetadataVFSTime() }))
	if _, err := svc.EnsureRoot(ctx); err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	if _, err := svc.Mkdir(ctx, MetadataVFSMkdirRequest{ParentPath: "/", Name: "docs"}); err != nil {
		t.Fatalf("Mkdir(docs) error = %v", err)
	}
	if _, err := svc.Mkdir(ctx, MetadataVFSMkdirRequest{ParentPath: "/", Name: "docs"}); !errors.Is(err, ErrNameConflict) {
		t.Fatalf("Mkdir(conflict) error = %v, want ErrNameConflict", err)
	}
	if _, err := svc.Mkdir(ctx, MetadataVFSMkdirRequest{ParentPath: "/", Name: "bad/name"}); !errors.Is(err, ErrFileNameInvalid) {
		t.Fatalf("Mkdir(invalid name) error = %v, want ErrFileNameInvalid", err)
	}
}

func TestMetadataVFSMkdirReusesSoftDeletedName(t *testing.T) {
	ctx := context.Background()
	repo := newFakeVFSNodeRepository()
	svc := NewMetadataVFSService(repo, WithMetadataVFSClock(func() time.Time { return fixedMetadataVFSTime() }))
	if _, err := svc.EnsureRoot(ctx); err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	first, err := svc.Mkdir(ctx, MetadataVFSMkdirRequest{ParentPath: "/", Name: "docs"})
	if err != nil {
		t.Fatalf("Mkdir(docs) error = %v", err)
	}
	if _, err := svc.Delete(ctx, first.Path); err != nil {
		t.Fatalf("Delete(docs) error = %v", err)
	}

	recreated, err := svc.Mkdir(ctx, MetadataVFSMkdirRequest{ParentPath: "/", Name: "docs"})
	if err != nil {
		t.Fatalf("Mkdir(reuse soft-deleted name) error = %v", err)
	}
	if recreated.Path != "/docs" {
		t.Fatalf("recreated path = %q, want /docs", recreated.Path)
	}
	active, err := svc.ListChildren(ctx, "/")
	if err != nil {
		t.Fatalf("ListChildren(/) error = %v", err)
	}
	if got := collectMetadataVFSNames(active.Items); strings.Join(got, ",") != "docs" {
		t.Fatalf("active names = %#v, want only recreated docs", got)
	}
}

func TestMetadataVFSSearchByNameAndPathPrefix(t *testing.T) {
	ctx := context.Background()
	repo := newFakeVFSNodeRepository()
	now := fixedMetadataVFSTime()
	svc := NewMetadataVFSService(repo, WithMetadataVFSClock(func() time.Time { return now }))
	root, err := svc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	anime := mustCreateMetadataVFSNode(t, repo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "anime",
		Path:      "/anime",
		Kind:      entity.VFSNodeKindVirtualDir,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreateMetadataVFSNode(t, repo, &entity.VFSNode{
		ParentID:  &anime.ID,
		Name:      "episode-01.mkv",
		Path:      "/anime/episode-01.mkv",
		Kind:      entity.VFSNodeKindFile,
		MimeType:  "video/mp4",
		Size:      1024,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	mustCreateMetadataVFSNode(t, repo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "docs",
		Path:      "/docs",
		Kind:      entity.VFSNodeKindVirtualDir,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})

	resp, err := svc.Search(ctx, "/anime", "episode")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if resp.PathPrefix != "/anime" || resp.Keyword != "episode" || len(resp.Items) != 1 {
		t.Fatalf("unexpected search response = %#v", resp)
	}
	item := resp.Items[0]
	if item.Path != "/anime/episode-01.mkv" || item.EntryKind != string(VirtualEntryKindFile) || !item.CanPreview || !item.CanDownload {
		t.Fatalf("unexpected search item = %#v", item)
	}

	resp, err = svc.Search(ctx, "/anime", "docs")
	if err != nil {
		t.Fatalf("Search(no match) error = %v", err)
	}
	if len(resp.Items) != 0 {
		t.Fatalf("Search(/anime, docs) returned %#v, want empty", resp.Items)
	}
}

func TestMetadataVFSRenameUpdatesSubtreePaths(t *testing.T) {
	ctx := context.Background()
	repo := newFakeVFSNodeRepository()
	now := fixedMetadataVFSTime()
	svc := NewMetadataVFSService(repo, WithMetadataVFSClock(func() time.Time { return now }))
	root, anime, season, _ := seedMetadataVFSAnimeTree(t, ctx, repo, svc, now)

	oldPath, renamed, err := svc.Rename(ctx, MetadataVFSRenameRequest{Path: anime.Path, NewName: "cartoons"})
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if oldPath != "/anime" || renamed.Path != "/cartoons" || renamed.Name != "cartoons" {
		t.Fatalf("unexpected rename result old=%q item=%#v", oldPath, renamed)
	}
	if _, err := svc.ResolveNode(ctx, "/anime"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("ResolveNode(old path) error = %v, want ErrFileNotFound", err)
	}
	if got, err := svc.ResolveNode(ctx, "/cartoons/season-1/episode-01.mkv"); err != nil || got.ParentID == nil || *got.ParentID != season.ID {
		t.Fatalf("ResolveNode(renamed descendant) = %#v, %v", got, err)
	}
	if _, err := repo.FindByID(ctx, root.ID); err != nil {
		t.Fatalf("root should still be resolvable by ID: %v", err)
	}
}

func TestMetadataVFSRenameSyncsPathBasedObjectLocator(t *testing.T) {
	ctx := context.Background()
	nodeRepo := newFakeVFSNodeRepository()
	objectRepo := newFakeStorageObjectRepository()
	sourceID := uint(7)
	source := &entity.StorageSource{ID: sourceID, Name: "Docs", DriverType: "local", MountPath: "/docs", RootPath: "/", IsEnabled: true}
	sourceRepo := newFakeMetadataVFSSyncSourceRepository(source)
	now := fixedMetadataVFSTime()
	svc := NewMetadataVFSService(
		nodeRepo,
		WithMetadataVFSClock(func() time.Time { return now }),
		WithMetadataVFSObjectLocatorSync(sourceRepo, objectRepo),
	)
	root, err := svc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	mount := mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "docs",
		Path:      "/docs",
		Kind:      entity.VFSNodeKindMount,
		SourceID:  &sourceID,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	object := &entity.StorageObject{
		SourceID:    sourceID,
		DriverType:  "local",
		LocatorType: "local_path",
		LocatorJSON: `{"path":"/old.txt"}`,
		Status:      entity.StorageObjectStatusAvailable,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := objectRepo.Create(ctx, object); err != nil {
		t.Fatalf("Create(object) error = %v", err)
	}
	mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:   &mount.ID,
		Name:       "old.txt",
		Path:       "/docs/old.txt",
		Kind:       entity.VFSNodeKindFile,
		ObjectID:   &object.ID,
		SourceID:   &sourceID,
		SyncState:  entity.VFSNodeSyncStateIndexed,
		CreatedAt:  now,
		UpdatedAt:  now,
		IndexedAt:  &now,
		LastSeenAt: &now,
	})

	if _, _, err := svc.Rename(ctx, MetadataVFSRenameRequest{Path: "/docs/old.txt", NewName: "new.txt"}); err != nil {
		t.Fatalf("Rename(file) error = %v", err)
	}
	updatedObject, err := objectRepo.FindByID(ctx, object.ID)
	if err != nil {
		t.Fatalf("FindByID(object) error = %v", err)
	}
	if updatedObject.LocatorJSON != `{"path":"/new.txt"}` {
		t.Fatalf("object locator = %s, want /new.txt", updatedObject.LocatorJSON)
	}
}

func TestMetadataVFSRenameRejectsRootMountAndConflicts(t *testing.T) {
	ctx := context.Background()
	repo := newFakeVFSNodeRepository()
	now := fixedMetadataVFSTime()
	svc := NewMetadataVFSService(repo, WithMetadataVFSClock(func() time.Time { return now }))
	root, anime, _, _ := seedMetadataVFSAnimeTree(t, ctx, repo, svc, now)

	if _, _, err := svc.Rename(ctx, MetadataVFSRenameRequest{Path: "/", NewName: "root"}); !errors.Is(err, ErrPathInvalid) {
		t.Fatalf("Rename(root) error = %v, want ErrPathInvalid", err)
	}

	mount := mustCreateMetadataVFSNode(t, repo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "pikpak",
		Path:      "/pikpak",
		Kind:      entity.VFSNodeKindMount,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if _, _, err := svc.Rename(ctx, MetadataVFSRenameRequest{Path: mount.Path, NewName: "cloud"}); !errors.Is(err, ErrSourceOperationUnsupported) {
		t.Fatalf("Rename(mount) error = %v, want ErrSourceOperationUnsupported", err)
	}

	cartoons := mustCreateMetadataVFSNode(t, repo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "cartoons",
		Path:      "/cartoons",
		Kind:      entity.VFSNodeKindVirtualDir,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if _, _, err := svc.Rename(ctx, MetadataVFSRenameRequest{Path: anime.Path, NewName: "cartoons"}); !errors.Is(err, ErrNameConflict) {
		t.Fatalf("Rename(active conflict) error = %v, want ErrNameConflict", err)
	}
	if _, err := svc.Delete(ctx, cartoons.Path); err != nil {
		t.Fatalf("Delete(conflicting soft target) error = %v", err)
	}
	if _, renamed, err := svc.Rename(ctx, MetadataVFSRenameRequest{Path: anime.Path, NewName: "cartoons"}); err != nil || renamed.Path != "/cartoons" {
		t.Fatalf("Rename(reuse soft-deleted target) item=%#v error=%v, want /cartoons nil", renamed, err)
	}
}

func TestMetadataVFSMoveUpdatesSubtreePaths(t *testing.T) {
	ctx := context.Background()
	repo := newFakeVFSNodeRepository()
	now := fixedMetadataVFSTime()
	svc := NewMetadataVFSService(repo, WithMetadataVFSClock(func() time.Time { return now }))
	_, anime, _, _ := seedMetadataVFSAnimeTree(t, ctx, repo, svc, now)
	if _, err := svc.Mkdir(ctx, MetadataVFSMkdirRequest{ParentPath: "/", Name: "library"}); err != nil {
		t.Fatalf("Mkdir(library) error = %v", err)
	}

	oldPath, moved, err := svc.Move(ctx, MetadataVFSMoveRequest{Path: anime.Path, TargetParentPath: "/library"})
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if oldPath != "/anime" || moved.Path != "/library/anime" {
		t.Fatalf("unexpected move result old=%q item=%#v", oldPath, moved)
	}
	if _, err := svc.ResolveNode(ctx, "/library/anime/season-1/episode-01.mkv"); err != nil {
		t.Fatalf("ResolveNode(moved descendant) error = %v", err)
	}
	if _, _, err := svc.Move(ctx, MetadataVFSMoveRequest{Path: "/library", TargetParentPath: "/library/anime"}); !errors.Is(err, ErrPathInvalid) {
		t.Fatalf("Move(into own subtree) error = %v, want ErrPathInvalid", err)
	}
}

func TestMetadataVFSMovePreflightsSubtreeConflicts(t *testing.T) {
	ctx := context.Background()
	repo := newFakeVFSNodeRepository()
	now := fixedMetadataVFSTime()
	svc := NewMetadataVFSService(repo, WithMetadataVFSClock(func() time.Time { return now }))
	_, anime, _, _ := seedMetadataVFSAnimeTree(t, ctx, repo, svc, now)
	if _, err := svc.Mkdir(ctx, MetadataVFSMkdirRequest{ParentPath: "/", Name: "library"}); err != nil {
		t.Fatalf("Mkdir(library) error = %v", err)
	}
	library, err := svc.ResolveNode(ctx, "/library")
	if err != nil {
		t.Fatalf("ResolveNode(library) error = %v", err)
	}
	mustCreateMetadataVFSNode(t, repo, &entity.VFSNode{
		ParentID:  &library.ID,
		Name:      "season-1",
		Path:      "/library/anime/season-1",
		Kind:      entity.VFSNodeKindVirtualDir,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})

	if _, _, err := svc.Move(ctx, MetadataVFSMoveRequest{Path: anime.Path, TargetParentPath: "/library"}); !errors.Is(err, ErrNameConflict) {
		t.Fatalf("Move(descendant target conflict) error = %v, want ErrNameConflict", err)
	}
	if _, err := svc.ResolveNode(ctx, "/anime/season-1/episode-01.mkv"); err != nil {
		t.Fatalf("source subtree should remain at old path after preflight conflict: %v", err)
	}
	if _, err := svc.ResolveNode(ctx, "/library/anime"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("target root should not be partially created, error = %v, want ErrFileNotFound", err)
	}
}

func TestMetadataVFSDeleteSoftDeletesSubtree(t *testing.T) {
	ctx := context.Background()
	repo := newFakeVFSNodeRepository()
	now := fixedMetadataVFSTime()
	svc := NewMetadataVFSService(repo, WithMetadataVFSClock(func() time.Time { return now }))
	_, anime, _, episode := seedMetadataVFSAnimeTree(t, ctx, repo, svc, now)

	deletedAt, err := svc.Delete(ctx, anime.Path)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !deletedAt.Equal(now) {
		t.Fatalf("Delete() deletedAt = %s, want %s", deletedAt, now)
	}
	if _, err := svc.ResolveNode(ctx, anime.Path); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("ResolveNode(deleted root) error = %v, want ErrFileNotFound", err)
	}
	if _, err := svc.ResolveNode(ctx, episode.Path); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("ResolveNode(deleted child) error = %v, want ErrFileNotFound", err)
	}

	all, err := repo.ListByPathPrefix(ctx, "/", domainrepo.VFSNodeListFilter{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("ListByPathPrefix(include deleted) error = %v", err)
	}
	deletedByPath := map[string]bool{}
	for _, item := range all {
		deletedByPath[item.Path] = item.IsDeleted
	}
	if !deletedByPath["/anime"] || !deletedByPath["/anime/season-1"] || !deletedByPath["/anime/season-1/episode-01.mkv"] {
		t.Fatalf("subtree was not soft-deleted: %#v", deletedByPath)
	}
}

func TestMetadataVFSDeleteRejectsRootAndMount(t *testing.T) {
	ctx := context.Background()
	repo := newFakeVFSNodeRepository()
	now := fixedMetadataVFSTime()
	svc := NewMetadataVFSService(repo, WithMetadataVFSClock(func() time.Time { return now }))
	root, err := svc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	if _, err := svc.Delete(ctx, "/"); !errors.Is(err, ErrPathInvalid) {
		t.Fatalf("Delete(root) error = %v, want ErrPathInvalid", err)
	}
	mount := mustCreateMetadataVFSNode(t, repo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "pikpak",
		Path:      "/pikpak",
		Kind:      entity.VFSNodeKindMount,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if _, err := svc.Delete(ctx, mount.Path); !errors.Is(err, ErrSourceOperationUnsupported) {
		t.Fatalf("Delete(mount) error = %v, want ErrSourceOperationUnsupported", err)
	}
	if got, err := svc.ResolveNode(ctx, mount.Path); err != nil || got.IsDeleted {
		t.Fatalf("mount should remain active after rejected delete: node=%#v err=%v", got, err)
	}
}

func TestMetadataVFSTagAttachAndList(t *testing.T) {
	ctx := context.Background()
	nodeRepo := newFakeVFSNodeRepository()
	tagRepo := newFakeVFSTagRepository()
	now := fixedMetadataVFSTime()
	svc := NewMetadataVFSService(
		nodeRepo,
		WithMetadataVFSTagRepository(tagRepo),
		WithMetadataVFSClock(func() time.Time { return now }),
	)
	if _, err := svc.EnsureRoot(ctx); err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	if _, err := svc.Mkdir(ctx, MetadataVFSMkdirRequest{ParentPath: "/", Name: "anime"}); err != nil {
		t.Fatalf("Mkdir(anime) error = %v", err)
	}
	tag := &entity.VFSTag{
		OwnerUserID: 1,
		Name:        "favorite",
		Color:       "#66ccff",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := tagRepo.Create(ctx, tag); err != nil {
		t.Fatalf("Create(tag) error = %v", err)
	}

	actorID := uint(9)
	if err := svc.AttachTag(ctx, "/anime", tag.ID, &actorID); err != nil {
		t.Fatalf("AttachTag() error = %v", err)
	}
	tags, err := svc.ListTags(ctx, "/anime")
	if err != nil {
		t.Fatalf("ListTags() error = %v", err)
	}
	if len(tags) != 1 || tags[0].ID != tag.ID || tags[0].Name != "favorite" {
		t.Fatalf("unexpected tags = %#v", tags)
	}
}

func TestMetadataVFSItemDownloadAvailabilityBySyncState(t *testing.T) {
	now := fixedMetadataVFSTime()
	for _, tc := range []struct {
		name        string
		syncState   string
		canDownload bool
	}{
		{name: "indexed", syncState: entity.VFSNodeSyncStateIndexed, canDownload: true},
		{name: "stale", syncState: entity.VFSNodeSyncStateStale, canDownload: true},
		{name: "pending", syncState: entity.VFSNodeSyncStatePending, canDownload: false},
		{name: "syncing", syncState: entity.VFSNodeSyncStateSyncing, canDownload: false},
		{name: "conflict", syncState: entity.VFSNodeSyncStateConflict, canDownload: false},
		{name: "missing", syncState: entity.VFSNodeSyncStateMissing, canDownload: false},
		{name: "error", syncState: entity.VFSNodeSyncStateError, canDownload: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := metadataVFSItemFromNode(&entity.VFSNode{
				Name:      "episode-01.mkv",
				Path:      "/episode-01.mkv",
				Kind:      entity.VFSNodeKindFile,
				SyncState: tc.syncState,
				CreatedAt: now,
				UpdatedAt: now,
			})
			if item.CanDownload != tc.canDownload {
				t.Fatalf("CanDownload for %s = %v, want %v", tc.syncState, item.CanDownload, tc.canDownload)
			}
		})
	}
}

func seedMetadataVFSAnimeTree(t *testing.T, ctx context.Context, repo *fakeVFSNodeRepository, svc *MetadataVFSService, now time.Time) (*entity.VFSNode, *entity.VFSNode, *entity.VFSNode, *entity.VFSNode) {
	t.Helper()

	root, err := svc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	anime := mustCreateMetadataVFSNode(t, repo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "anime",
		Path:      "/anime",
		Kind:      entity.VFSNodeKindVirtualDir,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	season := mustCreateMetadataVFSNode(t, repo, &entity.VFSNode{
		ParentID:  &anime.ID,
		Name:      "season-1",
		Path:      "/anime/season-1",
		Kind:      entity.VFSNodeKindVirtualDir,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	episode := mustCreateMetadataVFSNode(t, repo, &entity.VFSNode{
		ParentID:  &season.ID,
		Name:      "episode-01.mkv",
		Path:      "/anime/season-1/episode-01.mkv",
		Kind:      entity.VFSNodeKindFile,
		MimeType:  "video/mp4",
		Size:      2048,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	return root, anime, season, episode
}

func mustCreateMetadataVFSNode(t *testing.T, repo *fakeVFSNodeRepository, node *entity.VFSNode) *entity.VFSNode {
	t.Helper()
	if err := repo.Create(context.Background(), node); err != nil {
		t.Fatalf("Create(%s) error = %v", node.Path, err)
	}
	return node
}

func fixedMetadataVFSTime() time.Time {
	return time.Date(2026, 5, 7, 13, 0, 0, 0, time.UTC)
}

func collectMetadataVFSNames(items []appdto.VFSItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	sort.Strings(names)
	return names
}

func findMetadataVFSItem(t *testing.T, items []appdto.VFSItem, name string) appdto.VFSItem {
	t.Helper()

	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("item %q not found in %#v", name, items)
	return appdto.VFSItem{}
}

type fakeVFSNodeRepository struct {
	nextID uint
	nodes  map[uint]*entity.VFSNode
}

func newFakeVFSNodeRepository() *fakeVFSNodeRepository {
	return &fakeVFSNodeRepository{
		nextID: 1,
		nodes:  make(map[uint]*entity.VFSNode),
	}
}

func (r *fakeVFSNodeRepository) Create(_ context.Context, node *entity.VFSNode) error {
	if err := r.ensureNodeUnique(node, 0); err != nil {
		return err
	}
	if node.ID == 0 {
		node.ID = r.nextID
		r.nextID++
	}
	r.nodes[node.ID] = cloneVFSNodeForTest(node)
	*node = *cloneVFSNodeForTest(r.nodes[node.ID])
	return nil
}

func (r *fakeVFSNodeRepository) Update(_ context.Context, node *entity.VFSNode) error {
	if _, exists := r.nodes[node.ID]; !exists {
		return domainrepo.ErrNotFound
	}
	if err := r.ensureNodeUnique(node, node.ID); err != nil {
		return err
	}
	r.nodes[node.ID] = cloneVFSNodeForTest(node)
	*node = *cloneVFSNodeForTest(r.nodes[node.ID])
	return nil
}

func (r *fakeVFSNodeRepository) Delete(_ context.Context, id uint) error {
	node, exists := r.nodes[id]
	if !exists || node.IsDeleted {
		return domainrepo.ErrNotFound
	}
	deleted := 0
	for _, item := range r.nodes {
		if item.IsDeleted {
			continue
		}
		if item.Path == node.Path || strings.HasPrefix(item.Path, strings.TrimRight(node.Path, "/")+"/") {
			item.IsDeleted = true
			deleted++
		}
	}
	if deleted == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

func (r *fakeVFSNodeRepository) FindByID(_ context.Context, id uint) (*entity.VFSNode, error) {
	node, exists := r.nodes[id]
	if !exists {
		return nil, domainrepo.ErrNotFound
	}
	return cloneVFSNodeForTest(node), nil
}

func (r *fakeVFSNodeRepository) FindByPath(_ context.Context, pathValue string) (*entity.VFSNode, error) {
	for _, node := range r.nodes {
		if !node.IsDeleted && node.Path == pathValue {
			return cloneVFSNodeForTest(node), nil
		}
	}
	return nil, domainrepo.ErrNotFound
}

func (r *fakeVFSNodeRepository) ListChildren(_ context.Context, parentID *uint, filter domainrepo.VFSNodeListFilter) ([]*entity.VFSNode, error) {
	items := make([]*entity.VFSNode, 0)
	for _, node := range r.nodes {
		if !sameUintPtr(node.ParentID, parentID) {
			continue
		}
		if !fakeVFSNodeMatchesFilter(node, filter) {
			continue
		}
		items = append(items, cloneVFSNodeForTest(node))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if strings.ToLower(items[i].Name) == strings.ToLower(items[j].Name) {
			return items[i].ID < items[j].ID
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

func (r *fakeVFSNodeRepository) ListByPathPrefix(_ context.Context, pathPrefix string, filter domainrepo.VFSNodeListFilter) ([]*entity.VFSNode, error) {
	prefix, err := normalizeVirtualPath(pathPrefix)
	if err != nil {
		return nil, err
	}
	items := make([]*entity.VFSNode, 0)
	for _, node := range r.nodes {
		if prefix != "/" && node.Path != prefix && !strings.HasPrefix(node.Path, prefix+"/") {
			continue
		}
		if !fakeVFSNodeMatchesFilter(node, filter) {
			continue
		}
		items = append(items, cloneVFSNodeForTest(node))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Path == items[j].Path {
			return items[i].ID < items[j].ID
		}
		return items[i].Path < items[j].Path
	})
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

func (r *fakeVFSNodeRepository) UpsertByPath(ctx context.Context, node *entity.VFSNode) error {
	for _, existing := range r.nodes {
		if !existing.IsDeleted && existing.Path == node.Path {
			node.ID = existing.ID
			node.CreatedAt = existing.CreatedAt
			return r.Update(ctx, node)
		}
	}
	return r.Create(ctx, node)
}

func (r *fakeVFSNodeRepository) ensureNodeUnique(node *entity.VFSNode, excludeID uint) error {
	if node.IsDeleted {
		return nil
	}
	for _, existing := range r.nodes {
		if existing.ID == excludeID || existing.IsDeleted {
			continue
		}
		if existing.Path == node.Path {
			return domainrepo.ErrConflict
		}
		if sameUintPtr(existing.ParentID, node.ParentID) && existing.Name == node.Name {
			return domainrepo.ErrConflict
		}
	}
	return nil
}

func fakeVFSNodeMatchesFilter(node *entity.VFSNode, filter domainrepo.VFSNodeListFilter) bool {
	if !filter.IncludeDeleted && node.IsDeleted {
		return false
	}
	if filter.Kind != "" && node.Kind != filter.Kind {
		return false
	}
	if filter.MountID != nil && !sameUintPtr(node.MountID, filter.MountID) {
		return false
	}
	if filter.SourceID != nil && !sameUintPtr(node.SourceID, filter.SourceID) {
		return false
	}
	if filter.SyncState != "" && node.SyncState != filter.SyncState {
		return false
	}
	return true
}

func cloneVFSNodeForTest(node *entity.VFSNode) *entity.VFSNode {
	if node == nil {
		return nil
	}
	copied := *node
	copied.ParentID = cloneUintPtr(node.ParentID)
	copied.MountID = cloneUintPtr(node.MountID)
	copied.ObjectID = cloneUintPtr(node.ObjectID)
	copied.SourceID = cloneUintPtr(node.SourceID)
	copied.ProviderItemID = cloneStringPtrForTest(node.ProviderItemID)
	copied.ProviderParentID = cloneStringPtrForTest(node.ProviderParentID)
	copied.CreatedBy = cloneUintPtr(node.CreatedBy)
	copied.UpdatedBy = cloneUintPtr(node.UpdatedBy)
	if node.IndexedAt != nil {
		indexedAt := *node.IndexedAt
		copied.IndexedAt = &indexedAt
	}
	if node.LastSeenAt != nil {
		lastSeenAt := *node.LastSeenAt
		copied.LastSeenAt = &lastSeenAt
	}
	return &copied
}

func cloneStringPtrForTest(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

type fakeVFSTagRepository struct {
	nextID   uint
	tags     map[uint]*entity.VFSTag
	bindings map[uint]map[uint]*entity.VFSNodeTag
}

func newFakeVFSTagRepository() *fakeVFSTagRepository {
	return &fakeVFSTagRepository{
		nextID:   1,
		tags:     make(map[uint]*entity.VFSTag),
		bindings: make(map[uint]map[uint]*entity.VFSNodeTag),
	}
}

func (r *fakeVFSTagRepository) Create(_ context.Context, tag *entity.VFSTag) error {
	for _, existing := range r.tags {
		if existing.OwnerUserID == tag.OwnerUserID && existing.Name == tag.Name {
			return domainrepo.ErrConflict
		}
	}
	if tag.ID == 0 {
		tag.ID = r.nextID
		r.nextID++
	}
	r.tags[tag.ID] = cloneVFSTagForTest(tag)
	*tag = *cloneVFSTagForTest(r.tags[tag.ID])
	return nil
}

func (r *fakeVFSTagRepository) Update(_ context.Context, tag *entity.VFSTag) error {
	if _, exists := r.tags[tag.ID]; !exists {
		return domainrepo.ErrNotFound
	}
	r.tags[tag.ID] = cloneVFSTagForTest(tag)
	return nil
}

func (r *fakeVFSTagRepository) Delete(_ context.Context, id uint) error {
	if _, exists := r.tags[id]; !exists {
		return domainrepo.ErrNotFound
	}
	delete(r.tags, id)
	for nodeID := range r.bindings {
		delete(r.bindings[nodeID], id)
	}
	return nil
}

func (r *fakeVFSTagRepository) FindByID(_ context.Context, id uint) (*entity.VFSTag, error) {
	tag, exists := r.tags[id]
	if !exists {
		return nil, domainrepo.ErrNotFound
	}
	return cloneVFSTagForTest(tag), nil
}

func (r *fakeVFSTagRepository) FindByOwnerAndName(_ context.Context, ownerUserID uint, name string) (*entity.VFSTag, error) {
	for _, tag := range r.tags {
		if tag.OwnerUserID == ownerUserID && tag.Name == name {
			return cloneVFSTagForTest(tag), nil
		}
	}
	return nil, domainrepo.ErrNotFound
}

func (r *fakeVFSTagRepository) List(_ context.Context, filter domainrepo.VFSTagListFilter) ([]*entity.VFSTag, error) {
	items := make([]*entity.VFSTag, 0)
	for _, tag := range r.tags {
		if tag.OwnerUserID != filter.OwnerUserID {
			continue
		}
		if filter.Name != "" && tag.Name != filter.Name {
			continue
		}
		items = append(items, cloneVFSTagForTest(tag))
	}
	sortVFSTagsForTest(items)
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

func (r *fakeVFSTagRepository) UpsertByOwnerAndName(ctx context.Context, tag *entity.VFSTag) error {
	existing, err := r.FindByOwnerAndName(ctx, tag.OwnerUserID, tag.Name)
	if err == nil {
		tag.ID = existing.ID
		return r.Update(ctx, tag)
	}
	if !errors.Is(err, domainrepo.ErrNotFound) {
		return err
	}
	return r.Create(ctx, tag)
}

func (r *fakeVFSTagRepository) AttachToNode(_ context.Context, binding *entity.VFSNodeTag) error {
	if _, exists := r.tags[binding.TagID]; !exists {
		return domainrepo.ErrNotFound
	}
	if r.bindings[binding.NodeID] == nil {
		r.bindings[binding.NodeID] = make(map[uint]*entity.VFSNodeTag)
	}
	if _, exists := r.bindings[binding.NodeID][binding.TagID]; exists {
		return nil
	}
	copied := *binding
	copied.CreatedBy = cloneUintPtr(binding.CreatedBy)
	r.bindings[binding.NodeID][binding.TagID] = &copied
	return nil
}

func (r *fakeVFSTagRepository) DetachFromNode(_ context.Context, nodeID, tagID uint) error {
	if r.bindings[nodeID] == nil || r.bindings[nodeID][tagID] == nil {
		return domainrepo.ErrNotFound
	}
	delete(r.bindings[nodeID], tagID)
	return nil
}

func (r *fakeVFSTagRepository) ListTagsForNode(_ context.Context, nodeID uint) ([]*entity.VFSTag, error) {
	bindings := r.bindings[nodeID]
	if len(bindings) == 0 {
		return []*entity.VFSTag{}, nil
	}
	items := make([]*entity.VFSTag, 0, len(bindings))
	for tagID := range bindings {
		if tag, exists := r.tags[tagID]; exists {
			items = append(items, cloneVFSTagForTest(tag))
		}
	}
	sortVFSTagsForTest(items)
	return items, nil
}

func (r *fakeVFSTagRepository) ListBindingsForNode(_ context.Context, nodeID uint) ([]*entity.VFSNodeTag, error) {
	bindings := r.bindings[nodeID]
	items := make([]*entity.VFSNodeTag, 0, len(bindings))
	for _, binding := range bindings {
		copied := *binding
		copied.CreatedBy = cloneUintPtr(binding.CreatedBy)
		items = append(items, &copied)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].TagID < items[j].TagID
	})
	return items, nil
}

func (r *fakeVFSTagRepository) ListNodeIDsByTag(_ context.Context, tagID uint) ([]uint, error) {
	nodeIDs := make([]uint, 0)
	for nodeID, bindings := range r.bindings {
		if bindings[tagID] != nil {
			nodeIDs = append(nodeIDs, nodeID)
		}
	}
	sort.Slice(nodeIDs, func(i, j int) bool { return nodeIDs[i] < nodeIDs[j] })
	return nodeIDs, nil
}

func cloneVFSTagForTest(tag *entity.VFSTag) *entity.VFSTag {
	if tag == nil {
		return nil
	}
	copied := *tag
	return &copied
}

func sortVFSTagsForTest(items []*entity.VFSTag) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].ID < items[j].ID
		}
		return items[i].Name < items[j].Name
	})
}
