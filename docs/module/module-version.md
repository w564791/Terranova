# Module 多版本能力设计方案

## 📋 需求总结

1. **版本继承**: 更新 TF Module 版本时，从最新 Schema 版本复制数据
2. **默认版本**: 用户可设置 TF Module 的默认版本，系统永不自动修改
3. **复用 Schema 编辑**: 最大程度复用现有 Schema 编辑能力
4. **Demo 继承**: 新建 Module 不自动复制 Demo，但提供一键继承能力

---

## 🏗️ 数据库设计

### 新增表: `module_versions`

```sql
CREATE TABLE module_versions (
    id VARCHAR(30) PRIMARY KEY,           -- modv-xxx 语义化 ID
    module_id INT NOT NULL,               -- 外键关联 modules 表
    version VARCHAR(50) NOT NULL,         -- Terraform Module 版本 (如 6.1.5)
    source VARCHAR(500),                  -- Module source (可覆盖)
    module_source VARCHAR(500),           -- 完整 source URL
    is_default BOOLEAN DEFAULT false,     -- 是否为默认版本
    status VARCHAR(20) DEFAULT 'active',  -- active, deprecated, archived
    inherited_from_version_id VARCHAR(30),-- 继承自哪个版本（用于追溯）
    created_by VARCHAR(20),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    FOREIGN KEY (module_id) REFERENCES modules(id),
    UNIQUE (module_id, version)           -- 同一 Module 下版本唯一
);

-- 索引
CREATE INDEX idx_module_versions_module ON module_versions(module_id);
CREATE INDEX idx_module_versions_default ON module_versions(module_id, is_default);
```

### 修改表: `schemas`

```sql
-- 添加 module_version_id 字段
ALTER TABLE schemas ADD COLUMN module_version_id VARCHAR(30);
ALTER TABLE schemas ADD COLUMN inherited_from_schema_id INT;

-- 外键约束
ALTER TABLE schemas ADD CONSTRAINT fk_schemas_module_version 
    FOREIGN KEY (module_version_id) REFERENCES module_versions(id);

-- 索引
CREATE INDEX idx_schemas_module_version ON schemas(module_version_id);
```

### 修改表: `module_demos`

```sql
-- 添加 module_version_id 字段
ALTER TABLE module_demos ADD COLUMN module_version_id VARCHAR(30);
ALTER TABLE module_demos ADD COLUMN inherited_from_demo_id INT;

-- 外键约束
ALTER TABLE module_demos ADD CONSTRAINT fk_module_demos_module_version 
    FOREIGN KEY (module_version_id) REFERENCES module_versions(id);
```

---

## 📊 数据模型关系

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Module (平台模块)                              │
│  id: 48 (自增，保持不变)                                                 │
│  name: ec2-instance                                                     │
│  provider: AWS                                                          │
│  default_version_id: modv-abc123  ← 指向默认版本（用户手动设置）          │
├─────────────────────────────────────────────────────────────────────────┤
│                        Module Versions                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ modv-abc123 (v6.1.5)          │ modv-def456 (v6.2.0)               ││
│  │ is_default: true              │ is_default: false                  ││
│  │ inherited_from: null          │ inherited_from: modv-abc123        ││
│  │                               │                                    ││
│  │ Schemas:                      │ Schemas:                           ││
│  │ ├── v1 (inactive)             │ ├── v1 (从 v6.1.5 的 v10 继承)     ││
│  │ ├── v2 (inactive)             │ └── v2 (active, 用户修改)          ││
│  │ └── v10 (active)              │                                    ││
│  │                               │                                    ││
│  │ Demos:                        │ Demos:                             ││
│  │ ├── demo-1                    │ └── (空，需要手动继承)              ││
│  │ └── demo-2                    │                                    ││
│  └─────────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 🔄 核心流程设计

### 1. 创建新 TF Module 版本（继承 Schema）

```go
// POST /modules/:id/versions
type CreateModuleVersionRequest struct {
    Version           string `json:"version" binding:"required"`  // 新 TF 版本号
    Source            string `json:"source"`                      // 可选覆盖 source
    InheritSchemaFrom string `json:"inherit_schema_from"`         // 从哪个版本继承 Schema
}

func (s *ModuleVersionService) CreateVersion(moduleID uint, req *CreateModuleVersionRequest) (*ModuleVersion, error) {
    // 1. 创建新版本记录
    newVersion := &ModuleVersion{
        ID:        utils.GenerateID("modv"),
        ModuleID:  moduleID,
        Version:   req.Version,
        IsDefault: false,  // 永不自动设为默认
    }
    
    // 2. 如果指定了继承来源，复制 Schema
    if req.InheritSchemaFrom != "" {
        // 获取源版本的最新 active Schema
        sourceSchema, err := s.getLatestActiveSchema(req.InheritSchemaFrom)
        if err != nil {
            return nil, err
        }
        
        // 复制 Schema 数据
        newSchema := &Schema{
            ModuleID:              moduleID,
            ModuleVersionID:       newVersion.ID,
            Version:               "1",  // 新版本从 v1 开始
            Status:                "draft",  // 初始为草稿，让用户修改
            SchemaData:            sourceSchema.SchemaData,
            OpenAPISchema:         sourceSchema.OpenAPISchema,
            UIConfig:              sourceSchema.UIConfig,
            InheritedFromSchemaID: &sourceSchema.ID,
        }
        
        if err := s.db.Create(newSchema).Error; err != nil {
            return nil, err
        }
        
        newVersion.InheritedFromVersionID = &req.InheritSchemaFrom
    }
    
    return newVersion, s.db.Create(newVersion).Error
}
```

### 2. 设置默认版本（用户手动操作）

```go
// PUT /modules/:id/default-version
type SetDefaultVersionRequest struct {
    VersionID string `json:"version_id" binding:"required"`
}

func (s *ModuleVersionService) SetDefaultVersion(moduleID uint, versionID string) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        // 1. 取消当前默认版本
        if err := tx.Model(&ModuleVersion{}).
            Where("module_id = ? AND is_default = ?", moduleID, true).
            Update("is_default", false).Error; err != nil {
            return err
        }
        
        // 2. 设置新的默认版本
        if err := tx.Model(&ModuleVersion{}).
            Where("id = ? AND module_id = ?", versionID, moduleID).
            Update("is_default", true).Error; err != nil {
            return err
        }
        
        // 3. 更新 Module 的 default_version_id
        return tx.Model(&Module{}).
            Where("id = ?", moduleID).
            Update("default_version_id", versionID).Error
    })
}
```

### 3. Demo 一键继承

```go
// POST /modules/:id/versions/:versionId/inherit-demos
type InheritDemosRequest struct {
    FromVersionID string   `json:"from_version_id" binding:"required"`
    DemoIDs       []uint   `json:"demo_ids"`  // 可选，不传则继承全部
}

func (s *ModuleDemoService) InheritDemos(moduleID uint, targetVersionID string, req *InheritDemosRequest) error {
    // 获取源版本的 Demos
    var sourceDemos []ModuleDemo
    query := s.db.Where("module_id = ? AND module_version_id = ?", moduleID, req.FromVersionID)
    if len(req.DemoIDs) > 0 {
        query = query.Where("id IN ?", req.DemoIDs)
    }
    if err := query.Find(&sourceDemos).Error; err != nil {
        return err
    }
    
    // 复制 Demos
    for _, demo := range sourceDemos {
        newDemo := ModuleDemo{
            ModuleID:            moduleID,
            ModuleVersionID:     &targetVersionID,
            Name:                demo.Name,
            Description:         demo.Description,
            InheritedFromDemoID: &demo.ID,
            // ... 复制其他字段
        }
        
        // 同时复制 Demo 的版本数据
        if demo.CurrentVersion != nil {
            newDemoVersion := ModuleDemoVersion{
                ConfigData: demo.CurrentVersion.ConfigData,
                // ...
            }
            // ...
        }
    }
    
    return nil
}
```

---

## 🖥️ API 设计

### Module Versions API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/modules/:id/versions` | 获取 Module 的所有版本 |
| POST | `/modules/:id/versions` | 创建新版本（可继承 Schema） |
| GET | `/modules/:id/versions/:versionId` | 获取版本详情 |
| PUT | `/modules/:id/versions/:versionId` | 更新版本信息 |
| DELETE | `/modules/:id/versions/:versionId` | 删除版本 |
| PUT | `/modules/:id/default-version` | 设置默认版本 |
| POST | `/modules/:id/versions/:versionId/inherit-demos` | 继承 Demos |

### Schema API 调整

```go
// 现有 API 保持不变，增加 version_id 参数
GET  /modules/:id/schemas/v2?version_id=modv-xxx
POST /modules/:id/schemas/v2?version_id=modv-xxx
```

**已实现**：`GetSchemaV2` 和 `CreateSchemaV2` 方法已支持 `version_id` 查询参数：
- `GET /modules/:id/schemas/v2?version_id=modv-xxx` - 获取指定版本的 Schema
- `POST /modules/:id/schemas/v2?version_id=modv-xxx` - 创建 Schema 并关联到指定版本
- 如果不传 `version_id`，`CreateSchemaV2` 会自动关联到模块的默认版本

---

## 📱 前端交互设计

### 版本选择器
```
┌─────────────────────────────────────────────────────────────────┐
│  Module: ec2-instance                                           │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Version: [v6.1.5 (默认) ▼]                                  ││
│  │          ├── v6.1.5 (默认) ✓                                ││
│  │          ├── v6.2.0                                         ││
│  │          └── + 添加新版本                                    ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                 │
│  [设为默认版本]  [继承 Demos]  [删除版本]                         │
└─────────────────────────────────────────────────────────────────┘
```

### 创建新版本对话框
```
┌─────────────────────────────────────────────────────────────────┐
│  创建新 Terraform Module 版本                                    │
├─────────────────────────────────────────────────────────────────┤
│  版本号: [6.2.0                    ]                            │
│                                                                 │
│  Source (可选): [terraform-aws-modules/ec2-instance/aws]        │
│                                                                 │
│  ☑ 从现有版本继承 Schema                                         │
│     继承自: [v6.1.5 (Schema v10) ▼]                             │
│                                                                 │
│  ☐ 继承 Demos                                                   │
│     (创建后可单独继承)                                           │
│                                                                 │
│                              [取消]  [创建]                      │
└─────────────────────────────────────────────────────────────────┘
```

---

##  关键约束

1. **默认版本永不自动修改**
   - 只有用户手动点击"设为默认版本"才会修改
   - 创建新版本时 `is_default = false`
   - 删除默认版本时提示用户先设置其他版本为默认

2. **Schema 继承是复制，不是引用**
   - 继承后的 Schema 是独立的副本
   - 修改不会影响源 Schema
   - 保留 `inherited_from_schema_id` 用于追溯

3. **Demo 不自动继承**
   - 创建新版本时 Demo 列表为空
   - 提供"一键继承"按钮让用户选择性继承

---

## 📅 实施计划

| 阶段 | 任务 | 工作量 |
|------|------|--------|
| Phase 1 | 数据库迁移（新增表、修改表） | 0.5 天 |
| Phase 2 | 后端 API 开发 | 2 天 |
| Phase 3 | 前端版本选择器 | 1 天 |
| Phase 4 | Schema 编辑器适配 | 0.5 天 |
| Phase 5 | Demo 继承功能 | 0.5 天 |
| Phase 6 | 测试与修复 | 0.5 天 |

**总计**: 约 **5 天**

---

## 🔗 与语义化 ID 的关系

**此方案不依赖 modules 表的语义化 ID 迁移**：
- `module_versions` 表直接使用语义化 ID (`modv-xxx`)
- `modules` 表保持自增 ID 不变
- 未来可独立迁移 `modules` 表

---

## ✅ 对 Manifest 的影响

**本次变更对 Manifest 没有影响**

### 原因分析

1. **不添加 Module 语义化 ID**
   - `modules.id` 保持自增 int 类型
   - Manifest 中的 `module_id` 字段无需修改

2. **使用 `module_version` + 默认 Schema 组合**
   - Manifest 节点已有 `module_version` 字段存储 TF Module 版本（如 `6.1.5`）
   - 部署时自动使用该 TF 版本对应的**默认 Schema**
   - 无需在 Manifest 中新增字段

### Manifest 节点结构（保持不变）
```json
{
  "id": "node-xxx",
  "type": "module",
  "module_id": 48,                    // ← 保持 int，不变
  "module_source": "terraform-aws-modules/ec2-instance/aws",
  "module_version": "6.1.5",          // ← 已有，用于指定 TF 版本
  "config": { ... }
}
```

### 部署流程（无需修改）
```
Manifest 部署
    ↓
读取 module_id=48, module_version="6.1.5"
    ↓
查找 module_versions 表中 module_id=48 且 version="6.1.5" 的记录
    ↓
获取该版本的默认 Schema（status=active）
    ↓
生成 TF 代码并部署
```

### 变更范围确认

| 组件 | 是否需要修改 |
|------|-------------|
| `modules` 表 | ✅ 添加 `default_version_id` 字段 |
| `module_versions` 表 | ✅ 新建 |
| `schemas` 表 | ✅ 添加 `module_version_id` 字段 |
| `module_demos` 表 | ✅ 添加 `module_version_id` 字段 |
| **`manifest_versions` 表** | **❌ 不需要修改** |
| **`manifest_handler.go`** | **❌ 不需要修改** |

---

## 🚀 实施状态

**状态**: ✅ 后端代码已完成，待执行 SQL 迁移

### 已完成的文件

| 文件 | 说明 |
|------|------|
| `backend/internal/models/module_version.go` | ModuleVersion Model |
| `backend/internal/models/module.go` | 添加 `default_version_id` 字段 |
| `backend/internal/models/schema.go` | 添加 `module_version_id` 和 `inherited_from_schema_id` 字段 |
| `backend/internal/models/module_demo.go` | 添加 `module_version_id` 和 `inherited_from_demo_id` 字段 |
| `backend/services/module_version_service.go` | ModuleVersion Service |
| `backend/controllers/module_version_controller.go` | ModuleVersion Controller |
| `backend/internal/router/router_module.go` | 添加版本管理路由 |
| `scripts/create_module_versions_table.sql` | SQL 迁移脚本 |

### 执行步骤

1. **部署后端代码**（已完成编译验证）
2. **执行 SQL 迁移脚本**：
   ```bash
   psql -h localhost -U postgres -d iac_platform -f scripts/create_module_versions_table.sql
   ```
3. **验证迁移结果**：脚本会自动输出迁移统计

### API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/modules/:id/versions` | 获取模块的所有版本 |
| GET | `/api/v1/modules/:id/versions/:version_id` | 获取版本详情 |
| GET | `/api/v1/modules/:id/default-version` | 获取默认版本 |
| GET | `/api/v1/modules/:id/versions/compare` | 比较两个版本的 Schema 差异 |
| GET | `/api/v1/modules/:id/versions/:version_id/schemas` | 获取版本的所有 Schema |
| GET | `/api/v1/modules/:id/versions/:version_id/demos` | 获取版本的所有 Demo |
| POST | `/api/v1/modules/:id/versions` | 创建新版本 |
| PUT | `/api/v1/modules/:id/versions/:version_id` | 更新版本信息 |
| PUT | `/api/v1/modules/:id/default-version` | 设置默认版本 |
| POST | `/api/v1/modules/:id/versions/:version_id/inherit-demos` | 继承 Demos |
| DELETE | `/api/v1/modules/:id/versions/:version_id` | 删除版本 |
| POST | `/api/v1/modules/migrate-versions` | 迁移现有模块数据（管理员） |
