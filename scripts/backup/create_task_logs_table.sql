-- 创建task_logs表
-- 用于记录任务执行日志

CREATE TABLE IF NOT EXISTS task_logs (
    id SERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    phase VARCHAR(50) NOT NULL,
    content TEXT NOT NULL,
    level VARCHAR(20) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_task_logs_task ON task_logs(task_id);
CREATE INDEX IF NOT EXISTS idx_task_logs_phase ON task_logs(phase);
CREATE INDEX IF NOT EXISTS idx_task_logs_level ON task_logs(level);
CREATE INDEX IF NOT EXISTS idx_task_logs_created_at ON task_logs(created_at DESC);

-- 验证表创建
SELECT 
    table_name,
    (SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'task_logs') as column_count
FROM information_schema.tables
WHERE table_schema = 'public' AND table_name = 'task_logs';
