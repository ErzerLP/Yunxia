package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

type systemStatsUserRepository interface {
	Count(ctx context.Context) (int64, error)
}

type systemStatsSourceRepository interface {
	Count(ctx context.Context) (int64, error)
	ListEnabled(ctx context.Context) ([]*entity.StorageSource, error)
}

type systemStatsTaskRepository interface {
	List(ctx context.Context) ([]*entity.DownloadTask, error)
}

// SystemServiceOption 定义系统服务可选能力。
type SystemServiceOption func(*SystemService)

// WithSystemStatsDependencies 注入系统统计所需依赖。
func WithSystemStatsDependencies(
	userRepo systemStatsUserRepository,
	sourceRepo systemStatsSourceRepository,
	taskRepo systemStatsTaskRepository,
) SystemServiceOption {
	return func(s *SystemService) {
		s.statsUserRepo = userRepo
		s.statsSourceRepo = sourceRepo
		s.statsTaskRepo = taskRepo
	}
}

// WithSystemStatsFileDriver 注册系统统计使用的文件驱动。
func WithSystemStatsFileDriver(driverType string, driver FileDriver) SystemServiceOption {
	return func(s *SystemService) {
		if driverType == "" || driver == nil {
			return
		}
		if s.fileDrivers == nil {
			s.fileDrivers = make(map[string]FileDriver)
		}
		s.fileDrivers[driverType] = driver
	}
}

// GetStats 返回系统统计信息。
func (s *SystemService) GetStats(ctx context.Context) (*appdto.SystemStatsResponse, error) {
	resp := &appdto.SystemStatsResponse{}

	if s.statsSourceRepo != nil {
		sourcesTotal, err := s.statsSourceRepo.Count(ctx)
		if err != nil {
			return nil, err
		}
		resp.SourcesTotal = sourcesTotal

		filesTotal, storageUsedBytes, err := s.collectStorageStats(ctx)
		if err != nil {
			return nil, err
		}
		resp.FilesTotal = filesTotal
		resp.StorageUsedBytes = storageUsedBytes
	}

	if s.statsUserRepo != nil {
		usersTotal, err := s.statsUserRepo.Count(ctx)
		if err != nil {
			return nil, err
		}
		resp.UsersTotal = usersTotal
	}

	if s.statsTaskRepo != nil {
		tasks, err := s.statsTaskRepo.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			switch strings.ToLower(strings.TrimSpace(task.Status)) {
			case "running":
				resp.DownloadsRunning++
			case "completed":
				resp.DownloadsCompleted++
			}
		}
	}

	return resp, nil
}

func (s *SystemService) collectStorageStats(ctx context.Context) (int64, int64, error) {
	if s.statsSourceRepo == nil {
		return 0, 0, nil
	}

	sources, err := s.statsSourceRepo.ListEnabled(ctx)
	if err != nil {
		return 0, 0, err
	}

	var filesTotal int64
	var storageUsedBytes int64
	for _, source := range sources {
		sourceFilesTotal, sourceStorageUsedBytes, sourceErr := s.collectSourceStorageStats(ctx, source)
		if sourceErr != nil {
			return 0, 0, sourceErr
		}
		filesTotal += sourceFilesTotal
		storageUsedBytes += sourceStorageUsedBytes
	}

	return filesTotal, storageUsedBytes, nil
}

func (s *SystemService) collectSourceStorageStats(ctx context.Context, source *entity.StorageSource) (int64, int64, error) {
	if source == nil || !source.IsEnabled {
		return 0, 0, nil
	}

	if source.DriverType == "local" {
		return collectLocalSourceStats(source)
	}

	driver, exists := s.fileDrivers[source.DriverType]
	if !exists {
		return 0, 0, nil
	}
	return s.collectDriverStorageStats(ctx, driver, source, "/")
}

func collectLocalSourceStats(source *entity.StorageSource) (int64, int64, error) {
	_, physicalRoot, err := resolvePhysicalPath(source, "/")
	if err != nil {
		return 0, 0, err
	}
	if _, err := os.Stat(physicalRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	var filesTotal int64
	var storageUsedBytes int64
	err = filepath.WalkDir(physicalRoot, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == physicalRoot {
			return nil
		}

		relative, err := filepath.Rel(physicalRoot, current)
		if err != nil {
			return err
		}
		virtualPath := "/" + filepath.ToSlash(relative)
		if isHiddenVirtualPath(virtualPath) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		filesTotal++
		storageUsedBytes += info.Size()
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	return filesTotal, storageUsedBytes, nil
}

func (s *SystemService) collectDriverStorageStats(
	ctx context.Context,
	driver FileDriver,
	source *entity.StorageSource,
	virtualPath string,
) (int64, int64, error) {
	entries, err := driver.List(ctx, source, virtualPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, domainrepo.ErrNotFound) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	var filesTotal int64
	var storageUsedBytes int64
	for _, entry := range entries {
		if isHiddenStorageEntry(entry) {
			continue
		}
		if entry.IsDir {
			childFilesTotal, childStorageUsedBytes, childErr := s.collectDriverStorageStats(ctx, driver, source, entry.Path)
			if childErr != nil {
				return 0, 0, childErr
			}
			filesTotal += childFilesTotal
			storageUsedBytes += childStorageUsedBytes
			continue
		}

		filesTotal++
		storageUsedBytes += entry.Size
	}

	return filesTotal, storageUsedBytes, nil
}
