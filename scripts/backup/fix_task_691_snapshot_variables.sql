-- 修复任务691的快照变量数据
-- 将旧格式(只有引用)转换为新格式(包含完整数据)

-- 首先查看当前的快照数据
SELECT 
    id,
    workspace_id,
    jsonb_pretty(snapshot_variables) as current_snapshot
FROM workspace_tasks 
WHERE id = 691;

-- 更新快照变量为完整数据格式
-- 从workspace_variables表查询完整的变量数据并更新到快照中
UPDATE workspace_tasks t
SET snapshot_variables = (
    SELECT jsonb_agg(
        jsonb_build_object(
            'workspace_id', wv.workspace_id,
            'variable_id', wv.variable_id,
            'version', wv.version,
            'variable_type', wv.variable_type,
            'key', wv.key,
            'value', wv.value,
            'sensitive', wv.sensitive,
            'description', wv.description,
            'value_format', wv.value_format
        )
    )
    FROM (
        SELECT DISTINCT ON (variable_id)
            workspace_id,
            variable_id,
            version,
            variable_type,
            key,
            value,
            sensitive,
            description,
            value_format
        FROM workspace_variables
        WHERE workspace_id = t.workspace_id
          AND is_deleted = false
          AND variable_type = 'terraform'
        ORDER BY variable_id, version DESC
    ) wv
)
WHERE id = 691
  AND snapshot_variables IS NOT NULL;

-- 验证更新后的数据
SELECT 
    id,
    workspace_id,
    jsonb_pretty(snapshot_variables) as updated_snapshot,
    jsonb_array_length(snapshot_variables) as variable_count
FROM workspace_tasks 
WHERE id = 691;

-- 检查第一个变量是否包含key字段
SELECT 
    id,
    jsonb_array_element(snapshot_variables, 0)->>'key' as first_variable_key,
    jsonb_array_element(snapshot_variables, 0)->>'value' as first_variable_value
FROM workspace_tasks 
WHERE id = 691;
