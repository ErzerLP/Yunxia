package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	appaudit "yunxia/internal/application/audit"
	"yunxia/internal/domain/entity"
	"yunxia/internal/infrastructure/observability/logging"
	infraStorage "yunxia/internal/infrastructure/storage"
)

// SetupServiceOption 定义 SetupService 的可选配置。
type SetupServiceOption func(*SetupService)

// UserServiceOption 定义 UserService 的可选配置。
type UserServiceOption func(*UserService)

// ACLServiceOption 定义 ACLService 的可选配置。
type ACLServiceOption func(*ACLService)

// FileServiceOption 已在 storage_driver.go 定义。
// TrashServiceOption 已在 storage_driver.go 定义。
// UploadServiceOption 已在 storage_driver.go 定义。
// TaskServiceOption 已在 storage_driver.go 定义。
// ShareServiceOption 已在 storage_driver.go 定义。

// WithSetupAuditRecorder 为 SetupService 注入审计记录器。
func WithSetupAuditRecorder(recorder *appaudit.Recorder) SetupServiceOption {
	return func(s *SetupService) {
		s.auditRecorder = recorder
	}
}

// WithUserAuditRecorder 为 UserService 注入审计记录器。
func WithUserAuditRecorder(recorder *appaudit.Recorder) UserServiceOption {
	return func(s *UserService) {
		s.auditRecorder = recorder
	}
}

// WithACLAuditRecorder 为 ACLService 注入审计记录器。
func WithACLAuditRecorder(recorder *appaudit.Recorder) ACLServiceOption {
	return func(s *ACLService) {
		s.auditRecorder = recorder
	}
}

// WithSystemAuditRecorder 为 SystemService 注入审计记录器。
func WithSystemAuditRecorder(recorder *appaudit.Recorder) SystemServiceOption {
	return func(s *SystemService) {
		s.auditRecorder = recorder
	}
}

// WithSourceAuditRecorder 为 SourceService 注入审计记录器。
func WithSourceAuditRecorder(recorder *appaudit.Recorder) SourceServiceOption {
	return func(s *SourceService) {
		s.auditRecorder = recorder
	}
}

// WithFileAuditRecorder 为 FileService 注入审计记录器。
func WithFileAuditRecorder(recorder *appaudit.Recorder) FileServiceOption {
	return func(s *FileService) {
		s.auditRecorder = recorder
	}
}

// WithTrashAuditRecorder 为 TrashService 注入审计记录器。
func WithTrashAuditRecorder(recorder *appaudit.Recorder) TrashServiceOption {
	return func(s *TrashService) {
		s.auditRecorder = recorder
	}
}

// WithUploadAuditRecorder 为 UploadService 注入审计记录器。
func WithUploadAuditRecorder(recorder *appaudit.Recorder) UploadServiceOption {
	return func(s *UploadService) {
		s.auditRecorder = recorder
	}
}

// WithTaskAuditRecorder 为 TaskService 注入审计记录器。
func WithTaskAuditRecorder(recorder *appaudit.Recorder) TaskServiceOption {
	return func(s *TaskService) {
		s.auditRecorder = recorder
	}
}

// WithShareAuditRecorder 为 ShareService 注入审计记录器。
func WithShareAuditRecorder(recorder *appaudit.Recorder) ShareServiceOption {
	return func(s *ShareService) {
		s.auditRecorder = recorder
	}
}

func newServiceLogger(component string) *slog.Logger {
	return logging.Component(slog.Default(), component)
}

func recordServiceAudit(ctx context.Context, logger *slog.Logger, recorder *appaudit.Recorder, event appaudit.Event) {
	appaudit.RecordBestEffort(ctx, recorder, logger, event)
}

func encodeUintID(value uint) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(value), 10)
}

func userAuditView(user *entity.User) map[string]any {
	if user == nil {
		return nil
	}
	return map[string]any{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
		"role_key": user.RoleKey,
		"status":   user.Status,
	}
}

func systemConfigAuditView(cfg *entity.SystemConfig) map[string]any {
	if cfg == nil {
		return nil
	}
	return map[string]any{
		"site_name":          cfg.SiteName,
		"multi_user_enabled": cfg.MultiUserEnabled,
		"default_source_id":  cfg.DefaultSourceID,
		"max_upload_size":    cfg.MaxUploadSize,
		"default_chunk_size": cfg.DefaultChunkSize,
		"webdav_enabled":     cfg.WebDAVEnabled,
		"webdav_prefix":      cfg.WebDAVPrefix,
		"theme":              cfg.Theme,
		"language":           cfg.Language,
		"time_zone":          cfg.TimeZone,
	}
}

func shareAuditView(share *entity.ShareLink) map[string]any {
	if share == nil {
		return nil
	}
	return map[string]any{
		"id":                  share.ID,
		"user_id":             share.UserID,
		"source_id":           share.SourceID,
		"path":                share.Path,
		"target_virtual_path": share.TargetVirtualPath,
		"resolved_source_id":  share.ResolvedSourceID,
		"resolved_inner_path": share.ResolvedInnerPath,
		"name":                share.Name,
		"is_dir":              share.IsDir,
		"has_password":        share.PasswordHash != nil,
		"expires_at":          share.ExpiresAt,
	}
}

func taskAuditView(task *entity.DownloadTask) map[string]any {
	if task == nil {
		return nil
	}
	return map[string]any{
		"id":                       task.ID,
		"user_id":                  task.UserID,
		"type":                     task.Type,
		"status":                   task.Status,
		"source_id":                task.SourceID,
		"save_path":                task.SavePath,
		"save_virtual_path":        task.SaveVirtualPath,
		"resolved_source_id":       task.ResolvedSourceID,
		"resolved_inner_save_path": task.ResolvedInnerSavePath,
		"display_name":             task.DisplayName,
		"source_url":               task.SourceURL,
	}
}

func sourceAuditView(source *entity.StorageSource) map[string]any {
	if source == nil {
		return nil
	}
	view := map[string]any{
		"id":                source.ID,
		"name":              source.Name,
		"driver_type":       source.DriverType,
		"is_enabled":        source.IsEnabled,
		"is_webdav_exposed": source.IsWebDAVExposed,
		"webdav_read_only":  source.WebDAVReadOnly,
		"mount_path":        source.MountPath,
		"root_path":         source.RootPath,
		"sort_order":        source.SortOrder,
		"config":            map[string]any{},
	}
	switch source.DriverType {
	case "local":
		cfg, err := parseLocalSourceConfig(source)
		if err == nil {
			view["config"] = map[string]any{"base_path": cfg.BasePath}
		}
	case "s3":
		cfg, err := infraStorage.ParseS3ConfigJSON(source.ConfigJSON)
		if err == nil {
			view["config"] = map[string]any{
				"endpoint":         cfg.Endpoint,
				"region":           cfg.Region,
				"bucket":           cfg.Bucket,
				"base_prefix":      cfg.BasePrefix,
				"force_path_style": cfg.ForcePathStyle,
			}
		}
	}
	return view
}

func sourceTestDetail(source *entity.StorageSource, latencyMS int64, reachable bool) map[string]any {
	if source == nil {
		return nil
	}
	return map[string]any{
		"name":        source.Name,
		"driver_type": source.DriverType,
		"reachable":   reachable,
		"latency_ms":  latencyMS,
	}
}

func sourceErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrSourceDriverUnsupported):
		return "SOURCE_DRIVER_UNSUPPORTED"
	case errors.Is(err, ErrConfigInvalid):
		return "CONFIG_INVALID"
	case errors.Is(err, ErrSourceConnectionFailed):
		return "SOURCE_CONNECTION_FAILED"
	case errors.Is(err, ErrSourceNameConflict):
		return "SOURCE_NAME_CONFLICT"
	case errors.Is(err, ErrSourceMountPathConflict):
		return "MOUNT_PATH_CONFLICT"
	case errors.Is(err, ErrSourceInUse):
		return "SOURCE_IN_USE"
	case errors.Is(err, ErrPathInvalid):
		return "PATH_INVALID"
	default:
		return "INTERNAL_ERROR"
	}
}

func aclRuleAuditView(rule *entity.ACLRule) map[string]any {
	if rule == nil {
		return nil
	}
	return map[string]any{
		"id":                  rule.ID,
		"source_id":           rule.SourceID,
		"path":                rule.Path,
		"virtual_path":        rule.VirtualPath,
		"subject_type":        rule.SubjectType,
		"subject_id":          rule.SubjectID,
		"effect":              rule.Effect,
		"priority":            rule.Priority,
		"inherit_to_children": rule.InheritToChildren,
		"permissions": map[string]any{
			"read":   rule.Read,
			"write":  rule.Write,
			"delete": rule.Delete,
			"share":  rule.Share,
		},
	}
}

func aclErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrACLSubjectTypeInvalid):
		return "ACL_SUBJECT_TYPE_INVALID"
	case errors.Is(err, ErrACLEffectInvalid):
		return "ACL_EFFECT_INVALID"
	case errors.Is(err, ErrACLPermissionsInvalid):
		return "ACL_PERMISSIONS_INVALID"
	case errors.Is(err, ErrPathInvalid):
		return "PATH_INVALID"
	default:
		return "INTERNAL_ERROR"
	}
}

func mapFileErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrFileNotFound):
		return "FILE_NOT_FOUND"
	case errors.Is(err, ErrFileAlreadyExists):
		return "FILE_ALREADY_EXISTS"
	case errors.Is(err, ErrFileNameInvalid):
		return "FILE_NAME_INVALID"
	case errors.Is(err, ErrFileIsDirectory):
		return "FILE_IS_DIRECTORY"
	case errors.Is(err, ErrFileMoveConflict):
		return "FILE_MOVE_CONFLICT"
	case errors.Is(err, ErrFileCopyConflict):
		return "FILE_COPY_CONFLICT"
	case errors.Is(err, ErrUploadSessionNotFound):
		return "UPLOAD_SESSION_NOT_FOUND"
	case errors.Is(err, ErrUploadChunkConflict):
		return "UPLOAD_CHUNK_CONFLICT"
	case errors.Is(err, ErrUploadFinishIncomplete):
		return "UPLOAD_FINISH_INCOMPLETE"
	case errors.Is(err, ErrUploadHashMismatch):
		return "UPLOAD_HASH_MISMATCH"
	case errors.Is(err, ErrUploadInvalidState):
		return "UPLOAD_INVALID_STATE"
	case errors.Is(err, ErrUploadTooLarge):
		return "UPLOAD_TOO_LARGE"
	case errors.Is(err, ErrSourceDriverUnsupported):
		return "SOURCE_DRIVER_UNSUPPORTED"
	case errors.Is(err, ErrPathInvalid):
		return "PATH_INVALID"
	default:
		return "INTERNAL_ERROR"
	}
}

func taskErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrPermissionDenied):
		return "PERMISSION_DENIED"
	case errors.Is(err, ErrSourceDriverUnsupported):
		return "SOURCE_DRIVER_UNSUPPORTED"
	case errors.Is(err, ErrTaskInvalidState):
		return "TASK_INVALID_STATE"
	case errors.Is(err, ErrPathInvalid):
		return "PATH_INVALID"
	default:
		return "INTERNAL_ERROR"
	}
}

func shareErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrPermissionDenied):
		return "PERMISSION_DENIED"
	case errors.Is(err, ErrFileNotFound):
		return "FILE_NOT_FOUND"
	case errors.Is(err, ErrShareExpired):
		return "SHARE_EXPIRED"
	case errors.Is(err, ErrSharePasswordRequired):
		return "SHARE_PASSWORD_REQUIRED"
	case errors.Is(err, ErrSharePasswordInvalid):
		return "SHARE_PASSWORD_INVALID"
	case errors.Is(err, ErrSourceDriverUnsupported):
		return "SOURCE_DRIVER_UNSUPPORTED"
	case errors.Is(err, ErrPathInvalid):
		return "PATH_INVALID"
	default:
		return "INTERNAL_ERROR"
	}
}
