# 12 — Workspace Resources

> 源文件: `router_workspace.go`
> API 数量: 22
> 状态: ✅ 全部合格

## 全部 API 列表

### READ 级别

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/workspaces/:id/resources | JWT+AuditLogger | admin绕过 / WORKSPACE_RESOURCES **或** WORKSPACE_MANAGEMENT (WS/READ) | ✅ |
| 2 | GET | /api/v1/workspaces/:id/resources/:rid | JWT+AuditLogger | 同上 | ✅ |
| 3 | GET | /api/v1/workspaces/:id/resources/:rid/versions | JWT+AuditLogger | 同上 | ✅ |
| 4 | GET | /api/v1/workspaces/:id/resources/:rid/versions/compare | JWT+AuditLogger | 同上 | ✅ |
| 5 | GET | /api/v1/workspaces/:id/resources/:rid/versions/:v | JWT+AuditLogger | 同上 | ✅ |
| 6 | GET | /api/v1/workspaces/:id/resources/:rid/dependencies | JWT+AuditLogger | 同上 | ✅ |
| 7 | GET | /api/v1/workspaces/:id/resources/:rid/editing/status | JWT+AuditLogger | 同上 | ✅ |
| 8 | GET | /api/v1/workspaces/:id/resources/:rid/drift | JWT+AuditLogger | 同上 | ✅ |
| 9 | GET | /api/v1/workspaces/:id/resources/export/hcl | JWT+AuditLogger | 同上 | ✅ |

### WRITE 级别

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 10 | POST | /api/v1/workspaces/:id/resources | JWT+AuditLogger | admin绕过 / WORKSPACE_RESOURCES/WS/WRITE **或** WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 11 | POST | /api/v1/workspaces/:id/resources/import | JWT+AuditLogger | 同上 | ✅ |
| 12 | POST | /api/v1/workspaces/:id/resources/deploy | JWT+AuditLogger | admin绕过 / WORKSPACE_EXECUTION/WS/WRITE **或** WORKSPACE_MANAGEMENT/WS/WRITE | ✅ deploy用EXECUTION权限 |
| 13 | PUT | /api/v1/workspaces/:id/resources/:rid | JWT+AuditLogger | admin绕过 / WORKSPACE_RESOURCES/WS/WRITE | ✅ |
| 14 | PUT | /api/v1/workspaces/:id/resources/:rid/dependencies | JWT+AuditLogger | 同上 | ✅ |
| 15 | POST | /api/v1/workspaces/:id/resources/:rid/restore | JWT+AuditLogger | 同上 | ✅ |
| 16 | POST | /api/v1/workspaces/:id/resources/:rid/versions/:v/rollback | JWT+AuditLogger | 同上 | ✅ |
| 17 | POST | /api/v1/workspaces/:id/resources/:rid/editing/start | JWT+AuditLogger | 同上 | ✅ |
| 18 | POST | /api/v1/workspaces/:id/resources/:rid/editing/heartbeat | JWT+AuditLogger | 同上 | ✅ |
| 19 | POST | /api/v1/workspaces/:id/resources/:rid/editing/end | JWT+AuditLogger | 同上 | ✅ |
| 20 | POST | /api/v1/workspaces/:id/resources/:rid/drift/save | JWT+AuditLogger | 同上 | ✅ |
| 21 | POST | /api/v1/workspaces/:id/resources/:rid/drift/takeover | JWT+AuditLogger | 同上 | ✅ |

### ADMIN 级别

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 22 | DELETE | /api/v1/workspaces/:id/resources/:rid | JWT+AuditLogger | admin绕过 / WORKSPACE_RESOURCES/WS/ADMIN | ✅ |

## 无需修复
