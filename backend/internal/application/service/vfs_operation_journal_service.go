package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

const (
	defaultVFSOperationLockDuration = 5 * time.Minute
	vfsOperationPayloadMaxBytes     = 4096
	vfsOperationRedactedValue       = "[redacted]"
)

// VFSOperationRecordInput 表示写入 operation journal 的应用层输入。
type VFSOperationRecordInput struct {
	OperationType      string
	SourceNodeID       *uint
	TargetParentNodeID *uint
	ResultNodeID       *uint
	SourcePathSnapshot string
	TargetPathSnapshot string
	SourceIDSnapshot   *uint
	DriverTypeSnapshot string
	Payload            map[string]any
	PayloadJSON        string
	ErrorCode          string
	Error              error
	CreatedBy          *uint
	NextRetryAt        *time.Time
}

// VFSOperationProcessor 预留给后续真实 provider 补偿逻辑的处理器边界。
type VFSOperationProcessor interface {
	ProcessVFSOperation(ctx context.Context, operation *entity.VFSOperation) error
}

// VFSOperationJournalService 负责记录、领取和推进 VFS operation journal。
type VFSOperationJournalService struct {
	repo         domainrepo.VFSOperationRepository
	now          func() time.Time
	lockDuration time.Duration
}

// VFSOperationJournalServiceOption 定义 operation journal service 可选配置。
type VFSOperationJournalServiceOption func(*VFSOperationJournalService)

// WithVFSOperationJournalClock 注入时间源，便于测试重试与 lease。
func WithVFSOperationJournalClock(now func() time.Time) VFSOperationJournalServiceOption {
	return func(s *VFSOperationJournalService) {
		if now != nil {
			s.now = now
		}
	}
}

// WithVFSOperationJournalLockDuration 注入 operation worker lease 时长。
func WithVFSOperationJournalLockDuration(duration time.Duration) VFSOperationJournalServiceOption {
	return func(s *VFSOperationJournalService) {
		if duration > 0 {
			s.lockDuration = duration
		}
	}
}

// NewVFSOperationJournalService 创建 VFS operation journal 服务。
func NewVFSOperationJournalService(repo domainrepo.VFSOperationRepository, options ...VFSOperationJournalServiceOption) *VFSOperationJournalService {
	service := &VFSOperationJournalService{
		repo:         repo,
		now:          time.Now,
		lockDuration: defaultVFSOperationLockDuration,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// RecordPending 记录待执行/待补偿 operation。
func (s *VFSOperationJournalService) RecordPending(ctx context.Context, input VFSOperationRecordInput) (*entity.VFSOperation, error) {
	return s.record(ctx, input, entity.VFSOperationStatusPending)
}

// RecordFailure 记录已发生失败且需要后续 worker 重试/补偿的 operation。
func (s *VFSOperationJournalService) RecordFailure(ctx context.Context, input VFSOperationRecordInput) (*entity.VFSOperation, error) {
	if input.NextRetryAt == nil {
		next := s.currentTime().Add(vfsOperationRetryBackoff(1))
		input.NextRetryAt = &next
	}
	return s.record(ctx, input, entity.VFSOperationStatusFailed)
}

// MarkRunning 将 operation 标记为 running，并写入 worker lease。
func (s *VFSOperationJournalService) MarkRunning(ctx context.Context, id uint, workerID string) (*entity.VFSOperation, error) {
	if s == nil || s.repo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	if workerID == "" {
		workerID = "vfs-operation-worker"
	}
	operation, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if operation.Status == entity.VFSOperationStatusSucceeded || operation.Status == entity.VFSOperationStatusCanceled {
		return operation, nil
	}
	now := s.currentTime()
	lockedUntil := now.Add(s.lockDuration)
	operation.Status = entity.VFSOperationStatusRunning
	operation.LockedBy = workerID
	operation.LockedUntil = &lockedUntil
	operation.UpdatedAt = now
	if err := s.repo.Update(ctx, operation); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

// MarkSucceeded 将 operation 标记为 succeeded；重复调用保持成功终态。
func (s *VFSOperationJournalService) MarkSucceeded(ctx context.Context, id uint) (*entity.VFSOperation, error) {
	if s == nil || s.repo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	operation, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if operation.Status == entity.VFSOperationStatusSucceeded {
		return operation, nil
	}
	now := s.currentTime()
	operation.Status = entity.VFSOperationStatusSucceeded
	operation.ErrorCode = ""
	operation.ErrorMessage = ""
	operation.NextRetryAt = nil
	operation.LockedBy = ""
	operation.LockedUntil = nil
	operation.UpdatedAt = now
	if err := s.repo.Update(ctx, operation); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

// MarkFailed 将 operation 标记为 failed，递增 retry_count 并写入下一次重试时间。
func (s *VFSOperationJournalService) MarkFailed(ctx context.Context, id uint, err error) (*entity.VFSOperation, error) {
	if s == nil || s.repo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	operation, findErr := s.repo.FindByID(ctx, id)
	if findErr != nil {
		return nil, findErr
	}
	if operation.Status == entity.VFSOperationStatusSucceeded || operation.Status == entity.VFSOperationStatusCanceled {
		return operation, nil
	}
	now := s.currentTime()
	retryCount := operation.RetryCount + 1
	nextRetryAt := now.Add(vfsOperationRetryBackoff(retryCount))
	operation.Status = entity.VFSOperationStatusFailed
	operation.ErrorCode = stableVFSOperationErrorCode("", err)
	operation.ErrorMessage = sanitizeVFSOperationErrorMessage(operation.ErrorCode, err)
	operation.RetryCount = retryCount
	operation.NextRetryAt = &nextRetryAt
	operation.LockedBy = ""
	operation.LockedUntil = nil
	operation.UpdatedAt = now
	if err := s.repo.Update(ctx, operation); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

// ListDue 列出当前到期可领取的 pending/failed 以及 lease 已过期的 running operation。
func (s *VFSOperationJournalService) ListDue(ctx context.Context, limit int) ([]*entity.VFSOperation, error) {
	if s == nil || s.repo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	return s.repo.ListDue(ctx, domainrepo.VFSOperationDueFilter{
		DueBefore: s.currentTime(),
		Limit:     limit,
	})
}

// AcquireDue 领取当前到期 operation，并将其标记为 running。
func (s *VFSOperationJournalService) AcquireDue(ctx context.Context, workerID string, limit int) ([]*entity.VFSOperation, error) {
	if s == nil || s.repo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	now := s.currentTime()
	return s.repo.AcquireDue(ctx, domainrepo.VFSOperationDueFilter{
		DueBefore: now,
		Limit:     limit,
	}, domainrepo.VFSOperationLock{
		WorkerID:    workerID,
		LockedUntil: now.Add(s.lockDuration),
	})
}

// ProcessDue 提供 worker 骨架：领取到期 operation，委托 processor，按结果标记成功或失败。
func (s *VFSOperationJournalService) ProcessDue(ctx context.Context, workerID string, limit int, processor VFSOperationProcessor) (int, error) {
	if processor == nil {
		return 0, ErrSourceOperationUnsupported
	}
	operations, err := s.AcquireDue(ctx, workerID, limit)
	if err != nil {
		return 0, err
	}
	var joined error
	for _, operation := range operations {
		if processErr := processor.ProcessVFSOperation(ctx, operation); processErr != nil {
			joined = errors.Join(joined, sanitizedVFSOperationReturnedError(processErr))
			if _, markErr := s.MarkFailed(ctx, operation.ID, processErr); markErr != nil {
				joined = errors.Join(joined, sanitizedVFSOperationReturnedError(markErr))
			}
			continue
		}
		if _, markErr := s.MarkSucceeded(ctx, operation.ID); markErr != nil {
			joined = errors.Join(joined, sanitizedVFSOperationReturnedError(markErr))
		}
	}
	return len(operations), joined
}

func (s *VFSOperationJournalService) record(ctx context.Context, input VFSOperationRecordInput, status string) (*entity.VFSOperation, error) {
	if s == nil || s.repo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	if !isValidVFSOperationType(input.OperationType) {
		return nil, ErrConfigInvalid
	}
	now := s.currentTime()
	payloadJSON, err := normalizeVFSOperationPayload(input.Payload, input.PayloadJSON)
	if err != nil {
		return nil, err
	}
	errorCode := stableVFSOperationErrorCode(input.ErrorCode, input.Error)
	operation := &entity.VFSOperation{
		OperationType:      input.OperationType,
		Status:             status,
		SourceNodeID:       input.SourceNodeID,
		TargetParentNodeID: input.TargetParentNodeID,
		ResultNodeID:       input.ResultNodeID,
		SourcePathSnapshot: normalizeVFSOperationSnapshot(input.SourcePathSnapshot),
		TargetPathSnapshot: normalizeVFSOperationSnapshot(input.TargetPathSnapshot),
		SourceIDSnapshot:   input.SourceIDSnapshot,
		DriverTypeSnapshot: strings.TrimSpace(input.DriverTypeSnapshot),
		PayloadJSON:        payloadJSON,
		ErrorCode:          errorCode,
		ErrorMessage:       sanitizeVFSOperationErrorMessage(errorCode, input.Error),
		RetryCount:         0,
		NextRetryAt:        input.NextRetryAt,
		CreatedBy:          input.CreatedBy,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.repo.Create(ctx, operation); err != nil {
		return nil, err
	}
	return operation, nil
}

func (s *VFSOperationJournalService) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func isValidVFSOperationType(operationType string) bool {
	switch strings.TrimSpace(operationType) {
	case entity.VFSOperationTypeMkdir,
		entity.VFSOperationTypeRename,
		entity.VFSOperationTypeMove,
		entity.VFSOperationTypeCopy,
		entity.VFSOperationTypeDelete,
		entity.VFSOperationTypeImport,
		entity.VFSOperationTypeUploadCommit,
		entity.VFSOperationTypeTaskCommit,
		entity.VFSOperationTypeRefresh:
		return true
	default:
		return false
	}
}

func normalizeVFSOperationPayload(payload map[string]any, raw string) (string, error) {
	if payload != nil {
		data, err := json.Marshal(sanitizeVFSOperationPayloadMap(payload))
		if err != nil {
			return "", err
		}
		if len(data) > vfsOperationPayloadMaxBytes {
			return `{"omitted":true}`, nil
		}
		return string(data), nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "null") {
		return "{}", nil
	}
	if len([]byte(trimmed)) > vfsOperationPayloadMaxBytes {
		return `{"omitted":true}`, nil
	}
	if !json.Valid([]byte(trimmed)) {
		return "{}", nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return "{}", nil
	}
	data, err := json.Marshal(sanitizeVFSOperationPayloadMap(decoded))
	if err != nil {
		return "", err
	}
	if len(data) > vfsOperationPayloadMaxBytes {
		return `{"omitted":true}`, nil
	}
	return string(data), nil
}

func stableVFSOperationErrorCode(preferred string, err error) string {
	code := strings.TrimSpace(preferred)
	if code != "" {
		return code
	}
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrMetadataVFSMutationSyncFailed):
		return "METADATA_VFS_MUTATION_SYNC_FAILED"
	case errors.Is(err, ErrMetadataVFSCommitFailed):
		return "METADATA_VFS_COMMIT_FAILED"
	case errors.Is(err, ErrVFSSyncConflict):
		return "VFS_SYNC_CONFLICT"
	case errors.Is(err, ErrSourceOperationUnsupported), errors.Is(err, ErrSourceDriverUnsupported):
		return "SOURCE_OPERATION_UNSUPPORTED"
	default:
		return "VFS_OPERATION_FAILED"
	}
}

func sanitizeVFSOperationErrorMessage(errorCode string, err error) string {
	if err == nil {
		return ""
	}
	switch errorCode {
	case "METADATA_VFS_MUTATION_SYNC_FAILED":
		return ErrMetadataVFSMutationSyncFailed.Error()
	case "METADATA_VFS_COMMIT_FAILED":
		return ErrMetadataVFSCommitFailed.Error()
	case "VFS_SYNC_CONFLICT":
		return ErrVFSSyncConflict.Error()
	case "SOURCE_OPERATION_UNSUPPORTED":
		return ErrSourceOperationUnsupported.Error()
	default:
		return "vfs operation failed"
	}
}

func normalizeVFSOperationSnapshot(value string) string {
	const maxSnapshotLen = 1024
	trimmed := strings.TrimSpace(value)
	if containsSensitiveVFSOperationSnapshot(trimmed) {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= maxSnapshotLen {
		return trimmed
	}
	return string(runes[:maxSnapshotLen])
}

func containsSensitiveVFSOperationSnapshot(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	return looksLikeWindowsPhysicalPath(value) ||
		strings.Contains(lower, "sql:") ||
		strings.Contains(lower, "duplicate key") ||
		strings.Contains(lower, "provider payload") ||
		strings.Contains(lower, "provider-payload") ||
		strings.Contains(lower, "access_token") ||
		strings.Contains(lower, "refresh_token")
}

func sanitizedVFSOperationReturnedError(err error) error {
	if err == nil {
		return nil
	}
	code := stableVFSOperationErrorCode("", err)
	return errors.New(sanitizeVFSOperationErrorMessage(code, err))
}

func sanitizeVFSOperationPayloadMap(payload map[string]any) map[string]any {
	sanitized := make(map[string]any, len(payload))
	for key, value := range payload {
		if isSensitiveVFSOperationPayloadKey(key) {
			sanitized[key] = vfsOperationRedactedValue
			continue
		}
		sanitized[key] = sanitizeVFSOperationPayloadValue(value)
	}
	return sanitized
}

func sanitizeVFSOperationPayloadValue(value any) any {
	switch typed := value.(type) {
	case nil, bool, float64, int, int64, uint, uint64:
		return typed
	case string:
		if containsSensitiveVFSOperationText(typed) {
			return vfsOperationRedactedValue
		}
		return typed
	case []any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, sanitizeVFSOperationPayloadValue(item))
		}
		return items
	case map[string]any:
		return sanitizeVFSOperationPayloadMap(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil || containsSensitiveVFSOperationText(string(data)) {
			return vfsOperationRedactedValue
		}
		return typed
	}
}

func isSensitiveVFSOperationPayloadKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	sensitiveFragments := []string{
		"payload",
		"provider",
		"token",
		"secret",
		"password",
		"credential",
		"config",
		"dsn",
		"sql",
		"query",
		"locator",
		"path",
		"file_id",
		"object_key",
		"oss",
	}
	for _, fragment := range sensitiveFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func containsSensitiveVFSOperationText(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	sensitiveFragments := []string{
		"sql:",
		"select ",
		"insert ",
		"update ",
		"delete ",
		"duplicate key",
		"provider payload",
		"provider-payload",
		"access_token",
		"refresh_token",
		"secret",
		"password",
		"credential",
		"private_key",
	}
	for _, fragment := range sensitiveFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	if looksLikeWindowsPhysicalPath(trimmed) || looksLikeUnixPhysicalPath(trimmed) {
		return true
	}
	return false
}

func looksLikeWindowsPhysicalPath(value string) bool {
	if strings.HasPrefix(value, `\\`) {
		return true
	}
	if len(value) < 3 {
		return false
	}
	drive := value[0]
	return ((drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')) &&
		value[1] == ':' &&
		(value[2] == '\\' || value[2] == '/')
}

func looksLikeUnixPhysicalPath(value string) bool {
	physicalPrefixes := []string{
		"/mnt/",
		"/var/",
		"/tmp/",
		"/home/",
		"/root/",
		"/data/",
		"/opt/",
		"/etc/",
		"/srv/",
		"/usr/",
		"/volumes/",
		"/users/",
	}
	lower := strings.ToLower(value)
	for _, prefix := range physicalPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func vfsOperationRetryBackoff(retryCount int) time.Duration {
	switch {
	case retryCount <= 1:
		return time.Minute
	case retryCount == 2:
		return 5 * time.Minute
	case retryCount == 3:
		return 30 * time.Minute
	default:
		return 2 * time.Hour
	}
}
