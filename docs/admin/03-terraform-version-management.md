# Terraform版本管理完整文档

> **版本**: v1.1  
> **最后更新**: 2025-10-11  
> **状态**: 后端已实现，前端待开发，新增默认版本功能

## 📋 目录

1. [功能概述](#功能概述)
2. [数据库设计](#数据库设计)
3. [后端实现](#后端实现)
4. [API接口规范](#api接口规范)
5. [前端设计](#前端设计)
6. [默认版本功能](#默认版本功能)
7. [使用场景](#使用场景)
8. [测试指南](#测试指南)

---

## 功能概述

### 核心功能

Terraform版本管理模块提供以下功能：

1. **版本CRUD** - 创建、查询、更新、删除Terraform版本
2. **版本状态管理** - 启用/禁用、标记弃用
3. **版本使用检查** - 防止删除正在使用的版本
4. **默认版本设置** ⭐ - 设置全局默认版本（新功能）
5. **版本过滤** - 按启用状态、弃用状态过滤

### 业务规则

-  版本号必须唯一
-  正在使用的版本不能删除
-  Checksum必须是64位SHA256哈希值
-  下载URL必须是有效的URL格式
-  全局只能有一个默认版本 ⭐
-  设置新默认版本时自动取消旧的默认版本 ⭐

---

## 数据库设计

### terraform_versions表

```sql
CREATE TABLE terraform_versions (
    id SERIAL PRIMARY KEY,
    version VARCHAR(50) NOT NULL UNIQUE,
    download_url TEXT NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    enabled BOOLEAN DEFAULT true,
    deprecated BOOLEAN DEFAULT false,
    is_default BOOLEAN DEFAULT false,  -- ⭐ 新增：是否为默认版本
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 索引
CREATE INDEX idx_terraform_versions_enabled ON terraform_versions(enabled);
CREATE INDEX idx_terraform_versions_version ON terraform_versions(version);
CREATE INDEX idx_terraform_versions_is_default ON terraform_versions(is_default);  -- ⭐ 新增

-- 唯一约束：确保只有一个默认版本
CREATE UNIQUE INDEX idx_terraform_versions_unique_default 
ON terraform_versions(is_default) 
WHERE is_default = true;  -- ⭐ 新增

-- 默认数据
INSERT INTO terraform_versions (version, download_url, checksum, enabled, is_default) VALUES
('1.6.0', 'https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_linux_amd64.zip', 'abc123...', true, true),  -- 默认版本
('1.5.7', 'https://releases.hashicorp.com/terraform/1.5.7/terraform_1.5.7_linux_amd64.zip', 'def456...', true, false),
('1.4.6', 'https://releases.hashicorp.com/terraform/1.4.6/terraform_1.4.6_linux_amd64.zip', 'ghi789...', true, false);
```

### 数据库迁移脚本

```sql
-- scripts/add_default_version_field.sql
-- 添加is_default字段到terraform_versions表

-- 1. 添加字段
ALTER TABLE terraform_versions 
ADD COLUMN is_default BOOLEAN DEFAULT false;

-- 2. 创建索引
CREATE INDEX idx_terraform_versions_is_default ON terraform_versions(is_default);

-- 3. 创建唯一约束（确保只有一个默认版本）
CREATE UNIQUE INDEX idx_terraform_versions_unique_default 
ON terraform_versions(is_default) 
WHERE is_default = true;

-- 4. 设置第一个启用的版本为默认版本（如果还没有默认版本）
UPDATE terraform_versions 
SET is_default = true 
WHERE id = (
    SELECT id FROM terraform_versions 
    WHERE enabled = true 
    ORDER BY created_at ASC 
    LIMIT 1
)
AND NOT EXISTS (
    SELECT 1 FROM terraform_versions WHERE is_default = true
);

COMMENT ON COLUMN terraform_versions.is_default IS '是否为默认版本（全局唯一）';
```

### 字段说明

| 字段 | 类型 | 说明 | 约束 |
|------|------|------|------|
| id | SERIAL | 主键 | PRIMARY KEY |
| version | VARCHAR(50) | 版本号 | NOT NULL, UNIQUE |
| download_url | TEXT | 下载链接 | NOT NULL |
| checksum | VARCHAR(64) | SHA256校验和 | NOT NULL |
| enabled | BOOLEAN | 是否启用 | DEFAULT true |
| deprecated | BOOLEAN | 是否弃用 | DEFAULT false |
| is_default | BOOLEAN | 是否为默认版本 ⭐ | DEFAULT false, 全局唯一 |
| created_at | TIMESTAMP | 创建时间 | DEFAULT NOW() |
| updated_at | TIMESTAMP | 更新时间 | DEFAULT NOW() |

---

## 后端实现

### 1. Model定义

```go
// backend/internal/models/terraform_version.go
package models

import "time"

// TerraformVersion Terraform版本模型
type TerraformVersion struct {
	ID          int       `json:"id" db:"id"`
	Version     string    `json:"version" db:"version" binding:"required"`
	DownloadURL string    `json:"download_url" db:"download_url" binding:"required,url"`
	Checksum    string    `json:"checksum" db:"checksum" binding:"required,len=64"`
	Enabled     bool      `json:"enabled" db:"enabled"`
	Deprecated  bool      `json:"deprecated" db:"deprecated"`
	IsDefault   bool      `json:"is_default" db:"is_default"`  // ⭐ 新增
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// CreateTerraformVersionRequest 创建Terraform版本请求
type CreateTerraformVersionRequest struct {
	Version     string `json:"version" binding:"required"`
	DownloadURL string `json:"download_url" binding:"required,url"`
	Checksum    string `json:"checksum" binding:"required,len=64"`
	Enabled     bool   `json:"enabled"`
	Deprecated  bool   `json:"deprecated"`
}

// UpdateTerraformVersionRequest 更新Terraform版本请求
type UpdateTerraformVersionRequest struct {
	DownloadURL *string `json:"download_url" binding:"omitempty,url"`
	Checksum    *string `json:"checksum" binding:"omitempty,len=64"`
	Enabled     *bool   `json:"enabled"`
	Deprecated  *bool   `json:"deprecated"`
}

// TerraformVersionListResponse Terraform版本列表响应
type TerraformVersionListResponse struct {
	Items []TerraformVersion `json:"items"`
	Total int                `json:"total"`
}
```

### 2. Service层

```go
// backend/services/terraform_version_service.go
package services

import (
	"fmt"
	"iac-platform/internal/models"
	"gorm.io/gorm"
)

type TerraformVersionService struct {
	db *gorm.DB
}

func NewTerraformVersionService(db *gorm.DB) *TerraformVersionService {
	return &TerraformVersionService{db: db}
}

// List 获取所有Terraform版本
func (s *TerraformVersionService) List(enabled *bool, deprecated *bool) ([]models.TerraformVersion, error) {
	var versions []models.TerraformVersion
	query := s.db.Model(&models.TerraformVersion{})

	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}
	if deprecated != nil {
		query = query.Where("deprecated = ?", *deprecated)
	}

	err := query.Order("is_default DESC, created_at DESC").Find(&versions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query terraform versions: %w", err)
	}

	if versions == nil {
		versions = []models.TerraformVersion{}
	}
	return versions, nil
}

// GetByID 根据ID获取Terraform版本
func (s *TerraformVersionService) GetByID(id int) (*models.TerraformVersion, error) {
	var version models.TerraformVersion
	err := s.db.First(&version, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("terraform version not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get terraform version: %w", err)
	}
	return &version, nil
}

// GetDefault 获取默认版本 ⭐ 新增
func (s *TerraformVersionService) GetDefault() (*models.TerraformVersion, error) {
	var version models.TerraformVersion
	err := s.db.Where("is_default = ?", true).First(&version).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("no default version configured")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get default version: %w", err)
	}
	return &version, nil
}

// Create 创建Terraform版本
func (s *TerraformVersionService) Create(req *models.CreateTerraformVersionRequest) (*models.TerraformVersion, error) {
	// 检查版本是否已存在
	var count int64
	s.db.Model(&models.TerraformVersion{}).Where("version = ?", req.Version).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("version %s already exists", req.Version)
	}

	version := &models.TerraformVersion{
		Version:     req.Version,
		DownloadURL: req.DownloadURL,
		Checksum:    req.Checksum,
		Enabled:     req.Enabled,
		Deprecated:  req.Deprecated,
		IsDefault:   false, // 新创建的版本默认不是默认版本
	}

	err := s.db.Create(version).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create terraform version: %w", err)
	}
	return version, nil
}

// Update 更新Terraform版本
func (s *TerraformVersionService) Update(id int, req *models.UpdateTerraformVersionRequest) (*models.TerraformVersion, error) {
	version, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})
	if req.DownloadURL != nil {
		updates["download_url"] = *req.DownloadURL
	}
	if req.Checksum != nil {
		updates["checksum"] = *req.Checksum
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Deprecated != nil {
		updates["deprecated"] = *req.Deprecated
	}

	if len(updates) > 0 {
		err = s.db.Model(version).Updates(updates).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update terraform version: %w", err)
		}
	}

	return s.GetByID(id)
}

// SetDefault 设置默认版本 ⭐ 新增
func (s *TerraformVersionService) SetDefault(id int) (*models.TerraformVersion, error) {
	// 检查版本是否存在
	version, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 检查版本是否启用
	if !version.Enabled {
		return nil, fmt.Errorf("cannot set disabled version as default")
	}

	// 使用事务确保原子性
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 1. 取消所有版本的默认状态
		if err := tx.Model(&models.TerraformVersion{}).
			Where("is_default = ?", true).
			Update("is_default", false).Error; err != nil {
			return fmt.Errorf("failed to clear default flags: %w", err)
		}

		// 2. 设置新的默认版本
		if err := tx.Model(&models.TerraformVersion{}).
			Where("id = ?", id).
			Update("is_default", true).Error; err != nil {
			return fmt.Errorf("failed to set default version: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.GetByID(id)
}

// Delete 删除Terraform版本
func (s *TerraformVersionService) Delete(id int) error {
	// 检查版本是否存在
	version, err := s.GetByID(id)
	if err != nil {
		return err
	}

	// 不允许删除默认版本 ⭐ 新增
	if version.IsDefault {
		return fmt.Errorf("cannot delete default version, please set another version as default first")
	}

	// 检查是否有workspace在使用该版本
	inUse, err := s.CheckVersionInUse(id)
	if err != nil {
		return err
	}
	if inUse {
		return fmt.Errorf("version is in use by workspaces")
	}

	result := s.db.Delete(&models.TerraformVersion{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete terraform version: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("terraform version not found")
	}

	return nil
}

// CheckVersionInUse 检查版本是否被workspace使用
func (s *TerraformVersionService) CheckVersionInUse(id int) (bool, error) {
	version, err := s.GetByID(id)
	if err != nil {
		return false, err
	}

	var count int64
	err = s.db.Model(&models.Workspace{}).
		Where("terraform_version = ?", version.Version).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check version usage: %w", err)
	}

	return count > 0, nil
}
```

### 3. Controller层

```go
// backend/controllers/terraform_version_controller.go
package controllers

import (
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TerraformVersionController struct {
	service *services.TerraformVersionService
}

func NewTerraformVersionController(db *gorm.DB) *TerraformVersionController {
	return &TerraformVersionController{
		service: services.NewTerraformVersionService(db),
	}
}

// ListTerraformVersions 获取所有Terraform版本
func (c *TerraformVersionController) ListTerraformVersions(ctx *gin.Context) {
	var enabled *bool
	var deprecated *bool

	if enabledStr := ctx.Query("enabled"); enabledStr != "" {
		val := enabledStr == "true"
		enabled = &val
	}
	if deprecatedStr := ctx.Query("deprecated"); deprecatedStr != "" {
		val := deprecatedStr == "true"
		deprecated = &val
	}

	versions, err := c.service.List(enabled, deprecated)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.TerraformVersionListResponse{
		Items: versions,
		Total: len(versions),
	})
}

// GetTerraformVersion 获取单个Terraform版本
func (c *TerraformVersionController) GetTerraformVersion(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	version, err := c.service.GetByID(id)
	if err != nil {
		if err.Error() == "terraform version not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// GetDefaultVersion 获取默认版本 ⭐ 新增
func (c *TerraformVersionController) GetDefaultVersion(ctx *gin.Context) {
	version, err := c.service.GetDefault()
	if err != nil {
		if err.Error() == "no default version configured" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// CreateTerraformVersion 创建Terraform版本
func (c *TerraformVersionController) CreateTerraformVersion(ctx *gin.Context) {
	var req models.CreateTerraformVersionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	version, err := c.service.Create(&req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, version)
}

// UpdateTerraformVersion 更新Terraform版本
func (c *TerraformVersionController) UpdateTerraformVersion(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req models.UpdateTerraformVersionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	version, err := c.service.Update(id, &req)
	if err != nil {
		if err.Error() == "terraform version not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// SetDefaultVersion 设置默认版本 ⭐ 新增
func (c *TerraformVersionController) SetDefaultVersion(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	version, err := c.service.SetDefault(id)
	if err != nil {
		if err.Error() == "terraform version not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else if err.Error() == "cannot set disabled version as default" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// DeleteTerraformVersion 删除Terraform版本
func (c *TerraformVersionController) DeleteTerraformVersion(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	err = c.service.Delete(id)
	if err != nil {
		if err.Error() == "terraform version not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else if err.Error() == "version is in use by workspaces" || 
		          err.Error() == "cannot delete default version, please set another version as default first" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.Status(http.StatusNoContent)
}
```

### 4. Router配置

```go
// backend/internal/router/router.go
func SetupRouter(db *gorm.DB) *gin.Engine {
	r := gin.Default()
	
	// CORS配置
	r.Use(cors.Default())
	
	api := r.Group("/api/v1")
	{
		// Admin routes
		admin := api.Group("/admin")
		{
			tfVersionController := controllers.NewTerraformVersionController(db)
			
			// Terraform版本管理
			admin.GET("/terraform-versions", tfVersionController.ListTerraformVersions)
			admin.GET("/terraform-versions/default", tfVersionController.GetDefaultVersion)  // ⭐ 新增
			admin.GET("/terraform-versions/:id", tfVersionController.GetTerraformVersion)
			admin.POST("/terraform-versions", tfVersionController.CreateTerraformVersion)
			admin.PUT("/terraform-versions/:id", tfVersionController.UpdateTerraformVersion)
			admin.POST("/terraform-versions/:id/set-default", tfVersionController.SetDefaultVersion)  // ⭐ 新增
			admin.DELETE("/terraform-versions/:id", tfVersionController.DeleteTerraformVersion)
		}
	}
	
	return r
}
```

---

## API接口规范

### 1. 获取所有Terraform版本

```http
GET /api/v1/admin/terraform-versions
```

**Query参数**:
- `enabled` (optional): 过滤启用状态 (true/false)
- `deprecated` (optional): 过滤弃用状态 (true/false)

**响应示例**:
```json
{
  "items": [
    {
      "id": 1,
      "version": "1.6.0",
      "download_url": "https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_linux_amd64.zip",
      "checksum": "abc123...",
      "enabled": true,
      "deprecated": false,
      "is_default": true,
      "created_at": "2025-01-01T00:00:00Z",
      "updated_at": "2025-01-01T00:00:00Z"
    }
  ],
  "total": 1
}
```

### 2. 获取默认版本 ⭐ 新增

```http
GET /api/v1/admin/terraform-versions/default
```

**响应示例**:
```json
{
  "id": 1,
  "version": "1.6.0",
  "download_url": "https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_linux_amd64.zip",
  "checksum": "abc123...",
  "enabled": true,
  "deprecated": false,
  "is_default": true,
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-01T00:00:00Z"
}
```

### 3. 获取单个版本

```http
GET /api/v1/admin/terraform-versions/:id
```

### 4. 创建版本

```http
POST /api/v1/admin/terraform-versions
Content-Type: application/json

{
  "version": "1.6.0",
  "download_url": "https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_linux_amd64.zip",
  "checksum": "abc123...",
  "enabled": true,
  "deprecated": false
}
```

### 5. 更新版本

```http
PUT /api/v1/admin/terraform-versions/:id
Content-Type: application/json

{
  "enabled": false,
  "deprecated": true
}
```

### 6. 设置默认版本 ⭐ 新增

```http
POST /api/v1/admin/terraform-versions/:id/set-default
```

**响应示例**:
```json
{
  "id": 2,
  "version": "1.5.7",
  "download_url": "https://...",
  "checksum": "def456...",
  "enabled": true,
  "deprecated": false,
  "is_default": true,
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-11T00:00:00Z"
}
```

**错误响应**:
```json
{
  "error": "cannot set disabled version as default"
}
```

### 7. 删除版本

```http
DELETE /api/v1/admin/terraform-versions/:id
```

**错误响应**:
```json
{
  "error": "cannot delete default version, please set another version as default first"
}
```

---

## 前端设计

### 页面布局

```
┌─ Admin > Terraform Versions ─────────────────────┐
│                                                   │
│ Terraform Versions                                │
│                                                   │
│ Manage Terraform versions available for          │
│ workspaces. The default version will be used     │
│ when creating new workspaces.                     │
│                                                   │
│ [+ Add Version]                                   │
│                                                   │
│ ┌─ Available Versions ─────────────────────────┐ │
│ │ VERSION  DOWNLOAD URL    STATUS    ACTIONS   ││
│ ├───────────────────────────────────────────────┤ │
│ │ 1.6.0    https://...     ⭐ Default          ││
│ │          Checksum: abc    Enabled          ││
│ │          Added: 2025-01  [Edit] [Delete]     ││
│ ├───────────────────────────────────────────────┤ │
│ │ 1.5.7    https://...      Enabled          ││
│ │          Checksum: def   [Set Default]       ││
│ │          Added: 2025-01  [Edit] [Delete]     ││
│ ├───────────────────────────────────────────────┤ │
│ │ 1.4.6    https://...      Deprecated       ││
│ │          Checksum: ghi   [Set Default]       ││
│ │          Added: 2024-12  [Edit] [Delete]     ││
│ └───────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────┘
```

### 组件设计

#### 1. Admin.tsx - 主页面

```typescript
// frontend/src/pages/Admin.tsx
import React, { useState, useEffect } from 'react';
import { adminService } from '../services/admin';
import { useSimpleToast } from '../hooks/useSimpleToast';
import styles from './Admin.module.css';

interface TerraformVersion {
  id: number;
  version: string;
  download_url: string;
  checksum: string;
  enabled: boolean;
  deprecated: boolean;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

const Admin: React.FC = () => {
  const [versions, setVersions] = useState<TerraformVersion[]>([]);
  const [loading, setLoading] = useState(true);
  const [showDialog, setShowDialog] = useState(false);
  const [editingVersion, setEditingVersion] = useState<TerraformVersion | null>(null);
  const { showSuccess, showError } = useSimpleToast();

  useEffect(() => {
    loadVersions();
  }, []);

  const loadVersions = async () => {
    try {
      setLoading(true);
      const response = await adminService.getTerraformVersions();
      setVersions(response.items || []);
    } catch (error: any) {
      showError(error.message || '加载版本列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleSetDefault = async (id: number) => {
    try {
      await adminService.setDefaultVersion(id);
      showSuccess('默认版本设置成功');
      loadVersions();
    } catch (error: any) {
      showError(error.message || '设置默认版本失败');
    }
  };

  const handleDelete = async (id: number, version: string) => {
    if (!confirm(`确定要删除版本 ${version} 吗？`)) {
      return;
    }

    try {
      await adminService.deleteTerraformVersion(id);
      showSuccess('版本删除成功');
      loadVersions();
    } catch (error: any) {
      showError(error.message || '删除版本失败');
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div>
          <h1 className={styles.title}>Terraform Versions</h1>
          <p className={styles.description}>
            Manage Terraform versions available for workspaces. 
            The default version will be used when creating new workspaces.
          </p>
        </div>
        <button 
          className={styles.addButton}
          onClick={() => {
            setEditingVersion(null);
            setShowDialog(true);
          }}
        >
          + Add Version
        </button>
      </div>

      {loading ? (
        <div className={styles.loading}>Loading...</div>
      ) : (
        <div className={styles.versionList}>
          {versions.map(version => (
            <div key={version.id} className={styles.versionCard}>
              <div className={styles.versionInfo}>
                <div className={styles.versionHeader}>
                  <span className={styles.versionNumber}>{version.version}</span>
                  {version.is_default && (
                    <span className={styles.defaultBadge}>⭐ Default</span>
                  )}
                  {version.enabled && !version.deprecated && (
                    <span className={styles.enabledBadge}> Enabled</span>
                  )}
                  {version.deprecated && (
                    <span className={styles.deprecatedBadge}> Deprecated</span>
                  )}
                  {!version.enabled && (
                    <span className={styles.disabledBadge}>❌ Disabled</span>
                  )}
                </div>
                <div className={styles.versionDetails}>
                  <div>Download URL: {version.download_url}</div>
                  <div>Checksum: {version.checksum.substring(0, 16)}...</div>
                  <div>Added: {new Date(version.created_at).toLocaleDateString()}</div>
                </div>
              </div>
              <div className={styles.versionActions}>
                {!version.is_default && version.enabled && (
                  <button
                    className={styles.setDefaultButton}
                    onClick={() => handleSetDefault(version.id)}
                  >
                    Set Default
                  </button>
                )}
                <button
                  className={styles.editButton}
                  onClick={() => {
                    setEditingVersion(version);
                    setShowDialog(true);
                  }}
                >
                  Edit
                </button>
                <button
                  className={styles.deleteButton}
                  onClick={() => handleDelete(version.id, version.version)}
                >
                  Delete
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default Admin;
```

#### 2. adminService.ts - API服务

```typescript
// frontend/src/services/admin.ts
import api from './api';

export interface TerraformVersion {
  id: number;
  version: string;
  download_url: string;
  checksum: string;
  enabled: boolean;
  deprecated: boolean;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

export interface TerraformVersionListResponse {
  items: TerraformVersion[];
  total: number;
}

export const adminService = {
  // 获取所有版本
  getTerraformVersions: async (params?: {
    enabled?: boolean;
    deprecated?: boolean;
  }): Promise<TerraformVersionListResponse> => {
    const response = await api.get('/admin/terraform-versions', { params });
    return response.data;
  },

  // 获取默认版本 ⭐ 新增
  getDefaultVersion: async (): Promise<TerraformVersion> => {
    const response = await api.get('/admin/terraform-versions/default');
    return response.data;
  },

  // 获取单个版本
  getTerraformVersion: async (id: number): Promise<TerraformVersion> => {
    const response = await api.get(`/admin/terraform-versions/${id}`);
    return response.data;
  },

  // 创建版本
  createTerraformVersion: async (data: {
    version: string;
    download_url: string;
    checksum: string;
    enabled: boolean;
    deprecated: boolean;
  }): Promise<TerraformVersion> => {
    const response = await api.post('/admin/terraform-versions', data);
    return response.data;
  },

  // 更新版本
  updateTerraformVersion: async (
    id: number,
    data: {
      download_url?: string;
      checksum?: string;
      enabled?: boolean;
      deprecated?: boolean;
    }
  ): Promise<TerraformVersion> => {
    const response = await api.put(`/admin/terraform-versions/${id}`, data);
    return response.data;
  },

  // 设置默认版本 ⭐ 新增
  setDefaultVersion: async (id: number): Promise<TerraformVersion> => {
    const response = await api.post(`/admin/terraform-versions/${id}/set-default`);
    return response.data;
  },

  // 删除版本
  deleteTerraformVersion: async (id: number): Promise<void> => {
    await api.delete(`/admin/terraform-versions/${id}`);
  },
};
```

---

## 默认版本功能

### 功能说明

默认版本功能允许管理员设置一个全局默认的Terraform版本，该版本将在以下场景中使用：

1. **创建新Workspace时** - 如果用户未指定版本，自动使用默认版本
2. **版本选择器** - 在版本下拉列表中突出显示默认版本
3. **API响应** - 提供专门的API获取默认版本信息

### 业务规则

1. **全局唯一性**
   - 系统中只能有一个默认版本
   - 设置新默认版本时，自动取消旧的默认版本
   - 使用数据库唯一索引确保约束

2. **版本状态限制**
   - 只有启用的版本才能设置为默认版本
   - 禁用的版本不能设置为默认
   - 如果默认版本被禁用，需要先设置新的默认版本

3. **删除保护**
   - 默认版本不能被删除
   - 删除前必须先设置其他版本为默认

### 使用流程

```
1. 管理员创建多个Terraform版本
   ↓
2. 选择一个启用的版本，点击"Set Default"
   ↓
3. 系统自动取消旧的默认版本，设置新的默认版本
   ↓
4. 用户创建Workspace时，自动使用默认版本
```

### 前端交互

1. **版本列表显示**
   - 默认版本显示⭐图标
   - 默认版本排在列表最前面
   - 非默认版本显示"Set Default"按钮

2. **设置默认版本**
   - 点击"Set Default"按钮
   - 显示确认提示
   - 成功后刷新列表，显示新的默认版本

3. **删除限制**
   - 默认版本的删除按钮禁用或隐藏
   - 尝试删除时显示错误提示

---

## 使用场景

### 场景1：初始化系统

```bash
# 1. 添加第一个Terraform版本
POST /api/v1/admin/terraform-versions
{
  "version": "1.6.0",
  "download_url": "https://...",
  "checksum": "abc123...",
  "enabled": true
}

# 2. 设置为默认版本
POST /api/v1/admin/terraform-versions/1/set-default

# 3. 创建Workspace时自动使用默认版本
POST /api/v1/workspaces
{
  "name": "prod-network",
  // terraform_version 未指定，自动使用默认版本 1.6.0
}
```

### 场景2：版本升级

```bash
# 1. 添加新版本
POST /api/v1/admin/terraform-versions
{
  "version": "1.7.0",
  "download_url": "https://...",
  "checksum": "def456...",
  "enabled": true
}

# 2. 测试新版本（不设为默认）
# 在测试Workspace中手动指定使用1.7.0

# 3. 测试通过后，设置为默认版本
POST /api/v1/admin/terraform-versions/2/set-default

# 4. 新创建的Workspace自动使用1.7.0
```

### 场景3：版本弃用

```bash
# 1. 标记旧版本为弃用
PUT /api/v1/admin/terraform-versions/1
{
  "deprecated": true
}

# 2. 设置新版本为默认
POST /api/v1/admin/terraform-versions/2/set-default

# 3. 禁用旧版本
PUT /api/v1/admin/terraform-versions/1
{
  "enabled": false
}

# 4. 删除旧版本（如果没有Workspace使用）
DELETE /api/v1/admin/terraform-versions/1
```

---

## 测试指南

### 单元测试

```go
// backend/services/terraform_version_service_test.go
func TestSetDefault(t *testing.T) {
	// 测试设置默认版本
	// 测试只能有一个默认版本
	// 测试禁用的版本不能设为默认
}

func TestDeleteDefault(t *testing.T) {
	// 测试不能删除默认版本
}
```

### API测试

```bash
# 1. 创建测试版本
curl -X POST http://localhost:8080/api/v1/admin/terraform-versions \
  -H "Content-Type: application/json" \
  -d '{
    "version": "1.6.0",
    "download_url": "https://releases.hashicorp.com/terraform/1.6.0/terraform_1.6.0_linux_amd64.zip",
    "checksum": "abc123...",
    "enabled": true
  }'

# 2. 设置为默认版本
curl -X POST http://localhost:8080/api/v1/admin/terraform-versions/1/set-default

# 3. 获取默认版本
curl http://localhost:8080/api/v1/admin/terraform-versions/default

# 4. 尝试删除默认版本（应该失败）
curl -X DELETE http://localhost:8080/api/v1/admin/terraform-versions/1

# 5. 创建第二个版本并设为默认
curl -X POST http://localhost:8080/api/v1/admin/terraform-versions \
  -H "Content-Type: application/json" \
  -d '{
    "version": "1.7.0",
    "download_url": "https://...",
    "checksum": "def456...",
    "enabled": true
  }'

curl -X POST http://localhost:8080/api/v1/admin/terraform-versions/2/set-default

# 6. 验证只有一个默认版本
curl http://localhost:8080/api/v1/admin/terraform-versions
```

### 前端测试

1. **版本列表测试**
   - 访问 Admin 页面
   - 验证默认版本显示⭐图标
   - 验证默认版本排在最前面

2. **设置默认版本测试**
   - 点击非默认版本的"Set Default"按钮
   - 验证成功提示
   - 验证列表更新，新版本显示⭐图标

3. **删除限制测试**
   - 尝试删除默认版本
   - 验证显示错误提示
   - 设置其他版本为默认后，可以删除原默认版本

---

## 更新日志

| 版本 | 日期 | 变更说明 |
|------|------|----------|
| v1.1 | 2025-10-11 | 新增默认版本功能设计和文档 |
| v1.0 | 2025-10-09 | 初始版本，后端实现完成 |

---

## 相关文档

- [Admin模块README](./README.md)
- [Admin管理功能设计](./01-admin-management.md)
- [Admin API规范](./02-api-specification.md)
- [项目快速入口](../QUICK_START_FOR_AI.md)
