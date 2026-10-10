-- Manifest git source (kept in sync with versioned migration
-- 20261010_14_manifest_git_source). Additive and idempotent.
--   github_app_installations:  GitHub App installations registered by an org ADMIN
--                              (an installation belongs to one organization)
--   manifests.git_latest_*:    last push reported by a verified webhook
--                              ("new commit available"; never auto-published)

CREATE TABLE IF NOT EXISTS public.github_app_installations (
    id bigserial PRIMARY KEY,
    organization_id integer NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
    installation_id bigint NOT NULL CHECK (installation_id > 0),
    account_login character varying(255) NOT NULL,
    created_by character varying(20) NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_github_app_installations_installation ON public.github_app_installations (installation_id);
CREATE INDEX IF NOT EXISTS idx_github_app_installations_org ON public.github_app_installations (organization_id);
ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS git_latest_sha character varying(64);
ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS git_latest_ref character varying(255);
ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS git_latest_at timestamp with time zone;
CREATE INDEX IF NOT EXISTS idx_manifests_git_installation ON public.manifests (github_installation_id) WHERE source_type = 'git';
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_manifests_git_latest' AND conrelid = 'public.manifests'::regclass) THEN
        ALTER TABLE public.manifests ADD CONSTRAINT chk_manifests_git_latest CHECK (
            (git_latest_sha IS NULL AND git_latest_ref IS NULL AND git_latest_at IS NULL)
            OR (source_type = 'git' AND git_latest_sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$' AND git_latest_at IS NOT NULL));
    END IF;
END $$;
