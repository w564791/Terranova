# 工作空间精细化权限优先级设计

## 概述

在workspace_management统一权限的基础上，保留精细化权限选项，并让精细化权限具有更高的优先级。

## 权限优先级规则

### 优先级顺序（从高到低）

1. **精细化工作空间权限**（最高优先级）
   - TASK_DATA_ACCESS
   - WORKSPACE_EXECUTION
   - WORKSPACE_STATE
   - WORKSPACE_VARIABLES
   - WORKSPACE_RESOURCES

2. **统一工作空间权限**（次优先级）
   - WORKSPACE_MANAGEMENT

### 工作原理

**场景1：只有workspace_management权限**
- 用户拥有：workspace_management WRITE
- 结果：可以执行所有WRITE级别的操作

**场景2：同时拥有精细化权限和workspace_management**
- 用户拥有：workspace_management WRITE + workspace_execution READ
- 对于任务操作：使用workspace_execution READ（精细化权限优先）
- 对于变量操作：使用workspace_management WRITE（无精细化权限时使用统一权限）

**场景3：只有精细化权限**
- 用户拥有：workspace_execution WRITE + workspace_variables READ
- 对于任务操作：使用workspace_execution WRITE
- 对于变量操作：使用workspace_variables READ
- 对于资源操作：无权限（403）

## 实现方案

### 方案A：在路由层面实现（推荐）

修改路由权限检查，使用`RequireAnyPermission`支持多个权限选项：

```go
// 任务查看：优先使用workspace_execution READ，其次使用workspace_management READ
workspaces.GET("/:id/tasks", func(c *gin.Context) {
    role, _ := c.Get("role")
    if role == "admin" {
        taskController.GetTasks(c)
        return
    }
    iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
        {ResourceType: "workspace_execution", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
        {ResourceType: "workspace_management", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
    })(c)
    if !c.IsAborted() {
        taskController.GetTasks(c)
    }
})
```

### 方案B：在权限检查器层面实现

修改`calculateEffectiveLevel`函数，让精细化权限优先于workspace_management。

## 权限映射表

### 任务操作
| 操作 | 精细化权限 | 统一权限 |
|------|-----------|---------|
| 查看任务 | workspace_execution READ | workspace_management READ |
| 创建Plan | workspace_execution WRITE | workspace_management WRITE |
| 取消任务 | workspace_execution ADMIN | workspace_management ADMIN |

### 变量操作
| 操作 | 精细化权限 | 统一权限 |
|------|-----------|---------|
| 查看变量 | workspace_variables READ | workspace_management READ |
| 创建变量 | workspace_variables WRITE | workspace_management WRITE |
| 删除变量 | workspace_variables ADMIN | workspace_management WRITE |

### State操作
| 操作 | 精细化权限 | 统一权限 |
|------|-----------|---------|
| 查看State | workspace_state READ | workspace_management READ |
| 回滚State | workspace_state WRITE | workspace_management WRITE |
| 删除State | workspace_state ADMIN | workspace_management ADMIN |

### 资源操作
| 操作 | 精细化权限 | 统一权限 |
|------|-----------|---------|
| 查看资源 | workspace_resources READ | workspace_management READ |
| 创建资源 | workspace_resources WRITE | workspace_management WRITE |
| 删除资源 | workspace_resources ADMIN | workspace_management WRITE |

## 使用场景

### 场景1：简单授权（推荐给大多数用户）
只授予workspace_management权限，简单明了：
- READ：只读用户
- WRITE：开发者/运维
- ADMIN：管理员

### 场景2：精细化控制（高级用户）
为需要特殊权限控制的用户授予精细化权限：
- 只能查看任务但不能执行：workspace_execution READ
- 只能管理变量：workspace_variables WRITE
- 只能查看资源：workspace_resources READ

### 场景3：混合授权
基础权限 + 特殊限制：
- workspace_management WRITE（基础开发者权限）
- workspace_execution READ（限制：只能查看任务，不能创建）

## 优势

1. **向后兼容**：现有的精细化权限继续有效
2. **灵活性**：支持简单和复杂两种授权模式
3. **优先级清晰**：精细化权限优先，避免权限冲突
4. **易于理解**：精细化权限 > 统一权限的规则简单明了

## 实施建议

采用**方案A：在路由层面实现**，因为：
1. 实现简单，只需修改路由配置
2. 权限检查逻辑清晰可见
3. 不影响现有的权限检查器逻辑
4. 易于维护和调试

## 下一步

如果确认采用此方案，需要：
1. 修改所有工作空间路由，使用`RequireAnyPermission`支持精细化权限
2. 更新文档说明优先级规则
3. 更新前端权限管理页面，说明两种授权模式
4. 测试验证精细化权限优先级
