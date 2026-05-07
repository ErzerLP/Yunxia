package service

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/infrastructure/security"
)

func TestFileServiceResolveVFSObjectDownloadUsesStorageObjectLocator(t *testing.T) {
	ctx := context.Background()
	basePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(basePath, "objects"), 0o755); err != nil {
		t.Fatalf("MkdirAll(objects) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(basePath, "objects", "blob.bin"), []byte("object locator body"), 0o644); err != nil {
		t.Fatalf("WriteFile(blob) error = %v", err)
	}
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		ID:         12,
		Name:       "Docs",
		DriverType: "local",
		ConfigJSON: configJSON,
		MountPath:  "/docs",
		RootPath:   "/",
		IsEnabled:  true,
	}
	sourceRepo := newFakeMetadataVFSSyncSourceRepository(source)
	nodeRepo := newFakeVFSNodeRepository()
	objectRepo := newFakeStorageObjectRepository()
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	metadata := NewMetadataVFSService(nodeRepo, WithMetadataVFSClock(func() time.Time { return now }))
	root, err := metadata.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	mount := mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "docs",
		Path:      "/docs",
		Kind:      entity.VFSNodeKindMount,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	object := &entity.StorageObject{
		SourceID:    source.ID,
		DriverType:  "local",
		LocatorType: "local_path",
		LocatorJSON: `{"path":"/objects/blob.bin"}`,
		MimeType:    "application/octet-stream",
		Status:      entity.StorageObjectStatusAvailable,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := objectRepo.Create(ctx, object); err != nil {
		t.Fatalf("Create(object) error = %v", err)
	}
	mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:   &mount.ID,
		Name:       "display.bin",
		Path:       "/docs/display.bin",
		Kind:       entity.VFSNodeKindFile,
		ObjectID:   &object.ID,
		SourceID:   &source.ID,
		SyncState:  entity.VFSNodeSyncStateIndexed,
		CreatedAt:  now,
		UpdatedAt:  now,
		IndexedAt:  &now,
		LastSeenAt: &now,
	})
	fileSvc := NewFileService(
		sourceRepo,
		nil,
		nil,
		nil,
		WithFileMetadataVFSObjectAccess(metadata, objectRepo),
	)

	download, matched, err := fileSvc.ResolveVFSObjectDownload(ctx, "/docs/display.bin", "inline")
	if err != nil {
		t.Fatalf("ResolveVFSObjectDownload() error = %v", err)
	}
	if !matched || download == nil || download.File == nil {
		t.Fatalf("expected object locator download match, got matched=%v download=%#v", matched, download)
	}
	defer download.File.Close()
	body, err := io.ReadAll(download.File)
	if err != nil {
		t.Fatalf("ReadAll(download) error = %v", err)
	}
	if string(body) != "object locator body" || download.InnerPath != "/objects/blob.bin" {
		t.Fatalf("unexpected download body/path = %q %q", string(body), download.InnerPath)
	}
}

func TestFileServiceLegacyDownloadUsesMetadataVFSObjectLocator(t *testing.T) {
	ctx := context.Background()
	basePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(basePath, "objects"), 0o755); err != nil {
		t.Fatalf("MkdirAll(objects) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(basePath, "objects", "blob.bin"), []byte("legacy object body"), 0o644); err != nil {
		t.Fatalf("WriteFile(blob) error = %v", err)
	}
	configJSON, err := marshalLocalSourceConfig(basePath)
	if err != nil {
		t.Fatalf("marshalLocalSourceConfig() error = %v", err)
	}
	source := &entity.StorageSource{
		ID:         21,
		Name:       "Legacy Docs",
		DriverType: "local",
		ConfigJSON: configJSON,
		MountPath:  "/docs",
		RootPath:   "/",
		IsEnabled:  true,
	}
	sourceRepo := newFakeMetadataVFSSyncSourceRepository(source)
	nodeRepo := newFakeVFSNodeRepository()
	objectRepo := newFakeStorageObjectRepository()
	now := time.Date(2026, 5, 8, 11, 0, 0, 0, time.UTC)
	metadata := NewMetadataVFSService(nodeRepo, WithMetadataVFSClock(func() time.Time { return now }))
	root, err := metadata.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	mount := mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:  &root.ID,
		Name:      "docs",
		Path:      "/docs",
		Kind:      entity.VFSNodeKindMount,
		SourceID:  &source.ID,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
	})
	object := &entity.StorageObject{
		SourceID:    source.ID,
		DriverType:  "local",
		LocatorType: "local_path",
		LocatorJSON: `{"path":"/objects/blob.bin"}`,
		MimeType:    "application/octet-stream",
		Status:      entity.StorageObjectStatusAvailable,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := objectRepo.Create(ctx, object); err != nil {
		t.Fatalf("Create(object) error = %v", err)
	}
	mustCreateMetadataVFSNode(t, nodeRepo, &entity.VFSNode{
		ParentID:   &mount.ID,
		Name:       "display.bin",
		Path:       "/docs/display.bin",
		Kind:       entity.VFSNodeKindFile,
		ObjectID:   &object.ID,
		SourceID:   &source.ID,
		SyncState:  entity.VFSNodeSyncStateIndexed,
		CreatedAt:  now,
		UpdatedAt:  now,
		IndexedAt:  &now,
		LastSeenAt: &now,
	})
	tokenSvc := security.NewFileAccessTokenService("legacy-object-secret")
	fileSvc := NewFileService(
		sourceRepo,
		tokenSvc,
		nil,
		nil,
		WithFileMetadataVFSObjectAccess(metadata, objectRepo),
	)

	resp, err := fileSvc.AccessURL(ctx, appdtoAccessURLRequest(source.ID, "/display.bin"))
	if err != nil {
		t.Fatalf("AccessURL() error = %v", err)
	}
	parsed, err := url.Parse(resp.URL)
	if err != nil {
		t.Fatalf("Parse(access url) error = %v", err)
	}
	if got := parsed.Query().Get("source_id"); got != "21" {
		t.Fatalf("legacy access url source_id = %q, want 21", got)
	}
	if got := parsed.Query().Get("path"); got != "/display.bin" {
		t.Fatalf("legacy access url path = %q, want /display.bin", got)
	}

	redirectURL, err := fileSvc.ResolveDownloadRedirect(ctx, source.ID, "/display.bin", "attachment")
	if err != nil {
		t.Fatalf("ResolveDownloadRedirect() error = %v", err)
	}
	if redirectURL != "" {
		t.Fatalf("local metadata object redirect = %q, want empty", redirectURL)
	}
	file, _, _, err := fileSvc.ResolveDownload(ctx, source.ID, "/display.bin")
	if err != nil {
		t.Fatalf("ResolveDownload() error = %v", err)
	}
	defer file.Close()
	body, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll(download) error = %v", err)
	}
	if string(body) != "legacy object body" {
		t.Fatalf("legacy download body = %q, want object locator body", string(body))
	}
}

func appdtoAccessURLRequest(sourceID uint, filePath string) appdto.AccessURLRequest {
	return appdto.AccessURLRequest{
		SourceID:    sourceID,
		Path:        filePath,
		Purpose:     "download",
		Disposition: "attachment",
		ExpiresIn:   300,
	}
}
