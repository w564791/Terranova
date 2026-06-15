---
name: infrastructure_risk_baseline
layer: foundation
description: 基础设施变更风险基线规则，定义 high 和 critical 级别的风险判定标准
tags: ["foundation", "security", "risk", "baseline", "compliance"]
priority: 100
<!-- 该部分内容只是为了说明skill用途以及作用域,不要复制到skill正文里 -->
---

# 基础设施变更风险基线

定义 Terraform 变更中 high 和 critical 级别的风险判定规则。所有变更分析必须参照此基线评估风险。

## Critical 风险（必须阻止或人工审批）

### 网络暴露
- 安全组 inbound 规则 CIDR 包含 `0.0.0.0/0` 或 `::/0`，且端口为：
  - 数据库端口：3306, 5432, 1521, 1433, 27017, 6379, 9200, 9300, 5984, 8529
  - 管理端口：22, 3389, 5900, 5985, 5986
  - 全端口：-1 或 0-65535
  - 任何端口范围缩小的行为
- 安全组 inbound 从限定 IP 变更为 `0.0.0.0/0`
- 网络 ACL 允许全端口入站
- 源地址更改

### 计算资源
- 创建/修改带公网访问的资源
  - 包含且不仅包含ec2,loadblance,apigateway等

### 资源删除
- 删除 VPC 或子网（且有依赖资源）
- 删除 RDS 实例（且未启用 deletion_protection）
- 删除 S3 存储桶（且含数据或未设置 force_destroy）
- 批量删除超过 10 个资源

### 权限变更
- IAM Policy 新增 `*:*` 权限
- IAM Policy 新增 `iam:*`, `sts:AssumeRole` 到不受信实体
- 移除 MFA 条件约束
- S3 Bucket Policy 允许公开访问（Principal 为 `*`）
- KMS Key Policy 授权给外部账号

### 加密与数据保护
- 禁用 RDS/S3/EBS 加密
- 移除 CloudTrail 日志记录
- 禁用 S3 版本控制（已有数据的存储桶）

### 端口收窄（需查询 CMDB）
- 安全组端口范围收窄且有依赖方，依赖方服务端口不在新范围内

## High 风险（需关注，建议审批）

### 网络变更
- 安全组 inbound 开放非标准端口到 `0.0.0.0/0`（非 80/443）
- 安全组规则被修改但无法确认影响范围
- 路由表变更（可能影响流量路径）
- NAT Gateway 或 Internet Gateway 变更

### 资源删除
- 删除安全组（无论是否有依赖方 — CMDB 可能不完整，删除安全组本身就是高风险操作）
- 删除 IAM Role/Policy（无论是否被引用）
- 删除 ELB/ALB/NLB（可能影响服务可用性）
- 删除超过 5 个资源

### 权限变更
- IAM Policy 新增宽泛权限（如 `s3:*`, `ec2:*`）
- 修改信任策略（Trust Policy）
- 跨账号资源共享变更

### 配置变更
- RDS 实例类型降级（可能影响性能）
- EC2 实例类型变更（需要重启）
- Auto Scaling 最小/最大值变更
- 修改 EBS 卷类型或大小（可能需要停机）

## 风险等级判定优先级

1. 先检查是否命中 Critical 规则 → 命中即 critical
2. 再检查是否命中 High 规则 → 命中即 high
3. 有依赖方但无破坏性变更 → medium
4. 仅新增资源且无安全风险 → low
