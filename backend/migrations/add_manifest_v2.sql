-- Manifest 重构: 从拖拽画布到 VS Code Web 工作区
-- 设计文档: specs/2026-05-06-manifest-refactor-spec.md
--
-- 本迁移幂等: 全程 IF EXISTS / IF NOT EXISTS / DO blocks 检查列存在性

-- =============================================================================
-- 1. 新表 manifest_files
-- =============================================================================
-- 草稿与已发布版本快照统一存储
--   version_id IS NULL  AND owner_user_id 非空 → 用户私有草稿
--   version_id 非空     AND owner_user_id NULL → published 不可变快照
CREATE TABLE IF NOT EXISTS public.manifest_files (
    id            BIGSERIAL PRIMARY KEY,
    manifest_id   VARCHAR(36) NOT NULL,
    version_id    VARCHAR(36) NULL,
    owner_user_id VARCHAR(20) NULL,
    path          VARCHAR(512) NOT NULL,
    content       BYTEA NOT NULL,
    mime          VARCHAR(128) NOT NULL DEFAULT 'application/octet-stream',
    size          INTEGER NOT NULL,
    is_binary     BOOLEAN NOT NULL DEFAULT FALSE,
    mode          INTEGER NOT NULL DEFAULT 420,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_manifest_files_owner_consistent
        CHECK ((version_id IS NULL AND owner_user_id IS NOT NULL)
            OR (version_id IS NOT NULL AND owner_user_id IS NULL))
);

-- FK 单独声明，便于幂等
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_manifest_files_manifest'
    ) THEN
        ALTER TABLE public.manifest_files
            ADD CONSTRAINT fk_manifest_files_manifest
            FOREIGN KEY (manifest_id) REFERENCES public.manifests(id) ON DELETE CASCADE;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_manifest_files_version'
    ) THEN
        ALTER TABLE public.manifest_files
            ADD CONSTRAINT fk_manifest_files_version
            FOREIGN KEY (version_id) REFERENCES public.manifest_versions(id) ON DELETE CASCADE;
    END IF;
END $$;

-- 部分唯一索引避开 PostgreSQL UNIQUE 多 NULL 陷阱
CREATE UNIQUE INDEX IF NOT EXISTS uq_mf_draft
    ON public.manifest_files (manifest_id, owner_user_id, path) WHERE version_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_mf_published
    ON public.manifest_files (manifest_id, version_id, path) WHERE version_id IS NOT NULL;

-- 列表 / 树查询索引
CREATE INDEX IF NOT EXISTS idx_manifest_files_draft_listing
    ON public.manifest_files (manifest_id, owner_user_id) WHERE version_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_manifest_files_version_listing
    ON public.manifest_files (manifest_id, version_id) WHERE version_id IS NOT NULL;

-- =============================================================================
-- 2. 新表 manifest_deployment_varsets
-- =============================================================================
CREATE TABLE IF NOT EXISTS public.manifest_deployment_varsets (
    id              BIGSERIAL PRIMARY KEY,
    deployment_id   VARCHAR(36) NOT NULL,
    varset_id       VARCHAR(30) NOT NULL,
    priority        INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'uq_mdv'
    ) THEN
        ALTER TABLE public.manifest_deployment_varsets
            ADD CONSTRAINT uq_mdv UNIQUE (deployment_id, varset_id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_mdv_deployment'
    ) THEN
        ALTER TABLE public.manifest_deployment_varsets
            ADD CONSTRAINT fk_mdv_deployment
            FOREIGN KEY (deployment_id) REFERENCES public.manifest_deployments(id) ON DELETE CASCADE;
    END IF;

    -- 注意: variable_sets 的主键是 varset_id (varchar)，需要核对真实 schema
    -- 若 variable_sets.varset_id 不是主键/唯一约束，FK 会失败；改为 service 层校验
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_mdv_varset'
    ) AND EXISTS (
        SELECT 1 FROM information_schema.table_constraints tc
        JOIN information_schema.constraint_column_usage ccu
          ON tc.constraint_name = ccu.constraint_name
        WHERE tc.table_name = 'variable_sets'
          AND tc.constraint_type IN ('PRIMARY KEY', 'UNIQUE')
          AND ccu.column_name = 'varset_id'
    ) THEN
        ALTER TABLE public.manifest_deployment_varsets
            ADD CONSTRAINT fk_mdv_varset
            FOREIGN KEY (varset_id) REFERENCES public.variable_sets(varset_id) ON DELETE RESTRICT;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_mdv_deployment ON public.manifest_deployment_varsets (deployment_id);
CREATE INDEX IF NOT EXISTS idx_mdv_varset ON public.manifest_deployment_varsets (varset_id);

-- =============================================================================
-- 3. manifest_versions 改造: changelog + UNIQUE + version 格式 CHECK
-- =============================================================================
ALTER TABLE public.manifest_versions
    ADD COLUMN IF NOT EXISTS changelog TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_manifest_versions_name
    ON public.manifest_versions (manifest_id, version);

DO $$
BEGIN

    -- version 格式 CHECK: 仅对新写入生效，旧数据用 NOT VALID 容忍
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'chk_manifest_versions_semver'
    ) THEN
        ALTER TABLE public.manifest_versions
            ADD CONSTRAINT chk_manifest_versions_semver
            CHECK (version = 'draft' OR version ~ '^v[0-9]+\.[0-9]+\.[0-9]+$')
            NOT VALID;
    END IF;
END $$;

-- =============================================================================
-- 4. manifest_deployments 改造
-- =============================================================================
-- 4.1 workspace_id 类型修正: int → varchar(50) 对齐全平台语义ID
-- 旧 FK 指向 workspaces(id) 即 uint PK,改类型前必须 drop FK
DO $$
DECLARE
    col_type TEXT;
BEGIN
    SELECT data_type INTO col_type
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'manifest_deployments'
      AND column_name = 'workspace_id';

    IF col_type = 'integer' OR col_type = 'bigint' THEN
        -- 1. drop 旧 FK
        ALTER TABLE public.manifest_deployments
            DROP CONSTRAINT IF EXISTS manifest_deployments_workspace_id_fkey;

        -- 2. backfill: 用 workspaces.workspace_id 翻译 (旧 int FK 指向 workspaces.id)
        --    ALTER COLUMN ... USING 不允许子查询,所以分两步:
        --    a) 先通过 UPDATE 把 int 翻译成字符串列(临时存到一个新列)
        --    b) 再 ALTER 列类型把数字字符串转 varchar(50)
        ALTER TABLE public.manifest_deployments
            ADD COLUMN workspace_id_new VARCHAR(50);

        UPDATE public.manifest_deployments md
            SET workspace_id_new = COALESCE(
                (SELECT w.workspace_id FROM public.workspaces w WHERE w.id = md.workspace_id),
                md.workspace_id::text
            );

        -- 3. drop 旧列 + 重命名新列
        ALTER TABLE public.manifest_deployments DROP COLUMN workspace_id;
        ALTER TABLE public.manifest_deployments RENAME COLUMN workspace_id_new TO workspace_id;
        ALTER TABLE public.manifest_deployments ALTER COLUMN workspace_id SET NOT NULL;

        -- 4. 重建索引
        CREATE INDEX IF NOT EXISTS idx_manifest_deployments_workspace_id
            ON public.manifest_deployments (workspace_id);

        -- 5. 不重建 FK:workspaces.workspace_id 在 model 里有 uniqueIndex,
        --    本期保守走业务层校验,避免与现有 cascade 行为冲突
        RAISE NOTICE 'manifest_deployments.workspace_id 已从 int 转为 varchar(50);旧 FK 已 drop';
    END IF;
END $$;

-- 4.2 status CHECK
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'chk_md_status_valid'
    ) THEN
        -- 旧数据中可能有 'pending' / 'deployed' / 'failed' / 'archived',允许过渡
        -- 新代码只写 'active' / 'uninstalled'
        ALTER TABLE public.manifest_deployments
            ADD CONSTRAINT chk_md_status_valid
            CHECK (status IN ('active','uninstalled','pending','deploying','deployed','failed','archived'))
            NOT VALID;
    END IF;
END $$;

-- 4.3 严格单 manifest: 同一 workspace 仅一条活跃 deployment
CREATE UNIQUE INDEX IF NOT EXISTS uq_manifest_deployments_workspace_active
    ON public.manifest_deployments (workspace_id)
    WHERE status = 'active';

-- =============================================================================
-- 5. workspaces: 新增软链接字段
-- =============================================================================
ALTER TABLE public.workspaces
    ADD COLUMN IF NOT EXISTS manifest_deployment_id VARCHAR(36) NULL;
ALTER TABLE public.workspaces
    ADD COLUMN IF NOT EXISTS manifest_active_tag VARCHAR(50) NULL;
ALTER TABLE public.workspaces
    ADD COLUMN IF NOT EXISTS manifest_subpath VARCHAR(512) NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'chk_workspace_manifest_consistent'
    ) THEN
        ALTER TABLE public.workspaces
            ADD CONSTRAINT chk_workspace_manifest_consistent
            CHECK (
                (manifest_deployment_id IS NULL AND manifest_active_tag IS NULL)
                OR
                (manifest_deployment_id IS NOT NULL AND manifest_active_tag IS NOT NULL)
            )
            NOT VALID;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_workspaces_manifest_deployment
    ON public.workspaces (manifest_deployment_id) WHERE manifest_deployment_id IS NOT NULL;

-- =============================================================================
-- 6. workspace_tasks: 加 external_files 字段(Manifest Run 按钮使用)
-- =============================================================================
ALTER TABLE public.workspace_tasks
    ADD COLUMN IF NOT EXISTS external_files JSONB;

COMMENT ON COLUMN public.workspace_tasks.external_files IS 'Manifest [Run] 按钮临时文件: [{path, content_b64}],executor 走 Run 分支用此而不读 manifest_files,任务跑完即抛';

-- =============================================================================
-- 注释
-- =============================================================================
COMMENT ON TABLE  public.manifest_files                IS 'Manifest 文件存储: 草稿 (version_id NULL + owner_user_id 非空) 与 published 快照 (version_id 非空 + owner_user_id NULL) 统一存放';
COMMENT ON COLUMN public.manifest_files.owner_user_id  IS '草稿所属用户;published 行此字段为 NULL';
COMMENT ON COLUMN public.manifest_files.mode           IS 'POSIX file mode,本期未使用,留给未来 Git over HTTPS';
COMMENT ON TABLE  public.manifest_deployment_varsets   IS 'Manifest deployment 关联的 varset (per-deployment,priority 数字大者优先级高)';
COMMENT ON COLUMN public.workspaces.manifest_deployment_id IS '软链接到 manifest_deployments.id;NULL 表示未装 manifest';
COMMENT ON COLUMN public.workspaces.manifest_active_tag    IS '当前激活的 manifest 版本号 (vX.Y.Z);与 manifest_deployment_id 一致同生同死';
COMMENT ON COLUMN public.workspaces.manifest_subpath       IS 'terraform 执行根目录,空 = manifest 根';
