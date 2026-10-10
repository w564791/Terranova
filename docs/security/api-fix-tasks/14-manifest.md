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

16. `plan_data`（apply 用的二进制 plan，含明文敏感值）静态加密：信封加密（`internal/crypto` `SealPlanData`：每个 plan 随机 256 位数据密钥 AES-256-GCM，数据密钥由 `DATA_ENCRYPTION_KEY` 经 HMAC 派生的 KEK 包裹（信封 v2，头部记录 `key_version`；v1 为旧格式，KEK 由 `JWT_SECRET` 派生，仅解密，`CleanupPlanData` 以原过期时间 CAS 重新加密为 v2，见 `docs/security/signing-and-encryption-keys.md`）；头部（含 key_version）与任务 ID 作为 AAD，换任务 / 改过期时间均无法解开）。只有执行路径解密：local 执行器 apply 恢复 plan、plan parser 回退、agent plan-task 接口（交给执行该任务的 agent）；`WorkspaceTask.PlanData` 为 `json:"-"`，其余接口不返回（`TestPlanData_NeverSerialized`）。生命周期：apply 成功立即删除（local 执行器；agent 在终态上报时由平台删除），任何终态删除，过期删除（`PLAN_DATA_TTL`，默认 7 天，过期头部经认证；过期后 apply 失败，`error_code = plan_expired`，需重新 plan）。清理任务 `CleanupPlanData` 在 leader 启动时（恢复 pending 任务之前）执行一次、之后每 10 分钟：终态与过期删除，仍可能 apply 的旧明文行原地加密（需要应用内主密钥，SQL 迁移做不到；终态旧行无人读取，直接删除而非加密）。

17. 历史 `plan_json` 回填脱敏：迁移 `20261010_02_workspace_task_plan_json_redaction` 只加标记列 `plan_json_redaction_version`；回填 `services.BackfillPlanJSONRedaction` 在应用内执行（需要平台侧敏感集合与解密后的变量值），leader 启动后后台按 id 分批运行，用同一 `RedactPlanJSON` + 任务的 `PlanSensitivityForTask`（尽力而为：快照 / override 快照；无快照时用当前变量）覆盖原值。可重复执行：已达当前版本的行跳过（版本号升级即全部重跑），每行按读到的 plan_json 做 compare-and-set，与执行器并发写不冲突。`workspace_task_resource_changes` 中旧 agent 上传的变更明细不在本回填范围内。

18. 旧 agent 拒绝执行 manifest 绑定任务 + 任务 `error_reason`：agent 注册上报 `version`（构建 commit，≤50 字符）与 `capabilities`（`manifest_bundle_v1`：按 manifest 版本 bundle 取文件并校验；`task_data_overrides_v1`：task-data 携带变量 override / sensitive_keys）。平台只保存已知能力，未上报（旧版本）存 `[]`，重新注册覆盖。需要能力的任务：workspace 绑定 manifest 部署、Run 任务（external files）、带变量 override 的任务。派发时跳过缺能力的 agent；池中已连接的 agent 全部缺能力（或 K8s 预选的 agent 缺能力）时，CAS 置任务为 failed，`error_code = agent_upgrade_required`，`error_reason` = 第一个缺失能力，`error_message = "agent_upgrade_required: no agent in pool <pool> supports ..."`；有具备能力但忙的 agent 时照常重试。非 manifest 任务派发逻辑不变。`error_reason`（迁移 `20261010_03_workspace_task_error_reason`，varchar(64)）只存规则 / 能力名（`denylisted_file`、`hash_mismatch`、`no_valid_bundle`、`plan_data_expired`、`manifest_bundle_v1` 等），不含路径或内容；agent 上报时仅在 error_code 为已知码且 reason 为 token 时保存；任务列表与详情与 `error_code` 一起返回。执行器侧（agent 或 local）交接校验失败（收到的 bundle 哈希不等于 bundle_hash，`manifestbundle.ErrIntegrity`）归类为 `error_code = bundle_republish_required`、`error_reason = hash_mismatch`，经状态上报到平台；平台记 WARN 安全日志，但不据 agent 上报标记版本（agent 不可信；平台每次交接前已用 `VerifyForUse` 重算存储文件）。

19. 资源变更由平台推导 + agent 任务归属校验：
    - `workspace_task_resource_changes` 只由平台从脱敏后的 `plan_json` 推导（`PlanParserService.StoreResourceChangesFromPlanJSON`，推导前再次按任务平台敏感集合 `RedactPlanJSON`，幂等）：local 执行器、agent 上传 plan_json 时（`UploadPlanJSON`）都走这条路径。新 agent 不再上传资源变更。`parse-plan-changes` 对上传的 `resource_changes` 接受并丢弃（不拒绝：旧的非 manifest agent 仍会调用，拒绝只会产生告警，不带来安全收益）：有 plan_json 时重新推导；没有 plan_json（旧 agent 的 plan 上传失败）时只保存 address / type / name / module / action，before / after / after_unknown 为 NULL——不用平台敏感集合"脱敏后保存"，因为 agent 上传的值没有 before_sensitive / after_sensitive，provider 标记的敏感属性无法识别。
    - 回填 `BackfillResourceChangeRedaction`（迁移 `20261010_04_workspace_task_resource_change_redaction`：标记列 `redaction_version`、`details_purged boolean DEFAULT false`）：leader 启动后在 plan_json 回填之后后台按 task 分批执行。任务有 plan_json：按 resource address 用推导值覆盖 before / after / after_unknown（保留 apply_status、resource_id 等）；无 plan_json 或 plan 中没有该地址：before / after / after_unknown 置 NULL，`details_purged = true`（只有这条路径设置），保留 address / action。表中没有 before_sensitive / after_sensitive 列。每个 UPDATE 复查标记，可重复执行，与新写入并发安全。`details_purged` 由资源变更 API 返回。
    - 归属：池令牌由池内所有 agent 共享，平台能验证的最强身份是池；agent 在 `X-Agent-ID` 头中带自己的 ID（新能力 `agent_identity_header_v1`，manifest 任务必需）。所有 agent 任务路由（`/agents/tasks/:task_id/*`）在原有池-workspace 校验后加 `middleware.RequireTaskAgent`：任务必须已分配给调用池中的 agent；带头时必须等于 `task.agent_id`；上报过该能力的 agent 不带头一律拒绝；不带头的旧 agent 只做池绑定。不符 → 403（前一层对池外 workspace 已返回 403，不隐藏存在性，统一用 403 表示"不是你的"）；状态不符 → 409。状态：task data / plan-task / plan-data / plan-json / logs 读取只允许 `running`；状态上报、日志分片、parse-plan-changes 允许 `running`，以及 agent 自己结束任务后的 15 分钟内（`apply_pending`、`planned_and_finished`、`success`、`applied`、`failed`、`cancelled`、`partial_success`），用于重试与最后的日志；非 running 时状态上报只接受与当前相同的状态（重试）。workspace 路由（lock / unlock、state max-version / temp / promote / cleanup、fields、lock-hcl、provider schema）加 `RequireWorkspaceAgentTask`：调用方必须是在该 workspace 上 running（或刚结束、宽限期内）任务的 agent。C&C WebSocket 的 `task_completed` / `task_failed` / `log_stream`（含 resource_status_update）只接受分配给该连接 agent 的任务，其他丢弃并记 WARN（30 秒缓存）。HTTP state backend 使用按任务签发的 state token（绑定任务 + 状态），不变。
      （`X-Agent-ID` 头与 `agent_identity_header_v1` 已由第 20 条的按 agent 签发的 token 取代。）

20. 按 agent 签发的 JWT（agent token）：
    - 池令牌只用于 `POST /api/v1/agents/register`。注册响应返回 `agent_token`（HS256，`keys.PurposeAgent` 派生密钥，header `kid = agent-v<n>`；claims `typ=agent`、`agent_id`、`pool_id`、`gen`、`iat/nbf/exp/jti`；有效期 `AGENT_TOKEN_TTL`，默认 15m，范围 1m–24h）与 `agent_token_expires_at`。agent 在剩余不足 1/3 时用 `POST /api/v1/agents/token`（只接受 agent token）续期；后台续期协程保证空闲时也不过期。
    - 19 条 agent 任务 / workspace 路由、heartbeat / unregister、pool secrets、terraform-versions 与 C&C WebSocket 都用 agent token 认证，agent_id 只取自 token；`X-Agent-ID` 头不再读取（能力 `agent_identity_header_v1` 删除，旧值在注册时被当作未知能力丢弃）。C&C 的 `agent_id` 查询参数可省略，带了必须等于 token 中的 agent（否则 403）。
    - 撤销机制（数据库校验 + 代际号，而不是黑名单 / 只靠短有效期）：每次使用 token 都在数据库校验 agent 行存在且属于 token 的池、`revoked_at IS NULL`、`token_generation = gen`，且注册时用的池令牌（`agents.pool_token_hash`）仍有效、未过期；查询出错一律拒绝（503）。理由：agent 路由本来就每次查库（任务归属），多一次按主键的 join 成本可忽略；撤销立即生效，不需要维护 jti 黑名单及其清理；注销（删除行）和过期清理自然失效；撤销池令牌即使该池令牌签出的所有 agent token 失效。管理员撤销：`POST /api/v1/agent-pools/{pool_id}/agents/{agent_id}/revoke`（设置 `revoked_at`、`token_generation + 1`、状态 offline，并通过 `services.AgentRevocationHook` 撤销该 agent 取得的 run token）。已建立的 C&C 连接每 30 秒（健康检查周期）复查一次，被撤销即断开；数据库临时错误不断开，下个周期再查。被撤销的主机仍可用池令牌重新注册为新 agent——要彻底阻止需撤销池令牌。
    - 新能力 `agent_token_v1`：manifest 绑定任务需要（与 `manifest_bundle_v1`、`task_data_overrides_v1` 一起），缺失 → `agent_upgrade_required`（`error_reason = agent_token_v1`）。上报该能力的 agent 的任务、workspace 路由与 C&C 拒绝池令牌（403），所以同池其他 agent 无法借池令牌冒充。未上报的旧 agent 仍可用池令牌，只做池绑定（非 manifest 任务）。开发环境未配置 `SIGNING_ROOT_KEY`（legacy 签名模式）时不签发 agent token，并从该 agent 的能力中去掉 `agent_token_v1`（agent 回退到池令牌）。
    - 迁移 `20261010_11_agent_token`（`agents.token_generation integer DEFAULT 0 NOT NULL`、`revoked_at timestamptz`、`pool_token_hash varchar(64)`，只增列）。已有 agent 行没有 `pool_token_hash`，它们的 agent token 校验不通过，需重新注册（升级 agent 时本来就会重新注册）。
    - agent 客户端（`services.AgentAPIClient`）：注册用池令牌，之后所有请求与 C&C 连接用 `BearerToken()`；续期返回 401（被撤销 / 注销）立即视为丢失，`OnAgentTokenLost` 回调中 `cmd/agent` 退出进程，由编排重启后重新注册。

21. 按 run 签发的 token（run token，spec §6.1 / §9 第 5 步）与执行端 bundle 哈希不符：
    - `StateTokenService` 增加 run 路径。JWT 带 `typ`：task token 新签发带 `typ=task`（旧 token 无 typ 仍按 task 处理），task 路径行为不变（数据库出错仍退回只验 JWT、可跨 workspace GET）。`typ=run` 的 token 用 `run` 签名用途（kid `run-v<n>`，无 legacy 方案）验证，claims：`run_id`、`workspace_id`、`purpose`（preview|approval）、`session_id`（sandbox / session 内的 run）、`agent_id`（agent run）。按未验证的 typ 选验证路径，再用该路径的用途密钥验签，所以伪造 typ 无法跨路径（测试：run claims 用 state 密钥签 → 401；task claims 用 run 密钥签 → 401）。
    - 校验（每次使用都查库，出错一律拒绝 → 503，不退回只验 JWT）：`run_tokens` 中按 SHA-256 找到行，未撤销、未过期、run_id / workspace / purpose / session 与 claims 一致；run 状态为 pending / running；有 session 时 session 未关闭、status=active、未过期；有 agent_id 时该 agent 存在、未撤销、注册用的池令牌仍有效。
    - 过期时间 = min(run `created_at` + `MANIFEST_RUN_TIMEOUT`（默认 2h），session `expires_at`)；已超时的 run 不再签发。
    - state backend 中间件：`preview` run token 只能访问 run 自己的 workspace（不允许跨 workspace，包括 GET），拒绝 POST state / LOCK / UNLOCK / DELETE（403），只能 GET。approval run 写入的 state 版本 `created_by` = run 创建者（approval run 的跨 workspace 读取与 `task_id` 见第 22 条）。
    - agent 获取：`POST /api/v1/agents/runs/{run_id}/token`，只接受 agent token；run 必须是 `runner=agent`、pending / running，且 `manifest_runs.agent_id` 等于调用 agent（未指派、别的 agent、sandbox run → 403）。签发的 token 记录 `run_tokens.agent_id`。agent 客户端：`AgentAPIClient.ObtainRunToken(runID)`。
    - 撤销：`EndManifestRun`（run 进入 succeeded / failed / cancelled，同一事务只撤销该 run 的 token，session 继续）；`EndSandboxSession`（closed / expired：写 `closed_at`，按 `session_id` 批量写 `revoked_at`，然后调用 `SessionCredentials.RevokeSessionCredentials`——STS 回收挂钩，第 6 步接入 AgentCore 前为只记日志的占位实现）；`ExpireSandboxSessions` 由 leader 每分钟扫描过期 session（校验本身已拒绝过期 session 的 token，扫描负责落库撤销与 STS 回收）；agent 撤销 / 注销通过 `services.AgentRevocationHook = RevokeAgentRunTokens` 撤销该 agent 的全部 run token（另有校验时的 agent 存活检查兜底，覆盖过期清理直接删 agent 行的情况）。
    - 迁移 `20261010_12_run_token_binding`：`manifest_runs.agent_id`、`run_tokens.agent_id varchar(50)`、部分索引 `idx_run_tokens_agent_active`（只增）。
    - approval run 的创建 / 指派与结束见第 22 条；sandbox session 接口在第 6 步。
    - 执行端 bundle 哈希不符：执行端（agent 或 local）解包后哈希与平台给出的 bundle_hash 不符（`manifestbundle.ErrIntegrity`）→ 任务 `error_code = bundle_hash_mismatch`、`error_reason = hash_mismatch`、`error_message = "bundle_hash_mismatch: hash_mismatch (...)"`（原为 `bundle_republish_required`，该码保留给版本本身无合法 bundle 的情况）。平台收到状态上报（新码，或旧 agent 的 `bundle_republish_required` + `hash_mismatch`，仅任务仍为 running 的首次上报）时：写审计 `version.bundle_hash_mismatch`（`source = agent`、agent_id、task_id、version_id、`version_marked = false`）+ WARN 安全日志，然后自己重算该版本已存储文件（`VerifyForUse`；版本 = Run 任务的 `external_files.manifest_version_id`，否则 workspace 当前部署版本；草稿 Run 无版本只审计）。只有平台自己的校验失败才把版本标记为 `hash_mismatch`（并由 `VerifyForUse` 写 `source = platform_recheck:...` 的审计）；agent 的上报本身从不标记版本。

22. 审批与 apply 双哈希校验（spec §1 / §9 第 7 步）：
    - approval run：manifest 部署 workspace（deployment + tag，非 Manifest [Run] 草稿）的每个 `plan_and_apply` 任务对应一个 `purpose=approval`、`runner=agent` 的 `manifest_runs` 行（`task_id` 唯一）。plan 阶段派发前（`TaskQueueManager.TryExecuteNextTask` → `EnsureApprovalRun`）创建，`bundle_hash` = 当前部署版本的 bundle_hash，`status = running`，`created_by` = 任务创建者；版本 bundle_hash 为 NULL 时不建 run（执行端照旧以 `bundle_republish_required` 失败）。Local 执行同样记为 `runner=agent`（平台本身是 runner，`agent_id` 为空）；`runner` 只区分「我们的执行器」与云 sandbox。每个阶段推给 agent 时写 `manifest_runs.agent_id`（`AssignApprovalRunAgent`），只有该 agent 能拿到 run token。
    - run token 取代 task state token：agent `GetTaskData` 对有 approval run 的任务下发 `manifest_approval`（run_id、bundle_hash、approved_bundle_hash、approved_plan_hash），`state_backend.token` 为签给调用 agent 的 run token（`StateTokenService.ApprovalRunTaskData`；未指派 / 别的 agent / 池令牌 → 403，查询出错 → 503）；Local 执行在进程内签发（run 未指派 agent 时允许 `agentID = ""`）。approval run token 与它取代的 task token 一样可以跨 workspace GET（经 `getCrossWorkspaceState` 过滤与授权，写一律 403），写入的 state 版本 `task_id` = run 的任务。过期时间从 `max(created_at, approved_at)` 起算 `MANIFEST_RUN_TIMEOUT`，审批较晚时 apply 阶段仍可用。
    - run 结束（`EndApprovalRunPhase`）：任务进入终态（applied / success / planned_and_finished → succeeded，failed → failed，cancelled → cancelled）时 `EndManifestRun`（撤销该 run 全部 token）；plan 阶段结束（apply_pending / decision_required）只撤销 plan 阶段的 run token，apply 阶段重新签发。调用点：agent 状态上报、Local `executeTask` 结束、取消任务 / 取消之前任务。兜底：run token 校验时任务已是终态即拒绝（不论任务经哪条路径结束）；leader 每分钟 `EndFinishedApprovalRuns` 结束任务已终态的 run。
    - 审批（`POST /workspaces/{id}/tasks/{task_id}/confirm-apply`，沿用现有确认流程）：manifest 部署任务的确认即审批，与任务确认在同一事务中记录到 run 上，失败返回 409 + `error_code` / `error_reason`：没有 run（第 7 步之前跑的 plan）→ `approval_run_required`（重跑 plan）；run 不是 `purpose=approval`、`runner=agent`（preview / sandbox）→ `run_not_approvable`；run 已结束 → `run_not_active`；已审批 → `already_approved`；审批人未知 → 403。校验：run 的版本 bundle_hash 非 NULL（否则 `bundle_republish_required`）且等于 run.bundle_hash，部署当前 tag 仍解析到同一 bundle（否则 `approval_hash_mismatch` / `bundle_changed`）；平台解密已存的 plan.out 重新计算 SHA-256，必须等于执行端上报的 `workspace_tasks.plan_hash`（否则 `approval_hash_mismatch` / `plan_changed`；过期 / 已清理 → `plan_expired`）。记录 `approved_bundle_hash = run.bundle_hash`、`approved_plan_hash = plan.out 的 SHA-256`、`approved_by`、`approved_at`，`manifest_runs.plan_hash` = 脱敏 plan JSON 的哈希（`RedactedPlanHash`，即审批人看到的内容）；同时写 `manifest_deployments.approved_bundle_hash / approved_plan_hash`。
    - 审批绑定的是 plan.out：`terraform apply plan.out` 实际执行的是这个二进制 plan（内含配置快照与确切变更），脱敏 JSON 去掉了敏感值、不能代表要执行的内容；所以 `approved_plan_hash` 取 plan.out 的哈希（与现有工作目录复用用的 `workspace_tasks.plan_hash` 同一个值），脱敏哈希只作为「展示了什么」的记录。
    - 执行端（agent 与 Local 同一代码）：plan 阶段 bundle 解包后，部署交付的 bundle 必须等于 run.bundle_hash，工作目录中的 bundle 文件逐个回读重算哈希也必须相等。apply 阶段：manifest 部署 workspace 的 apply（`plan_and_apply` 与单独的 `apply` 任务）必须有已记录的审批，否则 `approval_hash_mismatch` / `not_approved`；交付的 bundle（当前部署）必须等于 `approved_bundle_hash`，工作目录（复用 plan 阶段的或重建的）中的 bundle 文件回读重算必须等于它（不在 / 非普通文件 / 内容或权限不同 → `bundle_changed`）；在执行 `terraform apply` 前一刻再次核对 bundle，并核对 plan.out 的 SHA-256 等于 `approved_plan_hash`（否则 `plan_changed`）。拒绝时任务 `error_code = approval_hash_mismatch`，`error_reason` = `not_approved` / `bundle_changed` / `plan_changed`，`error_message = "approval_hash_mismatch: <reason>: ..."`。交付归档本身哈希不符仍为 `bundle_hash_mismatch`。apply 时重建的 workspace 现在带上 manifest deployment / tag / subpath，重建工作目录时会解包 bundle，init / apply 在 subpath 中执行（与 plan 一致；之前重建的 workspace 缺这些字段）。
    - 新能力 `manifest_approval_v1`（manifest 任务必需，缺失 → `agent_upgrade_required`）：只有会做上述核对、使用 run token 的 agent 才能接 manifest 任务。
    - 数据库：迁移 `20261010_13_manifest_approval`（只增，可重复）：`manifest_runs.task_id integer`（唯一部分索引 `uq_manifest_runs_task`，外键 `fk_manifest_runs_task` → `workspace_tasks(id) ON DELETE SET NULL`）、`approved_bundle_hash`、`approved_plan_hash varchar(64)`、`approved_by varchar(20)`、`approved_at timestamptz`；`chk_manifest_runs_approval`：审批列要么全空，要么 `purpose='approval' AND runner='agent'`、全部非空、`approved_bundle_hash = bundle_hash`、`approved_plan_hash` 为 64 位小写十六进制——sandbox / preview run 无论经哪条路径写入都不能带审批（约束用 `pg_constraint` 守卫的 DO 块添加）。
    - 第 7 步之前已经处于 apply_pending 的 manifest 任务没有 run，确认时返回 `approval_run_required`，需重新 plan。
    - 任务详情（`GET /workspaces/{id}/tasks/{task_id}`，不含列表）在任务有 manifest run 时返回 `manifest_run`：`id`、`purpose`、`runner`、`status`、`bundle_hash`、`plan_out_hash`（plan.out 的 SHA-256 = `workspace_tasks.plan_hash`，审批绑定的值）、`redacted_plan_hash`（`manifest_runs.plan_hash`）、`approved_bundle_hash`、`approved_plan_hash`、`approved_by`、`approved_at`（未设置为 null）；没有 run 时不出现。只有哈希与身份，不含 plan 内容或 token；沿用任务详情原有的读权限。
23. Git 来源（spec §3.5 / §9 第 8 步）：
    - 创建时选 `source_type`（native / git，之后不可改）；git 需 `git_repo`（`<owner>/<repo>`，推荐）或兼容的 `git_repo_url`（`<GITHUB_URL>/<owner>/<repo>`，无凭证），GitHub 主机只取平台配置（请求中的主机一律拒绝 / 忽略，防 SSRF）+ 本组织经 setup callback 绑定（已验证）、account 为仓库 owner 的 `github_installation_id`；创建与发布都先检查绑定、再用单仓库 token 验证可访问，可选 `git_subpath`（bundle 根）。git manifest 的草稿写接口 409 `git_source_read_only`。
    - 发布 = 选 commit：`commit_sha` 必须是完整 SHA；平台在发布时拉取（`internal/gitsource.Fetcher`：临时 bare 仓库 depth 1 fetch 该 SHA，`ls-tree` + `cat-file` 读树，不 checkout；符号链接 / submodule → `git_symlink` / `git_submodule`），树走同一套发布规则（`ValidateForPublish`），存为 bundle，`source_ref = SHA`。run 永远不拉 git。
    - token：GitHub App installation token 按操作现签，单仓库 + `contents:read`（返回权限更宽即撤销并拒绝），用完撤销；只经 `GIT_ASKPASS` 读 git 进程环境变量注入，不进 URL / argv / 日志 / 错误（git 错误输出去 token）。App 凭证经 `gitsource.CredentialsProvider`（`GITHUB_APP_ID`、`GITHUB_APP_PRIVATE_KEY[_FILE]`），缺失即禁用（503 `git_source_disabled`）。
    - module source：git module 必须钉完整 SHA（`hcl_module_unpinned`，native 发布同样适用），钉了 SHA 的按 base 与平台 module 目录匹配；私有 module 必须 vendor（run 不注入 git 凭证）。
    - IAM：创建 git manifest / 发布 / 列分支与 commit = MANIFESTS WRITE；installation 绑定只经 GitHub App setup callback：`POST /organizations/{org_id}/github-app/connect`（ORGANIZATION ADMIN）返回带签名 state（`ghapp-state` 用途密钥，组织 + 发起用户 + nonce，10 分钟，单次）的安装 URL；`GET /api/v1/github-app/setup/callback` 校验 state、复查 ADMIN、用 `GITHUB_APP_CLIENT_ID/SECRET` 换 user token 证明该 GitHub 用户是 installation 账户的 admin（组织 active admin / 个人账户本人），之后才写库，user token 撤销不落库。手工登记 POST → 410；列表 / 删除 = ORGANIZATION ADMIN；installation 全平台唯一（UNIQUE 约束）。只读 `available-installations`（`{id, account}`）与 `installations/{id}/repositories`（`metadata:read` token，分页）= MANIFESTS WRITE，跨组织 404。
    - webhook `POST /api/v1/webhooks/github`：`X-Hub-Signature-256` 常量时间校验，无签名 401，未配置 secret 503；`X-GitHub-Delivery` 去重（`github_webhook_deliveries`，72h，重复 → 200 no-op）；push 只写 `manifests.git_latest_*` 提示，不发布、不拉取。
    - 数据库：迁移 `20261010_14_manifest_git_source`（只增）：`github_app_installations`、`manifests.git_latest_*` + `chk_manifests_git_latest`；`20261010_15_github_app_binding`（只增）：installation 验证列、UNIQUE 约束、`github_app_setup_nonces`、`github_webhook_deliveries`。
    - 代码：backend/internal/gitsource/*、backend/internal/manifestbundle/{hcl,rules,source}.go、backend/internal/handlers/{manifest_git_handler,manifest_handler,manifest_versions_handler}.go、backend/internal/router/{router,router_manifest}.go、backend/internal/migration/manifest_git_source.go + backend/migrations/add_manifest_git_source.sql、backend/internal/models/{manifest,manifest_v2}.go；绑定：backend/internal/gitsource/oauth.go、backend/internal/handlers/github_app_setup.go、backend/internal/migration/github_app_binding.go + backend/migrations/add_github_app_binding.sql。

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
backend/services/{agent_token_service,agent_api_client}.go、backend/internal/middleware/{agent_token_auth,pool_token_auth,agent_task_ownership}.go、backend/internal/handlers/{agent_handler,agent_pool_handler,agent_cc_handler_raw}.go、backend/internal/router/router_agent.go、backend/internal/migration/agent_token.go + backend/migrations/add_agent_token.sql、backend/agent/control/cc_manager.go、backend/cmd/agent/main.go（第 20 条）
backend/services/{run_token_service,state_token_service,manifest_bundle_mismatch,terraform_init_policy}.go、backend/internal/middleware/state_token_auth.go、backend/internal/handlers/{agent_handler,tf_state_backend_handler}.go、backend/internal/migration/run_token_binding.go + backend/migrations/add_run_token_binding.sql、backend/internal/models/{manifest_run,workspace}.go、frontend/src/utils/taskErrorCode.ts（第 21 条）
backend/services/{manifest_approval,manifest_approval_check,run_token_service,task_queue_manager,terraform_executor,terraform_init_policy,data_accessor,local_data_accessor,remote_data_accessor}.go、backend/controllers/workspace_task_controller.go、backend/internal/handlers/agent_handler.go、backend/internal/middleware/state_token_auth.go、backend/internal/migration/manifest_approval.go + backend/migrations/add_manifest_approval.sql、backend/internal/models/{manifest_run,workspace,agent}.go、frontend/src/utils/taskErrorCode.ts（第 22 条）
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

**第 5 步（run token）** —— 已完成（第 20、21 条；run 的创建 / 指派与 session 接口随第 6 / 7 步）。原始要求：
- `run_tokens` 带 `session_id`；token 过期时间取 run 超时和 session 过期里较早的那个。
- JWT 加 `typ` claim（`task`/`run`）；`run` 路径在数据库校验出错时拒绝，`task` 路径保持现有行为。
- `preview` token 拒绝 POST/LOCK/UNLOCK。
- run 结束只撤销这个 run 自己的 token；session 结束或过期时，统一回收它的所有 token 和 STS 凭证。

**第 6 步**：AgentCore 只支持 VPC 模式，代码里拒绝 Sandbox 和 Public 模式；创建 session 时检查 `WORKSPACE_STATE` READ 和 plan 权限。

**第 7 步** —— 已完成（第 22 条）。原始要求：审批只接受 `purpose=approval`、`runner=agent` 的 run；apply 之前 agent 要核对 `approved_bundle_hash` 和 `approved_plan_hash`；拒绝 NULL 哈希的版本。

**第 8 步（git 来源）** —— 已完成（第 23 条）。原始要求：
- 私有 module 的 `ref` 必须是 commit SHA（或者 vendor 进来）。
- git token 按 run 现签：GitHub App installation token，权限为单仓库 `contents:read`，有效期约 1 小时，不落库；通过 `GIT_ASKPASS` 注入，不能拼进 URL。
- 发布时由平台拉取仓库，并固定到某个 SHA；webhook 要验签。

**其他**
- `variable_sets` 表加 `org_id` 列，改为直接按列绑定组织。
- 跑过已删除迁移 `20261004_03_manifest_bundle_rules` 的开发库和测试库需要重建（`20261004_04` 不会修复它们）。


### Reserved env vars / git source immutability

- **422 `reserved_env_var`**: environment-category variable key is platform-reserved
  (`internal/reservedenv`). See `GET /api/v1/system/reserved-env-prefixes`.
- **409 `git_source_immutable`**: also enforced by DB trigger
  `manifests_git_source_immutable` (migration `20261010_16`).
- Platform proxy for exec: `IAC_EXEC_HTTP_PROXY` / `IAC_EXEC_HTTPS_PROXY` /
  `IAC_EXEC_NO_PROXY` (users cannot set `HTTP(S)_PROXY` per workspace).
