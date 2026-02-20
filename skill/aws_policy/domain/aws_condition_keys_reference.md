---
name: aws_condition_keys_reference
layer: domain
description: AWS IAM 策略常用条件键的快速参考，包括全局条件键、服务特定条件键、条件操作符和使用示例
tags: ["domain", "aws", "iam", "policy", "condition-keys", "security", "reference"]
---

# AWS 条件键快速参考

## 概述

本文档提供 AWS IAM 策略中常用条件键的快速参考。条件键用于限制权限的使用时机和方式，是实现最小权限原则的关键工具。

---

## 第一部分：全局条件键

### 身份和认证

| 条件键 | 类型 | 说明 | 示例值 |
|--------|------|------|--------|
| `aws:PrincipalArn` | ARN | 调用者的 ARN | `arn:aws:iam::123456789012:role/MyRole` |
| `aws:PrincipalAccount` | String | 调用者的账户 ID | `123456789012` |
| `aws:PrincipalOrgID` | String | 调用者的组织 ID | `o-xxxxxxxxxx` |
| `aws:PrincipalOrgPaths` | String | 调用者的组织路径 | `o-xxx/r-xxx/ou-xxx/` |
| `aws:PrincipalTag/key` | String | 调用者的标签值 | `${aws:PrincipalTag/Team}` |
| `aws:PrincipalType` | String | 调用者类型 | `User`, `Role`, `FederatedUser` |
| `aws:userid` | String | 调用者的用户 ID | `AIDAXXXXXXXXXXXXXXXXX` |
| `aws:username` | String | IAM 用户名 | `alice` |

### 安全和认证

| 条件键 | 类型 | 说明 | 示例值 |
|--------|------|------|--------|
| `aws:SecureTransport` | Bool | 是否使用 HTTPS | `true`, `false` |
| `aws:MultiFactorAuthPresent` | Bool | 是否使用 MFA | `true`, `false` |
| `aws:MultiFactorAuthAge` | Numeric | MFA 认证后的秒数 | `3600` |
| `aws:TokenIssueTime` | Date | 临时凭证发放时间 | `2024-01-01T00:00:00Z` |

### 网络和来源

| 条件键 | 类型 | 说明 | 示例值 |
|--------|------|------|--------|
| `aws:SourceIp` | IP | 请求来源 IP | `192.168.1.0/24` |
| `aws:SourceVpc` | String | 请求来源 VPC | `vpc-0123456789abcdef0` |
| `aws:SourceVpce` | String | 请求来源 VPC 端点 | `vpce-0123456789abcdef0` |
| `aws:VpcSourceIp` | IP | VPC 内的源 IP | `10.0.0.1` |
| `aws:SourceArn` | ARN | 请求来源资源 ARN | `arn:aws:s3:::my-bucket` |
| `aws:SourceAccount` | String | 请求来源账户 | `123456789012` |

### 时间和区域

| 条件键 | 类型 | 说明 | 示例值 |
|--------|------|------|--------|
| `aws:CurrentTime` | Date | 当前 UTC 时间 | `2024-01-01T12:00:00Z` |
| `aws:EpochTime` | Numeric | Unix 时间戳 | `1704067200` |
| `aws:RequestedRegion` | String | 请求的区域 | `us-east-1` |

### 标签

| 条件键 | 类型 | 说明 | 示例值 |
|--------|------|------|--------|
| `aws:RequestTag/key` | String | 请求中的标签值 | `aws:RequestTag/Environment` |
| `aws:ResourceTag/key` | String | 资源的标签值 | `aws:ResourceTag/Owner` |
| `aws:TagKeys` | ArrayOfString | 请求中的标签键列表 | `["Environment", "Owner"]` |

### 服务相关

| 条件键 | 类型 | 说明 | 示例值 |
|--------|------|------|--------|
| `aws:ViaAWSService` | Bool | 是否通过 AWS 服务调用 | `true`, `false` |
| `aws:PrincipalIsAWSService` | Bool | 调用者是否为 AWS 服务 | `true`, `false` |
| `aws:CalledVia` | ArrayOfString | 调用链中的服务 | `["cloudformation.amazonaws.com"]` |
| `aws:CalledViaFirst` | String | 调用链中的第一个服务 | `cloudformation.amazonaws.com` |
| `aws:CalledViaLast` | String | 调用链中的最后一个服务 | `lambda.amazonaws.com` |

---

## 第二部分：服务特定条件键

### S3 条件键

| 条件键 | 类型 | 说明 |
|--------|------|------|
| `s3:x-amz-server-side-encryption` | String | 服务端加密类型 (`AES256`, `aws:kms`) |
| `s3:x-amz-server-side-encryption-aws-kms-key-id` | ARN | KMS 密钥 ARN |
| `s3:x-amz-acl` | String | 对象 ACL (`private`, `public-read`, `bucket-owner-full-control`) |
| `s3:prefix` | String | 对象前缀 |
| `s3:delimiter` | String | 列表分隔符 |
| `s3:max-keys` | Numeric | 最大返回数量 |
| `s3:ExistingObjectTag/key` | String | 现有对象的标签值 |
| `s3:RequestObjectTag/key` | String | 请求中的对象标签值 |
| `s3:RequestObjectTagKeys` | ArrayOfString | 请求中的对象标签键 |
| `s3:VersionId` | String | 对象版本 ID |
| `s3:object-lock-mode` | String | 对象锁定模式 |
| `s3:object-lock-retain-until-date` | Date | 对象锁定保留日期 |

### KMS 条件键

| 条件键 | 类型 | 说明 |
|--------|------|------|
| `kms:ViaService` | String | 通过哪个服务使用密钥 (`s3.us-east-1.amazonaws.com`) |
| `kms:EncryptionContext:key` | String | 加密上下文的键值 |
| `kms:EncryptionContextKeys` | ArrayOfString | 加密上下文的键列表 |
| `kms:GrantIsForAWSResource` | Bool | 授权是否用于 AWS 资源 |
| `kms:CallerAccount` | String | 调用者账户 ID |
| `kms:KeyOrigin` | String | 密钥来源 (`AWS_KMS`, `EXTERNAL`, `AWS_CLOUDHSM`) |
| `kms:KeySpec` | String | 密钥规格 (`SYMMETRIC_DEFAULT`, `RSA_2048`) |
| `kms:KeyUsage` | String | 密钥用途 (`ENCRYPT_DECRYPT`, `SIGN_VERIFY`) |
| `kms:RetiringPrincipal` | ARN | 授权的退休主体 |
| `kms:GranteePrincipal` | ARN | 授权的受让主体 |
| `kms:GrantOperations` | ArrayOfString | 授权的操作列表 |

### Secrets Manager 条件键

| 条件键 | 类型 | 说明 |
|--------|------|------|
| `secretsmanager:VersionStage` | String | 密钥版本阶段 (`AWSCURRENT`, `AWSPREVIOUS`, `AWSPENDING`) |
| `secretsmanager:VersionId` | String | 密钥版本 ID |
| `secretsmanager:ResourceTag/key` | String | 密钥资源标签值 |
| `secretsmanager:SecretId` | String | 密钥 ID 或 ARN |
| `secretsmanager:Name` | String | 密钥名称 |
| `secretsmanager:Description` | String | 密钥描述 |
| `secretsmanager:KmsKeyId` | String | 加密密钥 ID |
| `secretsmanager:RotationLambdaARN` | ARN | 轮换 Lambda ARN |
| `secretsmanager:BlockPublicPolicy` | Bool | 是否阻止公开策略 |

### EC2 条件键

| 条件键 | 类型 | 说明 |
|--------|------|------|
| `ec2:InstanceType` | String | 实例类型 (`t3.micro`, `m5.large`) |
| `ec2:ResourceTag/key` | String | 资源标签值 |
| `ec2:Region` | String | 区域 |
| `ec2:AvailabilityZone` | String | 可用区 |
| `ec2:Vpc` | ARN | VPC ARN |
| `ec2:Subnet` | ARN | 子网 ARN |
| `ec2:ImageType` | String | 镜像类型 (`machine`, `kernel`, `ramdisk`) |
| `ec2:Owner` | String | 资源所有者账户 |
| `ec2:IsLaunchTemplateResource` | Bool | 是否为启动模板资源 |
| `ec2:CreateAction` | String | 创建操作 (`RunInstances`, `CreateVolume`) |
| `ec2:Tenancy` | String | 租户类型 (`default`, `dedicated`, `host`) |
| `ec2:VolumeType` | String | 卷类型 (`gp3`, `io2`, `st1`) |
| `ec2:VolumeSize` | Numeric | 卷大小 (GB) |
| `ec2:Encrypted` | Bool | 是否加密 |

### Lambda 条件键

| 条件键 | 类型 | 说明 |
|--------|------|------|
| `lambda:FunctionArn` | ARN | 函数 ARN |
| `lambda:FunctionUrlAuthType` | String | 函数 URL 认证类型 (`AWS_IAM`, `NONE`) |
| `lambda:Principal` | String | 调用主体 |
| `lambda:SourceFunctionArn` | ARN | 源函数 ARN |
| `lambda:Layer` | ArrayOfString | 层 ARN 列表 |
| `lambda:SecurityGroupIds` | ArrayOfString | 安全组 ID 列表 |
| `lambda:SubnetIds` | ArrayOfString | 子网 ID 列表 |
| `lambda:VpcIds` | String | VPC ID |

### RDS 条件键

| 条件键 | 类型 | 说明 |
|--------|------|------|
| `rds:DatabaseClass` | String | 数据库实例类型 (`db.t3.micro`) |
| `rds:DatabaseEngine` | String | 数据库引擎 (`mysql`, `postgres`) |
| `rds:DatabaseName` | String | 数据库名称 |
| `rds:StorageEncrypted` | Bool | 是否加密存储 |
| `rds:MultiAz` | Bool | 是否多可用区 |
| `rds:Vpc` | String | VPC ID |
| `rds:cluster-tag/key` | String | 集群标签值 |
| `rds:db-tag/key` | String | 实例标签值 |

### IAM 条件键

| 条件键 | 类型 | 说明 |
|--------|------|------|
| `iam:PassedToService` | String | 角色传递的目标服务 |
| `iam:PermissionsBoundary` | ARN | 权限边界策略 ARN |
| `iam:PolicyARN` | ARN | 策略 ARN |
| `iam:ResourceTag/key` | String | IAM 资源标签值 |
| `iam:AWSServiceName` | String | 服务链接角色的服务名 |
| `iam:OrganizationsPolicyId` | String | 组织策略 ID |

### STS 条件键

| 条件键 | 类型 | 说明 |
|--------|------|------|
| `sts:ExternalId` | String | 外部 ID |
| `sts:RoleSessionName` | String | 角色会话名称 |
| `sts:TransitiveTagKeys` | ArrayOfString | 可传递的标签键 |
| `sts:SourceIdentity` | String | 源身份 |
| `sts:AWSServiceName` | String | 服务名称 |

---

## 第三部分：条件操作符

### 字符串操作符

| 操作符 | 说明 | 示例 |
|--------|------|------|
| `StringEquals` | 精确匹配（区分大小写） | `"aws:PrincipalOrgID": "o-xxx"` |
| `StringNotEquals` | 不等于 | `"aws:PrincipalArn": "arn:..."` |
| `StringEqualsIgnoreCase` | 精确匹配（不区分大小写） | `"s3:prefix": "logs/"` |
| `StringLike` | 通配符匹配（`*`, `?`） | `"s3:prefix": "home/${aws:username}/*"` |
| `StringNotLike` | 通配符不匹配 | `"aws:PrincipalArn": "arn:*:role/Admin*"` |

### 数值操作符

| 操作符 | 说明 | 示例 |
|--------|------|------|
| `NumericEquals` | 等于 | `"s3:max-keys": "100"` |
| `NumericNotEquals` | 不等于 | `"ec2:VolumeSize": "500"` |
| `NumericLessThan` | 小于 | `"aws:MultiFactorAuthAge": "3600"` |
| `NumericLessThanEquals` | 小于等于 | `"ec2:VolumeSize": "100"` |
| `NumericGreaterThan` | 大于 | `"aws:EpochTime": "1704067200"` |
| `NumericGreaterThanEquals` | 大于等于 | `"ec2:VolumeSize": "50"` |

### 日期操作符

| 操作符 | 说明 | 示例 |
|--------|------|------|
| `DateEquals` | 等于 | `"aws:CurrentTime": "2024-01-01T00:00:00Z"` |
| `DateNotEquals` | 不等于 | `"aws:TokenIssueTime": "..."` |
| `DateLessThan` | 早于 | `"aws:CurrentTime": "2024-12-31T23:59:59Z"` |
| `DateLessThanEquals` | 早于等于 | `"aws:CurrentTime": "..."` |
| `DateGreaterThan` | 晚于 | `"aws:CurrentTime": "2024-01-01T00:00:00Z"` |
| `DateGreaterThanEquals` | 晚于等于 | `"aws:CurrentTime": "..."` |

### 布尔操作符

| 操作符 | 说明 | 示例 |
|--------|------|------|
| `Bool` | 布尔值匹配 | `"aws:SecureTransport": "true"` |

### IP 地址操作符

| 操作符 | 说明 | 示例 |
|--------|------|------|
| `IpAddress` | IP 地址/CIDR 匹配 | `"aws:SourceIp": "192.168.1.0/24"` |
| `NotIpAddress` | IP 地址/CIDR 不匹配 | `"aws:SourceIp": "10.0.0.0/8"` |

### ARN 操作符

| 操作符 | 说明 | 示例 |
|--------|------|------|
| `ArnEquals` | ARN 精确匹配 | `"aws:SourceArn": "arn:aws:s3:::bucket"` |
| `ArnNotEquals` | ARN 不等于 | `"aws:PrincipalArn": "arn:..."` |
| `ArnLike` | ARN 通配符匹配 | `"aws:PrincipalArn": "arn:*:role/Admin*"` |
| `ArnNotLike` | ARN 通配符不匹配 | `"aws:SourceArn": "arn:*:s3:::*"` |

### 空值操作符

| 操作符 | 说明 | 示例 |
|--------|------|------|
| `Null` | 检查键是否存在 | `"s3:x-amz-server-side-encryption": "true"` (不存在) |

### 集合操作符

| 操作符 | 说明 | 示例 |
|--------|------|------|
| `ForAllValues:StringEquals` | 所有值都匹配 | `"aws:TagKeys": ["Env", "Owner"]` |
| `ForAnyValue:StringEquals` | 任一值匹配 | `"secretsmanager:VersionStage": ["AWSCURRENT"]` |
| `ForAllValues:StringLike` | 所有值通配符匹配 | `"aws:TagKeys": ["*"]` |
| `ForAnyValue:StringLike` | 任一值通配符匹配 | `"s3:prefix": ["logs/*", "data/*"]` |

---

## 第四部分：常用条件模式

### 强制 HTTPS

```json
{
  "Condition": {
    "Bool": {
      "aws:SecureTransport": "false"
    }
  }
}
```

### 要求 MFA

```json
{
  "Condition": {
    "Bool": {
      "aws:MultiFactorAuthPresent": "true"
    }
  }
}
```

### MFA 有效期限制

```json
{
  "Condition": {
    "NumericLessThan": {
      "aws:MultiFactorAuthAge": "3600"
    }
  }
}
```

### 限制来源 IP

```json
{
  "Condition": {
    "IpAddress": {
      "aws:SourceIp": ["192.168.1.0/24", "10.0.0.0/8"]
    }
  }
}
```

### 限制 VPC 端点

```json
{
  "Condition": {
    "StringEquals": {
      "aws:SourceVpce": "vpce-1234567890abcdef0"
    }
  }
}
```

### 限制组织

```json
{
  "Condition": {
    "StringEquals": {
      "aws:PrincipalOrgID": "o-xxxxxxxxxx"
    }
  }
}
```

### 时间窗口限制

```json
{
  "Condition": {
    "DateGreaterThan": {
      "aws:CurrentTime": "2024-01-01T00:00:00Z"
    },
    "DateLessThan": {
      "aws:CurrentTime": "2024-12-31T23:59:59Z"
    }
  }
}
```

### 强制标签

```json
{
  "Condition": {
    "StringEquals": {
      "aws:RequestTag/Environment": ["dev", "staging", "prod"]
    },
    "ForAllValues:StringEquals": {
      "aws:TagKeys": ["Environment", "Owner", "CostCenter"]
    }
  }
}
```

### 基于资源标签

```json
{
  "Condition": {
    "StringEquals": {
      "aws:ResourceTag/Owner": "${aws:username}"
    }
  }
}
```

### 限制实例类型

```json
{
  "Condition": {
    "StringEquals": {
      "ec2:InstanceType": ["t3.micro", "t3.small", "t3.medium"]
    }
  }
}
```

### 限制服务使用 KMS

```json
{
  "Condition": {
    "StringEquals": {
      "kms:ViaService": "s3.us-east-1.amazonaws.com"
    }
  }
}
```

### 限制角色传递

```json
{
  "Condition": {
    "StringEquals": {
      "iam:PassedToService": "lambda.amazonaws.com"
    }
  }
}
```

### 排除 AWS 服务调用

```json
{
  "Condition": {
    "Bool": {
      "aws:ViaAWSService": "false"
    }
  }
}
```

---

## 第五部分：条件键使用注意事项

### 条件键可用性

| 场景 | 说明 |
|------|------|
| 全局条件键 | 大多数情况下可用 |
| 服务条件键 | 仅在特定服务操作中可用 |
| 请求条件键 | 仅在请求包含相关信息时可用 |
| 资源条件键 | 仅在资源存在时可用 |

### 常见问题

| 问题 | 原因 | 解决方案 |
|------|------|----------|
| 条件不生效 | 条件键不适用于该操作 | 检查服务文档确认支持的条件键 |
| 意外拒绝 | 条件键值为空 | 使用 `Null` 操作符检查键是否存在 |
| 通配符不匹配 | 使用了 `StringEquals` | 改用 `StringLike` |
| 大小写问题 | 值大小写不匹配 | 使用 `StringEqualsIgnoreCase` |

### 最佳实践

| 实践 | 说明 |
|------|------|
| 测试条件 | 使用 IAM Policy Simulator 测试 |
| 检查可用性 | 确认条件键适用于目标操作 |
| 使用正确操作符 | 根据值类型选择操作符 |
| 处理空值 | 使用 `Null` 条件处理可能不存在的键 |
| 组合条件 | 同一 Condition 块内的条件是 AND 关系 |