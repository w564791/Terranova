# IAM 安全审查完整报告（代码核实版）

> **日期**：2026-07-17  
> **审查方式**：对照未提交工作区源码通读 + 路径级证据链（非仅文档宣称）  
> **关联文档**：`32-iam-remediation-report.md`（产品 SoT）、`33-iam-optimization-code-review.md`、`34-iam-fix-progress-report.md`  
> **审查结论一句话**：**主路径 Critical 修复多数有效，但整改处于半完成态；R-S03 / Application 管理 / Role 策略面 / Run Trigger / vsnap 仍有可利用缺口。`34` 不可作为完成证明。**  
> **修复进度（2026-07-17 续）**：  
> - **Wave A**：Application org 绑定、vsnap/task 绑定、Run Trigger CUD 绑 source、系统 Role 策略只读、guard fail-closed、Resource 版本/回滚/依赖绑 path、ValidateToken schema、Team Token 禁 NULL expires、坏测试 panic 消除。  
> - **Wave B（部分）**：Team Token 事务化生成 + 唯一索引 SQL（已可执行）；临时权限真实邮箱 lookup；任务 ConfirmApply/GetTask/RetryStateSave/CancelPrevious/DownloadBackup 统一 `loadTaskInPathWorkspace`；Run Trigger 绑定单测；**AgentAuth 写入 APPLICATION principal 上下文**（任务 API 仍走 Pool Token）。  
> - **Wave C（代码侧已关）**：Remove/Clone fail-closed + `authOrgFromContext`；全局 task logs/stream 绑 workspace READ；CreateRunTrigger target WRITE；Application 无 org / RunTrigger 无 source 禁用；`IAM_SINGLE_TENANT` 才默认 org=1；§4.7 安全回归测。  
> - **SQL（本环境 Docker `iac-platform-postgres` 已执行）**：`patch_system_admin_iam_roles`、`patch_admin_role_iam_policies`（admin 含 8 条 IAM_*）、`patch_team_tokens_active_unique`（索引已建；活跃 NULL expires=0）、`patch_temp_permission_user_id_index`。  
> **门禁自测**：  
> - `valueobject` / `application/service` / `middleware` / `handlers` **通过**  
> - **`go test ./controllers` 全量通过**  
> - **`go test ./services` 全量通过**  
> - 临时权限：`user_id` 已是 varchar；**email OR user_id 双键匹配**  
> **残留**：其它环境四 SQL 需 release checklist；P2 覆盖率；list accessibility / MANAGEMENT 引擎化见 37 R2。  
> **Application principal（选项 A，2026-07-17）**：`/api/v1/app/*` 挂 `AgentAuth`；grant 存 `app_key`；Agent 任务仍 Pool Token。

---

## 1. 执行摘要

### 1.1 总体评级

| 维度 | 评级 | 说明 |
|------|------|------|
| 产品决策落地（D1–D5 主语义） | **中上** | NONE 拒绝、精确作用域优先、业务取消 system_admin bypass、TEAM principal 求值等已编码 |
| Critical 安全洞（R-S01/02） | **基本关闭** | Pool `status=active`、State list 不加载 content、敏感下载抬级 |
| High IDOR / 多租户边界（R-S03 等） | **未关完** | 变量/任务/资源主路径有绑定；vsnap / run trigger / resource 版本依赖 / Application org 仍开 |
| Role / 管理面防提权 | **部分缓解** | 有 policy 闭包防提权；系统 Role 策略可改、org 与 assignment scope 解耦 |
| 测试与回归门禁 | **不足** | 定向 IAM 包可绿；`./controllers` panic、`./services` 漂移、前端 build 失败 → **无全绿门禁** |
| 文档可信度 | **偏低** | `34` 将多项标为 ✅，与代码不符；上线 SQL 清单遗漏 |

### 1.2 是否可上线

| 场景 | 建议 |
|------|------|
| **公网 / 多组织生产** | **否** — 先关闭 §3 P0 |
| **单组织内网、信任管理员** | **有条件** — 仍建议先修 Application 跨 org 与 vsnap/trigger IDOR；必须执行 **两个** patch SQL |
| **仅合并到主分支继续开发** | **可以** — 但需把本报告 open items 挂 backlog，并修正 `34` 状态表 |

### 1.3 问题计数

| 级别 | 数量 | 编号 |
|------|------|------|
| **P0** | 4 | A-1, A-2（收窄后仍 P0）, A-3, A-4（rollback 等写路径可并入） |
| **P1** | 6+ | B-1～B-6 及测试/门禁项 |
| **文档/流程** | 2 | D-1, D-2 |

---

## 2. 审查范围与方法

### 2.1 范围

- 工作区未提交 IAM 相关改动（backend `internal/*`、`controllers/*`、`services/*`、migrations、docs/iam）
- 对照清单：R-S01～R-S09、33 号 CR 开项、用户补充的 P0/P1 清单
- **不含**：前端完整威胁建模、渗透测试、生产配置审计

### 2.2 方法

1. 读路由 → 中间件鉴权 scope → handler/service 是否二次绑定资源归属  
2. 读 checker / anti-escalation 是否 fail-closed  
3. 读新增测试是否锁住安全属性、是否假阳性  
4. 对照 `34` 状态表与代码差分

### 2.3 权威关系

| 文档 | 角色 |
|------|------|
| `32-...` | 产品决策与目标语义 SoT |
| `33-...` | 中间轮 CR（部分已 fixed） |
| `34-...` | **进度宣称** — 本报告指出其不可作为完成证据 |
| **本报告 `35-...`** | **2026-07-17 代码核实后的安全缺口与修复优先级** |

与 `34` 冲突时，**以本报告代码核实结论为准**（直至对应项修完并回归）。

---

## 3. 已确认有效的修复（保留）

以下经代码核对，**方向正确且主路径已实现**，应继续保留并加回归测试锁定。

| ID | 项 | 证据摘要 | 状态 |
|----|-----|----------|------|
| R-S01 | Pool 撤销后仍有效 | `checkPoolWorkspaceAccess` 要求 `status = active`；token 查询 `is_active = true` | ✅ 有效 |
| R-S02 | READ 读完整 State | `ListStateVersions*` 显式 `Select` 不含 content；download 等抬 SENSITIVE | ✅ 有效（list 侧有单测） |
| R-S04 部分 | 非法 org_id | `resolveOrgScopeID` 非法值 400；**缺失仍默认 1**（单租户 fallback，管理面有副作用，见 A-1） | 🔶 半完成 |
| R-S05 | system_admin 业务旁路 | 业务 IAM 不再 bypass；`RequireSystemAdmin` 平台 API；中间件测试强制 403 | ✅ 有效 |
| R-S06 | Team Token / TEAM 主体 | JWT type 优先；`principal_type=TEAM`；checker `resolvePrincipal` TEAM 路径 | ✅ 有效 |
| D1 | NONE = 不授权 | `effective >= required && != NONE`；deny reason「No permission」 | ✅ 有效 |
| 裁决 | 精确作用域优先 | WS > Project > Org；`calculateEffectiveLevel` + 单测 | ✅ 有效 |
| Secret | App secret 哈希 | `sha256:` 存储 + legacy 明文兼容 | ✅ 有效（管理面 IDOR 另计） |
| 变量 IDOR 主路径 | `Get/Update/Delete*InWorkspace` | 跨 WS 单测存在 | ✅ 主路径有效 |
| 任务/资源绑定 helper | `loadTaskInPathWorkspace` / `loadResourceInPathWorkspace` | 部分端点已接；**未全覆盖** | 🔶 半完成 |
| Role 防提权（部分） | `RoleAntiEscalationService` | 赋权时 policy 闭包 ⊆ actor；admin 系统 Role 仅超管可分配 | 🔶 半完成 |

---

## 4. P0 问题（必须修）

### A-1 Application 管理接口可跨组织越权

| 字段 | 内容 |
|------|------|
| **严重度** | **P0** |
| **关联** | R-S04 / 多租户隔离；R-S08 管理面 |
| **位置** | `internal/router/router_iam.go`（applications 路由）<br>`internal/handlers/application_handler.go`<br>`internal/middleware/iam_permission.go` → `resolveOrgScopeID`<br>`internal/application/service/application_service.go` |

**问题机制**

1. 路由对 `IAM_APPLICATIONS@ORGANIZATION` 鉴权；无 `org_id` 时 **默认组织 1**。  
2. **Create** 信任 body `org_id`，不与鉴权 org 对齐 → 可在 org1 权限下向 org2 创建应用。  
3. **Get / Update / Delete / RegenerateSecret** 仅按 path 数字 id 操作，**不校验** `app.OrgID` 是否等于鉴权 org。  
4. **List** 的 query `org_id` 可指向任意组织，鉴权侧仍可能是默认 1。

**影响**

- 拥有 org1 `IAM_APPLICATIONS` 权限的用户可枚举/读写/轮换 **其他组织** Application 密钥。  
- 密钥轮换与删除为直接凭证接管面。

**修复要求**

1. 中间件解析的 org 写入 context（强制管理 API 带 `org_id`，或 path `/orgs/:org_id/applications`）。  
2. Create：`req.OrgID` 必须等于鉴权 org，否则 403。  
3. List：仅允许 list 鉴权 org；禁止跨 org query。  
4. Get/Update/Delete/Regenerate：加载后 `app.OrgID == authOrg`，否则 404（防探测）或 403。  
5. 服务层二次校验（防 handler 遗漏）。

**验收测试**

- org1 管理员：create `org_id=2` → 403  
- org1 管理员：get org2 的 app id → 404/403  
- org1 管理员：list `?org_id=2` → 403  

---

### A-2 Role 管理仍可提权（部分缓解后的残留）

| 字段 | 内容 |
|------|------|
| **严重度** | **P0（收窄描述）** — 非「零防护」，但残留足够升级为特权 |
| **关联** | 32 §3.3 防提权；R-S08 |
| **位置** | `internal/handlers/role_handler.go`（AssignRole / AddRolePolicy）<br>`internal/application/service/role_anti_escalation.go`<br>`internal/router/router_iam.go`（`IAM_USERS` / `IAM_ROLES`） |

**已有防护（勿忽略）**

- `EnsureCanAssignRole`：目标 Role 的 policy 闭包 ⊆ actor 在 **赋值 scope** 上的有效权限。  
- 系统特权 Role：`is_system && name=="admin"` 仅 `is_system_admin` 可分配。  
- `EnsureCanAddRolePolicy`：新增单条策略时 actor 须已持有对应 resource@level。

**仍成立的缺口**

| 缺口 | 说明 |
|------|------|
| **系统 Role 策略可改** | 删除系统 Role 有 403；**AddRolePolicy / RemoveRolePolicy 不检查 `role.IsSystem`**。持有 `IAM_ROLES` WRITE 且闭包校验通过的调用者可改 `admin` 等系统模板，造成 **持久化全局特权放大** |
| **中间件 org 与 assignment.scope 解耦** | 路由只验默认/查询 org 上的 `IAM_USERS@ORG ADMIN`；`scope_type`/`scope_id` 可指向其它树节点，边界依赖 actor 是否碰巧有业务权限，而非「必须在已授权 org 内」 |
| **fail-open** | `guard == nil` 或 `checker == nil` 时防提权直接跳过（`EnsureCanAssignRole` 返回 nil） |
| **空策略 Role** | 闭包为空则允许赋值；后续若被加策略会突然变强 |
| **仅 admin 名单** | 其它 `is_system` Role 不享受「仅超管可分配」 |

**修复要求**

1. 系统 Role（或至少 `is_system`）：禁止非平台超管增删改 policy；或完全只读。  
2. assignment 的 scope 必须属于中间件已授权的 org 子树（org / project / workspace 归属校验）。  
3. 生产路径 **强制注入 checker**；`guard == nil` 时 500 fail-closed。  
4. 空策略 Role 赋值需额外策略（禁止或仅超管）。  
5. 特权系统 Role 名单可配置扩展，不写死单字符串。

**验收测试**

- 非超管给 `admin` 加 policy → 403  
- 仅有 org1 IAM_USERS ADMIN：向 org2 的 scope 赋 Role → 403  
- checker 未注入时启动/请求失败而非静默放行  

---

### A-3 子资源跨 Workspace 归属校验不完整（R-S03 未关完）

| 字段 | 内容 |
|------|------|
| **严重度** | **P0** |
| **关联** | R-S03 |

#### A-3.1 变量快照删除忽略路径 Workspace

- **位置**：`controllers/variable_snapshot_controller.go` → `DeleteSnapshot`  
- **行为**：只取 `vsnap_id`，调用 `DeleteSnapshot(vsnapID)`；path `:id` **未使用**。  
- **影响**：有 WS-A 删除权限即可删除 **任意 WS** 的变量快照（破坏性 IDOR）。

#### A-3.2 创建任务时 vsnap 仅校验存在

- **位置**：`controllers/workspace_task_controller.go` 创建任务段（`variable_snapshot_id`）  
- **行为**：`Count(vsnap_id)` 存在即接受，**不校验** snapshot 的 `workspace_id == path workspace`。  
- **影响**：将 **B 的变量/敏感值快照** 注入 **A 的任务执行**（数据外泄 + 执行语义污染）。

#### A-3.3 Run Trigger 未绑定路径 Workspace / 目标权限

- **位置**：`internal/handlers/run_trigger_handler.go`  
  - Create：目标 WS 仅「存在」校验  
  - Update / Delete：仅按 `trigger_id`，**忽略** path workspace  
- **影响**：跨 WS 改/删触发器；可能把触发链指到无权限的目标 WS（视 Create 校验强度，存在链式执行风险）。

**修复要求**

1. `DeleteSnapshot(workspaceID, vsnapID)` 双条件删除。  
2. 任务创建：`vsnap_id` 必须属于当前 workspace。  
3. Run Trigger：所有写操作 `source_workspace_id = path id`；target 需对 target 有明确权限（至少 WRITE/执行类）。  
4. 统一 helper + 表驱动跨 WS 拒绝测试。

**验收测试**

- 跨 WS 删 vsnap → 404  
- 跨 WS vsnap 创建任务 → 400/404  
- 跨 WS update/delete trigger → 404  

---

### A-4 Resource 版本写路径 / 敏感读未绑定（建议按写路径升 P0）

| 字段 | 内容 |
|------|------|
| **严重度** | **读：P1；回滚/依赖更新：建议 P0** |
| **位置** | `controllers/resource_controller.go` |

**已绑定**：CRUD、版本列表、快照、editing/drift（helper）。

**未绑定**（仍按 resource_id 全局操作）：

| 方法 | 约略行号 | 风险 |
|------|----------|------|
| `GetResourceVersion` | ~467 | 跨 WS 读版本详情 |
| `RollbackResource` | ~505 | **跨 WS 写回滚** |
| `CompareVersions` | ~548 | 跨 WS 读 diff |
| `GetResourceDependencies` | ~798 | 跨 WS 读依赖 |
| `UpdateDependencies` | ~828+ | **跨 WS 写依赖** |

**修复要求**：上述端点一律 `loadResourceInPathWorkspace` / `parseResourceIDInPathWorkspace`；路由级测试覆盖，禁止只测 helper。

---

## 5. P1 问题

### B-1 Application principal 无端到端认证链

| 字段 | 内容 |
|------|------|
| **严重度** | **P1**（若产品宣称 App 可走业务 IAM → **P0**） |
| **位置** | `middleware/agent_auth.go`、`router/router_agent.go`、`iam_permission.go` |

- `AgentAuthMiddleware` 校验 AppKey/Secret，仅 `Set("application_id")`，不设 `principal_type=APPLICATION` / `principal_id` / IAM 所需 `user_id`。  
- Agent 路由实际使用 **Pool Token**，**未挂载** `AgentAuthMiddleware`。  
- Checker 有 APPLICATION 分支 ≠ 生产可认证进入。

**修复方向**：明确产品路径——要么废弃 Application principal 对外宣称，要么打通认证中间件 + 路由 + IAM 上下文。

---

### B-2 Team Token 配额/同名竞态 + 历史永不过期

| 字段 | 内容 |
|------|------|
| **严重度** | **P1** |
| **位置** | `team_token_service.go`（Count 后 Create）<br>`middleware/middleware.go`（JWT team token 分支） |

1. **Check-then-act**：活跃数量上限 2、活跃 name 唯一均无事务/行锁/DB 唯一约束 → 并发可突破。  
2. **`expires_at IS NULL`**：中间件仅在 `ExpiresAt != nil && Before(now)` 时拒绝 → **NULL 仍接受**，与「禁止永不过期」决策冲突。

**修复要求**

- 部分唯一索引：`(team_id) WHERE is_active` 或等价配额约束；`(team_id, token_name) WHERE is_active`。  
- 创建走事务 + 冲突重试。  
- 拒绝或强制迁移 NULL `expires_at`；中间件对 NULL 直接 401。

---

### B-3 TeamTokenService.ValidateToken 列与 schema 不一致（生产缺陷）

| 字段 | 内容 |
|------|------|
| **严重度** | **P1**（服务路径） |
| **位置** | `team_token_service.go` ValidateToken：`Where("token_id = ? AND token_hash = ?")` |
| **模型** | `TeamToken.TokenID` 为 `gorm:"-"`；主键为 **`token_id_hash`** |

JWT 中间件使用 `token_id_hash`，服务 `ValidateToken` 使用不存在的 `token_id` 列 → 行为分裂。  
测试通过 `ALTER TABLE ... ADD COLUMN token_id` **人为补列**，造成 **假阳性**。

**修复要求**：ValidateToken 改为 `token_id_hash`（与中间件一致）；测试使用真实 schema，禁止假列。

---

### B-4 临时权限链路假邮箱

| 字段 | 内容 |
|------|------|
| **严重度** | **P1 / 功能失效** |
| **位置** | `permission_checker.go` → `CheckPermissionWithTemporary` |

```text
userEmail := fmt.Sprintf("user_%s@example.com", req.UserID)
```

临时授权若按真实邮箱匹配则 **不可用/恒失败**；主路径 IAM 修好不代表临时权限可用。

---

### B-5 任务绑定 helper 未统一（维护性 IDOR 回归风险）

| 字段 | 内容 |
|------|------|
| **严重度** | **P1** |
| **位置** | `workspace_task_controller.go` |

`loadTaskInPathWorkspace` 仅部分 handler 使用；ConfirmApply / RetryStateSave / 备份下载等 **内联** `id + workspace_id`（当前多数已绑，但易在后续改动中回退到 `First(&task, id)`）。

**建议**：全文件统一 helper；CI `grep` 禁止裸 `First(&task`。

---

### B-6 handlers 覆盖率与 e2e 缺失

| 字段 | 内容 |
|------|------|
| **严重度** | **P1（质量）** |
| **数据** | `34` 自报 handlers ~6.6%；集成 stub 未过滤 teamIDs |

- `permission_checker_integration_test.go` 中 `QueryTeamRoles` **忽略 teamIDs** → 无法证明非成员不继承 Team 权限。  
- Resource 绑定测试只打 helper，未覆盖版本/依赖路由。  
- 无真实 repo + 多 org 的 e2e 鉴权套件。

---

## 6. 测试质量专项

### 6.1 做得好的

- 安全属性导向：跨 WS 404、list 无 content、system_admin 不 bypass、TEAM principal 传递。  
- 分层：纯函数 / stub 集成 / middleware mock / sqlite 边界。  
- 与 R- 编号对照注释，便于审计。

### 6.2 缺陷清单

| # | 问题 | 影响 |
|---|------|------|
| T-1 | `controllers/module_controller_test.go` 使用 `&gorm.DB{}` | `go test ./controllers` **panic**（`GetModules` → gorm.Model nil） |
| T-2 | Module controller 生产错误路径返回 **mock 200 数据** | 测试「成功」语义错误；生产可掩盖 DB 故障 |
| T-3 | ValidateToken 测试 `ALTER` 补 `token_id` | **假阳性**，掩盖 schema 错误 |
| T-4 | Team role stub 忽略 `teamIDs` | 成员边界无证明 |
| T-5 | Resource 绑定未走真实路由 | 遗漏端点无回归网 |
| T-6 | `go test ./services` schema 漂移 / 大量失败 | 无包级绿灯 |
| T-7 | 前端 `npm run build` 失败（审查时环境结论） | 无全栈门禁 |
| T-8 | 覆盖率数字与行为不等价 | 高覆盖 ≠ 安全属性闭合 |

### 6.3 建议的最小门禁命令（修完 P0 后）

```bash
# IAM 核心（应稳定绿）
cd backend
go test ./internal/domain/valueobject/ ./internal/application/service/ \
  ./internal/middleware/ ./internal/handlers/ \
  -count=1

# 绑定与敏感面
go test ./controllers/ -run 'Binding|PathWorkspace' -count=1
go test ./services/ -run 'StateList|WorkspaceList|Variable' -count=1

# 禁止
# go test ./controllers  # 在修好 module_controller_test 之前不可作门禁
```

---

## 7. 文档可靠性

### 7.1 `34-iam-fix-progress-report.md` 偏差

| 文档宣称 | 代码核实 |
|----------|----------|
| R-S03 ✅「均绑定 path WS」 | **否** — vsnap 删/注入、run trigger 改删、resource 版本依赖等未绑 |
| R-S08 ✅ | Secret 哈希 ✅；**Application 跨 org 管理、App principal 链** 未闭环 |
| 上线清单仅 `patch_system_admin_iam_roles.sql` | **遗漏** `patch_admin_role_iam_policies.sql`（admin 角色补齐 IAM_* 策略） |
| 任务 IDOR「剩余洞已修」 | 主路径多已绑；**非全文件/全子资源** |

**结论**：`34` **不能作为完成证明或 release evidence**。应改为「部分完成」并链接本报告 open items。

### 7.2 上线 SQL（必须成对）

| 文件 | 作用 |
|------|------|
| `backend/migrations/patch_system_admin_iam_roles.sql` | 为 `is_system_admin` 用户补 `admin@ORGANIZATION` 绑定 |
| `backend/migrations/patch_admin_role_iam_policies.sql` | 为系统 Role `admin` 补齐全部 `IAM_*` 策略 |

只跑前者、不跑后者 → 超管有 admin 绑定仍可能 **IAM 管理面 403**。  
取消业务 bypass 后，**两补丁均属上线阻断项**。

### 7.3 文档维护动作

1. 修订 `34` §2.2 状态为 🔶 + 指向本报告 §3/§4。  
2. 更新 `docs/iam/README.md`：进度以 35 核实为准，34 为历史进度快照。  
3. Changelog / release notes 不得写「R-S03 全部完成」。

---

## 8. 与历史 CR（33）对照

| 33 号 Issue | 当时状态 | 本报告状态 |
|-------------|----------|------------|
| system_admin 业务旁路 | fixed | 仍 ✅ |
| TEAM 真主体 | fixed | 仍 ✅ |
| RequireAny org 对齐 | fixed | 仍 ✅（默认 1 的管理面副作用见 A-1） |
| 任务 IDOR 全文件 | open/部分 | 仍 🔶 |
| token_name 唯一 | open | 应用层有；**无 DB 约束/事务** → B-2 |
| NONE 注释 | open | 代码注释已对齐 D1 |

---

## 9. 修复优先级与实施波次

### Wave A — P0（阻断多组织生产）

| 顺序 | 项 | 预估影响面 |
|------|-----|------------|
| A1 | Application org 强制绑定（create/list/get/update/delete/secret） | handlers + service + 测试 |
| A2 | vsnap delete 绑 WS；task 创建 vsnap 绑 WS | controllers + service |
| A3 | Run trigger CUD 绑 source WS + target 权限 | handlers |
| A4 | 系统 Role 策略只读/仅超管；guard fail-closed；assignment scope∈org | role_handler + anti-escalation |
| A5 | Resource Rollback / UpdateDependencies（及建议一并做版本读）绑 WS | resource_controller |

### Wave B — P1

| 顺序 | 项 |
|------|-----|
| B1 | Team Token 唯一索引 + 事务；拒绝 NULL expires |
| B2 | 修 ValidateToken 列；删假阳性测试列 |
| B3 | 临时权限真实邮箱/主体 |
| B4 | 任务全端点 helper 化 |
| B5 | Application principal 产品决策 + 实现或降级文档 |
| B6 | stub 过滤 teamIDs；路由级 IDOR 测试；修 module_controller_test |

### Wave C — 门禁与文档

| 顺序 | 项 |
|------|-----|
| C1 | 修正 `34` + README + 上线清单双 SQL |
| C2 | CI：IAM 包 + Binding 测试必绿；controllers 全量在 T-1 修复后纳入 |
| C3 | 覆盖率目标改为「安全属性用例清单」而非仅语句覆盖率 |

---

## 10. 上线检查清单（修订版）

### 10.1 代码门禁

- [ ] Wave A 全部合并  
- [ ] 下列测试绿：`valueobject` / `application/service` / `middleware` / `handlers` + Binding/StateList 定向  
- [ ] 无 `NewXxxService(&gorm.DB{})` 式 panic 测试  
- [ ] Application / Role / vsnap / trigger / resource 版本 跨边界用例存在且失败路径为 403/404  

### 10.2 数据门禁

- [ ] 执行 `patch_system_admin_iam_roles.sql`  
- [ ] 执行 `patch_admin_role_iam_policies.sql`  
- [ ] 验收：每个活跃超管在每个活跃 org 有 `admin` 绑定  
- [ ] 验收：`admin` Role 含全部 `IAM_*` @ ORGANIZATION/ADMIN  
- [ ] 审计 `team_tokens.expires_at IS NULL` 行并吊销/补过期  

### 10.3 产品/运维

- [ ] 通知：State 内容类接口需 SENSITIVE；列表不再含 state content  
- [ ] 通知：业务 API 不再认裸 `is_system_admin`  
- [ ] 前端列表不依赖已 redacted 的 workspace 敏感配置字段  

### 10.4 文档

- [ ] `34` 状态降级 / 标注 superseded by `35`  
- [ ] Release note 与本报告 open items 一致  

---

## 11. 风险矩阵（摘要）

| ID | 标题 | 级别 | 利用难度 | 影响 |
|----|------|------|----------|------|
| A-1 | Application 跨 org | P0 | 低（有 org1 管理权限即可） | 他组织应用密钥/生命周期 |
| A-2 | Role 策略/边界 | P0 | 中（需 IAM 管理权限） | 持久化提权、系统 Role 污染 |
| A-3 | vsnap / trigger IDOR | P0 | 低 | 跨 WS 删数据、变量注入、触发链篡改 |
| A-4 | Resource 回滚等 | P0/P1 | 低 | 跨 WS 写状态/读敏感版本 |
| B-1 | App principal | P1 | — | 功能/模型不一致 |
| B-2 | Team Token 竞态/NULL | P1 | 中（并发） | 配额突破、长期凭证 |
| B-3 | ValidateToken 列 | P1 | — | 服务路径错误；测试假绿 |
| T-1 | controllers panic | 门禁 | — | CI 不可用 |
| D-1 | 文档虚高 | 流程 | — | 误上线、漏 SQL |

---

## 12. 最终结论

1. **不要**依据 `34-iam-fix-progress-report.md` 宣布 R-S03/R-S08「已完成」。  
2. **可以**肯定：Pool 撤销、State list 脱敏、业务取消 superadmin bypass、TEAM 主体求值、精确作用域裁决、部分 IDOR helper 与闭包防提权 —— 这些是实打实的进展。  
3. **在 Wave A 关闭前**，多组织生产环境存在 **可利用的管理面与跨 Workspace 问题**。  
4. **上线必须执行两个 SQL 补丁**，缺一不可。  
5. **测试与 CI**：当前不存在可信全仓绿灯；应以安全属性用例 + IAM 包定向测试作为最低门禁，并优先消灭 `module_controller_test` panic 与 ValidateToken 假阳性。

---

## 13. 附录：关键文件索引

| 主题 | 路径 |
|------|------|
| Org 默认 1 | `backend/internal/middleware/iam_permission.go` |
| Application 路由 | `backend/internal/router/router_iam.go` |
| Application handler | `backend/internal/handlers/application_handler.go` |
| Role 赋权/策略 | `backend/internal/handlers/role_handler.go` |
| 防提权 | `backend/internal/application/service/role_anti_escalation.go` |
| 变量快照删除 | `backend/controllers/variable_snapshot_controller.go` |
| 任务 vsnap | `backend/controllers/workspace_task_controller.go` |
| Run Trigger | `backend/internal/handlers/run_trigger_handler.go` |
| Resource 版本/依赖 | `backend/controllers/resource_controller.go` |
| AgentAuth | `backend/internal/middleware/agent_auth.go` |
| Team Token | `backend/internal/application/service/team_token_service.go` |
| JWT Team | `backend/internal/middleware/middleware.go` |
| State list | `backend/services/state_service.go` |
| SQL 补丁 | `backend/migrations/patch_system_admin_iam_roles.sql`<br>`backend/migrations/patch_admin_role_iam_policies.sql` |
| Module 坏测试 | `backend/controllers/module_controller_test.go` |

---

*报告结束。修复落地后建议复审并发布 `36-iam-remediation-verification.md` 作为关闭证据。*
