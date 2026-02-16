# 07 — 用户自服务（密码/Token）

> 源文件: `router_user.go`
> API 数量: 4 (MFA 已在 01-public-and-auth.md 中)

## 全部 API 列表

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | POST | /api/v1/user/reset-password | JWT+BypassIAMForAdmin | admin绕过 / USER_MANAGEMENT/USER/WRITE | ✅ 管理员重置他人密码 |
| 2 | POST | /api/v1/user/change-password | JWT+BypassIAMForAdmin | 无IAM | ⚠️ 用户改自己密码，风险低 |
| 3 | POST | /api/v1/user/tokens | JWT+BypassIAMForAdmin | 无IAM | ⚠️ 用户创建自己的Token |
| 4 | GET | /api/v1/user/tokens | JWT+BypassIAMForAdmin | 无IAM | ⚠️ 用户查看自己的Token |
| 5 | DELETE | /api/v1/user/tokens/:token_name | JWT+BypassIAMForAdmin | 无IAM | ⚠️ 用户撤销自己的Token |

## 修复建议 (#2-#5, 可选)

### 问题
用户自服务接口，handler 已校验仅操作自身数据，风险低。但缺少组织级策略控制能力。

### 修复方案（可选）
- `change-password`: handler 中可检查 SSO-only 用户禁止设本地密码
- Token 管理: 保持现状即可，或后续添加可选的组织级策略
- **推荐**: 保持现状，不做 IAM 变更

### 修改文件
```
backend/internal/handlers/user_token_handler.go   (可选: SSO策略检查)
```
