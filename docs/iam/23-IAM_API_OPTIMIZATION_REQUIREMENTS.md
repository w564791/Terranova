# IAM权限查询API优化需求文档

## 问题描述

### 当前问题
1. **权限管理页面** (`/iam/permissions`) 和 **团队详情页面** (`/iam/teams/{id}`) 存在严重的性能问题
2. 页面加载时会发起60+个API请求，导致：
   - 页面加载缓慢
   - 服务器负载过高
   - 用户体验差

### 根本原因
当前API设计只支持按作用域查询权限：
```
GET /api/v1/iam/permissions/{SCOPE_TYPE}/{scope_id}
```

前端需要：
1. 遍历所有 organizations, projects, workspaces
2. 对每个作用域调用上述API
3. 在前端过滤出目标用户/团队的权限

**示例**：如果系统有20个workspaces, 15个projects, 3个organizations
- 总请求数 = 20 + 15 + 3 = 38个请求
- 实际场景可能更多（60+个请求）

## 解决方案

### 新增API端点

#### 1. 查询用户的所有权限
```
GET /api/v1/iam/users/{user_id}/permissions
```

**响应示例**：
```json
{
  "data": [
    {
      "id": 1,
      "scope_type": "WORKSPACE",
      "scope_id": 26,
      "principal_type": "USER",
      "principal_id": 5,
      "permission_id": 8,
      "permission_level": "WRITE",
      "granted_by": 1,
      "granted_at": "2025-10-25T10:00:00Z",
      "expires_at": null,
      "reason": "项目需要",
      "source": "DIRECT"
    }
  ],
  "total": 10
}
```

#### 2. 查询团队的所有权限
```
GET /api/v1/iam/teams/{team_id}/permissions
```

**响应格式**：同上

#### 3. 通用权限查询接口（可选）
```
GET /api/v1/iam/permissions?principal_type={type}&principal_id={id}
```

**参数**：
- `principal_type`: USER | TEAM | APPLICATION
- `principal_id`: 主体ID

### 后端实现建议

#### SQL查询示例
```sql
-- 查询用户权限
SELECT p.* 
FROM permissions p
WHERE p.principal_type = 'USER' 
  AND p.principal_id = ?
ORDER BY p.granted_at DESC;

-- 查询团队权限
SELECT p.* 
FROM permissions p
WHERE p.principal_type = 'TEAM' 
  AND p.principal_id = ?
ORDER BY p.granted_at DESC;
```

#### Go代码结构建议
```go
// Handler
func GetUserPermissions(c *gin.Context) {
    userID := c.Param("user_id")
    
    permissions, err := permissionService.GetPermissionsByPrincipal("USER", userID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{
        "data": permissions,
        "total": len(permissions),
    })
}

// Service
func (s *PermissionService) GetPermissionsByPrincipal(principalType string, principalID string) ([]Permission, error) {
    var permissions []Permission
    err := s.db.Where("principal_type = ? AND principal_id = ?", principalType, principalID).
        Find(&permissions).Error
    return permissions, err
}
```

### 路由配置
```go
// 在 IAM 路由组中添加
iamGroup := v1.Group("/iam")
{
    // 用户权限
    iamGroup.GET("/users/:user_id/permissions", GetUserPermissions)
    
    // 团队权限
    iamGroup.GET("/teams/:team_id/permissions", GetTeamPermissions)
    
    // 通用查询（可选）
    iamGroup.GET("/permissions", QueryPermissions)
}
```

## 优化效果

### 请求数量对比

| 场景 | 优化前 | 优化后 | 减少比例 |
|------|--------|--------|----------|
| 权限管理页面（10个用户） | 60+ | ~15 | 75% |
| 团队详情页面 | 60+ | ~5 | 92% |

### 性能提升
- **页面加载时间**: 从 5-10秒 降低到 1-2秒
- **服务器负载**: 减少 75-90%
- **数据库查询**: 从 60+ 次减少到 5-15 次

## 实施计划

### 阶段1：后端开发（预计1-2天）
1.  创建需求文档
2. [ ] 实现 `GET /api/v1/iam/users/{user_id}/permissions`
3. [ ] 实现 `GET /api/v1/iam/teams/{team_id}/permissions`
4. [ ] 添加单元测试
5. [ ] 更新API文档

### 阶段2：前端适配（预计0.5天）
1. [ ] 更新 `PermissionManagement.tsx` 使用新API
2. [ ] 更新 `TeamDetail.tsx` 使用新API
3. [ ] 测试功能完整性

### 阶段3：测试与部署（预计0.5天）
1. [ ] 集成测试
2. [ ] 性能测试
3. [ ] 部署到测试环境
4. [ ] 部署到生产环境

## 兼容性说明

- 新API不影响现有API
- 前端可以逐步迁移
- 建议保留旧API一段时间以确保平滑过渡

## 附加优化建议

### 短期优化
1. 添加数据库索引：
   ```sql
   CREATE INDEX idx_permissions_principal ON permissions(principal_type, principal_id);
   ```

2. 添加响应缓存（可选）：
   - 缓存时间：5-10分钟
   - 权限变更时清除缓存

### 长期优化
1. 实现权限聚合查询（包含继承的权限）
2. 添加权限变更通知机制
3. 实现权限审计日志查询优化

## 联系人

- 前端负责人：[待填写]
- 后端负责人：[待填写]
- 测试负责人：[待填写]

## 参考资料

- 当前问题截图：见附件
- 性能分析报告：见附件
- API设计文档：本文档

---

**文档版本**: 1.0  
**创建日期**: 2025-10-25  
**最后更新**: 2025-10-25
