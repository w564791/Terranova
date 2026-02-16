# 18 — Admin Skills / Embedding / Cache ⚠️ P2

> 源文件: `router_ai.go`
> API 数量: 25

## 全部 API 列表

### Embedding 管理

| # | Method | Path | 认证 | 授权 | 目标权限 | 状态 |
|---|--------|------|------|------|----------|------|
| 1 | GET | /api/v1/admin/embedding/status | JWT+BypassIAMForAdmin | admin绕过/隐式拒绝非admin | EMBEDDING_MANAGEMENT/ORG/READ | ⚠️ |
| 2 | POST | /api/v1/admin/embedding/sync-all | JWT+BypassIAMForAdmin | 同上 | EMBEDDING_MANAGEMENT/ORG/WRITE | ⚠️ |

### Skill 管理

| # | Method | Path | 认证 | 授权 | 目标权限 | 状态 |
|---|--------|------|------|------|----------|------|
| 3 | GET | /api/v1/admin/skills | JWT+BypassIAMForAdmin | admin绕过/隐式拒绝 | SKILLS/ORG/READ | ⚠️ |
| 4 | GET | /api/v1/admin/skills/preview-discovery | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/READ | ⚠️ |
| 5 | GET | /api/v1/admin/skills/:id | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/READ | ⚠️ |
| 6 | POST | /api/v1/admin/skills | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/WRITE | ⚠️ |
| 7 | PUT | /api/v1/admin/skills/:id | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/WRITE | ⚠️ |
| 8 | DELETE | /api/v1/admin/skills/:id | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/ADMIN | ⚠️ |
| 9 | POST | /api/v1/admin/skills/:id/activate | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/WRITE | ⚠️ |
| 10 | POST | /api/v1/admin/skills/:id/deactivate | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/WRITE | ⚠️ |
| 11 | GET | /api/v1/admin/skills/:id/usage-stats | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/READ | ⚠️ |

### Module Skill

| # | Method | Path | 认证 | 授权 | 目标权限 | 状态 |
|---|--------|------|------|------|----------|------|
| 12 | GET | /api/v1/admin/modules/:mid/skill | JWT+BypassIAMForAdmin | admin绕过/隐式拒绝 | SKILLS/ORG/READ | ⚠️ |
| 13 | POST | /api/v1/admin/modules/:mid/skill/generate | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/WRITE | ⚠️ |
| 14 | PUT | /api/v1/admin/modules/:mid/skill | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/WRITE | ⚠️ |
| 15 | GET | /api/v1/admin/modules/:mid/skill/preview | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/READ | ⚠️ |

### Module Version Skill

| # | Method | Path | 认证 | 授权 | 目标权限 | 状态 |
|---|--------|------|------|------|----------|------|
| 16 | GET | /api/v1/admin/module-versions/:id/skill | JWT+BypassIAMForAdmin | admin绕过/隐式拒绝 | SKILLS/ORG/READ | ⚠️ |
| 17 | POST | /api/v1/admin/module-versions/:id/skill/generate | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/WRITE | ⚠️ |
| 18 | PUT | /api/v1/admin/module-versions/:id/skill | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/WRITE | ⚠️ |
| 19 | POST | /api/v1/admin/module-versions/:id/skill/inherit | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/WRITE | ⚠️ |
| 20 | DELETE | /api/v1/admin/module-versions/:id/skill | JWT+BypassIAMForAdmin | 同上 | SKILLS/ORG/ADMIN | ⚠️ |

### Embedding Cache

| # | Method | Path | 认证 | 授权 | 目标权限 | 状态 |
|---|--------|------|------|------|----------|------|
| 21 | POST | /api/v1/admin/embedding-cache/warmup | JWT+BypassIAMForAdmin | admin绕过/隐式拒绝 | EMBEDDING_MANAGEMENT/ORG/WRITE | ⚠️ |
| 22 | GET | /api/v1/admin/embedding-cache/warmup/progress | JWT+BypassIAMForAdmin | 同上 | EMBEDDING_MANAGEMENT/ORG/READ | ⚠️ |
| 23 | GET | /api/v1/admin/embedding-cache/stats | JWT+BypassIAMForAdmin | 同上 | EMBEDDING_MANAGEMENT/ORG/READ | ⚠️ |
| 24 | DELETE | /api/v1/admin/embedding-cache/clear | JWT+BypassIAMForAdmin | 同上 | EMBEDDING_MANAGEMENT/ORG/ADMIN | ⚠️ |
| 25 | POST | /api/v1/admin/embedding-cache/cleanup | JWT+BypassIAMForAdmin | 同上 | EMBEDDING_MANAGEMENT/ORG/WRITE | ⚠️ |

## 修复方案

新增 `SKILLS` 和 `EMBEDDING_MANAGEMENT` 资源类型，为每个接口添加 admin 绕过 + IAM fallback。

### 修改文件
```
backend/internal/router/router_ai.go
backend/internal/domain/valueobject/resource_type.go
数据库迁移 SQL
```
