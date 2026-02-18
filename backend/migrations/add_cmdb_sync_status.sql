-- Add CMDB sync status columns to workspaces table
-- These columns track the background CMDB sync state for frontend visibility and mutual exclusion

ALTER TABLE public.workspaces
    ADD COLUMN IF NOT EXISTS cmdb_sync_status character varying(20) DEFAULT 'idle'::character varying,
    ADD COLUMN IF NOT EXISTS cmdb_sync_triggered_by character varying(20),
    ADD COLUMN IF NOT EXISTS cmdb_sync_started_at timestamp without time zone,
    ADD COLUMN IF NOT EXISTS cmdb_sync_completed_at timestamp without time zone;

COMMENT ON COLUMN public.workspaces.cmdb_sync_status IS 'CMDB同步状态: idle(空闲), syncing(同步中)';
COMMENT ON COLUMN public.workspaces.cmdb_sync_triggered_by IS 'CMDB同步触发来源: auto(apply后自动), manual(手动sync), rebuild(手动rebuild)';
COMMENT ON COLUMN public.workspaces.cmdb_sync_started_at IS 'CMDB同步开始时间';
COMMENT ON COLUMN public.workspaces.cmdb_sync_completed_at IS 'CMDB同步完成时间';
