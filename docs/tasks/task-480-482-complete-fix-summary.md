# Task 480-482 Plan+Apply执行问题 - 完整修复总结

## 问题描述

在K8s/Agent模式下，plan_and_apply任务在执行confirm apply后无法正常调度到Agent执行，报错：
1. "apply task has no associated plan task"
2. "no snapshot data (snapshot_created_at is nil)"

## 根本原因分析

### 问题1：plan_task_id字段缺失

**原因**：在Agent模式下，有3个地方没有正确处理 `plan_task_id` 字段：

1. **服务端API** (`agent_handler.go` GetTaskData方法)
   - 返回任务数据时，没有包含 `plan_task_id` 字段

2. **服务端API** (`agent_handler.go` GetPlanTask方法)  
   - 返回Plan任务数据时，没有包含 `plan_task_id` 字段

3. **Agent客户端** (`agent_api_client.go` GetPlanTask方法)
   - 解析API响应时，没有解析 `plan_task_id` 字段

4. **Agent数据访问层** (`remote_data_accessor.go` GetTask方法)
   - 从缓存读取任务时，没有解析 `plan_task_id` 字段

### 问题2：快照数据缺失

**原因**：在Agent模式下，快照数据没有通过API传输：

1. **服务端API** (`agent_handler.go` GetPlanTask方法)
   - 返回Plan任务时，没有包含快照字段：
     - `snapshot_created_at`
     - `snapshot_resource_versions`
     - `snapshot_variables`
     - `snapshot_provider_config`

2. **Agent客户端** (`agent_api_client.go` GetPlanTask方法)
   - 解析API响应时，没有解析快照字段

## 已完成的修复

### 1. 修复服务端GetTaskData API 
**文件**：`backend/internal/handlers/agent_handler.go`
**位置**：GetTaskData方法，第82-90行

添加了 `plan_task_id` 字段到响应中：
```go
"task": gin.H{
    "id":           task.ID,
    "workspace_id": task.WorkspaceID,
    "task_type":    task.TaskType,
    "action":       task.TaskType,
    "context":      task.Context,
    "created_at":   task.CreatedAt,
    "plan_task_id": task.PlanTaskID, // 【修复】
},
```

### 2. 修复服务端GetPlanTask API 
**文件**：`backend/internal/handlers/agent_handler.go`
**位置**：GetPlanTask方法，第458-470行

添加了 `plan_task_id` 和所有快照字段到响应中：
```go
"task": gin.H{
    "id":                         task.ID,
    "workspace_id":               task.WorkspaceID,
    "task_type":                  task.TaskType,
    "context":                    task.Context,
    "plan_task_id":               task.PlanTaskID, // 【修复】
    "snapshot_created_at":        task.SnapshotCreatedAt, // 【修复】
    "snapshot_resource_versions": task.SnapshotResourceVersions, // 【修复】
    "snapshot_variables":         task.SnapshotVariables, // 【修复】
    "snapshot_provider_config":   task.SnapshotProviderConfig, // 【修复】
},
```

### 3. 修复Agent API Client 
**文件**：`backend/services/agent_api_client.go`
**位置**：GetPlanTask方法，第214-260行

添加了解析 `plan_task_id` 和所有快照字段的逻辑：
```go
// 【修复】解析 plan_task_id 字段
if planTaskID := getUint(taskData, "plan_task_id"); planTaskID > 0 {
    task.PlanTaskID = &planTaskID
}

// 【修复】解析快照字段
if snapshotCreatedAt, ok := taskData["snapshot_created_at"].(string); ok && snapshotCreatedAt != "" {
    if t, err := time.Parse(time.RFC3339, snapshotCreatedAt); err == nil {
        task.SnapshotCreatedAt = &t
    }
}

if snapshotResourceVersions, ok := taskData["snapshot_resource_versions"].(map[string]interface{}); ok {
    task.SnapshotResourceVersions = models.JSONB(snapshotResourceVersions)
}

if snapshotVariables, ok := taskData["snapshot_variables"].([]interface{}); ok {
    // Convert []interface{} to []WorkspaceVariable
    variables := make([]models.WorkspaceVariable, 0, len(snapshotVariables))
    for _, item := range snapshotVariables {
        if varMap, ok := item.(map[string]interface{}); ok {
            variable := models.WorkspaceVariable{
                ID:           getUint(varMap, "id"),
                WorkspaceID:  getString(varMap, "workspace_id"),
                Key:          getString(varMap, "key"),
                Value:        getString(varMap, "value"),
                VariableType: models.VariableType(getString(varMap, "variable_type")),
                Sensitive:    getBool(varMap, "sensitive"),
                Description:  getString(varMap, "description"),
                ValueFormat:  models.ValueFormat(getString(varMap, "value_format")),
            }
            variables = append(variables, variable)
        }
    }
    task.SnapshotVariables = models.WorkspaceVariableArray(variables)
}

if snapshotProviderConfig, ok := taskData["snapshot_provider_config"].(map[string]interface{}); ok {
    task.SnapshotProviderConfig = models.JSONB(snapshotProviderConfig)
}
```

### 4. 修复RemoteDataAccessor 
**文件**：`backend/services/remote_data_accessor.go`
**位置**：GetTask方法，第189-203行

添加了解析 `plan_task_id` 的逻辑：
```go
// 【修复】解析 plan_task_id 字段
if planTaskID := getUint(taskData, "plan_task_id"); planTaskID > 0 {
    task.PlanTaskID = &planTaskID
}
```

### 5. 防御性编程 
**文件**：`backend/controllers/workspace_task_controller.go`
**位置**：ConfirmApply方法，第498-511行

添加了自动修复逻辑（双重保险）：
```go
// 【新增】如果plan_task_id为空，自动设置为任务自身ID（防御性编程）
if task.PlanTaskID == nil {
    log.Printf("[WARN] Task %d plan_task_id is nil, auto-setting to self", task.ID)
    task.PlanTaskID = &task.ID
    // 立即保存到数据库
    if err := c.db.Model(&task).Update("plan_task_id", task.ID).Error; err != nil {
        log.Printf("[ERROR] Failed to set plan_task_id for task %d: %v", task.ID, err)
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to set plan_task_id",
        })
        return
    }
    log.Printf("[INFO] Task %d plan_task_id auto-fixed to %d", task.ID, task.ID)
}
```

## 修复的文件清单

1. `backend/internal/handlers/agent_handler.go` - 服务端API
2. `backend/services/agent_api_client.go` - Agent客户端
3. `backend/services/remote_data_accessor.go` - Agent数据访问层
4. `backend/controllers/workspace_task_controller.go` - 控制器（防御性编程）

## 下一步操作

### 必须执行：重启后端服务

```bash
# 停止当前服务
# 然后重新启动
cd backend
make run
```

### 测试验证

1. 创建新的plan_and_apply任务
2. 等待Plan阶段完成
3. 点击"Confirm Apply"
4. 验证Apply能够正常执行

## 预期结果

修复后，Agent能够：
1.  正确读取 `plan_task_id` 字段
2.  正确读取所有快照数据
3.  成功执行Apply阶段
4.  不会再报 "apply task has no associated plan task" 错误
5.  不会再报 "no snapshot data" 错误

## 技术总结

这个问题的根源在于：**Agent模式下的API数据传输不完整**

- Local模式：直接访问数据库，所有字段都能正确读取
- Agent模式：通过HTTP API传输数据，需要显式地在API响应中包含所有必要字段

修复的关键是确保：
1. 服务端API返回完整的数据
2. Agent客户端正确解析所有字段
3. 数据访问层正确使用解析后的数据
