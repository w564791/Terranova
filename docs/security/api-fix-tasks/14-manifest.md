# 14 — Manifest 可视化编排 ✅ 已修复（feat/manifest-iam）

> 源文件: `backend/internal/router/router_manifest.go`
> API 数量: 35（`router_manifest_permissions_test.go` 校验路由总数与每条路由的权限）
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
| 25b | POST | M/:id/v2/deployments/variable-preview | JWT | MANIFESTS/ORG/READ | 首装预览:workspace 属于本 org、version 已发布且属于本 manifest(否则 404);目标 ws WORKSPACE_VARIABLES READ;sensitive 值置空;varset 必须可挂载 | ✅ |
| 26 | POST | M/:id/v2/deployments/install | JWT | MANIFESTS/ORG/READ | 目标 ws WORKSPACE_RESOURCES WRITE；varset 必须可挂载；workspace 属于本 org、version 已发布且属于本 manifest(否则 404) | ✅ |
| 27 | POST | M/:id/v2/deployments/:deployment_id/upgrade | JWT | MANIFESTS/ORG/READ | 目标 ws WORKSPACE_RESOURCES WRITE；varset 必须可挂载 | ✅ |
| 28 | POST | M/:id/v2/deployments/:deployment_id/uninstall | JWT | MANIFESTS/ORG/READ | 目标 ws WORKSPACE_RESOURCES WRITE | ✅ |
| 29 | POST | M/:id/v2/deployments/:deployment_id/variable-preview | JWT | MANIFESTS/ORG/READ | 目标 ws WORKSPACE_VARIABLES READ；sensitive 值置空；varset 必须可挂载 | ✅ |
| 30 | GET | /api/v1/variable-sets/:varset_id/manifest-deployments | JWT | VARIABLE_SETS/ORG/READ + varset 组织可见 | — | ✅ |
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
5. `/organizations/:org_id/manifests/:id/*` 全部 24 条路由在各自 `RequirePermission` 之后挂 `ManifestInAuthOrg`（与部署 handler 的 `manifestInAuthOrg` 同一实现）：path manifest 不属于 path org 或不存在 => 404；无权限者先得到 403，无法用 404/403 探测 ID。`/:id` CRUD 与 export-zip 由 handler 按 organization_id 过滤。

### 遗留
- variable_sets 表无 org_id，组织归属按分配关系推导（`VariableSetService`）：
  - `GET /variable-sets` 列表（`ListForOrg`）与按 ID 的 `/variable-sets/:varset_id/...` 全部 12 条路由及上表 #30 共用同一可见规则 `VarsetVisibleInOrg`：global；分配到本组织 workspace/project；尚无分配且由调用者创建。守卫放在 `RequirePermission` 之后（与 manifest 路由同一 `manifestRouteChain`），不可见 → 404。
  - 写路由（更新、改 scope、删除、变量增改删、创建/删除分配）另需 `VarsetWritableInOrg`：已有分配全部在本组织内，或未分配且由调用者创建，或 global 且调用者为平台超管；任一分配在其他组织即只读 → 403；org 管理员可见 global 但写 → 403。
  - 分配目标：创建分配时 workspace 须 `EnsureWorkspaceInOrg`、project 须属于本组织（否则 404），并需目标 `WORKSPACE_VARIABLES` WRITE（workspace 级；project 目标按 PROJECT 作用域检查，即项目/组织级授权，单个 workspace 的授权不够）→ 否则 403；删除分配按分配记录上的目标做同样校验。改 scope 为 global 仅平台超管（403）。
  - 后续：给 variable_sets 加 org_id 列后改为直接按列绑定（推导规则下无创建者的未分配变量集对所有人不可见）。

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
