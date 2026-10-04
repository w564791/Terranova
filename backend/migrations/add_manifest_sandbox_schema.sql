-- Manifest dual-source / dual-runner schema (docs/manifest/manifest-sandbox-spec.md §3)
-- (kept in sync with versioned migration 20261004_02_manifest_sandbox_schema).
-- Additive and idempotent: new columns / tables / constraints only.

ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS source_type character varying(16) NOT NULL DEFAULT 'native';

ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS git_repo_url character varying(1024);

ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS git_subpath character varying(512);

ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS github_installation_id bigint;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_manifests_source_type' AND conrelid = 'public.manifests'::regclass) THEN
        ALTER TABLE public.manifests ADD CONSTRAINT chk_manifests_source_type CHECK (source_type IN ('native', 'git'));
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_manifests_git_fields' AND conrelid = 'public.manifests'::regclass) THEN
        ALTER TABLE public.manifests ADD CONSTRAINT chk_manifests_git_fields CHECK ((source_type = 'native' AND git_repo_url IS NULL AND git_subpath IS NULL AND github_installation_id IS NULL) OR (source_type = 'git' AND git_repo_url IS NOT NULL));
    END IF;
END $$;

ALTER TABLE public.manifest_versions ADD COLUMN IF NOT EXISTS bundle_hash character varying(64);

ALTER TABLE public.manifest_versions ADD COLUMN IF NOT EXISTS source_ref character varying(64);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_manifest_versions_bundle_hash' AND conrelid = 'public.manifest_versions'::regclass) THEN
        ALTER TABLE public.manifest_versions ADD CONSTRAINT chk_manifest_versions_bundle_hash CHECK (bundle_hash IS NULL OR bundle_hash ~ '^[0-9a-f]{64}$');
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_manifest_versions_source_ref' AND conrelid = 'public.manifest_versions'::regclass) THEN
        ALTER TABLE public.manifest_versions ADD CONSTRAINT chk_manifest_versions_source_ref CHECK (source_ref IS NULL OR source_ref ~ '^[0-9a-f]{40}([0-9a-f]{24})?$');
    END IF;
END $$;

ALTER TABLE public.manifest_deployments ADD COLUMN IF NOT EXISTS approved_bundle_hash character varying(64);

ALTER TABLE public.manifest_deployments ADD COLUMN IF NOT EXISTS approved_plan_hash character varying(64);

ALTER TABLE public.manifest_deployments ADD COLUMN IF NOT EXISTS sensitive_keys jsonb;

ALTER TABLE public.workspace_tasks ADD COLUMN IF NOT EXISTS sensitive_keys jsonb;

CREATE TABLE IF NOT EXISTS public.sandbox_sessions (
    id character varying(36) NOT NULL PRIMARY KEY,
    user_id character varying(20) NOT NULL,
    workspace_id character varying(50) NOT NULL,
    provider character varying(32) NOT NULL,
    network_mode character varying(16) NOT NULL DEFAULT 'vpc',
    status character varying(20) NOT NULL DEFAULT 'active',
    expires_at timestamp with time zone NOT NULL,
    closed_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT chk_sandbox_sessions_network_mode CHECK (network_mode = 'vpc'),
    CONSTRAINT uq_sandbox_sessions_id_workspace UNIQUE (id, workspace_id)
);

CREATE INDEX IF NOT EXISTS idx_sandbox_sessions_user_workspace ON public.sandbox_sessions (user_id, workspace_id);

CREATE INDEX IF NOT EXISTS idx_sandbox_sessions_open_expiry ON public.sandbox_sessions (expires_at) WHERE closed_at IS NULL;

CREATE TABLE IF NOT EXISTS public.manifest_runs (
    id character varying(36) NOT NULL PRIMARY KEY,
    manifest_id character varying(36) NOT NULL REFERENCES public.manifests(id) ON DELETE CASCADE,
    version_id character varying(36) REFERENCES public.manifest_versions(id) ON DELETE SET NULL,
    bundle_hash character varying(64) NOT NULL,
    workspace_id character varying(50) NOT NULL,
    runner character varying(16) NOT NULL,
    purpose character varying(16) NOT NULL,
    status character varying(20) NOT NULL DEFAULT 'pending',
    plan_hash character varying(64),
    plan_redacted jsonb,
    state_serial bigint,
    session_id character varying(36),
    created_by character varying(20) NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT chk_manifest_runs_runner CHECK (runner IN ('agent', 'sandbox')),
    CONSTRAINT chk_manifest_runs_purpose CHECK (purpose IN ('preview', 'approval')),
    CONSTRAINT chk_manifest_runs_sandbox_preview_only CHECK (runner <> 'sandbox' OR purpose = 'preview'),
    CONSTRAINT chk_manifest_runs_sandbox_session CHECK (runner <> 'sandbox' OR session_id IS NOT NULL),
    CONSTRAINT chk_manifest_runs_bundle_hash CHECK (bundle_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT fk_manifest_runs_session_workspace FOREIGN KEY (session_id, workspace_id) REFERENCES public.sandbox_sessions(id, workspace_id),
    CONSTRAINT uq_manifest_runs_id_workspace_purpose UNIQUE (id, workspace_id, purpose)
);

CREATE INDEX IF NOT EXISTS idx_manifest_runs_manifest ON public.manifest_runs (manifest_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_manifest_runs_workspace ON public.manifest_runs (workspace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_manifest_runs_session ON public.manifest_runs (session_id) WHERE session_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS public.run_tokens (
    id bigserial PRIMARY KEY,
    run_id character varying(36) NOT NULL,
    session_id character varying(36),
    workspace_id character varying(50) NOT NULL,
    purpose character varying(16) NOT NULL,
    token_hash character varying(64) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT uq_run_tokens_token_hash UNIQUE (token_hash),
    CONSTRAINT chk_run_tokens_purpose CHECK (purpose IN ('preview', 'approval')),
    CONSTRAINT fk_run_tokens_run FOREIGN KEY (run_id, workspace_id, purpose) REFERENCES public.manifest_runs(id, workspace_id, purpose) ON DELETE CASCADE,
    CONSTRAINT fk_run_tokens_session_workspace FOREIGN KEY (session_id, workspace_id) REFERENCES public.sandbox_sessions(id, workspace_id)
);

CREATE INDEX IF NOT EXISTS idx_run_tokens_run ON public.run_tokens (run_id);

CREATE INDEX IF NOT EXISTS idx_run_tokens_session_active ON public.run_tokens (session_id) WHERE session_id IS NOT NULL AND revoked_at IS NULL;

-- Backfill bundle_hash of existing versions. Same encoding as
-- manifestbundle.Hash (the Go migration uses that function directly):
--   sha256("terranova-bundle-v1" 0x00 { path 0x00 len(content) 0x00 content } sorted by path, byte order)
UPDATE public.manifest_versions v
   SET bundle_hash = encode(sha256(
         convert_to('terranova-bundle-v1', 'UTF8') || '\x00'::bytea ||
         COALESCE((SELECT string_agg(
                     convert_to(f.path, 'UTF8') || '\x00'::bytea ||
                     convert_to(octet_length(f.content)::text, 'UTF8') || '\x00'::bytea ||
                     f.content,
                     ''::bytea ORDER BY f.path COLLATE "C")
                   FROM public.manifest_files f
                  WHERE f.version_id = v.id), ''::bytea)), 'hex')
 WHERE v.bundle_hash IS NULL;
