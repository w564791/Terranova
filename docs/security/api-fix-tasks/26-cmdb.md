# 26 — CMDB 资源索引 🟡 P1

> 源文件: `router_cmdb.go`
> API 数量: 16

## 全部 API 列表

### 只读查询（所有认证用户可访问）

| # | Method | Path | 认证 | 授权 | 目标权限 | 状态 |
|---|--------|------|------|------|----------|------|
| 1 | GET | /api/v1/cmdb/search | JWT+BypassIAMForAdmin | 无IAM | CMDB/ORG/READ |  |
| 2 | GET | /api/v1/cmdb/suggestions | JWT+BypassIAMForAdmin | 无IAM | CMDB/ORG/READ |  |
| 3 | GET | /api/v1/cmdb/stats | JWT+BypassIAMForAdmin | 无IAM | CMDB/ORG/READ |  |
| 4 | GET | /api/v1/cmdb/resource-types | JWT+BypassIAMForAdmin | 无IAM | CMDB/ORG/READ |  |
| 5 | GET | /api/v1/cmdb/workspace-counts | JWT+BypassIAMForAdmin | 无IAM | CMDB/ORG/READ |  |
| 6 | GET | /api/v1/cmdb/workspaces/:workspace_id/tree | JWT+BypassIAMForAdmin | 无IAM | CMDB/ORG/READ |  |
| 7 | GET | /api/v1/cmdb/workspaces/:workspace_id/resources | JWT+BypassIAMForAdmin | 无IAM | CMDB/ORG/READ |  |

### 同步操作

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 8 | POST | /api/v1/cmdb/workspaces/:workspace_id/sync | JWT+BypassIAMForAdmin | cmdb/ORG/ADMIN | ✅ |
| 9 | POST | /api/v1/cmdb/sync-all | JWT+BypassIAMForAdmin | cmdb/ORG/ADMIN | ✅ |

### 外部数据源管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 10 | GET | /api/v1/cmdb/external-sources | JWT+BypassIAMForAdmin | cmdb/ORG/ADMIN | ✅ |
| 11 | POST | /api/v1/cmdb/external-sources | JWT+BypassIAMForAdmin | cmdb/ORG/ADMIN | ✅ |
| 12 | GET | /api/v1/cmdb/external-sources/:source_id | JWT+BypassIAMForAdmin | cmdb/ORG/ADMIN | ✅ |
| 13 | PUT | /api/v1/cmdb/external-sources/:source_id | JWT+BypassIAMForAdmin | cmdb/ORG/ADMIN | ✅ |
| 14 | DELETE | /api/v1/cmdb/external-sources/:source_id | JWT+BypassIAMForAdmin | cmdb/ORG/ADMIN | ✅ |
| 15 | POST | /api/v1/cmdb/external-sources/:source_id/sync | JWT+BypassIAMForAdmin | cmdb/ORG/ADMIN | ✅ |
| 16 | POST | /api/v1/cmdb/external-sources/:source_id/test | JWT+BypassIAMForAdmin | cmdb/ORG/ADMIN | ✅ |

## 修复方案

### 问题
1. **只读接口 (#1-#7) 缺少 IAM 权限检查**：所有认证用户（包括非admin）都可以无限制访问 CMDB 数据
2. **资源类型命名不一致**: 使用小写 `cmdb` 而非大写 `CMDB`，与其他资源类型风格不一致
3. **同步和外部数据源接口缺少 admin 绕过**: 直接使用 `RequirePermission` 中间件，无 admin 绕过逻辑

### 步骤
1. 为只读接口添加 admin 绕过 + IAM: `CMDB/ORG/READ`
2. 统一资源类型命名为大写 `CMDB`
3. 为同步和外部数据源接口添加 admin 绕过逻辑
4. 在 `permission_definitions` 注册 `CMDB` 资源类型

### 修改文件
```
backend/internal/router/router_cmdb.go
backend/internal/domain/valueobject/resource_type.go
数据库迁移 SQL
```
