# Swagger注解实现指南

## 当前状态
- 已完成Auth handler的所有API注解
- 已完成Workspace基本CRUD的注解
- 已完成Admin Terraform版本管理的注解
- 还有约120个API需要添加注解

## 实现方案

由于API数量较多（约135个），有以下几种实现方案：

### 方案1：手动逐个添加（推荐用于核心API）
优点：
- 注解质量高，描述准确
- 可以为每个API定制详细的参数说明
- 便于review和维护

缺点：
- 耗时较长
- 需要逐个controller处理

适用范围：
- 核心业务API（Workspace任务管理、资源管理等）
- 对外暴露的重要API

### 方案2：批量生成基础注解
优点：
- 快速完成所有API的基础文档
- 保证API可见性

缺点：
- 注解可能不够详细
- 需要后续优化

适用范围：
- 内部管理API
- 辅助功能API

### 方案3：分阶段实现（推荐）
第一阶段：核心API（优先级高）
-  Auth APIs
-  Workspace CRUD
-  Admin Terraform版本
- ⏳ Workspace任务管理（Plan/Apply）
- ⏳ 资源管理
- ⏳ Module管理

第二阶段：扩展功能
- Agent管理
- Agent Pool管理
- State版本控制
- 变量管理

第三阶段：高级功能
- 资源编辑协作
- 快照管理
- AI分析
- Dashboard

## 快速实现步骤

### 1. 为单个controller添加注解

示例（module_controller.go）：

```go
// GetModules 获取模块列表
// @Summary 获取模块列表
// @Description 获取模块列表，支持分页、按provider过滤和搜索
// @Tags Module
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param size query int false "每页数量" default(20)
// @Param provider query string false "Provider过滤（AWS/Azure/GCP等）"
// @Param search query string false "搜索关键词"
// @Success 200 {object} map[string]interface{} "成功返回模块列表"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/modules [get]
// @Security Bearer
func (mc *ModuleController) GetModules(c *gin.Context) {
    // ... 实现代码
}
```

### 2. 生成Swagger文档

```bash
cd backend
swag init -g main.go --output docs --parseDependency --parseInternal
```

### 3. 验证文档

访问：http://localhost:8080/swagger/index.html

## 注解模板

### GET请求（列表）
```go
// @Summary 获取XXX列表
// @Description 获取XXX列表，支持分页和搜索
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

### GET请求（单个）
```go
// @Summary 获取XXX详情
// @Description 根据ID获取XXX的详细信息
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

### POST请求
```go
// @Summary 创建XXX
// @Description 创建新的XXX
// @Tags XXX
// @Accept json
// @Produce json
// @Param request body RequestStruct true "XXX信息"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/xxx [post]
// @Security Bearer
```

### PUT请求
```go
// @Summary 更新XXX
// @Description 更新XXX信息
// @Tags XXX
// @Accept json
// @Produce json
// @Param id path int true "XXX ID"
// @Param request body RequestStruct true "更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/xxx/{id} [put]
// @Security Bearer
```

### DELETE请求
```go
// @Summary 删除XXX
// @Description 删除指定的XXX
// @Tags XXX
// @Accept json
// @Produce json
// @Param id path int true "XXX ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/xxx/{id} [delete]
// @Security Bearer
```

## 下一步行动

建议按以下优先级添加注解：

1. **高优先级**（核心业务流程）
   - workspace_task_controller.go（Plan/Apply任务）
   - resource_controller.go（资源管理）
   - module_controller.go（模块管理）

2. **中优先级**（管理功能）
   - agent_controller.go
   - agent_pool_controller.go
   - ai_controller.go
   - dashboard_controller.go

3. **低优先级**（辅助功能）
   - schema_controller.go
   - module_demo_controller.go
   - state_version_controller.go
   - workspace_variable_controller.go

## 自动化工具

可以考虑使用以下工具辅助：
1. swag CLI - 自动生成文档
2. IDE插件 - 快速插入注解模板
3. 脚本批量处理 - 为简单CRUD生成基础注解
