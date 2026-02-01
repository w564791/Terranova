# Slot ID 概念说明

> **文档版本**: v1.0  
> **创建日期**: 2025-11-08  
> **目的**: 解释Slot ID在系统中的作用和意义

## 📋 什么是Slot ID?

### 核心概念

**Slot ID** 是Pod槽位管理机制中的关键标识符，用于标识一个Agent Pod中的具体任务执行槽位。

### Pod槽位架构

```
每个Agent Pod = 1个容器 + 3个虚拟槽位

┌─────────────────────────────────────┐
│  Agent Pod (agent-pool-xxx-123)     │
│                                     │
│  ┌─────────────────────────────┐   │
│  │ Slot 0 (ID=0)               │   │
│  │ - 状态: running             │   │
│  │ - 任务: task-600            │   │
│  │ - 类型: plan_and_apply      │   │
│  └─────────────────────────────┘   │
│                                     │
│  ┌─────────────────────────────┐   │
│  │ Slot 1 (ID=1)               │   │
│  │ - 状态: reserved            │   │
│  │ - 任务: task-601            │   │
│  │ - 类型: plan_and_apply      │   │
│  └─────────────────────────────┘   │
│                                     │
│  ┌─────────────────────────────┐   │
│  │ Slot 2 (ID=2)               │   │
│  │ - 状态: idle                │   │
│  │ - 任务: null                │   │
│  └─────────────────────────────┘   │
└─────────────────────────────────────┘
```

## 🎯 Slot ID的作用

### 1. 任务隔离

**问题**: 一个Pod可以并发执行多个任务
```
Pod-1 同时执行:
- task-600 (plan_and_apply, 正在planning)
- task-601 (plan, 正在planning)
- task-602 (plan, 正在planning)
```

**解决**: 使用Slot ID区分不同任务的执行环境
```
Pod-1:
- Slot 0 → task-600 (独立工作目录)
- Slot 1 → task-601 (独立工作目录)
- Slot 2 → task-602 (独立工作目录)
```

### 2. 工作目录管理

**每个Slot有独立的工作目录**:
```
/tmp/iac-platform/workspaces/ws-xxx/600/  ← Slot 0的工作目录
/tmp/iac-platform/workspaces/ws-xxx/601/  ← Slot 1的工作目录
/tmp/iac-platform/workspaces/ws-xxx/602/  ← Slot 2的工作目录
```

### 3. Plan-Apply工作目录复用

**关键用途**: 判断Apply是否可以复用Plan的工作目录

**场景1: 同一个Slot (可以复用)**
```
Plan阶段:
- Agent: agent-pool-xxx-123
- Slot ID: 0
- 工作目录: /tmp/.../600/
- 执行Init, Plan

Apply阶段:
- Agent: agent-pool-xxx-123 (相同)
- Slot ID: 0 (相同) 
- 工作目录: /tmp/.../600/ (已存在)
- 跳过Init, 直接Apply 
```

**场景2: 不同Slot (不能复用)**
```
Plan阶段:
- Agent: agent-pool-xxx-123
- Slot ID: 0
- 工作目录: /tmp/.../600/

Apply阶段:
- Agent: agent-pool-xxx-123 (相同)
- Slot ID: 1 (不同) ❌
- 原因: Slot 0可能正在执行其他任务
- 必须重新Init ❌
```

### 4. Apply_pending任务保护

**预留机制**:
```
Plan完成后:
- 任务状态: apply_pending
- Slot状态: reserved (预留)
- Slot ID: 0

缩容检查:
- Pod-1: Slot 0=reserved, Slot 1=idle, Slot 2=idle
- 判断: 有reserved槽位，不能删除此Pod 
```

## 🔍 为什么需要Slot ID?

### 问题场景

**没有Slot ID的情况**:
```
Plan阶段:
- Agent: agent-pool-xxx-123
- 工作目录: /tmp/.../600/
- 执行Init, Plan

Apply阶段:
- Agent: agent-pool-xxx-123 (相同)
- 工作目录: /tmp/.../600/ (存在)
- 问题: 不知道这个工作目录是否被其他任务占用 ❌
- 风险: 可能复用了错误的工作目录 ❌
```

**有Slot ID的情况**:
```
Plan阶段:
- Agent: agent-pool-xxx-123
- Slot ID: 0
- 工作目录: /tmp/.../600/

Apply阶段:
- Agent: agent-pool-xxx-123 (相同)
- Slot ID: 0 (相同) 
- 确认: 这个工作目录就是为这个任务准备的 
- 安全: 可以放心复用 
```

## 📊 Slot ID vs Agent ID

| 维度 | Agent ID | Slot ID |
|------|----------|---------|
| 标识对象 | 整个Pod | Pod内的槽位 |
| 唯一性 | 全局唯一 | Pod内唯一(0,1,2) |
| 生命周期 | Pod创建到销毁 | 任务分配到释放 |
| 用途 | 标识Pod | 标识任务执行位置 |
| 数量 | 1个/Pod | 3个/Pod |

## 🎯 在Task 600优化中的作用

### 优化逻辑

```go
// Plan完成后记录
task.WarmupAgentID = "agent-pool-xxx-123"  // 记录Agent ID
task.WarmupSlotID = 0                       // 记录Slot ID

// Apply开始时检查
if task.WarmupAgentID == currentAgentID &&  // 同一个Pod
   task.WarmupSlotID == currentSlotID {     // 同一个Slot 
    // 可以安全复用工作目录
    // 跳过Init，节省54秒 
} else {
    // 不同Pod或不同Slot
    // 必须重新Init ❌
}
```

### 为什么需要同时检查Agent ID和Slot ID?

**只检查Agent ID的问题**:
```
Plan: Agent-123, Slot 0
Apply: Agent-123, Slot 1 (不同Slot)

问题: Slot 0可能正在执行其他任务
风险: 工作目录冲突 ❌
```

**同时检查Agent ID和Slot ID**:
```
Plan: Agent-123, Slot 0
Apply: Agent-123, Slot 0 (相同Slot) 

确认: 这个Slot就是为这个任务预留的
安全: 可以复用工作目录 
```

## 🔄 实际应用场景

### 场景1: 正常流程（同Slot）

```
时间线:
08:10:00 - Plan开始 (Agent-123, Slot 0)
08:12:00 - Plan完成 (记录: Agent-123, Slot 0)
08:12:00 - 状态: apply_pending (Slot 0 = reserved)
08:13:00 - 用户确认Apply
08:13:00 - Apply开始 (Agent-123, Slot 0) 
08:13:00 - 检查: 同Agent + 同Slot 
08:13:00 - 跳过Init，直接Apply 
08:13:30 - Apply完成 (节省54秒)
```

### 场景2: Slot被占用（不同Slot）

```
时间线:
08:10:00 - Plan开始 (Agent-123, Slot 0)
08:12:00 - Plan完成 (记录: Agent-123, Slot 0)
08:12:00 - 状态: apply_pending (Slot 0 = reserved)
08:12:30 - Slot 0被其他任务占用 (系统调度错误)
08:13:00 - 用户确认Apply
08:13:00 - Apply开始 (Agent-123, Slot 1) ❌
08:13:00 - 检查: 同Agent + 不同Slot ❌
08:13:00 - 必须重新Init ❌
08:14:00 - Apply完成 (正常流程)
```

### 场景3: Pod被销毁（不同Agent）

```
时间线:
08:10:00 - Plan开始 (Agent-123, Slot 0)
08:12:00 - Plan完成 (记录: Agent-123, Slot 0)
08:12:00 - 状态: apply_pending
08:12:30 - Pod被销毁 (缩容或故障)
08:12:35 - 新Pod创建 (Agent-456)
08:13:00 - 用户确认Apply
08:13:00 - Apply开始 (Agent-456, Slot 0) ❌
08:13:00 - 检查: 不同Agent ❌
08:13:00 - 必须重新Init ❌
08:14:00 - Apply完成 (正常流程)
```

## 📝 总结

### Slot ID的核心价值

1. **任务隔离**: 区分同一Pod内的不同任务
2. **工作目录管理**: 每个Slot有独立的工作目录
3. **安全复用**: 确保复用的是正确的工作目录
4. **性能优化**: 同Slot时可以跳过Init

### 关键判断逻辑

```
可以跳过Init的条件:
1. Agent ID相同 (同一个Pod)
2. Slot ID相同 (同一个槽位)
3. Plan Hash匹配 (文件完整性)

三个条件必须同时满足 
```

### 为什么Pod Name = Agent ID?

在当前实现中:
- 每个Pod有唯一的Agent ID
- Pod Name就是Agent ID
- 因此不需要单独的warmup_pod_name字段
- 只需要warmup_agent_id + warmup_slot_id即可

---

**相关文档**:
- [terraform-execution-phase2-pod-slot-implementation.md](terraform-execution-phase2-pod-slot-implementation.md) - Pod槽位架构
- [task-600-duplicate-init-analysis.md](task-600-duplicate-init-analysis.md) - 优化分析
