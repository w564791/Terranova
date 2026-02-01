package service

import (
	"context"
	"errors"

	"iac-platform/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserService 用户服务
type UserService struct {
	db *gorm.DB
}

// NewUserService 创建用户服务实例
func NewUserService(db *gorm.DB) *UserService {
	return &UserService{
		db: db,
	}
}

// ListUsersRequest 列出用户请求
type ListUsersRequest struct {
	Role     string `json:"role"`
	IsActive *bool  `json:"is_active"`
	Search   string `json:"search"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Role     *string `json:"role"`
	IsActive *bool   `json:"is_active"`
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role" binding:"required,oneof=user admin"`
}

// ListUsers 列出用户
func (s *UserService) ListUsers(ctx context.Context, req *ListUsersRequest) ([]*models.User, int64, error) {
	var users []*models.User
	var total int64

	query := s.db.WithContext(ctx).Model(&models.User{})

	// 应用筛选
	if req.Role != "" {
		query = query.Where("role = ?", req.Role)
	}
	if req.IsActive != nil {
		query = query.Where("is_active = ?", *req.IsActive)
	}
	if req.Search != "" {
		searchPattern := "%" + req.Search + "%"
		query = query.Where("username LIKE ? OR email LIKE ?", searchPattern, searchPattern)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 应用分页
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}
	query = query.Limit(req.Limit).Offset(req.Offset)

	// 获取用户列表
	if err := query.Order("created_at DESC").Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// GetUser 获取用户详情
func (s *UserService) GetUser(ctx context.Context, id string) (*models.User, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("user_id = ?", id).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

// UpdateUser 更新用户
func (s *UserService) UpdateUser(ctx context.Context, id string, req *UpdateUserRequest) error {
	user, err := s.GetUser(ctx, id)
	if err != nil {
		return err
	}

	updates := make(map[string]interface{})
	if req.Role != nil {
		updates["role"] = *req.Role
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if len(updates) == 0 {
		return errors.New("no fields to update")
	}

	return s.db.WithContext(ctx).Model(user).Updates(updates).Error
}

// DeactivateUser 停用用户
func (s *UserService) DeactivateUser(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("user_id = ?", id).
		Update("is_active", false).Error
}

// ActivateUser 激活用户
func (s *UserService) ActivateUser(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("user_id = ?", id).
		Update("is_active", true).Error
}

// GetUserStats 获取用户统计
func (s *UserService) GetUserStats(ctx context.Context) (map[string]interface{}, error) {
	var total int64
	var activeCount int64
	var adminCount int64

	if err := s.db.WithContext(ctx).Model(&models.User{}).Count(&total).Error; err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("is_active = ?", true).Count(&activeCount).Error; err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("role = ?", "admin").Count(&adminCount).Error; err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total":       total,
		"active":      activeCount,
		"inactive":    total - activeCount,
		"admin_count": adminCount,
		"user_count":  total - adminCount,
	}, nil
}

// CreateUser 创建用户
func (s *UserService) CreateUser(ctx context.Context, req *CreateUserRequest) (*models.User, error) {
	// 检查用户名是否已存在
	var existingUser models.User
	if err := s.db.WithContext(ctx).Where("username = ?", req.Username).First(&existingUser).Error; err == nil {
		return nil, errors.New("username already exists")
	}

	// 检查邮箱是否已存在
	if err := s.db.WithContext(ctx).Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		return nil, errors.New("email already exists")
	}

	// 使用 bcrypt 对密码进行哈希处理
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	user := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		Role:         req.Role,
		IsActive:     true,
	}

	if err := s.db.WithContext(ctx).Create(user).Error; err != nil {
		return nil, err
	}

	return user, nil
}

// DeleteUser 删除用户
func (s *UserService) DeleteUser(ctx context.Context, id string) error {
	// 检查用户是否存在
	user, err := s.GetUser(ctx, id)
	if err != nil {
		return err
	}

	// 不允许删除管理员账户（可选的安全措施）
	if user.Role == "admin" {
		var adminCount int64
		if err := s.db.WithContext(ctx).Model(&models.User{}).Where("role = ?", "admin").Count(&adminCount).Error; err != nil {
			return err
		}
		if adminCount <= 1 {
			return errors.New("cannot delete the last admin user")
		}
	}

	// 检查用户是否有关联的数据
	// 1. 检查workspace_tasks
	var taskCount int64
	if err := s.db.WithContext(ctx).Table("workspace_tasks").Where("created_by = ?", id).Count(&taskCount).Error; err != nil {
		return err
	}
	if taskCount > 0 {
		return errors.New("cannot delete user: user has created workspace tasks. Please reassign or delete these tasks first")
	}

	// 2. 检查workspaces
	var workspaceCount int64
	if err := s.db.WithContext(ctx).Table("workspaces").Where("created_by = ?", id).Count(&workspaceCount).Error; err != nil {
		return err
	}
	if workspaceCount > 0 {
		return errors.New("cannot delete user: user has created workspaces. Please reassign or delete these workspaces first")
	}

	// 3. 检查modules
	var moduleCount int64
	if err := s.db.WithContext(ctx).Table("modules").Where("created_by = ?", id).Count(&moduleCount).Error; err != nil {
		return err
	}
	if moduleCount > 0 {
		return errors.New("cannot delete user: user has created modules. Please reassign or delete these modules first")
	}

	// 删除用户
	return s.db.WithContext(ctx).Delete(user).Error
}
