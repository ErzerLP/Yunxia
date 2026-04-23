package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/repository"
)

var (
	// ErrInvalidTimeFilter 表示时间过滤参数非法。
	ErrInvalidTimeFilter = errors.New("invalid audit time filter")
)

type queryRepository interface {
	FindByID(ctx context.Context, id uint) (*entity.AuditLog, error)
	List(ctx context.Context, filter entity.AuditLogFilter) ([]*entity.AuditLog, int, error)
}

// QueryService 负责审计查询。
type QueryService struct {
	repo queryRepository
}

// NewQueryService 创建审计查询服务。
func NewQueryService(repo queryRepository) *QueryService {
	return &QueryService{repo: repo}
}

// List 返回审计列表。
func (s *QueryService) List(ctx context.Context, query appdto.AuditLogListQuery) (*appdto.AuditLogListResponse, error) {
	if s == nil || s.repo == nil {
		return nil, repository.ErrNotFound
	}

	startedAt, err := parseAuditTime(query.StartedAt)
	if err != nil {
		return nil, err
	}
	endedAt, err := parseAuditTime(query.EndedAt)
	if err != nil {
		return nil, err
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	items, total, err := s.repo.List(ctx, entity.AuditLogFilter{
		Page:         page,
		PageSize:     pageSize,
		ActorUserID:  query.ActorUserID,
		ActorRoleKey: strings.TrimSpace(query.ActorRoleKey),
		ResourceType: strings.TrimSpace(query.ResourceType),
		Action:       strings.TrimSpace(query.Action),
		Result:       strings.TrimSpace(query.Result),
		SourceID:     query.SourceID,
		VirtualPath:  strings.TrimSpace(query.VirtualPath),
		RequestID:    strings.TrimSpace(query.RequestID),
		EntryPoint:   strings.TrimSpace(query.EntryPoint),
		StartedAt:    startedAt,
		EndedAt:      endedAt,
	})
	if err != nil {
		return nil, err
	}
	return toAuditLogListResponse(items, total, page, pageSize), nil
}

// Get 返回单条审计详情。
func (s *QueryService) Get(ctx context.Context, id uint) (*appdto.AuditLogDetailResponse, error) {
	if s == nil || s.repo == nil {
		return nil, repository.ErrNotFound
	}
	item, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toAuditLogDetailResponse(item), nil
}

func parseAuditTime(value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*value))
	if err != nil {
		return nil, ErrInvalidTimeFilter
	}
	return &parsed, nil
}

func toAuditLogListResponse(items []*entity.AuditLog, total int, page int, pageSize int) *appdto.AuditLogListResponse {
	result := make([]appdto.AuditLogListItem, 0, len(items))
	for _, item := range items {
		result = append(result, appdto.AuditLogListItem{
			ID:         item.ID,
			OccurredAt: item.OccurredAt.Format(time.RFC3339),
			RequestID:  item.RequestID,
			EntryPoint: item.EntryPoint,
			Actor: appdto.AuditActorView{
				UserID:   item.ActorUserID,
				Username: item.ActorUsername,
				RoleKey:  item.ActorRoleKey,
			},
			ResourceType: item.ResourceType,
			Action:       item.Action,
			Result:       item.Result,
			ErrorCode:    item.ErrorCode,
			ResourceID:   item.ResourceID,
			SourceID:     item.SourceID,
			VirtualPath:  item.VirtualPath,
			Summary: Summary(Event{
				ResourceType: item.ResourceType,
				Action:       item.Action,
				Result:       Result(item.Result),
			}),
		})
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	return &appdto.AuditLogListResponse{
		Items:      result,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

func toAuditLogDetailResponse(item *entity.AuditLog) *appdto.AuditLogDetailResponse {
	if item == nil {
		return &appdto.AuditLogDetailResponse{}
	}
	return &appdto.AuditLogDetailResponse{
		ID:         item.ID,
		OccurredAt: item.OccurredAt.Format(time.RFC3339),
		Actor: appdto.AuditActorView{
			UserID:   item.ActorUserID,
			Username: item.ActorUsername,
			RoleKey:  item.ActorRoleKey,
		},
		Request: appdto.AuditRequestView{
			RequestID:  item.RequestID,
			EntryPoint: item.EntryPoint,
			ClientIP:   item.ClientIP,
			UserAgent:  item.UserAgent,
			Method:     item.Method,
			Path:       item.Path,
		},
		Target: appdto.AuditTargetView{
			ResourceID:       item.ResourceID,
			SourceID:         item.SourceID,
			VirtualPath:      item.VirtualPath,
			ResolvedSourceID: item.ResolvedSourceID,
			ResolvedPath:     item.ResolvedPath,
		},
		ResourceType: item.ResourceType,
		Action:       item.Action,
		Result:       item.Result,
		ErrorCode:    item.ErrorCode,
		Summary: Summary(Event{
			ResourceType: item.ResourceType,
			Action:       item.Action,
			Result:       Result(item.Result),
		}),
		Before: decodeAuditJSON(item.BeforeJSON),
		After:  decodeAuditJSON(item.AfterJSON),
		Detail: decodeAuditJSON(item.DetailJSON),
	}
}

func decodeAuditJSON(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]any{"raw": raw}
	}
	return result
}
