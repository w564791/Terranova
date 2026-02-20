# Agent Mode Database Access Audit

## 问题概述
Agent模式下，`TerraformExecutor`不能直接访问数据库（`s.db == nil`），必须通过`DataAccessor`接口或API调用来操作数据。

## 需要修复的函数

### 1. ExecutePlan (部分支持)
**状态**: 🟡 部分支持
**问题**:
- Line ~370: `s.db.Where(...).First(&tfLogVar)` - 读取TF_LOG变量
- Line ~470: `s.db.Save(task)` - 保存snapshot_id
- Line ~476: `if s.db != nil` - 资源变更解析被跳过

**修复方案**:
- TF_LOG读取：通过DataAccessor.GetWorkspaceVariables
- snapshot_id保存：通过DataAccessor.UpdateTask
- 资源变更解析：需要新增API支持

### 2. ExecuteApply (不支持Agent模式)
**状态**: ❌ 完全不支持
**问题**:
- Line ~550: `s.db.Where(...).First(&workspace)` - 获取workspace
- Line ~650: `s.db.First(&planTask)` - 获取plan task
- Line ~730: `s.db.Save(task)` - 多处保存task
- Line ~780: `NewApplyOutputParser(task.ID, s.db, ...)` - Apply解析器需要db
- Line ~850: `NewApplyParserService(s.db, ...)` - Apply解析服务需要db

**修复方案**:
- 使用DataAccessor替代所有s.db操作
- Apply解析器和服务需要支持Agent模式

### 3. PrepareStateFile (不支持Agent模式)
**状态**: ❌ 完全不支持
**问题**:
- Line ~250: `s.db.Where(...).Order(...).First(&stateVersion)` - 直接查询数据库

**修复方案**:
- 已有PrepareStateFileWithLogging使用DataAccessor，应该统一使用

### 4. SavePlanData (不支持Agent模式)
**状态**: ❌ 完全不支持
**问题**:
- Line ~540: `s.db.Save(task)` - 直接保存到数据库

**修复方案**:
- 这个函数已被SavePlanDataWithLogging替代，应该删除或标记为deprecated

### 5. SaveNewStateVersion (不支持Agent模式)
**状态**: ❌ 完全不支持
**问题**:
- Line ~700: `s.db.Save(task)` - 直接保存task

**修复方案**:
- 已有SaveNewStateVersionWithLogging，应该统一使用

### 6. SaveStateToDatabase (不支持Agent模式)
**状态**: ❌ 完全不支持
**问题**:
- Line ~730: `s.db.Model(...).Select(...).Scan(&maxVersion)` - 查询最大版本
- Line ~740: `s.db.Transaction(...)` - 使用事务

**修复方案**:
- 需要通过DataAccessor.SaveStateVersion

### 7. lockWorkspace (不支持Agent模式)
**状态**: ❌ 完全不支持
**问题**:
- Line ~760: `s.db.Model(...).Updates(...)` - 直接更新数据库

**修复方案**:
- 需要新增DataAccessor.LockWorkspace方法

### 8. GetTaskLogs (不支持Agent模式)
**状态**: ❌ 完全不支持
**问题**:
- Line ~775: `s.db.Where(...).Find(&logs)` - 直接查询数据库

**修复方案**:
- 需要新增DataAccessor.GetTaskLogs方法

### 9. CreateResourceSnapshot (不支持Agent模式)
**状态**: ❌ 完全不支持
**问题**:
- Line ~800: `s.db.Where(...).Find(&resources)` - 查询资源
- Line ~810: `s.db.First(&version)` - 查询版本

**修复方案**:
- 使用DataAccessor.GetWorkspaceResources和GetResourceVersion

### 10. ValidateResourceSnapshot (不支持Agent模式)
**状态**: ❌ 完全不支持
**问题**:
- 依赖CreateResourceSnapshot

**修复方案**:
- 修复CreateResourceSnapshot后自动支持

### 11. maskSensitiveVariables (不支持Agent模式)
**状态**: ❌ 完全不支持
**问题**:
- Line ~920: `s.db.Where(...).Find(&sensitiveVars)` - 查询敏感变量

**修复方案**:
- 使用DataAccessor.GetWorkspaceVariables

### 12. SaveNewStateVersionWithLogging (不支持Agent模式)
**状态**: ❌ 完全不支持
**问题**:
- Line ~1050: `s.db.Model(...).Select(...).Scan(&maxVersion)` - 查询最大版本
- 调用SaveStateToDatabase

**修复方案**:
- 使用DataAccessor.SaveStateVersion

## 修复优先级

### P0 - 关键功能（必须修复）
1. ExecuteApply - Apply功能完全不可用
2. SaveStateToDatabase - State保存失败
3. lockWorkspace - 错误处理失败

### P1 - 重要功能（应该修复）
4. CreateResourceSnapshot - Plan+Apply流程受影响
5. maskSensitiveVariables - 日志可能泄露敏感信息
6. 资源变更解析 - Structured Run Output不可用

### P2 - 次要功能（可以延后）
7. GetTaskLogs - 日志查询功能
8. PrepareStateFile - 已有替代方法

## 实施计划

1. 扩展DataAccessor接口，添加缺失的方法
2. 在LocalDataAccessor中实现这些方法
3. 在RemoteDataAccessor中通过API实现这些方法
4. 在AgentAPIClient中添加对应的API调用方法
5. 在AgentHandler中添加对应的API端点
6. 修改terraform_executor.go使用DataAccessor而不是s.db
7. 测试验证所有功能

## 当前状态
-  Status验证错误已修复
-  Agent模式下大部分数据库操作不支持
-  ExecuteApply在Agent模式下会panic
