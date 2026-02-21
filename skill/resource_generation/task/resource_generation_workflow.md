---
name: resource_generation_workflow
layer: task
description: 资源配置生成的通用工作流。根据用户描述和 Module Skill，生成 Terraform 资源配置。
tags: ["task", "generation", "workflow", "terraform", "resource", "config"]
domain_tags: ["cmdb", "matching", "region", "resource-management", "tagging"]
---

# 资源生成工作流

## 任务目标
根据用户的自然语言描述和自动加载的 Module Skill，生成 Terraform 资源配置。

## 输入

### 用户请求
{user_description}

### CMDB 数据（必须直接使用以下资源 ID，不要留空）
{cmdb_data}

**重要规则：**
- 上面列出的资源 ID 是用户已经确认选择的，必须直接使用
- 不要因为"不确定"而将字段留空
- 如果是数组类型字段（如 security_group_ids），直接使用所有提供的 ID

### Module Skill (自动加载)
系统自动加载的 Module Skill 文档，包含：
- **参数约束**：必填字段、可选字段、验证规则
- **参数关联关系**：条件显示、字段依赖
- **数据源映射**：字段与 CMDB 的对应关系
- **最佳实践**：配置建议

## 执行步骤

### 步骤 1: 需求分析
从用户描述中提取关键信息：
- 识别用户明确指定的配置值
- 参考 Module Skill 的"必填字段"确定需要填充的字段
- 识别用户提到的现有资源，提取关键词用于 CMDB 匹配

### 步骤 2: CMDB 资源匹配
从 CMDB 数据中匹配用户提到的资源：
- 精确匹配：用户提供完整资源 ID
- 名称匹配：搜索 name 包含关键词的资源
- 验证资源一致性（同一区域、正确的依赖关系）

### 步骤 3: 配置填充
参考 Module Skill 的"参数约束"填充配置值：
- 填充用户指定的值
- 填充 CMDB 资源 ID
- 填充默认值（参考 Module Skill 的"重要可选字段"）
- 处理条件字段（参考 Module Skill 的"参数关联关系"）

### 步骤 4: 标签生成（重要）
**必须参考 `aws_resource_tagging` Skill 生成资源标签**：

1. **查阅标签规范**：
   - 参考 `aws_resource_tagging` Skill 中定义的"标签详细规范"
   - 识别当前资源类型对应的必须标签、条件必须标签、可选标签

2. **标签值推断**：
   - 从用户描述中提取标签值（如用户说"取名为ken-test"，则 Name=ken-test）
   - 参考 `aws_resource_tagging` Skill 中的"允许值"列表
   - 无法推断时使用 `aws_resource_tagging` Skill 中定义的默认值（如 `novalue`）

3. **标签格式**：
   - 标签必须作为 `tags` 字段包含在配置输出中
   - 标签键值对格式参考 `aws_resource_tagging` Skill 中的"使用示例"

### 步骤 5: 输出生成
生成 JSON 格式的配置输出。

## 输出格式

```json
{
  "status": "complete | need_more_info | partial",
  "config": {},
  "message": "给用户的提示信息",
  "placeholders": []
}
```

### 状态说明
| 状态 | 含义 |
|------|------|
| `complete` | 所有必填字段已填充 |
| `need_more_info` | 必填字段缺失或无法推断 |
| `partial` | 部分字段使用了默认值 |

## 决策逻辑

### 必填字段缺失
- 返回 `need_more_info` 状态
- 在 `placeholders` 中列出缺失字段
- 从 Module Skill 提供建议值

### CMDB 资源未找到
- 返回 `need_more_info` 状态
- 提供 CMDB 中可用的资源列表

### 使用默认值
- 使用 Module Skill 中定义的默认值
- 在 `message` 中说明使用了哪些默认值

## 质量检查
- Module Skill "必填字段"中的所有字段都有值
- 资源 ID 来自 CMDB，未编造
- JSON 格式正确
- status 值为三个有效值之一
- **标签检查**（参考 `aws_resource_tagging` Skill）：
  - 必须包含 `aws_resource_tagging` Skill 中定义的"必须标签"
  - 根据资源类型包含对应的"条件必须标签"
  - 标签值符合 `aws_resource_tagging` Skill 中定义的"允许值"和"值格式"