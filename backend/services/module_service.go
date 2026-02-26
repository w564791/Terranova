package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"

	"gorm.io/gorm"
)

type ModuleService struct {
	db *gorm.DB
}

func NewModuleService(db *gorm.DB) *ModuleService {
	return &ModuleService{db: db}
}

func (ms *ModuleService) GetModules(page, size int, provider, search string) ([]models.Module, int64, error) {
	var modules []models.Module
	var total int64

	query := ms.db.Model(&models.Module{})

	if provider != "" {
		query = query.Where("provider = ?", provider)
	}

	if search != "" {
		query = query.Where("name ILIKE ? OR description ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * size
	if err := query.Offset(offset).Limit(size).Find(&modules).Error; err != nil {
		return nil, 0, err
	}

	// 填充默认版本信息
	for i := range modules {
		ms.fillDefaultVersionInfo(&modules[i])
	}

	return modules, total, nil
}

// fillDefaultVersionInfo 填充模块的默认版本信息
func (ms *ModuleService) fillDefaultVersionInfo(module *models.Module) {
	if module.DefaultVersionID == nil || *module.DefaultVersionID == "" {
		return
	}

	var defaultVersion models.ModuleVersion
	if err := ms.db.Where("id = ?", *module.DefaultVersionID).First(&defaultVersion).Error; err == nil {
		// 使用默认版本的版本号覆盖 module.Version
		module.Version = defaultVersion.Version
	}
}

func (ms *ModuleService) GetModuleByID(id uint) (*models.Module, error) {
	var module models.Module
	if err := ms.db.First(&module, id).Error; err != nil {
		return nil, err
	}
	// 填充默认版本信息
	ms.fillDefaultVersionInfo(&module)
	return &module, nil
}

func (ms *ModuleService) CreateModule(module *models.Module) error {
	log.Printf("[Module] CreateModule called: name=%s, provider=%s, version=%s", module.Name, module.Provider, module.Version)

	return ms.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(module).Error; err != nil {
			log.Printf("[Module] ERROR: Failed to create module: %v", err)
			return err
		}
		log.Printf("[Module] Module created with ID=%d", module.ID)

		// Auto-create initial ModuleVersion so the module is immediately usable
		versionID := generateModuleVersionID()
		version := module.Version
		if version == "" {
			version = "1.0.0"
		}
		newVersion := &models.ModuleVersion{
			ID:           versionID,
			ModuleID:     module.ID,
			Version:      version,
			Source:       module.Source,
			ModuleSource: module.ModuleSource,
			IsDefault:    true,
			Status:       models.ModuleVersionStatusActive,
			CreatedBy:    module.CreatedBy,
		}
		if err := tx.Create(newVersion).Error; err != nil {
			log.Printf("[Module] ERROR: Failed to create initial version for module %d: %v", module.ID, err)
			return fmt.Errorf("failed to create initial version: %w", err)
		}
		log.Printf("[Module] Initial version created: id=%s, version=%s, module_id=%d", versionID, version, module.ID)

		// Set as module's default version
		if err := tx.Model(&models.Module{}).Where("id = ?", module.ID).Update("default_version_id", versionID).Error; err != nil {
			log.Printf("[Module] ERROR: Failed to set default version: %v", err)
			return fmt.Errorf("failed to set default version: %w", err)
		}
		module.DefaultVersionID = &versionID

		log.Printf("[Module] ✅ Created module %d with initial version %s (v%s), default_version_id=%s", module.ID, versionID, version, versionID)
		return nil
	})
}

func (ms *ModuleService) UpdateModule(id uint, description, version, branch, status string) error {
	updates := map[string]interface{}{}
	if description != "" {
		updates["description"] = description
	}
	if version != "" {
		updates["version"] = version
	}
	if branch != "" {
		updates["branch"] = branch
	}
	if status != "" {
		updates["status"] = status
	}

	return ms.db.Model(&models.Module{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateModuleFields 更新模块字段（支持更多字段）
func (ms *ModuleService) UpdateModuleFields(module *models.Module) error {
	return ms.db.Save(module).Error
}

func (ms *ModuleService) DeleteModule(id uint) error {
	// 先检查模块状态
	var module models.Module
	if err := ms.db.First(&module, id).Error; err != nil {
		return err
	}

	// 如果模块是活跃状态，不允许删除
	if module.Status == "active" {
		return fmt.Errorf("无法删除活跃状态的模块，请先停用该模块")
	}

	// 开启事务
	tx := ms.db.Begin()

	// TODO: active_schema_id 列废弃后可删除此行
	// 1. 清除 module_versions 的 active_schema_id 引用（避免外键冲突）
	if err := tx.Model(&models.ModuleVersion{}).Where("module_id = ?", id).
		Update("active_schema_id", nil).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 2. 清除 module 的 default_version_id 引用
	if err := tx.Model(&models.Module{}).Where("id = ?", id).
		Update("default_version_id", nil).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 3. 删除关联的 schemas
	if err := tx.Where("module_id = ?", id).Delete(&models.Schema{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 4. 删除关联的 module_versions
	if err := tx.Where("module_id = ?", id).Delete(&models.ModuleVersion{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 5. 删除 module
	if err := tx.Delete(&models.Module{}, id).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

// SyncModuleFiles 同步Module文件内容
func (ms *ModuleService) SyncModuleFiles(id uint) error {
	var module models.Module
	if err := ms.db.First(&module, id).Error; err != nil {
		return err
	}

	// 更新同步状态
	if err := ms.db.Model(&module).Update("sync_status", "syncing").Error; err != nil {
		return err
	}

	// 模拟VCS同步过程，生成模拟的Module文件内容
	moduleFiles := ms.generateMockModuleFiles(module)

	moduleFilesBytes, err := json.Marshal(moduleFiles)
	if err != nil {
		return err
	}

	// 更新Module文件内容和同步状态
	updates := map[string]interface{}{
		"module_files": moduleFilesBytes,
		"sync_status":  "synced",
		"last_sync_at": "NOW()",
	}

	return ms.db.Model(&module).Updates(updates).Error
}

// generateMockModuleFiles 生成模拟的Module文件内容
func (ms *ModuleService) generateMockModuleFiles(module models.Module) map[string]string {
	// 根据Module类型生成不同的模拟文件
	if module.Provider == "AWS" && module.Name == "s3" {
		return ms.generateS3ModuleFiles()
	}
	if module.Provider == "AWS" && module.Name == "vpc" {
		return ms.generateVPCModuleFiles()
	}

	// 默认通用Module文件
	return ms.generateGenericModuleFiles(module)
}

// generateS3ModuleFiles 生成S3 Module文件
func (ms *ModuleService) generateS3ModuleFiles() map[string]string {
	return map[string]string{
		"variables.tf": `variable "name" {
  type        = string
  description = "S3 bucket name"
  default     = null
}

variable "tags" {
  type        = map(string)
  description = "Resource tags"
  default     = {}
}

variable "force_destroy" {
  type        = bool
  description = "Force destroy bucket"
  default     = false
}

variable "lifecycle_rule" {
  type = list(object({
    enabled = bool
    id      = string
    expiration = object({
      days = number
    })
    transition = list(object({
      days          = number
      storage_class = string
    }))
  }))
  description = "Lifecycle rules for S3 bucket"
  default     = []
}

variable "versioning" {
  type = object({
    enabled = bool
  })
  description = "Versioning configuration"
  default = {
    enabled = false
  }
}

variable "cors_rule" {
  type = list(object({
    allowed_headers = list(string)
    allowed_methods = list(string)
    allowed_origins = list(string)
    expose_headers  = list(string)
    max_age_seconds = number
  }))
  description = "CORS rules for S3 bucket"
  default     = []
}

variable "logging" {
  type = object({
    target_bucket = string
    target_prefix = string
  })
  description = "Logging configuration"
  default     = null
}`,
		"main.tf": `resource "aws_s3_bucket" "this" {
  bucket        = var.name
  force_destroy = var.force_destroy
  tags          = var.tags
}

resource "aws_s3_bucket_versioning" "this" {
  count  = var.versioning != null ? 1 : 0
  bucket = aws_s3_bucket.this.id
  versioning_configuration {
    status = var.versioning.enabled ? "Enabled" : "Suspended"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "this" {
  count  = length(var.lifecycle_rule) > 0 ? 1 : 0
  bucket = aws_s3_bucket.this.id

  dynamic "rule" {
    for_each = var.lifecycle_rule
    content {
      id     = rule.value.id
      status = rule.value.enabled ? "Enabled" : "Disabled"

      expiration {
        days = rule.value.expiration.days
      }

      dynamic "transition" {
        for_each = rule.value.transition
        content {
          days          = transition.value.days
          storage_class = transition.value.storage_class
        }
      }
    }
  }
}

resource "aws_s3_bucket_cors_configuration" "this" {
  count  = length(var.cors_rule) > 0 ? 1 : 0
  bucket = aws_s3_bucket.this.id

  dynamic "cors_rule" {
    for_each = var.cors_rule
    content {
      allowed_headers = cors_rule.value.allowed_headers
      allowed_methods = cors_rule.value.allowed_methods
      allowed_origins = cors_rule.value.allowed_origins
      expose_headers  = cors_rule.value.expose_headers
      max_age_seconds = cors_rule.value.max_age_seconds
    }
  }
}

resource "aws_s3_bucket_logging" "this" {
  count         = var.logging != null ? 1 : 0
  bucket        = aws_s3_bucket.this.id
  target_bucket = var.logging.target_bucket
  target_prefix = var.logging.target_prefix
}`,
		"outputs.tf": `output "bucket_id" {
  description = "S3 bucket ID"
  value       = aws_s3_bucket.this.id
}

output "bucket_arn" {
  description = "S3 bucket ARN"
  value       = aws_s3_bucket.this.arn
}

output "bucket_domain_name" {
  description = "S3 bucket domain name"
  value       = aws_s3_bucket.this.bucket_domain_name
}`,
	}
}

// generateVPCModuleFiles 生成VPC Module文件
func (ms *ModuleService) generateVPCModuleFiles() map[string]string {
	return map[string]string{
		"variables.tf": `variable "cidr_block" {
  type        = string
  description = "VPC CIDR block"
  default     = "10.0.0.0/16"
}

variable "enable_dns_hostnames" {
  type        = bool
  description = "Enable DNS hostnames"
  default     = true
}

variable "enable_dns_support" {
  type        = bool
  description = "Enable DNS support"
  default     = true
}

variable "tags" {
  type        = map(string)
  description = "Resource tags"
  default     = {}
}`,
		"main.tf": `resource "aws_vpc" "this" {
  cidr_block           = var.cidr_block
  enable_dns_hostnames = var.enable_dns_hostnames
  enable_dns_support   = var.enable_dns_support
  tags                 = var.tags
}`,
		"outputs.tf": `output "vpc_id" {
  description = "VPC ID"
  value       = aws_vpc.this.id
}

output "vpc_cidr_block" {
  description = "VPC CIDR block"
  value       = aws_vpc.this.cidr_block
}`,
	}
}

// generateGenericModuleFiles 生成通用Module文件
func (ms *ModuleService) generateGenericModuleFiles(module models.Module) map[string]string {
	return map[string]string{
		"variables.tf": fmt.Sprintf(`variable "name" {
  type        = string
  description = "%s resource name"
}

variable "tags" {
  type        = map(string)
  description = "Resource tags"
  default     = {}
}`, module.Provider),
		"main.tf": fmt.Sprintf(`# %s %s module main configuration
# This is a generic template`, module.Provider, module.Name),
		"outputs.tf": `output "resource_id" {
  description = "Resource ID"
  value       = "placeholder"
}`,
	}
}

// GetModuleFiles 获取Module文件内容
func (ms *ModuleService) GetModuleFiles(id uint) (map[string]string, error) {
	var module models.Module
	if err := ms.db.First(&module, id).Error; err != nil {
		return nil, err
	}

	if module.ModuleFiles == nil {
		return nil, fmt.Errorf("module files not synced")
	}

	var moduleFiles map[string]string

	// 处理不同的module_files类型
	switch v := module.ModuleFiles.(type) {
	case map[string]interface{}:
		// 如果是map[string]interface{}，转换为map[string]string
		moduleFiles = make(map[string]string)
		for key, value := range v {
			if str, ok := value.(string); ok {
				moduleFiles[key] = str
			}
		}
	case []byte:
		// 如果是字节数组，解析JSON
		err := json.Unmarshal(v, &moduleFiles)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported module_files format: %T", v)
	}

	return moduleFiles, nil
}
