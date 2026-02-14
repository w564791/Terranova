-- Run Task 功能数据库迁移脚本
-- 创建时间: 2025-01-06
-- 功能: 实现类似 Terraform Enterprise 的 Run Task 功能
-- 参考: https://developer.hashicorp.com/terraform/cloud-docs/integrations/run-tasks

-- ============================================================================
-- 1. Run Task 全局定义表
-- ============================================================================
CREATE TABLE IF NOT EXISTS run_tasks (
    id SERIAL PRIMARY KEY,
    run_task_id VARCHAR(50) UNIQUE NOT NULL,  -- 语义化ID，如 "rt-security-scan"
    name VARCHAR(100) NOT NULL,                -- 名称，只能包含字母、数字、破折号和下划线
    description TEXT,                          -- 描述（可选）
    endpoint_url VARCHAR(500) NOT NULL,        -- Endpoint URL，Run Tasks 会 POST 到这个 URL
    hmac_key_encrypted TEXT,                   -- HMAC密钥（加密存储，可选）
    enabled BOOLEAN DEFAULT true,              -- 是否启用
    
    -- 超时配置（符合 TFE 规范）
    -- TFE 规则：10分钟内必须收到进度更新，总运行时间不超过60分钟
    timeout_seconds INTEGER DEFAULT 600,       -- 进度更新超时（秒），默认10分钟
    max_run_seconds INTEGER DEFAULT 3600,      -- 最大运行时间（秒），默认60分钟
    
    -- 全局任务配置
    is_global BOOLEAN DEFAULT false,           -- 是否为全局任务（自动应用于所有 Workspace）
    
    -- 组织/团队归属
    organization_id VARCHAR(50),               -- 组织ID（可选）
    team_id VARCHAR(50),                       -- 团队ID（可选）
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 约束
    CONSTRAINT run_tasks_name_check CHECK (name ~ '^[a-zA-Z0-9_-]+$'),
    CONSTRAINT run_tasks_timeout_check CHECK (timeout_seconds >= 60 AND timeout_seconds <= 600),
    CONSTRAINT run_tasks_max_run_check CHECK (max_run_seconds >= 60 AND max_run_seconds <= 3600)
);

-- Run Task 索引
CREATE INDEX IF NOT EXISTS idx_run_tasks_name ON run_tasks(name);
CREATE INDEX IF NOT EXISTS idx_run_tasks_organization ON run_tasks(organization_id);
CREATE INDEX IF NOT EXISTS idx_run_tasks_team ON run_tasks(team_id);
CREATE INDEX IF NOT EXISTS idx_run_tasks_enabled ON run_tasks(enabled);
CREATE INDEX IF NOT EXISTS idx_run_tasks_is_global ON run_tasks(is_global) WHERE is_global = true;

COMMENT ON TABLE run_tasks IS 'Run Task 全局定义表，存储外部服务集成配置';
COMMENT ON COLUMN run_tasks.run_task_id IS '语义化ID，如 rt-security-scan';
COMMENT ON COLUMN run_tasks.name IS '名称，只能包含字母、数字、破折号和下划线';
COMMENT ON COLUMN run_tasks.endpoint_url IS 'Endpoint URL，Run Tasks 会 POST 到这个 URL';
COMMENT ON COLUMN run_tasks.hmac_key_encrypted IS 'HMAC密钥（AES-256加密存储）';
COMMENT ON COLUMN run_tasks.timeout_seconds IS '进度更新超时（秒），默认600秒（10分钟），范围60-600';
COMMENT ON COLUMN run_tasks.max_run_seconds IS '最大运行时间（秒），默认3600秒（60分钟），范围60-3600';
COMMENT ON COLUMN run_tasks.is_global IS '是否为全局任务，自动应用于所有 Workspace';

-- ============================================================================
-- 2. Workspace Run Task 关联表
-- ============================================================================
CREATE TABLE IF NOT EXISTS workspace_run_tasks (
    id SERIAL PRIMARY KEY,
    workspace_run_task_id VARCHAR(50) UNIQUE NOT NULL,  -- 语义化ID
    workspace_id VARCHAR(50) NOT NULL,                   -- 关联的 Workspace ID
    run_task_id VARCHAR(50) NOT NULL,                    -- 关联的 Run Task ID
    
    -- 执行配置
    stage VARCHAR(20) NOT NULL,                          -- 执行阶段: pre_plan, post_plan, pre_apply, post_apply
    enforcement_level VARCHAR(20) NOT NULL DEFAULT 'advisory',  -- 执行级别: advisory, mandatory
    
    -- 状态
    enabled BOOLEAN DEFAULT true,
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 外键约束
    CONSTRAINT fk_workspace_run_tasks_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    CONSTRAINT fk_workspace_run_tasks_run_task FOREIGN KEY (run_task_id) 
        REFERENCES run_tasks(run_task_id) ON DELETE CASCADE,
    
    -- 约束
    CONSTRAINT workspace_run_tasks_stage_check CHECK (stage IN ('pre_plan', 'post_plan', 'pre_apply', 'post_apply')),
    CONSTRAINT workspace_run_tasks_enforcement_check CHECK (enforcement_level IN ('advisory', 'mandatory')),
    
    -- 唯一约束：同一个workspace的同一个run_task在同一阶段只能配置一次
    UNIQUE(workspace_id, run_task_id, stage)
);

-- Workspace Run Task 索引
CREATE INDEX IF NOT EXISTS idx_workspace_run_tasks_workspace ON workspace_run_tasks(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_run_tasks_run_task ON workspace_run_tasks(run_task_id);
CREATE INDEX IF NOT EXISTS idx_workspace_run_tasks_stage ON workspace_run_tasks(stage);
CREATE INDEX IF NOT EXISTS idx_workspace_run_tasks_enabled ON workspace_run_tasks(enabled);

COMMENT ON TABLE workspace_run_tasks IS 'Workspace Run Task 关联表，配置 Workspace 使用的 Run Task';
COMMENT ON COLUMN workspace_run_tasks.stage IS '执行阶段: pre_plan(Plan前), post_plan(Plan后), pre_apply(Apply前), post_apply(Apply后)';
COMMENT ON COLUMN workspace_run_tasks.enforcement_level IS '执行级别: advisory(建议性，失败产生警告), mandatory(强制性，失败停止执行)';

-- ============================================================================
-- 3. Run Task 执行记录表
-- ============================================================================
CREATE TABLE IF NOT EXISTS run_task_results (
    id SERIAL PRIMARY KEY,
    result_id VARCHAR(50) UNIQUE NOT NULL,     -- 语义化ID
    
    -- 关联
    task_id BIGINT NOT NULL,                   -- 关联的 workspace_task ID
    workspace_run_task_id VARCHAR(50) NOT NULL, -- 关联的 workspace_run_task ID
    
    -- 执行信息
    stage VARCHAR(20) NOT NULL,                -- 执行阶段
    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- 状态: pending, running, passed, failed, error, timeout, skipped
    
    -- 一次性 Access Token（用于 Run Task 平台获取数据和回调）
    access_token VARCHAR(500),                 -- 一次性验证令牌（JWT格式）
    access_token_expires_at TIMESTAMP,         -- Token过期时间
    access_token_used BOOLEAN DEFAULT false,   -- Token是否已使用（回调后标记为已使用）
    
    -- 请求/响应
    request_payload JSONB,                     -- 发送给外部服务的请求
    response_payload JSONB,                    -- 外部服务的响应（回调数据）
    callback_url VARCHAR(500),                 -- 回调URL（用于异步结果）
    
    -- 结果详情
    message TEXT,                              -- 结果消息
    url VARCHAR(500),                          -- 详情链接（外部服务提供）
    
    -- 超时配置（符合 TFE 规范）
    timeout_seconds INTEGER DEFAULT 600,       -- 进度更新超时（秒）
    max_run_seconds INTEGER DEFAULT 3600,      -- 最大运行时间（秒）
    last_heartbeat_at TIMESTAMP,               -- 最后一次进度更新时间
    timeout_at TIMESTAMP,                      -- 进度更新超时时间点
    max_run_timeout_at TIMESTAMP,              -- 最大运行超时时间点
    
    -- 时间
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    
    -- 元数据
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 外键约束
    CONSTRAINT fk_run_task_results_task FOREIGN KEY (task_id) 
        REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    CONSTRAINT fk_run_task_results_workspace_run_task FOREIGN KEY (workspace_run_task_id) 
        REFERENCES workspace_run_tasks(workspace_run_task_id) ON DELETE CASCADE,
    
    -- 约束
    CONSTRAINT run_task_results_status_check CHECK (status IN ('pending', 'running', 'passed', 'failed', 'error', 'timeout', 'skipped'))
);

-- Run Task Results 索引
CREATE INDEX IF NOT EXISTS idx_run_task_results_task ON run_task_results(task_id);
CREATE INDEX IF NOT EXISTS idx_run_task_results_workspace_run_task ON run_task_results(workspace_run_task_id);
CREATE INDEX IF NOT EXISTS idx_run_task_results_status ON run_task_results(status);
CREATE INDEX IF NOT EXISTS idx_run_task_results_stage ON run_task_results(stage);
CREATE INDEX IF NOT EXISTS idx_run_task_results_created_at ON run_task_results(created_at);
CREATE INDEX IF NOT EXISTS idx_run_task_results_timeout_at ON run_task_results(timeout_at) WHERE status = 'running';
CREATE INDEX IF NOT EXISTS idx_run_task_results_max_run_timeout_at ON run_task_results(max_run_timeout_at) WHERE status = 'running';

COMMENT ON TABLE run_task_results IS 'Run Task 执行记录表，存储每次 Run Task 调用的结果';
COMMENT ON COLUMN run_task_results.status IS '状态: pending(等待), running(执行中), passed(通过), failed(失败), error(错误), timeout(超时), skipped(跳过)';
COMMENT ON COLUMN run_task_results.access_token IS '一次性验证令牌，Run Task 平台用于获取数据和回调';
COMMENT ON COLUMN run_task_results.access_token_used IS 'Token是否已使用，回调后标记为true，防止重复回调';
COMMENT ON COLUMN run_task_results.timeout_seconds IS '进度更新超时（秒），10分钟内必须收到进度更新';
COMMENT ON COLUMN run_task_results.max_run_seconds IS '最大运行时间（秒），总运行时间不超过60分钟';
COMMENT ON COLUMN run_task_results.last_heartbeat_at IS '最后一次进度更新时间，用于检测进度更新超时';
COMMENT ON COLUMN run_task_results.timeout_at IS '进度更新超时时间点';
COMMENT ON COLUMN run_task_results.max_run_timeout_at IS '最大运行超时时间点';

-- ============================================================================
-- 4. Run Task Outcomes 表（符合 TFE 规范）
-- ============================================================================
CREATE TABLE IF NOT EXISTS run_task_outcomes (
    id SERIAL PRIMARY KEY,
    
    -- 关联
    run_task_result_id VARCHAR(50) NOT NULL,   -- 关联的 run_task_result ID
    
    -- Outcome 标识（第三方服务提供）
    outcome_id VARCHAR(100) NOT NULL,          -- 第三方服务提供的唯一标识，如 "PRTNR-CC-TF-127"
    
    -- 描述
    description VARCHAR(500) NOT NULL,         -- 一行描述
    body TEXT,                                 -- Markdown 格式的详细内容（建议 < 1MB，最大 5MB）
    url VARCHAR(500),                          -- 详情链接
    
    -- 标签（JSON 格式，支持 severity 和 status 特殊处理）
    -- 格式: {"Status": [{"label": "Failed", "level": "error"}], "Severity": [...]}
    -- level 可选值: none(默认), info(蓝色), warning(黄色), error(红色)
    tags JSONB,
    
    -- 元数据
    created_at TIMESTAMP DEFAULT NOW(),
    
    -- 外键约束
    CONSTRAINT fk_run_task_outcomes_result FOREIGN KEY (run_task_result_id) 
        REFERENCES run_task_results(result_id) ON DELETE CASCADE
);

-- Run Task Outcomes 索引
CREATE INDEX IF NOT EXISTS idx_run_task_outcomes_result ON run_task_outcomes(run_task_result_id);
CREATE INDEX IF NOT EXISTS idx_run_task_outcomes_outcome_id ON run_task_outcomes(outcome_id);

COMMENT ON TABLE run_task_outcomes IS 'Run Task Outcomes 表，存储详细的检查结果（符合 TFE 规范）';
COMMENT ON COLUMN run_task_outcomes.outcome_id IS '第三方服务提供的唯一标识，如 PRTNR-CC-TF-127';
COMMENT ON COLUMN run_task_outcomes.description IS '一行描述';
COMMENT ON COLUMN run_task_outcomes.body IS 'Markdown 格式的详细内容（建议 < 1MB，最大 5MB）';
COMMENT ON COLUMN run_task_outcomes.tags IS '标签对象，severity 和 status 有特殊处理。格式: {"Status": [{"label": "Failed", "level": "error"}]}';

-- ============================================================================
-- 完成
-- ============================================================================
SELECT 'Run Task tables created successfully!' AS status;
