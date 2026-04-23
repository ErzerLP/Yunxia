package dto

// AuditLogListQuery 表示审计列表查询参数。
type AuditLogListQuery struct {
	Page         int     `form:"page"`
	PageSize     int     `form:"page_size"`
	ActorUserID  *uint   `form:"actor_user_id"`
	ActorRoleKey string  `form:"actor_role_key"`
	ResourceType string  `form:"resource_type"`
	Action       string  `form:"action"`
	Result       string  `form:"result"`
	SourceID     *uint   `form:"source_id"`
	VirtualPath  string  `form:"virtual_path"`
	RequestID    string  `form:"request_id"`
	EntryPoint   string  `form:"entrypoint"`
	StartedAt    *string `form:"started_at"`
	EndedAt      *string `form:"ended_at"`
}

// AuditActorView 表示审计 actor 视图。
type AuditActorView struct {
	UserID   *uint  `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	RoleKey  string `json:"role_key,omitempty"`
}

// AuditRequestView 表示审计请求视图。
type AuditRequestView struct {
	RequestID  string `json:"request_id"`
	EntryPoint string `json:"entrypoint"`
	ClientIP   string `json:"client_ip,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`
	Method     string `json:"method,omitempty"`
	Path       string `json:"path,omitempty"`
}

// AuditTargetView 表示审计目标视图。
type AuditTargetView struct {
	ResourceID       string `json:"resource_id,omitempty"`
	SourceID         *uint  `json:"source_id,omitempty"`
	VirtualPath      string `json:"virtual_path,omitempty"`
	ResolvedSourceID *uint  `json:"resolved_source_id,omitempty"`
	ResolvedPath     string `json:"resolved_path,omitempty"`
}

// AuditLogListItem 表示审计列表项。
type AuditLogListItem struct {
	ID           uint           `json:"id"`
	OccurredAt   string         `json:"occurred_at"`
	RequestID    string         `json:"request_id"`
	EntryPoint   string         `json:"entrypoint"`
	Actor        AuditActorView `json:"actor"`
	ResourceType string         `json:"resource_type"`
	Action       string         `json:"action"`
	Result       string         `json:"result"`
	ErrorCode    string         `json:"error_code,omitempty"`
	ResourceID   string         `json:"resource_id,omitempty"`
	SourceID     *uint          `json:"source_id,omitempty"`
	VirtualPath  string         `json:"virtual_path,omitempty"`
	Summary      string         `json:"summary"`
}

// AuditLogListResponse 表示审计列表响应。
type AuditLogListResponse struct {
	Items      []AuditLogListItem `json:"items"`
	Total      int                `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	TotalPages int                `json:"total_pages"`
}

// AuditLogDetailResponse 表示审计详情响应。
type AuditLogDetailResponse struct {
	ID           uint             `json:"id"`
	OccurredAt   string           `json:"occurred_at"`
	Actor        AuditActorView   `json:"actor"`
	Request      AuditRequestView `json:"request"`
	Target       AuditTargetView  `json:"target"`
	ResourceType string           `json:"resource_type"`
	Action       string           `json:"action"`
	Result       string           `json:"result"`
	ErrorCode    string           `json:"error_code,omitempty"`
	Summary      string           `json:"summary"`
	Before       map[string]any   `json:"before,omitempty"`
	After        map[string]any   `json:"after,omitempty"`
	Detail       map[string]any   `json:"detail,omitempty"`
}
