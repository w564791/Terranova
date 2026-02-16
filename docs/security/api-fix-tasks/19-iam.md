# 19 — IAM 权限/团队/组织/项目/应用/审计/用户/角色

> 源文件: `router_iam.go`
> API 数量: 50
> 状态: ✅ 全部合格

## 全部 API 列表

### 状态端点

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 1 | GET | /api/v1/iam/status | JWT+BypassIAMForAdmin | 无IAM（静态信息） | ✅ |

### 权限管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 2 | POST | /api/v1/iam/permissions/grant | JWT+BypassIAMForAdmin | admin绕过 / IAM_PERMISSIONS/ORG/ADMIN | ✅ |
| 3 | POST | /api/v1/iam/permissions/batch-grant | JWT+BypassIAMForAdmin | 同上 | ✅ |
| 4 | POST | /api/v1/iam/permissions/grant-preset | JWT+BypassIAMForAdmin | 同上 | ✅ |
| 5 | DELETE | /api/v1/iam/permissions/:scope_type/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_PERMISSIONS/ORG/ADMIN | ✅ |
| 6 | GET | /api/v1/iam/permissions/:scope_type/:scope_id | JWT+BypassIAMForAdmin | admin绕过 / IAM_PERMISSIONS/ORG/READ | ✅ |
| 7 | GET | /api/v1/iam/permissions/definitions | JWT+BypassIAMForAdmin | admin绕过 / IAM_PERMISSIONS/ORG/READ | ✅ |
| 8 | GET | /api/v1/iam/users/:id/permissions | JWT+BypassIAMForAdmin | admin绕过 / IAM_PERMISSIONS/ORG/READ | ✅ |
| 9 | GET | /api/v1/iam/teams/:id/permissions | JWT+BypassIAMForAdmin | admin绕过 / IAM_PERMISSIONS/ORG/READ | ✅ |

### 团队管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 10 | POST | /api/v1/iam/teams | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/WRITE | ✅ |
| 11 | GET | /api/v1/iam/teams | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/READ | ✅ |
| 12 | GET | /api/v1/iam/teams/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/READ | ✅ |
| 13 | DELETE | /api/v1/iam/teams/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/ADMIN | ✅ |
| 14 | POST | /api/v1/iam/teams/:id/members | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/WRITE | ✅ |
| 15 | DELETE | /api/v1/iam/teams/:id/members/:user_id | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/WRITE | ✅ |
| 16 | GET | /api/v1/iam/teams/:id/members | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/READ | ✅ |

### 团队 Token 管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 17 | POST | /api/v1/iam/teams/:id/tokens | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/WRITE | ✅ |
| 18 | GET | /api/v1/iam/teams/:id/tokens | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/READ | ✅ |
| 19 | DELETE | /api/v1/iam/teams/:id/tokens/:token_id | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/WRITE | ✅ |

### 团队角色管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 20 | POST | /api/v1/iam/teams/:id/roles | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/ADMIN | ✅ |
| 21 | GET | /api/v1/iam/teams/:id/roles | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/READ | ✅ |
| 22 | DELETE | /api/v1/iam/teams/:id/roles/:assignment_id | JWT+BypassIAMForAdmin | admin绕过 / IAM_TEAMS/ORG/ADMIN | ✅ |

### 组织管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 23 | POST | /api/v1/iam/organizations | JWT+BypassIAMForAdmin | admin绕过 / IAM_ORGANIZATIONS/ORG/ADMIN | ✅ |
| 24 | GET | /api/v1/iam/organizations | JWT+BypassIAMForAdmin | admin绕过 / IAM_ORGANIZATIONS/ORG/READ | ✅ |
| 25 | GET | /api/v1/iam/organizations/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_ORGANIZATIONS/ORG/READ | ✅ |
| 26 | PUT | /api/v1/iam/organizations/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_ORGANIZATIONS/ORG/WRITE | ✅ |
| 27 | DELETE | /api/v1/iam/organizations/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_ORGANIZATIONS/ORG/ADMIN | ✅ |

### 项目管理 (IAM)

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 28 | POST | /api/v1/iam/projects | JWT+BypassIAMForAdmin | admin绕过 / IAM_PROJECTS/ORG/WRITE | ✅ |
| 29 | GET | /api/v1/iam/projects | JWT+BypassIAMForAdmin | admin绕过 / IAM_PROJECTS/ORG/READ | ✅ |
| 30 | GET | /api/v1/iam/projects/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_PROJECTS/ORG/READ | ✅ |
| 31 | PUT | /api/v1/iam/projects/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_PROJECTS/ORG/WRITE | ✅ |
| 32 | DELETE | /api/v1/iam/projects/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_PROJECTS/ORG/ADMIN | ✅ |

### 应用管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 33 | POST | /api/v1/iam/applications | JWT+BypassIAMForAdmin | admin绕过 / IAM_APPLICATIONS/ORG/WRITE | ✅ |
| 34 | GET | /api/v1/iam/applications | JWT+BypassIAMForAdmin | admin绕过 / IAM_APPLICATIONS/ORG/READ | ✅ |
| 35 | GET | /api/v1/iam/applications/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_APPLICATIONS/ORG/READ | ✅ |
| 36 | PUT | /api/v1/iam/applications/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_APPLICATIONS/ORG/WRITE | ✅ |
| 37 | DELETE | /api/v1/iam/applications/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_APPLICATIONS/ORG/ADMIN | ✅ |
| 38 | POST | /api/v1/iam/applications/:id/regenerate-secret | JWT+BypassIAMForAdmin | admin绕过 / IAM_APPLICATIONS/ORG/ADMIN | ✅ |

### 审计日志

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 39 | GET | /api/v1/iam/audit/config | JWT+BypassIAMForAdmin | admin绕过 / IAM_AUDIT/ORG/READ | ✅ |
| 40 | PUT | /api/v1/iam/audit/config | JWT+BypassIAMForAdmin | admin绕过 / IAM_AUDIT/ORG/ADMIN | ✅ |
| 41 | GET | /api/v1/iam/audit/permission-history | JWT+BypassIAMForAdmin | admin绕过 / IAM_AUDIT/ORG/READ | ✅ |
| 42 | GET | /api/v1/iam/audit/access-history | JWT+BypassIAMForAdmin | admin绕过 / IAM_AUDIT/ORG/READ | ✅ |
| 43 | GET | /api/v1/iam/audit/denied-access | JWT+BypassIAMForAdmin | admin绕过 / IAM_AUDIT/ORG/READ | ✅ |
| 44 | GET | /api/v1/iam/audit/permission-changes-by-principal | JWT+BypassIAMForAdmin | admin绕过 / IAM_AUDIT/ORG/READ | ✅ |
| 45 | GET | /api/v1/iam/audit/permission-changes-by-performer | JWT+BypassIAMForAdmin | admin绕过 / IAM_AUDIT/ORG/READ | ✅ |

### 用户管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 46 | GET | /api/v1/iam/users/stats | JWT+BypassIAMForAdmin | admin绕过 / IAM_USERS/ORG/READ | ✅ |
| 47 | GET | /api/v1/iam/users | JWT+BypassIAMForAdmin | admin绕过 / IAM_USERS/ORG/READ | ✅ |
| 48 | POST | /api/v1/iam/users | JWT+BypassIAMForAdmin | admin绕过 / IAM_USERS/ORG/WRITE | ✅ |
| 49 | POST | /api/v1/iam/users/:id/roles | JWT+BypassIAMForAdmin | admin绕过 / IAM_USERS/ORG/ADMIN | ✅ |
| 50 | DELETE | /api/v1/iam/users/:id/roles/:assignment_id | JWT+BypassIAMForAdmin | admin绕过 / IAM_USERS/ORG/ADMIN | ✅ |
| 51 | GET | /api/v1/iam/users/:id/roles | JWT+BypassIAMForAdmin | admin绕过 / IAM_USERS/ORG/READ | ✅ |
| 52 | GET | /api/v1/iam/users/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_USERS/ORG/READ | ✅ |
| 53 | PUT | /api/v1/iam/users/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_USERS/ORG/WRITE | ✅ |
| 54 | POST | /api/v1/iam/users/:id/activate | JWT+BypassIAMForAdmin | admin绕过 / IAM_USERS/ORG/ADMIN | ✅ |
| 55 | POST | /api/v1/iam/users/:id/deactivate | JWT+BypassIAMForAdmin | admin绕过 / IAM_USERS/ORG/ADMIN | ✅ |
| 56 | DELETE | /api/v1/iam/users/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_USERS/ORG/ADMIN | ✅ |

### 角色管理

| # | Method | Path | 认证 | 授权 | 状态 |
|---|--------|------|------|------|------|
| 57 | GET | /api/v1/iam/roles | JWT+BypassIAMForAdmin | admin绕过 / IAM_ROLES/ORG/READ | ✅ |
| 58 | GET | /api/v1/iam/roles/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_ROLES/ORG/READ | ✅ |
| 59 | POST | /api/v1/iam/roles | JWT+BypassIAMForAdmin | admin绕过 / IAM_ROLES/ORG/WRITE | ✅ |
| 60 | PUT | /api/v1/iam/roles/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_ROLES/ORG/WRITE | ✅ |
| 61 | DELETE | /api/v1/iam/roles/:id | JWT+BypassIAMForAdmin | admin绕过 / IAM_ROLES/ORG/ADMIN | ✅ |
| 62 | POST | /api/v1/iam/roles/:id/policies | JWT+BypassIAMForAdmin | admin绕过 / IAM_ROLES/ORG/WRITE | ✅ |
| 63 | DELETE | /api/v1/iam/roles/:id/policies/:policy_id | JWT+BypassIAMForAdmin | admin绕过 / IAM_ROLES/ORG/WRITE | ✅ |
| 64 | POST | /api/v1/iam/roles/:id/clone | JWT+BypassIAMForAdmin | admin绕过 / IAM_ROLES/ORG/WRITE | ✅ |

## 无需修复

IAM 模块权限体系完善：
- 使用 6 种细粒度资源类型: IAM_PERMISSIONS, IAM_TEAMS, IAM_ORGANIZATIONS, IAM_PROJECTS, IAM_APPLICATIONS, IAM_AUDIT, IAM_USERS, IAM_ROLES
- READ/WRITE/ADMIN 分级清晰
- 所有接口均有 admin 绕过 + IAM fallback
