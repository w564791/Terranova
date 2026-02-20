# Plan+Apply Flow Redesign

## 概述

重新设计Plan+Apply流程，将其从"两个独立任务"改为"一个任务包含两个阶段"。

## 当前问题

1. **两任务模式的问题**：
   - Plan和Apply是两个独立的任务
   - Apply任务通过`plan_task_id`关联Plan任务
   - 用户体验不连贯
   - 状态管理复杂

2. **现有实现**：
   ```go
   // 当前：创建两个任务
   planTask := &WorkspaceTask{TaskType: "plan"}
   applyTask := &WorkspaceTask{TaskType: "apply", PlanTaskID: &planTask.ID}
   ```

## 新设计

### 1. 核心概念

**Plan+Apply是一个任务，包含两个阶段**：
- Plan阶段：执行terraform plan
- Apply阶段：执行terraform apply

### 2. 任务类型（TaskType）

```go
const (
    TaskTypePlan         TaskType = "plan"          // 单独的Plan任务
    TaskTypeApply        TaskType = "apply"         // 单独的Apply任务（向后兼容）
    TaskTypePlanAndApply TaskType = "plan_and_apply" // Plan+Apply组合任务
)
```

### 3. 任务状态（TaskStatus）

```go
const (
    TaskStatusPending      TaskStatus = "pending"        // 待执行
    TaskStatusWaiting      TaskStatus = "waiting"        // 等待前置任务
    TaskStatusRunning      TaskStatus = "running"        // 执行中
    TaskStatusPlanCompleted TaskStatus = "plan_completed" // Plan完成，等待Apply确认
    TaskStatusApplyPending  TaskStatus = "apply_pending"  // 等待用户确认Apply
    TaskStatusSuccess      TaskStatus = "success"        // 成功
    TaskStatusFailed       TaskStatus = "failed"         // 失败
    TaskStatusCancelled    TaskStatus = "cancelled"      // 已取消
)
```

### 4. 执行阶段（Stage）

```go
const (
    StagePending       = "pending"        // 待执行
    StagePlanning      = "planning"       // Plan阶段
    StagePlanCompleted = "plan_completed" // Plan完成
    StageApplyPending  = "apply_pending"  // 等待Apply确认
    StageApplying      = "applying"       // Apply阶段
    StageCompleted     = "completed"      // 完成
)
```

### 5. 数据库字段

```sql
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS snapshot_id VARCHAR(64);
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS apply_description TEXT;
```

**字段说明**：
- `snapshot_id`: 资源版本快照ID（Plan完成时创建）
- `apply_description`: Apply描述（用户确认Apply时输入）

### 6. 工作流程

```
1. 创建Plan+Apply任务
   POST /api/v1/workspaces/:id/tasks/plan
   Body: { "run_type": "plan_and_apply", "description": "..." }
   
   ↓ 创建任务
   task_type = "plan_and_apply"
   status = "pending"
   stage = "pending"

2. 执行Plan阶段
   status = "running"
   stage = "planning"
   
   ↓ Plan完成
   status = "plan_completed"
   stage = "plan_completed"
   - 保存Plan数据到plan_data字段
   - 创建资源版本快照（snapshot_id）
   - 前端显示"Confirm Apply"按钮

3. 用户确认Apply
   POST /api/v1/workspaces/:id/tasks/:task_id/confirm-apply
   Body: { "apply_description": "..." }
   
   ↓ 验证资源版本
   - 对比当前资源版本与snapshot_id
   - 如果变化，警告或拒绝
   
   ↓ 开始Apply
   status = "running"
   stage = "applying"

4. 执行Apply阶段
   - 从plan_data读取Plan数据
   - 执行terraform apply
   
   ↓ Apply完成
   status = "success"
   stage = "completed"
```

### 7. 资源版本快照机制

**目的**：确保Apply使用的资源版本与Plan时一致

**实现**：
```go
// 1. Plan完成时创建快照
type ResourceSnapshot struct {
    ResourceID string
    VersionID  uint
    Checksum   string
}

snapshot := []ResourceSnapshot{}
for _, resource := range resources {
    snapshot = append(snapshot, ResourceSnapshot{
        ResourceID: resource.ResourceID,
        VersionID:  *resource.CurrentVersionID,
        Checksum:   resource.CurrentVersion.Checksum,
    })
}
snapshotID := generateSnapshotID(snapshot)
task.SnapshotID = snapshotID

// 2. Apply前验证
currentSnapshot := getCurrentResourceSnapshot(workspaceID)
if currentSnapshot != task.SnapshotID {
    return error("Resources have changed since plan")
}
```

## 实现步骤

### Step 1: 更新模型定义

**文件**: `backend/internal/models/workspace.go`

```go
// 添加新的TaskType
const (
    TaskTypePlan         TaskType = "plan"
    TaskTypeApply        TaskType = "apply"
    TaskTypePlanAndApply TaskType = "plan_and_apply" // 新增
)

// 添加新的TaskStatus
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

// WorkspaceTask添加字段
type WorkspaceTask struct {
    // ... 现有字段 ...
    
    SnapshotID       string `json:"snapshot_id" gorm:"type:varchar(64)"`
    ApplyDescription string `json:"apply_description" gorm:"type:text"`
}
```

### Step 2: 数据库迁移

**文件**: `scripts/migrate_plan_apply_redesign.sql`

```sql
-- 添加新字段
ALTER TABLE workspace_tasks 
ADD COLUMN IF NOT EXISTS snapshot_id VARCHAR(64),
ADD COLUMN IF NOT EXISTS apply_description TEXT;

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_snapshot_id 
ON workspace_tasks(snapshot_id);

-- 添加注释
COMMENT ON COLUMN workspace_tasks.snapshot_id IS '资源版本快照ID';
COMMENT ON COLUMN workspace_tasks.apply_description IS 'Apply描述';
```

### Step 3: 修改CreatePlanTask

**文件**: `backend/controllers/workspace_task_controller.go`

```go
func (c *WorkspaceTaskController) CreatePlanTask(ctx *gin.Context) {
    // ... 解析请求 ...
    
    var req struct {
        Description string `json:"description"`
        RunType     string `json:"run_type"` // "plan" 或 "plan_and_apply"
    }
    
    // 确定任务类型
    var taskType models.TaskType
    if req.RunType == "plan_and_apply" {
        taskType = models.TaskTypePlanAndApply
    } else {
        taskType = models.TaskTypePlan
    }
    
    // 创建任务
    task := &models.WorkspaceTask{
        WorkspaceID:   uint(workspaceID),
        TaskType:      taskType,
        Status:        models.TaskStatusPending,
        ExecutionMode: workspace.ExecutionMode,
        CreatedBy:     &uid,
        Stage:         "pending",
        Description:   req.Description,
    }
    
    // 不再创建Apply任务
    
    // 异步执行
    go func() {
        if err := c.executor.ExecutePlanAndApply(execCtx, task); err != nil {
            // 处理错误
        }
    }()
}
```

### Step 4: 实现ConfirmApply接口

**文件**: `backend/controllers/workspace_task_controller.go`

```go
// ConfirmApply 确认执行Apply
// POST /api/v1/workspaces/:id/tasks/:task_id/confirm-apply
func (c *WorkspaceTaskController) ConfirmApply(ctx *gin.Context) {
    taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
        return
    }
    
    var req struct {
        ApplyDescription string `json:"apply_description"`
    }
    if err := ctx.ShouldBindJSON(&req); err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // 获取任务
    var task models.WorkspaceTask
    if err := c.db.First(&task, taskID).Error; err != nil {
        ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
        return
    }
    
    // 验证任务状态
    if task.Status != models.TaskStatusPlanCompleted {
        ctx.JSON(http.StatusBadRequest, gin.H{
            "error": "Task is not in plan_completed status",
        })
        return
    }
    
    // 验证资源版本
    if err := c.executor.ValidateResourceSnapshot(&task); err != nil {
        ctx.JSON(http.StatusConflict, gin.H{
            "error": "Resources have changed since plan",
            "details": err.Error(),
        })
        return
    }
    
    // 更新任务
    task.ApplyDescription = req.ApplyDescription
    task.Status = models.TaskStatusApplyPending
    task.Stage = "apply_pending"
    c.db.Save(&task)
    
    // 异步执行Apply
    go func() {
        execCtx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
        defer cancel()
        
        if err := c.executor.ExecuteApplyPhase(execCtx, &task); err != nil {
            // 处理错误
        }
    }()
    
    ctx.JSON(http.StatusOK, gin.H{
        "message": "Apply started",
        "task": task,
    })
}
```

### Step 5: 修改Executor

**文件**: `backend/services/terraform_executor.go`

```go
// ExecutePlanAndApply 执行Plan+Apply任务
func (s *TerraformExecutor) ExecutePlanAndApply(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 执行Plan阶段
    if err := s.ExecutePlanPhase(ctx, task); err != nil {
        return err
    }
    
    // 如果是plan_and_apply类型，等待用户确认
    if task.TaskType == models.TaskTypePlanAndApply {
        // 更新状态为plan_completed
        task.Status = models.TaskStatusPlanCompleted
        task.Stage = "plan_completed"
        s.db.Save(task)
        
        // 不继续执行Apply，等待用户调用ConfirmApply
        return nil
    }
    
    // 如果是单独的plan任务，直接完成
    task.Status = models.TaskStatusSuccess
    task.Stage = "completed"
    s.db.Save(task)
    
    return nil
}

// ExecutePlanPhase 执行Plan阶段
func (s *TerraformExecutor) ExecutePlanPhase(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // ... 现有的ExecutePlan逻辑 ...
    
    // Plan完成后创建资源版本快照
    snapshotID, err := s.CreateResourceSnapshot(task.WorkspaceID)
    if err != nil {
        log.Printf("Warning: Failed to create snapshot: %v", err)
    } else {
        task.SnapshotID = snapshotID
        s.db.Save(task)
    }
    
    return nil
}

// ExecuteApplyPhase 执行Apply阶段
func (s *TerraformExecutor) ExecuteApplyPhase(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // 更新状态
    task.Status = models.TaskStatusRunning
    task.Stage = "applying"
    s.db.Save(task)
    
    // ... 现有的ExecuteApply逻辑 ...
    
    // Apply完成
    task.Status = models.TaskStatusSuccess
    task.Stage = "completed"
    s.db.Save(task)
    
    return nil
}

// CreateResourceSnapshot 创建资源版本快照
func (s *TerraformExecutor) CreateResourceSnapshot(workspaceID uint) (string, error) {
    var resources []models.WorkspaceResource
    if err := s.db.Where("workspace_id = ? AND is_active = true", workspaceID).
        Find(&resources).Error; err != nil {
        return "", err
    }
    
    // 手动加载版本
    for i := range resources {
        if resources[i].CurrentVersionID != nil {
            var version models.ResourceCodeVersion
            if err := s.db.First(&version, *resources[i].CurrentVersionID).Error; err == nil {
                resources[i].CurrentVersion = &version
            }
        }
    }
    
    // 创建快照
    type ResourceSnapshot struct {
        ResourceID string `json:"resource_id"`
        VersionID  uint   `json:"version_id"`
        Checksum   string `json:"checksum"`
    }
    
    snapshot := []ResourceSnapshot{}
    for _, resource := range resources {
        if resource.CurrentVersion != nil {
            snapshot = append(snapshot, ResourceSnapshot{
                ResourceID: resource.ResourceID,
                VersionID:  resource.CurrentVersion.ID,
                Checksum:   resource.CurrentVersion.Checksum,
            })
        }
    }
    
    // 生成快照ID（使用JSON序列化后的hash）
    snapshotJSON, _ := json.Marshal(snapshot)
    hash := sha256.Sum256(snapshotJSON)
    snapshotID := hex.EncodeToString(hash[:])
    
    return snapshotID, nil
}

// ValidateResourceSnapshot 验证资源版本快照
func (s *TerraformExecutor) ValidateResourceSnapshot(task *models.WorkspaceTask) error {
    if task.SnapshotID == "" {
        return fmt.Errorf("no snapshot ID found")
    }
    
    // 获取当前快照
    currentSnapshotID, err := s.CreateResourceSnapshot(task.WorkspaceID)
    if err != nil {
        return fmt.Errorf("failed to create current snapshot: %w", err)
    }
    
    // 对比
    if currentSnapshotID != task.SnapshotID {
        return fmt.Errorf("resources have changed since plan (expected: %s, current: %s)",
            task.SnapshotID[:16], currentSnapshotID[:16])
    }
    
    return nil
}
```

### Step 6: 添加路由

**文件**: `backend/internal/router/router.go`

```go
// 添加ConfirmApply路由
workspaceGroup.POST("/:id/tasks/:task_id/confirm-apply", 
    taskController.ConfirmApply)
```

### Step 7: 前端修改

**文件**: `frontend/src/pages/TaskDetail.tsx`

```tsx
// 添加Confirm Apply按钮
{task.status === 'plan_completed' && task.task_type === 'plan_and_apply' && (
  <Button
    type="primary"
    icon={<CheckOutlined />}
    onClick={() => setShowConfirmApplyDialog(true)}
  >
    Confirm Apply
  </Button>
)}

// 添加Confirm Apply对话框
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
      rules={[{ required: true, message: 'Please input description' }]}
    >
      <Input.TextArea rows={4} />
    </Form.Item>
  </Form>
</Modal>

// 处理Confirm Apply
const handleConfirmApply = async () => {
  const values = await form.validateFields();
  
  try {
    await api.post(
      `/workspaces/${workspaceId}/tasks/${taskId}/confirm-apply`,
      { apply_description: values.apply_description }
    );
    
    message.success('Apply started');
    setShowConfirmApplyDialog(false);
    fetchTaskDetail(); // 刷新任务详情
  } catch (error) {
    message.error('Failed to start apply');
  }
};
```

**文件**: `frontend/src/components/NewRunDialog.tsx`

```tsx
// 添加Run Type选择
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

## 向后兼容性

1. **保留现有TaskType**：
   - `TaskTypePlan`: 单独的Plan任务
   - `TaskTypeApply`: 单独的Apply任务
   - 新增 `TaskTypePlanAndApply`: Plan+Apply组合任务

2. **现有任务不受影响**：
   - 已存在的Plan/Apply任务继续正常工作
   - 新创建的任务可以选择使用新流程

3. **API兼容**：
   - 现有API保持不变
   - 新增ConfirmApply API

## 测试计划

### 1. 单元测试
- [ ] TaskType枚举测试
- [ ] TaskStatus枚举测试
- [ ] 资源快照创建测试
- [ ] 资源快照验证测试

### 2. 集成测试
- [ ] 创建Plan+Apply任务
- [ ] Plan阶段执行
- [ ] Plan完成状态验证
- [ ] ConfirmApply API测试
- [ ] 资源版本变化检测
- [ ] Apply阶段执行
- [ ] 完整流程测试

### 3. UI测试
- [ ] Run Type选择
- [ ] Confirm Apply按钮显示
- [ ] Confirm Apply对话框
- [ ] 状态显示更新

## 部署步骤

1. **数据库迁移**：
   ```bash
   psql -U postgres -d iac_platform -f scripts/migrate_plan_apply_redesign.sql
   ```

2. **后端部署**：
   ```bash
   cd backend
   go build
   ./iac-platform-backend
   ```

3. **前端部署**：
   ```bash
   cd frontend
   npm run build
   ```

## 监控指标

- Plan+Apply任务创建数量
- Plan完成到Apply确认的平均时间
- 资源版本冲突次数
- Apply成功率

## 回滚计划

如果出现问题，可以：
1. 回滚数据库迁移（删除新增字段）
2. 回滚代码到上一版本
3. 现有Plan/Apply任务不受影响
