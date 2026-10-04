# Manifest 可视化编排器设计方案

## 一、概述

### 1.1 背景
IaC 平台提供了便捷的 UI 界面，但失去了代码的灵活性。Manifest 功能旨在提供一个可视化的拖拉拽界面，让用户可以组合多个 Module 资源，并生成 HCL 文件，兼顾初级用户和高级用户的需求。

### 1.2 核心目标
- **可视化编排**: 通过拖拉拽界面组合多个 Module
- **全局复用**: Organization 级别的模板，可部署到多个 Workspace
- **双向导入导出**: 支持 HCL 格式的导入导出
- **依赖管理**: 可视化定义 Module 之间的依赖关系和变量绑定
- **草稿保存**: 支持保存不完整的配置，随时继续编辑

### 1.3 技术选型
- **画布库**: React Flow（与 React 生态集成好，自定义节点灵活）
- **输出格式**: HCL / Terraform JSON

### 1.4 设计决策
- **Organization 级别**: Manifest 属于 Organization，可复用到多个 Workspace
- **版本管理**: 支持 Manifest 版本，不同 Workspace 可使用不同版本
- **部署概念**: 将 Manifest 部署到 Workspace，创建实际资源
- **复用现有接口**: 部署时复用现有的资源添加和部署接口

---

## 二、核心概念

### 2.1 概念模型

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Organization   │────▶│    Manifest     │────▶│ ManifestVersion │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                               │                        │
                               │                        │ 部署到
                               │                        ▼
                               │                 ┌─────────────────┐
                               │                 │   Deployment    │
                               │                 │  (部署记录)     │
                               │                 └─────────────────┘
                               │                        │
                               │                        │ 创建
                               ▼                        ▼
                        ┌─────────────────┐     ┌─────────────────┐
                        │   Workspace     │◀────│    Resources    │
                        └─────────────────┘     └─────────────────┘
```

### 2.2 核心实体

| 实体 | 说明 | 级别 |
|------|------|------|
| Manifest | 基础设施编排模板 | Organization |
| ManifestVersion | Manifest 的版本 | Organization |
| Deployment | 部署记录（Manifest → Workspace） | Workspace |
| Resources | 实际创建的资源 | Workspace |

### 2.3 使用流程

```
1. 在 Organization 级别创建 Manifest（模板）
2. 编辑 Manifest：添加 Module 节点、配置参数、定义依赖
3. 发布版本（可选）
4. 选择目标 Workspace 进行部署
5. 部署时可以覆盖部分参数（如环境变量）
6. 系统在 Workspace 中创建资源并执行 Terraform
7. 同一个 Manifest 可以部署到多个 Workspace
```

### 2.4 全局视角

在 Organization 级别可以看到：
- 所有 Manifest 列表
- 每个 Manifest 部署到了哪些 Workspace
- 每个 Workspace 使用的 Manifest 版本
- 部署状态和历史

---

## 三、数据模型设计

### 3.1 数据库表设计

#### manifests 表（Organization 级别）
```sql
CREATE TABLE manifests (
    id              VARCHAR(36) PRIMARY KEY,  -- 格式: mf-{ulid}
    organization_id VARCHAR(36) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    status          VARCHAR(20) DEFAULT 'draft',  -- draft, published, archived
    created_by      VARCHAR(26) NOT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(organization_id, name),
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id)
);

CREATE INDEX idx_manifests_organization_id ON manifests(organization_id);
CREATE INDEX idx_manifests_status ON manifests(status);
```

#### manifest_versions 表
```sql
CREATE TABLE manifest_versions (
    id              VARCHAR(36) PRIMARY KEY,  -- 格式: mfv-{ulid}
    manifest_id     VARCHAR(36) NOT NULL,
    version         VARCHAR(50) NOT NULL,     -- 如 v1.0.0, draft
    canvas_data     JSONB NOT NULL,           -- 画布数据（节点位置、缩放等）
    nodes           JSONB NOT NULL,           -- 节点配置
    edges           JSONB NOT NULL,           -- 连接关系
    variables       JSONB,                    -- 可配置的变量定义
    hcl_content     TEXT,                     -- 生成的 HCL 内容
    is_draft        BOOLEAN DEFAULT true,     -- 是否为草稿
    created_by      VARCHAR(26) NOT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(manifest_id, version),
    FOREIGN KEY (manifest_id) REFERENCES manifests(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id)
);

CREATE INDEX idx_manifest_versions_manifest_id ON manifest_versions(manifest_id);
```

#### manifest_deployments 表（部署记录）
```sql
CREATE TABLE manifest_deployments (
    id              VARCHAR(36) PRIMARY KEY,  -- 格式: mfd-{ulid}
    manifest_id     VARCHAR(36) NOT NULL,
    version_id      VARCHAR(36) NOT NULL,
    workspace_id    VARCHAR(36) NOT NULL,
    variable_overrides JSONB,                 -- 部署时覆盖的变量
    status          VARCHAR(20) DEFAULT 'pending',  -- pending, deploying, deployed, failed
    last_task_id    INTEGER,                  -- 最后一次部署的任务 ID
    deployed_by     VARCHAR(26) NOT NULL,
    deployed_at     TIMESTAMP,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(manifest_id, workspace_id),        -- 每个 Workspace 只能有一个部署
    FOREIGN KEY (manifest_id) REFERENCES manifests(id) ON DELETE CASCADE,
    FOREIGN KEY (version_id) REFERENCES manifest_versions(id),
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (deployed_by) REFERENCES users(id)
);

CREATE INDEX idx_manifest_deployments_manifest_id ON manifest_deployments(manifest_id);
CREATE INDEX idx_manifest_deployments_workspace_id ON manifest_deployments(workspace_id);
CREATE INDEX idx_manifest_deployments_status ON manifest_deployments(status);
```

#### manifest_deployment_resources 表（部署资源关联）
```sql
CREATE TABLE manifest_deployment_resources (
    id              VARCHAR(36) PRIMARY KEY,  -- 格式: mdr-{ulid}
    deployment_id   VARCHAR(36) NOT NULL,
    node_id         VARCHAR(50) NOT NULL,     -- manifest 中的节点 ID
    resource_id     INTEGER NOT NULL,         -- workspace_resources.id
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(deployment_id, node_id),
    FOREIGN KEY (deployment_id) REFERENCES manifest_deployments(id) ON DELETE CASCADE,
    FOREIGN KEY (resource_id) REFERENCES workspace_resources(id) ON DELETE CASCADE
);

CREATE INDEX idx_manifest_deployment_resources_deployment_id ON manifest_deployment_resources(deployment_id);
```

### 3.2 JSONB 数据结构

#### nodes 结构
```typescript
interface ManifestNode {
  id: string;                    // 节点唯一标识
  type: 'module' | 'variable';   // 节点类型
  
  // Module 节点属性
  module_id?: number;            // 关联的平台 Module ID（可选）
  is_linked: boolean;            // 是否已关联平台 Module
  link_status: 'linked' | 'unlinked' | 'mismatch';
  module_source?: string;        // Module source
  module_version?: string;       // Module 版本
  instance_name: string;         // 实例名称（如 vpc_main）
  resource_name: string;         // 资源名称
  
  // 未关联时的原始数据
  raw_source?: string;
  raw_version?: string;
  raw_config?: Record<string, any>;
  
  // 位置信息
  position: { x: number; y: number; };
  
  // 配置参数
  config: Record<string, any>;
  config_complete: boolean;
  
  // 暴露的端口（触手）
  ports: ManifestPort[];
}

interface ManifestPort {
  id: string;
  type: 'input' | 'output';
  name: string;
  data_type?: string;
  description?: string;
}
```

#### variables 结构（可配置变量）
```typescript
interface ManifestVariable {
  name: string;           // 变量名
  type: string;           // 类型：string, number, bool, list, map
  description?: string;   // 描述
  default?: any;          // 默认值
  required: boolean;      // 是否必填
  sensitive?: boolean;    // 是否敏感
}
```

部署时可以通过 `variable_overrides` 覆盖这些变量的值。

### 3.3 资源命名规则

#### 命名格式
```
{Provider}_{module_name}_{resource_name}
```

**注意**: Provider 使用大写（如 AWS、Google、Azure）

**示例**:
- `AWS_vpc_main` - AWS VPC 模块的主实例
- `AWS_ec2_web_server` - AWS EC2 模块的 Web 服务器实例
- `Google_gke_cluster_prod` - Google GKE 模块的生产集群

---

## 四、UI 设计

### 4.1 入口位置

Manifest 管理入口在 **Organization 级别**：
```
Organization
├── Workspaces
├── Modules
├── Manifests  ← 新增入口
├── Teams
└── Settings
```

### 4.2 Manifest 列表页

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  Manifests                                              [+ 创建 Manifest]    │
├──────────────────────────────────────────────────────────────────────────────┤
│  名称              版本        部署数    状态        最后更新        操作    │
├──────────────────────────────────────────────────────────────────────────────┤
│  vpc-ec2-stack     v1.2.0      3        [已发布]    2025-01-01     [编辑] [部署] │
│                    部署到: dev, staging, prod                                 │
│                                                                              │
│  database-stack    v1.0.0      1        [已发布]    2024-12-31     [编辑] [部署] │
│                    部署到: prod                                               │
│                                                                              │
│  new-stack         draft       0        [草稿]      2024-12-30     [编辑]      │
│                    未部署                                                     │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 4.3 Manifest 编辑器

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Manifest: vpc-ec2-stack (v1.2.0)                                           │
│  [保存草稿] [发布版本] [导出HCL] [导入HCL] [部署到 Workspace ▼]              │
├────────────┬────────────────────────────────────────────────┬───────────────┤
│            │                                                │               │
│  Module库  │              画布区域                          │   属性面板    │
│            │                                                │               │
│ ┌────────┐ │    ┌─────────┐         ┌─────────┐            │  节点配置     │
│ │ VPC    │ │    │   VPC   │────────▶│   EC2   │            │  表单         │
│ └────────┘ │    └─────────┘         └─────────┘            │               │
│ ┌────────┐ │                                                │               │
│ │ EC2    │ │                                                │               │
│ └────────┘ │                                                │               │
│            │                                                │               │
│ [搜索...]  │    [缩放: 100%] [适应画布] [自动布局]          │               │
└────────────┴────────────────────────────────────────────────┴───────────────┘
```

### 4.4 部署对话框

```
┌─────────────────────────────────────────────────────────────────┐
│  部署 Manifest                                                  │
│  ─────────────────────────────────────────────────────────────  │
│                                                                 │
│  Manifest: vpc-ec2-stack                                        │
│  版本: [v1.2.0 ▼]                                               │
│                                                                 │
│  目标 Workspace: [选择 Workspace ▼]                             │
│                                                                 │
│  ─────────────────────────────────────────────────────────────  │
│                                                                 │
│  变量覆盖（可选）:                                               │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  environment    [prod        ]  (默认: dev)             │   │
│  │  instance_type  [t3.large    ]  (默认: t3.micro)        │   │
│  │  vpc_cidr       [10.0.0.0/16 ]  (默认: 10.0.0.0/16)     │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  执行选项:                                                       │
│  ☑ 自动 Apply（跳过手动确认）                                    │
│  ☐ 仅 Plan（不执行 Apply）                                       │
│                                                                 │
│  [取消]                              [确认部署]                  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 4.5 部署状态页

```
┌─────────────────────────────────────────────────────────────────┐
│  Manifest: vpc-ec2-stack                                        │
│  [画布] [变量] [部署状态]                                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  部署列表 (3)                                                    │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Workspace      版本      状态      部署时间      操作   │   │
│  ├─────────────────────────────────────────────────────────┤   │
│  │  dev            v1.2.0    [已部署]  2025-01-01   [更新] [删除] │
│  │  staging        v1.1.0    [已部署]  2024-12-31   [更新] [删除] │
│  │  prod           v1.0.0    [已部署]  2024-12-15   [更新] [删除] │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  [+ 部署到新 Workspace]                                          │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 4.6 节点状态视觉效果

| 状态 | 图标 | 边框样式 | 边框颜色 | 说明 |
|------|------|---------|---------|------|
| 已创建 | ✓ | 实线 | 绿色 | 资源已创建 |
| 待创建 | + | 虚线 | 黄色 | 新节点，尚未部署 |
| 待更新 | ~ | 实线 | 橙色 | 配置已修改 |
| 待删除 | - | 虚线 | 红色 | 节点已删除 |
| 未关联 | | 灰色虚线 | 灰色 | Module 未在平台注册 |

---

## 五、功能流程

### 5.1 创建 Manifest

```
1. 进入 Organization → Manifests → 点击"创建 Manifest"
2. 填写基本信息（名称、描述）
3. 进入可视化编辑器
4. 从左侧 Module 库拖拽节点到画布
5. 配置节点参数、添加端口并连接
6. 定义可配置变量（部署时可覆盖）
7. 保存草稿
```

### 5.2 发布版本

```
1. 编辑完成后，点击"发布版本"
2. 输入版本号（如 v1.0.0）
3. 系统验证配置完整性
4. 创建新版本记录
5. 版本发布后不可修改，只能创建新版本
```

### 5.3 部署到 Workspace

```
1. 点击"部署到 Workspace"
2. 选择目标 Workspace
3. 选择要部署的版本
4. 覆盖变量值（可选）
5. 确认部署
6. 系统在 Workspace 中创建资源
7. 执行 Terraform plan/apply
8. 记录部署关系
```

### 5.4 更新部署

```
1. 在部署状态页，点击"更新"
2. 选择新版本
3. 修改变量覆盖（可选）
4. 确认更新
5. 系统更新 Workspace 中的资源
6. 执行 Terraform plan/apply
```

### 5.5 删除部署

```
1. 在部署状态页，点击"删除"
2. 确认删除（需输入 Workspace 名称）
3. 系统执行 terraform destroy
4. 删除 Workspace 中的资源
5. 删除部署记录
```

---

## 六、草稿功能

### 6.1 草稿状态

| 状态 | 说明 |
|------|------|
| draft | 草稿版本，可以修改 |
| published | 已发布版本，不可修改 |

### 6.2 草稿保存规则

以下情况都可以保存草稿：
- 必填参数未填写
- 参数类型错误（会标记错误）
- 循环依赖（会标记警告）
- 节点未连接

### 6.3 本地自动保存

每 30 秒自动保存到 localStorage，防止意外丢失：
```typescript
const LOCAL_STORAGE_KEY = `manifest_draft_${organizationId}_${manifestId}`;
```

### 6.4 Token 失效处理

1. **不强制退出页面**: 编辑器状态保存在前端内存/localStorage
2. **重新登录恢复**: 用户可在新 Tab 页重新登录
3. **保存时检测**: 保存操作时检测 Token 状态

---

## 七、HCL 生成规则

### 7.1 基本结构

```hcl
# Generated by IaC Platform Manifest Builder
# Manifest: vpc-ec2-stack
# Version: v1.2.0

variable "environment" {
  type        = string
  default     = "dev"
}

variable "instance_type" {
  type        = string
  default     = "t3.micro"
}

module "vpc_main" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.0.0"
  name    = "my-vpc-${var.environment}"
  cidr    = "10.0.0.0/16"
}

module "ec2_web" {
  source        = "terraform-aws-modules/ec2-instance/aws"
  version       = "5.0.0"
  vpc_id        = module.vpc_main.vpc_id
  instance_type = var.instance_type
  depends_on    = [module.vpc_main]
}
```

---

## 八、API 设计

### 8.1 认证与权限（MANIFESTS IAM，已实现）

所有接口都需要认证（JWT / API Token），授权全部走现有 role-policy IAM（`RequirePermission` / `RequireAnyPermission` + PermissionChecker），不再使用 `SYSTEM_SETTINGS` / `ORGANIZATION_MANAGEMENT` 作为替代。

`MANIFESTS` 是 **ORGANIZATION 级**资源类型（`valueobject.ResourceTypeManifests`），由迁移 `20261004_01_manifest_iam_resource`（`backend/internal/migration/manifests_iam.go`，同内容 SQL：`backend/migrations/add_manifest_iam_permissions.sql`）注册为 `orgpm-manifests`。内置角色授权（ORGANIZATION 作用域，与 MODULES 一致）：

| 内置角色 | MANIFESTS |
|---------|-----------|
| admin / org_admin | ADMIN |
| developer / viewer / user | READ |

> 同一迁移顺带注册了此前缺失的 `orgpm-variable-sets`（VARIABLE_SETS：admin/org_admin ADMIN，viewer READ，与 RUN_TASKS 一致），否则所有 `/variable-sets` 路由对非超管均 403。

| 操作 | 权限要求 |
|------|---------|
| 目录/详情/文件/版本/diff/导出/provider-schemas 读取 | `MANIFESTS` READ |
| 创建、更新、文件写入/移动/删除、draft reset、发布版本 | `MANIFESTS` WRITE |
| 删除 Manifest、归档/取消归档（`PUT /:id` 修改 `status` 进出 `archived`） | `MANIFESTS` ADMIN |
| 部署列表/详情/预览/安装/升级/卸载（路由层） | `MANIFESTS` READ |
| 安装 / 升级 / 卸载（handler 内对**目标 workspace**） | `WORKSPACE_RESOURCES` WRITE（workspace 作用域，或 org 级 `WORKSPACE_MANAGEMENT` 等价上级） |
| 部署详情 | 目标 workspace 可读 |
| 变量预览 | 目标 workspace `WORKSPACE_VARIABLES` READ |
| `/manifest-editor/*`（模块、demo、inputs） | `MODULES` READ |
| `GET /variable-sets/:varset_id/manifest-deployments` | `VARIABLE_SETS` READ |

`MANIFESTS` READ **不**隐含任何部署能力；部署能力只来自目标 workspace 上的 `WORKSPACE_RESOURCES` WRITE。部署相关接口都会校验 manifest 属于当前认证 org（否则 404）。

#### 前端约定字段

| 字段 | 位置 | 含义 |
|------|------|------|
| `can_write` | `GET /organizations/:org_id/manifests` 列表项与 `GET .../manifests/:id` | 调用者 MANIFESTS 有效级别 ≥ WRITE（显示编辑/发布入口） |
| `can_deploy` | 同上 | 调用者至少在一个可读 workspace 上拥有 `WORKSPACE_RESOURCES` WRITE（与 `capability` 过滤同一判定；显示"部署"入口） |
| `can_admin` | 同上 | 调用者 MANIFESTS 有效级别 = ADMIN（显示删除/归档入口；删除与归档仍由服务端要求 ADMIN） |
| `capabilities` | `GET /organizations/:org_id/manifests` 响应顶层 `{can_read, can_write, can_admin, can_deploy}` | 与列表项同一判定；**列表为空时也返回**（决定"新建"/"部署"入口） |
| `capability` | `GET /api/v1/workspaces?capability=WORKSPACE_RESOURCES:WRITE` | 部署目标选择器：仅返回调用者可读**且**具备该能力的 workspace。格式 `RESOURCE_TYPE:LEVEL`，资源必须是 workspace 级，LEVEL ∈ READ/WRITE/ADMIN；未知值返回 **400** |
| `sensitive` | `POST .../v2/deployments/:deployment_id/variable-preview` 响应 `{"variables":[{key,value,sensitive,source_type,...}]}` | 敏感变量 `value` 恒为空串（含被 override 覆盖的敏感 key），前端显示为掩码 |

补充：
- `GET /api/v1/variable-sets?workspace_id=<ws>`：只返回该 workspace 可挂载的变量集（global + 分配给该 workspace 或其 project 的）；调用者需能读该 workspace（且属于当前 org，否则 404/403）。安装/升级/预览时提交不可挂载的 varset 返回 400。
- 部署列表按调用者可读 workspace 在服务端过滤；调用者**没有任何可读 workspace** 时返回 **403**（复用 `RequireWorkspaceListAccess` 的语义），而非空列表。
- upgrade 的 `variable_overrides` 与已存覆盖**合并**：请求中缺省的 key 保留原值；敏感变量传空串（预览掩码占位）视为不修改，也不会新增空覆盖；只有 `unset_keys: []string` 中列出的 key 被删除。形状不变（扁平 key→string）。
- 没有批量文件 API：文件操作只有 `PUT/DELETE /files/*path` 与 `_move` / `_move_dir` / `_delete_dir`。

### 8.2 Manifest CRUD（Organization 级别）

前缀 `/api/v1/organizations/:org_id/manifests`

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `` | 列表（含 `can_write`/`can_deploy`） | MANIFESTS READ |
| POST | `` | 创建 | MANIFESTS WRITE |
| GET | `/:id` | 详情（含 `can_write`/`can_deploy`） | MANIFESTS READ |
| PUT | `/:id` | 更新；`status` 进出 `archived` 需 ADMIN | MANIFESTS WRITE（归档 ADMIN） |
| DELETE | `/:id` | 删除 | MANIFESTS ADMIN |
| GET | `/:id/export-zip` | 导出 zip | MANIFESTS READ |

### 8.3 文件与版本（v2）

前缀 `/api/v1/organizations/:org_id/manifests/:id`

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/provider-schemas` | provider schema | MANIFESTS READ |
| GET | `/files`、`/files/*path` | 文件树 / 文件内容 | MANIFESTS READ |
| PUT / DELETE | `/files/*path` | 写入 / 删除文件 | MANIFESTS WRITE |
| POST | `/files/_move`、`/files/_move_dir`、`/files/_delete_dir` | 移动 / 目录操作 | MANIFESTS WRITE |
| POST | `/draft/_reset_from` | 从版本重置 draft | MANIFESTS WRITE |
| POST | `/draft/_export` | 导出 draft | MANIFESTS READ |
| GET | `/v2/versions`、`/v2/versions/:version_id` | 版本列表 / 详情 | MANIFESTS READ |
| POST | `/v2/versions` | 发布版本 | MANIFESTS WRITE |
| GET | `/v2/versions/:version_id/diff`、`/v2/versions/:version_id/workdirs`、`/v2/draft/diff` | diff / workdirs | MANIFESTS READ |
| POST | `/v2/versions/:version_id/files/_export` | 导出版本文件 | MANIFESTS READ |

### 8.4 部署管理（v2）

前缀 `/api/v1/organizations/:org_id/manifests/:id`

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/v2/deployments` | 部署列表（按可读 workspace 过滤；无可读 workspace → 403） | MANIFESTS READ + workspace 列表访问 |
| GET | `/v2/deployments/:deployment_id` | 部署详情 | MANIFESTS READ + 目标 workspace 可读 |
| POST | `/v2/deployments/install` | 安装到 body 中的 workspace | MANIFESTS READ + 目标 `WORKSPACE_RESOURCES` WRITE |
| POST | `/v2/deployments/:deployment_id/upgrade` | 升级 | MANIFESTS READ + 目标 `WORKSPACE_RESOURCES` WRITE |
| POST | `/v2/deployments/:deployment_id/uninstall` | 卸载 | MANIFESTS READ + 目标 `WORKSPACE_RESOURCES` WRITE |
| POST | `/v2/deployments/:deployment_id/variable-preview` | 变量预览（`sensitive` 掩码） | MANIFESTS READ + 目标 `WORKSPACE_VARIABLES` READ |

### 8.5 其它视角

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/v1/workspaces/:id/manifest-summary` | workspace 上的 manifest 摘要 | WORKSPACES READ(org) 或 WORKSPACE_MANAGEMENT READ(ws)（未改动） |
| GET | `/api/v1/variable-sets/:varset_id/manifest-deployments` | 变量集被哪些部署引用 | VARIABLE_SETS READ |
| GET | `/api/v1/manifest-editor/modules`、`/modules/:module_id/demos`、`/modules/:module_id/inputs` | 编辑器模块数据 | MODULES READ |

### 8.6 Import/Export

旧的 HCL/JSON import 接口已不在路由中；导出通过 `GET /:id/export-zip`、`POST /draft/_export`、`POST /v2/versions/:version_id/files/_export`（均 MANIFESTS READ）。

### 8.7 部署时复用现有接口

部署 Manifest 时，复用现有的资源接口：
1. **添加资源**: `POST /api/v1/workspaces/:workspace_id/resources`
2. **部署资源**: `POST /api/v1/workspaces/:workspace_id/resources/deploy`

---

## 九、实现计划

### Phase 1: 基础框架（2周）
- [ ] 数据库表创建
- [ ] 后端 API 基础 CRUD
- [ ] 前端 React Flow 集成
- [ ] 基础画布功能
- [ ] Organization 级别入口

### Phase 2: 节点与连接（2周）
- [ ] 自定义节点组件
- [ ] 端口（触手）功能
- [ ] 连线功能
- [ ] 属性面板
- [ ] 可配置变量定义

### Phase 3: 版本与部署（1.5周）
- [ ] 版本管理
- [ ] 部署到 Workspace
- [ ] 变量覆盖
- [ ] 部署状态管理

### Phase 4: 导入导出（1周）
- [ ] HCL 生成逻辑
- [ ] HCL 导出功能
- [ ] HCL 导入功能（可选）

### Phase 5: 优化与测试（0.5周）
- [ ] 性能优化
- [ ] 错误处理
- [ ] 测试

**总计: 约 7 周**

---

## 十、MVP 范围

### MVP 必须有
- [ ] 基础画布（拖拽、缩放、平移）
- [ ] 节点创建和删除
- [ ] 依赖关系连线
- [ ] 端口（触手）功能
- [ ] 变量绑定连线
- [ ] 属性面板
- [ ] 保存草稿
- [ ] 部署到 Workspace
- [ ] HCL 导出

### MVP 可选
- [ ] 版本管理
- [ ] HCL 导入
- [ ] 变量覆盖
- [ ] 部署状态管理

### 后续迭代
- [ ] 全局依赖图
- [ ] 快捷键
- [ ] 模板市场
- [ ] 撤销/重做

---

## 十一、风险与注意事项

1. **性能**: 大量节点时的渲染性能
2. **HCL 解析**: 复杂 HCL 语法的解析
3. **版本兼容**: Module 版本升级时的兼容性
4. **循环依赖**: 需要检测并阻止
5. **Token 失效**: 需要优雅处理
6. **并发编辑**: 多人同时编辑的冲突处理
7. **部署冲突**: 同一 Workspace 只能有一个 Manifest 部署
8. **权限管理**: Organization 级别和 Workspace 级别的权限协调
