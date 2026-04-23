package gorm

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// UserRepository 提供用户仓储实现。
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建用户仓储。
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create 创建用户。
func (r *UserRepository) Create(ctx context.Context, user *entity.User) error {
	model := userModelFromEntity(user)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}

	*user = *userEntityFromModel(model)
	return nil
}

// FindByID 按 ID 查询用户。
func (r *UserRepository) FindByID(ctx context.Context, id uint) (*entity.User, error) {
	var model UserModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, normalizeError(err)
	}

	return userEntityFromModel(&model), nil
}

// FindByUsername 按用户名查询用户。
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*entity.User, error) {
	var model UserModel
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&model).Error; err != nil {
		return nil, normalizeError(err)
	}

	return userEntityFromModel(&model), nil
}

// List 返回筛选后的用户列表。
func (r *UserRepository) List(ctx context.Context, filter domainrepo.UserListFilter) ([]*entity.User, error) {
	query := r.db.WithContext(ctx).Model(&UserModel{})
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("username LIKE ? OR email LIKE ?", like, like)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}

	var models []UserModel
	if err := query.Order("created_at desc, id desc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*entity.User, 0, len(models))
	for index := range models {
		items = append(items, userEntityFromModel(&models[index]))
	}
	return items, nil
}

// Count 统计用户数量。
func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&UserModel{}).Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

// Update 更新用户。
func (r *UserRepository) Update(ctx context.Context, user *entity.User) error {
	model := userModelFromEntity(user)
	result := r.db.WithContext(ctx).
		Model(&UserModel{}).
		Where("id = ?", user.ID).
		Select("*").
		Omit("ID", "Username", "CreatedAt").
		Updates(model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

// UpdateTokenVersion 更新 token 版本。
func (r *UserRepository) UpdateTokenVersion(ctx context.Context, id uint, tokenVersion int) error {
	result := r.db.WithContext(ctx).Model(&UserModel{}).Where("id = ?", id).Update("token_version", tokenVersion)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}

	return nil
}

func normalizeError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainrepo.ErrNotFound
	}

	return err
}

func userModelFromEntity(user *entity.User) *UserModel {
	return &UserModel{
		ID:           user.ID,
		Username:     user.Username,
		Email:        user.Email,
		PasswordHash: user.PasswordHash,
		RoleKey:      user.RoleKey,
		Status:       user.Status,
		TokenVersion: user.TokenVersion,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
}

func userEntityFromModel(model *UserModel) *entity.User {
	return &entity.User{
		ID:           model.ID,
		Username:     model.Username,
		Email:        model.Email,
		PasswordHash: model.PasswordHash,
		RoleKey:      model.RoleKey,
		Status:       model.Status,
		TokenVersion: model.TokenVersion,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}
