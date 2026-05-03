package dto

// NotificationWebhookConfigRequest 表示 webhook 通道配置输入。
type NotificationWebhookConfigRequest struct {
	URL    string  `json:"url"`
	Secret *string `json:"secret,omitempty"`
}

// NotificationChannelUpsertRequest 表示通知通道创建/更新请求。
type NotificationChannelUpsertRequest struct {
	Name       string                           `json:"name" binding:"required"`
	Type       string                           `json:"type" binding:"required"`
	IsEnabled  *bool                            `json:"is_enabled"`
	EventTypes []string                         `json:"event_types"`
	Config     NotificationWebhookConfigRequest `json:"config" binding:"required"`
}

// NotificationChannelConfigView 表示前端可见的通道配置。
type NotificationChannelConfigView struct {
	URL              string `json:"url"`
	SecretConfigured bool   `json:"secret_configured"`
}

// NotificationChannelView 表示通知通道视图。
type NotificationChannelView struct {
	ID         uint                          `json:"id"`
	Name       string                        `json:"name"`
	Type       string                        `json:"type"`
	IsEnabled  bool                          `json:"is_enabled"`
	EventTypes []string                      `json:"event_types"`
	Config     NotificationChannelConfigView `json:"config"`
	CreatedAt  string                        `json:"created_at"`
	UpdatedAt  string                        `json:"updated_at"`
}

// NotificationChannelListResponse 表示通知通道列表响应。
type NotificationChannelListResponse struct {
	Items []NotificationChannelView `json:"items"`
}

// NotificationTestResponse 表示通知测试发送响应。
type NotificationTestResponse struct {
	OK bool `json:"ok"`
}

// NotificationEventView 表示通知事件视图。
type NotificationEventView struct {
	ID            uint           `json:"id"`
	UserID        uint           `json:"user_id,omitempty"`
	EventType     string         `json:"event_type"`
	Severity      string         `json:"severity"`
	Title         string         `json:"title"`
	Message       string         `json:"message"`
	Payload       map[string]any `json:"payload"`
	Status        string         `json:"status"`
	Attempts      int            `json:"attempts"`
	MaxAttempts   int            `json:"max_attempts"`
	LastAttemptAt *string        `json:"last_attempt_at"`
	NextAttemptAt *string        `json:"next_attempt_at"`
	DeliveredAt   *string        `json:"delivered_at"`
	LastError     *string        `json:"last_error"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

// NotificationEventListResponse 表示通知事件列表响应。
type NotificationEventListResponse struct {
	Items []NotificationEventView `json:"items"`
}
