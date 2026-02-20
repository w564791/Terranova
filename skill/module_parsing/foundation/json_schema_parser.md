---
name: json_schema_parser
layer: foundation
description: JSON Schema 解析的强制技术规范，确保正确解析 JSON 结构和数据类型
tags: ["foundation", "json", "schema", "parser", "parsing"]
---

## JSON Schema 解析规范

### 类型映射
| JSON 类型 | 说明 | 注意事项 |
|-----------|------|----------|
| string | 字符串 | 检查 format、pattern、minLength、maxLength |
| integer | 整数 | 检查 minimum、maximum、exclusiveMinimum、exclusiveMaximum |
| number | 浮点数 | 同 integer，但允许小数 |
| boolean | 布尔值 | 只能是 true 或 false |
| array | 数组 | 检查 items 定义、minItems、maxItems、uniqueItems |
| object | 对象 | 递归解析 properties、required、additionalProperties |
| null | 空值 | 表示字段可以为 null |

### 路径定位规则
- 使用点号 `.` 分隔对象路径：`components.schemas.ModuleInput`
- 数组索引使用方括号：`items[0]`
- 路径区分大小写

示例路径：
```
components.schemas.ModuleInput.properties.name.type
components.schemas.ModuleInput.required[0]
x-iac-platform.ui.fields.name.label
```

### 必须解析的字段
对于每个属性（property），必须提取以下信息：

| 字段 | 说明 | 默认值 |
|------|------|--------|
| type | 数据类型 | "string" |
| description | 字段描述 | "" |
| default | 默认值 | 无 |
| enum | 枚举值列表 | 无 |
| minimum | 最小值 | 无 |
| maximum | 最大值 | 无 |
| minLength | 最小长度 | 无 |
| maxLength | 最大长度 | 无 |
| pattern | 正则表达式 | 无 |
| format | 格式（如 date、email） | 无 |

### 嵌套结构处理
- 遇到 `type: "object"` 时，递归解析其 `properties`
- 遇到 `type: "array"` 时，解析其 `items` 定义
- 遇到 `$ref` 时，解析引用的定义
- 遇到 `allOf`、`anyOf`、`oneOf` 时，合并或选择解析

### 错误处理规则
1. **路径不存在**：返回空值（null/undefined），不报错
2. **类型不匹配**：记录警告，尝试类型转换
3. **无效 JSON**：必须报错，不能静默失败
4. **循环引用**：检测并中断，避免无限递归

### 特殊值处理
- `null` 值：保留为 null，不转换为字符串
- 空字符串 `""`：保留为空字符串
- 数字 `0`：保留为数字 0，不视为 false
- 空数组 `[]`：保留为空数组
- 空对象 `{}`：保留为空对象

### 验证规则
解析完成后，必须验证：
1. 所有 required 字段都存在于 properties 中
2. enum 值的类型与字段类型一致
3. minimum <= maximum（如果都存在）
4. minLength <= maxLength（如果都存在）