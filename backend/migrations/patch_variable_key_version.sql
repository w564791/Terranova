-- Per-row DATA_ENCRYPTION_KEY version of encrypted variable values
-- (0 = legacy JWT_SECRET-derived key or not encrypted). Applied by
-- cmd/migrate as 20261010_10_variable_key_version; idempotent.
ALTER TABLE public.workspace_variables ADD COLUMN IF NOT EXISTS key_version smallint NOT NULL DEFAULT 0;
ALTER TABLE public.varset_variables ADD COLUMN IF NOT EXISTS key_version smallint NOT NULL DEFAULT 0;
