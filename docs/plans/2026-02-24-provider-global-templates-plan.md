# Provider Global Templates Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor provider settings from workspace-only to a global template system with workspace-level overrides, and remove the mandatory provider requirement for task execution.

**Architecture:** New `provider_templates` table with CRUD API under `/global/settings/`, following the same pattern as TerraformVersion. Workspaces reference templates via `provider_template_ids` and optionally override fields via `provider_overrides`. Legacy `provider_config` is preserved as fallback. When no provider is configured, tasks execute without generating `provider.tf.json`.

**Tech Stack:** Go/Gin backend, PostgreSQL with JSONB, React/TypeScript frontend, GORM ORM.

**Design doc:** `docs/plans/2026-02-24-provider-global-templates-design.md`

---

### Task 1: Database Migration — Create provider_templates table

**Files:**
- Create: `backend/migrations/add_provider_templates.sql`

**Step 1: Write the migration SQL**

```sql
-- Create provider_templates table for global provider configuration management
CREATE TABLE IF NOT EXISTS public.provider_templates (
    id SERIAL PRIMARY KEY,
    name character varying(100) NOT NULL,
    type character varying(50) NOT NULL,
    source character varying(200) NOT NULL,
    config jsonb NOT NULL DEFAULT '{}',
    version character varying(50),
    constraint_op character varying(10),
    is_default boolean DEFAULT false,
    enabled boolean DEFAULT true,
    description text,
    created_by integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

-- Unique constraint on name
CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_templates_name ON public.provider_templates (name);

-- Index for type lookups
CREATE INDEX IF NOT EXISTS idx_provider_templates_type ON public.provider_templates (type);

-- Add workspace columns for template references
ALTER TABLE public.workspaces
    ADD COLUMN IF NOT EXISTS provider_template_ids jsonb,
    ADD COLUMN IF NOT EXISTS provider_overrides jsonb;

COMMENT ON TABLE public.provider_templates IS 'Global provider configuration templates';
COMMENT ON COLUMN public.provider_templates.type IS 'Provider type name: aws, kubernetes, tencentcloud, ode, etc.';
COMMENT ON COLUMN public.provider_templates.source IS 'Terraform registry source: hashicorp/aws, IBM/ode, etc.';
COMMENT ON COLUMN public.provider_templates.config IS 'Provider block configuration (auth, endpoints, etc.)';
COMMENT ON COLUMN public.provider_templates.version IS 'Provider version number (optional)';
COMMENT ON COLUMN public.provider_templates.constraint_op IS 'Version constraint operator: ~>, >=, =, etc. (optional)';
COMMENT ON COLUMN public.workspaces.provider_template_ids IS 'JSON array of referenced provider template IDs';
COMMENT ON COLUMN public.workspaces.provider_overrides IS 'Per-type field overrides applied on top of templates';
```

**Step 2: Apply migration to local database**

Run: `psql -U <user> -d <database> -f backend/migrations/add_provider_templates.sql`
Expected: Tables/columns created without errors.

**Step 3: Update init_seed_data.sql for fresh installs**

Add the `provider_templates` table DDL to `manifests/db/init_seed_data.sql` alongside the existing `terraform_versions` table definition.

**Step 4: Commit**

```bash
git add backend/migrations/add_provider_templates.sql manifests/db/init_seed_data.sql
git commit -m "feat(db): add provider_templates table and workspace reference columns"
```

---

### Task 2: Backend Model — ProviderTemplate

**Files:**
- Create: `backend/internal/models/provider_template.go`
- Modify: `backend/internal/models/workspace.go:186-187` (add new fields after ProviderConfig)

**Step 1: Create the ProviderTemplate model**

Create `backend/internal/models/provider_template.go`:

```go
package models

import "time"

// ProviderTemplate 全局Provider配置模板
type ProviderTemplate struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"type:varchar(100);not null;uniqueIndex"`
	Type        string    `json:"type" gorm:"type:varchar(50);not null;index"`
	Source      string    `json:"source" gorm:"type:varchar(200);not null"`
	Config      JSONB     `json:"config" gorm:"type:jsonb;not null;default:'{}'"`
	Version     string    `json:"version" gorm:"type:varchar(50)"`
	ConstraintOp string   `json:"constraint_op" gorm:"column:constraint_op;type:varchar(10)"`
	IsDefault   bool      `json:"is_default" gorm:"default:false"`
	Enabled     bool      `json:"enabled" gorm:"default:true"`
	Description string    `json:"description" gorm:"type:text"`
	CreatedBy   *uint     `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (ProviderTemplate) TableName() string {
	return "provider_templates"
}

// CreateProviderTemplateRequest 创建Provider模板请求
type CreateProviderTemplateRequest struct {
	Name         string                 `json:"name" binding:"required"`
	Type         string                 `json:"type" binding:"required"`
	Source       string                 `json:"source" binding:"required"`
	Config       map[string]interface{} `json:"config" binding:"required"`
	Version      string                 `json:"version"`
	ConstraintOp string                 `json:"constraint_op"`
	Enabled      bool                   `json:"enabled"`
	Description  string                 `json:"description"`
}

// UpdateProviderTemplateRequest 更新Provider模板请求
type UpdateProviderTemplateRequest struct {
	Name         *string                 `json:"name"`
	Type         *string                 `json:"type"`
	Source       *string                 `json:"source"`
	Config       map[string]interface{}  `json:"config"`
	Version      *string                 `json:"version"`
	ConstraintOp *string                 `json:"constraint_op"`
	Enabled      *bool                   `json:"enabled"`
	Description  *string                 `json:"description"`
}

// ProviderTemplateListResponse Provider模板列表响应
type ProviderTemplateListResponse struct {
	Items []ProviderTemplate `json:"items"`
	Total int                `json:"total"`
}
```

**Step 2: Add new fields to Workspace model**

In `backend/internal/models/workspace.go`, after line 187 (`ProviderConfig JSONB ...`), add:

```go
	// Provider模板引用
	ProviderTemplateIDs JSONB `json:"provider_template_ids" gorm:"type:jsonb"` // 引用的全局模板ID列表
	ProviderOverrides   JSONB `json:"provider_overrides" gorm:"type:jsonb"`    // 按provider类型的字段覆盖
```

**Step 3: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Compiles without errors.

**Step 4: Commit**

```bash
git add backend/internal/models/provider_template.go backend/internal/models/workspace.go
git commit -m "feat(models): add ProviderTemplate model and workspace reference fields"
```

---

### Task 3: Backend Service — ProviderTemplateService

**Files:**
- Create: `backend/services/provider_template_service.go`

**Step 1: Create the service with CRUD + SetDefault + ResolveConfig**

Create `backend/services/provider_template_service.go`. This follows the exact pattern of `terraform_version_service.go`:

```go
package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"time"

	"gorm.io/gorm"
)

// ProviderTemplateService Provider模板服务
type ProviderTemplateService struct {
	db *gorm.DB
}

// NewProviderTemplateService 创建Provider模板服务
func NewProviderTemplateService(db *gorm.DB) *ProviderTemplateService {
	return &ProviderTemplateService{db: db}
}

// List 获取Provider模板列表
func (s *ProviderTemplateService) List(enabled *bool, providerType string) ([]models.ProviderTemplate, error) {
	var templates []models.ProviderTemplate
	query := s.db.Order("is_default DESC, created_at DESC")

	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}
	if providerType != "" {
		query = query.Where("type = ?", providerType)
	}

	if err := query.Find(&templates).Error; err != nil {
		return nil, err
	}
	if templates == nil {
		templates = []models.ProviderTemplate{}
	}
	return templates, nil
}

// GetByID 根据ID获取模板
func (s *ProviderTemplateService) GetByID(id uint) (*models.ProviderTemplate, error) {
	var template models.ProviderTemplate
	if err := s.db.First(&template, id).Error; err != nil {
		return nil, err
	}
	return &template, nil
}

// GetByIDs 根据ID列表获取模板
func (s *ProviderTemplateService) GetByIDs(ids []uint) ([]models.ProviderTemplate, error) {
	var templates []models.ProviderTemplate
	if len(ids) == 0 {
		return templates, nil
	}
	if err := s.db.Where("id IN ?", ids).Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

// Create 创建模板
func (s *ProviderTemplateService) Create(req *models.CreateProviderTemplateRequest) (*models.ProviderTemplate, error) {
	// 检查名称是否重复
	var count int64
	s.db.Model(&models.ProviderTemplate{}).Where("name = ?", req.Name).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("provider template with name '%s' already exists", req.Name)
	}

	template := &models.ProviderTemplate{
		Name:         req.Name,
		Type:         req.Type,
		Source:       req.Source,
		Config:       models.JSONB(req.Config),
		Version:      req.Version,
		ConstraintOp: req.ConstraintOp,
		Enabled:      req.Enabled,
		Description:  req.Description,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.db.Create(template).Error; err != nil {
		return nil, err
	}
	return template, nil
}

// Update 更新模板
func (s *ProviderTemplateService) Update(id uint, req *models.UpdateProviderTemplateRequest) (*models.ProviderTemplate, error) {
	updates := make(map[string]interface{})

	if req.Name != nil {
		// 检查名称唯一性
		var count int64
		s.db.Model(&models.ProviderTemplate{}).Where("name = ? AND id != ?", *req.Name, id).Count(&count)
		if count > 0 {
			return nil, fmt.Errorf("provider template with name '%s' already exists", *req.Name)
		}
		updates["name"] = *req.Name
	}
	if req.Type != nil {
		updates["type"] = *req.Type
	}
	if req.Source != nil {
		updates["source"] = *req.Source
	}
	if req.Config != nil {
		updates["config"] = models.JSONB(req.Config)
	}
	if req.Version != nil {
		updates["version"] = *req.Version
	}
	if req.ConstraintOp != nil {
		updates["constraint_op"] = *req.ConstraintOp
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	updates["updated_at"] = time.Now()

	if err := s.db.Model(&models.ProviderTemplate{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}

	return s.GetByID(id)
}

// SetDefault 设置某个type的默认模板（原子事务）
func (s *ProviderTemplateService) SetDefault(id uint) (*models.ProviderTemplate, error) {
	template, err := s.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("provider template not found")
	}

	if !template.Enabled {
		return nil, fmt.Errorf("cannot set disabled template as default")
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 清除同类型的所有默认标记
		if err := tx.Model(&models.ProviderTemplate{}).
			Where("type = ? AND is_default = ?", template.Type, true).
			Update("is_default", false).Error; err != nil {
			return err
		}
		// 设置新的默认
		if err := tx.Model(&models.ProviderTemplate{}).
			Where("id = ?", id).
			Update("is_default", true).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

// Delete 删除模板（检查使用中）
func (s *ProviderTemplateService) Delete(id uint) error {
	template, err := s.GetByID(id)
	if err != nil {
		return fmt.Errorf("provider template not found")
	}

	if template.IsDefault {
		return fmt.Errorf("cannot delete default provider template, set another as default first")
	}

	if s.CheckTemplateInUse(id) {
		return fmt.Errorf("cannot delete provider template that is in use by workspaces")
	}

	return s.db.Delete(&models.ProviderTemplate{}, id).Error
}

// CheckTemplateInUse 检查模板是否被workspace使用
func (s *ProviderTemplateService) CheckTemplateInUse(id uint) bool {
	var count int64
	// 查询 provider_template_ids JSONB 数组中包含该 ID 的 workspace
	s.db.Model(&models.Workspace{}).
		Where("provider_template_ids @> ?::jsonb", fmt.Sprintf("[%d]", id)).
		Count(&count)
	return count > 0
}

// ResolveProviderConfig 解析最终的provider配置
// 优先级: 全局模板 < workspace覆盖
func (s *ProviderTemplateService) ResolveProviderConfig(templateIDs []uint, overrides map[string]interface{}) (map[string]interface{}, error) {
	templates, err := s.GetByIDs(templateIDs)
	if err != nil {
		return nil, err
	}

	if len(templates) == 0 {
		return nil, nil
	}

	providerMap := make(map[string]interface{})
	requiredProviders := make(map[string]interface{})

	for _, tmpl := range templates {
		// 深拷贝模板配置
		config := deepCopyJSONB(tmpl.Config)

		// 应用覆盖（浅合并）
		if overrides != nil {
			if override, ok := overrides[tmpl.Type]; ok {
				if overrideMap, ok := override.(map[string]interface{}); ok {
					for k, v := range overrideMap {
						config[k] = v
					}
				}
			}
		}

		providerMap[tmpl.Type] = []interface{}{config}

		// 构建 required_providers
		if tmpl.Source != "" {
			rp := map[string]interface{}{"source": tmpl.Source}
			if tmpl.Version != "" && tmpl.ConstraintOp != "" {
				if tmpl.ConstraintOp == "=" {
					rp["version"] = tmpl.Version
				} else {
					rp["version"] = tmpl.ConstraintOp + " " + tmpl.Version
				}
			}
			requiredProviders[tmpl.Type] = rp
		}
	}

	result := map[string]interface{}{
		"provider": providerMap,
	}

	if len(requiredProviders) > 0 {
		result["terraform"] = []interface{}{
			map[string]interface{}{"required_providers": []interface{}{requiredProviders}},
		}
	}

	return result, nil
}

// deepCopyJSONB 深拷贝JSONB
func deepCopyJSONB(src models.JSONB) map[string]interface{} {
	data, err := json.Marshal(src)
	if err != nil {
		return make(map[string]interface{})
	}
	var dst map[string]interface{}
	if err := json.Unmarshal(data, &dst); err != nil {
		return make(map[string]interface{})
	}
	return dst
}

// FilterTemplateSensitiveInfo 过滤模板中的敏感信息
func (s *ProviderTemplateService) FilterTemplateSensitiveInfo(config map[string]interface{}) map[string]interface{} {
	filtered := deepCopyJSONB(models.JSONB(config))

	// 通用敏感字段列表
	sensitiveKeys := []string{
		"access_key", "secret_key", "secret_id",
		"password", "token", "client_key", "client_secret",
	}

	for key := range filtered {
		for _, sk := range sensitiveKeys {
			if key == sk {
				filtered[key] = "***HIDDEN***"
			}
		}
		// 检查字段名是否包含敏感关键词
		for _, keyword := range []string{"password", "secret", "token", "key"} {
			if len(key) > len(keyword) && containsIgnoreCase(key, keyword) {
				filtered[key] = "***HIDDEN***"
			}
		}
	}

	return filtered
}

func containsIgnoreCase(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
```

**Step 2: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Compiles without errors.

**Step 3: Commit**

```bash
git add backend/services/provider_template_service.go
git commit -m "feat(services): add ProviderTemplateService with CRUD, SetDefault, and ResolveConfig"
```

---

### Task 4: Backend Controller — ProviderTemplateController

**Files:**
- Create: `backend/controllers/provider_template_controller.go`

**Step 1: Create the controller**

Create `backend/controllers/provider_template_controller.go` following the same pattern as `terraform_version_controller.go`:

```go
package controllers

import (
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ProviderTemplateController Provider模板控制器
type ProviderTemplateController struct {
	service *services.ProviderTemplateService
}

// NewProviderTemplateController 创建控制器
func NewProviderTemplateController(db *gorm.DB) *ProviderTemplateController {
	return &ProviderTemplateController{
		service: services.NewProviderTemplateService(db),
	}
}

// ListProviderTemplates 获取Provider模板列表
func (c *ProviderTemplateController) ListProviderTemplates(ctx *gin.Context) {
	var enabled *bool
	if e := ctx.Query("enabled"); e != "" {
		val := e == "true"
		enabled = &val
	}
	providerType := ctx.Query("type")

	templates, err := c.service.List(enabled, providerType)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"items": templates,
		"total": len(templates),
	})
}

// GetProviderTemplate 获取单个模板
func (c *ProviderTemplateController) GetProviderTemplate(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid template ID"})
		return
	}

	template, err := c.service.GetByID(uint(id))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "provider template not found"})
		return
	}

	// 过滤敏感信息
	template.Config = models.JSONB(c.service.FilterTemplateSensitiveInfo(template.Config))
	ctx.JSON(http.StatusOK, template)
}

// CreateProviderTemplate 创建模板
func (c *ProviderTemplateController) CreateProviderTemplate(ctx *gin.Context) {
	var req models.CreateProviderTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	template, err := c.service.Create(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, template)
}

// UpdateProviderTemplate 更新模板
func (c *ProviderTemplateController) UpdateProviderTemplate(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid template ID"})
		return
	}

	var req models.UpdateProviderTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	template, err := c.service.Update(uint(id), &req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, template)
}

// SetDefaultTemplate 设置默认模板
func (c *ProviderTemplateController) SetDefaultTemplate(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid template ID"})
		return
	}

	template, err := c.service.SetDefault(uint(id))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, template)
}

// DeleteProviderTemplate 删除模板
func (c *ProviderTemplateController) DeleteProviderTemplate(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid template ID"})
		return
	}

	if err := c.service.Delete(uint(id)); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusNoContent, nil)
}
```

**Step 2: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Compiles without errors.

**Step 3: Commit**

```bash
git add backend/controllers/provider_template_controller.go
git commit -m "feat(controllers): add ProviderTemplateController with CRUD endpoints"
```

---

### Task 5: Route Registration

**Files:**
- Modify: `backend/internal/router/router_global.go:53` (add after terraform-versions routes)

**Step 1: Register provider template routes**

In `backend/internal/router/router_global.go`, after line 52 (the closing `)` of the last terraform-versions DELETE route), add:

```go
		// Provider模板管理
		ptController := controllers.NewProviderTemplateController(db)

		globalSettings.GET("/provider-templates",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "READ"),
			ptController.ListProviderTemplates,
		)

		globalSettings.GET("/provider-templates/:id",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "READ"),
			ptController.GetProviderTemplate,
		)

		globalSettings.POST("/provider-templates",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "WRITE"),
			ptController.CreateProviderTemplate,
		)

		globalSettings.PUT("/provider-templates/:id",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "WRITE"),
			ptController.UpdateProviderTemplate,
		)

		globalSettings.POST("/provider-templates/:id/set-default",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "ADMIN"),
			ptController.SetDefaultTemplate,
		)

		globalSettings.DELETE("/provider-templates/:id",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "ADMIN"),
			ptController.DeleteProviderTemplate,
		)
```

**Step 2: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Compiles without errors.

**Step 3: Commit**

```bash
git add backend/internal/router/router_global.go
git commit -m "feat(router): register provider template API routes"
```

---

### Task 6: Remove Mandatory Provider Check

**Files:**
- Modify: `backend/controllers/workspace_task_controller.go:135-141`
- Modify: `backend/services/terraform_executor.go:4226-4228`

**Step 1: Remove task creation block**

In `backend/controllers/workspace_task_controller.go`, remove lines 135-141:

```go
	// 检查workspace是否配置了provider
	if workspace.ProviderConfig == nil || len(workspace.ProviderConfig) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Workspace has no provider configuration. Please configure a provider before creating tasks.",
		})
		return
	}
```

Replace with a log message:

```go
	// Provider配置可选 - 如果没有配置provider，terraform将使用module自带配置或环境变量
	if workspace.ProviderConfig == nil || len(workspace.ProviderConfig) == 0 {
		log.Printf("Workspace %s has no provider config, tasks will run without provider.tf.json", workspace.WorkspaceID)
	}
```

**Step 2: Make SnapshotProviderConfig nil-safe**

In `backend/services/terraform_executor.go`, change lines 4226-4228 from:

```go
	if planTask.SnapshotProviderConfig == nil {
		return fmt.Errorf("snapshot provider config missing")
	}
```

To:

```go
	if planTask.SnapshotProviderConfig == nil {
		logger.Info("No snapshot provider config (provider.tf.json will not be generated)")
	}
```

**Step 3: Skip provider.tf.json when config is nil**

In `backend/services/terraform_executor.go`, find the 3 places where `provider.tf.json` is generated (lines ~199, ~3357, ~4949). Each currently looks like:

```go
	cleanedProviderConfig := s.cleanProviderConfig(workspace.ProviderConfig)
	if err := s.writeJSONFile(workDir, "provider.tf.json", cleanedProviderConfig); err != nil {
		return fmt.Errorf("failed to write provider.tf.json: %w", err)
	}
```

Wrap each with a nil check:

```go
	if workspace.ProviderConfig != nil && len(workspace.ProviderConfig) > 0 {
		cleanedProviderConfig := s.cleanProviderConfig(workspace.ProviderConfig)
		if err := s.writeJSONFile(workDir, "provider.tf.json", cleanedProviderConfig); err != nil {
			return fmt.Errorf("failed to write provider.tf.json: %w", err)
		}
	} else {
		logger.Info("No provider config - skipping provider.tf.json generation")
	}
```

Apply this pattern to all 3 occurrences.

**Step 4: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Compiles without errors.

**Step 5: Commit**

```bash
git add backend/controllers/workspace_task_controller.go backend/services/terraform_executor.go
git commit -m "fix: allow task execution without provider configuration"
```

---

### Task 7: Workspace Controller — Handle Template References

**Files:**
- Modify: `backend/controllers/workspace_controller.go:446-539`

**Step 1: Extend update request struct**

In `backend/controllers/workspace_controller.go`, add new fields to the `req` struct (after line 458):

```go
		ProviderTemplateIDs []uint                 `json:"provider_template_ids"`
		ProviderOverrides   map[string]interface{} `json:"provider_overrides"`
```

**Step 2: Add template resolution logic**

After line 536 (the `NotifySettings` handling), add:

```go
	// 处理provider模板引用
	if req.ProviderTemplateIDs != nil {
		updates["provider_template_ids"] = req.ProviderTemplateIDs

		// 解析并缓存最终的provider_config
		ptService := services.NewProviderTemplateService(wc.db)
		resolvedConfig, err := ptService.ResolveProviderConfig(req.ProviderTemplateIDs, req.ProviderOverrides)
		if err != nil {
			log.Printf("Failed to resolve provider config: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "Provider模板解析失败",
				"error":   err.Error(),
			})
			return
		}

		if resolvedConfig != nil {
			updates["provider_config"] = resolvedConfig
			hash := calculateProviderConfigHash(resolvedConfig)
			if hash != "" {
				updates["provider_config_hash"] = hash
			}
		} else {
			updates["provider_config"] = nil
			updates["provider_config_hash"] = ""
		}
	}
	if req.ProviderOverrides != nil {
		updates["provider_overrides"] = req.ProviderOverrides
	}
```

Note: The controller needs access to `db`. Check if `wc` (WorkspaceController) already has a `db` field. If not, the `ProviderTemplateService` can be initialized differently — check the controller struct definition and constructor. Use whichever DB reference is available (e.g., `wc.workspaceService` may expose the DB, or the controller may already store it).

**Step 3: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Compiles without errors.

**Step 4: Commit**

```bash
git add backend/controllers/workspace_controller.go
git commit -m "feat(workspace): handle provider template references in workspace update"
```

---

### Task 8: Update ProviderService for Arbitrary Types

**Files:**
- Modify: `backend/services/provider_service.go:67-111`

**Step 1: Replace hardcoded type validation with generic validation**

Replace the `validateProviderFields` method (lines 67-111) with:

```go
// validateProviderFields 验证Provider字段（通用验证，支持任意类型）
func (s *ProviderService) validateProviderFields(providerType string, config map[string]interface{}) error {
	// AWS-specific validation (保留已有逻辑)
	if providerType == "aws" {
		if _, ok := config["region"]; !ok {
			return fmt.Errorf("region is required for AWS provider")
		}
		if accessKey, hasAccessKey := config["access_key"]; hasAccessKey {
			if accessKey == "" {
				return fmt.Errorf("access_key cannot be empty")
			}
			if _, hasSecretKey := config["secret_key"]; !hasSecretKey {
				return fmt.Errorf("secret_key is required when access_key is provided")
			}
		}
		if assumeRole, hasAssumeRole := config["assume_role"]; hasAssumeRole {
			if roleList, ok := assumeRole.([]interface{}); ok && len(roleList) > 0 {
				if role, ok := roleList[0].(map[string]interface{}); ok {
					if roleArn, ok := role["role_arn"].(string); !ok || roleArn == "" {
						return fmt.Errorf("role_arn is required in assume_role configuration")
					}
				}
			}
		}
	}

	// 其他类型不做强制字段验证（任意provider均可通过）
	return nil
}
```

This removes the "not yet supported" errors for azure, google, alicloud, and allows any provider type to pass validation.

**Step 2: Update BuildProviderTFJSON to handle nil gracefully**

In `backend/services/provider_service.go`, update `BuildProviderTFJSON` (line 173):

```go
func (s *ProviderService) BuildProviderTFJSON(workspace *models.Workspace) (map[string]interface{}, error) {
	if workspace.ProviderConfig == nil || len(workspace.ProviderConfig) == 0 {
		return nil, nil // No provider config is valid
	}

	if err := s.ValidateProviderConfig(workspace.ProviderConfig); err != nil {
		return nil, fmt.Errorf("invalid provider config: %w", err)
	}

	return workspace.ProviderConfig, nil
}
```

**Step 3: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Compiles without errors.

**Step 4: Commit**

```bash
git add backend/services/provider_service.go
git commit -m "fix(provider): support arbitrary provider types and optional config"
```

---

### Task 9: Frontend API Service — Provider Templates

**Files:**
- Modify: `frontend/src/services/admin.ts` (add provider template types and API methods)

**Step 1: Add types and API methods**

Append to `frontend/src/services/admin.ts` (after the existing `adminService` object, or extend it):

```typescript
// Provider模板
export interface ProviderTemplate {
  id: number;
  name: string;
  type: string;
  source: string;
  config: Record<string, any>;
  version: string;
  constraint_op: string;
  is_default: boolean;
  enabled: boolean;
  description: string;
  created_by: number | null;
  created_at: string;
  updated_at: string;
}

export interface CreateProviderTemplateRequest {
  name: string;
  type: string;
  source: string;
  config: Record<string, any>;
  version?: string;
  constraint_op?: string;
  enabled?: boolean;
  description?: string;
}

export interface UpdateProviderTemplateRequest {
  name?: string;
  type?: string;
  source?: string;
  config?: Record<string, any>;
  version?: string;
  constraint_op?: string;
  enabled?: boolean;
  description?: string;
}

export interface ProviderTemplatesResponse {
  items: ProviderTemplate[];
  total: number;
}
```

Add to the `adminService` object (before the closing `}`):

```typescript
  // Provider模板 CRUD
  getProviderTemplates: async (params?: {
    enabled?: boolean;
    type?: string;
  }): Promise<ProviderTemplatesResponse> => {
    const response = await api.get('/global/settings/provider-templates', { params });
    return response.data || response;
  },

  getProviderTemplate: async (id: number): Promise<ProviderTemplate> => {
    const response = await api.get(`/global/settings/provider-templates/${id}`);
    return response.data;
  },

  createProviderTemplate: async (
    data: CreateProviderTemplateRequest
  ): Promise<ProviderTemplate> => {
    const response = await api.post('/global/settings/provider-templates', data);
    return response.data;
  },

  updateProviderTemplate: async (
    id: number,
    data: UpdateProviderTemplateRequest
  ): Promise<ProviderTemplate> => {
    const response = await api.put(`/global/settings/provider-templates/${id}`, data);
    return response.data;
  },

  deleteProviderTemplate: async (id: number): Promise<void> => {
    await api.delete(`/global/settings/provider-templates/${id}`);
  },

  setDefaultProviderTemplate: async (id: number): Promise<ProviderTemplate> => {
    const response = await api.post(`/global/settings/provider-templates/${id}/set-default`);
    return response.data;
  },
```

**Step 2: Commit**

```bash
git add frontend/src/services/admin.ts
git commit -m "feat(frontend): add provider template API service methods"
```

---

### Task 10: Frontend Admin Page — Provider Templates Management

**Files:**
- Modify: `frontend/src/pages/Admin.tsx` (add Provider Templates section)

**Step 1: Add Provider Templates tab/section to Admin page**

This is a large UI component. Follow the exact same pattern as the IaC Engine Versions section in `Admin.tsx`. The key elements:

1. Add state for provider templates (similar to `versions` state)
2. Add a `loadProviderTemplates()` function
3. Add form state for create/edit dialog
4. Add table with columns: Name, Type, Source, Version, Status, Default, Actions
5. Add create/edit dialog with fields: name, type, source, config (JSON editor), version, constraint, enabled, description
6. Add set-default and delete functionality

**Important implementation notes:**
- The `config` field should use a JSON textarea editor (not individual form fields), since provider types are arbitrary
- Group the "Set as Default" logic per type (each type has its own default)
- Show `constraint_op + version` in the Version column (e.g., "~> 6.0")

Refer to the existing IaC Engine Versions section for exact styling patterns — use the same CSS classes from `Admin.module.css`.

**Step 2: Test locally**

Run: `cd frontend && npm run dev`
Navigate to Admin page, verify Provider Templates section renders correctly.

**Step 3: Commit**

```bash
git add frontend/src/pages/Admin.tsx frontend/src/pages/Admin.module.css
git commit -m "feat(admin): add Provider Templates management UI"
```

---

### Task 11: Frontend Workspace Settings — Provider Tab Redesign

**Files:**
- Modify: `frontend/src/pages/ProviderSettings.tsx` (redesign)
- Modify: `frontend/src/pages/ProviderSettings.module.css` (update styles)

**Step 1: Redesign ProviderSettings component**

Replace the current full-form approach with a template selection + override interface:

1. **Mode selector**: "Use Global Templates" / "Custom Configuration" / "None (use module defaults)"
2. **Template mode**:
   - Multi-select showing available templates from `adminService.getProviderTemplates({enabled: true})`
   - For each selected template, show collapsible override panel
   - "Preview" button showing resolved `provider.tf.json`
3. **Custom mode**: Keep the existing form as-is (backward compatibility)
4. **None mode**: Clear message explaining Terraform will use module defaults / env vars

**Key data flow:**
- When saving in template mode: `PATCH /workspaces/:id` with `provider_template_ids` + `provider_overrides`
- When saving in custom mode: `PATCH /workspaces/:id` with `provider_config` (existing behavior)
- When saving in none mode: `PATCH /workspaces/:id` with `provider_template_ids: []`, `provider_config: null`

**Step 2: Test locally**

Run: `cd frontend && npm run dev`
Navigate to workspace settings > provider tab, verify template selection and override functionality.

**Step 3: Commit**

```bash
git add frontend/src/pages/ProviderSettings.tsx frontend/src/pages/ProviderSettings.module.css
git commit -m "feat(workspace): redesign provider settings with template selection and override"
```

---

### Task 12: End-to-End Testing

**Files:**
- No new files; manual testing

**Step 1: Test global template CRUD**

1. Create a provider template via Admin UI
2. Verify it appears in the list
3. Edit, enable/disable, set as default
4. Verify delete blocked when in use

**Step 2: Test workspace template reference**

1. Select a global template in workspace settings
2. Add an override (e.g., different region)
3. Save and verify the resolved provider config via API
4. Run a plan task — verify `provider.tf.json` is correctly generated

**Step 3: Test no-provider execution**

1. Create a workspace with no provider configured (no templates, no custom config)
2. Upload a module that has its own provider block
3. Run a plan task — verify it executes without error
4. Verify no `provider.tf.json` is generated

**Step 4: Test backward compatibility**

1. Verify existing workspaces with legacy `provider_config` still work
2. Run tasks on these workspaces — verify they use the existing config
3. Switch a workspace from legacy to template mode — verify transition works

**Step 5: Commit any fixes from testing**

```bash
git add -A
git commit -m "fix: address issues found during e2e testing"
```
