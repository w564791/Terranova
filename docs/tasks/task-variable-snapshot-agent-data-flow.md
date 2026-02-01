# Agent模式下变量快照数据流程

## 完整数据流

### 1. Plan阶段 - 创建快照
```
CreateResourceVersionSnapshot (terraform_executor.go)
  ↓
保存到数据库: snapshot_variables = [
  {"variable_id": "var-xxx", "version": 1, "workspace_id": "ws-xxx", "variable_type": "terraform"}
]
```

### 2. Apply阶段 - Agent请求Plan任务数据
```
Agent调用: GET /api/v1/agents/tasks/{plan_task_id}/plan-task
  ↓
GetPlanTask (agent_handler.go)
  ↓
检测快照格式:
  - 如果snapshot_variables没有key字段(旧格式/引用格式)
  - 从数据库查询完整变量数据
  ↓
返回完整数据: snapshot_variables = [
  {
    "variable_id": "var-xxx",
    "version": 1,
    "workspace_id": "ws-xxx",
    "variable_type": "terraform",
    "key": "my_variable",        // ← 从数据库查询得到
    "value": "my_value",         // ← 从数据库查询得到
    "sensitive": false,
    "description": "...",
    "value_format": "string"
  }
]
  ↓
将snapshot_variables放入task.context["_snapshot_variables"]
```

### 3. Agent侧 - 缓存数据
```
RemoteDataAccessor.GetPlanTask (remote_data_accessor.go)
  ↓
从task.Context中提取_snapshot_variables
  ↓
缓存到taskData["snapshot_variables"]
```

### 4. Apply阶段 - 生成配置文件
```
GenerateConfigFilesFromSnapshot (terraform_executor.go)
  ↓
ResolveVariableSnapshots
  ↓
检测格式:
  - 如果有key字段 → 直接使用(新格式/完整数据)
  - 如果没有key字段 → 从数据库查询(旧格式/引用)
  ↓
返回完整的WorkspaceVariable数组
  ↓
生成variables.tf.json:
{
  "variable": {
    "my_variable": {      // ← 使用variable.Key
      "type": "string"
    }
  }
}
  ↓
生成variables.tfvars:
my_variable = "my_value"  // ← 使用variable.Key和variable.Value
```

## 关键修复点

### 修复1: GetPlanTask API (agent_handler.go)
```go
// 检测快照是否是引用格式
if _, hasKey := firstVar["key"]; !hasKey {
    // 从数据库查询完整数据
    var fullVariables []gin.H
    for _, snapVar := range snapshotVars {
        varID, _ := snapVar["variable_id"].(string)
        version, _ := snapVar["version"].(float64)
        
        var variable models.WorkspaceVariable
        if err := h.db.Where("variable_id = ? AND version = ?", varID, int(version)).
            First(&variable).Error; err == nil {
            fullVariables = append(fullVariables, gin.H{
                "key": variable.Key,      // ← 关键字段
                "value": variable.Value,  // ← 关键字段
                // ... 其他字段
            })
        }
    }
    taskResponse["snapshot_variables"] = fullVariables
}

// 将完整数据放入context
contextMap["_snapshot_variables"] = taskResponse["snapshot_variables"]
```

### 修复2: RemoteDataAccessor (remote_data_accessor.go)
```go
// 缓存snapshot_variables
if snapshotVariables, ok := task.Context["_snapshot_variables"].([]interface{}); ok {
    a.taskData["snapshot_variables"] = snapshotVariables
    log.Printf("[RemoteDataAccessor] Cached %d snapshot variables", len(snapshotVariables))
}
```

### 修复3: ResolveVariableSnapshots (terraform_executor.go)
```go
// 检测是否包含完整数据
if keyVal, hasKey := firstSnap["key"]; hasKey && keyVal != nil {
    // 新格式(完整数据):直接从map构建WorkspaceVariable
    variable := models.WorkspaceVariable{
        Key:   getString(snap, "key"),
        Value: getString(snap, "value"),
        // ...
    }
    variables = append(variables, variable)
}
```

## 验证步骤

1. 重启backend服务
2. 触发一个plan+apply任务
3. 在plan完成后,删除pod
4. Apply阶段会:
   - 调用GetPlanTask API获取plan任务数据
   - API检测到快照是引用格式,从数据库查询完整数据
   - 返回包含key和value的完整变量数据
   - RemoteDataAccessor缓存这些数据
   - ResolveVariableSnapshots识别为完整数据格式
   - 直接使用这些数据生成terraform配置文件

## 预期结果

**variables.tf.json:**
```json
{
  "variable": {
    "my_variable": {
      "type": "string",
      "description": "My variable description"
    }
  }
}
```

**variables.tfvars:**
```
my_variable = "my_value"
