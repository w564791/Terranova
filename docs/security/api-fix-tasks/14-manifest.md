# 14 — Manifest 可视化编排 🔴 P0

> 源文件: `router_manifest.go`
> API 数量: 20

## 全部 API 列表

| # | Method | Path | 认证 | 授权 | 目标权限 | 状态 |
|---|--------|------|------|------|----------|------|
| 1 | GET | /api/v1/organizations/:oid/manifests | JWT | 无 | MANIFESTS/ORG/READ | ❌ |
| 2 | POST | /api/v1/organizations/:oid/manifests | JWT | 无 | MANIFESTS/ORG/WRITE | ❌ |
| 3 | GET | /api/v1/organizations/:oid/manifests/:id | JWT | 无 | MANIFESTS/ORG/READ | ❌ |
| 4 | PUT | /api/v1/organizations/:oid/manifests/:id | JWT | 无 | MANIFESTS/ORG/WRITE | ❌ |
| 5 | DELETE | /api/v1/organizations/:oid/manifests/:id | JWT | 无 | MANIFESTS/ORG/ADMIN | ❌ |
| 6 | PUT | /api/v1/organizations/:oid/manifests/:id/draft | JWT | 无 | MANIFESTS/ORG/WRITE | ❌ |
| 7 | GET | /api/v1/organizations/:oid/manifests/:id/versions | JWT | 无 | MANIFESTS/ORG/READ | ❌ |
| 8 | POST | /api/v1/organizations/:oid/manifests/:id/versions | JWT | 无 | MANIFESTS/ORG/WRITE | ❌ |
| 9 | GET | /api/v1/organizations/:oid/manifests/:id/versions/:vid | JWT | 无 | MANIFESTS/ORG/READ | ❌ |
| 10 | GET | /api/v1/organizations/:oid/manifests/:id/deployments | JWT | 无 | MANIFESTS/ORG/READ | ❌ |
| 11 | POST | /api/v1/organizations/:oid/manifests/:id/deployments | JWT | 无 | MANIFESTS/ORG/WRITE | ❌ |
| 12 | GET | /api/v1/organizations/:oid/manifests/:id/deployments/:did | JWT | 无 | MANIFESTS/ORG/READ | ❌ |
| 13 | PUT | /api/v1/organizations/:oid/manifests/:id/deployments/:did | JWT | 无 | MANIFESTS/ORG/WRITE | ❌ |
| 14 | DELETE | /api/v1/organizations/:oid/manifests/:id/deployments/:did | JWT | 无 | MANIFESTS/ORG/ADMIN | ❌ |
| 15 | GET | /api/v1/organizations/:oid/manifests/:id/deployments/:did/resources | JWT | 无 | MANIFESTS/ORG/READ | ❌ |
| 16 | POST | /api/v1/organizations/:oid/manifests/:id/deployments/:did/uninstall | JWT | 无 | MANIFESTS/ORG/ADMIN | ❌ |
| 17 | GET | /api/v1/organizations/:oid/manifests/:id/export | JWT | 无 | MANIFESTS/ORG/READ | ❌ |
| 18 | GET | /api/v1/organizations/:oid/manifests/:id/export-zip | JWT | 无 | MANIFESTS/ORG/READ | ❌ |
| 19 | POST | /api/v1/organizations/:oid/manifests/import | JWT | 无 | MANIFESTS/ORG/WRITE | ❌ |
| 20 | POST | /api/v1/organizations/:oid/manifests/import-json | JWT | 无 | MANIFESTS/ORG/WRITE | ❌ |

## 修复方案

### 根因
`router_manifest.go` 中 orgManifests 路由组自行添加了 `middleware.JWTAuth()`，跳过了父路由 adminProtected 的 BypassIAMForAdmin 中间件链。

### 步骤
1. 移除 orgManifests 独立的 `middleware.JWTAuth()`，依赖父路由中间件
2. 在 `permission_definitions` 注册 `MANIFESTS` 资源类型
3. 为每个接口添加 admin 绕过 + IAM 权限检查
4. GET→READ, POST/PUT→WRITE, DELETE/uninstall→ADMIN

### 修改文件
```
backend/internal/router/router_manifest.go
backend/internal/domain/valueobject/resource_type.go
数据库迁移 SQL
```
