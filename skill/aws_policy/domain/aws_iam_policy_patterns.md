---
name: aws_iam_policy_patterns
layer: domain
description: AWS IAM 身份策略的常见模式和最佳实践，包括 EC2、S3、Lambda、RDS、CloudFormation 等服务的策略示例
tags: ["domain", "aws", "iam", "policy", "identity-policy", "ec2", "s3", "lambda", "rds", "cloudformation"]
---

# AWS IAM 策略模式

## 概述

本文档提供 IAM 身份策略（Identity-based Policies）的常见模式和完整示例。IAM 策略附加到用户、组或角色，定义他们可以执行的操作。

---

## 模式 1：EC2 实例管理（限制实例类型和标签）

**场景**：允许用户管理 EC2 实例，但限制实例类型和要求标签。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowEC2DescribeActions",
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances",
        "ec2:DescribeImages",
        "ec2:DescribeKeyPairs",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeSubnets",
        "ec2:DescribeVpcs"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowEC2RunInstancesWithRestrictions",
      "Effect": "Allow",
      "Action": "ec2:RunInstances",
      "Resource": [
        "arn:aws:ec2:*:*:instance/*",
        "arn:aws:ec2:*:*:volume/*",
        "arn:aws:ec2:*:*:network-interface/*"
      ],
      "Condition": {
        "StringEquals": {
          "ec2:InstanceType": [
            "t3.micro",
            "t3.small",
            "t3.medium"
          ]
        },
        "ForAllValues:StringEquals": {
          "aws:TagKeys": ["Environment", "Owner", "Project"]
        }
      }
    },
    {
      "Sid": "AllowEC2RunInstancesResources",
      "Effect": "Allow",
      "Action": "ec2:RunInstances",
      "Resource": [
        "arn:aws:ec2:*:*:subnet/*",
        "arn:aws:ec2:*:*:security-group/*",
        "arn:aws:ec2:*:*:key-pair/*",
        "arn:aws:ec2:*::image/*"
      ]
    },
    {
      "Sid": "AllowEC2ManageOwnInstances",
      "Effect": "Allow",
      "Action": [
        "ec2:StartInstances",
        "ec2:StopInstances",
        "ec2:RebootInstances",
        "ec2:TerminateInstances"
      ],
      "Resource": "arn:aws:ec2:*:*:instance/*",
      "Condition": {
        "StringEquals": {
          "ec2:ResourceTag/Owner": "${aws:username}"
        }
      }
    },
    {
      "Sid": "AllowCreateTags",
      "Effect": "Allow",
      "Action": "ec2:CreateTags",
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "ec2:CreateAction": "RunInstances"
        }
      }
    }
  ]
}
```

**关键点**：
- 使用 `ec2:InstanceType` 条件限制实例类型
- 使用 `aws:TagKeys` 强制要求标签
- 使用 `ec2:ResourceTag/Owner` 限制只能管理自己的实例

---

## 模式 2：S3 访问 + MFA 要求

**场景**：允许读取 S3，但写入操作需要 MFA。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowS3ListBuckets",
      "Effect": "Allow",
      "Action": [
        "s3:ListAllMyBuckets",
        "s3:GetBucketLocation"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowS3ReadAccess",
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:GetObjectVersion",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::company-data-bucket",
        "arn:aws:s3:::company-data-bucket/*"
      ]
    },
    {
      "Sid": "RequireMFAForWrite",
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:DeleteObjectVersion"
      ],
      "Resource": "arn:aws:s3:::company-data-bucket/*",
      "Condition": {
        "Bool": {
          "aws:MultiFactorAuthPresent": "true"
        }
      }
    }
  ]
}
```

**关键点**：
- 读取操作无需 MFA
- 写入和删除操作需要 `aws:MultiFactorAuthPresent` 为 true
- 分别指定桶 ARN（用于 ListBucket）和对象 ARN（用于 GetObject）

---

## 模式 3：Lambda 函数管理 + PassRole

**场景**：允许创建和管理 Lambda 函数，包括传递执行角色。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowLambdaReadActions",
      "Effect": "Allow",
      "Action": [
        "lambda:GetFunction",
        "lambda:GetFunctionConfiguration",
        "lambda:ListFunctions",
        "lambda:ListVersionsByFunction",
        "lambda:GetPolicy"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowLambdaManageActions",
      "Effect": "Allow",
      "Action": [
        "lambda:CreateFunction",
        "lambda:UpdateFunctionCode",
        "lambda:UpdateFunctionConfiguration",
        "lambda:DeleteFunction",
        "lambda:PublishVersion",
        "lambda:CreateAlias",
        "lambda:UpdateAlias",
        "lambda:DeleteAlias",
        "lambda:AddPermission",
        "lambda:RemovePermission"
      ],
      "Resource": "arn:aws:lambda:*:123456789012:function:*",
      "Condition": {
        "StringEquals": {
          "aws:RequestTag/Environment": ["dev", "staging"]
        }
      }
    },
    {
      "Sid": "AllowLambdaInvoke",
      "Effect": "Allow",
      "Action": "lambda:InvokeFunction",
      "Resource": "arn:aws:lambda:*:123456789012:function:*"
    },
    {
      "Sid": "AllowPassRoleToLambda",
      "Effect": "Allow",
      "Action": "iam:PassRole",
      "Resource": "arn:aws:iam::123456789012:role/lambda-execution-*",
      "Condition": {
        "StringEquals": {
          "iam:PassedToService": "lambda.amazonaws.com"
        }
      }
    },
    {
      "Sid": "AllowCloudWatchLogsForLambda",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents",
        "logs:DescribeLogGroups",
        "logs:DescribeLogStreams"
      ],
      "Resource": "arn:aws:logs:*:123456789012:log-group:/aws/lambda/*"
    }
  ]
}
```

**关键点**：
- `iam:PassRole` 允许将角色传递给 Lambda
- 使用 `iam:PassedToService` 限制只能传递给 Lambda 服务
- 角色名称使用前缀限制（`lambda-execution-*`）

---

## 模式 4：RDS 数据库访问

**场景**：允许管理 RDS 实例和连接数据库。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowRDSDescribeActions",
      "Effect": "Allow",
      "Action": [
        "rds:DescribeDBInstances",
        "rds:DescribeDBClusters",
        "rds:DescribeDBSnapshots",
        "rds:DescribeDBSubnetGroups",
        "rds:DescribeDBParameterGroups",
        "rds:ListTagsForResource"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowRDSManageDevInstances",
      "Effect": "Allow",
      "Action": [
        "rds:CreateDBInstance",
        "rds:DeleteDBInstance",
        "rds:ModifyDBInstance",
        "rds:RebootDBInstance",
        "rds:StartDBInstance",
        "rds:StopDBInstance"
      ],
      "Resource": "arn:aws:rds:*:123456789012:db:dev-*",
      "Condition": {
        "StringEquals": {
          "rds:DatabaseClass": [
            "db.t3.micro",
            "db.t3.small",
            "db.t3.medium"
          ]
        }
      }
    },
    {
      "Sid": "AllowRDSConnect",
      "Effect": "Allow",
      "Action": "rds-db:connect",
      "Resource": "arn:aws:rds-db:*:123456789012:dbuser:*/developer"
    },
    {
      "Sid": "AllowRDSCreateSnapshot",
      "Effect": "Allow",
      "Action": [
        "rds:CreateDBSnapshot",
        "rds:DeleteDBSnapshot"
      ],
      "Resource": [
        "arn:aws:rds:*:123456789012:db:dev-*",
        "arn:aws:rds:*:123456789012:snapshot:dev-*"
      ]
    }
  ]
}
```

**关键点**：
- 使用 `rds:DatabaseClass` 限制实例类型
- 使用 `rds-db:connect` 允许 IAM 数据库认证
- 资源名称前缀限制（`dev-*`）

---

## 模式 5：CloudFormation 栈管理

**场景**：允许创建和管理 CloudFormation 栈。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowCloudFormationReadActions",
      "Effect": "Allow",
      "Action": [
        "cloudformation:DescribeStacks",
        "cloudformation:DescribeStackEvents",
        "cloudformation:DescribeStackResources",
        "cloudformation:GetTemplate",
        "cloudformation:ListStacks",
        "cloudformation:ValidateTemplate"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowCloudFormationManageStacks",
      "Effect": "Allow",
      "Action": [
        "cloudformation:CreateStack",
        "cloudformation:UpdateStack",
        "cloudformation:DeleteStack",
        "cloudformation:CreateChangeSet",
        "cloudformation:ExecuteChangeSet",
        "cloudformation:DeleteChangeSet"
      ],
      "Resource": "arn:aws:cloudformation:*:123456789012:stack/dev-*/*",
      "Condition": {
        "StringEquals": {
          "aws:RequestTag/Environment": "dev"
        }
      }
    },
    {
      "Sid": "AllowPassRoleToCloudFormation",
      "Effect": "Allow",
      "Action": "iam:PassRole",
      "Resource": "arn:aws:iam::123456789012:role/cloudformation-*",
      "Condition": {
        "StringEquals": {
          "iam:PassedToService": "cloudformation.amazonaws.com"
        }
      }
    },
    {
      "Sid": "AllowTaggingStacks",
      "Effect": "Allow",
      "Action": [
        "cloudformation:TagResource",
        "cloudformation:UntagResource"
      ],
      "Resource": "arn:aws:cloudformation:*:123456789012:stack/dev-*/*"
    }
  ]
}
```

**关键点**：
- 栈名称前缀限制（`dev-*`）
- 需要 `iam:PassRole` 允许 CloudFormation 使用服务角色
- 使用 ChangeSet 进行安全的栈更新

---

## 模式 6：DynamoDB 表访问

**场景**：允许对特定 DynamoDB 表进行读写操作。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowDynamoDBDescribe",
      "Effect": "Allow",
      "Action": [
        "dynamodb:DescribeTable",
        "dynamodb:DescribeTimeToLive",
        "dynamodb:ListTables"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowDynamoDBReadWrite",
      "Effect": "Allow",
      "Action": [
        "dynamodb:GetItem",
        "dynamodb:BatchGetItem",
        "dynamodb:Query",
        "dynamodb:Scan",
        "dynamodb:PutItem",
        "dynamodb:UpdateItem",
        "dynamodb:DeleteItem",
        "dynamodb:BatchWriteItem"
      ],
      "Resource": [
        "arn:aws:dynamodb:*:123456789012:table/app-*",
        "arn:aws:dynamodb:*:123456789012:table/app-*/index/*"
      ]
    },
    {
      "Sid": "AllowDynamoDBStreams",
      "Effect": "Allow",
      "Action": [
        "dynamodb:DescribeStream",
        "dynamodb:GetRecords",
        "dynamodb:GetShardIterator",
        "dynamodb:ListStreams"
      ],
      "Resource": "arn:aws:dynamodb:*:123456789012:table/app-*/stream/*"
    }
  ]
}
```

**关键点**：
- 分别指定表、索引和流的 ARN
- 表名称前缀限制（`app-*`）
- 包含 GSI/LSI 索引的访问权限

---

## 模式 7：只读访问 + 时间限制

**场景**：临时只读访问，限制在特定时间段内有效。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowReadOnlyAccessWithTimeLimit",
      "Effect": "Allow",
      "Action": [
        "ec2:Describe*",
        "s3:GetObject",
        "s3:ListBucket",
        "rds:Describe*",
        "lambda:GetFunction",
        "lambda:ListFunctions",
        "dynamodb:GetItem",
        "dynamodb:Query",
        "dynamodb:Scan",
        "dynamodb:DescribeTable"
      ],
      "Resource": "*",
      "Condition": {
        "DateGreaterThan": {
          "aws:CurrentTime": "2024-01-01T00:00:00Z"
        },
        "DateLessThan": {
          "aws:CurrentTime": "2024-12-31T23:59:59Z"
        }
      }
    }
  ]
}
```

**关键点**：
- 使用 `aws:CurrentTime` 条件限制访问时间
- 组合 `DateGreaterThan` 和 `DateLessThan` 定义时间窗口
- 适用于临时审计或合规检查

---

## 模式 8：跨账户角色假设

**场景**：允许用户假设其他账户中的角色。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowAssumeRoleInTargetAccounts",
      "Effect": "Allow",
      "Action": "sts:AssumeRole",
      "Resource": [
        "arn:aws:iam::111111111111:role/CrossAccountReadOnly",
        "arn:aws:iam::222222222222:role/CrossAccountReadOnly"
      ],
      "Condition": {
        "Bool": {
          "aws:MultiFactorAuthPresent": "true"
        }
      }
    },
    {
      "Sid": "AllowAssumeRoleWithExternalId",
      "Effect": "Allow",
      "Action": "sts:AssumeRole",
      "Resource": "arn:aws:iam::333333333333:role/ThirdPartyAccess",
      "Condition": {
        "StringEquals": {
          "sts:ExternalId": "unique-external-id-12345"
        }
      }
    }
  ]
}
```

**关键点**：
- 明确指定可以假设的角色 ARN
- 使用 MFA 条件增强安全性
- 对第三方访问使用 `sts:ExternalId`

---

## 模式 9：成本和账单访问

**场景**：允许查看账单和成本信息。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowViewBilling",
      "Effect": "Allow",
      "Action": [
        "aws-portal:ViewBilling",
        "aws-portal:ViewUsage",
        "aws-portal:ViewPaymentMethods"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowCostExplorer",
      "Effect": "Allow",
      "Action": [
        "ce:GetCostAndUsage",
        "ce:GetCostForecast",
        "ce:GetReservationUtilization",
        "ce:GetSavingsPlansUtilization",
        "ce:GetDimensionValues",
        "ce:GetTags"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowBudgets",
      "Effect": "Allow",
      "Action": [
        "budgets:ViewBudget",
        "budgets:DescribeBudgets",
        "budgets:DescribeBudgetPerformanceHistory"
      ],
      "Resource": "arn:aws:budgets::123456789012:budget/*"
    },
    {
      "Sid": "DenyModifyBilling",
      "Effect": "Deny",
      "Action": [
        "aws-portal:ModifyBilling",
        "aws-portal:ModifyPaymentMethods"
      ],
      "Resource": "*"
    }
  ]
}
```

**关键点**：
- 分离查看和修改权限
- 使用显式 Deny 阻止修改账单设置
- 包含 Cost Explorer 和 Budgets 访问

---

## 模式 10：开发者访问 + 标签强制

**场景**：开发者可以创建资源，但必须添加特定标签。

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowDescribeActions",
      "Effect": "Allow",
      "Action": [
        "ec2:Describe*",
        "s3:ListAllMyBuckets",
        "lambda:ListFunctions",
        "rds:Describe*"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowCreateResourcesWithRequiredTags",
      "Effect": "Allow",
      "Action": [
        "ec2:RunInstances",
        "ec2:CreateVolume",
        "s3:CreateBucket",
        "lambda:CreateFunction",
        "rds:CreateDBInstance"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:RequestTag/Environment": ["dev", "staging", "prod"],
          "aws:RequestTag/CostCenter": "${aws:PrincipalTag/CostCenter}"
        },
        "ForAllValues:StringEquals": {
          "aws:TagKeys": ["Environment", "Owner", "CostCenter", "Project"]
        }
      }
    },
    {
      "Sid": "AllowManageOwnResources",
      "Effect": "Allow",
      "Action": [
        "ec2:StartInstances",
        "ec2:StopInstances",
        "ec2:TerminateInstances",
        "lambda:UpdateFunctionCode",
        "lambda:DeleteFunction"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:ResourceTag/Owner": "${aws:username}"
        }
      }
    },
    {
      "Sid": "DenyTagRemoval",
      "Effect": "Deny",
      "Action": [
        "ec2:DeleteTags",
        "s3:DeleteBucketTagging",
        "lambda:UntagResource",
        "rds:RemoveTagsFromResource"
      ],
      "Resource": "*",
      "Condition": {
        "ForAnyValue:StringEquals": {
          "aws:TagKeys": ["Environment", "Owner", "CostCenter"]
        }
      }
    }
  ]
}
```

**关键点**：
- 使用 `aws:RequestTag` 强制创建时必须有标签
- 使用 `aws:ResourceTag/Owner` 限制只能管理自己的资源
- 使用 `${aws:PrincipalTag/CostCenter}` 继承用户的成本中心标签
- 显式 Deny 阻止删除关键标签

---

## 最佳实践总结

| 实践 | 说明 |
|------|------|
| 使用条件键 | 限制实例类型、强制标签、要求 MFA |
| 资源前缀 | 使用命名约定（如 `dev-*`）限制资源范围 |
| 分离读写 | 读取操作和写入操作分开授权 |
| PassRole 限制 | 限制可以传递的角色和目标服务 |
| 显式 Deny | 对关键限制使用显式拒绝 |
| 标签强制 | 创建资源时强制要求标签 |
| 时间限制 | 对临时访问使用时间条件 |