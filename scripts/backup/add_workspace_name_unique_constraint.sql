-- Add unique constraint on workspace name
-- This script will:
-- 1. First check for duplicate workspace names
-- 2. Rename duplicates by appending a suffix
-- 3. Add unique constraint

-- Step 1: Check for duplicates (for reference)
SELECT name, COUNT(*) as count 
FROM workspaces 
GROUP BY name 
HAVING COUNT(*) > 1;

-- Step 2: Rename duplicate workspaces (keep the oldest one, rename newer ones)
-- This uses a CTE to identify duplicates and update them
WITH duplicates AS (
    SELECT id, name, workspace_id, created_at,
           ROW_NUMBER() OVER (PARTITION BY name ORDER BY created_at ASC) as rn
    FROM workspaces
),
to_rename AS (
    SELECT id, name, workspace_id
    FROM duplicates
    WHERE rn > 1
)
UPDATE workspaces w
SET name = w.name || '-' || SUBSTRING(w.workspace_id FROM 4 FOR 8)
FROM to_rename tr
WHERE w.id = tr.id;

-- Step 3: Verify no more duplicates exist
SELECT name, COUNT(*) as count 
FROM workspaces 
GROUP BY name 
HAVING COUNT(*) > 1;

-- Step 4: Add unique constraint on name column
-- Drop the constraint first if it exists (to make script idempotent)
ALTER TABLE workspaces DROP CONSTRAINT IF EXISTS workspaces_name_unique;

-- Add the unique constraint
ALTER TABLE workspaces ADD CONSTRAINT workspaces_name_unique UNIQUE (name);

-- Verify the constraint was added
SELECT conname, contype 
FROM pg_constraint 
WHERE conrelid = 'workspaces'::regclass AND conname = 'workspaces_name_unique';
