# Plan Data 保存 Bug 诊断报告

## 问题现状

### 症状
1. 新任务（Task 385+）的 `plan_json` 和 `plan_data` 字段为 NULL
2. `workspace_task_resource_changes` 表没有数据
3. API `/api/v1/workspaces/{workspace_id}/tasks/{task_id}/resource-changes` 返回空

### 对比数据
- **旧任务（Task 363-368，10月29日）**： plan_json 和 plan_data 有数据，resource_changes 有数据
- **新任务（Task 385-390，11月1日）**：❌ plan_json 和 plan_data 为 NULL，resource_changes 无数据

## 根本原因

在 Agent Mode 重构过程中，`SavePlanDataWithLogging` 函数改用了 `s.dataAccessor.UpdateTask(task)`，导致大字段没有被保存。

## 已完成的修复

### 文件：`backend/services/terraform_executor.go`

#### 修复 1：SavePlanDataWithLogging（第 2100 行左右）
```go
// 使用 Updates 显式更新 plan_data 和 plan_json
updates := map[string]interface{}{
    "plan_data": planData,
    "plan_json": planJSON,
}
s.db.Model(&models.WorkspaceTask{}).Where("id = ?", task.ID).Updates(updates)
```

#### 修复 2：ExecutePlan 最后的更新（第 775 行左右）
```go
// 使用 Updates 只更新指定字段，避免覆盖 plan_data/plan_json
updates := map[string]interface{}{
    "status": task.Status,
    "stage": task.Stage,
    "plan_output": task.PlanOutput,
    "completed_at": task.CompletedAt,
    "duration": task.Duration,
    "changes_add": task.ChangesAdd,
    "changes_change": task.ChangesChange,
    "changes_destroy": task.ChangesDestroy,
}
s.db.Model(&models.WorkspaceTask{}).Where("id = ?", task.ID).Updates(updates)
```

## 验证步骤

### 1. 确认代码已修改
```bash
cd backend
grep -n "使用 Updates 显式更新 plan_data" services/terraform_executor.go
# 应该看到第 2100 行左右有这行注释
```

### 2. 重新编译并运行
```bash
cd backend
# 停止当前服务（Ctrl+C）
# 清理旧的二进制文件
rm -f iac-platform
# 重新编译
go build -o iac-platform .
# 运行
./iac-platform
```

### 3. 创建新任务测试
创建一个新的 Plan 任务，然后验证：

```sql
-- 检查 plan_json 和 plan_data
SELECT id, (plan_json IS NOT NULL) as has_plan_json, 
       (plan_data IS NOT NULL) as has_plan_data 
FROM workspace_tasks WHERE id = <new_task_id>;

-- 检查 resource_changes
SELECT COUNT(*) FROM workspace_task_resource_changes WHERE task_id = <new_task_id>;
```

### 4. 查看日志
在日志中查找：
- "✓ Verification passed: plan_data and plan_json saved successfully"
- 或 "✗ Verification failed"

## 如果还是失败

如果按上述步骤操作后还是失败，可能的原因：
1. `go run main.go` 缓存了旧代码 → 使用 `go build` 重新编译
2. GORM 的 `Updates()` 方法有特殊行为 → 需要进一步调试
3. 数据库连接或事务问题 → 检查数据库日志

## 临时解决方案

如果修复还是不工作，可以手动触发解析：
```bash
# 对于已有 plan_json 的旧任务，手动触发解析
psql -d iac_platform -c "
-- 这里可以写 SQL 手动插入 resource_changes 数据
"
