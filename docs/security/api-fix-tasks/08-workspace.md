# 08 — Workspace CRUD + Pool 关联

> 源文件: `router_workspace.go`, `router_agent.go` (setupWorkspaceAgentRoutes)
> API 数量: 12
> 状态: ✅ 全部合格

## 全部 API 列表

### Workspace CRUD

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/workspaces | JWT+AuditLogger | admin绕过 / WORKSPACES/ORG/READ | ✅ |
| 2 | GET | /api/v1/workspaces/:id | JWT+AuditLogger | admin绕过 / WORKSPACES/ORG/READ **或** WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 3 | GET | /api/v1/workspaces/:id/overview | JWT+AuditLogger | admin绕过 / WORKSPACES/ORG/READ **或** WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 4 | PUT | /api/v1/workspaces/:id | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 5 | PATCH | /api/v1/workspaces/:id | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 6 | POST | /api/v1/workspaces/:id/lock | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 7 | POST | /api/v1/workspaces/:id/unlock | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 8 | DELETE | /api/v1/workspaces/:id | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ |
| 9 | POST | /api/v1/workspaces | JWT+AuditLogger | admin绕过 / WORKSPACES/ORG/WRITE | ✅ |

### Workspace Pool 关联

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 10 | GET | /api/v1/workspaces/:id/available-pools | JWT+AuditLogger | admin绕过 / WORKSPACES/WS/READ | ✅ |
| 11 | POST | /api/v1/workspaces/:id/set-current-pool | JWT+AuditLogger | admin绕过 / WORKSPACES/WS/WRITE | ✅ |
| 12 | GET | /api/v1/workspaces/:id/current-pool | JWT+AuditLogger | admin绕过 / WORKSPACES/WS/READ | ✅ |

## 无需修复

权限设计合理: 列表用ORG级，单个操作用WS级；GET→READ, PUT/POST→WRITE, DELETE→ADMIN。
