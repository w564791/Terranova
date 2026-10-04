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

6. 部署覆盖值脱敏（详见 design spec §8.4「覆盖值」）：
   - 部署详情/列表与任务详情/列表不再返回 `variable_overrides`，改为返回 `overrides: [{key, sensitive, has_value, value?}]`；
   - 敏感 key 永不带值，非敏感值需要目标 workspace 的 `WORKSPACE_VARIABLES` READ；
   - `sensitive_keys` 为 NULL 时全部按敏感处理；
   - 敏感标记是粘滞的，启动时回填 deployment 行；
   - 按部署的 variable-preview 会合并已存覆盖。
7. manifest 各 handler 与 varset 控制器的 500 响应统一走 `c.Error` + 全局 `ErrorHandler`：响应体为 `{error:"internal error", request_id}`，不含 SQL 文本；`X-Request-ID` 只复用符合 `^[A-Za-z0-9-]{8,64}$` 的值，否则生成 UUID。

8. install / upgrade 的变量变更需要目标 workspace 的 `WORKSPACE_VARIABLES` WRITE（403），包括：
   - 覆盖或 `unset_keys` 非空；
   - varset 列表按集合、priority 与同级顺序比较后有变化（原样回传不算；`varsets` 缺省 = 保持不变，不算变化）；
   - 首装时带 varset。

   只换版本仍只需 `WORKSPACE_RESOURCES` WRITE。`GET /workspaces?capability=` 的每一项带 `can_write_variables`。CORS 增加 `Access-Control-Expose-Headers: X-Request-ID`，`X-Request-ID` 也加入允许的请求头。

9. 版本 bundle 不可变（feat/manifest-sandbox step 3）：发布违反 bundle 规则（`.tfvars` / state / `.git/` / `.env` / 私钥文件名 / 凭证内容扫描等）→ 422 `bundle_rules_violated`，`problems: [{file, line?, rule, message}]`，不回显任何文件内容；无合法 bundle 的版本 install / 预览 / upgrade 目标 → 409 `bundle_republish_required`（uninstall 不受限，可从无效版本升级到合法版本）。完整性只在这些使用处与执行器取文件时校验，读接口不重算、不写库；篡改 → 409 + 粘滞的 `hash_mismatch` + WARN 安全日志 / 审计（manifest_id、version_id、request_id），迁移也不会按篡改后的内容重算。细节见 sandbox spec §3.3、design spec §8.4。

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
backend/controllers/workspace_task_controller.go
backend/controllers/varset_variable_controller.go
backend/services/manifest_overrides.go
backend/internal/handlers/manifest_{editor,files,provider_schema,versions}_handler.go (500 → c.Error)
backend/internal/middleware/middleware.go
backend/internal/models/manifest_v2.go
backend/main.go
backend/internal/manifestbundle/{rules,source}.go（step 3）
backend/internal/migration/manifest_bundle_v2.go + backend/migrations/add_manifest_bundle_v2.sql（step 3）
```

### 未完成项（2026-10-04 暂停时记录，合并 `feat/manifest-sandbox` 前必须处理）

**上生产前必须完成（security 阻塞项）**
1. 执行器拦截哈希为 NULL 的版本：在执行器取文件、校验哈希的那一处，`bundle_hash IS NULL` 时只放行 uninstall 发起的 destroy，其余任务失败，报错 `bundle_republish_required: <reason>`。
   - 因违反规则被置 NULL 的版本：解包后先删掉命中黑名单的文件（复用 `manifestbundle` 的黑名单函数，包括 `.terraformrc`、`terraform.rc`、`*.tfvars`、`*.tfstate*`、`.terraform/`），再强制把 `TF_CLI_CONFIG_FILE` 指向平台自己的文件，然后执行 destroy。
   - `hash_mismatch` 的版本：不自动 destroy。uninstall 返回 409 `{code:"untrusted_bundle_confirm_required", reason:"hash_mismatch"}`；带 `force_destroy_untrusted: true` 且是 org MANIFESTS ADMIN 才放行，非管理员返回 403；确认动作写审计（操作者、manifest_id、version_id、request_id）。前端用这个单独的 code 弹确认框。
   - 测试：违反规则的 NULL 版本跑 plan 失败、跑 destroy 通过，且 destroy 用的不是 bundle 里的 `.terraformrc`；`hash_mismatch` 版本不确认就不能 uninstall，非管理员确认返回 403，管理员确认后写入审计。
2. `MaxFiles = 2000`（`too_many_files`）已在本次暂停前的提交里完成。

**第 4 步（runner）**
- 解包：每个条目都要过 `manifestbundle.ValidatePath`；条目类型只接受普通文件和目录，链接、设备文件一律拒绝；写盘用 `O_EXCL|O_NOFOLLOW`；解包过程中累计计算大小和文件数（`MaxFileSize`、`MaxBundleSize`、`MaxFiles`），超限立刻停止。
- 解完后用 `manifestbundle.Hash` 重算，与 `bundle_hash` 一致才能 `init`；拒绝 NULL 哈希的版本；对不上按 `hash_mismatch` 处理（记录原因、WARN 审计）。
- `RemoteDataAccessor.SetVariableOverrides` 目前什么都不做，agent 模式会丢掉 override：改为通过 agent 已有的任务数据通道下发，不能走环境变量或日志。
- agent、本地、sandbox 三种 runner 共用一个 tfvars 生成函数，加一条测试保证输出逐字节一致。
- 审批 run 要拿锁；plan 先脱敏再存储，然后计算 `plan_hash`。

**第 5 步（run token）**
- `run_tokens` 带 `session_id`；token 过期时间取 run 超时和 session 过期里较早的那个。
- JWT 加 `typ` claim（`task`/`run`）；`run` 路径在数据库校验出错时拒绝，`task` 路径保持现有行为。
- `preview` token 拒绝 POST/LOCK/UNLOCK。
- run 结束只撤销这个 run 自己的 token；session 结束或过期时，统一回收它的所有 token 和 STS 凭证。

**第 6 步**：AgentCore 只支持 VPC 模式，代码里拒绝 Sandbox 和 Public 模式；创建 session 时检查 `WORKSPACE_STATE` READ 和 plan 权限。

**第 7 步**：审批只接受 `purpose=approval`、`runner=agent` 的 run；apply 之前 agent 要核对 `approved_bundle_hash` 和 `approved_plan_hash`；拒绝 NULL 哈希的版本。

**第 8 步（git 来源）**
- 私有 module 的 `ref` 必须是 commit SHA（或者 vendor 进来）。
- git token 按 run 现签：GitHub App installation token，权限为单仓库 `contents:read`，有效期约 1 小时，不落库；通过 `GIT_ASKPASS` 注入，不能拼进 URL。
- 发布时由平台拉取仓库，并固定到某个 SHA；webhook 要验签。

**其他**
- `variable_sets` 表加 `org_id` 列，改为直接按列绑定组织。
- 跑过已删除迁移 `20261004_03_manifest_bundle_rules` 的开发库和测试库需要重建（`20261004_04` 不会修复它们）。
