-- Migration: Update workspace_task_resource_changes table to use semantic workspace_id
-- This script migrates the workspace_id column from integer to VARCHAR(50)

BEGIN;

-- Step 1: Add new workspace_id_new column as VARCHAR(50)
ALTER TABLE workspace_task_resource_changes 
ADD COLUMN IF NOT EXISTS workspace_id_new VARCHAR(50);

-- Step 2: Populate workspace_id_new from workspaces table
UPDATE workspace_task_resource_changes wtrc
SET workspace_id_new = w.workspace_id
FROM workspaces w
WHERE wtrc.workspace_id::integer = w.id;

-- Step 3: Verify all records have been migrated
DO $$
DECLARE
    unmigrated_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO unmigrated_count
    FROM workspace_task_resource_changes
    WHERE workspace_id_new IS NULL;
    
    IF unmigrated_count > 0 THEN
        RAISE EXCEPTION 'Migration failed: % records have NULL workspace_id_new', unmigrated_count;
    END IF;
    
    RAISE NOTICE 'All records migrated successfully';
END $$;

-- Step 4: Drop old workspace_id column
ALTER TABLE workspace_task_resource_changes 
DROP COLUMN workspace_id;

-- Step 5: Rename workspace_id_new to workspace_id
ALTER TABLE workspace_task_resource_changes 
RENAME COLUMN workspace_id_new TO workspace_id;

-- Step 6: Add NOT NULL constraint
ALTER TABLE workspace_task_resource_changes 
ALTER COLUMN workspace_id SET NOT NULL;

-- Step 7: Create index on workspace_id
CREATE INDEX IF NOT EXISTS idx_workspace_task_resource_changes_workspace_id 
ON workspace_task_resource_changes(workspace_id);

-- Step 8: Verify the migration
DO $$
DECLARE
    total_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO total_count
    FROM workspace_task_resource_changes;
    
    RAISE NOTICE 'Migration completed. Total records: %', total_count;
END $$;

COMMIT;

-- Verification queries
SELECT 'workspace_task_resource_changes migration completed' AS status;
SELECT COUNT(*) as total_records FROM workspace_task_resource_changes;
SELECT workspace_id, COUNT(*) as count 
FROM workspace_task_resource_changes 
GROUP BY workspace_id 
ORDER BY count DESC 
LIMIT 10;
