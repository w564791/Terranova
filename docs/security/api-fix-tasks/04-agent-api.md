# 04 — Agent API（Pool Token 认证）

> 源文件: `router_agent.go`
> API 数量: 22
> 状态: ✅ 全部合格

## 全部 API 列表

### Agent 管理（PoolToken）

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | POST | /api/v1/agents/register | PoolToken | Token所属Pool | ✅ |
| 2 | GET | /api/v1/agents/pool/secrets | PoolToken | Token所属Pool | ✅ |
| 3 | GET | /api/v1/agents/:agent_id | PoolToken | Token所属Pool | ✅ |
| 4 | DELETE | /api/v1/agents/:agent_id | PoolToken | Token所属Pool | ✅ |
| 5 | GET | /api/v1/agents/control | 无(已废弃) | 返回410 Gone | ✅ |

### Agent Task API（PoolToken + TaskCheck）

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 6 | GET | /api/v1/agents/tasks/:task_id/data | PoolToken+TaskCheck | Pool必须授权task所属workspace | ✅ |
| 7 | POST | /api/v1/agents/tasks/:task_id/logs/chunk | PoolToken+TaskCheck | 同上 | ✅ |
| 8 | PUT | /api/v1/agents/tasks/:task_id/status | PoolToken+TaskCheck | 同上 | ✅ |
| 9 | POST | /api/v1/agents/tasks/:task_id/state | PoolToken+TaskCheck | 同上 | ✅ |
| 10 | GET | /api/v1/agents/tasks/:task_id/plan-task | PoolToken+TaskCheck | 同上 | ✅ |
| 11 | POST | /api/v1/agents/tasks/:task_id/plan-data | PoolToken+TaskCheck | 同上 | ✅ |
| 12 | POST | /api/v1/agents/tasks/:task_id/plan-json | PoolToken+TaskCheck | 同上 | ✅ |
| 13 | POST | /api/v1/agents/tasks/:task_id/parse-plan-changes | PoolToken+TaskCheck | 同上 | ✅ |
| 14 | GET | /api/v1/agents/tasks/:task_id/logs | PoolToken+TaskCheck | 同上 | ✅ |

### Agent Workspace API（PoolToken + WorkspaceCheck）

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 15 | POST | /api/v1/agents/workspaces/:wid/lock | PoolToken+WsCheck | Pool必须授权该workspace | ✅ |
| 16 | POST | /api/v1/agents/workspaces/:wid/unlock | PoolToken+WsCheck | 同上 | ✅ |
| 17 | GET | /api/v1/agents/workspaces/:wid/state/max-version | PoolToken+WsCheck | 同上 | ✅ |
| 18 | PATCH | /api/v1/agents/workspaces/:wid/fields | PoolToken+WsCheck | 同上 | ✅ |
| 19 | GET | /api/v1/agents/workspaces/:wid/terraform-lock-hcl | PoolToken+WsCheck | 同上 | ✅ |
| 20 | PUT | /api/v1/agents/workspaces/:wid/terraform-lock-hcl | PoolToken+WsCheck | 同上 | ✅ |

### Agent Terraform Version API（PoolToken）

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 21 | GET | /api/v1/agents/terraform-versions/default | PoolToken | Token所属Pool | ✅ |
| 22 | GET | /api/v1/agents/terraform-versions/:version | PoolToken | Token所属Pool | ✅ |

## 无需修复

Agent API 使用 PoolToken 认证 + Workspace 级授权检查，双层校验设计合理。
