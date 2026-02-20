# 11 — Workspace Variables

> 源文件: `router_workspace.go`
> API 数量: 7
> 状态: ✅ 全部合格

## 全部 API 列表

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/workspaces/:id/variables | JWT+AuditLogger | admin绕过 / WORKSPACE_VARIABLES **或** WORKSPACE_MANAGEMENT (WS/READ) | ✅ |
| 2 | GET | /api/v1/workspaces/:id/variables/:vid | JWT+AuditLogger | 同上 | ✅ |
| 3 | POST | /api/v1/workspaces/:id/variables | JWT+AuditLogger | admin绕过 / WORKSPACE_VARIABLES **或** WORKSPACE_MANAGEMENT (WS/WRITE) | ✅ |
| 4 | PUT | /api/v1/workspaces/:id/variables/:vid | JWT+AuditLogger | 同上 | ✅ |
| 5 | DELETE | /api/v1/workspaces/:id/variables/:vid | JWT+AuditLogger | admin绕过 / WORKSPACE_VARIABLES **或** WORKSPACE_MANAGEMENT (WS/ADMIN) | ✅ |
| 6 | GET | /api/v1/workspaces/:id/variables/:vid/versions | JWT+AuditLogger | admin绕过 / WORKSPACE_VARIABLES/WS/READ | ✅ |
| 7 | GET | /api/v1/workspaces/:id/variables/:vid/versions/:v | JWT+AuditLogger | 同上 | ✅ |

## 无需修复
