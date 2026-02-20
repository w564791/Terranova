-- 配置生成流程优化 - 阶段 1 & 2: 添加 description 字段并更新现有数据
-- 执行方式: psql -h localhost -p 15432 -U postgres -d iac_platform -f scripts/add_description_to_skills.sql

-- ========== 阶段 1: 数据模型变更 ==========

-- 1. 添加 description 字段（如果不存在）
ALTER TABLE skills ADD COLUMN IF NOT EXISTS description VARCHAR(500);

-- 2. 添加注释
COMMENT ON COLUMN skills.description IS 'Skill 的简短描述，用于 AI 智能选择，限制 500 字符';

-- 3. 从 metadata 迁移现有数据（如果 metadata.description 存在且 description 为空）
UPDATE skills 
SET description = metadata->>'description' 
WHERE metadata->>'description' IS NOT NULL 
  AND metadata->>'description' != ''
  AND (description IS NULL OR description = '');

-- ========== 阶段 2: 更新现有 Skills 的 description ==========

-- Domain Skills (15 个)
UPDATE skills SET description = 'Terraform Module 最佳实践，包括命名规范、变量设计、输出定义等' 
WHERE name = 'terraform_module_best_practices' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'AWS 资源标签规范，定义必填标签、可选标签和命名规则' 
WHERE name = 'aws_resource_tagging' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'Skill 文档编写标准，用于生成 Module Skill 文档' 
WHERE name = 'skill_doc_writing_standards' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'Schema 验证规则，用于验证用户输入是否符合 OpenAPI Schema 约束' 
WHERE name = 'schema_validation_rules' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'OpenAPI Schema 解释规则，用于理解 Module 的参数定义' 
WHERE name = 'openapi_schema_interpretation' AND (description IS NULL OR description = '');

UPDATE skills SET description = '从 CMDB 查询结果中匹配用户需要的资源，支持精确匹配和模糊匹配' 
WHERE name = 'cmdb_resource_matching' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'CMDB 资源类型与 Terraform 资源类型的映射关系' 
WHERE name = 'cmdb_resource_types' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'AWS 区域映射规则，处理区域代码和名称的转换' 
WHERE name = 'region_mapping' AND (description IS NULL OR description = '');

UPDATE skills SET description = '安全威胁检测规则，用于识别恶意输入和越狱攻击' 
WHERE name = 'security_detection_rules' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'S3 桶策略模式，用于生成允许/拒绝特定 Principal 访问的策略' 
WHERE name = 'aws_s3_policy_patterns' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'KMS 密钥策略模式，包括密钥管理员、密钥用户、服务集成等场景' 
WHERE name = 'aws_kms_policy_patterns' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'Secrets Manager 策略模式，包括基本访问、跨账户、Lambda 轮换等场景' 
WHERE name = 'aws_secrets_manager_policy_patterns' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'AWS 策略核心原则，包括最小权限、显式拒绝、策略结构等' 
WHERE name = 'aws_policy_core_principles' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'IAM 身份策略模式，包括 EC2、S3、Lambda、RDS 等服务的策略示例' 
WHERE name = 'aws_iam_policy_patterns' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'AWS 条件键参考，包括全局条件键、服务特定条件键和使用示例' 
WHERE name = 'aws_condition_keys_reference' AND (description IS NULL OR description = '');

-- Foundation Skills (5 个)
UPDATE skills SET description = 'Markdown 输出格式规范' 
WHERE name = 'markdown_output_format' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'JSON 输出格式规范' 
WHERE name = 'json_output_format' AND (description IS NULL OR description = '');

UPDATE skills SET description = '占位符标准规范' 
WHERE name = 'placeholder_standard' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'JSON Schema 解析器' 
WHERE name = 'json_schema_parser' AND (description IS NULL OR description = '');

UPDATE skills SET description = '通用输出格式规范' 
WHERE name = 'output_format_standard' AND (description IS NULL OR description = '');

-- Task Skills (5 个)
UPDATE skills SET description = 'CMDB 查询计划生成的任务工作流' 
WHERE name = 'cmdb_query_plan_workflow' AND (description IS NULL OR description = '');

UPDATE skills SET description = '检测用户输入是否安全，防止越狱攻击、提示注入等安全威胁' 
WHERE name = 'intent_assertion_workflow' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'CMDB 需求评估工作流，判断用户需求是否需要查询 CMDB' 
WHERE name = 'cmdb_need_assessment_workflow' AND (description IS NULL OR description = '');

UPDATE skills SET description = '资源配置生成工作流，根据用户描述生成 Terraform 配置' 
WHERE name = 'resource_generation_workflow' AND (description IS NULL OR description = '');

UPDATE skills SET description = 'Module Skill 生成工作流，从 Module Schema 自动生成 Skill 文档' 
WHERE name = 'module_skill_generation_workflow' AND (description IS NULL OR description = '');

-- ========== 验证结果 ==========

-- 查看更新后的 Skills
SELECT name, display_name, layer, description 
FROM skills 
WHERE is_active = true 
ORDER BY layer, priority;

-- 统计 description 填充情况
SELECT 
    layer,
    COUNT(*) as total,
    COUNT(CASE WHEN description IS NOT NULL AND description != '' THEN 1 END) as with_description,
    COUNT(CASE WHEN description IS NULL OR description = '' THEN 1 END) as without_description
FROM skills 
WHERE is_active = true
GROUP BY layer
ORDER BY layer;