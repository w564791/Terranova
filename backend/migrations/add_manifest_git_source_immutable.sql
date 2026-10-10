-- Manifest git source immutability (kept in sync with versioned migration
-- 20261010_16_manifest_git_source_immutable). Idempotent.

CREATE OR REPLACE FUNCTION public.trg_manifests_git_source_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.source_type IS DISTINCT FROM NEW.source_type
       OR OLD.git_repo_url IS DISTINCT FROM NEW.git_repo_url
       OR OLD.git_subpath IS DISTINCT FROM NEW.git_subpath
       OR OLD.github_installation_id IS DISTINCT FROM NEW.github_installation_id THEN
        RAISE EXCEPTION 'git_source_immutable: source_type, git_repo_url, git_subpath, github_installation_id are immutable after creation'
            USING ERRCODE = 'P0001';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS manifests_git_source_immutable ON public.manifests;

CREATE TRIGGER manifests_git_source_immutable
    BEFORE UPDATE ON public.manifests
    FOR EACH ROW
    EXECUTE FUNCTION public.trg_manifests_git_source_immutable();
