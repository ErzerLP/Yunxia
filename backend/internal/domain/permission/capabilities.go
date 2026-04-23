package permission

const (
	CapabilitySystemStatsRead   = "system.stats.read"
	CapabilitySystemConfigRead  = "system.config.read"
	CapabilitySystemConfigWrite = "system.config.write"

	CapabilityUserRead          = "user.read"
	CapabilityUserCreate        = "user.create"
	CapabilityUserUpdate        = "user.update"
	CapabilityUserLock          = "user.lock"
	CapabilityUserPasswordReset = "user.password.reset"
	CapabilityUserTokensRevoke  = "user.tokens.revoke"
	CapabilityUserRoleAssign    = "user.role.assign"

	CapabilityACLRead   = "acl.read"
	CapabilityACLManage = "acl.manage"

	CapabilitySourceRead       = "source.read"
	CapabilitySourceTest       = "source.test"
	CapabilitySourceCreate     = "source.create"
	CapabilitySourceUpdate     = "source.update"
	CapabilitySourceDelete     = "source.delete"
	CapabilitySourceSecretRead = "source.secret.read"

	CapabilityTaskReadAll    = "task.read_all"
	CapabilityTaskManageAll  = "task.manage_all"
	CapabilityShareReadAll   = "share.read_all"
	CapabilityShareManageAll = "share.manage_all"
)

var allCapabilities = []string{
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
	CapabilitySourceSecretRead,
	CapabilityTaskReadAll,
	CapabilityTaskManageAll,
	CapabilityShareReadAll,
	CapabilityShareManageAll,
}

func AllCapabilities() []string {
	out := make([]string, len(allCapabilities))
	copy(out, allCapabilities)
	return out
}
