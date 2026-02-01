# Task 510: Agent Cancel Task Bug Fix

## Problem Description

**Critical Bug**: When a task is cancelled in the database (status changed to `cancelled`), the agent continues running the task because:

1. The `CancelTask` controller function in `workspace_task_controller.go` only updates the database status
2. No cancellation signal is sent to the agent via the C&C WebSocket channel
3. The agent's `handleCancelTask` function exists but is not implemented (has TODO comment)

## Root Cause Analysis

### Server Side (backend/controllers/workspace_task_controller.go)
```go
func (c *WorkspaceTaskController) CancelTask(ctx *gin.Context) {
    // ... validation code ...
    
    // Updates database only
    task.Status = models.TaskStatusCancelled
    task.CompletedAt = timePtr(time.Now())
    task.ErrorMessage = "Task cancelled by user"
    
    if err := c.db.Save(&task).Error; err != nil {
        // ...
    }
    
    // ❌ MISSING: No signal sent to agent if task is running on agent
}
```

### Server C&C Handler (backend/internal/handlers/agent_cc_handler_raw.go)
- Has `SendTaskToAgent` method 
- Has `BroadcastCredentialsRefresh` method   
- **Missing `CancelTaskOnAgent` method** ❌

### Agent Side (backend/agent/control/cc_manager.go)
```go
// handleCancelTask handles cancel_task command
func (m *CCManager) handleCancelTask(payload map[string]interface{}) {
    taskID, ok := payload["task_id"].(float64)
    if !ok {
        log.Printf("Invalid task_id in cancel_task message")
        return
    }

    log.Printf("Received cancel command for task %d", uint(taskID))

    // ❌ TODO: Implement task cancellation
}
```

## Solution Design

### 1. Add CancelTaskOnAgent Method to RawAgentCCHandler

Add a new method to send cancel signals to agents:

```go
// CancelTaskOnAgent sends a cancel_task command to the agent running the task
func (h *RawAgentCCHandler) CancelTaskOnAgent(agentID string, taskID uint) error {
    h.mu.RLock()
    agentConn, ok := h.agents[agentID]
    h.mu.RUnlock()

    if !ok {
        return fmt.Errorf("agent %s not connected", agentID)
    }

    msg := CCMessage{
        Type: "cancel_task",
        Payload: map[string]interface{}{
            "task_id": taskID,
        },
    }

    return h.sendMessage(agentConn, msg)
}
```

### 2. Update CancelTask Controller

Modify the controller to send cancellation signal to agent:

```go
func (c *WorkspaceTaskController) CancelTask(ctx *gin.Context) {
    // ... existing validation code ...
    
    // Check if task is running on an agent
    if task.Status == models.TaskStatusRunning && task.AgentID != nil {
        // Send cancel signal to agent via C&C channel
        if c.agentCCHandler != nil {
            agentIDStr := fmt.Sprintf("%d", *task.AgentID) // Convert to string
            if err := c.agentCCHandler.CancelTaskOnAgent(agentIDStr, uint(taskID)); err != nil {
                log.Printf("[CancelTask] Failed to send cancel signal to agent: %v", err)
                // Continue with database update even if agent notification fails
            } else {
                log.Printf("[CancelTask] Sent cancel signal to agent for task %d", taskID)
            }
        }
    }
    
    // Update database status
    task.Status = models.TaskStatusCancelled
    task.CompletedAt = timePtr(time.Now())
    task.ErrorMessage = "Task cancelled by user"
    
    if err := c.db.Save(&task).Error; err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel task"})
        return
    }
    
    // ... rest of the code ...
}
```

### 3. Implement Agent-Side Task Cancellation

The agent needs to:
1. Maintain a map of running task contexts
2. Cancel the context when receiving cancel signal
3. Handle graceful termination of Terraform process

```go
type CCManager struct {
    // ... existing fields ...
    
    // Task cancellation support
    taskContexts map[uint]context.CancelFunc
    taskMutex    sync.RWMutex
}

// executeTask - Modified to support cancellation
func (m *CCManager) executeTask(taskID uint, workspaceID string, action string) {
    // Create cancellable context
    ctx, cancel := context.WithTimeout(m.ctx, 60*time.Minute)
    
    // Store cancel function
    m.taskMutex.Lock()
    if m.taskContexts == nil {
        m.taskContexts = make(map[uint]context.CancelFunc)
    }
    m.taskContexts[taskID] = cancel
    m.taskMutex.Unlock()
    
    // Ensure cleanup
    defer func() {
        m.taskMutex.Lock()
        delete(m.taskContexts, taskID)
        m.taskMutex.Unlock()
        cancel()
    }()
    
    // ... rest of execution code using ctx ...
}

// handleCancelTask - Implement cancellation
func (m *CCManager) handleCancelTask(payload map[string]interface{}) {
    taskID, ok := payload["task_id"].(float64)
    if !ok {
        log.Printf("Invalid task_id in cancel_task message")
        return
    }

    tid := uint(taskID)
    log.Printf("[Server->Agent] Received cancel command for task %d", tid)

    // Find and cancel the task context
    m.taskMutex.RLock()
    cancelFunc, exists := m.taskContexts[tid]
    m.taskMutex.RUnlock()

    if !exists {
        log.Printf("[Agent] Task %d not found in running tasks (may have already completed)", tid)
        return
    }

    // Cancel the task context
    cancelFunc()
    log.Printf("[Agent] Cancelled task %d execution", tid)
}
```

## Implementation Steps

1.  Add `CancelTaskOnAgent` method to `RawAgentCCHandler`
2.  Add `agentCCHandler` field to `WorkspaceTaskController`
3.  Update `CancelTask` controller to send cancel signal
4.  Add task context management to agent's `CCManager`
5.  Implement `handleCancelTask` in agent
6.  Ensure Terraform executor respects context cancellation
7.  Test the complete flow

## Testing Plan

1. Start an agent
2. Create a long-running plan task (e.g., with sleep in provider)
3. Cancel the task from UI
4. Verify:
   - Database status changes to `cancelled`
   - Agent receives cancel signal
   - Terraform process is terminated
   - Agent status is updated (task removed from current_tasks)
   - Next task in queue can start

## Files to Modify

1. `backend/internal/handlers/agent_cc_handler_raw.go` - Add CancelTaskOnAgent method
2. `backend/controllers/workspace_task_controller.go` - Send cancel signal to agent
3. `backend/agent/control/cc_manager.go` - Implement task cancellation
4. `backend/services/terraform_executor.go` - Ensure context cancellation is respected

## Related Issues

- Task 510: Original bug report
- Similar to credentials refresh broadcast mechanism
- Related to task queue management

## Priority

**CRITICAL** - This is a serious bug that can cause:
- Wasted agent resources
- Confusion for users (UI shows cancelled but agent still running)
- Potential conflicts if user creates new task while old one still running
- Agent may complete cancelled task and update database incorrectly
