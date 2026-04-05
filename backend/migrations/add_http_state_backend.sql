-- add_http_state_backend.sql
-- HTTP State Backend: unified lock fields + state token hash

-- 1. Workspace lock field refactor: merge UI lock and TF runtime lock
ALTER TABLE workspaces ADD COLUMN lock_id VARCHAR(255);
ALTER TABLE workspaces ADD COLUMN lock_info JSONB;

-- Migrate existing lock data
UPDATE workspaces
SET lock_id = gen_random_uuid()::text,
    lock_info = jsonb_build_object(
        'who', locked_by,
        'operation', 'ui_lock',
        'info', lock_reason,
        'created', locked_at
    )
WHERE is_locked = true;

-- Drop old lock fields (not yet shipped publicly, no compat burden)
ALTER TABLE workspaces DROP COLUMN is_locked;
ALTER TABLE workspaces DROP COLUMN locked_by;
ALTER TABLE workspaces DROP COLUMN locked_at;
ALTER TABLE workspaces DROP COLUMN lock_reason;

-- 2. WorkspaceTask: add state token hash field
ALTER TABLE workspace_tasks ADD COLUMN state_token_hash VARCHAR(64);

-- Index for token validation queries
CREATE INDEX idx_workspace_tasks_state_token ON workspace_tasks(state_token_hash) WHERE state_token_hash IS NOT NULL;
