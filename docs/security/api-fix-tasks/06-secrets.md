# 06 — 密文管理 🔴 P0

> 源文件: `router_secret.go`
> API 数量: 5

## 全部 API 列表

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | POST | /api/v1/:resourceType/:resourceId/secrets | JWT+AuditLogger | 无 | ❌ 任何认证用户可为任意资源创建密文 |
| 2 | GET | /api/v1/:resourceType/:resourceId/secrets | JWT+AuditLogger | 无 | ❌ 可读取任意资源密文列表 |
| 3 | GET | /api/v1/:resourceType/:resourceId/secrets/:secretId | JWT+AuditLogger | 无 | ❌ 可读取任意密文详情 |
| 4 | PUT | /api/v1/:resourceType/:resourceId/secrets/:secretId | JWT+AuditLogger | 无 | ❌ 可修改任意密文 |
| 5 | DELETE | /api/v1/:resourceType/:resourceId/secrets/:secretId | JWT+AuditLogger | 无 | ❌ 可删除任意密文 |

## 需修复项 (全部)

### 问题
通配符路由 `/:resourceType/:resourceId` 导致任何已认证用户可操作所有资源类型的密文（含云平台凭证）。

### 修复方案: 根据 resourceType 动态映射 IAM 权限

| resourceType | IAM ResourceType | ScopeType | GET | POST/PUT | DELETE |
|-------------|------------------|-----------|-----|----------|--------|
| agent_pool / agent-pool | AGENT_POOLS | ORGANIZATION | READ | WRITE | ADMIN |
| workspace | WORKSPACE_MANAGEMENT | WORKSPACE | READ | WRITE | ADMIN |
| module | MODULES | ORGANIZATION | READ | WRITE | ADMIN |
| 未知类型 | - | - | 403 | 403 | 403 |

### 修改文件
```
backend/internal/router/router_secret.go          (添加权限检查)
```

### 验证
- [ ] admin → 所有 resourceType 正常
- [ ] 非admin + 对应权限 → 正常
- [ ] 非admin 无权限 → 403
- [ ] 未知 resourceType → 403
- [ ] Agent PoolToken 的 /agents/pool/secrets 不受影响
