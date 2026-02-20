# 25 — Dashboard 🟢 P3

> 源文件: `router_dashboard.go`
> API 数量: 2
> 状态: ⚠️ 缺少 admin 绕过

## 全部 API 列表

| # | Method | Path | 认证 | 授权 | 目标权限 | 状态 |
|---|--------|------|------|------|----------|------|
| 1 | GET | /api/v1/dashboard/overview | JWT+AuditLogger | ORGANIZATION/ORG/READ（无admin绕过） | ORGANIZATION/ORG/READ + admin绕过 | ⚠️ |
| 2 | GET | /api/v1/dashboard/compliance | JWT+AuditLogger | ORGANIZATION/ORG/READ（无admin绕过） | ORGANIZATION/ORG/READ + admin绕过 | ⚠️ |

## 修复方案

### 问题
Dashboard 路由使用 `middleware.JWTAuth()` + `middleware.AuditLogger(db)` 自行挂载认证，直接用 `iamMiddleware.RequirePermission()` 作为中间件，缺少 admin 绕过逻辑。虽然 admin 用户如果被授予了 ORGANIZATION/ORG/READ 也能访问，但与其他所有模块的 admin 绕过模式不一致。

### 步骤
1. 改为标准 admin 绕过 + IAM fallback 模式
2. 保持使用 `ORGANIZATION/ORG/READ` 权限

### 修改文件
```
backend/internal/router/router_dashboard.go
```
