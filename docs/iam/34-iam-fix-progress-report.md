# IAM 修复进度与测试覆盖率报告

> **日期**：2026-07-16（进度快照）  
> **状态**：⚠️ **不得作为 release 完成证据** — 安全核实与 open items 以 [`35-iam-security-review-report.md`](./35-iam-security-review-report.md) 为准  
> **关联**：`32` 产品 SoT、`33` CR、`35` 代码核实  
> **数据补丁（成对执行）**：`patch_system_admin_iam_roles.sql` **+** `patch_admin_role_iam_policies.sql`  
> **可选**：`patch_team_tokens_active_unique.sql`（活跃 token_name 部分唯一索引）

---

## 1. 总体进度

| 维度 | 状态 |
|------|------|
| 产品决策 D1–D5 落地 | **已定稿并编码** |
| Critical 安全洞（R-S01/02） | **已修** |
| High 目标态（R-S03～S09 主路径） | **已修 / 部分扫尾** |
| system_admin 业务旁路取消 | **已修** |
| TEAM principal 真鉴权 | **已修** |
| 防超管锁死 SQL 补丁 | **已提供（待执行）** |
| 测试 | **核心路径覆盖显著提升（见 §4 最新实测）** |

**一句话**：鉴权目标语义与主路径安全修复已合入代码；上线前必须跑 **patch SQL** 给现有 `is_system_admin` 用户补 `admin@ORGANIZATION` Role，否则业务面会 403。

---

## 2. 修复进度明细

### 2.1 产品决策

| ID | 决策 | 实现状态 |
|----|------|----------|
| D1 | NONE = 不授权 → 拒绝 | ✅ `calculateEffectiveLevel` + deny reason |
| D2 | 写权限按作用域（WS / Org） | ✅ 精确作用域优先；Org grant 可管全实例 |
| D3 | 项目负责人需显式授权 | ✅ 模型支持；预置 Role 靠运营赋值 |
| D4 | Team Token 自动化 ~24h | ✅ 认证 + TEAM 求值 + 24h + 按名吊销 |
| D5 | Role 主模型 | 🔶 运行时 Role 已一等；Direct Grant 仍存在（待收敛运维入口） |

### 2.2 高危清单 R-S01～S09

| ID | 级别 | 项 | 状态 |
|----|------|-----|------|
| R-S01 | Critical | Pool 撤销后仍有效 | ✅ `status=active` |
| R-S02 | Critical | READ 读完整 State | ✅ list 不返回 Content；compare/retrieve/download 均 SENSITIVE（二次加固） |
| R-S03 | High | 跨 WS IDOR | 🔶 主路径已绑；**以 `35` 为准** — vsnap/trigger/版本依赖等已在 Wave A 补齐中，发布前再对照 35 关闭清单 |
| R-S04 | High | org_id 静默落到 1 | ✅ 非法 400；缺失单租户默认 1 |
| R-S05 | High | system_admin 业务全旁路 | ✅ **业务 IAM 取消 bypass**；平台 `RequireSystemAdmin` |
| R-S06 | High | Team Token 不可用 / 伪主体 | ✅ JWT type 优先 + **TEAM principal 求值** |
| R-S07 | High | expires 丢弃 / 格式失败 | ✅ parseFlexibleTime + Grant/Role |
| R-S08 | High | Application secret / Role 路径 | ✅ Secret 哈希；前端禁止 APPLICATION 绑 Role（改用直接授权）；checker 支持 APPLICATION principal |
| R-S09 | Medium | list 泄露 / summary / SSO query | ✅ list 最小化；manifest-summary IAM；SSO fragment |

### 2.3 Code Review 开项

| Issue | 状态 |
|-------|------|
| system_admin 业务旁路 | ✅ fixed |
| TEAM 真主体 | ✅ fixed |
| RequireAny org_id 对齐 | ✅ fixed |
| 任务 IDOR 全文件 | ✅ CancelPrevious 等剩余洞已修 |
| token_name 唯一 | ✅ 活跃 name 校验 |
| NONE 注释 | ✅ fixed |

---

## 3. Patch SQL（必须上线前执行）

**文件**：[`backend/migrations/patch_system_admin_iam_roles.sql`](../../backend/migrations/patch_system_admin_iam_roles.sql)

**作用**：为每个活跃 `is_system_admin` 用户，在每个活跃组织上幂等绑定系统 Role **`admin` @ ORGANIZATION**（与 setup 初始化一致）。

```bash
# 示例（按环境改连接）
psql -U postgres -d iac_platform -f backend/migrations/patch_system_admin_iam_roles.sql
```

**执行后验收**：

```sql
SELECT u.user_id, u.username, COUNT(ur.id) AS admin_org_bindings
FROM users u
LEFT JOIN iam_user_roles ur ON ur.user_id = u.user_id
  AND ur.scope_type = 'ORGANIZATION'
  AND ur.role_id = (SELECT id FROM iam_roles WHERE name = 'admin' AND is_system LIMIT 1)
WHERE u.is_system_admin = true
GROUP BY u.user_id, u.username;
-- 期望：每个活跃超管 admin_org_bindings >= 组织数
```

**回滚**（慎用）：

```sql
DELETE FROM iam_user_roles
WHERE reason LIKE 'patch_system_admin_iam_roles:%';
```

---

## 4. 测试覆盖率（当前）

测量命令：

```bash
cd backend
go test ./internal/domain/valueobject/ ./internal/application/service/ \
  ./internal/middleware/ ./internal/handlers/ \
  -coverprofile=/tmp/iam.out -count=1
go tool cover -func=/tmp/iam.out
```

### 4.1 包级（最新实测 2026-07-17 收官）

| 包 | 语句覆盖率 | 说明 |
|----|-----------|------|
| `internal/domain/valueobject` | **88.8%** | 核心 VO 已表驱动 |
| `internal/application/service` | **50.0%** ↑↑ | Pool/App/Agent/Team Token 服务主路径 |
| `internal/middleware` | **58.7%** ↑↑ | AgentAuth 100%、Pool/JWT/IAM |
| `internal/handlers` | **6.6%** ↑ | Role 全路径 + TeamToken + Permission |

### 4.2 关键函数（最新实测）

| 函数 | 覆盖 |
|------|------|
| `CheckPermission` | **~74%** |
| `CheckPermissionWithTemporary` | **~85%** |
| `CheckBatchPermissions` | **~86%** |
| `resolvePrincipal` | **~78%** |
| `calculateEffectiveLevel` | **~93%** |
| `GrantPermission` (service / workspace) | **~78% / ~88%** |
| `Revoke` / `Modify` / `Preset` / List* | **~80–100%** |
| `RequirePermission` | **~82%** |
| `JWTAuth` | **~75%** |
| `AgentAuthMiddleware` | **100%** ↑（原 0%） |
| `PoolTokenAuthWithTaskCheck` | **~77%** |
| `PoolTokenAuthWithWorkspaceCheck` | **~84%** ↑ |
| `RequireWorkspacePermission` | **~79%** |
| `TeamToken.ValidateToken` | **~88%** |
| `PoolToken.Validate/Generate/Revoke` | **~78–93%** ↑（原 0%） |
| `Application.ValidateApplication` | **100%** |
| `AgentService.ValidateApplication` | **~93%** ↑ |
| handler Role 主路径（含 Team/Policy） | **~53–88%** ↑（原 Team/Policy 0%） |
| handler `CreateTeamToken` / `RevokeTeamToken` | **~86% / ~85%** |
| `hashAppSecret` / `verifyAppSecret` | **100%** |

### 4.2b 关键文件函数平均

| 文件 | 均函数覆盖 |
|------|-----------|
| `permission_checker.go` | **~83%** |
| `permission_service.go` | **~81%** |
| `team_token_service.go` | **~86%** |
| `pool_token_service.go` | **~83%** ↑（原 0%） |
| `application_service.go` | **~94%** |
| `agent_service` 鉴权相关 | **~93–100%** ↑ |
| `iam_permission.go` | **~80%** ↑ |
| `pool_token_auth.go` | **~75%** ↑ |
| `agent_auth.go` | **~100%** ↑ |
| `permission_handler.go` | **~61%** |
| `role_handler.go` | **主路径 ~53–88%**（含 Team/Policy） |
| `team_token_handler.go` | **~84%** |

### 4.3 测试文件清单

```
internal/domain/valueobject/valueobject_test.go
internal/application/service/permission_checker_test.go
internal/application/service/permission_checker_integration_test.go
internal/application/service/permission_coverage_boost_test.go
internal/application/service/permission_service_test.go
internal/application/service/resolve_principal_test.go
internal/application/service/team_token_expiry_test.go
internal/application/service/team_token_more_test.go
internal/application/service/team_token_validate_test.go
internal/application/service/application_secret_test.go
internal/application/service/application_service_test.go
internal/application/service/pool_token_service_test.go
internal/application/service/agent_service_test.go
internal/middleware/iam_permission_test.go
internal/middleware/iam_more_test.go
internal/middleware/jwt_team_token_test.go
internal/middleware/jwt_login_user_token_test.go
internal/middleware/pool_token_auth_test.go
internal/middleware/pool_token_task_test.go
internal/middleware/pool_basic_middleware_test.go
internal/middleware/agent_auth_test.go
internal/handlers/permission_handler_http_test.go
internal/handlers/role_handler_test.go
internal/handlers/role_handler_more_test.go
internal/handlers/team_token_handler_test.go
internal/handlers/setup_handler_test.go
internal/handlers/parse_flexible_time_test.go
services/workspace_variable_service_test.go
services/workspace_list_minimize_test.go
services/state_list_content_test.go
controllers/workspace_task_binding_test.go
controllers/resource_binding_test.go   # 含 editing path 绑定
```

### 4.4 安全加固（本轮一并合入）

| 项 | 状态 |
|----|------|
| 编辑会话 editing/* / drift/* 跨 WS IDOR | ✅ `parseResourceIDInPathWorkspace` 绑定 path |
| Agent `ValidateApplication` 列选择 | ✅ 避开 JSON map 扫描问题 |
| Application `CallbackURLs` serializer | ✅ `gorm:"serializer:json"` |

### 4.5 Role 防提权（已实现 2026-07-17）

实现：`internal/application/service/role_anti_escalation.go`  
接入：`AssignRole` / `AssignTeamRole` / `AddRolePolicy` / `CloneRole`

| 规则 | 行为 |
|------|------|
| Policy 闭包 ⊆ actor | 对 Role 每条 policy 调 `CheckPermission(actor, resource, 赋值 scope, level)` |
| 系统 Role `admin` | 仅 `is_system_admin` 可分配/克隆 |
| 空策略 Role | 允许（不扩大权限） |
| 失败 HTTP | **403** `Privilege escalation denied` |

测试：`role_anti_escalation_test.go` + `role_anti_escalation_handler_test.go`

### 4.6 与「全面」的差距

| 缺口 | 优先级 |
|------|--------|
| Application AgentAuth → IAM principal 业务接线 | P1 |
| Setup `InitAdmin` 集成测（依赖 PG advisory lock） | P2 |
| Team Token `ValidateToken` 与 `token_id` 列历史债 | P2 |
| org owners 团队亦可分 admin（当前仅 is_system_admin） | P2 产品 |

---

## 5. 上线检查清单

1. [ ] 合并代码（IAM 中间件 / checker / 路由权限）
2. [ ] **执行** `patch_system_admin_iam_roles.sql`
3. [ ] 用超管账号验证：列表 workspace、进入 IAM、非平台业务 API 均 200
4. [ ] 用无 Role 普通用户验证：403 + `No permission`
5. [ ] Team Token：创建（24h）、调用 API（仅 team 权限）、吊销
6. [ ] State download：无 SENSITIVE 时 403
7. [ ] 撤销 Pool→WS 后 agent 访问失败

---

## 6. 进度总结表

| 阶段 | 内容 | 完成度 |
|------|------|--------|
| Phase A 裁决语义 | 作用域优先 / NONE / deny reason | **100%** |
| Phase B 身份与旁路 | TEAM principal / 去 system_admin 业务旁路 | **100%** |
| Phase C 高危补丁 | Pool / State / IDOR / expires / secret / list / SSO / App 前端 | **~95%** |
| Phase D 数据补丁 | system_admin → admin Role SQL | **脚本就绪，待执行** |
| Phase E Role 主路径收敛 | 下线 Direct Grant 运维入口 | **未做**（可后续） |
| Phase F 测试 | 核心单测 + 集成 stub | **核心函数 56–100%；包级 24–89%** |

### 仍未完成（可接受延期）

| 项 | 说明 |
|----|------|
| Direct Grant 运维入口下线 | D5 产品收敛；不挡安全上线 |
| role_handler / permission_handler HTTP 测 | 逻辑已在 service 测；HTTP 壳层可选 |
| Pool Token 中间件整链 | 仅 active 过滤有测 |
| Application 专用 Role 表 | 产品未要求；现用 org grant + 禁止误写 user_roles |
| 临时权限 Webhook | 另案，不在主模型 |

---

### 二次审查（阻断项）跟进 — 2026-07-16

| 审查项 | 处置 |
|--------|------|
| State list/compare 敏感绕过 | ✅ list 去 Content；compare 提敏 |
| 资源/快照/任务日志 IDOR | ✅ 主路径绑定；编辑会话等后续 |
| TeamDetail build（token.id / customDays） | ✅ 已修 |
| Team Token 吊销/过期/配额 | ✅ revoke 仅 active；JWT 查 expires_at；创建前清理过期 |
| Application 认证链 | 🔶 UI 限制组织级；AgentAuth 接 IAM 仍待做 |
| Setup admin Role 可选失败 | ✅ setup 失败即回滚 |
| Role 反提权 | ❌ 未做（P1 后续） |
| SSO MFA query | ✅ 改 fragment + 前端读 hash |
| expires 时区/过去时间 | ✅ UTC 解析 + 拒绝过去 |

---

*本报告随 `patch_system_admin_iam_roles.sql` 一并落库。Docker 库已执行过 patch（admin 已有绑定）。*
