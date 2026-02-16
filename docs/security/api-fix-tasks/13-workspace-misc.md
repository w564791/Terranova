# 13 — Workspace 其他功能

> 源文件: `router_workspace.go`, `router_drift.go`
> API 数量: 42
> 状态: ✅ 全部合格

## 全部 API 列表

### Snapshots

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/workspaces/:id/snapshots | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 2 | GET | /api/v1/workspaces/:id/snapshots/:sid | JWT+AuditLogger | 同上 | ✅ |
| 3 | POST | /api/v1/workspaces/:id/snapshots | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 4 | POST | /api/v1/workspaces/:id/snapshots/:sid/restore | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ |
| 5 | DELETE | /api/v1/workspaces/:id/snapshots/:sid | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ |

### Editing Takeover

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 6 | POST | /api/v1/workspaces/:id/resources/:rid/editing/takeover-request | JWT+AuditLogger | admin绕过 / WORKSPACE_RESOURCES/WS/WRITE | ✅ |
| 7 | POST | /api/v1/workspaces/:id/resources/:rid/editing/takeover-response | JWT+AuditLogger | 同上 | ✅ |
| 8 | GET | /api/v1/workspaces/:id/resources/:rid/editing/pending-requests | JWT+AuditLogger | 同上 | ✅ |
| 9 | GET | /api/v1/workspaces/:id/resources/:rid/editing/request-status/:reqid | JWT+AuditLogger | 同上 | ✅ |
| 10 | POST | /api/v1/workspaces/:id/resources/:rid/editing/force-takeover | JWT+AuditLogger | admin绕过 / WORKSPACE_RESOURCES/WS/ADMIN | ✅ |

### Workspace Run Tasks

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 11 | POST | /api/v1/workspaces/:id/tasks/:tid/override-run-tasks | JWT+AuditLogger | admin绕过 / WORKSPACE_EXECUTION/WS/ADMIN | ✅ |
| 12 | GET | /api/v1/workspaces/:id/tasks/:tid/run-task-results | JWT+AuditLogger | admin绕过 / WORKSPACE_EXECUTION/WS/READ | ✅ |
| 13 | GET | /api/v1/workspaces/:id/run-tasks | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 14 | POST | /api/v1/workspaces/:id/run-tasks | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 15 | PUT | /api/v1/workspaces/:id/run-tasks/:wrtid | JWT+AuditLogger | 同上 | ✅ |
| 16 | DELETE | /api/v1/workspaces/:id/run-tasks/:wrtid | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ |

### Workspace Notifications

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 17 | GET | /api/v1/workspaces/:id/notifications | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 18 | POST | /api/v1/workspaces/:id/notifications | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 19 | PUT | /api/v1/workspaces/:id/notifications/:wnid | JWT+AuditLogger | 同上 | ✅ |
| 20 | DELETE | /api/v1/workspaces/:id/notifications/:wnid | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ |
| 21 | GET | /api/v1/workspaces/:id/notification-logs | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 22 | GET | /api/v1/workspaces/:id/notification-logs/:lid | JWT+AuditLogger | 同上 | ✅ |
| 23 | GET | /api/v1/workspaces/:id/tasks/:tid/notification-logs | JWT+AuditLogger | 同上 | ✅ |

### Workspace Outputs

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 24 | GET | /api/v1/workspaces/:id/outputs | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 25 | GET | /api/v1/workspaces/:id/state-outputs | JWT+AuditLogger | 同上 | ✅ |
| 26 | GET | /api/v1/workspaces/:id/outputs/resources | JWT+AuditLogger | 同上 | ✅ |
| 27 | GET | /api/v1/workspaces/:id/available-outputs | JWT+AuditLogger | 同上 | ✅ |
| 28 | POST | /api/v1/workspaces/:id/outputs | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 29 | PUT | /api/v1/workspaces/:id/outputs/:oid | JWT+AuditLogger | 同上 | ✅ |
| 30 | DELETE | /api/v1/workspaces/:id/outputs/:oid | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ |
| 31 | POST | /api/v1/workspaces/:id/outputs/batch | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |

### Workspace Remote Data

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 32 | GET | /api/v1/workspaces/:id/remote-data | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 33 | GET | /api/v1/workspaces/:id/remote-data/accessible-workspaces | JWT+AuditLogger | 同上 | ✅ |
| 34 | GET | /api/v1/workspaces/:id/remote-data/source-outputs | JWT+AuditLogger | 同上 | ✅ |
| 35 | POST | /api/v1/workspaces/:id/remote-data | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 36 | PUT | /api/v1/workspaces/:id/remote-data/:rdid | JWT+AuditLogger | 同上 | ✅ |
| 37 | DELETE | /api/v1/workspaces/:id/remote-data/:rdid | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ |
| 38 | GET | /api/v1/workspaces/:id/outputs-sharing | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 39 | PUT | /api/v1/workspaces/:id/outputs-sharing | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |

### Workspace Run Triggers

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 40 | GET | /api/v1/workspaces/:id/run-triggers | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 41 | GET | /api/v1/workspaces/:id/run-triggers/inbound | JWT+AuditLogger | 同上 | ✅ |
| 42 | GET | /api/v1/workspaces/:id/run-triggers/available-targets | JWT+AuditLogger | 同上 | ✅ |
| 43 | GET | /api/v1/workspaces/:id/run-triggers/available-sources | JWT+AuditLogger | 同上 | ✅ |
| 44 | POST | /api/v1/workspaces/:id/run-triggers/inbound | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 45 | POST | /api/v1/workspaces/:id/run-triggers | JWT+AuditLogger | 同上 | ✅ |
| 46 | PUT | /api/v1/workspaces/:id/run-triggers/:trid | JWT+AuditLogger | 同上 | ✅ |
| 47 | DELETE | /api/v1/workspaces/:id/run-triggers/:trid | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ |
| 48 | GET | /api/v1/workspaces/:id/tasks/:tid/trigger-executions | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 49 | POST | /api/v1/workspaces/:id/tasks/:tid/trigger-executions/:eid/toggle | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |

### Workspace Drift

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 50 | GET | /api/v1/workspaces/:id/drift-config | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/READ | ✅ |
| 51 | PUT | /api/v1/workspaces/:id/drift-config | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/WRITE | ✅ |
| 52 | GET | /api/v1/workspaces/:id/drift-status | JWT+AuditLogger | 同上 | ✅ |
| 53 | POST | /api/v1/workspaces/:id/drift-check | JWT+AuditLogger | 同上 | ✅ |
| 54 | DELETE | /api/v1/workspaces/:id/drift-check | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/ADMIN | ✅ |
| 55 | GET | /api/v1/workspaces/:id/resources-drift | JWT+AuditLogger | admin绕过 / WORKSPACE_MANAGEMENT/WS/READ | ✅ |

### Workspace Drift (Resource Level)

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 56 | DELETE | /api/v1/workspaces/:id/resources/:rid/drift | JWT+AuditLogger | admin绕过 / WORKSPACE_RESOURCES/WS/WRITE | ✅ |

## 无需修复
