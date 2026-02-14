-- AI 错误分析功能数据库迁移脚本
-- 创建日期: 2025-10-16

-- 1. AI 配置表
CREATE TABLE IF NOT EXISTS ai_configs (
    id SERIAL PRIMARY KEY,
    service_type VARCHAR(50) NOT NULL DEFAULT 'bedrock',  -- bedrock, openai, claude, ollama
    aws_region VARCHAR(50),                                -- Bedrock 专用
    model_id VARCHAR(200),                                 -- 模型 ID
    custom_prompt TEXT,                                    -- 用户自定义补充 prompt
    enabled BOOLEAN DEFAULT true,                          -- 是否启用
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 2. AI 错误分析表
CREATE TABLE IF NOT EXISTS ai_error_analyses (
    id SERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id),
    error_message TEXT NOT NULL,
    error_type VARCHAR(100),                               -- 错误类型
    root_cause TEXT,                                       -- 根本原因
    solutions JSONB,                                       -- 解决方案数组
    prevention TEXT,                                       -- 预防措施
    severity VARCHAR(20),                                  -- low, medium, high, critical
    analysis_duration INTEGER,                             -- 分析耗时（毫秒）
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(task_id)                                        -- 每个任务只保存最新的分析结果
);

-- 3. AI 分析速率限制表
CREATE TABLE IF NOT EXISTS ai_analysis_rate_limits (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    last_analysis_at TIMESTAMP NOT NULL,
    UNIQUE(user_id)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_ai_analyses_task ON ai_error_analyses(task_id);
CREATE INDEX IF NOT EXISTS idx_ai_analyses_user ON ai_error_analyses(user_id);
CREATE INDEX IF NOT EXISTS idx_rate_limits_user ON ai_analysis_rate_limits(user_id);

-- 插入默认配置
INSERT INTO ai_configs (service_type, aws_region, model_id, custom_prompt, enabled) 
VALUES ('bedrock', 'us-east-1', 'anthropic.claude-3-5-sonnet-20240620-v1:0', '', false)
ON CONFLICT DO NOTHING;

-- 添加注释
COMMENT ON TABLE ai_configs IS 'AI 服务配置表';
COMMENT ON TABLE ai_error_analyses IS 'AI 错误分析结果表';
COMMENT ON TABLE ai_analysis_rate_limits IS 'AI 分析速率限制表';

COMMENT ON COLUMN ai_configs.service_type IS 'AI 服务类型: bedrock, openai, claude, ollama';
COMMENT ON COLUMN ai_configs.aws_region IS 'AWS Bedrock 区域';
COMMENT ON COLUMN ai_configs.model_id IS '模型 ID';
COMMENT ON COLUMN ai_configs.custom_prompt IS '用户自定义补充 prompt';
COMMENT ON COLUMN ai_configs.enabled IS '是否启用 AI 分析';

COMMENT ON COLUMN ai_error_analyses.task_id IS '关联的任务 ID';
COMMENT ON COLUMN ai_error_analyses.user_id IS '发起分析的用户 ID';
COMMENT ON COLUMN ai_error_analyses.error_message IS '错误信息';
COMMENT ON COLUMN ai_error_analyses.error_type IS '错误类型';
COMMENT ON COLUMN ai_error_analyses.root_cause IS '根本原因';
COMMENT ON COLUMN ai_error_analyses.solutions IS '解决方案数组 (JSON)';
COMMENT ON COLUMN ai_error_analyses.prevention IS '预防措施';
COMMENT ON COLUMN ai_error_analyses.severity IS '严重程度: low, medium, high, critical';
COMMENT ON COLUMN ai_error_analyses.analysis_duration IS '分析耗时（毫秒）';

COMMENT ON COLUMN ai_analysis_rate_limits.user_id IS '用户 ID';
COMMENT ON COLUMN ai_analysis_rate_limits.last_analysis_at IS '最后一次分析时间';
