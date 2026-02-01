---
name: module_skill_generation_workflow
layer: task
description: 根据 Module 的 OpenAPI Schema 生成结构化的 Skill 文档，用于指导 AI 理解 Module 配置规则和最佳实践
tags: ["module", "skill", "generation", "workflow", "task"]
domain_tags: ["openapi", "skill-generation", "terraform-docs"]
version: "1.1.0"
---

# Module Skill 生成工作流

## 任务目标
根据 Module 的 OpenAPI Schema 定义，生成一份结构化的 Skill 文档，用于指导 AI 在资源配置生成任务中理解该 Module 的配置规则和最佳实践。

## 依赖技能

### Foundation 层（强制规范）
| 技能名称 | 用途 |
|----------|------|
| `markdown_output_format` | Markdown 输出格式规范 |
| `json_schema_parser` | JSON Schema 解析规范 |

### Domain 层（领域规范）
| 技能名称 | 用途 |
|----------|------|
| `openapi_schema_interpretation` | OpenAPI Schema 解读规范 |
| `skill_doc_writing_standards` | Skill 文档写作规范 |
| `terraform_module_best_practices` | Terraform Module 最佳实践 |

## 输入
- Module 名称: {module_name}
- Module 提供商: {provider}
- Module 描述: {description}
- OpenAPI Schema: {openapi_schema}

## 执行步骤

### 步骤 1: Schema 解析
**遵循**: `json_schema_parser` 规范

1. 定位 `components.schemas.ModuleInput`
2. 提取 `required` 数组（必填字段列表）
3. 提取 `properties` 对象（字段定义）
4. 提取 `x-iac-platform` 扩展配置：
   - `x-iac-platform.ui.fields` - UI 配置
   - `x-iac-platform.external.sources` - 外部数据源
   - `x-iac-platform.outputs.items` - 输出定义

### 步骤 2: 字段分析
**遵循**: `openapi_schema_interpretation` 规范

1. **获取字段描述**（按优先级）：
   - 优先使用 `x-iac-platform.ui.fields[field].label`
   - 其次使用 `properties[field].description`
   - 最后使用字段名本身

2. **识别必填字段**：
   - 检查 `required` 数组
   - 检查 `ui.fields[field].required`
   - 取并集

3. **解析条件显示规则**：
   - 提取 `showWhen` 条件
   - 记录字段间的依赖关系

4. **提取字段分组**：
   - 从 `ui.fields[field].group` 获取
   - 按分组归类字段

5. **提取外部数据源**：
   - 从 `external.sources` 获取
   - 记录数据源 ID、API 路径、依赖字段

6. **识别 JSON 字符串字段**（重要）：
   - 检查 `type: string` 且 `format: json` 的字段
   - 这类字段的值必须是 **JSON 字符串**，而不是 JSON 对象
   - 在"重要可选字段"表格中，类型列显示为 `string (JSON)`
   - 在"参数约束"章节的"参数关联关系"后添加"特殊字段类型"说明

### 步骤 2.5: 复杂类型递归解析（重要）

对于 `type: array` 或 `type: object` 且包含嵌套结构的字段，需要递归解析其内部结构：

#### 2.5.1 识别需要展开的字段
- `type: array` 且 `items.type: object` 且 `items.properties` 存在
- `type: object` 且 `properties` 存在且子字段数量 > 2
- 排除使用 `additionalProperties` 的简单 key-value 映射类型

#### 2.5.2 展开规则
1. **展开深度**：最多展开 3 层嵌套
2. **路径表示法**：
   - 使用点号分隔嵌套路径（如 `filter.prefix`）
   - 数组元素使用 `[]` 表示（如 `rules[].name`、`transition[].days`）
3. **超深度处理**：超过 3 层的字段标注"详见 Schema 定义"

#### 2.5.3 输出格式
在"参数约束"章节后，增加"复杂字段结构"子章节，使用扁平化路径表格：

```
### 复杂字段结构

#### {field_name} 结构
| 字段路径 | 类型 | 默认值 | 描述 |
|----------|------|--------|------|
| id | string | - | 唯一标识符 |
| enabled | boolean | true | 是否启用 |
| config.timeout | integer | 30 | 超时时间（秒） |
| items[].name | string | - | 项目名称 |
| items[].value | string | - | 项目值 |
```

#### 2.5.4 跳过展开的情况
- 字段类型为 `object` 但使用 `additionalProperties`（表示 key-value 映射）
- 嵌套深度超过 3 层
- 子字段数量超过 20 个（标注"字段较多，详见 Schema 定义"）
- 字段已在其他地方详细说明

### 步骤 3: 内容生成
**遵循**: `skill_doc_writing_standards` 规范

按以下章节顺序生成文档：

#### 3.1 模块概述
- 2-3 句话描述模块用途
- 说明适用场景
- 基于 Module 的 `description` 和 `provider` 信息

#### 3.2 参数约束

**必填字段表格**：
| 字段名 | 类型 | 验证规则 | 描述 |
|--------|------|----------|------|

- 从 `required` 数组获取字段列表
- 从 `properties` 获取类型和约束
- 验证规则包括：enum、pattern、min/max 等

**重要可选字段表格**：
| 字段名 | 类型 | 默认值 | 描述 |
|--------|------|--------|------|

- 选择有 `default` 值的字段
- 选择常用的配置字段

**参数关联关系**：
- 列出条件必填关系（基于 `showWhen`）
- 列出互斥关系
- 列出依赖关系

#### 3.3 复杂字段结构（新增章节）
对于步骤 2.5 中识别的复杂类型字段，展开描述其内部结构：

**格式要求**：
- 每个复杂字段单独一个子章节
- 使用扁平化路径表格展示所有嵌套字段
- 包含字段路径、类型、默认值、描述
- 如有枚举值，在描述中列出可选值

#### 3.4 配置分组
按 `group` 分类说明字段：
- **基础配置 (basic)**：核心必填字段
- **高级配置 (advanced)**：可选功能字段
- **安全配置 (security)**：安全相关字段
- **网络配置 (network)**：网络相关字段
- 其他自定义分组...

#### 3.5 数据源映射
| 字段 | 数据源 ID | API 路径 | 说明 |
|------|-----------|----------|------|

- 从 `external.sources` 提取
- 说明哪些字段需要从外部获取数据

#### 3.6 最佳实践
**参考**: `terraform_module_best_practices` 规范

根据 Module 特点，从以下方面提取 3-5 条建议：
- 安全配置建议（加密、保护机制、最小权限）
- 高可用配置建议（多可用区、自动扩缩容）
- 成本优化建议（实例类型选择、Spot 实例）
- 标签规范建议

**注意**：最佳实践必须具体可操作，避免空泛建议。

#### 3.7 输出属性
| 输出名 | 类型 | 描述 |
|--------|------|------|

- 从 `x-iac-platform.outputs.items` 提取
- 说明每个输出的用途

### 步骤 4: 格式输出
**遵循**: `markdown_output_format` 规范

1. 使用正确的标题层级：
   - `#` 文档标题
   - `##` 主要章节
   - `###` 子章节

2. 表格格式规范：
   - 必须有表头和分隔行
   - 单元格内容不换行

3. 不添加任何额外内容：
   - 不添加解释性文字
   - 不添加 HTML 标签

## 输出要求
生成 Markdown 格式的 Skill 文档，包含以下章节：

1. **模块概述** - 2-3 句话
2. **参数约束** - 必填字段、可选字段、关联关系
3. **复杂字段结构** - 展开 array/object 类型字段的内部结构（新增）
4. **配置分组** - 按 UI 分组说明
5. **数据源映射** - 外部数据获取
6. **最佳实践** - 3-5 条具体建议
7. **输出属性** - Module 输出

## 生成规则
1. **只输出 Markdown 格式的文档**，不要有任何额外解释
2. 字段描述优先使用 UI label，其次使用 description
3. 枚举类型在验证规则列**列出所有可选值**
4. 如果某个章节没有内容，**省略该章节**
5. 保持简洁，每个字段描述**不超过一行**
6. **必须正确识别 required 数组中的字段作为必填字段**
7. **复杂类型展开**：对于 array/object 类型且包含嵌套 properties 的字段，必须展开描述其内部结构
8. **路径表示法**：使用点号分隔嵌套路径，数组元素使用 `[]` 表示
9. **展开深度**：最多展开 3 层，超过深度标注"详见 Schema 定义"
10. **JSON 字符串字段**：对于 `type: string, format: json` 的字段，类型显示为 `string (JSON)`，并在"特殊字段类型"章节说明输出格式必须是 JSON 字符串

## 质量检查
生成完成后，验证以下项目：

- [ ] 所有 `required` 数组中的字段都在必填表格中
- [ ] 枚举字段列出了所有可选值
- [ ] 字段描述来自 Schema，未编造
- [ ] 表格格式正确（有表头和分隔行）
- [ ] 最佳实践具有可操作性
- [ ] 没有空章节
- [ ] Markdown 格式正确
- [ ] array/object 类型字段已展开内部结构
- [ ] 嵌套字段使用正确的路径表示法
- [ ] 展开深度不超过 3 层
- [ ] `type: string, format: json` 字段已在"特殊字段类型"章节说明

## 示例输出结构

```markdown
# {module_name} Module Skill 文档

## 1. 模块概述
该模块用于...支持...适用于...

## 2. 参数约束

### 必填字段
| 字段名 | 类型 | 验证规则 | 描述 |
|--------|------|----------|------|
| name | string | - | 资源名称 |
| ... | ... | ... | ... |

### 重要可选字段
| 字段名 | 类型 | 默认值 | 描述 |
|--------|------|--------|------|
| ... | ... | ... | ... |

### 参数关联关系
- **enable_xxx = true** 时显示以下字段：xxx_config, xxx_options
- ...

### 特殊字段类型
以下字段类型为 `string (JSON)`，输出时必须是 **JSON 字符串**（而非 JSON 对象）：
| 字段名 | 说明 |
|--------|------|
| policy | 存储桶策略，值必须是 JSON 字符串，如 `"{\"Version\":\"2012-10-17\",\"Statement\":[...]}"` |

## 3. 复杂字段结构

### {complex_field_name} 结构
| 字段路径 | 类型 | 默认值 | 描述 |
|----------|------|--------|------|
| id | string | - | 规则唯一标识符 |
| enabled | boolean | true | 是否启用 |
| filter.prefix | string | - | 对象前缀过滤 |
| filter.tags | object | - | 标签过滤条件 |
| expiration.days | integer | 30 | 过期天数 |
| transition[].days | integer | 30 | 转换天数 |
| transition[].storage_class | string | STANDARD_IA | 目标存储类型 |

## 4. 配置分组

### 基础配置 (basic)
核心必填字段：name, instance_type, ...

### 高级配置 (advanced)
可选功能字段：...

## 5. 数据源映射
| 字段 | 数据源 | 说明 |
|------|--------|------|
| vpc_id | CMDB | 从 CMDB 获取 VPC ID |
| ... | ... | ... |

## 6. 最佳实践
1. **安全配置**：建议保持 `disable_api_termination` 为 true...
2. **高可用**：建议配置 `min_size` >= 2...
3. ...

## 7. 输出属性
| 输出名 | 类型 | 描述 |
|--------|------|------|
| id | string | 资源 ID |
| ... | ... | ... |