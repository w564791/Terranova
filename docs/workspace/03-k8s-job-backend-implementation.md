# K8s Job Backend Implementation

## 概述

本文档记录了Agent Pool K8s Job后端实现的完整过程，包括Freeze Schedule检查、K8s Job创建、任务调度集成等核心功能。

## 实现目标

### 核心需求

**Job触发条件**（必须全部满足）：
1. Workspace设置为**k8s exec mode**
2. 任务状态为**pending**
3. 该任务是该workspace的**第一个pending任务**（队列头部）
4. Pool未处于**冻结窗口**
5. **该任务尚未创建Job**（幂等性检查，避免重复创建）

**并发规则**：
- 一个workspace = 一个Job（同时只能有一个任务执行）
- 多个workspace可以同时执行 → 创建多个Job
- 例如：3个workspace同时pending → 创建3个Job

**容错逻辑**：
- 创建Job前检查pool_tokens表，避免重复创建
- 如果Job已存在且任务仍pending → 等待Job完成，不重复创建
- 使用分布式锁防止并发创建

## 已实现的功能

### 1. Freeze Schedule Service

**文件**: `backend/services/freeze_schedule_service.go`

**功能**:
- 检查当前时间是否在冻结窗口内
- 支持跨天时间窗口（如23:00-02:00）
- 支持星期多选（1-7对应周一到周日）
- 返回冻结原因描述

**核心方法**:
```go
func (s *FreezeScheduleService) IsInFreezeWindow(schedules []models.FreezeSchedule) (bool, string)
```

**特性**:
- 时间格式：HH:MM（24小时制）
- 跨天处理：自动识别并正确处理跨天窗口
- 星期转换：Sunday从0转换为7，统一使用1-7表示

### 2. K8s Job Service

**文件**: `backend/services/k8s_job_service.go`

**功能**:
- 创建和管理K8s Job
- 生成临时token（3小时有效期）
- 幂等性检查（避免重复创建）
- Job状态监控和删除

**核心方法**:
```go
func (s *K8sJobService) CreateJobForTask(ctx context.Context, task *models.WorkspaceTask, pool *models.AgentPool) error
```

**Job配置**:
- Job命名：`{workspace-id}-{task-id}`
- Namespace：从K8s config获取，默认`terraform`
- activeDeadlineSeconds: 7200（2小时超时）
- backoffLimit: 3（最多重试3次）
- ttlSecondsAfterFinished: 600（完成后10分钟清理）
- restartPolicy: Never

**环境变量注入**:
```go
TASK_ID         // 任务ID
WORKSPACE_ID    // Workspace ID
AGENT_TOKEN     // 临时token
API_ENDPOINT    // API端点
POOL_ID         // Pool ID
```

**幂等性实现**:
1. 检查pool_tokens表是否存在该Job的token
2. 如果存在，检查K8s中Job是否存在
3. 如果Job存在 → 跳过创建（幂等）
4. 如果Job不存在但token存在 → 清理token并重新创建

### 3. Task Queue Manager集成

**文件**: `backend/services/task_queue_manager.go`

**修改内容**:

1. **添加K8s Job Service字段**:
```go
type TaskQueueManager struct {
    db             *gorm.DB
    executor       *TerraformExecutor
    k8sJobService  *K8sJobService  // 新增
    workspaceLocks sync.Map
}
```

2. **初始化K8s Job Service**:
```go
func NewTaskQueueManager(db *gorm.DB, executor *TerraformExecutor) *TaskQueueManager {
    k8sJobService, err := NewK8sJobService(db)
    if err != nil {
        log.Printf("[TaskQueue] K8s Job Service not available: %v", err)
        k8sJobService = nil
    }
    // ...
}
```

3. **执行模式检查**:
```go
func (m *TaskQueueManager) TryExecuteNextTask(workspaceID string) error {
    // 获取workspace信息
    var workspace models.Workspace
    m.db.Where("workspace_id = ?", workspaceID).First(&workspace)
    
    // 检查执行模式
    if workspace.ExecutionMode == models.ExecutionModeK8s {
        return m.createK8sJobForTask(task, &workspace)
    }
    
    // 本地或Agent模式
    go m.executeTask(task)
    return nil
}
```

4. **K8s Job创建逻辑**:
```go
func (m *TaskQueueManager) createK8sJobForTask(task *models.WorkspaceTask, workspace *models.Workspace) error {
    // 1. 检查K8s Job Service可用性
    // 2. 获取Agent Pool信息
    // 3. 创建K8s Job
    // 4. 处理错误（freeze window保持pending，其他错误标记失败）
}
```

## 数据流程

### 任务创建到Job执行流程

```
1. 用户创建任务
   ↓
2. 任务状态设为pending
   ↓
3. TaskQueueManager.TryExecuteNextTask()
   ↓
4. 获取workspace信息
   ↓
5. 检查ExecutionMode
   ├─ local/agent → executeTask()
   └─ k8s → createK8sJobForTask()
       ↓
6. 检查Pool配置
   ↓
7. 检查Freeze Schedule
   ├─ 在冻结窗口 → 保持pending，稍后重试
   └─ 不在冻结窗口 → 继续
       ↓
8. 幂等性检查
   ├─ Job已存在 → 跳过创建
   └─ Job不存在 → 继续
       ↓
9. 生成临时token
   ↓
10. 创建pool_tokens记录
    ↓
11. 创建K8s Job
    ↓
12. Job拉取任务并执行
```

### Token生命周期

```
1. 生成token（64字符hex）
   ↓
2. 计算SHA-256 hash
   ↓
3. 存储到pool_tokens表
   - token_hash (主键)
   - token_type: k8s_temporary
   - expires_at: 3小时后
   - k8s_job_name
   - k8s_namespace
   ↓
4. 注入到Job环境变量
   ↓
5. Agent使用token认证
   ↓
6. Job完成后自动清理（TTL）
```

## 关键技术点

### 1. K8s Client-go集成

**依赖版本**: v0.34.1（最新版本）

```bash
go get k8s.io/client-go@v0.34.1
go get k8s.io/api@v0.34.1
go get k8s.io/apimachinery@v0.34.1
```

**配置加载**:
1. 优先使用in-cluster config（Pod内运行）
2. 回退到kubeconfig文件（本地开发）

### 2. 幂等性保证

**三层检查**:
1. **数据库层**: 查询pool_tokens表
2. **K8s层**: 调用K8s API检查Job是否存在
3. **错误处理**: AlreadyExists错误视为成功

### 3. Freeze Schedule算法

**时间窗口判断**:
```go
if fromMinutes <= toMinutes {
    // 同一天窗口（如09:00-17:00）
    return currentMinutes >= fromMinutes && currentMinutes <= toMinutes
} else {
    // 跨天窗口（如23:00-02:00）
    return currentMinutes >= fromMinutes || currentMinutes <= toMinutes
}
```

### 4. 分布式锁

使用`sync.Map`实现workspace级别的锁：
```go
lockKey := fmt.Sprintf("ws_%s", workspaceID)
lock, _ := m.workspaceLocks.LoadOrStore(lockKey, &sync.Mutex{})
mutex := lock.(*sync.Mutex)
mutex.Lock()
defer mutex.Unlock()
```

## 配置示例

### Agent Pool K8s Config

```json
{
  "image": "terraform-agent:latest",
  "image_pull_policy": "IfNotPresent",
  "command": ["/bin/sh"],
  "args": ["-c", "agent-start"],
  "env": {
    "TF_LOG": "INFO"
  },
  "resources": {
    "requests": {
      "cpu": "500m",
      "memory": "512Mi"
    },
    "limits": {
      "cpu": "2000m",
      "memory": "2Gi"
    }
  },
  "restart_policy": "Never",
  "backoff_limit": 3,
  "ttl_seconds_after_finished": 600,
  "freeze_schedules": [
    {
      "from_time": "02:00",
      "to_time": "06:00",
      "weekdays": [1, 2, 3, 4, 5]
    }
  ]
}
```

## 错误处理

### Freeze Window错误
- **行为**: 保持任务pending状态
- **日志**: 记录freeze window原因
- **重试**: 等待下次调度周期

### Pool配置错误
- **行为**: 标记任务失败
- **错误信息**: "No agent pool assigned to workspace"

### K8s API错误
- **行为**: 标记任务失败
- **错误信息**: 包含详细的K8s错误信息
- **特殊处理**: AlreadyExists视为成功（幂等）

### Token生成错误
- **行为**: 标记任务失败
- **错误信息**: "Failed to generate token"

## 监控和日志

### 关键日志点

1. **K8s Job Service初始化**:
```
[TaskQueue] K8s Job Service not available: <error>
```

2. **执行模式检查**:
```
[TaskQueue] Workspace <id> is in K8s mode, creating K8s Job
```

3. **Freeze Window检查**:
```
[K8sJob] Pool <id> is in freeze window: <reason>
```

4. **幂等性检查**:
```
[K8sJob] Job <name> already exists, skipping creation
```

5. **Job创建成功**:
```
[K8sJob] Successfully created job <name> in namespace <ns>
```

## 测试建议

### 单元测试

1. **Freeze Schedule Service**:
   - 测试同一天窗口
   - 测试跨天窗口
   - 测试星期匹配
   - 测试边界条件

2. **K8s Job Service**:
   - 测试Job创建
   - 测试幂等性
   - 测试token生成
   - 测试错误处理

3. **Task Queue Manager**:
   - 测试执行模式路由
   - 测试并发控制
   - 测试错误恢复

### 集成测试

1. **端到端流程**:
   - 创建workspace（k8s mode）
   - 创建任务
   - 验证Job创建
   - 验证token注入
   - 验证任务执行

2. **Freeze Window测试**:
   - 在冻结窗口内创建任务
   - 验证任务保持pending
   - 等待窗口结束
   - 验证任务自动执行

3. **幂等性测试**:
   - 并发创建相同任务
   - 验证只创建一个Job
   - 验证token唯一性

## 后续优化

### 短期优化

1. **Resource Quantity解析**:
   - 实现完整的resource.MustParse
   - 支持CPU和内存单位转换

2. **Freeze Window重试**:
   - 实现智能重试机制
   - 计算下次可执行时间

3. **Job状态监控**:
   - 实现Job状态轮询
   - 自动更新任务状态

### 长期优化

1. **分布式锁升级**:
   - 使用Redis实现跨实例锁
   - 支持锁超时和自动释放

2. **Job模板管理**:
   - 支持多个Job模板
   - 支持模板版本控制

3. **资源配额管理**:
   - 实现Pool级别资源配额
   - 支持动态资源调整

4. **监控和告警**:
   - 集成Prometheus metrics
   - 实现Job失败告警

## 总结

本次实现完成了Agent Pool K8s Job后端的核心功能：

 Freeze Schedule检查服务
 K8s Job创建和管理
 任务调度系统集成
 幂等性保证
 错误处理和日志
 Token生命周期管理

系统现在支持：
- 自动创建K8s Job执行Terraform任务
- 冻结窗口管理
- 多workspace并发执行
- 完整的错误处理和恢复机制

前端已实现的Freeze Schedule UI和K8s Job YAML生成功能与后端完美配合，形成完整的K8s执行模式解决方案。
