# 09 — Workspace Task 操作

> 源文件: `router_workspace.go`
> API 数量: 15
> 状态: ✅ 全部合格

## 全部 API 列表

### READ 级别 — 查看任务数据

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/workspaces/:id/tasks | JWT+AuditLogger | admin绕过 / TASK_DATA_ACCESS **或** WORKSPACE_EXECUTION **或** WORKSPACE_MANAGEMENT (WS/READ) | ✅ |
| 2 | GET | /api/v1/workspaces/:id/tasks/:tid | JWT+AuditLogger | 同上 | ✅ |
| 3 | GET | /api/v1/workspaces/:id/tasks/:tid/logs | JWT+AuditLogger | 同上 | ✅ |
| 4 | GET | /api/v1/workspaces/:id/tasks/:tid/comments | JWT+AuditLogger | 同上 | ✅ |
| 5 | GET | /api/v1/workspaces/:id/tasks/:tid/resource-changes | JWT+AuditLogger | 同上 | ✅ |
| 6 | GET | /api/v1/workspaces/:id/tasks/:tid/error-analysis | JWT+AuditLogger | 同上 | ✅ |
| 7 | GET | /api/v1/workspaces/:id/tasks/:tid/state-backup | JWT+AuditLogger | 同上 | ✅ |

### WRITE 级别 — 创建Plan/评论

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 8 | POST | /api/v1/workspaces/:id/tasks/plan | JWT+AuditLogger | admin绕过 / WORKSPACE_EXECUTION **或** WORKSPACE_MANAGEMENT (WS/WRITE) | ✅ |
| 9 | POST | /api/v1/workspaces/:id/tasks/:tid/comments | JWT+AuditLogger | 同上 | ✅ |

### ADMIN 级别 — 取消/确认Apply等危险操作

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 10 | POST | /api/v1/workspaces/:id/tasks/:tid/cancel | JWT+AuditLogger | admin绕过 / WORKSPACE_EXECUTION **或** WORKSPACE_MANAGEMENT (WS/ADMIN) | ✅ |
| 11 | POST | /api/v1/workspaces/:id/tasks/:tid/cancel-previous | JWT+AuditLogger | 同上 | ✅ |
| 12 | POST | /api/v1/workspaces/:id/tasks/:tid/confirm-apply | JWT+AuditLogger | 同上 | ✅ |
| 13 | PATCH | /api/v1/workspaces/:id/tasks/:tid/resource-changes/:rid | JWT+AuditLogger | 同上 | ✅ |
| 14 | POST | /api/v1/workspaces/:id/tasks/:tid/retry-state-save | JWT+AuditLogger | 同上 | ✅ |
| 15 | POST | /api/v1/workspaces/:id/tasks/:tid/parse-plan | JWT+AuditLogger | 同上 | ✅ |

## 无需修复

三级权限分级合理: READ(查看) → WRITE(创建Plan) → ADMIN(confirm-apply/cancel)。RequireAnyPermission 支持多资源类型 OR 逻辑。
