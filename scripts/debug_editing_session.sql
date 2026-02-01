-- 调试资源编辑会话问题
-- 用于排查"编辑已被其他窗口接管"的误报

-- 1. 查看资源20的所有锁
SELECT 
    id,
    resource_id,
    editing_user_id,
    session_id,
    last_heartbeat,
    NOW() - last_heartbeat AS time_since_heartbeat,
    CASE 
        WHEN NOW() - last_heartbeat > INTERVAL '2 minutes' THEN '已过期'
        WHEN NOW() - last_heartbeat > INTERVAL '1 minute' THEN '警告'
        ELSE '正常'
    END AS status
FROM resource_locks 
WHERE resource_id = 20
ORDER BY last_heartbeat DESC;

-- 2. 查看资源20的所有drift
SELECT 
    id,
    resource_id,
    user_id,
    session_id,
    base_version,
    status,
    last_heartbeat,
    NOW() - last_heartbeat AS time_since_heartbeat
FROM resource_drifts 
WHERE resource_id = 20
ORDER BY last_heartbeat DESC;

-- 3. 统计信息
SELECT 
    '锁总数' AS type,
    COUNT(*) AS count
FROM resource_locks
WHERE resource_id = 20
UNION ALL
SELECT 
    'Drift总数' AS type,
    COUNT(*) AS count
FROM resource_drifts
WHERE resource_id = 20;
