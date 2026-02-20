---
name: aws_kms_policy_patterns
layer: domain
description: AWS KMS 密钥策略的常见模式和最佳实践，包括管理员/用户分离、服务特定访问、加密上下文、跨账户访问等场景
tags: ["domain", "aws", "kms", "policy", "key-policy", "resource-policy", "encryption", "security"]
---

# AWS KMS 密钥策略模式

## 概述

本文档提供 KMS 密钥策略（Key Policy）的常见模式和完整示例。KMS 密钥策略是资源策略，直接附加到 KMS 密钥，定义谁可以管理和使用密钥。

**关键特点**：
- KMS 密钥**必须**有密钥策略
- 密钥策略中的 `"Resource": "*"` 表示"此密钥"，不是所有密钥
- **必须**包含根账户访问权限，防止密钥锁定
- 应将密钥管理员与密钥用户分开

---

## 模式 1：完整的密钥策略（管理员 + 用户 + 授权）

**场景**：标准的 KMS 密钥策略，分离管理员、用户和服务授权。

```json
{
  "Version": "2012-10-17",
  "Id": "key-policy-1",
  "Statement": [
    {
      "Sid": "EnableRootAccountAccess",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"
      },
      "Action": "kms:*",
      "Resource": "*"
    },
    {
      "Sid": "AllowKeyAdministration",
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::123456789012:role/KMSAdminRole",
          "arn:aws:iam::123456789012:user/KMSAdmin"
        ]
      },
      "Action": [
        "kms:Create*",
        "kms:Describe*",
        "kms:Enable*",
        "kms:List*",
        "kms:Put*",
        "kms:Update*",
        "kms:Revoke*",
        "kms:Disable*",
        "kms:Get*",
        "kms:Delete*",
        "kms:TagResource",
        "kms:UntagResource",
        "kms:ScheduleKeyDeletion",
        "kms:CancelKeyDeletion"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowKeyUsage",
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::123456789012:role/ApplicationRole",
          "arn:aws:iam::123456789012:role/DataProcessingRole"
        ]
      },
      "Action": [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:ReEncrypt*",
        "kms:GenerateDataKey*",
        "kms:DescribeKey"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowAttachmentOfPersistentResources",
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::123456789012:role/ApplicationRole",
          "arn:aws:iam::123456789012:role/DataProcessingRole"
        ]
      },
      "Action": [
        "kms:CreateGrant",
        "kms:ListGrants",
        "kms:RevokeGrant"
      ],
      "Resource": "*",
      "Condition": {
        "Bool": {
          "kms:GrantIsForAWSResource": "true"
        }
      }
    }
  ]
}
```

**关键点**：
- **根账户访问**：防止密钥锁定，始终保留恢复能力
- **管理员权限**：管理密钥生命周期，不能使用密钥加解密
- **用户权限**：使用密钥加解密，不能管理密钥
- **授权权限**：允许 AWS 服务代表用户使用密钥

---

## 模式 2：服务特定访问（S3、RDS）

**场景**：限制密钥只能被特定 AWS 服务使用。

```json
{
  "Version": "2012-10-17",
  "Id": "key-policy-s3-rds",
  "Statement": [
    {
      "Sid": "EnableRootAccountAccess",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"
      },
      "Action": "kms:*",
      "Resource": "*"
    },
    {
      "Sid": "AllowS3ToUseKey",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/S3AccessRole"
      },
      "Action": [
        "kms:Decrypt",
        "kms:GenerateDataKey"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "kms:ViaService": "s3.us-east-1.amazonaws.com"
        },
        "StringLike": {
          "kms:EncryptionContext:aws:s3:arn": "arn:aws:s3:::my-encrypted-bucket/*"
        }
      }
    },
    {
      "Sid": "AllowRDSToUseKey",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/RDSAccessRole"
      },
      "Action": [
        "kms:Decrypt",
        "kms:GenerateDataKey*",
        "kms:CreateGrant",
        "kms:DescribeKey"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "kms:ViaService": "rds.us-east-1.amazonaws.com"
        },
        "Bool": {
          "kms:GrantIsForAWSResource": "true"
        }
      }
    },
    {
      "Sid": "AllowEBSToUseKey",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/EC2Role"
      },
      "Action": [
        "kms:Decrypt",
        "kms:GenerateDataKey*",
        "kms:CreateGrant",
        "kms:DescribeKey"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "kms:ViaService": "ec2.us-east-1.amazonaws.com"
        },
        "Bool": {
          "kms:GrantIsForAWSResource": "true"
        }
      }
    }
  ]
}
```

**关键点**：
- `kms:ViaService` 限制只能通过特定服务使用密钥
- `kms:EncryptionContext` 进一步限制加密上下文
- RDS 和 EBS 需要 `CreateGrant` 权限
- 服务格式：`service.region.amazonaws.com`

---

## 模式 3：加密上下文条件

**场景**：使用加密上下文增强安全性，确保数据只能在正确的上下文中解密。

```json
{
  "Version": "2012-10-17",
  "Id": "key-policy-encryption-context",
  "Statement": [
    {
      "Sid": "EnableRootAccountAccess",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"
      },
      "Action": "kms:*",
      "Resource": "*"
    },
    {
      "Sid": "AllowEncryptWithContext",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/DataEncryptionRole"
      },
      "Action": [
        "kms:Encrypt",
        "kms:GenerateDataKey*"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "kms:EncryptionContext:Environment": ["production", "staging"],
          "kms:EncryptionContext:Application": "my-app"
        }
      }
    },
    {
      "Sid": "AllowDecryptWithContext",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/DataDecryptionRole"
      },
      "Action": "kms:Decrypt",
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "kms:EncryptionContext:Environment": "production",
          "kms:EncryptionContext:Application": "my-app"
        }
      }
    },
    {
      "Sid": "AllowDecryptAnyContext",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/AdminDecryptRole"
      },
      "Action": "kms:Decrypt",
      "Resource": "*"
    }
  ]
}
```

**关键点**：
- `kms:EncryptionContext:key` 检查特定的加密上下文键值
- 加密和解密可以有不同的上下文要求
- 管理员角色可以不受上下文限制
- 加密上下文提供额外的访问控制层

---

## 模式 4：跨账户密钥访问

**场景**：允许其他 AWS 账户使用密钥。

```json
{
  "Version": "2012-10-17",
  "Id": "key-policy-cross-account",
  "Statement": [
    {
      "Sid": "EnableRootAccountAccess",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"
      },
      "Action": "kms:*",
      "Resource": "*"
    },
    {
      "Sid": "AllowKeyAdministration",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/KMSAdminRole"
      },
      "Action": [
        "kms:Create*",
        "kms:Describe*",
        "kms:Enable*",
        "kms:List*",
        "kms:Put*",
        "kms:Update*",
        "kms:Revoke*",
        "kms:Disable*",
        "kms:Get*",
        "kms:Delete*",
        "kms:TagResource",
        "kms:UntagResource",
        "kms:ScheduleKeyDeletion",
        "kms:CancelKeyDeletion"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowCrossAccountAccess",
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::111111111111:root",
          "arn:aws:iam::222222222222:role/CrossAccountRole"
        ]
      },
      "Action": [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:ReEncrypt*",
        "kms:GenerateDataKey*",
        "kms:DescribeKey"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:PrincipalOrgID": "o-xxxxxxxxxx"
        }
      }
    },
    {
      "Sid": "AllowCrossAccountGrant",
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::111111111111:root",
          "arn:aws:iam::222222222222:role/CrossAccountRole"
        ]
      },
      "Action": [
        "kms:CreateGrant",
        "kms:ListGrants",
        "kms:RevokeGrant"
      ],
      "Resource": "*",
      "Condition": {
        "Bool": {
          "kms:GrantIsForAWSResource": "true"
        },
        "StringEquals": {
          "aws:PrincipalOrgID": "o-xxxxxxxxxx"
        }
      }
    }
  ]
}
```

**关键点**：
- 使用 `aws:PrincipalOrgID` 限制为同一组织
- 跨账户访问需要密钥策略和 IAM 策略双重授权
- 授权权限允许 AWS 服务代表其他账户使用密钥
- 可以指定账户根或特定角色

---

## 模式 5：组织限制

**场景**：只允许组织内的账户使用密钥。

```json
{
  "Version": "2012-10-17",
  "Id": "key-policy-organization",
  "Statement": [
    {
      "Sid": "EnableRootAccountAccess",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"
      },
      "Action": "kms:*",
      "Resource": "*"
    },
    {
      "Sid": "AllowOrganizationAccess",
      "Effect": "Allow",
      "Principal": "*",
      "Action": [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:ReEncrypt*",
        "kms:GenerateDataKey*",
        "kms:DescribeKey"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:PrincipalOrgID": "o-xxxxxxxxxx"
        },
        "ForAnyValue:StringEquals": {
          "aws:PrincipalOrgPaths": [
            "o-xxxxxxxxxx/r-xxxx/ou-xxxx-xxxxxxxx/",
            "o-xxxxxxxxxx/r-xxxx/ou-xxxx-yyyyyyyy/"
          ]
        }
      }
    },
    {
      "Sid": "DenyOutsideOrganization",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "kms:*",
      "Resource": "*",
      "Condition": {
        "StringNotEquals": {
          "aws:PrincipalOrgID": "o-xxxxxxxxxx"
        },
        "Bool": {
          "aws:PrincipalIsAWSService": "false"
        }
      }
    }
  ]
}
```

**关键点**：
- `aws:PrincipalOrgID` 限制为组织
- `aws:PrincipalOrgPaths` 限制为特定 OU
- 显式 Deny 阻止组织外访问
- `aws:PrincipalIsAWSService` 排除 AWS 服务

---

## 模式 6：CloudWatch Logs 集成

**场景**：允许 CloudWatch Logs 使用密钥加密日志组。

```json
{
  "Version": "2012-10-17",
  "Id": "key-policy-cloudwatch-logs",
  "Statement": [
    {
      "Sid": "EnableRootAccountAccess",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"
      },
      "Action": "kms:*",
      "Resource": "*"
    },
    {
      "Sid": "AllowCloudWatchLogs",
      "Effect": "Allow",
      "Principal": {
        "Service": "logs.us-east-1.amazonaws.com"
      },
      "Action": [
        "kms:Encrypt*",
        "kms:Decrypt*",
        "kms:ReEncrypt*",
        "kms:GenerateDataKey*",
        "kms:Describe*"
      ],
      "Resource": "*",
      "Condition": {
        "ArnLike": {
          "kms:EncryptionContext:aws:logs:arn": "arn:aws:logs:us-east-1:123456789012:log-group:*"
        }
      }
    },
    {
      "Sid": "AllowCloudWatchLogsSpecificGroups",
      "Effect": "Allow",
      "Principal": {
        "Service": "logs.us-east-1.amazonaws.com"
      },
      "Action": [
        "kms:Encrypt*",
        "kms:Decrypt*",
        "kms:ReEncrypt*",
        "kms:GenerateDataKey*",
        "kms:Describe*"
      ],
      "Resource": "*",
      "Condition": {
        "ArnEquals": {
          "kms:EncryptionContext:aws:logs:arn": [
            "arn:aws:logs:us-east-1:123456789012:log-group:/aws/lambda/my-function",
            "arn:aws:logs:us-east-1:123456789012:log-group:/aws/ecs/my-cluster"
          ]
        }
      }
    }
  ]
}
```

**关键点**：
- CloudWatch Logs 使用服务主体 `logs.region.amazonaws.com`
- 使用 `kms:EncryptionContext:aws:logs:arn` 限制日志组
- `ArnLike` 支持通配符，`ArnEquals` 精确匹配
- 区域特定的服务主体

---

## 模式 7：Secrets Manager 集成

**场景**：允许 Secrets Manager 使用密钥加密密钥。

```json
{
  "Version": "2012-10-17",
  "Id": "key-policy-secrets-manager",
  "Statement": [
    {
      "Sid": "EnableRootAccountAccess",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"
      },
      "Action": "kms:*",
      "Resource": "*"
    },
    {
      "Sid": "AllowSecretsManagerAccess",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/SecretsAccessRole"
      },
      "Action": [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:ReEncrypt*",
        "kms:GenerateDataKey*",
        "kms:DescribeKey"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "kms:ViaService": "secretsmanager.us-east-1.amazonaws.com"
        },
        "StringLike": {
          "kms:EncryptionContext:SecretARN": "arn:aws:secretsmanager:us-east-1:123456789012:secret:prod/*"
        }
      }
    },
    {
      "Sid": "AllowSecretsManagerGrant",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/SecretsAccessRole"
      },
      "Action": [
        "kms:CreateGrant",
        "kms:ListGrants",
        "kms:RevokeGrant"
      ],
      "Resource": "*",
      "Condition": {
        "Bool": {
          "kms:GrantIsForAWSResource": "true"
        }
      }
    }
  ]
}
```

**关键点**：
- 使用 `kms:ViaService` 限制为 Secrets Manager
- 使用 `kms:EncryptionContext:SecretARN` 限制特定密钥
- 需要授权权限用于密钥轮换

---

## 最佳实践总结

| 实践 | 说明 |
|------|------|
| 保留根账户访问 | 防止密钥锁定，始终包含根账户权限 |
| 分离管理员和用户 | 管理员管理密钥，用户使用密钥 |
| 使用 ViaService | 限制密钥只能通过特定服务使用 |
| 加密上下文 | 使用加密上下文增强安全性 |
| 组织限制 | 使用 `aws:PrincipalOrgID` 限制跨账户访问 |
| 授权权限 | 允许 AWS 服务代表用户使用密钥 |

---

## 常见错误

| 错误 | 问题 | 正确做法 |
|------|------|----------|
| 忘记根账户 | 可能导致密钥锁定 | 始终包含根账户访问 |
| Resource 使用具体 ARN | 密钥策略中 Resource 应为 `*` | 使用 `"Resource": "*"` |
| 混淆管理和使用 | 权限过于宽泛 | 分离管理员和用户权限 |
| 忘记 ViaService | 密钥可被任何服务使用 | 添加 `kms:ViaService` 条件 |
| 缺少授权权限 | AWS 服务无法使用密钥 | 添加 `CreateGrant` 权限 |