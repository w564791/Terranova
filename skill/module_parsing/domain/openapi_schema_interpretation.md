---
name: openapi_schema_interpretation
layer: domain
description: OpenAPI Schema 解读规范，定义如何从 OpenAPI 3.1.0 规范中提取和理解 Module 配置信息
tags: ["openapi", "schema", "interpretation", "parsing", "module"]
---

## OpenAPI Schema 解读规范

### Schema 结构定位
IaC Platform 的 OpenAPI Schema 遵循 OpenAPI 3.1.0 规范，关键路径如下：

| 内容 | 路径 |
|------|------|
| 必填字段列表 | `components.schemas.ModuleInput.required` |
| 字段属性定义 | `components.schemas.ModuleInput.properties` |
| UI 配置 | `x-iac-platform.ui.fields` |
| 外部数据源 | `x-iac-platform.external.sources` |
| 输出定义 | `x-iac-platform.outputs.items` |

### 字段描述优先级
获取字段描述时，按以下优先级选择：

1. **最高优先级**：`x-iac-platform.ui.fields[fieldName].label`
   - 这是面向用户的友好名称
   
2. **次优先级**：`components.schemas.ModuleInput.properties[fieldName].description`
   - 这是 Schema 中的技术描述
   
3. **最低优先级**：字段名本身
   - 将驼峰命名转换为空格分隔的词组
   - 例如：`instanceType` → `Instance Type`

### 必填字段识别
字段被视为必填的条件（满足任一即可）：

1. 字段名出现在 `components.schemas.ModuleInput.required` 数组中
2. `x-iac-platform.ui.fields[fieldName].required` 为 `true`

**注意**：两个来源的必填字段取**并集**。

### 条件显示规则
解析 `x-iac-platform.ui.fields[fieldName].showWhen` 字段：

**格式**：
- `"field_name == value"` - 当 field_name 等于 value 时显示
- `"field_name != value"` - 当 field_name 不等于 value 时显示
- `"field_name"` - 当 field_name 为真值时显示

**示例**：
```json
{
  "showWhen": "enable_karpenter == true"
}
```
表示：当 `enable_karpenter` 字段值为 `true` 时，该字段才显示。

### 字段分组
从 `x-iac-platform.ui.fields[fieldName].group` 提取分组信息：

| 分组标识 | 含义 | 说明 |
|----------|------|------|
| `basic` | 基础配置 | 用户必须关注的核心字段 |
| `advanced` | 高级配置 | 可选的高级功能字段 |
| `security` | 安全配置 | 安全相关字段 |
| `network` | 网络配置 | 网络相关字段 |
| `ebs` | 存储配置 | EBS/存储相关字段 |
| `size` | 容量配置 | 规格/容量相关字段 |
| `mapping` | 映射配置 | 资源映射相关字段 |
| 其他 | 自定义分组 | 根据 Module 特点自定义 |

### 外部数据源解析
从 `x-iac-platform.external.sources` 数组中提取数据源定义：

每个数据源包含以下字段：
| 字段 | 说明 | 示例 |
|------|------|------|
| `id` | 数据源唯一标识 | `"vpc_list"` |
| `path` | API 路径 | `"/api/v1/aws/ec2/vpcs"` |
| `method` | HTTP 方法 | `"GET"` |
| `dependsOn` | 依赖的字段 | `["aws_region"]` |
| `valueField` | 值字段 | `"id"` |
| `labelField` | 显示字段 | `"name"` |
| `params` | 请求参数 | `{"region": "{aws_region}"}` |

### 输出属性解析
从 `x-iac-platform.outputs.items` 数组中提取输出定义：

每个输出包含以下字段：
| 字段 | 说明 |
|------|------|
| `name` | 输出名称 |
| `type` | 输出类型（string, number, object 等） |
| `description` | 输出描述 |
| `sensitive` | 是否敏感数据 |

### 字段约束提取
从 `properties[fieldName]` 中提取约束信息：

| 约束类型 | Schema 字段 | 说明 |
|----------|-------------|------|
| 枚举值 | `enum` | 可选值列表 |
| 最小值 | `minimum` | 数值最小值 |
| 最大值 | `maximum` | 数值最大值 |
| 最小长度 | `minLength` | 字符串最小长度 |
| 最大长度 | `maxLength` | 字符串最大长度 |
| 正则模式 | `pattern` | 正则表达式验证 |
| 默认值 | `default` | 字段默认值 |
| 格式 | `format` | 特殊格式（date, email 等） |

### 参数关联关系识别
需要识别以下类型的关联关系：

1. **条件必填**：某字段在特定条件下变为必填
   - 通过 `showWhen` + `required` 组合判断

2. **互斥关系**：两个字段不能同时设置
   - 通常在 description 中说明

3. **依赖关系**：某字段依赖另一字段的值
   - 通过 `dependsOn` 或 `showWhen` 判断

4. **联动关系**：某字段的可选值依赖另一字段
   - 通过外部数据源的 `dependsOn` 判断