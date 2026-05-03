package entity

import "time"

// NotificationChannel 表示一个通知投递通道。
type NotificationChannel struct {
	ID         uint
	Name       string
	Type       string
	IsEnabled  bool
	EventTypes []string
	Config     NotificationChannelConfig
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NotificationChannelConfig 保存通道私有配置。
type NotificationChannelConfig struct {
	WebhookURL string `json:"webhook_url,omitempty"`
	Secret     string `json:"secret,omitempty"`
}

// NotificationEvent 表示一次已持久化的通知事件。
type NotificationEvent struct {
	ID            uint
	UserID        uint
	EventType     string
	Severity      string
	Title         string
	Message       string
	PayloadJSON   string
	Status        string
	Attempts      int
	MaxAttempts   int
	LastAttemptAt *time.Time
	NextAttemptAt *time.Time
	DeliveredAt   *time.Time
	LastError     *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
