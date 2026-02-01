---
name: aws_resource_tagging
layer: foundation
description: AWS资源标签规范，定义所有AWS资源必须遵循的标签命名规则和要求
tags: ["foundation", "aws", "tagging", "compliance", "governance", "resource-management"]
priority: 50
---

# AWS 资源标签规范

## 概述
本规范定义了所有 AWS 资源必须遵循的标签命名规则。生成资源配置时，**必须按照本规范添加标签**。

---

## 快速参考：资源类型标签清单

| 资源类型 | 全局必须 | 资源专属必须 |
|----------|----------|--------------|
| **EC2** | Name, business-line, managed-by, managed-by-terraform | component, env, cluster-name, backup-enabled, owner, created-by, service-name, group-name |
| **RDS** | Name, business-line, managed-by, managed-by-terraform | cluster, env, backup-enabled |
| **ALB** | Name, business-line, managed-by, managed-by-terraform | waf_enabled, env |
| **EKS Node** | Name, business-line, managed-by, managed-by-terraform | component(=eks), karpenter.sh/managed-by, env |
| **S3** | Name, business-line, managed-by, managed-by-terraform | env, backup-enabled |
| **EBS** | Name, business-line, managed-by, managed-by-terraform | component, env, backup-enabled |
| **Subnet** | Name, business-line, managed-by, managed-by-terraform | zone-group(可选) |
| **其他** | Name, business-line, managed-by, managed-by-terraform | env |

## 第一部分：全局必须标签（所有资源都要打）

**以下标签适用于所有 AWS 资源，必须包含：**

| 标签名 | 用途 | 值来源 | 默认值 |
|--------|------|--------|--------|
| `Name` | 资源名称，EC2 需与 Hostname 相同 | 用户指定 | 无默认值，必须指定 |
| `business-line` | 业务线代号 | 用户指定或推断 | 无默认值，必须指定  |
| `managed-by` | 资源管理责任人 | 用户指定或推断 | 无默认值，必须指定  |
| `managed-by-terraform` | IaC 管理标识 | 固定值 | `true` |


## 第二部分：资源类型专属标签

### EC2 实例

**除全局必须标签外，EC2 还必须包含：**

| 标签名 | 用途 | 值来源 | 默认值 |
|--------|------|--------|--------|
| `component` | 组件类型 | 根据用途推断 | 无默认值 |
| `env` | 环境标识 | 根据区域/描述推断 | 无默认值 |
| `cluster-name` | 集群名称 | 用户指定 | 无默认值 |
| `backup-enabled` | 备份策略 | 用户指定 | 无默认值，必须指定 |
| `owner` | 资源申请人 | 用户指定 | 无默认值，必须指定 |
| `created-by` | 资源创建人 | 用户指定 | 无默认值，必须指定 |
| `service-name` | 服务名称 | 根据用途推断 | 无默认值，必须指定 |
| `group-name` | 逻辑分组 | 用户指定 | 无默认值，必须指定 |

**component 允许值：**
| 值 | 说明 | 适用场景 |
|----|------|----------|
| `middleware` | 中间件 | ES、Kafka、ZK |
| `eks` | EKS 节点 | EKS 相关 |
| `biz_app` | 业务自研应用 | 业务应用 |
| `common_app` | 通用应用 | Apollo、Grafana 等 |
| `bd` | 大数据 | 大数据账号 |
| `novalue` | 未知 | 默认值 |

**env 允许值：**
| 环境 | 格式 | 示例 |
|------|------|------|
| 生产环境 | `pro` | `pro` |
| 开发环境 | `dev-<标识>` | `dev-feature1` |
| SIT 环境 | `sit-<标识>` | `sit-release1` |
| 压测环境 | `stress-<标识>` | `stress-2024q1` |

**EC2 完整标签示例：**
```json
{
  "Name": "ken-test",
  "business-line": "kc",
  "managed-by": "ken",
  "managed-by-terraform": "true",
  "component": "biz_app",
  "env": "pro",
  "cluster-name": "ken-test-cluster",
  "backup-enabled": "c3-ebs-daily-7",
  "owner": "zhangsan",
  "created-by": "ops-zhangsan",
  "service-name": "web-api",
  "group-name": "web-api-cluster"
}
```

---

### RDS 数据库

**除全局必须标签外，RDS 还必须包含：**

| 标签名 | 用途 | 值来源 | 默认值 |
|--------|------|--------|--------|
| `cluster` | RDS 集群名称 | 用户指定 | 无默认值，必须指定 |
| `env` | 环境标识 | 根据区域/描述推断 | 无默认值 |
| `backup-enabled` | 备份策略 | 用户指定 | 无默认值，必须指定 |

**RDS 完整标签示例：**
```json
{
  "Name": "prod-kc-mysql-cluster",
  "business-line": "kc",
  "managed-by": "dba-team",
  "managed-by-terraform": "true",
  "cluster": "prod-main-cluster",
  "env": "pro",
  "backup-enabled": "rds-daily-30"
}
```

---

### ALB 负载均衡

**除全局必须标签外，ALB 还必须包含：**

| 标签名 | 用途 | 值来源 | 默认值 |
|--------|------|--------|--------|
| `waf_enabled` | WAF 启用状态 | 用户指定 | 无默认值，必须指定 |
| `env` | 环境标识 | 根据区域/描述推断 | 无默认值 |

**waf_enabled 允许值：**
| 值 | 说明 |
|----|------|
| `Internal_VPN_AVD` | 内部 VPN ALB |
| `Internal_EW` | 内部东西向 ALB |
| `Public_Web_site` | CloudFront 主站备份 |
| `Public_Static_Content` | CloudFront S3 静态资源 |
| `Public_Push_Gateway` | CloudFront WebSocket |
| `Public_API_Gateway` | AWS API Gateway |
| `Third_Party` | 做市商 ALB |
| `disable` | 不启用 WAF（不推荐） |

**ALB 完整标签示例：**
```json
{
  "Name": "prod-kc-internal-alb",
  "business-line": "kc",
  "managed-by": "ops-team",
  "managed-by-terraform": "true",
  "waf_enabled": "Internal_EW",
  "env": "pro"
}
```

---

### EKS Node

**除全局必须标签外，EKS Node 还必须包含：**

| 标签名 | 用途 | 值来源 | 默认值 |
|--------|------|--------|--------|
| `component` | 组件类型 | 固定值 | `eks` |
| `karpenter.sh/managed-by` | EKS 集群名 | 用户指定 | 集群名称 |
| `env` | 环境标识 | 根据区域/描述推断 | 无默认值 |
| `group-name` | NodeGroup 名称 | 用户指定 | `eks-node` |

**EKS Node 完整标签示例：**
```json
{
  "Name": "prod-kc-eks-node-001",
  "business-line": "kc",
  "managed-by": "ops-team",
  "managed-by-terraform": "true",
  "component": "eks",
  "karpenter.sh/managed-by": "kucoin",
  "env": "pro",
  "group-name": "eks-node"
}
```

---

### S3 存储桶

**除全局必须标签外，S3 还必须包含：**

| 标签名 | 用途 | 值来源 | 默认值 |
|--------|------|--------|--------|
| `env` | 环境标识 | 根据区域/描述推断 | 无默认值 |
| `backup-enabled` | 备份策略 | 用户指定 | 无默认值 |

---

### EBS 卷

**除全局必须标签外，EBS 还必须包含：**

| 标签名 | 用途 | 值来源 | 默认值 |
|--------|------|--------|--------|
| `component` | 组件类型 | 根据用途推断 | 无默认值 |
| `env` | 环境标识 | 根据区域/描述推断 | 无默认值 |
| `backup-enabled` | 备份策略 | 用户指定 | 无默认值，必须指定 |

---

### Subnet 子网

**除全局必须标签外，Subnet 可选包含：**

| 标签名 | 用途 | 值来源 | 默认值 |
|--------|------|--------|--------|
| `zone-group` | 多 AZ 组标识 | 用户指定 | 可选 |

**zone-group 示例值：** `kc-private-eks`, `kc-private-ec2`, `web-private-eks`

---

## 第三部分：可选标签（推荐但非必须）

| 标签名 | 用途 | 适用场景 | 值格式 |
|--------|------|----------|--------|
| `business-line-shared` | 共用业务线 | 共享资源 | 冒号分隔，如 `kc:km` |

**注意**：`owner`、`created-by`、`service-name`、`group-name` 在 EC2 资源中是必须标签，在其他资源中是可选标签。



