# Workspace Variable Snapshot - Phase 2 实施代码

## 说明
本文件包含 Phase 2 需要添加/修改的完整代码。请按照以下步骤操作。

## 步骤1: 在 terraform_executor.go 中添加解析函数

在 `backend/services/terraform_executor.go` 文件中，找到 `TerraformExecutor` 的其他方法，在合适的位置添加以下函数：

```go
// ResolveVariableSnapshots 解析变量快照引用为实际变量值
// 支持新旧两种格式以保持向后兼容
func (s *TerraformExecutor) ResolveVariableSnapshots(
	snapshotData interface{},
	workspaceID string,
) ([]models.WorkspaceVariable, error) {
	// 处理nil情况
	if snapshotData == nil {
		return []models.WorkspaceVariable{}, nil
	}

	// 尝试将snapshotData转换为JSON
	snapshotBytes, err := json.Marshal(snapshotData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal snapshot data: %w", err)
	}

	// 先尝试解析为新格式（VariableSnapshot数组）
	var snapshots []models.VariableSnapshot
	if err := json.Unmarshal(snapshotBytes, &snapshots); err == nil && len(snapshots) > 0 {
		// 检查是否是新格式（只有variable_id和version字段）
		// 新格式的特征：第一个元素有variable_id字段
		if snapshots[0].VariableID != "" {
			// 新格式：需要从数据库查询
			// 使用批量查询优化性能
			if len(snapshots) == 0 {
				return []models.WorkspaceVariable{}, nil
			}

			// 构建批量查询
			variables := make([]models.WorkspaceVariable, 0, len(snapshots))
			for _, snap := range snapshots {
				var variable models.WorkspaceVariable
				err := s.db.Where("variable_id = ? AND version = ?", 
					snap.VariableID, snap.Version).First(&variable).Error
				if err != nil {
					return nil, fmt.Errorf("variable %s version %d not found: %w", 
						snap.VariableID, snap.Version, err)
				}
				variables = append(variables, variable)
			}
			
			log.Printf("[DEBUG] Resolved %d variable snapshots from references", len(variables))
			return variables, nil
		}
	}

	// 尝试解析为旧格式（完整的WorkspaceVariable数组）
	var variables []models.WorkspaceVariable
	if err := json.Unmarshal(snapshotBytes, &variables); err == nil {
		// 旧格式：直接返回
		log.Printf("[DEBUG] Using %d variables from old snapshot format", len(variables))
		return variables, nil
	}

	return nil, fmt.Errorf("failed to parse snapshot data in any known format")
}
```

## 步骤2: 修改 GenerateConfigFilesFromSnapshot 函数

在 `backend/services/terraform_executor.go` 中找到 `GenerateConfigFilesFromSnapshot` 函数。

### 2.1 修改函数签名

**查找**:
```go
func (s *TerraformExecutor) GenerateConfigFilesFromSnapshot(
	workspace *models.Workspace,
	resources []models.WorkspaceResource,
	variables models.WorkspaceVariableArray,
	workDir string,
	logger *TerraformLogger,
) error {
```

**替换为**:
```go
func (s *TerraformExecutor) GenerateConfigFilesFromSnapshot(
	workspace *models.Workspace,
	resources []models.WorkspaceResource,
	variableSnapshots interface{},  // 改为interface{}以支持两种格式
	workDir string,
	logger *TerraformLogger,
) error {
```

### 2.2 在函数开头添加解析逻辑

在函数的开头（在任何使用 variables 的代码之前）添加：

```go
	// 解析变量快照为实际变量值
	variables, err := s.ResolveVariableSnapshots(variableSnapshots, workspace.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to resolve variable snapshots: %w", err)
	}
	
	// 其余代码保持不变，继续使用 variables 变量
```

## 步骤3: 验证调用点

在 `terraform_executor.go` 中搜索所有调用 `GenerateConfigFilesFromSnapshot` 的地方。

通常的调用形式：
```go
if err := s.GenerateConfigFilesFromSnapshot(workspace, resources, planTask.SnapshotVariables, workDir, logger); err != nil {
```

**这些调用不需要修改**，因为：
- `planTask.SnapshotVariables` 的类型是 `WorkspaceVariableArray`
- `interface{}` 可以接受任何类型
- 解析函数会自动处理新旧两种格式

## 步骤4: 更新验证函数中的日志（可选）

在 `terraform_executor.go` 中找到类似这样的代码：

```go
if planTask.SnapshotVariables == nil {
    return fmt.Errorf("snapshot variables missing")
}
logger.Debug("  - Variables: %d", len(planTask.SnapshotVariables))
```

可以改为：

```go
if planTask.SnapshotVariables == nil {
    return fmt.Errorf("snapshot variables missing")
}

// 尝试获取变量数量（支持新旧格式）
varCount := 0
if snapshotBytes, err := json.Marshal(planTask.SnapshotVariables); err == nil {
    var snapshots []models.VariableSnapshot
    if err := json.Unmarshal(snapshotBytes, &snapshots); err == nil {
        varCount = len(snapshots)
    } else {
        var variables []models.WorkspaceVariable
        if err := json.Unmarshal(snapshotBytes, &variables); err == nil {
            varCount = len(variables)
        }
    }
}
logger.Debug("  - Variables: %d", varCount)
```

但这一步是可选的，不影响核心功能。

## 完整的测试代码

创建 `backend/services/terraform_executor_variable_snapshot_test.go`:

```go
package services

import (
	"encoding/json"
	"testing"

	"iac-platform/internal/models"
	
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveVariableSnapshots_NewFormat(t *testing.T) {
	// 设置测试数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	
	// 迁移表
	db.AutoMigrate(&models.WorkspaceVariable{})
	
	// 创建测试变量
	testVar1 := models.WorkspaceVariable{
		VariableID:  "var-test123",
		WorkspaceID: "ws-test",
		Key:         "TEST_VAR_1",
		Value:       "value1",
		Version:     1,
	}
	testVar2 := models.WorkspaceVariable{
		VariableID:  "var-test456",
		WorkspaceID: "ws-test",
		Key:         "TEST_VAR_2",
		Value:       "value2",
		Version:     2,
	}
	db.Create(&testVar1)
	db.Create(&testVar2)
	
	// 创建executor
	executor := &TerraformExecutor{db: db}
	
	// 测试新格式快照
	snapshotData := []models.VariableSnapshot{
		{VariableID: "var-test123", Version: 1},
		{VariableID: "var-test456", Version: 2},
	}
	
	variables, err := executor.ResolveVariableSnapshots(snapshotData, "ws-test")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(variables))
	assert.Equal(t, "TEST_VAR_1", variables[0].Key)
	assert.Equal(t, "TEST_VAR_2", variables[1].Key)
}

func TestResolveVariableSnapshots_OldFormat(t *testing.T) {
	// 设置测试数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	
	// 创建executor
	executor := &TerraformExecutor{db: db}
	
	// 测试旧格式快照（完整变量数据）
	snapshotData := []models.WorkspaceVariable{
		{
			VariableID:  "var-test123",
			WorkspaceID: "ws-test",
			Key:         "TEST_VAR",
			Value:       "test_value",
			Version:     1,
		},
	}
	
	variables, err := executor.ResolveVariableSnapshots(snapshotData, "ws-test")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(variables))
	assert.Equal(t, "TEST_VAR", variables[0].Key)
	assert.Equal(t, "test_value", variables[0].Value)
}

func TestResolveVariableSnapshots_Nil(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	executor := &TerraformExecutor{db: db}
	
	variables, err := executor.ResolveVariableSnapshots(nil, "ws-test")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(variables))
}

func TestResolveVariableSnapshots_VariableNotFound(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&models.WorkspaceVariable{})
	
	executor := &TerraformExecutor{db: db}
	
	// 测试不存在的变量
	snapshotData := []models.VariableSnapshot{
		{VariableID: "var-notexist", Version: 1},
	}
	
	_, err := executor.ResolveVariableSnapshots(snapshotData, "ws-test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
```

## 运行测试

```bash
cd backend
go test ./services -v -run TestResolveVariableSnapshots
```

## 手动测试步骤

```bash
# 1. 启动服务
cd backend
go run main.go

# 2. 创建workspace
curl -X POST http://localhost:8080/api/v1/workspaces \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-snapshot-phase2",
    "state_backend": "local"
  }'

# 3. 添加变量
curl -X POST http://localhost:8080/api/v1/workspaces/ws-xxx/variables \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "key": "AWS_REGION",
    "value": "us-east-1",
    "variable_type": "terraform"
  }'

# 4. 创建Plan任务
curl -X POST http://localhost:8080/api/v1/workspaces/ws-xxx/tasks/plan \
  -H "Authorization: Bearer $TOKEN"

# 5. 等待任务执行，检查日志
# 应该看到类似这样的日志：
# [DEBUG] Resolved 1 variable snapshots from references
# [DEBUG] Snapshot created for task 1: 0 resources, 1 variable references

# 6. 验证任务成功完成
curl http://localhost:8080/api/v1/workspaces/ws-xxx/tasks/1 \
  -H "Authorization: Bearer $TOKEN"
```

## 性能优化版本（可选）

如果变量数量很多，可以使用批量查询优化：

```go
// 在 ResolveVariableSnapshots 函数中，替换循环查询部分：

// 批量查询优化版本
if len(snapshots) > 0 {
    // 收集所有需要查询的 variable_id 和 version
    type VariableKey struct {
        VariableID string
        Version    int
    }
    keys := make([]VariableKey, len(snapshots))
    for i, snap := range snapshots {
        keys[i] = VariableKey{
            VariableID: snap.VariableID,
            Version:    snap.Version,
        }
    }
    
    // 构建批量查询
    var variables []models.WorkspaceVariable
    query := s.db.Where("workspace_id = ?", workspaceID)
    
    // 添加OR条件
    for i, key := range keys {
        if i == 0 {
            query = query.Where("(variable_id = ? AND version = ?)", key.VariableID, key.Version)
        } else {
            query = query.Or("(variable_id = ? AND version = ?)", key.VariableID, key.Version)
        }
    }
    
    if err := query.Find(&variables).Error; err != nil {
        return nil, fmt.Errorf("failed to query variables: %w", err)
    }
    
    // 验证所有变量都找到了
    if len(variables) != len(snapshots) {
        return nil, fmt.Errorf("some variables not found: expected %d, got %d", 
            len(snapshots), len(variables))
    }
    
    // 按照快照顺序排序变量
    variableMap := make(map[string]models.WorkspaceVariable)
    for _, v := range variables {
        key := fmt.Sprintf("%s-%d", v.VariableID, v.Version)
        variableMap[key] = v
    }
    
    orderedVariables := make([]models.WorkspaceVariable, 0, len(snapshots))
    for _, snap := range snapshots {
        key := fmt.Sprintf("%s-%d", snap.VariableID, snap.Version)
        if v, ok := variableMap[key]; ok {
            orderedVariables = append(orderedVariables, v)
        }
    }
    
    log.Printf("[DEBUG] Resolved %d variable snapshots from references (batch query)", len(orderedVariables))
    return orderedVariables, nil
}
```

## 完成检查清单

- [ ] 在 terraform_executor.go 中添加 ResolveVariableSnapshots 函数
- [ ] 修改 GenerateConfigFilesFromSnapshot 函数签名
- [ ] 在 GenerateConfigFilesFromSnapshot 开头添加解析逻辑
- [ ] 验证所有调用点（通常不需要修改）
- [ ] 运行单元测试
- [ ] 运行手动测试
- [ ] 检查日志输出
- [ ] 验证向后兼容性（旧快照仍能工作）

## Git Commit

```bash
git add backend/services/terraform_executor.go
git add backend/services/terraform_executor_variable_snapshot_test.go
git commit -m "feat: implement variable snapshot resolution (Phase 2)

- Add ResolveVariableSnapshots() to resolve variable references
- Update GenerateConfigFilesFromSnapshot() to use resolver
- Support both old (complete data) and new (references) formats
- Add comprehensive unit tests
- Maintain full backward compatibility

Completes variable snapshot implementation.

Related: #variable-version-control"
```

## 相关文档

- Phase 1 完成报告: `docs/workspace-variable-snapshot-phase1-complete.md`
- 实现指南: `docs/workspace-variable-snapshot-implementation.md`
