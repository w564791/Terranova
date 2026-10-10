-- Per-agent JWT revocation state (kept in sync with versioned migration
-- 20261010_11_agent_token). Additive and idempotent.
--   token_generation: carried in agent tokens (gen claim); revocation bumps it
--   revoked_at:       agent tokens (and the agent's run tokens) refused
--   pool_token_hash:  the pool token the agent registered with; its agent
--                     tokens are valid only while that pool token is active

ALTER TABLE public.agents ADD COLUMN IF NOT EXISTS token_generation integer DEFAULT 0 NOT NULL;
ALTER TABLE public.agents ADD COLUMN IF NOT EXISTS revoked_at timestamp with time zone;
ALTER TABLE public.agents ADD COLUMN IF NOT EXISTS pool_token_hash character varying(64);
