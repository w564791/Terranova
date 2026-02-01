---
name: cmdb_resource_matching
layer: domain
description: 从 CMDB 查询结果中匹配用户需要的资源，支持精确匹配和模糊匹配,不需要查询请求请勿使用
tags: ["domain", "cmdb", "matching", "resource", "lookup", "search"]
---

## CMDB 资源匹配规则

### 概述
本规范定义了如何从 CMDB 查询结果中匹配和选择正确的资源。当用户描述中提到现有资源（如 VPC、子网、安全组）时，需要从 CMDB 中查找对应的资源 ID。

### 资源 ID 使用原则

#### 核心原则
1. **优先使用 CMDB 查询结果**：如果 CMDB 返回了资源 ID，直接使用
2. **不要编造 ID**：如果 CMDB 未找到资源，返回 `need_more_info` 状态
3. **保持 ID 格式**：AWS 资源 ID 有特定格式，必须保持一致

#### 资源 ID 格式规范
| 资源类型 | ID 前缀 | 格式示例 |
|----------|---------|----------|
| VPC | `vpc-` | `vpc-0123456789abcdef0` |
| Subnet | `subnet-` | `subnet-0123456789abcdef0` |
| Security Group | `sg-` | `sg-0123456789abcdef0` |
| AMI | `ami-` | `ami-0123456789abcdef0` |
| Internet Gateway | `igw-` | `igw-0123456789abcdef0` |
| NAT Gateway | `nat-` | `nat-0123456789abcdef0` |
| Route Table | `rtb-` | `rtb-0123456789abcdef0` |
| Network ACL | `acl-` | `acl-0123456789abcdef0` |
| EBS Volume | `vol-` | `vol-0123456789abcdef0` |
| Snapshot | `snap-` | `snap-0123456789abcdef0` |
| Elastic IP | `eipalloc-` | `eipalloc-0123456789abcdef0` |
| Network Interface | `eni-` | `eni-0123456789abcdef0` |
| Key Pair | - | `my-key-pair`（无前缀） |
| IAM Role | `arn:aws:iam::` | `arn:aws:iam::123456789012:role/MyRole` |
| IAM Policy | `arn:aws:iam::` | `arn:aws:iam::123456789012:policy/MyPolicy` |
| KMS Key | - | `arn:aws:kms:region:account:key/key-id` |

### 匹配策略

#### 1. 精确匹配
当用户提供完整的资源 ID 时，直接使用：
```
用户输入: "使用 vpc-0123456789abcdef0"
匹配结果: vpc-0123456789abcdef0
```

#### 2. 名称匹配
当用户提供资源名称时，在 CMDB 中搜索：
```
用户输入: "使用 exchange VPC"
CMDB 查询: 搜索 name 包含 "exchange" 的 VPC
匹配结果: vpc-xxx (name: "exchange-vpc")
```

#### 3. 标签匹配
当用户描述资源特征时，通过标签匹配：
```
用户输入: "生产环境的子网"
CMDB 查询: 搜索 tags.environment = "production" 的 subnet
匹配结果: subnet-xxx (tags: {environment: "production"})
```

#### 4. 模糊匹配
当精确匹配失败时，尝试模糊匹配：
```
用户输入: "exchange vpc"
模糊匹配:
  - "exchange-vpc" ✓
  - "exchange_vpc" ✓
  - "exchangevpc" ✓
  - "vpc-exchange" ✓
```

### 匹配优先级

当多个资源匹配时，按以下优先级选择：

1. **精确名称匹配**（最高）
   - 名称完全一致

2. **前缀匹配**
   - 名称以搜索词开头

3. **包含匹配**
   - 名称包含搜索词

4. **标签匹配**
   - 标签值匹配搜索词

5. **最近创建**（最低）
   - 如果仍有多个匹配，选择最近创建的

### 多资源处理

#### 单选字段
当字段只需要一个资源时：
- 如果匹配到多个，选择优先级最高的
- 在 message 中说明选择了哪个资源

#### 多选字段
当字段需要多个资源时（如 `security_group_ids`）：
- 返回所有匹配的资源
- 使用数组格式：`["sg-xxx", "sg-yyy"]`

### 资源依赖验证

#### VPC 依赖
- **子网**必须属于指定的 VPC
- **安全组**必须属于指定的 VPC（除非是跨 VPC 引用）
- **路由表**必须属于指定的 VPC

#### 区域一致性
- 所有资源必须在同一区域
- 跨区域资源需要特别处理

#### 可用区一致性
- 子网有特定的可用区
- 某些资源（如 EBS）需要与实例在同一可用区

### 匹配失败处理

#### 未找到资源
```json
{
  "status": "need_more_info",
  "message": "未找到名为 'exchange' 的 VPC",
  "placeholders": [
    {
      "field": "vpc_id",
      "reason": "CMDB 中未找到匹配的 VPC",
      "suggestions": ["vpc-prod-main", "vpc-staging", "vpc-dev"]
    }
  ]
}
```

#### 多个匹配
```json
{
  "status": "need_more_info",
  "message": "找到多个匹配的 VPC，请指定具体名称",
  "placeholders": [
    {
      "field": "vpc_id",
      "reason": "找到 3 个匹配的 VPC",
      "suggestions": ["exchange-vpc-prod", "exchange-vpc-staging", "exchange-vpc-dev"]
    }
  ]
}
```

### CMDB 数据结构

#### VPC
```json
{
  "id": "vpc-0123456789abcdef0",
  "name": "exchange-vpc",
  "cidr_block": "10.0.0.0/16",
  "region": "ap-northeast-1",
  "tags": {
    "environment": "production",
    "business-line": "exchange"
  }
}
```

#### Subnet
```json
{
  "id": "subnet-0123456789abcdef0",
  "name": "private-subnet-1a",
  "vpc_id": "vpc-0123456789abcdef0",
  "cidr_block": "10.0.1.0/24",
  "availability_zone": "ap-northeast-1a",
  "is_public": false,
  "tags": {
    "type": "private",
    "tier": "application"
  }
}
```

#### Security Group
```json
{
  "id": "sg-0123456789abcdef0",
  "name": "web-sg",
  "vpc_id": "vpc-0123456789abcdef0",
  "description": "Security group for web servers",
  "tags": {
    "purpose": "web"
  }
}
```

### 关键词识别

#### 环境关键词
| 关键词 | 匹配标签 |
|--------|----------|
| 生产、prod、production | `environment: production` |
| 预发、staging、stg | `environment: staging` |
| 开发、dev、development | `environment: development` |
| 测试、test、testing | `environment: testing` |

#### 网络类型关键词
| 关键词 | 匹配条件 |
|--------|----------|
| 公有、public | `is_public: true` 或 `type: public` |
| 私有、private | `is_public: false` 或 `type: private` |
| 内网、internal | `type: internal` |
| 外网、external | `type: external` |

#### 业务线关键词
| 关键词 | 匹配标签 |
|--------|----------|
| exchange | `business-line: exchange` |
| wallet | `business-line: wallet` |
| platform | `business-line: platform` |

### 最佳实践

1. **优先使用 ID**
   - 如果用户提供了资源 ID，直接使用
   - 不需要再次查询 CMDB

2. **验证资源存在**
   - 即使用户提供了 ID，也应验证资源存在
   - 防止使用已删除的资源

3. **记录匹配过程**
   - 在 message 中说明如何匹配到资源
   - 帮助用户理解选择逻辑

4. **提供替代选项**
   - 匹配失败时，提供可用的资源列表
   - 帮助用户快速选择