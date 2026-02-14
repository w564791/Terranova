-- Terraform执行流程优化 - Phase 1
-- 添加plan hash字段用于优化Apply阶段
-- 创建时间: 2025-11-08
-- 目的: 保持工作目录，通过hash验证避免重复init

-- 添加plan hash字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS plan_hash VARCHAR(64);

-- 添加索引以提高查询性能
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_plan_hash 
ON workspace_tasks(plan_hash);

-- 添加注释
COMMENT ON COLUMN workspace_tasks.plan_hash IS 'SHA256 hash of plan.out file for verification';
