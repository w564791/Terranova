# IAM 权限资源类型参考手册

> 本文档按 IAM 资源类型（ResourceType）分类，说明每种权限的用途、可用级别、以及对应的 API 接口。
>
> 权限格式: `ResourceType / ScopeType / Level`
>
> 权限级别: `NONE`(显式拒绝) < `READ`(只读) < `WRITE`(读写) < `ADMIN`(管理)
>
> **级别包含关系**: 高级别自动包含低级别权限。ADMIN 可访问所有接口，WRITE 可访问 WRITE+READ 接口，READ 只能访问 READ 接口。
> 表格中的"最低级别"表示该接口**要求的最低权限**，持有更高级别的用户同样可以访问。
>
> 判定逻辑: `effectiveLevel >= requiredLevel`（见 `permission_checker.go:131`）
>
> 作用域优先级: `WORKSPACE`(3) > `PROJECT`(2) > `ORGANIZATION`(1)
>
> 源代码: `backend/internal/domain/valueobject/resource_type.go`

---

## 一、组织级资源 (ORGANIZATION Scope)

### 1. ORGANIZATION

**说明**: 组织管理，控制组织级总览信息的访问

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/dashboard/overview` — 仪表盘总览统计 |
| READ | `GET /api/v1/dashboard/compliance` — 合规性统计 |

**使用文件**: `router_dashboard.go`

---

### 2. WORKSPACES

**说明**: 工作空间管理，控制组织级别的工作空间列表/创建/删除，以及工作空间级别的读写操作

**组织级 (ORGANIZATION scope)**:

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/workspaces` — 工作空间列表 |
| READ | `GET /api/v1/projects` — 项目列表（含工作空间计数） |
| READ | `GET /api/v1/projects/:id/workspaces` — 项目下的工作空间列表 |
| WRITE | `POST /api/v1/workspaces` — 创建工作空间 |
| WRITE | `PUT /api/v1/workspaces/:id/settings` — 更新工作空间设置 |

**工作空间级 (WORKSPACE scope)**:

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/workspaces/:id/available-pools` — 可用Agent Pool列表 |
| READ | `GET /api/v1/workspaces/:id/current-pool` — 当前Agent Pool |
| WRITE | `POST /api/v1/workspaces/:id/set-current-pool` — 设置Agent Pool |

**使用文件**: `router_workspace.go`, `router_project.go`, `router_agent.go`

---

### 3. MODULES

**说明**: Terraform 模块管理，控制模块的CRUD、Schema、版本等操作

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/modules` — 模块列表 |
| READ | `GET /api/v1/modules/:id` — 模块详情 |
| READ | `GET /api/v1/modules/:id/files` — 模块文件列表 |
| READ | `GET /api/v1/modules/:id/schemas` — 模块Schema列表 |
| READ | `GET /api/v1/modules/:id/demos` — 模块Demo列表 |
| READ | `GET /api/v1/modules/:id/prompts` — 模块提示词 |
| READ | `GET /api/v1/modules/:id/schemas/v2` — Schema V2列表 |
| READ | `GET /api/v1/modules/:id/schemas/all` — 全部Schema |
| READ | `GET /api/v1/modules/:id/schemas/compare` — Schema对比 |
| READ | `GET /api/v1/modules/:id/validate` — 模块校验 |
| READ | `GET /api/v1/modules/:id/versions` — 版本列表 |
| READ | `GET /api/v1/modules/:id/versions/:vid` — 版本详情 |
| READ | `GET /api/v1/modules/:id/default-version` — 默认版本 |
| READ | `GET /api/v1/modules/:id/versions/compare` — 版本对比 |
| READ | `GET /api/v1/modules/:id/versions/:vid/schemas` — 版本Schema |
| READ | `GET /api/v1/modules/:id/versions/:vid/demos` — 版本Demo |
| WRITE | `POST /api/v1/modules` — 创建模块 |
| WRITE | `PUT /api/v1/modules/:id` — 更新模块 |
| WRITE | `PATCH /api/v1/modules/:id` — 部分更新模块 |
| WRITE | `POST /api/v1/modules/:id/sync` — 同步模块 |
| WRITE | `POST /api/v1/modules/parse-tf` — 解析TF |
| WRITE | `POST /api/v1/modules/parse-tf-v2` — 解析TF V2 |
| WRITE | `POST /api/v1/modules/:id/schemas` — 创建Schema |
| WRITE | `POST /api/v1/modules/:id/schemas/generate` — 生成Schema |
| WRITE | `POST /api/v1/modules/:id/schemas/v2` — 创建Schema V2 |
| WRITE | `PUT /api/v1/modules/:id/schemas/v2/:schemaId` — 更新Schema V2 |
| WRITE | `PATCH /api/v1/modules/:id/schemas/v2/:schemaId/fields` — 更新字段 |
| WRITE | `POST /api/v1/modules/:id/schemas/:schemaId/migrate-v2` — 迁移V2 |
| WRITE | `POST /api/v1/modules/:id/schemas/:schemaId/activate` — 激活Schema |
| WRITE | `POST /api/v1/modules/:id/demos` — 创建Demo |
| WRITE | `POST /api/v1/modules/:id/versions` — 创建版本 |
| WRITE | `PUT /api/v1/modules/:id/versions/:vid` — 更新版本 |
| WRITE | `PUT /api/v1/modules/:id/default-version` — 设置默认版本 |
| WRITE | `POST /api/v1/modules/:id/versions/:vid/inherit-demos` — 继承Demo |
| WRITE | `POST /api/v1/modules/:id/versions/:vid/import-demos` — 导入Demo |
| ADMIN | `DELETE /api/v1/modules/:id` — 删除模块 |
| ADMIN | `DELETE /api/v1/modules/:id/versions/:vid` — 删除版本 |
| ADMIN | `POST /api/v1/modules/migrate-versions` — 批量迁移版本 |

**使用文件**: `router_module.go`

---

### 4. MODULE_DEMOS

**说明**: 模块 Demo 管理，控制独立Demo路由的CRUD

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/demos/:id` — Demo详情 |
| READ | `GET /api/v1/demos/:id/versions` — Demo版本列表 |
| READ | `GET /api/v1/demos/:id/compare` — Demo对比 |
| READ | `GET /api/v1/demo-versions/:versionId` — 版本详情 |
| WRITE | `PUT /api/v1/demos/:id` — 更新Demo |
| WRITE | `POST /api/v1/demos/:id/rollback` — 回滚Demo |
| ADMIN | `DELETE /api/v1/demos/:id` — 删除Demo |

**使用文件**: `router_demo.go`

---

### 5. SCHEMAS

**说明**: Schema 独立管理路由

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/schemas/:id` — Schema详情 |
| WRITE | `PUT /api/v1/schemas/:id` — 更新Schema |

**使用文件**: `router_schema.go`

---

### 6. TASK_LOGS

**说明**: 全局任务日志，控制跨工作空间的任务日志查看

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/tasks/:task_id/output/stream` — 任务输出流 |
| READ | `GET /api/v1/tasks/:task_id/logs` — 任务日志 |
| READ | `GET /api/v1/tasks/:task_id/logs/download` — 下载日志 |
| READ | `GET /api/v1/terraform/streams/stats` — 流统计 |

**使用文件**: `router_task.go`

---

### 7. RUN_TASKS

**说明**: Run Task 管理，控制 Run Task 配置和工作空间绑定

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/run-tasks` — Run Task 列表 |
| READ | `GET /api/v1/run-tasks/:id` — Run Task 详情 |
| READ | `GET /api/v1/run-tasks/:id/workspaces` — 关联工作空间 |
| WRITE | `POST /api/v1/run-tasks` — 创建 Run Task |
| WRITE | `PUT /api/v1/run-tasks/:id` — 更新 Run Task |
| WRITE | `POST /api/v1/run-tasks/:id/workspaces` — 绑定工作空间 |
| ADMIN | `DELETE /api/v1/run-tasks/:id` — 删除 Run Task |

**使用文件**: `router_run_task.go`

---

### 8. AGENT_POOLS

**说明**: Agent Pool 管理，控制 Pool 的 CRUD、Token 管理、工作空间授权、K8s 配置

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/agent-pools` — Pool 列表 |
| READ | `GET /api/v1/agent-pools/:pool_id` — Pool 详情 |
| READ | `GET /api/v1/agent-pools/:pool_id/allowed-workspaces` — 允许的工作空间 |
| READ | `GET /api/v1/agent-pools/:pool_id/tokens` — Token 列表 |
| READ | `GET /api/v1/agent-pools/:pool_id/k8s-config` — K8s 配置 |
| WRITE | `POST /api/v1/agent-pools` — 创建 Pool |
| WRITE | `PUT /api/v1/agent-pools/:pool_id` — 更新 Pool |
| WRITE | `POST /api/v1/agent-pools/:pool_id/allow-workspaces` — 授权工作空间 |
| WRITE | `DELETE /api/v1/agent-pools/:pool_id/allowed-workspaces/:workspace_id` — 撤销授权 |
| WRITE | `POST /api/v1/agent-pools/:pool_id/tokens` — 创建 Token |
| WRITE | `DELETE /api/v1/agent-pools/:pool_id/tokens/:token_name` — 撤销 Token |
| WRITE | `POST /api/v1/agent-pools/:pool_id/tokens/:token_name/rotate` — 轮换 Token |
| WRITE | `POST /api/v1/agent-pools/:pool_id/sync-deployment` — 同步部署 |
| WRITE | `POST /api/v1/agent-pools/:pool_id/one-time-unfreeze` — 一次性解冻 |
| WRITE | `PUT /api/v1/agent-pools/:pool_id/k8s-config` — 更新 K8s 配置 |
| ADMIN | `DELETE /api/v1/agent-pools/:pool_id` — 删除 Pool |

**使用文件**: `router_agent.go`

---

### 9. AI_ANALYSIS

**说明**: AI 分析功能，控制 AI 错误分析、表单生成、向量搜索等操作

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/ai/cmdb/vector-search` — CMDB 向量搜索 |
| WRITE 或 ADMIN | `POST /api/v1/ai/analyze-error` — AI 错误分析 |
| WRITE 或 ADMIN | `POST /api/v1/ai/form/generate` — 表单生成 |
| WRITE 或 ADMIN | `POST /api/v1/ai/form/generate-with-cmdb` — CMDB 表单生成 |
| WRITE 或 ADMIN | `POST /api/v1/ai/form/generate-with-cmdb-skill` — Skill 表单生成 |
| WRITE 或 ADMIN | `POST /api/v1/ai/form/generate-with-cmdb-skill-sse` — Skill SSE 表单生成 |
| ADMIN | `POST /api/v1/ai/skill/preview-prompt` — 预览 Skill Prompt |

> 注意: 写入类接口使用 `RequireAnyPermission`，WRITE 和 ADMIN 级别均可访问。

**使用文件**: `router_ai.go`

---

### 10. AI_CONFIGS

**说明**: AI 配置管理，控制 AI 模型/区域/优先级配置

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/global/settings/ai-configs` — 配置列表 |
| READ | `GET /api/v1/global/settings/ai-configs/:id` — 配置详情 |
| READ | `GET /api/v1/global/settings/ai-config/regions` — 可用区域 |
| READ | `GET /api/v1/global/settings/ai-config/models` — 可用模型 |
| WRITE | `POST /api/v1/global/settings/ai-configs` — 创建配置 |
| WRITE | `PUT /api/v1/global/settings/ai-configs/:id` — 更新配置 |
| WRITE | `PUT /api/v1/global/settings/ai-configs/priorities` — 批量更新优先级 |
| ADMIN | `DELETE /api/v1/global/settings/ai-configs/:id` — 删除配置 |
| ADMIN | `PUT /api/v1/global/settings/ai-configs/:id/set-default` — 设为默认 |

**使用文件**: `router_global.go`

---

### 11. TERRAFORM_VERSIONS

**说明**: Terraform 版本管理，控制全局 Terraform 版本的 CRUD 和默认版本设置

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/global/settings/terraform-versions` — 版本列表 |
| READ | `GET /api/v1/global/settings/terraform-versions/default` — 默认版本 |
| READ | `GET /api/v1/global/settings/terraform-versions/:id` — 版本详情 |
| WRITE | `POST /api/v1/global/settings/terraform-versions` — 创建版本 |
| WRITE | `PUT /api/v1/global/settings/terraform-versions/:id` — 更新版本 |
| ADMIN | `POST /api/v1/global/settings/terraform-versions/:id/set-default` — 设为默认 |
| ADMIN | `DELETE /api/v1/global/settings/terraform-versions/:id` — 删除版本 |

**使用文件**: `router_global.go`

---

### 12. SYSTEM_SETTINGS

**说明**: 系统级平台配置和 MFA 全局配置

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/global/settings/platform-config` — 平台配置 |
| READ | `GET /api/v1/global/settings/mfa` — MFA 全局配置 |
| ADMIN | `PUT /api/v1/global/settings/platform-config` — 更新平台配置 |
| ADMIN | `PUT /api/v1/global/settings/mfa` — 更新 MFA 配置 |

> 注意: `SYSTEM_SETTINGS` 在 `resource_type.go` 中未定义常量，但在路由中直接使用字符串。建议后续补充。

**使用文件**: `router_global.go`

---

### 13. USER_MANAGEMENT

**说明**: 用户管理，控制管理员对用户的 MFA 状态查看和重置

**组织级 (ORGANIZATION scope)**:

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/admin/users/:user_id/mfa/status` — 用户 MFA 状态 |
| ADMIN | `POST /api/v1/admin/users/:user_id/mfa/reset` — 重置用户 MFA |

**用户级 (USER scope)**:

| 最低级别 | 接口 |
|------|------|
| WRITE | `PUT /api/v1/user/change-password` — 用户自己改密码 |

**使用文件**: `router_global.go`, `router_user.go`

---

### 14. IAM_PERMISSIONS

**说明**: IAM 权限管理，控制权限授予、撤销和查询

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/iam/permissions/:scope_type/:scope_id` — 查询权限列表 |
| READ | `GET /api/v1/iam/permissions/definitions` — 权限定义列表 |
| READ | `GET /api/v1/iam/users/:id/permissions` — 用户权限 |
| READ | `GET /api/v1/iam/teams/:id/permissions` — 团队权限 |
| ADMIN | `POST /api/v1/iam/permissions/grant` — 授权 |
| ADMIN | `POST /api/v1/iam/permissions/batch-grant` — 批量授权 |
| ADMIN | `POST /api/v1/iam/permissions/grant-preset` — 预设授权 |
| ADMIN | `DELETE /api/v1/iam/permissions/:scope_type/:id` — 撤销权限 |

**使用文件**: `router_iam.go`

---

### 15. IAM_TEAMS

**说明**: IAM 团队管理，控制团队 CRUD、成员管理、Token 和角色

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/iam/teams` — 团队列表 |
| READ | `GET /api/v1/iam/teams/:id` — 团队详情 |
| READ | `GET /api/v1/iam/teams/:id/members` — 成员列表 |
| READ | `GET /api/v1/iam/teams/:id/tokens` — Token 列表 |
| READ | `GET /api/v1/iam/teams/:id/roles` — 角色列表 |
| WRITE | `POST /api/v1/iam/teams` — 创建团队 |
| WRITE | `POST /api/v1/iam/teams/:id/members` — 添加成员 |
| WRITE | `DELETE /api/v1/iam/teams/:id/members/:user_id` — 移除成员 |
| WRITE | `POST /api/v1/iam/teams/:id/tokens` — 创建 Token |
| WRITE | `DELETE /api/v1/iam/teams/:id/tokens/:token_id` — 撤销 Token |
| ADMIN | `DELETE /api/v1/iam/teams/:id` — 删除团队 |
| ADMIN | `POST /api/v1/iam/teams/:id/roles` — 分配角色 |
| ADMIN | `DELETE /api/v1/iam/teams/:id/roles/:assignment_id` — 撤销角色 |

**使用文件**: `router_iam.go`

---

### 16. IAM_ORGANIZATIONS

**说明**: IAM 组织管理

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/iam/organizations` — 组织列表 |
| READ | `GET /api/v1/iam/organizations/:id` — 组织详情 |
| WRITE | `PUT /api/v1/iam/organizations/:id` — 更新组织 |
| ADMIN | `POST /api/v1/iam/organizations` — 创建组织 |
| ADMIN | `DELETE /api/v1/iam/organizations/:id` — 删除组织 |

**使用文件**: `router_iam.go`

---

### 17. IAM_PROJECTS

**说明**: IAM 项目管理

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/iam/projects` — 项目列表 |
| READ | `GET /api/v1/iam/projects/:id` — 项目详情 |
| WRITE | `POST /api/v1/iam/projects` — 创建项目 |
| WRITE | `PUT /api/v1/iam/projects/:id` — 更新项目 |
| ADMIN | `DELETE /api/v1/iam/projects/:id` — 删除项目 |

**使用文件**: `router_iam.go`

---

### 18. IAM_APPLICATIONS

**说明**: IAM 应用管理，控制 OAuth/API 应用的 CRUD 和密钥管理

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/iam/applications` — 应用列表 |
| READ | `GET /api/v1/iam/applications/:id` — 应用详情 |
| WRITE | `POST /api/v1/iam/applications` — 创建应用 |
| WRITE | `PUT /api/v1/iam/applications/:id` — 更新应用 |
| ADMIN | `DELETE /api/v1/iam/applications/:id` — 删除应用 |
| ADMIN | `POST /api/v1/iam/applications/:id/regenerate-secret` — 重新生成密钥 |

**使用文件**: `router_iam.go`

---

### 19. IAM_AUDIT

**说明**: IAM 审计日志，控制权限变更历史和访问记录的查询

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/iam/audit/config` — 审计配置 |
| READ | `GET /api/v1/iam/audit/permission-history` — 权限变更历史 |
| READ | `GET /api/v1/iam/audit/access-history` — 访问历史 |
| READ | `GET /api/v1/iam/audit/denied-access` — 拒绝记录 |
| READ | `GET /api/v1/iam/audit/permission-changes-by-principal` — 按主体查权限变更 |
| READ | `GET /api/v1/iam/audit/permission-changes-by-performer` — 按操作者查权限变更 |
| ADMIN | `PUT /api/v1/iam/audit/config` — 更新审计配置 |

**使用文件**: `router_iam.go`

---

### 20. IAM_USERS

**说明**: IAM 用户管理，控制用户 CRUD、激活/停用、角色分配

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/iam/users/stats` — 用户统计 |
| READ | `GET /api/v1/iam/users` — 用户列表 |
| READ | `GET /api/v1/iam/users/:id` — 用户详情 |
| READ | `GET /api/v1/iam/users/:id/roles` — 用户角色列表 |
| WRITE | `POST /api/v1/iam/users` — 创建用户 |
| WRITE | `PUT /api/v1/iam/users/:id` — 更新用户 |
| ADMIN | `POST /api/v1/iam/users/:id/roles` — 分配角色 |
| ADMIN | `DELETE /api/v1/iam/users/:id/roles/:assignment_id` — 撤销角色 |
| ADMIN | `POST /api/v1/iam/users/:id/activate` — 激活用户 |
| ADMIN | `POST /api/v1/iam/users/:id/deactivate` — 停用用户 |
| ADMIN | `DELETE /api/v1/iam/users/:id` — 删除用户 |

**使用文件**: `router_iam.go`

---

### 21. IAM_ROLES

**说明**: IAM 角色管理，控制角色 CRUD、策略管理和克隆

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/iam/roles` — 角色列表 |
| READ | `GET /api/v1/iam/roles/:id` — 角色详情 |
| WRITE | `POST /api/v1/iam/roles` — 创建角色 |
| WRITE | `PUT /api/v1/iam/roles/:id` — 更新角色 |
| WRITE | `POST /api/v1/iam/roles/:id/policies` — 添加策略 |
| WRITE | `DELETE /api/v1/iam/roles/:id/policies/:policy_id` — 移除策略 |
| WRITE | `POST /api/v1/iam/roles/:id/clone` — 克隆角色 |
| ADMIN | `DELETE /api/v1/iam/roles/:id` — 删除角色 |

**使用文件**: `router_iam.go`

---

### 22. APPLICATION_REGISTRATION

**说明**: 应用注册（在 `resource_type.go` 中定义，当前路由中未直接使用）

> 预留资源类型，暂无对应 API 接口。

---

### 23. PROJECTS

**说明**: 项目管理（在 `resource_type.go` 中定义为 `ResourceTypeAllProjects`，当前路由中未直接使用此资源类型做权限检查，项目列表接口复用 `WORKSPACES` 权限）

> 预留资源类型。项目相关接口当前使用 `WORKSPACES/ORG/READ` 或 `IAM_PROJECTS`。

---

### 24. AGENTS

**说明**: Agent 管理（在 `resource_type.go` 中定义，当前路由中未直接使用，Agent API 使用 PoolToken 认证而非 IAM）

> 预留资源类型。Agent API 使用 PoolToken 认证体系，不走 IAM 权限。

---

## 二、工作空间级资源 (WORKSPACE Scope)

### 25. WORKSPACE_MANAGEMENT

**说明**: 工作空间综合管理权限，作为大多数工作空间操作的 OR 备选权限。几乎所有 workspace 子操作都允许 `WORKSPACE_MANAGEMENT` 持有者访问。

| 最低级别 | 典型用途 |
|------|---------|
| READ | 工作空间详情、任务列表、状态查看、变量查看、资源查看、通知查看、快照查看、输出查看 |
| WRITE | 工作空间设置更新、任务执行、状态上传/回滚、变量创建更新、资源导入/移除、通知管理、快照创建恢复、Run Trigger 管理、Drift 检测 |
| ADMIN | 工作空间删除、锁定/解锁、强制取消任务、变量删除、资源销毁、快照删除 |

> 这是一个"超级权限"，几乎所有 workspace 级别的接口都在 `RequireAnyPermission` 中将 `WORKSPACE_MANAGEMENT` 作为备选项。

**使用文件**: `router_workspace.go`

---

### 26. WORKSPACE_EXECUTION

**说明**: 工作空间执行权限，控制 Plan/Apply/Destroy 等任务操作

| 最低级别 | 典型接口 |
|------|---------|
| READ | `GET /api/v1/workspaces/:id/tasks` — 任务列表 |
| READ | `GET /api/v1/workspaces/:id/tasks/:tid` — 任务详情 |
| READ | `GET /api/v1/workspaces/:id/tasks/:tid/logs` — 任务日志 |
| READ | `GET /api/v1/workspaces/:id/tasks/:tid/plan` — Plan 数据 |
| WRITE | `POST /api/v1/workspaces/:id/plan` — 触发 Plan |
| WRITE | `POST /api/v1/workspaces/:id/apply` — 触发 Apply |
| WRITE | `POST /api/v1/workspaces/:id/tasks/:tid/confirm` — 确认执行 |
| WRITE | `POST /api/v1/workspaces/:id/tasks/:tid/discard` — 丢弃 Plan |
| ADMIN | `POST /api/v1/workspaces/:id/destroy` — 销毁资源 |
| ADMIN | `POST /api/v1/workspaces/:id/tasks/:tid/cancel` — 强制取消 |
| ADMIN | `POST /api/v1/workspaces/:id/lock` — 锁定 |
| ADMIN | `POST /api/v1/workspaces/:id/unlock` — 解锁 |

> 注意: 所有这些接口同时也接受 `WORKSPACE_MANAGEMENT` 对应级别的权限。

**使用文件**: `router_workspace.go`

---

### 27. TASK_DATA_ACCESS

**说明**: 任务数据访问权限，控制工作空间内任务详情和日志的查看

| 最低级别 | 典型接口 |
|------|---------|
| READ | `GET /api/v1/workspaces/:id/tasks` — 任务列表 |
| READ | `GET /api/v1/workspaces/:id/tasks/:tid` — 任务详情 |
| READ | `GET /api/v1/workspaces/:id/tasks/:tid/logs` — 日志 |
| READ | `GET /api/v1/workspaces/:id/tasks/:tid/logs/download` — 下载日志 |
| READ | `GET /api/v1/workspaces/:id/tasks/:tid/plan` — Plan 数据 |
| READ | `GET /api/v1/workspaces/:id/tasks/:tid/plan-json` — Plan JSON |
| READ | `GET /api/v1/workspaces/:id/tasks/:tid/changes` — 变更详情 |

> 与 `WORKSPACE_EXECUTION/READ` 和 `WORKSPACE_MANAGEMENT/READ` 形成 OR 关系。

**使用文件**: `router_workspace.go`

---

### 28. WORKSPACE_STATE

**说明**: 工作空间状态管理权限，控制 Terraform State 的查看、上传、回滚和删除

| 最低级别 | 典型接口 |
|------|---------|
| READ | `GET /api/v1/workspaces/:id/current-state-version` — 当前状态版本 |
| READ | `GET /api/v1/workspaces/:id/state-versions` — 版本列表 |
| READ | `GET /api/v1/workspaces/:id/state-versions/:vid` — 版本详情 |
| READ | `GET /api/v1/workspaces/:id/state-versions/:vid/outputs` — 输出 |
| READ | `GET /api/v1/workspaces/:id/state-versions/:vid/resources` — 资源 |
| READ | `GET /api/v1/workspaces/:id/state-versions/compare` — 版本对比 |
| READ | `GET /api/v1/workspaces/:id/state-download` — 下载状态文件 |
| READ | `GET /api/v1/workspaces/:id/state-versions/:vid/download` — 下载指定版本 |
| WRITE | `POST /api/v1/workspaces/:id/state-versions` — 上传状态 |
| WRITE | `POST /api/v1/workspaces/:id/state-versions/:vid/rollback` — 回滚 |
| WRITE | `PUT /api/v1/workspaces/:id/state-versions/:vid/mark-safe` — 标记安全 |
| WRITE | `PUT /api/v1/workspaces/:id/state-versions/:vid/mark-tainted` — 标记污染 |
| ADMIN | `DELETE /api/v1/workspaces/:id/state-versions/:vid` — 删除版本 |

**使用文件**: `router_workspace.go`

---

### 29. WORKSPACE_STATE_SENSITIVE

**说明**: 状态文件敏感内容查看权限，控制包含 Secrets 的状态原始内容查看

| 最低级别 | 接口 |
|------|------|
| READ | `GET /api/v1/workspaces/:id/state-versions/:vid/content` — 状态原始内容（含敏感值） |

> 这是一个独立于 `WORKSPACE_STATE` 的权限，专门控制包含密文的状态内容。普通 `WORKSPACE_STATE/READ` 可查看元数据，但查看原始内容需要此权限。

**使用文件**: `router_workspace.go`

---

### 30. WORKSPACE_VARIABLES

**说明**: 工作空间变量管理权限

| 最低级别 | 典型接口 |
|------|---------|
| READ | `GET /api/v1/workspaces/:id/variables` — 变量列表 |
| READ | `GET /api/v1/workspaces/:id/variables/:vid` — 变量详情 |
| READ | `GET /api/v1/workspaces/:id/variables/sets` — 变量集列表 |
| READ | `GET /api/v1/workspaces/:id/variables/sets/:setId` — 变量集详情 |
| WRITE | `POST /api/v1/workspaces/:id/variables` — 创建变量 |
| WRITE | `PUT /api/v1/workspaces/:id/variables/:vid` — 更新变量 |
| ADMIN | `DELETE /api/v1/workspaces/:id/variables/:vid` — 删除变量 |

**使用文件**: `router_workspace.go`

---

### 31. WORKSPACE_RESOURCES

**说明**: 工作空间资源管理权限，控制 Terraform 托管资源的查看、导入和管理

| 最低级别 | 典型接口 |
|------|---------|
| READ | `GET /api/v1/workspaces/:id/resources` — 资源列表 |
| READ | `GET /api/v1/workspaces/:id/resources/:rid` — 资源详情 |
| READ | `GET /api/v1/workspaces/:id/resources/tree` — 资源树 |
| READ | `GET /api/v1/workspaces/:id/resources/summary` — 资源摘要 |
| READ | `GET /api/v1/workspaces/:id/resources/search` — 搜索 |
| READ | `GET /api/v1/workspaces/:id/resources/:rid/history` — 变更历史 |
| READ | `GET /api/v1/workspaces/:id/resources/:rid/dependencies` — 依赖关系 |
| READ | `GET /api/v1/workspaces/:id/resources/dependency-graph` — 依赖图 |
| READ | `GET /api/v1/workspaces/:id/resources/types` — 资源类型列表 |
| READ | `GET /api/v1/workspaces/:id/resources/export` — 导出资源 |
| WRITE | `POST /api/v1/workspaces/:id/resources/import` — 导入资源 |
| WRITE | `POST /api/v1/workspaces/:id/resources/:rid/taint` — 标记污染 |
| WRITE | `POST /api/v1/workspaces/:id/resources/:rid/untaint` — 取消污染 |
| WRITE | `POST /api/v1/workspaces/:id/resources/:rid/move` — 移动资源 |
| WRITE | `POST /api/v1/workspaces/:id/resources/:rid/remove` — 移除资源 |
| WRITE | `POST /api/v1/workspaces/:id/resources/batch-import` — 批量导入 |
| WRITE | `POST /api/v1/workspaces/:id/resources/rename` — 重命名资源 |
| WRITE | `POST /api/v1/workspaces/:id/resources/replace` — 替换资源 |
| WRITE | `POST /api/v1/workspaces/:id/resources/batch-taint` — 批量污染 |
| WRITE | `POST /api/v1/workspaces/:id/resources/batch-untaint` — 批量取消 |
| ADMIN | `POST /api/v1/workspaces/:id/resources/:rid/destroy` — 销毁资源 |
| ADMIN | `POST /api/v1/workspaces/:id/resources/batch-destroy` — 批量销毁 |

**使用文件**: `router_workspace.go`

---

## 三、项目级资源 (PROJECT Scope)

### 32. PROJECT_SETTINGS

**说明**: 项目设置（在 `resource_type.go` 中定义，当前路由中未直接使用）

> 预留资源类型，暂无对应 API 接口。

---

### 33. PROJECT_TEAM_MANAGEMENT

**说明**: 项目团队管理（在 `resource_type.go` 中定义，当前路由中未直接使用）

> 预留资源类型，暂无对应 API 接口。

---

### 34. PROJECT_WORKSPACES

**说明**: 项目工作空间管理（在 `resource_type.go` 中定义，当前路由中未直接使用）

> 预留资源类型，暂无对应 API 接口。

---

## 四、仅在路由中使用但未在 resource_type.go 定义的资源类型

### 35. cmdb (小写)

**说明**: CMDB 资源索引，控制同步操作和外部数据源管理

| 最低级别 | 接口 |
|------|------|
| ADMIN | `POST /api/v1/cmdb/workspaces/:workspace_id/sync` — 同步工作空间 |
| ADMIN | `POST /api/v1/cmdb/sync-all` — 同步全部 |
| ADMIN | `GET /api/v1/cmdb/external-sources` — 外部数据源列表 |
| ADMIN | `POST /api/v1/cmdb/external-sources` — 创建数据源 |
| ADMIN | `GET /api/v1/cmdb/external-sources/:source_id` — 数据源详情 |
| ADMIN | `PUT /api/v1/cmdb/external-sources/:source_id` — 更新数据源 |
| ADMIN | `DELETE /api/v1/cmdb/external-sources/:source_id` — 删除数据源 |
| ADMIN | `POST /api/v1/cmdb/external-sources/:source_id/sync` — 触发同步 |
| ADMIN | `POST /api/v1/cmdb/external-sources/:source_id/test` — 测试连接 |
| (无) | `GET /api/v1/cmdb/search` — 搜索（无权限检查） |
| (无) | `GET /api/v1/cmdb/suggestions` — 搜索建议（无权限检查） |
| (无) | `GET /api/v1/cmdb/stats` — 统计（无权限检查） |
| (无) | `GET /api/v1/cmdb/resource-types` — 类型列表（无权限检查） |
| (无) | `GET /api/v1/cmdb/workspace-counts` — 计数（无权限检查） |
| (无) | `GET /api/v1/cmdb/workspaces/:workspace_id/tree` — 资源树（无权限检查） |
| (无) | `GET /api/v1/cmdb/workspaces/:workspace_id/resources` — 详情（无权限检查） |

> **问题**: 1) 使用小写 `cmdb` 而非大写 `CMDB`，风格不一致；2) 只读接口缺少权限检查；3) 缺少 admin 绕过逻辑。

**使用文件**: `router_cmdb.go`

---

### 36. WORKSPACE_STATE_SENSITIVE

**说明**: 已在第 29 项说明。在 `resource_type.go` 中未定义常量，但在 `permission_definitions` 数据库表中有注册。

---

### 37. RUN_TASKS

**说明**: 已在第 7 项说明。在 `resource_type.go` 中未定义常量，但在 `permission_definitions` 数据库表中有注册。

---

## 五、无 IAM 权限的资源类型（待修复）

以下模块当前**没有 IAM 权限检查**，需要新增资源类型：

| 待新增资源类型 | 模块 | 接口数 | 修复任务文件 |
|---------------|------|--------|-------------|
| MANIFESTS | Manifest 可视化编排 | 20 | 14-manifest.md |
| SECRETS | 密文管理 | 5 | 06-secrets.md |
| NOTIFICATIONS | 通知管理 | 7 | 22-notifications.md |
| SKILLS | Admin Skill 管理 | 20 | 18-admin-skills-embedding.md |
| EMBEDDING_MANAGEMENT | Admin Embedding/Cache | 5 | 18-admin-skills-embedding.md |
| SSO_IDENTITY | SSO 身份管理 | 7 | 02-sso.md |
| SSO_ADMIN | SSO 管理端点 | 5 | 02-sso.md |

---

## 六、权限级别汇总矩阵

| 资源类型 | READ | WRITE | ADMIN | Scope |
|----------|:----:|:-----:|:-----:|-------|
| ORGANIZATION | ✅ | - | - | ORG |
| WORKSPACES | ✅ | ✅ | - | ORG+WS |
| MODULES | ✅ | ✅ | ✅ | ORG |
| MODULE_DEMOS | ✅ | ✅ | ✅ | ORG |
| SCHEMAS | ✅ | ✅ | - | ORG |
| TASK_LOGS | ✅ | - | - | ORG |
| RUN_TASKS | ✅ | ✅ | ✅ | ORG |
| AGENT_POOLS | ✅ | ✅ | ✅ | ORG |
| AI_ANALYSIS | ✅ | ✅ | ✅ | ORG |
| AI_CONFIGS | ✅ | ✅ | ✅ | ORG |
| TERRAFORM_VERSIONS | ✅ | ✅ | ✅ | ORG |
| SYSTEM_SETTINGS | ✅ | - | ✅ | ORG |
| USER_MANAGEMENT | ✅ | ✅ | ✅ | ORG+USER |
| IAM_PERMISSIONS | ✅ | - | ✅ | ORG |
| IAM_TEAMS | ✅ | ✅ | ✅ | ORG |
| IAM_ORGANIZATIONS | ✅ | ✅ | ✅ | ORG |
| IAM_PROJECTS | ✅ | ✅ | ✅ | ORG |
| IAM_APPLICATIONS | ✅ | ✅ | ✅ | ORG |
| IAM_AUDIT | ✅ | - | ✅ | ORG |
| IAM_USERS | ✅ | ✅ | ✅ | ORG |
| IAM_ROLES | ✅ | ✅ | ✅ | ORG |
| cmdb | - | - | ✅ | ORG |
| WORKSPACE_MANAGEMENT | ✅ | ✅ | ✅ | WS |
| WORKSPACE_EXECUTION | ✅ | ✅ | ✅ | WS |
| TASK_DATA_ACCESS | ✅ | - | - | WS |
| WORKSPACE_STATE | ✅ | ✅ | ✅ | WS |
| WORKSPACE_STATE_SENSITIVE | ✅ | - | - | WS |
| WORKSPACE_VARIABLES | ✅ | ✅ | ✅ | WS |
| WORKSPACE_RESOURCES | ✅ | ✅ | ✅ | WS |
