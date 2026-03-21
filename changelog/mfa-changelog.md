# MFA功能更新日志

## 2026-02-09

### 新增
- 多因素认证(MFA)功能，支持Google Authenticator
- 用户可在个人设置中启用/禁用MFA
- 管理员可配置MFA强制策略
- 备用恢复码支持

### 数据库迁移
执行SQL脚本：[scripts/add_mfa_fields.sql](../../scripts/add_mfa_fields.sql)

```bash
docker exec -i iac-platform-postgres psql -U postgres -d iac_platform < scripts/add_mfa_fields.sql