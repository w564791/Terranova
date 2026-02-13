package handlers

import (
	"log"
	"net/http"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type SetupHandler struct {
	db *gorm.DB
}

func NewSetupHandler(db *gorm.DB) *SetupHandler {
	return &SetupHandler{db: db}
}

// SetupStatusResponse 系统初始化状态响应
type SetupStatusResponse struct {
	Initialized bool   `json:"initialized"`
	HasAdmin    bool   `json:"has_admin"`
	Message     string `json:"message,omitempty"`
}

// SetupInitRequest 初始化管理员请求
type SetupInitRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// GetStatus 获取系统初始化状态
// @Summary 获取系统初始化状态
// @Description 检查系统是否已完成初始化（是否存在管理员用户）
// @Tags Setup
// @Produce json
// @Success 200 {object} SetupStatusResponse
// @Router /api/v1/setup/status [get]
func (h *SetupHandler) GetStatus(c *gin.Context) {
	var count int64
	if err := h.db.Model(&models.User{}).Where("role = ? AND is_active = ?", "admin", true).Count(&count).Error; err != nil {
		log.Printf("[Setup] Failed to check admin status: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to check system status",
			"timestamp": time.Now(),
		})
		return
	}

	initialized := count > 0

	response := SetupStatusResponse{
		Initialized: initialized,
		HasAdmin:    initialized,
	}

	if !initialized {
		response.Message = "系统尚未初始化，请创建管理员账号"
	} else {
		response.Message = "系统已初始化"
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "Success",
		"data":      response,
		"timestamp": time.Now(),
	})
}

// InitAdmin 初始化管理员账号
// @Summary 初始化系统管理员
// @Description 创建第一个系统管理员账号（仅在系统未初始化时可用）
// @Tags Setup
// @Accept json
// @Produce json
// @Param request body SetupInitRequest true "管理员信息"
// @Success 201 {object} map[string]interface{} "创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 409 {object} map[string]interface{} "系统已初始化"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/setup/init [post]
func (h *SetupHandler) InitAdmin(c *gin.Context) {
	// 1. 解析请求（在获取锁之前完成参数校验，避免持锁时间过长）
	var req SetupInitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 2. 生成密码哈希（CPU密集操作，在获取锁之前完成）
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[Setup] Failed to hash password: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to hash password",
			"timestamp": time.Now(),
		})
		return
	}

	// 3. 开启事务并获取 Advisory Lock（防止并发初始化竞态条件）
	tx := h.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 使用 PostgreSQL Advisory Lock 确保同一时间只有一个初始化请求能执行
	// lock key 73657475 是 "setup" 的 ASCII 编码，仅用于标识此操作
	if err := tx.Exec("SELECT pg_advisory_xact_lock(73657475)").Error; err != nil {
		tx.Rollback()
		log.Printf("[Setup] Failed to acquire advisory lock: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to acquire initialization lock",
			"timestamp": time.Now(),
		})
		return
	}

	// 4. 在事务内检查系统是否已初始化（持有锁，安全无竞态）
	var adminCount int64
	if err := tx.Model(&models.User{}).Where("role = ? AND is_active = ?", "admin", true).Count(&adminCount).Error; err != nil {
		tx.Rollback()
		log.Printf("[Setup] Failed to check admin status: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to check system status",
			"timestamp": time.Now(),
		})
		return
	}

	if adminCount > 0 {
		tx.Rollback()
		log.Printf("[Setup] System already initialized, rejecting init request")
		c.JSON(http.StatusConflict, gin.H{
			"code":      409,
			"message":   "系统已初始化，无法重复创建管理员",
			"timestamp": time.Now(),
		})
		return
	}

	// 5. 在事务内检查用户名和邮箱是否已存在
	var existingCount int64
	if err := tx.Model(&models.User{}).Where("username = ? OR email = ?", req.Username, req.Email).Count(&existingCount).Error; err != nil {
		tx.Rollback()
		log.Printf("[Setup] Failed to check existing user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to check existing user",
			"timestamp": time.Now(),
		})
		return
	}

	if existingCount > 0 {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{
			"code":      409,
			"message":   "用户名或邮箱已存在",
			"timestamp": time.Now(),
		})
		return
	}

	// 创建用户
	user := models.User{
		Username:      req.Username,
		Email:         req.Email,
		PasswordHash:  string(hashedPassword),
		Role:          "admin",
		IsActive:      true,
		IsSystemAdmin: true,
	}

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		log.Printf("[Setup] Failed to create admin user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to create admin user",
			"timestamp": time.Now(),
		})
		return
	}

	log.Printf("[Setup] Admin user created: %s (ID: %s)", user.Username, user.ID)

	// 6. 分配 admin IAM 角色
	// 查找系统 admin 角色
	var adminRole struct {
		ID int `gorm:"column:id"`
	}
	if err := tx.Table("iam_roles").Where("name = ? AND is_system = ?", "admin", true).First(&adminRole).Error; err != nil {
		log.Printf("⚠️ [Setup] Admin IAM role not found, skipping role assignment: %v", err)
		// 不回滚，角色分配是可选的
	} else {
		// 查找默认组织 ID 用于角色分配
		var defaultOrgForRole struct {
			ID int `gorm:"column:id"`
		}
		if err := tx.Table("organizations").Where("name = ?", "default").First(&defaultOrgForRole).Error; err != nil {
			log.Printf("⚠️ [Setup] Default organization not found for role assignment, skipping: %v", err)
		} else {
			// 分配角色 - 使用正确的字段名和类型
			// scope_type 只能是 ORGANIZATION, PROJECT, WORKSPACE
			// scope_id 是 integer 类型
			iamUserRole := map[string]interface{}{
				"user_id":     user.ID,
				"role_id":     adminRole.ID,
				"scope_type":  "ORGANIZATION",
				"scope_id":    defaultOrgForRole.ID,
				"assigned_by": user.ID,
				"assigned_at": time.Now(),
			}
			if err := tx.Table("iam_user_roles").Create(&iamUserRole).Error; err != nil {
				log.Printf("⚠️ [Setup] Failed to assign admin IAM role: %v", err)
				// 不回滚，角色分配是可选的
			} else {
				log.Printf("[Setup] Admin IAM role assigned to user %s", user.Username)
			}
		}
	}

	// 7. 关联到默认组织
	var defaultOrg struct {
		ID int `gorm:"column:id"`
	}
	if err := tx.Table("organizations").Where("name = ?", "default").First(&defaultOrg).Error; err != nil {
		log.Printf("⚠️ [Setup] Default organization not found, skipping org assignment: %v", err)
	} else {
		// user_organizations 表只有 user_id, org_id, joined_at 字段
		userOrg := map[string]interface{}{
			"user_id":   user.ID,
			"org_id":    defaultOrg.ID,
			"joined_at": time.Now(),
		}
		if err := tx.Table("user_organizations").Create(&userOrg).Error; err != nil {
			log.Printf("⚠️ [Setup] Failed to assign user to default org: %v", err)
		} else {
			log.Printf("[Setup] User %s assigned to default organization", user.Username)
		}
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		log.Printf("[Setup] Failed to commit transaction: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to complete setup",
			"timestamp": time.Now(),
		})
		return
	}

	log.Printf("🎉 [Setup] System initialization completed! Admin: %s", user.Username)

	c.JSON(http.StatusCreated, gin.H{
		"code":    201,
		"message": "系统初始化成功，管理员账号已创建",
		"data": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
		},
		"timestamp": time.Now(),
	})
}
