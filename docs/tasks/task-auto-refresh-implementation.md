# Task List Auto-Refresh Implementation

## Overview
Implement selective background refresh for incomplete tasks in the workspace detail page.

## Problem
Users don't know when tasks complete because the task list doesn't auto-refresh. They have to manually refresh the page to see status updates.

## Solution
Implement smart background polling that:
1. Only refreshes incomplete tasks (pending, running, apply_pending, requires_approval)
2. Merges results with existing completed tasks
3. Stops polling when all tasks are complete
4. Uses efficient backend queries

## Task Status Categories

### Incomplete Statuses (需要刷新)
- `pending` - waiting to start
- `running` - currently executing  
- `apply_pending` - waiting for apply confirmation
- `requires_approval` - needs manual approval
- `plan_completed` - plan done, waiting for next action

### Complete Statuses (不需要刷新)
- `success` - completed successfully
- `applied` - apply completed
- `failed` - errored
- `cancelled` - user cancelled

## Implementation Details

### Backend Changes
**No backend changes needed** - existing API already supports status filtering:
```
GET /api/v1/workspaces/{id}/tasks?status=running
GET /api/v1/workspaces/{id}/tasks?status=pending
```

### Frontend Changes

#### 1. Add Selective Refresh Hook
Create a custom hook `useTaskAutoRefresh` that:
- Polls for incomplete tasks every 5 seconds
- Merges with existing completed tasks
- Stops when no incomplete tasks exist
- Pauses when user is not on the Runs tab

#### 2. Update RunsTab Component
- Add the auto-refresh hook
- Merge incomplete task updates with existing data
- Show visual indicator when refreshing
- Preserve pagination and filters

#### 3. Update Latest Run Display
- Also refresh the global latest run
- Update both Overview and Runs tabs

## Benefits
1. **Efficient**: Only fetches incomplete tasks, not all tasks
2. **Smart**: Stops polling when nothing is running
3. **Non-intrusive**: Doesn't reset pagination or filters
4. **Real-time**: Users see updates within 5 seconds

## Implementation Steps
1. Create `useTaskAutoRefresh` hook
2. Integrate into RunsTab component
3. Update global latest run refresh
4. Test with various task states
5. Verify performance impact

## Performance Considerations
- Polling interval: 5 seconds (configurable)
- Only active when on Runs tab
- Stops when no incomplete tasks
- Uses existing backend API (no new endpoints)
- Minimal data transfer (only incomplete tasks)

## Testing Scenarios
1. Single running task completes
2. Multiple tasks in different states
3. All tasks complete (polling stops)
4. New task created while polling
5. User switches tabs (polling pauses)
6. Task cancelled/failed during polling
