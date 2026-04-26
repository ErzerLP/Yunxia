package service

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	appaudit "yunxia/internal/application/audit"
	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

// Downloader 定义离线下载器能力。
type Downloader interface {
	AddURI(ctx context.Context, uri string, dir string) (string, error)
	TellStatus(ctx context.Context, externalID string) (*DownloadStatus, error)
	Pause(ctx context.Context, externalID string) error
	Resume(ctx context.Context, externalID string) error
	Remove(ctx context.Context, externalID string) error
}

// DownloadStatus 表示下载器返回的状态。
type DownloadStatus struct {
	Status         string
	CompletedBytes int64
	TotalBytes     *int64
	DownloadSpeed  int64
	ETASeconds     *int64
	DisplayName    string
	ErrorMessage   *string
}

// TaskImportDriver 定义下载完成后导入非 local 存储源的最小能力。
type TaskImportDriver interface {
	ImportFile(ctx context.Context, source *entity.StorageSource, targetPath string, localPath string) error
}

// TaskService 负责离线下载任务接口。
type TaskService struct {
	taskRepo      domainrepo.TaskRepository
	sourceRepo    domainrepo.SourceRepository
	aclAuthorizer *ACLAuthorizer
	downloader    Downloader
	stagingRoot   string
	importDrivers map[string]TaskImportDriver
	vfsResolver   interface {
		ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error)
	}
	logger        *slog.Logger
	auditRecorder *appaudit.Recorder
}

type resolvedTaskTarget struct {
	source                  *entity.StorageSource
	savePath                string
	targetVirtualParentPath string
	saveVirtualPath         string
	resolvedSourceID        uint
	resolvedInnerSavePath   string
}

// NewTaskService 创建任务服务。
func NewTaskService(taskRepo domainrepo.TaskRepository, sourceRepo domainrepo.SourceRepository, downloader Downloader, options ...TaskServiceOption) *TaskService {
	service := &TaskService{
		taskRepo:      taskRepo,
		sourceRepo:    sourceRepo,
		downloader:    downloader,
		stagingRoot:   filepath.Join(os.TempDir(), "yunxia-download-staging"),
		importDrivers: make(map[string]TaskImportDriver),
		logger:        newServiceLogger("service.task"),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// List 返回任务列表。
func (s *TaskService) List(ctx context.Context) (*appdto.TaskListResponse, error) {
	items, err := s.taskRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	auth, _ := security.RequestAuthFromContext(ctx)
	result := make([]appdto.DownloadTaskView, 0, len(items))
	for _, item := range items {
		if !permission.CanReadTask(auth.UserID, item.UserID, auth.Capabilities) {
			continue
		}
		_ = s.refreshTask(ctx, item)
		result = append(result, toTaskView(item))
	}
	return &appdto.TaskListResponse{Items: result}, nil
}

// Create 创建任务。
func (s *TaskService) Create(ctx context.Context, req appdto.CreateTaskRequest) (*appdto.DownloadTaskView, error) {
	if s.downloader == nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_DRIVER_UNSUPPORTED",
			SourceID:     &req.SourceID,
		})
		return nil, ErrSourceDriverUnsupported
	}
	target, err := s.resolveCreateTarget(ctx, req)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    taskCreateErrorCode(err),
			SourceID:     &req.SourceID,
		})
		return nil, err
	}
	source := target.source
	if source.DriverType != "local" {
		if _, err := s.getTaskImportDriver(source.DriverType); err != nil {
			recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
				ResourceType: "task",
				Action:       "create",
				Result:       appaudit.ResultFailed,
				ErrorCode:    "SOURCE_DRIVER_UNSUPPORTED",
				SourceID:     &source.ID,
				VirtualPath:  target.saveVirtualPath,
			})
			return nil, err
		}
	}
	if err := s.authorizeTaskPath(ctx, source.ID, target.savePath, ACLActionWrite); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultDenied,
			ErrorCode:    "ACL_DENIED",
			SourceID:     &source.ID,
			VirtualPath:  target.saveVirtualPath,
		})
		return nil, err
	}

	stagingDir := s.newTaskStagingDir()
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			SourceID:     &source.ID,
			VirtualPath:  target.saveVirtualPath,
		})
		return nil, err
	}

	externalID, err := s.downloader.AddURI(ctx, req.URL, stagingDir)
	if err != nil {
		_ = os.RemoveAll(stagingDir)
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			SourceID:     &source.ID,
			VirtualPath:  target.saveVirtualPath,
		})
		return nil, err
	}

	now := time.Now()
	displayName := guessTaskDisplayName(req.URL)
	task := &entity.DownloadTask{
		UserID:                  s.currentTaskUserID(ctx),
		Type:                    req.Type,
		Status:                  "pending",
		SourceID:                source.ID,
		SavePath:                target.savePath,
		TargetVirtualParentPath: target.targetVirtualParentPath,
		SaveVirtualPath:         target.saveVirtualPath,
		ResolvedSourceID:        target.resolvedSourceID,
		ResolvedInnerSavePath:   target.resolvedInnerSavePath,
		StagingDir:              stagingDir,
		DisplayName:             displayName,
		SourceURL:               req.URL,
		ExternalID:              externalID,
		Progress:                0,
		DownloadedBytes:         0,
		TotalBytes:              nil,
		SpeedBytes:              0,
		ETASeconds:              nil,
		ErrorMessage:            nil,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := s.taskRepo.Create(ctx, task); err != nil {
		_ = os.RemoveAll(stagingDir)
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			SourceID:     &source.ID,
			VirtualPath:  target.saveVirtualPath,
		})
		return nil, err
	}

	view := toTaskView(task)
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "task",
		Action:       "create",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(task.ID),
		SourceID:     &source.ID,
		VirtualPath:  target.saveVirtualPath,
		After:        taskAuditView(task),
	})
	return &view, nil
}

func (s *TaskService) resolveCreateTarget(ctx context.Context, req appdto.CreateTaskRequest) (resolvedTaskTarget, error) {
	if strings.TrimSpace(req.TargetVirtualParentPath) != "" {
		virtualParentPath, err := normalizeVirtualPath(req.TargetVirtualParentPath)
		if err != nil {
			return resolvedTaskTarget{}, err
		}

		probeName := guessTaskDisplayName(req.URL)
		if validateFileName(probeName) != nil {
			probeName = "download"
		}
		resolved, err := s.requireTaskVFSResolver().ResolveWritableTarget(ctx, joinVirtualPath(virtualParentPath, probeName))
		if err != nil {
			return resolvedTaskTarget{}, err
		}
		if resolved.Source == nil {
			return resolvedTaskTarget{}, ErrNoBackingStorage
		}
		resolvedInnerParentPath, _, err := splitParentName(resolved.InnerPath)
		if err != nil {
			return resolvedTaskTarget{}, err
		}
		return resolvedTaskTarget{
			source:                  resolved.Source,
			savePath:                resolvedInnerParentPath,
			targetVirtualParentPath: virtualParentPath,
			saveVirtualPath:         virtualParentPath,
			resolvedSourceID:        resolved.Source.ID,
			resolvedInnerSavePath:   resolvedInnerParentPath,
		}, nil
	}

	if req.SourceID == 0 || strings.TrimSpace(req.SavePath) == "" {
		return resolvedTaskTarget{}, ErrPathInvalid
	}
	source, err := s.sourceRepo.FindByID(ctx, req.SourceID)
	if err != nil {
		return resolvedTaskTarget{}, err
	}
	savePath, err := normalizeVirtualPath(req.SavePath)
	if err != nil {
		return resolvedTaskTarget{}, err
	}
	saveVirtualPath := mergeMountAndInnerPath(source.MountPath, savePath)
	if saveVirtualPath == "" {
		saveVirtualPath = savePath
	}
	return resolvedTaskTarget{
		source:                source,
		savePath:              savePath,
		saveVirtualPath:       saveVirtualPath,
		resolvedSourceID:      source.ID,
		resolvedInnerSavePath: savePath,
	}, nil
}

func (s *TaskService) requireTaskVFSResolver() interface {
	ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error)
} {
	if s.vfsResolver == nil {
		return unsupportedTaskVFSResolver{}
	}
	return s.vfsResolver
}

type unsupportedTaskVFSResolver struct{}

func (unsupportedTaskVFSResolver) ResolveWritableTarget(context.Context, string) (ResolvedPath, error) {
	return ResolvedPath{}, ErrSourceDriverUnsupported
}

func (s *TaskService) newTaskStagingDir() string {
	return filepath.Join(s.stagingRoot, "task_"+stringsNoDash(uuid.NewString()))
}

// Get 返回单个任务。
func (s *TaskService) Get(ctx context.Context, id uint) (*appdto.DownloadTaskView, error) {
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeTaskOwnership(ctx, task, false); err != nil {
		return nil, err
	}
	_ = s.refreshTask(ctx, task)
	view := toTaskView(task)
	return &view, nil
}

// SyncAll 主动同步所有未终止的离线下载任务状态。
func (s *TaskService) SyncAll(ctx context.Context) error {
	items, err := s.taskRepo.List(ctx)
	if err != nil {
		return err
	}

	var syncErr error
	for _, item := range items {
		if isTerminalTaskStatus(item.Status) {
			continue
		}
		if err := s.refreshTask(ctx, item); err != nil {
			s.logger.Warn("sync task failed", slog.String("event", "task.sync.failed"), slog.Uint64("task_id", uint64(item.ID)), slog.Any("error", err))
			syncErr = errors.Join(syncErr, err)
		}
	}
	return syncErr
}

// StartSyncWorker 定期同步下载器状态并在完成后导入目标存储源。
func (s *TaskService) StartSyncWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.SyncAll(ctx)
		}
	}
}

// Cancel 取消任务。
func (s *TaskService) Cancel(ctx context.Context, id uint, deleteFile bool) (*appdto.CancelTaskResponse, error) {
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "cancel",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "TASK_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return nil, err
	}
	if err := s.authorizeTaskOwnership(ctx, task, true); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "cancel",
			Result:       appaudit.ResultDenied,
			ErrorCode:    "PERMISSION_DENIED",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, err
	}
	if s.downloader == nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "cancel",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_DRIVER_UNSUPPORTED",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, ErrSourceDriverUnsupported
	}
	if task.ExternalID != "" {
		if err := s.downloader.Remove(ctx, task.ExternalID); err != nil {
			recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
				ResourceType: "task",
				Action:       "cancel",
				Result:       appaudit.ResultFailed,
				ErrorCode:    "INTERNAL_ERROR",
				ResourceID:   encodeUintID(id),
				Before:       taskAuditView(task),
			})
			return nil, err
		}
	}
	now := time.Now()
	before := taskAuditView(task)
	task.Status = "canceled"
	task.FinishedAt = &now
	task.UpdatedAt = now
	if err := s.taskRepo.Update(ctx, task); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "cancel",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "task",
		Action:       "cancel",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		Before:       before,
		After:        taskAuditView(task),
		Detail:       map[string]any{"delete_file": deleteFile},
	})
	return &appdto.CancelTaskResponse{ID: id, Canceled: true, DeleteFile: deleteFile}, nil
}

// Pause 暂停任务。
func (s *TaskService) Pause(ctx context.Context, id uint) (*appdto.TaskActionResponse, error) {
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "pause",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "TASK_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return nil, err
	}
	if err := s.authorizeTaskOwnership(ctx, task, true); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "pause",
			Result:       appaudit.ResultDenied,
			ErrorCode:    "PERMISSION_DENIED",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, err
	}
	if s.downloader == nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "pause",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_DRIVER_UNSUPPORTED",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, ErrSourceDriverUnsupported
	}
	if task.Status != "pending" && task.Status != "running" {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "pause",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "TASK_INVALID_STATE",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, ErrTaskInvalidState
	}
	if err := s.downloader.Pause(ctx, task.ExternalID); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "pause",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, err
	}
	before := taskAuditView(task)
	task.Status = "paused"
	task.UpdatedAt = time.Now()
	if err := s.taskRepo.Update(ctx, task); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "pause",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "task",
		Action:       "pause",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		Before:       before,
		After:        taskAuditView(task),
	})
	return &appdto.TaskActionResponse{ID: task.ID, Status: task.Status}, nil
}

// Resume 恢复任务。
func (s *TaskService) Resume(ctx context.Context, id uint) (*appdto.TaskActionResponse, error) {
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "resume",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "TASK_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return nil, err
	}
	if err := s.authorizeTaskOwnership(ctx, task, true); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "resume",
			Result:       appaudit.ResultDenied,
			ErrorCode:    "PERMISSION_DENIED",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, err
	}
	if s.downloader == nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "resume",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_DRIVER_UNSUPPORTED",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, ErrSourceDriverUnsupported
	}
	if task.Status != "paused" {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "resume",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "TASK_INVALID_STATE",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, ErrTaskInvalidState
	}
	if err := s.downloader.Resume(ctx, task.ExternalID); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "resume",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, err
	}
	before := taskAuditView(task)
	task.Status = "running"
	task.UpdatedAt = time.Now()
	if err := s.taskRepo.Update(ctx, task); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "resume",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "task",
		Action:       "resume",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		Before:       before,
		After:        taskAuditView(task),
	})
	return &appdto.TaskActionResponse{ID: task.ID, Status: task.Status}, nil
}

func (s *TaskService) refreshTask(ctx context.Context, task *entity.DownloadTask) error {
	if s.downloader == nil || task.ExternalID == "" {
		return nil
	}
	if task.Status == "completed" || task.Status == "failed" || task.Status == "canceled" {
		return nil
	}
	status, err := s.downloader.TellStatus(ctx, task.ExternalID)
	if err != nil {
		return err
	}
	task.Status = status.Status
	task.DownloadedBytes = status.CompletedBytes
	task.TotalBytes = status.TotalBytes
	task.SpeedBytes = status.DownloadSpeed
	task.ETASeconds = status.ETASeconds
	task.ErrorMessage = status.ErrorMessage
	if status.DisplayName != "" {
		task.DisplayName = status.DisplayName
	}
	if status.TotalBytes != nil && *status.TotalBytes > 0 {
		task.Progress = float64(status.CompletedBytes) * 100 / float64(*status.TotalBytes)
	}
	if status.Status == "completed" {
		task.SpeedBytes = 0
		task.ETASeconds = nil
		if err := s.importCompletedTask(ctx, task); err != nil {
			message := err.Error()
			task.Status = "failed"
			task.ErrorMessage = &message
			now := time.Now()
			task.FinishedAt = &now
		} else {
			task.ErrorMessage = nil
			now := time.Now()
			task.FinishedAt = &now
		}
	}
	task.UpdatedAt = time.Now()
	return s.taskRepo.Update(ctx, task)
}

type stagedTaskFile struct {
	localPath    string
	relativePath string
}

func (s *TaskService) importCompletedTask(ctx context.Context, task *entity.DownloadTask) error {
	if strings.TrimSpace(task.StagingDir) == "" {
		return nil
	}

	files, err := listStagedTaskFiles(task.StagingDir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return ErrFileNotFound
	}

	sourceID := task.ResolvedSourceID
	if sourceID == 0 {
		sourceID = task.SourceID
	}
	source, err := s.sourceRepo.FindByID(ctx, sourceID)
	if err != nil {
		return err
	}

	baseTargetPath := task.ResolvedInnerSavePath
	if strings.TrimSpace(baseTargetPath) == "" {
		baseTargetPath = task.SavePath
	}
	baseTargetPath, err = normalizeVirtualPath(baseTargetPath)
	if err != nil {
		return err
	}

	for _, file := range files {
		targetPath := joinVirtualPath(baseTargetPath, filepath.ToSlash(file.relativePath))
		if err := s.importStagedFile(ctx, source, targetPath, file.localPath); err != nil {
			return err
		}
	}

	return os.RemoveAll(task.StagingDir)
}

func listStagedTaskFiles(stagingDir string) ([]stagedTaskFile, error) {
	files := make([]stagedTaskFile, 0)
	err := filepath.WalkDir(stagingDir, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".aria2") {
			return nil
		}
		relativePath, err := filepath.Rel(stagingDir, current)
		if err != nil {
			return err
		}
		if relativePath == "." || relativePath == "" {
			return nil
		}
		files = append(files, stagedTaskFile{
			localPath:    current,
			relativePath: relativePath,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func (s *TaskService) importStagedFile(ctx context.Context, source *entity.StorageSource, targetPath string, localPath string) error {
	if source.DriverType == "local" {
		return importLocalStagedFile(source, targetPath, localPath)
	}

	driver, err := s.getTaskImportDriver(source.DriverType)
	if err != nil {
		return err
	}
	return driver.ImportFile(ctx, source, targetPath, localPath)
}

func importLocalStagedFile(source *entity.StorageSource, targetPath string, localPath string) error {
	_, physicalPath, err := resolvePhysicalPath(source, targetPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(physicalPath); err == nil {
		return ErrFileAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(physicalPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(localPath, physicalPath); err != nil {
		if copyErr := copyFile(localPath, physicalPath); copyErr != nil {
			return copyErr
		}
		return os.Remove(localPath)
	}
	return nil
}

func (s *TaskService) getTaskImportDriver(driverType string) (TaskImportDriver, error) {
	driver, exists := s.importDrivers[driverType]
	if !exists {
		return nil, ErrSourceDriverUnsupported
	}
	return driver, nil
}

func isTerminalTaskStatus(status string) bool {
	switch status {
	case "completed", "failed", "canceled":
		return true
	default:
		return false
	}
}

func toTaskView(task *entity.DownloadTask) appdto.DownloadTaskView {
	var finishedAt *string
	if task.FinishedAt != nil {
		formatted := task.FinishedAt.Format(time.RFC3339)
		finishedAt = &formatted
	}
	speedBytes := task.SpeedBytes
	etaSeconds := task.ETASeconds
	errorMessage := task.ErrorMessage
	if isTerminalTaskStatus(task.Status) {
		speedBytes = 0
		etaSeconds = nil
	}
	if task.Status == "completed" {
		errorMessage = nil
	}

	return appdto.DownloadTaskView{
		ID:                      task.ID,
		Type:                    task.Type,
		Status:                  task.Status,
		SourceID:                task.SourceID,
		SavePath:                task.SavePath,
		TargetVirtualParentPath: task.TargetVirtualParentPath,
		SaveVirtualPath:         task.SaveVirtualPath,
		ResolvedSourceID:        task.ResolvedSourceID,
		ResolvedInnerSavePath:   task.ResolvedInnerSavePath,
		DisplayName:             task.DisplayName,
		SourceURL:               task.SourceURL,
		Progress:                task.Progress,
		DownloadedBytes:         task.DownloadedBytes,
		TotalBytes:              task.TotalBytes,
		SpeedBytes:              speedBytes,
		ETASeconds:              etaSeconds,
		ErrorMessage:            errorMessage,
		CreatedAt:               task.CreatedAt.Format(time.RFC3339),
		UpdatedAt:               task.UpdatedAt.Format(time.RFC3339),
		FinishedAt:              finishedAt,
	}
}

func guessTaskDisplayName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || name == "" {
		return rawURL
	}
	return name
}

func taskCreateErrorCode(err error) string {
	if errors.Is(err, domainrepo.ErrNotFound) {
		return "SOURCE_NOT_FOUND"
	}
	return taskErrorCode(err)
}

func (s *TaskService) authorizeTaskPath(ctx context.Context, sourceID uint, savePath string, action ACLAction) error {
	if s.aclAuthorizer == nil {
		return nil
	}
	return s.aclAuthorizer.AuthorizePath(ctx, sourceID, savePath, action)
}

func (s *TaskService) currentTaskUserID(ctx context.Context) uint {
	auth, ok := security.RequestAuthFromContext(ctx)
	if !ok {
		return 0
	}
	return auth.UserID
}

func (s *TaskService) authorizeTaskOwnership(ctx context.Context, task *entity.DownloadTask, manage bool) error {
	auth, ok := security.RequestAuthFromContext(ctx)
	if !ok {
		return ErrPermissionDenied
	}
	allowed := permission.CanReadTask(auth.UserID, task.UserID, auth.Capabilities)
	if manage {
		allowed = permission.CanManageTask(auth.UserID, task.UserID, auth.Capabilities)
	}
	if !allowed {
		return ErrPermissionDenied
	}
	return nil
}
