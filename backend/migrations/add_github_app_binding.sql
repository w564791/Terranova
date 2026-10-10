-- GitHub App installation binding (kept in sync with versioned migration
-- 20261010_15_github_app_binding). Additive and idempotent.
--   github_app_installations:   account_id / account_type and the OAuth proof
--                               (verified_*); unverified rows are not usable
--   uq_github_app_installations_installation: UNIQUE constraint (platform-wide)
--   github_app_setup_nonces:    consumed setup-state nonces (single use)
--   github_webhook_deliveries:  processed X-GitHub-Delivery ids (72h replay window)

ALTER TABLE public.github_app_installations ADD COLUMN IF NOT EXISTS account_id bigint;
ALTER TABLE public.github_app_installations ADD COLUMN IF NOT EXISTS account_type character varying(20);
ALTER TABLE public.github_app_installations ADD COLUMN IF NOT EXISTS verified_github_user_id bigint;
ALTER TABLE public.github_app_installations ADD COLUMN IF NOT EXISTS verified_github_login character varying(255);
ALTER TABLE public.github_app_installations ADD COLUMN IF NOT EXISTS verified_at timestamp with time zone;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'uq_github_app_installations_installation' AND conrelid = 'public.github_app_installations'::regclass) THEN
        IF EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
                   WHERE n.nspname = 'public' AND c.relname = 'uq_github_app_installations_installation' AND c.relkind = 'i') THEN
            ALTER TABLE public.github_app_installations ADD CONSTRAINT uq_github_app_installations_installation UNIQUE USING INDEX uq_github_app_installations_installation;
        ELSE
            ALTER TABLE public.github_app_installations ADD CONSTRAINT uq_github_app_installations_installation UNIQUE (installation_id);
        END IF;
    END IF;
END $$;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_github_app_installations_verified' AND conrelid = 'public.github_app_installations'::regclass) THEN
        ALTER TABLE public.github_app_installations ADD CONSTRAINT chk_github_app_installations_verified CHECK (
            verified_at IS NULL
            OR (verified_github_user_id IS NOT NULL AND verified_github_login IS NOT NULL
                AND account_id IS NOT NULL AND account_type IN ('Organization', 'User')));
    END IF;
END $$;
CREATE TABLE IF NOT EXISTS public.github_app_setup_nonces (
    nonce character varying(64) PRIMARY KEY,
    organization_id integer NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
    user_id character varying(20) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_github_app_setup_nonces_expires ON public.github_app_setup_nonces (expires_at);
CREATE TABLE IF NOT EXISTS public.github_webhook_deliveries (
    delivery_id character varying(64) PRIMARY KEY,
    event character varying(64) NOT NULL,
    received_at timestamp with time zone NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_github_webhook_deliveries_received ON public.github_webhook_deliveries (received_at);
