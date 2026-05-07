package gorm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	gormpkg "gorm.io/gorm"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

func TestVFSNodeRepositoryCRUDPathChildrenAndUpsert(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewVFSNodeRepository(db)
	now := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	root := &entity.VFSNode{
		Name:      "",
		Path:      "/",
		Kind:      entity.VFSNodeKindRoot,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.Create(ctx, root); err != nil {
		t.Fatalf("Create(root) error = %v", err)
	}

	sourceID := uint(7)
	indexedAt := now.Add(time.Minute)
	node := &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "anime",
		Path:      "/anime",
		Kind:      entity.VFSNodeKindVirtualDir,
		SourceID:  &sourceID,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
		IndexedAt: &indexedAt,
	}
	if err := repo.Create(ctx, node); err != nil {
		t.Fatalf("Create(child) error = %v", err)
	}

	gotByPath, err := repo.FindByPath(ctx, "/anime")
	if err != nil {
		t.Fatalf("FindByPath() error = %v", err)
	}
	if gotByPath.ID != node.ID || gotByPath.ParentID == nil || *gotByPath.ParentID != root.ID {
		t.Fatalf("unexpected FindByPath result = %#v", gotByPath)
	}

	children, err := repo.ListChildren(ctx, &root.ID, domainrepo.VFSNodeListFilter{})
	if err != nil {
		t.Fatalf("ListChildren() error = %v", err)
	}
	if len(children) != 1 || children[0].ID != node.ID {
		t.Fatalf("unexpected children = %#v", children)
	}

	prefixItems, err := repo.ListByPathPrefix(ctx, "/", domainrepo.VFSNodeListFilter{})
	if err != nil {
		t.Fatalf("ListByPathPrefix() error = %v", err)
	}
	if len(prefixItems) != 2 {
		t.Fatalf("ListByPathPrefix(/) returned %d items, want 2: %#v", len(prefixItems), prefixItems)
	}

	duplicate := &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "anime",
		Path:      "/anime",
		Kind:      entity.VFSNodeKindVirtualDir,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.Create(ctx, duplicate); !errors.Is(err, domainrepo.ErrConflict) {
		t.Fatalf("Create(duplicate) error = %v, want ErrConflict", err)
	}

	providerID := "provider-anime"
	upsert := &entity.VFSNode{
		ParentID:       &root.ID,
		Name:           "anime",
		Path:           "/anime",
		Kind:           entity.VFSNodeKindDir,
		ProviderItemID: &providerID,
		Size:           42,
		SyncState:      entity.VFSNodeSyncStateStale,
		CreatedAt:      now.Add(2 * time.Minute),
		UpdatedAt:      now.Add(3 * time.Minute),
		IndexedAt:      &indexedAt,
	}
	if err := repo.UpsertByPath(ctx, upsert); err != nil {
		t.Fatalf("UpsertByPath() error = %v", err)
	}
	if upsert.ID != node.ID || upsert.Kind != entity.VFSNodeKindDir || upsert.SyncState != entity.VFSNodeSyncStateStale || upsert.ProviderItemID == nil {
		t.Fatalf("unexpected upsert result = %#v", upsert)
	}

	upsert.SourceID = nil
	upsert.ProviderItemID = nil
	upsert.UpdatedAt = now.Add(4 * time.Minute)
	if err := repo.Update(ctx, upsert); err != nil {
		t.Fatalf("Update(clear nullable fields) error = %v", err)
	}
	updated, err := repo.FindByID(ctx, upsert.ID)
	if err != nil {
		t.Fatalf("FindByID(updated) error = %v", err)
	}
	if updated.SourceID != nil || updated.ProviderItemID != nil {
		t.Fatalf("nullable fields were not cleared: %#v", updated)
	}

	grandchild := &entity.VFSNode{
		ParentID:  &updated.ID,
		Name:      "ep01.mkv",
		Path:      "/anime/ep01.mkv",
		Kind:      entity.VFSNodeKindFile,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.Create(ctx, grandchild); err != nil {
		t.Fatalf("Create(grandchild) error = %v", err)
	}
	if err := repo.Delete(ctx, updated.ID); err != nil {
		t.Fatalf("Delete(soft subtree) error = %v", err)
	}
	if _, err := repo.FindByPath(ctx, "/anime"); !errors.Is(err, domainrepo.ErrNotFound) {
		t.Fatalf("FindByPath(deleted) error = %v, want ErrNotFound", err)
	}
	activeItems, err := repo.ListByPathPrefix(ctx, "/", domainrepo.VFSNodeListFilter{})
	if err != nil {
		t.Fatalf("ListByPathPrefix(active after soft delete) error = %v", err)
	}
	if len(activeItems) != 1 || activeItems[0].ID != root.ID {
		t.Fatalf("unexpected active items after soft delete = %#v", activeItems)
	}
	allItems, err := repo.ListByPathPrefix(ctx, "/", domainrepo.VFSNodeListFilter{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("ListByPathPrefix(include deleted) error = %v", err)
	}
	deletedByID := map[uint]bool{}
	for _, item := range allItems {
		deletedByID[item.ID] = item.IsDeleted
	}
	if !deletedByID[updated.ID] || !deletedByID[grandchild.ID] {
		t.Fatalf("soft delete did not mark subtree deleted: %#v", allItems)
	}

	replacement := &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "anime",
		Path:      "/anime",
		Kind:      entity.VFSNodeKindDir,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now.Add(5 * time.Minute),
		UpdatedAt: now.Add(5 * time.Minute),
	}
	if err := repo.Create(ctx, replacement); err != nil {
		t.Fatalf("Create(replacement after soft delete) error = %v", err)
	}
	if replacement.ID == updated.ID {
		t.Fatalf("replacement reused deleted node ID unexpectedly: %#v", replacement)
	}
}

func TestStorageObjectAndVFSMountRepositoryPersistJSONBAndExplicitFalse(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	assertJSONBColumn(t, db, &StorageObjectModel{}, "locator_json")
	assertJSONBColumn(t, db, &VFSMountModel{}, "root_locator_json")

	ctx := context.Background()
	now := time.Date(2026, 5, 7, 11, 0, 0, 0, time.UTC)
	objectRepo := NewStorageObjectRepository(db)
	object := &entity.StorageObject{
		SourceID:    10,
		DriverType:  "s3",
		LocatorType: "s3_key",
		LocatorJSON: "",
		Size:        100,
		Status:      entity.StorageObjectStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := objectRepo.Create(ctx, object); err != nil {
		t.Fatalf("Create(object) error = %v", err)
	}
	if object.LocatorJSON != "{}" {
		t.Fatalf("LocatorJSON = %q, want {}", object.LocatorJSON)
	}
	object.Status = entity.StorageObjectStatusAvailable
	object.Size = 256
	object.UpdatedAt = now.Add(time.Minute)
	if err := objectRepo.Update(ctx, object); err != nil {
		t.Fatalf("Update(object) error = %v", err)
	}
	upserted := &entity.StorageObject{
		SourceID:    10,
		DriverType:  "s3",
		LocatorType: "s3_key",
		LocatorJSON: `{"key":"anime/ep01.mkv"}`,
		Size:        512,
		Status:      entity.StorageObjectStatusAvailable,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := objectRepo.UpsertByLocator(ctx, upserted); err != nil {
		t.Fatalf("UpsertByLocator(create) error = %v", err)
	}
	firstObjectID := upserted.ID
	upserted.Size = 1024
	upserted.ETag = "etag-updated"
	upserted.UpdatedAt = now.Add(2 * time.Minute)
	if err := objectRepo.UpsertByLocator(ctx, upserted); err != nil {
		t.Fatalf("UpsertByLocator(update) error = %v", err)
	}
	if upserted.ID != firstObjectID || upserted.Size != 1024 || upserted.ETag != "etag-updated" {
		t.Fatalf("unexpected object after locator upsert = %#v", upserted)
	}
	foundByLocator, err := objectRepo.FindByLocator(ctx, 10, "s3", "s3_key", `{"key":"anime/ep01.mkv"}`)
	if err != nil {
		t.Fatalf("FindByLocator() error = %v", err)
	}
	if foundByLocator.ID != firstObjectID || foundByLocator.Size != 1024 {
		t.Fatalf("unexpected object by locator = %#v", foundByLocator)
	}
	listedObjects, err := objectRepo.List(ctx, domainrepo.StorageObjectListFilter{SourceID: 10, Status: entity.StorageObjectStatusAvailable})
	if err != nil {
		t.Fatalf("List(objects) error = %v", err)
	}
	if len(listedObjects) != 2 {
		t.Fatalf("unexpected listed objects = %#v", listedObjects)
	}

	nodeRepo := NewVFSNodeRepository(db)
	mountNode := &entity.VFSNode{
		Name:      "s3",
		Path:      "/s3",
		Kind:      entity.VFSNodeKindMount,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := nodeRepo.Create(ctx, mountNode); err != nil {
		t.Fatalf("Create(mount node) error = %v", err)
	}

	mountRepo := NewVFSMountRepository(db)
	mount := &entity.VFSMount{
		SourceID:        10,
		NodeID:          mountNode.ID,
		MountPath:       "/s3",
		RootLocatorJSON: "",
		Mode:            entity.VFSMountModeMirror,
		IsEnabled:       false,
		SortOrder:       3,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := mountRepo.Create(ctx, mount); err != nil {
		t.Fatalf("Create(mount) error = %v", err)
	}
	if mount.IsEnabled {
		t.Fatalf("explicit false IsEnabled was not preserved: %#v", mount)
	}
	gotMount, err := mountRepo.FindByMountPath(ctx, "/s3")
	if err != nil {
		t.Fatalf("FindByMountPath() error = %v", err)
	}
	if gotMount.RootLocatorJSON != "{}" || gotMount.IsEnabled {
		t.Fatalf("unexpected mount = %#v", gotMount)
	}

	upsert := &entity.VFSMount{
		SourceID:        10,
		NodeID:          mountNode.ID,
		MountPath:       "/s3",
		RootLocatorJSON: `{"prefix":"anime"}`,
		Mode:            entity.VFSMountModeWriteThrough,
		IsEnabled:       true,
		SortOrder:       1,
		CreatedAt:       now,
		UpdatedAt:       now.Add(time.Minute),
	}
	if err := mountRepo.UpsertByMountPath(ctx, upsert); err != nil {
		t.Fatalf("UpsertByMountPath() error = %v", err)
	}
	if upsert.ID != mount.ID || !upsert.IsEnabled || upsert.RootLocatorJSON != `{"prefix":"anime"}` {
		t.Fatalf("unexpected mount upsert result = %#v", upsert)
	}
	enabled := true
	mounts, err := mountRepo.List(ctx, domainrepo.VFSMountListFilter{Enabled: &enabled})
	if err != nil {
		t.Fatalf("List(mounts) error = %v", err)
	}
	if len(mounts) != 1 || mounts[0].ID != mount.ID {
		t.Fatalf("unexpected mounts = %#v", mounts)
	}
}

func TestVFSTagRepositoryUpsertAttachDetach(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	nodeRepo := NewVFSNodeRepository(db)
	node := &entity.VFSNode{
		Name:      "movie.mkv",
		Path:      "/movie.mkv",
		Kind:      entity.VFSNodeKindFile,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := nodeRepo.Create(ctx, node); err != nil {
		t.Fatalf("Create(node) error = %v", err)
	}

	tagRepo := NewVFSTagRepository(db)
	tag := &entity.VFSTag{
		OwnerUserID: 1,
		Name:        "anime",
		Color:       "#66ccff",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := tagRepo.UpsertByOwnerAndName(ctx, tag); err != nil {
		t.Fatalf("UpsertByOwnerAndName(create) error = %v", err)
	}
	firstID := tag.ID
	tag.Color = "#ffcc66"
	tag.UpdatedAt = now.Add(time.Minute)
	if err := tagRepo.UpsertByOwnerAndName(ctx, tag); err != nil {
		t.Fatalf("UpsertByOwnerAndName(update) error = %v", err)
	}
	if tag.ID != firstID || tag.Color != "#ffcc66" {
		t.Fatalf("unexpected tag after upsert = %#v", tag)
	}

	creatorID := uint(99)
	binding := &entity.VFSNodeTag{
		NodeID:    node.ID,
		TagID:     tag.ID,
		CreatedBy: &creatorID,
		CreatedAt: now,
	}
	if err := tagRepo.AttachToNode(ctx, binding); err != nil {
		t.Fatalf("AttachToNode() error = %v", err)
	}
	if err := tagRepo.AttachToNode(ctx, binding); err != nil {
		t.Fatalf("AttachToNode(duplicate) error = %v", err)
	}

	tags, err := tagRepo.ListTagsForNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("ListTagsForNode() error = %v", err)
	}
	if len(tags) != 1 || tags[0].ID != tag.ID {
		t.Fatalf("unexpected node tags = %#v", tags)
	}
	nodeIDs, err := tagRepo.ListNodeIDsByTag(ctx, tag.ID)
	if err != nil {
		t.Fatalf("ListNodeIDsByTag() error = %v", err)
	}
	if len(nodeIDs) != 1 || nodeIDs[0] != node.ID {
		t.Fatalf("unexpected node IDs = %#v", nodeIDs)
	}

	if err := tagRepo.DetachFromNode(ctx, node.ID, tag.ID); err != nil {
		t.Fatalf("DetachFromNode() error = %v", err)
	}
	if err := tagRepo.DetachFromNode(ctx, node.ID, tag.ID); !errors.Is(err, domainrepo.ErrNotFound) {
		t.Fatalf("DetachFromNode(missing) error = %v, want ErrNotFound", err)
	}
	tags, err = tagRepo.ListTagsForNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("ListTagsForNode(after detach) error = %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected no tags after detach, got %#v", tags)
	}
}

func TestVFSMetadataLikePatternEscapesWildcards(t *testing.T) {
	got := pathChildrenLikePattern(`/anime_%\raw`)
	want := `/anime\_\%\\raw/%`
	if got != want {
		t.Fatalf("pathChildrenLikePattern() = %q, want %q", got, want)
	}
}

func assertJSONBColumn(t *testing.T, db *gormpkg.DB, model any, column string) {
	t.Helper()

	columnTypes, err := db.Migrator().ColumnTypes(model)
	if err != nil {
		t.Fatalf("ColumnTypes(%T) error = %v", model, err)
	}
	for _, columnType := range columnTypes {
		if columnType.Name() != column {
			continue
		}
		if !strings.EqualFold(columnType.DatabaseTypeName(), "jsonb") {
			t.Fatalf("%T.%s type = %q, want jsonb", model, column, columnType.DatabaseTypeName())
		}
		return
	}
	t.Fatalf("column %s not found on %T", column, model)
}
