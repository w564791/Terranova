# TaskDetail页面UI优化指南

> **参考**: Terraform Enterprise任务详情页  
> **目标**: 提升UI美观度和用户体验  
> **优先级**: P1

## 🎯 TFE设计分析

### 关键设计元素

#### 1. 任务标题区域
```
update                                    [CURRENT] [✓ Applied]
```
- 任务描述作为主标题
- 状态标签（CURRENT, Applied等）
- 绿色边框表示成功

#### 2. 统计信息卡片
```
Policy checks          Plan & apply duration    Resources changed
✓ 162 of 162 passed   Less than a minute       +0  ~0  -13
```
- 3列网格布局
- 简洁的图标和数字
- 清晰的标签

#### 3. 执行流程时间线
```
[用户头像] Ken Bai triggered a run from Bitbucket 4 months ago    [Run Details ▼]

✓ Plan finished        4 months ago    Resources: 0 to add, 0 to change, 13 to destroy  [▼]
✓ Sentinel policies passed    4 months ago    0 failed  [▼]
✓ Apply finished       4 months ago    Resources: 0 added, 0 changed, 13 destroyed  [▼]
```
- 每个阶段一个卡片
- 绿色勾选图标表示成功
- 可展开查看详情
- 显示资源变更统计

#### 4. 评论区域
```
[用户头像] ken    4 months ago
    Run confirmed

Comment: Leave feedback or record a decision.
[Add Comment]
```

## 📋 优化建议

### Phase 1: 基础优化（1-2小时）

#### 1.1 添加任务标题和状态标签

```typescript
// TaskDetail.tsx
<div className={styles.taskHeader}>
  <div className={styles.task[ERROR] Failed to process response: The system encountered an unexpected error during processing. Try your request again.
