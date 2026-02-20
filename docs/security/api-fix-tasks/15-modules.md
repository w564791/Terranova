# 15 — Module 管理（含 Schema V2 / Version）

> 源文件: `router_module.go`
> API 数量: ~30
> 状态: ✅ 全部合格

## 全部 API 列表

### Module CRUD

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/modules | JWT+AuditLogger | admin绕过 / MODULES/ORG/READ | ✅ |
| 2 | GET | /api/v1/modules/:id | JWT+AuditLogger | 同上 | ✅ |
| 3 | GET | /api/v1/modules/:id/files | JWT+AuditLogger | 同上 | ✅ |
| 4 | GET | /api/v1/modules/:id/schemas | JWT+AuditLogger | 同上 | ✅ |
| 5 | GET | /api/v1/modules/:id/demos | JWT+AuditLogger | 同上 | ✅ |
| 6 | GET | /api/v1/modules/:id/prompts | JWT+AuditLogger | 同上 | ✅ |
| 7 | POST | /api/v1/modules | JWT+AuditLogger | admin绕过 / MODULES/ORG/WRITE | ✅ |
| 8 | PUT | /api/v1/modules/:id | JWT+AuditLogger | 同上 | ✅ |
| 9 | PATCH | /api/v1/modules/:id | JWT+AuditLogger | 同上 | ✅ |
| 10 | POST | /api/v1/modules/:id/sync | JWT+AuditLogger | 同上 | ✅ |
| 11 | POST | /api/v1/modules/parse-tf | JWT+AuditLogger | 同上 | ✅ |
| 12 | POST | /api/v1/modules/:id/schemas | JWT+AuditLogger | 同上 | ✅ |
| 13 | POST | /api/v1/modules/:id/schemas/generate | JWT+AuditLogger | 同上 | ✅ |
| 14 | POST | /api/v1/modules/:id/demos | JWT+AuditLogger | 同上 | ✅ |
| 15 | DELETE | /api/v1/modules/:id | JWT+AuditLogger | admin绕过 / MODULES/ORG/ADMIN | ✅ |

### Schema V2

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 16 | POST | /api/v1/modules/parse-tf-v2 | JWT+AuditLogger | admin绕过 / MODULES/ORG/WRITE | ✅ |
| 17 | GET | /api/v1/modules/:id/schemas/v2 | JWT+AuditLogger | admin绕过 / MODULES/ORG/READ | ✅ |
| 18 | GET | /api/v1/modules/:id/schemas/all | JWT+AuditLogger | 同上 | ✅ |
| 19 | POST | /api/v1/modules/:id/schemas/v2 | JWT+AuditLogger | admin绕过 / MODULES/ORG/WRITE | ✅ |
| 20 | PUT | /api/v1/modules/:id/schemas/v2/:schemaId | JWT+AuditLogger | 同上 | ✅ |
| 21 | PATCH | /api/v1/modules/:id/schemas/v2/:schemaId/fields | JWT+AuditLogger | 同上 | ✅ |
| 22 | POST | /api/v1/modules/:id/schemas/:schemaId/migrate-v2 | JWT+AuditLogger | 同上 | ✅ |
| 23 | GET | /api/v1/modules/:id/schemas/compare | JWT+AuditLogger | admin绕过 / MODULES/ORG/READ | ✅ |
| 24 | POST | /api/v1/modules/:id/schemas/:schemaId/activate | JWT+AuditLogger | admin绕过 / MODULES/ORG/WRITE | ✅ |
| 25 | POST | /api/v1/modules/:id/validate | JWT+AuditLogger | admin绕过 / MODULES/ORG/READ | ✅ |

### Module Version

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 26 | GET | /api/v1/modules/:id/versions | JWT+AuditLogger | admin绕过 / MODULES/ORG/READ | ✅ |
| 27 | GET | /api/v1/modules/:id/versions/:vid | JWT+AuditLogger | 同上 | ✅ |
| 28 | GET | /api/v1/modules/:id/default-version | JWT+AuditLogger | 同上 | ✅ |
| 29 | GET | /api/v1/modules/:id/versions/compare | JWT+AuditLogger | 同上 | ✅ |
| 30 | GET | /api/v1/modules/:id/versions/:vid/schemas | JWT+AuditLogger | 同上 | ✅ |
| 31 | GET | /api/v1/modules/:id/versions/:vid/demos | JWT+AuditLogger | 同上 | ✅ |
| 32 | POST | /api/v1/modules/:id/versions | JWT+AuditLogger | admin绕过 / MODULES/ORG/WRITE | ✅ |
| 33 | PUT | /api/v1/modules/:id/versions/:vid | JWT+AuditLogger | 同上 | ✅ |
| 34 | PUT | /api/v1/modules/:id/default-version | JWT+AuditLogger | 同上 | ✅ |
| 35 | POST | /api/v1/modules/:id/versions/:vid/inherit-demos | JWT+AuditLogger | 同上 | ✅ |
| 36 | POST | /api/v1/modules/:id/versions/:vid/import-demos | JWT+AuditLogger | 同上 | ✅ |
| 37 | DELETE | /api/v1/modules/:id/versions/:vid | JWT+AuditLogger | admin绕过 / MODULES/ORG/ADMIN | ✅ |
| 38 | POST | /api/v1/modules/migrate-versions | JWT+AuditLogger | admin绕过 / MODULES/ORG/ADMIN | ✅ |

## 无需修复

Module 权限统一使用 MODULES 资源类型，READ/WRITE/ADMIN 分级清晰。
