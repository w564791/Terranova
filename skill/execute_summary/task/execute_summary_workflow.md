-----

## name: execute_summary_workflow
layer: task
description: 执行流程摘要工作流，在 Plan/Apply 阶段完成后分析变更影响、风险语义及执行结果，支持人机协同决策
tags: [“task”, “summary”, “impact_analysis”, “risk_analysis”, “execute”, “plan”, “apply”]
priority: 0
domain_tags: [“cmdb”, “resource”, “risk”, “decision”]

# 执行流程摘要工作流（V3 Final）

-----

## 一、阶段识别（强约束）

系统必须在上下文中提供 `stage` 字段，合法值为 `plan` | `apply`。

- `stage` 缺失或非法时，立即返回，禁止继续执行：
  
  ```json
  { "error": "INVALID_STAGE_CONTEXT" }
  ```
- 禁止自行推断阶段

-----

## 二、可用工具

|工具                         |说明                             |适用阶段        |
|---------------------------|-------------------------------|------------|
|`query_module_resources`   |查询指定 module 下的完整资源列表（含子 module）|plan / apply|
|`query_cmdb_dependencies`  |查询哪些资源依赖了指定资源                  |plan / apply|
|`query_resource_attributes`|查询指定资源的完整属性                    |plan / apply|
|`query_state_resources`    |查询工作空间当前完整资源概览                 |plan / apply|
|`query_plan_summary`       |查询 Plan 阶段影响分析结果               |**仅 apply** |

**工具入参说明：**

- `query_module_resources`: workspace_id, module_path
- `query_cmdb_dependencies`: resource_id, dependency_field, workspace_id（可选）
  - 安全组: `security_group_ids` 或 `vpc_security_group_ids`
  - VPC: `vpc_id`
  - 子网: `subnet_id` 或 `subnet_ids`
  - IAM Role: `role_arn` 或 `iam_role`
- `query_resource_attributes`: workspace_id, terraform_address 或 cloud_resource_id
- `query_state_resources`: workspace_id
- `query_plan_summary`: task_id

-----

## 三、工具调用规则（强制）

```yaml
mandatory_calls:
  - condition: action in ["update", "delete"]
    tool: query_cmdb_dependencies
    rule: MUST_CALL
    description: 所有 update/delete 资源必须查询依赖关系，禁止跳过

  - condition: action == "create"
    tool: query_cmdb_dependencies
    rule: OPTIONAL_CALL
    description: 仅当资源类型为共享型（如 security_group、subnet、iam_role）时调用

禁止：
  - 禁止编造依赖关系
  - 禁止在未调用工具的情况下填写 direct_dependencies 非零值
  - 工具未返回依赖时，affected_resources 为空数组，不得填写任何数据
```

-----

## 四、风险语义标准

### 4.1 impact_type（必须从以下选择，且只能选一个）

|值         |适用场景                   |
|----------|-----------------------|
|`network` |VPC、子网、路由表、NAT、负载均衡    |
|`iam`     |IAM Role、Policy、用户、权限边界|
|`security`|安全组、NACLs、WAF、KMS      |
|`compute` |EC2、ECS、Lambda、ASG     |
|`storage` |S3、EBS、RDS、ElastiCache |
|`other`   |不属于以上任何类型              |

### 4.2 risk_factors（可多选，必须从以下集合选择）

|值                          |含义                    |
|---------------------------|----------------------|
|`external_exposure_change` |外部暴露面发生变化（如入站规则、公网 IP）|
|`permission_scope_change`  |权限范围变化（收紧或放宽）         |
|`resource_deletion`        |资源被删除                 |
|`dependency_break`         |依赖链可能断裂               |
|`configuration_drift`      |配置偏移（与预期/基线不符）        |
|`high_blast_radius`        |变更影响面大                |
|`sensitive_resource_change`|涉及敏感资源（生产环境、核心基础设施）   |

-----

## 五、不确定性模型（结构化）

```yaml
uncertainty_schema:
  level: low | medium | high

  reason_code（枚举，level != low 时必须提供）:
    - NO_CMDB_DEPENDENCY_DATA       # CMDB 无依赖数据
    - EXTERNAL_IP_UNKNOWN_USAGE     # 外部 IP/网段是否在用无法确认
    - CROSS_SYSTEM_DEPENDENCY       # 存在跨系统依赖，无法完整追踪
    - AMBIGUOUS_POLICY_CHANGE       # 策略变更语义不明确（收紧 or 放宽）
    - RESOURCE_USAGE_UNVERIFIABLE   # 资源是否仍在使用无法验证

  reason_text: string  # 必须具体描述，禁止使用"信息不足"等模糊表述

输出规则:
  - level == low  时：reason_code 和 reason_text 字段省略，不输出
  - level != low  时：reason_code 和 reason_text 均为必填
```

-----

## 六、blast_radius 判定规则（强约束）

`direct_dependencies` 必须来自 `query_cmdb_dependencies` 查询结果，禁止估算。

```yaml
blast_radius_level 判定（按优先级从高到低匹配）:

  high:
    - direct_dependencies >= 3
    - OR resource_type in [vpc, security_group, subnet, iam_role, nat_gateway, internet_gateway]

  medium:
    - direct_dependencies in [1, 2]
    - OR (direct_dependencies == 0 AND resource_type 为共享型资源且无法排除间接依赖)

  low:
    - direct_dependencies == 0 AND resource_type 为独立资源（如单实例 EC2、独立 EBS）

保守原则:
  - 无法通过工具确认依赖时（uncertainty.reason_code == NO_CMDB_DEPENDENCY_DATA），
    blast_radius_level 至少评估为 medium，indirect_estimate 至少为 medium

indirect_estimate 字段含义:
  - 表示无法通过 CMDB 查到的潜在间接影响，与 blast_radius_level 互补
  - 不替代 blast_radius_level，两者均须输出
```

-----

## 七、风险评分规则（强约束）

### 7.1 risk_level 计算（按优先级从高到低匹配，取最高）

```yaml
critical:
  - risk_factors 包含 external_exposure_change AND blast_radius_level == high
  - OR risk_factors 包含 resource_deletion AND direct_dependencies >= 3

high:
  - blast_radius_level == high
  - OR risk_factors 包含 external_exposure_change（不论 blast_radius）
  - OR risk_factors 包含 sensitive_resource_change
  - OR uncertainty.level == high

medium:
  - blast_radius_level == medium
  - OR risk_factors 包含 configuration_drift
  - OR risk_factors 包含 permission_scope_change AND uncertainty.level == medium

low:
  - blast_radius_level == low
  - AND risk_factors 不包含 external_exposure_change / sensitive_resource_change / resource_deletion
```

### 7.2 confidence 计算

```yaml
low:
  - uncertainty.level == high
  - OR (uncertainty.reason_code == NO_CMDB_DEPENDENCY_DATA AND blast_radius_level != low)

high:
  - uncertainty.level == low AND direct_dependencies 有明确查询数据

medium:
  - 其他所有情况
```

### 7.3 多资源聚合规则

```yaml
变更包含多个资源时，最终 risk_evaluation 按如下规则聚合:
  risk_level:                    取所有资源中最高值
  confidence:                    取所有资源中最低值
  requires_human_confirmation:   任一资源触发则整体为 true
```

-----

## 八、人工确认触发规则（满足任一即触发）

```yaml
requires_human_confirmation_rules:
  - risk_level in ["high", "critical"] AND confidence == "low"
  - uncertainty.level == "high"
  - risk_factors 包含 external_exposure_change
  - risk_factors 包含 resource_deletion AND direct_dependencies > 0
```

-----

## 九、决策场景定义（固定映射，禁止自由生成）

### 9.1 scenario 选择规则（按优先级匹配）

```yaml
优先级: impact_type > action

rules:
  - if: impact_type == "security" AND action == "update"
    scenario: SECURITY_GROUP_CHANGE

  - if: impact_type == "iam"
    scenario: IAM_PERMISSION_CHANGE

  - if: impact_type == "network" AND blast_radius_level == "high"
    scenario: NETWORK_CORE_CHANGE

  - if: action == "delete"
    scenario: RESOURCE_DELETION

  default: RESOURCE_DELETION  # 兜底，禁止使用 GENERIC_CHANGE
```

### 9.2 scenario 固定内容（title 和 label 均为固定值，禁止自由生成）

```yaml
SECURITY_GROUP_CHANGE:
  title: "安全组规则变更确认"
  actions:
    - code: VERIFIED_UNUSED
      label: "确认原规则未被实际使用"
    - code: MIGRATED
      label: "流量来源已完成迁移"
    - code: TEMP_CHANGE
      label: "临时变更，已知风险"
    - code: MISCONFIG_FIX
      label: "修复错误配置"

RESOURCE_DELETION:
  title: "资源删除确认"
  actions:
    - code: DECOMMISSIONED
      label: "资源已废弃下线"
    - code: MIGRATED
      label: "依赖已迁移至其他资源"
    - code: REPLACED
      label: "已由新资源替代"
    - code: ABORT
      label: "终止本次变更"

IAM_PERMISSION_CHANGE:
  title: "IAM 权限变更确认"
  actions:
    - code: APPROVED_SCOPE
      label: "变更范围已审批"
    - code: TEMP_CHANGE
      label: "临时授权，已知风险"
    - code: LEAST_PRIVILEGE_ADJUSTMENT
      label: "最小权限收紧调整"
    - code: ABORT
      label: "终止本次变更"

NETWORK_CORE_CHANGE:
  title: "核心网络架构变更确认"
  actions:
    - code: ARCH_CHANGE_APPROVED
      label: "架构变更已审批"
    - code: MIGRATED
      label: "依赖流量已迁移"
    - code: RISK_ACCEPTED
      label: "风险已知，确认接受"
    - code: ABORT
      label: "终止本次变更"
```

-----

## 十、affected_resources 字段规范

```yaml
affected_resources_schema:
  - address:      string           # 资源完整地址，如 module.xxx.aws_security_group.sg
  - type:         string           # 资源类型，如 aws_security_group
  - relationship: direct|indirect  # direct = CMDB 直接查询到；indirect = 间接推断
  - impact:       string           # 影响说明
  - workspace_id: string           # 所在工作空间（跨空间依赖时尤为重要）
```

-----

## 十一、分析流程

### Plan 阶段

```
1.  验证 stage == "plan"，否则返回 INVALID_STAGE_CONTEXT
2.  解析变更资源列表（resource、action、type）
3.  变更资源属于某 module 且数量不完整时，调用 query_module_resources 补全视图
4.  对所有 action in [update, delete] → 强制调用 query_cmdb_dependencies（禁止跳过）
5.  根据查询结果记录 direct_dependencies（禁止估算）
6.  对每个资源：
    a. 判定 impact_type（从枚举选一个）
    b. 提取 risk_factors（从枚举多选）
    c. 按第六节规则判定 blast_radius_level 和 indirect_estimate
    d. 按第五节规则判定 uncertainty（level + reason_code + reason_text）
    e. 按第七节规则计算 risk_level 和 confidence
7.  按第七节聚合规则生成全局 risk_evaluation
8.  按第八节规则判断 requires_human_confirmation
9.  若 true → 按第九节 scenario 选择规则选定 scenario，输出固定 decision_hints
```

### Apply 阶段

> Apply 阶段只做执行结果分析，不做决策回顾。

```
1. 确认 stage == "apply"，否则返回 INVALID_STAGE_CONTEXT
2. 调用 query_plan_summary 获取 Plan 阶段分析结果（可选，用于对比预测）
3. 对比 Plan 预测 vs 实际执行结果，标记 unexpected_changes
4. 汇总 resource_results（每个资源的执行状态）
5. 汇总 affected_resources（实际影响范围）
```

-----

## 十三、输出格式

### Plan 阶段

```json
{
  "changes_overview": "变更概述（自然语言）",

  "impact_analysis": {
    "summary": "影响分析摘要",
    "details": [
      {
        "resource": "资源地址",
        "action": "create|update|delete",
        "impact": "影响说明",
        "impact_type": "network|iam|security|compute|storage|other",
        "risk_factors": [],
        "uncertainty": {
          "level": "low|medium|high"
          // reason_code 和 reason_text 仅在 level != low 时输出
        },
        "blast_radius": {
          "direct_dependencies": 0,
          "blast_radius_level": "low|medium|high",
          "indirect_estimate": "low|medium|high"
        }
      }
    ]
  },

  "affected_resources": [
    {
      "address": "资源完整地址",
      "type": "资源类型",
      "relationship": "direct|indirect",
      "impact": "影响说明",
      "workspace_id": "所在工作空间"
    }
  ],

  "risk_evaluation": {
    "risk_level": "low|medium|high|critical",
    "confidence": "low|medium|high",
    "requires_human_confirmation": true,

    "decision_hints": [
      {
        "scenario": "SECURITY_GROUP_CHANGE|RESOURCE_DELETION|IAM_PERMISSION_CHANGE|NETWORK_CORE_CHANGE",
        "title": "固定值，来自第九节 scenario 定义",
        "recommended_actions": [
          {
            "code": "来自 scenario.actions",
            "label": "固定值，来自第九节 scenario 定义"
          }
        ]
      }
    ]
  }
}
```

### Apply 阶段

```json
{
  "execution_summary": "执行结果总结（自然语言）",

  "resource_results": [
    {
      "address": "资源地址",
      "action": "create|update|delete",
      "status": "success|failed"
    }
  ],

  "impact_confirmation": {
    "predicted_vs_actual": "一致|存在偏差",
    "unexpected_changes": []
  },

  "affected_resources": [
    {
      "address": "资源完整地址",
      "type": "资源类型",
      "relationship": "direct|indirect",
      "impact": "实际影响说明",
      "workspace_id": "所在工作空间"
    }
  ]
}
```

-----

## 十四、全局输出规则（严格执行）

```yaml
- 只输出 JSON，禁止输出任何解释文字和 markdown 代码块标记
- 禁止编造依赖数据，所有 direct_dependencies 必须来自工具查询
- 所有枚举字段必须使用规定值，禁止自造值
- scenario、title、label 必须使用第九节固定定义，禁止自由生成
- uncertainty.reason_code 必须使用第五节枚举，禁止自由文本
- risk_level / confidence 必须按第七节规则计算，禁止主观判断
- blast_radius_level 必须按第六节规则判定
- 分析过程在内部完成，最终响应只输出 JSON
```