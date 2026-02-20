-- 清理K8s Job的pool_tokens记录
-- 用于重新创建Job时使用新的配置

-- 查看要删除的记录
SELECT token_name, k8s_job_name, k8s_namespace, pool_id, created_at
FROM pool_tokens
WHERE k8s_job_name = 'ws-0yrm628p8h3f9mw0-363';

-- 删除记录
DELETE FROM pool_tokens WHERE k8s_job_name = 'ws-0yrm628p8h3f9mw0-363';

-- 确认删除
SELECT COUNT(*) as remaining_count FROM pool_tokens WHERE k8s_job_name = 'ws-0yrm628p8h3f9mw0-363';
