package permission

import "fmt"

func ResolveCapabilities(roleKey string) ([]string, error) {
	switch roleKey {
	case RoleSuperAdmin:
		return AllCapabilities(), nil
	case RoleAdmin:
		return []string{
			CapabilitySystemStatsRead,
			CapabilitySystemConfigRead,
			CapabilitySystemConfigWrite,
			CapabilityUserRead,
			CapabilityUserCreate,
			CapabilityUserUpdate,
			CapabilityUserLock,
			CapabilityUserPasswordReset,
			CapabilityUserTokensRevoke,
			CapabilityUserRoleAssign,
			CapabilityACLRead,
			CapabilityACLManage,
			CapabilitySourceRead,
			CapabilitySourceTest,
			CapabilitySourceCreate,
			CapabilitySourceUpdate,
			CapabilitySourceDelete,
			CapabilityTaskReadAll,
			CapabilityTaskManageAll,
			CapabilityShareReadAll,
			CapabilityShareManageAll,
		}, nil
	case RoleOperator:
		return []string{
			CapabilitySystemStatsRead,
			CapabilitySourceRead,
			CapabilitySourceTest,
			CapabilityTaskReadAll,
			CapabilityTaskManageAll,
		}, nil
	case RoleUser:
		return []string{}, nil
	default:
		return nil, fmt.Errorf("invalid role_key: %s", roleKey)
	}
}
