-- Add alias column to provider_templates for multi-provider-per-type support
ALTER TABLE public.provider_templates
    ADD COLUMN IF NOT EXISTS alias character varying(50);

COMMENT ON COLUMN public.provider_templates.alias IS 'Terraform provider alias for multi-provider-per-type (optional, main provider has no alias)';
