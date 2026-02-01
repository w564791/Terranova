# Router权限ID完整清单

## 📋 文档说明

本文档列出router.go中所有路由及其对应的权限ID定义，用于确保权限系统的完整性。

**生成时间**: 2025-10-24  
**审计范围**: backend/internal/router/router.go

---

##  已定义权限的路由

### 1. Dashboard路由组 (`/api/v1/dashboard`)

| 路由 | 方法 | 权限ID | 作用域 | 级别 | 状态 |
|------|------|--------|--------|------|------|
| `/overview` | GET | ORGANIZATION | ORGANIZATION | READ |  |
| `/compliance` | GET | ORGANIZATION | ORGANIZATION | READ |  |

### 2. Workspaces路由组 (`/api/v1/workspaces`)

#### 基础操作

| 路由 | 方法 | 权限ID | 作用域 | 级别 | 状态 |
|------|------|--------|--------|------|------|
| `/` | GET | WORKSPACES | ORGANIZATION | READ |  |
| `/:id` | GET | WORKSPACES / WORKSPACE_MANAGEMENT | ORGANIZATION / WORKSPACE | READ |  |
| `/:id/overview` | GET | WORKSPACES / WORKSPACE_MANAGEMENT | ORGANIZATION / WORKSPACE | READ |  |
| `/:id` | PUT | WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id` | PATCH | WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/lock` | POST | WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/unlock` | POST | WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id` | DELETE | WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN |  |
| `/form-data` | GET | - | - | - |  **缺失** |
| `/` | POST | - | - | - |  **缺失** |

#### 任务操作

| 路由 | 方法 | 权限ID | 作用域 | 级别 | 状态 |
|------|------|--------|--------|------|------|
| `/:id/tasks` | GET | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/tasks/:task_id` | GET | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/tasks/:task_id/logs` | GET | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/tasks/:task_id/comments` | GET | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/tasks/:task_id/resource-changes` | GET | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/tasks/:task_id/state-backup` | GET | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/tasks/plan` | POST | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/tasks/:task_id/comments` | POST | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/tasks/:task_id/cancel` | POST | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN |  |
| `/:id/tasks/:task_id/cancel-previous` | POST | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN |  |
| `/:id/tasks/:task_id/confirm-apply` | POST | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN |  |
| `/:id/tasks/:task_id/resource-changes/:resource_id` | PATCH | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN |  |
| `/:id/tasks/:task_id/retry-state-save` | POST | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN |  |
| `/:id/tasks/:task_id/parse-plan` | POST | WORKSPACE_EXECUTION / WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN |  |

#### State操作

| 路由 | 方法 | 权限ID | 作用域 | 级别 | 状态 |
|------|------|--------|--------|------|------|
| `/:id/current-state` | GET | WORKSPACE_STATE / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/state-versions` | GET | WORKSPACE_STATE / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/state-versions/compare` | GET | WORKSPACE_STATE / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/state-versions/:version/metadata` | GET | WORKSPACE_STATE / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/state-versions/:version` | GET | WORKSPACE_STATE / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/state-versions/:version/rollback` | POST | WORKSPACE_STATE / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/state-versions/:version` | DELETE | WORKSPACE_STATE / WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN |  |

#### Variable操作

| 路由 | 方法 | 权限ID | 作用域 | 级别 | 状态 |
|------|------|--------|--------|------|------|
| `/:id/variables` | GET | WORKSPACE_VARIABLES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/variables/:var_id` | GET | WORKSPACE_VARIABLES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/variables` | POST | WORKSPACE_VARIABLES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/variables/:var_id` | PUT | WORKSPACE_VARIABLES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/variables/:var_id` | DELETE | WORKSPACE_VARIABLES / WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN/WRITE |  |

#### Resource操作

| 路由 | 方法 | 权限ID | 作用域 | 级别 | 状态 |
|------|------|--------|--------|------|------|
| `/:id/resources` | GET | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/resources/:resource_id` | GET | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/resources/:resource_id/versions` | GET | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/resources/:resource_id/versions/compare` | GET | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/resources/:resource_id/versions/:version` | GET | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/resources/:resource_id/dependencies` | GET | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/snapshots` | GET | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/snapshots/:snapshot_id` | GET | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/resources/:resource_id/editing/status` | GET | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/resources/:resource_id/drift` | GET | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | READ |  |
| `/:id/resources` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/resources/import` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/resources/deploy` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/resources/:resource_id` | PUT | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/resources/:resource_id` | DELETE | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN/WRITE |  |
| `/:id/resources/:resource_id/dependencies` | PUT | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/resources/:resource_id/restore` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/resources/:resource_id/versions/:version/rollback` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/snapshots` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/snapshots/:snapshot_id/restore` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/snapshots/:snapshot_id` | DELETE | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN/WRITE |  |
| `/:id/resources/:resource_id/editing/start` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/resources/:resource_id/editing/heartbeat` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/resources/:resource_id/editing/end` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/resources/:resource_id/drift/save` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/resources/:resource_id/drift/takeover` | POST | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | WRITE |  |
| `/:id/resources/:resource_id/drift` | DELETE | WORKSPACE_RESOURCES / WORKSPACE_MANAGEMENT | WORKSPACE | ADMIN/WRITE |  |

### 3. Modules路由组 (`/api/v1/modules`)

| 路由 | 方法 | 权限ID | 作用域 | 级别 | 状态 |
|------|------|--------|--------|------|------|
| `/` | GET | MODULES | ORGANIZATION | READ |  |
| `/:id` | GET | MODULES | ORGANIZATION | READ |  |
| `/:id/files` | GET | MODULES | ORGANIZATION | READ |  |
| `/:id/schemas` | GET | MODULES | ORGANIZATION | READ |  |
| `/:id/demos` | GET | MODULES | ORGANIZATION | READ |  |
| `/` | POST | MODULES | ORGANIZATION | WRITE |  |
| `/:id` | PUT | MODULES | ORGANIZATION | WRITE |  |
| `/:id` | PATCH | MODULES | ORGANIZATION | WRITE |  |
| `/:id/sync` | POST | MODULES | ORGANIZATION | WRITE |  |
| `/parse-tf` | POST | MODULES | ORGANIZATION | WRITE |  |
| `/:id/schemas` | POST | MODULES | ORGANIZATION | WRITE |  |
| `/:id/schemas/generate` | POST | MODULES | ORGANIZATION | WRITE |  |
| `/:id/demos` | POST | MODULES | ORGANIZATION | WRITE |  |
| `/:id` | DELETE | MODULES | ORGANIZATION | ADMIN |  |

---

##  缺少权限定义的路由

### 1. Workspaces相关 (2个)

| 路由 | 方法 | 当前状态 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|----------|------------|------------|----------|
| `/workspaces/form-data` | GET | 仅Admin | WORKSPACES | ORGANIZATION | READ |
| `/workspaces` | POST | 仅Admin | WORKSPACES | ORGANIZATION | WRITE |

### 2. User相关 (1个)

| 路由 | 方法 | 当前状态 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|----------|------------|------------|----------|
| `/user/reset-password` | POST | JWT认证 | USER_MANAGEMENT | USER | WRITE |

### 3. Demos相关 (6个)

| 路由 | 方法 | 当前状态 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|----------|------------|------------|----------|
| `/demos/:id` | GET | Admin | MODULE_DEMOS | ORGANIZATION | READ |
| `/demos/:id` | PUT | Admin | MODULE_DEMOS | ORGANIZATION | WRITE |
| `/demos/:id` | DELETE | Admin | MODULE_DEMOS | ORGANIZATION | ADMIN |
| `/demos/:id/versions` | GET | Admin | MODULE_DEMOS | ORGANIZATION | READ |
| `/demos/:id/compare` | GET | Admin | MODULE_DEMOS | ORGANIZATION | READ |
| `/demos/:id/rollback` | POST | Admin | MODULE_DEMOS | ORGANIZATION | WRITE |
| `/demo-versions/:versionId` | GET | JWT认证 | MODULE_DEMOS | ORGANIZATION | READ |

### 4. Schemas相关 (2个)

| 路由 | 方法 | 当前状态 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|----------|------------|------------|----------|
| `/schemas/:id` | GET | Admin | SCHEMAS | ORGANIZATION | READ |
| `/schemas/:id` | PUT | Admin | SCHEMAS | ORGANIZATION | WRITE |

### 5. Tasks相关 (4个)

| 路由 | 方法 | 当前状态 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|----------|------------|------------|----------|
| `/tasks/:task_id/output/stream` | GET | JWT认证 | TASK_LOGS | ORGANIZATION | READ |
| `/tasks/:task_id/logs` | GET | JWT认证 | TASK_LOGS | ORGANIZATION | READ |
| `/tasks/:task_id/logs/download` | GET | JWT认证 | TASK_LOGS | ORGANIZATION | READ |
| `/terraform/streams/stats` | GET | JWT认证 | TASK_LOGS | ORGANIZATION | READ |

### 6. Agents相关 (8个)

| 路由 | 方法 | 当前状态 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|----------|------------|------------|----------|
| `/agents/register` | POST | Admin | AGENTS | ORGANIZATION | WRITE |
| `/agents/heartbeat` | POST | Admin | AGENTS | ORGANIZATION | WRITE |
| `/agents` | GET | Admin | AGENTS | ORGANIZATION | READ |
| `/agents/:id` | GET | Admin | AGENTS | ORGANIZATION | READ |
| `/agents/:id` | PUT | Admin | AGENTS | ORGANIZATION | WRITE |
| `/agents/:id` | DELETE | Admin | AGENTS | ORGANIZATION | ADMIN |
| `/agents/:id/revoke-token` | POST | Admin | AGENTS | ORGANIZATION | ADMIN |
| `/agents/:id/regenerate-token` | POST | Admin | AGENTS | ORGANIZATION | ADMIN |

### 7. Agent Pools相关 (7个)

| 路由 | 方法 | 当前状态 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|----------|------------|------------|----------|
| `/agent-pools` | POST | Admin | AGENT_POOLS | ORGANIZATION | WRITE |
| `/agent-pools` | GET | Admin | AGENT_POOLS | ORGANIZATION | READ |
| `/agent-pools/:id` | GET | Admin | AGENT_POOLS | ORGANIZATION | READ |
| `/agent-pools/:id` | PUT | Admin | AGENT_POOLS | ORGANIZATION | WRITE |
| `/agent-pools/:id` | DELETE | Admin | AGENT_POOLS | ORGANIZATION | ADMIN |
| `/agent-pools/:id/agents` | POST | Admin | AGENT_POOLS | ORGANIZATION | WRITE |
| `/agent-pools/:id/agents/:agent_id` | DELETE | Admin | AGENT_POOLS | ORGANIZATION | WRITE |

### 8. IAM相关 (51个)

所有IAM路由当前都是Admin only，建议添加细粒度权限：

#### 权限管理 (7个)

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/iam/permissions/check` | POST | IAM_PERMISSIONS | ORGANIZATION | READ |
| `/iam/permissions/grant` | POST | IAM_PERMISSIONS | ORGANIZATION | ADMIN |
| `/iam/permissions/batch-grant` | POST | IAM_PERMISSIONS | ORGANIZATION | ADMIN |
| `/iam/permissions/grant-preset` | POST | IAM_PERMISSIONS | ORGANIZATION | ADMIN |
| `/iam/permissions/:scope_type/:id` | DELETE | IAM_PERMISSIONS | ORGANIZATION | ADMIN |
| `/iam/permissions/:scope_type/:scope_id` | GET | IAM_PERMISSIONS | ORGANIZATION | READ |
| `/iam/permissions/definitions` | GET | IAM_PERMISSIONS | ORGANIZATION | READ |

#### 团队管理 (7个)

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/iam/teams` | POST | IAM_TEAMS | ORGANIZATION | WRITE |
| `/iam/teams` | GET | IAM_TEAMS | ORGANIZATION | READ |
| `/iam/teams/:id` | GET | IAM_TEAMS | ORGANIZATION | READ |
| `/iam/teams/:id` | DELETE | IAM_TEAMS | ORGANIZATION | ADMIN |
| `/iam/teams/:id/members` | POST | IAM_TEAMS | ORGANIZATION | WRITE |
| `/iam/teams/:id/members/:user_id` | DELETE | IAM_TEAMS | ORGANIZATION | WRITE |
| `/iam/teams/:id/members` | GET | IAM_TEAMS | ORGANIZATION | READ |

#### 组织管理 (4个)

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/iam/organizations` | POST | IAM_ORGANIZATIONS | ORGANIZATION | ADMIN |
| `/iam/organizations` | GET | IAM_ORGANIZATIONS | ORGANIZATION | READ |
| `/iam/organizations/:id` | GET | IAM_ORGANIZATIONS | ORGANIZATION | READ |
| `/iam/organizations/:id` | PUT | IAM_ORGANIZATIONS | ORGANIZATION | WRITE |

#### 项目管理 (5个)

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/iam/projects` | POST | IAM_PROJECTS | ORGANIZATION | WRITE |
| `/iam/projects` | GET | IAM_PROJECTS | ORGANIZATION | READ |
| `/iam/projects/:id` | GET | IAM_PROJECTS | ORGANIZATION | READ |
| `/iam/projects/:id` | PUT | IAM_PROJECTS | ORGANIZATION | WRITE |
| `/iam/projects/:id` | DELETE | IAM_PROJECTS | ORGANIZATION | ADMIN |

#### 应用管理 (6个)

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/iam/applications` | POST | IAM_APPLICATIONS | ORGANIZATION | WRITE |
| `/iam/applications` | GET | IAM_APPLICATIONS | ORGANIZATION | READ |
| `/iam/applications/:id` | GET | IAM_APPLICATIONS | ORGANIZATION | READ |
| `/iam/applications/:id` | PUT | IAM_APPLICATIONS | ORGANIZATION | WRITE |
| `/iam/applications/:id` | DELETE | IAM_APPLICATIONS | ORGANIZATION | ADMIN |
| `/iam/applications/:id/regenerate-secret` | POST | IAM_APPLICATIONS | ORGANIZATION | ADMIN |

#### 审计日志 (7个)

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/iam/audit/config` | GET | IAM_AUDIT | ORGANIZATION | READ |
| `/iam/audit/config` | PUT | IAM_AUDIT | ORGANIZATION | ADMIN |
| `/iam/audit/permission-history` | GET | IAM_AUDIT | ORGANIZATION | READ |
| `/iam/audit/access-history` | GET | IAM_AUDIT | ORGANIZATION | READ |
| `/iam/audit/denied-access` | GET | IAM_AUDIT | ORGANIZATION | READ |
| `/iam/audit/permission-changes-by-principal` | GET | IAM_AUDIT | ORGANIZATION | READ |
| `/iam/audit/permission-changes-by-performer` | GET | IAM_AUDIT | ORGANIZATION | READ |

#### 用户管理 (8个)

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/iam/users/stats` | GET | IAM_USERS | ORGANIZATION | READ |
| `/iam/users` | GET | IAM_USERS | ORGANIZATION | READ |
| `/iam/users/:id/roles` | POST | IAM_USERS | ORGANIZATION | ADMIN |
| `/iam/users/:id/roles/:assignment_id` | DELETE | IAM_USERS | ORGANIZATION | ADMIN |
| `/iam/users/:id/roles` | GET | IAM_USERS | ORGANIZATION | READ |
| `/iam/users/:id` | GET | IAM_USERS | ORGANIZATION | READ |
| `/iam/users/:id` | PUT | IAM_USERS | ORGANIZATION | WRITE |
| `/iam/users/:id/activate` | POST | IAM_USERS | ORGANIZATION | ADMIN |
| `/iam/users/:id/deactivate` | POST | IAM_USERS | ORGANIZATION | ADMIN |

#### 角色管理 (7个)

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/iam/roles` | GET | IAM_ROLES | ORGANIZATION | READ |
| `/iam/roles/:id` | GET | IAM_ROLES | ORGANIZATION | READ |
| `/iam/roles` | POST | IAM_ROLES | ORGANIZATION | WRITE |
| `/iam/roles/:id` | PUT | IAM_ROLES | ORGANIZATION | WRITE |
| `/iam/roles/:id` | DELETE | IAM_ROLES | ORGANIZATION | ADMIN |
| `/iam/roles/:id/policies` | POST | IAM_ROLES | ORGANIZATION | WRITE |
| `/iam/roles/:id/policies/:policy_id` | DELETE | IAM_ROLES | ORGANIZATION | WRITE |

### 9. Admin相关 (16个)

#### Terraform版本管理 (7个)

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/admin/terraform-versions` | GET | TERRAFORM_VERSIONS | ORGANIZATION | READ |
| `/admin/terraform-versions/default` | GET | TERRAFORM_VERSIONS | ORGANIZATION | READ |
| `/admin/terraform-versions/:id` | GET | TERRAFORM_VERSIONS | ORGANIZATION | READ |
| `/admin/terraform-versions` | POST | TERRAFORM_VERSIONS | ORGANIZATION | WRITE |
| `/admin/terraform-versions/:id` | PUT | TERRAFORM_VERSIONS | ORGANIZATION | WRITE |
| `/admin/terraform-versions/:id/set-default` | POST | TERRAFORM_VERSIONS | ORGANIZATION | ADMIN |
| `/admin/terraform-versions/:id` | DELETE | TERRAFORM_VERSIONS | ORGANIZATION | ADMIN |

#### AI配置管理 (9个)

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/admin/ai-configs` | GET | AI_CONFIGS | ORGANIZATION | READ |
| `/admin/ai-configs` | POST | AI_CONFIGS | ORGANIZATION | WRITE |
| `/admin/ai-configs/:id` | GET | AI_CONFIGS | ORGANIZATION | READ |
| `/admin/ai-configs/:id` | PUT | AI_CONFIGS | ORGANIZATION | WRITE |
| `/admin/ai-configs/:id` | DELETE | AI_CONFIGS | ORGANIZATION | ADMIN |
| `/admin/ai-configs/priorities` | PUT | AI_CONFIGS | ORGANIZATION | WRITE |
| `/admin/ai-configs/:id/set-default` | PUT | AI_CONFIGS | ORGANIZATION | ADMIN |
| `/admin/ai-config/regions` | GET | AI_CONFIGS | ORGANIZATION | READ |
| `/admin/ai-config/models` | GET | AI_CONFIGS | ORGANIZATION | READ |

### 10. AI分析 (1个)

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/ai/analyze-error` | POST | AI_ANALYSIS | ORGANIZATION | WRITE |

---

## 📊 统计摘要

| 类别 | 数量 | 说明 |
|------|------|------|
| 总路由数 | 150+ | 所有API端点 |
| 已定义权限 | 约90个 | 主要是Workspaces和Modules |
| 缺少权限定义 | 约60个 | 需要补充权限ID |
| 公开路由 | 4个 | 无需认证 |

### 缺失权限分类统计

| 模块 | 缺失数量 |
|------|----------|
| Workspaces | 2 |
| User | 1 |
| Demos | 7 |
| Schemas | 2 |
| Tasks | 4 |
| Agents | 8 |
| Agent Pools | 7 |
| IAM | 51 |
| Admin (Terraform) | 7 |
| Admin (AI) | 9 |
| AI Analysis | 1 |
| **总计** | **99** |

---

## 🎯 建议的权限ID体系

基于业务语义ID方案，建议的权限ID前缀：

| 权限类别 | 前缀 | 示例 |
|----------|------|------|
| Workspace权限 | wspm | wspm-management-read |
| Organization权限 | orgpm | orgpm-modules-write |
| Project权限 | pjpm | pjpm-resources-read |
| Module权限 | mdpm | mdpm-demos-write |
| Team权限 | tmpm | tmpm-members-write |
| User权限 | uspm | uspm-profile-write |
| Agent权限 | agpm | agpm-register-write |
| Agent Pool权限 | appm | appm-manage-write |
| IAM权限 | iampm | iampm-permissions-admin |
| Admin权限 | admpm | admpm-terraform-write |

---

##  下一步行动

1. **立即**: 为缺失的路由添加权限检查
2. **短期**: 实施业务语义ID体系
3. **中期**: 完善权限定义和文档
4. **长期**: 移除Admin绕过机制，统一使用IAM权限

---

**文档维护**: 每次添加新路由时，必须同步更新此清单
