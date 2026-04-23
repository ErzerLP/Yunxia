package dto

// UserListQuery 表示用户列表查询参数。
type UserListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Keyword  string `form:"keyword"`
	Status   string `form:"status"`
}

// UserAdminView 表示管理员视角下的用户信息。
type UserAdminView struct {
	ID          uint    `json:"id"`
	Username    string  `json:"username"`
	Email       string  `json:"email"`
	RoleKey     string  `json:"role_key"`
	Status      string  `json:"status"`
	LastLoginAt *string `json:"last_login_at"`
	CreatedAt   string  `json:"created_at"`
}

// UserListResponse 表示用户列表响应。
type UserListResponse struct {
	Items []UserAdminView `json:"items"`
}

// CreateUserRequest 表示管理员创建用户请求。
type CreateUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=8"`
	Email    string `json:"email"`
	RoleKey  string `json:"role_key" binding:"required"`
}

// UpdateUserRequest 表示管理员更新用户请求。
type UpdateUserRequest struct {
	Email   string `json:"email"`
	RoleKey string `json:"role_key" binding:"required"`
	Status  string `json:"status" binding:"required"`
}

// ResetUserPasswordRequest 表示管理员重置密码请求。
type ResetUserPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// RevokeUserTokensResponse 表示撤销用户令牌响应。
type RevokeUserTokensResponse struct {
	ID      uint `json:"id"`
	Revoked bool `json:"revoked"`
}
