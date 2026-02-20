---
name: json_output_format
layer: foundation
description: JSON 输出格式的强制技术规范，确保 AI 生成的资源配置输出格式正确、结构统一
tags: ["foundation", "json", "output", "format", "validation"]
---

## JSON 输出格式规范

### 输出结构
所有资源配置生成的输出必须遵循以下 JSON 结构：

```json
{
  "status": "complete | need_more_info | partial",
  "config": {
    // 生成的配置，键值对形式
  },
  "message": "给用户的提示信息",
  "placeholders": [
    {
      "field": "字段名",
      "reason": "需要用户补充的原因",
      "suggestions": ["建议值1", "建议值2"]
    }
  ]
}
```

### 状态定义

| 状态 | 含义 | 使用场景 |
|------|------|----------|
| `complete` | 配置完整 | 所有必填字段已填充，值符合约束 |
| `need_more_info` | 缺少必要信息 | 必填字段缺失或无法推断 |
| `partial` | 部分完成 | 部分字段使用了占位符或默认值 |

### 字段规范

#### config 对象
- **键名**：必须与 Schema 中定义的字段名完全一致
- **值类型**：必须与 Schema 中定义的类型一致
- **空值处理**：
  - 未提供的可选字段：不包含在 config 中
  - 显式设置为空：使用 `null`
  - 空字符串：使用 `""`
  - 空数组：使用 `[]`
  - 空对象：使用 `{}`

#### message 字段
- **长度**：不超过 200 字符
- **语言**：使用中文
- **内容要求**：
  - `complete` 状态：简要说明配置内容
  - `need_more_info` 状态：明确说明缺少什么信息
  - `partial` 状态：说明哪些字段使用了默认值或占位符

#### placeholders 数组
- **仅在** `need_more_info` 或 `partial` 状态时包含
- **每个占位符必须包含**：
  - `field`：字段名（必须与 Schema 一致）
  - `reason`：需要补充的原因
  - `suggestions`：建议值数组（可为空数组）

### 类型映射规则

| Schema 类型 | JSON 输出类型 | 示例 |
|-------------|--------------|------|
| string | string | `"value"` |
| integer | number | `42` |
| number | number | `3.14` |
| boolean | boolean | `true` / `false` |
| array | array | `["a", "b"]` |
| object | object | `{"key": "value"}` |

### 特殊值处理

#### 资源 ID
- 必须使用 CMDB 返回的实际 ID
- 格式示例：
  - VPC: `"vpc-0123456789abcdef0"`
  - Subnet: `"subnet-0123456789abcdef0"`
  - Security Group: `"sg-0123456789abcdef0"`
  - AMI: `"ami-0123456789abcdef0"`

#### 枚举值
- 必须使用 Schema 中定义的枚举值之一
- 大小写敏感

#### 数组字段
- 即使只有一个元素，也必须使用数组格式
- 示例：`"security_group_ids": ["sg-xxx"]`

### 占位符规范

**参考 `placeholder_standard` Skill 中定义的统一占位符格式。**

### 禁止事项

1. **禁止输出非 JSON 内容**
   - 不要在 JSON 前后添加任何文字说明
   - 不要使用 Markdown 代码块包裹

2. **禁止编造资源 ID**
   - 如果 CMDB 未返回资源，使用 `need_more_info` 状态
   - 资源 ID 字段使用标准占位符：`"{{PLACEHOLDER:字段名}}"`

3. **禁止类型不匹配**
   - 数字不要用字符串表示：❌ `"port": "80"` → ✅ `"port": 80`
   - 布尔值不要用字符串：❌ `"enabled": "true"` → ✅ `"enabled": true`

4. **禁止包含注释**
   - JSON 不支持注释，不要使用 `//` 或 `/* */`

### 验证规则

输出前必须验证：
1. JSON 语法正确（可被解析）
2. 包含所有必需字段（status, config, message）
3. status 值为三个有效值之一
4. config 中的字段名与 Schema 一致
5. 值类型与 Schema 定义一致

### 错误处理

| 错误类型 | 处理方式 |
|----------|----------|
| Schema 解析失败 | 返回 `need_more_info`，message 说明原因 |
| CMDB 查询失败 | 返回 `need_more_info`，placeholders 列出缺失资源 |
| 用户输入不完整 | 返回 `need_more_info`，placeholders 列出缺失字段 |
| 值不符合约束 | 返回 `partial`，message 说明约束问题 |

### 示例输出

#### 完整配置
```json
{
  "status": "complete",
  "config": {
    "name": "web-server-prod",
    "instance_type": "t3.medium",
    "vpc_id": "vpc-0123456789abcdef0",
    "subnet_id": "subnet-0123456789abcdef0",
    "security_group_ids": ["sg-0123456789abcdef0"],
    "enable_monitoring": true
  },
  "message": "已生成 EC2 实例配置，使用 t3.medium 实例类型，部署在 exchange VPC 中"
}
```

#### 需要更多信息
```json
{
  "status": "need_more_info",
  "config": {
    "name": "web-server-prod",
    "instance_type": "t3.medium"
  },
  "message": "需要指定 VPC 和子网信息",
  "placeholders": [
    {
      "field": "vpc_id",
      "reason": "未找到匹配的 VPC，请指定 VPC 名称或 ID",
      "suggestions": []
    },
    {
      "field": "subnet_id",
      "reason": "需要指定部署的子网",
      "suggestions": []
    }
  ]
}
```

#### 部分完成
```json
{
  "status": "partial",
  "config": {
    "name": "web-server-prod",
    "instance_type": "t3.medium",
    "vpc_id": "vpc-0123456789abcdef0",
    "subnet_id": "subnet-0123456789abcdef0",
    "enable_monitoring": true
  },
  "message": "配置基本完成，安全组使用了默认值，建议根据实际需求调整",
  "placeholders": [
    {
      "field": "security_group_ids",
      "reason": "未指定安全组，使用了 VPC 默认安全组",
      "suggestions": ["sg-default-web", "sg-default-ssh"]
    }
  ]
}