# Task 510: Apply Pending Task Restart Fix

## Problem Description

After server/agent restart, `apply_pending` tasks fail with error:
```
apply task has no associated plan task
```

### Root Cause

1. When a `plan_and_apply` task completes its plan phase successfully, it should set `plan_task_id` to point to itself
2. However, in some cases (especially older tasks or due to race conditions), `plan_task_id` might be NULL
3. After restart, when the agent tries to execute the apply phase, it checks for `plan_task_id` and fails if it's NULL
4. This violates the user's requirement that apply_pending tasks should not fail after restart

### Error Location

In `backend/services/terraform_executor.go`, `ExecuteApply` function:

```go
if task.PlanTaskID == nil {
    err := fmt.Errorf("apply task has no associated plan task")
    logger.LogError("fetching", err, map[string]interface{}{
        "task_id":      task.ID,
        "workspace_id": task.WorkspaceID,
    }, nil)
    logger.StageEnd("fetching")
    s.saveTaskFailure(task, logger, err, "apply")
    return err
}
```

## Solution Design

### Approach 1: Fix at Task Loading Time (Recommended)

When loading task data for execution, automatically fix `plan_task_id` for `plan_and_apply` tasks:

**Location**: `backend/services/remote_data_accessor.go` - `LoadTaskData` method

```go
// After loading task data, fix plan_task_id for plan_and_apply tasks
if task.TaskType == models.TaskTypePlanAndApply {
    if task.Status == models.TaskStatusApplyPending && task.PlanTaskID == nil {
        // For plan_and_apply tasks, plan_task_id should point to itself
        task.PlanTaskID = &task.ID
        log.Printf("[LoadTaskData] Auto-fixed plan_task_id for plan_and_apply task %d", task.ID)
        
        // Update in database via API
        if err := a.apiClient.UpdateTaskPlanTaskID(task.ID, task.ID); err != nil {
            log.Printf("[LoadTaskData] Warning: failed to update plan_task_id in database: %v", err)
            // Don't fail - the in-memory fix is enough for execution
        }
    }
}
```

### Approach 2: Fix at ExecuteApply Time (Fallback)

Add a fallback check in `ExecuteApply` to handle this case gracefully:

**Location**: `backend/services/terraform_executor.go` - `ExecuteApply` function

```go
// Check and fix plan_task_id for plan_and_apply tasks
if task.PlanTaskID == nil {
    // For plan_and_apply tasks, plan_task_id should point to itself
    if task.TaskType == models.TaskTypePlanAndApply {
        logger.Warn("plan_task_id is NULL for plan_and_apply task, auto-fixing to self-reference")
        task.PlanTaskID = &task.ID
        
        // Update in database
        if err := s.dataAccessor.UpdateTask(task); err != nil {
            logger.Warn("Failed to update plan_task_id: %v", err)
        }
    } else {
        // For standalone apply tasks, this is an error
        err := fmt.Errorf("apply task has no associated plan task")
        logger.LogError("fetching", err, map[string]interface{}{
            "task_id":      task.ID,
            "workspace_id": task.WorkspaceID,
        }, nil)
        logger.StageEnd("fetching")
        s.saveTaskFailure(task, logger, err, "apply")
        return err
    }
}
```

### Approach 3: Database Migration (Preventive)

Create a migration to fix existing tasks in the database:

**File**: `scripts/fix_apply_pending_plan_task_id.sql`

```sql
-- Fix plan_task_id for plan_and_apply tasks in apply_pending status
UPDATE workspace_tasks
SET plan_task_id = id
WHERE task_type = 'plan_and_apply'
  AND status = 'apply_pending'
  AND plan_task_id IS NULL;
```

## Implementation Plan

1.  Implement Approach 2 (ExecuteApply fix) - immediate fix
2.  Implement Approach 3 (database migration) - fix existing data
3. ⏳ Implement Approach 1 (LoadTaskData fix) - prevent future issues
4. ⏳ Add API endpoint for updating plan_task_id
5. ⏳ Test the complete flow

## Files to Modify

1. `backend/services/terraform_executor.go` - Add fallback check in ExecuteApply
2. `backend/services/remote_data_accessor.go` - Add auto-fix in LoadTaskData
3. `backend/services/agent_api_client.go` - Add UpdateTaskPlanTaskID method
4. `backend/controllers/workspace_task_controller.go` - Add API endpoint
5. `scripts/fix_apply_pending_plan_task_id.sql` - Database migration

## Testing Plan

1. Create a plan_and_apply task
2. Manually set plan_task_id to NULL in database
3. Restart server and agent
4. Verify task executes successfully
5. Verify plan_task_id is auto-fixed

## Related Issues

- Task 474: Status flow issues
- Task 480: Plan task ID missing
- Task 497: Server restart apply pending fix
- Task 500: Apply pending auto-execute bug

## Priority

**CRITICAL** - This blocks apply_pending tasks from executing after restart
