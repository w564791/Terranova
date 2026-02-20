# Agent-Workspace 双向授权系统 - 最终方案

## 1. 核心概念

### 1.1 角色定义
- **Application**: 应用凭证,包含 AppKey/AppSecret,用于 Agent 注册认证
- **Agent**: Agent 运行实例,每次启动注册获得唯一 agent_id
- **Workspace**: 工作空间,包含任务和资源
- **双向授权**: Agent 和 Workspace 必须互相允许才能建立访问关系

### 1.2 设计原则
- **简单**: 只实现核心的注册和双向授权功能
- **无状态**: Agent 重启即新实例,agent_id 保存在内存
- **双向控制**: Agent 控制可访问的 workspace,workspace 控制可用的 agent
- **唯一激活**: 每个 workspace 同时只能激活一个 agent
- **agent ID**:  agent id 类型为string,agent-{16位随机数a-z0-9}
- **token ID**:  agent token id 类型为string,token-a-{16位随机数a-z0-9}
- **pool ID**:  agent pool id 类型为string,pool-{16位随机数a-z0-9}
## 2. 数据库设计

### 2.0 数据库优化前置工作

**重要**: 在实施本方案前，需要先执行数据库表结构优化，确保符合项目语义化 ID 规范。

#### 2.0.1 现有问题
1. **agent_pools 表**: 使用 `id INTEGER` 而非语义化的 `pool_id VARCHAR(50)`
2. **agents 表**: 使用 `id INTEGER` 而非语义化的 `agent_id VARCHAR(50)`
3. **审计字段**: `created_by` 和 `updated_by` 需要统一为 `VARCHAR(50)` 类型

#### 2.0.2 迁移脚本
执行 `scripts/migrate_agents_and_pools_to_semantic_id.sql` 完成以下优化：

**agent_pools 表变更**:
- 主键: `id INTEGER` → `pool_id VARCHAR(50)` (格式: `pool-{16位随机a-z0-9}`)
- 外键更新: `agents.pool_id` 和 `workspaces.agent_pool_id` 改为 VARCHAR(50)
- 新增字段: `organization_id`, `is_shared`, `max_agents`, `status`
- 审计字段: 确保 `created_by` 和 `updated_by` 为 VARCHAR(50)

**agents 表变更**:
- 主键: `id INTEGER` → `agent_id VARCHAR(50)` (格式: `agent-{16位随机a-z0-9}`)
- 字段调整: `last_heartbeat` → `last_ping_at`
- 新增字段: `application_id`, `ip_address`, `version`, `registered_at`
- 审计字段: 新增 `created_by` 和 `updated_by` 为 VARCHAR(50)

**迁移安全性**:
-  agent_pools 现有 1 条记录会被保留并转换为 `pool-default00000001`
-  agents 表无现有数据，可安全迁移
-  使用条件检查确保脚本幂等性

### 2.1 Applications 表 (现有,需修复)
```sql
-- 修复 callback_urls 字段类型
ALTER TABLE applications 
ALTER COLUMN callback_urls TYPE JSONB USING callback_urls::JSONB;

-- 如果字段不存在则添加
ALTER TABLE applications 
ADD COLUMN IF NOT EXISTS callback_urls JSONB DEFAULT '[]'::JSONB;
```

### 2.2 Agent Pools 表 (已存在，需优化)
```sql
-- 优化后的 agent_pools 表结构
CREATE TABLE agent_pools (
    pool_id VARCHAR(50) PRIMARY KEY,   -- 格式: pool-{16位随机小写字母+数字}
    name VARCHAR(100) NOT NULL,
    description TEXT,
    pool_type VARCHAR(20) NOT NULL,    -- static, k8s, etc.
    k8s_config JSONB,
    
    -- 管理字段
    organization_id VARCHAR(50) REFERENCES organizations(org_id) ON DELETE CASCADE,
    is_shared BOOLEAN DEFAULT false,
    max_agents INTEGER DEFAULT 10,
    status VARCHAR(20) DEFAULT 'active',
    
    -- 审计字段
    created_by VARCHAR(50),
    updated_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_agent_pools_organization ON agent_pools(organization_id);
CREATE INDEX idx_agent_pools_status ON agent_pools(status);
CREATE INDEX idx_agent_pools_pool_type ON agent_pools(pool_type);
```

### 2.3 Agents 表 (已存在，需优化)
```sql
-- 优化后的 agents 表结构
CREATE TABLE agents (
    agent_id VARCHAR(50) PRIMARY KEY,  -- 格式: agent-{16位随机小写字母+数字}
    application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    pool_id VARCHAR(50) REFERENCES agent_pools(pool_id) ON DELETE SET NULL,
    
    -- Agent 信息
    name VARCHAR(100),                 -- Agent 名称(可重复,用于标识)
    ip_address VARCHAR(50),            -- IP 地址
    version VARCHAR(50),               -- Agent 版本
    token_hash VARCHAR(255) NOT NULL,  -- 认证 token 哈希
    
    -- 状态
    status VARCHAR(20) NOT NULL DEFAULT 'idle', -- idle, busy, offline
    last_ping_at TIMESTAMP,            -- 最后心跳时间
    
    -- 扩展字段
    capabilities JSONB,                -- Agent 能力
    metadata JSONB,                    -- 元数据
    
    -- 审计字段
    registered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(50),
    updated_by VARCHAR(50),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_agents_pool ON agents(pool_id);
CREATE INDEX idx_agents_application ON agents(application_id);
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_agents_last_ping ON agents(last_ping_at);
```

### 2.4 Agent 允许的 Workspace (Agent 侧)
```sql
CREATE TABLE agent_allowed_workspaces (
    id SERIAL PRIMARY KEY,
    agent_id VARCHAR(50) NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    
    -- 状态
    status VARCHAR(20) NOT NULL DEFAULT 'active', -- active, revoked
    
    -- 审计
    allowed_by VARCHAR(50),            -- 谁允许的(可选)
    allowed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_by VARCHAR(50),
    revoked_at TIMESTAMP,
    
    -- 时间戳
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(agent_id, workspace_id)
);

CREATE INDEX idx_agent_allowed_ws_agent ON agent_allowed_workspaces(agent_id);
CREATE INDEX idx_agent_allowed_ws_workspace ON agent_allowed_workspaces(workspace_id);
CREATE INDEX idx_agent_allowed_ws_status ON agent_allowed_workspaces(status);
```

### 2.5 Workspace 允许的 Agent (Workspace 侧)
```sql
CREATE TABLE workspace_allowed_agents (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id VARCHAR(50) NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    
    -- 状态
    status VARCHAR(20) NOT NULL DEFAULT 'active', -- active, revoked
    is_current BOOLEAN DEFAULT false,  -- 是否为当前使用的 agent
    
    -- 审计
    allowed_by VARCHAR(50) NOT NULL,   -- 谁允许的
    allowed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    activated_by VARCHAR(50),          -- 谁激活的
    activated_at TIMESTAMP,
    revoked_by VARCHAR(50),
    revoked_at TIMESTAMP,
    
    -- 时间戳
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(workspace_id, agent_id)
);

-- 约束: 每个 workspace 只能有一个 is_current=true 的 agent
CREATE UNIQUE INDEX idx_workspace_one_current_agent 
ON workspace_allowed_agents(workspace_id) 
WHERE is_current = true AND status = 'active';

CREATE INDEX idx_workspace_allowed_agents_workspace ON workspace_allowed_agents(workspace_id);
CREATE INDEX idx_workspace_allowed_agents_agent ON workspace_allowed_agents(agent_id);
CREATE INDEX idx_workspace_allowed_agents_status ON workspace_allowed_agents(status);
CREATE INDEX idx_workspace_allowed_agents_current ON workspace_allowed_agents(is_current);
```

### 2.6 访问日志 (可选,用于审计)
```sql
CREATE TABLE agent_access_logs (
    id SERIAL PRIMARY KEY,
    agent_id VARCHAR(50) NOT NULL,
    workspace_id INTEGER NOT NULL,
    
    -- 访问信息
    action VARCHAR(50) NOT NULL,       -- 操作类型: task.run, task.query 等
    task_id VARCHAR(100),              -- 关联的任务 ID
    request_ip VARCHAR(50),            -- 请求 IP
    request_path TEXT,                 -- 请求路径
    
    -- 结果
    success BOOLEAN NOT NULL DEFAULT true,
    error_message TEXT,
    response_time_ms INTEGER,
    
    -- 时间戳
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_agent_access_logs_agent ON agent_access_logs(agent_id);
CREATE INDEX idx_agent_access_logs_workspace ON agent_access_logs(workspace_id);
CREATE INDEX idx_agent_access_logs_created_at ON agent_access_logs(created_at);
CREATE INDEX idx_agent_access_logs_success ON agent_access_logs(success);
```

## 3. API 设计

### 3.1 Agent 注册与管理

#### 注册 Agent
```
POST /api/v1/agents/register

Headers:
  X-App-Key: {app_key}
  X-App-Secret: {app_secret}

Body:
{
  "name": "idc-hk-ap1",      // 可选,Agent 名称
  "version": "1.0.0"         // 可选,Agent 版本
}

Response: 200 OK
{
  "agent_id": "agent-a1b2c3d4e5f6g7h8",
  "status": "idle",
  "registered_at": "2025-10-27T08:00:00Z"
}

Error: 401 Unauthorized
{
  "error": "invalid application credentials"
}
```

#### Agent 心跳
```
POST /api/v1/agents/{agent_id}/ping

Headers:
  X-App-Key: {app_key}
  X-App-Secret: {app_secret}

Body:
{
  "status": "idle"  // idle 或 busy
}

Response: 200 OK
{
  "message": "ping received",
  "last_ping_at": "2025-10-27T08:00:30Z"
}
```

#### 获取 Agent 信息
```
GET /api/v1/agents/{agent_id}

Headers:
  X-App-Key: {app_key}
  X-App-Secret: {app_secret}

Response: 200 OK
{
  "agent_id": "agent-a1b2c3d4e5f6g7h8",
  "name": "idc-hk-ap1",
  "status": "idle",
  "ip_address": "10.130.72.100",
  "version": "1.0.0",
  "last_ping_at": "2025-10-27T08:00:30Z",
  "registered_at": "2025-10-27T08:00:00Z"
}
```

#### 注销 Agent
```
DELETE /api/v1/agents/{agent_id}

Headers:
  X-App-Key: {app_key}
  X-App-Secret: {app_secret}

Response: 200 OK
{
  "message": "agent unregistered successfully"
}
```

### 3.2 Agent 允许 Workspace (Agent 侧)

#### 批量允许 Workspace
```
POST /api/v1/agents/{agent_id}/allow-workspaces

Headers:
  X-App-Key: {app_key}
  X-App-Secret: {app_secret}

Body:
{
  "workspace_ids": [1, 2, 3]
}

Response: 200 OK
{
  "message": "workspaces allowed successfully",
  "allowed_count": 3
}
```

#### 查看允许的 Workspace 列表
```
GET /api/v1/agents/{agent_id}/allowed-workspaces

Headers:
  X-App-Key: {app_key}
  X-App-Secret: {app_secret}

Response: 200 OK
{
  "workspaces": [
    {
      "workspace_id": 1,
      "workspace_name": "Dev Team",
      "status": "active",
      "allowed_at": "2025-10-27T08:00:00Z"
    },
    {
      "workspace_id": 2,
      "workspace_name": "Prod Team",
      "status": "active",
      "allowed_at": "2025-10-27T08:01:00Z"
    }
  ],
  "total": 2
}
```

#### 撤销对 Workspace 的允许
```
DELETE /api/v1/agents/{agent_id}/allowed-workspaces/{workspace_id}

Headers:
  X-App-Key: {app_key}
  X-App-Secret: {app_secret}

Response: 200 OK
{
  "message": "workspace access revoked successfully"
}
```

### 3.3 Workspace 允许 Agent (Workspace 侧)

#### 查看可用的 Agent 列表
```
GET /api/v1/workspaces/{workspace_id}/available-agents

Response: 200 OK
{
  "agents": [
    {
      "agent_id": "agent-a1b2c3d4e5f6g7h8",
      "name": "idc-hk-ap1",
      "status": "idle",
      "ip_address": "10.130.72.100",
      "version": "1.0.0",
      "last_ping_at": "2025-10-27T08:00:30Z",
      "is_allowed": true,      // workspace 是否已允许
      "is_current": false      // 是否为当前使用的 agent
    },
    {
      "agent_id": "agent-b2c3d4e5f6g7h8i9",
      "name": "idc-hk-ap2",
      "status": "busy",
      "ip_address": "10.130.72.101",
      "version": "1.0.0",
      "last_ping_at": "2025-10-27T08:00:35Z",
      "is_allowed": false,
      "is_current": false
    }
  ],
  "total": 2
}
```

#### 允许 Agent 访问
```
POST /api/v1/workspaces/{workspace_id}/allow-agent

Body:
{
  "agent_id": "agent-a1b2c3d4e5f6g7h8"
}

Response: 200 OK
{
  "message": "agent allowed successfully"
}

Error: 400 Bad Request
{
  "error": "agent has not allowed this workspace"
}
```

#### 设置当前使用的 Agent
```
POST /api/v1/workspaces/{workspace_id}/set-current-agent

Body:
{
  "agent_id": "agent-a1b2c3d4e5f6g7h8"
}

Response: 200 OK
{
  "message": "current agent set successfully",
  "previous_agent_id": "agent-xxx",  // 之前的 agent (如果有)
  "current_agent_id": "agent-a1b2c3d4e5f6g7h8"
}

Error: 400 Bad Request
{
  "error": "workspace has running tasks, cannot switch agent"
}

Error: 403 Forbidden
{
  "error": "agent not allowed by workspace"
}
```

#### 获取当前 Agent
```
GET /api/v1/workspaces/{workspace_id}/current-agent

Response: 200 OK
{
  "agent_id": "agent-a1b2c3d4e5f6g7h8",
  "name": "idc-hk-ap1",
  "status": "idle",
  "ip_address": "10.130.72.100",
  "last_ping_at": "2025-10-27T08:00:30Z"
}

Response: 404 Not Found
{
  "error": "no current agent configured"
}
```

#### 撤销 Agent 访问
```
DELETE /api/v1/workspaces/{workspace_id}/allowed-agents/{agent_id}

Response: 200 OK
{
  "message": "agent access revoked successfully"
}
```

### 3.4 访问验证

#### 验证 Agent 访问权限
```
GET /api/v1/validate-agent-access?agent_id={agent_id}&workspace_id={workspace_id}

Response: 200 OK
{
  "allowed": true,
  "is_current": true,
  "agent_status": "idle",
  "last_ping_at": "2025-10-27T08:00:30Z"
}

Response: 403 Forbidden
{
  "allowed": false,
  "reason": "agent not allowed by workspace"
}
```

## 4. 核心流程

### 4.1 Agent 启动流程
```
1. Agent 进程启动
2. 读取配置 (AppKey/AppSecret, name, version)
3. 调用 POST /api/v1/agents/register
4. 获得 agent_id,保存到内存
5. 启动心跳循环 (每 30 秒调用 /ping)
6. 启动任务执行循环
```

### 4.2 Agent 允许 Workspace 流程
```
1. Agent 调用 POST /api/v1/agents/{agent_id}/allow-workspaces
2. 平台验证 AppKey/AppSecret
3. 平台创建 agent_allowed_workspaces 记录
4. 返回成功
```

### 4.3 Workspace 配置 Agent 流程
```
1. Workspace 管理员查看可用 agent 列表
   GET /api/v1/workspaces/{workspace_id}/available-agents
   
2. 选择一个 agent,允许访问
   POST /api/v1/workspaces/{workspace_id}/allow-agent
   
3. 设置为当前使用的 agent
   POST /api/v1/workspaces/{workspace_id}/set-current-agent
   
4. 平台检查:
   - Agent 是否 allowed 该 workspace
   - Workspace 是否有 running 任务
   - 如果有其他 current agent,先设为 inactive
   
5. 更新 workspace_allowed_agents 表
6. 返回成功
```

### 4.4 Agent 访问 Workspace 资源流程
```
1. Agent 发起请求 (如执行任务)
   POST /api/v1/workspaces/{workspace_id}/tasks/{task_id}/run
   Headers: X-App-Key, X-App-Secret, X-Agent-ID
   
2. 中间件验证:
   - 验证 AppKey/AppSecret
   - 验证 agent_id 存在且 active
   - 查询双向允许关系:
     * agent_allowed_workspaces: agent_id + workspace_id + status=active
     * workspace_allowed_agents: workspace_id + agent_id + status=active + is_current=true
   - 检查 agent 心跳 (last_ping_at < 5分钟)
   
3. 验证通过,执行任务
4. 记录访问日志 (可选)
```

### 4.5 撤销流程

#### Agent 撤销允许
```
1. Agent 调用 DELETE /api/v1/agents/{agent_id}/allowed-workspaces/{workspace_id}
2. 更新 agent_allowed_workspaces.status = 'revoked'
3. 如果 workspace 正在使用该 agent:
   - 更新 workspace_allowed_agents.is_current = false
   - 可选: 通知 workspace 管理员
4. 返回成功
```

#### Workspace 撤销允许
```
1. Workspace 管理员调用 DELETE /api/v1/workspaces/{workspace_id}/allowed-agents/{agent_id}
2. 检查是否有 running 任务
3. 更新 workspace_allowed_agents.status = 'revoked'
4. 如果是 current agent,设置 is_current = false
5. 返回成功
```

## 5. 验证逻辑

### 5.1 双向验证函数
```go
func ValidateAgentAccess(agentID string, workspaceID uint) (bool, error) {
    // 1. 检查 agent 是否存在且在线
    var agent Agent
    err := db.Where("agent_id = ? AND status != ?", agentID, "offline").
        Where("last_ping_at > ?", time.Now().Add(-5*time.Minute)).
        First(&agent).Error
    if err != nil {
        return false, errors.New("agent not found or offline")
    }
    
    // 2. 检查 agent 是否 allowed workspace
    var agentAllow AgentAllowedWorkspace
    err = db.Where("agent_id = ? AND workspace_id = ? AND status = ?", 
        agentID, workspaceID, "active").First(&agentAllow).Error
    if err != nil {
        return false, errors.New("agent has not allowed this workspace")
    }
    
    // 3. 检查 workspace 是否 allowed agent 且为 current
    var workspaceAllow WorkspaceAllowedAgent
    err = db.Where("workspace_id = ? AND agent_id = ? AND status = ? AND is_current = ?", 
        workspaceID, agentID, "active", true).First(&workspaceAllow).Error
    if err != nil {
        return false, errors.New("workspace has not set this agent as current")
    }
    
    return true, nil
}
```

### 5.2 中间件
```go
func AgentAuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. 验证 AppKey/AppSecret
        appKey := c.GetHeader("X-App-Key")
        appSecret := c.GetHeader("X-App-Secret")
        app, err := validateApplication(appKey, appSecret)
        if err != nil {
            c.JSON(401, gin.H{"error": "invalid credentials"})
            c.Abort()
            return
        }
        
        // 2. 获取 agent_id
        agentID := c.GetHeader("X-Agent-ID")
        if agentID == "" {
            agentID = c.Param("agent_id")
        }
        
        // 3. 验证 agent 属于该 application
        var agent Agent
        err = db.Where("agent_id = ? AND application_id = ?", 
            agentID, app.ID).First(&agent).Error
        if err != nil {
            c.JSON(403, gin.H{"error": "agent not found"})
            c.Abort()
            return
        }
        
        c.Set("agent_id", agentID)
        c.Set("agent", agent)
        c.Set("application", app)
        c.Next()
    }
}

func AgentWorkspaceAuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        agentID := c.GetString("agent_id")
        workspaceID := c.GetUint("workspace_id")
        
        allowed, err := ValidateAgentAccess(agentID, workspaceID)
        if err != nil || !allowed {
            c.JSON(403, gin.H{"error": "access denied", "reason": err.Error()})
            c.Abort()
            return
        }
        
        c.Next()
    }
}
```

## 6. 定时任务

### 6.1 清理离线 Agent
```go
// 每 5 分钟执行一次
func CleanupOfflineAgents() {
    // 标记 5 分钟没有心跳的为 offline
    db.Model(&Agent{}).
        Where("last_ping_at < ?", time.Now().Add(-5*time.Minute)).
        Where("status != ?", "offline").
        Update("status", "offline")
    
    // 删除 24 小时没有心跳的
    db.Where("last_ping_at < ?", time.Now().Add(-24*time.Hour)).
        Delete(&Agent{})
}
```

### 6.2 清理孤立的授权记录
```go
// 每天执行一次
func CleanupOrphanedAllowances() {
    // 清理 agent 已删除的授权记录
    db.Exec(`
        DELETE FROM agent_allowed_workspaces 
        WHERE agent_id NOT IN (SELECT agent_id FROM agents)
    `)
    
    db.Exec(`
        DELETE FROM workspace_allowed_agents 
        WHERE agent_id NOT IN (SELECT agent_id FROM agents)
    `)
}
```

## 7. 前端页面

### 7.1 Application 管理页面 (已有,需增强)
- 应用列表
- 创建应用
- 查看应用详情
- **新增**: 查看该应用注册的所有 agent

### 7.2 Agent 管理页面 (新增)
- Agent 列表 (按 application 分组)
- Agent 状态监控 (在线/离线/忙碌)
- Agent 允许的 workspace 列表
- 配置 agent 允许的 workspace

### 7.3 Workspace 设置页面 (需增强)
- **新增标签页**: Agent 配置
  - 可用 agent 列表 (已被 agent allow 的)
  - 当前使用的 agent
  - 允许/撤销 agent
  - 切换当前 agent
  - Agent 状态监控

## 8. 实施步骤

### 阶段 0: 数据库优化 (前置工作)
1. **执行数据库迁移脚本**
   ```bash
   psql -U postgres -d iac_platform -f scripts/migrate_agents_and_pools_to_semantic_id.sql
   ```
2. **验证迁移结果**
   - 检查 agent_pools.pool_id 是否为 VARCHAR(50)
   - 检查 agents.agent_id 是否为 VARCHAR(50)
   - 确认审计字段 created_by/updated_by 为 VARCHAR(50)
   - 验证外键关系正确

### 阶段 1: 基础表创建 (第1天)
1. **修复 Application 表**
   - 修复 callback_urls 字段类型

2. **创建授权关系表**
   - agent_allowed_workspaces
   - workspace_allowed_agents
   - agent_access_logs

### 阶段 2: 后端实现 - Agent 管理 (第2-3天)
   - Agent 注册 API
   - Agent 心跳 API
   - Agent 信息查询 API
   - Agent 注销 API

### 阶段 3: 后端实现 - 授权管理 (第4-5天)
   - Agent 允许 workspace API
   - Workspace 允许 agent API
   - 双向验证逻辑
   - 中间件

### 阶段 4: 前端实现 - Agent 管理 (第6-7天)
   - Agent 列表页面
   - Agent 详情页面
   - Agent 授权配置

### 阶段 5: 前端实现 - Workspace Agent 配置 (第8-9天)
   - Workspace 的 Agent 配置页面
   - Agent 选择和切换
   - 状态监控

### 阶段 6: 测试和文档 (第10天)
   - 单元测试
   - 集成测试
   - API 文档
   - 用户文档

## 9. 注意事项

1. **安全**:
   - AppKey/AppSecret 必须通过 HTTPS 传输
   - AppSecret 不存储在数据库(只在 Application 创建时返回一次)
   - 所有 Agent API 都需要验证 AppKey/AppSecret

2. **性能**:
   - Agent 心跳频率不要太高 (建议 30 秒)
   - 使用索引优化查询
   - 定期清理离线 agent 和孤立记录

3. **可靠性**:
   - Agent 重启自动重新注册
   - 心跳超时自动标记离线
   - 双向验证确保安全

4. **扩展性**:
   - 预留 metadata 字段用于扩展
   - 访问日志支持审计
   - 状态字段支持未来扩展

## 10. 总结

这个方案实现了:
-  Agent 注册和生命周期管理
-  Agent ↔ Workspace 双向授权
-  Workspace 只能激活一个 agent
-  双向验证机制
-  心跳和在线状态监控
-  简单清晰的 API 设计
-  完整的审计日志

符合 TFE 官方的设计模式,同时保持简单实用。
