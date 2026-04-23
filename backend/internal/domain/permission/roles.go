package permission

const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleOperator   = "operator"
	RoleUser       = "user"
)

func IsValidRole(roleKey string) bool {
	switch roleKey {
	case RoleSuperAdmin, RoleAdmin, RoleOperator, RoleUser:
		return true
	default:
		return false
	}
}
