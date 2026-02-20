# 权限精细度优先级系统

## 概述

权限检查应该遵循"精细度优先"原则：更精细的权限优先于更粗粒度的权限。

## 优先级层级

### 1. 作用域优先级（Scope Priority）

从高到低：
1. **WORKSPACE** - 最精细，只影响单个工作空间
2. **PROJECT** - 中等，影响项目下的所有工作空间
3. **ORGANIZATION** - 最粗，影响整个组织

### 2. 资源类型优先级（Resource Type Priority）

在同一作用域内，从高到低：

**工作空间级别：**
1. **WORKSPACE_VARIABLES** - 只管理变量
2. **WORKSPACE_STATE** - 只管理State
3. **WORKSPACE_RESOURCES** - 只管理资源
4. **WORKSPACE_EXECUTION** - 只管理任务执行
5. **TASK_DATA_ACCESS** - 只访问任务数据
6. **WORKSPACE_MANAGEMENT** - 管理整个工作空间（最粗）

**组织级别：**
1. **MODULES** - 只管理模块
2. **PROJECTS** - 只管理项目
3. **WORKSPACES** - 只管理工作空间列表
4. **ORGANIZATION** - 管理整个组织（最粗）

## 权限检查逻辑

### 当前实现（需要优化）

```go
// 当前：只检查单一权限类型
RequirePermission("workspace_management", "WORKSPACE", "WRITE")

// 问题：如果用户只有workspace_variables权限，会被拒绝
```

### 优化后的实现

```go
// 优化：检查多个权限，精细权限优先
RequireAnyPermission([]PermissionRequirement{
    {ResourceType: "workspace_variables", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"}, // 精细权限优先
    {ResourceType: "workspace_management", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"}, // 粗粒度权限
})
```

## 实现策略

### 方案1：修改RequireAnyPermission逻辑

让`RequireAnyPermission`真正实现"有任意一个权限就允许"：

```go
func (m *IAMPermissionMiddleware) RequireAnyPermission(permissions []PermissionRequirement) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 检查每个权限
        for _, perm := range permissions {
            result, err := m.permissionChecker.CheckPermission(ctx, req)
            if err == nil && result.IsAllowed {
                // 有一个权限满足，允许访问
                c.Next()
                return
            }
        }
        // 所有权限都不满足，拒绝访问
        c.JSON(403, ...)
        c.Abort()
    }
}
```

### 方案2：修改calculateEffectiveLevel逻辑

在计算有效权限时，考虑资源类型的精细度：

```go
func (c *PermissionCheckerImpl) calculateEffectiveLevel(grants []*entity.PermissionGrant) valueobject.PermissionLevel {
    // 1. 按作用域分组
    workspaceGrants := filterByScope(grants, WORKSPACE)
    projectGrants := filterByScope(grants, PROJECT)
    orgGrants := filterByScope(grants, ORGANIZATION)
    
    // 2. 在每个作用域内，按资源类型精细度排序
    // 精细权限优先
    if len(workspaceGrants) > 0 {
        return maxLevelByGranularity(workspaceGrants)
    }
    
    if len(projectGrants) > 0 {
        return maxLevel(projectGrants)
    }
    
    if len(orgGrants) > 0 {
        return maxLevel(orgGrants)
    }
    
    return NONE
}
```

## 建议的实现

**推荐方案1**：修改`RequireAnyPermission`

原因：
1. 更简单直接
2. 不影响现有的权限计算逻辑
3. 符合"有任意一个权限就允许"的语义

## 示例

**场景：**
- ken有WORKSPACE_VARIABLES WRITE权限
- 路由要求：workspace_variables WRITE 或 workspace_management WRITE

**当前行为：**
- 检查workspace_management → 没有 → 拒绝

**优化后行为：**
- 检查workspace_variables → 有 → 允许
- 不需要检查workspace_management

## 下一步

1. 修改`RequireAnyPermission`实现
2. 确保真正实现"OR"逻辑
3. 测试ken的变量管理权限
