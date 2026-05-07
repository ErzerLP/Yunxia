package service

import (
	"context"
	"os"
	"testing"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/infrastructure/security"
)

func TestLegacyFileFacadeRoutesMutationsThroughVFS(t *testing.T) {
	ctx := context.Background()
	source := &entity.StorageSource{
		ID:         31,
		Name:       "Legacy",
		DriverType: "local",
		MountPath:  "/legacy",
		RootPath:   "/",
		IsEnabled:  true,
	}
	vfs := &legacyFileFacadeFakeVFS{
		mkdirItem: &appdto.VFSItem{
			Name:        "newdir",
			Path:        "/legacy/newdir",
			ParentPath:  "/legacy",
			SourceID:    &source.ID,
			EntryKind:   string(VirtualEntryKindDirectory),
			MimeType:    "inode/directory",
			CanDelete:   true,
			CanDownload: false,
		},
	}
	facade := NewLegacyFileFacade(
		newFakeMetadataVFSSyncSourceRepository(source),
		vfs,
		&legacyFileFacadeFakeFile{},
	)

	created, err := facade.Mkdir(ctx, appdto.MkdirRequest{
		SourceID:   source.ID,
		ParentPath: "/",
		Name:       "newdir",
	})
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if vfs.mkdirReq.ParentPath != "/legacy" || vfs.mkdirReq.Name != "newdir" {
		t.Fatalf("expected v1 mkdir to route through VFS /legacy, got %+v", vfs.mkdirReq)
	}
	if created.Path != "/newdir" || created.ParentPath != "/" || !created.IsDir {
		t.Fatalf("unexpected legacy file item = %+v", created)
	}
}

func TestLegacyFileFacadeListConvertsVFSToSourcePaths(t *testing.T) {
	ctx := context.Background()
	source := &entity.StorageSource{
		ID:         32,
		Name:       "Legacy",
		DriverType: "local",
		MountPath:  "/legacy",
		RootPath:   "/",
		IsEnabled:  true,
	}
	otherSourceID := uint(99)
	vfs := &legacyFileFacadeFakeVFS{
		listResp: &appdto.VFSListResponse{
			CurrentPath: "/legacy",
			Items: []appdto.VFSItem{
				{
					Name:        "visible.txt",
					Path:        "/legacy/visible.txt",
					ParentPath:  "/legacy",
					SourceID:    &source.ID,
					EntryKind:   string(VirtualEntryKindFile),
					MimeType:    "text/plain",
					CanDownload: true,
					CanDelete:   true,
				},
				{
					Name:      "other-mount",
					Path:      "/legacy/other-mount",
					SourceID:  &otherSourceID,
					EntryKind: string(VirtualEntryKindDirectory),
				},
			},
		},
	}
	facade := NewLegacyFileFacade(
		newFakeMetadataVFSSyncSourceRepository(source),
		vfs,
		&legacyFileFacadeFakeFile{},
	)

	listed, _, _, total, _, err := facade.List(ctx, appdto.FileListQuery{SourceID: source.ID, Path: "/"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if vfs.listPath != "/legacy" {
		t.Fatalf("expected VFS list path /legacy, got %q", vfs.listPath)
	}
	if total != 1 || len(listed.Items) != 1 {
		t.Fatalf("expected only same-source VFS item, total=%d items=%+v", total, listed.Items)
	}
	if listed.Items[0].Path != "/visible.txt" || listed.Items[0].SourceID != source.ID {
		t.Fatalf("unexpected legacy list item = %+v", listed.Items[0])
	}
}

type legacyFileFacadeFakeVFS struct {
	listPath   string
	listResp   *appdto.VFSListResponse
	searchResp *appdto.VFSSearchResponse
	mkdirReq   appdto.VFSMkdirRequest
	mkdirItem  *appdto.VFSItem
	renameReq  appdto.VFSRenameRequest
	moveReq    appdto.VFSMoveCopyRequest
	copyReq    appdto.VFSMoveCopyRequest
	deleteReq  appdto.VFSDeleteRequest
}

func (f *legacyFileFacadeFakeVFS) List(_ context.Context, currentPath string) (*appdto.VFSListResponse, error) {
	f.listPath = currentPath
	if f.listResp != nil {
		return f.listResp, nil
	}
	return &appdto.VFSListResponse{CurrentPath: currentPath}, nil
}

func (f *legacyFileFacadeFakeVFS) Search(_ context.Context, pathPrefix string, keyword string) (*appdto.VFSSearchResponse, error) {
	if f.searchResp != nil {
		return f.searchResp, nil
	}
	return &appdto.VFSSearchResponse{PathPrefix: pathPrefix, Keyword: keyword}, nil
}

func (f *legacyFileFacadeFakeVFS) Mkdir(_ context.Context, req appdto.VFSMkdirRequest) (*appdto.VFSItem, error) {
	f.mkdirReq = req
	return f.mkdirItem, nil
}

func (f *legacyFileFacadeFakeVFS) Rename(_ context.Context, req appdto.VFSRenameRequest) (string, string, *appdto.VFSItem, error) {
	f.renameReq = req
	return req.Path, pathForRenamedVFSItem(f.mkdirItem), f.mkdirItem, nil
}

func (f *legacyFileFacadeFakeVFS) Move(_ context.Context, req appdto.VFSMoveCopyRequest) (string, string, error) {
	f.moveReq = req
	return req.Path, req.TargetPath + "/" + "moved", nil
}

func (f *legacyFileFacadeFakeVFS) Copy(_ context.Context, req appdto.VFSMoveCopyRequest) (string, string, error) {
	f.copyReq = req
	return req.Path, req.TargetPath + "/" + "copied", nil
}

func (f *legacyFileFacadeFakeVFS) Delete(_ context.Context, req appdto.VFSDeleteRequest) (time.Time, error) {
	f.deleteReq = req
	return time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), nil
}

type legacyFileFacadeFakeFile struct{}

func (legacyFileFacadeFakeFile) AccessURL(context.Context, appdto.AccessURLRequest) (*appdto.AccessURLResponse, error) {
	return nil, ErrSourceDriverUnsupported
}

func (legacyFileFacadeFakeFile) ResolveDownload(context.Context, uint, string) (*os.File, os.FileInfo, string, error) {
	return nil, nil, "", ErrSourceDriverUnsupported
}

func (legacyFileFacadeFakeFile) ResolveDownloadRedirect(context.Context, uint, string, string) (string, error) {
	return "", nil
}

func (legacyFileFacadeFakeFile) ValidateFileAccessToken(string) (*security.FileAccessClaims, error) {
	return nil, ErrInvalidCredentials
}

func (legacyFileFacadeFakeFile) AuthenticateBearerToken(context.Context, string) (*security.RequestAuth, error) {
	return nil, ErrInvalidCredentials
}

func pathForRenamedVFSItem(item *appdto.VFSItem) string {
	if item == nil {
		return ""
	}
	return item.Path
}
