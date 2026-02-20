---
name: aws_secrets_manager_policy_patterns
layer: domain
description: AWS Secrets Manager 资源策略的常见模式和最佳实践，包括版本控制、跨账户访问、Lambda 轮换、区域限制等场景
tags: ["domain", "aws", "secrets-manager", "policy", "resource-policy", "security", "secrets"]
---

# AWS Secrets Manager 策略模式

## 概述

本文档提供 Secrets Manager 资源策略的常见模式和完整示例。Secrets Manager 资源策略直接附加到密钥，定义谁可以访问密钥。

**关键特点**：
- 资源策略是可选的（与 KMS 不同）
- 可以与 IAM 策略配合使用
- 支持版本控制访问
- 支持跨账户访问

---

## 模式 1：基本资源策略

**场景**：允许特定角色访问密钥。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowReadSecret",
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::123456789012:role/ApplicationRole",
          "arn:aws:iam::123456789012:role/LambdaExecutionRole"
        ]
      },
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowListSecrets",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/ApplicationRole"
      },
      "Action": [
        "secretsmanager:ListSecrets",
        "secretsmanager:ListSecretVersionIds"
      ],
      "Resource": "*"
    },
    {
      "Sid": "DenyUnauthorizedAccess",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "secretsmanager:*",
      "Resource": "*",
      "Condition": {
        "StringNotEquals": {
          "aws:PrincipalArn": [
            "arn:aws:iam::123456789012:role/ApplicationRole",
            "arn:aws:iam::123456789012:role/LambdaExecutionRole",
            "arn:aws:iam::123456789012:role/SecretsAdminRole"
          ]
        }
      }
    }
  ]
}
```

**关键点**：
- `GetSecretValue` 是读取密钥值的核心权限
- `DescribeSecret` 获取密钥元数据
- 使用显式 Deny 限制未授权访问
- Resource 为 `*` 表示此密钥

---

## 模式 2：跨账户访问

**场景**：允许其他 AWS 账户访问密钥。

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
          "arn:aws:iam::222222222222:role/CrossAccountRole"
        ]
      },
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:PrincipalOrgID": "o-xxxxxxxxxx"
        }
      }
    },
    {
      "Sid": "AllowCrossAccountDescribe",
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::111111111111:root",
          "arn:aws:iam::222222222222:role/CrossAccountRole"
        ]
      },
      "Action": [
        "secretsmanager:ListSecretVersionIds",
        "secretsmanager:GetResourcePolicy"
      ],
      "Resource": "*",
      "Condition": {
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
- 跨账户访问需要资源策略和 IAM 策略双重授权
- 可以指定账户根或特定角色
- 如果密钥使用 CMK 加密，还需要 KMS 权限

---

## 模式 3：版本特定访问控制

**场景**：控制对密钥不同版本的访问。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowCurrentVersionOnly",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/ProductionRole"
      },
      "Action": "secretsmanager:GetSecretValue",
      "Resource": "*",
      "Condition": {
        "ForAnyValue:StringEquals": {
          "secretsmanager:VersionStage": "AWSCURRENT"
        }
      }
    },
    {
      "Sid": "AllowPreviousVersionForRollback",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/OpsRole"
      },
      "Action": "secretsmanager:GetSecretValue",
      "Resource": "*",
      "Condition": {
        "ForAnyValue:StringEquals": {
          "secretsmanager:VersionStage": ["AWSCURRENT", "AWSPREVIOUS"]
        }
      }
    },
    {
      "Sid": "AllowAllVersionsForAdmin",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/SecretsAdminRole"
      },
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:ListSecretVersionIds",
        "secretsmanager:UpdateSecretVersionStage"
      ],
      "Resource": "*"
    },
    {
      "Sid": "DenyPendingVersionAccess",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "secretsmanager:GetSecretValue",
      "Resource": "*",
      "Condition": {
        "ForAnyValue:StringEquals": {
          "secretsmanager:VersionStage": "AWSPENDING"
        },
        "StringNotEquals": {
          "aws:PrincipalArn": [
            "arn:aws:iam::123456789012:role/RotationLambdaRole",
            "arn:aws:iam::123456789012:role/SecretsAdminRole"
          ]
        }
      }
    }
  ]
}
```

**关键点**：
- `secretsmanager:VersionStage` 控制版本访问
- `AWSCURRENT` - 当前活动版本
- `AWSPREVIOUS` - 上一个版本（用于回滚）
- `AWSPENDING` - 轮换中的新版本
- 生产环境通常只需要 AWSCURRENT

---

## 模式 4：区域限制

**场景**：限制只能从特定区域访问密钥。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowAccessFromSpecificRegions",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/ApplicationRole"
      },
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:RequestedRegion": ["us-east-1", "us-west-2"]
        }
      }
    },
    {
      "Sid": "DenyAccessFromOtherRegions",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "secretsmanager:*",
      "Resource": "*",
      "Condition": {
        "StringNotEquals": {
          "aws:RequestedRegion": ["us-east-1", "us-west-2"]
        }
      }
    }
  ]
}
```

**关键点**：
- `aws:RequestedRegion` 限制请求来源区域
- 适用于多区域部署的合规要求
- 使用显式 Deny 阻止其他区域访问

---

## 模式 5：基于标签的访问控制

**场景**：根据密钥标签和用户标签控制访问。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowAccessToOwnEnvironmentSecrets",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"
      },
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "secretsmanager:ResourceTag/Environment": "${aws:PrincipalTag/Environment}"
        }
      }
    },
    {
      "Sid": "AllowAccessToTeamSecrets",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"
      },
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "secretsmanager:ResourceTag/Team": "${aws:PrincipalTag/Team}"
        }
      }
    },
    {
      "Sid": "DenyAccessToProductionWithoutClearance",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "secretsmanager:GetSecretValue",
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "secretsmanager:ResourceTag/Environment": "production"
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
- `secretsmanager:ResourceTag/key` 检查密钥标签
- `${aws:PrincipalTag/key}` 引用调用者标签
- 实现基于属性的访问控制 (ABAC)
- 适用于多团队、多环境场景

---

## 模式 6：Lambda 轮换函数权限

**场景**：允许 Lambda 函数执行密钥轮换。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowRotationLambda",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/SecretsRotationLambdaRole"
      },
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret",
        "secretsmanager:PutSecretValue",
        "secretsmanager:UpdateSecretVersionStage"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowSecretsManagerToInvokeLambda",
      "Effect": "Allow",
      "Principal": {
        "Service": "secretsmanager.amazonaws.com"
      },
      "Action": "secretsmanager:RotateSecret",
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:SourceAccount": "123456789012"
        }
      }
    },
    {
      "Sid": "AllowApplicationRead",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/ApplicationRole"
      },
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": "*",
      "Condition": {
        "ForAnyValue:StringEquals": {
          "secretsmanager:VersionStage": "AWSCURRENT"
        }
      }
    }
  ]
}
```

**关键点**：
- 轮换 Lambda 需要 `PutSecretValue` 和 `UpdateSecretVersionStage`
- 使用 `aws:SourceAccount` 验证服务调用来源
- 应用程序只能访问 AWSCURRENT 版本
- 轮换过程中 AWSPENDING 版本只对 Lambda 可见

---

## 模式 7：VPC 端点限制

**场景**：只允许通过 VPC 端点访问密钥。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowAccessFromVPCEndpoint",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/ApplicationRole"
      },
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:SourceVpce": "vpce-1234567890abcdef0"
        }
      }
    },
    {
      "Sid": "DenyAccessFromOutsideVPC",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "secretsmanager:*",
      "Resource": "*",
      "Condition": {
        "StringNotEquals": {
          "aws:SourceVpce": "vpce-1234567890abcdef0"
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
- `aws:SourceVpce` 限制为特定 VPC 端点
- `aws:ViaAWSService` 排除 AWS 服务调用
- 适用于高安全性要求的内部密钥

---

## 模式 8：时间限制访问

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
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": "*",
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
        "AWS": "arn:aws:iam::123456789012:role/SupportRole"
      },
      "Action": [
        "secretsmanager:GetSecretValue"
      ],
      "Resource": "*",
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
- 适用于审计、临时访问场景
- 组合 `DateGreaterThan` 和 `DateLessThan`

---

## 模式 9：IP 地址限制

**场景**：只允许特定 IP 地址访问密钥。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowAccessFromCorporateNetwork",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:role/AdminRole"
      },
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret",
        "secretsmanager:UpdateSecret"
      ],
      "Resource": "*",
      "Condition": {
        "IpAddress": {
          "aws:SourceIp": [
            "192.168.1.0/24",
            "10.0.0.0/8"
          ]
        }
      }
    },
    {
      "Sid": "DenyAccessFromOutsideCorporateNetwork",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "secretsmanager:*",
      "Resource": "*",
      "Condition": {
        "NotIpAddress": {
          "aws:SourceIp": [
            "192.168.1.0/24",
            "10.0.0.0/8"
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
- 使用 `IpAddress` 和 `NotIpAddress` 条件
- `aws:ViaAWSService` 排除 AWS 服务调用
- 支持 CIDR 格式

---

## 最佳实践总结

| 实践 | 说明 |
|------|------|
| 最小权限 | 只授予 `GetSecretValue`，不授予管理权限 |
| 版本控制 | 生产环境只允许访问 AWSCURRENT |
| 组织限制 | 使用 `aws:PrincipalOrgID` 限制跨账户访问 |
| VPC 限制 | 敏感密钥使用 VPC 端点限制 |
| 标签控制 | 使用 ABAC 实现细粒度访问控制 |
| 轮换权限 | 轮换 Lambda 需要特殊权限 |

---

## 常见错误

| 错误 | 问题 | 正确做法 |
|------|------|----------|
| 忘记 KMS 权限 | 使用 CMK 加密时无法解密 | 同时授予 KMS 解密权限 |
| 版本控制不当 | 应用访问到 AWSPENDING | 限制为 AWSCURRENT |
| 跨账户缺少 IAM | 只有资源策略不够 | 需要双重授权 |
| 忘记 ViaAWSService | 阻止了 AWS 服务调用 | 添加排除条件 |