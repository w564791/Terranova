# IAM权限查询API优化 - 部署指南

## 概述

本文档提供IAM权限查询API优化的部署步骤和测试方法。

## 已完成的工作

###  后端实现
1. **Handler层** (`internal/handlers/permission_handler.go`)
   - 添加 `ListUserPermissions` 方法
   - 添加 `ListTeamPermissions` 方法

2. **Service层** (`internal/application/service/permission_service.go`)
   - 添加 `ListPermissionsByPrincipal` 接口方法
   - 实现该方法

3. **Repository层** 
   - 接口定义 (`internal/domain/repository/permission_repository.go`)
   - 实现 (`internal/infrastructure/persistence/permission_repository_impl.go`)
   - 添加 `ListPermissionsByPrincipal` 方法

4. **路由配置** (`internal/router/router.go`)
   - 添加 `GET /api/v1/iam/users/:user_id/permissions`
   - 添加 `GET /api/v1/iam/teams/:team_id/permissions`

###  前端实现
1. **TeamDetail.tsx** - 优化团队详情页面
2. **PermissionManagement.tsx** - 优化权限管理页面

###  数据库优化
- 创建索引脚本 (`migrations/add_permission_indexes.sql`)

## 部署步骤

### 步骤1: 数据库索引创建（5分钟）

```bash
# 连接到数据库
psql -h localhost -U postgres -d iac_platform

# 执行索引创建脚本
\i backend/migrations/add_permission_indexes.sql

# 验证索引创建成功
SELECT 
    tablename,
    indexname
FROM pg_indexes
WHERE tablename IN ('org_permissions', 'project_permissions', 'workspace_permissions')
    AND indexname LIKE 'idx_%_principal%';
```

**预期输出**：应该看到6个新索引

### 步骤2: 后端编译和部署（10分钟）

```bash
# 进入后端目录
cd ../iac-platform/backend

# 编译
go build -o main .

# 运行测试（可选）
go test ./internal/...

# 重启后端服务
# 方法1: 如果使用systemd
sudo systemctl restart iac-platform-backend

# 方法2: 如果使用docker
docker-compose restart backend

# 方法3: 如果直接运行
pkill -f "iac-platform.*main"
./main &
```

### 步骤3: 前端部署（5分钟）

```bash
# 进入前端目录
cd ../iac-platform/frontend

# 构建（如果需要）
npm run build

# 如果是开发环境，重启开发服务器
# Ctrl+C 停止当前服务器
npm run dev
```

### 步骤4: 验证部署（10分钟）

#### 4.1 验证后端API

```bash
# 测试用户权限API
curl -X GET "http://localhost:8080/api/v1/iam/users/1/permissions" \
  -H "Authorization: Bearer YOUR_TOKEN"

# 测试团队权限API
curl -X GET "http://localhost:8080/api/v1/iam/teams/1/permissions" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**预期响应**：
```json
{
  "data": [
    {
      "id": 1,
      "scope_type": "WORKSPACE",
      "scope_id": 26,
      "principal_type": "USER",
      "principal_id": 1,
      "permission_id": 8,
      "permission_level": "WRITE",
      "granted_at": "2025-10-25T10:00:00Z"
    }
  ],
  "total": 1
}
```

#### 4.2 验证前端页面

1. **团队详情页面**
   - 访问: `http://localhost:5173/iam/teams/12`
   - 打开浏览器开发者工具 -> Network标签
   - 刷新页面
   - **验证**: API请求数应该从60+减少到~5个

2. **权限管理页面**
   - 访问: `http://localhost:5173/iam/permissions`
   - 打开浏览器开发者工具 -> Network标签
   - 刷新页面
   - **验证**: API请求数应该从60+减少到~15个（取决于用户数量）

## 性能测试

### 测试场景1: 团队详情页面

**测试前**:
```
- API请求数: 60+
- 页面加载时间: 5-10秒
- 数据库查询: 60+次
```

**测试后**:
```
- API请求数: ~5
- 页面加载时间: 1-2秒
- 数据库查询: ~5次
```

### 测试场景2: 权限管理页面（10个用户）

**测试前**:
```
- API请求数: 60+
- 页面加载时间: 5-10秒
- 数据库查询: 60+次
```

**测试后**:
```
- API请求数: ~15 (10个用户权限 + 10个用户角色 + 其他)
- 页面加载时间: 1-2秒
- 数据库查询: ~15次
```

## 回滚计划

如果部署后出现问题，可以快速回滚：

### 回滚后端
```bash
# 恢复到之前的版本
git checkout HEAD~1 backend/

# 重新编译和部署
cd backend
go build -o main .
# 重启服务
```

### 回滚前端
```bash
# 恢复到之前的版本
git checkout HEAD~1 frontend/src/pages/admin/TeamDetail.tsx
git checkout HEAD~1 frontend/src/pages/admin/PermissionManagement.tsx

# 重新构建
npm run build
```

### 回滚数据库索引（可选）
```sql
-- 删除索引（如果需要）
DROP INDEX IF EXISTS idx_org_permissions_principal;
DROP INDEX IF EXISTS idx_project_permissions_principal;
DROP INDEX IF EXISTS idx_workspace_permissions_principal;
-- ... 其他索引
```

## 监控指标

部署后需要监控以下指标：

1. **API响应时间**
   - `/api/v1/iam/users/:id/permissions` 应该 < 200ms
   - `/api/v1/iam/teams/:id/permissions` 应该 < 200ms

2. **数据库查询性能**
   ```sql
   -- 查看慢查询
   SELECT * FROM pg_stat_statements 
   WHERE query LIKE '%permissions%' 
   ORDER BY mean_exec_time DESC 
   LIMIT 10;
   ```

3. **页面加载时间**
   - 团队详情页: < 2秒
   - 权限管理页: < 3秒

## 常见问题

### Q1: 新API返回404
**A**: 检查路由是否正确注册，确认后端服务已重启

### Q2: 前端仍然发起大量请求
**A**: 清除浏览器缓存，确认前端代码已更新

### Q3: 数据库查询仍然很慢
**A**: 确认索引已创建成功，使用 `EXPLAIN ANALYZE` 分析查询计划

### Q4: 权限数据不显示
**A**: 检查API响应格式，确认数据结构匹配

## 联系支持

如有问题，请联系：
- 后端团队: [待填写]
- 前端团队: [待填写]
- DBA团队: [待填写]

---

**文档版本**: 1.0  
**创建日期**: 2025-10-25  
**最后更新**: 2025-10-25
