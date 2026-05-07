package service

import (
	"context"
	"errors"
	"testing"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

func TestVFSTagServiceCRUDAndNodeBinding(t *testing.T) {
	ctx := context.Background()
	now := fixedMetadataVFSTime()
	nodeRepo := newFakeVFSNodeRepository()
	tagRepo := newFakeVFSTagRepository()
	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSClock(func() time.Time { return now }))
	root, err := metadataSvc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "movie.mkv",
		Path:      "/movie.mkv",
		Kind:      entity.VFSNodeKindFile,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	svc := NewVFSTagService(nodeRepo, tagRepo, WithVFSTagClock(func() time.Time { return now }))
	ownerID := uint(7)

	created, err := svc.CreateTag(ctx, ownerID, appdto.VFSTagUpsertRequest{Name: "番剧", Color: "#66ccff"})
	if err != nil {
		t.Fatalf("CreateTag() error = %v", err)
	}
	if created.ID == 0 || created.Name != "番剧" || created.Color != "#66ccff" {
		t.Fatalf("unexpected created tag = %#v", created)
	}
	listed, err := svc.ListTags(ctx, ownerID)
	if err != nil {
		t.Fatalf("ListTags() error = %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != created.ID {
		t.Fatalf("unexpected tag list = %#v", listed)
	}

	attached, err := svc.AttachTag(ctx, ownerID, appdto.VFSNodeTagRequest{Path: "/movie.mkv", TagID: created.ID})
	if err != nil {
		t.Fatalf("AttachTag() error = %v", err)
	}
	if len(attached.Tags) != 1 || attached.Tags[0].Name != "番剧" {
		t.Fatalf("unexpected attached tags = %#v", attached)
	}
	nodeTags, err := svc.ListNodeTags(ctx, ownerID, "/movie.mkv")
	if err != nil {
		t.Fatalf("ListNodeTags() error = %v", err)
	}
	if len(nodeTags.Tags) != 1 || nodeTags.Tags[0].ID != created.ID {
		t.Fatalf("unexpected node tags = %#v", nodeTags)
	}

	detached, err := svc.DetachTag(ctx, ownerID, appdto.VFSNodeTagRequest{Path: "/movie.mkv", TagID: created.ID})
	if err != nil {
		t.Fatalf("DetachTag() error = %v", err)
	}
	if len(detached.Tags) != 0 {
		t.Fatalf("expected no tags after detach, got %#v", detached)
	}
}

func TestVFSTagServiceRejectsCrossOwnerTagBinding(t *testing.T) {
	ctx := context.Background()
	now := fixedMetadataVFSTime()
	nodeRepo := newFakeVFSNodeRepository()
	tagRepo := newFakeVFSTagRepository()
	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSClock(func() time.Time { return now }))
	root, err := metadataSvc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "file.txt",
		Path:      "/file.txt",
		Kind:      entity.VFSNodeKindFile,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	svc := NewVFSTagService(nodeRepo, tagRepo, WithVFSTagClock(func() time.Time { return now }))
	ownerTag, err := svc.CreateTag(ctx, 1, appdto.VFSTagUpsertRequest{Name: "private"})
	if err != nil {
		t.Fatalf("CreateTag(owner) error = %v", err)
	}

	if _, err := svc.AttachTag(ctx, 2, appdto.VFSNodeTagRequest{Path: "/file.txt", TagID: ownerTag.ID}); !errors.Is(err, ErrTagForbidden) {
		t.Fatalf("AttachTag(cross owner) error = %v, want ErrTagForbidden", err)
	}
	if _, err := svc.UpdateTag(ctx, 2, ownerTag.ID, appdto.VFSTagUpsertRequest{Name: "steal"}); !errors.Is(err, ErrTagForbidden) {
		t.Fatalf("UpdateTag(cross owner) error = %v, want ErrTagForbidden", err)
	}
}

func TestVFSTagServiceDetachMissingBindingReturnsStableError(t *testing.T) {
	ctx := context.Background()
	now := fixedMetadataVFSTime()
	nodeRepo := newFakeVFSNodeRepository()
	tagRepo := newFakeVFSTagRepository()
	metadataSvc := NewMetadataVFSService(nodeRepo, WithMetadataVFSClock(func() time.Time { return now }))
	root, err := metadataSvc.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "file.txt",
		Path:      "/file.txt",
		Kind:      entity.VFSNodeKindFile,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	svc := NewVFSTagService(nodeRepo, tagRepo, WithVFSTagClock(func() time.Time { return now }))
	tag, err := svc.CreateTag(ctx, 1, appdto.VFSTagUpsertRequest{Name: "todo"})
	if err != nil {
		t.Fatalf("CreateTag() error = %v", err)
	}

	if _, err := svc.DetachTag(ctx, 1, appdto.VFSNodeTagRequest{Path: "/file.txt", TagID: tag.ID}); !errors.Is(err, ErrTagBindingNotFound) {
		t.Fatalf("DetachTag(missing binding) error = %v, want ErrTagBindingNotFound", err)
	}
	if _, err := svc.AttachTag(ctx, 1, appdto.VFSNodeTagRequest{Path: "/missing.txt", TagID: tag.ID}); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("AttachTag(missing node) error = %v, want ErrFileNotFound", err)
	}
	if _, err := svc.AttachTag(ctx, 1, appdto.VFSNodeTagRequest{Path: "/file.txt", TagID: 999}); !errors.Is(err, domainrepo.ErrNotFound) {
		t.Fatalf("AttachTag(missing tag) error = %v, want domainrepo.ErrNotFound", err)
	}
}
