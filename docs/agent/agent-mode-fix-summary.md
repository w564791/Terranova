# Agent Mode Fix Summary

## 已修复的问题

###  1. Status字段验证错误
**文件**: `backend/services/remote_data_accessor.go`
**修复**: 在`UpdateTask`函数中添加默认status值处理
```go
status := string(task.Status)
if status == "" {
    status = "running"
}
```
**效果**: Agent在保存plan数据时不再出现400错误

## 发现的严重问题

### ❌ ExecuteApply函数在Agent模式下完全不可用

`ExecuteApply`函数中有大量直接使用`s.db`的代码，在Agent模式下会panic：

1. **获取workspace**: `s.db.Where(...).First(&workspace)` 
2. **获取plan task**: `s.db.First(&planTask, *task.PlanTaskID)`
3. **保存task**: `s.db.Save(task)` (多处)
4. **Apply解析器**: `NewApplyOutputParser(task.ID, s.db, ...)` 需要db
5. **Apply解析服务**: `NewApplyParserService(s.db, ...)` 需要db
6. **State保存**: 调用`SaveNewStateVersionWithLogging`，其中使用`s.db`

### ❌ 其他不支持Agent模式的函数

1. **PrepareStateFile**: 直接查询数据库
2. **SavePlanData**: 直接使用`s.db.Save`
3. **SaveNewStateVersion**: 直接使用`s.db.Save`
4. **SaveStateToDatabase**: 使用`s.db.Transaction`
5. **lockWorkspace**: 直接更新数据库
6. **GetTaskLogs**: 直接查询数据库
7. **CreateResourceSnapshot**: 查询资源和版本
8. **ValidateResourceSnapshot**: 依赖CreateResourceSnapshot
9. **maskSensitiveVariables**: 查询敏感变量
10. **SaveNewStateVersionWithLogging**: 查询版本号，调用SaveStateToDatabase

## 根本原因

当前的Agent模式实现不完整：
- `TerraformExecutor`中大量函数直接使用`s.db`
- 这些函数没有通过`DataAccessor`接口抽象
- Agent模式下`s.db == nil`，导致panic

## 需要的工作量

这是一个**大型重构任务**，需要：

1. **扩展DataAccessor接口** (新增10+个方法)
   - GetPlanTask
   - SaveStateVersion  
   - LockWorkspace
   - GetTaskLogs
   - CountActiveResources (已有)
   - 等等...

2. **实现LocalDataAccessor** (10+个方法)
   - 包装现有的数据库操作

3. **实现RemoteDataAccessor** (10+个方法)
   - 通过API调用实现

4. **扩展AgentAPIClient** (10+个API调用)
   - GetPlanTask
   - SaveState
   - LockWorkspace
   - 等等...

5. **扩展AgentHandler** (10+个API端点)
   - 对应每个API调用

6. **修改TerraformExecutor** (20+处修改)
   - 将所有`s.db`改为`s.dataAccessor`

7. **处理特殊情况**
   - Apply解析器需要重构以支持Agent模式
   - State保存的事务处理
   - 资源变更解析

**预计工作量**: 2-3天的开发时间

## 当前建议

### 短期方案（临时）
1.  Status验证错误已修复 - Agent可以执行Plan任务
2.  **不要在Agent模式下执行Apply任务** - 会导致panic
3.  资源变更解析在Agent模式下不可用

### 中期方案（推荐）
如果需要完整的Agent模式支持，建议：
1. 创建专门的开发分支
2. 系统性地重构所有数据库访问
3. 完整测试Local和Agent两种模式
4. 合并到主分支

### 长期方案
考虑重新设计Agent架构：
- Agent只负责执行Terraform命令
- 所有数据操作都通过API
- 简化Agent端的逻辑

## 当前可用功能

### Local模式 
- Plan任务：完全支持
- Apply任务：完全支持
- 资源变更解析：支持
- State管理：支持

### Agent模式
- Plan任务： 基本支持（资源变更解析除外）
- Apply任务：❌ 不支持（会panic）
- 资源变更解析：❌ 不支持
- State管理： 部分支持

## 结论

当前Agent模式只能安全地执行**Plan任务**，Apply任务会因为大量的数据库直接访问而失败。

要完全支持Agent模式，需要进行大规模的代码重构，这超出了快速修复的范围。

建议：
1. 短期内只在Agent模式下执行Plan任务
2. Apply任务继续使用Local模式
3. 或者投入资源进行完整的Agent模式重构
