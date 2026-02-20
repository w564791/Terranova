-- 添加审计日志的额外字段
-- 为access_logs表添加user_agent, request_body, request_headers字段

ALTER TABLE access_logs 
ADD COLUMN IF NOT EXISTS user_agent TEXT,
ADD COLUMN IF NOT EXISTS request_body TEXT,
ADD COLUMN IF NOT EXISTS request_headers JSONB;

-- 添加索引以提高查询性能
CREATE INDEX IF NOT EXISTS idx_access_logs_user_id ON access_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_access_logs_accessed_at ON access_logs(accessed_at);
CREATE INDEX IF NOT EXISTS idx_access_logs_resource_type ON access_logs(resource_type);
CREATE INDEX IF NOT EXISTS idx_access_logs_is_allowed ON access_logs(is_allowed);

COMMENT ON COLUMN access_logs.user_agent IS 'User Agent字符串';
COMMENT ON COLUMN access_logs.request_body IS '请求体内容';
COMMENT ON COLUMN access_logs.request_headers IS '请求头（JSON格式）';
