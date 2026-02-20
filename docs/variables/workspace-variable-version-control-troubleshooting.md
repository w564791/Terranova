# Workspace Variables 版本控制 - 问题诊断和解决方案

## 当前问题

**错误信息**: `duplicate key value violates unique constraint "idx_variable_id"`

**原因分析**: 
在创建新版本时，GORM 的 BeforeCreate hook 被触发，尝试生成新的 variable_id，导致与已存在的 variable_id 冲突。

## 根本原因

当使用 `db.Create(&newVersion)` 时：
1. GORM 会触发 BeforeCreate hook
2. BeforeCreate hook 检查 `v.VariableID == ""`
3. 但由于我们从 current 复制了整个对象，variable_id 不为空
4. 理论上不应该重新生成，但仍然出现了冲突

## 解决方案

### 方案 1: 完全跳过 BeforeCreate hook（推荐）

修改 `backend/services/workspace_variable_service.go` 的 UpdateVariable 方法：

```go
// 保存新版本时跳过所有 hooks
if err := s.db.Session(&gorm.Session{SkipHooks: true}).
    Omit("Workspace").
    Create(&newVersion).Error; err != nil {
    return nil, fmt.Errorf("创建新版本失败: %w", err)
}
```

**注意**: 跳过 hooks 后，需要手动处理加密：

```go
// 在 Create 之前手动加密
if newVersion.Sensitive && newVersion.Value != "" && !crypto.IsEncrypted(newVersion.Value) {
    encrypted, err := crypto.EncryptValue(newVersion.Value)
    if err != nil {
        return nil, fmt.Errorf("加密失败: %w", err)
    }
    newVersion.Value = encrypted
}

// 然后跳过 hooks 创建
if err := s.db.Session(&gorm.Session{SkipHooks: true}).
    Omit("Workspace").
    Create(&newVersion).Error; err != nil {
    return nil, fmt.Errorf("创建新版本失败: %w", err)
}
```

### 方案 2: 使用原生 SQL（最可靠）

```go
// 使用原生 SQL 插入，完全避免 hooks
sql := `
INSERT INTO workspace_variables (
    variable_id, workspace_id, key, version, value, 
    variable_type, value_format, sensitive, description, 
    is_deleted, created_at, updated_at, created_by
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?)
`

if err := s.db.Exec(sql,
    newVersion.VariableID,
    newVersion.WorkspaceID,
    newVersion.Key,
    newVersion.Version,
    newVersion.Value,
    newVersion.VariableType,
    newVersion.ValueFormat,
    newVersion.Sensitive,
    newVersion.Description,
    newVersion.IsDeleted,
    newVersion.CreatedBy,
).Error; err != nil {
    return nil, fmt.Errorf("创建新版本失败: %w", err)
}
```

### 方案 3: 修改 BeforeCreate hook 逻辑

在 `backend/internal/models/variable.go` 中：

```go
func (v *WorkspaceVariable) BeforeCreate(tx *gorm.DB) error {
    // 只在 variable_id 为空且 Version 为 1 时生成（真正的新变量）
    if v.VariableID == "" && v.Version == 1 {
        varID, err := infrastructure.GenerateVariableID()
        if err != nil {
            return fmt.Errorf("failed to generate variable_id: %w", err)
        }
        v.VariableID = varID
    }
    
    // 加密逻辑...
    return nil
}
```

## 推荐实施步骤

1. **立即修复**: 使用方案 1，在 UpdateVariable 中手动加密并跳过 hooks
2. **同时修复**: DeleteVariable 也需要同样处理
3. **测试验证**: 确保更新和删除都能正常工作
4. **长期优化**: 考虑使用方案 3 优化 BeforeCreate 逻辑

## 临时解决方案（快速修复）

如果需要立即解决，可以：

1. 重启后端服务
2. 清理可能的脏数据：
```sql
-- 查看是否有重复的 variable_id
SELECT variable_id, COUNT(*) 
FROM workspace_variables 
GROUP BY variable_id 
HAVING COUNT(*) > 1;

-- 如果有重复，删除版本号较小的记录（保留最新版本）
DELETE FROM workspace_variables 
WHERE id IN (
    SELECT id FROM (
        SELECT id, ROW_NUMBER() OVER (PARTITION BY variable_id ORDER BY version DESC) as rn
        FROM workspace_variables
    ) t WHERE rn > 1
);
```

## 下一步行动

请选择一个方案并实施，推荐使用方案 1（跳过 hooks + 手动加密）。
