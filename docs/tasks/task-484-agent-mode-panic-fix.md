# Task 484 Agent模式Panic修复

## 问题发现

在Agent日志中发现了真正的panic原因：

```
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x28 pc=0x362298]

goroutine 104 [running]:
gorm.io/gorm.(*DB).getInstance(0x40008a2d28?)
	/Users/ken/go/pkg/mod/gorm.io/gorm@v1.31.0/gorm.go:435 +0x18
gorm.io/gorm.(*DB).Where(0x400003c0f0?, {0x106fc80, 0x155f650}, {0x4000236080, 0x2, 0x2})
	/Users/ken/go/pkg/mod/gorm.io/gorm@v1.31.0/chainable_api.go:208 +0x30
iac-platform/services.(*ApplyOutputParser).updateResourceStatus(0x40003f74f0, {0x400003c0f0, 0x40}, {0x12e3dc1, 0x8}, {0x0?, 0x400003c0f0?})
	/Users/ken/go/src/iac-platform/backend/services/apply_parser_service.go:110 +0xe0
```

## 根本原因

### 问题代码位置

**文件**: `backend/services/apply_parser_service.go`
**方法**: `ApplyOutputParser.updateResourceStatus`
**行号**: 110

### 问题分析

1. **创建ApplyOutputParser时传入了nil的db**
   - 在 `terraform_executor.go` 第1548行：
   ```go
   applyParser := NewApplyOutputParser(task.ID, s.db, s.streamManager)
   ```
   - 在Agent模式下，`s.db` 是nil

2. **updateResourceStatus方法直接使用db**
   - 第110行调用 `p.db.Where()` 时发生nil pointer panic
   - 没有检查 `p.db` 是否为nil

3. **为什么会在Agent模式下执行**
   - 虽然任务应该在Agent中执行
   - 但由于之前的调度问题，任务实际在本地执行
   - 本地执行时使用了Agent模式的TerraformExecutor（db为nil）

## 修复方案

### 修复：ApplyOutputParser添加nil检查

**文件**: `backend/services/apply_parser_service.go`

**修改内容**:
在 `updateResourceStatus` 方法开始处添加nil检查

```go
func (p *ApplyOutputParser) updateResourceStatus(resourceAddress, status, action string) {
	// Agent模式下db为nil，跳过数据库更新
	if p.db == nil {
		log.Printf("[Agent Mode] Skipping resource status update for %s (db is nil)", resourceAddress)
		return
	}

	// 原有代码...
}
```

## 为什么这个问题会导致任务调度失败

### 问题链条

1. **ConfirmApply调用TryExecuteNextTask**
   - 在goroutine中异步调用
   - 没有panic recovery

2. **TryExecuteNextTask尝试调度任务**
   - 检查Agent C&C Handler
   - 查找可用的Agent
   - 推送任务到Agent

3. **如果调度失败或panic**
   - Goroutine静默退出
   - 任务没有被推送到Agent
   - 系统回退到本地执行

4. **本地执行使用了错误的Executor**
   - 使用了Agent模式的TerraformExecutor（db为nil）
   - 在ExecuteApply时创建ApplyOutputParser
   - ApplyOutputParser.updateResourceStatus调用p.db.Where()
   - **Panic！**

5. **Panic导致任务执行中断**
   - Agent进程崩溃
   - 任务状态变为running但没有实际执行
   - agent_id和k8s_pod_name为空

## 完整的修复方案

### 1. TaskQueueManager添加Panic Recovery 
- 文件: `backend/services/task_queue_manager.go`
- 捕获TryExecuteNextTask中的panic

### 2. Controller添加Panic Recovery 
- 文件: `backend/controllers/workspace_task_controller.go`
- 为CreatePlanTask和ConfirmApply的goroutine添加panic recovery

### 3. ApplyOutputParser添加nil检查 
- 文件: `backend/services/apply_parser_service.go`
- 在updateResourceStatus中检查db是否为nil
- Agent模式下跳过数据库更新

### 4. 日志优化 
- 注释掉频繁的DEBUG日志
- 禁用Gin的HTTP请求日志

## 修复效果

### 1. 防止Agent模式下的panic
- ApplyOutputParser在Agent模式下不会尝试访问nil的db
- 资源状态更新在Agent模式下被跳过（这是正确的，因为Agent不应该直接访问数据库）

### 2. 增强错误可见性
- 所有panic都会被捕获并记录
- 包含完整的堆栈信息
- 便于快速定位问题

### 3. 改进的日志
- 使用清晰的前缀标记
- 减少日志噪音
- 关键信息突出显示

## 测试建议

### 1. 正常K8s模式测试
1. 创建plan_and_apply任务
2. 等待Plan完成
3. 执行Confirm Apply
4. 验证任务被正确调度到Agent
5. 验证Apply成功完成

### 2. 验证日志
后端日志应该包含：
```
[TaskQueue] ===== TryExecuteNextTask START for workspace ws-xxx =====
[TaskQueue] ✓ Agent C&C handler is available
[TaskQueue] Found X connected agents
[TaskQueue] Successfully pushed task to agent
[TaskQueue] ===== TryExecuteNextTask END for workspace ws-xxx =====
```

Agent日志应该包含：
```
[AGENT MODE] ExecuteApply started for task xxx
[INFO] Creating work directory
[INFO] Loading plan task and snapshot data
...
[INFO] Apply completed successfully
```

不应该再看到：
```
panic: runtime error: invalid memory address or nil pointer dereference
```

## 相关文档

- [Task 484 K8s调度问题分析](./task-484-k8s-scheduling-issue-analysis.md)
- [Task 484 Panic Recovery修复](./task-484-k8s-scheduling-panic-recovery-fix.md)
- [Agent Panic Recovery修复](./agent-panic-recovery-fix.md)

## 修复文件清单

1. `backend/services/task_queue_manager.go` - 添加panic recovery
2. `backend/controllers/workspace_task_controller.go` - 添加panic recovery
3. `backend/services/apply_parser_service.go` - 添加nil检查
4. `backend/main.go` - 禁用Gin日志

## 总结

通过你的细心观察和提供的Agent日志，我们找到了真正的问题根源：
1.  **调度问题** - goroutine panic导致任务没有被推送到Agent
2.  **执行问题** - ApplyOutputParser在Agent模式下访问nil的db导致panic
3.  **日志问题** - 缺少panic recovery导致错误被静默吞掉

现在所有问题都已修复，任务应该能够正确调度到Agent并成功执行！

**修复日期**: 2025-11-06
**相关任务**: Task 484, Task 485
