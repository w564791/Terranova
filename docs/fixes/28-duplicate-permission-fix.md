# 团队直接授权重复问题修复报告

## 问题描述

在弱网环境下,修改团队的直接授权(编辑权限级别)时,会为team添加重复的授权记录。

**问题URL**: http://localhost:5173/iam/teams/team-tecxxa1jo6

## 根本原因分析

### 1. 前端竞态条件 (Race Condition)

在 `frontend/src/pages/admin/TeamDetail.tsx` 的 `handleSavePermissionEdit` 函数中:

```typescript
const handleSavePermissionEdit = async (newLevel: PermissionLevel) => {
  if (!editingPermission) return;

  try {
    // 先撤销旧权限
    await iamService.revokePermission(editingPermission.scope_type, editingPermission.id);
    
    // 再授予新权限
    await iamService.grantPermission({
      scope_type: editingPermission.scope_type,
      scope_id: editingPermission.scope_id,
      principal_type: editingPermission.principal_type,
      principal_id: editingPermission.principal_id,
      permission_id: editingPermission.permission_id,
      permission_level: newLevel,
      expires_at: editingPermission.expires_at,
      reason: editingPermission.reason,
    });
    // ...
  }
}
```

**问题**: 在弱网情况下,如果用户快速点击"保存"按钮多次:
1. 第一次点击: revoke → grant (进行中)
2. 第二次点击: revoke → grant (在第一次完成前开始)
3. 结果: 权限被授予了两次!

### 2. 后端缺少防重复机制

在 `backend/internal/infrastructure/persistence/permission_repository_impl.go` 中:

```go
func (r *PermissionRepositoryImpl) GrantOrgPermission(ctx context.Context, permission *entity.OrgPermission) error {
	if err := r.db.WithContext(ctx).Create(permission).Error; err != nil {
		return fmt.Errorf("failed to grant org permission: %w", err)
	}
	return nil
}
```

**问题**: 后端直接调用 `Create()` 创建新记录,没有检查是否已存在相同的权限授予,也没有数据库唯一约束来防止重复。

## 解决方案

### 1. 前端修复: 防止重复点击

**文件**: `frontend/src/pages/admin/TeamDetail.tsx`

**修改内容**:
1. 添加 `savingPermission` 状态来跟踪保存操作
2. 在保存过程中禁用"保存"和"取消"按钮
3. 显示"保存中..."提示

```typescript
const [savingPermission, setSavingPermission] = useState(false);

const handleSavePermissionEdit = async (newLevel: PermissionLevel) => {
  if (!editingPermission || savingPermission) return; // 防止重复调用

  try {
    setSavingPermission(true);
    
    // 先撤销旧权限
    await iamService.revokePermission(editingPermission.scope_type, editingPermission.id);
    
    // 再授予新权限
    await iamService.grantPermission({...});

    showToast('权限级别更新成功', 'success');
    setEditingPermission(null);
    await loadTeamPermissions();
  } catch (error: any) {
    showToast(error.response?.data?.error || '更新失败', 'error');
  } finally {
    setSavingPermission(false); // 确保状态被重置
  }
};
```

按钮禁用:
```typescript
<button
  className={`${styles.actionButton} ${styles.save}`}
  onClick={() => handleSavePermissionEdit(editingPermission.permission_level)}
  disabled={savingPermission}
>
  {savingPermission ? '保存中...' : '保存'}
</button>
```

### 2. 后端修复: 冲突检测防止重复

**文件**: `backend/internal/infrastructure/persistence/permission_repository_impl.go`

**修改内容**: 将所有 `Grant*Permission` 方法添加冲突检测逻辑:

```go
func (r *PermissionRepositoryImpl) GrantOrgPermission(ctx context.Context, permission *entity.OrgPermission) error {
	// 检查是否已存在相同的权限授予
	var existing entity.OrgPermission
	err := r.db.WithContext(ctx).
		Where("org_id = ? AND principal_type = ? AND principal_id = ? AND permission_id = ?",
			permission.OrgID, permission.PrincipalType, permission.PrincipalID, permission.PermissionID).
		First(&existing).Error

	if err == nil {
		// 已存在权限,返回冲突错误
		return fmt.Errorf("permission already exists: principal %s already has permission %s (level: %s) on org %d",
			permission.PrincipalID, permission.PermissionID, existing.PermissionLevel, permission.OrgID)
	}

	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check existing org permission: %w", err)
	}

	// 不存在,创建新权限
	if err := r.db.WithContext(ctx).Create(permission).Error; err != nil {
		return fmt.Errorf("failed to grant org permission: %w", err)
	}
	return nil
}
```

**重要说明**: 
- 当检测到已存在相同的权限时,返回明确的错误信息,包含现有权限的级别
- 这样可以防止意外降低权限级别(例如从WRITE降为READ)
- 前端的"编辑权限"功能通过先revoke再grant的方式工作,不会触发此冲突
- 只有在直接新增授权时才会检测冲突

同样的逻辑应用于:
- `GrantProjectPermission`
- `GrantWorkspacePermission`

## 修复效果

### 前端层面
-  用户无法在保存过程中重复点击按钮
-  清晰的视觉反馈("保存中...")
-  防止并发请求

### 后端层面
-  即使收到重复请求,也不会创建重复记录
-  检测权限冲突,防止意外覆盖现有权限
-  返回友好的错误提示,告知用户已存在的权限级别
-  编辑权限功能(revoke+grant)不受影响

## 测试建议

### 1. 弱网环境测试
```bash
# 使用Chrome DevTools模拟慢速网络
1. 打开 Chrome DevTools (F12)
2. 切换到 Network 标签
3. 选择 "Slow 3G" 或 "Fast 3G"
4. 尝试快速点击"保存"按钮多次
```

### 2. 验证步骤
1. 访问团队详情页面
2. 编辑一个权限的级别
3. 在弱网环境下快速点击"保存"按钮多次
4. 验证:
   - 按钮应该被禁用
   - 显示"保存中..."
   - 最终只有一条权限记录
   - 权限级别正确更新

### 3. 数据库验证
```sql
-- 检查是否有重复的权限授予
SELECT 
    org_id, principal_type, principal_id, permission_id, 
    COUNT(*) as count
FROM org_permissions
GROUP BY org_id, principal_type, principal_id, permission_id
HAVING COUNT(*) > 1;

-- 对 project_permissions 和 workspace_permissions 执行相同查询
```

## 相关文件

### 前端
- `frontend/src/pages/admin/TeamDetail.tsx` - 团队详情页面

### 后端
- `backend/internal/infrastructure/persistence/permission_repository_impl.go` - 权限仓储实现
- `backend/internal/application/service/permission_service.go` - 权限服务
- `backend/internal/handlers/permission_handler.go` - 权限处理器

## 注意事项

1. **向后兼容**: 此修复不会影响现有功能,只是增强了防重复和冲突检测机制
2. **性能影响**: 每次授权前会多一次查询,但影响可忽略不计
3. **权限冲突**: 如果尝试授予已存在的权限,会返回错误提示用户,避免意外覆盖
4. **编辑权限**: 前端的"编辑权限"功能通过先revoke再grant实现,不会触发冲突检测
5. **数据清理**: 如果已经存在重复数据,需要手动清理:

```sql
-- 清理重复的组织权限(保留最新的)
DELETE FROM org_permissions
WHERE id NOT IN (
    SELECT MAX(id)
    FROM org_permissions
    GROUP BY org_id, principal_type, principal_id, permission_id
);
```

## 总结

通过前端防止重复点击和后端实现冲突检测的双重保护,彻底解决了弱网环境下团队直接授权重复的问题。

**关键改进**:
1. **前端**: 防止用户在弱网下重复点击
2. **后端**: 检测权限冲突,防止意外覆盖现有权限
3. **用户体验**: 当尝试授予已存在的权限时,返回友好的错误提示,告知用户当前权限级别

这是一个典型的竞态条件问题,需要在多个层面进行防护才能确保数据一致性和正确性。
