package audit

import (
	"context"
	"log/slog"
	"time"

	"yunxia/internal/domain/entity"
	"yunxia/internal/infrastructure/observability/logging"
)

type createRepository interface {
	Create(ctx context.Context, log *entity.AuditLog) error
}

// Recorder 负责把审计事件写入仓储。
type Recorder struct {
	repo   createRepository
	logger *slog.Logger
	now    func() time.Time
}

// NewRecorder 创建审计记录器。
func NewRecorder(repo createRepository, logger *slog.Logger) *Recorder {
	return &Recorder{
		repo:   repo,
		logger: logger,
		now:    time.Now,
	}
}

// Record 把事件转换为审计实体并落库。
func (r *Recorder) Record(ctx context.Context, event Event) error {
	if r == nil || r.repo == nil {
		return nil
	}

	now := r.now()
	snapshot := SnapshotFromContext(ctx)
	target := normalizeTarget(event)

	log := &entity.AuditLog{
		OccurredAt:       now,
		RequestID:        snapshot.RequestID,
		EntryPoint:       snapshot.EntryPoint,
		ActorUserID:      snapshot.ActorUserID,
		ActorUsername:    snapshot.ActorUsername,
		ActorRoleKey:     snapshot.ActorRoleKey,
		ClientIP:         snapshot.ClientIP,
		UserAgent:        snapshot.UserAgent,
		Method:           snapshot.Method,
		Path:             snapshot.Path,
		ResourceType:     event.ResourceType,
		Action:           event.Action,
		Result:           string(event.Result),
		ErrorCode:        event.ErrorCode,
		ResourceID:       target.ResourceID,
		SourceID:         target.SourceID,
		VirtualPath:      target.VirtualPath,
		ResolvedSourceID: target.ResolvedSourceID,
		ResolvedPath:     target.ResolvedPath,
		BeforeJSON:       encodeOptionalJSON(event.Before),
		AfterJSON:        encodeOptionalJSON(event.After),
		DetailJSON:       encodeOptionalJSON(event.Detail),
		CreatedAt:        now,
	}
	return r.repo.Create(ctx, log)
}

// RecordBestEffort 以不影响主流程的方式记录审计日志。
func RecordBestEffort(ctx context.Context, recorder *Recorder, fallback *slog.Logger, event Event) {
	if recorder == nil {
		return
	}
	if err := recorder.Record(ctx, event); err != nil {
		logging.FromContext(ctx, fallback).Error("audit write failed",
			slog.String("event", "audit.write.failed"),
			slog.String("resource_type", event.ResourceType),
			slog.String("action", event.Action),
			slog.String("result", string(event.Result)),
			slog.Any("error", err),
		)
	}
}

func normalizeTarget(event Event) Target {
	target := event.Target
	if target.ResourceID == "" {
		target.ResourceID = event.ResourceID
	}
	if target.SourceID == nil {
		target.SourceID = event.SourceID
	}
	if target.VirtualPath == "" {
		target.VirtualPath = event.VirtualPath
	}
	return target
}

func encodeOptionalJSON(value map[string]any) string {
	if len(value) == 0 {
		return ""
	}
	return mustJSON(value)
}
