# Workspace Variables 版本控制 - 最终修复方案

## 问题分析

经过诊断发现：
1.  数据库中没有重复的 variable_id
2.  所有变量都是 version 1，没有历史版本
3. ❌ 但在更新时仍然出现 duplicate variable_id 错误

## 根本原因

即使使用了 `SkipHooks: true`，GORM 仍然可能在某些情况下触发 hooks。最可靠的解决方案是**使用原生 SQL 插入**。

## 最终解决方案：使用原生 SQL

修改 `backend/services/workspace_variable_service.go` 的 UpdateVariable 方法：

```go
// 手动处理加密
if newVersion.Sensitive && newVersion.Value != "" && !crypto.IsEncrypted(newVersion.Value) {
    encrypted, err := crypto.EncryptValue(newVersion.Value)
    if err != nil {
        return nil, fmt.Errorf("加密失败: %w", err)
    }
    newVersion.Value = encrypted
}

// 使用原生 SQL 插入，完全避免 GORM hooks
sql := `
INSERT INTO workspace_variables (
    variable_id, workspace_id, key, version, value,
    variable_type, value_format, sensitive, description,
    is_deleted, created_at, updated_at, created_by
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?)
RETURNING id
`

var newID uint
if err := s.db.Raw(sql,
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
).Scan(&newID).Error; err != nil {
    return nil, fmt.Errorf("创建新版本失败: %w", err)
}

newVersion.ID = newID
```

## 为什么原生 SQL 是最佳方案

1. **完全避免 hooks**：不会触发任何 GORM hooks
2. **性能更好**：直接执行 SQL，没有额外开销
3. **可控性强**：明确知道执行的每一步
4. **可靠性高**：不依赖 GORM 的内部机制

## 实施步骤

1. 修改 UpdateVariable 方法使用原生 SQL
2. 修改 DeleteVariable 方法使用原生 SQL
3. 重新编译后端
4. 重启服务
5. 测试更新功能

## 预期结果

-  更新变量时不再出现 duplicate variable_id 错误
-  版本号正确递增
-  variable_id 保持不变
-  敏感数据正确加密
-  返回正确的版本变更信息
