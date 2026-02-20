# Plan+Apply流程重设计 - 需求验证

## 原始需求回顾

### 用户需求
1. **Plan任务** (task_type = "plan"): 单独的任务，只执行Plan
2. **Plan+Apply任务** (task_type = "plan_and_apply"): **一个任务**，包含Plan和Apply两个阶段

### 关键要求
-  Plan+Apply是**一个任务**，不是两个
-  Plan完成后可以中断
-  Apply使用数据库中的Plan数据（强制）
-  需要资源版本快照机制
-  Apply时验证资源版本是否变化

## 设计验证

###  1. 任务类型设计

**需求**: Plan+Apply是一个任务

**设计**:
```go
const (
    TaskTypePlan         TaskType = "plan"          // 单独Plan
    TaskTypeApply        TaskType = "apply"         // 单独Apply（向后兼容）
    TaskTypePlanAndApply TaskType = "plan_and_apply" //  一个任务
)
```

**验证**:  符合需求
- 创建时只创建一个task记录
- task_type = "plan_and_apply"
- 不再创建两个独立的任务

###  2. 状态流转设计

**需求**: Plan完成后可以中断，等待用户确认

**设计**:
```go
const (
    TaskStatusPending       // 待执行
    TaskStatusRunning       // 执行中
    TaskStatusPlanCompleted //  Plan完成，可以中断
    TaskStatusApplyPending  //  等待用户确认
    TaskStatusSuccess       // 成功
    TaskStatusFailed        // 失败
    TaskStatusCancelled     // 已取消
)
```

**流程**:
```
pending → running(planning) → plan_completed → 
[用户确认] → apply_pending → running(applying) → success
```

**验证**:  符合需求
- Plan完成后进入`plan_completed`状态
- 可以在此状态中断、取消或继续
- 需要用户主动调用ConfirmApply才继续

###  3. Plan数据存储

**需求**: Apply使用数据库中的Plan数据（强制）

**设计**:
```go
type WorkspaceTask struct {
    PlanData []byte `json:"-" gorm:"type:bytea"` //  Plan二进制数据
    PlanJSON JSONB  `json:"plan_json"`           //  Plan JSON数据
}
```

**实现**:
```go
// Plan完成时保存
task.PlanData = planFileContent
task.PlanJSON = planJSON
db.Save(task)

// Apply时读取
planFile := restorePlanFromDatabase(task.PlanData)
terraform apply planFile
```

**验证**:  符合需求
- Plan数据强制保存到数据库
- Apply必须从数据库读取
- 不依赖文件系统

###  4. 资源版本快照机制

**需求**: 创建资源版本快照，Apply时验证

**设计**:
```go
type WorkspaceTask struct {
    SnapshotID string `json:"snapshot_id"` //  快照ID
}

// Plan完成时创建快照
snapshotID := CreateResourceSnapshot(workspaceID)
task.SnapshotID = snapshotID

// Apply前验证
currentSnapshotID := CreateResourceSnapshot(workspaceID)
if currentSnapshotID != task.SnapshotID {
    return error("资源已变化")
}
```

**快照内容**:
```go
type ResourceSnapshot struct {
    ResourceID string // 资源ID
    VersionID  uint   // 版本ID
    Checksum   string // 校验和
}
```

**验证**:  符合需求
- Plan完成时自动创建快照
- 记录所有资源的版本信息
- Apply前强制验证
- 如果资源变化，拒绝Apply

###  5. API设计

**需求**: 创建Plan+Apply任务，确认Apply

**设计**:

**创建任务**:
```
POST /api/v1/workspaces/:id/tasks/plan
Body: {
  "run_type": "plan_and_apply",  //  指定类型
  "description": "..."
}

Response: {
  "task": {
    "id": 123,
    "task_type": "plan_and_apply", //  一个任务
    "status": "pending"
  }
}
```

**确认Apply**:
```
POST /api/v1/workspaces/:id/tasks/:task_id/confirm-apply
Body: {
  "apply_description": "..."  //  Apply描述
}

Response: {
  "message": "Apply started",
  "task": {
    "status": "running",
    "stage": "applying"
  }
}
```

**验证**:  符合需求
- 创建时只返回一个任务
- 有专门的ConfirmApply接口
- 支持Apply描述

###  6. 完整工作流程

**需求**: 完整的Plan+Apply流程

**设计流程**:
```
1. 用户创建Plan+Apply任务
   POST /tasks/plan { run_type: "plan_and_apply" }
   ↓
   创建一个任务: task_type = "plan_and_apply"

2. 系统执行Plan
   status = "running", stage = "planning"
   ↓
   执行terraform plan
   ↓
   保存Plan数据到plan_data
   创建资源快照到snapshot_id
   ↓
   status = "plan_completed"

3. 前端显示Plan结果
   - 显示资源变更统计
   - 显示"Confirm Apply"按钮
   - 用户可以查看详细Plan输出

4. 用户确认Apply
   POST /tasks/:id/confirm-apply { apply_description: "..." }
   ↓
   验证资源版本快照
   ↓
   如果资源未变化:
     status = "running", stage = "applying"
     从plan_data恢复Plan文件
     执行terraform apply
     ↓
     status = "success"
   
   如果资源已变化:
     返回409 Conflict错误
     提示用户重新Plan
```

**验证**:  完全符合需求

###  7. 向后兼容性

**需求**: 不影响现有功能

**设计**:
- 保留`TaskTypePlan`和`TaskTypeApply`
- 现有的两任务模式继续工作
- 新增`TaskTypePlanAndApply`作为新选项
- API保持兼容，只是新增参数

**验证**:  符合需求

## 设计完整性检查

###  数据库层
- [x] 添加snapshot_id字段
- [x] 添加apply_description字段
- [x] 添加新的TaskType枚举值
- [x] 添加新的TaskStatus枚举值
- [x] 创建迁移脚本

###  模型层
- [x] 更新TaskType枚举
- [x] 更新TaskStatus枚举
- [x] 添加SnapshotID字段
- [x] 添加ApplyDescription字段

###  服务层
- [x] CreateResourceSnapshot方法
- [x] ValidateResourceSnapshot方法
- [ ] ExecutePlanAndApply方法（待实现）
- [ ] ExecuteApplyPhase方法（待实现）

###  控制器层
- [ ] 修改CreatePlanTask支持run_type（待实现）
- [ ] 实现ConfirmApply接口（待实现）

###  路由层
- [ ] 添加confirm-apply路由（待实现）

###  前端层
- [ ] NewRunDialog添加Run Type选择（待实现）
- [ ] TaskDetail添加Confirm Apply按钮（待实现）
- [ ] 实现Confirm Apply对话框（待实现）

## 与原始需求的对比

| 需求项 | 原始需求 | 当前设计 | 状态 |
|--------|----------|----------|------|
| 任务类型 | Plan+Apply是一个任务 | task_type = "plan_and_apply" |  完全符合 |
| Plan完成可中断 | 支持 | status = "plan_completed" |  完全符合 |
| Plan数据存储 | 强制使用数据库 | plan_data字段 |  完全符合 |
| 资源版本快照 | 需要 | snapshot_id + 验证机制 |  完全符合 |
| Apply验证 | 验证资源版本 | ValidateResourceSnapshot |  完全符合 |
| 用户确认 | 需要用户输入描述 | apply_description |  完全符合 |
| 向后兼容 | 不影响现有功能 | 保留旧类型 |  完全符合 |

## 设计优势

### 1. 清晰的状态管理
- 每个状态都有明确的含义
- 状态转换逻辑清晰
- 易于监控和调试

### 2. 强制的数据一致性
- Plan数据必须保存到数据库
- Apply必须使用数据库中的Plan
- 资源版本强制验证

### 3. 良好的用户体验
- 一个任务ID贯穿始终
- Plan完成后可以查看结果
- 明确的确认步骤
- 清晰的错误提示

### 4. 完整的审计追踪
- 记录Plan时的资源版本
- 记录Apply描述
- 完整的状态变更历史

### 5. 安全性保障
- 防止使用过期的Plan
- 防止资源版本不一致
- 明确的权限检查点

## 潜在风险和缓解措施

### 风险1: 资源快照性能
**风险**: 大量资源时快照创建可能较慢
**缓解**: 
- 异步创建快照
- 只记录关键信息（ID、版本、checksum）
- 使用索引优化查询

### 风险2: Plan数据存储
**风险**: Plan文件可能很大
**缓解**:
- 使用bytea类型存储
- 考虑压缩
- 定期清理旧Plan数据

### 风险3: 并发问题
**风险**: 多人同时操作同一workspace
**缓解**:
- Workspace锁定机制
- 乐观锁
- 明确的错误提示

## 结论

 **设计完全符合原始需求**

当前设计：
1.  Plan+Apply是一个任务（不是两个）
2.  Plan完成后可以中断
3.  Apply强制使用数据库中的Plan数据
4.  实现了资源版本快照机制
5.  Apply时验证资源版本
6.  保持向后兼容性

**已完成**:
- 设计文档
- 数据库迁移
- 模型层更新
- 资源快照机制

**待完成**:
- 控制器实现
- 路由配置
- 前端UI

**预计完成时间**: 4-7小时
