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
| `manifest_versions` | `bundle_hash varchar(64)`、`source_ref varchar(64)` | `bundle_hash` 为 NULL 或 64 位小写 hex；`source_ref` 为 NULL 或 40/64 位小写 hex（git SHA-1/SHA-256）。`bundle_hash` 保持可空，便于新旧版本混跑时滚动上线；发布（PublishVersion）在同一事务内写入，存量由迁移回填。均不出 JSON。 |
| `manifest_deployments` | `approved_bundle_hash`、`approved_plan_hash`（varchar(64)） | 审批接入前恒为 NULL（step 7 使用），不出 JSON。 |
| `manifest_deployments` / `workspace_tasks` | `sensitive_keys jsonb`（可空、无默认值） | 覆盖值为敏感的 key 列表（任务行是部署覆盖快照的同一标记）。迁移不回填、不批量标记；NULL = 尚未计算，API 一律按全部敏感处理（不返回任何值）。由启动时的 Go 回填任务按部署时同一敏感判定写入（见设计文档 §8.4）。不出 JSON。 |
| `sandbox_sessions`（新） | `id`、`user_id`、`workspace_id`、`provider`、`network_mode DEFAULT 'vpc'`、`status`、`expires_at`、`closed_at`、时间戳 | `chk_sandbox_sessions_network_mode`：只允许 `vpc`（§6.2 的数据库兜底）；`UNIQUE(id, workspace_id)` 供复合外键使用。 |
| `manifest_runs`（新） | `manifest_id`（FK CASCADE）、`version_id`（FK SET NULL，草稿预览为空）、`bundle_hash NOT NULL`、`workspace_id`、`runner`、`purpose`、`status`、`plan_hash`、`plan_redacted jsonb`、`state_serial`、`session_id`、`created_by` | runner ∈ agent\|sandbox，purpose ∈ preview\|approval；sandbox ⇒ purpose=preview 且 session 非空；`bundle_hash` 64 位 hex；`(session_id, workspace_id)` 复合 FK → session，run 不能跨出其 session 的 workspace；`UNIQUE(id, workspace_id, purpose)`。 |
| `run_tokens`（新，§9 step 5 的表，提前建） | `run_id`、`session_id`、`workspace_id`、`purpose`、`token_hash`、`expires_at`、`revoked_at` | `token_hash` 唯一；purpose ∈ preview\|approval；`(run_id, workspace_id, purpose)` 复合 FK → run（ON DELETE CASCADE），token 的 workspace/purpose 必须与 run 一致；`(session_id, workspace_id)` 复合 FK → session。另有按 session 未撤销 token 的部分索引，供 session 关闭时批量写 `revoked_at`。 |

- `status` 列（session、run）不加 CHECK：后续新增状态不需要删约束。
- run 暂不关联 deployment（`deployment_id` 留到 step 7 再定）。

### 3.2 bundle_hash 编码（`internal/manifestbundle`）
```
sha256( "terranova-bundle-v1" 0x00
        对每个文件，按 path 字节序排序：path 0x00 十进制(len(content)) 0x00 content )
```
输出小写 hex。path 为 `manifest_files.path`（相对路径，不能为空、不能重复）；长度前缀保证二进制内容（可含 NUL）无歧义；空 bundle 也有确定的哈希。native 版本的文件集 = `manifest_files WHERE version_id = 版本 id`（草稿行 `version_id IS NULL`，不参与）。改编码必须换版本前缀，不得原地修改。迁移回填、发布与后续 run/审批都只用 `manifestbundle.Hash` / `VersionHash`；SQL 补丁里的等价回填（`ORDER BY path COLLATE "C"`）与 Go 实现由黄金向量测试锁定。

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
