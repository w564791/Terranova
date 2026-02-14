-- 为 Global Run Task 添加默认运行阶段和执行级别字段
-- 执行时间: 2025-12-07

-- 添加 global_stages 字段（全局任务默认执行阶段，逗号分隔）
ALTER TABLE run_tasks ADD COLUMN IF NOT EXISTS global_stages VARCHAR(100) DEFAULT 'post_plan';

-- 添加 global_enforcement_level 字段（全局任务默认执行级别）
ALTER TABLE run_tasks ADD COLUMN IF NOT EXISTS global_enforcement_level VARCHAR(20) DEFAULT 'advisory';

-- 更新现有的 Global Run Task，设置默认值
UPDATE run_tasks 
SET global_stages = 'post_plan', 
    global_enforcement_level = 'advisory' 
WHERE is_global = true 
  AND (global_stages IS NULL OR global_stages = '');

-- 添加注释
COMMENT ON COLUMN run_tasks.global_stages IS '全局任务默认执行阶段，逗号分隔，如 post_plan,pre_apply';
COMMENT ON COLUMN run_tasks.global_enforcement_level IS '全局任务默认执行级别: advisory 或 mandatory';
