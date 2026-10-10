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

已落地（step 7，迁移 `20261010_13_manifest_approval`）：manifest 部署的每个 plan_and_apply 任务在 plan 派发前建一个 `purpose=approval`、`runner=agent` 的 run（`manifest_runs.task_id`；Local 执行也记为 agent runner，`agent_id` 为空），run token 取代 task state token。审批（confirm-apply）只接受这样的 run（`chk_manifest_runs_approval` 在数据库兜底），记录 `approved_bundle_hash` = run 的 bundle_hash、`approved_plan_hash` = plan.out（二进制 plan，`terraform apply` 实际执行的内容）的 SHA-256，由平台解密已存 plan 复算；`manifest_runs.plan_hash` 为脱敏 plan JSON 的哈希。执行端在 init 前与 `terraform apply` 前一刻核对部署 bundle、工作目录中的 bundle 文件与 plan.out，不符以 `approval_hash_mismatch`（`not_approved` / `bundle_changed` / `plan_changed`）拒绝。详见 `docs/security/api-fix-tasks/14-manifest.md` 第 22 条。

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
| `manifests` | `source_type varchar(16) NOT NULL DEFAULT 'native'`、`git_repo_url varchar(1024)`、`git_subpath varchar(512)`、`github_installation_id bigint` | `chk_manifests_source_type`（native\|git）；`chk_manifests_git_fields`：native ⇒ 三个 git 字段全为 NULL，git ⇒ `git_repo_url` 非空。存量行经默认值成为 native。创建时选 `source_type`（step 8 起可选 git，见 §3.5），更新时传入不同值返回 400。git 字段（`git_repo_url`、`git_subpath`、`github_installation_id`，以及 step 8 的 `git_latest_*`）只在 git manifest 上出现在 JSON。 |
| `manifest_versions` | `bundle_hash varchar(64)`、`source_ref varchar(64)` | `bundle_hash` 为 NULL 或 64 位小写 hex；`source_ref` 为 NULL 或 40/64 位小写 hex（git SHA-1/SHA-256）。`bundle_hash` 保持可空，便于新旧版本混跑时滚动上线；发布（PublishVersion）在同一事务内写入，存量由迁移回填（step 2 为 v1；step 3 的迁移 `20261004_04` 校验后改写为 v2 并按 bundle 规则判定，见 §3.3）。`bundle_hash` 与 step 3 新增的 `bundle_invalid_reason` 出现在版本列表/详情 JSON；`source_ref`（git 版本所钉的 commit SHA）step 8 起也出现在 JSON（native 为空，不输出）。 |
| `manifest_deployments` | `approved_bundle_hash`、`approved_plan_hash`（varchar(64)） | 最近一次审批的哈希（step 7 起由 confirm-apply 写入，权威记录在 `manifest_runs.approved_*`），不出 JSON。 |
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
- `Source` 接口（`ReadFiles`）：`NativeDraft`（调用者的草稿）、版本快照（包内）、`GitCommit`（step 8：`gitsource.Fetcher` 在平台侧取回的某个 commit 的文件，见 §3.5）。
- `Pack` / `PackFiles`：校验规则（`Validate`）并计算哈希；有违规时不产出 bundle。
- `Store`：在发布事务内写版本行，再用 `VersionHash` 重算并与打包哈希比对（不一致 → `ErrIntegrity`），最后写 `bundle_hash`、清空 `bundle_invalid_reason`。
- `OpenVersion`（按版本）/ `OpenBundle`（按哈希）：只读存储的文件、`bundle_hash`、`bundle_invalid_reason`，**不重算、不写库**。
- `RequireValid`：`bundle_hash` 为 NULL 或带任何 `bundle_invalid_reason` 即无效（记录的原因优先于残留哈希）。
- `Verify`：纯重算比对。`VerifyForUse`：真正使用版本处的完整性闸门（见下）。

**完整性只在使用处校验**：版本列表 / 详情、编辑器 `ListFiles` / `ReadFile`（`?version=`）、导出、diff、workdirs、敏感 key 计算、outputs、AI 工具都只读存储值，不重算哈希、不写库。只在 install、首装预览、按部署预览（目标版本，未给则当前版本）、upgrade 的**目标**版本，以及执行器取文件（`GetManifestBundleByTag` → `LoadRunnableManifestBundle`，runner 交接点，local 与 agent 同一路径）重算：
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
| `git_symlink` / `git_submodule` | 仅 git 来源（step 8）：commit 树里的符号链接（`120000`）/ submodule（`160000`）条目。由 fetcher 在读内容前判定，不读其内容 |

**HCL 静态检查（仅发布，`manifestbundle.CheckHCL` / `ValidateForPublish`）**：对 Terraform 会加载的每个配置文件（任意深度，本地 module 也会被加载）：`*.tf`、`*.tf.json`（含 `override.tf`、`*_override.tf` 及其 `.json`）、OpenTofu 的 `*.tofu`、`*.tofu.json`，不区分大小写，连 Terraform 自己忽略的 `.`/`#` 开头、`~` 结尾的文件也检查（宁多勿少）。用 `hclparse`（原生 `ParseHCL`、JSON `ParseJSON`）解析，按 body schema 检查块，不用正则。problem 带 `file` 与命中块 / 属性的 1 起 `line`。

| rule | 条件 |
|---|---|
| `hcl_parse_error` | 文件无法解析，或检查的块头不合法（标签数不对等）；解析失败一律算命中，绝不跳过 |
| `hcl_provisioner` | 任意 `resource`（含 `null_resource`、`terraform_data`）或 `removed` 块里的任意 `provisioner`（local-exec / remote-exec / file 等，任何 `when`） |
| `hcl_external_data` | `data "external"`（顶层或 `check` 块内的嵌套 data）；`required_providers` 把 `hashicorp/external` 映射到任意本地名 |
| `hcl_http_data` | `data "http"`（同上）；`required_providers` 映射 `hashicorp/http` |
| `hcl_module_source` | `module` 的 `source` 不是静态字符串、缺失，或既不是留在 bundle 内的相对路径（`./`、`../`，相对声明文件所在目录解析后不越出 bundle 根），也不在白名单内 |
| `hcl_module_unpinned` | step 8：git module source（`git::`、`github.com/`、`bitbucket.org/`、`git@`）没有钉完整 commit SHA（`?ref=<40/64 位小写 hex>`，除 `depth` 外不许其他参数，如 `sshkey`）。不论策略如何都先判；钉了 SHA 的仍须在白名单内 |

module source 白名单唯一入口 `manifestbundle.PublishModuleSourcePolicy`（step 8 已在此扩展：git module source 必须钉 SHA，钉了 SHA 的按去掉 `?query` 后的 base 与目录条目（条目本身带不带 ref 都行）匹配，含 `//子目录`）：仓库里没有独立的 module source / registry 白名单配置，因此白名单 = 平台 module 目录中 `status='active'` 的 module 的 `modules.module_source` 及其 `module_versions.module_source`（即编辑器可选的 module；`modules.source` 是导入方式标识，不算）。精确匹配，另允许 `<条目>//<子目录>` 形式。目录只在遇到第一个非本地 source 时才查询。HCL 检查只在发布时执行：迁移与存量重判仍只用 `Validate`，不会让已有合法哈希的版本因新规则失效。`manifestbundle.LocalModulesOnly`（nil 策略）只允许本地路径，供以后检查不可信 bundle 使用。

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

**执行器闸门**：执行器取文件（`LoadRunnableManifestBundle` → `manifestbundle.RequireValidForRun`，agent 模式由平台在 `GetTaskData` 里执行）先做 `VerifyForUse`，再 `RequireValid`：`bundle_hash` 为 NULL 的版本（规则违规或 `hash_mismatch`）一律不交给 Terraform，plan / apply / drift 任务失败，错误为 `bundle_republish_required: <reason>`（`*RepublishRequiredError`）。uninstall 不受影响：它只解绑元信息（清 workspace 的 manifest 三列与 manifest 资源行），不创建任务；之后在 workspace 跑的 Plan+Apply 不再加载 manifest 文件，按 state 销毁残留资源，bundle 代码（包括 destroy-time provisioner、`.terraformrc` 等）不会被执行，所以无效版本无需确认即可 uninstall。

**暂未覆盖（后续步骤）**：
- ~~Agent 模式 `RemoteDataAccessor.GetManifestFilesByTag` 仍不支持~~ —— step 4 已改为 bundle 交接（§3.4）。
- 编辑器 ExternalFiles 的「Run」仍是 workspace 上的 plan 任务（step 6 改为 sandbox 预览 run），但 step 4 起文件在建任务时校验、执行时复核（§3.4）。

### 3.4 Runner（step 4）

**接口**（`services/runner.go`）：`Runner{Kind, Supports(purpose), Start(RunRequest)}`，任务队列按 workspace 执行模式选 runner，统一经 `StartRun` 投递：
- `LocalRunner`：现有本进程 executor（CAS 置 running → `executeTask`）；
- `AgentRunner`：现有 agent / K8s 驱动（`pushTaskToAgent`，C&C 推给 agent，agent 用同一个 `TerraformExecutor` + `RemoteDataAccessor`）；
- `SandboxRunner`：step 6 占位，只支持 preview，`Start` 返回 `ErrSandboxRunnerNotImplemented`。

规格里的四步（prepare → exec → fetch → destroy）仍在 executor 内部完成，step 4 只抽投递接口，不重写执行流程；sandbox 实现时再按四步拆。

**purpose 与锁**：`plan`、`drift_check` 为 preview；`plan_and_apply`（plan 与 apply 两个阶段）与 `apply` 为 approval。`StartRun` 拒绝未持 workspace 锁的 approval 投递（`ErrApprovalRequiresWorkspaceLock`）；`TryExecuteNextTask` 与 `ExecuteConfirmedApply`（原先不加锁）都先拿 workspace advisory lock（`WorkspaceLockKey`）、锁内复查任务状态再投递。HTTP state backend 下只有 preview 的 `init` / `plan` 带 `-lock=false`，approval 的 plan 现在持 state 锁（原先 `plan_and_apply` 的 plan 也是 `-lock=false`）。顺带修了 `pglock`：advisory lock 是会话级的，原实现 lock / unlock 走连接池里任意连接，unlock 可能落在别的会话上导致锁泄漏，同进程第二次 `TryLock` 也可能在同一会话上重入、被当成拿到锁；现在每把锁从 `TryLock` 到 `Unlock` 独占一个连接。

**bundle 交接**：执行器不再拿「文件列表」，而是 `DataAccessor.GetManifestBundleByTag` 返回的 `ManifestBundleHandoff`（平台侧 `RequireValidForRun` 之后的确定性 tar 归档 + `bundle_hash`）。Local 进程内构建；agent / K8s 由 `GetTaskData` 下发 `manifest_bundle`（`archive_b64`、`bundle_hash`、`version_id`、deployment/tag），平台拒绝时下发 `manifest_bundle_error`，agent 还原为同一个 `bundle_republish_required: <reason>`（同一 `error_code`）。`GetTaskData` 同时下发 workspace 的 `manifest_deployment_id` / `manifest_active_tag` / `manifest_subpath`：此前 agent 收不到这三列，会把 manifest workspace 当 UI workspace 生成空 `main.tf.json` 去 plan（可能销毁 manifest 资源）；现在缺 bundle 时任务直接失败，不回退。

落盘统一走 `manifestbundle.Unpack`：工作目录先清空；每个条目过 `ValidatePath`；只接受普通文件和目录（符号链接、硬链接、设备、fifo 等一律拒绝）；文件用 `O_CREAT|O_EXCL|O_NOFOLLOW` 创建，父目录逐级 `Lstat` 校验（已有的非目录、符号链接拒绝）；`MaxFileSize` / `MaxBundleSize` / 条目数（`MaxFiles`，目录也计入）在读 header 时累计检查、超限立即停止，不再读后续内容；全部写完后 `Hash` 必须等于 `bundle_hash` 才继续（之后才到 `init`），不一致报 `ErrIntegrity` 并记 `[SECURITY]` 日志；空哈希直接 `bundle_republish_required`。版本本身的 `hash_mismatch` 记录仍在平台侧 `RequireValidForRun` 完成（agent 不写库）。

**变量**：任务的 override 快照（`variable_overrides` + `sensitive_keys`）经 `GetTaskData` 的 task 对象下发，`RemoteDataAccessor.SetVariableOverrides` 不再是空操作，不走环境变量、不进日志。override 规则（`ApplyVariableOverrides`）与 tfvars / `variables.tf.json` 生成（`RenderTFVars` / `VariablesTFJSON`，只含 Terraform 变量、按 key 排序）由 local、agent、快照 apply（以及之后的 sandbox）共用，测试保证 local 与 agent 输出逐字节一致（含 override 与敏感值）。override 不再把变量降为非敏感：原变量敏感或 `sensitive_keys` 判定敏感（NULL = 全部敏感）即声明 `sensitive`。

**编辑器 Run（external_files）**：建任务时校验（`PrepareManifestRunFiles`），不通过不建任务。不带 `manifest_version_id` 视为草稿，必须通过发布规则（bundle 规则 + HCL 静态检查，同一 module source 白名单），否则 422 `bundle_rules_violated`；带 `manifest_version_id`（编辑器 `?version=` 打开的已发布版本）时对该版本做 `RequireValidForRun`（重算；NULL → 409 `bundle_republish_required`，篡改 → 记 `hash_mismatch`），且文件哈希必须等于其 `bundle_hash`（否则 409 `bundle_hash_mismatch`）。服务端只比对、不返回版本内容，所以不新增读路径、不需要额外 IAM。通过后文件连同 `bundle_hash` 固化在 `external_files`，执行器走与部署 bundle 相同的归档 + `Unpack` 交接，落盘哈希不符即失败；没有 `bundle_hash` 的旧 Run 任务拒绝执行（需重新发起）。编辑器读（`ListFiles` / `ReadFile ?version=`）仍按设计不校验，校验只在 run。前端需在从版本视图 Run 时带上 `manifest_version_id`，并展示 422 / 409。

**plan 脱敏**：`RedactPlanJSON` 按 plan 格式自带的敏感标记脱敏（`resource_changes` / `resource_drift` / `output_changes` 的 `before_sensitive` / `after_sensitive`，`planned_values` / `prior_state` 的 `sensitive_values` 与敏感 output，`configuration` 中声明 sensitive 的变量值与 default），并把 `configuration.provider_config` 里的常量全部替换（provider 块里的凭证 Terraform 不标记）。脱敏发生在 `terraform show -json` 之后、任何使用之前：变更统计、agent 上传的 resource changes、`plan_json` 入库（local 保存、agent 上传接口在平台侧再脱敏一次以兼容旧 agent、plan parser 的 plan_data 回退路径）都只见脱敏结果；run task 回调与 UI 读到的也是脱敏后的 plan。`RedactedPlanHash` / `RedactPlanForStorage` 在脱敏后的 plan 上算 `plan_hash`（`sha256("terranova-plan-v1" 0x00 规范 JSON)`），供 step 6/7 写 `manifest_runs.plan_redacted` / `plan_hash`；`workspace_tasks.plan_hash` 仍是 `plan.out` 二进制文件的哈希（apply 复用工作目录时校验文件用），语义不变。

**平台侧敏感集合**：`RedactPlanJSON(plan, ps)` 在 HCL `sensitive = true` 之外再并上平台侧敏感集合 `PlanSensitivity`：workspace 敏感变量、varset 敏感变量（含 active deployment 的 varset，均来自任务变量快照）、deployment overrides 中 `sensitive_keys` 列出的键（NULL = 全部 override 敏感）。按变量名脱敏 `variables[name].value` 与 root module 的 `default`；按值把 plan 中任何等于（≥4 字符）或包含（≥8 字符）这些敏感值的字符串叶子整体替换（HCL 格式值取其字符串叶子），覆盖未在 HCL 中声明 sensitive 的变量流入资源属性 / output 的情况；派生值（编码、哈希、拼接拆分）无法识别，HCL 声明 sensitive 仍是可靠手段。执行器（local / agent）用与 tfvars 相同的变量来源（快照 + overrides）计算；平台侧（agent 上传 plan_json、plan parser 回退、历史回填）用 `PlanSensitivityForTask` 按任务的变量快照 + override 快照计算。脱敏标记统一为 `(sensitive value)`，与 Terraform CLI 及前端（PlanCompleteView / ApplyingView / StateResourceViewer）显示一致。

### 3.5 Git 来源（step 8，迁移 `20261010_14_manifest_git_source`）

**原则**：只在发布时由平台拉取，钉到一个完整 commit SHA；拉到的树走与 native 草稿完全相同的发布规则（`ValidateForPublish`：路径 / 黑名单 / 大小 / secret scan / HCL 检查 / module source 白名单），存为不可变 bundle（`bundle_hash`，`source_ref = SHA`）。run、部署、审批、apply 只用存储的 bundle，永远不拉 git、不需要 git 凭证。

**创建**（`POST /organizations/{org_id}/manifests`，MANIFESTS WRITE）：`source_type: "git"` + `git_repo_url`（必须是 `<GITHUB_URL>/<owner>/<repo>[.git]`，https、无凭证 / query / fragment，存为规范形式）+ `github_installation_id`（必须已由本组织 ADMIN 登记，且其 account 就是仓库 owner）+ 可选 `git_subpath`（bundle 根目录，空 = 仓库根；不能有 `.` / `..` / 空段 / 开头 `/` / 开头 `-`）。创建时用单仓库 token 验证仓库可访问。之后不可改来源。GitHub App 未配置 → 503 `git_source_disabled`。

**编辑器只读**：git manifest 的草稿写接口（PUT / DELETE 文件、move、delete_dir、reset_from）→ 409 `git_source_read_only`（`ManifestNativeOnly` 中间件）。

**发布**（`POST .../v2/versions`）：git manifest 必须给 `commit_sha`（40/64 位小写 hex；分支名、短 SHA 都拒绝），native 给了则 400。流程：事务外签 token、拉取（`gitsource.Fetcher`）→ 事务内与 native 相同的 `PackFilesForPublish` + fetcher 报告的树级 problem 合并 → 建版本（`source_ref = SHA`，`changelog` 默认取 commit 标题）→ `Store`。错误：422 `git_repo_not_accessible` / `git_commit_not_found` / `git_subpath_not_found`，502 `git_fetch_failed`（git 输出只记日志，已去除 token），422 `bundle_rules_violated`（`error: "commit violates the bundle rules"`）。

**拉取**（`internal/gitsource.Fetcher`，git CLI）：临时 bare 仓库 `fetch --depth=1 --no-tags --no-recurse-submodules <clone URL> <sha>`，`rev-parse <sha>^{commit}` 必须等于 SHA；不 checkout：`ls-tree -r -z -l` 列树、`cat-file --batch` 读 blob，所以 hooks、过滤器（LFS smudge）、符号链接、submodule 都不会落盘。符号链接 / submodule / 超过 `MaxFileSize` 的条目直接记 problem 且不读内容；条目数 > `MaxFiles`、总大小 > `MaxBundleSize` 同样提前拒绝。git 进程：`credential.helper=` 清空、`core.hooksPath=/dev/null`、`http.followRedirects=false`、只允许 https（配置的 `GITHUB_URL` 是 http 时只允许 http，供测试）、`GIT_CONFIG_NOSYSTEM`、`GIT_CONFIG_GLOBAL=/dev/null`、`HOME` 指向临时目录、默认 2 分钟超时。

**凭证**：GitHub App installation token，每次操作（创建校验、发布、列分支 / commit）现签：`POST /app/installations/{id}/access_tokens`，`repositories = [该仓库]`、`permissions = {contents: read}`；返回的权限若超出 `contents`/`metadata` 的 read 或不止一个仓库即撤销并拒绝；用完 `DELETE /installation/token` 撤销（约 1 小时自然过期）。token 只在内存：不落库、不记日志；只经 `GIT_ASKPASS`（临时目录里的脚本，打印 git 进程环境变量 `TERRANOVA_GIT_TOKEN`）交给 git，从不进 URL 或 argv；git 的错误输出先去掉 token 再返回；`gitsource.Token` 用任何格式化动词打印都是 `***`。App 凭证：`GITHUB_APP_ID` + `GITHUB_APP_PRIVATE_KEY`（PEM 或其 base64）/ `GITHUB_APP_PRIVATE_KEY_FILE`，经 `gitsource.CredentialsProvider`（默认读环境变量，同 `keys.KeyProvider` 模式，可换成 KMS 实现）；`GITHUB_URL`（默认 `https://github.com`）、`GITHUB_API_URL`（默认 `https://api.github.com`，GHES 为 `<GITHUB_URL>/api/v3`）。未配置 → git 来源整体禁用（`git_source_disabled`）。

**installation 登记**（组织 ADMIN，`ORGANIZATION` 资源）：`GET/POST /organizations/{org_id}/github-app/installations`、`DELETE .../{installation_id}`。登记时用 App JWT 查询 installation，`account_login` 以 GitHub 返回为准；一个 installation 只能属于一个组织（全局唯一，别的组织已登记 → 409）；有 git manifest 在用时不能删除（409）。表 `github_app_installations`（FK organizations ON DELETE CASCADE）。

**commit 选择器**（MANIFESTS WRITE，只给能发布的人）：`GET .../manifests/{id}/git/branches`、`GET .../git/commits?ref=&per_page=`（GitHub REST，token 在 `Authorization` 头）。

**webhook**（`POST /api/v1/webhooks/github`，无登录，签名即认证）：`X-Hub-Signature-256` 用 `GITHUB_WEBHOOK_SECRET` 做 HMAC-SHA256、`hmac.Equal` 比较；无签名 / 错误 → 401，未配置 secret → 503，body 上限 5 MB。`ping` → 200；`push`：installation 与规范化仓库 URL 都匹配的 git manifest 写 `git_latest_sha` / `git_latest_ref` / `git_latest_at`（「有新 commit 可发布」提示），**从不自动发布、也不拉取**；其他事件 202 忽略。

**module source**：bundle 内的 git module 必须钉 SHA（`hcl_module_unpinned`，native 与 git 发布都适用；仅发布时检查，已有版本不受影响），且仍须在平台 module 目录白名单内；否则 vendor 进 bundle（本地相对路径）。run 不注入任何 git 凭证，因此私有仓库的 module 必须 vendor（钉 SHA 的公共 module 在 `init` 时照常下载）。

**数据库**：迁移 `20261010_14_manifest_git_source`（只增，可重复）：`github_app_installations`（`installation_id bigint` 全局唯一、`> 0`，`account_login`，`created_by`）；`manifests.git_latest_sha varchar(64)`、`git_latest_ref varchar(255)`、`git_latest_at timestamptz`，`chk_manifests_git_latest`：三列全空，或 `source_type='git'`、SHA 为 40/64 位小写 hex 且时间非空。

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
7. 审批与 apply 双哈希校验（已完成，见 §2）
8. Git 来源（平台侧按 SHA 拉取打包；module `ref` 必须是 SHA 或 vendor；webhook 验签）—— 已完成，见 §3.5

## 10. AgentCore 上线前需实测
- Code Interpreter 能否自带 terraform 与 provider 二进制（或改用 AgentCore Runtime 自定义镜像）。
- 单个 session 最长运行时间是否覆盖大 workspace 的 plan。
- VPC 模式下到 provider mirror 与目标云 API 的路径（VPC 端点 / NAT + 白名单）。

## 11. 实现中的变更（2026-10-10）
1. **uninstall 不跑 destroy**：只解除 workspace 与 manifest 的绑定，资源由用户在 workspace 上自行 Plan + Apply 删除。哈希为 NULL 的版本（违反规则或 `hash_mismatch`）在执行器上一律不能运行（plan/apply/drift），返回 `error_code=bundle_republish_required`，没有例外。原定的净化后 destroy、`untrusted_bundle_confirm_required` 确认流程不做；以后若加 destroy 任务类型再启用。
2. **`plan_data` 保留期**：信封加密，`PLAN_DATA_TTL` 默认 7 天，apply 成功、任务终态或过期即删除；过期后 apply 返回 `plan_expired`。
3. **agent 能力门槛**：绑定 manifest 的 workspace、带外部文件或 override 的任务，只派给上报了 bundle 校验等能力位的 agent；没有就返回 `agent_upgrade_required`（`error_reason` 写缺少的能力名）。
4. **tfvars** 统一由 `encoding/json` 生成 `terranova.auto.tfvars.json`，三种 runner 共用。
5. **插件缓存** 每个任务一份，`init` 不带 `-upgrade`，强制 lock 校验；sandbox 不挂宿主机缓存。
6. **资源变更** 由平台从脱敏后的 `plan_json` 解析，不信任 agent 上传；agent 访问任务的接口统一校验 `task.agent_id` 与任务状态。

## 12. 待定：provider mirror
第 6 步依赖它：AgentCore 只允许 VPC 模式，VPC 默认无公网，sandbox 的 `init` 需要内部 `network_mirror`（否则就得开 NAT 放行 registry，等于放宽出网）。平台 CLI 配置的 `provider_installation` 只配 `network_mirror`、不留 `direct`。
