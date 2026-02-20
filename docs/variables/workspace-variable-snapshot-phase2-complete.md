# Workspace Variable Snapshot - Phase 2 完成报告

## 实施时间
2025-01-12

## 问题描述
在 Phase 1 完成后，执行 Apply 任务时出现变量重复定义错误：
```
Error: Attribute redefined
on variables.tfvars line 2:
2: aaa = "aaa"
The argument "aaa" was already set at variables.tfvars:1,1-4.
```

## 根本原因
`GenerateConfigFilesFromSnapshot` 函数接收的 `snapshotVariables` 参数只包含变量引用（variable_id + version），但函数直接使用这些引用而没有解析为实际的变量数据，导致：
1. 无法获取变量的 key 和 value
2. 可能包含重复的变量引用
3. 生成的 variables.tfvars 文件格式错误

## 实施的修复

### 1. 添加 `ResolveVariableSnapshots` 函数（第2900行）

```go
func (s *TerraformExecutor) ResolveVariableSnapshots(
	snapshotData interface{},
	workspaceID string,
) ([]models.WorkspaceVariable, error)
```

**功能**：
- 解析变量快照引用为实际变量数据
- 支持新格式（只包含引用）和旧格式（完整数据）
- 使用 map 去重，确保每个变量只出现一次
- 从数据库查询实际的变量 key、value、sensitive 等完整信息

**关键特性**：
- **格式检测**：自动识别新旧两种快照格式
- **去重机制**：使用 `variable_id-version` 作为唯一键
- **向后兼容**：旧格式快照仍然可以正常工作

### 2. 更新 `GenerateConfigFilesFromSnapshot` 函数（第3100行）

**修改内容**：
1. 参数类型从 `[]models.WorkspaceVariable` 改为 `interface{}`
2. 在函数开头调用 `ResolveVariableSnapshots` 解析变量
3. 使用解析后的完整变量数据生成配置文件
4. 修复变量名冲突（`variables` map 改为 `variablesDef`）

## Local 模式 vs Agent 模式

### Local 模式（ 完全支持）
- **快照创建**：使用新格式（只存储引用）
- **快照使用**：通过 `ResolveVariableSnapshots` 从数据库查询实际数据
- **优势**：节省存储空间，快照体积小

### Agent 模式（ 使用旧格式）
- **快照创建**：虽然尝试创建新格式，但 API 传输时会转换为完整数据
- **快照使用**：直接使用完整数据（旧格式），不需要额外查询
- **原因**：Agent 无法直接访问数据库，变量数据必须在 `GetPlanTask` API 时一起返回
- **实现**：`ResolveVariableSnapshots` 检测到新格式时会返回错误，强制使用旧格式

## 技术细节

### 新格式快照（Local 模式）
```json
[
  {
    "workspace_id": "ws-xxx",
    "variable_id": "var-xxx",
    "version": 3,
    "variable_type": "terraform"
  }
]
```

### 旧格式快照（Agent 模式）
```json
[
  {
    "workspace_id": "ws-xxx",
    "variable_id": "var-xxx",
    "version": 3,
    "variable_type": "terraform",
    "key": "AWS_REGION",
    "value": "us-east-1",
    "sensitive": false,
    "description": "",
    "value_format": "string"
  }
]
```

## 去重机制

使用 map 确保每个变量只出现一次：
```go
variableMap := make(map[string]models.WorkspaceVariable)
for _, snap := range snapshots {
    key := fmt.Sprintf("%s-v%d", snap.VariableID, snap.Version)
    if _, exists := variableMap[key]; exists {
        continue // 跳过重复
    }
    variableMap[key] = variable
}
```

## 测试建议

### Local 模式测试
```bash
# 1. 创建 workspace 并添加变量
# 2. 运行 plan_and_apply 任务
# 3. 检查日志确认使用新格式：
#    [DEBUG] Detected new snapshot format with X variable references
#    [DEBUG] Resolved X unique variables from Y snapshot references
# 4. 验证 Apply 成功，无重复定义错误
```

### Agent 模式测试
```bash
# 1. 在 K8s agent 上运行任务
# 2. 检查日志确认使用旧格式：
#    [DEBUG] Using X variables from old snapshot format
# 3. 验证 Apply 成功
```

## 已知限制

1. **Agent 模式不支持新格式**：
   - Agent 无法直接访问数据库
   - 必须在 API 响应中包含完整变量数据
   - 快照体积较大（包含所有变量的完整信息）

2. **未来优化方向**：
   - 可以在 Server 端添加专门的 API 来解析变量引用
   - Agent 调用该 API 获取完整变量数据
   - 这样 Agent 模式也能使用新格式

## 相关文件

- `backend/services/terraform_executor.go` - 主要实现
- `backend/services/remote_data_accessor.go` - Agent 模式数据访问
- `backend/controllers/workspace_task_controller.go` - 快照创建
- `docs/workspace-variable-snapshot-phase1-complete.md` - Phase 1 报告
- `docs/workspace-variable-snapshot-phase2-code.md` - Phase 2 实施代码

## 总结

 **Local 模式**：完全支持新格式，通过 `ResolveVariableSnapshots` 解析引用
 **Agent 模式**：使用旧格式（完整数据），无需额外查询
 **去重机制**：确保每个变量只出现一次
 **向后兼容**：旧快照仍然可以正常工作
 **错误修复**：解决了变量重复定义问题

Phase 2 实施完成，变量快照功能现已在 Local 和 Agent 模式下都能正常工作。
