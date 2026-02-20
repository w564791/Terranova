# Task 518: Agent Cancel Task Incomplete Fix

## Problem Description

**Issue**: When a task is cancelled in the database (status changed to `cancelled`), the agent continues running the task because the cancellation signal is never sent to the agent.

**Root Cause**: The previous fix (Task 510) was incomplete:

1.  The `CancelTaskOnAgent` method exists in `RawAgentCCHandler` 
2.  The agent's `handleCancelTask` function is implemented
3. ❌ The `agentCCHandler` is NOT injected into `WorkspaceTaskController`
4. ❌ The `CancelTask` controller function has a TODO comment and never calls `CancelTaskOnAgent`

## Current Code Analysis

### 1. WorkspaceTaskController.CancelTask (Line ~1100)

```go
// If task is running on Agent, send cancel signal
if task.Status == models.TaskStatusRunning && task.AgentID != nil {
    // Get Agent information
    var agent models.Agent
    if err := c.db.Where("id = ?", *task.AgentID).First(&agent).Error; err == nil {
        log.Printf("[CancelTask] Attempting to send cancel signal to agent %s for task %d", agent.AgentID, taskID)
        
        // TODO: Need to inject agentCCHandler in WorkspaceTaskController
        // Temporarily log, actual sending needs agentCCHandler injected in main.go
        log.Printf("[CancelTask] Agent cancellation signal not yet implemented - need to inject agentCCHandler")
    } else {
        log.Printf("[CancelTask] Failed to get agent info for task %d: %v", taskID, err)
    }
}
```

**Problem**: The code logs that it needs to send the signal but never actually sends it.

### 2. WorkspaceTaskController Structure (Line ~18)

```go
type WorkspaceTaskController struct {
    db            *gorm.DB
    executor      *services.TerraformExecutor
    streamManager *services.OutputStreamManager
    queueManager  *services.TaskQueueManager
    // ❌ MISSING: agentCCHandler field
}
```

### 3. NewWorkspaceTaskController (Line ~27)

```go
func NewWorkspaceTaskController(db *gorm.DB, streamManager *services.OutputStreamManager, queueManager *services.TaskQueueManager) *WorkspaceTaskController {
    // ❌ MISSING: agentCCHandler parameter
    executor := services.NewTerraformExecutor(db, streamManager)
    return &WorkspaceTaskController{
        db:            db,
        executor:      executor,
        streamManager: streamManager,
        queueManager:  queueManager,
    }
}
```

### 4. Router Initialization (backend/internal/router/router_workspace.go)

```go
taskController := controllers.NewWorkspaceTaskController(db, streamManager, queueManager)
// ❌ MISSING: agentCCHandler parameter
```

## Solution Implementation

### Step 1: Update WorkspaceTaskController Structure

Add `agentCCHandler` field to the controller:

```go
type WorkspaceTaskController struct {
    db             *gorm.DB
    executor       *services.TerraformExecutor
    streamManager  *services.OutputStreamManager
    queueManager   *services.TaskQueueManager
    agentCCHandler *handlers.RawAgentCCHandler // NEW
}
```

### Step 2: Update Constructor

Add `agentCCHandler` parameter:

```go
func NewWorkspaceTaskController(
    db *gorm.DB, 
    streamManager *services.OutputStreamManager, 
    queueManager *services.TaskQueueManager,
    agentCCHandler *handlers.RawAgentCCHandler, // NEW
) *WorkspaceTaskController {
    executor := services.NewTerraformExecutor(db, streamManager)
    return &WorkspaceTaskController{
        db:             db,
        executor:       executor,
        streamManager:  streamManager,
        queueManager:   queueManager,
        agentCCHandler: agentCCHandler, // NEW
    }
}
```

### Step 3: Update CancelTask Function

Replace the TODO section with actual cancellation logic:

```go
// If task is running on Agent, send cancel signal
if task.Status == models.TaskStatusRunning && task.AgentID != nil {
    // Get Agent information
    var agent models.Agent
    if err := c.db.Where("id = ?", *task.AgentID).First(&agent).Error; err == nil {
        // Send cancel signal to agent via C&C channel
        if c.agentCCHandler != nil {
            if err := c.agentCCHandler.CancelTaskOnAgent(agent.AgentID, uint(taskID)); err != nil {
                log.Printf("[CancelTask] Failed to send cancel signal to agent %s: %v", agent.AgentID, err)
                // Continue with database update even if agent notification fails
            } else {
                log.Printf("[CancelTask] Sent cancel signal to agent %s for task %d", agent.AgentID, taskID)
            }
        } else {
            log.Printf("[CancelTask] agentCCHandler is nil, cannot send cancel signal")
        }
    } else {
        log.Printf("[CancelTask] Failed to get agent info for task %d: %v", taskID, err)
    }
}
```

### Step 4: Update Router Initialization

Pass `agentCCHandler` to the controller:

```go
// In backend/internal/router/router_workspace.go
func SetupWorkspaceRoutes(r *gin.Engine, db *gorm.DB, streamManager *services.OutputStreamManager, queueManager *services.TaskQueueManager, agentCCHandler *handlers.RawAgentCCHandler) {
    taskController := controllers.NewWorkspaceTaskController(db, streamManager, queueManager, agentCCHandler)
    // ... rest of the code
}
```

### Step 5: Update Main.go

Ensure `agentCCHandler` is passed through the router setup chain.

## Verification Steps

1. Start the backend server
2. Start an agent
3. Create a long-running plan task
4. Cancel the task from UI
5. Verify:
   -  Database status changes to `cancelled`
   -  Agent receives cancel signal (check agent logs)
   -  Terraform process is terminated
   -  Agent status is updated (task removed from current_tasks)
   -  Next task in queue can start

## Expected Log Output

### Server Side:
```
[CancelTask] Attempting to send cancel signal to agent agent-xxx for task 123
[Raw] Sending cancel_task command to agent agent-xxx for task 123
[Raw] Successfully sent message to agent agent-xxx
[CancelTask] Sent cancel signal to agent agent-xxx for task 123
```

### Agent Side:
```
[Server->Agent] Received cancel command for task 123
[Agent] Cancelled task 123 execution
```

## Files to Modify

1.  `backend/controllers/workspace_task_controller.go` - Add field, update constructor, implement cancellation
2.  `backend/internal/router/router_workspace.go` - Pass agentCCHandler to controller
3.  `backend/internal/router/router.go` - Update SetupWorkspaceRoutes signature
4.  `backend/main.go` - Pass agentCCHandler through initialization chain

## Related Code

- `backend/internal/handlers/agent_cc_handler_raw.go` - Already has `CancelTaskOnAgent` method 
- `backend/agent/control/cc_manager.go` - Already has `handleCancelTask` implementation 

## Priority

**CRITICAL** - This is a serious bug that causes:
- Wasted agent resources
- User confusion (UI shows cancelled but agent still running)
- Potential conflicts if user creates new task while old one still running
- Agent may complete cancelled task and update database incorrectly
