# 14 — Manifest 可视化编排 ✅ 已修复（feat/manifest-iam）

> 源文件: `backend/internal/router/router_manifest.go`
> API 数量: 34（`router_manifest_permissions_test.go` 校验路由总数与每条路由的权限）
> 资源类型: `MANIFESTS`（ORGANIZATION 级，迁移 `20261004_01_manifest_iam_resource`）

## 全部 API 列表

前缀 `M = /api/v1/organizations/:org_id/manifests`

| # | Method | Path | 认证 | 路由层授权 | handler 内追加校验 | 状态 |
|---|--------|------|------|-----------|-------------------|------|
| 1 | GET | M | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 2 | POST | M | JWT | MANIFESTS/ORG/WRITE | — | ✅ |
| 3 | GET | M/:id | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 4 | PUT | M/:id | JWT | MANIFESTS/ORG/WRITE | status 进出 archived 需 MANIFESTS ADMIN | ✅ |
| 5 | DELETE | M/:id | JWT | MANIFESTS/ORG/ADMIN | — | ✅ |
| 6 | GET | M/:id/export-zip | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 7 | GET | M/:id/provider-schemas | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 8 | GET | M/:id/files | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 9 | GET | M/:id/files/*path | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 10 | PUT | M/:id/files/*path | JWT | MANIFESTS/ORG/WRITE | — | ✅ |
| 11 | DELETE | M/:id/files/*path | JWT | MANIFESTS/ORG/WRITE | — | ✅ |
| 12 | POST | M/:id/files/_move | JWT | MANIFESTS/ORG/WRITE | — | ✅ |
| 13 | POST | M/:id/files/_move_dir | JWT | MANIFESTS/ORG/WRITE | — | ✅ |
| 14 | POST | M/:id/files/_delete_dir | JWT | MANIFESTS/ORG/WRITE | — | ✅ |
| 15 | POST | M/:id/draft/_reset_from | JWT | MANIFESTS/ORG/WRITE | — | ✅ |
| 16 | POST | M/:id/draft/_export | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 17 | GET | M/:id/v2/versions | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 18 | GET | M/:id/v2/versions/:version_id | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 19 | POST | M/:id/v2/versions | JWT | MANIFESTS/ORG/WRITE | — | ✅ |
| 20 | GET | M/:id/v2/versions/:version_id/diff | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 21 | GET | M/:id/v2/versions/:version_id/workdirs | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 22 | GET | M/:id/v2/draft/diff | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 23 | POST | M/:id/v2/versions/:version_id/files/_export | JWT | MANIFESTS/ORG/READ | — | ✅ |
| 24 | GET | M/:id/v2/deployments | JWT | MANIFESTS/ORG/READ + RequireWorkspaceListAccess | manifest 属于 auth org；按可读 workspace 过滤（无可读 → 403） | ✅ |
| 25 | GET | M/:id/v2/deployments/:deployment_id | JWT | MANIFESTS/ORG/READ | manifest 属于 auth org；目标 workspace 可读 | ✅ |
| 26 | POST | M/:id/v2/deployments/install | JWT | MANIFESTS/ORG/READ | 目标 ws WORKSPACE_RESOURCES WRITE；varset 必须可挂载 | ✅ |
| 27 | POST | M/:id/v2/deployments/:deployment_id/upgrade | JWT | MANIFESTS/ORG/READ | 目标 ws WORKSPACE_RESOURCES WRITE；varset 必须可挂载 | ✅ |
| 28 | POST | M/:id/v2/deployments/:deployment_id/uninstall | JWT | MANIFESTS/ORG/READ | 目标 ws WORKSPACE_RESOURCES WRITE | ✅ |
| 29 | POST | M/:id/v2/deployments/:deployment_id/variable-preview | JWT | MANIFESTS/ORG/READ | 目标 ws WORKSPACE_VARIABLES READ；sensitive 值置空；varset 必须可挂载 | ✅ |
| 30 | GET | /api/v1/variable-sets/:varset_id/manifest-deployments | JWT | VARIABLE_SETS/ORG/READ | — | ✅ |
| 31 | GET | /api/v1/workspaces/:id/manifest-summary | JWT | WORKSPACES/ORG/READ 或 WORKSPACE_MANAGEMENT/WS/READ | —（未改动） | ✅ |
| 32 | GET | /api/v1/manifest-editor/modules | JWT | MODULES/ORG/READ | — | ✅ |
| 33 | GET | /api/v1/manifest-editor/modules/:module_id/demos | JWT | MODULES/ORG/READ | — | ✅ |
| 34 | GET | /api/v1/manifest-editor/modules/:module_id/inputs | JWT | MODULES/ORG/READ | — | ✅ |

相关（非本文件路由）：
- `GET /api/v1/workspaces?capability=RESOURCE:LEVEL` — 部署目标选择器，未知值 400。
- `GET /api/v1/variable-sets?workspace_id=` — 仅返回该 workspace 可挂载的变量集，需能读该 workspace。

## 修复说明

### 根因
manifest 路由原先以 `SYSTEM_SETTINGS` 作为临时权限，且 `MANIFESTS` 资源类型未注册；部署接口只检查 `WORKSPACE_MANAGEMENT`，变量预览会把 workspace/任意 varset 的敏感值返回给 MANIFESTS READ 用户。

### 已做
1. 注册 `MANIFESTS`（valueobject + swagger enum + 迁移 `20261004_01_manifest_iam_resource` / `migrations/add_manifest_iam_permissions.sql` / `manifests/db/init_seed_data.sql`）；内置角色 admin/org_admin ADMIN，developer/viewer/user READ。同时补注册缺失的 `VARIABLE_SETS`。
2. 每条路由使用现有 `RequirePermission` / `RequireAnyPermission`（见上表）。
3. 部署写操作在 handler 内对目标 workspace 检查 `WORKSPACE_RESOURCES` WRITE（`RequireWorkspaceResourcePermission`，与 `RequireWorkspacePermission` 同一实现）。
4. 部署列表复用 `RequireWorkspaceListAccess` 服务端过滤。
5. 部署接口校验 manifest 属于认证 org。

### 遗留
- files / versions / provider-schemas 等 v2 handler 仍未把 manifest 绑定到 org_id（跨 org 依赖路由层 org 权限，handler 内未做 404）。
- variable_sets 表无 org_id（既有租户隔离缺口）。

### 修改文件
```
backend/internal/router/router_manifest.go
backend/internal/router/router_variable_set.go
backend/internal/domain/valueobject/resource_type.go
backend/internal/migration/manifests_iam.go (+ runner.go)
backend/migrations/add_manifest_iam_permissions.sql
manifests/db/init_seed_data.sql
backend/internal/middleware/iam_permission.go
backend/internal/application/service/workspace_list_access.go
backend/internal/handlers/manifest_handler.go
backend/internal/handlers/manifest_deployments_v2_handler.go
backend/services/variable_resolution_service.go
backend/services/variable_set_service.go
backend/controllers/variable_set_controller.go
backend/controllers/workspace_controller.go (swagger 注释)
```
