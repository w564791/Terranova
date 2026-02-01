# Terraform Variables 注释规范

## 概述

本文档定义了在Terraform `variables.tf` 文件中使用行尾注释来标记变量属性的规范。这些注释将被 `tf2openapi` 工具解析，生成包含完整元数据的OpenAPI Schema。

## 注释格式

### 基本语法

```hcl
variable "name" {
  description = "描述"  # @key:value key2:value2
  type        = string
  default     = null
}
```

注释以 `#` 开头，使用 `@key:value` 格式，多个属性用空格分隔。

### 支持的注释位置

1. **变量块上方** - 用于标记整个变量的属性
2. **属性行尾** - 用于标记特定属性（如description行）
3. **嵌套属性行尾** - 用于标记object内部属性

## 支持的属性

### 分组和显示

| 属性 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `level` | basic/advanced | advanced | 变量级别，决定分组 |
| `group` | string | - | 自定义分组ID |
| `order` | number | - | 显示顺序 |
| `hidden` | bool | false | 默认隐藏 |
| `alias` | string | - | 中文别名 |

### Terraform行为

| 属性 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `computed` | bool | false | 计算字段，只读 |
| `force_new` | bool | false | 变更需重建资源 |
| `write_only` | bool | false | 只写字段 |
| `sensitive` | bool | false | 敏感字段 |
| `deprecated` | string | - | 弃用警告信息 |

### 验证和约束

| 属性 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `conflicts_with` | string[] | - | 冲突字段列表 |
| `exactly_one_of` | string[] | - | 必须且只能选一个 |
| `at_least_one_of` | string[] | - | 至少选一个 |
| `required_with` | string[] | - | 依赖字段 |

### UI控件

| 属性 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `widget` | string | auto | UI控件类型 |
| `placeholder` | string | - | 占位符文本 |
| `prefix` | string | - | 输入前缀 |
| `suffix` | string | - | 输入后缀 |

## 示例

### 基础用法

```hcl
variable "name" {
  description = "Name to be used on EC2 instance created"  # @level:basic alias:实例名称
  type        = string
  default     = ""
}

variable "instance_type" {
  description = "The type of instance to start"  # @level:basic widget:select
  type        = string
  default     = "t3.micro"
}

variable "ami" {
  description = "ID of AMI to use for the instance"  # @level:basic force_new:true conflicts_with:ami_ssm_parameter
  type        = string
  default     = null

  validation {
    condition     = can(regex("^ami-", var.ami))
    error_message = "AMI ID must start with 'ami-'."
  }
}
```

### 高级用法

```hcl
variable "arn" {
  description = "The ARN of the bucket"  # @computed:true
  type        = string
  default     = null
}

variable "bucket_domain_name" {
  description = "The bucket domain name"  # @computed:true
  type        = string
  default     = null
}

variable "password" {
  description = "Database password"  # @sensitive:true write_only:true widget:password
  type        = string
  default     = null
}

variable "old_setting" {
  description = "Deprecated setting"  # @deprecated:Use_new_setting_instead
  type        = string
  default     = null
}
```

### 冲突和依赖关系

```hcl
variable "acl" {
  description = "The canned ACL to apply"  # @conflicts_with:grant,object_ownership
  type        = string
  default     = null
}

variable "grant" {
  description = "An ACL policy grant"  # @conflicts_with:acl
  type = list(object({
    id          = string
    type        = string
    permissions = list(string)
    uri         = string
  }))
  default = []
}

variable "object_lock_enabled" {
  description = "Enable object lock"  # @force_new:true required_with:versioning.enabled
  type        = bool
  default     = false
}

variable "kms_master_key_id" {
  description = "KMS key ARN"  # @required_with:sse_algorithm
  type        = string
  default     = null
}
```

### 分组示例

```hcl
variable "bucket" {
  description = "Bucket name"  # @level:basic group:basic order:1
  type        = string
}

variable "acl" {
  description = "Access control"  # @group:security order:1
  type        = string
  default     = "private"
}

variable "versioning" {
  description = "Versioning configuration"  # @group:versioning order:1
  type = object({
    enabled    = bool  # @alias:启用版本控制
    mfa_delete = bool  # @alias:MFA删除保护
  })
  default = {
    enabled    = false
    mfa_delete = false
  }
}

variable "lifecycle_rule" {
  description = "Lifecycle rules"  # @group:lifecycle widget:object-list
  type = list(object({
    id      = string           # @alias:规则ID
    enabled = bool             # @alias:启用
    prefix  = optional(string) # @alias:前缀过滤
    tags    = optional(map(string))  # @alias:标签过滤 widget:key-value
    transition = optional(list(object({
      days          = number  # @alias:天数 min:0
      storage_class = string  # @alias:存储类型 widget:select
    })))
    expiration = optional(object({
      days = number  # @alias:过期天数 min:1
    }))
  }))
  default = []
}
```

### 嵌套对象属性注释

```hcl
variable "server_side_encryption_configuration" {
  description = "Server-side encryption configuration"  # @group:security
  type = object({
    rule = object({
      apply_server_side_encryption_by_default = object({
        sse_algorithm     = string  # @widget:select enum:AES256,aws:kms alias:加密算法
        kms_master_key_id = optional(string)  # @alias:KMS密钥ARN placeholder:arn:aws:kms:...
      })
      bucket_key_enabled = optional(bool)  # @alias:启用桶密钥
    })
  })
  default = null
}
```

## 解析规则

### 1. 注释提取

工具会扫描每行末尾的 `# @...` 注释，提取键值对。

### 2. 值类型转换

- `true`/`false` → boolean
- 数字 → number
- 逗号分隔 → array (如 `conflicts_with:a,b,c`)
- 下划线替换空格 (如 `deprecated:Use_new_setting` → "Use new setting")

### 3. 继承规则

- 变量级注释应用于整个变量
- 嵌套属性注释仅应用于该属性
- 子属性可覆盖父属性的设置

## 生成的OpenAPI Schema

输入:
```hcl
variable "ami" {
  description = "ID of AMI to use"  # @level:basic force_new:true conflicts_with:ami_ssm_parameter
  type        = string
  default     = null
}
```

输出:
```json
{
  "ami": {
    "type": "string",
    "title": "Ami",
    "description": "ID of AMI to use",
    "x-force-new": true,
    "x-conflicts-with": ["ami_ssm_parameter"]
  }
}
```

UI配置:
```json
{
  "ami": {
    "widget": "text",
    "label": "Ami",
    "group": "basic",
    "order": 1,
    "help": "ID of AMI to use"
  }
}
```

## 最佳实践

1. **保持注释简洁** - 只添加必要的属性
2. **使用有意义的alias** - 提供清晰的中文名称
3. **正确标记level** - 常用参数标记为basic
4. **声明依赖关系** - 使用conflicts_with和required_with
5. **标记敏感字段** - 密码等使用sensitive:true
