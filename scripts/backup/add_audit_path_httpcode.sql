-- 为access_logs表添加path和http_code字段

ALTER TABLE access_logs 
ADD COLUMN IF NOT EXISTS request_path TEXT,
ADD COLUMN IF NOT EXISTS http_code INTEGER;

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_access_logs_http_code ON access_logs(http_code);
CREATE INDEX IF NOT EXISTS idx_access_logs_request_path ON access_logs(request_path);

COMMENT ON COLUMN access_logs.request_path IS '请求路径';
COMMENT ON COLUMN access_logs.http_code IS 'HTTP状态码';
