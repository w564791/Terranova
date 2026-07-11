-- Manifest provider schema cache (post_init capture)
-- Keyed by (manifest_id, subpath, schema_kind). Provider versions stored for cheap skip.

CREATE TABLE IF NOT EXISTS public.manifest_provider_schemas (
    id                     VARCHAR(40) PRIMARY KEY,
    manifest_id            VARCHAR(36)  NOT NULL,
    subpath                VARCHAR(512) NOT NULL DEFAULT '',
    schema_kind            VARCHAR(16)  NOT NULL DEFAULT 'types',
    providers              JSONB        NOT NULL DEFAULT '[]'::jsonb,
    provider_versions_key  VARCHAR(128) NOT NULL,
    resources              JSONB,
    data_sources           JSONB,
    content_hash           VARCHAR(64),
    terraform_version      VARCHAR(32),
    source_workspace_id    VARCHAR(50),
    source_task_id         BIGINT,
    captured_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_mps_manifest_subpath_kind UNIQUE (manifest_id, subpath, schema_kind)
);

CREATE INDEX IF NOT EXISTS idx_mps_manifest_id
    ON public.manifest_provider_schemas (manifest_id);
CREATE INDEX IF NOT EXISTS idx_mps_provider_versions_key
    ON public.manifest_provider_schemas (provider_versions_key);

COMMENT ON TABLE public.manifest_provider_schemas IS
  'Provider type catalog captured after terraform init (post_init). Shared by manifest+subpath.';
COMMENT ON COLUMN public.manifest_provider_schemas.provider_versions_key IS
  'Canonical fingerprint of lock provider versions; unchanged => skip re-capture';
COMMENT ON COLUMN public.manifest_provider_schemas.subpath IS
  'Normalized terraform workdir under manifest; empty string = root';
