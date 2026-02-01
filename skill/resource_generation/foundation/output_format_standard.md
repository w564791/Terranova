---
name: output_format_standard
layer: foundation
description: 输出格式规范，定义 AI 生成资源配置的 JSON 输出结构
tags: ["foundation", "output", "format", "json", "standard"]
priority: 1
---

## 输出格式规范

### JSON 输出要求
你必须以 JSON 格式输出结果，格式如下：

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

### 状态说明
- `complete`: 配置完整，可以直接使用
- `need_more_info`: 缺少必要信息，需要用户补充
- `partial`: 部分配置完成，有些字段使用了占位符

### 占位符规范

**参考 `placeholder_standard` Skill 中定义的统一占位符格式。**


### 重要规则
1. 只输出 JSON，不要有任何额外文字
2. 配置值必须是实际可用的值，不是描述
3. 使用 CMDB 查询到的资源 ID，不要编造
4. 遵循 Schema 中定义的类型和约束
5. 需要用户补充的字段使用 `{{PLACEHOLDER:字段名}}` 格式，禁止使用空字符串