-- Fix Task 572 and Similar Cancelled Status Issues
-- Issue: Tasks were cancelled but status remains "running" instead of "cancelled"
-- Date: 2025-11-07
-- Reference: docs/task-572-cancelled-status-bug-analysis.md

-- Step 1: Check current state of task 572
SELECT 
    id,
    workspace_id,
    stage,
    status,
    completed_at,
    error_message,
    created_at,
    updated_at
FROM workspace_tasks 
WHERE id = 572;

-- Step 2: Check for cancellation logs
SELECT 
    id,
    task_id,
    phase,
    content,
    level,
    created_at
FROM task_logs 
WHERE task_id = 572 
    AND content LIKE '%cancelled%'
ORDER BY created_at DESC;

-- Step 3: Find all tasks with inconsistent cancelled state
-- (has completed_at but status is still "running" or "pending", and has cancellation log)
SELECT 
    t.id,
    t.workspace_id,
    t.status,
    t.stage,
    t.completed_at,
    t.error_message,
    COUNT(l.id) as cancel_log_count
FROM workspace_tasks t
LEFT JOIN task_logs l ON l.task_id = t.id AND l.content LIKE '%cancelled%'
WHERE t.completed_at IS NOT NULL
    AND t.status IN ('running', 'pending')
GROUP BY t.id, t.workspace_id, t.status, t.stage, t.completed_at, t.error_message
HAVING COUNT(l.id) > 0
ORDER BY t.id;

-- Step 4: Fix task 572 specifically
UPDATE workspace_tasks 
SET 
    status = 'cancelled',
    error_message = 'Task cancelled by user',
    updated_at = NOW()
WHERE id = 572 
    AND status = 'running' 
    AND completed_at IS NOT NULL;

-- Step 5: Fix all other tasks with the same issue
-- (has completed_at, status is running/pending, and has cancellation log)
UPDATE workspace_tasks 
SET 
    status = 'cancelled',
    error_message = CASE 
        WHEN error_message IS NULL OR error_message = '' THEN 'Task cancelled by user'
        ELSE error_message
    END,
    updated_at = NOW()
WHERE id IN (
    SELECT DISTINCT t.id
    FROM workspace_tasks t
    INNER JOIN task_logs l ON l.task_id = t.id
    WHERE t.completed_at IS NOT NULL
        AND t.status IN ('running', 'pending')
        AND l.content LIKE '%cancelled%'
);

-- Step 6: Verify the fix for task 572
SELECT 
    id,
    workspace_id,
    stage,
    status,
    completed_at,
    error_message,
    updated_at
FROM workspace_tasks 
WHERE id = 572;

-- Step 7: Verify all fixed tasks
SELECT 
    id,
    workspace_id,
    status,
    stage,
    completed_at,
    error_message,
    updated_at
FROM workspace_tasks 
WHERE status = 'cancelled'
    AND updated_at > NOW() - INTERVAL '1 minute'
ORDER BY id;

-- Step 8: Check for any remaining inconsistent tasks
SELECT 
    t.id,
    t.workspace_id,
    t.status,
    t.stage,
    t.completed_at,
    t.started_at,
    t.created_at
FROM workspace_tasks t
WHERE t.completed_at IS NOT NULL
    AND t.status IN ('running', 'pending')
ORDER BY t.id;
