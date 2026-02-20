# Admin管理功能

## 概述

Admin管理模块提供平台级别的配置和管理功能，包括Terraform版本管理、系统配置等。这是平台的基础设施功能，为其他模块提供支持。

---

## 功能范围

### 1. Terraform版本管理 🔥

管理平台支持的Terraform版本，包括版本配置、下载链接、校验和等。

#### 核心功能
-  查看所有已配置的Terraform版本
-  添加新的Terraform版本
-  编辑版本信息
-  启用/禁用版本
-  标记版本为Deprecated
-  删除版本

#### 版本配置字段
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| version | string | 是 | 版本号（如1.5.0） |
| download_url | string | 是 | 下载链接 |
| checksum | string | 是 | SHA256校验和 |
| enabled | boolean | 是 | 是否启用该版本 |
| deprecated | boolean | 否 | 是否标记为已弃用 |

---

## UI设计

### Admin导航

在左侧导航栏添加"Admin"入口：

```
Dashboard
Workspaces
Modules
Admin          ← 新增
```

### Terraform版本管理页面

```
┌─ Admin > Terraform Versions ─────────────────────┐
│                                                   │
│ Terraform Versions                                │
│                                                   │
│ Manage Terraform versions available for          │
│ workspaces. Configure download URLs and           │
│ checksums for version verification.               │
│                                                   │
│ [+ Add Version]                                   │
│                                                   │
│ ┌─ Available Versions ─────────────────────────┐ │
│ │ VERSION  DOWNLOAD URL         STATUS  ACTIONS││
│ ├───────────────────────────────────────────────┤ │
│ │ 1.5.0    https://releases...   Enabled     ││
│ │          Checksum: abc123...                  ││
│ │          Added: 2025-01-01   [Edit] [Delete] ││
│ ├───────────────────────────────────────────────┤ │
│ │ 1.4.6    https://releases...   Enabled     ││
│ │          Checksum: def456...                  ││
│ │          Added: 2025-01-01   [Edit] [Delete] ││
│ ├───────────────────────────────────────────────┤ │
│ │ 1.3.9    https://releases...   Deprecated  ││
│ │          Checksum: ghi789...                  ││
│ │          Added: 2024-12-01   [Edit] [Delete] ││
│ ├───────────────────────────────────────────────┤ │
│ │ 1.2.0    https://releases...  ❌ Disabled    ││
│ │          Checksum: jkl012...                  ││
│ │          Added: 2024-11-01   [Edit] [Delete] ││
│ └───────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────┘
```

### 添加/编辑版本对话框

```
┌─ Add Terraform Version ──────────────────────────┐
│                                                   │
│ Version *                                         │
│ [1.5.0_________________]                          │
│ Example: 1.5.0, 1.4.6                            │
│                                                   │
│ Download URL *                                    │
│ [https://releases.hashicorp.com/terraform/...]   │
│ [___________________________________________]     │
│ Official Terraform release URL                    │
│                                                   │
│ SHA256 Checksum *                                 │
│ [abc123def456789...]                              │
│ [___________________________________________]     │
│ Used to verify download integrity                 │
│                                                   │
│ Options                                           │
│ ☑ Enable this version                            │
│   Make this version available for workspaces      │
│                                                   │
│ ☐ Mark as deprecated                             │
│   Show warning when using this version            │
│                                                   │
│                          [Cancel] [Save]          │
└───────────────────────────────────────────────────┘
```

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
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 索引
CREATE INDEX idx_terraform_versions_enabled ON terraform_versions(enabled);
CREATE INDEX idx_terraform_versions_version ON terraform_versions(version);

-- 默认数据
INSERT INTO terraform_versions (version, download_url, checksum, enabled) VALUES
('1.5.0', 'https://releases.hashicorp.com/terraform/1.5.0/terraform_1.5.0_linux_amd64.zip', 'abc123...', true),
('1.4.6', 'https://releases.hashicorp.com/terraform/1.4.6/terraform_1.4.6_linux_amd64.zip', 'def456...', true),
('1.3.9', 'https://releases.hashicorp.com/terraform/1.3.9/terraform_1.3.9_linux_amd64.zip', 'ghi789...', false);
```

---

## API设计

### 1. 获取所有Terraform版本

```
GET /api/v1/admin/terraform-versions
```

**Query参数**:
- `enabled` (optional): 过滤启用状态 (true/false)
- `deprecated` (optional): 过滤弃用状态 (true/false)

**响应**:
```json
{
  "items": [
    {
      "id": 1,
      "version": "1.5.0",
      "download_url": "https://releases.hashicorp.com/terraform/1.5.0/terraform_1.5.0_linux_amd64.zip",
      "checksum": "abc123...",
      "enabled": true,
      "deprecated": false,
      "created_at": "2025-01-01T00:00:00Z",
      "updated_at": "2025-01-01T00:00:00Z"
    }
  ],
  "total": 10
}
```

### 2. 创建Terraform版本

```
POST /api/v1/admin/terraform-versions
```

**请求体**:
```json
{
  "version": "1.5.0",
  "download_url": "https://releases.hashicorp.com/terraform/1.5.0/terraform_1.5.0_linux_amd64.zip",
  "checksum": "abc123...",
  "enabled": true,
  "deprecated": false
}
```

**响应**: 201 Created
```json
{
  "id": 1,
  "version": "1.5.0",
  "download_url": "https://...",
  "checksum": "abc123...",
  "enabled": true,
  "deprecated": false,
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-01T00:00:00Z"
}
```

### 3. 更新Terraform版本

```
PUT /api/v1/admin/terraform-versions/:id
```

**请求体**:
```json
{
  "download_url": "https://...",
  "checksum": "abc123...",
  "enabled": true,
  "deprecated": false
}
```

**响应**: 200 OK

### 4. 删除Terraform版本

```
DELETE /api/v1/admin/terraform-versions/:id
```

**响应**: 204 No Content

**注意**: 如果有workspace正在使用该版本，应该返回错误或警告。

---

## 业务逻辑

### 版本验证

1. **版本号格式验证**
   - 必须符合语义化版本格式（如1.5.0）
   - 不能重复

2. **下载URL验证**
   - 必须是有效的URL
   - 建议验证URL可访问性

3. **Checksum验证**
   - 必须是64位SHA256哈希值
   - 格式：小写十六进制字符串

### 版本状态管理

1. **启用/禁用**
   - 禁用的版本不会在创建workspace时显示
   - 已使用该版本的workspace不受影响

2. **Deprecated标记**
   - 标记为deprecated的版本会显示警告
   - 仍然可以使用，但建议升级

3. **删除限制**
   - 如果有workspace正在使用，不允许删除
   - 或者提供强制删除选项（需要确认）

### 与Workspace的集成

1. **创建Workspace时**
   - 只显示enabled=true的版本
   - Deprecated版本显示警告图标

2. **Workspace详情页**
   - 显示当前使用的Terraform版本
   - 如果版本已deprecated，显示升级建议

---

## 前端实现

### 组件结构

```
pages/
├── Admin.tsx                    # Admin主页面
└── Admin.module.css             # 样式文件

components/
├── TerraformVersionList.tsx     # 版本列表
├── TerraformVersionDialog.tsx   # 添加/编辑对话框
└── TerraformVersionItem.tsx     # 版本列表项
```

### 状态管理

```typescript
interface TerraformVersion {
  id: number;
  version: string;
  download_url: string;
  checksum: string;
  enabled: boolean;
  deprecated: boolean;
  created_at: string;
  updated_at: string;
}

const [versions, setVersions] = useState<TerraformVersion[]>([]);
const [loading, setLoading] = useState(true);
const [showDialog, setShowDialog] = useState(false);
const [editingVersion, setEditingVersion] = useState<TerraformVersion | null>(null);
```

### API调用

```typescript
// services/admin.ts
export const adminService = {
  // 获取所有版本
  getTerraformVersions: async (params?: {
    enabled?: boolean;
    deprecated?: boolean;
  }) => {
    return api.get('/admin/terraform-versions', { params });
  },

  // 创建版本
  createTerraformVersion: async (data: CreateTerraformVersionRequest) => {
    return api.post('/admin/terraform-versions', data);
  },

  // 更新版本
  updateTerraformVersion: async (id: number, data: UpdateTerraformVersionRequest) => {
    return api.put(`/admin/terraform-versions/${id}`, data);
  },

  // 删除版本
  deleteTerraformVersion: async (id: number) => {
    return api.delete(`/admin/terraform-versions/${id}`);
  }
};
```

---

## 后端实现

### Model

```go
// internal/models/terraform_version.go
type TerraformVersion struct {
    ID          int       `json:"id" db:"id"`
    Version     string    `json:"version" db:"version" binding:"required"`
    DownloadURL string    `json:"download_url" db:"download_url" binding:"required,url"`
    Checksum    string    `json:"checksum" db:"checksum" binding:"required,len=64"`
    Enabled     bool      `json:"enabled" db:"enabled"`
    Deprecated  bool      `json:"deprecated" db:"deprecated"`
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type CreateTerraformVersionRequest struct {
    Version     string `json:"version" binding:"required"`
    DownloadURL string `json:"download_url" binding:"required,url"`
    Checksum    string `json:"checksum" binding:"required,len=64"`
    Enabled     bool   `json:"enabled"`
    Deprecated  bool   `json:"deprecated"`
}

type UpdateTerraformVersionRequest struct {
    DownloadURL string `json:"download_url" binding:"omitempty,url"`
    Checksum    string `json:"checksum" binding:"omitempty,len=64"`
    Enabled     *bool  `json:"enabled"`
    Deprecated  *bool  `json:"deprecated"`
}
```

### Service

```go
// services/terraform_version_service.go
type TerraformVersionService struct {
    db *sql.DB
}

func (s *TerraformVersionService) List(enabled *bool, deprecated *bool) ([]models.TerraformVersion, error)
func (s *TerraformVersionService) GetByID(id int) (*models.TerraformVersion, error)
func (s *TerraformVersionService) Create(req *models.CreateTerraformVersionRequest) (*models.TerraformVersion, error)
func (s *TerraformVersionService) Update(id int, req *models.UpdateTerraformVersionRequest) (*models.TerraformVersion, error)
func (s *TerraformVersionService) Delete(id int) error
func (s *TerraformVersionService) CheckVersionInUse(id int) (bool, error)
```

### Controller

```go
// controllers/terraform_version_controller.go
func ListTerraformVersions(c *gin.Context) {
    enabled := c.Query("enabled")
    deprecated := c.Query("deprecated")
    
    versions, err := service.List(enabled, deprecated)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{
        "items": versions,
        "total": len(versions),
    })
}

func CreateTerraformVersion(c *gin.Context) {
    var req models.CreateTerraformVersionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    version, err := service.Create(&req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(201, version)
}

func UpdateTerraformVersion(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    
    var req models.UpdateTerraformVersionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    version, err := service.Update(id, &req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, version)
}

func DeleteTerraformVersion(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    
    // 检查是否有workspace在使用
    inUse, err := service.CheckVersionInUse(id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    if inUse {
        c.JSON(400, gin.H{"error": "Version is in use by workspaces"})
        return
    }
    
    if err := service.Delete(id); err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.Status(204)
}
```

### Router

```go
// internal/router/router.go
func SetupRouter() *gin.Engine {
    r := gin.Default()
    
    api := r.Group("/api/v1")
    {
        // Admin routes
        admin := api.Group("/admin")
        {
            admin.GET("/terraform-versions", controllers.ListTerraformVersions)
            admin.POST("/terraform-versions", controllers.CreateTerraformVersion)
            admin.PUT("/terraform-versions/:id", controllers.UpdateTerraformVersion)
            admin.DELETE("/terraform-versions/:id", controllers.DeleteTerraformVersion)
        }
    }
    
    return r
}
```

---

## 安全考虑

### 1. 权限控制
- Admin功能应该只对管理员开放
- 需要实现基于角色的访问控制（RBAC）
- 普通用户只能查看，不能修改

### 2. 输入验证
- 版本号格式验证
- URL格式验证
- Checksum格式验证（64位SHA256）
- 防止SQL注入

### 3. 下载安全
- 验证下载URL的合法性
- 使用Checksum验证下载文件的完整性
- 建议只允许官方Terraform下载链接

---

## 测试计划

### 单元测试
- [ ] TerraformVersionService.Create
- [ ] TerraformVersionService.Update
- [ ] TerraformVersionService.Delete
- [ ] TerraformVersionService.CheckVersionInUse
- [ ] 版本号格式验证
- [ ] Checksum格式验证

### 集成测试
- [ ] API端点测试
- [ ] 数据库操作测试
- [ ] 权限控制测试

### E2E测试
- [ ] 添加版本流程
- [ ] 编辑版本流程
- [ ] 删除版本流程
- [ ] 启用/禁用版本
- [ ] 标记Deprecated

---

## 实现计划

### Phase 1: 数据库和后端（1小时）
- [ ] 创建terraform_versions表
- [ ] 实现Model
- [ ] 实现Service
- [ ] 实现Controller
- [ ] 添加路由

### Phase 2: 前端页面（1.5小时）
- [ ] 创建Admin.tsx页面
- [ ] 实现版本列表组件
- [ ] 实现添加/编辑对话框
- [ ] 实现删除功能
- [ ] API集成

### Phase 3: 导航和集成（30分钟）
- [ ] 更新左侧导航
- [ ] 添加路由
- [ ] 权限控制

### Phase 4: 测试和优化（30分钟）
- [ ] 功能测试
- [ ] UI优化
- [ ] 错误处理

**总计**: 约3.5小时

---

## 未来扩展

### 1. 自动版本检测
- 定期检查Terraform官方发布
- 自动提示新版本
- 一键添加新版本

### 2. 版本使用统计
- 统计每个版本的使用情况
- 显示最常用的版本
- 帮助决策哪些版本可以弃用

### 3. 批量操作
- 批量启用/禁用版本
- 批量标记Deprecated

### 4. 版本迁移工具
- 帮助workspace升级Terraform版本
- 自动检测兼容性问题

---

## 相关文档

### Admin模块文档
- [README.md](./README.md) - Admin模块总览
- [02-api-specification.md](./02-api-specification.md) - Admin API规范
- [development-progress.md](./development-progress.md) - 开发进度

### 开发规范（参考Workspace模块）
- [../workspace/09-api-specification.md](../workspace/09-api-specification.md) - API开发规范
- [../workspace/11-frontend-design.md](../workspace/11-frontend-design.md) - 前端设计规范
- [../workspace/10-implementation-guide.md](../workspace/10-implementation-guide.md) - 实现指南

### 项目文档
- [../QUICK_START_FOR_AI.md](../QUICK_START_FOR_AI.md) - AI开发快速入口
- [../workspace/README.md](../workspace/README.md) - Workspace模块文档
