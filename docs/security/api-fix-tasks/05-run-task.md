# 05 — Run Task（回调 + 管理）🔴 P0

> 源文件: `router_run_task.go`
> API 数量: 10

## 全部 API 列表

### Run Task 回调（公开路由，无认证）

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | PATCH | /api/v1/run-task-results/:result_id/callback | 无 | 无 | ❌ 可伪造回调结果 |
| 2 | POST | /api/v1/run-task-results/:result_id/callback | 无 | 无 | ❌ 同上 |
| 3 | GET | /api/v1/run-task-results/:result_id | 无 | 无 | ❌ 可枚举获取扫描结果 |

### Run Task 管理（JWT + admin绕过 + IAM）

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 4 | POST | /api/v1/run-tasks | JWT+BypassIAMForAdmin | admin绕过 / RUN_TASKS/ORG/WRITE | ✅ |
| 5 | GET | /api/v1/run-tasks | JWT+BypassIAMForAdmin | admin绕过 / RUN_TASKS/ORG/READ | ✅ |
| 6 | GET | /api/v1/run-tasks/:run_task_id | JWT+BypassIAMForAdmin | admin绕过 / RUN_TASKS/ORG/READ | ✅ |
| 7 | PUT | /api/v1/run-tasks/:run_task_id | JWT+BypassIAMForAdmin | admin绕过 / RUN_TASKS/ORG/WRITE | ✅ |
| 8 | DELETE | /api/v1/run-tasks/:run_task_id | JWT+BypassIAMForAdmin | admin绕过 / RUN_TASKS/ORG/ADMIN | ✅ |
| 9 | POST | /api/v1/run-tasks/test | JWT+BypassIAMForAdmin | admin绕过 / RUN_TASKS/ORG/WRITE | ✅ |
| 10 | POST | /api/v1/run-tasks/:run_task_id/test | JWT+BypassIAMForAdmin | admin绕过 / RUN_TASKS/ORG/READ | ✅ |

## 需修复项 (#1-#3)

### 问题
外部 Run Task 回调接口完全公开，任何人知道 `result_id` 即可伪造安全扫描结果，绕过 pre-plan/post-plan 审批门禁。

### 修复方案: HMAC 签名验证
- 不走 JWT/IAM 体系（服务间认证场景）
- 使用 Run Task 创建时生成的 `hmac_key` 做 HMAC-SHA256 签名校验
- 签名通过 Header `X-Run-Task-Signature: sha256=<hex>` 传递

### 修改文件
```
backend/internal/middleware/hmac_auth.go          (新建)
backend/internal/router/router_run_task.go        (添加中间件)
```

### 验证
- [ ] 无签名 → 401
- [ ] 错误签名 → 401
- [ ] 正确签名 → 正常
