-- 创建接管请求表
CREATE TABLE IF NOT EXISTS takeover_requests (
    id SERIAL PRIMARY KEY,
    resource_id INTEGER NOT NULL,
    requester_user_id VARCHAR(255) NOT NULL,
    requester_name VARCHAR(255) NOT NULL,
    requester_session VARCHAR(255) NOT NULL,
    target_user_id VARCHAR(255) NOT NULL,
    target_session VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',  -- pending, approved, rejected, expired
    is_same_user BOOLEAN NOT NULL DEFAULT false,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT fk_resource FOREIGN KEY (resource_id) REFERENCES workspace_resources(id) ON DELETE CASCADE
);

-- 创建索引以优化查询性能
CREATE INDEX IF NOT EXISTS idx_takeover_target_session ON takeover_requests(target_session, status);
CREATE INDEX IF NOT EXISTS idx_takeover_requester_session ON takeover_requests(requester_session, status);
CREATE INDEX IF NOT EXISTS idx_takeover_resource_status ON takeover_requests(resource_id, status);
CREATE INDEX IF NOT EXISTS idx_takeover_expires_at ON takeover_requests(expires_at, status);

-- 添加注释
COMMENT ON TABLE takeover_requests IS '资源编辑接管请求表';
COMMENT ON COLUMN takeover_requests.resource_id IS '资源ID';
COMMENT ON COLUMN takeover_requests.requester_user_id IS '请求接管的用户ID';
COMMENT ON COLUMN takeover_requests.requester_name IS '请求者用户名';
COMMENT ON COLUMN takeover_requests.requester_session IS '请求者的session_id';
COMMENT ON COLUMN takeover_requests.target_user_id IS '被接管的用户ID';
COMMENT ON COLUMN takeover_requests.target_session IS '被接管的session_id';
COMMENT ON COLUMN takeover_requests.status IS '请求状态: pending-待处理, approved-已同意, rejected-已拒绝, expired-已过期';
COMMENT ON COLUMN takeover_requests.is_same_user IS '是否同一用户的多窗口接管';
COMMENT ON COLUMN takeover_requests.expires_at IS '请求过期时间（30秒后）';
