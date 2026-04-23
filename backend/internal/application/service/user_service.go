package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	appaudit "yunxia/internal/application/audit"
	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

// UserService 负责管理员用户管理。
type UserService struct {
	userRepo      domainrepo.UserRepository
	hasher        passwordHasher
	logger        *slog.Logger
	auditRecorder *appaudit.Recorder
}

// NewUserService 创建用户管理服务。
func NewUserService(userRepo domainrepo.UserRepository, hasher passwordHasher, options ...UserServiceOption) *UserService {
	service := &UserService{
		userRepo: userRepo,
		hasher:   hasher,
		logger:   newServiceLogger("service.user"),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// List 返回用户列表。
func (s *UserService) List(ctx context.Context, query appdto.UserListQuery) (*appdto.UserListResponse, error) {
	if query.Status != "" && !permission.IsValidStatus(strings.TrimSpace(query.Status)) {
		return nil, ErrUserStatusInvalid
	}

	items, err := s.userRepo.List(ctx, domainrepo.UserListFilter{
		Keyword: strings.TrimSpace(query.Keyword),
		Status:  strings.TrimSpace(query.Status),
	})
	if err != nil {
		return nil, err
	}

	views := make([]appdto.UserAdminView, 0, len(items))
	for _, item := range items {
		views = append(views, toUserAdminView(item))
	}
	pageItems, _, _ := paginateItems(views, query.Page, query.PageSize)
	return &appdto.UserListResponse{Items: pageItems}, nil
}

// Create 创建用户。
func (s *UserService) Create(ctx context.Context, req appdto.CreateUserRequest) (*appdto.UserAdminView, error) {
	actor, err := currentRequestAuth(ctx)
	if err != nil {
		return nil, ErrPermissionDenied
	}
	roleKey, err := normalizeInputRoleKey(req.RoleKey)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "USER_ROLE_INVALID",
		})
		return nil, err
	}
	if !permission.CanAssignRole(actor.RoleKey, roleKey) {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "create",
			Result:       appaudit.ResultDenied,
			ErrorCode:    "ROLE_ASSIGNMENT_FORBIDDEN",
			Detail: map[string]any{
				"target_role_key": roleKey,
			},
		})
		return nil, ErrRoleAssignmentForbidden
	}
	if _, err := s.userRepo.FindByUsername(ctx, req.Username); err == nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "USER_NAME_CONFLICT",
		})
		return nil, ErrUserNameConflict
	} else if !errors.Is(err, domainrepo.ErrNotFound) {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
		})
		return nil, err
	}

	passwordHash, err := s.hasher.Hash(req.Password)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
		})
		return nil, err
	}
	user := &entity.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		RoleKey:      roleKey,
		Status:       permission.StatusActive,
		TokenVersion: 0,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
		})
		return nil, err
	}

	view := toUserAdminView(user)
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "user",
		Action:       "create",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(user.ID),
		After:        userAuditView(user),
	})
	return &view, nil
}

// Update 更新用户资料。
func (s *UserService) Update(ctx context.Context, id uint, req appdto.UpdateUserRequest) (*appdto.UserAdminView, error) {
	actor, err := currentRequestAuth(ctx)
	if err != nil {
		return nil, ErrPermissionDenied
	}
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "USER_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return nil, err
	}
	before := userAuditView(user)

	nextRoleKey, err := normalizeInputRoleKey(req.RoleKey)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "USER_ROLE_INVALID",
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}
	nextStatus, err := normalizeInputStatus(req.Status)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "USER_STATUS_INVALID",
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}
	if !permission.CanManageTargetRole(actor.RoleKey, user.RoleKey) || !permission.CanAssignRole(actor.RoleKey, nextRoleKey) {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "update",
			Result:       appaudit.ResultDenied,
			ErrorCode:    "ROLE_ASSIGNMENT_FORBIDDEN",
			ResourceID:   encodeUintID(id),
			Before:       before,
			Detail: map[string]any{
				"target_role_key": nextRoleKey,
				"target_status":   nextStatus,
			},
		})
		return nil, ErrRoleAssignmentForbidden
	}
	if user.RoleKey == permission.RoleSuperAdmin && (nextRoleKey != permission.RoleSuperAdmin || nextStatus == permission.StatusLocked) {
		activeSuperAdmins, err := s.countActiveSuperAdmins(ctx)
		if err != nil {
			return nil, err
		}
		if activeSuperAdmins == 1 {
			recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
				ResourceType: "user",
				Action:       "update",
				Result:       appaudit.ResultDenied,
				ErrorCode:    "LAST_SUPER_ADMIN_FORBIDDEN",
				ResourceID:   encodeUintID(id),
				Before:       before,
				Detail: map[string]any{
					"target_role_key": nextRoleKey,
					"target_status":   nextStatus,
				},
			})
			return nil, ErrLastSuperAdminForbidden
		}
	}

	changedPrivilege := user.RoleKey != nextRoleKey || user.Status != nextStatus
	user.Email = req.Email
	user.RoleKey = nextRoleKey
	user.Status = nextStatus
	if changedPrivilege {
		user.TokenVersion++
	}
	if err := s.userRepo.Update(ctx, user); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}

	view := toUserAdminView(user)
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "user",
		Action:       "update",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		Before:       before,
		After:        userAuditView(user),
	})
	return &view, nil
}

// ResetPassword 重置用户密码。
func (s *UserService) ResetPassword(ctx context.Context, id uint, req appdto.ResetUserPasswordRequest) error {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "reset_password",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "USER_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return err
	}

	passwordHash, err := s.hasher.Hash(req.NewPassword)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "reset_password",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
		})
		return err
	}
	user.PasswordHash = passwordHash
	if err := s.userRepo.Update(ctx, user); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "reset_password",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
		})
		return err
	}
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "user",
		Action:       "reset_password",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		Detail: map[string]any{
			"target_user_id":   id,
			"password_changed": true,
		},
	})
	return nil
}

// RevokeTokens 撤销用户所有访问令牌。
func (s *UserService) RevokeTokens(ctx context.Context, id uint) (*appdto.RevokeUserTokensResponse, error) {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "revoke_tokens",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "USER_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return nil, err
	}
	if err := s.userRepo.UpdateTokenVersion(ctx, id, user.TokenVersion+1); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "user",
			Action:       "revoke_tokens",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
		})
		return nil, err
	}
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "user",
		Action:       "revoke_tokens",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		Detail: map[string]any{
			"target_user_id": id,
			"revoked":        true,
		},
	})
	return &appdto.RevokeUserTokensResponse{
		ID:      id,
		Revoked: true,
	}, nil
}

func toUserAdminView(user *entity.User) appdto.UserAdminView {
	var lastLoginAt *string
	return appdto.UserAdminView{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		RoleKey:     user.RoleKey,
		Status:      user.Status,
		LastLoginAt: lastLoginAt,
		CreatedAt:   user.CreatedAt.Format(time.RFC3339),
	}
}

func normalizeInputRoleKey(roleKey string) (string, error) {
	roleKey = strings.TrimSpace(roleKey)
	if !permission.IsValidRole(roleKey) {
		return "", ErrUserRoleInvalid
	}
	return roleKey, nil
}

func normalizeInputStatus(status string) (string, error) {
	status = strings.TrimSpace(status)
	if !permission.IsValidStatus(status) {
		return "", ErrUserStatusInvalid
	}
	return status, nil
}

func currentRequestAuth(ctx context.Context) (security.RequestAuth, error) {
	auth, ok := security.RequestAuthFromContext(ctx)
	if !ok {
		return security.RequestAuth{}, ErrPermissionDenied
	}
	return auth, nil
}

func (s *UserService) countActiveSuperAdmins(ctx context.Context) (int, error) {
	users, err := s.userRepo.List(ctx, domainrepo.UserListFilter{Status: permission.StatusActive})
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range users {
		if item.RoleKey == permission.RoleSuperAdmin {
			count++
		}
	}
	return count, nil
}
