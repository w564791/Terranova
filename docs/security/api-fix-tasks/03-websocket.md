# 03 — WebSocket 端点

> 源文件: `router.go`
> API 数量: 3

## 全部 API 列表

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/ws/editing/:session_id | JWT | 无 | ✅ session_id 提供隔离 |
| 2 | GET | /api/v1/ws/sessions | JWT | 无 | ✅ 查看活跃会话 |
| 3 | GET | /api/v1/ws/agent-pools/:pool_id/metrics | JWT | 无 | ⚠️ 可查看任意Pool指标 |

## 需修复项

### Agent Metrics WebSocket (#3)
- **问题**: 任何认证用户可订阅任意 Pool 的实时指标
- **修复**: 可选添加 `AGENT_POOLS/ORGANIZATION/READ` 权限检查
- **文件**: `router.go`
