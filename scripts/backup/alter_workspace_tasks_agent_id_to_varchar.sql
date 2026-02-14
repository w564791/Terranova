-- 将 workspace_tasks 表的 agent_id 字段从 integer 改为 varchar(50)
-- 这样可以存储 agent 的语义化ID（如 agent-pool-xxx-timestamp）

-- 1. 先删除外键约束（如果存在）
-- ALTER TABLE workspace_tasks DROP CONSTRAINT IF EXISTS fk_workspace_tasks_agent;

-- 2. 修改字段类型
ALTER TABLE workspace_tasks 
ALTER COLUMN agent_id TYPE varchar(50) USING agent_id::varchar;

-- 3. 确保索引存在
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_agent_id ON workspace_tasks(agent_id);

-- 验证修改
\d workspace_tasks
