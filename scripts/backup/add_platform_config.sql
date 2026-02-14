-- 添加平台配置项到 system_configs 表
-- 这些配置用于 Run Task callback URL 和 Agent 连接
-- 注意：value 字段是 JSONB 类型，需要用 JSON 格式

-- 平台基础 URL（用于 Run Task callback、Agent 连接等）
INSERT INTO system_configs (key, value, description, updated_at)
VALUES ('platform_base_url', '"http://localhost:8080"', '平台基础URL，用于Run Task回调和Agent连接', NOW())
ON CONFLICT (key) DO UPDATE SET 
    description = EXCLUDED.description,
    updated_at = NOW();

-- 平台协议（http/https）
INSERT INTO system_configs (key, value, description, updated_at)
VALUES ('platform_protocol', '"http"', '平台协议（http或https）', NOW())
ON CONFLICT (key) DO UPDATE SET 
    description = EXCLUDED.description,
    updated_at = NOW();

-- 平台主机地址
INSERT INTO system_configs (key, value, description, updated_at)
VALUES ('platform_host', '"localhost"', '平台主机地址', NOW())
ON CONFLICT (key) DO UPDATE SET 
    description = EXCLUDED.description,
    updated_at = NOW();

-- 平台 API 端口（已存在，更新描述）
UPDATE system_configs SET description = '平台API端口' WHERE key = 'platform_api_port';

-- Agent CC 端口（已存在，更新描述）
UPDATE system_configs SET description = 'Agent控制通道端口' WHERE key = 'platform_cc_port';

-- 验证插入结果
SELECT * FROM system_configs WHERE key LIKE 'platform_%' ORDER BY key;
