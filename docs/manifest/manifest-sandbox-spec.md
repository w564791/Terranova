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
- `sandbox_sessions`（新）：user + workspace 绑定、provider、过期时间；过期时其下所有平台 run/shell token 一起回收。客户云身份由用户自备，平台不注入 STS。
- 部署：`approved_bundle_hash`、`approved_plan_hash`。

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
1. 凭证：参考 GitLab Terraform 方案。**平台侧**只发按 run / shell 签发的平台 token（类似 `CI_JOB_TOKEN`，复用 `StateTokenService`）：绑定 run/session + workspace，只能访问 state backend 和该范围所需接口，结束即撤销；`preview` token 在中间件拒绝 POST/LOCK。**绝不**注入 workspace 的 apply 凭证，也**绝不**由平台 AssumeRole / 注入客户云 STS（客户云由用户在外部 sandbox 自备，见 §10.2）。
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
5. 按 run 签发的 token：在现有 `StateTokenService` 上改。新表 `run_tokens`（`run_id`、`session_id`、`workspace_id`、`purpose`、`token_hash`、`expires_at`、`revoked_at`），`workspace_tasks` 旧字段不动；JWT 加 `typ` claim（`task`/`run`），`ValidateToken` 分支：`run` 路径数据库出错即拒绝（不退回只验 JWT），过期时间取 min(run 超时, session 过期)，`preview` 在中间件拒绝 POST/LOCK/UNLOCK；`task` 路径行为暂不变，另行评估。run 结束写 `revoked_at`（只撤销该 run 的 token，session 保留到自身过期或用户关闭，届时按 `session_id` 批量写 `revoked_at`）；私有 module 的 installation token 仍可按需注入（`GIT_ASKPASS`）；**不**注入客户云 STS
6. AgentCore provider + session 接口（仅 VPC）
7. 审批与 apply 双哈希校验
8. Git 来源（平台侧按 SHA 拉取打包；module `ref` 必须是 SHA 或 vendor；webhook 验签）

## 10. AgentCore 上线前需实测
- Code Interpreter 能否自带 terraform 与 provider 二进制（或改用 AgentCore Runtime 自定义镜像）。
- 单个 session 最长运行时间是否覆盖大 workspace 的 plan。
- VPC 模式下到 provider mirror 与目标云 API 的路径（VPC 端点 / NAT + 白名单）。

### 10.1 参考：k8s-infra AgentCore（2026-10-18）
Jinlong 已在 k8s-infra 跑通 AgentCore session 流程。场景不同，按下表取用。第 6 步按 2026-10-25 决议：**外部建沙箱、平台只登记**；Shell 与自定义镜像按 10.2。

| 模块 | 处理 | 说明 |
| :--- | :--- | :--- |
| driver 抽象 / 冷启动 / 释放 | 参考 | 生产不跑 DockerDriver；外部沙箱 + 平台登记 |
| `taskRunning` / `runSeq` | 复用思路 | plan / Shell 进行中不释放；DB 租约 + 心跳 |
| 30 天个人 Token | 不用 | `typ=run` / `typ=shell` 短 token |
| 热更新 workspace / 跨 run 复用容器 | 不用 | 冻结 bundle；一次任务一份干净环境 |
| Shell 与 preview 共用同一凭据会话 | 不用 | 见 10.2 拆分 |

### 10.2 第 6 步最小切片（2026-10-25 锁定）：登记 + Shell + 基础镜像

**原则**：沙箱由用户外部创建，平台不管容器内部生命周期；`DELETE` 只解绑平台记录。
平台 Bedrock/AgentCore 接入可放 Bot vault（仅 1:1 安全表单）；**客户云身份不进 Bot vault、不进聊天、不进镜像**。

#### Warm pool（runtime / harness 常驻）
- 外部 AgentCore **runtime + harness 可常驻**，只为避免冷启动；新建 session = **唤醒**，不是平台冷拉容器，也不随 session 拆掉 runtime。
- 表字段拆分：可选 **`runtime_ref`**（常驻层，你外部维护） vs **`sandbox_ref` / session**（可唤醒会话）。常驻健康与 session 状态在 UI 分开展示。
- 常驻**只省冷启动，不省隔离**。硬规则：
  1. Runtime/harness **不得缓存** `typ=shell` / run token / 任何客户云密钥；平台票只活在 session；关 session 必须吊销且进程内清掉。
  2. 唤醒 = 签发平台新票 + 注入本 run 冻结 bundle（再校哈希）+ 空 workdir（或按 `session_id` 隔离目录）；**不**注入客户云 STS。
  3. **禁止**复用上一 session 的 `.terraform`、plugin cache、环境变量 / state 脏目录。

#### 表
- **`sandboxes`**：`id, org_id, workspace_id, sandbox_ref, runtime_ref?, label, driver, image_digest, status, lease_expires_at, created_by, created_at, updated_at`（`assume_role_arn` / `assume_external_id` 若已落库可保留但本切片不使用）；唯一 `(org_id, sandbox_ref)`。
  - `driver`：`agentcore_external`（preview，无 Shell）| `shell_debug`（UI Shell）。
  - `status`：`registered` →（握手后来拉成功）`ready` → `lease_expired` | `unbound`。
  - `sandbox_ref`：`^[A-Za-z0-9][A-Za-z0-9._:-]{1,127}$`，禁止 `://` / 任意 URL；另做账号/区域白名单。
  - `runtime_ref`：可选，指向外部常驻 runtime/harness（同格式约束，禁 URL）。
- **`shell_sessions`**：`id, sandbox_id, org_id, status, expires_at, created_by, …`；与凭据同生共死。

#### API
- `POST /api/v1/orgs/:org_id/sandboxes` `{ workspace_id, sandbox_ref, driver, label?, image_digest? }` → 201，初始可 `ready`（有 vault）或等握手。
  - 权限：目标 workspace `WORKSPACE_RESOURCES` WRITE；跨 org → 404；缺平台 AgentCore vault → `422 vault_credential_required`；非法 ref → `422 invalid_sandbox_ref`。
- `GET` 列表/详情；`DELETE` → 解绑 + **吊销该沙箱未过期 run/shell 凭据**，不毁外部沙箱。
- 控制面：**沙箱主动来拉**（同现有 agent）：短 ticket → 换 token / 拉 bundle；平台不存回调 URL、不用 vault 凭据打外部地址。
- **唤醒 / 数据面：** 平台侧只走 **InvokeHarness**（或 AWS SDK 等价入口），**禁止**直接 `invoke-agent-runtime`。CLI 若缺 `invoke-harness`，用 SDK；不得为了图方便绕成 PUBLIC 直连。

- `POST /sandboxes/:id/shell-sessions` → 签发平台 `typ=shell`（claim 含 `shell_session_id`），权限=该用户平台权限；可只读 state、不拿锁；**不**附带客户云凭证；寿命 = 会话寿命（硬顶建议 ≤15min），心跳同步续租且不超过会话过期。
- `DELETE /shell-sessions/:id`（超时/断线宽限同）→ **先吊销该会话全部 shell 凭据**，再标结束。禁止凭据挂到第二个 Shell；禁止会话结束后凭据残留。
- `POST /shell-sessions/:id/plans` → 创建历史任务 `task_type=shell_plan`（`-lock=false`），可查日志/红action结果；无 mirror → `503 provider_unreachable`。状态保持 `ready`，创建沙箱不因缺 mirror 失败。

#### 硬规则
1. Preview / `shell_plan`：平台侧只读能力 + 用户自备云身份；`-lock=false`；**不得**进入审批哈希。apply 只走 runner 审批路径。
2. `typ=run` 与 `typ=shell` 不得混用；平台 AgentCore 接入与客户云凭据分槽；**平台不注入客户云 STS**（见上）。
3. 禁 `apply` / `destroy` / `state push` / 拿锁；入口包装 + agent 双拦。
4. 出网白名单；禁 `external` / `local-exec`；不因只读放松。
5. 每次执行 / 冷握手重查 org + workspace 权限；记录带 `org_id`。
6. 自定义 **Dockerfile** 单独镜像流水线（底包含 terraform 等）；凭据不进镜像；登记可选 `image_digest`，握手校验；**不等于**生产启用 DockerDriver。
7. `shell_plan` 与外部 preview plan 均不可作 Deploy approval 输入。
8. **网络模式：** runtime/harness 同时支持 VPC 与 PUBLIC，**仅按 `APP_ENV` 分叉**——`production` 只收 VPC，`PUBLIC` → `422 network_mode_not_allowed`；非 production 允许登记 PUBLIC。不设 `IAC_AGENTCORE_ALLOW_PUBLIC`（也不接受请求头/客户端开关）。即使测试环境已登记 PUBLIC，绑定生产 workspace / Deploy 时仍按存档 `network_mode` 再拦一遍。前端黄条不够，API 必须拒。




#### 身份模型（2026-10-25 用户改向 · 平台 token，不注入客户云凭证）
**否决**：平台对客户云做 AssumeRole，或把客户 AK/SK/SessionToken 注入 sandbox / AgentCore（进程环境、InvokeHarness 消息体、浏览器创建响应、镜像、Bot vault、聊天）——全部禁止。

1. **平台 token（要做）**：开 Shell / sandbox session 时，在**用户个人名下**签发平台 `typ=shell`（或等价）token，权限 = 该用户在 Terranova 上自己的权限（读 state、拉 bundle、登记范围内的只读接口等）；claim 绑 `shell_session_id`；寿命 = 会话寿命（硬顶 ≤15min）；关会话 / 超时 / 解绑 → **同事务吊销**；用户可自行吊销。与 `typ=run` 分轨。
2. **客户云凭证（用户自备）**：类似本机 TFE / `credentials.temp`——用户在外部 sandbox 里自己配置云身份。平台**不存、不传、不注入、不代管**客户云密钥。
3. **Bot vault：** 只放平台调 Bedrock/AgentCore 的接入；客户云密钥不进。本设计讨论房禁止粘贴任何云密钥。
4. **`shell_plan`：** 可进任务历史；云侧能否真正 plan 取决于用户自备身份；**仍不进审批哈希、禁 apply**。InvokeHarness 只负责唤醒/会话，消息中不得带任何云密。
5. **历史字段：** `assume_role_arn` / `assume_external_id` / `cloud_readonly` 若表上仍保留，本切片**不作为开会话前置、不做注入**；后续若产品再开「平台代管客户云身份」另起一刀，默认关闭。

#### 前端
- 登记流（非代建）；解绑二次确认「只删平台登记」。
- Shell 入口只挂 `shell_debug`；与 `agentcore_external` 分开。
- 任务历史展示 `shell_plan`，标「预览 / 来自 Shell，不可用于 apply」。
- 无 mirror 时 plan 明确 `provider_unreachable`。
- 状态机区分：runtime 健康（常驻） vs session（唤醒中·秒级 / 活跃 / 休眠 / 已解绑）；runtime 不可用单独告警，不与单个 session 失败混写。

**仍前置**：provider mirror（第 12 节）才能真正跑通 sandbox/`shell_plan` 的 `init`；不开 NAT。

#### 实现约定（库表）
**禁止向用户下发可执行 DDL / 手工 SQL。** 新表、索引、约束一律走项目 migrate（或后端启动自动迁移）；文档与聊天只描述模型字段与行为，不贴可让用户复制执行的 `.sql`。运维只读统计脚本若必须保留，须标明只读、且不作为建表途径。


## 11. 实现中的变更（2026-10-10）
1. **uninstall 不跑 destroy**：只解除 workspace 与 manifest 的绑定，资源由用户在 workspace 上自行 Plan + Apply 删除。哈希为 NULL 的版本（违反规则或 `hash_mismatch`）在执行器上一律不能运行（plan/apply/drift），返回 `error_code=bundle_republish_required`，没有例外。原定的净化后 destroy、`untrusted_bundle_confirm_required` 确认流程不做；以后若加 destroy 任务类型再启用。
2. **`plan_data` 保留期**：信封加密，`PLAN_DATA_TTL` 默认 7 天，apply 成功、任务终态或过期即删除；过期后 apply 返回 `plan_expired`。
3. **agent 能力门槛**：绑定 manifest 的 workspace、带外部文件或 override 的任务，只派给上报了 bundle 校验等能力位的 agent；没有就返回 `agent_upgrade_required`（`error_reason` 写缺少的能力名）。
4. **tfvars** 统一由 `encoding/json` 生成 `terranova.auto.tfvars.json`，三种 runner 共用。
5. **插件缓存** 每个任务一份，`init` 不带 `-upgrade`，强制 lock 校验；sandbox 不挂宿主机缓存。
6. **资源变更** 由平台从脱敏后的 `plan_json` 解析，不信任 agent 上传；agent 访问任务的接口统一校验 `task.agent_id` 与任务状态。

## 12. 待定：provider mirror
第 6 步依赖它：AgentCore 只允许 VPC 模式，VPC 默认无公网，sandbox 的 `init` 需要内部 `network_mirror`（否则就得开 NAT 放行 registry，等于放宽出网）。平台 CLI 配置的 `provider_installation` 只配 `network_mirror`、不留 `direct`。

## 13. 收尾备忘（2026-10-10）
下次接着做时优先核对：
- **密钥 PR**：加密用独立 `DATA_ENCRYPTION_KEY`；签名用独立 `SIGNING_ROOT_KEY`（不得从 `JWT_SECRET` 派生），HKDF 分 user/state/agent/run/runtask；`LEGACY_TOKEN_CUTOFF` 硬截止；删 `JWT_SECRET` 前须确认无旧 `key_version` 行。
- **reserved env**：保存 422 + 执行器再剥；`HTTP(S)_PROXY`/`NO_PROXY` 改平台配置注入；前缀接口与剥除用同一份常量。
- **git**：`git_subpath` 经 `ValidatePath`；来源字段不可变（handler + DB trigger）；GitHub host 只取平台配置。
- **TLS**：泄露的 mkcert 证书已删；生产禁 `InsecureSkipVerify` / 明文 HTTP（除显式 cluster-internal）。
- **第 6 步**：登记 + Shell 按 §10.2 推进；真正 `init` 仍卡在内部 `network_mirror`。
