# IAM 修复建议（可执行 Backlog）

> **日期**：2026-07-17  
> **权威关系**：产品语义以 [`32`](./32-iam-remediation-report.md) 为准；缺口核实基线见 [`35`](./35-iam-security-review-report.md) / [`36`](./36-iam-remaining-issues-and-fix-plan.md)  
> **本文角色**：在 Wave A–C **代码侧主路径已关** 的前提下，给出下一波 **仍建议做的修复**（运维、模型收敛、设计债、测试加固）  
> **原则**：先闭合上线阻断与可回归洞，再收敛架构；不回退已确认有效的 fail-closed / path 绑定 / 无 super-admin 业务旁路

---

## 0. 现状摘要（写建议前的共识）

| 已达标（勿回退） | 未达标 / 部分达标 |
|------------------|-------------------|
| D1 NONE 拒绝、D2 精确 scope 优先 | D5 Role 唯一主模型（Direct Grant 仍开放） |
| 业务 IAM 不 bypass `is_system_admin` | 列表按 effective READ 过滤（目前主要是字段脱敏） |
| 跨 org Application、跨 WS 子资源主路径绑定 | MANAGEMENT 引擎蕴含（仍靠路由拼装） |
| Role 防提权 + guard fail-closed | Application 一等 Role / 业务路由挂载未钉死 |
| Team Token 24h、Validate 真 schema、临时权限双键 | 运维 4 SQL 需进流水线；双轨全局 `/tasks/:id` |
| 核心包 `go test` 可绿 | 端点全集 HTTP 测 / e2e 仍薄 |

**上线判断（建议）**

| 场景 | 建议 |
|------|------|
| 多组织公网 | **先做 Wave R0 + R1** 再上 |
| 单组织内网 | **R0 必须**；R1 强烈建议同批 |
| 仅合并主分支继续开发 | R0 可异步，但 R1/R2 进 backlog 不可丢 |

---

## 1. 修复波次总览

```text
R0  上线阻断（运维 + 配置契约）     ── 1～2 人日，无代码或极少代码
R1  安全/一致性扫尾（仍可能被打）  ── 3～5 人日
R2  模型与架构收敛（对齐 32 终态）  ── 1～2 周
R3  测试与可观测性加固              ── 与 R1/R2 穿插
```

| Wave | 目标 | 不做完的后果 |
|------|------|----------------|
| **R0** | 取消 bypass 后系统可登录管业务；DB 约束到位 | 超管 403、token 竞态、环境漂移 |
| **R1** | 残留 IDOR/旁路/双条件查询闭合 | 点状越权、service 被内部误用 |
| **R2** | Role 主模型、list 鉴权、MANAGEMENT 引擎化 | 治理双轨、列表/对象语义不一致 |
| **R3** | 属性测试与 CI 门禁 | 回归靠人肉、文档宣称失真 |

---

## 2. Wave R0 — 上线阻断（优先执行）

### R0-1 四条 patch SQL 进安装/升级流水线

| SQL | 作用 | 优先级 |
|-----|------|--------|
| `backend/migrations/patch_system_admin_iam_roles.sql` | 活跃 `is_system_admin` 幂等绑 `admin@ORGANIZATION` | **必须** |
| `backend/migrations/patch_admin_role_iam_policies.sql` | 系统 admin Role 补齐 `IAM_*` 策略 | **必须（与上成对）** |
| `backend/migrations/patch_team_tokens_active_unique.sql` | 活跃 `token_name` 部分唯一索引 | 强烈建议 |
| `backend/migrations/patch_temp_permission_user_id_index.sql` | 临时权限 `user_id` 索引 | 建议 |

**建议动作**

1. 写入 `Makefile` / 部署 runbook：**成对执行前两条**，失败则中止发布。  
2. 验收 SQL（超管绑定数 ≥ 活跃组织数；admin Role 含 IAM_*）。  
3. 文档：安装说明写明「未跑 patch → 业务面 403 是预期行为，不是回归」。

**验收**

```sql
-- 超管在每个 org 有 admin Role（示意，以实际库名为准）
SELECT u.user_id, COUNT(ur.id) AS admin_org_bindings
FROM users u
LEFT JOIN iam_user_roles ur ON ur.user_id = u.user_id
  AND ur.scope_type = 'ORGANIZATION'
  AND ur.role_id = (SELECT id FROM iam_roles WHERE name = 'admin' AND is_system LIMIT 1)
WHERE u.is_system_admin = true AND u.is_active = true
GROUP BY u.user_id;
```

---

### R0-2 环境变量契约固化

| 变量 | 语义 | 建议默认 |
|------|------|----------|
| `IAM_SINGLE_TENANT` | 缺 `org_id` 时是否默认 org=1 | 多租户：**不设 / false**；单租户演示：`1` |

**建议动作**

1. 启动日志打印：`IAM_SINGLE_TENANT=%v`（避免静默错误配置）。  
2. 合并 `isSingleTenantIAM`（middleware）与 `middlewareIsSingleTenant`（handlers）为 **单一函数**（如 `internal/iam/tenant.go`），禁止拷贝分叉。  
3. `docker-compose.example.yml` / 部署模板注明多租户勿开。

**验收**：多租户下无 org 的管理 API → **400**；`IAM_SINGLE_TENANT=1` → 可默认 1。

---

### R0-3 Application principal 产品一句话决策

二选一写进 32/README，避免「代码有、生产永远不走」：

| 选项 | 动作 |
|------|------|
| **A. 启用** | Agent/API 路由挂 `AgentAuth` + 业务 IAM；补 e2e：App grant → 200，无 grant → 403 |
| **B. 预留** | 文档明确「仅密钥校验 / 未进业务 IAM」；checker 路径保留但不宣称已生产可用 |

**已记录决策（2026-07-17，修订）**：**A（启用）**。

| 项 | 落地 |
|----|------|
| 认证 | `X-App-Key` + `X-App-Secret` → APPLICATION principal（`principal_id=app_key`） |
| API 面 | `/api/v1/app/whoami`<br>`POST /api/v1/app/permissions/check`<br>`GET /api/v1/app/workspaces`（org `WORKSPACES` READ + **auth_org** + **tag 过滤**）<br>`GET /api/v1/app/workspaces/:id`（org 归属 + tag 匹配 + IAM） |
| 授权 | Direct Grant 存 **app_key**（handler 将数字 id 解析为 key） |
| Tag 匹配 | `applications.workspace_tag_filter` JSON：`{"env":"prod"}` 或 `{"env":["prod","staging"]}`，与 `workspace.tags` **AND** 匹配；空=不限 tag |
| 兼容 | Checker 展开 `app_key`↔数字 id，历史 grant 仍可命中 |
| Agent 任务 | **仍走 Pool Token**（`/agents/*`）；与 App IAM 面分离 |
| 前端 | GrantPermission 主体选项值为 `app_key` |

**非 A 的路径**：**B（预留）** 已废弃为本产品选择。

---

## 3. Wave R1 — 安全与一致性扫尾

### R1-1 服务层双条件查询（闭合「controller 绑、service 全局」）

| 项 | 建议 |
|----|------|
| Variable | 新增 `GetVariableVersionInWorkspace(ws, variableID, version)`；controller 只调此 API |
| Resource / 其它 | 全仓 grep `GetResource(` / `GetVariable(` / `First(&task,`：业务入口禁止裸 ID |
| AI summary 等 | 统一 `Where("id = ? AND workspace_id = ?", ...)` 或共用 `loadTaskInPathWorkspace` |

**验收测试**

- 同 variable_id 若仅存在于 ws-b，path=ws-a → 404（即使 controller 漏绑，service 也拒）。  
- 任务创建：跨 WS vsnap id → **400**。

**涉及文件（示意）**

- `services/workspace_variable_service.go`  
- `controllers/workspace_variable_controller.go`  
- `controllers/ai_summary_controller.go`  
- 新增/扩展 `*_binding_test.go`

---

### R1-2 全局敏感 API 策略二选一

**推荐方案（干净）**

- 废弃或 301/410 全局：  
  `GET /api/v1/tasks/:task_id/logs`、stream 等  
- 统一：  
  `GET /api/v1/workspaces/{id}/tasks/{task_id}/logs`

**过渡方案（已基本落地）**

- 保留全局路径，但强制：load task → `RequireWorkspacePermission(ws, READ)` → fail-closed（iam nil → 500）。  
- 禁止仅 org 级 TASK_LOGS + 裸 id。

**建议**：过渡方案保留 ≤1 个大版本；前端改走 workspace 路径后删全局写路径。

---

### R1-3 Team Token 配额硬化 — **部分落地**

| 层 | 状态 |
|----|------|
| DB | `patch_team_tokens_active_unique`（活跃 name 唯一） |
| 配额=2 | 事务内 Count+Create；PG 对 `teams` **FOR UPDATE** 串行化 Generate；**仍 best-effort**（无「最多 2 条」DB 约束） |
| 文档 | 接受极端并发可能 >2；生产靠事务+行锁显著降低 |

---

### R1-4 特权系统 Role 名单

```go
// 现状：仅 name=="admin"
// 建议：配置化或统一规则
// 选项 A：全部 is_system 仅超管可 Assign（简单、偏严）
// 选项 B：IAM_PRIVILEGED_SYSTEM_ROLES=admin,platform_ops（可配置）
```

**建议默认**：**选项 A**（`is_system` 均不可被非超管分配）；策略只读逻辑已覆盖 `IsSystem`，与 Assign 对齐。

**测试**：非超管 Assign 任意 `is_system` Role → 403。

---

### R1-5 Direct Grant 接口收紧（R2 前置，可先半步）

在未删 API 前，至少：

1. 路由仅 `RequireSystemAdmin` 或 `IAM_*` ADMIN + **同等闭包检查**（与 `EnsureCanAddRolePolicy` 同级）。  
2. OpenAPI / 前端标注 **deprecated**。  
3. 审计日志强制记录 grant 全字段。

完整下线放到 R2-1。

---

### R1-6 写路径 HTTP 表驱动测试（最小集）

| 用例 | 期望 |
|------|------|
| Resource Rollback / StartEditing 跨 WS | 404 |
| Create task + 跨 WS vsnap | 400 |
| GetVariableVersion 跨 WS | 404 |
| 全局 task logs 跨 WS（若保留） | 403 |
| CreateRunTrigger 无 target WRITE | 403（已有则保持） |
| App Update/Delete/Regenerate 跨 org | 404（已有则保持） |
| Remove/Clone guard=nil | 500（已有则保持） |
| system_admin 业务 RequireWorkspace | 403 当无 Role（已有则保持） |

**CI 门禁（保持 + 可选增强）**

```bash
cd backend
go test ./internal/domain/valueobject/ \
  ./internal/application/service/ \
  ./internal/middleware/ \
  ./internal/handlers/ \
  ./controllers/ \
  ./services/ \
  -count=1 -timeout 180s
```

可选：`rg` 禁止 controllers 新增 `First\(&task,`（白名单文件除外）。

---

## 4. Wave R2 — 对齐 32 号终态（架构收敛）

### R2-1 Role 唯一主模型（D5）— **主路径已下线 Direct Grant（2026-07-17）**

| 步骤 | 状态 |
|------|------|
| 1 | 管理台 USER/TEAM 仅 Role；APPLICATION 保留组织 Direct Grant | ✅ |
| 2 | `POST /permissions/grant|batch|preset`：USER/TEAM → **410**；`IAM_ALLOW_DIRECT_GRANT=1` 应急 | ✅ |
| 3 | 数据迁移：存量 → legacy Role（**未做**，Checker 仍可读表） | 待办 |
| 4 | checker 只读 legacy；HTTP 写停（USER/TEAM） | ✅ 半完成 |
| 文档 | `39-direct-grant-retirement.md` | ✅ |

**验收**：业务管理员无法经 UI/API 写入 `*_permissions` 单条 grant；赋权只能走 Role。

---

### R2-2 列表 accessibility（32 §4）

| 步骤 | 内容 |
|------|------|
| 1 | checker 增加 `ListAccessibleScopeIDs(principal, resourceType, minLevel)` |
| 2 | `GET /workspaces`（及同类 list）按 id 并集过滤或 SQL 下推 |
| 3 | 仅 Org 观察者 Role：可 list 全量元数据（产品定义） |
| 4 | 仅单 WS Role：list 只含该 WS；detail 行为一致 |

**验收**

- 用户仅有 ws-a READ：list 不含 ws-b；get ws-b → 403/404。  
- 用户有 Org WORKSPACES READ：list 含组织内实例（按产品定义）。

---

### R2-3 MANAGEMENT 引擎蕴含（32 §2.4 方案 A）

引擎内定义（示例语义）：

```text
WORKSPACE_MANAGEMENT @ level L
  ⇒ 同 scope 下 EXECUTION / STATE / VARS / RESOURCES / TASK_DATA … 对 required ≤ L 的检查可通过
精细资源不可反向满足 MANAGEMENT
```

路由只声明 **真实资源类型**，逐步删掉超长 `RequireAnyPermission` 重复蕴含。

**验收**：仅持 MANAGEMENT WRITE 的用户，可写变量/触发执行（按蕴含表）；仅持 VARS READ 不能 MANAGEMENT 写。

---

### R2-4 Application 身份闭合

若 R0-3 选 **A**：

1. `iam_application_roles` 或复用 grant 但文档与 TFE 对齐说明。  
2. 业务路由挂载 AgentAuth 的路径清单 + e2e。  
3. 密钥轮换后旧 secret 立即失效测。

若选 **B**：从对外「已支持 Application IAM」表述中删除，避免误用。

---

### R2-5 资源访问规范（防回归架构）

约定（写进 CONTRIBUTING / IAM README）：

```text
1. 路由：声明 resource_type + scope + level
2. Handler：加载资源必须 *InWorkspace / *InOrg / *InSource
3. Service：对外业务方法禁止裸主键 API；内部私有方法可裸 ID
4. 新增全局 /:id 敏感读：Code Review 必拒，除非有二次绑定 + 测试
```

可选实现：`internal/access/` 包统一 `MustLoadWorkspaceTask(db, ws, taskID)`。

---

### R2-6 无显式 deny 的产品说明

32 已定：不做「NONE grant 覆盖上层 ADMIN」。

- UI/运维文档写清：要限制某 WS，用 **更精确 scope 的授权**，不要期望 ban。  
- 若未来合规要 ban：独立 `deny policy` 产品，不与 NONE 混用（另开设计）。

---

## 5. Wave R3 — 测试与文档

### R3-1 测试结构

| 动作 | 说明 |
|------|------|
| 重命名 `permission_coverage_boost_test.go` | 如 `permission_extended_scenarios_test.go`，或拆入 checker/service 测 |
| 补 middleware+handler 串联 1～2 条 | Team Token JWT → RequirePermission → 200/403 |
| 不追求 handlers 覆盖率数字 | 锁安全属性即可 |

### R3-2 文档权威链

| 文档 | 角色 |
|------|------|
| 32 | 产品 SoT |
| 35/36 | 历史核实 / 已关项 |
| **37（本文）** | **下一波修复 Backlog** |
| 34 | 禁止作完成证明 |

更新 `docs/iam/README.md` 指向 37。

### R3-3 发布检查清单（复制用）

```text
[ ] patch_system_admin_iam_roles.sql
[ ] patch_admin_role_iam_policies.sql
[ ] patch_team_tokens_active_unique.sql
[ ] patch_temp_permission_user_id_index.sql
[ ] IAM_SINGLE_TENANT 与环境一致
[ ] go test 核心包全绿
[ ] Application principal 决策已记录（启用/预留）
[ ] 超管账号：业务 Org 可进、IAM 管理可进
[ ] 抽样：跨 org App / 跨 WS task log / 非超管赋 admin Role → 拒
```

---

## 6. 按文件的改动地图（便于排期）

| 区域 | R0 | R1 | R2 |
|------|----|----|-----|
| `migrations/patch_*.sql` + 部署脚本 | 执行/挂钩 | — | — |
| `internal/middleware/iam_permission.go` | 单租户日志/统一函数 | — | MANAGEMENT 配合 |
| `internal/handlers/*` | — | fail-closed 已齐；Direct Grant 收紧 | Role 单轨、list |
| `internal/application/service/*` | — | version InWorkspace、特权 Role | ListAccessible、蕴含 |
| `controllers/*` | — | 双条件、表驱动写路径测 | 去全局敏感路径 |
| `services/*` | — | 双条件 API | list 过滤协作 |
| `frontend` IAM 页 | — | 隐藏/警告 Direct Grant | 仅 Role 运维 |
| `docs/iam/*` | 决策 B/A | — | 终态对齐说明 |

---

## 7. 明确「不要做」的事

1. **不要**恢复业务 API 的 `is_system_admin` bypass。  
2. **不要**把「精确 scope 优先」改回「全层取 max」而不改 32 SoT。  
3. **不要**用提高 coverage 数字替代跨租户属性测试。  
4. **不要**在未跑 R0 SQL 时把「超管 403」当回归 bug 去改回 bypass。  
5. **不要**新增全局裸 `task_id` 敏感读而不绑 workspace。

---

## 8. 建议排期（示例）

| 周 | 交付 |
|----|------|
| 第 1 周 | R0 全做完 + R1-1/R1-2/R1-6 核心测试 |
| 第 2 周 | R1-3～R1-5 + Direct Grant deprecated |
| 第 3–4 周 | R2-1 半程（API 收口）+ R2-2 list 过滤 MVP |
| 后续 | R2-3 MANAGEMENT 引擎化、R2-4 Application 若选 A、R2-5 规范固化 |

---

## 9. 成功标准（Definition of Done）

**R0+R1 完成即视为「修复建议最小闭环」：**

- [ ] 新环境按 runbook 安装后，超管与 Org admin 可完成日常运维（无静默 403）  
- [ ] 已知 P0/P1 代码洞无复开；R1 双条件与写路径测在 CI 绿  
- [ ] 多租户缺 org_id → 400；单租户行为可配置且有日志  
- [ ] Application 启用/预留决策已文档化  

**R2 完成即视为「对齐 32 终态」：**

- [ ] 业务授权只走 Role  
- [ ] list 与 detail 权限语义一致  
- [ ] MANAGEMENT 蕴含在引擎单测锁定  
- [ ] 无新增全局裸 ID 敏感 API  

---

## 10. 与前序文档的关系

| 文档 | 关系 |
|------|------|
| 32 | 目标语义；本文不修改 D1–D5 |
| 35 | Wave A 核实基线；多项已在后续关闭 |
| 36 | 残留清单；其中 P0/C 波代码项多数已关，本文承接 **仍开放项 + 终态债** |
| 34 | 不作完成证据 |

**冲突处理**：实现是否已修，以代码 + 36「本轮核实」为准；**下一步做什么，以本文 37 为准**。
