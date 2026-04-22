package dto

// ACLPermissions 表示 ACL 权限集合。
type ACLPermissions struct {
	Read   bool `json:"read"`
	Write  bool `json:"write"`
	Delete bool `json:"delete"`
	Share  bool `json:"share"`
}

// ACLRuleListQuery 表示 ACL 规则列表查询。
type ACLRuleListQuery struct {
	SourceID uint   `form:"source_id" binding:"required"`
	Path     string `form:"path"`
}

// ACLRuleView 表示 ACL 规则响应结构。
type ACLRuleView struct {
	ID                uint           `json:"id"`
	SourceID          uint           `json:"source_id"`
	Path              string         `json:"path"`
	VirtualPath       string         `json:"virtual_path,omitempty"`
	SubjectType       string         `json:"subject_type"`
	SubjectID         uint           `json:"subject_id"`
	Effect            string         `json:"effect"`
	Priority          int            `json:"priority"`
	Permissions       ACLPermissions `json:"permissions"`
	InheritToChildren bool           `json:"inherit_to_children"`
}

// ACLRuleListResponse 表示 ACL 规则列表响应。
type ACLRuleListResponse struct {
	Items []ACLRuleView `json:"items"`
}

// CreateACLRuleRequest 表示创建 ACL 规则请求。
type CreateACLRuleRequest struct {
	SourceID          uint           `json:"source_id" binding:"required"`
	Path              string         `json:"path" binding:"required"`
	SubjectType       string         `json:"subject_type" binding:"required"`
	SubjectID         uint           `json:"subject_id" binding:"required"`
	Effect            string         `json:"effect" binding:"required"`
	Priority          int            `json:"priority"`
	Permissions       ACLPermissions `json:"permissions" binding:"required"`
	InheritToChildren bool           `json:"inherit_to_children"`
}

// UpdateACLRuleRequest 表示更新 ACL 规则请求。
type UpdateACLRuleRequest struct {
	Path              string         `json:"path" binding:"required"`
	SubjectType       string         `json:"subject_type" binding:"required"`
	SubjectID         uint           `json:"subject_id" binding:"required"`
	Effect            string         `json:"effect" binding:"required"`
	Priority          int            `json:"priority"`
	Permissions       ACLPermissions `json:"permissions" binding:"required"`
	InheritToChildren bool           `json:"inherit_to_children"`
}
