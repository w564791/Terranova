# 权限系统设计方案（最终版）

> 基于 Terraform Enterprise 三层权限模型，适配 Golang 实现

-----

## 📋 目录

1. [系统概述](#1-系统概述)
2. [核心架构设计](#2-核心架构设计)
3. [数据库设计](#3-数据库设计)
4. [服务层设计](#4-服务层设计golang)
5. [API 接口设计](#5-api-接口设计)
6. [实施路线图](#6-实施路线图)

-----

## 1. 系统概述

### 1.1 设计目标

|目标         |说明                                            |
|-----------|----------------------------------------------|
|**三层权限模型** |组织（Organization）→ 项目（Project）→ 工作空间（Workspace）|
|**团队优先管理** |基于团队（Team）授权，用户通过加入团队获得权限                     |
|**全局与局部权限**|支持应用注册（全局）和任务数据访问（局部）                         |
|**权限继承覆盖** |上层权限可影响下层，支持显式拒绝（NONE）                        |
|**细粒度控制**  |READ / WRITE / ADMIN 三级权限                     |
|**完整审计**   |记录所有权限变更和资源访问日志                               |

### 1.2 核心特性

```yaml
权限管理方式:
  - 基于团队（Team-based RBAC）
  - 支持用户直接授权（特殊场景）
  - 应用（Application）仅限组织级权限

权限决策机制:
  - 多层级权限收集（组织 → 项目 → 工作空间）
  - 显式拒绝优先（NONE > ADMIN > WRITE > READ）
  - 最近作用域优先（工作空间 > 项目 > 组织）
  - 团队权限自动继承

性能优化:
  - Redis 缓存权限结果（TTL 5-10分钟）
  - 数据库查询索引优化
  - 批量权限检查接口
```

-----

## 2. 核心架构设计

### 2.1 三层权限模型

```
┌─────────────────────────────────────────────────────────────────┐
│                     Organization (组织层)                        │
│  ┌────────────────────────────────────────────────────────┐    │
│  │  全局权限管理                                            │    │
│  │  • 应用注册权限 (APPLICATION_REGISTRATION)               │    │
│  │  • 组织设置 (ORGANIZATION_SETTINGS)                     │    │
│  │  • 用户管理 (USER_MANAGEMENT)                           │    │
│  │  • 所有项目访问 (ALL_PROJECTS)                          │    │
│  │                                                          │    │
│  │  固定团队:                                               │    │
│  │  - owners (组织所有者)                                   │    │
│  │  - admins (组织管理员)                                   │    │
│  └────────────────────────────────────────────────────────┘    │
└──────────────────┬──────────────────────────────────────────────┘
                   │
        ┌──────────┴──────────┬──────────────┬───────────┐
        ▼                     ▼              ▼           ▼
   ┌─────────┐          ┌─────────┐    ┌─────────┐  ┌─────────┐
   │ Project │          │ Project │    │ Project │  │ Default │
   │  ML训练  │          │  数据标注 │    │  API服务 │  │ Project │
   └────┬────┘          └────┬────┘    └────┬────┘  └────┬────┘
        │                    │              │           │
        │ 项目级权限          │              │           │
        │ • 项目设置          │              │           │
        │ • 成员管理          │              │           │
        │ • 工作空间批量授权  │              │           │
        │                    │              │           │
   ┌────┼────┬────┬────┐    │         ┌────┴────┐     │
   ▼    ▼    ▼    ▼    ▼    ▼         ▼         ▼     ▼
[WS1][WS2][WS3][WS4][WS5][WS6]    [WS7]    [WS8] [WS9]
 任务池 数据集 模型库 训练池 测试集 标注池  API网关 文档库 临时池
 
 工作空间级权限:
 • 任务数据访问 (TASK_DATA_ACCESS)
 • 工作空间执行 (WORKSPACE_EXECUTION)
 • 状态管理 (WORKSPACE_STATE)
 • 变量管理 (WORKSPACE_VARIABLES)
```

### 2.2 权限类型定义

#### 2.2.1 作用域（Scope）

|作用域             |说明  |适用资源               |
|----------------|----|-------------------|
|**ORGANIZATION**|组织全局|应用注册、组织设置、用户管理、所有项目|
|**PROJECT**     |项目范围|项目设置、项目团队、项目内所有工作空间|
|**WORKSPACE**   |工作空间|任务数据、执行操作、状态管理、变量配置|

#### 2.2.2 权限等级（Permission Level）

```
等级优先级: ADMIN > WRITE > READ > NONE

NONE (0)  - 显式拒绝，最高优先级
  └─ 使用场景: 临时禁止某用户访问特定资源
  
READ (1)  - 只读权限
  ├─ 查看资源列表
  ├─ 读取资源详情
  ├─ 下载数据
  └─ 查看日志

WRITE (2) - 读写权限
  ├─ 包含 READ 所有权限
  ├─ 创建资源
  ├─ 修改资源
  ├─ 执行操作
  └─ 上传数据

ADMIN (3) - 管理权限
  ├─ 包含 WRITE 所有权限
  ├─ 删除资源
  ├─ 管理权限
  ├─ 配置设置
  └─ 查看审计日志
```

#### 2.2.3 资源类型（Resource Type）

```go
// 组织级资源
const (
    ResourceTypeAppRegistration   = "APPLICATION_REGISTRATION"    // 应用注册
    ResourceTypeOrgSettings       = "ORGANIZATION_SETTINGS"       // 组织设置
    ResourceTypeUserManagement    = "USER_MANAGEMENT"             // 用户管理
    ResourceTypeAllProjects       = "ALL_PROJECTS"                // 所有项目
)

// 项目级资源
const (
    ResourceTypeProjectSettings   = "PROJECT_SETTINGS"            // 项目设置
    ResourceTypeProjectTeams      = "PROJECT_TEAM_MANAGEMENT"     // 项目团队
    ResourceTypeProjectWorkspaces = "PROJECT_WORKSPACES"          // 项目工作空间
)

// 工作空间级资源
const (
    ResourceTypeTaskData          = "TASK_DATA_ACCESS"            // 任务数据
    ResourceTypeWorkspaceExec     = "WORKSPACE_EXECUTION"         // 执行操作
    ResourceTypeWorkspaceState    = "WORKSPACE_STATE"             // 状态管理
    ResourceTypeWorkspaceVars     = "WORKSPACE_VARIABLES"         // 变量管理
)
```

### 2.3 权限决策流程

```
┌─────────────────────────────────────────────────────────────┐
│                    用户请求访问资源                          │
│        User: alice, Resource: task_data, Scope: ws_001       │
└──────────────────────┬──────────────────────────────────────┘
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  1. 解析请求参数                                             │
│     ├─ 用户ID: alice                                        │
│     ├─ 资源类型: TASK_DATA_ACCESS                           │
│     ├─ 作用域: WORKSPACE (ws_001)                           │
│     └─ 所需等级: WRITE                                      │
└──────────────────────┬──────────────────────────────────────┘
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  2. 查询用户所属团队                                         │
│     └─ teams: [ml_engineers, data_team]                     │
└──────────────────────┬──────────────────────────────────────┘
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  3. 收集权限授予 (按层级)                                    │
│                                                              │
│  3.1 组织级权限 (org_001)                                   │
│      ├─ alice 直接授权: 无                                  │
│      ├─ ml_engineers: ALL_PROJECTS = READ                   │
│      └─ data_team: 无                                       │
│                                                              │
│  3.2 项目级权限 (project_ml)                                │
│      ├─ alice 直接授权: PROJECT_WORKSPACES = WRITE          │
│      ├─ ml_engineers: PROJECT_WORKSPACES = READ             │
│      └─ data_team: 无                                       │
│                                                              │
│  3.3 工作空间级权限 (ws_001)                                │
│      ├─ alice 直接授权: 无                                  │
│      ├─ ml_engineers: 无                                    │
│      └─ data_team: TASK_DATA_ACCESS = WRITE                 │
└──────────────────────┬──────────────────────────────────────┘
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  4. 计算有效权限                                             │
│     ├─ 过滤过期权限                                         │
│     ├─ 检查 NONE (无)                                       │
│     └─ 取最高等级: max(READ, WRITE, READ, WRITE) = WRITE    │
└──────────────────────┬──────────────────────────────────────┘
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  5. 权限判定                                                 │
│     有效权限 (WRITE) >= 所需权限 (WRITE) ?                   │
│     ✓ 是 → 允许访问                                         │
└──────────────────────┬──────────────────────────────────────┘
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  6. 记录访问日志                                             │
│     ├─ user: alice                                          │
│     ├─ action: WRITE                                        │
│     ├─ resource: ws_001/task_data                           │
│     ├─ result: ALLOWED                                      │
│     └─ timestamp: 2025-10-19 10:30:00                       │
└─────────────────────────────────────────────────────────────┘
```

-----

## 3. 数据库设计

### 3.1 核心实体表

#### 3.1.1 组织表（organizations）

```sql
CREATE TABLE organizations (
    org_id          VARCHAR(50) PRIMARY KEY COMMENT '组织ID',
    org_name        VARCHAR(200) NOT NULL COMMENT '组织名称（唯一标识）',
    display_name    VARCHAR(200) COMMENT '显示名称',
    description     TEXT COMMENT '组织描述',
    contact_email   VARCHAR(200) COMMENT '联系邮箱',
    is_active       BOOLEAN DEFAULT TRUE COMMENT '是否启用',
    settings_json   JSON COMMENT '组织配置',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_name (org_name),
    INDEX idx_active (is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='组织表';
```

#### 3.1.2 项目表（projects）

```sql
CREATE TABLE projects (
    project_id      VARCHAR(50) PRIMARY KEY COMMENT '项目ID',
    org_id          VARCHAR(50) NOT NULL COMMENT '所属组织ID',
    project_name    VARCHAR(200) NOT NULL COMMENT '项目名称',
    display_name    VARCHAR(200) COMMENT '显示名称',
    description     TEXT COMMENT '项目描述',
    is_default      BOOLEAN DEFAULT FALSE COMMENT '是否默认项目',
    is_active       BOOLEAN DEFAULT TRUE COMMENT '是否启用',
    settings_json   JSON COMMENT '项目配置',
    created_by      VARCHAR(50) COMMENT '创建人',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    FOREIGN KEY (org_id) REFERENCES organizations(org_id) ON DELETE CASCADE,
    UNIQUE KEY uk_org_name (org_id, project_name),
    INDEX idx_org_active (org_id, is_active),
    INDEX idx_default (is_default)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目表';
```

#### 3.1.3 工作空间表（workspaces）

```sql
CREATE TABLE workspaces (
    workspace_id    VARCHAR(50) PRIMARY KEY COMMENT '工作空间ID',
    project_id      VARCHAR(50) NOT NULL COMMENT '所属项目ID',
    workspace_name  VARCHAR(200) NOT NULL COMMENT '工作空间名称',
    display_name    VARCHAR(200) COMMENT '显示名称',
    workspace_type  ENUM('TASK_POOL', 'DATASET', 'MODULE', 'API_SERVICE', 'OTHER') 
                    DEFAULT 'TASK_POOL' COMMENT '工作空间类型',
    description     TEXT COMMENT '描述',
    config_json     JSON COMMENT '配置（变量、设置等）',
    is_locked       BOOLEAN DEFAULT FALSE COMMENT '是否锁定（锁定后不可修改）',
    is_active       BOOLEAN DEFAULT TRUE COMMENT '是否启用',
    created_by      VARCHAR(50) COMMENT '创建人',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE CASCADE,
    UNIQUE KEY uk_project_name (project_id, workspace_name),
    INDEX idx_project_active (project_id, is_active),
    INDEX idx_type (workspace_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='工作空间表（任务池/数据集等）';
```

#### 3.1.4 用户表（users）

```sql
CREATE TABLE users (
    user_id         VARCHAR(50) PRIMARY KEY COMMENT '用户ID',
    username        VARCHAR(100) NOT NULL UNIQUE COMMENT '用户名（登录名）',
    email           VARCHAR(200) NOT NULL UNIQUE COMMENT '邮箱',
    display_name    VARCHAR(200) COMMENT '显示名称',
    avatar_url      VARCHAR(500) COMMENT '头像URL',
    is_active       BOOLEAN DEFAULT TRUE COMMENT '是否启用',
    is_system_admin BOOLEAN DEFAULT FALSE COMMENT '是否系统超级管理员',
    password_hash   VARCHAR(255) COMMENT '密码哈希',
    last_login_at   TIMESTAMP NULL COMMENT '最后登录时间',
    last_login_ip   VARCHAR(50) COMMENT '最后登录IP',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_username (username),
    INDEX idx_email (email),
    INDEX idx_active (is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';
```

#### 3.1.5 团队表（teams）

```sql
CREATE TABLE teams (
    team_id         VARCHAR(50) PRIMARY KEY COMMENT '团队ID',
    org_id          VARCHAR(50) NOT NULL COMMENT '所属组织ID',
    team_name       VARCHAR(200) NOT NULL COMMENT '团队名称',
    display_name    VARCHAR(200) COMMENT '显示名称',
    description     TEXT COMMENT '团队描述',
    is_secret       BOOLEAN DEFAULT FALSE COMMENT '是否秘密团队（不在列表显示）',
    is_system       BOOLEAN DEFAULT FALSE COMMENT '是否系统预置团队（不可删除）',
    created_by      VARCHAR(50) COMMENT '创建人',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    FOREIGN KEY (org_id) REFERENCES organizations(org_id) ON DELETE CASCADE,
    UNIQUE KEY uk_org_team (org_id, team_name),
    INDEX idx_org (org_id),
    INDEX idx_system (is_system)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='团队表';
```

#### 3.1.6 团队成员关系表（team_members）

```sql
CREATE TABLE team_members (
    team_id         VARCHAR(50) COMMENT '团队ID',
    user_id         VARCHAR(50) COMMENT '用户ID',
    role_in_team    ENUM('MEMBER', 'MAINTAINER') DEFAULT 'MEMBER' 
                    COMMENT '团队内角色：成员/维护者',
    joined_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '加入时间',
    joined_by       VARCHAR(50) COMMENT '添加人',
    
    PRIMARY KEY (team_id, user_id),
    FOREIGN KEY (team_id) REFERENCES teams(team_id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE,
    INDEX idx_user (user_id),
    INDEX idx_team (team_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='团队成员关系表';
```

#### 3.1.7 应用表（applications）

```sql
CREATE TABLE applications (
    app_id          VARCHAR(50) PRIMARY KEY COMMENT '应用ID',
    org_id          VARCHAR(50) NOT NULL COMMENT '所属组织ID',
    app_name        VARCHAR(200) NOT NULL COMMENT '应用名称',
    app_key         VARCHAR(500) NOT NULL UNIQUE COMMENT 'API Key/Token',
    app_secret      VARCHAR(500) COMMENT 'API Secret（加密存储）',
    description     TEXT COMMENT '应用描述',
    callback_urls   JSON COMMENT '回调URL列表',
    is_active       BOOLEAN DEFAULT TRUE COMMENT '是否启用',
    created_by      VARCHAR(50) COMMENT '创建人',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at      TIMESTAMP NULL COMMENT '应用过期时间',
    last_used_at    TIMESTAMP NULL COMMENT '最后使用时间',
    
    FOREIGN KEY (org_id) REFERENCES organizations(org_id) ON DELETE CASCADE,
    UNIQUE KEY uk_org_app (org_id, app_name),
    INDEX idx_org_active (org_id, is_active),
    INDEX idx_key (app_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='应用表（外部系统）';
```

### 3.2 权限定义表

#### 3.2.1 权限定义表（permission_definitions）

```sql
CREATE TABLE permission_definitions (
    permission_id   VARCHAR(50) PRIMARY KEY COMMENT '权限ID',
    permission_name VARCHAR(200) NOT NULL UNIQUE COMMENT '权限名称',
    resource_type   VARCHAR(100) NOT NULL COMMENT '资源类型',
    scope_level     ENUM('ORGANIZATION', 'PROJECT', 'WORKSPACE') NOT NULL 
                    COMMENT '适用作用域层级',
    display_name    VARCHAR(200) COMMENT '显示名称',
    description     TEXT COMMENT '权限描述',
    is_system       BOOLEAN DEFAULT TRUE COMMENT '是否系统内置',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_resource (resource_type),
    INDEX idx_scope (scope_level),
    INDEX idx_system (is_system)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='权限定义表';
```

#### 3.2.2 权限预设表（permission_presets）

```sql
CREATE TABLE permission_presets (
    preset_id       VARCHAR(50) PRIMARY KEY COMMENT '预设ID',
    preset_name     VARCHAR(100) NOT NULL COMMENT '预设名称（READ/WRITE/ADMIN）',
    scope_level     ENUM('ORGANIZATION', 'PROJECT', 'WORKSPACE') NOT NULL 
                    COMMENT '适用层级',
    display_name    VARCHAR(200) COMMENT '显示名称',
    description     TEXT COMMENT '描述',
    
    UNIQUE KEY uk_preset (preset_name, scope_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='权限预设（固定权限集）';
```

#### 3.2.3 权限预设详情表（preset_permissions）

```sql
CREATE TABLE preset_permissions (
    preset_id       VARCHAR(50) COMMENT '预设ID',
    permission_id   VARCHAR(50) COMMENT '权限ID',
    permission_level ENUM('READ', 'WRITE', 'ADMIN') NOT NULL COMMENT '权限等级',
    
    PRIMARY KEY (preset_id, permission_id),
    FOREIGN KEY (preset_id) REFERENCES permission_presets(preset_id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permission_definitions(permission_id),
    INDEX idx_preset (preset_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='权限预设包含的具体权限';
```

### 3.3 权限分配表（核心）

#### 3.3.1 组织级权限分配表（org_permissions）

```sql
CREATE TABLE org_permissions (
    assignment_id   VARCHAR(50) PRIMARY KEY COMMENT '分配ID',
    org_id          VARCHAR(50) NOT NULL COMMENT '组织ID',
    principal_type  ENUM('TEAM', 'USER', 'APPLICATION') NOT NULL COMMENT '主体类型',
    principal_id    VARCHAR(50) NOT NULL COMMENT '主体ID（团队/用户/应用）',
    permission_id   VARCHAR(50) NOT NULL COMMENT '权限ID',
    permission_level ENUM('NONE', 'READ', 'WRITE', 'ADMIN') NOT NULL COMMENT '权限等级',
    granted_by      VARCHAR(50) COMMENT '授权人',
    granted_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '授权时间',
    expires_at      TIMESTAMP NULL COMMENT '过期时间（NULL表示永久）',
    reason          TEXT COMMENT '授权原因',
    
    FOREIGN KEY (org_id) REFERENCES organizations(org_id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permission_definitions(permission_id),
    UNIQUE KEY uk_assignment (org_id, principal_type, principal_id, permission_id),
    INDEX idx_principal (principal_type, principal_id),
    INDEX idx_org_principal (org_id, principal_type, principal_id),
    INDEX idx_permission (permission_id, permission_level),
    INDEX idx_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='组织级权限分配表';
```

#### 3.3.2 项目级权限分配表（project_permissions）

```sql
CREATE TABLE project_permissions (
    assignment_id   VARCHAR(50) PRIMARY KEY COMMENT '分配ID',
    project_id      VARCHAR(50) NOT NULL COMMENT '项目ID',
    principal_type  ENUM('TEAM', 'USER') NOT NULL COMMENT '主体类型',
    principal_id    VARCHAR(50) NOT NULL COMMENT '主体ID',
    permission_id   VARCHAR(50) NOT NULL COMMENT '权限ID',
    permission_level ENUM('NONE', 'READ', 'WRITE', 'ADMIN') NOT NULL COMMENT '权限等级',
    granted_by      VARCHAR(50) COMMENT '授权人',
    granted_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '授权时间',
    expires_at      TIMESTAMP NULL COMMENT '过期时间',
    reason          TEXT COMMENT '授权原因',
    
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permission_definitions(permission_id),
    UNIQUE KEY uk_assignment (project_id, principal_type, principal_id, permission_id),
    INDEX idx_principal (principal_type, principal_id),
    INDEX idx_project_principal (project_id, principal_type, principal_id),
    INDEX idx_permission (permission_id, permission_level),
    INDEX idx_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目级权限分配表';
```

#### 3.3.3 工作空间级权限分配表（workspace_permissions）

```sql
CREATE TABLE workspace_permissions (
    assignment_id   VARCHAR(50) PRIMARY KEY COMMENT '分配ID',
    workspace_id    VARCHAR(50) NOT NULL COMMENT '工作空间ID',
    principal_type  ENUM('TEAM', 'USER') NOT NULL COMMENT '主体类型',
    principal_id    VARCHAR(50) NOT NULL COMMENT '主体ID',
    permission_id   VARCHAR(50) NOT NULL COMMENT '权限ID',
    permission_level ENUM('NONE', 'READ', 'WRITE', 'ADMIN') NOT NULL COMMENT '权限等级',
    granted_by      VARCHAR(50) COMMENT '授权人',
    granted_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '授权时间',
    expires_at      TIMESTAMP NULL COMMENT '过期时间',
    reason          TEXT COMMENT '授权原因',
    
    FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permission_definitions(permission_id),
    UNIQUE KEY uk_assignment (workspace_id, principal_type, principal_id, permission_id),
    INDEX idx_principal (principal_type, principal_id),
    INDEX idx_workspace_principal (workspace_id, principal_type, principal_id),
    INDEX idx_permission (permission_id, permission_level),
    INDEX idx_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='工作空间级权限分配表';
```

### 3.4 审计日志表

#### 3.4.1 权限变更审计日志（permission_audit_log）

```sql
CREATE TABLE permission_audit_log (
    log_id          VARCHAR(50) PRIMARY KEY COMMENT '日志ID',
    action_type     ENUM('GRANT', 'REVOKE', 'MODIFY', 'EXPIRE') NOT NULL COMMENT '操作类型',
    scope_type      ENUM('ORGANIZATION', 'PROJECT', 'WORKSPACE') NOT NULL COMMENT '作用域类型',
    scope_id        VARCHAR(50) NOT NULL COMMENT '作用域ID',
    principal_type  ENUM('TEAM', 'USER', 'APPLICATION') NOT NULL COMMENT '主体类型',
    principal_id    VARCHAR(50) NOT NULL COMMENT '主体ID',
    permission_id   VARCHAR(50) COMMENT '权限ID',
    old_level       ENUM('NONE', 'READ', 'WRITE', 'ADMIN') COMMENT '原权限等级',
    new_level       ENUM('NONE', 'READ', 'WRITE', 'ADMIN') COMMENT '新权限等级',
    performed_by    VARCHAR(50) NOT NULL COMMENT '操作人',
    performed_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '操作时间',
    reason          TEXT COMMENT '操作原因',
    ip_address      VARCHAR(50) COMMENT 'IP地址',
    user_agent      TEXT COMMENT 'User Agent',
    
    INDEX idx_scope (scope_type, scope_id),
    INDEX idx_principal (principal_type, principal_id),
    INDEX idx_performer (performed_by),
    INDEX idx_time (performed_at),
    INDEX idx_action (action_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='权限变更审计日志';
```

#### 3.4.2 资源访问日志（resource_access_log）

```sql
CREATE TABLE resource_access_log (
    log_id          VARCHAR(50) PRIMARY KEY COMMENT '日志ID',
    user_id         VARCHAR(50) NOT NULL COMMENT '用户ID',
    resource_type   VARCHAR(100) NOT NULL COMMENT '资源类型',
    resource_id     VARCHAR(50) NOT NULL COMMENT '资源ID',
    action          VARCHAR(100) NOT NULL COMMENT '操作动作（READ/WRITE/DELETE等）',
    is_allowed      BOOLEAN NOT NULL COMMENT '是否允许',
    deny_reason     VARCHAR(500) COMMENT '拒绝原因',
    effective_level VARCHAR(20) COMMENT '有效权限等级',
    accessed_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '访问时间',
    ip_address      VARCHAR(50) COMMENT 'IP地址',
    duration_ms     INT COMMENT '请求耗时（毫秒）',
    
    INDEX idx_user (user_id),
    INDEX idx_resource (resource_type, resource_id),
    INDEX idx_time (accessed_at),
    INDEX idx_allowed (is_allowed)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='资源访问日志';

-- 按月分区（提升查询性能）
ALTER TABLE resource_access_log PARTITION BY RANGE (TO_
```sql
-- 按月分区（提升查询性能）
ALTER TABLE resource_access_log PARTITION BY RANGE (TO_DAYS(accessed_at)) (
    PARTITION p202501 VALUES LESS THAN (TO_DAYS('2025-02-01')),
    PARTITION p202502 VALUES LESS THAN (TO_DAYS('2025-03-01')),
    PARTITION p202503 VALUES LESS THAN (TO_DAYS('2025-04-01')),
    PARTITION p202504 VALUES LESS THAN (TO_DAYS('2025-05-01')),
    PARTITION p202505 VALUES LESS THAN (TO_DAYS('2025-06-01')),
    PARTITION p202506 VALUES LESS THAN (TO_DAYS('2025-07-01')),
    PARTITION pfuture VALUES LESS THAN MAXVALUE
);
```

### 3.5 初始化数据脚本

```sql
-- =============================================
-- 初始化权限定义
-- =============================================
INSERT INTO permission_definitions 
(permission_id, permission_name, resource_type, scope_level, display_name, description, is_system) 
VALUES
-- 组织级权限
('perm_app_reg',        'application_registration',     'APPLICATION_REGISTRATION',     'ORGANIZATION', '应用注册',     '管理应用注册权限',         TRUE),
('perm_org_settings',   'organization_settings',        'ORGANIZATION_SETTINGS',        'ORGANIZATION', '组织设置',     '管理组织配置',             TRUE),
('perm_user_mgmt',      'user_management',              'USER_MANAGEMENT',              'ORGANIZATION', '用户管理',     '管理组织用户',             TRUE),
('perm_all_projects',   'all_projects',                 'ALL_PROJECTS',                 'ORGANIZATION', '所有项目',     '访问所有项目',             TRUE),

-- 项目级权限
('perm_proj_settings',  'project_settings',             'PROJECT_SETTINGS',             'PROJECT',      '项目设置',     '管理项目配置',             TRUE),
('perm_proj_teams',     'project_team_management',      'PROJECT_TEAM_MANAGEMENT',      'PROJECT',      '项目团队',     '管理项目团队',             TRUE),
('perm_proj_workspaces','project_workspaces',           'PROJECT_WORKSPACES',           'PROJECT',      '项目工作空间', '管理项目内工作空间',       TRUE),

-- 工作空间级权限
('perm_task_data',      'task_data_access',             'TASK_DATA_ACCESS',             'WORKSPACE',    '任务数据',     '访问任务数据',             TRUE),
('perm_ws_exec',        'workspace_execution',          'WORKSPACE_EXECUTION',          'WORKSPACE',    '工作空间执行', '执行工作空间操作',         TRUE),
('perm_ws_state',       'workspace_state',              'WORKSPACE_STATE',              'WORKSPACE',    '状态管理',     '管理工作空间状态',         TRUE),
('perm_ws_vars',        'workspace_variables',          'WORKSPACE_VARIABLES',          'WORKSPACE',    '变量管理',     '管理工作空间变量',         TRUE);

-- =============================================
-- 初始化权限预设
-- =============================================
INSERT INTO permission_presets 
(preset_id, preset_name, scope_level, display_name, description) 
VALUES
-- 组织级预设
('preset_org_read',     'READ',     'ORGANIZATION', '组织只读',     '查看组织信息和项目列表'),
('preset_org_write',    'WRITE',    'ORGANIZATION', '组织编辑',     '管理组织资源（不含用户管理）'),
('preset_org_admin',    'ADMIN',    'ORGANIZATION', '组织管理员',   '完全控制组织'),

-- 项目级预设
('preset_proj_read',    'READ',     'PROJECT',      '项目只读',     '查看项目信息和工作空间'),
('preset_proj_write',   'WRITE',    'PROJECT',      '项目编辑',     '管理项目工作空间'),
('preset_proj_admin',   'ADMIN',    'PROJECT',      '项目管理员',   '完全控制项目'),

-- 工作空间级预设
('preset_ws_read',      'READ',     'WORKSPACE',    '工作空间只读', '查看数据和配置'),
('preset_ws_write',     'WRITE',    'WORKSPACE',    '工作空间编辑', '读写数据和执行操作'),
('preset_ws_admin',     'ADMIN',    'WORKSPACE',    '工作空间管理员','完全控制工作空间');

-- =============================================
-- 权限预设详情配置
-- =============================================
-- 组织级 READ 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level) VALUES
('preset_org_read', 'perm_all_projects', 'READ');

-- 组织级 WRITE 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level) VALUES
('preset_org_write', 'perm_all_projects', 'WRITE'),
('preset_org_write', 'perm_org_settings', 'WRITE');

-- 组织级 ADMIN 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level) VALUES
('preset_org_admin', 'perm_app_reg', 'ADMIN'),
('preset_org_admin', 'perm_org_settings', 'ADMIN'),
('preset_org_admin', 'perm_user_mgmt', 'ADMIN'),
('preset_org_admin', 'perm_all_projects', 'ADMIN');

-- 项目级 READ 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level) VALUES
('preset_proj_read', 'perm_proj_workspaces', 'READ');

-- 项目级 WRITE 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level) VALUES
('preset_proj_write', 'perm_proj_workspaces', 'WRITE'),
('preset_proj_write', 'perm_proj_settings', 'WRITE');

-- 项目级 ADMIN 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level) VALUES
('preset_proj_admin', 'perm_proj_settings', 'ADMIN'),
('preset_proj_admin', 'perm_proj_teams', 'ADMIN'),
('preset_proj_admin', 'perm_proj_workspaces', 'ADMIN');

-- 工作空间级 READ 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level) VALUES
('preset_ws_read', 'perm_task_data', 'READ'),
('preset_ws_read', 'perm_ws_state', 'READ');

-- 工作空间级 WRITE 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level) VALUES
('preset_ws_write', 'perm_task_data', 'WRITE'),
('preset_ws_write', 'perm_ws_exec', 'WRITE'),
('preset_ws_write', 'perm_ws_state', 'WRITE');

-- 工作空间级 ADMIN 预设
INSERT INTO preset_permissions (preset_id, permission_id, permission_level) VALUES
('preset_ws_admin', 'perm_task_data', 'ADMIN'),
('preset_ws_admin', 'perm_ws_exec', 'ADMIN'),
('preset_ws_admin', 'perm_ws_state', 'ADMIN'),
('preset_ws_admin', 'perm_ws_vars', 'ADMIN');
```

-----

## 4. 服务层设计（Golang）

### 4.1 项目结构

```
permission-system/
├── cmd/
│   └── server/
│       └── main.go                 # 服务入口
├── internal/
│   ├── domain/                     # 领域模型
│   │   ├── entity/                 # 实体定义
│   │   │   ├── user.go
│   │   │   ├── team.go
│   │   │   ├── organization.go
│   │   │   ├── project.go
│   │   │   ├── workspace.go
│   │   │   ├── permission.go
│   │   │   └── application.go
│   │   ├── valueobject/            # 值对象
│   │   │   ├── permission_level.go
│   │   │   ├── scope_type.go
│   │   │   └── resource_type.go
│   │   └── repository/             # 仓储接口
│   │       ├── user_repo.go
│   │       ├── team_repo.go
│   │       ├── permission_repo.go
│   │       └── audit_repo.go
│   │
│   ├── application/                # 应用服务层
│   │   ├── service/
│   │   │   ├── permission_service.go      # 权限管理服务
│   │   │   ├── permission_checker.go      # 权限检查服务
│   │   │   ├── team_service.go            # 团队管理服务
│   │   │   ├── org_service.go             # 组织管理服务
│   │   │   ├── project_service.go         # 项目管理服务
│   │   │   └── workspace_service.go       # 工作空间管理服务
│   │   ├── dto/                           # 数据传输对象
│   │   │   ├── permission_dto.go
│   │   │   ├── team_dto.go
│   │   │   └── check_request.go
│   │   └── usecase/                       # 用例
│   │       ├── grant_permission.go
│   │       ├── revoke_permission.go
│   │       └── check_permission.go
│   │
│   ├── infrastructure/             # 基础设施层
│   │   ├── persistence/            # 持久化实现
│   │   │   ├── mysql/
│   │   │   │   ├── user_repo_impl.go
│   │   │   │   ├── team_repo_impl.go
│   │   │   │   ├── permission_repo_impl.go
│   │   │   │   └── audit_repo_impl.go
│   │   │   └── db.go               # 数据库连接
│   │   ├── cache/                  # 缓存实现
│   │   │   ├── redis_cache.go
│   │   │   └── permission_cache.go
│   │   └── middleware/             # 中间件
│   │       ├── auth_middleware.go
│   │       └── permission_middleware.go
│   │
│   └── interfaces/                 # 接口层
│       ├── http/                   # HTTP 接口
│       │   ├── handler/
│       │   │   ├── permission_handler.go
│       │   │   ├── team_handler.go
│       │   │   ├── org_handler.go
│       │   │   └── workspace_handler.go
│       │   └── router/
│       │       └── router.go
│       └── grpc/                   # gRPC 接口（可选）
│           └── permission_grpc.go
│
├── pkg/                            # 公共包
│   ├── errors/                     # 错误定义
│   ├── logger/                     # 日志工具
│   ├── utils/                      # 工具函数
│   └── constants/                  # 常量定义
│
├── config/                         # 配置文件
│   ├── config.yaml
│   └── config.go
│
├── migrations/                     # 数据库迁移
│   ├── 001_init_schema.up.sql
│   └── 002_init_data.up.sql
│
├── go.mod
└── go.sum
```

### 4.2 核心类型定义

#### 4.2.1 权限等级（permission_level.go）

```go
package valueobject

type PermissionLevel int

const (
    PermissionLevelNone  PermissionLevel = 0  // 显式拒绝
    PermissionLevelRead  PermissionLevel = 1  // 只读
    PermissionLevelWrite PermissionLevel = 2  // 可编辑
    PermissionLevelAdmin PermissionLevel = 3  // 管理员
)

// String 返回权限等级字符串
func (p PermissionLevel) String() string

// IsValid 验证权限等级是否有效
func (p PermissionLevel) IsValid() bool

// GreaterThanOrEqual 判断是否大于等于目标等级
func (p PermissionLevel) GreaterThanOrEqual(target PermissionLevel) bool

// ParsePermissionLevel 从字符串解析权限等级
func ParsePermissionLevel(s string) (PermissionLevel, error)
```

#### 4.2.2 作用域类型（scope_type.go）

```go
package valueobject

type ScopeType string

const (
    ScopeTypeOrganization ScopeType = "ORGANIZATION"
    ScopeTypeProject      ScopeType = "PROJECT"
    ScopeTypeWorkspace    ScopeType = "WORKSPACE"
)

// String 返回作用域类型字符串
func (s ScopeType) String() string

// IsValid 验证作用域类型是否有效
func (s ScopeType) IsValid() bool
```

#### 4.2.3 资源类型（resource_type.go）

```go
package valueobject

type ResourceType string

// 组织级资源
const (
    ResourceTypeAppRegistration   ResourceType = "APPLICATION_REGISTRATION"
    ResourceTypeOrgSettings       ResourceType = "ORGANIZATION_SETTINGS"
    ResourceTypeUserManagement    ResourceType = "USER_MANAGEMENT"
    ResourceTypeAllProjects       ResourceType = "ALL_PROJECTS"
)

// 项目级资源
const (
    ResourceTypeProjectSettings   ResourceType = "PROJECT_SETTINGS"
    ResourceTypeProjectTeams      ResourceType = "PROJECT_TEAM_MANAGEMENT"
    ResourceTypeProjectWorkspaces ResourceType = "PROJECT_WORKSPACES"
)

// 工作空间级资源
const (
    ResourceTypeTaskData          ResourceType = "TASK_DATA_ACCESS"
    ResourceTypeWorkspaceExec     ResourceType = "WORKSPACE_EXECUTION"
    ResourceTypeWorkspaceState    ResourceType = "WORKSPACE_STATE"
    ResourceTypeWorkspaceVars     ResourceType = "WORKSPACE_VARIABLES"
)

// GetScopeLevel 返回资源类型对应的作用域层级
func (r ResourceType) GetScopeLevel() ScopeType
```

#### 4.2.4 权限授予实体（permission.go）

```go
package entity

import "time"

// PermissionGrant 权限授予记录
type PermissionGrant struct {
    AssignmentID    string                      // 分配ID
    ScopeType       valueobject.ScopeType       // 作用域类型
    ScopeID         string                      // 作用域ID
    PrincipalType   PrincipalType               // 主体类型（USER/TEAM/APPLICATION）
    PrincipalID     string                      // 主体ID
    PermissionID    string                      // 权限ID
    PermissionLevel valueobject.PermissionLevel // 权限等级
    GrantedBy       string                      // 授权人
    GrantedAt       time.Time                   // 授权时间
    ExpiresAt       *time.Time                  // 过期时间
    Reason          string                      // 授权原因
    Source          string                      // 来源（direct/team/inherited）
}

// IsExpired 判断权限是否过期
func (p *PermissionGrant) IsExpired() bool

// IsValid 判断权限是否有效（未过期）
func (p *PermissionGrant) IsValid() bool
```

### 4.3 权限检查器（Permission Checker）

#### 4.3.1 权限检查器接口（permission_checker.go）

```go
package service

import (
    "context"
    "permission-system/internal/domain/entity"
    "permission-system/internal/domain/valueobject"
)

// PermissionChecker 权限检查器接口
type PermissionChecker interface {
    // CheckPermission 检查用户是否拥有指定权限
    // 返回: 是否允许, 有效权限等级, 错误
    CheckPermission(ctx context.Context, req *CheckPermissionRequest) (*CheckPermissionResult, error)
    
    // CheckBatchPermissions 批量检查权限（优化性能）
    CheckBatchPermissions(ctx context.Context, reqs []*CheckPermissionRequest) ([]*CheckPermissionResult, error)
    
    // GetUserEffectivePermissions 获取用户的所有有效权限汇总
    GetUserEffectivePermissions(ctx context.Context, userID string, orgID string) (*UserPermissionSummary, error)
    
    // GetUserTeams 获取用户所属的所有团队
    GetUserTeams(ctx context.Context, userID string) ([]string, error)
    
    // InvalidateCache 使指定用户的权限缓存失效
    InvalidateCache(ctx context.Context, userID string) error
}

// CheckPermissionRequest 权限检查请求
type CheckPermissionRequest struct {
    UserID        string                      // 用户ID
    ResourceType  valueobject.ResourceType    // 资源类型
    ScopeType     valueobject.ScopeType       // 作用域类型
    ScopeID       string                      // 作用域ID
    RequiredLevel valueobject.PermissionLevel // 所需权限等级
}

// CheckPermissionResult 权限检查结果
type CheckPermissionResult struct {
    IsAllowed      bool                        // 是否允许
    EffectiveLevel valueobject.PermissionLevel // 有效权限等级
    Grants         []*entity.PermissionGrant   // 所有相关的权限授予记录
    DenyReason     string                      // 拒绝原因（如果不允许）
    CacheHit       bool                        // 是否命中缓存
}

// UserPermissionSummary 用户权限汇总
type UserPermissionSummary struct {
    UserID       string                                                    // 用户ID
    Teams        []string                                                  // 所属团队
    Organization map[string]map[string]valueobject.PermissionLevel         // 组织级权限
    Projects     map[string]map[string]valueobject.PermissionLevel         // 项目级权限
    Workspaces   map[string]map[string]valueobject.PermissionLevel         // 工作空间级权限
}
```

#### 4.3.2 权限检查器实现（permission_checker_impl.go）

```go
package service

// PermissionCheckerImpl 权限检查器实现
type PermissionCheckerImpl struct {
    permissionRepo repository.PermissionRepository  // 权限仓储
    teamRepo       repository.TeamRepository        // 团队仓储
    cache          cache.PermissionCache            // 权限缓存
    auditRepo      repository.AuditRepository       // 审计日志仓储
    logger         logger.Logger                    // 日志器
}

// NewPermissionChecker 创建权限检查器实例
func NewPermissionChecker(
    permissionRepo repository.PermissionRepository,
    teamRepo repository.TeamRepository,
    cache cache.PermissionCache,
    auditRepo repository.AuditRepository,
    logger logger.Logger,
) PermissionChecker

// CheckPermission 检查权限
// 1. 检查缓存
// 2. 收集所有权限授予（组织->项目->工作空间）
// 3. 计算有效权限
// 4. 记录访问日志
// 5. 缓存结果
func (c *PermissionCheckerImpl) CheckPermission(
    ctx context.Context,
    req *CheckPermissionRequest,
) (*CheckPermissionResult, error)

// collectAllGrants 收集用户的所有权限授予记录
// 按层级收集：组织级 -> 项目级 -> 工作空间级
// 每层收集：用户直接授权 + 用户所属团队授权
func (c *PermissionCheckerImpl) collectAllGrants(
    ctx context.Context,
    userID string,
    resourceType valueobject.ResourceType,
    scopeType valueobject.ScopeType,
    scopeID string,
) ([]*entity.PermissionGrant, error)

// collectOrgLevelGrants 收集组织级权限
func (c *PermissionCheckerImpl) collectOrgLevelGrants(
    ctx context.Context,
    userID string,
    userTeams []string,
    resourceType valueobject.ResourceType,
    orgID string,
) ([]*entity.PermissionGrant, error)

// collectProjectLevelGrants 收集项目级权限
func (c *PermissionCheckerImpl) collectProjectLevelGrants(
    ctx context.Context,
    userID string,
    userTeams []string,
    resourceType valueobject.ResourceType,
    projectID string,
) ([]*entity.PermissionGrant, error)

// collectWorkspaceLevelGrants 收集工作空间级权限
func (c *PermissionCheckerImpl) collectWorkspaceLevelGrants(
    ctx context.Context,
    userID string,
    userTeams []string,
    resourceType valueobject.ResourceType,
    workspaceID string,
) ([]*entity.PermissionGrant, error)

// calculateEffectiveLevel 计算有效权限等级
// 1. 过滤过期权限
// 2. 检查 NONE（显式拒绝优先）
// 3. 返回最高权限等级
func (c *PermissionCheckerImpl) calculateEffectiveLevel(
    grants []*entity.PermissionGrant,
) valueobject.PermissionLevel

// getOrgIDFromScope 根据作用域获取组织ID
func (c *PermissionCheckerImpl) getOrgIDFromScope(
    ctx context.Context,
    scopeType valueobject.ScopeType,
    scopeID string,
) (string, error)

// getProjectIDFromScope 根据作用域获取项目ID
func (c *PermissionCheckerImpl) getProjectIDFromScope(
    ctx context.Context,
    scopeType valueobject.ScopeType,
    scopeID string,
) (string, error)

// logAccess 记录资源访问日志
func (c *PermissionCheckerImpl) logAccess(
    ctx context.Context,
    req *CheckPermissionRequest,
    result *CheckPermissionResult,
) error

// getCacheKey 生成缓存键
func (c *PermissionCheckerImpl) getCacheKey(req *CheckPermissionRequest) string

// GetUserTeams 获取用户所属团队
func (c *PermissionCheckerImpl) GetUserTeams(
    ctx context.Context,
    userID string,
) ([]string, error)

// GetUserEffectivePermissions 获取用户有效权限汇总
func (c *PermissionCheckerImpl) GetUserEffectivePermissions(
    ctx context.Context,
    userID string,
    orgID string,
) (*UserPermissionSummary, error)

// CheckBatchPermissions 批量检查权限（优化性能）
func (c *PermissionCheckerImpl) CheckBatchPermissions(
    ctx context.Context,
    reqs []*CheckPermissionRequest,
) ([]*CheckPermissionResult, error)

// InvalidateCache 使缓存失效
func (c *PermissionCheckerImpl) InvalidateCache(
    ctx context.Context,
    userID string,
) error
```

### 4.4 权限管理服务（Permission Service）

```go
package service

// PermissionService 权限管理服务接口
type PermissionService interface {
    // GrantPermission 授予权限
    GrantPermission(ctx context.Context, req *GrantPermissionRequest) error
    
    // RevokePermission 撤销权限
    RevokePermission(ctx context.Context, req *RevokePermissionRequest) error
    
    // ModifyPermission 修改权限等级
    ModifyPermission(ctx context.Context, req *ModifyPermissionRequest) error
    
    // GrantPresetPermissions 授予预设权限集（READ/WRITE/ADMIN）
    GrantPresetPermissions(ctx context.Context, req *GrantPresetRequest) error
    
    // ListPermissions 列出指定作用域的所有权限分配
    ListPermissions(ctx context.Context, scopeType valueobject.ScopeType, scopeID string) ([]*entity.PermissionGrant, error)
    
    // GetPermissionHistory 获取权限变更历史
    GetPermissionHistory(ctx context.Context, req *PermissionHistoryRequest) ([]*entity.PermissionAuditLog, error)
}

// GrantPermissionRequest 授予权限请求
type GrantPermissionRequest struct {
    ScopeType       valueobject.ScopeType       // 作用域类型
    ScopeID         string                      // 作用域ID
    PrincipalType   entity.PrincipalType        // 主体类型
    PrincipalID     string                      // 主体ID
    PermissionID    string                      // 权限ID
    PermissionLevel valueobject.PermissionLevel // 权限等级
    GrantedBy       string                      // 授权人
    ExpiresAt       *time.Time                  // 过期时间
    Reason          string                      // 授权原因
}

// PermissionServiceImpl 权限管理服务实现
type PermissionServiceImpl struct {
    permissionRepo repository.PermissionRepository
    auditRepo      repository.AuditRepository
    cache          cache.PermissionCache
    checker        PermissionChecker
    logger         logger.Logger
}

// GrantPermission 授予权限
// 1. 验证授权人权限
// 2. 检查是否已存在相同授权
// 3. 插入权限分配记录
// 4. 记录审计日志
// 5. 使相关缓存失效
func (s *PermissionServiceImpl) GrantPermission(
    ctx context.Context,
    req *GrantPermissionRequest,
) error

// RevokePermission 撤销权限
func (s *PermissionServiceImpl) RevokePermission(
    ctx context.Context,
    req *RevokePermissionRequest,
) error

// ModifyPermission 修改权限等级
func (s *PermissionServiceImpl) ModifyPermission(
    ctx context.Context,
    req *ModifyPermissionRequest,
) error

// GrantPresetPermissions 授予预设权限集
// 根据预设（READ/WRITE/ADMIN）批量授予一组权限
func (s *PermissionServiceImpl) GrantPresetPermissions(
    ctx context.Context,
    req *GrantPresetRequest,
) error
```

### 4.5 团队管理服务（Team Service）

```go
package service

// TeamService 团队管理服务接口
type TeamService interface {
    // CreateTeam 创建团队
    CreateTeam(ctx context.Context, req *CreateTeamRequest) (*entity.Team, error)
    
    // DeleteTeam 删除团队
    DeleteTeam(ctx context.Context, teamID string, deletedBy string) error
    
    // AddTeamMember 添加团队成员
    AddTeamMember(ctx context.Context, teamID string, userID string, role entity.TeamRole, addedBy string) error
    
    // RemoveTeamMember 移除团队成员
    RemoveTeamMember(ctx context.Context, teamID string, userID string, removedBy string) error
    
    // ListTeamMembers 列出团队成员
    ListTeamMembers(ctx context.Context, teamID string) ([]*entity.User, error)
    
    // ListUserTeams 列出用户所属的所有团队
    ListUserTeams(ctx context.Context, userID string) ([]*entity.Team, error)
    
    // GetTeamPermissions 获取团队的所有权限
    GetTeamPermissions(ctx context.Context, teamID string) ([]*entity.PermissionGrant, error)
}

// TeamServiceImpl 团队管理服务实现
type TeamServiceImpl struct {
    teamRepo       repository.TeamRepository
    permissionRepo repository.PermissionRepository
    auditRepo      repository.AuditRepository
    cache          cache.PermissionCache
    logger         logger.Logger
}

// AddTeamMember 添加团队成员
// 1. 验证操作人权限
// 2. 检查用户是否已在团队中
// 3. 添加成员关系
// 4. 使用户权限缓存失效
// 5. 记录审计日志
func (s *TeamServiceImpl) AddTeamMember(
    ctx context.Context,
    teamID string,
    userID string,
    role entity.TeamRole,
    addedBy string,
) error

// RemoveTeamMember 移除团队成员
func (s *TeamServiceImpl) RemoveTeamMember(
    ctx context.Context,
    teamID string,
    userID string,
    removedBy string,
) error
```

### 4.6 权限缓存（Permission Cache）

```go
package cache

import (
    "context"
    "time"
)

// PermissionCache 权限缓存接口
type PermissionCache interface {
    // GetPermissionResult 获取权限检查结果缓存
    GetPermissionResult(ctx context.Context, key string) (*service.CheckPermissionResult, error)
    
    // SetPermissionResult 设置权限检查结果缓存
    SetPermissionResult(ctx context.Context, key string, result *service.CheckPermissionResult, ttl time.Duration) error
    
    // GetUserTeams 获取用户团队缓存
    GetUserTeams(ctx context.Context, userID string) ([]string, error)
    
    // SetUserTeams 设置用户团队缓存
    SetUserTeams(ctx context.Context, userID string, teams []string, ttl time.Duration) error
    
    // InvalidateUser​​​​​​​​​​​​​​​​
```go
    // InvalidateUser 使指定用户的所有缓存失效
    InvalidateUser(ctx context.Context, userID string) error
    
    // InvalidateTeam 使指定团队相关的所有用户缓存失效
    InvalidateTeam(ctx context.Context, teamID string) error
    
    // InvalidateScope 使指定作用域相关的缓存失效
    InvalidateScope(ctx context.Context, scopeType valueobject.ScopeType, scopeID string) error
}

// RedisCacheImpl Redis 缓存实现
type RedisCacheImpl struct {
    client      *redis.Client
    keyPrefix   string        // 缓存键前缀
    defaultTTL  time.Duration // 默认过期时间（5分钟）
    logger      logger.Logger
}

// NewRedisCache 创建 Redis 缓存实例
func NewRedisCache(
    client *redis.Client,
    keyPrefix string,
    defaultTTL time.Duration,
    logger logger.Logger,
) PermissionCache

// generatePermissionKey 生成权限检查缓存键
// 格式: permission:result:{userID}:{resourceType}:{scopeType}:{scopeID}:{level}
func (c *RedisCacheImpl) generatePermissionKey(
    userID string,
    resourceType string,
    scopeType string,
    scopeID string,
    level string,
) string

// generateUserTeamsKey 生成用户团队缓存键
// 格式: permission:user:teams:{userID}
func (c *RedisCacheImpl) generateUserTeamsKey(userID string) string

// generateScopePattern 生成作用域缓存键模式（用于批量删除）
// 格式: permission:result:*:{scopeType}:{scopeID}:*
func (c *RedisCacheImpl) generateScopePattern(
    scopeType valueobject.ScopeType,
    scopeID string,
) string

// GetPermissionResult 获取权限检查结果
func (c *RedisCacheImpl) GetPermissionResult(
    ctx context.Context,
    key string,
) (*service.CheckPermissionResult, error)

// SetPermissionResult 设置权限检查结果
func (c *RedisCacheImpl) SetPermissionResult(
    ctx context.Context,
    key string,
    result *service.CheckPermissionResult,
    ttl time.Duration,
) error

// InvalidateUser 使用户缓存失效
// 删除用户的所有权限检查结果和团队缓存
func (c *RedisCacheImpl) InvalidateUser(
    ctx context.Context,
    userID string,
) error

// InvalidateTeam 使团队缓存失效
// 1. 查询团队所有成员
// 2. 使每个成员的缓存失效
func (c *RedisCacheImpl) InvalidateTeam(
    ctx context.Context,
    teamID string,
) error
```

### 4.7 仓储接口（Repository）

#### 4.7.1 权限仓储（permission_repo.go）

```go
package repository

// PermissionRepository 权限仓储接口
type PermissionRepository interface {
    // QueryOrgPermissions 查询组织级权限
    QueryOrgPermissions(
        ctx context.Context,
        orgID string,
        principalType entity.PrincipalType,
        principalID string,
        resourceType valueobject.ResourceType,
    ) ([]*entity.PermissionGrant, error)
    
    // QueryProjectPermissions 查询项目级权限
    QueryProjectPermissions(
        ctx context.Context,
        projectID string,
        principalType entity.PrincipalType,
        principalID string,
        resourceType valueobject.ResourceType,
    ) ([]*entity.PermissionGrant, error)
    
    // QueryWorkspacePermissions 查询工作空间级权限
    QueryWorkspacePermissions(
        ctx context.Context,
        workspaceID string,
        principalType entity.PrincipalType,
        principalID string,
        resourceType valueobject.ResourceType,
    ) ([]*entity.PermissionGrant, error)
    
    // GrantOrgPermission 授予组织级权限
    GrantOrgPermission(ctx context.Context, grant *entity.PermissionGrant) error
    
    // GrantProjectPermission 授予项目级权限
    GrantProjectPermission(ctx context.Context, grant *entity.PermissionGrant) error
    
    // GrantWorkspacePermission 授予工作空间级权限
    GrantWorkspacePermission(ctx context.Context, grant *entity.PermissionGrant) error
    
    // RevokePermission 撤销权限
    RevokePermission(ctx context.Context, assignmentID string) error
    
    // UpdatePermission 更新权限等级
    UpdatePermission(ctx context.Context, assignmentID string, newLevel valueobject.PermissionLevel) error
    
    // ListPermissionsByScopeAndPrincipal 列出指定作用域和主体的所有权限
    ListPermissionsByScopeAndPrincipal(
        ctx context.Context,
        scopeType valueobject.ScopeType,
        scopeID string,
        principalType entity.PrincipalType,
        principalID string,
    ) ([]*entity.PermissionGrant, error)
    
    // ListPermissionsByScope 列出指定作用域的所有权限分配
    ListPermissionsByScope(
        ctx context.Context,
        scopeType valueobject.ScopeType,
        scopeID string,
    ) ([]*entity.PermissionGrant, error)
    
    // GetPresetPermissions 获取预设权限集包含的权限列表
    GetPresetPermissions(
        ctx context.Context,
        presetName string,
        scopeLevel valueobject.ScopeType,
    ) ([]*PresetPermissionDetail, error)
}

// PresetPermissionDetail 预设权限详情
type PresetPermissionDetail struct {
    PermissionID    string
    ResourceType    valueobject.ResourceType
    PermissionLevel valueobject.PermissionLevel
}
```

#### 4.7.2 团队仓储（team_repo.go）

```go
package repository

// TeamRepository 团队仓储接口
type TeamRepository interface {
    // CreateTeam 创建团队
    CreateTeam(ctx context.Context, team *entity.Team) error
    
    // DeleteTeam 删除团队
    DeleteTeam(ctx context.Context, teamID string) error
    
    // GetTeamByID 根据ID获取团队
    GetTeamByID(ctx context.Context, teamID string) (*entity.Team, error)
    
    // ListTeamsByOrg 列出组织的所有团队
    ListTeamsByOrg(ctx context.Context, orgID string) ([]*entity.Team, error)
    
    // AddMember 添加团队成员
    AddMember(ctx context.Context, teamID string, userID string, role entity.TeamRole) error
    
    // RemoveMember 移除团队成员
    RemoveMember(ctx context.Context, teamID string, userID string) error
    
    // ListMembers 列出团队成员
    ListMembers(ctx context.Context, teamID string) ([]*entity.User, error)
    
    // GetUserTeams 获取用户所属的所有团队ID
    GetUserTeams(ctx context.Context, userID string) ([]string, error)
    
    // GetUserTeamsInOrg 获取用户在指定组织中的所有团队
    GetUserTeamsInOrg(ctx context.Context, userID string, orgID string) ([]*entity.Team, error)
    
    // IsMember 判断用户是否是团队成员
    IsMember(ctx context.Context, teamID string, userID string) (bool, error)
}
```

#### 4.7.3 审计日志仓储（audit_repo.go）

```go
package repository

// AuditRepository 审计日志仓储接口
type AuditRepository interface {
    // LogPermissionChange 记录权限变更日志
    LogPermissionChange(ctx context.Context, log *entity.PermissionAuditLog) error
    
    // LogResourceAccess 记录资源访问日志
    LogResourceAccess(ctx context.Context, log *entity.ResourceAccessLog) error
    
    // QueryPermissionHistory 查询权限变更历史
    QueryPermissionHistory(
        ctx context.Context,
        scopeType valueobject.ScopeType,
        scopeID string,
        startTime time.Time,
        endTime time.Time,
        limit int,
    ) ([]*entity.PermissionAuditLog, error)
    
    // QueryAccessHistory 查询资源访问历史
    QueryAccessHistory(
        ctx context.Context,
        userID string,
        resourceType string,
        startTime time.Time,
        endTime time.Time,
        limit int,
    ) ([]*entity.ResourceAccessLog, error)
    
    // QueryDeniedAccess 查询被拒绝的访问记录
    QueryDeniedAccess(
        ctx context.Context,
        startTime time.Time,
        endTime time.Time,
        limit int,
    ) ([]*entity.ResourceAccessLog, error)
}
```

#### 4.7.4 组织/项目/工作空间仓储

```go
package repository

// OrganizationRepository 组织仓储
type OrganizationRepository interface {
    GetByID(ctx context.Context, orgID string) (*entity.Organization, error)
    GetProjectsByOrg(ctx context.Context, orgID string) ([]*entity.Project, error)
}

// ProjectRepository 项目仓储
type ProjectRepository interface {
    GetByID(ctx context.Context, projectID string) (*entity.Project, error)
    GetOrgID(ctx context.Context, projectID string) (string, error)
    ListWorkspacesByProject(ctx context.Context, projectID string) ([]*entity.Workspace, error)
}

// WorkspaceRepository 工作空间仓储
type WorkspaceRepository interface {
    GetByID(ctx context.Context, workspaceID string) (*entity.Workspace, error)
    GetProjectID(ctx context.Context, workspaceID string) (string, error)
}
```

-----

## 5. API 接口设计

### 5.1 HTTP 接口

#### 5.1.1 权限检查接口

```go
// POST /api/v1/permissions/check
// 检查单个权限
type CheckPermissionRequest struct {
    UserID        string `json:"user_id" binding:"required"`
    ResourceType  string `json:"resource_type" binding:"required"`
    ScopeType     string `json:"scope_type" binding:"required"`
    ScopeID       string `json:"scope_id" binding:"required"`
    RequiredLevel string `json:"required_level" binding:"required"`
}

type CheckPermissionResponse struct {
    IsAllowed      bool     `json:"is_allowed"`
    EffectiveLevel string   `json:"effective_level"`
    DenyReason     string   `json:"deny_reason,omitempty"`
    CacheHit       bool     `json:"cache_hit"`
}

// POST /api/v1/permissions/check-batch
// 批量检查权限
type BatchCheckRequest struct {
    Checks []CheckPermissionRequest `json:"checks" binding:"required"`
}

type BatchCheckResponse struct {
    Results []CheckPermissionResponse `json:"results"`
}

// GET /api/v1/users/{user_id}/permissions
// 获取用户有效权限汇总
type UserPermissionsResponse struct {
    UserID       string                           `json:"user_id"`
    Teams        []string                         `json:"teams"`
    Organization map[string]map[string]string     `json:"organization"` // orgID -> resourceType -> level
    Projects     map[string]map[string]string     `json:"projects"`     // projectID -> resourceType -> level
    Workspaces   map[string]map[string]string     `json:"workspaces"`   // workspaceID -> resourceType -> level
}
```

#### 5.1.2 权限管理接口

```go
// POST /api/v1/permissions/grant
// 授予权限
type GrantPermissionRequest struct {
    ScopeType       string  `json:"scope_type" binding:"required"`
    ScopeID         string  `json:"scope_id" binding:"required"`
    PrincipalType   string  `json:"principal_type" binding:"required"` // TEAM/USER/APPLICATION
    PrincipalID     string  `json:"principal_id" binding:"required"`
    PermissionID    string  `json:"permission_id" binding:"required"`
    PermissionLevel string  `json:"permission_level" binding:"required"`
    ExpiresAt       *string `json:"expires_at,omitempty"`
    Reason          string  `json:"reason,omitempty"`
}

// POST /api/v1/permissions/revoke
// 撤销权限
type RevokePermissionRequest struct {
    AssignmentID string `json:"assignment_id" binding:"required"`
    Reason       string `json:"reason,omitempty"`
}

// POST /api/v1/permissions/grant-preset
// 授予预设权限集（READ/WRITE/ADMIN）
type GrantPresetRequest struct {
    ScopeType     string `json:"scope_type" binding:"required"`
    ScopeID       string `json:"scope_id" binding:"required"`
    PrincipalType string `json:"principal_type" binding:"required"`
    PrincipalID   string `json:"principal_id" binding:"required"`
    PresetName    string `json:"preset_name" binding:"required"` // READ/WRITE/ADMIN
    Reason        string `json:"reason,omitempty"`
}

// GET /api/v1/permissions/{scope_type}/{scope_id}
// 列出指定作用域的所有权限分配
type ListPermissionsResponse struct {
    Permissions []PermissionGrantDTO `json:"permissions"`
    Total       int                  `json:"total"`
}

type PermissionGrantDTO struct {
    AssignmentID    string  `json:"assignment_id"`
    PrincipalType   string  `json:"principal_type"`
    PrincipalID     string  `json:"principal_id"`
    PrincipalName   string  `json:"principal_name"`
    PermissionID    string  `json:"permission_id"`
    PermissionName  string  `json:"permission_name"`
    PermissionLevel string  `json:"permission_level"`
    GrantedBy       string  `json:"granted_by"`
    GrantedAt       string  `json:"granted_at"`
    ExpiresAt       *string `json:"expires_at,omitempty"`
}

// GET /api/v1/permissions/history
// 获取权限变更历史
type PermissionHistoryRequest struct {
    ScopeType string `form:"scope_type"`
    ScopeID   string `form:"scope_id"`
    StartTime string `form:"start_time"`
    EndTime   string `form:"end_time"`
    Limit     int    `form:"limit" binding:"max=1000"`
}

type PermissionHistoryResponse struct {
    History []PermissionAuditLogDTO `json:"history"`
    Total   int                     `json:"total"`
}
```

#### 5.1.3 团队管理接口

```go
// POST /api/v1/teams
// 创建团队
type CreateTeamRequest struct {
    OrgID       string `json:"org_id" binding:"required"`
    TeamName    string `json:"team_name" binding:"required"`
    DisplayName string `json:"display_name"`
    Description string `json:"description"`
    IsSecret    bool   `json:"is_secret"`
}

// DELETE /api/v1/teams/{team_id}
// 删除团队

// POST /api/v1/teams/{team_id}/members
// 添加团队成员
type AddTeamMemberRequest struct {
    UserID string `json:"user_id" binding:"required"`
    Role   string `json:"role" binding:"required"` // MEMBER/MAINTAINER
}

// DELETE /api/v1/teams/{team_id}/members/{user_id}
// 移除团队成员

// GET /api/v1/teams/{team_id}/members
// 列出团队成员
type ListTeamMembersResponse struct {
    Members []TeamMemberDTO `json:"members"`
    Total   int             `json:"total"`
}

type TeamMemberDTO struct {
    UserID      string `json:"user_id"`
    Username    string `json:"username"`
    DisplayName string `json:"display_name"`
    Role        string `json:"role"`
    JoinedAt    string `json:"joined_at"`
}

// GET /api/v1/users/{user_id}/teams
// 列出用户所属团队
type ListUserTeamsResponse struct {
    Teams []TeamDTO `json:"teams"`
    Total int       `json:"total"`
}

// GET /api/v1/teams/{team_id}/permissions
// 获取团队的所有权限
type TeamPermissionsResponse struct {
    Permissions []PermissionGrantDTO `json:"permissions"`
    Total       int                  `json:"total"`
}
```

#### 5.1.4 审计日志接口

```go
// GET /api/v1/audit/permissions
// 查询权限变更日志
type QueryPermissionAuditRequest struct {
    ScopeType     string `form:"scope_type"`
    ScopeID       string `form:"scope_id"`
    PrincipalType string `form:"principal_type"`
    PrincipalID   string `form:"principal_id"`
    ActionType    string `form:"action_type"` // GRANT/REVOKE/MODIFY
    StartTime     string `form:"start_time"`
    EndTime       string `form:"end_time"`
    Limit         int    `form:"limit" binding:"max=1000"`
}

// GET /api/v1/audit/access
// 查询资源访问日志
type QueryAccessAuditRequest struct {
    UserID       string `form:"user_id"`
    ResourceType string `form:"resource_type"`
    ResourceID   string `form:"resource_id"`
    IsAllowed    *bool  `form:"is_allowed"`
    StartTime    string `form:"start_time"`
    EndTime      string `form:"end_time"`
    Limit        int    `form:"limit" binding:"max=1000"`
}

// GET /api/v1/audit/denied
// 查询被拒绝的访问
type QueryDeniedAccessResponse struct {
    DeniedAccess []ResourceAccessLogDTO `json:"denied_access"`
    Total        int                    `json:"total"`
}
```

### 5.2 中间件设计

#### 5.2.1 权限检查中间件（permission_middleware.go）

```go
package middleware

// RequirePermission 权限检查中间件
// 用于保护需要权限的 API 端点
func RequirePermission(
    checker service.PermissionChecker,
    resourceType valueobject.ResourceType,
    scopeType valueobject.ScopeType,
    requiredLevel valueobject.PermissionLevel,
) gin.HandlerFunc

// 使用示例:
// router.GET("/api/workspaces/:workspace_id/tasks",
//     RequirePermission(checker, ResourceTypeTaskData, ScopeTypeWorkspace, PermissionLevelRead),
//     handler.GetTasks,
// )

// extractScopeID 从请求中提取作用域ID
// 支持: URL参数、Query参数、请求体
func extractScopeID(c *gin.Context, scopeType valueobject.ScopeType) (string, error)

// extractUserID 从上下文中提取当前用户ID
func extractUserID(c *gin.Context) (string, error)

// handlePermissionDenied 处理权限拒绝
func handlePermissionDenied(c *gin.Context, result *service.CheckPermissionResult)
```

#### 5.2.2 认证中间件（auth_middleware.go）

```go
package middleware

// AuthMiddleware 认证中间件
// 验证 JWT Token 并提取用户信息
func AuthMiddleware(jwtSecret string) gin.HandlerFunc

// AdminOnly 超级管理员专用中间件
func AdminOnly() gin.HandlerFunc

// extractToken 从请求头提取 Token
func extractToken(c *gin.Context) (string, error)

// validateToken 验证 Token 并返回用户信息
func validateToken(token string, secret string) (*UserClaims, error)

type UserClaims struct {
    UserID      string `json:"user_id"`
    Username    string `json:"username"`
    IsAdmin     bool   `json:"is_admin"`
    OrgID       string `json:"org_id"`
}
```

### 5.3 路由设计（router.go）

```go
package router

func SetupRouter(
    permissionChecker service.PermissionChecker,
    permissionService service.PermissionService,
    teamService service.TeamService,
    auditService service.AuditService,
    config *config.Config,
) *gin.Engine

// setupRoutes 配置路由
func setupRoutes(r *gin.Engine, deps *Dependencies)

// 路由分组结构:
// /api/v1
//   /permissions
//     POST   /check                     # 检查权限
//     POST   /check-batch               # 批量检查
//     POST   /grant                     # 授予权限
//     POST   /revoke                    # 撤销权限
//     POST   /grant-preset              # 授予预设权限
//     GET    /:scope_type/:scope_id     # 列出权限
//     GET    /history                   # 权限变更历史
//   /teams
//     POST   /                          # 创建团队
//     GET    /:team_id                  # 获取团队详情
//     DELETE /:team_id                  # 删除团队
//     POST   /:team_id/members          # 添加成员
//     DELETE /:team_id/members/:user_id # 移除成员
//     GET    /:team_id/members          # 列出成员
//     GET    /:team_id/permissions      # 团队权限
//   /users
//     GET    /:user_id/permissions      # 用户权限汇总
//     GET    /:user_id/teams            # 用户团队
//   /audit
//     GET    /permissions               # 权限变更日志
//     GET    /access                    # 访问日志
//     GET    /denied                    # 拒绝访问日志
```

-----

## 6. 实施路线图

### 6.1 第一阶段：核心功能（2-3周）

```yaml
Week 1-2:
  数据库:
    - 创建所有核心表
    - 初始化权限定义和预设
    - 创建必要索引
  
  服务层:
    - 实现基础实体和值对象
    - 实现仓储接口和 MySQL 实现
    - 实现权限检查器核心逻辑
    - 实现权限管理服务基础功能
  
  接口层:
    - 实现权限检查 API
    - 实现权限授予/撤销 API
    - 实现基础认证中间件

Week 3:
  测试与优化:
    - 单元测试（核心决策逻辑）
    - 集成测试（数据库交互）
    - 性能测试（权限检查响应时间）
```

### 6.2 第二阶段：团队管理（1-2周）

```yaml
Week 4-5:
  服务层:
    - 实现团队管理服务
    - 实现团队成员关系管理
    - 完善权限继承逻辑
  
  接口层:
    - 实现团队管理 API
    - 实现团队成员管理 API
    - 实现团队权限查询 API
  
  测试:
    - 团队权限继承测试
    - 团队成员变更影响测试
```

### 6.3 第三阶段：缓存与性能优化（1周）

```yaml
Week 6:
  缓存层:
    - 实现 Redis 缓存
    - 实现权限结果缓存
    - 实现缓存失效策略
  
  性能优化:
    - 批量权限检查优化
    - 数据库查询优化
    - 添加复合索引
  
  监控:
    - 添加性能指标收集
    - 添加缓存命中率监控
```

### 6.4 第四阶段：审计与监控（1周）

```yaml
Week 7:
  审计功能:
    - 实现审计日志记录
    - 实现日志查询 API
    - 实现日志分区策略
  
  监控告警:
    - 权限变更告警
    - 异常访问告警
    - 性能指标看板
```

### 6.5 第五阶段：高级功能（1-2周）

```yaml
Week 8-9:
  扩展功能:
    - 权限预设模板
    - 批量权限操作
    - 权限导入/导出
    - 权限有效期管理
  
  用户界面:
    - 权限管理后台
    - 权限可视化
    - 审计日志查询界面
```

### 6.6 部署与上线

```yaml
部署准备:
  - 准备生产环境配置
  - 数据库迁移脚本
  - 监控告警配置
  - 备份恢复方案

灰度发布:
  - 10% 流量测试（1天）
  - 50% 流量测试（2天）
  - 100% 全量上线

上线后观察:
  - 性能指标监控
  - 错误率监控
  - 缓存命中率
  - 数据库慢查询
```

-----

## 7. 关键技术点总结

### 7.1 性能优化策略

```yaml
数据库层:
  - 合理使用索引（复合索引）
  - 分区表（访问日志按月分区）
  - 连接池配置优化
  - 读写分离（读操作走从库）

缓存层:
  - Redis 缓存权限检查结果（TTL 5分钟）
  - 缓存用户团队关系（TTL 10分钟）
  - 缓存失效策略（权限变更时精准失效）
  - 缓存预热（系统启动时加载热点数据）

应用层:
  - 批量权限检查接口
  - 异步审计日志写入
  - 并发控制（限流、熔断）
  - 查询结果分页
```

### 7.2 安全考虑

```yaml
权限控制:
  - 最小权限原则
  - 显式拒绝优先（NONE > ADMIN）
  - 权限操作需要二次验证（敏感操作）
  - 应用权限仅限组织级

审计追踪:
  - 所有权限变更记录审计日志
  - 所有访问记录访问日志
  - 记录操作人、IP、时间
  - 定期审计日志分析

数据保护:
  - API Key 加密存储
  - 敏感信息脱敏
  - 数据库连接加密
  - 定期备份
```

### 7.3 可扩展性

```yaml
横向扩展:
  - 无状态服务设计
  - Redis 集群
  - 数据库读写分离
  - 负载均衡

功能扩展:
  - 插件化权限定义
  - 自定义权限规则
  - 动态权限策略
  - 第三方系统集成
```

-----

## 8. 示例场景

### 8.1 场景1：授予团队工作空间权限

```go
// 1. 给 "ml_engineers" 团队授予 workspace_001 的任务数据写权限
request := &service.GrantPermissionRequest{
    ScopeType:       valueobject.ScopeTypeWorkspace,
    ScopeID:         "workspace_001",
    PrincipalType:   entity.PrincipalTypeTeam,
    PrincipalID:     "team_ml_engineers",
    PermissionID:    "perm_task_data",
    PermissionLevel: valueobject.PermissionLevelWrite,
    GrantedBy:       "user_admin",
    Reason:          "允许ML团队访问训练数据",
}

err := permissionService.GrantPermission(ctx, request)

// 2. 团队成员 alice 自动继承该权限
checkReq := &service.CheckPermissionRequest{
    UserID:        "user_alice",
    ResourceType:  valueobject.ResourceTypeTaskData,
    ScopeType:     valueobject.ScopeTypeWorkspace,
    ScopeID:       "workspace_001",
    RequiredLevel: valueobject.PermissionLevelWrite,
}

result, _ := permissionChecker.CheckPermission(ctx, checkReq)
// result.IsAllowed = true
// result.EffectiveLevel = WRITE
```

### 8.2 场景2：使用预设权限快速授权

```go
// 给用户授予项目的 ADMIN 预设权限集
// 自动包含: PROJECT_SETTINGS(ADMIN), PROJECT_TEAMS(ADMIN), PROJECT_WORKSPACES(ADMIN)
request := &service.GrantPresetRequest{
    ScopeType:     valueobject.ScopeTypeProject,
    ScopeID:       "project_ml",
    PrincipalType: entity.PrincipalTypeUser,
    PrincipalID:   "user_bob",
    PresetName:    "ADMIN",
    GrantedBy:     "user_admin",
    Reason:        "指定为项目管理员",
}

err := permissionService.GrantPresetPermissions(ctx, request)
```

### 8.3 场景3：应用注册权限（全局）

```go
// 1. 授予应用组织级注册权限
request := &service.GrantPermissionRequest{
    ScopeType:       valueobject.ScopeTypeOrganization,
    ScopeID:         "org_001",
    PrincipalType:   entity.PrincipalTypeApplication,
    PrincipalID:     "app_external_system",
    PermissionID:    "perm_app_reg",
    PermissionLevel: valueobject.PermissionLevelWrite,
    GrantedBy:       "user_admin",
    ExpiresAt:       &expiryTime, // 1年后过期
    Reason:          "允许外部系统注册应用",
}

err := permissionService.GrantPermission(ctx, request)

// 2. 外部系统使用 API Key 调用注册接口
// 系统自动检查应用的组织级权限
```

### 8.4 场景4：权限继承与覆盖

```go
// 组织级: ml_engineers 团队有 ALL_PROJECTS 的 READ 权限
// 项目级: alice 个人有 project_ml 的 PROJECT_WORKSPACES 的 WRITE 权限
// 工作空间级: data_team 有 workspace_001 的 TASK_DATA_ACCESS 的 ADMIN 权限

// Alice 同时属于 ml_engineers 和 data_team
// 检查 alice 对 workspace_001 的任务数据权限

checkReq := &service.CheckPermissionRequest{
    UserID:        "user_alice",
    ResourceType:  valueobject.ResourceTypeTaskData,
    ScopeType:     valueobject.ScopeTypeWorkspace,
    ScopeID:       "workspace_001",
    RequiredLevel: valueobject.PermissionLevelWrite,
}

result, _ := permissionChecker.CheckPermission(ctx, checkReq)

// 权限来源:
// 1. 组织级: READ (来自 ml_engineers 团队)
// 2. 项目级: WRITE (来自 alice 直接授权)
// 3. 工作空间级: ADMIN (来自 data_team 团队)
//
// 最终有效权限: max(READ, WRITE, ADMIN) = ADMIN
// result.IsAllowed = true
// result.EffectiveLevel = ADMIN
```

### 8.5 场景5：显式拒绝

```go
// 临时禁止某个用户访问特定工作空间
request := &service.GrantPermissionRequest{
    ScopeType:       valueobject.ScopeTypeWorkspace,
    ScopeID:         "workspace_sensitive",
    PrincipalType:   entity.PrincipalTypeUser,
    PrincipalID:     "user_suspicious",
    PermissionID:    "perm_task_data",
    PermissionLevel: valueobject.PermissionLevelNone, // 显式拒绝
    GrantedBy:       "user_security",
    ExpiresAt:       &threeDaysLater,
    Reason:          "安全审查期间临时禁止访问",
}

err := permissionService.GrantPermission(ctx, request)

// 即使该用户通过团队拥有 ADMIN 权限，也会被拒绝
// NONE 的优先级最高
```

-----

## 9. 配置文件示例

### 9.1 应用配置（config.yaml）

```yaml
server:
  host: 0.0.0.0
  port: 8080
  mode: release # debug/release
  read_timeout: 30s
  write_timeout: 30s

database:
  mysql:
    host: localhost
    port: 3306
    username: permission_user
    password: ${DB_PASSWORD}
    database: permission_system
    max_open_conns: 100
    max_idle_conns: 10
    conn_max_lifetime: 3600s
    
  read_replicas: # 读写分离
    - host: replica1.example.com
      port: 3306
    - host: replica2.example.com
      port: 3306

cache:
  redis:
    host: localhost
    port: 6379
    password: ${REDIS_PASSWORD}
    db: 0
    pool_size: 100
    
  permission_cache:
    ttl: 5m              # 权限检查结果缓存时间
    user_teams_ttl: 10m  # 用户团队缓存时间
    key_prefix: "perm:"

security:
  jwt:
    secret: ${JWT_SECRET}
    token_expire: 24h
    refresh_expire: 168h
  
  api_key:
    encryption_key: ${API_KEY_ENCRYPTION}

logging:
  level: info # debug/info/warn/error
  format: json
  output: stdout
  
  audit:
    enabled: true
    async: true        # 异步写入审计日志
    buffer_size: 1000  # 缓冲区大小
    flush_interval: 10s

monitoring:
  prometheus:
    enabled: true
    port: 9090
    path: /metrics
  
  tracing:
    enabled: true
    jaeger_endpoint: http://jaeger:14268/api/traces

performance:
  rate_limit:
    enabled: true
    requests_per_second: 1000
    burst: 2000
  
  circuit_breaker:
    enabled: true
    threshold: 10           # 错误阈值
    timeout: 60s            # 熔断超时
  
  batch_check:
    max_batch_size: 100     # 批量检查最大数量
    
system:
  default_org: "default"
  system_teams:
    - "owners"
    - "admins"
```

### 9.2 环境变量（.env）

```bash
# 数据库
DB_PASSWORD=your_db_password
DB_ENCRYPTION_KEY=your_encryption_key

# Redis
REDIS_PASSWORD=your_redis_password

# JWT
JWT_SECRET=your_jwt_secret

# API Key 加密
API_KEY_ENCRYPTION=your_api_key_encryption_key

# 监控
JAEGER_ENDPOINT=http://jaeger:14268/api/traces

# 运行模式
APP_ENV=production
LOG_LEVEL=info
```

-----

## 10. 测试策略

### 10.1 单元测试

```go
// 测试权限等级计算
func TestCalculateEffectiveLevel(t *testing.T) {
    tests := []struct {
        name     string
        grants   []*entity.PermissionGrant
        expected valueobject.PermissionLevel
    }{
        {
            name: "多个权限取最高",
            grants: []*entity.PermissionGrant{
                {PermissionLevel: valueobject.PermissionLevelRead},
                {PermissionLevel: valueobject.PermissionLevelWrite},
                {PermissionLevel: valueobject.PermissionLevelRead},
            },
            expected: valueobject.PermissionLevelWrite,
        },
        {
            name: "存在NONE则拒绝",
            grants: []*entity.PermissionGrant{
                {PermissionLevel: valueobject.PermissionLevelAdmin},
                {PermissionLevel: valueobject.PermissionLevelNone},
            },
            expected: valueobject.PermissionLevelNone,
        },
        // ... 更多测试用例
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := calculateEffectiveLevel(tt.grants)
            assert.Equal(t, tt.expected, result)
        })
    }
}

// 测试权限继承
func TestPermissionInheritance(t *testing.T) {
    // 设置测试数据
    // 1. 组织级团队权限
    // 2. 项目级用户权限
    // 3. 工作空间级团队权限
    // 验证用户最终有效权限
}

// 测试缓存失效
func TestCacheInvalidation(t *testing.T) {
    // 1. 检查权限（缓存结果）
    // 2. 修改权限
    // 3. 再次检查权限（应从数据库查询）
}
```

### 10.2 集成测试

```go
// 测试完整权限检查流程
func TestPermissionCheckFlow(t *testing.T) {
    // 1. 创建测试组织、项目、工作空间
    // 2. 创建测试用户和团队
    // 3. 授予各级权限
    // 4. 验证权限检查结果
    // 5. 清理测试数据
}

// 测试团队成员变更影响
func TestTeamMembershipChange(t *testing.T) {
    // 1. 用户加入团队
    // 2. 验证用户获得团队权限
    // 3. 用户退出团队
    // 4. 验证用户失去团队权限
    // 5. 验证缓存正确失效
}

// 测试权限过期
func TestPermissionExpiration(t *testing.T) {
    // 1. 授予带过期时间的权限
    // 2. 验证权限在过期前有效
    // 3. 模拟时间推进到过期后
    // 4. 验证权限失效
}
```

### 10.3 性能测试

```go
// 并发权限检查测试
func BenchmarkPermissionCheck(b *testing.B) {
    // 模拟高并发场景
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            checker.CheckPermission(ctx, request)
        }
    })
}

// 批量检查性能测试
func BenchmarkBatchCheck(b *testing.B) {
    requests := generateBatchRequests(100)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        checker.CheckBatchPermissions(ctx, requests)
    }
}

// 缓存命中率测试
func TestCacheHitRate(t *testing.T) {
    // 1. 执行1000次相同的权限检查
    // 2. 统计缓存命中次数
    // 3. 验证命中率 > 95%
}
```

### 10.4 压力测试

```bash
# 使用 k6 进行压力测试
cat > load_test.js << 'EOF'
import http from 'k6/http';
import { check } from 'k6';

export let options = {
  stages: [
    { duration: '2m', target: 100 },  // 2分钟内增加到100并发
    { duration: '5m', target: 100 },  // 保持100并发5分钟
    { duration: '2m', target: 200 },  // 增加到200并发
    { duration: '5m', target: 200 },  // 保持200并发5分钟
    { duration: '2m', target: 0 },    // 降到0
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'], // 95%请求在500ms内完成
    http_req_failed: ['rate<0.01'],   // 错误率低于1%
  },
};

export default function () {
  const payload = JSON.stringify({
    user_id: 'user_test',
    resource_type: 'TASK_DATA_ACCESS',
    scope_type: 'WORKSPACE',
    scope_id: 'ws_001',
    required_level: 'READ',
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer test_token',
    },
  };

  let res = http.post('http://localhost:8080/api/v1/permissions/check', payload, params);
  
  check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 500ms': (r) => r.timings.duration < 500,
  });
}
EOF

k6 run load_test.js
```

-----

## 11. 监控指标

### 11.1 业务指标

```yaml
权限检查:
  - permission_check_total: 权限检查总次数
  - permission_check_allowed: 权限通过次数
  - permission_check_denied: 权限拒绝次数
  - permission_check_duration: 权限检查耗时（p50/p95/p99）
  - permission_cache_hit_rate: 缓存命中率

权限管理:
  - permission_grant_total: 权限授予次数
  - permission_revoke_total: 权限撤销次数
  - permission_modify_total: 权限修改次数
  - active_permissions_count: 活跃权限数量

团队管理:
  - team_member_add_total: 团队成员添加次数
  - team_member_remove_total: 团队成员移除次数
  - active_teams_count: 活跃团队数量
  - team_members_count: 团队成员总数

审计日志:
  - audit_log_write_total: 审计日志写入次数
  - audit_log_write_failed: 审计日志写入失败次数
  - access_log_write_total: 访问日志写入次数
```

### 11.2 技术指标

```yaml
数据库:
  - db_query_duration: 数据库查询耗时
  - db_connection_active: 活跃连接数
  - db_connection_idle: 空闲连接数
  - db_query_error_rate: 查询错误率

缓存:
  - cache_hit_count: 缓存命中次数
  - cache_miss_count: 缓存未命中次数
  - cache_operation_duration: 缓存操作耗时
  - cache_memory_usage: 缓存内存占用

系统:
  - http_request_duration: HTTP请求耗时
  - http_request_total: HTTP请求总数
  - http_request_error_rate: HTTP错误率
  - goroutine_count: Goroutine数量
  - memory_usage: 内存占用
  - cpu_usage: CPU使用率
```

### 11.3 告警规则

```yaml
高优先级告警:
  - 权限检查错误率 > 5%
  - 数据库连接失败
  - Redis连接失败
  - 服务不可用

中优先级告警:
  - 权限检查P95延迟 > 500ms
  - 缓存命中率 < 80%
  - 数据库慢查询 > 10/min
  - 内存使用率 > 80%

低优先级告警:
  - 审计日志写入延迟 > 10s
  - 权限变更频率异常
  - 异常访问模式
```

-----

## 12. 最佳实践建议

### 12.1 权限设计原则

```yaml
1. 最小权限原则:
   - 默认无权限
   - 按需授权
   - 定期审查清理过期权限

2. 团队优先:
   - 优先使用团队管理权限
   - 减少直接给用户授权
   - 便于批量管理

3. 显式优于隐式:
   - 明确定义权限边界
   - 避免权限泄露
   - 使用显式拒绝处理特殊情况

4. 层级清晰:
   - 组织 → 项目 → 工作空间
   - 权限继承关系明确
   - 避免循环依赖

5. 审计完整:
   - 记录所有权限变更
   - 记录所有访问行为
   - 定期审计分析
```

### 12.2 性能优化建议

```yaml
1. 缓存策略:
   - 高频权限检查结果缓存
   - 用户团队关系缓存
   - 权限变更时精准失效缓存

2. 数据库优化:
   - 合理使用索引
   - 避免N+1查询
   - 使用批量操作
   - 读写分离

3. 批量操作:
   - 提供批量权限检查接口
   - 批量授予/撤销权限
   - 减少数据库往返次数

4. 异步处理:
   - 审计日志异步写入
   - 缓存失效异步处理
   - 非核心操作后台执行

5. 限流保护:
   - API 限流
   - 数据库连接池
   - Redis 连接池
```

### 12.3 安全建议

```yaml
1. 敏感操作:
   - 权限授予需要二次确认
   - 关键权限变更需要审批
   - 超级管理员操作记录

2. 数据保护:
   - API Key 加密存储
   - 传输数据加密（HTTPS）
   - 敏感信息脱敏

3. 防御措施:
   - 防止权限提升攻击
   - 防止 SQL 注入
   - 防止缓存穿透/击穿

4. 审计追溯:
   - 完整的操作日志
   - IP 和 User-Agent 记录
   - 异常行为告警

5. 定期审查:
   - 定期清理过期权限
   - 定期审查高权限用户
   - 定期安全扫描
```

-----

## 13. 总结

本权限系统设计方案基于 Terraform Enterprise 的成熟三层权限模型，具有以下特点：

### 核心优势

 **清晰的层级结构**：组织 → 项目 → 工作空间，权限继承关系明确  
 **团队优先管理**：通过团队批量管理权限，减少运维成本  
 **灵活的权限控制**：支持全局/局部权限，支持显式拒绝  
 **完整的审计追溯**：所有权限变更和访问行为可追溯  
 **高性能设计**：多级缓存、批量操作、异步处理  
 **易于扩展**：模块化设计，支持自定义权限规则

### 技术栈

- **语言**：Golang
- **数据库**：MySQL（支持读写分离）
- **缓存**：Redis
- **监控**：Prometheus + Grafana
- **日志**：结构化日志 + ELK

### 实施建议

建议按照分阶段实施：先实现核心权限检查功能，再完善团队管理和缓存优化，最后添加审计和监控。整个项目预计 8-10 周完成。​​​​​​​​​​​​​​​​
