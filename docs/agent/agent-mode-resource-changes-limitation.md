# Agent Mode Resource-Changes 功能限制说明

## 当前状态

### Agent Mode 重构完成情况
 **Phase 6 完成**：所有核心功能已重构
- Plan 执行 
- Apply 执行 
- State 管理 
- Workspace 锁定 
- 任务状态更新 

### 已知限制

#### Resource-Changes 详细视图在 Agent 模式下不可用

**原因**：
1. Agent 模式下，任务在 Agent 机器上执行
2. `plan_data` 和 `plan_json` 是大字段（50KB+），不通过 API 传输
3. 没有 `plan_json`，无法解析生成 `workspace_task_resource_changes` 数据
4. 因此 `/api/v1/workspaces/{workspace_id}/tasks/{task_id}/resource-changes` 返回空

**影响范围**：
- ❌ Structured Run Output 视图不可用
- ❌ 详细的资源变更列表不可用
-  Plan Summary（add/change/destroy 统计）仍然可用
-  Plan Output（文本日志）完整可用
-  核心 Plan/Apply 功能完全正常

## 技术细节

### Local 模式 vs Agent 模式

| 功能 | Local 模式 | Agent 模式 |
|------|-----------|-----------|
| Plan 执行 |  |  |
| Apply 执行 |  |  |
| State 管理 |  |  |
| Plan Summary |  |  |
| Plan Output |  |  |
| plan_data 存储 |  | ❌ |
| plan_json 存储 |  | ❌ |
| Resource-Changes 详细视图 |  | ❌ |

### 为什么不在 Agent 模式存储大字段

1. **网络传输成本**：plan_json 可能有几百 KB，通过 API 传输效率低
2. **存储需求**：Agent 模式通常用于大规模部署，存储所有 plan_json 会占用大量空间
3. **实际需求**：大多数情况下，Plan Summary 和 Plan Output 已经足够

## 解决方案选项

### 选项 1：接受限制（推荐）
- Agent 模式专注于核心 Plan/Apply 功能
- Resource-Changes 详细视图仅在 Local 模式可用
- 这是合理的权衡

### 选项 2：扩展 Agent API（未来工作）
如果确实需要 Resource-Changes 视图，可以：
1. 添加 API 端点上传 plan_json
2. 在服务器端存储并解析
3. 需要考虑存储和性能影响

### 选项 3：Agent 端解析（复杂）
在 Agent 端解析并只上传解析结果，但这需要：
1. Agent 端实现完整的解析逻辑
2. 定义解析结果的数据格式
3. 增加 Agent 的复杂度

## 当前建议

**对于需要 Resource-Changes 详细视图的 workspace，使用 Local 模式。**

Agent 模式适用于：
- 大规模部署
- 只需要核心 Plan/Apply 功能
- 不需要详细的资源变更视图

## 相关文档

- `docs/agent/agent-mode-complete-refactoring-plan.md` - 完整重构计划
- `docs/agent/phase6-terraform-executor-refactoring.md` - Phase 6 实施指南
- `docs/agent/plan-data-save-bug-diagnosis.md` - Plan Data 保存问题诊断
