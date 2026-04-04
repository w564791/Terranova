-- Variable Snapshots table
CREATE TABLE IF NOT EXISTS public.variable_snapshots (
    id              SERIAL PRIMARY KEY,
    vsnap_id        VARCHAR(30) NOT NULL,
    workspace_id    VARCHAR(50) NOT NULL,
    variable_id     VARCHAR(20) NOT NULL,
    version         INTEGER NOT NULL,
    variable_type   VARCHAR(20) NOT NULL,
    source_type     VARCHAR(20) NOT NULL,
    created_at      TIMESTAMP WITHOUT TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by      VARCHAR(20)
);

CREATE INDEX IF NOT EXISTS idx_vsnap_id ON public.variable_snapshots USING btree (vsnap_id);
CREATE INDEX IF NOT EXISTS idx_vsnap_workspace ON public.variable_snapshots USING btree (workspace_id);

-- Add snapshot reference to workspace_tasks
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS variable_snapshot_id VARCHAR(30);

-- Remove old column
ALTER TABLE workspace_tasks DROP COLUMN IF EXISTS snapshot_variables;
