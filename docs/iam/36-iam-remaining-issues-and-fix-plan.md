# IAM 二次审查：残留问题与修复建议

> **日期**：2026-07-17  
> **审查对象**：工作区未提交 IAM 整改（相对 `main` / 前序 32–35 文档）  
> **方法**：源码通读 + 路由/中间件/服务绑定核对 + 定向 `go test`  
> **关联**：`32` 产品 SoT · `33` CR · `34` 进度宣称 · `35` 安全核实  
> **结论一句话**：**Wave A/B 主路径已落地；Wave C（P0/P1 代码项 C1–C6）已于代码侧关闭，核心包可绿。运维四 SQL 与 Application principal 生产挂载仍需环境验收。**

---

## 0. 相对上一轮（35 / 首轮 review）的变化

| 项 | 上一轮 | 本轮核实 |
|----|--------|----------|
| Application org 绑定（Create/List/Get/Update/Delete/Secret） | P0 开 | **已修**（handler + `*InOrg`） |
| vsnap 删除 / 任务创建 vsnap 绑定 | P0 开 | **已修** |
| Run Trigger Update/Delete 绑 source | P0 开 | **已修** + 有 HTTP 级测试 |
| Resource 版本/回滚/依赖绑定 | P0/P1 开 | **已修**（helper 接入主端点） |
| 系统 Role 策略只读 + Assign fail-closed | 部分 | **Assign/Add/Team/Remove/Clone 均 fail-closed**（Wave C1） |
| 临时权限假邮箱 | P1 | **已改为真实邮箱 lookup + user_id 双键**；factory 已注入 |
| Team Token 竞态 / NULL expires | P1 | **事务 + middleware 拒 NULL**；**SQL 部分唯一索引已提供（待执行）** |
| ValidateToken 假列 | P1 | **已修**（走 `token_id_hash`） |
| Application principal 上下文 | P1 | **AgentAuth 已写 APPLICATION principal**；**业务路由是否挂载仍需产品确认** |
| `go test ./services` | 红 | **本轮全绿** |
| `go test ./controllers` | 曾 panic | **本轮全绿** |

### 本轮实测门禁

```text
ok  ./internal/application/service
ok  ./internal/handlers
ok  ./internal/middleware
ok  ./controllers
ok  ./services
```

> 注意：全绿 ≠ 攻击面闭合；handlers 覆盖率仍约 **7–8%**，安全属性依赖点状用例。

---

## 1. 总体评级（本轮）

| 维度 | 评级 | 说明 |
|------|------|------|
| 产品决策 D1–D5 主语义 | **中上** | NONE 拒绝、精确作用域、业务取消 super-admin bypass、TEAM 求值已编码 |
| Critical（原 R-S01/02） | **基本关闭** | Pool active、State list 无 content |
| IDOR / 子资源绑定 | **中上** | Workspace 子资源主路径多数已绑；**全局 task 日志已绑 workspace READ（C2）** |
| Role / 管理面防提权 | **中上** | 主体闭合；**Remove/Clone fail-closed（C1）** |
| 自动化测试 | **中** | 属性设计好；端点全集与 e2e 仍薄 |
| 上线就绪（多组织） | **否** | 先完成 §3 P0/P1 + 双 SQL + team_tokens 索引 |
| 上线就绪（单组织信任内网） | **有条件** | 执行全部 patch SQL；修 task 日志与 fail-open |

---

## 2. 已确认有效（勿回退）

继续保留并加回归锁：

1. **Application**：`auth_org_id` + `Get/Update/Delete/Regenerate*InOrg`；跨 org → 404/403  
2. **RoleAntiEscalationService**：闭包 ⊆ actor、空策略禁非超管、系统 Role 策略只读、scope∈auth org、checker nil fail-closed  
3. **Assign / AddRolePolicy / AssignTeamRole**：handler `guard == nil` → 500  
4. **Resource / Task / Variable / vsnap**：path workspace 绑定 helper  
5. **Run Trigger Update/Delete*InSource**  
6. **State list 不加载 content**  
7. **业务 IAM 不 bypass is_system_admin**  
8. **Team Token**：24h 上限、NULL expires 拒绝、Generate 事务、Validate 真 schema  
9. **临时权限**：`UserEmailLookup` + email/user_id 双键（factory 注入）  
10. **AgentAuth**：写入 `principal_type=APPLICATION`

---

## 3. 残留问题清单

### P0 — 建议发布前关闭

#### P0-1 全局任务日志 / 流：水平越权 — **✅ 已关（Wave C2）**

| 字段 | 内容 |
|------|------|
| **位置** | `router_task.go`、`task_log_controller.go`、`terraform_output_controller.go` |
| **修复** | JWT + 加载 task 后 `RequireWorkspacePermission(ws, READ)`；stream 在 Upgrade 前校验；缺 IAM → 500。 |
| **测试** | `TestTaskLogController_CrossWorkspaceDenied` |

#### P0-2 Role：RemovePolicy / Clone 防提权 fail-open — **✅ 已关（Wave C1）**

| 字段 | 内容 |
|------|------|
| **位置** | `role_handler.go` `RemoveRolePolicy`、`role_clone.go` |
| **修复** | `guard == nil` → 500；Clone 用 `authOrgFromContext`（多租户缺 org → 400）。 |
| **测试** | `TestRemoveRolePolicy_FailClosedWithoutGuard`、`TestCloneRole_FailClosedWithoutGuard` |

---

### P1 — 强烈建议同波次修

#### P1-1 Run Trigger Create：目标 Workspace 无权限校验 — **✅ 已关（Wave C3）**

Create/CreateInbound 对 target/source 调 `RequireWorkspacePermission(..., WRITE)`；`TestCreateRunTrigger_RequiresTargetWrite`。

#### P1-2 Application Service 无 org 兼容入口 — **✅ 已关（Wave C4）**

无 org 入口返回 `ErrApplicationOrgForbidden`；请用 `*InOrg`。

#### P1-3 默认 org_id = 1 — **✅ 已关（Wave C5）**

`resolveOrgScopeID` / `authOrgFromContext`：仅 `IAM_SINGLE_TENANT=1|true|yes` 默认 1，否则缺 org → error/0。

#### P1-4 无绑定的旧 Service API — **✅ 已关（Wave C4）**

`Update/DeleteRunTrigger` 无 source 返回 disabled error；Application 同。

#### P1-5 全局 `/tasks/:id/*` 与 Workspace 路径双轨

Workspace 下任务 API 已绑定；全局 task 日志路径仍弱（P0-1）。同类模式若再出现（按裸 id 读敏感物）应统一禁止。

#### P1-6 ai_summary 等确认路径仍 `First(&task, id)`

| 位置 | 说明 |
|------|------|
| `ai_summary_controller.go` 等 | 部分先校验 summary.workspace，再裸加载 task；应统一 `task.workspace_id == path workspace` |

#### P1-7 Team Token DB 约束待执行 + 配额无唯一

| 字段 | 内容 |
|------|------|
| **已有** | 应用层事务；`patch_team_tokens_active_unique.sql`（活跃 name 唯一） |
| **缺口** | 补丁**未默认跑迁移流水线**；活跃数量上限 2 **无 DB 约束**（仅事务 Count，极端并发仍可能竞态） |

#### P1-8 Application principal 生产可达性未证明

AgentAuth 已写 principal；主 Agent 任务链仍是 Pool Token。需产品决策：App 密钥是否进入业务 IAM 路由。若否，文档降级「仅密钥校验/预留」；若是，挂载路由 + e2e。

#### P1-9 特权系统 Role 名单写死 `admin`

其它 `is_system` Role 不享受「仅超管可分配」；策略只读已覆盖 `IsSystem`，分配特权名单建议可配置。

---

### P2 — 质量 / 可维护性 / 测试

| ID | 问题 | 建议 |
|----|------|------|
| P2-1 | Resource/Task 绑定测试偏 **helper**，非全端点 HTTP | 写路径表驱动跨 WS 404 |
| P2-2 | Application 缺 Update/Delete/**RegenerateSecret** 跨 org HTTP 测 | 补 3 用例 |
| P2-3 | 缺「任务创建跨 WS vsnap → 400」controller 测 | 补 1 用例 |
| P2-4 | handlers 覆盖 ~7.5% | 不追求数字；锁安全属性即可 |
| P2-5 | `QueryTeamRoles` 空 UserID = 任意 team 兼容 | 删兼容分支，强制显式 team_id |
| P2-6 | `permission_coverage_boost_test` 命名/目的偏灌水 | 改名或并入真实场景测 |
| P2-7 | `GetVariableVersion` 归属校验后仍按全局 variable_id 取版本 | 服务层 `GetVariableVersionInWorkspace` 双条件 |
| P2-8 | CloneRole 读 `org_id` 而非 `auth_org_id` | 统一 `authOrgFromContext` |
| P2-9 | `CreateApplication` body `org_id` 仍 required | 改为可选，强制 auth org |
| P2-10 | 文档 `34` 仍可能被误读为完成证明 | README 明确以 35/36 为准 |

---

### 上线阻断（运维，非代码洞）

| SQL | 作用 |
|-----|------|
| `migrations/patch_system_admin_iam_roles.sql` | 超管补 `admin@ORGANIZATION`（取消 bypass 后必须） |
| `migrations/patch_admin_role_iam_policies.sql` | admin 角色补齐 `IAM_*` 策略 |
| `migrations/patch_team_tokens_active_unique.sql` | 活跃 token_name 唯一 |
| `migrations/patch_temp_permission_user_id_index.sql` | 临时权限 user_id 索引（性能/正确性辅助） |

**必须成对执行**前两个；后两个强烈建议同批。

---

## 4. 修复建议（可执行）

### 4.1 P0-1：全局任务日志绑定 Workspace 权限

**推荐方案（优先）**：废弃或重定向全局路径，统一走：

```http
GET /api/v1/workspaces/{id}/tasks/{task_id}/logs
```

（该路径已有 workspace IAM。）

**若必须保留全局路径**：

1. 加载 task → 取 `workspace_id`  
2. 调用 `RequireWorkspacePermission(c, task.WorkspaceID, "READ")` 或等价  
   （`TASK_DATA_ACCESS` / `WORKSPACE_MANAGEMENT` / 组织 WORKSPACES READ，与列表任务策略对齐）  
3. 禁止仅 org 级 `TASK_LOGS` + 裸 id  
4. 删除 controller 内 TODO；404 统一「task not found」防探测  

**测试**：

- 用户有 org1 TASK_LOGS、无 WS-B 权限：读 B 的 task_id → **403/404**  
- 用户有 WS-A 读权限：读 A 的 task → 200  

**涉及文件**：`router_task.go`、`task_log_controller.go`、Terraform output stream 控制器。

---

### 4.2 P0-2：防提权 fail-closed 统一

```go
// 所有 mutate role / policy / clone 入口统一：
if h.guard == nil {
    c.JSON(500, gin.H{"message": "Anti-escalation not configured"})
    return
}
```

- `RemoveRolePolicy`：与 Add 相同，先 `EnsureCanMutateSystemRolePolicies`（已有），改为 fail-closed  
- `CloneRole`：`EnsureCanCloneRole` + `authOrgFromContext(c)`（勿默认 1 + 手读 org_id）  

**测试**：`NewRoleHandler(db, nil)` 调 Remove/Clone → 500；非超管删系统 Role policy → 403。

---

### 4.3 P1-1：CreateRunTrigger 校验 target 权限

在 Create 中（handler 或 service）：

1. target workspace 存在（已有）  
2. 调用 IAM：`RequireWorkspacePermission` 或 checker  
   `WORKSPACE_MANAGEMENT` / `WORKSPACE_EXECUTION` @ target **WRITE**（产品定级）  
3. 可选：同 org 约束（经 project relation 溯源 org）  

**测试**：仅 source 有写、target 无写 → 403；两边都有 → 201。

---

### 4.4 P1-2 / P1-4：删除或收紧无绑定 API

| 动作 | 说明 |
|------|------|
| `Update/Delete/RegenerateSecret` | 要求 `orgID != 0`，否则 `ErrApplicationOrgForbidden` |
| 删除或 unexport 无 org 重载 | 全仓 grep 调用点改为 `*InOrg` |
| `UpdateRunTrigger`/`DeleteRunTrigger` | 改为调用 `*InSource` 或删除；全仓只保留绑定版 |

---

### 4.5 P1-3：org 缺省策略

```text
if multi_tenant && org_id missing → 400
if single_tenant → default 1 (显式配置 SINGLE_TENANT=true)
```

`authOrgFromContext` 与 middleware 共用同一策略，避免一个默认 1、一个 400。

---

### 4.6 P1-6 / 变量版本：统一归属 helper

- AI summary / 任何 `First(&task, id)`：改为 `Where("id = ? AND workspace_id = ?", ...)`  
- `GetVariableVersion` → `GetVariableVersionInWorkspace(ws, variableID, version)`  

---

### 4.7 测试补齐（最小集）

| 用例 | 期望 |
|------|------|
| App RegenerateSecret 跨 org | 404 |
| App Delete/Update 跨 org | 404 |
| RemoveRolePolicy guard=nil | 500 |
| CloneRole 超权 / guard=nil | 403 / 500 |
| CreateRunTrigger 无 target 权限 | 403 |
| 全局 task logs 跨 WS | 403/404 |
| Create task 跨 WS vsnap | 400 |
| Resource Rollback HTTP 跨 WS | 404 |
| Team 非成员不继承（stub 过滤 teamIDs） | deny |

**CI 建议门禁**（保持）：

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

可选：`rg` 门禁禁止新增 `First(&task,` 于 controllers（白名单除外）。

---

### 4.8 运维清单

1. 备份 DB  
2. 执行四个 patch SQL（前两个阻断；后两个强烈建议）  
3. 验收：每个 `is_system_admin` 在每 org 有 `admin` 绑定；`admin` 角色含 `IAM_*`  
4. 清理 team_tokens 活跃重名后再建唯一索引  
5. 冒烟：超管 IAM 管理面 200；普通用户跨 WS 日志 403  

---

## 5. 建议实施波次（Wave C）— 代码侧状态（2026-07-17）

| 顺序 | 项 | 预估 | 优先级 | 状态 |
|------|-----|------|--------|------|
| C1 | RemovePolicy/Clone fail-closed + Clone 用 auth_org | 0.5d | P0 | ✅ |
| C2 | 全局 task logs/stream 绑 workspace READ（JWT + controller） | 1d | P0 | ✅ |
| C3 | CreateRunTrigger/CreateInbound 校验 target/source WRITE | 0.5d | P1 | ✅ |
| C4 | Application 无 org / RunTrigger 无 source API 禁用 | 0.5d | P1 | ✅ |
| C5 | 多租户强制 org_id（`IAM_SINGLE_TENANT` 才默认 1） | 0.5d | P1 | ✅ |
| C6 | 最小安全回归测（logs/Remove/Clone/CreateTrigger/App 跨 org） | 1d | P1 | ✅ |
| C7 | SQL 补丁执行说明写入 release checklist | 0.5d | 运维 | ⚠ 本机 Docker 已执行；其它环境待 checklist |
| C8 | 文档：标记 34 为历史；README 以 35/36 为核实 SoT | 0.5d | 文档 | 部分 |

**代码侧 P0/P1（C1–C6）已关闭。** 多组织生产仍需 C7 环境验收 + 产品确认 Application principal 挂载。

---

## 6. 验收标准（Definition of Done）

- [x] C1–C2 合并；对应单测绿  
- [x] C3–C6 合并；§4.7 主表用例已补  
- [ ] 四 SQL 在**所有**目标环境执行并有验收查询结果  
- [x] 本地 CI 等价门禁：`go test` handlers/middleware/controllers/services/application/service  
- [x] 本文件状态表：P0 代码项 ✅  
- [ ] 禁止再用 `34` 单独作为 release 证据（流程）  

---

## 7. 不在本建议范围（明确排除）

- 前端完整威胁建模与 XSS/CSRF  
- Pool Token / Agent 协议全量渗透  
- 性能与缓存正确性专项  
- 将 Direct Grant 收敛为纯 Role 模型（产品 D5 长期项）  

---

## 8. 给决策者的摘要

| 问题 | 回答 |
|------|------|
| 主路径 IAM 整改是否有效？ | **是**，较 35 开篇已明显推进，且本轮核心测试全绿 |
| 能否多组织公网生产？ | **否**，至少先关 P0-1/P0-2 + SQL |
| 能否单组织内网灰度？ | **有条件可以**，仍建议同批修全局 task 日志与 fail-open |
| 下一步谁来做？ | Wave C 表；优先 C1+C2（小 diff、高安全收益） |

---

*文档维护：本文件为「残留问题 + 修复建议」SoT；实现关闭后请回写状态并考虑产出 `37-...` 复审。*
