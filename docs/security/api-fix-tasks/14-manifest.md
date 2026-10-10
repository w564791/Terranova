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

10. 执行器闸门与发布 HCL 检查（feat/manifest-sandbox）：执行器取文件处 `bundle_hash IS NULL`（规则违规或 `hash_mismatch`）一律拒绝，任务失败报 `bundle_republish_required: <reason>`；uninstall 仍只解绑元信息（之后的 Plan+Apply 不加载 bundle），不需确认。native 发布新增 HCL 静态检查（422 同一 problem 形状，带行号）：`hcl_parse_error`、`hcl_provisioner`、`hcl_external_data`、`hcl_http_data`、`hcl_module_source`；module source 白名单 = bundle 内相对路径 + 平台 module 目录中 active module 的 `module_source`（`manifestbundle.PublishModuleSourcePolicy`）。只在发布时检查，不追溯已有合法版本。细节见 sandbox spec §3.3。

11. 执行器 provider 安装（step 4 前置）：`terraform init` 从不加 `-upgrade`，provider 与 `.terraform.lock.hcl` 不符（版本约束或 checksum）即失败、不重试；manifest bundle 自带的 lock 优先于 workspace 已存 lock（不再被覆盖）；workspace provider 配置被修改后（`provider_config_hash != last_init_hash`）本次不恢复已存 lock、按新约束重新解析并保存新 lock。插件缓存改为按任务私有目录（`<workDir>/.terranova-plugin-cache`，首次尝试时清空重建，随工作目录删除），进程环境与 workspace 变量里的 `TF_PLUGIN_CACHE_DIR`、`TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE`、`TF_CLI_ARGS_init` 一律忽略；不再写工作目录 `.terraformrc`（Terraform 从未读取它，且其中的 `plugin_cache_may_break_dependency_lock_file = true` 会绕过 lock 校验），也不设置 `TF_CLI_CONFIG_FILE`。任务失败带结构化 `error_code`（`workspace_tasks.error_code`，迁移 `20261010_01_workspace_task_error_code`），bundle 闸门失败为 `bundle_republish_required`，任务详情/列表输出；agent 上报只接受已知码。

12. Runner 接口与 bundle 交接（step 4）：任务投递统一经 `Runner`（local / agent+K8s / sandbox 占位），approval（`plan_and_apply`、`apply`）必须持 workspace advisory lock 才能投递，`ExecuteConfirmedApply` 补上了锁；approval 的 plan 持 state 锁（只有 preview 用 `-lock=false`）。`pglock` 每把锁独占连接（修复会话级锁在连接池上泄漏 / 重入）。manifest 文件改为「归档 + `bundle_hash`」交接，`manifestbundle.Unpack` 逐条校验路径与类型、`O_EXCL|O_NOFOLLOW` 写盘、累计限额即时中止、落盘后重算哈希一致才进入 `init`。agent / K8s 经 `GetTaskData` 收到 manifest 三列、已校验 bundle（或拒绝原因）和 override 快照：修复 agent 模式把 manifest workspace 当 UI workspace 生成空配置、以及 override 在 agent 模式被丢弃的问题。tfvars / `variables.tf.json` 生成由各 runner 共用，override 不再把敏感变量降为非敏感。细节见 sandbox spec §3.4。

13. plan 脱敏与编辑器 Run 校验（step 4）：`plan_json` 入库前脱敏（`RedactPlanJSON`，按 plan 自带的敏感标记 + provider 配置常量），local 保存、agent 上传（平台侧再脱敏）、plan parser 回退路径一致；脱敏后的 plan 的哈希由 `RedactedPlanHash` 计算（供 `manifest_runs.plan_hash`）。历史 `plan_json` 未回填脱敏，`plan_data`（apply 需要的二进制 plan）仍含明文敏感值。编辑器 Run 的 `external_files` 建任务时校验：草稿走发布规则（422），带 `manifest_version_id` 时校验版本并比对 `bundle_hash`（409），固化 `bundle_hash` 后执行时复核。细节见 sandbox spec §3.4。

14. 变量值文件注入（tfvars）：原 `variables.tfvars` 只转义 `"` 与换行，`\"` 可闭合字符串注入其它赋值，`${` / `%{` 会被当模板，HCL 格式值原样写入（可带第二个赋值）。现所有 runner 共用 `RenderTFVars` 生成 `terranova.auto.tfvars.json`（`encoding/json`，Terraform 自动加载，去掉 `-var-file`）：JSON 变量文件无求值上下文，字符串一律字面量；HCL 格式的 object / list / bool / number 用 hclsyntax 解析为单个表达式、无上下文求值（与 Terraform 读 .tfvars 相同：不允许变量与函数）后转为 JSON 值，否则拒绝（`ErrInvalidTFVar`，报错只含变量名）；变量名必须是标识符，重复、非法 UTF-8 拒绝。HCL object / list 变量在 `variables.tf.json` 声明为 `any`（原先声明 `string` 导致此类值本来就无法使用）。日志按值脱敏（`RenderTFVarsMasked`）。用户 bundle 禁止 `*.tfvars(.json)`，自动加载不会与用户文件冲突。

15. plan 脱敏并上平台侧敏感集合（`PlanSensitivity`：workspace / varset 敏感变量、deployment override 的 `sensitive_keys`，NULL = 全部敏感）：按变量名脱敏 `variables` 与默认值，按值替换 plan 中等于 / 包含敏感值的字符串叶子；执行器与 agent 上传（`PlanSensitivityForTask`）使用同一集合。标记统一 `(sensitive value)`。

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
backend/internal/manifestbundle/hcl.go、backend/services/local_data_accessor.go（第 10 条）
backend/internal/manifestbundle/archive.go、backend/services/{runner,manifest_handoff,task_variables}.go、backend/internal/pglock/advisory_lock.go、backend/internal/handlers/agent_handler.go（第 12 条）
backend/services/{plan_redaction,manifest_run_files}.go、backend/controllers/workspace_task_controller.go（第 13 条）
```

### 未完成项（2026-10-04 暂停时记录，合并 `feat/manifest-sandbox` 前必须处理）

**上生产前必须完成（security 阻塞项）**
1. ~~执行器拦截哈希为 NULL 的版本~~ —— 已完成（见上文第 10 条）。原计划的「uninstall 发起的 sanitized destroy / `force_destroy_untrusted` 确认」未实施：核实后 uninstall 只解绑元信息、不创建 destroy 任务，之后的 Plan+Apply 不加载 bundle，bundle 内容不会被执行，因此执行器对所有 NULL 版本一律拒绝即可，不需要放行例外、确认流程和 `workspace_tasks` 新列。若以后改为「uninstall 直接带 bundle 跑 destroy」，需重新设计：先删黑名单文件、平台自有 `TF_CLI_CONFIG_FILE`（`provider_installation` 只含 `network_mirror`；仓库目前没有 provider mirror 配置，在有之前所有不可信 destroy 都必须管理员确认）、`CheckHCL(..., LocalModulesOnly)` 命中或 `hash_mismatch` 需 org MANIFESTS ADMIN 确认并在执行时复核。
2. `MaxFiles = 2000`（`too_many_files`）已在本次暂停前的提交里完成。

**第 4 步（runner）**
- ~~解包、哈希复核、override 下发、共用 tfvars、审批 run 拿锁、plan 先脱敏再算哈希~~ —— 已完成（第 12、13 条）；`manifest_runs` 的写入（`plan_redacted` / `plan_hash`）随 step 6/7 的 run 记录落地。以下为原始要求：
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
