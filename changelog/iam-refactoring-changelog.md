# IAM权限系统重构更新日志

## 2026-02-16

### 概述

移除遗留的 `users.role` 字段（"admin"/"user"），全面切换到 IAM 权限系统。解决了角色修改需要重新登录、仅有两级权限、`BypassIAMForAdmin` 绕过所有 IAM 检查等问题。

---

### 破坏性变更

- **JWT token 不再包含 `role` 字段**，前端需使用 `is_system_admin` 替代
- **`/api/v1/auth/me` 返回值**：`role` 字段替换为 `is_system_admin`（boolean）
- **登录响应**：`role` 字段替换为 `is_system_admin`（boolean）
- **删除中间件**：`BypassIAMForAdmin()`、`RequireRole()` 已移除
- **CMDB 只读路由新增权限检查**：`GET /cmdb/*` 需要 `WORKSPACES/ORGANIZATION/READ` 权限
- **Secrets 路由新增权限检查**：`/:resourceType/:resourceId/secrets/*` 需要 `SYSTEM_SETTINGS` 权限

### 后端变更

#### 中间件层

- `JWTAuth()`：每次请求从数据库查询 `is_system_admin`，权限修改立即生效，无需重新登录
- `RequirePermission()`、`RequireAnyPermission()`：新增 `is_system_admin` 直接放行逻辑
- `RequireAnyPermission()`：修复 scope_id 解析 bug —— 原先所有权限条目共用同一个 scope_id（URL 路径参数），导致 ORGANIZATION scope 传入 `ws-xxx` 而非 `1`。现在按每个权限条目的 scope_type 分别解析
- 新增 `RequireSystemAdmin()` 中间件，用于 SSO 配置等系统级操作

#### 路由层（12 个路由文件重构）

- **~350 个路由 handler 闭包**简化为直接中间件链：
  ```go
  // 旧：闭包内手动检查 role
  workspaces.GET("/path", func(c *gin.Context) {
      role, _ := c.Get("role")
      if role == "admin" { handler(c); return }
      iamMiddleware.RequireAnyPermission(...)(c)
      if !c.IsAborted() { handler(c) }
  })
  // 新：直接中间件链
  workspaces.GET("/path",
      iamMiddleware.RequireAnyPermission(...),
      handler,
  )
  ```
- **93 个 workspace 路由**添加双 scope 权限模式（ORGANIZATION + WORKSPACE），确保组织管理员能访问工作空间功能：
  ```go
  iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
      {ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
      {ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
  })
  ```
- `router_ai.go`：无效资源类型 `"WORKSPACE"` 修正为 `"WORKSPACE_MANAGEMENT"`
- `router_cmdb.go`：无效资源类型 `"cmdb"` 修正为 `"SYSTEM_SETTINGS"`
- `router_sso.go`：`RequireRole("admin")` 替换为 `RequireSystemAdmin()`

#### 安全修复

| 路由 | 问题 | 修复 |
|---|---|---|
| `router_secret.go` 全部 5 个路由 | 零 IAM 检查，任何登录用户可操作 | 添加 `SYSTEM_SETTINGS` 权限（READ/WRITE/ADMIN） |
| `router_cmdb.go` 7 个只读路由 | 零 IAM 检查，暴露所有 workspace 资源数据 | 添加 `WORKSPACES/ORGANIZATION/READ` 权限 |
| `router_ai.go` embedding/config-status | 缺少 IAM 检查 | 添加 `AI_ANALYSIS/ORGANIZATION/READ` 权限 |
| `router_user.go` reset-password | ScopeType=`"USER"` 无效，ParseScopeType 返回 400 | 修正为 `"ORGANIZATION"` |
| `router_notification.go` 全部路由 | 原依赖 `BypassIAMForAdmin` 作为入口守卫，移除后无保护 | 添加 `SYSTEM_SETTINGS` 权限 |
| `router_manifest.go` 全部路由 | 同上 | 添加 `SYSTEM_SETTINGS` 权限 |

#### Handler/Service 层

- `handlers/auth.go`：JWT 生成不再包含 `role`；`GetMe` 返回 `is_system_admin`
- `handlers/mfa_handler.go`：MFA 流程使用 `is_system_admin`
- `handlers/setup.go`：`InitAdmin` 设置 `is_system_admin = true`
- `handlers/permission_handler.go`：admin 检查改用 `c.Get("is_system_admin")`
- `models/user.go`：`Role` 字段标记 `json:"-"`（API 不再返回）；`IsSystemAdmin` 为活跃字段
- `services/sso/sso_service.go`：SSO 用户创建不再设置 `DefaultRole`

### 前端变更

| 文件 | 变更 |
|---|---|
| `store/slices/authSlice.ts` | auth state 使用 `is_system_admin` 替代 `role` |
| `components/ProtectedRoute.tsx` | 移除 `/admin/*` 硬编码拦截，权限由后端 IAM 控制 |
| `components/Layout.tsx` | 导航菜单基于 IAM 权限动态显示（`IAM_PERMISSIONS`、`SYSTEM_SETTINGS`、`WORKSPACES`、`MODULES`、`ORGANIZATION`） |
| `pages/admin/UserManagement.tsx` | 用户管理使用 `is_system_admin` |
| `pages/admin/PermissionManagement.tsx` | 权限页面守卫使用 `is_system_admin` |
| `pages/CMDB.tsx` | admin 功能可见性使用 `is_system_admin` |

### 数据库变更

执行种子数据更新：

```sql
-- 1. admin 角色：旧 cmdb 权限替换为 SYSTEM_SETTINGS
UPDATE iam_role_policies
SET permission_id = 'orgpm-system-settings'
WHERE role_id = 1 AND permission_id = 'perm_cmdb_7663010ab76689d8';

-- 2. 删除旧的 cmdb 权限定义
DELETE FROM permission_definitions WHERE id = 'perm_cmdb_7663010ab76689d8';
```

补齐内置角色策略（已包含在 `migrations/fix_builtin_role_policies.sql` 中）：
- org_admin(role_id=2)：补齐 13 项组织级权限
- project_admin(role_id=3)：补齐 10 项项目级权限
- workspace_admin(role_id=4)：补敏感状态权限
- developer(role_id=5)：补 10 项组织只读 + AI 写入权限
- viewer(role_id=6)：补 30 项新增资源的 READ 权限
- user(role_id=30)：补 7 项最低可用权限

```bash
# 对已有环境执行迁移
docker exec -i iac-platform-postgres-pg18 psql -U postgres -d iac_platform < backend/migrations/fix_builtin_role_policies.sql
```

### 权限架构

```
Request -> JWTAuth (从DB查询 user_id, is_system_admin)
        -> AuditLogger
        -> IAM Permission Check:
             - is_system_admin == true? -> ALLOW (bypass)
             - 检查 IAM grants           -> ALLOW/DENY
```

| 中间件 | 用途 | 使用场景 |
|---|---|---|
| `JWTAuth()` | 认证，设置 `user_id` 和 `is_system_admin` | 所有需认证路由 |
| `RequirePermission(resource, scope, level)` | 单一 IAM 权限检查（含 admin bypass） | 指定单一权限的路由 |
| `RequireAnyPermission([]PermissionRequirement)` | OR 逻辑 IAM 检查（含 admin bypass） | 需要多种权限路径的路由 |
| `RequireSystemAdmin()` | 要求 `is_system_admin == true` | 系统级操作（SSO 配置） |

### `is_system_admin` 说明

- 仅在系统初始化时设置（`/api/v1/setup/init`）
- 不能通过用户管理界面修改（设计如此）
- 绕过所有 IAM 权限检查
- 用于不属于 IAM 资源类型的系统级操作

### 影响统计

- **37 个文件变更**
- **~350 个路由 handler 闭包**简化为直接中间件链
- **93 个 workspace 路由**添加双 scope 权限支持
- **6 个安全漏洞**修复（secrets、CMDB、AI、user、notification、manifest）
- **零残留引用**：`BypassIAMForAdmin`、`RequireRole`、`c.Get("role")` 已完全移除
