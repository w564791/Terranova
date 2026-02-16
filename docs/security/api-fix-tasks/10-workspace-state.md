# 10 — Workspace State 操作

> 源文件: `router_workspace.go`
> API 数量: 14
> 状态: ✅ 全部合格

## 全部 API 列表

### READ 级别 — 元数据

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/workspaces/:id/current-state | JWT+AuditLogger | admin绕过 / WORKSPACE_STATE **或** WORKSPACE_MANAGEMENT (WS/READ) | ✅ |
| 2 | GET | /api/v1/workspaces/:id/state-versions | JWT+AuditLogger | 同上 | ✅ |
| 3 | GET | /api/v1/workspaces/:id/state/versions | JWT+AuditLogger | 同上 | ✅ |
| 4 | GET | /api/v1/workspaces/:id/state/versions/:v | JWT+AuditLogger | 同上 | ✅ |
| 5 | GET | /api/v1/workspaces/:id/state/versions/:v/download | JWT+AuditLogger | 同上 | ✅ |
| 6 | GET | /api/v1/workspaces/:id/state-versions/compare | JWT+AuditLogger | 同上 | ✅ |
| 7 | GET | /api/v1/workspaces/:id/state-versions/:v/metadata | JWT+AuditLogger | 同上 | ✅ |
| 8 | GET | /api/v1/workspaces/:id/state-versions/:v | JWT+AuditLogger | 同上 | ✅ |

### READ 级别 — 敏感State内容（独立权限）

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 9 | GET | /api/v1/workspaces/:id/state/versions/:v/retrieve | JWT+AuditLogger | admin绕过 / **WORKSPACE_STATE_SENSITIVE**/WS/READ **或** WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ 细粒度控制 |

### WRITE 级别

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 10 | POST | /api/v1/workspaces/:id/state/upload | JWT+AuditLogger | admin绕过 / WORKSPACE_STATE/WS/WRITE **或** WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 11 | POST | /api/v1/workspaces/:id/state/upload-file | JWT+AuditLogger | 同上 | ✅ |

### ADMIN 级别

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 12 | POST | /api/v1/workspaces/:id/state/rollback | JWT+AuditLogger | admin绕过 / WORKSPACE_STATE/WS/ADMIN **或** WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ |
| 13 | POST | /api/v1/workspaces/:id/state-versions/:v/rollback | JWT+AuditLogger | 同上 | ✅ |
| 14 | DELETE | /api/v1/workspaces/:id/state-versions/:v | JWT+AuditLogger | 同上 | ✅ |

## 无需修复

亮点: WORKSPACE_STATE_SENSITIVE 与 WORKSPACE_STATE 权限分离，state完整内容（含密码/密钥）需独立权限。
