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
- `sandbox_sessions`（新）：user + workspace 绑定、provider、过期时间、只读 state token、只读 STS 角色；三者同寿命，过期一起回收。
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
1. 凭证：sandbox 只拿只读短期 STS（assume 只读角色），绝不注入 workspace 的 apply 凭证；state token 只读，服务端拒绝 POST/LOCK。
2. AgentCore 只允许 VPC 网络模式，Sandbox / Public 模式代码层直接拒绝；AWS 侧配不了 VPC 端点则本地模式不上线。
3. 出网默认拒绝，只放行 provider mirror / registry 白名单与目标云 API；DNS 走 Route 53 Resolver DNS Firewall。
4. 禁止访问 `169.254.169.254`、平台 API（state 读取通道除外，见 §7）、数据库、K8s API；`automountServiceAccountToken: false`；节点强制 IMDSv2、hop limit 1。
5. `init` 校验 `.terraform.lock.hcl` checksum，走内部 mirror。
6. plan JSON 与 state 一样含 `sensitive` 值：入库前脱敏或加密，前端只展示脱敏结果。
7. git：GitHub App 只读、钉 SHA、webhook 验签；发布前检查 provisioner、`external` data source、module source 白名单。

## 7. 待定：sandbox 读 state 的通道
§6.4 禁止 sandbox 访问平台 API，但 HTTP state backend 就在平台 API 上，两条冲突。二选一：
- **A（推荐）**：`prepare` 由平台把该 workspace 的 state 快照（固定 serial）推进 sandbox，plan 用本地 state 跑。sandbox 不需要任何回平台的网络，也不需要 state token；`state_serial` 天然就是预览基准。
- **B**：只放行一个独立的只读 state 入口（单路径、只 GET），其余平台 API 仍拒绝。
选 A 时 `sandbox_sessions` 去掉只读 state token 字段。

## 8. 前端
- 新建 manifest 先选「平台内编辑」或「Git 仓库」，选后不可改；列表带来源标记。
- git 项目编辑器只读，提示「修改请走 Git」；版本列表显示 commit SHA 与提交信息，发布 = 选 commit。
- 编辑器里是「预览 plan」：跑在 sandbox，标「仅供预览」，无审批按钮；先选 workspace（只列有 state 读权限的），顶部显示 session 剩余时间，过期提示重新开始，不后台续期。
- 部署面板里是「生成待审批的 plan」：跑在 agent，审批和 apply 只从这里进。
- 两边变更只显示脱敏值。加载预算沿用 Phase 1：列表不预载 schema/模块/React Flow/Monaco；Deploy 两步（先可写 workspace，再 varset 与预览）；编辑器按 tab 读文件。

## 9. 实现顺序（`feat/manifest-sandbox`，每步一提交、带测试、推远程）
1. IAM（Phase 1 锁定版，独立可先合）
2. 迁移 + 回填
3. Bundle
4. Runner 接口 + K8s 实现（approval 必须加锁；先脱敏再算 `plan_hash`）
5. 只读凭证（只读 STS；若 §7 选 B 再做只读 state token）
6. AgentCore provider + session 接口（仅 VPC）
7. 审批与 apply 双哈希校验
8. Git 来源

## 10. AgentCore 上线前需实测
- Code Interpreter 能否自带 terraform 与 provider 二进制（或改用 AgentCore Runtime 自定义镜像）。
- 单个 session 最长运行时间是否覆盖大 workspace 的 plan。
- VPC 模式下到 provider mirror 与目标云 API 的路径（VPC 端点 / NAT + 白名单）。
