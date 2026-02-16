# Refactoring Summary: users.role -> IAM Permission System

## Background

The legacy `users.role` field (string: "admin"/"user") was used throughout the codebase for access control. This created several problems:

1. Role changes required re-login (role was baked into JWT claims)
2. Only two levels of access (admin/user), no fine-grained permissions
3. Redundant with the IAM permission system already in place
4. `BypassIAMForAdmin()` middleware bypassed all IAM checks for admin users, defeating the purpose of IAM

## Goal

Replace all `users.role` based access control with:
- `is_system_admin` boolean (for system-level operations like initial setup, SSO config)
- IAM `RequirePermission` / `RequireAnyPermission` middleware (for all other access control)

## Changes by Phase

### Phase 1: Middleware Foundation
- **`middleware/iam_permission.go`**: Added `is_system_admin` bypass at the top of both `RequirePermission` and `RequireAnyPermission` - system admins pass through without IAM grant checks
- **`middleware/middleware.go`**: JWTAuth now queries DB for `is_system_admin` on every request (both `login_token` and `user_token` paths), ensuring permission changes take effect immediately without re-login

### Phase 2: Remove BypassIAMForAdmin
- **`router/router.go`**: Removed `middleware.BypassIAMForAdmin()` from the protected route group. All routes now go through IAM permission checks (with system admin bypass built into the IAM middleware itself)

### Phase 3: Refactor Router Handler Closures (12 files)
Replaced the old closure pattern:
```go
// OLD: closure with role check
workspaces.GET("/path", func(c *gin.Context) {
    role, _ := c.Get("role")
    if role == "admin" {
        handler(c)
        return
    }
    iamMiddleware.RequireAnyPermission(...)(c)
    if !c.IsAborted() {
        handler(c)
    }
})
```
With the clean middleware chain:
```go
// NEW: direct middleware chain
workspaces.GET("/path",
    iamMiddleware.RequireAnyPermission(...),
    handler,
)
```

**Files refactored:**
- `router_agent.go`
- `router_ai.go` (also replaced `BypassIAMForAdmin()` with `RequirePermission("AI_ANALYSIS", "ORGANIZATION", "ADMIN")`)
- `router_demo.go`
- `router_global.go`
- `router_iam.go`
- `router_module.go`
- `router_project.go`
- `router_run_task.go`
- `router_schema.go`
- `router_task.go`
- `router_user.go`
- `router_workspace.go` (including all 8 helper functions: project, run task, output, notification, run trigger, drift, remote data routes)

### Phase 4-5: Handler-Level Role Checks and Setup
- **`handlers/setup.go`**: `InitAdmin` now sets `is_system_admin = true` instead of `role = "admin"`; status check uses `is_system_admin`
- **`handlers/permission_handler.go`**: Admin checks use `c.Get("is_system_admin")` instead of `c.Get("role")`
- **`handlers/user_handler.go`**: Removed `role` from user response

### Phase 6-7: Auth/JWT and Middleware Cleanup
- **`handlers/auth.go`**: JWT generation no longer includes `role` claim; `GetMe` returns `is_system_admin` instead of `role`; login response includes `is_system_admin`
- **`handlers/mfa_handler.go`**: MFA setup/verify uses `is_system_admin` instead of `role` in JWT generation
- **`middleware/middleware.go`**: Added `RequireSystemAdmin()` middleware for system-level operations (SSO config)
- Removed all `BypassIAMForAdmin()` and `RequireRole()` middleware functions

### Phase 8-9: User Model/Service and SSO Cleanup
- **`models/user.go`**: `Role` field marked as deprecated with `json:"-"` (hidden from API responses), kept for DB compatibility; `IsSystemAdmin` field is the active field
- **`application/service/user_service.go`**: User creation/update no longer sets `role`; admin operations use `is_system_admin`
- **`services/sso/sso_service.go`**: Removed `DefaultRole` from SSO user creation; SSO users are regular users by default
- **`services/ai_cmdb_service.go`**: Removed role reference

### Phase 10: Frontend Changes
- **`store/slices/authSlice.ts`**: Auth state uses `is_system_admin` boolean instead of `role` string
- **`components/ProtectedRoute.tsx`**: Admin route guard checks `user.is_system_admin` instead of `user.role === "admin"`
- **`components/Layout.tsx`**: Admin menu visibility uses `is_system_admin`
- **`pages/admin/UserManagement.tsx`**: User management UI uses `is_system_admin` for display and editing
- **`pages/admin/PermissionManagement.tsx`**: Permission page guard uses `is_system_admin`
- **`pages/CMDB.tsx`**: Admin feature visibility uses `is_system_admin`

### SSO Router Fix
- **`router/router_sso.go`**: Replaced `middleware.RequireRole("admin")` with `middleware.RequireSystemAdmin()` for SSO admin routes

### Security Fix: Notification and Manifest Routes
In the original code, `BypassIAMForAdmin()` was actually an **admin-only guard** (not just a bypass) - it let admins through and **rejected all non-admins with 403**. Two route files that were behind this guard had no IAM checks of their own:

- **`router/router_notification.go`**: Added `iamMiddleware` parameter; all routes now use `SYSTEM_SETTINGS` IAM permission (READ for queries, WRITE for mutations, ADMIN for deletes)
- **`router/router_manifest.go`**: Added `iamMiddleware` parameter; all routes now use `SYSTEM_SETTINGS` IAM permission
- **`router/router.go`**: Updated callers to pass `iamMiddleware`

## Architecture After Refactoring

### Access Control Model
```
Request -> JWTAuth (sets user_id, is_system_admin from DB)
        -> AuditLogger
        -> IAM Permission Check:
             - is_system_admin == true? -> ALLOW (bypass)
             - Check IAM grants in DB  -> ALLOW/DENY
```

### Key Middleware
| Middleware | Purpose | Usage |
|---|---|---|
| `JWTAuth()` | Authentication, sets `user_id` and `is_system_admin` | All authenticated routes |
| `RequirePermission(resource, scope, level)` | Single IAM permission check with admin bypass | Routes needing specific permission |
| `RequireAnyPermission([]PermissionRequirement)` | OR-logic IAM check with admin bypass | Routes accepting multiple permission paths |
| `RequireSystemAdmin()` | Requires `is_system_admin == true` | System-level operations (SSO config) |

### What `is_system_admin` Means
- Set only during system initialization (`/api/v1/setup/init`)
- Cannot be changed through normal user management (by design)
- Bypasses all IAM permission checks
- Used for system-level operations that don't map to IAM resource types

### Dual-Scope Permission Pattern (ORGANIZATION + WORKSPACE)
The IAM permission checker matches **exact resource types** - `WORKSPACES` (ORGANIZATION scope) and `WORKSPACE_MANAGEMENT` (WORKSPACE scope) are different resource types with no cross-type inheritance. An org_admin with `WORKSPACES` at ORGANIZATION scope would get 403 on routes that only check WORKSPACE-scope types.

**Fix applied to all workspace routes:** Every `RequireAnyPermission` block now includes `{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "..."}` as its first entry, alongside the fine-grained WORKSPACE-scope permissions.

**Routes fixed:**
- `router_workspace.go`: 93 `RequireAnyPermission` blocks (main function + 8 helper functions)
- `router_agent.go`: 3 routes in `setupWorkspaceAgentRoutes`
- `router_ai.go`: 6 embedding routes (workspace group)

### Resource Type Corrections
- `router_ai.go`: Invalid `"WORKSPACE"` (singular) resource type changed to `"WORKSPACE_MANAGEMENT"`
- `router_cmdb.go`: Invalid `"cmdb"` (lowercase) resource type changed to `"SYSTEM_SETTINGS"`

### Frontend Permission-Based Navigation
- `components/Layout.tsx`: Navigation menu items now use IAM permission checks (`IAM_PERMISSIONS`, `SYSTEM_SETTINGS`, `WORKSPACES`, `MODULES`, `ORGANIZATION`) instead of hardcoded `requireAdmin: true`. Non-system-admin users see menu items based on their actual IAM permissions.
- `components/ProtectedRoute.tsx`: Removed `/admin/*` hard block - backend handles all access control via IAM middleware.

## Stats
- **34+ files changed** (including notification, manifest, and dual-scope permission fixes)
- **~350 route handler closures** simplified to direct middleware chains
- **93 workspace route permission blocks** updated with ORGANIZATION-scope fallbacks
- **Zero remaining references** to `BypassIAMForAdmin`, `RequireRole`, or `c.Get("role")` in active code
- **All routes have explicit IAM permission checks** - no route relies solely on the removed `BypassIAMForAdmin` guard
- **All workspace routes support dual-scope permissions** - org_admin users can access workspace features
