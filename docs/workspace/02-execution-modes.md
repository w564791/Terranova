# Workspace模块 - 执行模式详解

参考文档 [02-agent-k8s-implementation.md](./02-agent-k8s-implementation.md)

> **文档版本**: v1.0  
> **创建日期**: 2025-10-09  
> **状态**: 完整设计

## 📘 概述

Workspace模块支持三种执行模式：Local、Agent和K8s。每种模式适用于不同的场景，提供灵活的执行策略。

## 🎯 三种执行模式

### 1. Local模式

**适用场景**:
- 开发和测试环境
- 小规模部署
- 快速验证

**特点**:
- 在平台服务器本地执行
- 无需额外配置
- 执行速度快
- 资源受限于服务器

**实现状态**:  已完成

**核心组件**:
- `LocalExecutorService`: 本地执行服务
- `TerraformExecutor`: Terraform命令执行器
- `TaskWorker`: 任务工作器

### 2. Agent模式

**适用场景**:
- 生产环境
- 大规模部署
- 需要隔离执行环境
- AWS ASG等动态扩缩容场景

**特点**:
- 分布式执行
- 支持多Agent负载均衡
- 一个Token可注册多个Agent
- 任务锁机制防止冲突
- 数据持久化

**实现状态**:  数据库+模型+服务层完成，控制器待实现

**核心组件**:
- `AgentService`: Agent管理服务
- `AgentPoolService`: Agent池管理服务
- `TaskLockService`: 任务锁服务
- `AgentExecutorService`: Agent执行器（待实现）

**Agent选择策略**:
1. **Round Robin**: 轮询选择
2. **Least Busy**: 选择任务最少的Agent
3. **Random**: 随机选择
4. **Label Match**: 根据标签匹配度选择

### 3. K8s模式

**适用场景**:
- 云原生环境
- 需要资源隔离
- 需要弹性扩缩容
- 多租户场景

**特点**:
- 容器化执行
- 资源限制和配额
- 自动清理
- ServiceAccount权限控制

**实现状态**:  数据库+模型完成，服务层和控制器待实现

**核心组件**:
- `K8sConfigService`: K8s配置服务（待实现）
- `K8sExecutorService`: K8s执行器（待实现）

## 🏗️ 执行器接口设计

### 统一执行器接口

```go
type Executor interface {
    // ExecutePlan 执行Plan任务
    ExecutePlan(ctx context.Context, task *models.WorkspaceTask) error
    
    // ExecuteApply 执行Apply任务
    ExecuteApply(ctx context.Context, task *models.WorkspaceTask) error
    
    // GetStatus 获取任务状态
    GetStatus(ctx context.Context, taskID uint) (*TaskStatus, error)
    
    // Cancel 取消任务
    Cancel(ctx context.Context, taskID uint) error
}
```

### Local执行器

```go
type LocalExecutor struct {
    terraformExecutor *TerraformExecutor
    workspaceService  *WorkspaceService
}

func (e *LocalExecutor) ExecutePlan(ctx context.Context, task *models.WorkspaceTask) error {
    // 1. 准备工作目录
    // 2. 写入Terraform配置
    // 3. 执行terraform init
    // 4. 执行terraform plan
    // 5. 保存输出
    // 6. 更新任务状态
}
```

### Agent执行器

```go
type AgentExecutor struct {
    agentPoolService *AgentPoolService
    taskLockService  *TaskLockService
    agentService     *AgentService
}

func (e *AgentExecutor) ExecutePlan(ctx context.Context, task *models.WorkspaceTask) error {
    // 1. 从Agent Pool选择Agent
    // 2. 获取任务锁
    // 3. 发送任务到Agent
    // 4. 监控执行状态
    // 5. 更新任务状态
    // 6. 释放锁
}
```

### K8s执行器

```go
type K8sExecutor struct {
    k8sConfigService *K8sConfigService
    k8sClient        *kubernetes.Clientset
}

func (e *K8sExecutor) ExecutePlan(ctx context.Context, task *models.WorkspaceTask) error {
    // 1. 获取K8s配置
    // 2. 创建ConfigMap（Terraform配置）
    // 3. 创建Pod
    // 4. 监控Pod状态
    // 5. 获取日志
    // 6. 清理资源
    // 7. 更新任务状态
}
```

## 📊 执行模式对比

| 特性 | Local | Agent | K8s |
|------|-------|-------|-----|
| 执行位置 | 平台服务器 | 远程Agent | K8s集群 |
| 资源隔离 | ❌ |  |  |
| 弹性扩展 | ❌ |  |  |
| 配置复杂度 | 低 | 中 | 高 |
| 执行速度 | 快 | 中 | 中 |
| 成本 | 低 | 中 | 高 |
| 适用场景 | 开发/测试 | 生产 | 云原生 |

## 🔄 执行流程

### Local模式执行流程

```
1. 创建任务 → 2. 准备工作目录 → 3. 写入配置
    ↓
4. terraform init → 5. terraform plan/apply → 6. 保存输出
    ↓
7. 更新状态 → 8. 清理资源 → 9. 完成
```

### Agent模式执行流程

```
1. 创建任务 → 2. 选择Agent → 3. 获取任务锁
    ↓
4. 发送任务到Agent → 5. Agent执行 → 6. 监控状态
    ↓
7. 获取结果 → 8. 释放锁 → 9. 更新状态 → 10. 完成
```

### K8s模式执行流程

```
1. 创建任务 → 2. 获取K8s配置 → 3. 创建ConfigMap
    ↓
4. 创建Pod → 5. 监控Pod → 6. 获取日志
    ↓
7. 获取结果 → 8. 清理资源 → 9. 更新状态 → 10. 完成
```

## 🔧 配置示例

### Local模式配置

```json
{
  "execution_mode": "local",
  "terraform_version": "1.6.0",
  "workdir": "/workspace"
}
```

### Agent模式配置

```json
{
  "execution_mode": "agent",
  "agent_pool_id": 1,
  "selection_strategy": "least_busy",
  "required_labels": ["production", "aws"]
}
```

### K8s模式配置

```json
{
  "execution_mode": "k8s",
  "k8s_config_id": 1,
  "namespace": "iac-platform",
  "pod_template": {
    "image": "hashicorp/terraform:1.6.0",
    "resources": {
      "requests": {"cpu": "500m", "memory": "512Mi"},
      "limits": {"cpu": "1000m", "memory": "1Gi"}
    }
  }
}
```

## 📝 最佳实践

### Local模式
1. 仅用于开发和测试
2. 定期清理工作目录
3. 监控服务器资源使用

### Agent模式
1. 使用Agent Pool进行负载均衡
2. 配置合适的任务锁超时时间
3. 定期检查Agent健康状态
4. 使用标签进行精细化调度

### K8s模式
1. 配置资源限制
2. 使用ServiceAccount控制权限
3. 配置镜像拉取密钥
4. 定期清理失败的Pod

## 🚀 未来扩展

1. **混合模式**: 支持多种执行模式混合使用
2. **智能调度**: 基于AI的执行模式选择
3. **成本优化**: 根据成本自动选择执行模式
4. **故障转移**: 执行失败时自动切换执行模式

---

**相关文档**:
- [00-overview.md](./00-overview.md) - 总览和架构
- [02-agent-k8s-design.md](./02-agent-k8s-design.md) - Agent/K8s详细设计
- [04-task-workflow.md](./04-task-workflow.md) - 任务工作流
