-- Manifest 重构 PR4-4: 清理旧画布遗留 schema
-- 设计文档: specs/2026-05-06-manifest-refactor-spec.md
--
-- 背景: manifest 已从"拖拽画布"迁移到 VS Code Web 工作区,版本内容统一存
--       manifest_files (version_id = manifest_versions.id),manifest_versions
--       不再需要画布字段。代码侧 (PR4-3) 已删除对应 struct 字段与 handler。
--
-- 本迁移幂等: 全程 IF EXISTS / DO blocks 检查列与索引存在性。
-- 无显式 BEGIN/COMMIT: 每个 DO block / 语句各自原子。

-- =============================================================================
-- 1. manifest_versions: 删除旧画布字段
--    canvas_data / nodes / edges / hcl_content / is_draft
--    新模型: 版本只是 manifest_files 快照的元信息行
-- =============================================================================

-- 先清理依赖 is_draft 的旧索引(add_manifest_v2 之前的 schema 可能建过)
DROP INDEX IF EXISTS public.idx_manifest_versions_is_draft;

-- 删除旧的 version='draft' 行: 新模型不再用 manifest_versions 存草稿,
-- 草稿统一在 manifest_files (version_id IS NULL)。这些行无 manifest_files 快照,
-- 留着只会污染 latest_version 查询。先删行再删列。
DELETE FROM public.manifest_versions WHERE version = 'draft';

ALTER TABLE public.manifest_versions DROP COLUMN IF EXISTS canvas_data;
ALTER TABLE public.manifest_versions DROP COLUMN IF EXISTS nodes;
ALTER TABLE public.manifest_versions DROP COLUMN IF EXISTS edges;
ALTER TABLE public.manifest_versions DROP COLUMN IF EXISTS hcl_content;
ALTER TABLE public.manifest_versions DROP COLUMN IF EXISTS is_draft;

COMMENT ON TABLE public.manifest_versions IS
    'Manifest 版本元信息行;文件内容存 manifest_files (version_id = 本行 id)。画布字段已于 PR4-4 移除。';

-- =============================================================================
-- 2. manifest_deployment_resources: 标记弃用
--    新模型部署 (install/upgrade/uninstall) 是纯元信息操作,不再写此表
--    (资源到 module 的映射改由 workspace_resources.tf_code 解析)。
--    保守起见本期只加弃用注释,不 drop 表,留待后续确认无历史数据依赖后再清。
-- =============================================================================

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'public' AND table_name = 'manifest_deployment_resources'
    ) THEN
        COMMENT ON TABLE public.manifest_deployment_resources IS
            'DEPRECATED (PR4-4): 旧画布部署的节点->资源映射表;新模型不再写入,保留仅为历史数据。';
        RAISE NOTICE 'manifest_deployment_resources 已标记 DEPRECATED (未 drop)';
    END IF;
END $$;
