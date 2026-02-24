-- Create provider_templates table for global provider configuration management
CREATE TABLE IF NOT EXISTS public.provider_templates (
    id SERIAL PRIMARY KEY,
    name character varying(100) NOT NULL,
    type character varying(50) NOT NULL,
    source character varying(200) NOT NULL,
    config jsonb NOT NULL DEFAULT '{}',
    version character varying(50),
    constraint_op character varying(10),
    is_default boolean DEFAULT false,
    enabled boolean DEFAULT true,
    description text,
    created_by integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

-- Unique constraint on name
CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_templates_name ON public.provider_templates (name);

-- Index for type lookups
CREATE INDEX IF NOT EXISTS idx_provider_templates_type ON public.provider_templates (type);

-- Add workspace columns for template references
ALTER TABLE public.workspaces
    ADD COLUMN IF NOT EXISTS provider_template_ids jsonb,
    ADD COLUMN IF NOT EXISTS provider_overrides jsonb;

COMMENT ON TABLE public.provider_templates IS 'Global provider configuration templates';
COMMENT ON COLUMN public.provider_templates.type IS 'Provider type name: aws, kubernetes, tencentcloud, ode, etc.';
COMMENT ON COLUMN public.provider_templates.source IS 'Terraform registry source: hashicorp/aws, IBM/ode, etc.';
COMMENT ON COLUMN public.provider_templates.config IS 'Provider block configuration (auth, endpoints, etc.)';
COMMENT ON COLUMN public.provider_templates.version IS 'Provider version number (optional)';
COMMENT ON COLUMN public.provider_templates.constraint_op IS 'Version constraint operator: ~>, >=, =, etc. (optional)';
COMMENT ON COLUMN public.workspaces.provider_template_ids IS 'JSON array of referenced provider template IDs';
COMMENT ON COLUMN public.workspaces.provider_overrides IS 'Per-type field overrides applied on top of templates';
