---
name: aws_policy_core_principles
layer: domain
description: AWS IAM 和资源策略的核心原则，包括最小权限、显式拒绝、策略结构、验证规则和常见错误
tags: ["domain", "aws", "iam", "policy", "security", "best-practices", "least-privilege"]
---

# AWS 策略核心原则

## 概述

本文档定义了生成 AWS IAM 策略和资源策略时必须遵循的核心原则。这些原则确保生成的策略安全、合规且符合 AWS 最佳实践。

---

## 第一部分：五大核心原则

### 1. 最小权限原则 (Least Privilege)

**定义**：仅授予完成任务所需的最低权限，不多不少。

| 做法 | 示例 |
|------|------|
| ✅ 使用具体操作 | `s3:GetObject`, `s3:PutObject` |
| ❌ 使用通配符操作 | `s3:*` |
| ✅ 指定具体资源 ARN | `arn:aws:s3:::my-bucket/*` |
| ❌ 使用通配符资源 | `*` |

### 2. 显式拒绝优于隐式允许 (Explicit Deny)

**定义**：对关键限制使用显式 Deny，而非仅依赖缺少 Allow。

**适用场景**：
- 强制加密传输（拒绝非 HTTPS）
- 强制服务端加密（拒绝未加密上传）
- 阻止特定高危操作

```json
{
  "Effect": "Deny",
  "Action": "s3:*",
  "Resource": "*",
  "Condition": {
    "Bool": {
      "aws:SecureTransport": "false"
    }
  }
}
```

### 3. 条件键约束 (Condition Keys)

**定义**：使用条件键限制权限的使用时机和方式。

**常用条件类型**：

| 条件类型 | 用途 | 示例条件键 |
|----------|------|------------|
| 安全传输 | 强制 HTTPS | `aws:SecureTransport` |
| 身份验证 | 要求 MFA | `aws:MultiFactorAuthPresent` |
| 网络限制 | 限制来源 IP/VPC | `aws:SourceIp`, `aws:SourceVpc` |
| 时间限制 | 限制访问时间 | `aws:CurrentTime` |
| 加密要求 | 强制加密 | `s3:x-amz-server-side-encryption` |

### 4. 资源特定性 (Resource Specificity)

**定义**：尽可能避免资源使用通配符 (*)。

**ARN 格式**：
```
arn:partition:service:region:account-id:resource-type/resource-id
```

**示例**：

| 资源类型 | 具体 ARN 示例 |
|----------|---------------|
| S3 桶 | `arn:aws:s3:::my-bucket` |
| S3 对象 | `arn:aws:s3:::my-bucket/*` |
| KMS 密钥 | `arn:aws:kms:us-east-1:123456789012:key/key-id` |
| Lambda 函数 | `arn:aws:lambda:us-east-1:123456789012:function:my-function` |
| DynamoDB 表 | `arn:aws:dynamodb:us-east-1:123456789012:table/my-table` |

### 5. 操作粒度 (Action Granularity)

**定义**：使用具体操作而非宽泛通配符。

**常见服务的操作分类**：

| 服务 | 读取操作 | 写入操作 | 管理操作 |
|------|----------|----------|----------|
| S3 | `GetObject`, `ListBucket` | `PutObject`, `DeleteObject` | `PutBucketPolicy`, `DeleteBucket` |
| KMS | `Decrypt`, `DescribeKey` | `Encrypt`, `GenerateDataKey` | `CreateKey`, `ScheduleKeyDeletion` |
| Lambda | `GetFunction`, `ListFunctions` | `InvokeFunction` | `CreateFunction`, `DeleteFunction` |

---

## 第二部分：策略类型

### IAM 策略（身份策略）

**定义**：附加到用户、组或角色，定义他们可以执行的操作。

**特点**：
- 不包含 Principal 字段（主体由附加对象决定）
- 可以是托管策略或内联策略

**结构**：
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "描述性语句ID",
      "Effect": "Allow",
      "Action": ["service:Action"],
      "Resource": ["arn:aws:service:region:account:resource"],
      "Condition": {}
    }
  ]
}
```

### 资源策略

**定义**：直接附加到 AWS 资源（S3 桶、KMS 密钥等）。

**特点**：
- **必须包含 Principal 字段**
- 定义谁可以访问该资源

**支持资源策略的服务**：
- S3（桶策略）
- KMS（密钥策略）
- Secrets Manager（密钥策略）
- SNS（主题策略）
- SQS（队列策略）
- Lambda（资源策略）
- ECR（仓库策略）
- EventBridge（事件总线策略）

**结构**：
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "描述性语句ID",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/MyRole"
      },
      "Action": ["service:Action"],
      "Resource": ["arn:aws:service:region:account:resource"],
      "Condition": {}
    }
  ]
}
```

---

## 第三部分：Principal 格式

### Principal 类型

| 类型 | 格式 | 示例 |
|------|------|------|
| AWS 账户 | `"AWS": "arn:aws:iam::account-id:root"` | `"AWS": "arn:aws:iam::123456789012:root"` |
| IAM 角色 | `"AWS": "arn:aws:iam::account-id:role/role-name"` | `"AWS": "arn:aws:iam::123456789012:role/MyRole"` |
| IAM 用户 | `"AWS": "arn:aws:iam::account-id:user/user-name"` | `"AWS": "arn:aws:iam::123456789012:user/Alice"` |
| AWS 服务 | `"Service": "service.amazonaws.com"` | `"Service": "s3.amazonaws.com"` |
| 所有人 | `"*"` | 谨慎使用，通常需配合条件 |

### 多主体格式

```json
{
  "Principal": {
    "AWS": [
      "arn:aws:iam::111111111111:role/Role1",
      "arn:aws:iam::222222222222:role/Role2"
    ],
    "Service": "lambda.amazonaws.com"
  }
}
```

### Terraform 场景：避免循环依赖

当资源策略中引用的 IAM Role 和资源本身在同一 Terraform 配置中创建时，直接使用 `"Principal": {"AWS": "arn:..."}` 会触发 AWS 的 **Principal 强校验**（验证 Role 是否存在），导致循环依赖。

**解决方案**：使用 `Principal: "*"` 配合 `aws:PrincipalArn` 条件，跳过强校验：

```json
{
  "Sid": "AllowRoleAccessAvoidCircularDependency",
  "Effect": "Allow",
  "Principal": "*",
  "Action": ["s3:GetObject", "s3:PutObject"],
  "Resource": ["arn:aws:s3:::my-bucket", "arn:aws:s3:::my-bucket/*"],
  "Condition": {
    "StringLike": {
      "aws:PrincipalArn": "arn:aws:iam::123456789012:role/MyAppRole*"
    }
  }
}
```

| 方式 | Principal | 是否强校验 | 循环依赖风险 |
|------|-----------|------------|--------------|
| 直接 Principal | `"AWS": "arn:...role/xxx"` | 是 | **高** |
| Condition 方式 | `"*"` + `aws:PrincipalArn` | 否 | **无** |

**安全要求**：使用此模式时，`aws:PrincipalArn` 中**必须包含具体账户 ID**，禁止使用通配符账户（`arn:aws:iam::*:role/...`），防止任意账户的同名 Role 获得访问权限。

**适用范围**：本平台生成的资源策略默认采用此模式，因为 Role 和资源通常在同一 Terraform 配置中管理。

---

## 第四部分：策略验证规则

### 必须检查项

| 检查项 | 说明 |
|--------|------|
| JSON 语法 | 策略必须是有效的 JSON |
| Version 字段 | 必须包含 `"Version": "2012-10-17"` |
| Statement 数组 | 必须包含至少一个 Statement |
| Effect 字段 | 每个 Statement 必须有 `Allow` 或 `Deny` |
| Action 字段 | 必须指定操作（字符串或数组） |
| Resource 字段 | IAM 策略必须指定资源 |
| Principal 字段 | 资源策略必须指定主体 |

### ARN 格式验证

| 服务 | ARN 格式 |
|------|----------|
| S3 桶 | `arn:aws:s3:::bucket-name` |
| S3 对象 | `arn:aws:s3:::bucket-name/key` 或 `arn:aws:s3:::bucket-name/*` |
| KMS | `arn:aws:kms:region:account:key/key-id` 或 `arn:aws:kms:region:account:alias/alias-name` |
| Lambda | `arn:aws:lambda:region:account:function:function-name` |
| DynamoDB | `arn:aws:dynamodb:region:account:table/table-name` |
| Secrets Manager | `arn:aws:secretsmanager:region:account:secret:secret-name` |

### 条件操作符验证

| 操作符 | 适用类型 | 示例 |
|--------|----------|------|
| `StringEquals` | 精确字符串匹配 | `"aws:PrincipalOrgID": "o-xxxxx"` |
| `StringLike` | 通配符字符串匹配 | `"s3:prefix": "home/${aws:username}/*"` |
| `Bool` | 布尔值 | `"aws:SecureTransport": "true"` |
| `IpAddress` | IP 地址/CIDR | `"aws:SourceIp": "192.168.1.0/24"` |
| `DateGreaterThan` | 日期比较 | `"aws:CurrentTime": "2024-01-01T00:00:00Z"` |
| `ArnLike` | ARN 通配符匹配 | `"aws:PrincipalArn": "arn:aws:iam::*:role/Admin*"` |

---

## 第五部分：常见错误避免

### 错误清单

| 错误 | 问题 | 正确做法 |
|------|------|----------|
| 资源使用 `*` | 权限过于宽泛 | 指定具体资源 ARN |
| 资源策略缺少 Principal | 策略无效 | 添加 Principal 字段 |
| 缺少 Version 字段 | 可能使用旧版策略语言 | 添加 `"Version": "2012-10-17"` |
| ARN 格式错误 | 策略不生效 | 检查服务的 ARN 格式 |
| 条件操作符错误 | `StringEquals` vs `StringLike` | 根据是否需要通配符选择 |
| 操作过于宽泛 | `s3:*` 授予所有 S3 权限 | 使用具体操作如 `s3:GetObject` |
| 忘记桶和对象分开 | S3 桶操作和对象操作需要不同 ARN | 桶：`arn:aws:s3:::bucket`，对象：`arn:aws:s3:::bucket/*` |

### KMS 特殊注意事项

- KMS 密钥策略中的 `"Resource": "*"` 表示"此密钥"，不是所有密钥
- 必须包含根账户访问权限，防止密钥锁定
- 使用 `kms:ViaService` 限制哪些服务可以使用密钥

---

## 第六部分：策略生成工作流

### 步骤 1：理解需求

收集以下信息：
- 哪些资源需要访问控制？
- 谁/什么需要访问（主体）？
- 需要哪些操作？
- 是否有条件要求（MFA、IP、时间）？

### 步骤 2：选择策略类型

| 场景 | 策略类型 |
|------|----------|
| 控制用户/角色能做什么 | IAM 策略 |
| 控制谁能访问特定资源 | 资源策略 |
| 跨账户访问 | 两者都需要 |

### 步骤 3：生成基础策略

- 从最小权限开始
- 使用具体操作，不用通配符
- 尽可能指定确切资源

### 步骤 4：添加安全条件

- 强制加密（传输/静态）
- 为敏感操作添加 MFA 要求
- 如适用，按 IP、VPC 或时间限制

### 步骤 5：验证和文档化

- 检查 JSON 语法
- 验证 ARN 格式
- 为每个 Statement 添加描述性 Sid