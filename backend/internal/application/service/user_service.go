package service

import (
	"context"
	"errors"
	"strings"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
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
	if query.Status != "" && query.Status != "active" && query.Status != "locked" {
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
	role, err := normalizeInputRole(req.Role)
	if err != nil {
		return nil, err
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
		Role:         role,
		IsLocked:     false,
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
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	role, err := normalizeInputRole(req.Role)
	if err != nil {
		return nil, err
	}
	isLocked, err := normalizeInputStatus(req.Status)
	if err != nil {
		return nil, err
	}

	user.Email = req.Email
	user.Role = role
	user.IsLocked = isLocked
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
		Role:        outputRole(user.Role),
		Status:      outputStatus(user.IsLocked),
		LastLoginAt: lastLoginAt,
		CreatedAt:   user.CreatedAt.Format(time.RFC3339),
	}
}

func normalizeInputRole(role string) (string, error) {
	switch strings.TrimSpace(role) {
	case "admin":
		return "admin", nil
	case "normal", "user":
		return "user", nil
	default:
		return "", ErrUserRoleInvalid
	}
}

func normalizeInputStatus(status string) (bool, error) {
	switch strings.TrimSpace(status) {
	case "active":
		return false, nil
	case "locked":
		return true, nil
	default:
		return false, ErrUserStatusInvalid
	}
}

func outputRole(role string) string {
	if role == "admin" {
		return "admin"
	}
	return "normal"
}

func outputStatus(isLocked bool) string {
	if isLocked {
		return "locked"
	}
	return "active"
}
