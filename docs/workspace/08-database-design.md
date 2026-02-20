# Workspace模块 - 数据库设计

> **文档版本**: v1.0  
> **创建日期**: 2025-10-09  
> **状态**: 完整设计

## 📘 概述

本文档详细说明Workspace模块的数据库表结构、索引设计和约束关系。

## 🗄️ 核心表

### 1. workspaces表

**用途**: 存储Workspace基本信息

```sql
CREATE TABLE workspaces (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    
    -- 执行配置
    execution_mode VARCHAR(50) DEFAULT 'local', -- 'local', 'agent', 'k8s'
    terraform_version VARCHAR(50) DEFAULT '1.6.0',
    working_directory VARCHAR(255) DEFAULT '/',
    
    -- Agent模式配置
    agent_pool_id INTEGER REFERENCES agent_pools(id),
    
    -- K8s模式配置
    k8s_config_id INTEGER REFERENCES k8s_configs(id),
    
    -- 自动化配置
    auto_apply BOOLEAN DEFAULT false,
    auto_destroy BOOLEAN DEFAULT false,
    
    -- 生命周期状态
    state VARCHAR(50) DEFAULT 'created',
    -- 'created', 'planning', 'plan_done', 'waiting_apply', 
    -- 'applying', 'completed', 'failed'
    
    -- State版本
    current_version INTEGER DEFAULT 0,
    current_state_id INTEGER REFERENCES workspace_state_versions(id),
    
    -- 锁定机制
    is_locked BOOLEAN DEFAULT false,
    locked_by INTEGER REFERENCES users(id),
    locked_at TIMESTAMP,
    lock_reason TEXT,
    
    -- 标签和元数据
    tags JSONB DEFAULT '[]',
    metadata JSONB DEFAULT '{}',
    
    -- 审计字段
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX idx_workspaces_name ON workspaces(name);
CREATE INDEX idx_workspaces_state ON workspaces(state);
CREATE INDEX idx_workspaces_execution_mode ON workspaces(execution_mode);
CREATE INDEX idx_workspaces_created_by ON workspaces(created_by);
CREATE INDEX idx_workspaces_deleted_at ON workspaces(deleted_at);
```

### 2. workspace_tasks表

**用途**: 存储Plan/Apply任务

```sql
CREATE TABLE workspace_tasks (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id),
    
    -- 任务类型和状态
    task_type VARCHAR(50) NOT NULL, -- 'plan', 'apply', 'destroy'
    status VARCHAR(50) DEFAULT 'pending',
    -- 'pending', 'running', 'success', 'failed', 'cancelled'
    
    -- 执行信息
    agent_id INTEGER REFERENCES agents(id),
    k8s_config_id INTEGER REFERENCES k8s_configs(id),
    k8s_pod_name VARCHAR(255),
    execution_node VARCHAR(255),
    
    -- 任务锁
    locked_by VARCHAR(255), -- Agent ID
    locked_at TIMESTAMP,
    lock_expires_at TIMESTAMP,
    
    -- 输出和错误
    output TEXT,
    error TEXT,
    plan_json JSONB,
    
    -- 时间统计
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration_seconds INTEGER,
    
    -- 重试
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    
    -- 审计
    message TEXT,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_workspace_tasks_workspace_id ON workspace_tasks(workspace_id);
CREATE INDEX idx_workspace_tasks_status ON workspace_tasks(status);
CREATE INDEX idx_workspace_tasks_task_type ON workspace_tasks(task_type);
CREATE INDEX idx_workspace_tasks_agent_id ON workspace_tasks(agent_id);
CREATE INDEX idx_workspace_tasks_locked_by ON workspace_tasks(locked_by);
CREATE INDEX idx_workspace_tasks_created_at ON workspace_tasks(created_at);
```

### 3. workspace_state_versions表

**用途**: 存储State版本历史

```sql
CREATE TABLE workspace_state_versions (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id),
    
    -- 版本信息
    version INTEGER NOT NULL,
    content JSONB NOT NULL,
    checksum VARCHAR(64) NOT NULL, -- MD5/SHA256
    size_bytes INTEGER,
    
    -- 关联任务
    task_id INTEGER REFERENCES workspace_tasks(id),
    
    -- 审计
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(workspace_id, version)
);

CREATE INDEX idx_state_versions_workspace_id ON workspace_state_versions(workspace_id);
CREATE INDEX idx_state_versions_version ON workspace_state_versions(workspace_id, version);
CREATE INDEX idx_state_versions_created_at ON workspace_state_versions(created_at);
```

### 4. agents表

**用途**: 存储Agent信息

```sql
CREATE TABLE agents (
    id SERIAL PRIMARY KEY,
    
    -- Agent标识
    agent_id VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    
    -- 类型和状态
    agent_type VARCHAR(50) NOT NULL, -- 'remote', 'k8s'
    status VARCHAR(50) DEFAULT 'offline',
    -- 'online', 'offline', 'busy', 'error'
    
    -- 标签和能力
    labels JSONB DEFAULT '[]',
    capabilities JSONB DEFAULT '{}',
    
    -- Token认证
    token VARCHAR(255) UNIQUE,
    token_expires_at TIMESTAMP,
    
    -- 连接信息
    endpoint VARCHAR(255),
    last_heartbeat_at TIMESTAMP,
    
    -- 统计
    total_tasks INTEGER DEFAULT 0,
    success_tasks INTEGER DEFAULT 0,
    failed_tasks INTEGER DEFAULT 0,
    
    -- 元数据
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX idx_agents_agent_id ON agents(agent_id);
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_agents_token ON agents(token);
CREATE INDEX idx_agents_deleted_at ON agents(deleted_at);
```

### 5. agent_pools表

**用途**: 存储Agent池配置

```sql
CREATE TABLE agent_pools (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    
    -- 池类型和策略
    pool_type VARCHAR(50) NOT NULL, -- 'static', 'dynamic'
    selection_strategy VARCHAR(50) DEFAULT 'round_robin',
    -- 'round_robin', 'least_busy', 'random', 'label_match'
    
    -- 标签要求
    required_labels JSONB DEFAULT '[]',
    
    -- Agent列表
    agent_ids JSONB DEFAULT '[]',
    
    -- 元数据
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX idx_agent_pools_name ON agent_pools(name);
CREATE INDEX idx_agent_pools_deleted_at ON agent_pools(deleted_at);
```

### 6. k8s_configs表

**用途**: 存储K8s集群配置

```sql
CREATE TABLE k8s_configs (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    
    -- K8s连接
    kubeconfig TEXT, -- base64编码
    context_name VARCHAR(255),
    namespace VARCHAR(255) DEFAULT 'default',
    
    -- Pod模板
    pod_template JSONB NOT NULL,
    service_account_name VARCHAR(255) DEFAULT 'default',
    image_pull_secrets JSONB DEFAULT '[]',
    
    -- 配置状态
    is_default BOOLEAN DEFAULT false,
    status VARCHAR(50) DEFAULT 'active',
    
    -- 元数据
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX idx_k8s_configs_name ON k8s_configs(name);
CREATE INDEX idx_k8s_configs_is_default ON k8s_configs(is_default);
CREATE INDEX idx_k8s_configs_deleted_at ON k8s_configs(deleted_at);
```

## 🔗 关系图

```
users
  ↓ (created_by)
workspaces ←→ workspace_tasks
  ↓              ↓
workspace_state_versions
  
workspaces → agent_pools → agents
workspaces → k8s_configs
workspace_tasks → agents
workspace_tasks → k8s_configs
```

## 📊 索引策略

### 查询优化索引

**高频查询**:
1. 按Workspace ID查询任务
2. 按状态查询任务
3. 按Agent ID查询任务
4. 按时间范围查询

**复合索引**:
```sql
CREATE INDEX idx_tasks_workspace_status 
ON workspace_tasks(workspace_id, status);

CREATE INDEX idx_tasks_status_created 
ON workspace_tasks(status, created_at);
```

## 🔒 约束和触发器

### 更新时间触发器

```sql
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_workspaces_updated_at 
BEFORE UPDATE ON workspaces
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

### 软删除约束

```sql
ALTER TABLE workspaces 
ADD CONSTRAINT check_deleted_at 
CHECK (deleted_at IS NULL OR deleted_at >= created_at);
```

## 📈 容量规划

### 估算

**假设**:
- 1000个Workspaces
- 每个Workspace平均100个任务
- 每个任务平均10个State版本

**存储需求**:
- workspaces: ~1MB
- workspace_tasks: ~100MB
- workspace_state_versions: ~10GB (取决于State大小)

### 分区策略

**按时间分区**:
```sql
CREATE TABLE workspace_tasks_2025_10 
PARTITION OF workspace_tasks
FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');
```

---

**相关文档**:
- [00-overview.md](./00-overview.md) - 总览和架构
- [03-state-management.md](./03-state-management.md) - State管理
- [09-api-specification.md](./09-api-specification.md) - API规范
