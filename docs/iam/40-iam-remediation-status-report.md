# IAM 整改状态与测试覆盖报告

| 字段 | 内容 |
|------|------|
| **文档编号** | 40 |
| **日期** | 2026-07-17 |
| **性质** | 进度核实报告（含证据）+ 待办与建议 |
| **权威关系** | 产品语义以 [`32`](./32-iam-remediation-report.md) 为准；历史缺口基线见 [`35`](./35-iam-security-review-report.md) / [`36`](./36-iam-remaining-issues-and-fix-plan.md)；波次 backlog 见 [`37`](./37-iam-fix-recommendations.md)；D5 细则见 [`39`](./39-direct-grant-retirement.md)；Application 见 [`38`](./38-application-principal-integration.md) |
| **证据来源** | 源码锚点 + `go test -cover`（本机 2026-07-17）+ 测试函数清单 |
| **结论一句话** | **Wave A/B/C 安全主路径与 D5 Direct Grant HTTP 下线（含 Application Role）代码侧已基本闭合；多租户公网上线仍被运维 SQL 流水线、列表鉴权、MANAGEMENT 引擎化与「业务控制器覆盖极薄」卡住。整体修复约 65–70%。** |

---

## 0. 执行摘要

### 0.1 进度条（主观加权）

```text
Wave A/B/C  安全主路径 / IDOR          ████████████████████░  ~95%
D5 Direct Grant 下线 + App Role        ██████████████████░░  ~90%
Application principal（选项 A）        ████████████████░░░░  ~80%
R0 上线阻断（运维 SQL / 契约）         ████████░░░░░░░░░░░░  ~40%
R1 安全扫尾                            ███████████░░░░░░░░░  ~55%
R2 架构终态（list / MANAGEMENT 等）    █████████░░░░░░░░░░░  ~45%
R3 测试门禁 / e2e                      ████████░░░░░░░░░░░░  ~40%
────────────────────────────────────────────────────────
整体「IAM issue 修复」                 ██████████████░░░░░░  ~65–70%
多租户公网「可宣称安全上线」           ██████████░░░░░░░░░░  ~50%
单组织内网有条件可用                   ███████████████░░░░░  ~75%
```

### 0.2 上线判断（建议）

| 场景 | 判断 | 条件 |
|------|------|------|
| 多组织公网 | **不建议** | 先完成 §6 P0（SQL 流水线 + 关键路径 e2e）与 R1 双条件扫尾 |
| 单组织内网 / 演示 | **有条件可用** | 跑齐 §5 全部 patch；确认 `IAM_SINGLE_TENANT`；禁止依赖 Direct Grant UI |
| 仅合并开发主干 | **可** | 不回退 fail-closed / Role 主路径；R0/R2 进 backlog 不可丢 |

### 0.3 测试覆盖一句话

- **裁决引擎与 IAM service 较稳**（`permission_checker` **80%**、IAM-ish service **~79%**）。
- **handlers 全包仅 10.8%**；IAM 子集约 **53%**——有点测，非端点全集。
- **controllers 1.8% / services 12.5% / persistence 0%**——业务面 IDOR 回归锁不足。

---

## 1. 产品决策落地度（相对 32 号 D1–D5）

| # | 决策 | 落地状态 | 证据摘要 |
|---|------|----------|----------|
| **D1** | NONE = 拒绝 | ✅ | `permission_checker` 无 grant → deny；单测 `TestCalculateEffectiveLevel_NoGrantsIsNone`、`TestCheckPermission_NoGrantsDenied` |
| **D2** | 精确作用域优先 | ✅ | `TestCalculateEffectiveLevel_ScopePriority`、`TestCheckPermission_WorkspaceReadBeatsOrgWrite` |
| **D3** | 负责人须显式 Role | ✅ 设计/主路径 | 无「创建者隐式 ADMIN」；workspace 创建走 `workspace_admin` Role 赋值 |
| **D4** | Team Token 自动化 | ✅ 主规格 | 24h 上限、NULL expires 拒、Validate 真 schema、Generate 事务；配额 best-effort |
| **D5** | Role 主模型 | ✅ 管理写路径 | Direct Grant HTTP **410**（USER/TEAM/**APPLICATION**）；FE 仅 Role；App Role API；遗留 grant **Checker 只读双读** |

---

## 2. 分波次进度与证据

### 2.1 Wave A / B / C（安全 + IDOR）— ~95%

文档 `36` 已关闭的 C 波代码项在当前树仍成立：

| 项 | 状态 | 代码 / 测试证据 |
|----|------|-----------------|
| Application org 绑定 | ✅ | `application_org_binding_test.go`：`CreateRejectsCrossOrg`、`GetCrossOrg404`、`UpdateDeleteRegenerate_CrossOrg` 等 |
| Role 防提权 fail-closed | ✅ | `role_anti_escalation.go` + `TestRemoveRolePolicy_FailClosedWithoutGuard`、`TestCloneRole_FailClosedWithoutGuard`、`TestNilCheckerFailClosed` |
| Assign 提权 403 | ✅ | `TestAssignRole_PrivilegeEscalation403`、`TestAssignTeamRole_Escalation403` |
| 全局 task log 绑 WS | ✅ | `task_log_controller.go` / `terraform_output_controller.go` 调 `RequireWorkspacePermission` |
| 业务面不 bypass 超管 | ✅ | `TestRequireWorkspacePermission_SystemAdminNoBypass` |
| 临时权限双键 | ✅ | `temp_permission_dual_key_test.go` |
| Team Token | ✅ 主路径 | `team_token_*_test.go`（24h、配额、NULL expires、Validate） |
| 默认 org=1 仅单租户 | ✅ | `authOrgFromContext` / `resolveOrgScopeID` + `IAM_SINGLE_TENANT` |

**仍开放（代码债，非全未修）：**

| 项 | 说明 | 证据 / 位置 |
|----|------|-------------|
| 全局 `/tasks/:id/*` 双轨 | 全局路径先 `First(&task, id)` 再绑 WS；WS 下路径有 `loadTaskInPathWorkspace` | `task_log_controller.go:38` vs `workspace_task_controller.go` `loadTaskInPathWorkspace` |
| 部分 service 裸 ID | R1-1 未彻底 | 见 `37` §3 R1-1；变量侧已有 `GetVariableVersionsInWorkspace` 等部分下沉 |
| 特权 Role 仅 `admin` 名 | R1-4 | `isPrivilegedSystemRole` 仍窄（建议扩 `is_system`） |

### 2.2 Application principal（R0-3 选项 A）— ~80%

| 能力 | 状态 | 证据 |
|------|------|------|
| AgentAuth → APPLICATION principal | ✅ | middleware + `application_principal_handler_test.go` |
| `/api/v1/app/*`（whoami / check / workspaces） | ✅ | `router_application.go`；workspace 测：`TestApplicationListWorkspaces_*`、`GetWorkspace_*` |
| principal_id = app_key | ✅ | `resolveApplicationPrincipalID` + `application_principal_id_test.go`；Checker 别名 `ExpandApplicationPrincipalIDs` |
| workspace_tag_filter AND | ✅ | `patch_applications_workspace_tag_filter.sql`；`TestApplicationListWorkspaces_TagFilter`、`TagMismatch404` |
| Application Role 表 + 求值 | ✅ | `entity/application_role.go`；`QueryApplicationRoles` in `permission_repository_impl.go` + `permission_checker.go:808` |
| Application Role HTTP | ✅ | `role_application_handler.go`；路由 `POST/GET/DELETE .../applications/:id/roles`；`TestRoleHandler_ApplicationRoleAssignListRevoke` |
| 生产 e2e / 密钥轮换 | ⚠️ 弱 | 有 `docs/iam/scripts/app-principal-smoke.sh`；非 CI 门禁 |

### 2.3 D5 Direct Grant 下线 — ~90%

| 步骤 | 状态 | 证据 |
|------|------|------|
| USER/TEAM grant/batch/preset → 410 | ✅ | `rejectRetiredDirectGrant`；`TestPermissionHandler_GrantPermission_UserRetired410` / `TeamRetired410` / `GrantPreset_*` / `BatchGrant_*` |
| APPLICATION grant → 410 | ✅（本波） | `TestPermissionHandler_GrantPermission_ApplicationRetired410`；消息指向 Role API |
| 应急开关 | ✅ | `IAM_ALLOW_DIRECT_GRANT=1`；`TestPermissionHandler_GrantPermission_InvalidExpires_WithBypass` |
| FE 仅 Role | ✅ | `frontend/.../GrantPermission.tsx`：全部主体「分配角色」；App → `/iam/applications/.../roles` |
| Creator → Role | ✅ | `workspace_controller.grantCreatorPermissions` → `AssignBuiltinRoleToUser(..., "workspace_admin")`；缺角色回退内部 preset |
| 存量迁移 SQL | ✅ 脚本 / ⚠️ 未强制执行 | `patch_legacy_direct_grants_to_roles.sql`（**不删**原 grant 行） |
| Checker 只读 legacy | ✅ 有意双读 | 求值仍读 `*_permissions` + Role |
| 清理遗留 grant 行 | ❌ 运维后置 | `39` §4 说明手工确认后再删 |

**锚点摘录：**

```go
// permission_handler.go
const directGrantRetiredMsg = "Direct Grant is retired; assign a Role instead (POST /iam/users|teams|applications/:id/roles)."
// rejectRetiredDirectGrant → 410；IAM_ALLOW_DIRECT_GRANT 可旁路
```

```go
// router_iam.go
POST   /applications/:id/roles
GET    /applications/:id/roles
DELETE /applications/:id/roles/:assignment_id
```

### 2.4 Wave R0 — ~40%

| 项 | 脚本 / 行为 | 落地 |
|----|-------------|------|
| 超管绑 admin Role | `patch_system_admin_iam_roles.sql` | ⚠️ 需进安装流水线 |
| admin Role 含 IAM_* | `patch_admin_role_iam_policies.sql` | ⚠️ 同上 |
| team_tokens 部分唯一 | `patch_team_tokens_active_unique.sql` | ⚠️ 同上 |
| temp perm user_id 索引 | `patch_temp_permission_user_id_index.sql` | ⚠️ 同上 |
| workspace_admin seed | `patch_workspace_admin_role_ensure.sql` | ⚠️ 本波新增，同左 |
| application_roles 表 | `patch_iam_application_roles.sql` | ⚠️ 本波新增，同左 |
| legacy DG → Role | `patch_legacy_direct_grants_to_roles.sql` | ⚠️ 可选迁移，同左 |
| 租户 env 契约 | `IAM_SINGLE_TENANT` | ⚠️ 逻辑有；单函数合并/启动日志未完全固化 |

**证据：** `backend/migrations/` 下相关 patch 文件共 **9** 份（含 Application/D5 相关）；**本报告不声称任何环境已 apply**。

### 2.5 Wave R1 / R2 剩余架构 — 见 §6

| 大项 | 进度 | 备注 |
|------|------|------|
| R1-1 双条件 service | 部分 | 变量等已有 `*InWorkspace`；未全仓扫清 |
| R1-2 全局 API 策略 | 部分 | 日志路径已绑权限；双轨仍在 |
| R1-3 Token 配额 DB 硬约束 | 未 | 事务+行锁 best-effort |
| R1-4 全部 is_system 限制 | 未 | 仅特权名单窄 |
| R2-2 列表 accessibility | 未 | list 与 detail 语义可能不一致 |
| R2-3 MANAGEMENT 引擎蕴含 | 未 | 路由拼装 |
| R2-5 访问规范硬门禁 | 未 | 文档建议级 |

---

## 3. 测试覆盖报告（实测）

### 3.1 复现命令与环境

```bash
cd backend
go test ./internal/handlers/ \
        ./internal/application/service/ \
        ./internal/middleware/ \
        ./internal/domain/valueobject/ \
        -coverprofile=/tmp/iam_cover.out -count=1
# 旁证（另测）：
go test ./controllers/ ./services/ -cover -count=1
```

| 项 | 值 |
|----|-----|
| 测量日 | 2026-07-17 |
| 工具 | Go `testing` + `go tool cover` / coverprofile 语句累计 |
| 注意 | **statement coverage**；非 branch；handlers 包含大量非 IAM 代码 |

### 3.2 包级结果

| 包 | statement coverage | 语句 (hit/total) | 判读 |
|----|-------------------|------------------|------|
| `internal/domain/valueobject` | **88.8%** | 71/80 | 基础类型扎实 |
| `internal/application/service` | **53.5%** | 781/1459 | IAM 高、Agent 等拖低 |
| `internal/middleware` | **53.9%** | 428/794 | 鉴权中等 |
| `internal/handlers`（全量） | **10.8%** | 909/8396 | **不可代表 IAM** |
| `controllers`（旁证） | **~1.8%** | — | 业务面极薄 |
| `services`（旁证） | **~12.5%** | — | 同上 |
| `internal/infrastructure/persistence` | **0%** | — | 仓储无测 |

### 3.3 IAM 热点文件

| 文件 | coverage | hit/total stmts | 风险注释 |
|------|----------|-----------------|----------|
| `permission_checker.go` | **80.0%** | 264/330 | 裁决主路径较好 |
| `permission_service.go` | **85.7%** | 90/105 | Grant/Revoke/Preset/BuiltinRole |
| `team_token_service.go` | **85.1%** | 97/114 | Token 生命周期较好 |
| `app_principal_aliases.go` | **87.1%** | 27/31 | id↔key 展开 |
| `role_anti_escalation.go` | **63.5%** | 80/126 | scope 校验分支偏弱 |
| `role_handler.go` | **65.4%** | 293/448 | CRUD+赋权主路径有测 |
| `role_application_handler.go` | **56.7%** | 72/127 | 新路径有 assign/list/revoke 测 |
| `permission_handler.go` | **51.8%** | 183/353 | 410 主路径有；Batch 深路径薄 |
| `application_principal*`（合计） | **~71.6%** | 106/148 | app 面相对好 |
| `org_binding.go` | **38.2%** | 21/55 | **弱**——跨 org 边界分支 |
| `role_clone.go` | **23.3%** | 14/60 | **弱**——仅 fail-closed 点测 |

### 3.4 IAM 聚合切片（更有参考价值）

| 切片定义 | coverage | hit/total |
|----------|----------|-----------|
| handlers 中文件名含 `role_` / `permission_` / `application_` / `org_binding` / `team_` | **53.4%** | 794/1488 |
| service 中 `permission_` / `role_` / `team_token` / `app_principal` | **~79.0%** | 558/706 |

### 3.5 用例规模

| 范围 | 约数 |
|------|------|
| handlers + service + middleware 全部 `Test*` | **~223** |
| IAM 聚焦文件内 `Test*` | **~129** |

### 3.6 关键路径已有测试（抽样清单，作「有锁」证据）

**裁决 / Role**

- `TestCalculateEffectiveLevel_*`、`TestCheckPermission_WorkspaceReadBeatsOrgWrite`、`TestCheckPermission_RoleAtOrg`、`TestCheckPermission_TeamRoleGrant`
- `TestEnsureCanAssignRole_*`、`TestEnsureCanAddRolePolicy`、`TestEnsureCanCloneRole`、`TestNilCheckerFailClosed`

**HTTP 管理面**

- Direct Grant 410：`User` / `Team` / `Application` / Preset / Batch
- Role：`AssignAndRevoke`、`TeamRoleAssignListRevoke`、`ApplicationRoleAssignListRevoke`
- 防提权：`AssignRole_PrivilegeEscalation403`、`Remove/Clone_FailClosedWithoutGuard`
- App org：`CreateRejectsCrossOrg`、跨 org 404 系列
- App 面：whoami/check、list/get workspace、tag 过滤

**身份**

- Team Token：Generate 24h / 配额 / Validate / NULL expires
- Temp permission 双键
- `TestAssignBuiltinRoleToUser`（幂等 + 缺角色）

### 3.7 覆盖率解读（避免误读）

| 误读 | 正确理解 |
|------|----------|
| 「handlers 10.8% → IAM 很差」 | 包内大量非 IAM handler 稀释；IAM 子集 ~53% |
| 「service 53% → 引擎不稳」 | IAM-ish ~79%、checker 80%；Agent 等拉低整包 |
| 「测试绿 = 攻击面闭合」 | 无端点全集、无多租户 e2e、controllers 几乎无锁 |
| 「coverage 高 = 安全属性测全」 | 多为 happy path + 若干 403/410；并发/竞态/迁移幂等仍缺 |

---

## 4. 文档与代码一致性（已知漂移）

| 文档表述 | 当前代码 | 处理建议 |
|----------|----------|----------|
| `37` §0「D5 Direct Grant 仍开放」 | **已关闭 HTTP 写**（含 APPLICATION） | 以本文 / `39` 为准修订 37 摘要 |
| `37` R2-1「APPLICATION 保留组织 DG」 | FE+API 改 Role；DG 410 | 同上 |
| `37` R2-1「迁移未做」 | **SQL 已写** `patch_legacy_direct_grants_to_roles.sql` | 状态改为「脚本就绪、环境未强制」 |
| `36`「Application 生产挂载仍需确认」 | `/api/v1/app/*` 已挂 | 改为「需环境验收 e2e」 |
| `34` 进度宣称 | 不作完成证据 | 继续以代码 + 本报告为准 |

---

## 5. 迁移 / 运维资产清单（证据：仓库内文件）

按建议执行顺序（**未在本报告环境执行**）：

| 顺序 | 文件 | 用途 |
|------|------|------|
| 1 | `patch_system_admin_iam_roles.sql` | 超管 ↔ org admin Role |
| 2 | `patch_admin_role_iam_policies.sql` | admin Role 补 IAM_* |
| 3 | `patch_team_tokens_active_unique.sql` | 活跃 token 名唯一 |
| 4 | `patch_temp_permission_user_id_index.sql` | 临时权限索引 |
| 5 | `patch_workspace_admin_role_ensure.sql` | creator 所需 Role |
| 6 | `patch_applications_workspace_tag_filter.sql` | tag 列 |
| 7 | `patch_application_principal_id_to_app_key.sql` | 历史 id→key（可选） |
| 8 | `patch_iam_application_roles.sql` | App Role 表 |
| 9 | `patch_legacy_direct_grants_to_roles.sql` | 存量 DG → 合成 Role（不删 grant） |

验收提示：

1. 超管在各 org 有 admin Role 绑定。  
2. 创建 workspace 后 creator 有 `workspace_admin`（或日志中 fallback preset）。  
3. `iam_application_roles` 可 INSERT；`POST .../applications/:key/roles` 200。  
4. legacy 脚本后 USER 权限不降（双读期）。

---

## 6. 待做事项（按优先级）

### P0 — 上线前必须

| ID | 事项 | 验收 | 预估 |
|----|------|------|------|
| **P0-1** | 将 §5 SQL 写入安装/升级 runbook 与发布失败中止 | 新环境按文档安装后超管可运维；未跑 patch 文档声明「403 为预期」 | 0.5–1d |
| **P0-2** | 多租户：缺 `org_id` → 400 的集成测进 CI | 自动化断言 | 0.5d |
| **P0-3** | Application 冒烟进 CI 或 staging 清单 | smoke：有 Role → list WS 200；无 Role → 403；坏 secret → 401 | 0.5–1d |
| **P0-4** | 确认生产未开 `IAM_ALLOW_DIRECT_GRANT` | 配置审计 | 0.1d |

### P1 — 强烈建议同波次

| ID | 事项 | 验收 | 预估 |
|----|------|------|------|
| **P1-1** | R1-1：Variable/Resource/Task/AI 等 service **双条件** 查；禁业务入口裸主键 | 跨 WS id → 404，即使 controller 漏绑 | 2–3d |
| **P1-2** | 全局敏感 `/tasks/:id`：废弃写路径或强制二次绑定 + 表驱动测 | 跨 WS 403/404 锁死 | 1–2d |
| **P1-3** | R1-4：非超管不可 Assign **任意** `is_system` Role | 单测 403 | 0.5d |
| **P1-4** | 抬升弱覆盖：`org_binding`、`role_clone`、BatchGrant 深路径 | 见 §7 目标 | 1–2d |
| **P1-5** | controllers 关键 IDOR 路径表驱动进 CI | rollback / vsnap / run trigger / task log 等 | 2d |
| **P1-6** | 环境执行 legacy DG 迁移后 **抽样 diff** Role vs grant | 权限不降；再计划删 grant | 1d 运维 |

### P2 — 对齐 32 终态

| ID | 事项 | 验收 |
|----|------|------|
| **P2-1** | R2-2 列表 accessibility：`ListAccessibleScopeIDs` + workspaces list 过滤 | 仅 ws-a READ 时 list 不含 ws-b |
| **P2-2** | R2-3 MANAGEMENT 引擎蕴含 | 单测：MANAGEMENT WRITE ⇒ 精细写；精细不可反推 MANAGEMENT |
| **P2-3** | 确认双读无误后清理 `*_permissions` 业务行 | 删除脚本 + 回滚预案 |
| **P2-4** | Team Token 活跃数 DB 硬约束（若产品坚持 ≤2） | 约束或触发器 + 并发测 |
| **P2-5** | 合并 `isSingleTenantIAM` 单实现 + 启动日志打印 | 配置可观测 |
| **P2-6** | 修订 `37` 过时摘要；权威链指向本文 | 文档一致 |

### P3 — 工程硬化

| ID | 事项 |
|----|------|
| **P3-1** | `persistence` 层 Query*Roles / Grant 仓储单测 |
| **P3-2** | OpenAPI / `docs.go` 标注 grant 410 与 App Role 路径 |
| **P3-3** | 前端 Application 详情页 Role 列表/撤销 UI（管理台增强） |
| **P3-4** | 密钥轮换后旧 secret 立即失效测 |

---

## 7. 建议（策略与门禁）

### 7.1 近 2 周建议排期

```text
Week 1
  ├── P0-1 SQL runbook + 发布门禁
  ├── P0-2/P0-3 多租户 org + App smoke 进 CI/staging
  └── P1-4 弱文件覆盖补测（org_binding / clone / batch）

Week 2
  ├── P1-1 双条件 service 扫清（按 grep 清单）
  ├── P1-2 全局 task 路径策略落地
  ├── P1-3 is_system Assign 收紧
  └── P1-5 controllers IDOR 表驱动最小集
```

### 7.2 建议 CI 门禁（可渐进）

```bash
# 必绿（现已可做）
cd backend && go test ./internal/domain/valueobject/ \
  ./internal/application/service/ \
  ./internal/middleware/ \
  ./internal/handlers/ \
  ./controllers/ ./services/ \
  -count=1 -timeout 180s

# 建议增加的覆盖率底线（实现后 fail CI）
# - permission_checker.go          >= 75%
# - role_anti_escalation.go        >= 60%
# - IAM-ish handlers 切片          >= 55%  → 目标 70%
# - 禁止 controllers 新增裸 First(&task,  白名单文件除外（rg 门禁）
```

### 7.3 覆盖率目标（3 个迭代）

| 切片 | 当前 | 目标 T+1 | 目标 T+2 |
|------|------|----------|----------|
| permission_checker | 80% | 保持 ≥75% | ≥85% |
| role_anti_escalation | 63.5% | ≥70% | ≥80% |
| IAM-ish handlers | 53.4% | ≥60% | ≥70% |
| org_binding | 38% | ≥60% | ≥75% |
| role_clone | 23% | ≥50% | ≥70% |
| controllers（IAM 相关路径） | ~2% | 关键路径有表驱动 | 持续加 |

### 7.4 明确「不要做」

1. **不要**在未理解双读的情况下批量 DELETE `*_permissions`。  
2. **不要**默认开启 `IAM_ALLOW_DIRECT_GRANT`。  
3. **不要**恢复 USER/TEAM/APPLICATION 的平行 Direct Grant UI。  
4. **不要**用「全包 coverage 10%」单独否决合并——应看 IAM 切片 + 安全属性测。  
5. **不要**把 `is_system_admin` 重新做成业务 API 旁路。

### 7.5 成功标准（更新后的 DoD）

**最小闭环（可内网上线）：**

- [x] 业务授权 HTTP 主路径仅 Role（含 Application）  
- [x] Wave C 已知 P0 代码洞主路径关闭  
- [x] Application 选项 A 代码挂载  
- [ ] §5 SQL 在目标环境执行并验收  
- [ ] App smoke + 缺 org_id 行为在 CI/staging 可重复  

**对齐 32 终态：**

- [ ] list/detail 权限一致（R2-2）  
- [ ] MANAGEMENT 引擎蕴含（R2-3）  
- [ ] 遗留 Direct Grant 数据清理完成  
- [ ] controllers 关键路径 IDOR 有回归锁  

---

## 8. 风险登记册（当前）

| 风险 | 级别 | 现状 | 缓解 |
|------|------|------|------|
| 环境未跑 SQL → 超管/creator 403 或无 Role | 高 | 脚本有、流水线无 | P0-1 |
| 双读期权限「看起来重复」运维困惑 | 中 | 有意设计 | 文档 + 迁移后清理 |
| list 过宽 / detail 拒绝 语义不一致 | 中高 | R2-2 未做 | 排期 P2-1 |
| controllers 无测导致 IDOR 回归 | 高 | 1.8% | P1-5 |
| `org_binding` 低覆盖漏跨 org | 中 | 38% | P1-4 |
| Team Token 并发下可能 >配额 | 低–中 | best-effort | P2-4 或接受文档 |
| 应急 Direct Grant 被误开 | 中 | 环境变量 | P0-4 审计 |

---

## 9. 附录

### 9.1 文档权威链

```text
32 产品语义 SoT
  ↑
35 / 36 安全核实与残留（历史基线）
  ↑
37 修复波次 backlog（部分摘要已过时，以 40 更新）
  ↑
38 Application 集成
39 Direct Grant 下线细则
40 本文：状态 + 覆盖 + 待办（进度 SoT）
```

### 9.2 关键代码锚点索引

| 主题 | 路径 |
|------|------|
| Direct Grant 410 | `backend/internal/handlers/permission_handler.go` `rejectRetiredDirectGrant` |
| App Role HTTP | `backend/internal/handlers/role_application_handler.go` |
| App Role 路由 | `backend/internal/router/router_iam.go` |
| App Role 求值 | `backend/internal/application/service/permission_checker.go` `QueryApplicationRoles` |
| Creator Role | `backend/controllers/workspace_controller.go` `grantCreatorPermissions` |
| 防提权 | `backend/internal/application/service/role_anti_escalation.go` |
| 裁决引擎 | `backend/internal/application/service/permission_checker.go` |
| App 面 | `backend/internal/router/router_application.go` + `application_principal_*.go` |
| FE 授权 | `frontend/src/pages/admin/GrantPermission.tsx` |
| 迁移 | `backend/migrations/patch_*.sql`（§5） |

### 9.3 建议的回归命令（开发自检）

```bash
cd backend
go test ./internal/handlers/ -count=1 -run 'GrantPermission|BatchGrant|GrantPreset|ApplicationRole|AssignRole|CloneRole|RemoveRole|Application'
go test ./internal/application/service/ -count=1 -run 'CheckPermission|AssignBuiltin|EnsureCan|TeamToken|ExpandApplication|Temporary'
go test ./internal/middleware/ -count=1 -run 'RequireWorkspace|SystemAdmin'
cd ../frontend && npm run build
```

### 9.4 变更记录

| 日期 | 说明 |
|------|------|
| 2026-07-17 | 初版：汇总 A/B/C、R0–R3、D5/App Role 现状；写入 cover 实测与待办。 |

---

**报告结束。** 若需将本文结论同步回写 `37` §0/R2-1 过时表，或落地 P0 SQL runbook 补丁，可另开变更。
