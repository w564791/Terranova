---
name: aws_s3_policy_patterns
layer: domain
description: AWS S3 桶策略的常见模式和最佳实践，包括 HTTPS 强制、跨账户访问、CloudFront OAI、VPC 端点限制等场景
tags: ["domain", "aws", "s3", "policy", "bucket-policy", "resource-policy", "security", "encryption"]
---

# AWS S3 桶策略模式

## 概述

本文档提供 S3 桶策略（Bucket Policy）的常见模式和完整示例。S3 桶策略是资源策略，直接附加到 S3 桶，定义谁可以访问桶及其对象。

**关键特点**：
- **推荐使用 `Principal: "*"` 配合 Condition**：避免 Principal 强校验导致的循环依赖问题（详见模式 9）
- 不同操作需要不同的 Resource ARN 格式：

| 操作类型 | 示例操作 | Resource ARN 格式 |
|----------|----------|-------------------|
| 桶级操作 | `s3:ListBucket`, `s3:GetBucketLocation`, `s3:GetBucketVersioning` | `arn:aws:s3:::bucket-name` |
| 对象级操作 | `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject` | `arn:aws:s3:::bucket-name/*` 或 `arn:aws:s3:::bucket-name/prefix/*` |
| 混合操作 | 同时包含桶和对象操作 | 需要同时指定两种 ARN |

**注意**：`s3:ListBucket` 是桶级操作，即使是列出对象，Resource 也必须是桶 ARN（不带 `/*`）

---

## 模式 1：强制 HTTPS 和服务端加密

**场景**：拒绝非 HTTPS 请求，强制所有上传使用服务端加密。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DenyInsecureTransport",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:*",
      "Resource": [
        "arn:aws:s3:::my-secure-bucket",
        "arn:aws:s3:::my-secure-bucket/*"
      ],
      "Condition": {
        "Bool": {
          "aws:SecureTransport": "false"
        }
      }
    },
    {
      "Sid": "DenyUnencryptedUploads",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:PutObject",
      "Resource": "arn:aws:s3:::my-secure-bucket/*",
      "Condition": {
        "Null": {
          "s3:x-amz-server-side-encryption": "true"
        }
      }
    },
    {
      "Sid": "DenyNonAES256Encryption",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:PutObject",
      "Resource": "arn:aws:s3:::my-secure-bucket/*",
      "Condition": {
        "StringNotEquals": {
          "s3:x-amz-server-side-encryption": ["AES256", "aws:kms"]
        }
      }
    }
  ]
}
```

**关键点**：
- `aws:SecureTransport: false` 拒绝 HTTP 请求
- `s3:x-amz-server-side-encryption` 检查加密头
- 使用 `Null` 条件检查头是否存在
- 同时指定桶和对象 ARN

---

## 模式 2：带条件的跨账户访问

**场景**：允许其他 AWS 账户访问桶，但限制来源和操作。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowCrossAccountRead",
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::111111111111:root",
          "arn:aws:iam::222222222222:role/DataAccessRole"
        ]
      },
      "Action": [
        "s3:GetObject",
        "s3:GetObjectVersion",
        "s3:GetObjectTagging"
      ],
      "Resource": "arn:aws:s3:::shared-data-bucket/*",
      "Condition": {
        "StringEquals": {
          "aws:PrincipalOrgID": "o-xxxxxxxxxx"
        }
      }
    },
    {
      "Sid": "AllowCrossAccountList",
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::111111111111:root",
          "arn:aws:iam::222222222222:role/DataAccessRole"
        ]
      },
      "Action": [
        "s3:ListBucket",
        "s3:GetBucketLocation"
      ],
      "Resource": "arn:aws:s3:::shared-data-bucket",
      "Condition": {
        "StringEquals": {
          "aws:PrincipalOrgID": "o-xxxxxxxxxx"
        },
        "StringLike": {
          "s3:prefix": ["shared/*", "public/*"]
        }
      }
    },
    {
      "Sid": "AllowCrossAccountWrite",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::222222222222:role/DataWriteRole"
      },
      "Action": [
        "s3:PutObject",
        "s3:PutObjectTagging"
      ],
      "Resource": "arn:aws:s3:::shared-data-bucket/uploads/*",
      "Condition": {
        "StringEquals": {
          "s3:x-amz-acl": "bucket-owner-full-control"
        }
      }
    }
  ]
}
```

**关键点**：
- 使用 `aws:PrincipalOrgID` 限制为同一组织
- 使用 `s3:prefix` 限制可列出的前缀
- 写入时要求 `bucket-owner-full-control` ACL
- 分别处理桶操作（ListBucket）和对象操作（GetObject）

---

## 模式 3：CloudFront 源访问身份 (OAI)

**场景**：只允许 CloudFront 分发访问 S3 桶，阻止直接访问。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowCloudFrontOAI",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::cloudfront:user/CloudFront Origin Access Identity E1234567890ABC"
      },
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::my-website-bucket/*"
    },
    {
      "Sid": "AllowCloudFrontOAC",
      "Effect": "Allow",
      "Principal": {
        "Service": "cloudfront.amazonaws.com"
      },
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::my-website-bucket/*",
      "Condition": {
        "StringEquals": {
          "AWS:SourceArn": "arn:aws:cloudfront::123456789012:distribution/E1234567890ABC"
        }
      }
    },
    {
      "Sid": "DenyDirectAccess",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::my-website-bucket/*",
      "Condition": {
        "StringNotLike": {
          "aws:Referer": [
            "https://www.example.com/*",
            "https://example.com/*"
          ]
        }
      }
    }
  ]
}
```

**关键点**：
- OAI 使用特殊的 CloudFront 用户 ARN
- OAC（Origin Access Control）使用 `cloudfront.amazonaws.com` 服务主体
- OAC 需要 `AWS:SourceArn` 条件验证分发
- 可选：使用 Referer 条件作为额外保护

---

## 模式 4：VPC 端点限制

**场景**：只允许通过特定 VPC 端点访问桶。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowAccessFromVPCEndpoint",
      "Effect": "Allow",
      "Principal": "*",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::internal-data-bucket",
        "arn:aws:s3:::internal-data-bucket/*"
      ],
      "Condition": {
        "StringEquals": {
          "aws:SourceVpce": "vpce-1234567890abcdef0"
        }
      }
    },
    {
      "Sid": "AllowAccessFromVPC",
      "Effect": "Allow",
      "Principal": "*",
      "Action": [
        "s3:GetObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::internal-data-bucket",
        "arn:aws:s3:::internal-data-bucket/*"
      ],
      "Condition": {
        "StringEquals": {
          "aws:SourceVpc": "vpc-0123456789abcdef0"
        }
      }
    },
    {
      "Sid": "DenyAccessFromOutsideVPC",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:*",
      "Resource": [
        "arn:aws:s3:::internal-data-bucket",
        "arn:aws:s3:::internal-data-bucket/*"
      ],
      "Condition": {
        "StringNotEquals": {
          "aws:SourceVpc": "vpc-0123456789abcdef0"
        }
      }
    }
  ]
}
```

**关键点**：
- `aws:SourceVpce` 限制为特定 VPC 端点
- `aws:SourceVpc` 限制为特定 VPC
- 使用显式 Deny 阻止 VPC 外部访问
- 适用于内部数据桶

---

## 模式 5：基于标签的访问控制 (ABAC)

**场景**：根据对象标签和用户标签控制访问。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowAccessToOwnDepartmentObjects",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"
      },
      "Action": [
        "s3:GetObject",
        "s3:GetObjectTagging"
      ],
      "Resource": "arn:aws:s3:::department-data-bucket/*",
      "Condition": {
        "StringEquals": {
          "s3:ExistingObjectTag/Department": "${aws:PrincipalTag/Department}"
        }
      }
    },
    {
      "Sid": "AllowPutObjectWithCorrectTags",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"
      },
      "Action": "s3:PutObject",
      "Resource": "arn:aws:s3:::department-data-bucket/*",
      "Condition": {
        "StringEquals": {
          "s3:RequestObjectTag/Department": "${aws:PrincipalTag/Department}",
          "s3:RequestObjectTag/Classification": ["public", "internal"]
        }
      }
    },
    {
      "Sid": "DenyAccessToConfidentialWithoutClearance",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::department-data-bucket/*",
      "Condition": {
        "StringEquals": {
          "s3:ExistingObjectTag/Classification": "confidential"
        },
        "StringNotEquals": {
          "aws:PrincipalTag/ClearanceLevel": "high"
        }
      }
    }
  ]
}
```

**关键点**：
- `s3:ExistingObjectTag` 检查现有对象的标签
- `s3:RequestObjectTag` 检查上传时请求的标签
- `${aws:PrincipalTag/xxx}` 引用调用者的标签
- 实现基于属性的访问控制 (ABAC)

---

## 模式 6：带时间条件的临时访问

**场景**：在特定时间窗口内允许访问。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowTemporaryAccess",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::111111111111:role/AuditRole"
      },
      "Action": [
        "s3:GetObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::audit-data-bucket",
        "arn:aws:s3:::audit-data-bucket/*"
      ],
      "Condition": {
        "DateGreaterThan": {
          "aws:CurrentTime": "2024-01-01T00:00:00Z"
        },
        "DateLessThan": {
          "aws:CurrentTime": "2024-01-31T23:59:59Z"
        }
      }
    },
    {
      "Sid": "AllowBusinessHoursOnly",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/BusinessRole"
      },
      "Action": [
        "s3:GetObject",
        "s3:PutObject"
      ],
      "Resource": "arn:aws:s3:::business-data-bucket/*",
      "Condition": {
        "DateGreaterThan": {
          "aws:CurrentTime": "2024-01-01T09:00:00Z"
        },
        "DateLessThan": {
          "aws:CurrentTime": "2024-12-31T18:00:00Z"
        }
      }
    }
  ]
}
```

**关键点**：
- 使用 `aws:CurrentTime` 进行时间限制
- 组合 `DateGreaterThan` 和 `DateLessThan`
- 适用于审计、合规检查等临时访问场景

---

## 模式 7：基于 IP 的限制

**场景**：只允许特定 IP 地址或范围访问。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowAccessFromCorporateNetwork",
      "Effect": "Allow",
      "Principal": "*",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::corporate-data-bucket",
        "arn:aws:s3:::corporate-data-bucket/*"
      ],
      "Condition": {
        "IpAddress": {
          "aws:SourceIp": [
            "192.168.1.0/24",
            "10.0.0.0/8",
            "203.0.113.0/24"
          ]
        }
      }
    },
    {
      "Sid": "DenyAccessFromBlockedIPs",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:*",
      "Resource": [
        "arn:aws:s3:::corporate-data-bucket",
        "arn:aws:s3:::corporate-data-bucket/*"
      ],
      "Condition": {
        "IpAddress": {
          "aws:SourceIp": [
            "192.168.100.0/24"
          ]
        }
      }
    },
    {
      "Sid": "DenyAccessFromOutsideCorporateNetwork",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:*",
      "Resource": [
        "arn:aws:s3:::corporate-data-bucket",
        "arn:aws:s3:::corporate-data-bucket/*"
      ],
      "Condition": {
        "NotIpAddress": {
          "aws:SourceIp": [
            "192.168.1.0/24",
            "10.0.0.0/8",
            "203.0.113.0/24"
          ]
        },
        "Bool": {
          "aws:ViaAWSService": "false"
        }
      }
    }
  ]
}
```

**关键点**：
- 使用 `IpAddress` 条件允许特定 IP
- 使用 `NotIpAddress` 拒绝其他 IP
- `aws:ViaAWSService` 排除 AWS 服务调用
- 支持 CIDR 格式

---

## 模式 8：复制目标桶策略

**场景**：允许 S3 复制服务写入目标桶。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowS3ReplicationFromSource",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::111111111111:role/S3ReplicationRole"
      },
      "Action": [
        "s3:ReplicateObject",
        "s3:ReplicateDelete",
        "s3:ReplicateTags",
        "s3:ObjectOwnerOverrideToBucketOwner"
      ],
      "Resource": "arn:aws:s3:::replication-destination-bucket/*"
    },
    {
      "Sid": "AllowS3ReplicationVersioning",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::111111111111:role/S3ReplicationRole"
      },
      "Action": [
        "s3:GetBucketVersioning",
        "s3:PutBucketVersioning"
      ],
      "Resource": "arn:aws:s3:::replication-destination-bucket"
    }
  ]
}
```

**关键点**：
- 允许源账户的复制角色写入
- 包含 `s3:ObjectOwnerOverrideToBucketOwner` 转移所有权
- 需要版本控制相关权限

---

## 模式 9：避免循环依赖的 IAM Role 授权（重要）

**场景**：授权 IAM Role 访问 S3 桶，但 Role 和 S3 桶在同一 Terraform 配置中创建，直接使用 Principal 会导致循环依赖。

### 问题说明

当 S3 桶策略直接引用 IAM Role ARN 作为 Principal 时，AWS 会对 Principal 进行**强校验**（验证 Role 是否存在）。如果 Role 和 S3 桶在同一 Terraform 配置中创建，会产生循环依赖：
- S3 桶策略需要 Role ARN → 依赖 Role
- Role 可能需要访问 S3 桶 → 依赖 S3 桶

### 解决方案：使用 Condition 替代 Principal

使用 `aws:PrincipalArn` 条件配合 `StringLike` 运算符，可以避免 Principal 强校验：

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowIAMRoleAccessWithoutCircularDependency",
      "Effect": "Allow",
      "Principal": "*",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::my-bucket",
        "arn:aws:s3:::my-bucket/*"
      ],
      "Condition": {
        "StringLike": {
          "aws:PrincipalArn": "arn:aws:iam::123456789012:role/MyAppRole*"
        }
      }
    }
  ]
}
```

### 对比：直接使用 Principal（会导致循环依赖）

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DirectPrincipalCausesCircularDependency",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/MyAppRole"
      },
      "Action": ["s3:GetObject", "s3:PutObject"],
      "Resource": "arn:aws:s3:::my-bucket/*"
    }
  ]
}
```

### 关键点

| 方式 | Principal | Condition | 是否强校验 | 循环依赖风险 |
|------|-----------|-----------|------------|--------------|
| 直接 Principal | `"AWS": "arn:aws:iam::xxx:role/xxx"` | 无 | **是** | **高** |
| Condition 方式 | `"*"` | `aws:PrincipalArn` + `StringLike` | **否** | **无** |

###  安全警告：必须指定账户 ID

**高风险行为**：在 `aws:PrincipalArn` 中使用通配符账户 ID（`*`）：
```json
// ❌ 危险：允许任何 AWS 账户的同名角色访问
"aws:PrincipalArn": "arn:aws:iam::*:role/lambda-execution-role"
```

**正确做法**：始终指定具体的账户 ID：
```json
// ✅ 安全：只允许指定账户的角色访问
"aws:PrincipalArn": "arn:aws:iam::123456789012:role/lambda-execution-role*"
```

**或者**：配合 `aws:PrincipalAccount` 条件限制账户：
```json
"Condition": {
  "StringLike": {
    "aws:PrincipalArn": "arn:aws:iam::*:role/lambda-execution-role*"
  },
  "StringEquals": {
    "aws:PrincipalAccount": "123456789012"
  }
}
```

### 推荐模式

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowSpecificRolePattern",
      "Effect": "Allow",
      "Principal": "*",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:ListBucket"],
      "Resource": [
        "arn:aws:s3:::${bucket_name}",
        "arn:aws:s3:::${bucket_name}/*"
      ],
      "Condition": {
        "StringLike": {
          "aws:PrincipalArn": [
            "arn:aws:iam::${account_id}:role/${role_name_prefix}*"
          ]
        },
        "StringEquals": {
          "aws:PrincipalAccount": "${account_id}"
        }
      }
    }
  ]
}
```

**安全增强**：
- 使用 `aws:PrincipalAccount` 限制为同一账户
- 使用 `StringLike` 配合前缀匹配，支持 Role 名称变化
- 可以组合 `aws:PrincipalOrgID` 限制为同一组织

---

## 最佳实践总结

| 实践 | 说明 |
|------|------|
| 强制 HTTPS | 使用 `aws:SecureTransport` 拒绝 HTTP |
| 强制加密 | 使用 `s3:x-amz-server-side-encryption` 检查加密 |
| 分离 ARN | 桶操作用桶 ARN，对象操作用对象 ARN |
| 组织限制 | 使用 `aws:PrincipalOrgID` 限制跨账户访问 |
| VPC 限制 | 使用 `aws:SourceVpc` 或 `aws:SourceVpce` |
| 显式 Deny | 对关键限制使用显式拒绝 |
| 标签控制 | 使用 `s3:ExistingObjectTag` 和 `s3:RequestObjectTag` |
| **避免循环依赖** | **使用 `aws:PrincipalArn` + `StringLike` 替代直接 Principal** |

---

## 常见错误

| 错误 | 问题 | 正确做法 |
|------|------|----------|
| 只用桶 ARN | GetObject 不生效 | 对象操作需要 `bucket/*` ARN |
| 忘记 Principal | 策略无效 | 资源策略必须有 Principal |
| 通配符 Principal | 公开访问风险 | 配合条件使用或指定具体主体 |
| 条件冲突 | 策略不生效 | 检查条件逻辑是否正确 |