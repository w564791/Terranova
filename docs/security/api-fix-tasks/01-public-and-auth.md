# 01 — 公开端点 + 认证 + Setup

> 源文件: `router.go`
> API 数量: 18

## 全部 API 列表

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /health | 无 | 无 | ✅ 健康检查，必须公开 |
| 2 | GET | /metrics | 无 | 无 |  暴露系统指标，建议加 Basic Auth |
| 3 | GET | /static/* | 无 | 无 | ✅ 静态资源 |
| 4 | GET | /swagger/*any | 无 | 无 | ✅ 生产环境可考虑禁用 |
| 5 | GET | /api/v1/setup/status | 无 | 无 | ✅ 判断是否需要初始化 |
| 6 | POST | /api/v1/setup/init | 无 | Handler幂等检查+Advisory Lock | ✅ 已有完善保护 |
| 7 | POST | /api/v1/auth/login | 无 | 无 | ✅ 登录入口 |
| 8 | POST | /api/v1/auth/mfa/verify | MFA Token | 无 | ✅ MFA验证流程 |
| 9 | POST | /api/v1/auth/mfa/setup | MFA Token | 无 | ✅ 首次MFA设置 |
| 10 | POST | /api/v1/auth/mfa/enable | MFA Token | 无 | ✅ 启用MFA |
| 11 | POST | /api/v1/auth/refresh | JWT | 无 | ✅ Token刷新 |
| 12 | GET | /api/v1/auth/me | JWT | 无 | ✅ 获取当前用户 |
| 13 | POST | /api/v1/auth/logout | JWT | 无 | ✅ 登出 |
| 14 | POST | /api/v1/iam/permissions/check | JWT+AuditLogger | 无(设计如此) | ✅ 用户检查自身权限 |
| 15 | GET | /api/v1/workspaces/:id/state-outputs/full | 临时Token | Token验证 | ✅ 跨workspace数据引用 |

### MFA 用户自服务 (在 protected 路由组下)

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 16 | GET | /api/v1/user/mfa/status | JWT+AuditLogger | 无 | ✅ 查看自身MFA |
| 17 | POST | /api/v1/user/mfa/setup | JWT+AuditLogger | 无 | ✅ |
| 18 | POST | /api/v1/user/mfa/verify | JWT+AuditLogger | 无 | ✅ |
| 19 | POST | /api/v1/user/mfa/disable | JWT+AuditLogger | 无 | ✅ |
| 20 | POST | /api/v1/user/mfa/backup-codes/regenerate | JWT+AuditLogger | 无 | ✅ |

## 需修复项

### GET /metrics

- **问题**: Prometheus 指标端点完全公开，暴露 goroutine 数、请求延迟、内存使用等
- **修复**: 添加 Basic Auth 或独立 metrics token，不走 JWT/IAM 体系
- **文件**: `router.go`, 新建 `middleware/metrics_auth.go`
