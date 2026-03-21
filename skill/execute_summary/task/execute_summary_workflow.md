---
name: execute_summary_workflow
layer: task
description: 执行流程摘要工作流，在 Plan/Apply 阶段完成后分析变更影响和执行结果
tags: ["task", "summary", "impact_analysis", "execute", "plan", "apply"]
priority: 0
domain_tags: ["cmdb", "resource"]
---

# 执行流程摘要工作流

分析 Terraform 执行流程的变更内容，生成结构化的影响分析报告。

## 阶段识别

系统会在上下文中告诉你当前所处的阶段：
- **plan**: Plan 阶段刚完成，分析变更影响、评估风险
- **apply**: Apply 阶段刚完成，总结执行结果，与 Plan 阶段预测对比

## 可用工具

按需调用，不要一次全部调用：

### query_module_resources
查询指定 module 下的完整资源列表（包含子 module）。
**使用场景**: 变更只包含 module 的部分资源时（如部分失败后重新执行），补全完整的 module 资源视图。
- 输入: workspace_id, module_path

### query_cmdb_dependencies
查询哪些资源依赖了指定资源。
**使用场景**: 变更涉及被其他资源引用的资源时，发现影响范围。自行判断依赖字段：
- 安全组: `security_group_ids` 或 `vpc_security_group_ids`
- VPC: `vpc_id`
- 子网: `subnet_id` 或 `subnet_ids`
- IAM Role: `role_arn` 或 `iam_role`
- 输入: resource_id, dependency_field, workspace_id（可选）

### query_resource_attributes
查询指定资源的完整属性。
**使用场景**: 需要查看资源详细配置来判断变更影响。
- 输入: workspace_id, terraform_address 或 cloud_resource_id

### query_state_resources
查询工作空间当前完整资源概览。
**使用场景**: 需要了解整体资源全貌，评估变更在整体架构中的位置。
- 输入: workspace_id

### query_plan_summary（仅 apply 阶段可用）
查询 Plan 阶段的影响分析结果。
**使用场景**: Apply 阶段专用，对比 Plan 预测与实际执行结果。
- 输入: task_id

## 分析流程

### Plan 阶段

1. **识别变更资源**: 从资源变更列表识别类型和操作（create/update/delete）
2. **补全 Module 上下文**: 变更资源属于某 module 且数量不完整时，调用 `query_module_resources`
3. **分析依赖影响**: 对被修改或删除的资源，判断其常见被依赖字段，调用 `query_cmdb_dependencies` 查找依赖方
4. **评估风险等级**:
   - **low**: 仅新增资源，无依赖影响
   - **medium**: 修改资源但无破坏性变更，或有少量依赖方
   - **high**: 删除资源且有依赖方，或修改安全相关配置
   - **critical**: 删除核心网络资源（VPC/子网）、大规模删除、修改 IAM 权限

### Apply 阶段

1. **查询 Plan 预测**: 调用 `query_plan_summary` 获取 Plan 阶段影响分析
2. **对比执行结果**: 实际结果与 Plan 预测对比，标注一致/偏差
3. **确认影响范围**: Plan 预测有依赖影响时，确认实际影响
4. **总结执行结论**: 成功/失败资源、是否有意外变更

## 输出格式

### Plan 阶段

```json
{
  "changes_overview": "变更概述（自然语言）",
  "impact_analysis": {
    "summary": "影响分析摘要",
    "details": [
      {
        "resource": "资源地址",
        "action": "create/update/delete",
        "impact": "影响说明",
        "dependencies_affected": 0
      }
    ]
  },
  "affected_resources": [
    {
      "address": "被影响的依赖资源地址",
      "type": "资源类型",
      "impact": "影响说明",
      "workspace_id": "所在工作空间"
    }
  ],
  "risk_level": "low|medium|high|critical"
}
```

### Apply 阶段

```json
{
  "execution_summary": "执行结果总结（自然语言）",
  "resource_results": [
    {
      "address": "资源地址",
      "action": "create/update/delete",
      "status": "success/failed"
    }
  ],
  "impact_confirmation": {
    "predicted_vs_actual": "Plan 预测与实际结果对比",
    "unexpected_changes": ["意外变更或偏差"]
  },
  "affected_resources": [
    {
      "address": "实际被影响的依赖资源",
      "type": "资源类型",
      "impact": "实际影响说明"
    }
  ]
}
```

## 规则

- 只输出 JSON，不要 markdown 代码块标记
- 不要编造数据，依赖关系必须通过工具查询确认
- 工具查询没发现依赖方时，affected_resources 为空数组
- 每次工具调用要有明确目的，避免不必要查询
- 风险等级判断保守，宁可高估不要低估
