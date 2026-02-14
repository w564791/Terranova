-- 更新 Skills 的 tags 和 domain_tags
-- 用于支持双向标签匹配的 Domain Skill 自动发现功能
-- 执行时间: 2026-01-29

-- ========== 更新 Domain Skills 的 tags ==========

-- schema_validation_rules: Schema 验证规则
UPDATE skills 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb),
    '{tags}',
    '["schema", "validation", "openapi", "constraint", "form"]'::jsonb
)
WHERE name = 'schema_validation_rules';

-- cmdb_resource_matching: CMDB 资源匹配
UPDATE skills 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb),
    '{tags}',
    '["cmdb", "matching", "resource", "lookup", "search"]'::jsonb
)
WHERE name = 'cmdb_resource_matching';

-- cmdb_resource_types: CMDB 资源类型
UPDATE skills 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb),
    '{tags}',
    '["cmdb", "resource_type", "mapping", "terraform"]'::jsonb
)
WHERE name = 'cmdb_resource_types';

-- region_mapping: 区域映射
UPDATE skills 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb),
    '{tags}',
    '["region", "mapping", "aws", "location", "geography"]'::jsonb
)
WHERE name = 'region_mapping';

-- ========== 更新 Task Skills 的 domain_tags ==========

-- resource_generation_workflow: 资源配置生成工作流
UPDATE skills 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb),
    '{domain_tags}',
    '["schema", "validation", "security"]'::jsonb
)
WHERE name = 'resource_generation_workflow';

-- cmdb_query_plan_workflow: CMDB 查询计划工作流
UPDATE skills 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb),
    '{domain_tags}',
    '["cmdb", "resource_type", "region", "mapping"]'::jsonb
)
WHERE name = 'cmdb_query_plan_workflow';

-- intent_assertion_workflow: 意图断言工作流
-- 不需要 Domain Skills，保持空
UPDATE skills 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb),
    '{domain_tags}',
    '[]'::jsonb
)
WHERE name = 'intent_assertion_workflow';

-- module_skill_generation_workflow: Module Skill 生成工作流
UPDATE skills 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb),
    '{domain_tags}',
    '["schema", "module"]'::jsonb
)
WHERE name = 'module_skill_generation_workflow';

-- ========== 验证更新结果 ==========

-- 查看 Domain Skills 的 tags
SELECT name, layer, metadata->>'tags' as tags
FROM skills
WHERE layer = 'domain'
ORDER BY name;

-- 查看 Task Skills 的 domain_tags
SELECT name, layer, metadata->>'domain_tags' as domain_tags
FROM skills
WHERE layer = 'task'
ORDER BY name;

-- 测试标签匹配查询
-- 查找 tags 包含 "schema" 的 Domain Skills
SELECT name, metadata->>'tags' as tags
FROM skills
WHERE layer = 'domain' 
  AND is_active = true
  AND metadata->>'tags' LIKE '%schema%';

-- 查找 tags 包含 "cmdb" 的 Domain Skills
SELECT name, metadata->>'tags' as tags
FROM skills
WHERE layer = 'domain' 
  AND is_active = true
  AND metadata->>'tags' LIKE '%cmdb%';