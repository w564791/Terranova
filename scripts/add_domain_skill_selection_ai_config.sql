-- 配置生成流程优化 - 阶段 5: 添加 domain_skill_selection AI 配置
-- 执行方式: PGPASSWORD=postgres123 psql -h localhost -p 15432 -U postgres -d iac_platform -f scripts/add_domain_skill_selection_ai_config.sql

-- 1. 检查是否已存在
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM ai_configs WHERE service_type = 'domain_skill_selection') THEN
        INSERT INTO ai_configs (
            service_type,
            aws_region,
            model_id,
            custom_prompt,
            enabled,
            capabilities,
            mode,
            skill_composition,
            priority,
            created_at,
            updated_at
        ) VALUES (
            'domain_skill_selection',
            'us-east-1',
            'anthropic.claude-3-5-sonnet-20241022-v2:0',
            'AI 智能选择 Domain Skills - 根据用户需求自动选择相关的 Domain Skills',
            false,  -- 非默认配置，enabled=false
            '["domain_skill_selection"]',
            'prompt',
            '{}',
            100,
            NOW(),
            NOW()
        );
        RAISE NOTICE 'domain_skill_selection AI 配置已创建';
    ELSE
        RAISE NOTICE 'domain_skill_selection AI 配置已存在，跳过创建';
    END IF;
END $$;

-- 2. 验证配置
SELECT id, service_type, aws_region, model_id, capabilities, mode, priority, enabled
FROM ai_configs 
WHERE service_type = 'domain_skill_selection';

-- 3. 查看所有 AI 配置
SELECT id, service_type, capabilities, mode, priority, enabled
FROM ai_configs 
WHERE enabled = true
ORDER BY priority;