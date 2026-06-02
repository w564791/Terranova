-- Manifest deployment 变量注入执行路径修复
-- 设计文档: specs/2026-05-06-manifest-refactor-spec.md §11
--
-- 背景: deployment 选定的 varsets 已能通过变量快照(引用机制)参与 plan/apply,
--       但 variable_overrides(应急覆盖,最高优先级,扁平 key=value 无 variable_id)
--       无法用引用快照表达。改为任务创建时把当时 active deployment 的 overrides
--       固化到任务行,执行时 overlay,语义与现有快照一致(可复现)。
--
-- 幂等: IF NOT EXISTS。

ALTER TABLE public.workspace_tasks
    ADD COLUMN IF NOT EXISTS variable_overrides JSONB;

COMMENT ON COLUMN public.workspace_tasks.variable_overrides IS
    'Manifest deployment 应急覆盖快照(任务创建时固化,执行时 overlay 到 Terraform 变量,最高优先级)。扁平 {"key":"value"}。';
