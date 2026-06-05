-- Add after_unknown column to workspace_task_resource_changes
-- Stores the after_unknown map from Terraform plan JSON, marking fields as "known after apply"

ALTER TABLE workspace_task_resource_changes
ADD COLUMN IF NOT EXISTS after_unknown jsonb;

COMMENT ON COLUMN workspace_task_resource_changes.after_unknown
IS 'Terraform plan after_unknown map, marks fields as known after apply';
