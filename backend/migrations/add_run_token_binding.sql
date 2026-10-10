-- Run token binding (kept in sync with versioned migration
-- 20261010_12_run_token_binding). Additive and idempotent.
--   manifest_runs.agent_id: the agent the run is assigned to; only it may
--                           obtain the run's token (with its agent token)
--   run_tokens.agent_id:    the agent that obtained the token; revoking or
--                           deregistering the agent revokes its run tokens

ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS agent_id character varying(50);
ALTER TABLE public.run_tokens ADD COLUMN IF NOT EXISTS agent_id character varying(50);
CREATE INDEX IF NOT EXISTS idx_run_tokens_agent_active ON public.run_tokens (agent_id) WHERE agent_id IS NOT NULL AND revoked_at IS NULL;
