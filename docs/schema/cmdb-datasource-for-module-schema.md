# Module Schema CMDB 数据源设计方案

## 实现状态

| 功能 | 状态 | 说明 |
|------|------|------|
| Schema 编辑器 CMDB 配置 | ✅ 已完成 | `OpenAPISchemaEditor/index.tsx` |
| FieldUIConfig 类型定义 | ✅ 已完成 | `services/schemaV2.ts` |
| CMDBSelectWidget 组件 | ✅ 已完成 | `OpenAPIFormRenderer/widgets/CMDBSelectWidget.tsx` |
| Widget 类型注册 | ✅ 已完成 | `OpenAPIFormRenderer/types.ts`, `widgets/index.ts` |
| getWidgetType 识别 | ✅ 已完成 | `services/schemaV2.ts` |
| 后端 CMDB 搜索 API | ✅ 已有 | `GET /api/v1/cmdb/search` |

### 已实现的文件

1. **Schema 编辑器** (`frontend/src/components/OpenAPISchemaEditor/index.tsx`)
   - 数据源类型选择：无 / CMDB 资源 / 自定义数据源
   - CMDB 配置面板：资源类型输入、值字段选择
   - 资源类型推荐字段映射

2. **类型定义** (`frontend/src/services/schemaV2.ts`)
   - `FieldUIConfig.cmdbSource` 接口定义
   - `getWidgetType()` 函数识别 cmdbSource 配置

3. **CMDBSelectWidget** (`frontend/src/components/OpenAPIFormRenderer/widgets/CMDBSelectWidget.tsx`)
   - 搜索模式：从 CMDB 搜索资源
   - 手动输入模式：直接输入任意值
   - 根据 valueField 提取对应字段值

4. **Widget 注册** (`frontend/src/components/OpenAPIFormRenderer/widgets/index.ts`)
   - 注册 `cmdb-select` widget 类型

## 1. 概述

### 1.1 目标

为 Module Schema 的字段配置 CMDB 数据源功能，让用户在填写表单时可以从 CMDB 中搜索并选择已有的云资源。

### 1.2 核心原则

1. **CMDB 是辅助功能，不是限制功能** - 用户可以从 CMDB 搜索选择，也可以手动输入任意值
2. **valueField 是预定义的枚举** - CMDB 字段是固定的 Key，不允许任意输入
3. **友好的显示名称** - 配置界面使用中文友好名称，而非工程化的 Key 名称
4. **搜索与填充分离** - 用户可以通过任意字段搜索，但填充的值由 valueField 决定

## 2. CMDB 字段定义

### 2.1 预定义字段枚举

CMDB 提供以下固定字段，每个字段有唯一的 Key 和友好的显示名称：

| Key | 显示名称 | 说明 | 典型使用场景 |
|-----|---------|------|-------------|
| `cloud_id` | 资源 ID | 云资源唯一标识符 | VPC ID、子网 ID、安全组 ID |
| `cloud_arn` | ARN | AWS ARN / Azure Resource ID | IAM 角色、策略、跨服务引用 |
| `cloud_name` | 资源名称 | 云资源的名称 | 命名引用、标签配置 |
| `cloud_region` | 区域 | 云资源所在区域 | 多区域部署配置 |
| `cloud_account` | 账户 ID | 云账户标识符 | 跨账户配置 |
| `terraform_address` | Terraform 地址 | 完整的 Terraform 资源地址 | 状态文件定位、调试 |
| `description` | 描述 | 资源描述信息 | 文档、注释 |

### 2.2 字段与数据库映射

| Key | 数据库表 | 数据库字段 |
|-----|---------|-----------|
| `cloud_id` | `resource_index` | `cloud_resource_id` |
| `cloud_arn` | `resource_index` | `cloud_resource_arn` |
| `cloud_name` | `resource_index` | `cloud_resource_name` |
| `cloud_region` | `resource_index` | `cloud_region` |
| `cloud_account` | `resource_index` | `cloud_account_id` |
| `terraform_address` | `resource_index` | `terraform_address` |
| `description` | `resource_index` | `description` |

### 2.3 资源类型与推荐字段

不同的 Terraform 资源类型有不同的推荐 valueField：

| 资源类型 | 推荐 valueField | 说明 |
|---------|----------------|------|
| `aws_security_group` | `cloud_id` | 安全组使用 sg-xxx 格式的 ID |
| `aws_iam_role` | `cloud_arn` | IAM 角色使用 ARN |
| `aws_iam_policy` | `cloud_arn` | IAM 策略使用 ARN |
| `aws_iam_instance_profile` | `cloud_arn` | 实例配置文件使用 ARN |
| `aws_subnet` | `cloud_id` | 子网使用 subnet-xxx 格式的 ID |
| `aws_vpc` | `cloud_id` | VPC 使用 vpc-xxx 格式的 ID |
| `aws_s3_bucket` | `cloud_id` | S3 桶使用桶名作为 ID |
| `aws_kms_key` | `cloud_arn` | KMS 密钥使用 ARN |
| `aws_lb` | `cloud_arn` | 负载均衡器使用 ARN |
| `aws_lb_target_group` | `cloud_arn` | 目标组使用 ARN |
| `aws_ami` | `cloud_id` | AMI 使用 ami-xxx 格式的 ID |
| `aws_key_pair` | `cloud_name` | 密钥对使用名称 |
| `aws_acm_certificate` | `cloud_arn` | ACM 证书使用 ARN |
| `aws_eks_cluster` | `cloud_name` | EKS 集群使用名称 |
| `aws_rds_cluster` | `cloud_id` | RDS 集群使用集群标识符 |
| `aws_db_instance` | `cloud_id` | RDS 实例使用实例标识符 |

## 3. Schema 配置规范

### 3.1 CMDB 数据源配置结构

在 Module Schema 的 `x-iac-platform.ui.fields` 中配置 CMDB 数据源：

```
字段配置结构:
├── cmdbSource
│   ├── enabled: boolean          # 是否启用 CMDB 数据源
│   ├── resourceType: string      # 资源类型（如 aws_security_group）
│   └── valueField: enum          # 值字段（预定义枚举）
├── searchable: boolean           # 是否支持搜索（默认 true）
└── allowCustom: boolean          # 是否允许自定义输入（默认 true，不可设为 false）
```

### 3.2 配置示例

**示例 1：安全组选择（使用资源 ID）**

```
字段: vpc_security_group_ids
配置:
  - cmdbSource.enabled: true
  - cmdbSource.resourceType: aws_security_group
  - cmdbSource.valueField: cloud_id (资源 ID)
  - allowCustom: true

效果:
  - 用户搜索 "web" → 命中名称包含 "web" 的安全组
  - 用户选择 "web-server-sg" → 填充值为 "sg-0123456789abcdef0"
  - 用户也可以直接输入 "sg-custom-value"
```

**示例 2：IAM 角色选择（使用 ARN）**

```
字段: iam_instance_profile
配置:
  - cmdbSource.enabled: true
  - cmdbSource.resourceType: aws_iam_instance_profile
  - cmdbSource.valueField: cloud_arn (ARN)
  - allowCustom: true

效果:
  - 用户搜索 "ec2" → 命中名称或描述包含 "ec2" 的实例配置文件
  - 用户选择 "ec2-profile" → 填充值为 "arn:aws:iam::123456789012:instance-profile/ec2-profile"
  - 用户也可以直接输入自定义 ARN
```

**示例 3：RDS 实例选择（使用资源 ID）**

```
字段: db_instance_identifier
配置:
  - cmdbSource.enabled: true
  - cmdbSource.resourceType: aws_db_instance
  - cmdbSource.valueField: cloud_id (资源 ID)
  - allowCustom: true

效果:
  - 用户搜索 "mysql" → 命中标签、名称或描述包含 "mysql" 的 RDS 实例
  - 用户选择 "production-mysql" → 填充值为 "db-prod-mysql-001"
  - 用户也可以直接输入自定义实例标识符
```

## 4. 用户交互设计

### 4.1 Schema 编辑器界面

在 Schema 编辑器中配置 CMDB 数据源时的界面设计：

```
┌─────────────────────────────────────────────────────────────┐
│ 数据源配置                                                   │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ 数据源类型                                                   │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ ○ 无    ○ 静态选项    ○ API 接口    ● CMDB 资源        │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ ┌─ CMDB 配置 ─────────────────────────────────────────────┐ │
│ │                                                         │ │
│ │ 资源类型                                                 │ │
│ │ ┌─────────────────────────────────────────────────────┐ │ │
│ │ │ ▼ aws_db_instance                                   │ │ │
│ │ └─────────────────────────────────────────────────────┘ │ │
│ │                                                         │ │
│ │ 值字段（选择资源后填充的值）                              │ │
│ │ ┌─────────────────────────────────────────────────────┐ │ │
│ │ │ ▼ ARN                                               │ │ │
│ │ ├─────────────────────────────────────────────────────┤ │ │
│ │ │   资源 ID    - 云资源唯一标识符 (sg-xxx, vpc-xxx)   │ │ │
│ │ │ ● ARN        - AWS ARN / Azure Resource ID          │ │ │
│ │ │   资源名称   - 云资源的名称                          │ │ │
│ │ │   区域       - 云资源所在区域                        │ │ │
│ │ │   账户 ID    - 云账户标识符                          │ │ │
│ │ │   Terraform 地址 - 完整的 TF 资源地址               │ │ │
│ │ │   描述       - 资源描述信息                          │ │ │
│ │ └─────────────────────────────────────────────────────┘ │ │
│ │                                                         │ │
│ │ 💡 提示：                                               │ │
│ │ • CMDB 数据源是辅助功能，用户仍可手动输入任意值          │ │
│ │ • 值字段决定了用户选择资源后，实际填充的是哪个字段的值    │ │
│ │ • 搜索时会匹配资源的 ID、名称、描述和标签                │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 表单渲染界面

用户在填写表单时的界面设计：

```
┌─────────────────────────────────────────────────────────────┐
│ IAM 实例配置文件                                             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ 🔍 搜索 CMDB 资源...                              [✏️]  │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ ┌─ 搜索结果 ───────────────────────────────────────────────┐│
│ │                                                          ││
│ │ ┌────────────────────────────────────────────────────┐  ││
│ │ │ 📦 ec2-web-profile                                 │  ││
│ │ │    arn:aws:iam::123456789012:instance-profile/...  │  ││
│ │ │    @ workspace-prod                                │  ││
│ │ └────────────────────────────────────────────────────┘  ││
│ │                                                          ││
│ │ ┌────────────────────────────────────────────────────┐  ││
│ │ │ 📦 ec2-api-profile                                 │  ││
│ │ │    arn:aws:iam::123456789012:instance-profile/...  │  ││
│ │ │    @ workspace-prod                                │  ││
│ │ └────────────────────────────────────────────────────┘  ││
│ │                                                          ││
│ │ ─────────────────────────────────────────────────────── ││
│ │ 💡 找不到？点击 [✏️] 切换到手动输入模式                  ││
│ └──────────────────────────────────────────────────────────┘│
│                                                             │
│ 选择资源后，将填充其 ARN 值                                  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 4.3 手动输入模式

```
┌─────────────────────────────────────────────────────────────┐
│ IAM 实例配置文件                                             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ arn:aws:iam::123456789012:instance-profile/my-profile   │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ [🔍 从 CMDB 搜索]                                           │
│                                                             │
│ 💡 您可以直接输入值，或点击上方按钮从 CMDB 搜索选择          │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## 5. 搜索逻辑

### 5.1 搜索字段

当用户在 CMDB 数据源中搜索时，系统会在以下字段中进行匹配：

| 搜索字段 | 说明 |
|---------|------|
| `cloud_resource_id` | 云资源 ID |
| `cloud_resource_name` | 云资源名称 |
| `cloud_resource_arn` | ARN |
| `description` | 资源描述 |
| `tags` | 资源标签（JSON 全文搜索） |

### 5.2 搜索与填充分离

**关键概念**：搜索命中的字段与最终填充的字段是分离的。

```
示例场景：
- 配置: valueField = cloud_arn
- 用户搜索: "mysql"
- 搜索命中: tags.Database = "mysql"
- 填充值: arn:aws:rds:us-east-1:123456789012:db:production-mysql

无论用户通过什么字段搜索命中资源，最终填充的值都是 valueField 指定的字段。
```

### 5.3 搜索结果显示

搜索结果应显示足够的信息帮助用户识别资源：

```
搜索结果项显示内容：
├── 主标题: cloud_resource_name（资源名称）
├── 副标题: valueField 对应的值（用户选择后将填充的值）
├── 来源: workspace_name 或 external_source_name
└── 标签: 资源类型标签
```

## 6. API 设计

### 6.1 获取 CMDB 资源选项

**端点**: `GET /api/v1/cmdb/options`

**请求参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `resource_type` | string | 是 | 资源类型 |
| `value_field` | string | 是 | 值字段（枚举值） |
| `query` | string | 否 | 搜索关键词 |
| `workspace_id` | string | 否 | 限制特定 workspace |
| `limit` | int | 否 | 返回数量限制，默认 50 |

**响应结构**:

```
响应:
├── options: 选项列表
│   ├── value: 选项值（根据 value_field 提取）
│   ├── label: 显示标签（资源名称）
│   ├── description: 资源描述
│   ├── workspace_name: 所属 workspace 名称
│   └── extra: 额外信息
│       ├── cloud_id: 资源 ID
│       ├── cloud_arn: ARN
│       └── cloud_name: 资源名称
├── total: 总数
└── has_more: 是否还有更多
```

### 6.2 获取 CMDB 字段定义

**端点**: `GET /api/v1/cmdb/fields`

**响应结构**:

```
响应:
└── fields: 字段列表
    ├── key: 字段 Key（如 cloud_id）
    ├── label: 显示名称（如 "资源 ID"）
    ├── description: 字段说明
    └── examples: 示例值列表
```

## 7. 后端实现要点

### 7.1 服务层函数

| 函数名 | 说明 |
|--------|------|
| `GetCMDBFieldDefinitions()` | 获取 CMDB 字段定义列表 |
| `GetCMDBResourceOptions()` | 根据配置获取资源选项列表 |
| `ExtractValueByField()` | 根据 valueField 从资源中提取值 |
| `SearchCMDBResources()` | 搜索 CMDB 资源（多字段匹配） |

### 7.2 字段提取逻辑

`ExtractValueByField()` 函数根据 valueField 枚举值从资源记录中提取对应字段的值：

```
输入: resource_index 记录, valueField 枚举值
输出: 对应字段的值

映射关系:
- cloud_id → resource.CloudResourceID
- cloud_arn → resource.CloudResourceARN
- cloud_name → resource.CloudResourceName
- cloud_region → resource.CloudRegion
- cloud_account → resource.CloudAccountID
- terraform_address → resource.TerraformAddress
- description → resource.Description
```

## 8. 前端实现要点

### 8.1 组件结构

| 组件名 | 说明 |
|--------|------|
| `CMDBFieldSelector` | Schema 编辑器中的 valueField 下拉选择器 |
| `CMDBSourceConfig` | Schema 编辑器中的 CMDB 数据源配置面板 |
| `CMDBSelectWidget` | 表单渲染器中的 CMDB 资源选择组件 |

### 8.2 CMDBFieldSelector 组件

显示友好名称的下拉选择器：

```
选项列表:
├── 资源 ID - 云资源唯一标识符 (sg-xxx, vpc-xxx)
├── ARN - AWS ARN / Azure Resource ID
├── 资源名称 - 云资源的名称
├── 区域 - 云资源所在区域
├── 账户 ID - 云账户标识符
├── Terraform 地址 - 完整的 TF 资源地址
└── 描述 - 资源描述信息
```

### 8.3 CMDBSelectWidget 组件

表单渲染时的资源选择组件，支持两种模式：

1. **搜索模式**（默认）
   - 显示搜索框
   - 实时搜索 CMDB 资源
   - 选择后填充 valueField 对应的值

2. **手动输入模式**
   - 显示普通文本输入框
   - 用户可输入任意值
   - 提供切换回搜索模式的按钮

## 9. 数据流程

### 9.1 Schema 配置流程

```
1. 用户在 Schema 编辑器中选择字段
2. 用户选择数据源类型为 "CMDB 资源"
3. 用户选择资源类型（如 aws_db_instance）
4. 用户选择值字段（如 "ARN"）
5. 系统保存配置到 Schema 的 x-iac-platform.ui.fields 中
```

### 9.2 表单渲染流程

```
1. FormRenderer 解析 Schema，发现字段配置了 cmdbSource
2. 渲染 CMDBSelectWidget 组件
3. 用户输入搜索关键词
4. 调用 GetCMDBResourceOptions() API
5. 显示搜索结果列表
6. 用户选择资源
7. 调用 ExtractValueByField() 提取值
8. 填充到表单字段
```

### 9.3 表单提交流程

```
1. 用户完成表单填写
2. 表单验证（不验证值是否来自 CMDB）
3. 提交表单数据
4. 后端接收并处理（不区分值来源）
```

## 10. 注意事项

### 10.1 不强制 CMDB 来源

- CMDB 数据源是**辅助功能**，不是**限制功能**
- 用户始终可以手动输入任意值
- 后端不验证值是否来自 CMDB
- `allowCustom` 配置项默认为 `true`，且不建议设为 `false`

### 10.2 友好的显示名称

- Schema 编辑器中使用中文友好名称（如 "资源 ID"、"ARN"）
- 内部存储使用英文 Key（如 `cloud_id`、`cloud_arn`）
- 前端通过 `GetCMDBFieldDefinitions()` API 获取字段定义

### 10.3 搜索性能

- 搜索结果默认限制 50 条
- 支持分页加载更多
- 建议对常用资源类型建立索引

### 10.4 外部 CMDB 数据源

- 支持从外部 CMDB 数据源（`source_type=external`）搜索
- 外部数据源的资源与内部资源统一搜索
- 搜索结果中标识数据来源

## 11. 后续扩展

### 11.1 智能推荐

根据资源类型自动推荐 valueField：
- 当用户选择 `aws_iam_role` 时，自动推荐 `cloud_arn`
- 当用户选择 `aws_security_group` 时，自动推荐 `cloud_id`

### 11.2 字段验证提示

当用户手动输入时，可以提供格式验证提示：
- ARN 格式验证：`arn:aws:service:region:account:resource`
- 资源 ID 格式验证：`sg-xxx`、`subnet-xxx` 等

### 11.3 跨 Workspace 搜索

支持配置是否允许跨 Workspace 搜索资源：
- 默认只搜索当前 Workspace 的资源
- 可配置允许搜索所有 Workspace 的资源
