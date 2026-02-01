-- 清理重复的 Schema 版本
-- 保留每个 module_id 中最新的一个 Schema，删除其他重复的

-- 首先查看当前状态
SELECT module_id, version, COUNT(*) as count, 
       MIN(created_at) as first_created, 
       MAX(created_at) as last_created
FROM schemas 
GROUP BY module_id, version 
HAVING COUNT(*) > 1;

-- 删除重复的 Schema，保留每个 module_id + version 组合中最新的一个
DELETE FROM schemas 
WHERE id NOT IN (
    SELECT MAX(id) 
    FROM schemas 
    GROUP BY module_id, version
);

-- 重新编号版本（可选）
-- 为每个模块的 Schema 重新分配简单的版本号 1, 2, 3...
-- 注意：这会修改现有版本号

-- 创建临时表存储新版本号
WITH ranked_schemas AS (
    SELECT id, module_id, version,
           ROW_NUMBER() OVER (PARTITION BY module_id ORDER BY created_at ASC) as new_version
    FROM schemas
)
UPDATE schemas s
SET version = rs.new_version::text
FROM ranked_schemas rs
WHERE s.id = rs.id;

-- 验证结果
SELECT module_id, id, version, status, created_at 
FROM schemas 
ORDER BY module_id, created_at DESC;
