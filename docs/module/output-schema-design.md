# Module Output Schema 设计方案

## 1. 概述

本文档描述了在 OpenAPI v3 Schema 中添加 Output 定义的设计方案。Output 定义的核心价值是为用户配置 Workspace Outputs 时提供**智能提示**。

## 2. Variables vs Outputs

| 类型 | 用途 | 用户交互 | Schema 用途 |
|------|------|----------|-------------|
| **Variables** | 模块的**输入参数** | 用户通过表单填写 | 生成输入表单 |
| **Outputs** | 模块的**输出值** | 用户**不填写** | 提供智能提示 |

## 3. Output 识别的核心价值

### 3.1 Workspace Outputs 配置智能提示

当用户在 Workspace Settings 中配置 Outputs 时，系统根据已添加的资源及其模块的 Output 定义，提供智能提示：

```
+-------------------------------------------------------------+
|  配置 Workspace Output                                      |
+-------------------------------------------------------------+
|                                                             |
|  Output 名称                                                |
|  +-----------------------------------------------------+   |
|  | s3_bucket_arn                                       |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  Output 值                                                  |
|  +-----------------------------------------------------+   |
|  | module.my_s3_bucket.                                |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  [i] 可用的模块输出：                                       |
|  +-----------------------------------------------------+   |
|  | [+] my_s3_bucket (S3 Bucket)                        |   |
|  |     |-- bucket_id        存储桶ID                   |   |
|  |     |-- bucket_arn       存储桶ARN <- 点击插入      |   |
|  |     |-- bucket_domain_name  域名                    |   |
|  |     +-- bucket_regional_domain_name  区域域名       |   |
|  |                                                     |   |
|  | [+] my_kms_key (KMS Key)                            |   |
|  |     |-- key_id           密钥ID                     |   |
|  |     +-- key_arn          密钥ARN                    |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  描述                                                       |
|  +-----------------------------------------------------+   |
|  | S3 存储桶的 ARN                                     |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  [ ] 敏感数据                                               |
|                                                             |
|  [取消]                                    [保存]           |
+-------------------------------------------------------------+
```

### 3.2 资源引用智能提示

当用户在配置资源参数时输入 `${module.` 或 `module.`，提供可用输出的智能提示：

```
+-------------------------------------------------------------+
|  IAM Policy Resource                                        |
|  +-----------------------------------------------------+   |
|  | ${module.my_s3_bucket._                             |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  +-----------------------------------------------------+   |
|  | bucket_id        存储桶ID (string)                  |   |
|  | bucket_arn       存储桶ARN (string) <- 推荐         |   |
|  | bucket_domain_name  域名 (string)                   |   |
|  +-----------------------------------------------------+   |
+-------------------------------------------------------------+
```

### 3.3 模块文档展示

在模块详情页展示该模块的输出列表，帮助用户了解模块能提供哪些输出值。使用 Tab 区分 Variables 和 Outputs：

```
+-------------------------------------------------------------+
|  S3 Bucket Module                                           |
+-------------------------------------------------------------+
|  [ Variables (12) ]  [ Outputs (8) ]                        |
+-------------------------------------------------------------+
|                                                             |
|  输出名称              类型      描述                       |
|  -----------------------------------------------------------+
|  bucket_id            string    存储桶名称                  |
|  bucket_arn           string    存储桶 ARN                  |
|  bucket_domain_name   string    存储桶域名                  |
|  bucket_regional_domain_name  string  区域域名              |
|  bucket_hosted_zone_id  string  托管区域 ID                 |
|  bucket_website_endpoint  string  网站端点                  |
|  bucket_website_domain  string  网站域名                    |
|  s3_bucket_policy     string    存储桶策略 (敏感)           |
|                                                             |
+-------------------------------------------------------------+
```

## 4. OpenAPI Schema 扩展设计

### 4.1 完整 Schema 结构

```json
{
  "openapi": "3.1.0",
  "info": {
    "title": "S3 Bucket Module",
    "version": "5.0.0",
    "description": "Terraform module for AWS S3 Bucket",
    "x-module-source": "terraform-aws-modules/s3-bucket/aws",
    "x-provider": "aws"
  },
  "components": {
    "schemas": {
      "ModuleInput": {
        "type": "object",
        "description": "模块输入参数（用户填写）",
        "properties": {
          "bucket": {
            "type": "string",
            "description": "The name of the bucket"
          },
          "acl": {
            "type": "string",
            "description": "The canned ACL to apply",
            "default": "private"
          }
        },
        "required": ["bucket"]
      },
      "ModuleOutput": {
        "type": "object",
        "description": "模块输出定义（只读，用于智能提示）",
        "properties": {
          "bucket_id": {
            "type": "string",
            "description": "The name of the bucket",
            "x-alias": "存储桶ID",
            "x-value-expression": "aws_s3_bucket.this[0].id"
          },
          "bucket_arn": {
            "type": "string",
            "description": "The ARN of the bucket",
            "x-alias": "存储桶ARN",
            "x-value-expression": "aws_s3_bucket.this[0].arn"
          },
          "bucket_domain_name": {
            "type": "string",
            "description": "The bucket domain name",
            "x-alias": "存储桶域名",
            "x-value-expression": "aws_s3_bucket.this[0].bucket_domain_name"
          }
        }
      }
    }
  },
  "x-iac-platform": {
    "ui": {
      "fields": {
        "bucket": {
          "widget": "text",
          "label": "存储桶名称",
          "group": "basic",
          "order": 1
        }
      },
      "groups": [
        {"id": "basic", "title": "基础配置", "order": 1},
        {"id": "advanced", "title": "高级配置", "order": 2}
      ]
    },
    "outputs": {
      "description": "模块输出列表（用于智能提示）",
      "items": [
        {
          "name": "bucket_id",
          "alias": "存储桶ID",
          "type": "string",
          "description": "The name of the bucket",
          "sensitive": false,
          "valueExpression": "aws_s3_bucket.this[0].id"
        },
        {
          "name": "bucket_arn",
          "alias": "存储桶ARN",
          "type": "string",
          "description": "The ARN of the bucket",
          "sensitive": false,
          "valueExpression": "aws_s3_bucket.this[0].arn"
        },
        {
          "name": "bucket_domain_name",
          "alias": "存储桶域名",
          "type": "string",
          "description": "The bucket domain name",
          "sensitive": false,
          "valueExpression": "aws_s3_bucket.this[0].bucket_domain_name"
        }
      ]
    }
  }
}
```

### 4.2 Output 属性说明

| 属性 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | Output 名称 |
| `alias` | string | 否 | 中文别名，用于 UI 显示 |
| `type` | string | 是 | 输出值类型：string, number, bool, list, map, object |
| `description` | string | 否 | 描述信息 |
| `sensitive` | boolean | 否 | 是否敏感数据，默认 false |
| `valueExpression` | string | 否 | 值表达式（来自 outputs.tf 的 value） |
| `dependsOn` | array | 否 | 依赖的资源列表 |

## 5. Terraform 注释规范

### 5.1 outputs.tf 注释格式

```hcl
output "bucket_id" {
  description = "The name of the bucket"  # @alias:存储桶ID
  value       = aws_s3_bucket.this[0].id
}

output "bucket_arn" {
  description = "The ARN of the bucket"  # @alias:存储桶ARN
  value       = aws_s3_bucket.this[0].arn
}

output "s3_bucket_policy" {
  description = "The policy of the bucket"  # @alias:存储桶策略 @sensitive:true
  value       = aws_s3_bucket_policy.this[0].policy
  sensitive   = true
}
```

### 5.2 支持的注释属性

| 属性 | 示例 | 说明 |
|------|------|------|
| `alias` | `@alias:存储桶ID` | 中文别名 |
| `sensitive` | `@sensitive:true` | 标记为敏感数据 |
| `deprecated` | `@deprecated:Use_bucket_id` | 弃用警告 |
| `group` | `@group:identifiers` | 分组（用于文档展示） |

## 6. 多文件导入方案

### 6.1 前端上传组件

```typescript
interface ImportSchemaRequest {
  files: FileContent[];
  moduleName?: string;
  moduleVersion?: string;
  moduleSource?: string;
  provider?: string;
}

interface FileContent {
  type: 'variables' | 'outputs';
  filename: string;
  content: string;
}
```

### 6.2 上传 UI 设计

```
+-------------------------------------------------------------+
|  导入 Terraform 文件                                        |
+-------------------------------------------------------------+
|                                                             |
|  +-----------------------------------------------------+   |
|  |                                                     |   |
|  |     [icon] 拖拽文件到此处，或点击选择文件           |   |
|  |                                                     |   |
|  |     支持: variables.tf, outputs.tf                  |   |
|  |                                                     |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  已选择的文件：                                             |
|  +-----------------------------------------------------+   |
|  | [x] variables.tf (12 个变量)              [移除]    |   |
|  | [x] outputs.tf (8 个输出)                 [移除]    |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  [取消]                                    [开始解析]       |
+-------------------------------------------------------------+
```

### 6.3 后端 API

```
POST /api/v1/modules/{moduleId}/schema/import

Request Body:
{
  "files": [
    {
      "type": "variables",
      "filename": "variables.tf",
      "content": "variable \"bucket\" { ... }"
    },
    {
      "type": "outputs",
      "filename": "outputs.tf",
      "content": "output \"bucket_id\" { ... }"
    }
  ],
  "moduleName": "S3 Bucket",
  "moduleVersion": "5.0.0",
  "moduleSource": "terraform-aws-modules/s3-bucket/aws",
  "provider": "aws"
}

Response:
{
  "success": true,
  "schema": { /* 生成的 OpenAPI Schema */ },
  "stats": {
    "variablesCount": 12,
    "outputsCount": 8,
    "basicVariables": 5,
    "advancedVariables": 7
  }
}
```

## 7. 智能提示 API

### 7.1 获取 Workspace 可用的 Output 提示

```
GET /api/v1/workspaces/{workspaceId}/available-outputs

Response:
{
  "resources": [
    {
      "resourceName": "my_s3_bucket",
      "resourceType": "aws_s3",
      "moduleName": "S3 Bucket",
      "outputs": [
        {
          "name": "bucket_id",
          "alias": "存储桶ID",
          "type": "string",
          "description": "The name of the bucket",
          "sensitive": false,
          "reference": "module.my_s3_bucket.bucket_id"
        },
        {
          "name": "bucket_arn",
          "alias": "存储桶ARN",
          "type": "string",
          "description": "The ARN of the bucket",
          "sensitive": false,
          "reference": "module.my_s3_bucket.bucket_arn"
        }
      ]
    },
    {
      "resourceName": "my_kms_key",
      "resourceType": "aws_kms",
      "moduleName": "KMS Key",
      "outputs": [
        {
          "name": "key_arn",
          "alias": "密钥ARN",
          "type": "string",
          "description": "The ARN of the KMS key",
          "sensitive": false,
          "reference": "module.my_kms_key.key_arn"
        }
      ]
    }
  ]
}
```

### 7.2 前端智能提示组件

```typescript
interface OutputSuggestion {
  resourceName: string;
  resourceType: string;
  moduleName: string;
  outputs: OutputItem[];
}

interface OutputItem {
  name: string;
  alias: string;
  type: string;
  description: string;
  sensitive: boolean;
  reference: string;  // 完整引用路径，如 "module.my_s3_bucket.bucket_arn"
}

// 使用示例
<OutputSuggestionDropdown
  workspaceId={workspaceId}
  onSelect={(output) => {
    // 插入引用到输入框
    setOutputValue(output.reference);
  }}
/>
```

## 8. 实现计划

### Phase 1: 扩展 tf2openapi 工具 (2-3小时)

1. 添加 `parseOutputs` 函数解析 outputs.tf
2. 支持 output 注释解析（@alias, @sensitive 等）
3. 生成 `ModuleOutput` Schema 和 `x-iac-platform.outputs`

### Phase 2: 后端 API (2-3小时)

1. 扩展 Schema 导入 API 支持多文件
2. 新增 `/available-outputs` API 提供智能提示数据
3. 存储 Output 定义到数据库

### Phase 3: 前端智能提示 (3-4小时)

1. 更新 Schema 导入组件支持多文件上传
2. 实现 Output 智能提示下拉组件
3. 集成到 Workspace Outputs 配置页面
4. 集成到资源参数配置表单

### Phase 4: 模块文档展示 (1-2小时)

1. 在模块详情页添加 Outputs Tab
2. 展示模块的输出列表

## 9. 总结

| 功能 | 用途 | 优先级 |
|------|------|--------|
| Output 解析 | 从 outputs.tf 提取输出定义 | P0 |
| 智能提示 API | 提供 Workspace 可用输出列表 | P0 |
| Workspace Outputs 智能提示 | 配置 Output 时提示可用值 | P0 |
| 资源引用智能提示 | 配置参数时提示可用引用 | P1 |
| 模块文档展示 | 展示模块输出列表 | P2 |

**预计总工时：8-12小时**
