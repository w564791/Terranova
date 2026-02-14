-- AI Provider 能力场景管理迁移脚本
-- 添加 capabilities 和 priority 字段

-- 1. 添加新字段
ALTER TABLE ai_configs 
ADD COLUMN IF NOT EXISTS capabilities JSONB DEFAULT '[]',
ADD COLUMN IF NOT EXISTS priority INTEGER DEFAULT 0;

-- 2. 创建索引
CREATE INDEX IF NOT EXISTS idx_ai_configs_priority 
ON ai_configs(priority DESC);

CREATE INDEX IF NOT EXISTS idx_ai_configs_capabilities 
ON ai_configs USING GIN(capabilities);

-- 3. 迁移现有数据
-- 将当前启用的第一个配置设置为默认配置
UPDATE ai_configs 
SET capabilities = '["*"]'
WHERE enabled = true 
AND id = (SELECT id FROM ai_configs WHERE enabled = true ORDER BY id LIMIT 1);

-- 其他启用的配置设置为未配置
UPDATE ai_configs 
SET capabilities = '[]'
WHERE enabled = true 
AND capabilities IS NULL OR capabilities = '[]'::jsonb;

-- 4. 添加注释
COMMENT ON COLUMN ai_configs.capabilities IS '支持的能力场景，["*"]表示默认配置，[]表示未配置';
COMMENT ON COLUMN ai_configs.priority IS '优先级，数值越大优先级越高';
