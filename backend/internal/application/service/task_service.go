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
	Status          string
	CompletedBytes  int64
	TotalBytes      *int64
	DownloadSpeed   int64
	ETASeconds      *int64
	ProgressPercent *float64
	DisplayName     string
	ErrorMessage    *string
}

// TaskTerminalStatusObserver 接收任务终态变更通知，用于同步跨模块反向引用。
type TaskTerminalStatusObserver interface {
	OnTaskTerminalStatus(ctx context.Context, task *entity.DownloadTask) error
}

// TaskService 负责离线下载任务接口。
type TaskService struct {
	taskRepo               domainrepo.TaskRepository
	sourceRepo             domainrepo.SourceRepository
	aclAuthorizer          *ACLAuthorizer
	downloader             Downloader
	downloadRouter         *DownloaderRouter
	stagingRoot            string
	stagingRoots           map[string]string
	importDrivers          map[string]TaskImportDriver
	nativeDownloadDrivers  map[string]NativeDownloadDriver
	terminalStatusObserver TaskTerminalStatusObserver
	vfsResolver            interface {
		ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error)
	}
	metadataReader    metadataVFSReader
	metadataCommitter MetadataVFSCommitter
	operationJournal  vfsOperationJournalRecorder
	logger            *slog.Logger
	auditRecorder     *appaudit.Recorder
}

type resolvedTaskTarget struct {
	source                  *entity.StorageSource
	savePath                string
	targetVFSParentNodeID   uint
	targetVirtualParentPath string
	saveVirtualPath         string
	resolvedSourceID        uint
	resolvedInnerSavePath   string
}

const maxTaskTargetFilenameRunes = 240

// NewTaskService 创建任务服务。
func NewTaskService(taskRepo domainrepo.TaskRepository, sourceRepo domainrepo.SourceRepository, downloader Downloader, options ...TaskServiceOption) *TaskService {
	service := &TaskService{
		taskRepo:              taskRepo,
		sourceRepo:            sourceRepo,
		downloader:            downloader,
		downloadRouter:        NewDownloaderRouter(downloader),
		stagingRoot:           filepath.Join(os.TempDir(), "yunxia-download-staging"),
		stagingRoots:          make(map[string]string),
		importDrivers:         make(map[string]TaskImportDriver),
		nativeDownloadDrivers: make(map[string]NativeDownloadDriver),
		logger:                newServiceLogger("service.task"),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// SetTerminalStatusObserver 注册任务终态观察者。用于依赖后置装配的服务。
func (s *TaskService) SetTerminalStatusObserver(observer TaskTerminalStatusObserver) {
	s.terminalStatusObserver = observer
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
	targetFilename, err := normalizeTaskTargetFilename(req.TargetFilename)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "FILE_NAME_INVALID",
			SourceID:     &req.SourceID,
		})
		return nil, err
	}
	req.TargetFilename = targetFilename

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
	if view, handled, err := s.tryCreateNativeDownloadTask(ctx, req, target); handled || err != nil {
		return view, err
	}
	if source.DriverType != "local" {
		if _, err := s.getTaskImportDriver(source.DriverType); err != nil {
			if errors.Is(err, ErrSourceDriverUnsupported) {
				err = ErrSourceOperationUnsupported
			}
			recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
				ResourceType: "task",
				Action:       "create",
				Result:       appaudit.ResultFailed,
				ErrorCode:    taskCreateErrorCode(err),
				SourceID:     &source.ID,
				VirtualPath:  target.saveVirtualPath,
			})
			return nil, err
		}
	}
	downloaderType, selectedDownloader, err := s.selectDownloaderForURL(req.URL)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    taskCreateErrorCode(err),
			SourceID:     &source.ID,
			VirtualPath:  target.saveVirtualPath,
		})
		return nil, err
	}

	stagingDir := s.newTaskStagingDir(downloaderType)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    taskCreateErrorCode(err),
			SourceID:     &source.ID,
			VirtualPath:  target.saveVirtualPath,
		})
		return nil, err
	}

	externalID, err := selectedDownloader.AddURI(ctx, req.URL, stagingDir)
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
		DownloaderType:          downloaderType,
		Status:                  "pending",
		SourceID:                source.ID,
		SavePath:                target.savePath,
		TargetVFSParentNodeID:   target.targetVFSParentNodeID,
		TargetVirtualParentPath: target.targetVirtualParentPath,
		TargetFilename:          targetFilename,
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

func (s *TaskService) tryCreateNativeDownloadTask(ctx context.Context, req appdto.CreateTaskRequest, target resolvedTaskTarget) (*appdto.DownloadTaskView, bool, error) {
	source := target.source
	if source == nil || ClassifyDownloadLink(req.URL) == RSSLinkTypeUnsupported {
		return nil, false, nil
	}
	driver, exists := s.nativeDownloadDrivers[source.DriverType]
	if !exists || driver == nil {
		return nil, false, nil
	}

	nativeTask, err := driver.CreateNativeDownload(ctx, source, NativeDownloadRequest{
		URL:            req.URL,
		TargetDirPath:  target.savePath,
		TargetFilename: req.TargetFilename,
	})
	if err != nil {
		if errors.Is(err, ErrSourceOperationUnsupported) || errors.Is(err, ErrDownloadLinkUnsupported) {
			return nil, false, nil
		}
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    taskCreateErrorCode(err),
			SourceID:     &source.ID,
			VirtualPath:  target.saveVirtualPath,
		})
		return nil, true, err
	}
	if nativeTask == nil || strings.TrimSpace(nativeTask.ExternalID) == "" {
		err := ErrCloudProviderUnavailable
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    taskCreateErrorCode(err),
			SourceID:     &source.ID,
			VirtualPath:  target.saveVirtualPath,
		})
		return nil, true, err
	}

	now := time.Now()
	displayName := strings.TrimSpace(nativeTask.DisplayName)
	progress := float64(0)
	if nativeTask.ProgressPercent != nil {
		progress = clampTaskProgressPercent(*nativeTask.ProgressPercent)
	}
	task := &entity.DownloadTask{
		UserID:                  s.currentTaskUserID(ctx),
		Type:                    req.Type,
		DownloaderType:          DownloaderTypePikPakNative,
		Status:                  "pending",
		SourceID:                source.ID,
		SavePath:                target.savePath,
		TargetVFSParentNodeID:   target.targetVFSParentNodeID,
		TargetVirtualParentPath: target.targetVirtualParentPath,
		TargetFilename:          req.TargetFilename,
		SaveVirtualPath:         target.saveVirtualPath,
		ResolvedSourceID:        target.resolvedSourceID,
		ResolvedInnerSavePath:   target.resolvedInnerSavePath,
		StagingDir:              "",
		DisplayName:             displayName,
		SourceURL:               req.URL,
		ExternalID:              strings.TrimSpace(nativeTask.ExternalID),
		Progress:                progress,
		DownloadedBytes:         0,
		TotalBytes:              nil,
		SpeedBytes:              0,
		ETASeconds:              nil,
		ErrorMessage:            nil,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := s.taskRepo.Create(ctx, task); err != nil {
		_ = driver.CancelNativeDownload(ctx, source, task.ExternalID, false)
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			SourceID:     &source.ID,
			VirtualPath:  target.saveVirtualPath,
		})
		return nil, true, err
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
	return &view, true, nil
}

func (s *TaskService) resolveCreateTarget(ctx context.Context, req appdto.CreateTaskRequest) (resolvedTaskTarget, error) {
	if strings.TrimSpace(req.TargetVirtualParentPath) != "" {
		virtualParentPath, err := normalizeVirtualPath(req.TargetVirtualParentPath)
		if err != nil {
			return resolvedTaskTarget{}, err
		}

		probeName := taskTargetProbeName(req)
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
			targetVFSParentNodeID:   resolveMetadataVFSNodeID(ctx, s.metadataReader, virtualParentPath),
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
		targetVFSParentNodeID: resolveMetadataVFSNodeID(ctx, s.metadataReader, saveVirtualPath),
		saveVirtualPath:       saveVirtualPath,
		resolvedSourceID:      source.ID,
		resolvedInnerSavePath: savePath,
	}, nil
}

func taskTargetProbeName(req appdto.CreateTaskRequest) string {
	if strings.TrimSpace(req.TargetFilename) != "" {
		return req.TargetFilename
	}
	probeName := guessTaskDisplayName(req.URL)
	if validateFileName(probeName) != nil {
		return "download"
	}
	return probeName
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

func (s *TaskService) newTaskStagingDir(downloaderType string) string {
	stagingRoot := s.stagingRoot
	if s.stagingRoots != nil {
		if typedRoot := strings.TrimSpace(s.stagingRoots[downloaderType]); typedRoot != "" {
			stagingRoot = typedRoot
		}
	}
	return filepath.Join(stagingRoot, "task_"+stringsNoDash(uuid.NewString()))
}

func (s *TaskService) selectDownloaderForURL(rawURL string) (string, Downloader, error) {
	if s.downloadRouter != nil {
		return s.downloadRouter.Select(rawURL)
	}
	if ClassifyDownloadLink(rawURL) != RSSLinkTypeHTTP {
		return "", nil, ErrDownloadLinkUnsupported
	}
	if s.downloader == nil {
		return "", nil, ErrSourceDriverUnsupported
	}
	return DownloaderTypeAria2, s.downloader, nil
}

func (s *TaskService) downloaderForTask(task *entity.DownloadTask) (Downloader, error) {
	if task == nil {
		return nil, ErrTaskInvalidState
	}
	downloaderType := task.DownloaderType
	if downloaderType == "" {
		downloaderType = DownloaderTypeAria2
	}
	if downloaderType == DownloaderTypePikPakNative {
		return nativeTaskDownloader{service: s, sourceID: taskResolvedSourceID(task)}, nil
	}
	if s.downloadRouter != nil {
		return s.downloadRouter.Get(downloaderType)
	}
	if downloaderType != DownloaderTypeAria2 {
		return nil, ErrSourceDriverUnsupported
	}
	if s.downloader == nil {
		return nil, ErrSourceDriverUnsupported
	}
	return s.downloader, nil
}

type nativeTaskDownloader struct {
	service  *TaskService
	sourceID uint
}

func (d nativeTaskDownloader) AddURI(context.Context, string, string) (string, error) {
	return "", ErrSourceOperationUnsupported
}

func (d nativeTaskDownloader) TellStatus(ctx context.Context, externalID string) (*DownloadStatus, error) {
	source, driver, err := d.resolve(ctx)
	if err != nil {
		return nil, err
	}
	status, err := driver.GetNativeDownloadStatus(ctx, source, externalID)
	if err != nil {
		return nil, err
	}
	return downloadStatusFromNative(status), nil
}

func (d nativeTaskDownloader) Pause(ctx context.Context, externalID string) error {
	source, driver, err := d.resolve(ctx)
	if err != nil {
		return err
	}
	return driver.PauseNativeDownload(ctx, source, externalID)
}

func (d nativeTaskDownloader) Resume(ctx context.Context, externalID string) error {
	source, driver, err := d.resolve(ctx)
	if err != nil {
		return err
	}
	return driver.ResumeNativeDownload(ctx, source, externalID)
}

func (d nativeTaskDownloader) Remove(ctx context.Context, externalID string) error {
	source, driver, err := d.resolve(ctx)
	if err != nil {
		return err
	}
	return driver.CancelNativeDownload(ctx, source, externalID, false)
}

func (d nativeTaskDownloader) resolve(ctx context.Context) (*entity.StorageSource, NativeDownloadDriver, error) {
	if d.service == nil || d.service.sourceRepo == nil || d.sourceID == 0 {
		return nil, nil, ErrSourceDriverUnsupported
	}
	source, err := d.service.sourceRepo.FindByID(ctx, d.sourceID)
	if err != nil {
		return nil, nil, err
	}
	driver, exists := d.service.nativeDownloadDrivers[source.DriverType]
	if !exists || driver == nil {
		return nil, nil, ErrSourceDriverUnsupported
	}
	return source, driver, nil
}

func downloadStatusFromNative(status *NativeDownloadStatus) *DownloadStatus {
	if status == nil {
		return &DownloadStatus{Status: "pending"}
	}
	progress := status.ProgressPercent
	if progress != nil {
		clamped := clampTaskProgressPercent(*progress)
		progress = &clamped
	}
	if status.Status == "completed" && progress == nil {
		value := float64(100)
		progress = &value
	}
	return &DownloadStatus{
		Status:          status.Status,
		CompletedBytes:  status.CompletedBytes,
		TotalBytes:      status.TotalBytes,
		DownloadSpeed:   status.DownloadSpeed,
		ETASeconds:      status.ETASeconds,
		ProgressPercent: progress,
		DisplayName:     status.DisplayName,
		ErrorMessage:    status.ErrorMessage,
	}
}

func taskResolvedSourceID(task *entity.DownloadTask) uint {
	if task == nil {
		return 0
	}
	if task.ResolvedSourceID != 0 {
		return task.ResolvedSourceID
	}
	return task.SourceID
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
	if isTerminalTaskStatus(task.Status) {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "cancel",
			Result:       appaudit.ResultSuccess,
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
			After:        taskAuditView(task),
			Detail:       map[string]any{"delete_file": deleteFile, "idempotent": true},
		})
		return &appdto.CancelTaskResponse{ID: id, Canceled: task.Status == "canceled", DeleteFile: deleteFile}, nil
	}

	now := time.Now()
	before := taskAuditView(task)
	task.Status = "canceled"
	message := "download canceled by user"
	task.ErrorMessage = &message
	task.SpeedBytes = 0
	task.ETASeconds = nil
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
	if task.ExternalID != "" {
		if task.DownloaderType == DownloaderTypePikPakNative {
			if err := s.cancelNativeDownloadTask(ctx, task, deleteFile); err != nil {
				s.logger.Warn("task cancel native downloader remove failed after local cancellation", slog.String("event", "task.cancel.native_remove_failed"), slog.Uint64("task_id", uint64(task.ID)), slog.String("downloader_type", task.DownloaderType), slog.Any("error", err))
			}
		} else if selectedDownloader, err := s.downloaderForTask(task); err != nil {
			s.logger.Warn("task cancel downloader unavailable after local cancellation", slog.String("event", "task.cancel.downloader_unavailable"), slog.Uint64("task_id", uint64(task.ID)), slog.String("downloader_type", task.DownloaderType), slog.Any("error", err))
		} else if err := selectedDownloader.Remove(ctx, task.ExternalID); err != nil {
			s.logger.Warn("task cancel downloader remove failed after local cancellation", slog.String("event", "task.cancel.downloader_remove_failed"), slog.Uint64("task_id", uint64(task.ID)), slog.String("downloader_type", task.DownloaderType), slog.Any("error", err))
		}
	}
	s.notifyTaskTerminalStatus(ctx, task)
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

func (s *TaskService) cancelNativeDownloadTask(ctx context.Context, task *entity.DownloadTask, deleteFile bool) error {
	if task == nil {
		return ErrTaskInvalidState
	}
	source, err := s.sourceRepo.FindByID(ctx, taskResolvedSourceID(task))
	if err != nil {
		return err
	}
	driver, exists := s.nativeDownloadDrivers[source.DriverType]
	if !exists || driver == nil {
		return ErrSourceDriverUnsupported
	}
	return driver.CancelNativeDownload(ctx, source, task.ExternalID, deleteFile)
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
	selectedDownloader, err := s.downloaderForTask(task)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "pause",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_DRIVER_UNSUPPORTED",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, err
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
	if err := selectedDownloader.Pause(ctx, task.ExternalID); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "pause",
			Result:       appaudit.ResultFailed,
			ErrorCode:    taskErrorCode(err),
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
	selectedDownloader, err := s.downloaderForTask(task)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "resume",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_DRIVER_UNSUPPORTED",
			ResourceID:   encodeUintID(id),
			Before:       taskAuditView(task),
		})
		return nil, err
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
	if err := selectedDownloader.Resume(ctx, task.ExternalID); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "task",
			Action:       "resume",
			Result:       appaudit.ResultFailed,
			ErrorCode:    taskErrorCode(err),
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
	if task.ExternalID == "" {
		return nil
	}
	if task.Status == "completed" || task.Status == "failed" || task.Status == "canceled" {
		return nil
	}
	selectedDownloader, err := s.downloaderForTask(task)
	if err != nil {
		return err
	}
	status, err := selectedDownloader.TellStatus(ctx, task.ExternalID)
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
	if status.ProgressPercent != nil {
		task.Progress = clampTaskProgressPercent(*status.ProgressPercent)
	}
	if status.Status == "failed" {
		if task.ErrorMessage == nil || strings.TrimSpace(*task.ErrorMessage) == "" {
			message := "download failed"
			task.ErrorMessage = &message
		}
		task.SpeedBytes = 0
		task.ETASeconds = nil
		now := time.Now()
		task.FinishedAt = &now
	}
	if status.Status == "canceled" {
		if task.ErrorMessage == nil || strings.TrimSpace(*task.ErrorMessage) == "" {
			message := "download canceled by downloader"
			task.ErrorMessage = &message
		}
		task.SpeedBytes = 0
		task.ETASeconds = nil
		now := time.Now()
		task.FinishedAt = &now
	}
	if status.Status == "completed" {
		task.SpeedBytes = 0
		task.ETASeconds = nil
		if task.Progress < 100 {
			task.Progress = 100
		}
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
	if err := s.taskRepo.Update(ctx, task); err != nil {
		return err
	}
	s.notifyTaskTerminalStatus(ctx, task)
	return nil
}

type stagedTaskFile struct {
	localPath    string
	relativePath string
}

func (s *TaskService) importCompletedTask(ctx context.Context, task *entity.DownloadTask) error {
	if strings.TrimSpace(task.StagingDir) == "" {
		return s.commitCompletedTaskWithoutStaging(ctx, task)
	}

	files, err := listStagedTaskFiles(task.StagingDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			imported, nodeID, existsErr := s.completedLocalTaskTargetExists(ctx, task)
			if existsErr != nil {
				return existsErr
			}
			if imported {
				task.StagingDir = ""
				if nodeID > 0 {
					task.ResultVFSNodeID = nodeID
				}
				return nil
			}
		}
		return err
	}
	if len(files) == 0 {
		imported, nodeID, existsErr := s.completedLocalTaskTargetExists(ctx, task)
		if existsErr != nil {
			return existsErr
		}
		if imported {
			_ = os.RemoveAll(task.StagingDir)
			task.StagingDir = ""
			if nodeID > 0 {
				task.ResultVFSNodeID = nodeID
			}
			return nil
		}
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
		targetPath := joinVirtualPath(baseTargetPath, taskImportRelativePath(task, file, len(files)))
		info, err := os.Stat(file.localPath)
		if err != nil {
			return err
		}
		if err := s.importStagedFile(ctx, source, targetPath, file.localPath); err != nil {
			if errors.Is(err, ErrFileAlreadyExists) {
				if recovered, nodeID, recoverErr := s.recoverCompletedTaskResult(ctx, task, source, targetPath); recoverErr != nil {
					return recoverErr
				} else if recovered {
					_ = os.Remove(file.localPath)
					if nodeID > 0 && task.ResultVFSNodeID == 0 {
						task.ResultVFSNodeID = nodeID
					}
					continue
				}
			}
			return err
		}
		nodeID, err := s.commitTaskImportedMetadataFile(ctx, task, source, targetPath, info)
		if err != nil {
			if recovered, recoveredNodeID, recoverErr := s.recoverCompletedTaskResult(ctx, task, source, targetPath); recoverErr != nil {
				return recoverErr
			} else if recovered {
				if recoveredNodeID > 0 && task.ResultVFSNodeID == 0 {
					task.ResultVFSNodeID = recoveredNodeID
				}
				continue
			}
			return err
		}
		if nodeID > 0 && task.ResultVFSNodeID == 0 {
			task.ResultVFSNodeID = nodeID
		}
	}

	if s.metadataCommitter != nil && task.ResultVFSNodeID == 0 {
		err := ErrMetadataVFSCommitFailed
		s.recordTaskMetadataCommitFailure(ctx, task, source, "", task.SaveVirtualPath, err)
		return err
	}

	if err := os.RemoveAll(task.StagingDir); err != nil {
		return err
	}
	task.StagingDir = ""
	return nil
}

func (s *TaskService) commitTaskImportedMetadataFile(ctx context.Context, task *entity.DownloadTask, source *entity.StorageSource, targetPath string, info os.FileInfo) (uint, error) {
	if info == nil {
		return 0, nil
	}
	return s.commitTaskMetadataFileObject(ctx, task, source, targetPath, info.Size(), detectContentType(path.Base(targetPath)))
}

func (s *TaskService) commitTaskMetadataFileObject(ctx context.Context, task *entity.DownloadTask, source *entity.StorageSource, targetPath string, size int64, mimeType string) (uint, error) {
	if s.metadataCommitter == nil {
		return 0, nil
	}
	if task == nil || source == nil {
		return 0, nil
	}
	innerParentPath, filename, err := splitParentName(targetPath)
	if err != nil {
		return 0, err
	}
	actorID := task.UserID
	result, err := s.metadataCommitter.CommitFileObject(ctx, MetadataVFSFileObjectCommitRequest{
		Source:                  source,
		VirtualParentPath:       taskImportedMetadataVirtualParentPath(task, source, innerParentPath),
		ResolvedInnerParentPath: innerParentPath,
		Filename:                filename,
		ObjectPath:              targetPath,
		Size:                    size,
		MimeType:                mimeType,
		ActorID:                 &actorID,
	})
	if err != nil {
		s.recordTaskMetadataCommitFailure(ctx, task, source, targetPath, taskImportedMetadataVirtualParentPath(task, source, innerParentPath), err)
		return 0, maskMetadataVFSCommitError(err)
	}
	if result == nil || result.Node == nil {
		err := ErrMetadataVFSCommitFailed
		s.recordTaskMetadataCommitFailure(ctx, task, source, targetPath, taskImportedMetadataVirtualParentPath(task, source, innerParentPath), err)
		return 0, maskMetadataVFSCommitError(err)
	}
	return result.Node.ID, nil
}

func (s *TaskService) recoverCompletedTaskResult(ctx context.Context, task *entity.DownloadTask, source *entity.StorageSource, targetPath string) (bool, uint, error) {
	if task == nil || source == nil || strings.TrimSpace(targetPath) == "" {
		return false, 0, nil
	}
	if source.DriverType != "local" {
		return false, 0, nil
	}
	_, physicalPath, err := resolvePhysicalPath(source, targetPath)
	if err != nil {
		return false, 0, err
	}
	info, err := os.Stat(physicalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, 0, nil
		}
		return false, 0, err
	}
	if info.IsDir() {
		return false, 0, nil
	}
	if nodeID, ok := s.resolveExistingTaskResultNode(ctx, task, source, targetPath); ok {
		return true, nodeID, nil
	}
	nodeID, err := s.commitTaskImportedMetadataFile(ctx, task, source, targetPath, info)
	if err != nil {
		if existingID, ok := s.resolveExistingTaskResultNode(ctx, task, source, targetPath); ok {
			return true, existingID, nil
		}
		return false, 0, err
	}
	return true, nodeID, nil
}

func (s *TaskService) resolveExistingTaskResultNode(ctx context.Context, task *entity.DownloadTask, source *entity.StorageSource, targetPath string) (uint, bool) {
	if s.metadataReader == nil || task == nil || source == nil {
		return 0, false
	}
	innerParentPath, filename, err := splitParentName(targetPath)
	if err != nil || filename == "" {
		return 0, false
	}
	virtualParentPath := taskImportedMetadataVirtualParentPath(task, source, innerParentPath)
	if strings.TrimSpace(virtualParentPath) == "" {
		return 0, false
	}
	node, err := s.metadataReader.ResolveNode(ctx, joinVirtualPath(virtualParentPath, filename))
	if err != nil || metadataVFSNodeUnavailable(node) || node.Kind != entity.VFSNodeKindFile {
		return 0, false
	}
	if node.SourceID == nil || *node.SourceID != source.ID {
		return 0, false
	}
	return node.ID, true
}

func (s *TaskService) recordTaskMetadataCommitFailure(ctx context.Context, task *entity.DownloadTask, source *entity.StorageSource, objectPath string, virtualParentPath string, err error) {
	if s == nil || s.operationJournal == nil || task == nil || source == nil || err == nil {
		return
	}
	sourceID := source.ID
	input := VFSOperationRecordInput{
		OperationType:      entity.VFSOperationTypeTaskCommit,
		TargetParentNodeID: optionalUintPtr(task.TargetVFSParentNodeID),
		SourcePathSnapshot: objectPath,
		TargetPathSnapshot: taskCommitTargetPathSnapshot(task, virtualParentPath, objectPath),
		SourceIDSnapshot:   &sourceID,
		DriverTypeSnapshot: source.DriverType,
		ErrorCode:          "METADATA_VFS_COMMIT_FAILED",
		Error:              err,
		CreatedBy:          optionalUintPtr(task.UserID),
		Payload: map[string]any{
			"phase":           "metadata_commit",
			"task_id":         task.ID,
			"downloader_type": task.DownloaderType,
			"status":          task.Status,
		},
	}
	_, _ = s.operationJournal.RecordFailure(ctx, input)
}

func taskCommitTargetPathSnapshot(task *entity.DownloadTask, virtualParentPath string, objectPath string) string {
	if task == nil {
		return ""
	}
	parent := strings.TrimSpace(virtualParentPath)
	if parent == "" {
		parent = strings.TrimSpace(task.SaveVirtualPath)
	}
	if parent == "" {
		return objectPath
	}
	if strings.TrimSpace(objectPath) == "" {
		return parent
	}
	_, filename, err := splitParentName(objectPath)
	if err != nil || filename == "" {
		return parent
	}
	return joinVirtualPath(parent, filename)
}

func taskImportedMetadataVirtualParentPath(task *entity.DownloadTask, source *entity.StorageSource, innerParentPath string) string {
	if task == nil || source == nil {
		return ""
	}
	virtualBase := strings.TrimSpace(task.TargetVirtualParentPath)
	if virtualBase == "" {
		virtualBase = strings.TrimSpace(task.SaveVirtualPath)
	}
	if virtualBase != "" {
		normalizedVirtualBase, virtualErr := normalizeVirtualPath(virtualBase)
		normalizedInnerParent, innerErr := normalizeVirtualPath(innerParentPath)
		baseInner := strings.TrimSpace(task.ResolvedInnerSavePath)
		if baseInner == "" {
			baseInner = task.SavePath
		}
		normalizedBaseInner, baseErr := normalizeVirtualPath(baseInner)
		if virtualErr == nil && innerErr == nil && baseErr == nil && isSubPath(normalizedBaseInner, normalizedInnerParent) {
			if normalizedInnerParent == normalizedBaseInner {
				return normalizedVirtualBase
			}
			suffix := strings.TrimPrefix(normalizedInnerParent, normalizedBaseInner)
			return joinVirtualPath(normalizedVirtualBase, strings.TrimPrefix(suffix, "/"))
		}
	}
	return mergeMountAndInnerPath(source.MountPath, innerParentPath)
}

func taskImportRelativePath(task *entity.DownloadTask, file stagedTaskFile, fileCount int) string {
	if task == nil || fileCount != 1 || strings.TrimSpace(task.TargetFilename) == "" {
		return filepath.ToSlash(file.relativePath)
	}
	return taskTargetFilenameWithOriginalExtension(task.TargetFilename, file.relativePath)
}

func normalizeTaskTargetFilename(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", nil
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "..") || hasRSSWindowsDrivePrefix(trimmed) {
		return "", ErrFileNameInvalid
	}
	cleaned := sanitizeRSSPathSegment(trimmed)
	if cleaned == "" || strings.Contains(cleaned, "..") || validateFileName(cleaned) != nil {
		return "", ErrFileNameInvalid
	}
	return truncateTaskFilename(cleaned, maxTaskTargetFilenameRunes), nil
}

func taskTargetFilenameWithOriginalExtension(targetFilename string, originalRelativePath string) string {
	filename := truncateTaskFilename(strings.TrimSpace(targetFilename), maxTaskTargetFilenameRunes)
	if filename == "" || taskFilenameHasExplicitExtension(filename) {
		return filename
	}
	originalName := path.Base(filepath.ToSlash(originalRelativePath))
	extension := path.Ext(originalName)
	if !isSafeTaskFilenameExtension(extension) {
		return filename
	}
	baseLimit := maxTaskTargetFilenameRunes - len([]rune(extension))
	if baseLimit < 1 {
		baseLimit = maxTaskTargetFilenameRunes
	}
	return truncateTaskFilename(filename, baseLimit) + extension
}

func taskFilenameHasExplicitExtension(filename string) bool {
	return isSafeTaskFilenameExtension(path.Ext(filename))
}

func isSafeTaskFilenameExtension(extension string) bool {
	if len(extension) < 2 || len(extension) > 16 {
		return false
	}
	hasLetter := false
	for _, r := range extension[1:] {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			hasLetter = true
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return hasLetter
}

func truncateTaskFilename(filename string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(filename))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return strings.TrimSpace(string(runes[:maxRunes]))
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

func (s *TaskService) commitCompletedTaskWithoutStaging(ctx context.Context, task *entity.DownloadTask) error {
	if task == nil || task.ResultVFSNodeID > 0 || s.metadataCommitter == nil {
		return nil
	}
	sourceID := task.ResolvedSourceID
	if sourceID == 0 {
		sourceID = task.SourceID
	}
	source, err := s.sourceRepo.FindByID(ctx, sourceID)
	if err != nil {
		return err
	}
	if source.DriverType == "local" {
		imported, nodeID, err := s.completedLocalTaskTargetExists(ctx, task)
		if err != nil {
			return err
		}
		if imported {
			task.ResultVFSNodeID = nodeID
			return nil
		}
		return ErrFileNotFound
	}
	if task.DownloaderType != DownloaderTypePikPakNative {
		err := ErrMetadataVFSCommitFailed
		s.recordTaskMetadataCommitFailure(ctx, task, source, "", task.SaveVirtualPath, err)
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
	filename, err := completedTaskResultFilename(task)
	if err != nil {
		s.recordTaskMetadataCommitFailure(ctx, task, source, "", task.SaveVirtualPath, err)
		return err
	}
	targetPath := joinVirtualPath(baseTargetPath, filename)
	nodeID, err := s.commitTaskMetadataFileObject(ctx, task, source, targetPath, completedTaskResultSize(task), detectContentType(filename))
	if err != nil {
		return err
	}
	if nodeID == 0 {
		err := ErrMetadataVFSCommitFailed
		s.recordTaskMetadataCommitFailure(ctx, task, source, targetPath, taskImportedMetadataVirtualParentPath(task, source, baseTargetPath), err)
		return err
	}
	task.ResultVFSNodeID = nodeID
	return nil
}

func (s *TaskService) completedLocalTaskTargetExists(ctx context.Context, task *entity.DownloadTask) (bool, uint, error) {
	if task == nil {
		return false, 0, nil
	}
	sourceID := task.ResolvedSourceID
	if sourceID == 0 {
		sourceID = task.SourceID
	}
	source, err := s.sourceRepo.FindByID(ctx, sourceID)
	if err != nil {
		return false, 0, err
	}
	if source.DriverType != "local" {
		return false, 0, nil
	}

	baseTargetPath := task.ResolvedInnerSavePath
	if strings.TrimSpace(baseTargetPath) == "" {
		baseTargetPath = task.SavePath
	}
	baseTargetPath, err = normalizeVirtualPath(baseTargetPath)
	if err != nil {
		return false, 0, err
	}

	for _, relativePath := range completedTaskSingleFileCandidateRelativePaths(task) {
		targetPath := joinVirtualPath(baseTargetPath, relativePath)
		_, physicalPath, err := resolvePhysicalPath(source, targetPath)
		if err != nil {
			return false, 0, err
		}
		info, err := os.Stat(physicalPath)
		if err == nil && !info.IsDir() {
			nodeID, commitErr := s.commitTaskImportedMetadataFile(ctx, task, source, targetPath, info)
			if commitErr != nil {
				return false, 0, commitErr
			}
			return true, nodeID, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, 0, err
		}
	}
	return false, 0, nil
}

func completedTaskResultFilename(task *entity.DownloadTask) (string, error) {
	for _, candidate := range completedTaskSingleFileCandidateRelativePaths(task) {
		if validateFileName(candidate) == nil {
			return candidate, nil
		}
	}
	return "", ErrMetadataVFSCommitFailed
}

func completedTaskResultSize(task *entity.DownloadTask) int64 {
	if task == nil {
		return 0
	}
	if task.TotalBytes != nil && *task.TotalBytes > 0 {
		return *task.TotalBytes
	}
	if task.DownloadedBytes > 0 {
		return task.DownloadedBytes
	}
	return 0
}

func completedTaskSingleFileCandidateRelativePaths(task *entity.DownloadTask) []string {
	if task == nil {
		return nil
	}
	candidates := make([]string, 0, 4)
	add := func(value string) {
		value = strings.TrimSpace(filepath.ToSlash(value))
		if value == "" || value == "." || strings.Contains(value, "/") || strings.Contains(value, "\\") {
			return
		}
		for _, existing := range candidates {
			if existing == value {
				return
			}
		}
		candidates = append(candidates, value)
	}

	displayName := strings.TrimSpace(task.DisplayName)
	if strings.TrimSpace(task.TargetFilename) != "" {
		add(taskTargetFilenameWithOriginalExtension(task.TargetFilename, displayName))
		add(task.TargetFilename)
	}
	add(displayName)
	if task.DownloaderType != DownloaderTypePikPakNative {
		add(guessTaskDisplayName(task.SourceURL))
	}
	return candidates
}

func (s *TaskService) importStagedFile(ctx context.Context, source *entity.StorageSource, targetPath string, localPath string) error {
	if source.DriverType == "local" {
		return importLocalStagedFile(source, targetPath, localPath)
	}

	driver, err := s.getTaskImportDriver(source.DriverType)
	if err != nil {
		if errors.Is(err, ErrSourceDriverUnsupported) {
			return ErrSourceOperationUnsupported
		}
		return err
	}
	return normalizeImportDriverError(driver.ImportFile(ctx, source, targetPath, localPath))
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

func (s *TaskService) notifyTaskTerminalStatus(ctx context.Context, task *entity.DownloadTask) {
	if s.terminalStatusObserver == nil || task == nil || !isTerminalTaskStatus(task.Status) {
		return
	}
	if err := s.terminalStatusObserver.OnTaskTerminalStatus(ctx, task); err != nil {
		s.logger.Warn("task terminal observer failed", slog.String("event", "task.terminal_observer.failed"), slog.Uint64("task_id", uint64(task.ID)), slog.String("status", task.Status), slog.Any("error", err))
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
	} else if task.Status == "failed" && (errorMessage == nil || strings.TrimSpace(*errorMessage) == "") {
		message := "download failed"
		errorMessage = &message
	} else if task.Status == "canceled" && (errorMessage == nil || strings.TrimSpace(*errorMessage) == "") {
		message := "download canceled"
		errorMessage = &message
	}

	return appdto.DownloadTaskView{
		ID:                      task.ID,
		Type:                    task.Type,
		DownloaderType:          task.DownloaderType,
		Status:                  task.Status,
		SourceID:                task.SourceID,
		SavePath:                task.SavePath,
		TargetVFSParentNodeID:   task.TargetVFSParentNodeID,
		TargetVirtualParentPath: task.TargetVirtualParentPath,
		TargetFilename:          task.TargetFilename,
		SaveVirtualPath:         task.SaveVirtualPath,
		ResolvedSourceID:        task.ResolvedSourceID,
		ResolvedInnerSavePath:   task.ResolvedInnerSavePath,
		ResultVFSNodeID:         task.ResultVFSNodeID,
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

func clampTaskProgressPercent(progress float64) float64 {
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
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
