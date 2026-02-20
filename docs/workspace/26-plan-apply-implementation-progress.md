# Plan+Apply Flow Redesign - Implementation Progress

## 已完成的工作

### 1. 设计文档 
- 创建了完整的设计文档 `25-plan-apply-redesign.md`
- 定义了新的TaskType、TaskStatus和工作流程
- 设计了资源版本快照机制

### 2. 数据库迁移 
- 创建迁移脚本 `scripts/migrate_plan_apply_redesign.sql`
- 添加字段：
  - `snapshot_id` VARCHAR(64) - 资源版本快照ID
  - `apply_description` TEXT - Apply描述
- 已执行迁移（需要验证）

### 3. 模型层更新 
**文件**: `backend/internal/models/workspace.go`

添加的枚举：
```go
// TaskType
const (
    TaskTypePlan         TaskType = "plan"
    TaskTypeApply        TaskType = "apply"
    TaskTypePlanAndApply TaskType = "plan_and_apply" // 新增
)

// TaskStatus
const (
    TaskStatusPending       TaskStatus = "pending"
    TaskStatusWaiting       TaskStatus = "waiting"
    TaskStatusRunning       TaskStatus = "running"
    TaskStatusPlanCompleted TaskStatus = "plan_completed" // 新增
    TaskStatusApplyPending  TaskStatus = "apply_pending"  // 新增
    TaskStatusSuccess       TaskStatus = "success"
    TaskStatusFailed        TaskStatus = "failed"
    TaskStatusCancelled     TaskStatus = "cancelled"
)
```

添加的字段：
```go
type WorkspaceTask struct {
    // ... 现有字段 ...
    
    // Plan+Apply流程字段
    SnapshotID       string `json:"snapshot_id" gorm:"type:varchar(64)"`
    ApplyDescription string `json:"apply_description" gorm:"type:text"`
}
```

### 4. 资源快照机制 
**文件**: `backend/services/terraform_executor.go`

实现的函数：
- `CreateResourceSnapshot(workspaceID uint) (string, error)`
  - 获取workspace的所有活跃资源
  - 记录每个资源的版本ID和checksum
  - 生成快照ID（SHA256 hash）
  
- `ValidateResourceSnapshot(task *WorkspaceTask) error`
  - 对比当前资源快照与保存的快照ID
  - 如果不匹配，返回错误

## 待完成的工作

### 5. 修改CreatePlanTask Controller ⏳
**文件**: `backend/controllers/workspace_task_controller.go`

需要修改：
```go
func (c *WorkspaceTaskController) CreatePlanTask(ctx *gin.Context) {
    // 1. 解析run_type参数
    var req struct {
        Description string `json:"description"`
        RunType     string `json:"run_type"` // "plan" 或 "plan_and_apply"
    }
    
    // 2. 根据run_type创建对应的任务类型
    var taskType models.TaskType
    if req.RunType == "plan_and_apply" {
        taskType = models.TaskTypePlanAndApply
    } else {
        taskType = models.TaskTypePlan
    }
    
    // 3. 创建单个任务（不再创建Apply任务）
    task := &models.WorkspaceTask{
        TaskType: taskType,
        // ...
    }
    
    // 4. 调用新的ExecutePlanAndApply方法
    go func() {
        if err := c.executor.ExecutePlanAndApply(execCtx, task); err != nil {
            // 处理错误
        }
    }()
}
```

### 6. 实现ExecutePlanAndApply方法 ⏳
**文件**: `backend/services/terraform_executor.go`

需要添加：
```go
func (s *TerraformExecutor) ExecutePlanAndApply(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 执行Plan阶段
    if err := s.ExecutePlan(ctx, task); err != nil {
        return err
    }
    
    // 2. Plan完成后创建资源快照
    snapshotID, err := s.CreateResourceSnapshot(task.WorkspaceID)
    if err != nil {
        log.Printf("Warning: Failed to create snapshot: %v", err)
    } else {
        task.SnapshotID = snapshotID
        s.db.Save(task)
    }
    
    // 3. 如果是plan_and_apply类型，更新状态并等待确认
    if task.TaskType == models.TaskTypePlanAndApply {
        task.Status = models.TaskStatusPlanCompleted
        task.Stage = "plan_completed"
        s.db.Save(task)
        return nil // 等待用户调用ConfirmApply
    }
    
    // 4. 如果是单独的plan任务，直接完成
    task.Status = models.TaskStatusSuccess
    task.Stage = "completed"
    s.db.Save(task)
    
    return nil
}
```

### 7. 实现ConfirmApply API ⏳
**文件**: `backend/controllers/workspace_task_controller.go`

需要添加：
```go
func (c *WorkspaceTaskController) ConfirmApply(ctx *gin.Context) {
    // 1. 解析参数
    taskID := ctx.Param("task_id")
    var req struct {
        ApplyDescription string `json:"apply_description"`
    }
    
    // 2. 获取任务并验证状态
    var task models.WorkspaceTask
    if task.Status != models.TaskStatusPlanCompleted {
        return error
    }
    
    // 3. 验证资源版本快照
    if err := c.executor.ValidateResourceSnapshot(&task); err != nil {
        return conflict error
    }
    
    // 4. 更新任务并异步执行Apply
    task.ApplyDescription = req.ApplyDescription
    task.Status = models.TaskStatusApplyPending
    
    go func() {
        if err := c.executor.ExecuteApplyPhase(execCtx, &task); err != nil {
            // 处理错误
        }
    }()
}
```

### 8. 实现ExecuteApplyPhase方法 ⏳
**文件**: `backend/services/terraform_executor.go`

需要添加：
```go
func (s *TerraformExecutor) ExecuteApplyPhase(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 1. 更新状态
    task.Status = models.TaskStatusRunning
    task.Stage = "applying"
    s.db.Save(task)
    
    // 2. 执行Apply（使用现有的ExecuteApply逻辑）
    if err := s.ExecuteApply(ctx, task); err != nil {
        return err
    }
    
    // 3. Apply完成
    task.Status = models.TaskStatusSuccess
    task.Stage = "completed"
    s.db.Save(task)
    
    return nil
}
```

### 9. 添加API路由 ⏳
**文件**: `backend/internal/router/router.go`

需要添加：
```go
// 添加ConfirmApply路由
workspaceGroup.POST("/:id/tasks/:task_id/confirm-apply", 
    taskController.ConfirmApply)
```

### 10. 前端UI修改 ⏳

#### NewRunDialog.tsx
添加Run Type选择：
```tsx
<Form.Item
  name="run_type"
  label="Run Type"
  initialValue="plan_and_apply"
>
  <Radio.Group>
    <Radio value="plan">Plan Only</Radio>
    <Radio value="plan_and_apply">Plan and Apply</Radio>
  </Radio.Group>
</Form.Item>
```

#### TaskDetail.tsx
1. 添加Confirm Apply按钮：
```tsx
{task.status === 'plan_completed' && task.task_type === 'plan_and_apply' && (
  <Button
    type="primary"
    icon={<CheckOutlined />}
    onClick={() => setShowConfirmApplyDialog(true)}
  >
    Confirm Apply
  </Button>
)}
```

2. 添加Confirm Apply对话框：
```tsx
<Modal
  title="Confirm Apply"
  open={showConfirmApplyDialog}
  onOk={handleConfirmApply}
  onCancel={() => setShowConfirmApplyDialog(false)}
>
  <Form form={form}>
    <Form.Item
      name="apply_description"
      label="Apply Description"
      rules={[{ required: true }]}
    >
      <Input.TextArea rows={4} />
    </Form.Item>
  </Form>
</Modal>
```

3. 实现handleConfirmApply：
```tsx
const handleConfirmApply = async () => {
  const values = await form.validateFields();
  
  try {
    await api.post(
      `/workspaces/${workspaceId}/tasks/${taskId}/confirm-apply`,
      { apply_description: values.apply_description }
    );
    
    message.success('Apply started');
    setShowConfirmApplyDialog(false);
    fetchTaskDetail();
  } catch (error) {
    message.error('Failed to start apply');
  }
};
```

### 11. 测试 ⏳
- [ ] 单元测试
- [ ] 集成测试
- [ ] UI测试
- [ ] 端到端测试

## 下一步行动

### 立即执行（按顺序）：

1. **修改ExecutePlan方法**
   - 在Plan完成后创建资源快照
   - 根据TaskType决定是否等待Apply确认

2. **修改CreatePlanTask Controller**
   - 支持run_type参数
   - 创建plan_and_apply类型任务

3. **实现ConfirmApply Controller**
   - 验证任务状态
   - 验证资源版本
   - 触发Apply执行

4. **添加API路由**
   - 注册ConfirmApply端点

5. **前端UI修改**
   - NewRunDialog添加Run Type选择
   - TaskDetail添加Confirm Apply按钮和对话框

6. **测试验证**
   - 创建plan_and_apply任务
   - 验证Plan完成后状态
   - 测试Confirm Apply流程
   - 验证资源版本检测

## 关键注意事项

1. **向后兼容性**
   - 保留现有的plan和apply任务类型
   - 现有任务不受影响

2. **错误处理**
   - 资源版本冲突时的处理
   - Plan失败时的状态回滚
   - Apply失败时的状态管理

3. **日志记录**
   - 记录快照创建和验证过程
   - 记录状态转换

4. **性能考虑**
   - 快照创建的性能影响
   - 大量资源时的处理

## 预期效果

完成后，用户将能够：
1. 创建Plan+Apply组合任务
2. Plan完成后查看结果
3. 确认后执行Apply
4. 系统自动检测资源版本变化
5. 如果资源变化，拒绝Apply并提示用户

## 时间估算

- 后端实现：2-3小时
- 前端实现：1-2小时
- 测试验证：1-2小时
- 总计：4-7小时
