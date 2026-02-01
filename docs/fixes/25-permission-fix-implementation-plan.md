# 权限修复实施计划

## 📋 实施概览

**开始时间**: 2025-10-24  
**预计完成**: 分阶段实施  
**总计缺失**: 99个路由需要添加权限检查

---

##  已完成 (2/99)

### Phase 1: Workspaces相关 (2个) -  完成

| 路由 | 方法 | 权限ID | 状态 |
|------|------|--------|------|
| `/workspaces/form-data` | GET | WORKSPACES.ORGANIZATION.READ |  已修复 |
| `/workspaces` | POST | WORKSPACES.ORGANIZATION.WRITE |  已修复 |

**修复代码**:
```go
workspaces.GET("/form-data", func(c *gin.Context) {
    role, _ := c.Get("role")
    if role == "admin" {
        helperController.GetWorkspaceFormData(c)
        return
    }
    iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "READ")(c)
    if !c.IsAborted() {
        helperController.GetWorkspaceFormData(c)
    }
})

workspaces.POST("", func(c *gin.Context) {
    role, _ := c.Get("role")
    if role == "admin" {
        workspaceController.CreateWorkspace(c)
        return
    }
    iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "WRITE")(c)
    if !c.IsAborted() {
        workspaceController.CreateWorkspace(c)
    }
})
```

---

## 🔄 待实施 (97/99)

### Phase 2: User相关 (1个) - 优先级: 中

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/user/reset-password` | POST | USER_MANAGEMENT | USER | WRITE |

**实施建议**:
```go
user := protected.Group("/user")
{
    authHandler := handlers.NewAuthHandler(db)
    user.POST("/reset-password", func(c *gin.Context) {
        role, _ := c.Get("role")
        if role == "admin" {
            authHandler.ResetPassword(c)
            return
        }
        iamMiddleware.RequirePermission("USER_MANAGEMENT", "USER", "WRITE")(c)
        if !c.IsAborted() {
            authHandler.ResetPassword(c)
        }
    })
}
```

### Phase 3: Demos相关 (7个) - 优先级: 中

所有Demo路由建议使用 `MODULE_DEMOS.ORGANIZATION` 权限。

**实施模式**:
```go
demos := protected.Group("/demos")
{
    demoController := controllers.NewModuleDemoController(db)
    
    // READ权限
    demos.GET("/:id", func(c *gin.Context) {
        role, _ := c.Get("role")
        if role == "admin" {
            demoController.GetDemoByID(c)
            return
        }
        iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "READ")(c)
        if !c.IsAborted() {
            demoController.GetDemoByID(c)
        }
    })
    
    // WRITE权限
    demos.PUT("/:id", func(c *gin.Context) {
        role, _ := c.Get("role")
        if role == "admin" {
            demoController.UpdateDemo(c)
            return
        }
        iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "WRITE")(c)
        if !c.IsAborted() {
            demoController.UpdateDemo(c)
        }
    })
    
    // ADMIN权限
    demos.DELETE("/:id", func(c *gin.Context) {
        role, _ := c.Get("role")
        if role == "admin" {
            demoController.DeleteDemo(c)
            return
        }
        iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "ADMIN")(c)
        if !c.IsAborted() {
            demoController.DeleteDemo(c)
        }
    })
    
    // 其他路由类似...
}
```

### Phase 4: Schemas相关 (2个) - 优先级: 低

| 路由 | 方法 | 建议权限ID | 建议作用域 | 建议级别 |
|------|------|------------|------------|----------|
| `/schemas/:id` | GET | SCHEMAS | ORGANIZATION | READ |
| `/schemas/:id` | PUT | SCHEMAS | ORGANIZATION | WRITE |

### Phase 5: Tasks相关 (4个) - 优先级: 中

所有Task日志路由建议使用 `TASK_LOGS.ORGANIZATION.READ` 权限。

**实施模式**:
```go
taskLogController := controllers.NewTaskLogController(db)
outputController := controllers.NewTerraformOutputController(streamManager)

api.GET("/tasks/:task_id/output/stream", middleware.JWTAuth(), func(c *gin.Context) {
    role, _ := c.Get("role")
    if role == "admin" {
        outputController.StreamTaskOutput(c)
        return
    }
    iamMiddleware.RequirePermission("TASK_LOGS", "ORGANIZATION", "READ")(c)
    if !c.IsAborted() {
        outputController.StreamTaskOutput(c)
    }
})

// 其他3个路由类似...
```

### Phase 6: Agents相关 (8个) - 优先级: 高

所有Agent路由建议使用 `AGENTS.ORGANIZATION` 权限。

**权限级别分配**:
- READ: `GET /agents`, `GET /agents/:id`
- WRITE: `POST /agents/register`, `POST /agents/heartbeat`, `PUT /agents/:id`
- ADMIN: `DELETE /agents/:id`, `POST /agents/:id/revoke-token`, `POST /agents/:id/regenerate-token`

### Phase 7: Agent Pools相关 (7个) - 优先级: 高

所有Agent Pool路由建议使用 `AGENT_POOLS.ORGANIZATION` 权限。

**权限级别分配**:
- READ: `GET /agent-pools`, `GET /agent-pools/:id`
- WRITE: `POST /agent-pools`, `PUT /agent-pools/:id`, `POST /agent-pools/:id/agents`, `DELETE /agent-pools/:id/agents/:agent_id`
- ADMIN: `DELETE /agent-pools/:id`

### Phase 8: IAM相关 (51个) - 优先级: 高

IAM路由需要细粒度权限控制，建议分模块实施：

#### 8.1 权限管理 (7个)
- 权限ID: `IAM_PERMISSIONS.ORGANIZATION`
- READ: check, list, definitions
- ADMIN: grant, batch-grant, grant-preset, revoke

#### 8.2 团队管理 (7个)
- 权限ID: `IAM_TEAMS.ORGANIZATION`
- READ: list, get, list members
- WRITE: create, add member, remove member
- ADMIN: delete

#### 8.3 组织管理 (4个)
- 权限ID: `IAM_ORGANIZATIONS.ORGANIZATION`
- READ: list, get
- WRITE: update
- ADMIN: create

#### 8.4 项目管理 (5个)
- 权限ID: `IAM_PROJECTS.ORGANIZATION`
- READ: list, get
- WRITE: create, update
- ADMIN: delete

#### 8.5 应用管理 (6个)
- 权限ID: `IAM_APPLICATIONS.ORGANIZATION`
- READ: list, get
- WRITE: create, update
- ADMIN: delete, regenerate-secret

#### 8.6 审计日志 (7个)
- 权限ID: `IAM_AUDIT.ORGANIZATION`
- READ: 所有查询操作
- ADMIN: config update

#### 8.7 用户管理 (8个)
- 权限ID: `IAM_USERS.ORGANIZATION`
- READ: stats, list, get, list roles
- WRITE: update
- ADMIN: assign role, revoke role, activate, deactivate

#### 8.8 角色管理 (7个)
- 权限ID: `IAM_ROLES.ORGANIZATION`
- READ: list, get
- WRITE: create, update, add policy, remove policy
- ADMIN: delete

### Phase 9: Admin - Terraform版本管理 (7个) - 优先级: 高

- 权限ID: `TERRAFORM_VERSIONS.ORGANIZATION`
- READ: list, get, get default
- WRITE: create, update
- ADMIN: delete, set-default

### Phase 10: Admin - AI配置管理 (9个) - 优先级: 高

- 权限ID: `AI_CONFIGS.ORGANIZATION`
- READ: list, get, get regions, get models
- WRITE: create, update, update priorities
- ADMIN: delete, set-default

### Phase 11: AI分析 (1个) - 优先级: 低

- 权限ID: `AI_ANALYSIS.ORGANIZATION.WRITE`

---

## 📊 实施进度

| Phase | 模块 | 路由数 | 优先级 | 状态 |
|-------|------|--------|--------|------|
| 1 | Workspaces | 2 | 高 |  完成 |
| 2 | User | 1 | 中 | ⏳ 待实施 |
| 3 | Demos | 7 | 中 | ⏳ 待实施 |
| 4 | Schemas | 2 | 低 | ⏳ 待实施 |
| 5 | Tasks | 4 | 中 | ⏳ 待实施 |
| 6 | Agents | 8 | 高 | ⏳ 待实施 |
| 7 | Agent Pools | 7 | 高 | ⏳ 待实施 |
| 8 | IAM | 51 | 高 | ⏳ 待实施 |
| 9 | Terraform | 7 | 高 | ⏳ 待实施 |
| 10 | AI Configs | 9 | 高 | ⏳ 待实施 |
| 11 | AI Analysis | 1 | 低 | ⏳ 待实施 |
| **总计** | | **99** | | **2/99 (2%)** |

---

## 🎯 实施策略

### 1. 代码模式

所有权限修复遵循统一模式：

```go
routeGroup.METHOD("/path", func(c *gin.Context) {
    // 1. 检查admin角色
    role, _ := c.Get("role")
    if role == "admin" {
        controller.Handler(c)
        return
    }
    
    // 2. IAM权限检查
    iamMiddleware.RequirePermission("RESOURCE_TYPE", "SCOPE_TYPE", "LEVEL")(c)
    
    // 3. 如果权限检查通过，执行业务逻辑
    if !c.IsAborted() {
        controller.Handler(c)
    }
})
```

### 2. 权限级别映射

| HTTP方法 | 通常权限级别 |
|----------|-------------|
| GET | READ |
| POST (create) | WRITE |
| PUT/PATCH | WRITE |
| DELETE | ADMIN |
| POST (dangerous) | ADMIN |

### 3. 测试策略

每个Phase完成后需要测试：
1. Admin用户可以访问所有路由
2. 有权限的非Admin用户可以访问
3. 无权限的用户被拒绝（403）
4. 未认证用户被拒绝（401）

---

## 📝 下一步行动

1. **立即**: 继续实施Phase 2-5（优先级中等的路由）
2. **短期**: 实施Phase 6-10（优先级高的路由）
3. **中期**: 完成Phase 11，并进行全面测试
4. **长期**: 考虑移除Admin绕过机制，完全使用IAM权限

---

##  验收标准

- [ ] 所有99个缺失权限的路由都已添加IAM权限检查
- [ ] Admin用户可以访问所有路由
- [ ] 非Admin用户根据IAM权限访问
- [ ] 所有权限检查都有审计日志
- [ ] 更新权限ID清单文档
- [ ] 编写权限测试用例

---

**文档维护**: 每完成一个Phase，更新此文档的进度
