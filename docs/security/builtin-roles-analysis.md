# 内置角色权限分析

> 数据来源: `manifests/db/init_seed_data.sql`
> 表: `iam_roles`, `iam_role_policies`, `permission_definitions`

## 一、角色清单

| ID | name | display_name | is_system | 说明 |
|----|------|-------------|-----------|------|
| 1 | admin | 超级管理员 | 是 | 拥有系统所有权限 |
| 2 | org_admin | 组织管理员 | 是 | 组织级管理员 |
| 3 | project_admin | 项目管理员 | 是 | 项目级管理员 |
| 4 | workspace_admin | 工作空间管理员 | 是 | 工作空间级管理员 |
| 5 | developer | 开发者 | 是 | 可执行任务的开发者 |
| 6 | viewer | 查看者 | 是 | 只读 |
| 26 | iam_admin | IAM管理员 | 是 | IAM 全部管理权限 |
| 27 | iam_write | IAM编辑者 | 是 | IAM 读写 |
| 28 | iam_read | IAM查看者 | 是 | IAM 只读 |
| 30 | user | 普通用户 | 是 | 默认角色 |

> 角色 25(运维) 和 29(ops) 为用户自建角色，不在分析范围内。

---

## 二、各角色的策略明细

### Role 1: admin（超级管理员）

> **注意**: admin 角色在路由层通过 `if role == "admin" { bypass }` 硬编码绕过 IAM，所以 DB 中的策略仅为形式存在。实际上 admin 可访问一切接口。

DB 中注册的策略（全部 ADMIN 级别，覆盖 ORG/PROJECT/WS 三个 scope）：

| 资源类型 | ORG | PROJECT | WS |
|----------|:---:|:-------:|:--:|
| APPLICATION_REGISTRATION | ADMIN | ADMIN | ADMIN |
| USER_MANAGEMENT | ADMIN | ADMIN | ADMIN |
| PROJECT_SETTINGS | ADMIN | ADMIN | ADMIN |
| PROJECT_TEAM_MANAGEMENT | ADMIN | ADMIN | ADMIN |
| PROJECT_WORKSPACES | ADMIN | ADMIN | ADMIN |
| TASK_DATA_ACCESS | ADMIN | ADMIN | ADMIN |
| WORKSPACE_EXECUTION | ADMIN | ADMIN | ADMIN |
| WORKSPACE_STATE | ADMIN | ADMIN | ADMIN |
| WORKSPACE_VARIABLES | ADMIN | ADMIN | ADMIN |
| ORGANIZATION | ADMIN | ADMIN | ADMIN |
| PROJECTS | ADMIN | ADMIN | ADMIN |
| WORKSPACES | ADMIN | ADMIN | ADMIN |
| WORKSPACE_RESOURCES | ADMIN | ADMIN | ADMIN |
| MODULES | ADMIN | ADMIN | ADMIN |
| WORKSPACE_MANAGEMENT | ADMIN | ADMIN | ADMIN |
| ORGANIZATION_SETTINGS | ADMIN | ADMIN | ADMIN |
| ALL_PROJECTS | ADMIN | ADMIN | ADMIN |
| IAM_ORGANIZATIONS | ADMIN | - | - |
| cmdb | ADMIN | - | - |

**缺失**: MODULE_DEMOS, SCHEMAS, TASK_LOGS, AGENTS, AGENT_POOLS, IAM_PERMISSIONS, IAM_TEAMS, IAM_PROJECTS, IAM_APPLICATIONS, IAM_AUDIT, IAM_USERS, IAM_ROLES, TERRAFORM_VERSIONS, AI_CONFIGS, AI_ANALYSIS, RUN_TASKS, WORKSPACE_STATE_SENSITIVE
（不影响功能，因为代码层直接绕过）

---

### Role 2: org_admin（组织管理员）

| 资源类型 | 级别 | Scope |
|----------|------|-------|
| ORGANIZATION | ADMIN | ORG |
| PROJECTS | ADMIN | ORG |
| WORKSPACES | ADMIN | ORG |
| MODULES | ADMIN | ORG |

**仅 4 项权限。**

---

### Role 3: project_admin（项目管理员）

| 资源类型 | 级别 | Scope |
|----------|------|-------|
| WORKSPACES | ADMIN | PROJECT |

**仅 1 项权限。**

---

### Role 4: workspace_admin（工作空间管理员）

| 资源类型 | 级别 | Scope |
|----------|------|-------|
| TASK_DATA_ACCESS | ADMIN | WS |
| WORKSPACE_EXECUTION | ADMIN | WS |
| WORKSPACE_STATE | ADMIN | WS |
| WORKSPACE_VARIABLES | ADMIN | WS |
| WORKSPACE_RESOURCES | ADMIN | WS |
| WORKSPACE_MANAGEMENT | ADMIN | WS |

**6 项权限，全部 WS 级 ADMIN。**

---

### Role 5: developer（开发者）

| 资源类型 | 级别 | Scope |
|----------|------|-------|
| WORKSPACE_EXECUTION | WRITE | WS |
| WORKSPACE_VARIABLES | WRITE | WS |
| WORKSPACE_RESOURCES | WRITE | WS |
| WORKSPACE_MANAGEMENT | WRITE | WS |
| TASK_DATA_ACCESS | READ | WS |
| WORKSPACE_STATE | READ | WS |

**6 项权限，全部 WS 级。无任何 ORG 级权限。**

---

### Role 6: viewer（查看者）

全部 READ 级别，覆盖 ORG/PROJECT/WS 三个 scope：

| 资源类型 |
|----------|
| APPLICATION_REGISTRATION |
| USER_MANAGEMENT |
| PROJECT_SETTINGS |
| PROJECT_TEAM_MANAGEMENT |
| PROJECT_WORKSPACES |
| TASK_DATA_ACCESS |
| WORKSPACE_EXECUTION |
| WORKSPACE_STATE |
| WORKSPACE_VARIABLES |
| ORGANIZATION |
| PROJECTS |
| WORKSPACES |
| WORKSPACE_RESOURCES |
| MODULES |
| WORKSPACE_MANAGEMENT |
| ORGANIZATION_SETTINGS |
| ALL_PROJECTS |

**17 种资源 × 3 scope = 51 条策略，全部 READ。**

---

### Role 26: iam_admin（IAM管理员）

| 资源类型 | 级别 | Scope |
|----------|------|-------|
| IAM_PERMISSIONS | ADMIN | ORG |
| IAM_TEAMS | ADMIN | ORG |
| IAM_ORGANIZATIONS | ADMIN | ORG |
| IAM_PROJECTS | ADMIN | ORG |
| IAM_APPLICATIONS | ADMIN | ORG |
| IAM_AUDIT | ADMIN | ORG |
| IAM_USERS | ADMIN | ORG |
| IAM_ROLES | ADMIN | ORG |

**8 项 IAM 权限，全部 ADMIN。**

---

### Role 27: iam_write（IAM编辑者）

| 资源类型 | 级别 | Scope |
|----------|------|-------|
| IAM_PERMISSIONS | WRITE | ORG |
| IAM_TEAMS | WRITE | ORG |
| IAM_ORGANIZATIONS | WRITE | ORG |
| IAM_PROJECTS | WRITE | ORG |
| IAM_APPLICATIONS | WRITE | ORG |
| IAM_USERS | WRITE | ORG |
| IAM_ROLES | WRITE | ORG |
| IAM_AUDIT | **READ** | ORG |

> 注意: IAM_AUDIT 只有 READ，不能修改审计配置。合理。

---

### Role 28: iam_read（IAM查看者）

| 资源类型 | 级别 | Scope |
|----------|------|-------|
| IAM_PERMISSIONS | READ | ORG |
| IAM_TEAMS | READ | ORG |
| IAM_ORGANIZATIONS | READ | ORG |
| IAM_PROJECTS | READ | ORG |
| IAM_APPLICATIONS | READ | ORG |
| IAM_AUDIT | READ | ORG |
| IAM_USERS | READ | ORG |
| IAM_ROLES | READ | ORG |

---

### Role 30: user（普通用户）

**无任何策略。** 这是 2026-02 新增的角色，DB 中没有 role_policies 记录。

---

## 三、问题汇总

### 问题 1: 新增资源类型未补充到现有角色

以下资源类型在路由中已使用，但**没有被任何系统角色**的策略覆盖：

| 资源类型 | 路由使用 | 角色覆盖 |
|----------|---------|---------|
| MODULE_DEMOS | router_demo.go | ❌ 无 |
| SCHEMAS | router_schema.go | ❌ 无 |
| TASK_LOGS | router_task.go | ❌ 无 |
| AGENTS | (预留) | ❌ 无 |
| AGENT_POOLS | router_agent.go | ❌ 无 |
| AI_ANALYSIS | router_ai.go | ❌ 无 |
| AI_CONFIGS | router_global.go | ❌ 无 |
| TERRAFORM_VERSIONS | router_global.go | ❌ 无 |
| SYSTEM_SETTINGS | router_global.go | ❌ 无 |
| RUN_TASKS | router_run_task.go | ❌ 无 |
| WORKSPACE_STATE_SENSITIVE | router_workspace.go | ❌ 无 |

**影响**: 非 admin 用户即使被赋予 org_admin/developer/viewer 角色，也无法访问这些模块。只有 admin 可以（因为代码层硬编码绕过）。

### 问题 2: org_admin 权限严重不足

当前只有 4 项（ORGANIZATION, PROJECTS, WORKSPACES, MODULES），缺失：

| 应有权限 | 说明 |
|----------|------|
| MODULE_DEMOS | 管理 Demo |
| SCHEMAS | 管理 Schema |
| TASK_LOGS | 查看全局任务日志 |
| AGENT_POOLS | 管理 Agent Pool |
| AI_ANALYSIS | 使用 AI 分析 |
| AI_CONFIGS | 管理 AI 配置 |
| TERRAFORM_VERSIONS | 管理 TF 版本 |
| SYSTEM_SETTINGS | 平台配置 |
| RUN_TASKS | 管理 Run Task |
| USER_MANAGEMENT | 管理用户（当前只在 viewer 有 READ） |
| APPLICATION_REGISTRATION | 应用管理 |

### 问题 3: project_admin 权限严重不足

仅 1 项 `WORKSPACES/PROJECT/ADMIN`。应至少包含：
- PROJECT_SETTINGS, PROJECT_TEAM_MANAGEMENT, PROJECT_WORKSPACES 的 ADMIN
- WS 级权限（WORKSPACE_MANAGEMENT, WORKSPACE_EXECUTION 等）的 PROJECT scope

### 问题 4: developer 无 ORG 级权限

developer 无法：
- 查看模块列表（MODULES/ORG/READ）
- 查看 Task 日志（TASK_LOGS/ORG/READ）
- 使用 AI 分析（AI_ANALYSIS/ORG/WRITE）
- 查看全局 Dashboard（ORGANIZATION/ORG/READ）

### 问题 5: viewer 缺少新增资源的 READ

viewer 17 种资源的 READ 是早期定义的，后来新增的 11 种资源类型（见问题 1）完全缺失。

### 问题 6: user 角色为空壳

无任何策略，实际等同于被拒绝所有操作（除 admin 绕过的接口外）。

---

## 四、建议修复方案

### 4.1 修复分层

修复分三层，按顺序执行：

| 层级 | 改什么 | 文件 |
|------|--------|------|
| 第一层 | `permission_definitions` 表补注册缺失的资源类型 | 迁移 SQL |
| 第二层 | `resource_type.go` 补常量 + IsValid/GetScopeLevel 联动 | Go 代码 |
| 第三层 | `iam_role_policies` 表补齐各角色的策略行 | 迁移 SQL |

---

### 4.2 第一层：permission_definitions 补注册

路由中使用但 DB 未注册的资源类型：

| 资源类型 | 当前状态 | 需要的动作 |
|----------|---------|-----------|
| SYSTEM_SETTINGS | DB 未注册，`resource_type.go` 也未定义常量 | 新增 |
| WORKSPACE_STATE_SENSITIVE | ✅ DB 已注册 (`wspm-workspace-state-sensitive`) | 无 |
| RUN_TASKS | ✅ DB 已注册 (`orgpm-rt-create-8k4m`) | 无 |
| cmdb | ✅ DB 已注册（小写命名不规范，属 26-cmdb.md 修复范畴） | 暂不动 |

需执行的 SQL：

```sql
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES ('orgpm-system-settings', 'SYSTEM_SETTINGS', 'SYSTEM_SETTINGS', 'ORGANIZATION',
        '系统设置', '管理平台配置和MFA全局配置', true, NOW())
ON CONFLICT (id) DO NOTHING;
```

已注册的 permission_id 映射（后续 role_policies 引用）：

```
orgpm-000000000001       → APPLICATION_REGISTRATION
orgpm-000000000002       → ORGANIZATION
orgpm-000000000003       → USER_MANAGEMENT
orgpm-000000000004       → PROJECTS
orgpm-000000000023       → WORKSPACES
orgpm-000000000025       → MODULES
orgpm-000000000028       → ORGANIZATION_SETTINGS
orgpm-000000000030       → ALL_PROJECTS
pjpm-000000000005        → PROJECT_SETTINGS
pjpm-000000000006        → PROJECT_TEAM_MANAGEMENT
pjpm-000000000007        → PROJECT_WORKSPACES
wspm-000000000008        → TASK_DATA_ACCESS
wspm-000000000009        → WORKSPACE_EXECUTION
wspm-000000000010        → WORKSPACE_STATE
wspm-000000000011        → WORKSPACE_VARIABLES
wspm-000000000024        → WORKSPACE_RESOURCES
wspm-000000000026        → WORKSPACE_MANAGEMENT
orgpm-module-demos       → MODULE_DEMOS
orgpm-schemas            → SCHEMAS
orgpm-task-logs          → TASK_LOGS
orgpm-agents             → AGENTS
orgpm-agent-pools        → AGENT_POOLS
orgpm-iam-permissions    → IAM_PERMISSIONS
orgpm-iam-teams          → IAM_TEAMS
orgpm-iam-organizations  → IAM_ORGANIZATIONS
orgpm-iam-projects       → IAM_PROJECTS
orgpm-iam-applications   → IAM_APPLICATIONS
orgpm-iam-audit          → IAM_AUDIT
orgpm-iam-users          → IAM_USERS
orgpm-iam-roles          → IAM_ROLES
orgpm-terraform-versions → TERRAFORM_VERSIONS
orgpm-ai-configs         → AI_CONFIGS
orgpm-ai-analysis        → AI_ANALYSIS
orgpm-rt-create-8k4m     → RUN_TASKS
orgpm-system-settings    → SYSTEM_SETTINGS (新增)
wspm-workspace-state-sensitive → WORKSPACE_STATE_SENSITIVE
```

---

### 4.3 第二层：resource_type.go 补常量

路由中直接写字符串但 `resource_type.go` 没有对应常量的：

| 路由中的字符串 | 需新增常量 | Scope |
|---------------|-----------|-------|
| `"SYSTEM_SETTINGS"` | `ResourceTypeSystemSettings` | ORGANIZATION |
| `"RUN_TASKS"` | `ResourceTypeRunTasks` | ORGANIZATION |
| `"WORKSPACE_STATE_SENSITIVE"` | `ResourceTypeWorkspaceStateSensitive` | WORKSPACE |

同时更新 `IsValid()` 和 `GetScopeLevel()` 的 switch 分支，否则 `ParseResourceType()` 会返回 `invalid resource type` 错误。

---

### 4.4 第三层：iam_role_policies 补齐各角色策略

#### Role 2: org_admin — 补齐为"除 IAM 管理外的组织全能管理员"

> 定位: 组织内所有业务资源的最高管理者，但不管 IAM 体系本身。

现有 4 项，补充后完整策略：

| 资源类型 | 级别 | Scope | 状态 |
|----------|------|-------|------|
| ORGANIZATION | ADMIN | ORG | ✅ 已有 |
| PROJECTS | ADMIN | ORG | ✅ 已有 |
| WORKSPACES | ADMIN | ORG | ✅ 已有 |
| MODULES | ADMIN | ORG | ✅ 已有 |
| MODULE_DEMOS | ADMIN | ORG | **新增** |
| SCHEMAS | ADMIN | ORG | **新增** |
| TASK_LOGS | ADMIN | ORG | **新增** |
| AGENT_POOLS | ADMIN | ORG | **新增** |
| AI_CONFIGS | ADMIN | ORG | **新增** |
| TERRAFORM_VERSIONS | ADMIN | ORG | **新增** |
| RUN_TASKS | ADMIN | ORG | **新增** |
| SYSTEM_SETTINGS | ADMIN | ORG | **新增** (待确认) |
| USER_MANAGEMENT | ADMIN | ORG | **新增** |
| APPLICATION_REGISTRATION | ADMIN | ORG | **新增** |
| ORGANIZATION_SETTINGS | ADMIN | ORG | **新增** |
| ALL_PROJECTS | ADMIN | ORG | **新增** |
| AI_ANALYSIS | WRITE | ORG | **新增** (使用AI，不需ADMIN) |

> **待确认**: SYSTEM_SETTINGS/ADMIN 是否给 org_admin？该权限控制平台配置和 MFA 全局配置，如果只想让 admin 管理则不给。

---

#### Role 3: project_admin — 补齐为"项目内全能管理员"

> 定位: 项目范围内的完全管理权限，包括项目设置、团队和项目内所有工作空间。

现有 1 项，补充后完整策略：

| 资源类型 | 级别 | Scope | 状态 |
|----------|------|-------|------|
| WORKSPACES | ADMIN | PROJECT | ✅ 已有 |
| PROJECT_SETTINGS | ADMIN | PROJECT | **新增** |
| PROJECT_TEAM_MANAGEMENT | ADMIN | PROJECT | **新增** |
| PROJECT_WORKSPACES | ADMIN | PROJECT | **新增** |
| TASK_DATA_ACCESS | ADMIN | PROJECT | **新增** |
| WORKSPACE_EXECUTION | ADMIN | PROJECT | **新增** |
| WORKSPACE_STATE | ADMIN | PROJECT | **新增** |
| WORKSPACE_VARIABLES | ADMIN | PROJECT | **新增** |
| WORKSPACE_RESOURCES | ADMIN | PROJECT | **新增** |
| WORKSPACE_MANAGEMENT | ADMIN | PROJECT | **新增** |
| WORKSPACE_STATE_SENSITIVE | READ | PROJECT | **新增** |

---

#### Role 4: workspace_admin — 补敏感状态权限

> 定位: 工作空间完全管理，包括查看敏感状态内容。

现有 6 项，补充 1 项：

| 资源类型 | 级别 | Scope | 状态 |
|----------|------|-------|------|
| TASK_DATA_ACCESS | ADMIN | WS | ✅ 已有 |
| WORKSPACE_EXECUTION | ADMIN | WS | ✅ 已有 |
| WORKSPACE_STATE | ADMIN | WS | ✅ 已有 |
| WORKSPACE_VARIABLES | ADMIN | WS | ✅ 已有 |
| WORKSPACE_RESOURCES | ADMIN | WS | ✅ 已有 |
| WORKSPACE_MANAGEMENT | ADMIN | WS | ✅ 已有 |
| WORKSPACE_STATE_SENSITIVE | READ | WS | **新增** |

---

#### Role 5: developer — 补 ORG 级只读 + AI 写入 + 敏感状态

> 定位: 能执行工作空间任务的开发者，同时能浏览组织级资源（模块、日志等），使用 AI 辅助。

现有 6 项，补充后完整策略：

| 资源类型 | 级别 | Scope | 状态 |
|----------|------|-------|------|
| WORKSPACE_EXECUTION | WRITE | WS | ✅ 已有 |
| WORKSPACE_VARIABLES | WRITE | WS | ✅ 已有 |
| WORKSPACE_RESOURCES | WRITE | WS | ✅ 已有 |
| WORKSPACE_MANAGEMENT | WRITE | WS | ✅ 已有 |
| TASK_DATA_ACCESS | READ | WS | ✅ 已有 |
| WORKSPACE_STATE | READ | WS | ✅ 已有 |
| ORGANIZATION | READ | ORG | **新增** (Dashboard) |
| WORKSPACES | READ | ORG | **新增** (工作空间列表) |
| PROJECTS | READ | ORG | **新增** (项目列表) |
| MODULES | READ | ORG | **新增** (模块列表) |
| MODULE_DEMOS | READ | ORG | **新增** (Demo) |
| SCHEMAS | READ | ORG | **新增** (Schema) |
| TASK_LOGS | READ | ORG | **新增** (全局日志) |
| AI_ANALYSIS | WRITE | ORG | **新增** (使用AI分析) |
| WORKSPACE_STATE_SENSITIVE | READ | WS | **新增** (查看State内容) |

> **待确认**: developer 是否需要 AGENT_POOLS/ORG/READ？让开发者能看到 Pool 列表选择执行环境。

---

#### Role 6: viewer — 补齐新增资源的 READ

> 定位: 全组织只读，能看到一切但不能改。

现有 17 种资源（每种 ORG/PROJECT/WS 三个 scope），补充后新增：

| 资源类型 | 级别 | Scope | 状态 |
|----------|------|-------|------|
| *(现有 17 种)* | READ | ORG+PROJECT+WS | ✅ 已有 |
| MODULE_DEMOS | READ | ORG+PROJECT+WS | **新增** (3条) |
| SCHEMAS | READ | ORG+PROJECT+WS | **新增** (3条) |
| TASK_LOGS | READ | ORG+PROJECT+WS | **新增** (3条) |
| AGENT_POOLS | READ | ORG+PROJECT+WS | **新增** (3条) |
| AI_ANALYSIS | READ | ORG+PROJECT+WS | **新增** (3条) |
| AI_CONFIGS | READ | ORG+PROJECT+WS | **新增** (3条) |
| TERRAFORM_VERSIONS | READ | ORG+PROJECT+WS | **新增** (3条) |
| RUN_TASKS | READ | ORG+PROJECT+WS | **新增** (3条) |
| WORKSPACE_STATE_SENSITIVE | READ | ORG+PROJECT+WS | **新增** (3条) |
| SYSTEM_SETTINGS | READ | ORG+PROJECT+WS | **新增** (3条) |

共新增 10 种 × 3 scope = **30 条策略**。

---

#### Role 30: user — 定义最低可用权限

> 定位: 登录后默认角色，能浏览基本信息、查看被分配的工作空间任务。

| 资源类型 | 级别 | Scope | 说明 |
|----------|------|-------|------|
| ORGANIZATION | READ | ORG | 看到 Dashboard |
| WORKSPACES | READ | ORG | 工作空间列表 |
| PROJECTS | READ | ORG | 项目列表 |
| MODULES | READ | ORG | 模块列表 |
| TASK_DATA_ACCESS | READ | WS | 查看任务数据 |
| WORKSPACE_STATE | READ | WS | 查看状态 |
| WORKSPACE_MANAGEMENT | READ | WS | 查看工作空间信息 |

> **待确认**: user 角色是"最低可用"（能看基本信息），还是完全空白（必须手动授权）？

---

#### Role 26/27/28: iam_admin / iam_write / iam_read — 不需改动

已完整覆盖所有 IAM_* 资源类型，无缺失。

---

### 4.5 待确认事项

| # | 问题 | 影响 |
|---|------|------|
| 1 | org_admin 是否该有 SYSTEM_SETTINGS/ADMIN？ | 该权限控制平台配置和 MFA 全局配置，给 org_admin 还是只留给 admin？ |
| 2 | user 角色的定位？ | "最低可用"（能看基本信息）还是空白（必须手动授权）？ |
| 3 | developer 是否需要 AGENT_POOLS/ORG/READ？ | 让开发者能看到 Pool 列表选择执行环境 |
| 4 | 迁移 SQL 放哪个目录？ | 是否有现成的 migration 目录规范 |

---

### 4.6 实施步骤

```
步骤 1: resource_type.go
  - 新增 SYSTEM_SETTINGS, RUN_TASKS, WORKSPACE_STATE_SENSITIVE 常量
  - 更新 IsValid() / GetScopeLevel() switch

步骤 2: 迁移 SQL (一个文件，幂等)
  - INSERT permission_definitions (SYSTEM_SETTINGS)
  - INSERT iam_role_policies (各角色补齐策略)
  - 使用 ON CONFLICT DO NOTHING 或 WHERE NOT EXISTS 保证可重复执行

步骤 3: 更新种子数据 init_seed_data.sql (可选)
  - 同步更新种子数据，保证新部署环境也有完整角色策略
```
