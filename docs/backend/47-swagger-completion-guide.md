# Swagger API文档完成指南

## 当前完成状态

###  已完成 (42个API - 31%)

1. **Auth Handler** (6个API) 
2. **Workspace Controller** (7个API) 
3. **Terraform Version Controller** (7个API) 
4. **Module Controller** (8个API) 
5. **Schema Controller** (5个API) 
6. **Dashboard Controller** (2个API) 
7. **Workspace Variable Controller** (5个API) 
8. **Workspace Helper Controller** (1个API) 
9. **Health Check** (1个API) 

### ⏳ 待完成 (93个API - 69%)

## 快速完成剩余API的方法

### 方法1：使用模板批量添加

为每个controller的函数添加Swagger注解，使用以下模板：

#### GET列表模板
```go
// @Summary 获取XXX列表
// @Description 获取XXX列表，支持分页和过滤
// @Tags XXX
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param size query int false "每页数量" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/xxx [get]
// @Security Bearer
```

#### GET单个模板
```go
// @Summary 获取XXX详情
// @Description 根据ID获取XXX详情
// @Tags XXX
// @Accept json
// @Produce json
// @Param id path int true "XXX ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/xxx/{id} [get]
// @Security Bearer
```

#### POST模板
```go
// @Summary 创建XXX
// @Description 创建新的XXX
// @Tags XXX
// @Accept json
// @Produce json
// @Param request body object true "XXX信息"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/xxx [post]
// @Security Bearer
```

#### PUT模板
```go
// @Summary 更新XXX
// @Description 更新XXX信息
// @Tags XXX
// @Accept json
// @Produce json
// @Param id path int true "XXX ID"
// @Param request body object true "更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/xxx/{id} [put]
// @Security Bearer
```

#### DELETE模板
```go
// @Summary 删除XXX
// @Description 删除指定的XXX
// @Tags XXX
// @Accept json
// @Produce json
// @Param id path int true "XXX ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/xxx/{id} [delete]
// @Security Bearer
```

### 方法2：按优先级逐个完成

#### 第一优先级（核心业务）

**Agent Controller** (8个API)
- RegisterAgent
- ListAgents
- GetAgent
- UpdateAgent
- DeleteAgent
- Heartbeat
- RevokeToken
- RegenerateToken

**Agent Pool Controller** (7个API)
- CreatePool
- ListPools
- GetPool
- UpdatePool
- DeletePool
- AddAgent
- RemoveAgent

**AI Controller** (9个API)
- ListConfigs
- GetConfig
- CreateConfig
- UpdateConfig
- DeleteConfig
- GetAvailableModels
- GetAvailableRegions
- AnalyzeError
- GetTaskAnalysis

#### 第二优先级（辅助功能）

**Module Demo Controller** (7个API)
- CreateDemo
- GetDemosByModuleID
- GetDemoByID
- UpdateDemo
- DeleteDemo
- GetVersionsByDemoID
- GetVersionByID
- CompareVersions
- RollbackToVersion

**State Version Controller** (7个API)
- GetStateVersions
- GetStateVersionMetadata
- GetStateVersion
- GetCurrentState
- RollbackState
- CompareVersions
- DeleteStateVersion

**Task Log Controller** (4个API)
- GetTaskLogs
- DownloadTaskLogs

**Terraform Output Controller** (2个API)
- StreamTaskOutput
- GetStreamStats

#### 第三优先级（大型controllers）

**Workspace Task Controller** (~15个API)
- CreatePlanTask
- CreateApplyTask
- GetTask
- GetTasks
- GetTaskLogs
- ConfirmApply
- CancelPreviousTasks
- CancelTask
- RetryStateSave
- DownloadStateBackup
- CreateComment
- GetComments

**Resource Controller** (~30个API)
- 资源CRUD (6个)
- 资源版本管理 (4个)
- 快照管理 (5个)
- 资源依赖 (2个)
- 资源导入部署 (2个)
- 资源编辑协作 (8个)

**Workspace Task Resource Controller** (4个API)
- GetTaskResourceChanges
- UpdateResourceApplyStatus

## 执行步骤

### 步骤1：为单个controller添加注解

1. 打开controller文件
2. 在每个函数前添加Swagger注解
3. 确保Router路径正确
4. 保存文件

### 步骤2：生成Swagger文档

```bash
cd backend
swag init -g main.go --output docs --parseDependency --parseInternal
```

### 步骤3：验证文档

```bash
# 启动服务
go run main.go

# 访问Swagger UI
open http://localhost:8080/swagger/index.html
```

### 步骤4：测试API

在Swagger UI中测试每个API端点，确保：
- 参数正确
- 响应格式正确
- 错误处理正确

## 注意事项

1. **Router路径**：确保@Router注解中的路径与router.go中定义的路径一致
2. **参数类型**：path参数用`path`，query参数用`query`，body参数用`body`
3. **Tags分组**：使用有意义的Tags对API进行分组
4. **Security**：需要认证的API添加`@Security Bearer`

## 完成检查清单

- [ ] Agent Controller (8个API)
- [ ] Agent Pool Controller (7个API)
- [ ] AI Controller (9个API)
- [ ] Module Demo Controller (7个API)
- [ ] State Version Controller (7个API)
- [ ] Task Log Controller (4个API)
- [ ] Terraform Output Controller (2个API)
- [ ] Workspace Task Controller (15个API)
- [ ] Resource Controller (30个API)
- [ ] Workspace Task Resource Controller (4个API)

## 预计完成时间

- 简单controller（<10个API）：15-30分钟
- 中等controller（10-20个API）：30-60分钟
- 复杂controller（>20个API）：1-2小时

总计：约4-6小时可完成所有剩余API的Swagger注解

## 参考资源

- [Swag官方文档](https://github.com/swaggo/swag)
- [Swagger注解示例](./swagger-implementation-guide.md)
- [已完成的controller示例](../backend/controllers/module_controller.go)
