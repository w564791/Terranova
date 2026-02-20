# 22 — 通知管理 ⚠️ P1

> 源文件: `router_notification.go`
> API 数量: 7
> 状态: ⚠️ 全部缺 IAM 权限检查

## 全部 API 列表

| # | Method | Path | 认证 | 授权 | 目标权限 | 状态 |
|---|--------|------|------|------|----------|------|
| 1 | GET | /api/v1/notifications | JWT+BypassIAMForAdmin | admin绕过/隐式拒绝非admin | NOTIFICATIONS/ORG/READ | ⚠️ |
| 2 | GET | /api/v1/notifications/available | JWT+BypassIAMForAdmin | 同上 | NOTIFICATIONS/ORG/READ | ⚠️ |
| 3 | GET | /api/v1/notifications/:notification_id | JWT+BypassIAMForAdmin | 同上 | NOTIFICATIONS/ORG/READ | ⚠️ |
| 4 | POST | /api/v1/notifications | JWT+BypassIAMForAdmin | 同上 | NOTIFICATIONS/ORG/WRITE | ⚠️ |
| 5 | PUT | /api/v1/notifications/:notification_id | JWT+BypassIAMForAdmin | 同上 | NOTIFICATIONS/ORG/WRITE | ⚠️ |
| 6 | DELETE | /api/v1/notifications/:notification_id | JWT+BypassIAMForAdmin | 同上 | NOTIFICATIONS/ORG/ADMIN | ⚠️ |
| 7 | POST | /api/v1/notifications/:notification_id/test | JWT+BypassIAMForAdmin | 同上 | NOTIFICATIONS/ORG/WRITE | ⚠️ |

## 修复方案

### 根因
`SetupNotificationRoutes` 接收的是 `adminProtected` 路由组（已有 JWT + BypassIAMForAdmin），但未自行添加 IAM 权限检查。当前 admin 用户可正常访问，非 admin 用户被 BypassIAMForAdmin 隐式拒绝。

### 步骤
1. 在 `permission_definitions` 注册 `NOTIFICATIONS` 资源类型
2. 为每个接口添加 admin 绕过 + IAM fallback 模式
3. GET→READ, POST/PUT→WRITE, DELETE→ADMIN

### 修改文件
```
backend/internal/router/router_notification.go
backend/internal/domain/valueobject/resource_type.go
数据库迁移 SQL
```
