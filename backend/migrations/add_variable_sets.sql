-- Variable Sets: organization-level variable collections

-- 1. variable_sets table
CREATE TABLE IF NOT EXISTS public.variable_sets (
    id              SERIAL PRIMARY KEY,
    varset_id       VARCHAR(30) NOT NULL UNIQUE,
    name            VARCHAR(100) NOT NULL,
    description     TEXT,
    scope           VARCHAR(20) NOT NULL DEFAULT 'specific',
    is_deleted      BOOLEAN DEFAULT false NOT NULL,
    created_at      TIMESTAMP WITHOUT TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITHOUT TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by      VARCHAR(20)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_variable_sets_name
    ON public.variable_sets (name) WHERE is_deleted = false;
CREATE INDEX IF NOT EXISTS idx_variable_sets_scope
    ON public.variable_sets USING btree (scope);
CREATE INDEX IF NOT EXISTS idx_variable_sets_is_deleted
    ON public.variable_sets USING btree (is_deleted);

COMMENT ON TABLE public.variable_sets IS 'Organization-level variable set collections';
COMMENT ON COLUMN public.variable_sets.varset_id IS 'Semantic ID: varset-{16 random alphanumeric}';
COMMENT ON COLUMN public.variable_sets.scope IS 'global: applies to all workspaces; specific: manually assigned';

-- 2. varset_variables table
CREATE TABLE IF NOT EXISTS public.varset_variables (
    id              SERIAL PRIMARY KEY,
    variable_id     VARCHAR(20) NOT NULL UNIQUE,
    varset_id       VARCHAR(30) NOT NULL,
    key             VARCHAR(100) NOT NULL,
    value           TEXT,
    variable_type   VARCHAR(20) DEFAULT 'terraform' NOT NULL,
    value_format    VARCHAR(20) DEFAULT 'string' NOT NULL,
    sensitive       BOOLEAN DEFAULT false NOT NULL,
    description     TEXT,
    is_deleted      BOOLEAN DEFAULT false NOT NULL,
    created_at      TIMESTAMP WITHOUT TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITHOUT TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by      VARCHAR(20),
    CONSTRAINT fk_varset_variables_varset
        FOREIGN KEY (varset_id) REFERENCES variable_sets(varset_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_varset_key_type
    ON public.varset_variables USING btree (varset_id, key, variable_type) WHERE is_deleted = false;
CREATE INDEX IF NOT EXISTS idx_varset_variables_varset_id
    ON public.varset_variables USING btree (varset_id);
CREATE INDEX IF NOT EXISTS idx_varset_variables_is_deleted
    ON public.varset_variables USING btree (is_deleted);

COMMENT ON TABLE public.varset_variables IS 'Variables within a variable set';
COMMENT ON COLUMN public.varset_variables.variable_id IS 'Semantic ID: var-{16 random alphanumeric}';

-- 3. varset_assignments table
CREATE TABLE IF NOT EXISTS public.varset_assignments (
    id              SERIAL PRIMARY KEY,
    varset_id       VARCHAR(30) NOT NULL,
    scope_type      VARCHAR(20) NOT NULL,
    project_id      INTEGER,
    workspace_id    VARCHAR(50),
    attached_at     TIMESTAMP WITHOUT TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    attached_by     VARCHAR(20),
    CONSTRAINT chk_scope_target CHECK (
        (scope_type = 'project' AND project_id IS NOT NULL AND workspace_id IS NULL) OR
        (scope_type = 'workspace' AND workspace_id IS NOT NULL AND project_id IS NULL)
    ),
    CONSTRAINT fk_varset_assignments_varset
        FOREIGN KEY (varset_id) REFERENCES variable_sets(varset_id) ON DELETE CASCADE,
    CONSTRAINT fk_varset_assignments_project
        FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT fk_varset_assignments_workspace
        FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_varset_assignment_project
    ON public.varset_assignments USING btree (varset_id, project_id) WHERE project_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_varset_assignment_workspace
    ON public.varset_assignments USING btree (varset_id, workspace_id) WHERE workspace_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_varset_assignments_project_id
    ON public.varset_assignments USING btree (project_id) WHERE project_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_varset_assignments_workspace_id
    ON public.varset_assignments USING btree (workspace_id) WHERE workspace_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_varset_assignments_varset_id
    ON public.varset_assignments USING btree (varset_id);

COMMENT ON TABLE public.varset_assignments IS 'Soft-link assignments between variable sets and projects/workspaces';
COMMENT ON COLUMN public.varset_assignments.scope_type IS 'project or workspace';
COMMENT ON COLUMN public.varset_assignments.attached_at IS 'Determines priority within same scope level (later = higher)';
