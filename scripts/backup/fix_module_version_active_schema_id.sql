-- 修复存量数据：同步 module_versions.active_schema_id 与 schemas.status = 'active'
-- 
-- 问题：当 Schema 被设为 active 时，module_versions.active_schema_id 没有同步更新
-- 解决：将每个 Module Version 的 active_schema_id 更新为该版本关联的 active Schema

-- 1. 查看当前不一致的数据
SELECT 
    mv.id as version_id,
    mv.version,
    m.name as module_name,
    mv.active_schema_id as current_active_schema_id,
    s.id as should_be_active_schema_id,
    s.version as schema_version,
    s.status as schema_status
FROM module_versions mv
JOIN modules m ON mv.module_id = m.id
LEFT JOIN schemas s ON s.module_version_id = mv.id AND s.status = 'active'
WHERE mv.active_schema_id IS DISTINCT FROM s.id
ORDER BY m.name, mv.version;

-- 2. 修复：更新 module_versions.active_schema_id 为关联的 active Schema
UPDATE module_versions mv
SET active_schema_id = (
    SELECT s.id 
    FROM schemas s 
    WHERE s.module_version_id = mv.id 
    AND s.status = 'active'
    ORDER BY s.created_at DESC
    LIMIT 1
)
WHERE EXISTS (
    SELECT 1 
    FROM schemas s 
    WHERE s.module_version_id = mv.id 
    AND s.status = 'active'
);

-- 3. 对于没有关联 module_version_id 的旧数据，按 module_id 匹配
-- 这是兼容旧数据的逻辑：如果 Schema 没有 module_version_id，但有 module_id
UPDATE module_versions mv
SET active_schema_id = (
    SELECT s.id 
    FROM schemas s 
    WHERE s.module_id = mv.module_id 
    AND s.module_version_id IS NULL
    AND s.status = 'active'
    ORDER BY s.created_at DESC
    LIMIT 1
)
WHERE mv.active_schema_id IS NULL
AND EXISTS (
    SELECT 1 
    FROM schemas s 
    WHERE s.module_id = mv.module_id 
    AND s.module_version_id IS NULL
    AND s.status = 'active'
);

-- 4. 验证修复结果
SELECT 
    mv.id as version_id,
    mv.version,
    m.name as module_name,
    mv.active_schema_id,
    s.version as schema_version,
    s.status as schema_status
FROM module_versions mv
JOIN modules m ON mv.module_id = m.id
LEFT JOIN schemas s ON mv.active_schema_id = s.id
ORDER BY m.name, mv.version;