package entity

import "time"

// RefreshToken 表示持久化的刷新令牌。
type RefreshToken struct {
	ID        uint
	UserID    uint
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}
