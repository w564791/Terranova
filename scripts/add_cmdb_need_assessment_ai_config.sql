-- 创建 CMDB 需求评估 AI 配置
-- 用于判断用户需求是否需要查询 CMDB 获取现有资源
-- enabled=true 表示全局默认，priority=-10 表示最低优先级

INSERT INTO ai_configs (
    service_type,
    aws_region,
    model_id,
    custom_prompt,
    enabled,
    rate_limit_seconds,
    use_inference_profile,
    base_url,
    api_key,
    capabilities,
    priority,
    capability_prompts,
    top_k,
    similarity_threshold,
    embedding_batch_enabled,
    embedding_batch_size,
    mode,
    skill_composition,
    created_at,
    updated_at
) VALUES (
    'bedrock',
    'eu-west-1',
    'anthropic.claude-3-haiku-20240307-v1:0',  -- 使用更便宜的 Haiku 模型，因为这是轻量级判断任务
    '',
    false,  -- 专用配置，不是全局默认
    10,     -- 较短的速率限制，因为这是快速判断
    true,
    '',
    '',
    '["cmdb_need_assessment"]',
    -10,   -- 最低优先级
    '{}',  -- 使用 Skill 模式，不需要 capability_prompts
    50,
    0.3,
    false,
    10,
    'skill',
    '{
        "task_skill": "cmdb_need_assessment_workflow",
        "domain_skills": ["cmdb_resource_types"],
        "foundation_skills": ["output_format_standard"],
        "conditional_rules": [],
        "auto_load_module_skill": false
    }',
    NOW(),
    NOW()
);

-- 验证插入结果
SELECT id, service_type, model_id, mode, capabilities, priority, enabled, skill_composition 
FROM ai_configs 
WHERE capabilities::text LIKE '%cmdb_need_assessment%';