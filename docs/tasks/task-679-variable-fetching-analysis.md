# Task 679 - Variable Fetching Analysis

## Issue Description
Task 679 发现变量文件（variables.tf.json 和 variables.tfvars）为空，需要检查 local/agent/k8s agent 模式是否正确按照以下方式获取变量：
- 按 workspace_id, variable_id, version, variable_type 过滤
- environment 类型变量需要 export 到 OS 环境变量
- terraform 类型变量需要生成 variables.tf.json 和 variables.tfvars

## Analysis Results

### 1. Local Mode (LocalDataAccessor)

**Location**: `backend/services/local_data_accessor.go`

**GetWorkspaceVariables 实现**:
```go
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

** 正确实现**:
-  按 workspace_id 过滤
-  按 variable_type 过滤
-  自动获取每个 variable_id 的最新 version（MAX(version)）
-  排除已删除的变量（is_deleted = false）

### 2. Agent Mode (RemoteDataAccessor)

**Location**: `backend/services/remote_data_accessor.go`

**GetWorkspaceVariables 实现**:
```go
func (a *RemoteDataAccessor) GetWorkspaceVariables(workspaceID string, varType models.VariableType) ([]models.WorkspaceVariable, error) {
    variablesData, ok := a.taskData["variables"].([]interface{})
    if !ok {
        return []models.WorkspaceVariable{}, nil
    }

    variables := make([]models.WorkspaceVariable, 0)
    for _, item := range variablesData {
        varMap, ok := item.(map[string]interface{})
        if !ok {
            continue
        }

        // Filter by variable type
        if getString(varMap, "variable_type") != string(varType) {
            continue
        }

        variable := models.WorkspaceVariable{
            ID:           getUint(varMap, "id"),
            WorkspaceID:  getString(varMap, "workspace_id"),
            Key:          getString(varMap, "key"),
            Value:        getString(varMap, "value"),
            VariableType: models.VariableType(getString(varMap, "variable_type")),
            Sensitive:    getBool(varMap, "sensitive"),
            Description:  getString(varMap, "description"),
            ValueFormat:  models.ValueFormat(getString(varMap, "value_format")),
        }

        variables = append(variables, variable)
    }

    return variables, nil
}
```

** 依赖服务端数据**:
-  按 variable_type 过滤（在 Agent 端）
-  workspace_id, variable_id, version 的过滤依赖服务端 API 返回的数据
-  需要检查服务端 GetTaskData API 是否正确过滤

### 3. Environment Variable Export

**Location**: `backend/services/terraform_executor.go`

**buildEnvironmentVariables 实现**:
```go
func (s *TerraformExecutor) buildEnvironmentVariables(
    workspace *models.Workspace,
) []string {
    env := append(os.Environ(),
        "TF_IN_AUTOMATION=true",
        "TF_INPUT=false",
    )

    // 从workspace_variables表读取环境变量（使用 DataAccessor）
    envVars, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeEnvironment)
    if err != nil {
        log.Printf("WARNING: Failed to get environment variables for workspace %s: %v", workspace.WorkspaceID, err)
    } else {
        log.Printf("DEBUG: Loaded %d environment variables for workspace %s", len(envVars), workspace.WorkspaceID)

        // 注入环境变量
        for _, v := range envVars {
            // 跳过TF_CLI_ARGS，它会被特殊处理添加到命令参数中
            if v.Key == "TF_CLI_ARGS" {
                continue
            }
            env = append(env, fmt.Sprintf("%s=%s", v.Key, v.Value))
            log.Printf("DEBUG: Added environment variable: %s", v.Key)
        }
    }

    return env
}
```

** 正确实现**:
-  调用 GetWorkspaceVariables 获取 environment 类型变量
-  将变量 export 到 OS 环境变量（env = append(env, fmt.Sprintf("%s=%s", v.Key, v.Value))）
-  适用于 Local 和 Agent 模式（通过 DataAccessor 抽象）

### 4. Terraform Variable Files Generation

**Location**: `backend/services/terraform_executor.go`

**generateVariablesTFJSON 实现**:
```go
func (s *TerraformExecutor) generateVariablesTFJSON(
    workspace *models.Workspace,
    workDir string,
) error {
    variables := make(map[string]interface{})

    // 使用 DataAccessor 获取变量定义
    workspaceVars, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeTerraform)
    if err != nil {
        log.Printf("Warning: failed to get variables: %v", err)
        return s.writeJSONFile(workDir, "variables.tf.json", map[string]interface{}{})
    }

    for _, v := range workspaceVars {
        varDef := map[string]interface{}{
            "type": "string",
        }
        if v.Description != "" {
            varDef["description"] = v.Description
        }
        if v.Sensitive {
            varDef["sensitive"] = true
        }
        variables[v.Key] = varDef
    }

    // 如果没有变量，创建空文件
    if len(variables) == 0 {
        return s.writeJSONFile(workDir, "variables.tf.json", map[string]interface{}{})
    }

    config := map[string]interface{}{
        "variable": variables,
    }

    return s.writeJSONFile(workDir, "variables.tf.json", config)
}
```

**generateVariablesTFVars 实现**:
```go
func (s *TerraformExecutor) generateVariablesTFVars(
    workspace *models.Workspace,
    workDir string,
) error {
    var tfvars strings.Builder

    // 使用 DataAccessor 获取变量值
    workspaceVars, err := s.dataAccessor.GetWorkspaceVariables(workspace.WorkspaceID, models.VariableTypeTerraform)
    if err != nil {
        log.Printf("Warning: failed to get variables: %v", err)
        return s.writeFile(workDir, "variables.tfvars", "")
    }

    for _, v := range workspaceVars {
        // 根据ValueFormat处理
        if v.ValueFormat == models.ValueFormatHCL {
            // HCL格式处理...
        } else {
            // String格式需要加引号
            escapedValue := strings.ReplaceAll(v.Value, "\"", "\\\"")
            escapedValue = strings.ReplaceAll(escapedValue, "\n", "\\n")
            tfvars.WriteString(fmt.Sprintf("%s = \"%s\"\n", v.Key, escapedValue))
        }
    }

    return s.writeFile(workDir, "variables.tfvars", tfvars.String())
}
```

** 正确实现**:
-  调用 GetWorkspaceVariables 获取 terraform 类型变量
-  生成 variables.tf.json（变量定义）
-  生成 variables.tfvars（变量赋值）
-  适用于 Local 和 Agent 模式（通过 DataAccessor 抽象）

### 5. K8s Agent Mode

K8s Agent 模式使用与普通 Agent 模式相同的代码路径：
- 使用 RemoteDataAccessor
- 通过 GetTaskData API 获取变量数据
- 使用相同的 buildEnvironmentVariables 和 generateVariablesTFJSON/generateVariablesTFVars

## Root Cause Analysis

Task 679 变量文件为空的可能原因：

### 1.  Agent 模式依赖服务端 API

Agent 模式的变量数据来自服务端 GetTaskData API，需要检查：

**需要检查的 API**: `backend/internal/handlers/agent_handler.go` 中的 GetTaskData

关键问题：
1. GetTaskData API 是否返回了变量数据？
2. 返回的变量数据是否已经按 workspace_id, variable_id, version 过滤？
3. 是否包含了 snapshot_variables（快照变量）？

### 2.  快照变量处理

从代码中看到，Apply 阶段使用快照变量：

```go
// ExecuteApply 中
snapshotVariables, err := s.ResolveVariableSnapshots(planTask.SnapshotVariables, logger)
```

**ResolveVariableSnapshots** 支持两种格式：
1. **新格式**：只包含引用（workspace_id, variable_id, version, variable_type）
2. **旧格式**：完整的 WorkspaceVariable 数据

**潜在问题**：
- 如果 planTask.SnapshotVariables 为空或格式不正确，会导致变量文件为空
- Agent 模式下，快照变量应该通过 GetPlanTask API 获取

## GetTaskData API 实现验证

**Location**: `backend/internal/handlers/agent_handler.go`

**GetTaskData 实现**:
```go
func (h *AgentHandler) GetTaskData(c *gin.Context) {
    // ... 获取 task 和 workspace ...

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

    // Build response
    response := gin.H{
        "task":      task,
        "workspace": workspace,
        "resources": resources,
        "variables": variables,  //  正确返回变量数据
        // ...
    }
}
```

** GetTaskData API 实现正确**:
-  正确按 workspace_id 过滤
-  自动获取每个 variable_id 的最新 version（MAX(version)）
-  排除已删除的变量（is_deleted = false）
-  返回的变量数据包含所有必要字段（variable_id, version, variable_type, key, value 等）

## Recommendations

### 1.  GetTaskData API 已验证正确

### 2. 检查变量快照创建

在 Plan 阶段，应该创建变量快照：

```go
// CreateResourceVersionSnapshot 中
variableSnapshots := make([]map[string]interface{}, 0, len(variables))
for _, v := range variables {
    variableSnapshots = append(variableSnapshots, map[string]interface{}{
        "workspace_id":  v.WorkspaceID,
        "variable_id":   v.VariableID,
        "version":       v.Version,
        "variable_type": string(v.VariableType),
    })
}
```

### 3. 添加调试日志

建议在以下位置添加调试日志：

```go
// 在 generateVariablesTFJSON 中
log.Printf("[DEBUG] Task %d: Generating variables.tf.json for workspace %s", task.ID, workspace.WorkspaceID)
log.Printf("[DEBUG] Task %d: Found %d terraform variables", task.ID, len(workspaceVars))
for _, v := range workspaceVars {
    log.Printf("[DEBUG] Task %d: Variable %s (id=%s, version=%d)", task.ID, v.Key, v.VariableID, v.Version)
}
```

### 4. 验证 Task 679 的具体情况

需要检查：
1. Task 679 的 plan_task_id 是否正确？
2. Plan Task 的 snapshot_variables 字段是否有数据？
3. GetTaskData API 返回的数据中是否包含变量？

## Conclusion

**代码实现是正确的**：
-  Local 模式正确按 workspace_id, variable_id, version, variable_type 过滤
-  Environment 变量正确 export 到 OS 环境
-  Terraform 变量正确生成 variables.tf.json 和 variables.tfvars

**Task 679 问题可能原因**：
1.  GetTaskData API 未返回变量数据（Agent 模式）
2.  Plan Task 的 snapshot_variables 为空
3.  Workspace 本身没有配置变量

**下一步行动**：
1. 检查 GetTaskData API 实现
2. 查询 Task 679 的 plan_task_id 和 snapshot_variables
3. 验证 workspace ws-mb7m9ii5ey 是否有变量配置
