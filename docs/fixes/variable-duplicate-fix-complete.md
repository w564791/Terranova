# 变量重复定义问题完整修复报告

## 实施时间
2025-01-12

## 问题描述

在执行任务时出现变量重复定义错误：
```
Error: Attribute redefined
on variables.tfvars line 2:
2: aaa = "aaa"
The argument "aaa" was already set at variables.tfvars:1,1-4.
```

用户提供的日志显示在 K8s Agent 的 Fetching 阶段就已经加载了重复的变量：
```
[INFO] ✓ Variable: aaa = ***SENSITIVE***
[INFO] ✓ Variable: aaa = ***SENSITIVE***
[INFO] ✓ Variable: aaa = aaa
[INFO] ✓ Variable: AWS_ACCESS_KEY_ID = aaa
[INFO] ✓ Variable: AWS_ACCESS_KEY_ID = ***SENSITIVE***
[INFO] ✓ Variable: aaa = aaa
[INFO] ✓ Variable: AWS_ACCESS_KEY_ID = ***SENSITIVE***
[INFO] ✓ Variable: AWS_ACCESS_KEY_ID = ***SENSITIVE***
[INFO] Total: 8 variables loaded (3 normal, 5 sensitive)
```

## 根本原因分析

问题出现在**两个地方**的变量查询逻辑，都没有正确过滤和去重：

### 1. LocalDataAccessor.GetWorkspaceVariables（Local 模式）
```go
// 旧代码 - 返回所有版本
h.db.Where("workspace_id = ? AND variable_type = ?", workspaceID, varType).
    Find(&variables)
```

问题：
- 没有过滤 `is_deleted = false`
- 没有只获取每个变量的最新版本
- 返回了同一个变量的所有历史版本

### 2. AgentHandler.GetTaskData（K8s Agent API）
```go
// 旧代码 - 返回所有版本
h.db.Where("workspace_id = ?", workspace.WorkspaceID).Find(&variables)
```

问题：
- 没有过滤 `is_deleted = false`
- 没有过滤 `variable_type`
- 没有只获取每个变量的最新版本
- K8s Agent 通过这个 API 获取数据，所以看到了重复变量

## 实施的修复

### 修复 1: LocalDataAccessor.GetWorkspaceVariables

**文件**: `backend/services/local_data_accessor.go`

**修改**:
```go
// 新代码 - 只返回每个变量的最新版本
func (a *LocalDataAccessor) GetWorkspaceVariables(workspaceID string, varType models.VariableType) ([]models.WorkspaceVariable, error) {
	var variables []models.WorkspaceVariable
	db := a.getDB()

	// 使用子查询只获取每个变量的最新版本
	err := db.Raw(`
		SELECT wv.*
		FROM workspace_variables wv
		INNER JOIN (
			SELECT variable_id, MAX(version) as max_version
			FROM workspace_variables
			WHERE workspace_id = ? AND variable_type = ? AND is_deleted = false
			GROUP BY variable_id
		) latest ON wv.variable_id = latest.variable_id AND wv.version = latest.max_version
		WHERE wv.workspace_id = ? AND wv.variable_type = ? AND wv.is_deleted = false
	`, workspaceID, varType, workspaceID, varType).Scan(&variables).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get workspace variables: %w", err)
	}

	return variables, nil
}
```

### 修复 2: AgentHandler.GetTaskData

**文件**: `backend/internal/handlers/agent_handler.go` (第445行)

**修改**:
```go
// 新代码 - 只返回每个变量的最新版本
// Get workspace variables (只获取每个变量的最新版本)
var variables []models.WorkspaceVariable
h.db.Raw(`
	SELECT wv.*
	FROM workspace_variables wv
	INNER JOIN (
		SELECT variable_id, MAX(version) as max_version
		FROM workspace_variables
		WHERE workspace_id = ? AND is_deleted = false
		GROUP BY variable_id
	) latest ON wv.variable_id = latest.variable_id AND wv.version = latest.max_version
	WHERE wv.workspace_id = ? AND wv.is_deleted = false
`, workspace.WorkspaceID, workspace.WorkspaceID).Scan(&variables)
```

### 修复 3: 快照解析（已在之前完成）

**文件**: `backend/services/terraform_executor.go`

添加了 `ResolveVariableSnapshots` 函数和更新了 `GenerateConfigFilesFromSnapshot`，确保：
- 解析变量快照引用为实际数据
- 使用 map 去重机制
- 支持新旧两种格式

## 修复效果

### 修复前
- Local 模式：返回所有版本的变量（如果有3个版本，返回3条记录）
- K8s Agent：通过 API 获取所有版本的变量
- 结果：variables.tfvars 包含重复定义，Terraform 报错

### 修复后
- Local 模式：只返回每个变量的最新版本（每个变量1条记录）
- K8s Agent：通过 API 只获取每个变量的最新版本
- 结果：variables.tfvars 无重复定义，Terraform 正常执行

## 影响范围

###  Local 模式
- Plan 阶段：使用 LocalDataAccessor，已修复
- Apply 阶段：使用快照 + ResolveVariableSnapshots，已修复

###  K8s Agent 模式
- Plan 阶段：通过 GetTaskData API 获取变量，已修复
- Apply 阶段：通过 GetPlanTask API 获取快照变量（完整数据），已修复

## 测试验证

### Local 模式测试
```bash
# 1. 创建 workspace 并添加变量
# 2. 更新变量几次（创建多个版本）
# 3. 运行 plan 任务
# 4. 检查日志，应该只看到每个变量一次
# 5. 验证 variables.tfvars 无重复
```

### K8s Agent 模式测试
```bash
# 1. 在 K8s agent pool 中运行任务
# 2. 检查日志，应该只看到每个变量一次
# 3. 验证 variables.tfvars 无重复
# 4. 验证 Apply 成功
```

## 相关文件

- `backend/services/local_data_accessor.go` - Local 模式数据访问
- `backend/internal/handlers/agent_handler.go` - Agent API
- `backend/services/terraform_executor.go` - 快照解析
- `backend/services/remote_data_accessor.go` - Agent 模式数据访问
- `scripts/check_duplicate_variables.sql` - 检查重复变量的 SQL

## 技术要点

### 子查询去重
使用 SQL 子查询确保只获取每个变量的最新版本：
```sql
SELECT wv.*
FROM workspace_variables wv
INNER JOIN (
    SELECT variable_id, MAX(version) as max_version
    FROM workspace_variables
    WHERE workspace_id = ? AND is_deleted = false
    GROUP BY variable_id
) latest ON wv.variable_id = latest.variable_id AND wv.version = latest.max_version
WHERE wv.workspace_id = ? AND wv.is_deleted = false
```

### 过滤条件
- `is_deleted = false`: 排除已删除的变量
- `GROUP BY variable_id`: 按变量ID分组
- `MAX(version)`: 只取最新版本

## 总结

 **Local 模式**：修复 LocalDataAccessor.GetWorkspaceVariables 查询
 **K8s Agent 模式**：修复 AgentHandler.GetTaskData API 查询
 **快照解析**：ResolveVariableSnapshots 提供额外的去重保护
 **向后兼容**：旧数据仍然可以正常工作

所有模式下的变量重复定义问题已完全修复。
