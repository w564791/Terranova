# Task 518: Agent Cancel Task Complete Fix

## Summary

Successfully completed the incomplete fix from Task 510 by properly injecting the `agentCCHandler` into the `WorkspaceTaskController`, enabling the server to send cancellation signals to agents when tasks are cancelled.

## Problem

The previous fix (Task 510) was incomplete:
-  `CancelTaskOnAgent` method existed in `RawAgentCCHandler`
-  Agent's `handleCancelTask` function was implemented
- ❌ `agentCCHandler` was NOT injected into `WorkspaceTaskController`
- ❌ `CancelTask` controller had a TODO comment and never called `CancelTaskOnAgent`

This caused agents to continue running tasks even after they were cancelled in the database.

## Solution Implemented

### 1. Updated WorkspaceTaskController (backend/controllers/workspace_task_controller.go)

**Added agentCCHandler field:**
```go
type WorkspaceTaskController struct {
    db             *gorm.DB
    executor       *services.TerraformExecutor
    streamManager  *services.OutputStreamManager
    queueManager   *services.TaskQueueManager
    agentCCHandler interface {
        CancelTaskOnAgent(agentID string, taskID uint) error
    }
}
```

**Updated constructor:**
```go
func NewWorkspaceTaskController(
    db *gorm.DB,
    streamManager *services.OutputStreamManager,
    queueManager *services.TaskQueueManager,
    agentCCHandler interface {
        CancelTaskOnAgent(agentID string, taskID uint) error
    },
) *WorkspaceTaskController {
    // ...
    agentCCHandler: agentCCHandler,
}
```

**Implemented cancellation in CancelTask:**
```go
// If task is running on Agent, send cancel signal
if task.Status == models.TaskStatusRunning && task.AgentID != nil {
    var agent models.Agent
    if err := c.db.Where("id = ?", *task.AgentID).First(&agent).Error; err == nil {
        if c.agentCCHandler != nil {
            if err := c.agentCCHandler.CancelTaskOnAgent(agent.AgentID, uint(taskID)); err != nil {
                log.Printf("[CancelTask] Failed to send cancel signal to agent %s: %v", agent.AgentID, err)
            } else {
                log.Printf("[CancelTask] Sent cancel signal to agent %s for task %d", agent.AgentID, taskID)
            }
        }
    }
}
```

### 2. Updated Router (backend/internal/router/router_workspace.go)

**Added agentCCHandler parameter:**
```go
func setupWorkspaceRoutes(
    api *gin.RouterGroup,
    db *gorm.DB,
    streamManager *services.OutputStreamManager,
    iamMiddleware *middleware.IAMPermissionMiddleware,
    wsHub *websocket.Hub,
    queueManager *services.TaskQueueManager,
    agentCCHandler *handlers.RawAgentCCHandler, // NEW
) {
    taskController := controllers.NewWorkspaceTaskController(db, streamManager, queueManager, agentCCHandler)
    // ...
}
```

### 3. Updated Main Router (backend/internal/router/router.go)

**Added rawCCHandler parameter to Setup:**
```go
func Setup(
    db *gorm.DB,
    streamManager *services.OutputStreamManager,
    wsHub *websocket.Hub,
    agentCCHandler *handlers.AgentCCHandler,
    queueManager *services.TaskQueueManager,
    rawCCHandler *handlers.RawAgentCCHandler, // NEW
) *gin.Engine {
    // ...
    setupWorkspaceRoutes(api, db, streamManager, iamMiddleware, wsHub, queueManager, rawCCHandler)
}
```

### 4. Updated Main.go (backend/main.go)

**Passed rawCCHandler to router:**
```go
// 初始化路由，传入rawCCHandler以支持任务取消功能
r := router.Setup(db, streamManager, wsHub, nil, queueManager, rawCCHandler)
```

## Files Modified

1.  `backend/controllers/workspace_task_controller.go` - Added field, updated constructor, implemented cancellation
2.  `backend/internal/router/router_workspace.go` - Added parameter, passed to controller
3.  `backend/internal/router/router.go` - Added parameter, passed to setupWorkspaceRoutes
4.  `backend/main.go` - Passed rawCCHandler to router.Setup

## How It Works

### Complete Flow:

1. **User cancels task** → POST `/api/v1/workspaces/{id}/tasks/{task_id}/cancel`
2. **Controller receives request** → `WorkspaceTaskController.CancelTask()`
3. **Check if task is running on agent** → `task.Status == Running && task.AgentID != nil`
4. **Get agent info** → Query database for agent details
5. **Send cancel signal** → `agentCCHandler.CancelTaskOnAgent(agent.AgentID, taskID)`
6. **C&C Handler sends WebSocket message** → `cancel_task` command to agent
7. **Agent receives message** → `CCManager.handleCancelTask()`
8. **Agent cancels context** → Calls stored `cancelFunc()` for the task
9. **Terraform process terminates** → Context cancellation propagates
10. **Database updated** → Task status set to `cancelled`

## Expected Log Output

### Server Side:
```
[CancelTask] Sent cancel signal to agent agent-xxx for task 123
[Raw] Sending cancel_task command to agent agent-xxx for task 123
[Raw] Successfully sent message to agent agent-xxx
```

### Agent Side:
```
[Server->Agent] Received cancel command for task 123
[Agent] Cancelled task 123 execution
```

## Testing Checklist

- [ ] Start backend server
- [ ] Start an agent
- [ ] Create a long-running plan task
- [ ] Cancel the task from UI
- [ ] Verify database status changes to `cancelled`
- [ ] Verify agent receives cancel signal (check agent logs)
- [ ] Verify Terraform process is terminated
- [ ] Verify agent status is updated (task removed from current_tasks)
- [ ] Verify next task in queue can start

## Benefits

1. **Resource Efficiency**: Agents no longer waste resources on cancelled tasks
2. **User Experience**: UI accurately reflects task state
3. **System Reliability**: Prevents conflicts from cancelled tasks completing
4. **Queue Management**: Allows next tasks to start immediately after cancellation

## Related Issues

- Task 510: Original incomplete fix
- Task 518: This complete fix

## Priority

**CRITICAL** - This bug caused:
- Wasted agent resources
- User confusion (UI shows cancelled but agent still running)
- Potential conflicts if user creates new task while old one still running
- Agent may complete cancelled task and update database incorrectly
