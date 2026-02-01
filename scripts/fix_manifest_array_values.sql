-- 修复 Manifest 中错误的数组值（字符串格式应该是数组格式）
-- 这个脚本会查找所有包含换行符的字符串值，并将其转换为数组

-- 首先查看有问题的数据
SELECT 
    mv.id as version_id,
    mv.manifest_id,
    mv.version,
    jsonb_pretty(mv.nodes) as nodes
FROM manifest_versions mv
WHERE mv.nodes::text LIKE '%\n%'
LIMIT 10;

-- 如果需要修复，可以使用以下方法：
-- 1. 导出数据
-- 2. 在应用层处理（推荐）
-- 3. 或者使用 PL/pgSQL 函数

-- 注意：由于 JSON 结构复杂，建议在应用层处理
-- 可以在 ManifestEditor 加载时自动修复

-- 示例：查看所有 manifest_versions 的 nodes 数据
SELECT 
    mv.id,
    mv.manifest_id,
    mv.version,
    mv.is_draft,
    jsonb_array_length(mv.nodes) as node_count
FROM manifest_versions mv
ORDER BY mv.updated_at DESC
LIMIT 20;
