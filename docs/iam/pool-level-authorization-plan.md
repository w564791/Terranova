# Pool级别授权架构调整方案

## 当前状态 vs 目标状态

### 当前实现 (Agent级别授权)
```
Agent --allow--> Workspace
Workspace --allow--> Agent (选择current agent)
Pool: 只是agents的组织容器
```

### 目标架构 (Pool级别授权)
```
Pool --allow--> Workspace (Pool管理workspace准入)
Workspace --select--> Pool (Workspace选择使用哪个pool)
Pool中的所有agents共享相同的workspace访问权限
```

## 需要的变更

### 1. 数据库变更

#### 新表: pool_allowed_workspaces
```sql
CREATE TABLE pool_allowed_workspaces (
    id SERIAL PRIMARY KEY,
    pool_id VARCHAR(50) NOT NULL REFERENCES agent_pools(pool_id),
    workspace_id VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    allowed_by VARCHAR(50),
    allowed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_by VARCHAR(50),
    revoked_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(pool_id, workspace_id)
);
```

#### 修改workspace表
```sql
ALTER TABLE workspaces ADD COLUMN current_pool_id VARCHAR(50) REFERENCES agent_pools(pool_id);
```

#### 可以废弃的表
- agent_allowed_workspaces (改为pool级别)
- workspace_allowed_agents (改为workspace选择pool)

### 2. 后端API变更

#### 新增Pool授权API
```
POST   /api/v1/agent-pools/:pool_id/allow-workspaces
GET    /api/v1/agent-pools/:pool_id/allowed-workspaces
DELETE /api/v1/agent-pools/:pool_id/allowed-workspaces/:workspace_id
```

#### 修改Workspace API
```
GET    /api/v1/workspaces/:id/available-pools (获取可用的pools)
POST   /api/v1/workspaces/:id/set-current-pool (设置当前pool)
GET    /api/v1/workspaces/:id/current-pool (获取当前pool)
```

#### 修改验证逻辑
```go
ValidatePoolAccess(poolID, workspaceID) 检查:
1. Pool允许该workspace (pool_allowed_workspaces)
2. Workspace设置了该pool为current (workspaces.current_pool_id)
3. Pool中有在线的agents
```

### 3. 前端变更

#### AgentPoolDetail页面
显示:
- Pool信息
- **Pool允许的workspace列表** (新增)
- Pool中的agents列表

操作:
- 添加/移除允许的workspace
- 编辑pool信息
- 删除pool

#### WorkspaceAgentConfig组件
改为:
- 显示可用的pools列表
- 选择当前pool
- 显示当前pool中的agents

### 4. 实施步骤

1. 创建数据库迁移脚本
2. 创建新的model: PoolAllowedWorkspace
3. 修改agent_service.go添加pool级别验证
4. 创建pool授权handler
5. 更新路由
6. 修改前端AgentPoolDetail显示workspace列表
7. 修改WorkspaceAgentConfig显示pool选择

## 估算工作量

- 数据库: 1小时
- 后端API: 2小时
- 前端调整: 2小时
- 测试验证: 1小时
- **总计: 约6小时**

## 建议

由于这是架构级别的调整,建议:
1. 先确认Pool级别授权是否真的是最终需求
2. 评估是否可以保留Agent级别授权作为更细粒度的控制
3. 考虑是否需要同时支持两种模式

当前Agent级别授权系统已完整实现并可用,可以先使用,然后再决定是否需要调整为Pool级别。
