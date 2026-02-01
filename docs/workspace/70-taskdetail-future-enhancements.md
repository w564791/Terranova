# TaskDetail页面未来增强计划

> **创建日期**: 2025-10-11  
> **状态**: 规划中  
> **优先级**: P1

## 📋 当前设计

### 两层日志展示架构

#### 1. 红色错误卡片区域（上层）
**当前功能**：
- 显示task.error_message字段
- 红色主题，醒目
- 用于快速查看错误摘要

**未来优化方向**：
- 改造为**Structured Run Output**（结构化运行输出）
- 参考TFE的设计，显示每个阶段的卡片
- 每个阶段可折叠/展开
- 显示阶段状态图标（✓/✗/⟳/○）
- 显示阶段执行时间
- 显示资源变更统计

#### 2. 黑色日志查看器区域（下层）
**当前功能**：
- 显示完整的执行日志
- 终端风格（黑色背景）
- 包含所有阶段的原始输出
- 支持实时流式传输
- 支持阶段标记

**保持不变**：
- 继续作为详细日志输出
- 提供完整的执行过程
- 方便调试和问题排查

## 🎯 Structured Run Output设计

### 参考TFE的阶段卡片设计

```
┌─────────────────────────────────────────────────────────┐
│ ✓ Plan finished        4 months ago                [▼] │
│   Resources: 0 to add, 0 to change, 13 to destroy      │
│                                                         │
│   [展开后显示Plan的关键信息]                            │
│   - 资源变更列表                                        │
│   - 执行时间                                            │
│   - 状态信息                                            │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ ✓ Sentinel policies passed    4 months ago        [▼] │
│   0 failed                                              │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ ✓ Apply finished       4 months ago                [▼] │
│   Resources: 0 added, 0 changed, 13 destroyed           │
└─────────────────────────────────────────────────────────┘
```

### 实现方案

#### 1. 数据结构

```typescript
interface StageCard {
  name: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  startTime?: string;
  endTime?: string;
  duration?: number;
  summary: string;
  details?: {
    resourcesAdd?: number;
    resourcesChange?: number;
    resourcesDestroy?: number;
    policiesPassed?: number;
    policiesFailed?: number;
    [key: string]: any;
  };
  expanded: boolean;
}
```

#### 2. 组件设计

```typescript
// components/StructuredRunOutput.tsx
const StructuredRunOutput: React.FC<{ task: Task }> = ({ task }) => {
  const [stages, setStages] = useState<StageCard[]>([]);

  useEffect(() => {
    // 从task数据解析阶段信息
    const parsedStages = parseTaskStages(task);
    setStages(parsedStages);
  }, [task]);

  return (
    <div className={styles.structuredOutput}>
      {stages.map((stage, index) => (
        <StageCard 
          key={index}
          stage={stage}
          onToggle={() => toggleStage(index)}
        />
      ))}
    </div>
  );
};

const StageCard: React.FC<{ stage: StageCard; onToggle: () => void }> = ({ 
  stage, 
  onToggle 
}) => {
  return (
    <div className={`${styles.stageCard} ${styles[`status-${stage.status}`]}`}>
      <div className={styles.stageHeader} onClick={onToggle}>
        <div className={styles.stageLeft}>
          <span className={styles.stageIcon}>
            {stage.status === 'completed' && '✓'}
            {stage.status === 'running' && '⟳'}
            {stage.status === 'failed' && '✗'}
            {stage.status === 'pending' && '○'}
          </span>
          <span className={styles.stageName}>{stage.name}</span>
          <span className={styles.stageTime}>
            {formatRelativeTime(stage.endTime || stage.startTime)}
          </span>
        </div>
        <div className={styles.stageRight}>
          <span className={styles.stageSummary}>{stage.summary}</span>
          <span className={styles.expandIcon}>
            {stage.expanded ? '▼' : '▶'}
          </span>
        </div>
      </div>
      
      {stage.expanded && (
        <div className={styles.stageDetails}>
          {/* 显示详细信息 */}
          {stage.details && (
            <div className={styles.detailsContent}>
              {/* 资源变更 */}
              {stage.details.resourcesAdd !== undefined && (
                <div className={styles.detailItem}>
                  <span>Resources:</span>
                  <span className={styles.changeAdd}>
                    +{stage.details.resourcesAdd}
                  </span>
                  <span className={styles.changeModify}>
                    ~{stage.details.resourcesChange}
                  </span>
                  <span className={styles.changeDestroy}>
                    -{stage.details.resourcesDestroy}
                  </span>
                </div>
              )}
              
              {/* 策略检查 */}
              {stage.details.policiesPassed !== undefined && (
                <div className={styles.detailItem}>
                  <span>Policies:</span>
                  <span className={styles.policyPassed}>
                    ✓ {stage.details.policiesPassed} passed
                  </span>
                  {stage.details.policiesFailed > 0 && (
                    <span className={styles.policyFailed}>
                      ✗ {stage.details.policiesFailed} failed
                    </span>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
};
```

#### 3. 样式设计

```css
.structuredOutput {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 0 20px 16px 20px;
}

.stageCard {
  background: var(--color-white);
  border: 1px solid var(--color-gray-200);
  border-radius: var(--radius-md);
  overflow: hidden;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
}

.stageCard.status-completed {
  border-left: 4px solid var(--color-green-500);
}

.stageCard.status-failed {
  border-left: 4px solid var(--color-red-500);
}

.stageCard.status-running {
  border-left: 4px solid var(--color-blue-500);
}

.stageHeader {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px;
  cursor: pointer;
  transition: background 0.2s;
}

.stageHeader:hover {
  background: var(--color-gray-50);
}

.stageLeft {
  display: flex;
  align-items: center;
  gap: 12px;
}

.stageIcon {
  font-size: 18px;
  width: 24px;
  text-align: center;
}

.status-completed .stageIcon {
  color: var(--color-green-600);
}

.status-failed .stageIcon {
  color: var(--color-red-600);
}

.status-running .stageIcon {
  color: var(--color-blue-600);
}

.stageName {
  font-size: 15px;
  font-weight: 600;
  color: var(--color-gray-900);
}

.stageTime {
  font-size: 13px;
  color: var(--color-gray-500);
}

.stageRight {
  display: flex;
  align-items: center;
  gap: 16px;
}

.stageSummary {
  font-size: 13px;
  color: var(--color-gray-600);
}

.expandIcon {
  font-size: 12px;
  color: var(--color-gray-500);
}

.stageDetails {
  padding: 16px;
  background: var(--color-gray-50);
  border-top: 1px solid var(--color-gray-200);
}

.detailsContent {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.detailItem {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: var(--color-gray-700);
}
```

## 📊 实施计划

### Phase 1: 数据准备（后端）

#### 1.1 解析Plan JSON获取资源变更
```go
// 在ExecutePlan中添加
func (s *TerraformExecutor) parsePlanChanges(planJSON map[string]interface{}) (int, int, int) {
    add, change, destroy := 0, 0, 0
    
    if resourceChanges, ok := planJSON["resource_changes"].([]interface{}); ok {
        for _, rc := range resourceChanges {
            change := rc.(map[string]interface{})
            actions := change["change"].(map[string]interface{})["actions"].([]interface{})
            
            for _, action := range actions {
                switch action.(string) {
                case "create":
                    add++
                case "update":
                    change++
                case "delete":
                    destroy++
                }
            }
        }
    }
    
    return add, change, destroy
}

// 保存到Task
task.ChangesAdd = add
task.ChangesChange = change
task.ChangesDestroy = destroy
```

#### 1.2 添加数据库字段
```sql
ALTER TABLE workspace_tasks 
ADD COLUMN IF NOT EXISTS changes_add INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS changes_change INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS changes_destroy INTEGER DEFAULT 0;
```

### Phase 2: 前端实现（1-2天）

#### 2.1 创建StructuredRunOutput组件
- 解析task数据生成阶段卡片
- 实现折叠/展开功能
- 显示阶段状态和时间
- 显示资源变更统计

#### 2.2 集成到TaskDetail
- 替换当前的红色错误卡片区域
- 保持黑色日志查看器不变

### Phase 3: 增强功能（可选）

#### 3.1 实时更新
- WebSocket推送阶段状态变化
- 实时更新阶段卡片

#### 3.2 详细信息
- 点击阶段卡片展开
- 显示该阶段的关键信息
- 可以跳转到日志的对应位置

## 🎯 设计理念

### 两层架构的优势

1. **Structured Run Output（红色区域）**
   - 结构化展示
   - 快速了解执行流程
   - 每个阶段的状态一目了然
   - 适合快速浏览

2. **Detailed Logs（黑色区域）**
   - 原始日志输出
   - 完整的执行过程
   - 方便调试和问题排查
   - 适合深入分析

### 用户体验

- 用户首先看到结构化输出，快速了解执行情况
- 如果需要详细信息，可以展开阶段卡片
- 如果需要原始日志，可以查看下方的日志查看器
- 两者互补，提供最佳的用户体验

## 📝 实施优先级

### P0 - 已完成
-  WebSocket实时日志流
-  历史日志查询
-  阶段标记系统
-  基础UI布局

### P1 - 下一步
- ⏳ Structured Run Output组件
- ⏳ 阶段卡片展示
- ⏳ 资源变更统计
- ⏳ 折叠/展开功能

### P2 - 未来增强
- ⏳ 实时阶段状态更新
- ⏳ 策略检查结果展示
- ⏳ 成本估算展示
- ⏳ 评论功能

---

**当前状态**: 基础架构已完成，红色区域预留用于Structured Run Output
**下一步**: 实现StructuredRunOutput组件，替换当前的简单错误卡片
