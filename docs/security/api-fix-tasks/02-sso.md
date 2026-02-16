# 02 — SSO（公开 + 身份管理 + 管理端点）

> 源文件: `router_sso.go`
> API 数量: 17

## 全部 API 列表

### SSO 公开端点（无认证）

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/auth/sso/providers | 无 | 无 | ✅ 登录页展示 |
| 2 | GET | /api/v1/auth/sso/:provider/login | 无 | 无 | ✅ 发起OAuth重定向 |
| 3 | GET | /api/v1/auth/sso/:provider/callback | 无 | 无 | ✅ OAuth回调 |
| 4 | POST | /api/v1/auth/sso/:provider/callback | 无 | 无 | ✅ 兼容POST回调 |
| 5 | GET | /api/v1/auth/sso/:provider/callback/redirect | 无 | 无 | ✅ 前端重定向模式 |

### SSO 身份管理（JWT，无IAM，无AuditLogger）

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 6 | GET | /api/v1/auth/sso/identities | JWT | 无 | ⚠️ 缺AuditLogger |
| 7 | POST | /api/v1/auth/sso/identities/link | JWT | 无 | ⚠️ 缺AuditLogger |
| 8 | DELETE | /api/v1/auth/sso/identities/:id | JWT | 无 | ⚠️ 缺AuditLogger |
| 9 | PUT | /api/v1/auth/sso/identities/:id/primary | JWT | 无 | ⚠️ 缺AuditLogger |

### SSO 管理端点（JWT + RequireRole("admin")，旧版）

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 10 | GET | /api/v1/admin/sso/providers | JWT | RequireRole("admin") | ⚠️ 旧版Role |
| 11 | GET | /api/v1/admin/sso/providers/:id | JWT | RequireRole("admin") | ⚠️ 旧版Role |
| 12 | POST | /api/v1/admin/sso/providers | JWT | RequireRole("admin") | ⚠️ 旧版Role |
| 13 | PUT | /api/v1/admin/sso/providers/:id | JWT | RequireRole("admin") | ⚠️ 旧版Role |
| 14 | DELETE | /api/v1/admin/sso/providers/:id | JWT | RequireRole("admin") | ⚠️ 旧版Role |
| 15 | GET | /api/v1/admin/sso/config | JWT | RequireRole("admin") | ⚠️ 旧版Role |
| 16 | PUT | /api/v1/admin/sso/config | JWT | RequireRole("admin") | ⚠️ 旧版Role |
| 17 | GET | /api/v1/admin/sso/logs | JWT | RequireRole("admin") | ⚠️ 旧版Role |

## 需修复项

### SSO 身份管理 (#6-#9)
- **问题**: 缺少 AuditLogger 中间件，身份变更操作无审计记录
- **修复**: `ssoAuth` 路由组添加 `middleware.AuditLogger(db)`；确认 handler 校验 user_id 一致性
- **文件**: `router_sso.go`

### SSO 管理端点 (#10-#17)
- **问题**: 使用旧版 `RequireRole("admin")`，未迁移到 IAM 体系；无法委派给非admin
- **修复**: 新增 `SSO_MANAGEMENT` 资源类型，替换为 admin绕过 + IAM fallback；添加 AuditLogger
- **权限映射**: GET→READ, POST/PUT→WRITE, DELETE/PUT config→ADMIN
- **文件**: `router_sso.go`, `router.go`(传参), 迁移SQL(注册权限)
