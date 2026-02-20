# 24 — 全局 Task 日志

> 源文件: `router_task.go`
> API 数量: 4
> 状态: ✅ 全部合格

## 全部 API 列表

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/tasks/:task_id/output/stream | JWT+AuditLogger | admin绕过 / TASK_LOGS/ORG/READ | ✅ |
| 2 | GET | /api/v1/tasks/:task_id/logs | JWT+AuditLogger | admin绕过 / TASK_LOGS/ORG/READ | ✅ |
| 3 | GET | /api/v1/tasks/:task_id/logs/download | JWT+AuditLogger | admin绕过 / TASK_LOGS/ORG/READ | ✅ |
| 4 | GET | /api/v1/terraform/streams/stats | JWT+AuditLogger | admin绕过 / TASK_LOGS/ORG/READ | ✅ |

## 无需修复

全局 Task 日志接口统一使用 TASK_LOGS 资源类型，仅 READ 级别。
