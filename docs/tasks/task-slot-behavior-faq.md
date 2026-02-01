# Agent 槽位行为 FAQ

## 用户问题

### 1. 这个Pod能不能运行2个plan+apply任务？

**答案：不能**

根据当前实现（修改后的代码），一个Pod**最多只能运行1个plan+apply任务**。

**原因**：在 `FindPodWithFreeSlot` 函数中有明确的检查：

```go
// 如果是plan+apply任务，检查Pod上是否已有其他plan+apply任务（running或reserved）
if taskType == string(models.TaskTypePlanAndApply) {
    hasOtherPlanAndApply := false
    for _, slot := range pod.Slots {
        if (slot.Status == "running" || slot.Status == "reserved") && 
           slot.TaskType == string(models.TaskTypePlanAndApply) {
            hasOtherPlanAndApply = true
            break
        }
    }
    
    if hasOtherPlanAndApply {
        pod.mu.RUnlock()
        log.Printf("[PodManager] Pod %s already has a plan+apply task, cannot accept another",
            pod.PodName)
        continue  // 跳过这个Pod，不能分配
    }
}
```

**设计理由**：
- Plan+apply任务通常需要修改基础设施状态
- 为了避免冲突和竞态条件，限制每个Pod只能有1个plan+apply任务
- 这是一个安全性设计

---

### 2. Pod能否混合运行plan任务和plan+apply任务？

**答案：能**

一个Pod可以同时运行：
- **1个plan+apply任务** + **3个plan任务** = 总共4个任务

**示例场景**：

```
Pod-1 (4个槽位):
- Slot 0: running (task-100, plan_and_apply)  ← 1个plan+apply
- Slot 1: running (task-101, plan)            ← plan任务
- Slot 2: running (task-102, plan)            ← plan任务
- Slot 3: running (task-103, plan)            ← plan任务

状态： 合法，可以混合运行
```

**代码逻辑**：
- Plan任务分配时，不检查是否有plan+apply任务
- Plan+apply任务分配时，只检查是否已有其他plan+apply任务
- 因此可以混合运行

---

### 3. 预留槽位是否会占用真实槽位？

**答案：会占用槽位编号，但不占用执行容量**

这是一个容易混淆的概念，让我详细解释：

#### 3.1 预留槽位占用槽位编号

```
Pod-1 (4个槽位):
- Slot 0: reserved (task-100, apply_pending)  ← 占用了Slot 0
- Slot 1: idle
- Slot 2: idle
- Slot 3: idle

可用槽位：Slot 1, 2, 3 (共3个)
```

**代码证据**：
```go
// 查找空闲槽位
for i, slot := range pod.Slots {
    if slot.Status == "idle" {  // 只有idle才能分配
        pod.mu.RUnlock()
        return pod, i, nil
    }
}
```

Reserved槽位的status是"reserved"，不是"idle"，所以会被跳过。

#### 3.2 预留槽位不占用执行容量

"不占用执行容量"的意思是：
- Reserved槽位不会阻止其他槽位被使用
- Reserved槽位不会阻止Pod接受新任务
- Reserved槽位只是标记，表示这个槽位预留给某个apply_pending任务

**示例**：

```
场景1：有reserved槽位
Pod-1:
- Slot 0: reserved (task-100)
- Slot 1: idle
- Slot 2: idle
- Slot 3: idle

新的plan任务到来： 可以分配到Slot 1, 2, 3
新的plan+apply任务到来： 可以分配到Slot 1, 2, 3
（因为reserved不算作running的plan+apply任务）
```

```
场景2：有running的plan+apply任务
Pod-1:
- Slot 0: running (task-101, plan_and_apply)
- Slot 1: idle
- Slot 2: idle
- Slot 3: idle

新的plan任务到来： 可以分配到Slot 1, 2, 3
新的plan+apply任务到来：❌ 不能分配（已有running的plan+apply）
```

#### 3.3 总结

| 槽位状态 | 占用槽位编号 | 阻止新plan任务 | 阻止新plan+apply任务 |
|---------|------------|--------------|-------------------|
| idle    | 否         | 否           | 否                |
| reserved| 是         | 否           | 否                |
| running (plan) | 是 | 否           | 否                |
| running (plan+apply) | 是 | 否    | 是                |

---

## 完整的槽位规则总结

### 规则1：槽位数量
- 每个Pod有4个槽位（Slot 0, 1, 2, 3）

### 规则2：Plan任务
- 可以使用任意空闲（idle）槽位
- 不受其他任务类型影响
- 最多4个并发（如果4个槽位都是idle）

### 规则3：Plan+Apply任务
- 可以使用任意空闲（idle）槽位
- **限制**：每个Pod最多1个running或reserved的plan+apply任务
- 如果Pod上已有plan+apply任务（running或reserved），新的plan+apply任务不能分配到这个Pod

### 规则4：Reserved槽位
- 占用槽位编号（Slot 0/1/2/3中的一个）
- 不阻止其他槽位使用
- 不阻止新任务分配
- 用于apply_pending任务，防止Pod被删除

### 规则5：混合运行
-  1个plan+apply + 3个plan = 4个任务
-  4个plan = 4个任务
- ❌ 2个plan+apply（不允许）

---

## 实际场景示例

### 场景A：最大容量
```
Pod-1:
- Slot 0: running (task-100, plan_and_apply)
- Slot 1: running (task-101, plan)
- Slot 2: running (task-102, plan)
- Slot 3: running (task-103, plan)

状态： 满载，4个任务同时运行
```

### 场景B：有预留槽位
```
Pod-1:
- Slot 0: reserved (task-200, apply_pending)
- Slot 1: running (task-201, plan)
- Slot 2: running (task-202, plan)
- Slot 3: idle

新plan任务： 可以分配到Slot 3
新plan+apply任务： 可以分配到Slot 3（reserved不算running）
```

### 场景C：不能分配第二个plan+apply
```
Pod-1:
- Slot 0: running (task-300, plan_and_apply)
- Slot 1: idle
- Slot 2: idle
- Slot 3: idle

新plan任务： 可以分配到Slot 1, 2, 3
新plan+apply任务：❌ 不能分配（已有running的plan+apply）
```

---

## 如果需要修改规则

如果用户希望允许一个Pod运行多个plan+apply任务，需要：

1. 移除 `FindPodWithFreeSlot` 中的plan+apply检查逻辑
2. 考虑并发安全性和资源竞争问题
3. 评估对基础设施状态管理的影响

**不推荐**这样做，因为可能导致：
- 多个plan+apply任务修改同一资源
- 状态文件冲突
- 不可预测的基础设施变更
