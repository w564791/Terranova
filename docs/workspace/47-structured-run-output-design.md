# Structured Run Output 功能设计

> **文档版本**: v1.0  
> **创建日期**: 2025-10-15  
> **状态**: 设计完成  
> **前置阅读**: [15-terraform-execution-detail.md](./15-terraform-execution-detail.md)

## 📋 概述

Structured Run Output 是一种新的任务执行结果展示模式，相比传统的 Console UI（日志流模式），它提供了更结构化、更直观的资源变更展示方式。

## 🎯 功能目标

### 核心目标
1. **结构化展示**: 以阶段Tab的方式展示任务执行流程，而非原始日志
2. **资源级可见性**: 用户可以清晰看到每个资源的变更详情
3. **实时状态更新**: Apply阶段实时显示资源的变更状态
4. **用户友好**: 降低理解Terraform输出的门槛

### 对比传统模式

| 特性 | Console UI | Structured Run Output |
|------|-----------|----------------------|
| 展示方式 | 原始日志流 | 结构化阶段Tab |
| 资源可见性 | 需要解析日志 | 直接展示资源列表 |
| 变更详情 | 混在日志中 | 可折叠的资源详情 |
| 实时更新 | 日志追加 | 资源状态更新 |
| 学习曲线 | 需要理解Terraform | 直观易懂 |

## 🏗️ 架构设计

### 1. UI模式配置

#### Workspace Settings 配置项
在 Workspace Settings → General 页面添加 **User Interface** 配置：

```typescript
interface WorkspaceUIConfig {
  ui_mode: 'console' | 'structured';  // UI展示模式
}
```

#### 数据库Schema
```sql
ALTER TABLE workspaces 
ADD COLUMN ui_mode VARCHAR(20) DEFAULT 'console';
```

### 2. 执行阶段展示

#### 阶段Tab设计
Structured模式下，任务详情页显示以下阶段Tab：

```
┌─────────────────────────────────────────────────────────────┐
│  Planning → Post Plan → Plan Complete → Apply Pending →    │
│  Applying → Post Apply → Complete                           │
└─────────────────────────────────────────────────────────────┘
```

#### 阶段状态映射

| 阶段名称 | 对应Stage | 状态标识 |
|---------|----------|---------|
| Planning | planning | 进行中：转圈动画 |
| Post Plan | post_plan | 进行中：转圈动画 |
| Plan Complete | plan_complete | 可展开：查看变更 |
| Apply Pending | pre_apply | 等待中：时钟图标 |
| Applying | applying | 进行中：转圈动画 |
| Post Apply | post_apply | 进行中：转圈动画 |
| Complete | completion | 完成：打勾图标 |

### 3. Plan数据解析和存储

#### 执行流程

```
terraform plan -no-color -out=tfplan (当前已经实现)
    ↓
保存 plan_data 到数据库 (workspace_tasks.plan_data) (当前已经实现)
    ↓
Plan 流程完成 (当前已经实现)
    ↓
【新增步骤】从数据库读取 plan_data
    ↓
恢复为临时文件 tfplan
    ↓
terraform show -json tfplan
    ↓
解析 resource_changes 数组
    ↓
存储到 workspace_task_resource_changes 表
```

#### 数据库Schema

```sql
CREATE TABLE workspace_task_resource_changes (
    id SERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    
    -- 资源标识
    resource_address VARCHAR(500) NOT NULL,  -- 完整地址
    resource_type VARCHAR(100) NOT NULL,     -- 资源类型
    resource_name VARCHAR(200) NOT NULL,     -- 资源名称
    module_address VARCHAR(500),             -- 模块地址
    
    -- 变更信息
    action VARCHAR(20) NOT NULL,             -- create/update/delete/replace
    changes_before JSONB,                    -- before 数据（完整）
    changes_after JSONB,                     -- after 数据（完整）
    
    -- Apply 阶段状态（用于实时更新）
    apply_status VARCHAR(20) DEFAULT 'pending',
    apply_started_at TIMESTAMP,
    apply_completed_at TIMESTAMP,
    apply_error TEXT,
    
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

#### 解析规则

从 `terraform show -json tfplan` 的输出中解析 `resource_changes` 数组：

```go
type ResourceChange struct {
    Address       string                 `json:"address"`
    ModuleAddress string                 `json:"module_address"`
    Type          string                 `json:"type"`
    Name          string                 `json:"name"`
    Change        ResourceChangeDetail   `json:"change"`
}

type ResourceChangeDetail struct {
    Actions []string               `json:"actions"`
    Before  map[string]interface{} `json:"before"`
    After   map[string]interface{} `json:"after"`
}
```

**Actions 判断逻辑**：
- `["no-op"]` → 忽略（无变更）
- `["create"]` → 创建
- `["update"]` → 更新
- `["delete"]` → 删除
- `["delete", "create"]` → 重建（replace）

### 4. Plan Complete Tab 展示

#### 资源列表展示

```
Plan Complete (3 to add, 2 to change, 1 to destroy)
├─ [+] module.ken-test-222222.module.policy[0].aws_iam_policy.this
│   └─ (展开) 显示变更字段
├─ [~] module.ken-test-222222.module.role[0].aws_iam_role.complete
│   └─ (展开) 显示变更字段
│       ├─ + tags.managed-by-ken: "true"
│       ├─ ~ managed_policy_arns: [1 item] → [2 items]
│       └─ ... 13 unchanged elements hidden (可展开)
└─ [-] module.ew-expose-1.aws_lb_listener_rule.this["443-HTTPS-rule-122-g1"]
    └─ (展开) 显示删除的资源详情
```

#### 图标说明
- `[+]` 创建（绿色）
- `[~]` 更新（黄色）
- `[-]` 删除（红色）
- `[±]` 重建（橙色）

#### 变更详情展示规则

1. **只显示有变更的字段**
   - 新增字段：`+ field_name: "value"`
   - 修改字段：`~ field_name: "old" → "new"`
   - 删除字段：`- field_name: "value"`

2. **未变更字段默认隐藏**
   - 显示：`... N unchanged elements hidden`
   - 用户可点击展开查看

3. **嵌套对象处理**
   - 支持多层级展开
   - 每层显示变更摘要

### 5. Applying Tab 实时更新

#### 资源状态展示

```
Applying (2 in progress, 1 completed, 0 failed)
├─ [⟳] module.ken-test-222222.module.policy[0].aws_iam_policy.this
│   └─ Creating... (正在创建)
├─ [✓] module.ken-test-222222.module.role[0].aws_iam_role.complete
│   └─ Created (已创建)
└─ [ ] module.ken-test-222222.module.role[0].aws_iam_instance_profile.this[0]
    └─ Pending (等待中)
```

#### 状态图标（SVG，不用emoji）

```css
/* 进行中 - CSS Spinner */
.status-applying {
  animation: spin 1s linear infinite;
}

/* 完成 - SVG Checkmark */
.status-completed {
  /* ✓ SVG图标 */
}

/* 失败 - SVG Cross */
.status-failed {
  /* ✗ SVG图标 */
}
```

#### 实时更新机制

使用现有的 WebSocket 连接：

```typescript
// 前端订阅资源状态更新
ws.on('resource_status_update', (data) => {
  // data: { task_id, resource_address, status, timestamp }
  updateResourceStatus(data);
});
```

## 🔌 API设计

### 1. 获取资源变更列表

```
GET /api/v1/workspaces/:workspace_id/tasks/:task_id/resource-changes
```

**响应**：
```json
{
  "summary": {
    "add": 2,
    "change": 3,
    "destroy": 1
  },
  "resources": [
    {
      "id": 1,
      "resource_address": "module.ken-test-222222.module.policy[0].aws_iam_policy.this",
      "resource_type": "aws_iam_policy",
      "resource_name": "this",
      "module_address": "module.ken-test-222222.module.policy[0]",
      "action": "update",
      "changes_before": {
        "tags": {
          "managed-by": "ken",
          "managed-by-terraform": "true"
        }
      },
      "changes_after": {
        "tags": {
          "managed-by": "ken",
          "managed-by-ken": "true",
          "managed-by-terraform": "true"
        }
      },
      "apply_status": "pending"
    }
  ]
}
```

### 2. 更新资源Apply状态

```
PATCH /api/v1/workspaces/:workspace_id/tasks/:task_id/resource-changes/:id
```

**请求**：
```json
{
  "apply_status": "applying",
  "apply_started_at": "2025-10-15T10:00:00Z"
}
```

### 3. WebSocket 事件

```typescript
// 资源状态更新事件
{
  "event": "resource_status_update",
  "data": {
    "task_id": 123,
    "resource_id": 456,
    "resource_address": "module.xxx.aws_iam_policy.this",
    "apply_status": "completed",
    "timestamp": "2025-10-15T10:00:00Z"
  }
}
```

## 💻 前端实现

### 1. Settings 页面

#### WorkspaceSettings.tsx 修改

```typescript
// 添加 UI Mode 配置项
<div className={styles.formGroup}>
  <label>User Interface</label>
  <select
    value={workspace.ui_mode || 'console'}
    onChange={(e) => handleFieldChange('ui_mode', e.target.value)}
  >
    <option value="console">Console UI</option>
    <option value="structured">Structured Run Output</option>
  </select>
  <p className={styles.helpText}>
    Console UI: 显示完整的Terraform日志输出<br/>
    Structured Run Output: 结构化展示资源变更
  </p>
</div>
```

### 2. TaskDetail 页面

#### 模式判断

```typescript
const TaskDetail: React.FC = () => {
  const { workspace, task } = useTaskDetail();
  
  // 根据 workspace.ui_mode 选择展示模式
  if (workspace.ui_mode === 'structured') {
    return <StructuredRunOutput task={task} />;
  } else {
    return <ConsoleOutput task={task} />;
  }
};
```

### 3. StructuredRunOutput 组件

```typescript
const StructuredRunOutput: React.FC<Props> = ({ task }) => {
  const [activeStage, setActiveStage] = useState<string>('planning');
  const [resourceChanges, setResourceChanges] = useState<ResourceChange[]>([]);
  
  // 阶段Tab
  const stages = [
    { key: 'planning', label: 'Planning', status: getStageStatus('planning') },
    { key: 'post_plan', label: 'Post Plan', status: getStageStatus('post_plan') },
    { key: 'plan_complete', label: 'Plan Complete', status: getStageStatus('plan_complete') },
    { key: 'apply_pending', label: 'Apply Pending', status: getStageStatus('apply_pending') },
    { key: 'applying', label: 'Applying', status: getStageStatus('applying') },
    { key: 'post_apply', label: 'Post Apply', status: getStageStatus('post_apply') },
    { key: 'complete', label: 'Complete', status: getStageStatus('complete') },
  ];
  
  return (
    <div className={styles.structuredOutput}>
      {/* 阶段Tab */}
      <div className={styles.stageTabs}>
        {stages.map(stage => (
          <StageTab
            key={stage.key}
            stage={stage}
            active={activeStage === stage.key}
            onClick={() => setActiveStage(stage.key)}
          />
        ))}
      </div>
      
      {/* 阶段内容 */}
      <div className={styles.stageContent}>
        {activeStage === 'plan_complete' && (
          <PlanCompleteView
            taskId={task.id}
            resourceChanges={resourceChanges}
          />
        )}
        {activeStage === 'applying' && (
          <ApplyingView
            taskId={task.id}
            resourceChanges={resourceChanges}
          />
        )}
      </div>
    </div>
  );
};
```

### 4. PlanCompleteView 组件

```typescript
const PlanCompleteView: React.FC<Props> = ({ taskId, resourceChanges }) => {
  const [expandedResources, setExpandedResources] = useState<Set<number>>(new Set());
  
  const toggleResource = (id: number) => {
    const newExpanded = new Set(expandedResources);
    if (newExpanded.has(id)) {
      newExpanded.delete(id);
    } else {
      newExpanded.add(id);
    }
    setExpandedResources(newExpanded);
  };
  
  return (
    <div className={styles.planComplete}>
      <div className={styles.summary}>
        Plan: {summary.add} to add, {summary.change} to change, {summary.destroy} to destroy
      </div>
      
      <div className={styles.resourceList}>
        {resourceChanges.map(resource => (
          <ResourceItem
            key={resource.id}
            resource={resource}
            expanded={expandedResources.has(resource.id)}
            onToggle={() => toggleResource(resource.id)}
          />
        ))}
      </div>
    </div>
  );
};
```

### 5. ResourceItem 组件

```typescript
const ResourceItem: React.FC<Props> = ({ resource, expanded, onToggle }) => {
  const getActionIcon = (action: string) => {
    switch (action) {
      case 'create': return <PlusIcon className={styles.iconCreate} />;
      case 'update': return <TildeIcon className={styles.iconUpdate} />;
      case 'delete': return <MinusIcon className={styles.iconDelete} />;
      case 'replace': return <ReplaceIcon className={styles.iconReplace} />;
    }
  };
  
  const changedFields = computeChangedFields(resource.changes_before, resource.changes_after);
  const unchangedCount = computeUnchangedCount(resource.changes_before, resource.changes_after);
  
  return (
    <div className={styles.resourceItem}>
      <div className={styles.resourceHeader} onClick={onToggle}>
        {getActionIcon(resource.action)}
        <span className={styles.resourceAddress}>{resource.resource_address}</span>
        <ChevronIcon className={expanded ? styles.chevronDown : styles.chevronRight} />
      </div>
      
      {expanded && (
        <div className={styles.resourceDetails}>
          {changedFields.map(field => (
            <FieldChange key={field.path} field={field} />
          ))}
          
          {unchangedCount > 0 && (
            <div className={styles.unchangedHint}>
              ... {unchangedCount} unchanged elements hidden
            </div>
          )}
        </div>
      )}
    </div>
  );
};
```

## 🔧 后端实现

### 1. Plan数据解析服务

```go
// services/plan_parser_service.go
type PlanParserService struct {
    db *gorm.DB
}

func (s *PlanParserService) ParseAndStorePlanChanges(taskID uint) error {
    // 1. 获取任务
    var task models.WorkspaceTask
    if err := s.db.First(&task, taskID).Error; err != nil {
        return err
    }
    
    // 2. 从数据库恢复 plan 文件
    planFile, err := s.restorePlanFile(&task)
    if err != nil {
        return err
    }
    defer os.Remove(planFile)
    
    // 3. 执行 terraform show -json
    planJSON, err := s.executeTerraformShowJSON(planFile)
    if err != nil {
        return err
    }
    
    // 4. 解析 resource_changes
    resourceChanges, err := s.parseResourceChanges(planJSON)
    if err != nil {
        return err
    }
    
    // 5. 存储到数据库
    return s.storeResourceChanges(task.WorkspaceID, taskID, resourceChanges)
}

func (s *PlanParserService) parseResourceChanges(planJSON map[string]interface{}) ([]*models.WorkspaceTaskResourceChange, error) {
    resourceChanges := []*models.WorkspaceTaskResourceChange{}
    
    changes, ok := planJSON["resource_changes"].([]interface{})
    if !ok {
        return nil, fmt.Errorf("invalid plan JSON structure")
    }
    
    for _, item := range changes {
        rc := item.(map[string]interface{})
        change := rc["change"].(map[string]interface{})
        actions := change["actions"].([]interface{})
        
        // 忽略 no-op
        if len(actions) == 1 && actions[0].(string) == "no-op" {
            continue
        }
        
        // 判断操作类型
        action := s.determineAction(actions)
        
        resourceChange := &models.WorkspaceTaskResourceChange{
            ResourceAddress: rc["address"].(string),
            ResourceType:    rc["type"].(string),
            ResourceName:    rc["name"].(string),
            ModuleAddress:   getStringOrEmpty(rc, "module_address"),
            Action:          action,
            ChangesBefore:   change["before"],
            ChangesAfter:    change["after"],
            ApplyStatus:     "pending",
        }
        
        resourceChanges = append(resourceChanges, resourceChange)
    }
    
    return resourceChanges, nil
}

func (s *PlanParserService) determineAction(actions []interface{}) string {
    if len(actions) == 1 {
        return actions[0].(string)
    }
    
    // ["delete", "create"] = replace
    if len(actions) == 2 {
        if actions[0].(string) == "delete" && actions[1].(string) == "create" {
            return "replace"
        }
    }
    
    return "unknown"
}
```

### 2. Controller

```go
// controllers/workspace_task_resource_controller.go
func GetTaskResourceChanges(c *gin.Context) {
    workspaceID := c.Param("workspace_id")
    taskID := c.Param("task_id")
    
    var changes []models.WorkspaceTaskResourceChange
    if err := db.Where("workspace_id = ? AND task_id = ?", workspaceID, taskID).
        Find(&changes).Error; err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    // 计算摘要
    summary := computeSummary(changes)
    
    c.JSON(200, gin.H{
        "summary": summary,
        "resources": changes,
    })
}

func computeSummary(changes []models.WorkspaceTaskResourceChange) map[string]int {
    summary := map[string]int{
        "add": 0,
        "change": 0,
        "destroy": 0,
    }
    
    for _, change := range changes {
        switch change.Action {
        case "create":
            summary["add"]++
        case "update":
            summary["change"]++
        case "delete":
            summary["destroy"]++
        case "replace":
            // replace = 1 delete + 1 create
            summary["add"]++
            summary["destroy"]++
        }
    }
    
    return summary
}
```

## 📊 数据流图

```
┌─────────────┐
│   Plan 执行  │
└──────┬──────┘
       │
       ↓
┌─────────────────────┐
│ 保存 plan_data      │
│ (workspace_tasks)   │
└──────┬──────────────┘
       │
       ↓
┌─────────────────────┐
│ Plan 完成后触发     │
│ ParseAndStore       │
└──────┬──────────────┘
       │
       ↓
┌─────────────────────┐
│ 从DB恢复plan文件    │
└──────┬──────────────┘
       │
       ↓
┌─────────────────────┐
│ terraform show -json│
└──────┬──────────────┘
       │
       ↓
┌─────────────────────┐
│ 解析resource_changes│
└──────┬──────────────┘
       │
       ↓
┌─────────────────────────────────┐
│ 存储到                           │
│ workspace_task_resource_changes │
└──────┬──────────────────────────┘
       │
       ↓
┌─────────────────────┐
│ 前端通过API获取     │
│ 展示资源变更        │
└─────────────────────┘
```

## 🧪 测试计划

### 1. 单元测试
- [ ] Plan数据解析逻辑测试
- [ ] Actions判断逻辑测试
- [ ] 数据存储测试

### 2. 集成测试
- [ ] 完整Plan流程测试
- [ ] API接口测试
- [ ] WebSocket实时更新测试

### 3. UI测试
- [ ] Settings配置切换测试
- [ ] 阶段Tab展示测试
- [ ] 资源列表展开/折叠测试
- [ ] 实时状态更新测试

## 📝 实施计划

### Phase 1: 数据层（1-2天）
- [x] 创建数据库迁移脚本
- [ ] 创建Model定义
- [ ] 实现Plan解析服务
- [ ] 实现数据存储逻辑

### Phase 2: API层（1天）
- [ ] 实现资源变更API
- [ ] 实现状态更新API
- [ ] 集成WebSocket事件

### Phase 3: 前端基础（2天）
- [ ] Settings页面UI配置
- [ ] TaskDetail模式切换
- [ ] 阶段Tab组件
- [ ] 基础样式

### Phase 4: 前端高级（2-3天）
- [ ] PlanCompleteView实现
- [ ] ResourceItem组件
- [ ] 变更详情展示
- [ ] ApplyingView实现
- [ ] 实时状态更新

### Phase 5: 测试和优化（1-2天）
- [ ] 功能测试
- [ ] 性能优化
- [ ] UI/UX优化
- [ ] 文档完善

## 🔗 相关文档

- [15-terraform-execution-detail.md](./15-terraform-execution-detail.md) - Terraform执行流程详细设计
- [11-frontend-design.md](./11-frontend-design.md) - 前端设计规范
- [09-api-specification.md](./09-api-specification.md) - API规范

---

**状态**: 设计完成，待实施
