package dto

// SetupStatusResponse 表示初始化状态。
type SetupStatusResponse struct {
	IsInitialized bool `json:"is_initialized"`
	SetupRequired bool `json:"setup_required"`
	HasSuperAdmin bool `json:"has_super_admin"`
}

// SetupInitRequest 表示初始化请求。
type SetupInitRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=8"`
	Email    string `json:"email"`
}

// LoginRequest 表示登录请求。
type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=8"`
}

// RefreshRequest 表示刷新令牌请求。
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest 表示登出请求。
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// UserSummary 表示前端可见用户摘要。
type UserSummary struct {
	ID        uint   `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	RoleKey   string `json:"role_key"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// CurrentUserResponse 表示当前登录用户及能力集合。
type CurrentUserResponse struct {
	User         UserSummary `json:"user"`
	Capabilities []string    `json:"capabilities"`
}

// TokenPair 表示前端使用的令牌对。
type TokenPair struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
}

// SetupInitResponse 表示初始化响应。
type SetupInitResponse struct {
	User   UserSummary `json:"user"`
	Tokens TokenPair   `json:"tokens"`
}

// LoginResponse 表示登录响应。
type LoginResponse struct {
	User   UserSummary `json:"user"`
	Tokens TokenPair   `json:"tokens"`
}

// RefreshResponse 表示刷新响应。
type RefreshResponse struct {
	Tokens TokenPair `json:"tokens"`
}
