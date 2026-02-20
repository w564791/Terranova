# Task 681 Fix - Deployment & Verification Guide

## 修复完成状态

 **所有修复已完成并部署**

### 已完成项目

1.  数据库审计字段添加完成
2.  GetNextExecutableTask修复完成
3.  pushTaskToAgent防御性检查完成
4.  WorkspaceTask模型更新完成
5.  ConfirmApply审计记录完成

## 验证步骤

### 1. 验证数据库字段

```bash
# 检查字段是否创建成功
docker exec iac-platform-postgres psql -U postgres -d iac_platform -c "\d workspace_tasks" | grep apply_confirmed

# 预期输出:
# apply_confirmed_by | character varying(255)
# apply_confirmed_at | timestamp without time zone
```

### 2. 验证代码修复

```bash
# 检查GetNextExecutableTask是否排除apply_pending
grep -A 5 "检查plan_and_apply pending任务" backend/services/task_queue_manager.go

# 预期看到: status = ? 而不是 status IN (?)

# 检查防御性检查
grep -A 3 "CRITICAL SECURITY CHECK" backend/services/task_queue_manager.go

# 预期看到: Reject apply_pending tasks
```

### 3. 功能测试

#### 测试1: 正常ConfirmApply流程

```bash
# 1. 创建plan_and_apply任务
curl -X POST http://localhost:8080/api/v1/workspaces/{workspace_id}/tasks/plan \
  -H "Authorization: Bearer {token}" \
  -H "Content-Type: application/json" \
  -d '{"run_type": "plan_and_apply", "description": "Test task"}'

# 2. 等待plan完成，任务进入apply_pending状态

# 3. 确认apply
curl -X POST http://localhost:8080/api/v1/workspaces/{workspace_id}/tasks/{task_id}/confirm-apply \
  -H "Authorization: Bearer {token}" \
  -H "Content-Type: application/json" \
  -d '{"apply_description": "Confirmed by user"}'

# 4. 查询任务，验证审计字段
curl http://localhost:8080/api/v1/workspaces/{workspace_id}/tasks/{task_id} \
  -H "Authorization: Bearer {token}"

# 预期: apply_confirmed_by 和 apply_confirmed_at 有值
```

#### 测试2: 服务器重启场景（关键测试）

```bash
# 1. 创建plan_and_apply任务并等待plan完成（apply_pending状态）

# 2. 记录任务ID和状态
TASK_ID=xxx
docker exec iac-platform-postgres psql -U postgres -d iac_platform -c \
  "SELECT id, status, stage, apply_confirmed_by FROM workspace_tasks WHERE id = $TASK_ID;"

# 3. 重启后端服务
docker restart iac-platform-backend  # 或使用你的重启命令

# 4. 等待30秒后再次查询
sleep 30
docker exec iac-platform-postgres psql -U postgres -d iac_platform -c \
  "SELECT id, status, stage, apply_confirmed_by, started_at FROM workspace_tasks WHERE id = $TASK_ID;"

# 预期结果:
# - status 仍然是 apply_pending（不是 running 或 applied）
# - apply_confirmed_by 仍然是 NULL
# - started_at 仍然是 NULL（apply阶段未开始）
```

#### 测试3: 防御性检查验证

```bash
# 监控后端日志
tail -f /path/to/backend.log | grep "SECURITY: Rejecting apply_pending"

# 如果看到这个日志，说明有代码路径试图绕过主要修复
# 这是正常的防御机制，但需要调查是什么触发的
```

### 4. 审计查询

#### 查询所有确认记录

```sql
-- 查看所有通过ConfirmApply确认的任务
SELECT 
    id,
    workspace_id,
    task_type,
    status,
    apply_confirmed_by,
    apply_confirmed_at,
    apply_description,
    started_at,
    completed_at
FROM workspace_tasks
WHERE apply_confirmed_by IS NOT NULL
ORDER BY apply_confirmed_at DESC
LIMIT 20;
```

#### 检测未经确认的Apply执行（安全检查）

```sql
-- 查找可疑的apply执行（修复后应该为空）
SELECT 
    id,
    workspace_id,
    task_type,
    status,
    stage,
    apply_confirmed_by,
    apply_description,
    started_at,
    created_at
FROM workspace_tasks
WHERE task_type = 'plan_and_apply'
  AND status IN ('running', 'applied')
  AND apply_confirmed_by IS NULL
  AND started_at > '2025-11-14 11:00:00'  -- 修复部署时间
ORDER BY started_at DESC;

-- 如果有结果，说明仍有bug或有其他执行路径
```

#### 对比Task 681（bug案例）

```sql
-- 查看Task 681的审计信息（应该为空，因为是bug导致的）
SELECT 
    id,
    workspace_id,
    status,
    apply_confirmed_by,
    apply_confirmed_at,
    apply_description,
    started_at
FROM workspace_tasks
WHERE id = 681;

-- 预期结果:
-- apply_confirmed_by: NULL（证明未经确认）
-- apply_description: NULL（证明未经ConfirmApply）
-- 这是bug的证据
```

## 监控建议

### 1. 实时监控脚本

创建 `scripts/monitor_apply_confirmations.sh`:

```bash
#!/bin/bash
# 监控未经确认的apply执行

while true; do
    echo "=== Checking for unauthorized applies at $(date) ==="
    
    docker exec iac-platform-postgres psql -U postgres -d iac_platform -c "
        SELECT COUNT(*) as unauthorized_count
        FROM workspace_tasks
        WHERE task_type = 'plan_and_apply'
          AND status IN ('running', 'applied')
          AND apply_confirmed_by IS NULL
          AND started_at > NOW() - INTERVAL '1 hour';
    "
    
    sleep 300  # 每5分钟检查一次
done
```

### 2. 日志监控

```bash
# 监控防御性检查触发
grep "SECURITY: Rejecting apply_pending" /path/to/backend.log | tail -20

# 监控ConfirmApply调用
grep "ConfirmApply.*confirmed by user" /path/to/backend.log | tail -20
```

## 回滚计划（如果需要）

### 快速回滚步骤

```bash
# 1. 回滚代码
git revert HEAD~3  # 回滚最近3个commit

# 2. 重启服务
docker restart iac-platform-backend

# 3. 可选：回滚数据库（如果审计字段导致问题）
docker exec iac-platform-postgres psql -U postgres -d iac_platform -c "
    ALTER TABLE workspace_tasks 
    DROP COLUMN IF EXISTS apply_confirmed_by,
    DROP COLUMN IF EXISTS apply_confirmed_at;
"
```

## 性能影响评估

### 预期影响

- **GetNextExecutableTask**: 无影响（查询条件更严格，性能可能略有提升）
- **pushTaskToAgent**: 增加一次状态检查（<1ms）
- **ConfirmApply**: 增加审计字段写入（<1ms）
- **数据库**: 新增2个字段和1个索引（存储增加<1KB/任务）

### 实际测量

```sql
-- 测量查询性能
EXPLAIN ANALYZE
SELECT * FROM workspace_tasks
WHERE workspace_id = 'ws-xxx'
  AND task_type = 'plan_and_apply'
  AND status = 'pending'
ORDER BY created_at ASC
LIMIT 1;
```

## 成功标准

修复成功的标志：

1.  数据库字段创建成功
2.  服务器重启后apply_pending任务不自动执行
3.  ConfirmApply正常工作并记录审计信息
4.  防御性检查日志正常（如果触发）
5.  无未经确认的apply执行记录

## 问题排查

### 如果apply_pending仍然自动执行

1. 检查代码是否正确部署
```bash
grep "status = ?" backend/services/task_queue_manager.go | grep -A 2 "plan_and_apply pending"
```

2. 检查防御性检查是否生效
```bash
grep "SECURITY: Rejecting" /path/to/backend.log
```

3. 检查是否有其他代码路径
```bash
grep -r "TaskStatusApplyPending" backend/ | grep -v ".go~"
```

### 如果审计字段未记录

1. 检查ConfirmApply是否获取到user_id
```bash
grep "ConfirmApply.*confirmed by user" /path/to/backend.log
grep "WARN.*ConfirmApply.*No user_id" /path/to/backend.log
```

2. 检查JWT中间件是否正常
```bash
# 测试API调用是否包含user_id
curl -v http://localhost:8080/api/v1/workspaces/{id}/tasks/{task_id}/confirm-apply \
  -H "Authorization: Bearer {token}"
```

## 联系和支持

如果遇到问题：

1. 查看完整分析文档: `docs/task-681-auto-apply-bug-analysis.md`
2. 查看实施文档: `docs/task-681-auto-apply-bug-fix-implementation.md`
3. 检查后端日志中的 `[TaskQueue]` 和 `[ConfirmApply]` 标签
4. 使用SQL查询检查任务状态和审计记录

## 总结

Task 681 bug修复已全部完成，包括：
- 核心bug修复（GetNextExecutableTask）
- 防御性安全检查（pushTaskToAgent）
- 审计追踪（数据库字段 + ConfirmApply记录）

系统现在具有三层防护，确保apply操作必须经过用户显式确认，并且所有确认操作都有完整的审计记录。
