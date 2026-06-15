---
name: cmdb_resource_types
layer: domain
description: CMDB 资源类型映射表
tags: ["cmdb", "resource_type", "mapping", "terraform"]
<!-- 该部分内容只是为了说明skill用途以及作用域,不要复制到skill正文里 -->
---

## CMDB 资源类型映射

### AWS 资源类型
| 用户描述 | 资源类型 | 字段名 |
|---------|---------|--------|
| VPC | aws_vpc | vpc_id |
| 子网 | aws_subnet | subnet_id / subnet_ids |
| 安全组 | aws_security_group | security_group_ids |
| AMI | aws_ami | ami_id |
| IAM 角色 | aws_iam_role | iam_role_arn |
| IAM 策略 | aws_iam_policy | policy_arn |
| KMS 密钥 | aws_kms_key | kms_key_id |
| S3 存储桶 | aws_s3_bucket | bucket_name |
| RDS 实例 | aws_db_instance | db_instance_id |
| EKS 集群 | aws_eks_cluster | cluster_name |

### 关键词识别
- "exchange vpc" → 搜索名称包含 "exchange" 的 VPC
- "生产环境子网" → 搜索标签包含 "production" 的子网
- "东京区域" → 过滤 ap-northeast-1 区域的资源
