---
name: placeholder_standard
layer: foundation
description: 占位符基线规范，定义所有需要用户补充的字段必须使用的统一占位符格式
tags: ["foundation", "placeholder", "standard", "baseline"]
priority: 0
---

# 占位符基线规范

## 标准占位符格式

当某个字段需要用户补充时，**必须使用以下统一格式**：

```
{{PLACEHOLDER:字段名}}
```

## 使用示例

| 场景 | 正确格式 |
|------|----------|
| 备份策略未指定 | `"{{PLACEHOLDER:backup-enabled}}"` |
| 集群名称未指定 | `"{{PLACEHOLDER:cluster-name}}"` |
| VPC ID 未找到 | `"{{PLACEHOLDER:vpc_id}}"` |
| 安全组未指定 | `"{{PLACEHOLDER:security_group_ids}}"` |
| AMI ID 未指定 | `"{{PLACEHOLDER:ami_id}}"` |

## 禁止的格式

以下格式**严格禁止使用**：

| 禁止格式 | 原因 |
|----------|------|
| `""` (空字符串) | 无法识别为占位符 |
| `"<需要用户指定>"` | 格式不统一 |
| `"<PLACEHOLDER>"` | 格式不统一 |
| `"YOUR_VPC_ID"` | 格式不统一 |
| `"xxx-placeholder"` | 格式不统一 |
| `"TBD"` / `"TODO"` | 格式不统一 |
| `null` | 无法区分是占位符还是空值 |

## 规则说明

1. **字段名必须与 Schema 一致**：占位符中的字段名必须与配置 Schema 中定义的字段名完全一致
2. **大小写敏感**：字段名区分大小写
3. **必须是字符串**：占位符值必须是字符串类型，即使原字段是数组或对象
4. **数组字段**：数组类型字段使用 `["{{PLACEHOLDER:字段名}}"]` 格式

## 数组字段示例

```json
{
  "security_group_ids": ["{{PLACEHOLDER:security_group_ids}}"],
  "subnet_ids": ["{{PLACEHOLDER:subnet_ids}}"]
}
```

## 对象字段示例

```json
{
  "tags": {
    "backup-enabled": "{{PLACEHOLDER:backup-enabled}}",
    "cluster-name": "{{PLACEHOLDER:cluster-name}}"
  }
}