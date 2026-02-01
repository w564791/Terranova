# Task 498: Agent Mode Resource Status WebSocket Fix

## Issue Description

When using agent mode, tasks complete successfully but resources remain stuck in `pending` status. The root cause is that resource status updates sent via WebSocket from the agent are not being persisted to the database on the server side.

## Root Cause Analysis

### Current Flow (Broken)

1. **Agent Side** (`terraform_executor.go`):
   - `ApplyOutputParser` parses terraform output
   - Calls `UpdateResourceStatus()` on `RemoteDataAccessor`
   - `RemoteDataAccessor.UpdateResourceStatus()` broadcasts to local `OutputStreamManager`

2. **Agent C&C Manager** (`cc_manager.go`):
   - `forwardLogsToServer()` captures all messages from `OutputStreamManager`
   - Sends `resource_status_update` messages to server via WebSocket as `log_stream` type

3. **Server Side** (`agent_cc_handler_raw.go`):
   - `handleLogStream()` receives the message
   - **ONLY forwards to frontend WebSocket clients**
   - **DOES NOT update database** ❌

### The Problem

The `handleLogStream` function treats ALL messages uniformly:

```go
func (h *RawAgentCCHandler) handleLogStream(agentConn *RawAgentConnection, payload map[string]interface{}) {
    // ... extract fields ...
    
    // Forward to OutputStreamManager for frontend WebSocket clients
    if h.streamManager != nil {
        stream := h.streamManager.GetOrCreate(uint(taskID))
        if stream != nil {
            outputMsg := services.OutputMessage{
                Type:      msgType,  // "resource_status_update"
                Line:      line,
                // ...
            }
            stream.Broadcast(outputMsg)  // Only broadcasts, doesn't persist!
        }
    }
}
```

**Missing**: Database update for `resource_status_update` messages!

## Solution

Add special handling for `resource_status_update` messages in `handleLogStream` to persist status changes to the database.

### Implementation Steps

1. **Detect `resource_status_update` messages** in `handleLogStream`
2. **Parse the JSON payload** from the `line` field
3. **Update the database** using the same logic as Local mode
4. **Continue forwarding** to frontend WebSocket clients

### Code Changes

#### File: `backend/internal/handlers/agent_cc_handler_raw.go`

**Modified Function:** `handleLogStream()`

```go
func (h *RawAgentCCHandler) handleLogStream(agentConn *RawAgentConnection, payload map[string]interface{}) {
    taskID, ok := payload["task_id"].(float64)
    if !ok {
        log.Printf("[Raw] Invalid task_id in log_stream message from agent %s", agentConn.AgentID)
        return
    }

    // Extract log message fields
    msgType, _ := payload["type"].(string)
    line, _ := payload["line"].(string)
    lineNum, _ := payload["line_num"].(float64)
    stage, _ := payload["stage"].(string)
    status, _ := payload["status"].(string)

    // **NEW**: Handle resource_status_update messages specially
    if msgType == "resource_status_update" {
        h.handleResourceStatusUpdate(uint(taskID), line)
    }

    // Forward to OutputStreamManager for frontend WebSocket clients
    if h.streamManager != nil {
        stream := h.streamManager.GetOrCreate(uint(taskID))
        if stream != nil {
            outputMsg := services.OutputMessage{
                Type:      msgType,
                Line:      line,
                Timestamp: time.Now(),
                LineNum:   int(lineNum),
                Stage:     stage,
                Status:    status,
            }
            stream.Broadcast(outputMsg)
        }
    }
}

// **NEW**: handleResourceStatusUpdate updates resource status in database
func (h *RawAgentCCHandler) handleResourceStatusUpdate(taskID uint, jsonData string) {
    // Parse JSON data from line field
    var data map[string]interface{}
    if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
        log.Printf("[Raw] Failed to parse resource_status_update JSON: %v", err)
        return
    }

    resourceAddress, _ := data["resource_address"].(string)
    applyStatus, _ := data["apply_status"].(string)
    action, _ := data["action"].(string)

    if resourceAddress == "" || applyStatus == "" {
        log.Printf("[Raw] Invalid resource_status_update data: address=%s, status=%s", resourceAddress, applyStatus)
        return
    }

    // Update database
    now := time.Now()
    updates := map[string]interface{}{
        "apply_status": applyStatus,
        "updated_at":   now,
    }

    if applyStatus == "applying" {
        updates["apply_started_at"] = now
    } else if applyStatus == "completed" {
        updates["apply_completed_at"] = now
    }

    if err := h.db.Model(&models.WorkspaceTaskResourceChange{}).
        Where("task_id = ? AND resource_address = ?", taskID, resourceAddress).
        Updates(updates).Error; err != nil {
        log.Printf("[Raw] Failed to update resource status in DB: %v", err)
        return
    }

    log.Printf("[Raw] Updated resource %s status to %s (task %d)", resourceAddress, applyStatus, taskID)
}
```

## Testing Scenarios

### Scenario 1: Agent Mode Apply with Resource Status Updates

1. Create a plan_and_apply task in agent mode
2. Agent executes terraform apply
3. Agent sends resource_status_update messages via WebSocket
4. **Expected**: 
   - Resources show "applying" status in real-time
   - Resources show "completed" status when done
   - Database is updated correctly
   - Frontend displays correct status

### Scenario 2: Multiple Resources

1. Create task with multiple resources (create, update, delete)
2. Agent executes apply
3. **Expected**:
   - Each resource transitions: pending → applying → completed
   - All status changes persisted to database
   - Frontend shows real-time updates

### Scenario 3: Apply Failure

1. Create task that will fail during apply
2. Agent executes apply
3. **Expected**:
   - Resources that completed show "completed"
   - Resources that didn't start remain "pending"
   - Failed resources show appropriate status

## Verification

After implementing the fix:

1. Check database after task completion:
   ```sql
   SELECT resource_address, apply_status, apply_started_at, apply_completed_at
   FROM workspace_task_resource_changes
   WHERE task_id = <task_id>
   ORDER BY id;
   ```

2. All resources should have:
   - `apply_status = 'completed'` (or appropriate status)
   - `apply_started_at` timestamp
   - `apply_completed_at` timestamp

3. No resources should remain in `pending` status after task completion

## Related Files

- `backend/internal/handlers/agent_cc_handler_raw.go` - Main fix location
- `backend/services/remote_data_accessor.go` - Agent-side status update
- `backend/services/apply_parser_service.go` - Resource status parsing
- `backend/agent/control/cc_manager.go` - WebSocket message forwarding

### Frontend Fix

**文件：** `frontend/src/components/StructuredRunOutput.tsx`

**问题：** 前端 WebSocket 更新逻辑使用 `resource.id` 匹配，但 Agent 模式下的消息中没有 `resource_id`，导致实时更新失败。

**修复：** 改用 `resource_address` 作为主要匹配字段（Agent 和 Local 模式都支持）

```typescript
// 修复前：只使用 resource_id 匹配
if (resource.id === data.resource_id) {
  // 更新...
}

// 修复后：优先使用 resource_address 匹配
if (resource.resource_address === data.resource_address) {
  console.log(`Updating resource ${resource.resource_address}: ${resource.apply_status} -> ${data.apply_status}`);
  return {
    ...resource,
    apply_status: data.apply_status,
    apply_started_at: data.apply_started_at,
    apply_completed_at: data.apply_completed_at
  };
}
// 备用：使用 resource_id 匹配（Local 模式）
if (data.resource_id && resource.id === data.resource_id) {
  // 更新...
}
```

## 完整修复总结

### 修复了两个问题：

1. **后端问题（主要）：** 服务器接收到 Agent 的资源状态更新后，只转发给前端，不写入数据库
   - **症状：** 刷新页面后资源状态丢失，全部显示 pending
   - **修复：** 在 `agent_cc_handler_raw.go` 中添加数据库更新逻辑

2. **前端问题（次要）：** 前端 WebSocket 更新使用错误的匹配字段
   - **症状：** 任务执行时看不到实时状态变化（applying/completed）
   - **修复：** 在 `StructuredRunOutput.tsx` 中改用 `resource_address` 匹配

### 修复后的完整流程：

1. **Agent 端：** 解析 terraform 输出 → 发送 `resource_status_update` 到服务器
2. **服务器端：** 接收消息 → **写入数据库** → 转发给前端
3. **前端：** 接收 WebSocket 消息 → **通过 resource_address 匹配** → 实时更新 UI
4. **刷新页面：** 从数据库读取 → 显示正确的状态

## Impact

- **Positive**: Resource status tracking works correctly in agent mode
- **Positive**: Frontend displays accurate real-time status (both during execution and after refresh)
- **Positive**: Database consistency maintained
- **No Breaking Changes**: Existing functionality preserved
- **Backward Compatible**: Works with existing agents and Local mode

## Date

2025-01-06
