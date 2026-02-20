# 21 — 全局设置 (TF版本/AI配置/平台/MFA) + Admin MFA

> 源文件: `router_global.go`
> API 数量: 21
> 状态: ✅ 全部合格

## 全部 API 列表

### Terraform 版本管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/global/settings/terraform-versions | JWT+BypassIAMForAdmin | admin绕过 / TERRAFORM_VERSIONS/ORG/READ | ✅ |
| 2 | GET | /api/v1/global/settings/terraform-versions/default | JWT+BypassIAMForAdmin | admin绕过 / TERRAFORM_VERSIONS/ORG/READ | ✅ |
| 3 | GET | /api/v1/global/settings/terraform-versions/:id | JWT+BypassIAMForAdmin | admin绕过 / TERRAFORM_VERSIONS/ORG/READ | ✅ |
| 4 | POST | /api/v1/global/settings/terraform-versions | JWT+BypassIAMForAdmin | admin绕过 / TERRAFORM_VERSIONS/ORG/WRITE | ✅ |
| 5 | PUT | /api/v1/global/settings/terraform-versions/:id | JWT+BypassIAMForAdmin | admin绕过 / TERRAFORM_VERSIONS/ORG/WRITE | ✅ |
| 6 | POST | /api/v1/global/settings/terraform-versions/:id/set-default | JWT+BypassIAMForAdmin | admin绕过 / TERRAFORM_VERSIONS/ORG/ADMIN | ✅ |
| 7 | DELETE | /api/v1/global/settings/terraform-versions/:id | JWT+BypassIAMForAdmin | admin绕过 / TERRAFORM_VERSIONS/ORG/ADMIN | ✅ |

### AI 配置管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 8 | GET | /api/v1/global/settings/ai-configs | JWT+BypassIAMForAdmin | admin绕过 / AI_CONFIGS/ORG/READ | ✅ |
| 9 | POST | /api/v1/global/settings/ai-configs | JWT+BypassIAMForAdmin | admin绕过 / AI_CONFIGS/ORG/WRITE | ✅ |
| 10 | GET | /api/v1/global/settings/ai-configs/:id | JWT+BypassIAMForAdmin | admin绕过 / AI_CONFIGS/ORG/READ | ✅ |
| 11 | PUT | /api/v1/global/settings/ai-configs/:id | JWT+BypassIAMForAdmin | admin绕过 / AI_CONFIGS/ORG/WRITE | ✅ |
| 12 | DELETE | /api/v1/global/settings/ai-configs/:id | JWT+BypassIAMForAdmin | admin绕过 / AI_CONFIGS/ORG/ADMIN | ✅ |
| 13 | PUT | /api/v1/global/settings/ai-configs/priorities | JWT+BypassIAMForAdmin | admin绕过 / AI_CONFIGS/ORG/WRITE | ✅ |
| 14 | PUT | /api/v1/global/settings/ai-configs/:id/set-default | JWT+BypassIAMForAdmin | admin绕过 / AI_CONFIGS/ORG/ADMIN | ✅ |
| 15 | GET | /api/v1/global/settings/ai-config/regions | JWT+BypassIAMForAdmin | admin绕过 / AI_CONFIGS/ORG/READ | ✅ |
| 16 | GET | /api/v1/global/settings/ai-config/models | JWT+BypassIAMForAdmin | admin绕过 / AI_CONFIGS/ORG/READ | ✅ |

### 平台配置管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 17 | GET | /api/v1/global/settings/platform-config | JWT+BypassIAMForAdmin | admin绕过 / SYSTEM_SETTINGS/ORG/READ | ✅ |
| 18 | PUT | /api/v1/global/settings/platform-config | JWT+BypassIAMForAdmin | admin绕过 / SYSTEM_SETTINGS/ORG/ADMIN | ✅ |

### MFA 全局配置

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 19 | GET | /api/v1/global/settings/mfa | JWT+BypassIAMForAdmin | admin绕过 / SYSTEM_SETTINGS/ORG/READ | ✅ |
| 20 | PUT | /api/v1/global/settings/mfa | JWT+BypassIAMForAdmin | admin绕过 / SYSTEM_SETTINGS/ORG/ADMIN | ✅ |

### Admin 用户 MFA 管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 21 | GET | /api/v1/admin/users/:user_id/mfa/status | JWT+BypassIAMForAdmin | admin绕过 / USER_MANAGEMENT/ORG/READ | ✅ |
| 22 | POST | /api/v1/admin/users/:user_id/mfa/reset | JWT+BypassIAMForAdmin | admin绕过 / USER_MANAGEMENT/ORG/ADMIN | ✅ |

## 无需修复

全局设置使用多种资源类型（TERRAFORM_VERSIONS, AI_CONFIGS, SYSTEM_SETTINGS, USER_MANAGEMENT），权限分级清晰。
