# Agent v3.2 Phase 5: Task Scheduler Implementation - COMPLETE

## Overview

Phase 5 implements the task scheduling system that enables the server to push tasks to agents via the C&C (Command & Control) WebSocket channel. This completes the Agent v3.2 architecture by connecting the task queue manager with the agent execution system.

## Implementation Date

October 30, 2025

## What Was Implemented

### 1. TaskQueueManager Agent Mode Support

**File**: `backend/services/task_queue_manager.go`

#### Changes:
- Added `AgentCCHandler` interface to TaskQueueManager struct
- Created `AgentCCHandler` interface with methods:
  - `SendTaskToAgent(agentID string, taskID uint, workspaceID string, action string) error`
  - `IsAgentAvailable(agentID string, taskType models.TaskType) bool`
  - `GetConnectedAgents() []string`
- Added `SetAgentCCHandler()` method for dependency injection
- Implemented `pushTaskToAgent()` method with:
  - Agent pool validation
  - Online agent discovery
  - Agent availability checking
  - Task assignment with retry logic
  - Exponential backoff for failures

#### Execution Flow:
```
TryExecuteNextTask()
  ├─> Check workspace execution mode
  ├─> If Agent mode:
  │   └─> pushTaskToAgent()
  │       ├─> Validate pool assignment
  │       ├─> Find online agents in pool
  │       ├─> Check agent availability
  │       ├─> Send task via C&C channel
  │       └─> Update task status to running
  ├─> If K8s mode:
  │   └─> createK8sJobForTask()
  └─> If Local mode:
      └─> executeTask()
```

### 2. Main.go Integration

**File**: `backend/main.go`

#### Changes:
- Initialize `AgentCCHandler` after database setup
- Inject `AgentCCHandler` into `TaskQueueManager` via `SetAgentCCHandler()`
- Pass `AgentCCHandler` to router setup

#### Initialization Order:
```go
1. Database initialization
2. Task queue manager creation
3. Agent C&C handler creation
4. Inject handler into queue manager
5. Router setup with handler
```

### 3. Router Updates

**Files**: 
- `backend/internal/router/router.go`
- `backend/internal/router/router_agent.go`

#### Changes:
- Updated `Setup()` function signature to accept `agentCCHandler` parameter
- Modified `setupAgentAPIRoutes()` to use passed handler instead of creating new one
- Ensures single AgentCCHandler instance is shared across the system

### 4. Agent-Side Task Execution

**File**: `backend/agent/control/cc_manager.go`

#### New Methods:
- `executeTask(taskID uint, workspaceID string, action string)` - Main execution logic
- `sendTaskCompletedNotification(taskID uint)` - Success notification
- `sendTaskFailedNotification(taskID uint, errorMsg string)` - Failure notification

#### Execution Flow:
```
handleRunTask() receives task from server
  └─> executeTask() in goroutine
      ├─> Update agent status (planRunning++ or applyRunning=true)
      ├─> Load task data via RemoteDataAccessor
      ├─> Execute plan or apply via TerraformExecutor
      ├─> Send completion/failure notification
      └─> Update agent status (cleanup)
```

#### Agent Status Tracking:
- `planRunning`: Number of concurrent plan tasks
- `applyRunning`: Boolean for apply task (exclusive)
- `currentTasks`: Array of task IDs currently executing
- Status updated in heartbeat messages every 10 seconds

## Key Features

### 1. Intelligent Agent Selection

The system selects agents based on:
- **Pool membership**: Only agents in the workspace's assigned pool
- **Online status**: Only agents with status="online"
- **Availability**: Checks if agent can accept the task type
  - Plan tasks: `planRunning < planLimit`
  - Apply tasks: `planRunning == 0 && !applyRunning`

### 2. Retry Logic with Exponential Backoff

Failed task assignments retry with delays:
- 1st retry: 5 seconds
- 2nd retry: 10 seconds
- 3rd retry: 20 seconds
- 4th retry: 40 seconds
- 5th+ retry: 60 seconds (max)

### 3. Task Status Management

Task status transitions:
```
pending → running (when pushed to agent)
running → success/failed (when agent completes)
```

For plan_and_apply tasks:
```
pending → running (plan phase)
running → plan_completed
plan_completed → apply_pending
apply_pending → running (apply phase)
running → applied/failed
```

### 4. Agent Capacity Management

Agents report capacity in heartbeat:
- `plan_limit`: Maximum concurrent plan tasks (default: 3)
- `plan_running`: Current plan tasks
- `apply_running`: Whether apply is running

Server respects these limits when assigning tasks.

## Testing Checklist

- [ ] Create workspace with Agent execution mode
- [ ] Assign agent pool to workspace
- [ ] Start agent with pool token
- [ ] Verify agent connects to C&C channel
- [ ] Create plan task
- [ ] Verify task is pushed to agent
- [ ] Verify agent executes task
- [ ] Verify task status updates
- [ ] Create plan_and_apply task
- [ ] Verify apply phase execution
- [ ] Test with multiple agents
- [ ] Test agent unavailability scenarios
- [ ] Test retry logic
- [ ] Verify agent status tracking

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                         Server                               │
│                                                              │
│  ┌──────────────────┐         ┌──────────────────┐         │
│  │ TaskQueueManager │────────>│ AgentCCHandler   │         │
│  │                  │         │                  │         │
│  │ - pushTaskToAgent│         │ - SendTaskToAgent│         │
│  │ - Agent mode     │         │ - IsAgentAvailable│        │
│  │   detection      │         │ - Connected agents│        │
│  └──────────────────┘         └──────────────────┘         │
│           │                            │                     │
│           │                            │ WebSocket           │
│           v                            v                     │
│  ┌──────────────────────────────────────────────┐          │
│  │         Database (Task Status)                │          │
│  └──────────────────────────────────────────────┘          │
└─────────────────────────────────────────────────────────────┘
                                 │
                                 │ C&C WebSocket
                                 │ (run_task message)
                                 v
┌─────────────────────────────────────────────────────────────┐
│                         Agent                                │
│                                                              │
│  ┌──────────────────┐         ┌──────────────────┐         │
│  │   CCManager      │────────>│ TerraformExecutor│         │
│  │                  │         │                  │         │
│  │ - handleRunTask  │         │ - ExecutePlan    │         │
│  │ - executeTask    │         │ - ExecuteApply   │         │
│  │ - Status tracking│         │                  │         │
│  └──────────────────┘         └──────────────────┘         │
│           │                            │                     │
│           │                            v                     │
│           │                   ┌──────────────────┐          │
│           │                   │RemoteDataAccessor│          │
│           │                   │                  │          │
│           │                   │ - LoadTaskData   │          │
│           │                   │ - SaveState      │          │
│           │                   │ - UploadLogs     │          │
│           │                   └──────────────────┘          │
│           │                            │                     │
│           v                            v                     │
│  ┌──────────────────────────────────────────────┐          │
│  │    Notifications (task_completed/failed)      │          │
│  └──────────────────────────────────────────────┘          │
└─────────────────────────────────────────────────────────────┘
```

## Integration Points

### 1. With Existing Systems

- **Task Queue**: Seamlessly integrates with existing queue management
- **K8s Mode**: Coexists with K8s execution mode
- **Local Mode**: Coexists with local execution mode
- **DataAccessor**: Uses RemoteDataAccessor for agent-side data access

### 2. With Agent v3.2 Components

- **Phase 1**: Uses DataAccessor interface
- **Phase 2**: Uses Agent API endpoints
- **Phase 3**: Uses C&C WebSocket channel
- **Phase 4**: Uses Pool Token authentication

## Error Handling

### Server-Side

1. **No pool assigned**: Retry after 10 seconds
2. **No online agents**: Retry after 15 seconds
3. **No available agents**: Retry after 10 seconds
4. **Send failure**: Exponential backoff retry

### Agent-Side

1. **Task data load failure**: Send failure notification
2. **Execution failure**: Send failure notification with error
3. **Connection loss**: Automatic reconnection with backoff

## Performance Considerations

### 1. Concurrent Execution

- Agents can run multiple plan tasks concurrently (up to `plan_limit`)
- Apply tasks are exclusive (blocks all other tasks)
- Server tracks agent capacity in real-time

### 2. Retry Strategy

- Exponential backoff prevents server overload
- Per-task retry counter prevents infinite loops
- Workspace-level locking prevents race conditions

### 3. Status Updates

- Agent heartbeat every 10 seconds
- Task status updated immediately on state changes
- Completion notifications sent asynchronously

## Next Steps

1. **Testing**: Comprehensive end-to-end testing
2. **Monitoring**: Add metrics for task assignment success rate
3. **Optimization**: Fine-tune retry delays based on production data
4. **Documentation**: Update user guide with Agent mode setup
5. **UI Updates**: Show agent assignment in task details

## Related Documentation

- [Agent v3.2 Overview](./agent-v3.2.md)
- [Agent v3.2 Implementation Guide](./agent-v3.2-implementation-guide.md)
- [Phase 1: DataAccessor](./agent-v3.2-phase1-completion.md)
- [Phase 2: Server API](./agent-v3.2-phase2-completion.md)
- [Phase 3: Agent Client](./agent-v3.2-implementation-progress.md)
- [Phase 4: Pool Token Auth](./agent-v3.2-implementation-progress.md)

## Conclusion

Phase 5 completes the Agent v3.2 task scheduler implementation. The system now supports three execution modes:

1. **Local Mode**: Tasks execute on the server
2. **K8s Mode**: Tasks execute in Kubernetes Jobs
3. **Agent Mode**: Tasks execute on remote agents (NEW)

The Agent mode provides:
-  Distributed task execution
-  Agent capacity management
-  Automatic retry with backoff
-  Real-time status tracking
-  Seamless integration with existing systems

**Status**:  COMPLETE - Ready for testing
