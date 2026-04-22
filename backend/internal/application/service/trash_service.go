package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// TrashService 负责回收站列表、恢复和清理。
type TrashService struct {
	sourceRepo    domainrepo.SourceRepository
	trashItemRepo domainrepo.TrashItemRepository
	aclAuthorizer *ACLAuthorizer
	fileDrivers   map[string]FileDriver
}

// NewTrashService 创建回收站服务。
func NewTrashService(
	sourceRepo domainrepo.SourceRepository,
	trashItemRepo domainrepo.TrashItemRepository,
	options ...TrashServiceOption,
) *TrashService {
	service := &TrashService{
		sourceRepo:    sourceRepo,
		trashItemRepo: trashItemRepo,
		fileDrivers:   make(map[string]FileDriver),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// List 返回指定 source 的回收站列表。
func (s *TrashService) List(ctx context.Context, query appdto.TrashListQuery) (*appdto.TrashListResponse, error) {
	if _, err := s.sourceRepo.FindByID(ctx, query.SourceID); err != nil {
		return nil, err
	}
	items, err := s.trashItemRepo.ListBySourceID(ctx, query.SourceID)
	if err != nil {
		return nil, err
	}

	views := make([]appdto.TrashItemView, 0, len(items))
	for _, item := range items {
		visible, err := s.canViewTrashItem(ctx, item)
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		views = append(views, toTrashItemView(item))
	}
	pageItems, _, _ := paginateItems(views, query.Page, query.PageSize)
	return &appdto.TrashListResponse{Items: pageItems}, nil
}

// Restore 将回收站项恢复到原路径。
func (s *TrashService) Restore(ctx context.Context, id uint) (*appdto.TrashRestoreResponse, error) {
	item, err := s.trashItemRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeTrashRestore(ctx, item); err != nil {
		return nil, err
	}
	source, err := s.sourceRepo.FindByID(ctx, item.SourceID)
	if err != nil {
		return nil, err
	}

	if source.DriverType == "local" {
		if err := restoreLocalTrash(source, item); err != nil {
			return nil, err
		}
	} else {
		driver, err := s.getFileDriver(source.DriverType)
		if err != nil {
			return nil, err
		}
		if err := restoreDriverTrash(ctx, driver, source, item); err != nil {
			return nil, err
		}
	}

	if err := s.trashItemRepo.Delete(ctx, item.ID); err != nil {
		return nil, err
	}
	return &appdto.TrashRestoreResponse{
		ID:                  item.ID,
		Restored:            true,
		RestoredPath:        item.OriginalPath,
		RestoredVirtualPath: item.OriginalVirtualPath,
	}, nil
}

// Delete 永久删除单个回收站项。
func (s *TrashService) Delete(ctx context.Context, id uint) (*appdto.TrashDeleteResponse, error) {
	item, err := s.trashItemRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeTrashDelete(ctx, item); err != nil {
		return nil, err
	}
	if err := s.deleteTrashItemStorage(ctx, item); err != nil {
		return nil, err
	}
	if err := s.trashItemRepo.Delete(ctx, item.ID); err != nil {
		return nil, err
	}
	return &appdto.TrashDeleteResponse{
		ID:      &item.ID,
		Deleted: true,
	}, nil
}

// ClearSource 清空指定 source 的回收站。
func (s *TrashService) ClearSource(ctx context.Context, sourceID uint) (*appdto.TrashDeleteResponse, error) {
	if _, err := s.sourceRepo.FindByID(ctx, sourceID); err != nil {
		return nil, err
	}
	items, err := s.trashItemRepo.ListBySourceID(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	deletedCount := 0
	for _, item := range items {
		if err := s.authorizeTrashDelete(ctx, item); err != nil {
			if errors.Is(err, ErrACLDenied) {
				continue
			}
			return nil, err
		}
		if err := s.deleteTrashItemStorage(ctx, item); err != nil {
			return nil, err
		}
		if err := s.trashItemRepo.Delete(ctx, item.ID); err != nil {
			return nil, err
		}
		deletedCount++
	}
	return &appdto.TrashDeleteResponse{
		SourceID:     &sourceID,
		Cleared:      true,
		DeletedCount: deletedCount,
	}, nil
}

func (s *TrashService) deleteTrashItemStorage(ctx context.Context, item *entity.TrashItem) error {
	source, err := s.sourceRepo.FindByID(ctx, item.SourceID)
	if err != nil {
		return err
	}

	if source.DriverType == "local" {
		_, trashPhysical, err := resolvePhysicalPath(source, item.TrashPath)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(trashPhysical); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}

	driver, err := s.getFileDriver(source.DriverType)
	if err != nil {
		return err
	}
	if err := driver.Delete(ctx, source, item.TrashPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *TrashService) canViewTrashItem(ctx context.Context, item *entity.TrashItem) (bool, error) {
	if s.aclAuthorizer == nil {
		return true, nil
	}
	if err := s.aclAuthorizer.AuthorizePath(ctx, item.SourceID, item.OriginalPath, ACLActionWrite); err == nil {
		return true, nil
	} else if !errors.Is(err, ErrACLDenied) {
		return false, err
	}
	if err := s.aclAuthorizer.AuthorizePath(ctx, item.SourceID, item.OriginalPath, ACLActionDelete); err == nil {
		return true, nil
	} else if !errors.Is(err, ErrACLDenied) {
		return false, err
	}
	return false, nil
}

func (s *TrashService) authorizeTrashRestore(ctx context.Context, item *entity.TrashItem) error {
	if s.aclAuthorizer == nil {
		return nil
	}
	return s.aclAuthorizer.AuthorizePath(ctx, item.SourceID, item.OriginalPath, ACLActionWrite)
}

func (s *TrashService) authorizeTrashDelete(ctx context.Context, item *entity.TrashItem) error {
	if s.aclAuthorizer == nil {
		return nil
	}
	return s.aclAuthorizer.AuthorizePath(ctx, item.SourceID, item.OriginalPath, ACLActionDelete)
}

func (s *TrashService) getFileDriver(driverType string) (FileDriver, error) {
	driver, exists := s.fileDrivers[driverType]
	if !exists {
		return nil, ErrSourceDriverUnsupported
	}
	return driver, nil
}

func restoreLocalTrash(source *entity.StorageSource, item *entity.TrashItem) error {
	_, originalPhysical, err := resolvePhysicalPath(source, item.OriginalPath)
	if err != nil {
		return err
	}
	_, trashPhysical, err := resolvePhysicalPath(source, item.TrashPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(originalPhysical); err == nil {
		return ErrFileAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(originalPhysical), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(trashPhysical); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrFileNotFound
		}
		return err
	}
	if err := os.Rename(trashPhysical, originalPhysical); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrFileAlreadyExists
		}
		return err
	}
	return nil
}

func restoreDriverTrash(ctx context.Context, driver FileDriver, source *entity.StorageSource, item *entity.TrashItem) error {
	originalParent := path.Dir(item.OriginalPath)
	if originalParent == "." {
		originalParent = "/"
	}
	if err := ensureDriverPath(ctx, driver, source, originalParent); err != nil {
		return err
	}
	if _, err := driver.Stat(ctx, source, item.OriginalPath); err == nil {
		return ErrFileAlreadyExists
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := driver.Move(ctx, source, item.TrashPath, originalParent); err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return ErrFileNotFound
		case errors.Is(err, fs.ErrExist):
			return ErrFileAlreadyExists
		case errors.Is(err, os.ErrInvalid):
			return ErrPathInvalid
		default:
			return err
		}
	}
	return nil
}

func toTrashItemView(item *entity.TrashItem) appdto.TrashItemView {
	return appdto.TrashItemView{
		ID:                  item.ID,
		SourceID:            item.SourceID,
		OriginalPath:        item.OriginalPath,
		OriginalVirtualPath: item.OriginalVirtualPath,
		TrashPath:           item.TrashPath,
		Name:                item.Name,
		Size:                item.Size,
		DeletedAt:           item.DeletedAt.Format(time.RFC3339),
		ExpiresAt:           item.ExpiresAt.Format(time.RFC3339),
	}
}
