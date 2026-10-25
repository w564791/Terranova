-- sandboxes + shell_sessions registration (kept in sync with versioned migration
-- 20261025_01_sandbox_registration). Additive and idempotent.
-- Platform only registers externally managed sandboxes; DELETE unbinds only.
-- See docs/manifest/manifest-sandbox-spec.md §10.2.

CREATE TABLE IF NOT EXISTS public.sandboxes (
    id character varying(36) NOT NULL PRIMARY KEY,
    org_id bigint NOT NULL,
    workspace_id character varying(50) NOT NULL,
    sandbox_ref character varying(256) NOT NULL,
    runtime_ref character varying(256),
    label character varying(256),
    driver character varying(32) NOT NULL,
    image_digest character varying(128),
    assume_role_arn character varying(256),
    assume_external_id character varying(128),
    network_mode character varying(16),
    status character varying(32) NOT NULL DEFAULT 'registered',
    lease_expires_at timestamp with time zone,
    created_by character varying(20) NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT chk_sandboxes_driver CHECK (driver IN ('agentcore_external', 'shell_debug')),
    CONSTRAINT chk_sandboxes_status CHECK (status IN ('registered', 'ready', 'lease_expired', 'unbound')),
    CONSTRAINT chk_sandboxes_network_mode CHECK (network_mode IS NULL OR network_mode IN ('VPC', 'PUBLIC')),
    CONSTRAINT chk_sandboxes_sandbox_ref CHECK (sandbox_ref ~ '^[A-Za-z0-9][A-Za-z0-9._:/-]{1,255}$'),
    CONSTRAINT chk_sandboxes_runtime_ref CHECK (runtime_ref IS NULL OR runtime_ref ~ '^[A-Za-z0-9][A-Za-z0-9._:/-]{1,255}$'),
    CONSTRAINT uq_sandboxes_org_ref UNIQUE (org_id, sandbox_ref)
);
CREATE INDEX IF NOT EXISTS idx_sandboxes_org ON public.sandboxes (org_id);
CREATE INDEX IF NOT EXISTS idx_sandboxes_workspace ON public.sandboxes (workspace_id);
CREATE INDEX IF NOT EXISTS idx_sandboxes_status ON public.sandboxes (status) WHERE status <> 'unbound';
COMMENT ON COLUMN public.sandboxes.network_mode IS 'Snapshot VPC|PUBLIC when known; production attach requires VPC (ENV=production; empty/PUBLIC/other → network_mode_not_allowed)';
COMMENT ON COLUMN public.sandboxes.runtime_ref IS 'Optional warm-pool runtime/harness pointer; session isolation is separate';
COMMENT ON TABLE public.sandboxes IS 'Platform registration of externally managed sandboxes; DELETE unbinds only';

CREATE TABLE IF NOT EXISTS public.shell_sessions (
    id character varying(36) NOT NULL PRIMARY KEY,
    sandbox_id character varying(36) NOT NULL REFERENCES public.sandboxes(id),
    org_id bigint NOT NULL,
    status character varying(20) NOT NULL DEFAULT 'active',
    expires_at timestamp with time zone NOT NULL,
    closed_at timestamp with time zone,
    created_by character varying(20) NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT chk_shell_sessions_status CHECK (status IN ('active', 'closed'))
);
CREATE INDEX IF NOT EXISTS idx_shell_sessions_sandbox ON public.shell_sessions (sandbox_id);
CREATE INDEX IF NOT EXISTS idx_shell_sessions_org ON public.shell_sessions (org_id);
CREATE INDEX IF NOT EXISTS idx_shell_sessions_open ON public.shell_sessions (expires_at) WHERE closed_at IS NULL;
COMMENT ON TABLE public.shell_sessions IS 'Shell sessions share credential lifetime; revoke on close (§10.2)';
