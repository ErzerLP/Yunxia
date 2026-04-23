package permission

const (
	StatusActive = "active"
	StatusLocked = "locked"
)

func IsValidStatus(status string) bool {
	switch status {
	case StatusActive, StatusLocked:
		return true
	default:
		return false
	}
}
