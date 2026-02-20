# IAM Role系统使用指南

## 概述

IAM Role系统类似于AWS IAM Role的概念，提供了一种灵活的权限管理方式。通过Role，可以将多个权限策略打包在一起，然后将Role分配给用户，而不是逐个授予权限。

## 核心概念

### 1. IAM Role（角色）
- 角色是一组权限策略的集合
- 可以在不同的作用域（组织/项目/工作空间）分配给用户
- 支持系统预定义角色和自定义角色

### 2. Role Policy（角色策略）
- 定义角色包含哪些权限
- 每个策略指定：权限ID、权限级别（READ/WRITE/ADMIN）、适用的作用域类型

### 3. User Role Assignment（用户角色分配）
- 将角色分配给用户
- 指定分配的作用域和作用域ID
- 支持设置过期时间

## 系统预定义角色

### 1. admin（超级管理员）
- **权限范围**：所有权限的ADMIN级别
- **适用场景**：系统管理员，拥有完全控制权
- **作用域**：组织/项目/工作空间

### 2. org_admin（组织管理员）
- **权限范围**：组织级别的所有资源管理权限
- **包含权限**：
  - ORGANIZATION（组织管理）- ADMIN
  - PROJECTS（项目管理）- ADMIN
  - WORKSPACES（工作空间管理）- ADMIN
  - MODULES（模块管理）- ADMIN
- **适用场景**：组织管理员
- **作用域**：组织

### 3. project_admin（项目管理员）
- **权限范围**：项目级别的资源管理权限
- **包含权限**：
  - WORKSPACES（工作空间管理）- ADMIN
- **适用场景**：项目管理员
- **作用域**：项目

### 4. workspace_admin（工作空间管理员）
- **权限范围**：工作空间的完全管理权限
- **包含权限**：
  - WORKSPACE_MANAGEMENT - ADMIN
  - WORKSPACE_EXECUTION - ADMIN
  - WORKSPACE_STATE - ADMIN
  - WORKSPACE_VARIABLES - ADMIN
  - WORKSPACE_RESOURCES - ADMIN
  - TASK_DATA_ACCESS - ADMIN
- **适用场景**：工作空间管理员
- **作用域**：工作空间

### 5. developer（开发者）
- **权限范围**：工作空间的开发权限
- **包含权限**：
  - WORKSPACE_MANAGEMENT - WRITE
  - WORKSPACE_EXECUTION - WRITE
  - WORKSPACE_VARIABLES - WRITE
  - WORKSPACE_RESOURCES - WRITE
  - WORKSPACE_STATE - READ
  - TASK_DATA_ACCESS - READ
- **适用场景**：开发人员，可以创建和管理工作空间、执行任务
- **作用域**：工作空间

### 6. viewer（查看者）
- **权限范围**：所有资源的只读权限
- **包含权限**：所有权限的READ级别
- **适用场景**：只需要查看资源的用户
- **作用域**：组织/项目/工作空间

## 使用方法

### 1. 初始化IAM Role系统

```bash
# 执行SQL脚本创建表和预定义角色
psql -U postgres -d iac_platform -f scripts/create_iam_roles.sql
```

这个脚本会：
- 创建必要的数据库表
- 创建6个系统预定义角色
- 为每个角色配置相应的权限策略
- 自动为现有的admin用户分配超级管理员角色

### 2. 为用户分配角色

#### 方法1：通过SQL直接分配

```sql
-- 为用户分配组织管理员角色
INSERT INTO iam_user_roles (user_id, role_id, scope_type, scope_id, reason)
SELECT 
    2,  -- 用户ID
    id, -- 角色ID
    'ORGANIZATION',
    1,  -- 组织ID
    '分配组织管理员权限'
FROM iam_roles WHERE name = 'org_admin';

-- 为用户分配工作空间开发者角色
INSERT INTO iam_user_roles (user_id, role_id, scope_type, scope_id, reason)
SELECT 
    3,  -- 用户ID
    id, -- 角色ID
    'WORKSPACE',
    10, -- 工作空间ID
    '分配开发者权限'
FROM iam_roles WHERE name = 'developer';
```

#### 方法2：通过API分配（待实现）

```bash
# POST /api/iam/users/{user_id}/roles
curl -X POST http://localhost:8080/api/iam/users/2/roles \
  -H "Content-Type: application/json" \
  -d '{
    "role_name": "org_admin",
    "scope_type": "ORGANIZATION",
    "scope_id": 1,
    "reason": "分配组织管理员权限"
  }'
```

### 3. 创建自定义角色

```sql
-- 1. 创建自定义角色
INSERT INTO iam_roles (name, display_name, description, is_system)
VALUES ('custom_role', '自定义角色', '根据业务需求定制的角色', FALSE);

-- 2. 为角色添加权限策略
INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
SELECT 
    (SELECT id FROM iam_roles WHERE name = 'custom_role'),
    id,
    'WRITE',
    'WORKSPACE'
FROM iam_permission_definitions
WHERE resource_type IN ('WORKSPACE_EXECUTION', 'WORKSPACE_RESOURCES');
```

### 4. 查询用户的角色和权限

```sql
-- 查询用户的所有角色
SELECT 
    u.username,
    r.display_name as role_name,
    ur.scope_type,
    ur.scope_id,
    ur.assigned_at
FROM iam_user_roles ur
JOIN users u ON ur.user_id = u.id
JOIN iam_roles r ON ur.role_id = r.id
WHERE u.id = 2;

-- 查询角色包含的所有权限
SELECT 
    r.display_name as role_name,
    pd.display_name as permission_name,
    rp.permission_level,
    rp.scope_type
FROM iam_role_policies rp
JOIN iam_roles r ON rp.role_id = r.id
JOIN iam_permission_definitions pd ON rp.permission_id = pd.id
WHERE r.name = 'developer';
```

## 权限检查逻辑

当用户访问资源时，系统会：

1. **检查用户的直接权限授予**
2. **检查用户的角色权限**（新增）
3. **按作用域层级继承**（工作空间 > 项目 > 组织）
4. **取最高权限级别**

### 示例

用户A在工作空间W1有以下权限来源：
- 直接授予：WORKSPACE_EXECUTION - READ
- developer角色：WORKSPACE_EXECUTION - WRITE
- 最终有效权限：WRITE（取最高）

## 迁移现有系统

### 步骤1：执行初始化脚本

```bash
psql -U postgres -d iac_platform -f scripts/create_iam_roles.sql
```

### 步骤2：验证admin用户已分配角色

```sql
SELECT 
    u.username,
    r.display_name,
    ur.scope_type,
    ur.scope_id
FROM iam_user_roles ur
JOIN users u ON ur.user_id = u.id
JOIN iam_roles r ON ur.role_id = r.id
WHERE u.role = 'admin';
```

### 步骤3：更新权限检查逻辑

需要修改 `backend/internal/application/service/permission_checker.go`，在权限检查时同时考虑：
- 直接授予的权限
- 通过角色获得的权限

### 步骤4：移除role bypass逻辑

在确认所有admin用户都已分配超级管理员角色后，可以逐步移除代码中的 `if role == "admin"` bypass逻辑。

## 最佳实践

### 1. 使用角色而非直接授权
-  推荐：为用户分配"developer"角色
- ❌ 不推荐：逐个授予WORKSPACE_EXECUTION、WORKSPACE_RESOURCES等权限

### 2. 按职责划分角色
- 为不同的工作职责创建对应的角色
- 例如：运维角色、测试角色、审计角色等

### 3. 最小权限原则
- 只授予用户完成工作所需的最小权限
- 使用viewer角色作为默认角色

### 4. 定期审计
- 定期检查用户的角色分配
- 及时回收不再需要的权限

### 5. 使用过期时间
- 为临时权限设置过期时间
- 例如：临时授予某个项目的访问权限

## 与AWS IAM的对比

| 特性 | AWS IAM | 本系统 |
|------|---------|--------|
| Role概念 |  |  |
| Policy概念 |  | （Role Policy） |
| 作用域 | Account/Resource | Organization/Project/Workspace |
| 权限继承 |  |  |
| 临时凭证 |  | ⏳（计划中） |
| 条件策略 |  | ⏳（计划中） |

## 常见问题

### Q1: 如何快速创建超级管理员？
```sql
-- 为用户分配admin角色
INSERT INTO iam_user_roles (user_id, role_id, scope_type, scope_id, reason)
SELECT 
    <user_id>,
    id,
    'ORGANIZATION',
    1,
    '创建超级管理员'
FROM iam_roles WHERE name = 'admin';
```

### Q2: 角色和直接授权有什么区别？
- **角色**：一组权限的集合，便于批量管理
- **直接授权**：单个权限的授予，更灵活但管理复杂

### Q3: 可以同时使用角色和直接授权吗？
可以。系统会合并两种方式的权限，取最高权限级别。

### Q4: 如何撤销用户的角色？
```sql
DELETE FROM iam_user_roles 
WHERE user_id = <user_id> AND role_id = (SELECT id FROM iam_roles WHERE name = '<role_name>');
```

### Q5: 系统预定义角色可以修改吗？
系统预定义角色（is_system=true）不能删除，但可以修改其包含的权限策略。

## 下一步计划

1.  创建数据库schema和预定义角色
2. ⏳ 更新权限检查逻辑以支持角色
3. ⏳ 创建角色管理API
4. ⏳ 创建前端角色管理界面
5. ⏳ 移除role bypass逻辑
6. ⏳ 添加角色审计日志

## 参考资料

- [AWS IAM Roles](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles.html)
- [RBAC (Role-Based Access Control)](https://en.wikipedia.org/wiki/Role-based_access_control)
