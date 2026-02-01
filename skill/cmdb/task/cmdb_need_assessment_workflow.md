---
name: cmdb_need_assessment_workflow
layer: task
description: CMDB 需求评估工作流，判断用户需求是否需要查询 CMDB 获取现有资源
tags: ["task", "cmdb", "assessment", "resource", "detection"]
priority: 0
domain_tags: ["cmdb"]
---

# CMDB 需求评估工作流

分析用户的需求描述，判断是否需要从 CMDB（配置管理数据库）查询现有资源。

## 任务目标

根据用户的自然语言描述，智能判断是否需要查询 CMDB 来获取现有资源的信息（如 VPC ID、Subnet ID、IAM Role ARN 等）。

## 判断规则

### 需要查询 CMDB 的情况

1. **明确引用 CMDB**
   - 用户提到 "cmdb"、"配置库"、"资产库"、"资源库"
   - 用户提到 "来自 cmdb"、"从 cmdb 获取"

2. **引用现有资源**
   - 用户提到 "现有的"、"已有的"、"existing"
   - 用户提到 "使用"、"引用"、"关联"、"绑定"、"连接"
   - 用户提到 "attach"、"associate"、"from"

3. **权限策略引用**
   - 用户提到需要允许/拒绝特定服务或角色访问
   - 用户提到 "role"、"角色"、"policy"、"策略"、"权限"
   - 用户提到 "principal"、"assume"、"trust"

4. **网络资源引用**
   - 用户提到 VPC、子网、安全组等网络资源
   - 用户提到 "vpc"、"subnet"、"security group"、"安全组"、"子网"

5. **服务资源引用**
   - 用户提到特定 AWS 服务的资源（ec2、lambda、ecs、rds 等）
   - 用户提到 ARN 或资源 ID

6. **环境标识**
   - 用户提到特定环境（production、staging、dev 等）
   - 用户提到特定项目或应用名称

### 不需要查询 CMDB 的情况

1. **创建全新资源**
   - 用户只是创建全新的资源，不引用任何现有资源
   - 用户的需求完全自包含，不依赖外部资源
   - **重要**：用户说"创建一个 S3 桶"、"创建一个 EC2 实例"等，这是创建新资源，不需要查询 CMDB

2. **配置独立资源**
   - 用户配置的资源不需要与其他资源关联
   - 所有必要的 ID 和 ARN 用户会自行提供

## 关键区分规则

**只查询需要引用的现有资源，不查询要创建的资源**

| 用户描述 | 需要查询 CMDB 的资源 | 不需要查询的资源 |
|----------|---------------------|-----------------|
| "创建一个 S3 桶，允许 ec2 role 访问" | `aws_iam_role` (ec2 role) | `aws_s3_bucket` (要创建的) |
| "创建一个 EC2，使用现有 VPC" | `aws_vpc`, `aws_subnet` | `aws_instance` (要创建的) |
| "创建一个 Lambda，允许访问 RDS" | `aws_db_instance` (RDS) | `aws_lambda_function` (要创建的) |
| "创建一个 S3 桶，启用版本控制" | 无 | `aws_s3_bucket` (要创建的) |

## 输入

用户需求描述：
```
{{user_description}}
```

## 输出格式

请返回 JSON 格式，不要有任何额外文字：

```json
{
  "need_cmdb": true/false,
  "reason": "简短说明判断理由（不超过30字）",
  "resource_types": ["需要查询的资源类型列表，如 aws_iam_role, aws_vpc 等"],
  "query_plan": [
    {
      "resource_type": "资源类型",
      "target_field": "目标字段名（用于区分同类型的多个资源）",
      "filters": {
        "name_contains": "名称包含的关键词（可选）",
        "tags": {"标签键": "标签值"}
      },
      "limit": 10
    }
  ]
}
```

### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `need_cmdb` | boolean | 是 | 是否需要查询 CMDB |
| `reason` | string | 是 | 判断理由，不超过 30 字 |
| `resource_types` | string[] | 是 | 需要查询的资源类型列表 |
| `query_plan` | object[] | 否 | 查询计划，仅当 `need_cmdb=true` 时需要 |

### query_plan 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `resource_type` | string | 是 | 资源类型，如 `aws_iam_role`、`aws_vpc` |
| `target_field` | string | **是** | **目标字段名，用于区分同类型的多个资源（如 `ec2_role`、`lambda_role`）** |
| `filters` | object | 否 | 过滤条件 |
| `filters.name_contains` | string | 否 | 名称包含的关键词 |
| `filters.tags` | object | 否 | 标签过滤，键值对形式 |
| `limit` | number | 否 | 返回结果数量限制，默认 10 |

**重要**：`target_field` 是必填字段！当有多个同类型资源时，必须使用不同的 `target_field` 来区分它们。

## 示例

### 示例 1：需要 CMDB（带查询计划）

用户输入：
> 创建一个 S3 桶，允许来自 cmdb 中 ec2 的 role 仅可读

输出：
```json
{
  "need_cmdb": true,
  "reason": "用户明确提到从 cmdb 获取 ec2 的 role",
  "resource_types": ["aws_iam_role"],
  "query_plan": [
    {
      "resource_type": "aws_iam_role",
      "target_field": "ec2_role",
      "filters": {
        "name_contains": "ec2"
      },
      "limit": 10
    }
  ]
}
```

### 示例 1.5：需要 CMDB（多个同类型资源）

用户输入：
> 创建一个 S3 桶，允许 ec2 role 只读，允许 lambda role 只写

输出：
```json
{
  "need_cmdb": true,
  "reason": "用户需要两个不同的 IAM Role",
  "resource_types": ["aws_iam_role"],
  "query_plan": [
    {
      "resource_type": "aws_iam_role",
      "target_field": "ec2_role",
      "filters": {
        "name_contains": "ec2"
      },
      "limit": 10
    },
    {
      "resource_type": "aws_iam_role",
      "target_field": "lambda_role",
      "filters": {
        "name_contains": "lambda"
      },
      "limit": 10
    }
  ]
}
```

**重要**：当用户需要多个同类型资源用于不同目的时，必须为每个资源创建独立的查询计划项，并使用不同的 `target_field` 来区分。

### 示例 2：需要 CMDB（多资源查询）

用户输入：
> 在 production 环境的 VPC 中创建一个 EC2 实例

输出：
```json
{
  "need_cmdb": true,
  "reason": "用户提到使用 production 环境的 VPC",
  "resource_types": ["aws_vpc", "aws_subnet"],
  "query_plan": [
    {
      "resource_type": "aws_vpc",
      "target_field": "vpc",
      "filters": {
        "tags": {"Environment": "production"}
      },
      "limit": 5
    },
    {
      "resource_type": "aws_subnet",
      "target_field": "subnet",
      "filters": {
        "tags": {"Environment": "production"}
      },
      "limit": 10
    }
  ]
}
```

### 示例 3：不需要 CMDB

用户输入：
> 创建一个名为 my-bucket 的 S3 桶，启用版本控制

输出：
```json
{
  "need_cmdb": false,
  "reason": "创建独立的 S3 桶，不引用现有资源",
  "resource_types": [],
  "query_plan": []
}
```

## 注意事项

1. 如果不确定，倾向于返回 `need_cmdb: true`，宁可多查询也不要遗漏
2. 关注用户描述中的关键词和上下文
3. 考虑资源之间的依赖关系