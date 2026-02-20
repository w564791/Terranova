-- 通知系统数据库迁移脚本
-- 创建时间: 2025-12-12
-- 功能: 实现通知系统，支持 Webhook 和 Lark Robot
-- 参考文档: docs/notification/README.md

-- ============================================================================
-- 1. 通知配置表 (notification_configs)
-- ============================================================================
CREATE TABLE IF NOT EXISTS notification_configs (
    id SERIAL PRIMARY KEY,
    notification_id VARCHAR(50) UNIQUE NOT NULL,  -- 语义化ID，如 "notif-lark-ops"
    name VARCHAR(100) NOT NULL,                    -- 名称，只能包含字母、数字、破折号和下划线
    description TEXT,                              -- 描述（可选）
    
    -- 通知类型
    notification_type VARCHAR(20) NOT NULL,        -- 类型: webhook, lark_robot
    
    -- Endpoint 配置
    endpoint_url VARCHAR(500) NOT NULL,            -- Endpoint URL
    
    -- 认证配置（根据类型不同使用不同字段）
    -- Webhook: 可选的 HMAC 密钥
    -- Lark Robot: 签名密钥（secret）
    secret_encrypted TEXT,                         -- 密钥（加密存储，可选）
    
    -- 自定义 Headers（JSON 格式）
    -- 默认包含 Content-Type: application/json
    custom_headers JSONB DEFAULT '{"Content-Type": "application/json"}',
    
    -- 状态
    enabled BOOLEAN DEFAULT true,                  -- 是否启用
    
    -- 全局配置
    is_global BOOLEAN DEFAULT false,               -- 是否为全局通知（自动应用于所有 Workspace）
    
    -- 全局通知默认触发事件（仅当 is_global=true 时有效）
    -- 逗号分隔，如 "task_completed,task_failed"
    global_events VARCHAR(500) DEFAULT 'task_completed,task_failed',
    
    -- 重试配置
    retry_count INTEGER DEFAULT 3,                 -- 重试次数
    retry_interval_seconds INTEGER DEFAULT 30,     -- 重试间隔（秒）
    
    -- 超时配置
    timeout_seconds INTEGER DEFAULT 30,            -- 请求超时（秒）
    
    -- 组织/团队归属
    organization_id VARCHAR(50),                   -- 组织ID（可选）
    team_id VARCHAR(50),                           -- 团队ID（可选）
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 约束
    CONSTRAINT notification_configs_name_check CHECK (name ~ '^[a-zA-Z0-9_-]+$'),
    CONSTRAINT notification_configs_type_check CHECK (notification_type IN ('webhook', 'lark_robot')),
    CONSTRAINT notification_configs_timeout_check CHECK (timeout_seconds >= 5 AND timeout_seconds <= 120),
    CONSTRAINT notification_configs_retry_check CHECK (retry_count >= 0 AND retry_count <= 10)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_notification_configs_name ON notification_configs(name);
CREATE INDEX IF NOT EXISTS idx_notification_configs_type ON notification_configs(notification_type);
CREATE INDEX IF NOT EXISTS idx_notification_configs_organization ON notification_configs(organization_id);
CREATE INDEX IF NOT EXISTS idx_notification_configs_team ON notification_configs(team_id);
CREATE INDEX IF NOT EXISTS idx_notification_configs_enabled ON notification_configs(enabled);
CREATE INDEX IF NOT EXISTS idx_notification_configs_is_global ON notification_configs(is_global) WHERE is_global = true;

COMMENT ON TABLE notification_configs IS '通知配置表，存储通知服务集成配置';
COMMENT ON COLUMN notification_configs.notification_id IS '语义化ID，如 notif-lark-ops';
COMMENT ON COLUMN notification_configs.notification_type IS '通知类型: webhook(普通Webhook), lark_robot(飞书机器人)';
COMMENT ON COLUMN notification_configs.secret_encrypted IS '密钥（AES-256加密存储），Webhook用于HMAC签名，Lark Robot用于签名验证';
COMMENT ON COLUMN notification_configs.custom_headers IS '自定义HTTP Headers，JSON格式';
COMMENT ON COLUMN notification_configs.is_global IS '是否为全局通知，自动应用于所有 Workspace';
COMMENT ON COLUMN notification_configs.global_events IS '全局通知默认触发事件，逗号分隔';

-- ============================================================================
-- 2. Workspace 通知关联表 (workspace_notifications)
-- ============================================================================
CREATE TABLE IF NOT EXISTS workspace_notifications (
    id SERIAL PRIMARY KEY,
    workspace_notification_id VARCHAR(50) UNIQUE NOT NULL,  -- 语义化ID
    workspace_id VARCHAR(50) NOT NULL,                       -- 关联的 Workspace ID
    notification_id VARCHAR(50) NOT NULL,                    -- 关联的 Notification ID
    
    -- 触发事件配置（逗号分隔）
    -- 如 "task_completed,task_failed,approval_required"
    events VARCHAR(500) NOT NULL DEFAULT 'task_completed,task_failed',
    
    -- 状态
    enabled BOOLEAN DEFAULT true,
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 外键约束
    CONSTRAINT fk_workspace_notifications_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    CONSTRAINT fk_workspace_notifications_notification FOREIGN KEY (notification_id) 
        REFERENCES notification_configs(notification_id) ON DELETE CASCADE,
    
    -- 唯一约束：同一个 workspace 的同一个 notification 只能配置一次
    UNIQUE(workspace_id, notification_id)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_workspace_notifications_workspace ON workspace_notifications(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_notifications_notification ON workspace_notifications(notification_id);
CREATE INDEX IF NOT EXISTS idx_workspace_notifications_enabled ON workspace_notifications(enabled);

COMMENT ON TABLE workspace_notifications IS 'Workspace 通知关联表，配置 Workspace 使用的通知';
COMMENT ON COLUMN workspace_notifications.events IS '触发事件，逗号分隔，如 task_completed,task_failed';

-- ============================================================================
-- 3. 通知发送记录表 (notification_logs)
-- ============================================================================
CREATE TABLE IF NOT EXISTS notification_logs (
    id SERIAL PRIMARY KEY,
    log_id VARCHAR(50) UNIQUE NOT NULL,            -- 语义化ID
    
    -- 关联
    task_id BIGINT,                                -- 关联的 workspace_task ID（可选）
    workspace_id VARCHAR(50),                      -- 关联的 Workspace ID
    notification_id VARCHAR(50) NOT NULL,          -- 关联的 Notification ID
    workspace_notification_id VARCHAR(50),         -- 关联的 Workspace Notification ID（可选，全局通知时为空）
    
    -- 事件信息
    event VARCHAR(50) NOT NULL,                    -- 触发事件
    
    -- 发送状态
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- 状态: pending, sending, success, failed
    
    -- 请求/响应
    request_payload JSONB,                         -- 发送的请求体
    request_headers JSONB,                         -- 发送的请求头（脱敏后）
    response_status_code INTEGER,                  -- 响应状态码
    response_body TEXT,                            -- 响应体（截断保存）
    error_message TEXT,                            -- 错误信息
    
    -- 重试信息
    retry_count INTEGER DEFAULT 0,                 -- 已重试次数
    max_retry_count INTEGER DEFAULT 3,             -- 最大重试次数
    next_retry_at TIMESTAMP,                       -- 下次重试时间
    
    -- 时间
    sent_at TIMESTAMP,                             -- 发送时间
    completed_at TIMESTAMP,                        -- 完成时间
    
    -- 元数据
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 外键约束
    CONSTRAINT fk_notification_logs_notification FOREIGN KEY (notification_id) 
        REFERENCES notification_configs(notification_id) ON DELETE CASCADE,
    
    -- 约束
    CONSTRAINT notification_logs_status_check CHECK (status IN ('pending', 'sending', 'success', 'failed'))
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_notification_logs_task ON notification_logs(task_id);
CREATE INDEX IF NOT EXISTS idx_notification_logs_workspace ON notification_logs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_notification_logs_notification ON notification_logs(notification_id);
CREATE INDEX IF NOT EXISTS idx_notification_logs_event ON notification_logs(event);
CREATE INDEX IF NOT EXISTS idx_notification_logs_status ON notification_logs(status);
CREATE INDEX IF NOT EXISTS idx_notification_logs_created_at ON notification_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_notification_logs_next_retry ON notification_logs(next_retry_at) WHERE status = 'failed' AND next_retry_at IS NOT NULL;

COMMENT ON TABLE notification_logs IS '通知发送记录表，存储每次通知发送的结果';
COMMENT ON COLUMN notification_logs.status IS '状态: pending(等待), sending(发送中), success(成功), failed(失败)';
COMMENT ON COLUMN notification_logs.retry_count IS '已重试次数';
COMMENT ON COLUMN notification_logs.next_retry_at IS '下次重试时间，用于重试调度';

-- ============================================================================
-- 完成
-- ============================================================================
SELECT 'Notification tables created successfully!' AS status;
