# IAM 本次优化 Code Review

> **范围**：2026-07-16 工作区未提交改动（IAM 高危修复 + 裁决语义 + 文档）  
> **审查方式**：对照 diff + 关键路径通读  
> **关联**：`32-iam-remediation-report.md` §12

---

## Summary

本次改动方向正确，集中打在 Critical/High 安全洞（Pool 撤销、State 敏感下载、跨 WS IDOR、Team Token 认证、expires 链路、Secret 哈希、org 作用域、列表最小化、SSO fragment）。裁决算法从「跨层 max」改为「精确作用域优先」与产品决策一致。

整体 **可合入方向**，但有几处 **残留风险 / 行为回归点** 需在测试与后续迭代中盯住：`system_admin` 业务旁路仍在、Team Token 鉴权仍是合成 `user_id` 而非真正 TEAM principal、`RequireAnyPermission` 与 `RequirePermission` 对非法 org 行为不完全一致、部分 handler 的 IDOR 只修了变量与任务评论/取消。

---

## Issues

### Issue 1 -- Severity: bug（行为残留）
- File: `backend/internal/middleware/iam_permission.go`（system_admin 分支）
- Description: ~~superadmin 业务旁路~~
- Status: **fixed** — 业务 IAM 全部取消 bypass；平台 API 用 `RequireSystemAdmin()`。

### Issue 2 -- Severity: bug（功能不完整）
- File: `permission_checker.go` + middleware principal
- Description: ~~Team Token 伪 user_id 求值~~
- Status: **fixed** — `PrincipalType=TEAM` 仅 team grant + team roles；中间件传入 `principal_type/id`。

### Issue 3 -- Severity: suggestion
- File: `backend/internal/middleware/iam_permission.go`（RequireAnyPermission org 分支）
- Description: ~~非法 org_id 行为不一致~~
- Status: **fixed** — 与 RequirePermission 一致返回 400。

### Issue 4 -- Severity: suggestion
- File: `backend/controllers/workspace_task_controller.go`（loadTaskInPathWorkspace）
- Description: 仅 CancelTask / CreateComment 接入；同文件 ConfirmApply、GetTaskLogs、部分 cancel-previous 等若仍 `First(&task, taskID)` 则 IDOR 未清干净。
- Suggestion: 全文件扫 `First(&task` / `First(&variable` 类模式，统一走 bind helper。
- Status: open（部分修复）

### Issue 5 -- Severity: nit
- File: `backend/internal/domain/valueobject/permission_level.go:9`
- Description: 注释仍写「显式拒绝（最高优先级）」，与 D1「NONE=不授权」及实现不一致。
- Suggestion: 改为「无权限 / 未授权」。
- Status: open

### Issue 6 -- Severity: suggestion
- File: `backend/internal/application/service/team_token_service.go`（RevokeTokenByName）
- Description: 按 `token_name` 吊销；若允许同 team 重名，First 命中任意一条。列表也不返回稳定 revoke id。
- Suggestion: DB 唯一约束 `(team_id, token_name)` 或在 active 范围内唯一。
- Status: open

### Issue 7 -- Severity: nit
- File: `backend/internal/application/service/permission_checker.go`（`_ = policyScope`）
- Description: policy scope 解析后丢弃，仅用赋值 scope 做合并优先级——正确，但可读性差。
- Suggestion: 注释说明「policy.ScopeType 仅作策略适用层过滤时可保留；合并用 assignment scope」。
- Status: open

### Issue 8 -- Severity: suggestion（回归风险）
- File: `backend/internal/router/router_workspace.go`（state download）
- Description: 将 download 提到 SENSITIVE 正确，但前端若仅有 STATE READ 会突然 403。
- Suggestion: changelog/UI 提示；或 metadata 接口保持 READ、content 才 SENSITIVE（当前 metadata 路由仍 READ，合理）。
- Status: open（产品沟通）

### Issue 9 -- Severity: suggestion
- File: `backend/services/workspace_service.go`（list 最小化）
- Description: list 清空 `state_config` 等为 nil；若前端列表依赖这些字段会空白。
- Suggestion: 确认前端列表不依赖；详情页另拉完整配置。
- Status: open

---

## 做得好的地方

1. Pool 鉴权补 `status=active` —— 与 Revoke 语义一致，Critical 修复准确。  
2. State content 路径统一抬到 SENSITIVE/ADMIN —— 堵住 READ 下完整 state 下载。  
3. 裁决算法改精确作用域优先 + deny reason 修正 —— 与 32 号决策对齐。  
4. Role 收集只上卷一次 + grant ScopeType=赋值作用域 —— 避免重复与错误优先级。  
5. JWT 先看 type 再要求 user_id —— 修复 Team Token 认证死锁。  
6. expires 多格式解析、App Secret 哈希兼容明文 —— 实用且可迁移。  
7. SSO token 放 fragment —— 降低 query 泄露面。  

---

## 测试补齐（已执行）

| 层级 | 文件 | 覆盖要点 |
|------|------|----------|
| valueobject | `valueobject_test.go` | Level/Scope/Resource/Principal 解析、JSON、优先级 |
| checker 纯函数 | `permission_checker_test.go` | 作用域优先 / 过期 / deny reason |
| checker 集成 | `permission_checker_integration_test.go` | stub repo：无权限/Org WRITE/WS 压 Org/Team/过期/Role/校验 |
| middleware IAM | `iam_permission_test.go` | 401/admin bypass/allow/deny/org_id/WS path/OR/RequireWorkspace |
| middleware JWT | `jwt_team_token_test.go` | team_token 无 user_id 通过；login 无 user_id 拒绝 |
| pool | `pool_token_auth_test.go` | active vs revoked |
| handlers | `parse_flexible_time_test.go` | RFC3339 + datetime-local |
| team token | `team_token_expiry_test.go` | 24h 上限、按 name 吊销 |
| app secret | `application_secret_test.go` | hash + legacy 明文 |
| 变量 IDOR | `workspace_variable_service_test.go` | 跨 WS 拒绝 |
| list 最小化 | `workspace_list_minimize_test.go` | 敏感字段 nil |
| task 绑定 | `workspace_task_binding_test.go` | loadTaskInPathWorkspace 跨 WS |

### 仍未覆盖（诚实）

- `permission_service` Grant/Revoke 写库路径  
- Role handler HTTP  
- Pool Token 中间件整链（authenticate + task check）  
- 前端 SSO/TeamDetail  

---

*Review 完成；nit Issue 5（NONE 注释）已在 `permission_level.go` 修正。*
