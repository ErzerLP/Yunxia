package permission

func HasCapability(capabilities []string, target string) bool {
	for _, item := range capabilities {
		if item == target {
			return true
		}
	}
	return false
}

func CanAssignRole(actorRoleKey, targetRoleKey string) bool {
	switch actorRoleKey {
	case RoleSuperAdmin:
		return IsValidRole(targetRoleKey)
	case RoleAdmin:
		return targetRoleKey == RoleOperator || targetRoleKey == RoleUser
	default:
		return false
	}
}

func CanManageTargetRole(actorRoleKey, targetRoleKey string) bool {
	switch actorRoleKey {
	case RoleSuperAdmin:
		return IsValidRole(targetRoleKey)
	case RoleAdmin:
		return targetRoleKey == RoleOperator || targetRoleKey == RoleUser
	default:
		return false
	}
}

func CanReadTask(actorUserID, ownerID uint, capabilities []string) bool {
	return actorUserID == ownerID || HasCapability(capabilities, CapabilityTaskReadAll)
}

func CanManageTask(actorUserID, ownerID uint, capabilities []string) bool {
	return actorUserID == ownerID || HasCapability(capabilities, CapabilityTaskManageAll)
}

func CanReadShare(actorUserID, ownerID uint, capabilities []string) bool {
	return actorUserID == ownerID || HasCapability(capabilities, CapabilityShareReadAll)
}

func CanManageShare(actorUserID, ownerID uint, capabilities []string) bool {
	return actorUserID == ownerID || HasCapability(capabilities, CapabilityShareManageAll)
}
