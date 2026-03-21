# SSO 单点登录 - Changelog

## v1.0.0 (2026-02-10)

### 新增功能

- **多 Provider SSO 支持**：支持 Auth0、Google、GitHub、Azure AD、Okta 及通用 OIDC 协议
- **管理员配置页面**：Global Settings → SSO Config，可视化管理 Provider 和全局策略
- **禁用本地登录**：全局开关禁用密码登录，超级管理员（is_system_admin）始终可用密码登录
- **自动创建用户**：SSO 首次登录可自动创建平台用户
- **邮箱匹配关联**：SSO 登录时自动匹配已有同邮箱用户并关联身份
- **SSO 登录日志**：记录所有 SSO 登录尝试（成功/失败/新用户创建/身份关联）
- **用户身份管理**：支持绑定/解绑多个 SSO 身份、设置主要登录方式

### 数据库变更

- 建表脚本：`scripts/create_sso_tables.sql`
- 新增表：`user_identities`（用户身份关联）、`sso_providers`（Provider 配置）、`sso_login_logs`（登录日志）
- 修改表：`users` 添加 `is_sso_user` 字段，`password_hash` 改为可空

### API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/auth/sso/providers` | 获取可用 Provider 列表（公开） |
| GET | `/api/v1/auth/sso/:provider/login` | 发起 SSO 登录 |
| GET | `/api/v1/auth/sso/:provider/callback` | SSO 回调处理 |
| GET | `/api/v1/auth/sso/identities` | 获取当前用户绑定的身份 |
| POST | `/api/v1/auth/sso/identities/link` | 绑定新 SSO 身份 |
| DELETE | `/api/v1/auth/sso/identities/:id` | 解绑 SSO 身份 |
| PUT | `/api/v1/auth/sso/identities/:id/primary` | 设置主要登录方式 |
| GET | `/api/v1/admin/sso/providers` | 管理员获取 Provider 列表（摘要） |
| GET | `/api/v1/admin/sso/providers/:id` | 管理员获取 Provider 详情（脱敏） |
| POST | `/api/v1/admin/sso/providers` | 创建 Provider |
| PUT | `/api/v1/admin/sso/providers/:id` | 更新 Provider |
| DELETE | `/api/v1/admin/sso/providers/:id` | 删除 Provider |
| GET | `/api/v1/admin/sso/config` | 获取 SSO 全局配置 |
| PUT | `/api/v1/admin/sso/config` | 更新 SSO 全局配置 |
| GET | `/api/v1/admin/sso/logs` | 获取 SSO 登录日志 |

### 验证状态

- ✅ Auth0 - 已验证通过
- ⏳ Google - 未验证
- ⏳ GitHub - 未验证
- ⏳ Azure AD - 未验证
- ⏳ Okta - 未验证

### 安全设计

- Provider 列表 API 不返回 oauth_config 详情
- Provider 详情 API 脱敏 client_secret（返回 ******）
- 前端编辑 Provider 时不回显 client_secret
- OAuth Token 使用 AES-256-GCM 加密存储（复用平台 crypto 包）
- State 参数防 CSRF（一次性使用，10 分钟过期）
- 禁用本地登录时超管例外（安全兜底）
- SSO登陆用户接受全局MFA开启约束