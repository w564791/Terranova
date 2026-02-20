# Task #474 状态不一致问题修复方案

## 问题总结

1. **Confirm Apply 按钮不显示**：前端检查 `plan_completed`，后端使用 `apply_pending`
2. **needs_attention 过滤失效**：过滤器不包含 `apply_pending` 状态

## 修复方案：统一使用 `apply_pending`

### 影响范围

 **所有执行模式都使用相同的状态设置逻辑**：
- Local 模式：`terraform_executor.go` 中的 `ExecutePlan`
- Agent 模式：Agent 也使用 `terraform_executor.go`（通过 `NewTerraformExecutorWithAccessor`）
- K8s 模式：K8s Job 也使用相同的 executor 代码

**结论**：状态设置逻辑在 `terraform_executor.go` 中统一实现，三种模式都会设置为 `apply_pending`，因此修复方案对所有模式都有效。

### 修复步骤

#### 步骤1：修复前端 Confirm Apply 按钮显示逻辑

**文件**：`frontend/src/pages/TaskDetail.tsx`

**位置**：第 587 行

**修改前**：
```tsx
{task.status === 'plan_completed' && task.task_type === 'plan_and_apply' && canConfirmApply && (
  <button
    className={styles.confirmApplyButton}
    onClick={() => handleActionWithComment('confirm_apply')}
  >
    Confirm Apply
  </button>
)}
```

**修改后**：
```tsx
{task.status === 'apply_pending' && task.task_type === 'plan_and_apply' && canConfirmApply && (
  <button
    className={styles.confirmApplyButton}
    onClick={() => handleActionWithComment('confirm_apply')}
  >
    Confirm Apply
  </button>
)}
```

**说明**：将状态检查从 `plan_completed` 改为 `apply_pending`

** 重要提示**：这个修改只是让按钮显示出来。实际上，当前代码中 ConfirmApply 接口仍然检查 `plan_completed` 状态（第 445 行），所以还需要同步修改后端验证逻辑。

---

#### 步骤2：修复后端 ConfirmApply 状态验证逻辑

**文件**：`backend/controllers/workspace_task_controller.go`

**位置**：第 445-450 行

**修改前**：
```go
// 验证任务状态
if task.Status != models.TaskStatusPlanCompleted {
    ctx.JSON(http.StatusBadRequest, gin.H{
        "error":          "Task is not in plan_completed status",
        "current_status": task.Status,
    })
    return
}
```

**修改后**：
```go
// 验证任务状态
if task.Status != models.TaskStatusApplyPending {
    ctx.JSON(http.StatusBadRequest, gin.H{
        "error":          "Task is not in apply_pending status",
        "current_status": task.Status,
    })
    return
}
```

**说明**：将状态验证从 `plan_completed` 改为 `apply_pending`

** 队列机制确认**：
- ConfirmApply 成功后，会调用 `c.queueManager.TryExecuteNextTask(workspace.WorkspaceID)`
- 这会通知队列管理器尝试执行下一个任务
- Agent/K8s Agent 通过队列管理器获取任务，因此可以正常接收到 Apply 任务
- 适用于所有三种执行模式（Local/Agent/K8s）

---

#### 步骤3：修复后端 needs_attention 过滤逻辑

**文件**：`backend/controllers/workspace_task_controller.go`

**位置**：第 234-236 行

**修改前**：
```go
case "needs_attention":
    query = query.Where("status IN ?", []string{"requires_approval", "plan_completed"})
```

**修改后**：
```go
case "needs_attention":
    query = query.Where("status IN ?", []string{"requires_approval", "apply_pending"})
```

**说明**：将过滤条件从 `plan_completed` 改为 `apply_pending`

---

#### 步骤4：更新前端状态分类逻辑（可选但推荐）

**文件**：`frontend/src/pages/WorkspaceDetail.tsx`

**位置**：第 1006-1020 行（`getStatusCategory` 函数）

**修改前**：
```tsx
const getStatusCategory = (status: string): string => {
  if (status === 'success' || status === 'applied') {
    return 'success';
  }
  if (status === 'requires_approval' || status === 'plan_completed') {
    return 'attention';
  }
  if (status === 'failed') {
    return 'error';
  }
  if (status === 'running') {
    return 'running';
  }
  if (status === 'pending' || status === 'apply_pending') {
    return 'pending';
  }
  return 'neutral';
};
```

**修改后**：
```tsx
const getStatusCategory = (status: string): string => {
  if (status === 'success' || status === 'applied') {
    return 'success';
  }
  if (status === 'requires_approval' || status === 'apply_pending') {
    return 'attention';
  }
  if (status === 'failed') {
    return 'error';
  }
  if (status === 'running') {
    return 'running';
  }
  if (status === 'pending') {
    return 'pending';
  }
  return 'neutral';
};
```

**说明**：
- 将 `apply_pending` 从 `pending` 分类移到 `attention` 分类
- 移除 `plan_completed`（因为从未使用）
- 这样 `apply_pending` 状态的任务会显示黄色指示条，更符合"需要注意"的语义

---

#### 步骤5：更新状态显示文本（可选但推荐）

**文件**：`frontend/src/pages/WorkspaceDetail.tsx`

**位置**：第 982-1004 行（`getFinalStatus` 函数）

**修改前**：
```tsx
const getFinalStatus = (run: Run): string => {
  if (run.status === 'plan_completed') {
    return 'Apply Pending';
  } else if (run.status === 'success' || run.status === 'applied') {
    // ...
  }
  // ...
};
```

**修改后**：
```tsx
const getFinalStatus = (run: Run): string => {
  if (run.status === 'apply_pending') {
    return 'Apply Pending';
  } else if (run.status === 'success' || run.status === 'applied') {
    // ...
  }
  // ...
};
```

**说明**：将状态文本映射从 `plan_completed` 改为 `apply_pending`

---

#### 步骤6：清理未使用的状态定义（可选）

**文件**：`backend/internal/models/workspace.go`

**位置**：第 30-40 行

**修改前**：
```go
const (
    TaskStatusPending       TaskStatus = "pending"
    TaskStatusWaiting       TaskStatus = "waiting"
    TaskStatusRunning       TaskStatus = "running"
    TaskStatusPlanCompleted TaskStatus = "plan_completed"  // Plan完成，等待Apply确认
    TaskStatusApplyPending  TaskStatus = "apply_pending"   // 等待用户确认Apply
    TaskStatusSuccess       TaskStatus = "success"
    TaskStatusApplied       TaskStatus = "applied"
    TaskStatusFailed        TaskStatus = "failed"
    TaskStatusCancelled     TaskStatus = "cancelled"
)
```

**修改后**：
```go
const (
    TaskStatusPending       TaskStatus = "pending"
    TaskStatusWaiting       TaskStatus = "waiting"
    TaskStatusRunning       TaskStatus = "running"
    TaskStatusApplyPending  TaskStatus = "apply_pending"   // Plan完成，等待用户确认Apply
    TaskStatusSuccess       TaskStatus = "success"
    TaskStatusApplied       TaskStatus = "applied"
    TaskStatusFailed        TaskStatus = "failed"
    TaskStatusCancelled     TaskStatus = "cancelled"
)
```

**说明**：移除 `TaskStatusPlanCompleted` 定义，因为从未被使用

---

### 验证步骤

#### 1. 验证 Confirm Apply 按钮显示

```bash
# 1. 创建一个 plan_and_apply 任务
curl -X POST http://localhost:8080/api/v1/workspaces/ws-mb7m9ii5ey/tasks/plan \
  -H "Content-Type: application/json" \
  -d '{"run_type": "plan_and_apply", "description": "Test apply_pending fix"}'

# 2. 等待 Plan 完成，状态变为 apply_pending

# 3. 访问任务详情页面
# http://localhost:5173/workspaces/ws-mb7m9ii5ey/tasks/{task_id}

# 4. 验证：应该看到 "Confirm Apply" 按钮
```

#### 2. 验证 needs_attention 过滤器

```bash
# 1. 访问 Runs 页面
# http://localhost:5173/workspaces/ws-mb7m9ii5ey?tab=runs

# 2. 点击 "Needs Attention" 过滤按钮

# 3. 验证：apply_pending 状态的任务应该出现在列表中
```

#### 3. 验证三种执行模式

**Local 模式**：
```bash
# 设置 workspace 为 local 模式
curl -X PUT http://localhost:8080/api/v1/workspaces/ws-mb7m9ii5ey \
  -H "Content-Type: application/json" \
  -d '{"execution_mode": "local"}'

# 创建任务并验证
```

**Agent 模式**：
```bash
# 设置 workspace 为 agent 模式
curl -X PUT http://localhost:8080/api/v1/workspaces/ws-mb7m9ii5ey \
  -H "Content-Type: application/json" \
  -d '{"execution_mode": "agent", "agent_pool_id": "pool-xxx"}'

# 创建任务并验证
```

**K8s 模式**：
```bash
# 设置 workspace 为 k8s 模式
curl -X PUT http://localhost:8080/api/v1/workspaces/ws-mb7m9ii5ey \
  -H "Content-Type: application/json" \
  -d '{"execution_mode": "k8s", "k8s_config_id": 1}'

# 创建任务并验证
```

---

### 回归测试清单

- [ ] Local 模式：Plan+Apply 任务显示 Confirm Apply 按钮
- [ ] Agent 模式：Plan+Apply 任务显示 Confirm Apply 按钮
- [ ] K8s 模式：Plan+Apply 任务显示 Confirm Apply 按钮
- [ ] needs_attention 过滤器包含 apply_pending 任务
- [ ] apply_pending 任务显示黄色指示条（attention 分类）
- [ ] 状态文本显示为 "Apply Pending"
- [ ] Confirm Apply 功能正常工作
- [ ] 现有的 Plan Only 任务不受影响

---

### 风险评估

**低风险**：
- 修改的是前端显示逻辑和后端过滤逻辑
- 不涉及核心业务逻辑修改
- 不需要数据库迁移
- 不影响现有任务的执行

**影响范围**：
- 前端：任务详情页面、任务列表页面
- 后端：任务列表过滤接口
- 所有执行模式（local/agent/k8s）

---

### 实施建议

1. **先修复核心问题**（步骤1和步骤2）：
   - 这两个修改是必须的，直接解决用户反馈的问题
   
2. **然后优化显示**（步骤3和步骤4）：
   - 这些修改提升用户体验，但不是必须的
   
3. **最后清理代码**（步骤5）：
   - 移除未使用的定义，保持代码整洁

4. **充分测试**：
   - 在所有三种执行模式下测试
   - 验证现有功能不受影响

---

### 相关文件清单

**前端**：
- `frontend/src/pages/TaskDetail.tsx` - 任务详情页面
- `frontend/src/pages/WorkspaceDetail.tsx` - 工作空间详情页面（包含任务列表）

**后端**：
- `backend/controllers/workspace_task_controller.go` - 任务控制器
- `backend/internal/models/workspace.go` - 模型定义
- `backend/services/terraform_executor.go` - 执行器（状态设置逻辑）

**文档**：
- `docs/task-474-status-flow-issue-analysis.md` - 问题分析报告
- `docs/task-474-status-fix-implementation-plan.md` - 本修复方案
