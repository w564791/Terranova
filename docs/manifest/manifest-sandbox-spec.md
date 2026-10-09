# Manifest：双来源 + 双 Runner 方案（锁定版）

状态：已锁定（2026-10-04，设计群）。实现分支：`feat/manifest-sandbox`。

## 1. 范围与决策
- 来源：创建时二选一 `native`（平台内编辑 + 草稿）或 `git`（GitHub App 只读，钉 SHA），创建后不可改。
- 版本 = 不可变 bundle，以内容哈希 `bundle_hash` 标识。native 发布时由草稿打包；git 由 commit SHA 打包。下游（run、部署、apply）只读 bundle，不再读草稿或分支。
- Runner 两种，同一个四步接口：`prepare(bundle, state) → exec(init|validate|plan) → fetch(plan JSON) → destroy`。
  - **远程**：现有 K8s 驱动（agent / server），可跑 `preview` 与 `approval`。
  - **本地 sandbox**：只走云方案，第一个 provider 为 **AWS AgentCore**，只跑 `preview`。Kata 不进本期。
- 本期没有本地（用户机器）驱动。

## 2. Plan 的两种用途
| purpose | runner | state | 加锁 | 能否审批 |
|---|---|---|---|---|
| `preview` | sandbox 或 agent | 只读，标注「基于 state serial N」，serial 变化即过期 | `-lock=false` | 否 |
| `approval` | 仅 agent（K8s 驱动） | 正常 | 必须加锁 | 是 |

审批接口只接受 `purpose=approval` 的 run。apply 前 agent 校验 `approved_bundle_hash` 与 `approved_plan_hash`，任一不一致即拒绝。

## 3. 数据模型
- `manifests`：`source_type`（native|git，不可变）；git 另存 repo、subpath、GitHub App installation ID。
- `manifest_versions`：`bundle_hash`、`source_ref`（git 为 SHA，native 为空）。存量回填为 native 并计算哈希。
- `manifest_runs`（新）：`runner`（agent|sandbox）、`purpose`（preview|approval）、`bundle_hash`、`workspace_id`、`state_serial`、`plan_hash`、脱敏后 plan。native 草稿预览时临时打包算哈希。
- `sandbox_sessions`（新）：user + workspace 绑定、provider、过期时间、只读 STS 角色；过期时 STS 与其下所有 run token 一起回收。
- 部署：`approved_bundle_hash`、`approved_plan_hash`。

### 3.1 已落地 schema（step 2，迁移 `20261004_02_manifest_sandbox_schema`）
只做加法、可重复执行；同一份 DDL 在 `backend/migrations/add_manifest_sandbox_schema.sql` 与 `manifests/db/init_seed_data.sql`（`-- >>> migrations/add_manifest_sandbox_schema.sql >>>` 标记块）中保持一致，由 `manifest_sandbox_schema_test.go` 校验同步。

| 表 | 新增 | 约束 / 说明 |
|---|---|---|
| `manifests` | `source_type varchar(16) NOT NULL DEFAULT 'native'`、`git_repo_url varchar(1024)`、`git_subpath varchar(512)`、`github_installation_id bigint` | `chk_manifests_source_type`（native\|git）；`chk_manifests_git_fields`：native ⇒ 三个 git 字段全为 NULL，git ⇒ `git_repo_url` 非空。存量行经默认值成为 native。API 只输出 `source_type`；创建固定为 native（git 创建在 step 8），更新时传入不同值返回 400。git 字段不出 JSON。 |
| `manifest_versions` | `bundle_hash varchar(64)`、`source_ref varchar(64)` | `bundle_hash` 为 NULL 或 64 位小写 hex；`source_ref` 为 NULL 或 40/64 位小写 hex（git SHA-1/SHA-256）。`bundle_hash` 保持可空，便于新旧版本混跑时滚动上线；发布（PublishVersion）在同一事务内写入，存量由迁移回填（step 2 为 v1；step 3 的迁移 `20261004_04` 校验后改写为 v2 并按 bundle 规则判定，见 §3.3）。`bundle_hash` 与 step 3 新增的 `bundle_invalid_reason` 出现在版本列表/详情 JSON；`source_ref` 不出 JSON。 |
| `manifest_deployments` | `approved_bundle_hash`、`approved_plan_hash`（varchar(64)） | 审批接入前恒为 NULL（step 7 使用），不出 JSON。 |
| `manifest_deployments` / `workspace_tasks` | `sensitive_keys jsonb`（可空、无默认值） | 覆盖值为敏感的 key 列表（任务行是部署覆盖快照的同一标记）。迁移不回填、不批量标记；NULL = 尚未计算，API 一律按全部敏感处理（不返回任何值）。由启动时的 Go 回填任务按部署时同一敏感判定写入（见设计文档 §8.4）。不出 JSON。 |
| `sandbox_sessions`（新） | `id`、`user_id`、`workspace_id`、`provider`、`network_mode DEFAULT 'vpc'`、`status`、`expires_at`、`closed_at`、时间戳 | `chk_sandbox_sessions_network_mode`：只允许 `vpc`（§6.2 的数据库兜底）；`UNIQUE(id, workspace_id)` 供复合外键使用。 |
| `manifest_runs`（新） | `manifest_id`（FK CASCADE）、`version_id`（FK SET NULL，草稿预览为空）、`bundle_hash NOT NULL`、`workspace_id`、`runner`、`purpose`、`status`、`plan_hash`、`plan_redacted jsonb`、`state_serial`、`session_id`、`created_by` | runner ∈ agent\|sandbox，purpose ∈ preview\|approval；sandbox ⇒ purpose=preview 且 session 非空；`bundle_hash` 64 位 hex；`(session_id, workspace_id)` 复合 FK → session，run 不能跨出其 session 的 workspace；`UNIQUE(id, workspace_id, purpose)`。 |
| `run_tokens`（新，§9 step 5 的表，提前建） | `run_id`、`session_id`、`workspace_id`、`purpose`、`token_hash`、`expires_at`、`revoked_at` | `token_hash` 唯一；purpose ∈ preview\|approval；`(run_id, workspace_id, purpose)` 复合 FK → run（ON DELETE CASCADE），token 的 workspace/purpose 必须与 run 一致；`(session_id, workspace_id)` 复合 FK → session。另有按 session 未撤销 token 的部分索引，供 session 关闭时批量写 `revoked_at`。 |

- `status` 列（session、run）不加 CHECK：后续新增状态不需要删约束。
- run 暂不关联 deployment（`deployment_id` 留到 step 7 再定）。

### 3.2 bundle_hash 编码（`internal/manifestbundle`，`terranova-bundle-v2`）
```
sha256( "terranova-bundle-v2" 0x00
        对每个文件，按 path 字节序排序：path 0x00 mode 0x00 十进制(len(content)) 0x00 content )
```
输出小写 hex。path 为 `manifest_files.path`（相对路径，不能为空、不能重复）；mode 为归一化后的八进制文本：任一可执行位（0o111）置位 → `755`，否则 `644`（含 0/未知；git 的 `100755` / `100644` 同样映射，见 `ModeFromGit`，symlink / submodule 不是 bundle 文件）。发布时 `manifest_files.mode` 按归一化值写入。长度前缀保证二进制内容（可含 NUL）无歧义；空 bundle 也有确定的哈希。native 版本的文件集 = `manifest_files WHERE version_id = 版本 id`（草稿行 `version_id IS NULL`，不参与）。改编码必须换版本前缀，不得原地修改。发布与后续 run/审批只用 `manifestbundle.Hash` / `VersionHash`（v2）；v2 黄金向量在 PostgreSQL 17 上用等价 SQL 表达式独立算出。

**v1（`terranova-bundle-v1`，只含 path + content）**：step 2（迁移 `20261004_02` 与 SQL 补丁 `add_manifest_sandbox_schema.sql`，内容自 68ff86e 未改）回填的编码，Go 侧为 `LegacyHashV1` / `VersionHashV1`，与 SQL 的等价性由 v1 黄金向量锁定。v1 不再用于新哈希，只用于迁移 `20261004_04` 校验存量（§3.3）。

### 3.3 Bundle（step 3，迁移 `20261004_04_manifest_bundle_v2`）
版本发布后不可变：所有下游只读版本的 bundle，不再读草稿。

**存储**：复用 `manifest_files` 的版本行（`version_id = 版本 id`、`owner_user_id IS NULL`），按 `manifest_versions.bundle_hash` 内容寻址（部分索引 `idx_manifest_versions_bundle_hash`）。不引入新存储，也不做跨版本去重；同一文件集在不同版本里各存一份，哈希相同。哈希编码见 §3.2（v2，含文件 mode）。

**`internal/manifestbundle`**
- `Source` 接口（`ReadFiles`）：`NativeDraft`（调用者的草稿）、版本快照（包内）、`GitCommit`（占位，返回 `ErrGitSourceNotImplemented`，step 8 实现）。
- `Pack` / `PackFiles`：校验规则（`Validate`）并计算哈希；有违规时不产出 bundle。
- `Store`：在发布事务内写版本行，再用 `VersionHash` 重算并与打包哈希比对（不一致 → `ErrIntegrity`），最后写 `bundle_hash`、清空 `bundle_invalid_reason`。
- `OpenVersion`（按版本）/ `OpenBundle`（按哈希）：只读存储的文件、`bundle_hash`、`bundle_invalid_reason`，**不重算、不写库**。
- `RequireValid`：`bundle_hash` 为 NULL 或带任何 `bundle_invalid_reason` 即无效（记录的原因优先于残留哈希）。
- `Verify`：纯重算比对。`VerifyForUse`：真正使用版本处的完整性闸门（见下）。

**完整性只在使用处校验**：版本列表 / 详情、编辑器 `ListFiles` / `ReadFile`（`?version=`）、导出、diff、workdirs、敏感 key 计算、outputs、AI 工具都只读存储值，不重算哈希、不写库。只在 install、首装预览、按部署预览（目标版本，未给则当前版本）、upgrade 的**目标**版本，以及执行器取文件（`LocalDataAccessor.GetManifestFilesByTag`，runner 交接点）重算：
- 不一致 → 尽力把版本记为 `bundle_hash = NULL`、`bundle_invalid_reason = 'hash_mismatch'`（写失败只记日志），输出 WARN 安全日志 `[WARN] [security] manifest bundle hash mismatch: manifest_id=… version_id=… request_id=… source=…`，并写一条 `audit_logs`（`MANIFEST_VERSION` / `version.bundle_hash_mismatch`，`new_values` 带 level、manifest_id、version_id、request_id、source）。部署路径返回 **409** `bundle_republish_required`（`reason: "hash_mismatch"`），执行器报错。执行器的记录写在事务外，外层回滚不会丢。
- **`hash_mismatch` 粘滞**：一旦记录，任何重算、回填、迁移都不会把它改回合法；使用处直接拒绝，不再重算、不再重复上报。只有发布新版本才会产生合法 bundle（仅对新版本）。

**规则**：每个违规为 `Problem{file, line?, rule, message}`（422 的 problem 形状）。
- `file`：违规路径；bundle 级规则（`bundle_too_large`、`too_many_files`）为空串，原因串里只有规则名。
- `line`：只有 secret-scan 命中才带，为首个命中的 1 起行号；路径 / 大小 / denylist 规则不带。
- `message`：每条规则的固定文案，绝不含文件内容或命中文本。
- 原因串（`bundle_invalid_reason` 与日志）为 `rule @ file`，以 `; ` 连接，最多 20 条，超出追加 `(+N more)`；不可打印的路径加引号；不含行号与文案。

上限常量 `MaxPathLen` / `MaxFileSize` / `MaxBundleSize` / `MaxFiles` 均导出，供 step 4 解包等复用。编辑器写草稿时只限制路径与单文件大小，文件数与总大小只在发布时检查。迁移 04 对存量版本用同一套规则：超限 → `bundle_hash = NULL` + 原因，迁移不失败。

| rule | 条件 |
|---|---|
| `path_invalid` | 空、以 `/` 开头或结尾、空段或 `.`/`..` 段、反斜杠、控制字符、非 UTF-8 |
| `path_too_long` | 超过 256 字节 |
| `path_not_nfc` | 非 Unicode NFC（允许 Unicode；编辑器写入仍限 ASCII） |
| `path_duplicate` / `path_case_duplicate` | 路径重复 / 忽略大小写（NFC 后）冲突，冲突双方都报 |
| `denylisted_file` | 文件名（不区分大小写）：`*.tfvars`、`*.tfvars.json`、`*.tfstate`、`*.tfstate.backup`、`.terraformrc`、`terraform.rc`、`.env`、`.env.*`、私钥/证书库 `*.pem`、`*.key`、`*.p12`、`*.pfx`、`id_rsa`、`id_dsa`、`id_ecdsa`、`id_ed25519`、`.git` 文件；任意深度的 `.terraform/`、`.git/` 目录段。允许：`.terraform.lock.hcl`、`id_rsa.pub` 等公钥、`.gitignore`、`.envrc` |
| `file_too_large` / `bundle_too_large` | 单文件超过 1 MB / 内容总和超过 50 MB |
| `too_many_files` | 文件数超过 `MaxFiles` = 2000。在任何逐文件检查之前判定，命中即只返回这一条（`file` 为空串、无 `line`），不再做路径 / 内容扫描 |
| `secret_scan:<kind>` | 内容命中高置信度凭证格式：`aws_access_key`、`private_key`、`github_token`、`slack_token`（新写的最小扫描器，代码库原先没有） |

**HCL 静态检查（仅发布，`manifestbundle.CheckHCL` / `ValidateForPublish`）**：对 Terraform 会加载的每个配置文件（任意深度，本地 module 也会被加载）：`*.tf`、`*.tf.json`（含 `override.tf`、`*_override.tf` 及其 `.json`）、OpenTofu 的 `*.tofu`、`*.tofu.json`，不区分大小写，连 Terraform 自己忽略的 `.`/`#` 开头、`~` 结尾的文件也检查（宁多勿少）。用 `hclparse`（原生 `ParseHCL`、JSON `ParseJSON`）解析，按 body schema 检查块，不用正则。problem 带 `file` 与命中块 / 属性的 1 起 `line`。

| rule | 条件 |
|---|---|
| `hcl_parse_error` | 文件无法解析，或检查的块头不合法（标签数不对等）；解析失败一律算命中，绝不跳过 |
| `hcl_provisioner` | 任意 `resource`（含 `null_resource`、`terraform_data`）或 `removed` 块里的任意 `provisioner`（local-exec / remote-exec / file 等，任何 `when`） |
| `hcl_external_data` | `data "external"`（顶层或 `check` 块内的嵌套 data）；`required_providers` 把 `hashicorp/external` 映射到任意本地名 |
| `hcl_http_data` | `data "http"`（同上）；`required_providers` 映射 `hashicorp/http` |
| `hcl_module_source` | `module` 的 `source` 不是静态字符串、缺失，或既不是留在 bundle 内的相对路径（`./`、`../`，相对声明文件所在目录解析后不越出 bundle 根），也不在白名单内 |

module source 白名单唯一入口 `manifestbundle.PublishModuleSourcePolicy`（step 8 在此扩展）：仓库里没有独立的 module source / registry 白名单配置，因此白名单 = 平台 module 目录中 `status='active'` 的 module 的 `modules.module_source` 及其 `module_versions.module_source`（即编辑器可选的 module；`modules.source` 是导入方式标识，不算）。精确匹配，另允许 `<条目>//<子目录>` 形式。目录只在遇到第一个非本地 source 时才查询。HCL 检查只在发布时执行：迁移与存量重判仍只用 `Validate`，不会让已有合法哈希的版本因新规则失效。`manifestbundle.LocalModulesOnly`（nil 策略）只允许本地路径，供以后检查不可信 bundle 使用。

**发布**：在同一事务内依次执行：
1. 读调用者的草稿；没有 `.tf` 文件则返回 400。
2. 违反规则则返回 **422**（不建版本）：`{error:"draft violates the bundle rules", code:"bundle_rules_violated", problems:[{file, line?, rule, message}]}`。
3. 从 bundle 取变量元信息。
4. 建版本，执行 `Store`。

成功响应带 `bundle_hash`。

**存量（迁移 `20261004_04_manifest_bundle_v2`）**：只做加法：新增 `bundle_invalid_reason text` 列和上述部分索引。DDL 同步在 `backend/migrations/add_manifest_bundle_v2.sql` 与 seed（由测试校验）；SQL 补丁只含 DDL，哈希处理只在 Go 里。Go 遍历全部版本，文件一律不动，违规也不会让迁移失败，可重复执行：
- `hash_mismatch`：不再判定（粘滞）；若有残留哈希则置回 NULL。
- 有存储哈希（step 2 的 v1，或已是 v2）：当前文件的 v1 或 v2 哈希必须与之相等，**否则**置 `bundle_hash = NULL` + `hash_mismatch`，记 WARN 安全日志，绝不按当前内容重算（不洗白篡改）。
- 哈希匹配，或从未有哈希：按规则判定。合法 → 写 v2 哈希、原因置 NULL；违规 → `bundle_hash = NULL` + `rule @ file` 原因，记 `[migration] manifest version <id>: bundle invalid, republish required: <reason>`。因规则失败的 NULL 行再次运行时会重新判定。
- 最后一行汇总。

本迁移取代了分支上曾短暂存在的 `20261004_03_manifest_bundle_rules`（未合并，只有开发/测试库跑过；不做补偿，跑过 03 的开发库需重建）。step 2 的 Go 回填改用 `VersionHashV1`，与恢复到 68ff86e 的 SQL 补丁一致，并在列存在时跳过带原因的行。SQL 补丁本身没有原因守卫：手工重跑可能给 NULL 行写回 v1 哈希，但记录的原因始终优先（`RequireValid` / `VerifyForUse`），版本仍被拒绝；再跑 04 会把这些哈希清回 NULL。

**NULL 语义与 409**：`bundle_hash IS NULL`（或带原因）= 该版本没有合法 bundle，需重新发布。以下路径都返回 **409** `{error:"this version has no valid bundle; please republish it", code:"bundle_republish_required", version_id, reason}`（前端按 `code` 判断）：install、首装预览、按部署预览（检查目标版本，未给目标时检查当前版本）、upgrade 的**目标**版本。从无效版本升级到合法版本允许；uninstall 不受影响。

**下游读取**：都经 `OpenVersion` 读版本内容，不读草稿：
- install（workdir 校验、资源解析）与预览；
- 导出：版本导出与 `export-zip`（后者只在没有任何已发布版本时才退回调用者草稿）；
- diff 的版本侧（版本不存在现返回 404）；
- workdirs、敏感 key 计算、执行器取文件、outputs 模块源解析与 AI 工具。

**执行器闸门**：执行器取文件（`LocalDataAccessor.GetManifestFilesByTag` → `manifestbundle.RequireValidForRun`）先做 `VerifyForUse`，再 `RequireValid`：`bundle_hash` 为 NULL 的版本（规则违规或 `hash_mismatch`）一律不交给 Terraform，plan / apply / drift 任务失败，错误为 `bundle_republish_required: <reason>`（`*RepublishRequiredError`）。uninstall 不受影响：它只解绑元信息（清 workspace 的 manifest 三列与 manifest 资源行），不创建任务；之后在 workspace 跑的 Plan+Apply 不再加载 manifest 文件，按 state 销毁残留资源，bundle 代码（包括 destroy-time provisioner、`.terraformrc` 等）不会被执行，所以无效版本无需确认即可 uninstall。

**暂未覆盖（后续步骤）**：
- Agent 模式 `RemoteDataAccessor.GetManifestFilesByTag` 仍不支持。
- 编辑器 ExternalFiles 的「Run」仍用草稿内容（step 4/6 改为预览 run）。

## 4. 接口
- `POST/DELETE .../sandbox-sessions`：创建校验目标 workspace `WORKSPACE_STATE` READ + plan 权限；session 不可换 workspace。
- `POST .../sandbox-sessions/:id/runs`：在 session 内发起 preview run。
- `POST .../deployments/:id/plan`：agent 上生成 approval run。
- `POST .../deployments/:id/approve`、apply：只认 approval run + 双哈希。

## 5. IAM（沿用 Phase 1 锁定版）
- 目录：org `MANIFESTS` READ/WRITE，删除归档需 ADMIN。
- 部署（install/upgrade/uninstall）：`MANIFESTS` READ + 目标 workspace `WORKSPACE_RESOURCES` WRITE（显式检查，不改 `RequireWorkspacePermission`）。
- sandbox session：目标 workspace `WORKSPACE_STATE` READ + plan 权限；仅 `MANIFESTS` WRITE 不够。
- 编辑器 modules/demos/inputs：`MODULES` READ；varset：`VARIABLE_SETS` READ 且按 workspace 收口。
- 列表项带 `can_write`、`can_deploy`；`GET /workspaces?capability=WORKSPACE_RESOURCES:WRITE`；preview 带 `sensitive`；无批量读文件接口。

## 6. 安全硬规则
1. 凭证：参考 GitLab Terraform 方案，凭证与数据按 run 直接注入。sandbox 只拿只读短期 STS（assume 只读角色），绝不注入 workspace 的 apply 凭证。平台访问只用按 run 签发的 token（类似 `CI_JOB_TOKEN`，复用 `StateTokenService`，task 换成 `run_id` 并带 `purpose`）：绑定 run + workspace，只能访问 state backend 和该 run 所需接口，run 结束即撤销；`preview` token 在中间件拒绝 POST/LOCK。
2. AgentCore 只允许 VPC 网络模式，Sandbox / Public 模式代码层直接拒绝；AWS 侧配不了 VPC 端点则本地模式不上线。
3. 出网默认拒绝，只放行 provider mirror / registry 白名单与目标云 API；DNS 走 Route 53 Resolver DNS Firewall。
4. sandbox 可访问平台（仅凭 run token）；禁止访问 `169.254.169.254`、数据库、K8s API；`automountServiceAccountToken: false`；节点强制 IMDSv2、hop limit 1。
5. `init` 校验 `.terraform.lock.hcl` checksum，走内部 mirror。
6. plan JSON 与 state 一样含 `sensitive` 值：入库前脱敏或加密，前端只展示脱敏结果。
7. git：webhook 验签；发布前检查 provisioner、`external` data source、module source 白名单。
   - 拉仓库：发布时在平台侧按 SHA 拉取并打 bundle，sandbox / agent 只拿 bundle，不需要 git 凭证。
   - 私有 module 源（`git::https://...`）：`init` 时需要凭证，注入按 run 现签的 GitHub App installation token（单 repo `contents:read`，约 1 小时过期，不落库），经 `GIT_ASKPASS` 或 credential helper 注入，禁止拼进 URL（否则会留在 `.terraform/modules/modules.json`、plan 输出和日志）。bundle 与日志不得含 token，提交前扫描。
   - 可复现性：发布时要求 module 源的 `ref` 是 commit SHA（分支或 tag 拒绝发布），或把 module vendor 进 bundle；否则同一个 `bundle_hash` 在不同时间 init 会拉到不同代码，哈希校验失去意义。

## 7. sandbox 读 state（已定）
按 GitLab 方案：sandbox 通过 HTTP state backend 读 state，凭按 run 签发的 token（§6.1）。preview run 只读、`-lock=false`，run 记录读到的 `state_serial` 作为预览基准。`sandbox_sessions` 不再单独存 state token，token 挂在 run 上。

## 8. 前端
- 新建 manifest 先选「平台内编辑」或「Git 仓库」，选后不可改；列表带来源标记。
- git 项目编辑器只读，提示「修改请走 Git」；版本列表显示 commit SHA 与提交信息，发布 = 选 commit。
- 编辑器里是「预览 plan」：跑在 sandbox，标「仅供预览」，无审批按钮；先选 workspace（只列有 state 读权限的），顶部显示 session 剩余时间，过期提示重新开始，不后台续期。
- 部署面板里是「生成待审批的 plan」：跑在 agent，审批和 apply 只从这里进。
- 预览结果标「基于 state serial N」；workspace 有新 apply、serial 变化后提示「state 已更新，请重新预览」。
- 两边变更只显示脱敏值。加载预算沿用 Phase 1：列表不预载 schema/模块/React Flow/Monaco；Deploy 两步（先可写 workspace，再 varset 与预览）；编辑器按 tab 读文件。

## 9. 实现顺序

原则：最大限度复用现有代码。IAM 沿用角色策略只加 `MANIFESTS`；迁移只做加法；agent runner 用现有 K8s 驱动只抽接口；token 沿用 `StateTokenService`/`StateTokenAuth`。真正新写的只有 bundle 构建、AgentCore provider、git 拉取三块，每一步提交都检查有没有重复造轮子。
（`feat/manifest-sandbox`，每步一提交、带测试、推远程）
1. IAM（Phase 1 锁定版，独立可先合）
2. 迁移 + 回填
3. Bundle
4. Runner 接口 + K8s 实现（approval 必须加锁；先脱敏再算 `plan_hash`）
5. 按 run 签发的 token：在现有 `StateTokenService` 上改。新表 `run_tokens`（`run_id`、`session_id`、`workspace_id`、`purpose`、`token_hash`、`expires_at`、`revoked_at`），`workspace_tasks` 旧字段不动；JWT 加 `typ` claim（`task`/`run`），`ValidateToken` 分支：`run` 路径数据库出错即拒绝（不退回只验 JWT），过期时间取 min(run 超时, session 过期)，`preview` 在中间件拒绝 POST/LOCK/UNLOCK；`task` 路径行为暂不变，另行评估。run 结束写 `revoked_at`（只撤销该 run 的 token，session 保留到自身过期或用户关闭，届时按 `session_id` 批量写 `revoked_at`）+ 只读 STS + 私有 module 的 installation token 注入（`GIT_ASKPASS`）
6. AgentCore provider + session 接口（仅 VPC）
7. 审批与 apply 双哈希校验
8. Git 来源（平台侧按 SHA 拉取打包；module `ref` 必须是 SHA 或 vendor；webhook 验签）

## 10. AgentCore 上线前需实测
- Code Interpreter 能否自带 terraform 与 provider 二进制（或改用 AgentCore Runtime 自定义镜像）。
- 单个 session 最长运行时间是否覆盖大 workspace 的 plan。
- VPC 模式下到 provider mirror 与目标云 API 的路径（VPC 端点 / NAT + 白名单）。
