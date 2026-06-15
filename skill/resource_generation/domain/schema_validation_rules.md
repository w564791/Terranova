---
name: schema_validation_rules
layer: domain
description: Schema 验证规则
tags: ["schema", "validation", "openapi", "constraint", "form"]
<!-- 该部分内容只是为了说明skill用途以及作用域,不要复制到skill正文里 -->
---

## Schema 验证规则

### 字段类型处理
- `string`: 字符串值，注意最大长度限制
- `integer`: 整数值，注意最小/最大值限制
- `boolean`: true 或 false
- `array`: 数组，注意元素类型和数量限制
- `object`: 嵌套对象，递归验证

### 必填字段
- 标记为 `required` 的字段必须提供值
- 如果用户未提供，返回 `need_more_info` 状态

### 枚举字段
- 只能使用 `enum` 中定义的值
- 如果用户提供的值不在枚举中，选择最接近的或询问用户

### 默认值
- 如果字段有 `default` 值且用户未指定，使用默认值
- 在 message 中说明使用了哪些默认值

### 格式验证
- 遵循 `pattern` 正则表达式约束
- 遵循 `minLength`/`maxLength` 长度约束
- 遵循 `minimum`/`maximum` 数值约束
