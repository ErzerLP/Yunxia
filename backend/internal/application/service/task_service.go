package service

import (
	"context"
	"net/url"
	"path"
	"time"

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

// TaskService 负责离线下载任务接口。
type TaskService struct {
	taskRepo      domainrepo.TaskRepository
	sourceRepo    domainrepo.SourceRepository
	aclAuthorizer *ACLAuthorizer
	downloader    Downloader
}

// NewTaskService 创建任务服务。
func NewTaskService(taskRepo domainrepo.TaskRepository, sourceRepo domainrepo.SourceRepository, downloader Downloader, options ...TaskServiceOption) *TaskService {
	service := &TaskService{
		taskRepo:   taskRepo,
		sourceRepo: sourceRepo,
		downloader: downloader,
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
		return nil, ErrSourceDriverUnsupported
	}
	source, err := s.sourceRepo.FindByID(ctx, req.SourceID)
	if err != nil {
		return nil, err
	}
	if source.DriverType != "local" {
		return nil, ErrSourceDriverUnsupported
	}
	savePath, err := normalizeVirtualPath(req.SavePath)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeTaskPath(ctx, req.SourceID, savePath, ACLActionWrite); err != nil {
		return nil, err
	}

	externalID, err := s.downloader.AddURI(ctx, req.URL, savePath)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	displayName := guessTaskDisplayName(req.URL)
	saveVirtualPath := mergeMountAndInnerPath(source.MountPath, savePath)
	if saveVirtualPath == "" {
		saveVirtualPath = savePath
	}
	task := &entity.DownloadTask{
		UserID:                s.currentTaskUserID(ctx),
		Type:                  req.Type,
		Status:                "pending",
		SourceID:              req.SourceID,
		SavePath:              savePath,
		SaveVirtualPath:       saveVirtualPath,
		ResolvedSourceID:      source.ID,
		ResolvedInnerSavePath: savePath,
		DisplayName:           displayName,
		SourceURL:             req.URL,
		ExternalID:            externalID,
		Progress:              0,
		DownloadedBytes:       0,
		TotalBytes:            nil,
		SpeedBytes:            0,
		ETASeconds:            nil,
		ErrorMessage:          nil,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := s.taskRepo.Create(ctx, task); err != nil {
		return nil, err
	}

	view := toTaskView(task)
	return &view, nil
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

// Cancel 取消任务。
func (s *TaskService) Cancel(ctx context.Context, id uint, deleteFile bool) (*appdto.CancelTaskResponse, error) {
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeTaskOwnership(ctx, task, true); err != nil {
		return nil, err
	}
	if s.downloader == nil {
		return nil, ErrSourceDriverUnsupported
	}
	if task.ExternalID != "" {
		if err := s.downloader.Remove(ctx, task.ExternalID); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	task.Status = "canceled"
	task.FinishedAt = &now
	task.UpdatedAt = now
	if err := s.taskRepo.Update(ctx, task); err != nil {
		return nil, err
	}
	return &appdto.CancelTaskResponse{ID: id, Canceled: true, DeleteFile: deleteFile}, nil
}

// Pause 暂停任务。
func (s *TaskService) Pause(ctx context.Context, id uint) (*appdto.TaskActionResponse, error) {
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeTaskOwnership(ctx, task, true); err != nil {
		return nil, err
	}
	if s.downloader == nil {
		return nil, ErrSourceDriverUnsupported
	}
	if task.Status != "pending" && task.Status != "running" {
		return nil, ErrTaskInvalidState
	}
	if err := s.downloader.Pause(ctx, task.ExternalID); err != nil {
		return nil, err
	}
	task.Status = "paused"
	task.UpdatedAt = time.Now()
	if err := s.taskRepo.Update(ctx, task); err != nil {
		return nil, err
	}
	return &appdto.TaskActionResponse{ID: task.ID, Status: task.Status}, nil
}

// Resume 恢复任务。
func (s *TaskService) Resume(ctx context.Context, id uint) (*appdto.TaskActionResponse, error) {
	task, err := s.taskRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeTaskOwnership(ctx, task, true); err != nil {
		return nil, err
	}
	if s.downloader == nil {
		return nil, ErrSourceDriverUnsupported
	}
	if task.Status != "paused" {
		return nil, ErrTaskInvalidState
	}
	if err := s.downloader.Resume(ctx, task.ExternalID); err != nil {
		return nil, err
	}
	task.Status = "running"
	task.UpdatedAt = time.Now()
	if err := s.taskRepo.Update(ctx, task); err != nil {
		return nil, err
	}
	return &appdto.TaskActionResponse{ID: task.ID, Status: task.Status}, nil
}

func (s *TaskService) refreshTask(ctx context.Context, task *entity.DownloadTask) error {
	if s.downloader == nil || task.ExternalID == "" {
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
		now := time.Now()
		task.FinishedAt = &now
	}
	task.UpdatedAt = time.Now()
	return s.taskRepo.Update(ctx, task)
}

func toTaskView(task *entity.DownloadTask) appdto.DownloadTaskView {
	var finishedAt *string
	if task.FinishedAt != nil {
		formatted := task.FinishedAt.Format(time.RFC3339)
		finishedAt = &formatted
	}

	return appdto.DownloadTaskView{
		ID:                    task.ID,
		Type:                  task.Type,
		Status:                task.Status,
		SourceID:              task.SourceID,
		SavePath:              task.SavePath,
		SaveVirtualPath:       task.SaveVirtualPath,
		ResolvedSourceID:      task.ResolvedSourceID,
		ResolvedInnerSavePath: task.ResolvedInnerSavePath,
		DisplayName:           task.DisplayName,
		SourceURL:             task.SourceURL,
		Progress:              task.Progress,
		DownloadedBytes:       task.DownloadedBytes,
		TotalBytes:            task.TotalBytes,
		SpeedBytes:            task.SpeedBytes,
		ETASeconds:            task.ETASeconds,
		ErrorMessage:          task.ErrorMessage,
		CreatedAt:             task.CreatedAt.Format(time.RFC3339),
		UpdatedAt:             task.UpdatedAt.Format(time.RFC3339),
		FinishedAt:            finishedAt,
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
