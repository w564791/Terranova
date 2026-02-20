-- Migration: Add workspace_id column to workspace_task_resource_changes table
-- This script adds the workspace_id VARCHAR(50) column and populates it from workspace_tasks

BEGIN;

-- Step 1: Add workspace_id column as VARCHAR(50)
ALTER TABLE workspace_task_resource_changes 
ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(50);

-- Step 2: Populate workspace_id from workspace_tasks table
UPDATE workspace_task_resource_changes wtrc
SET workspace_id = wt.workspace_id
FROM workspace_tasks wt
WHERE wtrc.task_id = wt.id;

-- Step 3: Verify all records have been populated
DO $$
DECLARE
    null_count INTEGER;
    total_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO null_count
    FROM workspace_task_resource_changes
    WHERE workspace_id IS NULL;
    
    SELECT COUNT(*) INTO total_count
    FROM workspace_task_resource_changes;
    
    IF null_count > 0 THEN
        RAISE WARNING 'Warning: % out of % records have NULL workspace_id', null_count, total_count;
    ELSE
        RAISE NOTICE 'Success: All % records have workspace_id populated', total_count;
    END IF;
END $$;

-- Step 4: Add NOT NULL constraint (only if all records are populated)
DO $$
DECLARE
    null_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO null_count
    FROM workspace_task_resource_changes
    WHERE workspace_id IS NULL;
    
    IF null_count = 0 THEN
        ALTER TABLE workspace_task_resource_changes 
        ALTER COLUMN workspace_id SET NOT NULL;
        RAISE NOTICE 'NOT NULL constraint added to workspace_id';
    ELSE
        RAISE WARNING 'Skipping NOT NULL constraint due to NULL values';
    END IF;
END $$;

-- Step 5: Create index on workspace_id
CREATE INDEX IF NOT EXISTS idx_workspace_task_resource_changes_workspace_id 
ON workspace_task_resource_changes(workspace_id);

COMMIT;

-- Verification queries
SELECT 'workspace_task_resource_changes migration completed' AS status;
SELECT COUNT(*) as total_records FROM workspace_task_resource_changes;
SELECT COUNT(*) as records_with_workspace_id 
FROM workspace_task_resource_changes 
WHERE workspace_id IS NOT NULL;
SELECT workspace_id, COUNT(*) as count 
FROM workspace_task_resource_changes 
WHERE workspace_id IS NOT NULL
GROUP BY workspace_id 
ORDER BY count DESC 
LIMIT 10;
