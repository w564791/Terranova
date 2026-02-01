# 资源级别代码版本管理设计

> **文档版本**: v1.0  
> **创建日期**: 2025-10-11  
> **状态**: 设计方案  
> **优先级**: P1（重要改进）

## 📋 概述

本文档描述资源级别的代码版本管理方案，相比Workspace全局级别的版本管理，资源级别提供了更细粒度的控制和更高效的操作。

## 💡 设计理念

### 核心思想

将Terraform代码的版本管理从**Workspace全局级别**下沉到**资源级别**，每个资源（如aws_s3_bucket.my_bucket）作为独立的版本管理单元。

### 优势分析

| 特性 | Workspace级别 | 资源级别 | 优势 |
|------|--------------|---------|------|
| 版本粒度 | 整个Workspace | 单个资源 |  更细粒度 |
| 回滚效率 | 全量回滚 | 使用-target |  更高效 |
| 变更历史 | 混合在一起 | 独立追踪 |  更清晰 |
| 协作能力 | 容易冲突 | 并行工作 |  更好协作 |
| 部署灵活性 | 全部部署 | 选择性部署 |  更灵活 |

## 🏗️ 数据库设计

### 1. workspace_resources 表（资源表）

```sql
CREATE TABLE workspace_resources (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    
    -- 资源标识
    resource_id VARCHAR(100) NOT NULL,  -- 全局唯一ID，如 "aws_s3_bucket.my_bucket"
    resource_type VARCHAR(50) NOT NULL, -- 资源类型，如 "aws_s3_bucket"
    resource_name VARCHAR(100) NOT NULL, -- 资源名称，如 "my_bucket"
    
    -- 当前版本信息
    current_version_id INTEGER REFERENCES resource_code_versions(id),
    is_active BOOLEAN DEFAULT true,     -- 是否激活（用于软删除）
    
    -- 元数据
    description TEXT,
    tags JSONB,
    
    -- 审计字段
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(workspace_id, resource_id)
);

CREATE INDEX idx_workspace_resources_workspace ON workspace_resources(workspace_id);
CREATE INDEX idx_workspace_resources_type ON workspace_resources(resource_type);
CREATE INDEX idx_workspace_resources_active ON workspace_resources(is_active);

COMMENT ON TABLE workspace_resources IS '工作空间资源表';
COMMENT ON COLUMN workspace_resources.resource_id IS '资源全局唯一标识，格式：type.name';
COMMENT ON COLUMN workspace_resources.current_version_id IS '当前使用的版本ID';
COMMENT ON COLUMN workspace_resources.is_active IS '是否激活，false表示软删除';
```

### 2. resource_code_versions 表（资源代码版本表）

```sql
CREATE TABLE resource_code_versions (
    id SERIAL PRIMARY KEY,
    resource_id INTEGER NOT NULL REFERENCES workspace_resources(id) ON DELETE CASCADE,
    
    -- 版本信息
    version INTEGER NOT NULL,           -- 版本号，从1开始递增
    is_latest BOOLEAN DEFAULT false,    -- 是否是最新版本
    
    -- 代码内容
    tf_code JSONB NOT NULL,            -- Terraform代码（JSON格式）
    variables JSONB,                    -- 资源特定的变量
    
    -- 变更信息
    change_summary TEXT,                -- 变更摘要
    change_type VARCHAR(20),            -- 变更类型：create, update, delete
    diff_from_previous TEXT,            -- 与上一版本的差异
    
    -- 关联信息
    state_version_id INTEGER REFERENCES workspace_state_versions(id), -- 关联的State版本
    task_id INTEGER REFERENCES workspace_tasks(id), -- 创建此版本的任务
    
    -- 审计字段
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(resource_id, version)
);

CREATE INDEX idx_resource_code_versions_resource ON resource_code_versions(resource_id, version DESC);
CREATE INDEX idx_resource_code_versions_latest ON resource_code_versions(is_latest) WHERE is_latest = true;

COMMENT ON TABLE resource_code_versions IS '资源代码版本表';
COMMENT ON COLUMN resource_code_versions.is_latest IS '是否是最新版本，每个资源只有一个最新版本';
COMMENT ON COLUMN resource_code_versions.change_type IS '变更类型：create/update/delete/rollback';
```

### 3. workspace_resources_snapshot 表（资源快照表）

```sql
CREATE TABLE workspace_resources_snapshot (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    
    -- 快照信息
    snapshot_name VARCHAR(100),         -- 快照名称，如 "v1.0.0", "before-migration"
    resources_versions JSONB NOT NULL,  -- 资源版本映射 {"resource_id": version_id}
    
    -- 关联信息
    task_id INTEGER REFERENCES workspace_tasks(id),
    state_version_id INTEGER REFERENCES workspace_state_versions(id),
    
    -- 审计字段
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    description TEXT
);

CREATE INDEX idx_workspace_resources_snapshot_workspace ON workspace_resources_snapshot(workspace_id);

COMMENT ON TABLE workspace_resources_snapshot IS '工作空间资源快照表，用于记录某个时间点所有资源的版本组合';
COMMENT ON COLUMN workspace_resources_snapshot.resources_versions IS 'JSON格式：{"resource_id": version_id}';
```

### 4. resource_dependencies 表（资源依赖关系表）

```sql
CREATE TABLE resource_dependencies (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    
    -- 依赖关系
    resource_id INTEGER NOT NULL REFERENCES workspace_resources(id) ON DELETE CASCADE,
    depends_on_resource_id INTEGER NOT NULL REFERENCES workspace_resources(id) ON DELETE CASCADE,
    
    -- 依赖类型
    dependency_type VARCHAR(20) DEFAULT 'explicit', -- explicit/implicit
    
    -- 审计字段
    created_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(resource_id, depends_on_resource_id)
);

CREATE INDEX idx_resource_dependencies_resource ON resource_dependencies(resource_id);
CREATE INDEX idx_resource_dependencies_depends_on ON resource_dependencies(depends_on_resource_id);

COMMENT ON TABLE resource_dependencies IS '资源依赖关系表';
COMMENT ON COLUMN resource_dependencies.dependency_type IS 'explicit: 显式依赖(depends_on), implicit: 隐式依赖(引用)';
```

## 📊 数据模型关系图

```
┌─────────────────┐
│   Workspace     │
└────────┬────────┘
         │
         ├──> WorkspaceResource (1:N)
         │    ├── resource_id (唯一标识)
         │    ├── current_version_id
         │    └── is_active
         │
         └──> WorkspaceResourcesSnapshot (1:N)
              └── resources_versions (JSON)

┌─────────────────────┐
│ WorkspaceResource   │
└──────────┬──────────┘
           │
           ├──> ResourceCodeVersion (1:N)
           │    ├── version (递增)
           │    ├── is_latest (唯一)
           │    ├── tf_code (JSONB)
           │    └── change_type
           │
           └──> ResourceDependency (1:N)
                ├── depends_on_resource_id
                └── dependency_type
```

## 🔄 核心工作流程

### 1. 添加资源

```go
// AddResource 添加新资源到Workspace
func (s *ResourceService) AddResource(
    workspaceID uint,
    resourceType string,
    resourceName string,
    tfCode map[string]interface{},
    userID uint,
) (*WorkspaceResource, error) {
    // 1. 创建资源记录
    resource := &WorkspaceResource{
        WorkspaceID:  workspaceID,
        ResourceID:   fmt.Sprintf("%s.%s", resourceType, resourceName),
        ResourceType: resourceType,
        ResourceName: resourceName,
        IsActive:     true,
        CreatedBy:    &userID,
    }
    
    if err := s.db.Create(resource).Error; err != nil {
        return nil, err
    }
    
    // 2. 创建第一个版本
    version := &ResourceCodeVersion{
        ResourceID:    resource.ID,
        Version:       1,
        IsLatest:      true,
        TFCode:        tfCode,
        ChangeType:    "create",
        ChangeSummary: "Initial creation",
        CreatedBy:     &userID,
    }
    
    if err := s.db.Create(version).Error; err != nil {
        return nil, err
    }
    
    // 3. 更新资源的当前版本
    resource.CurrentVersionID = &version.ID
    s.db.Save(resource)
    
    return resource, nil
}
```

### 2. 更新资源

```go
// UpdateResource 更新资源配置
func (s *ResourceService) UpdateResource(
    resourceID uint,
    tfCode map[string]interface{},
    changeSummary string,
    userID uint,
) (*ResourceCodeVersion, error) {
    // 1. 获取资源
    var resource WorkspaceResource
    if err := s.db.First(&resource, resourceID).Error; err != nil {
        return nil, err
    }
    
    // 2. 获取当前最新版本号
    var maxVersion int
    s.db.Model(&ResourceCodeVersion{}).
        Where("resource_id = ?", resourceID).
        Select("COALESCE(MAX(version), 0)").
        Scan(&maxVersion)
    
    // 3. 计算差异
    var currentVersion ResourceCodeVersion
    s.db.Where("resource_id = ? AND is_latest = true", resourceID).
        First(&currentVersion)
    
    diff := calculateDiff(currentVersion.TFCode, tfCode)
    
    // 4. 创建新版本
    newVersion := &ResourceCodeVersion{
        ResourceID:       resourceID,
        Version:          maxVersion + 1,
        IsLatest:         true,
        TFCode:           tfCode,
        ChangeType:       "update",
        ChangeSummary:    changeSummary,
        DiffFromPrevious: diff,
        CreatedBy:        &userID,
    }
    
    return s.db.Transaction(func(tx *gorm.DB) error {
        // 5. 旧版本标记为非最新
        tx.Model(&ResourceCodeVersion{}).
            Where("resource_id = ? AND is_latest = true", resourceID).
            Update("is_latest", false)
        
        // 6. 创建新版本
        if err := tx.Create(newVersion).Error; err != nil {
            return err
        }
        
        // 7. 更新资源的当前版本
        resource.CurrentVersionID = &newVersion.ID
        return tx.Save(&resource).Error
    })
}
```

### 3. 生成Terraform配置（从资源聚合）

```go
// GenerateMainTF 从资源聚合生成main.tf.json
func (s *TerraformExecutor) GenerateMainTF(workspaceID uint) (map[string]interface{}, error) {
    // 1. 获取所有激活的资源
    var resources []WorkspaceResource
    s.db.Where("workspace_id = ? AND is_active = true", workspaceID).
        Preload("CurrentVersion").
        Find(&resources)
    
    // 2. 聚合所有资源的TF代码
    mainTF := make(map[string]interface{})
    
    for _, resource := range resources {
        if resource.CurrentVersion == nil {
            continue
        }
        
        // 合并资源的TF代码到main.tf
        mergeTFCode(mainTF, resource.CurrentVersion.TFCode)
    }
    
    return mainTF, nil
}

// mergeTFCode 合并TF代码
func mergeTFCode(target, source map[string]interface{}) {
    for key, value := range source {
        if existing, ok := target[key]; ok {
            // 如果key已存在，合并内容
            if existingMap, ok := existing.(map[string]interface{}); ok {
                if sourceMap, ok := value.(map[string]interface{}); ok {
                    for k, v := range sourceMap {
                        existingMap[k] = v
                    }
                    continue
                }
            }
        }
        target[key] = value
    }
}
```

### 4. 资源级别回滚

```go
// RollbackResource 回滚资源到指定版本
func (s *ResourceService) RollbackResource(
    resourceID uint,
    targetVersion int,
    userID uint,
) error {
    // 1. 获取目标版本
    var targetVer ResourceCodeVersion
    if err := s.db.Where("resource_id = ? AND version = ?", 
        resourceID, targetVersion).First(&targetVer).Error; err != nil {
        return fmt.Errorf("target version not found: %w", err)
    }
    
    // 2. 获取资源信息
    var resource WorkspaceResource
    if err := s.db.First(&resource, resourceID).Error; err != nil {
        return err
    }
    
    // 3. 创建新版本（内容是旧版本的）
    var maxVersion int
    s.db.Model(&ResourceCodeVersion{}).
        Where("resource_id = ?", resourceID).
        Select("COALESCE(MAX(version), 0)").
        Scan(&maxVersion)
    
    newVersion := &ResourceCodeVersion{
        ResourceID:    resourceID,
        Version:       maxVersion + 1,
        IsLatest:      true,
        TFCode:        targetVer.TFCode,
        Variables:     targetVer.Variables,
        ChangeType:    "rollback",
        ChangeSummary: fmt.Sprintf("Rollback to version %d", targetVersion),
        CreatedBy:     &userID,
    }
    
    return s.db.Transaction(func(tx *gorm.DB) error {
        // 4. 旧版本标记为非最新
        tx.Model(&ResourceCodeVersion{}).
            Where("resource_id = ? AND is_latest = true", resourceID).
            Update("is_latest", false)
        
        // 5. 创建新版本
        if err := tx.Create(newVersion).Error; err != nil {
            return err
        }
        
        // 6. 更新资源的当前版本
        resource.CurrentVersionID = &newVersion.ID
        return tx.Save(&resource).Error
    })
}

// ExecuteResourceRollback 执行资源回滚（使用terraform apply -target）
func (s *TerraformExecutor) ExecuteResourceRollback(
    workspaceID uint,
    resourceIDs []string,
) error {
    // 1. 生成target参数
    targets := make([]string, len(resourceIDs))
    for i, rid := range resourceIDs {
        targets[i] = fmt.Sprintf("-target=%s", rid)
    }
    
    // 2. 创建Plan任务（带target）
    task := &WorkspaceTask{
        WorkspaceID: workspaceID,
        TaskType:    TaskTypePlan,
        Context: map[string]interface{}{
            "targets": targets,
        },
    }
    
    // 3. 执行Plan
    if err := s.ExecutePlan(context.Background(), task); err != nil {
        return err
    }
    
    // 4. 执行Apply
    applyTask := &WorkspaceTask{
        WorkspaceID: workspaceID,
        TaskType:    TaskTypeApply,
        PlanTaskID:  &task.ID,
        Context: map[string]interface{}{
            "targets": targets,
        },
    }
    
    return s.ExecuteApply(context.Background(), applyTask)
}
```

### 5. 选择性部署

```go
// DeploySelectedResources 部署选定的资源
func (s *ResourceService) DeploySelectedResources(
    workspaceID uint,
    resourceIDs []uint,
    userID uint,
) error {
    // 1. 获取资源的resource_id
    var resources []WorkspaceResource
    s.db.Where("id IN ?", resourceIDs).Find(&resources)
    
    targetResourceIDs := make([]string, len(resources))
    for i, res := range resources {
        targetResourceIDs[i] = res.ResourceID
    }
    
    // 2. 执行部署
    return s.executor.ExecuteResourceRollback(workspaceID, targetResourceIDs)
}
```

### 6. 快照管理

```go
// CreateSnapshot 创建资源快照
func (s *ResourceService) CreateSnapshot(
    workspaceID uint,
    snapshotName string,
    description string,
    userID uint,
) (*WorkspaceResourcesSnapshot, error) {
    // 1. 获取所有资源的当前版本
    var resources []WorkspaceResource
    s.db.Where("workspace_id = ? AND is_active = true", workspaceID).
        Find(&resources)
    
    // 2. 构建版本映射
    versionsMap := make(map[string]interface{})
    for _, res := range resources {
        if res.CurrentVersionID != nil {
            versionsMap[fmt.Sprintf("%d", res.ID)] = *res.CurrentVersionID
        }
    }
    
    // 3. 创建快照
    snapshot := &WorkspaceResourcesSnapshot{
        WorkspaceID:       workspaceID,
        SnapshotName:      snapshotName,
        ResourcesVersions: versionsMap,
        Description:       description,
        CreatedBy:         &userID,
    }
    
    return snapshot, s.db.Create(snapshot).Error
}

// RestoreSnapshot 恢复快照
func (s *ResourceService) RestoreSnapshot(
    snapshotID uint,
    userID uint,
) error {
    // 1. 获取快照
    var snapshot WorkspaceResourcesSnapshot
    if err := s.db.First(&snapshot, snapshotID).Error; err != nil {
        return err
    }
    
    // 2. 恢复每个资源到快照中的版本
    for resourceIDStr, versionID := range snapshot.ResourcesVersions {
        resourceID, _ := strconv.ParseUint(resourceIDStr, 10, 32)
        
        // 获取版本信息
        var version ResourceCodeVersion
        if err := s.db.First(&version, versionID).Error; err != nil {
            continue
        }
        
        // 回滚资源
        s.RollbackResource(uint(resourceID), version.Version, userID)
    }
    
    return nil
}
```

## 🔌 API设计

### 资源管理API

```go
// 获取资源列表
GET /api/v1/workspaces/:id/resources
Response: {
    "resources": [
        {
            "id": 1,
            "resource_id": "aws_s3_bucket.my_bucket",
            "resource_type": "aws_s3_bucket",
            "resource_name": "my_bucket",
            "current_version": {
                "version": 3,
                "change_summary": "Enable versioning"
            },
            "is_active": true
        }
    ]
}

// 添加资源
POST /api/v1/workspaces/:id/resources
Request: {
    "resource_type": "aws_s3_bucket",
    "resource_name": "my_bucket",
    "tf_code": {
        "resource": {
            "aws_s3_bucket": {
                "my_bucket": {
                    "bucket": "my-unique-bucket-name",
                    "acl": "private"
                }
            }
        }
    },
    "variables": {},
    "description": "My S3 bucket"
}

// 更新资源
PUT /api/v1/workspaces/:id/resources/:resource_id
Request: {
    "tf_code": {...},
    "change_summary": "Enable versioning"
}

// 删除资源（软删除）
DELETE /api/v1/workspaces/:id/resources/:resource_id

// 获取资源版本历史
GET /api/v1/workspaces/:id/resources/:resource_id/versions
Response: {
    "versions": [
        {
            "version": 3,
            "is_latest": true,
            "change_type": "update",
            "change_summary": "Enable versioning",
            "created_at": "2025-10-11T10:00:00Z"
        },
        {
            "version": 2,
            "is_latest": false,
            "change_type": "update",
            "change_summary": "Add tags",
            "created_at": "2025-10-10T10:00:00Z"
        }
    ]
}

// 获取特定版本详情
GET /api/v1/workspaces/:id/resources/:resource_id/versions/:version

// 回滚资源到指定版本
POST /api/v1/workspaces/:id/resources/:resource_id/versions/:version/rollback
Request: {
    "execute_immediately": true  // 是否立即执行apply
}

// 对比两个版本
GET /api/v1/workspaces/:id/resources/:resource_id/versions/compare?from=1&to=3
Response: {
    "from_version": 1,
    "to_version": 3,
    "diff": "...",
    "changes": [
        {
            "field": "versioning",
            "old_value": null,
            "new_value": {"enabled": true}
        }
    ]
}

// 批量部署资源
POST /api/v1/workspaces/:id/resources/deploy
Request: {
    "resource_ids": [1, 2, 3]
}
```

### 快照管理API

```go
// 创建快照
POST /api/v1/workspaces/:id/snapshots
Request: {
    "snapshot_name": "v1.0.0",
    "description": "Stable release"
}

// 获取快照列表
GET /api/v1/workspaces/:id/snapshots
Response: {
    "snapshots": [
        {
            "id": 1,
            "snapshot_name": "v1.0.0",
            "description": "Stable release",
            "resources_count": 5,
            "created_at": "2025-10-11T10:00:00Z"
        }
    ]
}

// 获取快照详情
GET /api/v1/workspaces/:id/snapshots/:snapshot_id

// 恢复快照
POST /api/v1/workspaces/:id/snapshots/:snapshot_id/restore
Request: {
    "execute_immediately": true
}

// 删除快照
DELETE /api/v1/workspaces/:id/snapshots/:snapshot_id
```

### 依赖关系API

```go
// 获取资源依赖关系
GET /api/v1/workspaces/:id/resources/:resource_id/dependencies
Response: {
    "depends_on": [
        {
            "resource_id": "aws_vpc.main",
            "dependency_type": "explicit"
        }
    ],
    "depended_by": [
        {
            "resource_id": "aws_subnet.private",
            "dependency_type": "implicit"
        }
    ]
}

// 更新资源依赖关系
PUT /api/v1/workspaces/:id/resources/:resource_id/dependencies
Request: {
    "depends_on": ["aws_vpc.main", "aws_security_group.default"]
}
```

## 💻 前端UI设计

### 1. 资源列表页面

```
┌─────────────────────────────────────────────────────────┐
│ Workspace: Production                                    │
├─────────────────────────────────────────────────────────┤
│ Resources (5)                          [+ Add Resource]  │
├─────────────────────────────────────────────────────────┤
│ ☑ aws_s3_bucket.my_bucket              v3  [Edit] [...]│
│   └─ Enable versioning                                   │
│                                                           │
│ ☑ aws_vpc.main                         v2  [Edit] [...]│
│   └─ Add tags                                            │
│                                                           │
│ ☑ aws_subnet.private                   v1  [Edit] [...]│
│   └─ Initial creation                                    │
│                                                           │
│ [Deploy Selected (2)]  [Create Snapshot]                │
└─────────────────────────────────────────────────────────┘
```

### 2. 资源版本历史页面

```
┌─────────────────────────────────────────────────────────┐
│ Resource: aws_s3_bucket.my_bucket                        │
├─────────────────────────────────────────────────────────┤
│ Version History                                          │
├─────────────────────────────────────────────────────────┤
│ v3 (Current) ● Enable versioning                         │
│   2025-10-11 10:00  by Alice                            │
│   [View Code] [Compare]                                  │
│                                                           │
│ v2           ● Add tags                                  │
│   2025-10-10 10:00  by Bob                              │
│   [View Code] [Compare] [Rollback]                      │
│                                                           │
│ v1           ● Initial creation                          │
│   2025-10-09 10:00  by Alice                            │
│   [View Code] [Compare] [Rollback]                      │
└─────────────────────────────────────────────────────────┘
```

### 3. 快照管理页面

```
┌─────────────────────────────────────────────────────────┐
│ Snapshots                              [+ Create Snapshot]│
├─────────────────────────────────────────────────────────┤
│ v1.0.0                                                   │
│   Stable release                                         │
│   5 resources  2025-10-11 10:00                         │
│   [View] [Restore] [Delete]                             │
│                                                           │
│ before-migration                                         │
│   Backup before database migration                       │
│   5 resources  2025-10-10 10:00                         │
│   [View] [Restore] [Delete]                             │
└─────────────────────────────────────────────────────────┘
```

## 📋 实施计划

### Phase 1: 数据库和模型（1周）
- [ ] 创建4个新表（resources, versions, snapshots, dependencies）
- [ ] 创建Go模型定义
- [ ] 编写数据库迁移脚本
- [ ] 测试数据库设计

### Phase 2: 核心服务（2周）
- [ ] 实现ResourceService（CRUD）
- [ ] 实现版本管理逻辑
- [ ] 实现快照管理
- [ ] 实现依赖关系管理
- [ ] 修改TerraformExecutor（支持-target）

### Phase 3: API接口（1周）
- [ ] 实现资源管理API
- [ ] 实现版本管理API
- [ ] 实现快照管理API
- [ ] 实现依赖关系API
- [ ] 编写API测试

### Phase 4: 前端UI（2周）
- [ ] 资源列表页面
- [ ] 资源编辑页面
- [ ] 版本历史页面
- [ ] 版本对比页面
- [ ] 快照管理页面

### Phase 5: 集成测试（1周）
- [ ] 端到端测试
- [ ] 性能测试
- [ ] 并发测试
- [ ] 回滚测试

##  注意事项

### 1. Terraform依赖关系

**问题**: 资源之间可能有依赖关系（如VPC和Subnet）

**解决方案**:
- 记录资源依赖关系到`resource_dependencies`表
- 回滚时自动包含依赖资源
- 使用Terraform的`-target`参数时按依赖顺序执行

```go
// 获取资源及其依赖
func (s *ResourceService) GetResourceWithDependencies(resourceID uint) ([]string, error) {
    var deps []ResourceDependency
    s.db.Where("resource_id = ?", resourceID).Find(&deps)
    
    targets := []string{getResourceID(resourceID)}
    for _, dep := range deps {
        targets = append(targets, getResourceID(dep.DependsOnResourceID))
    }
    
    return targets, nil
}
```

### 2. State一致性

**问题**: 资源版本变更后，State也需要相应更新

**解决方案**:
- 每次Apply后，记录资源版本和State版本的映射
- 在`resource_code_versions`表中添加`state_version_id`字段
- 回滚时同时考虑State版本

### 3. 并发控制

**问题**: 多人同时修改不同资源时的并发控制

**解决方案**:
- 使用乐观锁（版本号）
- 在更新资源时检查版本号
- 如果版本号不匹配，提示用户刷新后重试

```go
// 使用乐观锁更新资源
func (s *ResourceService) UpdateResourceWithLock(
    resourceID uint,
    expectedVersion int,
    tfCode map[string]interface{},
    changeSummary string,
    userID uint,
) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        var resource WorkspaceResource
        if err := tx.First(&resource, resourceID).Error; err != nil {
            return err
        }
        
        // 检查当前版本
        var currentVersion ResourceCodeVersion
        if err := tx.Where("id = ?", resource.CurrentVersionID).
            First(&currentVersion).Error; err != nil {
            return err
        }
        
        // 版本号不匹配，说明有其他人已经修改
        if currentVersion.Version != expectedVersion {
            return fmt.Errorf("resource has been modified by another user, please refresh")
        }
        
        // 继续更新...
        return nil
    })
}
```

### 4. 性能优化

**问题**: 大量资源时的查询性能

**解决方案**:
- 添加适当的索引
- 使用缓存（Redis）缓存资源列表
- 分页查询
- 延迟加载版本历史

```go
// 使用缓存
func (s *ResourceService) GetResourcesWithCache(workspaceID uint) ([]WorkspaceResource, error) {
    cacheKey := fmt.Sprintf("workspace:%d:resources", workspaceID)
    
    // 尝试从缓存获取
    if cached, err := s.cache.Get(cacheKey); err == nil {
        var resources []WorkspaceResource
        json.Unmarshal([]byte(cached), &resources)
        return resources, nil
    }
    
    // 从数据库查询
    var resources []WorkspaceResource
    s.db.Where("workspace_id = ? AND is_active = true", workspaceID).
        Preload("CurrentVersion").
        Find(&resources)
    
    // 缓存结果（5分钟）
    data, _ := json.Marshal(resources)
    s.cache.Set(cacheKey, string(data), 5*time.Minute)
    
    return resources, nil
}
```

### 5. 资源导入

**问题**: 如何导入现有的Terraform资源

**解决方案**:
- 提供导入功能，从现有的main.tf.json导入
- 自动解析资源并创建版本记录
- 支持批量导入

```go
// ImportResourcesFromTF 从Terraform配置导入资源
func (s *ResourceService) ImportResourcesFromTF(
    workspaceID uint,
    tfCode map[string]interface{},
    userID uint,
) error {
    // 解析resource块
    if resources, ok := tfCode["resource"].(map[string]interface{}); ok {
        for resourceType, resourcesOfType := range resources {
            if resourceMap, ok := resourcesOfType.(map[string]interface{}); ok {
                for resourceName, resourceConfig := range resourceMap {
                    // 为每个资源创建记录
                    s.AddResource(
                        workspaceID,
                        resourceType,
                        resourceName,
                        map[string]interface{}{
                            "resource": map[string]interface{}{
                                resourceType: map[string]interface{}{
                                    resourceName: resourceConfig,
                                },
                            },
                        },
                        userID,
                    )
                }
            }
        }
    }
    
    return nil
}
```

## 🎯 与现有系统的集成

### 1. 修改TerraformExecutor

需要修改`GenerateConfigFiles`方法，从资源聚合生成配置：

```go
// GenerateConfigFiles 生成所有配置文件（修改版）
func (s *TerraformExecutor) GenerateConfigFiles(
    workspace *models.Workspace,
    workDir string,
) error {
    // 1. 从资源聚合生成 main.tf.json
    mainTF, err := s.GenerateMainTFFromResources(workspace.ID)
    if err != nil {
        return fmt.Errorf("failed to generate main.tf from resources: %w", err)
    }
    
    if err := s.writeJSONFile(workDir, "main.tf.json", mainTF); err != nil {
        return fmt.Errorf("failed to write main.tf.json: %w", err)
    }
    
    // 2-4. 其他文件生成保持不变
    // ...
    
    return nil
}
```

### 2. 支持-target参数

修改Plan和Apply执行，支持target参数：

```go
// ExecutePlan 执行Plan任务（支持target）
func (s *TerraformExecutor) ExecutePlan(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // ... 前面的代码保持不变
    
    // 构建terraform plan命令
    args := []string{"plan", "-out=" + planFile, "-no-color", "-var-file=variables.tfvars"}
    
    // 添加target参数（如果有）
    if targets, ok := task.Context["targets"].([]string); ok {
        for _, target := range targets {
            args = append(args, target)
        }
    }
    
    cmd := exec.CommandContext(ctx, "terraform", args...)
    // ...
}
```

### 3. 迁移现有数据

提供迁移脚本，将现有的Workspace代码迁移到资源级别：

```sql
-- 迁移脚本
-- 1. 为每个workspace创建资源记录
INSERT INTO workspace_resources (workspace_id, resource_id, resource_type, resource_name, is_active, created_at)
SELECT 
    w.id,
    CONCAT(resource_type, '.', resource_name),
    resource_type,
    resource_name,
    true,
    NOW()
FROM workspaces w
CROSS JOIN LATERAL (
    -- 从tf_code中提取资源信息
    SELECT 
        key as resource_type,
        subkey as resource_name
    FROM jsonb_each(w.tf_code->'resource') as t1(key, value)
    CROSS JOIN LATERAL jsonb_object_keys(value) as t2(subkey)
) resources;

-- 2. 为每个资源创建初始版本
-- （需要在应用层实现，因为需要提取每个资源的具体配置）
```

## 📚 最佳实践

### 1. 资源命名规范

```
格式: <resource_type>.<resource_name>

示例:
- aws_s3_bucket.my_bucket
- aws_vpc.main
- aws_subnet.private_1a

建议:
- 使用有意义的名称
- 使用下划线分隔单词
- 避免使用特殊字符
```

### 2. 版本管理策略

```
- 每次修改都创建新版本
- 使用有意义的change_summary
- 定期创建快照（如发布前）
- 保留所有历史版本（不删除）
```

### 3. 回滚策略

```
- 回滚前先创建快照
- 回滚后执行Plan查看变更
- 确认无误后再Apply
- 记录回滚原因
```

### 4. 依赖管理

```
- 明确记录资源依赖关系
- 回滚时自动包含依赖资源
- 使用Terraform的depends_on显式声明依赖
```

## 🔄 与Workspace级别版本管理的对比

| 特性 | Workspace级别 | 资源级别 | 推荐 |
|------|--------------|---------|------|
| 适用场景 | 整体回滚 | 日常操作 | 两者结合 |
| 粒度 | 粗 | 细 | 资源级别 |
| 效率 | 低 | 高 | 资源级别 |
| 复杂度 | 低 | 中 | 可接受 |
| 灵活性 | 低 | 高 | 资源级别 |

**建议方案**:
- 保留Workspace级别的快照功能（用于整体回滚）
- 实现资源级别的版本管理（用于日常操作）
- 两者结合，提供最大的灵活性

## 🎉 总结

资源级别的代码版本管理是一个**优秀的设计改进**，它提供了：

### 核心优势
1.  **更细粒度的控制** - 每个资源独立管理
2.  **更高效的操作** - 使用-target只操作特定资源
3.  **更清晰的历史** - 每个资源的变更历史独立
4.  **更好的协作** - 多人可以并行工作
5.  **更灵活的部署** - 选择性部署和回滚

### 实施建议
- **Phase 1-2**: 优先实现（4周）
- **Phase 3-4**: 次要功能（3周）
- **Phase 5**: 测试和优化（1周）

### 技术要点
- 使用JSONB存储资源配置
- 使用-target实现选择性部署
- 使用快照实现整体回滚
- 使用依赖关系表管理资源依赖

这个设计完全可以替代之前的Workspace全局代码版本管理方案，并提供更好的用户体验！

---

**下一步**: 开始Phase 1的数据库设计和模型实现
