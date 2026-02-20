-- 为 AI 表单生成功能添加配置
-- 需要在 ai_configs 表中添加支持 form_generation 能力的配置

-- 检查是否已有支持 form_generation 的配置
DO $$
BEGIN
    -- 如果已有配置支持 form_generation，则更新
    IF EXISTS (
        SELECT 1 FROM ai_configs 
        WHERE capabilities::text LIKE '%form_generation%'
    ) THEN
        RAISE NOTICE 'AI config with form_generation capability already exists';
    ELSE
        -- 检查是否有现有的 AI 配置，如果有则添加 form_generation 能力
        IF EXISTS (SELECT 1 FROM ai_configs WHERE enabled = true LIMIT 1) THEN
            -- 更新第一个启用的配置，添加 form_generation 能力
            UPDATE ai_configs 
            SET capabilities = capabilities || '["form_generation"]'::jsonb
            WHERE id = (SELECT id FROM ai_configs WHERE enabled = true ORDER BY priority DESC LIMIT 1)
            AND NOT (capabilities::text LIKE '%form_generation%');
            
            RAISE NOTICE 'Added form_generation capability to existing AI config';
        ELSE
            RAISE NOTICE 'No enabled AI config found. Please configure AI service first.';
        END IF;
    END IF;
END $$;

-- 查看当前 AI 配置
SELECT id, service_type, model_id, capabilities, priority, enabled 
FROM ai_configs 
ORDER BY priority DESC;
