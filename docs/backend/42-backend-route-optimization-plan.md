# 后端路由优化方案

## 当前问题
后端API路径使用 `/api/v1/admin/terraform-versions` 和 `/api/v1/admin/ai-configs`，与前端新路径 `/global/settings/` 不匹配。

## 优化目标
将后端API路径从 `/api/v1/admin/` 改为 `/api/v1/global/settings/`，保持前后端路径一致性。

## 路径调整

### Terraform版本管理
**当前路径** → **新路径**
- GET `/api/v1/admin/terraform-versions` → `/api/v1/global/settings/terraform-versions`
- GET `/api/v1/admin/terraform-versions/default` → `/api/v1/global/settings/terraform-versions/default`
- GET `/api/v1/admin/terraform-versions/:id` → `/api/v1/global/settings/terraform-versions/:id`
- POST `/api/v1/admin/terraform-versions` → `/api/v1/global/settings/terraform-versions`
- PUT `/api/v1/admin/terraform-versions/:id` → `/api/v1/global/settings/terraform-versions/:id`
- POST `/api/v1/admin/terraform-versions/:id/set-default` → `/api/v1/global/settings/terraform-versions/:id/set-default`
- DELETE `/api/v1/admin/terraform-versions/:id` → `/api/v1/global/settings/terraform-versions/:id`

### AI配置管理
**当前路径** → **新路径**
- GET `/api/v1/admin/ai-configs` → `/api/v1/global/settings/ai-configs`
- POST `/api/v1/admin/ai-configs` → `/api/v1/global/settings/ai-configs`
- GET `/api/v1/admin/ai-configs/:id` → `/api/v1/global/settings/ai-configs/:id`
- PUT `/api/v1/admin/ai-configs/:id` → `/api/v1/global/settings/ai-configs/:id`
- DELETE `/api/v1/admin/ai-configs/:id` → `/api/v1/global/settings/ai-configs/:id`
- PUT `/api/v1/admin/ai-configs/priorities` → `/api/v1/global/settings/ai-configs/priorities`
- PUT `/api/v1/admin/ai-configs/:id/set-default` → `/api/v1/global/settings/ai-configs/:id/set-default`
- GET `/api/v1/admin/ai-config/regions` → `/api/v1/global/settings/ai-config/regions`
- GET `/api/v1/admin/ai-config/models` → `/api/v1/global/settings/ai-config/models`

## 需要修改的文件

### 后端文件
1. **backend/internal/router/router.go**
   - 将 `admin := adminProtected.Group("/admin")` 改为 `globalSettings := adminProtected.Group("/global/settings")`
   - 更新所有相关路由注册

### 前端文件
前端已经在使用正确的路径，无需修改。

## 实施步骤

### 步骤1: 修改后端路由组名称
```go
// 从
admin := adminProtected.Group("/admin")

// 改为
globalSettings := adminProtected.Group("/global/settings")
```

### 步骤2: 更新所有路由注册
将所有 `admin.GET/POST/PUT/DELETE` 改为 `globalSettings.GET/POST/PUT/DELETE`

### 步骤3: 测试验证
- [ ] Terraform版本管理所有API
- [ ] AI配置管理所有API
- [ ] 前端页面正常访问和操作

## 向后兼容（可选）
如果需要保持向后兼容，可以添加路由别名：
```go
// 新路径
globalSettings := adminProtected.Group("/global/settings")
// ... 注册所有路由

// 旧路径别名（可选，用于向后兼容）
adminAlias := adminProtected.Group("/admin")
// ... 注册相同的路由
```

## 风险评估

**中等风险**
- 后端API路径变更，需要确保前端调用正确
- 如果有其他服务调用这些API，需要同步更新
- 建议先在测试环境验证

**注意事项**
- 确保前端已经使用新的API路径
- 检查是否有硬编码的API路径
- 更新API文档和Swagger注释

## 确认事项

请确认以下内容后再执行：

1.  是否需要修改后端API路径？
2.  是否需要保持向后兼容（添加旧路径别名）？
3.  前端是否已经准备好使用新的API路径？

确认无误后，我将开始执行后端路由优化。
