# 16 — Demo + Schema 独立路由

> 源文件: `router_demo.go`, `router_schema.go`
> API 数量: 9
> 状态: ✅ 全部合格

## 全部 API 列表

### Demo 管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/demos/:id | JWT+BypassIAMForAdmin | admin绕过 / MODULE_DEMOS/ORG/READ | ✅ |
| 2 | PUT | /api/v1/demos/:id | JWT+BypassIAMForAdmin | admin绕过 / MODULE_DEMOS/ORG/WRITE | ✅ |
| 3 | DELETE | /api/v1/demos/:id | JWT+BypassIAMForAdmin | admin绕过 / MODULE_DEMOS/ORG/ADMIN | ✅ |
| 4 | GET | /api/v1/demos/:id/versions | JWT+BypassIAMForAdmin | admin绕过 / MODULE_DEMOS/ORG/READ | ✅ |
| 5 | GET | /api/v1/demos/:id/compare | JWT+BypassIAMForAdmin | 同上 | ✅ |
| 6 | POST | /api/v1/demos/:id/rollback | JWT+BypassIAMForAdmin | admin绕过 / MODULE_DEMOS/ORG/WRITE | ✅ |
| 7 | GET | /api/v1/demo-versions/:versionId | JWT | admin绕过 / MODULE_DEMOS/ORG/READ | ✅ |

### Schema 管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 8 | GET | /api/v1/schemas/:id | JWT+BypassIAMForAdmin | admin绕过 / SCHEMAS/ORG/READ | ✅ |
| 9 | PUT | /api/v1/schemas/:id | JWT+BypassIAMForAdmin | admin绕过 / SCHEMAS/ORG/WRITE | ✅ |

## 无需修复
