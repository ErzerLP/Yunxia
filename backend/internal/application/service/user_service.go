package service

import (
	"context"
	"errors"
	"strings"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

// UserService 负责管理员用户管理。
type UserService struct {
	userRepo domainrepo.UserRepository
	hasher   passwordHasher
}

// NewUserService 创建用户管理服务。
func NewUserService(userRepo domainrepo.UserRepository, hasher passwordHasher) *UserService {
	return &UserService{
		userRepo: userRepo,
		hasher:   hasher,
	}
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
		return nil, err
	}
	if !permission.CanAssignRole(actor.RoleKey, roleKey) {
		return nil, ErrRoleAssignmentForbidden
	}
	if _, err := s.userRepo.FindByUsername(ctx, req.Username); err == nil {
		return nil, ErrUserNameConflict
	} else if !errors.Is(err, domainrepo.ErrNotFound) {
		return nil, err
	}

	passwordHash, err := s.hasher.Hash(req.Password)
	if err != nil {
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
		return nil, err
	}

	view := toUserAdminView(user)
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
		return nil, err
	}

	nextRoleKey, err := normalizeInputRoleKey(req.RoleKey)
	if err != nil {
		return nil, err
	}
	nextStatus, err := normalizeInputStatus(req.Status)
	if err != nil {
		return nil, err
	}
	if !permission.CanManageTargetRole(actor.RoleKey, user.RoleKey) || !permission.CanAssignRole(actor.RoleKey, nextRoleKey) {
		return nil, ErrRoleAssignmentForbidden
	}
	if user.RoleKey == permission.RoleSuperAdmin && (nextRoleKey != permission.RoleSuperAdmin || nextStatus == permission.StatusLocked) {
		activeSuperAdmins, err := s.countActiveSuperAdmins(ctx)
		if err != nil {
			return nil, err
		}
		if activeSuperAdmins == 1 {
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
		return nil, err
	}

	view := toUserAdminView(user)
	return &view, nil
}

// ResetPassword 重置用户密码。
func (s *UserService) ResetPassword(ctx context.Context, id uint, req appdto.ResetUserPasswordRequest) error {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	passwordHash, err := s.hasher.Hash(req.NewPassword)
	if err != nil {
		return err
	}
	user.PasswordHash = passwordHash
	return s.userRepo.Update(ctx, user)
}

// RevokeTokens 撤销用户所有访问令牌。
func (s *UserService) RevokeTokens(ctx context.Context, id uint) (*appdto.RevokeUserTokensResponse, error) {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.userRepo.UpdateTokenVersion(ctx, id, user.TokenVersion+1); err != nil {
		return nil, err
	}
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
