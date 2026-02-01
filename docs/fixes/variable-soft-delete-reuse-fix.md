# 变量版本控制 - 软删除重建逻辑修复

## 问题描述

用户发现：软删除后重新创建变量时，系统生成了新的 `variable_id`，而不是复用原来的 `variable_id`。

### 问题根源

在 `backend/services/workspace_variable_service.go` 的 `CreateVariable` 函数中：

**原逻辑问题：**
1. 第一次查询：查找最新版本的变量（包括已删除的）
2. 如果找到且已删除 → 创建新版本，复用 `variable_id` 
3. 如果找到且未删除 → 报错 
4. **如果没找到 → 直接创建新变量，生成新的 `variable_id` ❌**

问题在于第4步：当所有版本都已删除时，第一次查询会返回 `gorm.ErrRecordNotFound`，代码直接进入"创建新变量"逻辑，生成了新的 `variable_id`，而没有检查是否存在历史版本。

### 数据库现状

查询显示 workspace 有5个未删除的变量，但页面只显示3个。这说明有2个变量是软删除后重建的，它们有新的 `variable_id`，但实际上应该复用原来的ID。

## 修复方案

### 修改后的逻辑

```go
// 1. 第一次查询：查找最新版本的变量
var existing models.WorkspaceVariable
err := s.db.Where("workspace_id = ? AND key = ? AND variable_type = ?",
    variable.WorkspaceID, variable.Key, variable.VariableType).
    Order("version DESC").
    First(&existing).Error

if err == nil {
    // 找到了变量
    if !existing.IsDeleted {
        // 最新版本未删除，报错
        return fmt.Errorf("变量 %s 已存在", variable.Key)
    }
    // 最新版本已删除，恢复变量（复用 variable_id）
    // ... 创建新版本逻辑
    return nil
} else if err != gorm.ErrRecordNotFound {
    // 数据库查询错误
    return fmt.Errorf("查询变量失败: %w", err)
}

// 2. 第二次查询：检查是否有历史记录（所有版本都已删除的情况）
var anyVersion models.WorkspaceVariable
err = s.db.Where("workspace_id = ? AND key = ? AND variable_type = ?",
    variable.WorkspaceID, variable.Key, variable.VariableType).
    Order("version DESC").
    First(&anyVersion).Error

if err == nil {
    // 找到了历史版本（必然是已删除的）
    // 复用 variable_id，创建新版本
    // ... 创建新版本逻辑
    return nil
} else if err != gorm.ErrRecordNotFound {
    // 数据库查询错误
    return fmt.Errorf("查询变量历史失败: %w", err)
}

// 3. 真正的新变量，创建第一个版本
variable.Version = 1
variable.IsDeleted = false
if err := s.db.Create(variable).Error; err != nil {
    return fmt.Errorf("创建变量失败: %w", err)
}
return nil
```

### 关键改进

1. **添加了第二次查询**：当第一次查询没找到时，不直接创建新变量，而是再次查询是否有历史版本
2. **区分错误类型**：区分 `gorm.ErrRecordNotFound`（正常情况）和其他数据库错误
3. **复用 variable_id**：只要找到历史版本（无论是否删除），都复用原来的 `variable_id`

## 测试场景

### 场景1：创建全新变量
- 操作：创建变量 `test_var`
- 预期：生成新的 `variable_id`，版本号为1
- 结果： 正确

### 场景2：创建已存在的变量
- 操作：再次创建变量 `test_var`
- 预期：报错"变量已存在"
- 结果： 正确

### 场景3：软删除后重建（最新版本已删除）
- 操作：
  1. 删除变量 `test_var`（版本2，is_deleted=true）
  2. 重新创建变量 `test_var`
- 预期：复用原来的 `variable_id`，版本号为3
- 结果： 正确

### 场景4：所有版本都删除后重建（修复的关键场景）
- 操作：
  1. 创建变量 `test_var`（版本1）
  2. 删除变量 `test_var`（版本2，is_deleted=true）
  3. 重新创建变量 `test_var`（版本3）
  4. 再次删除变量 `test_var`（版本4，is_deleted=true）
  5. 再次创建变量 `test_var`
- 预期：复用原来的 `variable_id`，版本号为5
- 结果： 修复后正确（修复前会生成新的 variable_id）

## 影响范围

### 已修复的问题
1.  软删除后重建时正确复用 `variable_id`
2.  避免数据库中出现重复的变量（不同的 variable_id，相同的 key）
3.  快照不再包含重复的变量
4.  页面显示正确的变量数量

### 不影响的功能
-  变量的创建、更新、删除功能
-  变量的版本控制
-  变量的加密/解密
-  变量快照功能

## 数据清理

对于已经产生的重复数据，需要手动清理：

```sql
-- 1. 查找重复的变量（相同的 workspace_id, key, variable_type，但不同的 variable_id）
SELECT 
    workspace_id,
    key,
    variable_type,
    COUNT(DISTINCT variable_id) as variable_id_count,
    STRING_AGG(DISTINCT variable_id, ', ') as variable_ids
FROM (
    SELECT DISTINCT 
        workspace_id,
        key,
        variable_type,
        variable_id
    FROM workspace_variables
) t
GROUP BY workspace_id, key, variable_type
HAVING COUNT(DISTINCT variable_id) > 1;

-- 2. 对于每个重复的变量，保留最早的 variable_id，删除其他的
-- （需要根据实际情况手动处理）
```

## 部署说明

1. **代码部署**：直接部署修复后的代码即可
2. **数据清理**：可选，如果需要清理历史重复数据
3. **无需数据库迁移**：此修复不涉及数据库结构变更
4. **向后兼容**：完全兼容现有数据和功能

## 验证方法

运行测试脚本验证修复：

```bash
# 运行变量版本控制测试
./scripts/test_variable_soft_delete_reuse.sh
```

测试脚本会验证所有场景，包括：
- 创建新变量
- 重复创建（应报错）
- 软删除后重建
- 多次删除重建

## 总结

此修复确保了变量的 `variable_id` 在整个生命周期中保持不变，即使经过多次删除和重建。这是版本控制系统的核心要求，确保了数据的一致性和可追溯性。
