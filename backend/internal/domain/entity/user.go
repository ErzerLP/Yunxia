package entity

import "time"

// User 表示系统用户。
type User struct {
	ID           uint
	Username     string
	Email        string
	PasswordHash string
	RoleKey      string
	Status       string
	TokenVersion int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
