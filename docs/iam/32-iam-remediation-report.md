# IAM 权限系统整改报告

> **状态**：已定稿 + 高危补丁进行中  
> **日期**：2026-07-16  
> **范围**：裁决语义、授权模型、Role 主路径、列表鉴权、Team Token / 自动化身份；**含运行面 Pool ACL、State 敏感下载、子资源 IDOR、org 作用域、临时授权创建链路、Application 凭据**  
> **前序**：设计 Review 见会话分析；历史方案见 `03-iac-platform-permission-system-design-v2.md`（部分条款以本文为准）

---

## 0. 产品决策（锁定）

| # | 议题 | 决策 |
|---|------|------|
| D1 | `NONE` 语义 | **不授权 = 拒绝访问**。有效权限为 NONE 时必须 deny。不做「显式拒绝 grant 覆盖上层 ADMIN」的独立产品能力；缺省即拒绝。 |
| D2 | Workspace 写权限范围 | **按授权作用域生效**：授到指定 Workspace → 只能改该 Workspace；授到组织级（同资源类型）→ 可改组织内全部对应实例。 |
| D3 | 项目负责人改成员 | **允许，但必须显式授权**（Role 含对应管理权限）；不因「创建者 / 项目名」隐式成为管理员。 |
| D4 | Team Token | **自动化鉴权凭证**（目标对齐 TFE）：可代表 **用户** 或 **应用**；会话/在线窗口按设计为约 **24h**（实现以配置为准）。 |
| D5 | Role vs Direct Grant | **Role 是主模型**；一用户可绑定多个 Role。**取消日常双轨运维**（Direct Grant 仅保留迁移/系统内部窄用途，不对业务管理员暴露为平行入口）。 |

本文后续整改项均以上表为准；与旧文档冲突时 **以本文为准**。

---

## 1. 目标架构（整改后）

```
                    ┌─────────────────────────┐
                    │  is_system_admin        │  仅平台级旁路
                    │  (SSO/全局设置等)        │  （业务权限不依赖此字段）
                    └───────────┬─────────────┘
                                │ bypass 仅系统面
                                ▼
┌──────────────┐    ┌───────────────────────┐    ┌────────────────────┐
│ Identity     │───▶│ PermissionChecker     │───▶│ effective Level    │
│ User / Team  │    │ Role 展开 + 作用域合并 │    │ >= required ?      │
│ App (token)  │    └───────────────────────┘    └────────────────────┘
└──────────────┘
        ▲
        │ 绑定
┌───────┴────────┐
│ Role 赋值      │  scope = ORG | PROJECT | WORKSPACE
│ (可多 Role)    │  policies = ResourceType × Level × policy scope
└────────────────┘
```

**运维主路径**：创建/克隆 Role → 配置 policies → 将 Role 赋给 User / Team（在指定 scope）→ 自动化主体用 User Token / Team Token / Application 凭证访问。

---

## 2. 裁决语义（必须实现一致）

### 2.1 权限等级

| Level | 值 | 含义 |
|-------|-----|------|
| NONE | 0 | **无有效授权**；访问拒绝 |
| READ | 1 | 只读 |
| WRITE | 2 | 读写（含 READ） |
| ADMIN | 3 | 管理（含 WRITE） |

判定：`is_allowed = (effective != NONE) && (effective >= required)`。

拒绝原因文案：

- 无任何有效 grant / Role 策略命中 → `"No permission"`（**禁止**再写 `"Access explicitly denied"`）
- 有授权但 level 不足 → `"Insufficient permission: have X, need Y"`

### 2.2 作用域合并（精确优先）

对**同一 ResourceType**，收集 User 直接展开结果 + 所属 Team 的 Role + 自身 Role 后：

1. 过滤过期 Role 赋值 / 过期策略（若有）
2. 按 grant 实际作用域分组：`WORKSPACE` > `PROJECT` > `ORGANIZATION`
3. **若存在更精确作用域上的有效授权集合，则只在该层取 max(level)**  
   - 例：WS 上有 READ，Org 上有 WRITE → **effective = READ**（WS 层优先）  
   - 例：仅 Org 上有 WRITE → **effective = WRITE**（可作用于该 Org 下实例，见 2.3）
4. 任一层都无有效授权 → **NONE → 拒绝**

> 说明：D1 明确不做「写入一条 NONE grant 用来 ban 掉上层 ADMIN」。若未来需要强制 ban，另开「deny policy」产品，不与 NONE 混用。

### 2.3 组织级授权 = 全实例（D2）

对实例类 API（路径含 workspace id 等）：

| 授权位置 | 效果 |
|----------|------|
| Role 赋在 **WORKSPACE scope**，policy 含 `WORKSPACE_*` WRITE | 仅该 workspace |
| Role 赋在 **PROJECT scope**，policy 含项目/工作空间相关权限 | 该 project 下实例（按 policy 定义） |
| Role 赋在 **ORGANIZATION scope**，policy 含 `WORKSPACES` / 或约定可下沉的 workspace 资源 | **组织内全部对应实例** |

路由层 OR（`WORKSPACES@ORG` OR `WORKSPACE_MANAGEMENT@WS`）与引擎语义对齐，**禁止**仅靠路由手写、引擎不认的隐含规则长期漂移。

### 2.4 MANAGEMENT 与精细权限（引擎内统一）

整改目标（二选一落地，推荐 A）：

- **方案 A（推荐）**：引擎定义蕴含  
  - `WORKSPACE_MANAGEMENT` level L 可满足同 scope 下各精细资源（EXECUTION / STATE / VARS / RESOURCES / TASK_DATA…）对 level ≤ L 的要求  
  - 精细资源 **不能** 反向满足 MANAGEMENT  
- **方案 B**：引擎不蕴含，路由禁止 MANAGEMENT 顶替精细写操作  

选定 A 后删除「每个路由手写一长串 RequireAnyPermission」中的重复蕴含，改为引擎一次计算（路由只声明真实资源类型）。

### 2.5 System Admin

- **现状（问题，见 R-S05）**：中间件对全部挂 IAM 的业务 API 直接 bypass；整改前无审计。
- **目标**：`is_system_admin` **仅**平台级能力（初始化、SSO、全局系统设置等）
- **已做**：旁路写 `[IAM-AUDIT] system_admin_bypass` + `permission_source=system_admin`
- **未做完**：业务路由仍放行 superadmin；需分批改为 Org admin Role + 仅 `RequireSystemAdmin` 挂平台 API
- 新建业务资源 **不依赖** system admin；靠 Role + 创建时绑定（见 5.3）

---

## 3. 授权模型：Role 主路径（D5）

### 3.1 主模型

```
iam_roles
  └── iam_role_policies   (permission / resource_type, level, scope_type)
iam_user_roles            (user_id, role_id, scope_type, scope_id, expires_at?)
iam_team_roles            (team_id, role_id, scope_type, scope_id, expires_at?)  // 若已有则沿用
```

- 用户可持有 **多个 Role**（多 scope、多角色叠加）
- 叠加规则：同一 ResourceType 下按 **§2.2** 合并（精确 scope 优先，同层 max）
- Team 成员自动继承 Team 上绑定的 Role

### 3.2 废弃日常 Direct Grant 双轨

| 能力 | 整改后 |
|------|--------|
| 管理台「直接 grant 单条 permission」 | **下线或仅 superadmin/迁移工具** |
| API `POST /permissions/grant` 等 | 标记 deprecated；新客户端只用 Role 赋值 |
| 表 `org/project/workspace_permissions` | 迁移：展开为 Role 或一次性合成「legacy custom role」后只读；最终停止写入 |
| Preset READ/WRITE/ADMIN | 改为 **系统预置 Role** 或「一键绑定预置 Role」，不再批量插入 permission 行 |

### 3.3 防提权（授权治理，必做）— **已实现主路径**

无论谁调用「赋 Role / 改 Role Policy」：

1. **Actor 必须对被操作 scope 具备显式管理权限**  
   - 例：改 project 成员/赋 project 内 Role → 需要该 PROJECT（或父 ORG）上 **已授权** 的 `PROJECT_TEAM_MANAGEMENT` / `IAM_*` 等（具体 resource 在实现清单中钉死），**无隐式**  
   - 路由层：`IAM_USERS`/`IAM_TEAMS` ADMIN、`IAM_ROLES` WRITE 等中间件
2. **不可赋出超出自身有效权限集合的 Role**（Role 的 policy 闭包 ⊆ actor 在该 scope 的有效权限）  
   - ✅ `RoleAntiEscalationService.EnsureCanAssignRole` / `EnsureCanAddRolePolicy` / `EnsureCanCloneRole`
3. **系统 Role**（全局 `admin`）仅 `is_system_admin` 可分配/克隆（org owners 白名单为后续增强）
4. 自定义 Role 若包含 `IAM_*` 等高危 policy：分配时仍走闭包检查，actor 必须自身已有对应 `IAM_*@level`

### 3.4 项目负责人管成员（D3）

- 预置 Role 示例：`project_admin`  
  - 含：`PROJECT_SETTINGS`、`PROJECT_TEAM_MANAGEMENT`、本项目 workspace 管理相关 policy  
  - **不含** 自动的组织级 IAM 全能
- 用户只有在被 **显式** 赋予 `project_admin`（或等价自定义 Role）于该 `PROJECT` scope 后，才能：
  - 增减项目成员 / 给成员赋本项目 scope 的 Role  
  - 不能越权赋 Org 级 Role 或改其他项目

---

## 4. 列表 vs 对象鉴权

### 4.1 问题

当前 `GET /workspaces` 仅要求 Org `WORKSPACES READ` 且 **不过滤实例** → 有权 list 则近乎看见全部；仅有 WS Role 的用户无法 list。

### 4.2 整改

| 接口类型 | 规则 |
|----------|------|
| 列表 | 返回 **effective ≥ READ** 的实例并集（来自：Org 级该资源 READ+，或 Project/WS 级 Role 覆盖的 id） |
| 详情/写 | 对该 `scope_id` 做 §2 裁决 |
| 仅 Org 观察者 Role | 可 list 全量元数据（产品定义的观察者）；与「只授了单个 WS」用户分离 |

实现建议：checker 增加 `ListAccessibleScopeIDs(user, resourceType, minLevel)`，列表查询走 id 过滤或 post-filter（数据量大时必须下推 SQL）。

---

## 5. 身份与自动化（D4）

### 5.1 主体类型（运行时一等公民）

| Principal | 来源 | 鉴权求值 |
|-----------|------|----------|
| USER | 登录 JWT / User Token | 用户 Role + 所属 Team 的 Role |
| TEAM | Team Token | **仅该 Team 绑定的 Role**（不继承「创建人」个人 Role） |
| APPLICATION | App 凭证 / API Key | 仅 Application 被赋予的 Role（Org 级为主，对齐 TFE 应用权限） |

`PermissionChecker` 入参从「只有 user_id」扩展为：

```text
PrincipalType + PrincipalID
// TEAM: 直接用 team roles
// USER: user roles ∪ teams(user).roles
// APPLICATION: application roles
```

### 5.2 Team Token（整改规格）

| 项 | 规格 |
|----|------|
| 用途 | CI/CD、Agent、自动化管道（对齐 TFE Team Token 思路） |
| 有效期 | 默认 **24h** 滑动或固定窗口（配置项；吊销立即失效） |
| Claims | `type=team_token`, `team_id`, `token_id`, 可选 `principal_hint`；**禁止**伪造任意 user_id 走用户权限 |
| 中间件 | 校验 token 存活性后 `Set(principal_type=TEAM, principal_id=team_id)` |
| 与 checker | 必须按 TEAM 求值；当前「强制 user_id + 注释说按 team」的断裂 **必须修** |

User Token / Application Token 同理：身份字段与 checker 主体一致。

### 5.3 资源创建时的 Role

- 创建 Workspace 后：可选将创建者所在 Team 或创建者本人 **绑定预置 `workspace_admin` Role 到该 WS scope**（替代 GrantPreset 插 permission 行）
- 绑定失败应可配置为失败回滚或告警工单，避免「资源在、权限无」

---

## 6. 与现状差距（整改清单）

### P0 — 正确性 / 安全

| ID | 项 | 现状 | 目标 |
|----|----|------|------|
| R-01 | `calculateEffectiveLevel` | 跨层全局 max，忽略语义文档 | §2.2 精确作用域优先；无授权 → NONE → deny |
| R-02 | deny reason | 「explicitly denied」误用 | 「No permission」/ level 不足 |
| R-03 | Team Token 身份 | claims 无稳定 principal；checker 只认 user | §5.1–5.2 |
| R-04 | 授权提权 | 有 Org IAM 即可赋任意 Role/policy | §3.3 闭包校验 |
| R-05 | 列表过滤 | Org READ 全量 list | §4.2 可访问实例并集 |
| R-06 | Role 主路径 | Direct Grant + Role 双轨 | §3.2 收敛；管理台只暴露 Role |

### P1 — 模型完整

| ID | 项 | 目标 |
|----|----|------|
| R-07 | MANAGEMENT 蕴含 | §2.4 方案 A 进引擎 |
| R-08 | 项目成员管理 | 预置 `project_admin` + 显式赋值（D3） |
| R-09 | Application 主体 | 凭证 → APPLICATION principal → Role |
| R-10 | `PROVIDER_TEMPLATES` 等资源注册 | ResourceType 目录与路由一致，禁止未注册类型上路由 |
| R-11 | org_id 默认 1 | 单租户过渡期保留；多租户前改为强制上下文，禁止静默回落 |
| R-12 | collectRoleGrants 重复上卷 | 单次收集、正确标注 scope，避免重复与错误 ScopeID |

### P2 — 工程与文档

| ID | 项 | 目标 |
|----|----|------|
| R-13 | 缓存 | 未实现前保持正确；实现时按 Role 变更精确失效 |
| R-14 | 批量 check | list 场景批量/SQL 下推 |
| R-15 | 文档收敛 | 本文为裁决与模型 SoT；旧 v2/粒度文头注「部分废止」 |
| R-16 | 审计 | 赋 Role / 改 policy 全量审计；system_admin 访问可区分 |

---

## 7. 实施阶段

### Phase A — 语义纠偏（不改表或小改）

1. 重写 `calculateEffectiveLevel` 为 §2.2  
2. 修正 deny reason  
3. 单测：  
   - 仅 Org WRITE → WS 操作允许  
   - WS READ + Org WRITE → effective READ  
   - 无 Role → deny  
   - 多 Role 同层 max  

**出口**：裁决与 D1/D2 一致；无行为回归清单。

### Phase B — Role 主路径与防提权

1. 管理 API：赋 Role / 改 policy 走 §3.3  
2. 预置 Role：`org_admin` / `project_admin` / `workspace_admin` / `developer` / `viewer` 与 policy 对齐现网路由  
3. Direct Grant API deprecated；UI 只保留 Role  
4. 迁移脚本：历史 permission 行 → 自定义 Role 或映射到预置 Role  

**出口**：日常运维只碰 Role；提权用例被拒。

### Phase C — 身份与列表

1. Checker 支持 USER / TEAM / APPLICATION  
2. Team Token 24h + 吊销；中间件设正确 principal  
3. Workspace/Project 列表按可访问 id 过滤  
4. 创建 WS 绑定 workspace_admin Role  

**出口**：自动化链路可用；最小权限用户 list 正确。

### Phase D — 清理

1. 停止写入 direct permission 表（或表只读）  
2. 删除死代码与矛盾文档标注  
3. ResourceType 注册表 + CI 检查路由字符串  

---

## 8. 测试与验收标准

| 场景 | 期望 |
|------|------|
| 用户无任何 Role | 业务 API 403，reason=No permission |
| Org 级 Role 含 WORKSPACES WRITE | 可改任意 WS（该 org） |
| 仅 WS scope Role WRITE | 只能改该 WS；list 仅见该 WS（及规则允许的） |
| 用户同时 Org WRITE + 该 WS READ（同资源） | 该 WS 上 effective=READ |
| 未赋 project_admin | 不能改项目成员 |
| 显式 project_admin@projectP | 可管理 P 的成员与 P 内 Role（闭包内） |
| Team Token | 仅获该 Team 的 Role；过期/吊销 401 |
| 低权限用户尝试赋 admin Role | 403 |
| system_admin | 平台 API 可用；旁路有审计日志；业务侧逐步收敛到仅平台 API |
| 撤销 Pool→WS 授权后 | Pool Token 访问任务/state/锁 **403** |
| 普通 STATE READ | 不能 download/retrieve 完整 state |
| 路径 WS-A + 子资源属 WS-B | 404/403，禁止 IDOR |
| Team Token 无 user_id | 可认证；主体 TEAM；最长 24h；按 token_name 可吊销 |
| expires_at datetime-local | 赋 Role / Grant 成功写入并在到期后失效 |

### 8.1 自动化测试（已落地）

| 包 | 覆盖 |
|----|------|
| `internal/application/service` | `calculateEffectiveLevel` 作用域优先 / 过期 / NONE；AppSecret 哈希；Team Token 24h 与按名吊销 |
| `internal/middleware` | `checkPoolWorkspaceAccess` 仅 `status=active` |
| `internal/handlers` | `parseFlexibleTime`（RFC3339 + datetime-local） |
| `services` | `GetVariableInWorkspace` 跨 WS 拒绝 |

---

## 9. 明确不在本次整改范围 / 后续

- 临时任务 Webhook 权限半成品：另案，不进入 Role 主模型  
- 多组织完整租户隔离：R-11 过渡期保留单 org 默认，但 **禁止非法 org_id 静默变 1**（已修）  
- **Pool ACL 与 IAM 正交**，但 **撤销生效与查询过滤属于安全必修，已纳入 §12（不可再排除）**  
- Application Role 独立表 / 前端入口改道：§12 R-S08 部分完成（secret 哈希），角色写入路径仍待 Phase B  

---

## 10. 文档与代码锚点

| 类型 | 路径 |
|------|------|
| **本报告（SoT）** | `docs/iam/32-iam-remediation-report.md` |
| 裁决实现 | `backend/internal/application/service/permission_checker.go` |
| 中间件 | `backend/internal/middleware/iam_permission.go` / `pool_token_auth.go` / `middleware.go` |
| IAM 路由 | `backend/internal/router/router_iam.go` / `router_workspace.go` |
| Role 实体 | `backend/internal/domain/entity/role.go` 等 |
| 权限参考（需随整改更新） | `docs/security/iam-permissions-reference.md` |

---

## 12. 高危遗漏补录（安全 Review 追加）

> 初版将部分运行面问题排除或弱化。以下条目 **一律纳入整改范围**；状态为 2026-07-16 落地情况。

| ID | 级别 | 问题 | 根因 | 整改 | 状态 |
|----|------|------|------|------|------|
| **R-S01** | Critical | 已撤销 Pool→WS 授权仍有效 | `checkPoolWorkspaceAccess` 未过滤 `status`；Revoke 仅改 status | 查询强制 `status=active` | **已修** + 单测 |
| **R-S02** | Critical | 普通 READ 可下载完整 State | `/download` 与旧 `/state-versions/:version` 仅 STATE/ORG READ；`/retrieve` 才要 SENSITIVE | 下载路由与 retrieve 对齐：`WORKSPACE_STATE_SENSITIVE` / ORG ADMIN / MANAGEMENT ADMIN | **已修** |
| **R-S03** | High | 父 Workspace 与子资源未绑定（IDOR） | 路由鉴权 WS-A，handler 按全局子资源 ID 操作 B | `GetVariableInWorkspace`；`loadTaskInPathWorkspace`（Cancel/Comment）；同类 handler 继续扫 | **已修关键路径**；Run Trigger 等需继续扫 |
| **R-S04** | High | 组织作用域与路径脱钩 | 中间件只读 query `org_id`，非法回落 1 | 优先 path `:org_id`；非法 org_id **400**，缺失单租户默认 1 打日志 | **已修** |
| **R-S05** | High | `system_admin` 绕过全部业务 IAM 且无审计 | 中间件直接 `Next()` | **业务 IAM 取消 bypass**（`RequirePermission`/`RequireAnyPermission`/`RequireWorkspacePermission`/`/permissions/check`）；平台 API 仅 `RequireSystemAdmin()` | **已修** |
| **R-S06** | High | Team Token 全链路不可用 + 吊销/永不过期 | JWT 在识别 type 前强制 `user_id`；吊销用数字 id 而表无 id；UI 永不过期 | type 优先；**checker 真 TEAM principal**（仅 team grant/role）；按 `token_name` 吊销；最长 24h；活跃 name 唯一；前端对齐 | **已修** |
| **R-S07** | High | 临时授权写了等于没写 / 格式失败变永久 | Grant Handler 丢弃 `expires_at`；Role 只收 RFC3339 | `parseFlexibleTime` + Grant/Batch/AssignRole/TeamRole 写入 | **已修** |
| **R-S08** | High | Application 主体不可运行 + Secret 明文 | 前端 APPLICATION 仍写 user_roles；Secret 明文等值比较 | Secret **SHA-256 存储 + 兼容旧明文**；Role 存储改道与前端入口 **待 Phase B** | **部分** |
| **R-S09** | Medium | 列表配置泄露 / manifest-summary 仅 JWT / SSO token 在 query | list 带 `state_config` 等；summary 无 IAM；redirect `?token=` | list 最小化；summary 挂 WORKSPACES/MANAGEMENT READ；token 改 **URL fragment** + 前端读 hash | **已修** |

### 12.1 与初版 §9 的修正

- ~~「Pool 授权排除在范围外」~~ → **撤销不生效是 Critical，不可排除**。Pool ACL 仍可与用户 IAM 正交，但必须正确 enforce。  
- `system_admin`：**当前问题**（全业务 bypass），不是仅目标态描述；审计已加，收敛未完。  
- Team Token：问题比「R-03 principal 缺失」更深——**认证阶段即 401**。  

### 12.2 代码锚点（补丁）

| 项 | 路径 |
|----|------|
| Pool active 过滤 | `backend/internal/middleware/pool_token_auth.go` |
| State 敏感下载 | `backend/internal/router/router_workspace.go` |
| 变量/任务 WS 绑定 | `workspace_variable_service.go` / `workspace_task_controller.go` |
| org_id / admin 审计 | `backend/internal/middleware/iam_permission.go` |
| JWT team_token | `backend/internal/middleware/middleware.go` |
| Team Token 24h / 吊销 | `team_token_service.go` / `team_token_handler.go` / `TeamDetail.tsx` |
| expires 解析 | `permission_handler.go` `parseFlexibleTime` / `role_handler.go` |
| App secret | `application_service.go` |
| list 最小化 | `workspace_service.go` `toWorkspaceListItem` |
| manifest-summary | `router_manifest.go` |
| SSO fragment | `sso_handler.go` / `SSOCallback.tsx` |
| 裁决语义 | `permission_checker.go` `calculateEffectiveLevel` |

---

## 11. 变更记录

| 日期 | 说明 |
|------|------|
| 2026-07-16 | 初版落库。锁定 D1–D5；定义裁决、Role 主路径、Team Token、列表过滤与分期实施。 |
| 2026-07-16 | 补录 §12 九项高危；落地 Pool/State/IDOR/org/JWT Team Token/expires/secret/list/SSO 等补丁与单测；修正「Pool 排除」错误。 |
| 2026-07-16 | 目标态落地：业务侧取消 system_admin bypass；CheckPermission 支持 TEAM/APPLICATION principal；RequireAny org_id 对齐；GetComments IDOR；token_name 活跃唯一。 |

---

*评审结论：按本报告实施后，IAM 从「文档一套、代码一套、Grant/Role 双轨」收敛为「Role 主模型 + 明确作用域 + 默认拒绝 + 自动化主体一等公民」。*
