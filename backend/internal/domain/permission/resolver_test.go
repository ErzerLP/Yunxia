package permission

import (
	"reflect"
	"testing"
)

func TestResolveCapabilitiesByRole(t *testing.T) {
	cases := []struct {
		name string
		role string
		want []string
	}{
		{
			name: "super admin gets all capabilities",
			role: RoleSuperAdmin,
			want: AllCapabilities(),
		},
		{
			name: "admin excludes source secret read",
			role: RoleAdmin,
			want: []string{
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
				CapabilityAuditRead,
			},
		},
		{
			name: "operator only gets runtime capabilities",
			role: RoleOperator,
			want: []string{
				CapabilitySystemStatsRead,
				CapabilitySourceRead,
				CapabilitySourceTest,
				CapabilityTaskReadAll,
				CapabilityTaskManageAll,
			},
		},
		{
			name: "user gets no governance capabilities",
			role: RoleUser,
			want: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveCapabilities(tc.role)
			if err != nil {
				t.Fatalf("ResolveCapabilities() error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ResolveCapabilities() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOwnerOrCapabilityHelpers(t *testing.T) {
	if !CanReadTask(7, 7, nil) {
		t.Fatalf("expected owner to read own task")
	}
	if CanManageTask(7, 9, nil) {
		t.Fatalf("expected non-owner without capability to fail")
	}
	if !CanManageTask(7, 9, []string{CapabilityTaskManageAll}) {
		t.Fatalf("expected task.manage_all to grant cross-user task management")
	}
	if !CanReadShare(7, 9, []string{CapabilityShareReadAll}) {
		t.Fatalf("expected share.read_all to grant cross-user share read")
	}
	if !CanAssignRole(RoleAdmin, RoleUser) {
		t.Fatalf("expected admin to assign user role")
	}
	if CanAssignRole(RoleAdmin, RoleAdmin) {
		t.Fatalf("expected admin to be blocked from assigning admin role")
	}
}
