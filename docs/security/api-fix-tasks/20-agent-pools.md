# 20 — Agent Pool 管理 (JWT)

> 源文件: `router_agent.go` (`setupAgentPoolRoutes` + `setupWorkspaceAgentRoutes`)
> API 数量: 19
> 状态: ✅ 全部合格

## 全部 API 列表

### Agent Pool CRUD

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | POST | /api/v1/agent-pools | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/WRITE | ✅ |
| 2 | GET | /api/v1/agent-pools | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/READ | ✅ |
| 3 | GET | /api/v1/agent-pools/:pool_id | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/READ | ✅ |
| 4 | PUT | /api/v1/agent-pools/:pool_id | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/WRITE | ✅ |
| 5 | DELETE | /api/v1/agent-pools/:pool_id | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/ADMIN | ✅ |

### Pool 授权管理 (Pool 侧)

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 6 | POST | /api/v1/agent-pools/:pool_id/allow-workspaces | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/WRITE | ✅ |
| 7 | GET | /api/v1/agent-pools/:pool_id/allowed-workspaces | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/READ | ✅ |
| 8 | DELETE | /api/v1/agent-pools/:pool_id/allowed-workspaces/:workspace_id | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/WRITE | ✅ |

### Pool Token 管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 9 | POST | /api/v1/agent-pools/:pool_id/tokens | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/WRITE | ✅ |
| 10 | GET | /api/v1/agent-pools/:pool_id/tokens | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/READ | ✅ |
| 11 | DELETE | /api/v1/agent-pools/:pool_id/tokens/:token_name | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/WRITE | ✅ |
| 12 | POST | /api/v1/agent-pools/:pool_id/tokens/:token_name/rotate | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/WRITE | ✅ |

### K8s Pool 管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 13 | POST | /api/v1/agent-pools/:pool_id/sync-deployment | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/WRITE | ✅ |
| 14 | POST | /api/v1/agent-pools/:pool_id/one-time-unfreeze | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/WRITE | ✅ |
| 15 | PUT | /api/v1/agent-pools/:pool_id/k8s-config | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/WRITE | ✅ |
| 16 | GET | /api/v1/agent-pools/:pool_id/k8s-config | JWT+BypassIAMForAdmin | admin绕过 / AGENT_POOLS/ORG/READ | ✅ |

### Workspace-Pool 关联 (Workspace 侧)

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 17 | GET | /api/v1/workspaces/:id/available-pools | JWT+BypassIAMForAdmin | admin绕过 / WORKSPACES/WS/READ | ✅ |
| 18 | POST | /api/v1/workspaces/:id/set-current-pool | JWT+BypassIAMForAdmin | admin绕过 / WORKSPACES/WS/WRITE | ✅ |
| 19 | GET | /api/v1/workspaces/:id/current-pool | JWT+BypassIAMForAdmin | admin绕过 / WORKSPACES/WS/READ | ✅ |

## 无需修复

Agent Pool 权限统一使用 AGENT_POOLS 资源类型，READ/WRITE/ADMIN 分级清晰。Workspace 侧关联接口复用 WORKSPACES 权限。
