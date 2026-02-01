-- Add show_unchanged_resources field to workspaces table
-- This field controls whether to include plan_json in task detail API response
-- Default: false (exclude plan_json to reduce response size)

ALTER TABLE workspaces 
ADD COLUMN IF NOT EXISTS show_unchanged_resources BOOLEAN DEFAULT false;

-- Add comment
COMMENT ON COLUMN workspaces.show_unchanged_resources IS 'Whether to include full plan_json data in API responses (affects performance with large datasets)';
