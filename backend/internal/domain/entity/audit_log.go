package entity

import "time"

// AuditLog 表示一条审计记录。
type AuditLog struct {
	ID               uint
	OccurredAt       time.Time
	RequestID        string
	EntryPoint       string
	ActorUserID      *uint
	ActorUsername    string
	ActorRoleKey     string
	ClientIP         string
	UserAgent        string
	Method           string
	Path             string
	ResourceType     string
	Action           string
	Result           string
	ErrorCode        string
	ResourceID       string
	SourceID         *uint
	VirtualPath      string
	ResolvedSourceID *uint
	ResolvedPath     string
	BeforeJSON       string
	AfterJSON        string
	DetailJSON       string
	CreatedAt        time.Time
}

// AuditLogFilter 定义审计查询条件。
type AuditLogFilter struct {
	Page         int
	PageSize     int
	ActorUserID  *uint
	ActorRoleKey string
	ResourceType string
	Action       string
	Result       string
	SourceID     *uint
	VirtualPath  string
	RequestID    string
	EntryPoint   string
	StartedAt    *time.Time
	EndedAt      *time.Time
}
