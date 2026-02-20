# Workspace Drift Detection 技术方案

## 实现状态: ✅ 已完成

## 1. 需求概述

### 1.1 功能目标
为 Workspace 添加后台 Terraform Drift 检测功能，通过定时执行 `terraform plan --refresh-only` 来检测云端资源与 Terraform 状态之间的漂移。

### 1.2 核心需求
1. **配置管理**：Workspace 可配置 drift 检测的开始/结束时间和检测频率
2. **智能调度**：当 Agent 不存在或离线时自动跳过任务
3. **后台运行**：不显示在任务列表中，独立的后台任务机制
4. **状态展示**：资源列表显示 drift 状态，使用不同颜色标识
5. **每日限制**：每个 Workspace 每天最多运行 1 次 drift 检测

### 1.3 资源状态定义
| 状态 | 颜色 | 说明 |
|------|------|------|
| Drift | 黄色 | 存在漂移（至少1个子资源有drift） |
| Synced | 绿色 | 无漂移（已apply且无drift），或从未检测过 |
| Unapplied | 灰色 | 未应用（从未执行过apply） |

### 1.4 重要设计决策

1. **资源映射规则**：
   - `workspace_resources.resource_id` 对应 terraform module 名称（如 `aaaaa`）
   - 通过 `workspace_id + resource_id` 唯一定位资源
   - terraform plan 输出的 `module.aaaaa.xxx` 中的 `aaaaa` 对应 `resource_id`

2. **默认配置**：
   - `drift_check_enabled`: **true**（默认启用）
   - `drift_check_start_time`: **07:00:00**（早上7点）
   - `drift_check_end_time`: **22:00:00**（晚上10点）
   - `drift_check_interval`: **1440**（每天1次，即1440分钟）

3. **状态更新规则**：
   - 从未检测过的资源显示为 **Synced**（绿色）
   - 只有**全量 apply**（不带 `--target`）才会清除 drift 状态
   - 带 `--target` 的 apply 不修改 drift 状态

4. **手动触发**：
   - 手动触发不受"每天最多1次"限制
   - 可随时手动触发检测

5. **队列控制**：
   - 全局统一队列，每次只能有 **1 个 workspace** 运行 drift 检测
   - 避免多个 workspace 同时检测导致资源竞争

---

## 2. 数据库设计

### 2.1 workspaces 表新增字段

```sql
-- Drift 检测配置字段（默认启用，每天早7点到晚10点，每天1次）
ALTER TABLE workspaces ADD COLUMN drift_check_enabled BOOLEAN DEFAULT true;
ALTER TABLE workspaces ADD COLUMN drift_check_start_time TIME DEFAULT '07:00:00';
ALTER TABLE workspaces ADD COLUMN drift_check_end_time TIME DEFAULT '22:00:00';
ALTER TABLE workspaces ADD COLUMN drift_check_interval INT DEFAULT 1440; -- 每天1次（1440分钟）
```

**字段说明**：
- `drift_check_enabled`: 是否启用 drift 检测（默认 **true**）
- `drift_check_start_time`: 每天允许检测的开始时间（默认 **07:00:00**）
- `drift_check_end_time`: 每天允许检测的结束时间（默认 **22:00:00**）
- `drift_check_interval`: 检测间隔（分钟），默认 **1440**（每天1次）

### 2.2 workspace_resources 表新增字段

```sql
-- 记录资源最后一次 apply 时间
ALTER TABLE workspace_resources ADD COLUMN last_applied_at TIMESTAMP;
CREATE INDEX idx_workspace_resources_last_applied ON workspace_resources(last_applied_at);
```

**字段说明**：
- `last_applied_at`: 资源最后一次成功 apply 的时间，NULL 表示从未 apply 过

### 2.3 新建 workspace_drift_results 表

```sql
CREATE TABLE workspace_drift_results (
    id SERIAL PRIMARY KEY,
    workspace_id VARCHAR(50) NOT NULL UNIQUE,  -- 每个 workspace 只保留一条记录
    has_drift BOOLEAN DEFAULT false,
    drift_count INT DEFAULT 0,                  -- 有 drift 的资源数量
    total_resources INT DEFAULT 0,              -- 检测的总资源数
    drift_details JSONB,                        -- 详细的 drift 信息
    check_status VARCHAR(20) DEFAULT 'pending', -- pending/running/success/failed/skipped
    error_message TEXT,                         -- 错误信息
    last_check_at TIMESTAMP,                    -- 最后检测时间
    last_check_date DATE,                       -- 最后检测日期（用于每日限制）
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    CONSTRAINT fk_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE
);

CREATE INDEX idx_drift_results_workspace ON workspace_drift_results(workspace_id);
CREATE INDEX idx_drift_results_check_date ON workspace_drift_results(last_check_date);
CREATE INDEX idx_drift_results_has_drift ON workspace_drift_results(has_drift);
```

### 2.4 drift_details JSONB 结构

```json
{
  "check_time": "2026-01-20T10:30:00Z",
  "terraform_version": "1.6.0",
  "plan_output_summary": "Plan: 0 to add, 2 to change, 0 to destroy.",
  "resources": [
    {
      "resource_id": 123,
      "resource_name": "my_s3_bucket",
      "resource_type": "module",
      "has_drift": true,
      "drifted_children": [
        {
          "address": "module.my_s3_bucket.aws_s3_bucket.this",
          "type": "aws_s3_bucket",
          "name": "this",
          "action": "update",
          "changes": {
            "tags": {
              "before": {"env": "dev"},
              "after": {"env": "prod"}
            },
            "versioning": {
              "before": {"enabled": true},
              "after": {"enabled": false}
            }
          }
        },
        {
          "address": "module.my_s3_bucket.aws_s3_bucket_policy.this",
          "type": "aws_s3_bucket_policy",
          "name": "this",
          "action": "update",
          "changes": {
            "policy": {
              "before": "{...old policy...}",
              "after": "{...new policy...}"
            }
          }
        }
      ]
    },
    {
      "resource_id": 124,
      "resource_name": "my_ec2_instance",
      "resource_type": "module",
      "has_drift": false,
      "drifted_children": []
    }
  ]
}
```

---

## 3. 后端架构设计

### 3.0 复用现有执行流程（最小修改方案）

**核心思路**：复用现有的 `TerraformExecutor.ExecutePlan` 和 `TaskQueueManager`，支持 Local/Agent/K8s Agent 三种模式。

**修改点**：
1. **models/workspace.go**：添加 `TaskTypeDriftCheck` 和 `IsBackground` 字段
2. **terraform_executor.go**：在 `ExecutePlan` 中检测 `drift_check` 类型，添加 `--refresh-only` 参数
3. **services/drift_check_scheduler.go**：新建调度器（只负责创建任务）
4. **services/drift_check_service.go**：解析 plan 输出，保存 drift 结果

**不需要修改**：
- Agent 代码（完全复用）
- TaskQueueManager（复用现有队列）
- Agent C&C Handler（复用现有通信）

### 3.1 核心组件

```
┌─────────────────────────────────────────────────────────────────┐
│                        main.go                                   │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              DriftCheckScheduler                         │    │
│  │  - Start(interval time.Duration)                        │    │
│  │  - Stop()                                               │    │
│  │  - checkWorkspaces()                                    │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    DriftCheckService                             │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  - GetWorkspacesNeedingCheck() []Workspace              │    │
│  │  - ShouldRunCheck(workspace) bool                       │    │
│  │  - ExecuteDriftCheck(workspace) error                   │    │
│  │  - ParsePlanOutput(output) DriftDetails                 │    │
│  │  - SaveDriftResult(workspace, result) error             │    │
│  │  - GetDriftResult(workspaceID) *DriftResult             │    │
│  │  - GetResourceDriftStatus(resourceID) string            │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Agent C&C Handler                             │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  - ExecuteDriftCheckTask(workspace) (output, error)     │    │
│  │  - 复用现有的 terraform plan 执行逻辑                    │    │
│  │  - 添加 --refresh-only 参数                             │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2 调度器逻辑（全局队列控制）

```go
// DriftCheckScheduler 定时检查需要执行 drift 检测的 workspace
// 使用全局队列，每次只能有 1 个 workspace 运行 drift 检测
type DriftCheckScheduler struct {
    db              *gorm.DB
    driftService    *DriftCheckService
    agentCCHandler  *RawAgentCCHandler
    ticker          *time.Ticker
    stopChan        chan struct{}
    
    // 全局队列控制
    isRunning       bool           // 是否有任务正在运行
    runningMutex    sync.Mutex     // 保护 isRunning 的互斥锁
    taskQueue       chan string    // workspace_id 队列
}

func NewDriftCheckScheduler(db *gorm.DB, driftService *DriftCheckService, agentCCHandler *RawAgentCCHandler) *DriftCheckScheduler {
    return &DriftCheckScheduler{
        db:             db,
        driftService:   driftService,
        agentCCHandler: agentCCHandler,
        stopChan:       make(chan struct{}),
        taskQueue:      make(chan string, 100), // 缓冲队列，最多100个待检测workspace
    }
}

func (s *DriftCheckScheduler) Start(interval time.Duration) {
    s.ticker = time.NewTicker(interval)
    
    // 启动队列消费者（单线程，保证每次只有1个workspace在检测）
    go s.queueConsumer()
    
    // 启动调度器
    go func() {
        for {
            select {
            case <-s.ticker.C:
                s.checkWorkspaces()
            case <-s.stopChan:
                return
            }
        }
    }()
}

// queueConsumer 队列消费者，串行处理 drift 检测任务
func (s *DriftCheckScheduler) queueConsumer() {
    for {
        select {
        case workspaceID := <-s.taskQueue:
            s.runningMutex.Lock()
            s.isRunning = true
            s.runningMutex.Unlock()
            
            // 执行 drift 检测（同步，阻塞直到完成）
            s.executeDriftCheck(workspaceID)
            
            s.runningMutex.Lock()
            s.isRunning = false
            s.runningMutex.Unlock()
            
        case <-s.stopChan:
            return
        }
    }
}

func (s *DriftCheckScheduler) checkWorkspaces() {
    // 1. 获取所有启用了 drift 检测的 workspace
    workspaces := s.driftService.GetWorkspacesNeedingCheck()
    
    for _, ws := range workspaces {
        // 2. 检查是否在允许的时间窗口内
        if !s.isInTimeWindow(ws) {
            continue
        }
        
        // 3. 检查今天是否已经执行过（自动调度限制）
        if s.hasRunToday(ws) {
            continue
        }
        
        // 4. 检查 Agent 是否可用
        if !s.isAgentAvailable(ws) {
            log.Printf("[DriftCheck] Skipping workspace %s: agent not available", ws.WorkspaceID)
            continue
        }
        
        // 5. 加入队列（非阻塞）
        select {
        case s.taskQueue <- ws.WorkspaceID:
            log.Printf("[DriftCheck] Workspace %s added to queue", ws.WorkspaceID)
        default:
            log.Printf("[DriftCheck] Queue full, skipping workspace %s", ws.WorkspaceID)
        }
    }
}

// TriggerManualCheck 手动触发检测（不受每日限制）
func (s *DriftCheckScheduler) TriggerManualCheck(workspaceID string) error {
    // 检查 workspace 是否存在
    var ws models.Workspace
    if err := s.db.Where("workspace_id = ?", workspaceID).First(&ws).Error; err != nil {
        return fmt.Errorf("workspace not found: %w", err)
    }
    
    // 检查 Agent 是否可用
    if !s.isAgentAvailable(&ws) {
        return fmt.Errorf("agent not available for workspace %s", workspaceID)
    }
    
    // 加入队列
    select {
    case s.taskQueue <- workspaceID:
        log.Printf("[DriftCheck] Manual check for workspace %s added to queue", workspaceID)
        return nil
    default:
        return fmt.Errorf("queue full, please try again later")
    }
}
```

### 3.3 Drift 检测执行流程

```
┌──────────────────────────────────────────────────────────────────┐
│                    Drift Check Execution Flow                     │
└──────────────────────────────────────────────────────────────────┘

1. 调度器触发
   │
   ▼
2. 检查前置条件
   ├── drift_check_enabled = true?
   ├── 当前时间在 start_time ~ end_time 之间?
   ├── 今天是否已执行过? (last_check_date != today)
   └── Agent 是否在线?
   │
   ▼
3. 更新状态为 "running"
   │
   ▼
4. 通过 Agent 执行 terraform plan --refresh-only
   ├── 复用现有的 Agent C&C 通信机制
   ├── 不创建 workspace_tasks 记录
   └── 直接获取 plan 输出
   │
   ▼
5. 解析 plan 输出
   ├── 提取 resource_changes
   ├── 按 module 分组
   └── 生成 drift_details JSON
   │
   ▼
6. 保存结果到 workspace_drift_results
   ├── 更新 has_drift, drift_count
   ├── 更新 drift_details
   ├── 更新 last_check_at, last_check_date
   └── 更新 check_status = "success"
   │
   ▼
7. 更新 workspace 统计字段
   ├── drift_count
   └── last_drift_check
```

### 3.4 API 接口设计

#### 3.4.1 获取 Workspace Drift 配置
```
GET /api/workspaces/:id/drift-config

Response:
{
  "drift_check_enabled": true,
  "drift_check_start_time": "09:00:00",
  "drift_check_end_time": "18:00:00",
  "drift_check_interval": 60
}
```

#### 3.4.2 更新 Workspace Drift 配置
```
PUT /api/workspaces/:id/drift-config

Request:
{
  "drift_check_enabled": true,
  "drift_check_start_time": "09:00:00",
  "drift_check_end_time": "18:00:00",
  "drift_check_interval": 60
}
```

#### 3.4.3 获取 Drift 检测结果
```
GET /api/workspaces/:id/drift-result

Response:
{
  "workspace_id": "ws-xxx",
  "has_drift": true,
  "drift_count": 2,
  "total_resources": 5,
  "check_status": "success",
  "last_check_at": "2026-01-20T10:30:00Z",
  "drift_details": { ... }
}
```

#### 3.4.4 手动触发 Drift 检测
```
POST /api/workspaces/:id/drift-check

Response:
{
  "message": "Drift check started",
  "status": "running"
}
```

#### 3.4.5 获取资源 Drift 状态（批量）
```
GET /api/workspaces/:id/resources/drift-status

Response:
{
  "resources": [
    {
      "resource_id": 123,
      "status": "drift",      // drift | synced | unapplied
      "has_drift": true,
      "last_applied_at": "2026-01-15T10:00:00Z",
      "drifted_children_count": 2
    },
    {
      "resource_id": 124,
      "status": "synced",
      "has_drift": false,
      "last_applied_at": "2026-01-18T14:00:00Z",
      "drifted_children_count": 0
    },
    {
      "resource_id": 125,
      "status": "unapplied",
      "has_drift": false,
      "last_applied_at": null,
      "drifted_children_count": 0
    }
  ]
}
```

---

## 4. 前端设计

### 4.1 Health Tab 设计

Drift 资源列表将放在 `/workspaces/:id?tab=health` 页面中，支持展开查看详情。

#### 4.1.1 页面结构

```tsx
// HealthTab.tsx
const HealthTab: React.FC<{ workspaceId: string }> = ({ workspaceId }) => {
  const [driftResult, setDriftResult] = useState<DriftResult | null>(null);
  const [expandedResources, setExpandedResources] = useState<Set<number>>(new Set());
  const [loading, setLoading] = useState(true);

  return (
    <div className={styles.healthContainer}>
      {/* Drift 检测配置卡片 */}
      <DriftConfigCard workspaceId={workspaceId} />
      
      {/* Drift 检测状态概览 */}
      <DriftStatusOverview driftResult={driftResult} />
      
      {/* 资源 Drift 列表（可展开） */}
      <DriftResourceList 
        resources={driftResult?.drift_details?.resources || []}
        expandedResources={expandedResources}
        onToggleExpand={handleToggleExpand}
      />
    </div>
  );
};
```

#### 4.1.2 Drift 状态概览卡片

```tsx
// DriftStatusOverview.tsx
const DriftStatusOverview: React.FC<{ driftResult: DriftResult | null }> = ({ driftResult }) => {
  return (
    <div className={styles.statusOverview}>
      <div className={styles.statusHeader}>
        <h3>Drift Detection Status</h3>
        <span className={styles.lastCheck}>
          Last check: {driftResult?.last_check_at ? formatRelativeTime(driftResult.last_check_at) : 'Never'}
        </span>
      </div>
      
      {/* 状态统计条 */}
      <div className={styles.statusBar}>
        <div 
          className={styles.statusBarDrift} 
          style={{ width: `${(driftResult?.drift_count || 0) / (driftResult?.total_resources || 1) * 100}%` }}
        />
        <div 
          className={styles.statusBarSynced} 
          style={{ width: `${((driftResult?.total_resources || 0) - (driftResult?.drift_count || 0)) / (driftResult?.total_resources || 1) * 100}%` }}
        />
      </div>
      
      {/* 统计数字 */}
      <div className={styles.statusStats}>
        <div className={styles.statItem}>
          <span className={styles.statValue} style={{ color: '#ffc107' }}>{driftResult?.drift_count || 0}</span>
          <span className={styles.statLabel}>Drifted</span>
        </div>
        <div className={styles.statItem}>
          <span className={styles.statValue} style={{ color: '#28a745' }}>
            {(driftResult?.total_resources || 0) - (driftResult?.drift_count || 0)}
          </span>
          <span className={styles.statLabel}>Synced</span>
        </div>
        <div className={styles.statItem}>
          <span className={styles.statValue}>{driftResult?.total_resources || 0}</span>
          <span className={styles.statLabel}>Total</span>
        </div>
      </div>
    </div>
  );
};
```

#### 4.1.3 可展开的资源 Drift 列表

```tsx
// DriftResourceList.tsx
const DriftResourceList: React.FC<{
  resources: DriftResource[];
  expandedResources: Set<number>;
  onToggleExpand: (resourceId: number) => void;
}> = ({ resources, expandedResources, onToggleExpand }) => {
  return (
    <div className={styles.resourceList}>
      <div className={styles.listHeader}>
        <h3>Resources</h3>
        <div className={styles.filterButtons}>
          <button className={styles.filterActive}>All</button>
          <button>Drifted</button>
          <button>Synced</button>
          <button>Unapplied</button>
        </div>
      </div>
      
      {resources.map(resource => (
        <div key={resource.resource_id} className={styles.resourceItem}>
          {/* 资源行（可点击展开） */}
          <div 
            className={`${styles.resourceRow} ${resource.has_drift ? styles.driftRow : styles.syncedRow}`}
            onClick={() => onToggleExpand(resource.resource_id)}
          >
            <span className={styles.expandIcon}>
              {expandedResources.has(resource.resource_id) ? '▼' : '▶'}
            </span>
            <span className={styles.resourceName}>{resource.resource_name}</span>
            <ResourceStatusBadge status={resource.has_drift ? 'drift' : 'synced'} />
            {resource.has_drift && (
              <span className={styles.driftCount}>
                {resource.drifted_children?.length || 0} drifted
              </span>
            )}
          </div>
          
          {/* 展开的子资源详情 */}
          {expandedResources.has(resource.resource_id) && resource.drifted_children && (
            <div className={styles.childrenList}>
              {resource.drifted_children.map((child, index) => (
                <DriftChildItem key={index} child={child} />
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  );
};

// 子资源 Drift 详情
const DriftChildItem: React.FC<{ child: DriftedChild }> = ({ child }) => {
  const [showChanges, setShowChanges] = useState(false);
  
  return (
    <div className={styles.childItem}>
      <div className={styles.childHeader} onClick={() => setShowChanges(!showChanges)}>
        <span className={styles.childAddress}>{child.address}</span>
        <span className={styles.childType}>{child.type}</span>
        <span className={`${styles.childAction} ${styles[`action-${child.action}`]}`}>
          {child.action}
        </span>
      </div>
      
      {/* 变更详情（JSON diff） */}
      {showChanges && child.changes && (
        <div className={styles.changesDetail}>
          {Object.entries(child.changes).map(([key, value]) => (
            <div key={key} className={styles.changeItem}>
              <div className={styles.changeKey}>{key}</div>
              <div className={styles.changeValues}>
                <div className={styles.changeBefore}>
                  <span className={styles.changeLabel}>Before:</span>
                  <pre>{JSON.stringify(value.before, null, 2)}</pre>
                </div>
                <div className={styles.changeAfter}>
                  <span className={styles.changeLabel}>After:</span>
                  <pre>{JSON.stringify(value.after, null, 2)}</pre>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
```

#### 4.1.4 样式定义

```css
/* HealthTab.module.css */

/* 资源行状态颜色 */
.driftRow {
  background-color: #fff3cd;
  border-left: 4px solid #ffc107;
}

.syncedRow {
  background-color: #d4edda;
  border-left: 4px solid #28a745;
}

.unappliedRow {
  background-color: #e9ecef;
  border-left: 4px solid #6c757d;
}

/* 子资源操作类型颜色 */
.action-create { color: #28a745; }
.action-update { color: #ffc107; }
.action-delete { color: #dc3545; }
.action-replace { color: #fd7e14; }

/* 变更详情 */
.changeBefore {
  background-color: #ffebee;
  border-left: 3px solid #dc3545;
}

.changeAfter {
  background-color: #e8f5e9;
  border-left: 3px solid #28a745;
}
```

### 4.2 资源列表状态显示

```tsx
// 资源状态颜色定义
const RESOURCE_STATUS_COLORS = {
  drift: {
    background: '#fff3cd',  // 黄色背景
    border: '#ffc107',
    text: '#856404',
    label: 'Drift'
  },
  synced: {
    background: '#d4edda',  // 绿色背景
    border: '#28a745',
    text: '#155724',
    label: 'Synced'
  },
  unapplied: {
    background: '#e9ecef',  // 灰色背景
    border: '#6c757d',
    text: '#495057',
    label: 'Unapplied'
  }
};

// 资源状态徽章组件
const ResourceStatusBadge: React.FC<{ status: string }> = ({ status }) => {
  const colors = RESOURCE_STATUS_COLORS[status] || RESOURCE_STATUS_COLORS.unapplied;
  
  return (
    <span style={{
      backgroundColor: colors.background,
      border: `1px solid ${colors.border}`,
      color: colors.text,
      padding: '2px 8px',
      borderRadius: '4px',
      fontSize: '12px'
    }}>
      {colors.label}
    </span>
  );
};
```

### 4.2 Workspace 设置页面 - Drift 配置

```tsx
// Drift 检测配置表单
const DriftConfigForm: React.FC<{ workspaceId: string }> = ({ workspaceId }) => {
  const [config, setConfig] = useState({
    drift_check_enabled: false,
    drift_check_start_time: '09:00',
    drift_check_end_time: '18:00',
    drift_check_interval: 60
  });

  return (
    <div className="drift-config-section">
      <h3>Drift Detection Settings</h3>
      
      <div className="form-group">
        <label>
          <input
            type="checkbox"
            checked={config.drift_check_enabled}
            onChange={(e) => setConfig({...config, drift_check_enabled: e.target.checked})}
          />
          Enable Drift Detection
        </label>
      </div>
      
      <div className="form-row">
        <div className="form-group">
          <label>Start Time</label>
          <input
            type="time"
            value={config.drift_check_start_time}
            onChange={(e) => setConfig({...config, drift_check_start_time: e.target.value})}
            disabled={!config.drift_check_enabled}
          />
        </div>
        
        <div className="form-group">
          <label>End Time</label>
          <input
            type="time"
            value={config.drift_check_end_time}
            onChange={(e) => setConfig({...config, drift_check_end_time: e.target.value})}
            disabled={!config.drift_check_enabled}
          />
        </div>
      </div>
      
      <div className="form-group">
        <label>Check Interval (minutes)</label>
        <select
          value={config.drift_check_interval}
          onChange={(e) => setConfig({...config, drift_check_interval: parseInt(e.target.value)})}
          disabled={!config.drift_check_enabled}
        >
          <option value={30}>30 minutes</option>
          <option value={60}>1 hour</option>
          <option value={120}>2 hours</option>
          <option value={240}>4 hours</option>
        </select>
        <small>Note: Drift check runs at most once per day</small>
      </div>
      
      <button onClick={handleSave}>Save Configuration</button>
    </div>
  );
};
```

### 4.3 资源列表页面改造

```tsx
// WorkspaceResources.tsx 改造
const WorkspaceResources: React.FC = () => {
  const [resources, setResources] = useState([]);
  const [driftStatus, setDriftStatus] = useState<Map<number, ResourceDriftStatus>>(new Map());
  
  useEffect(() => {
    // 加载资源列表
    loadResources();
    // 加载 drift 状态
    loadDriftStatus();
  }, [workspaceId]);
  
  const loadDriftStatus = async () => {
    const response = await api.get(`/workspaces/${workspaceId}/resources/drift-status`);
    const statusMap = new Map();
    response.data.resources.forEach(r => statusMap.set(r.resource_id, r));
    setDriftStatus(statusMap);
  };
  
  return (
    <div className="resources-list">
      {resources.map(resource => {
        const status = driftStatus.get(resource.id);
        return (
          <div 
            key={resource.id} 
            className={`resource-item status-${status?.status || 'unknown'}`}
          >
            <div className="resource-header">
              <span className="resource-name">{resource.resource_name}</span>
              <ResourceStatusBadge status={status?.status || 'unknown'} />
            </div>
            {status?.status === 'drift' && (
              <div className="drift-info">
                <span className="drift-count">
                  {status.drifted_children_count} drifted resources
                </span>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
};
```

---

## 5. 实现步骤

### Phase 1: 数据库变更
1. 创建数据库迁移脚本
2. 执行迁移，添加新字段和新表
3. 更新 GORM 模型

### Phase 2: 后端核心服务
1. 创建 `DriftCheckService` 服务
2. 创建 `DriftCheckScheduler` 调度器
3. 实现 terraform plan --refresh-only 执行逻辑
4. 实现 plan 输出解析逻辑
5. 在 main.go 中启动调度器

### Phase 3: API 接口
1. 添加 drift 配置 CRUD 接口
2. 添加 drift 结果查询接口
3. 添加手动触发接口
4. 添加资源 drift 状态批量查询接口

### Phase 4: 前端实现
1. 资源列表添加状态颜色显示
2. Workspace 设置页面添加 drift 配置
3. 添加 drift 详情查看功能

### Phase 5: 测试与优化
1. 单元测试
2. 集成测试
3. 性能优化

---

## 6. 注意事项

### 6.1 TF_CLI_ARGS 处理
Drift 检测需要检查所有资源，不受 `TF_CLI_ARGS` 中的 `--target` 参数限制。

**处理逻辑**：
```go
// RemoveTargetFromTFCliArgs 从 TF_CLI_ARGS 环境变量中移除所有 --target 参数
func RemoveTargetFromTFCliArgs(envVars map[string]string) map[string]string {
    result := make(map[string]string)
    
    for key, value := range envVars {
        if key == "TF_CLI_ARGS" || key == "TF_CLI_ARGS_plan" || key == "TF_CLI_ARGS_apply" {
            // 移除 --target 参数
            // 格式可能是: --target=module.xxx 或 --target module.xxx
            cleaned := removeTargetArgs(value)
            if cleaned != "" {
                result[key] = cleaned
            }
            // 如果清理后为空，则不添加该环境变量
        } else {
            result[key] = value
        }
    }
    
    return result
}

// removeTargetArgs 从参数字符串中移除所有 --target 相关参数
func removeTargetArgs(args string) string {
    // 使用正则表达式匹配并移除:
    // 1. --target=xxx (等号形式)
    // 2. --target xxx (空格形式)
    // 3. -target=xxx (短横线形式)
    // 4. -target xxx (短横线空格形式)
    
    patterns := []string{
        `--target=[^\s]+`,      // --target=value
        `--target\s+[^\s]+`,    // --target value
        `-target=[^\s]+`,       // -target=value
        `-target\s+[^\s]+`,     // -target value
    }
    
    result := args
    for _, pattern := range patterns {
        re := regexp.MustCompile(pattern)
        result = re.ReplaceAllString(result, "")
    }
    
    // 清理多余的空格
    result = strings.TrimSpace(result)
    result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
    
    return result
}
```

**示例**：
| 原始 TF_CLI_ARGS | 处理后 |
|------------------|--------|
| `--target=module.s3 --target=module.ec2` | `` (空) |
| `--parallelism=10 --target=module.s3` | `--parallelism=10` |
| `-target module.s3 -compact-warnings` | `-compact-warnings` |

### 6.2 Agent 可用性检查
- 检查 workspace 关联的 agent pool 是否有在线 agent
- 如果是 K8s 模式，检查是否可以创建 pod
- Agent 不可用时记录日志但不报错

### 6.2 并发控制
- 同一 workspace 同时只能有一个 drift 检测任务在运行
- 使用 check_status 字段进行状态控制

### 6.3 错误处理
- terraform plan 执行失败时记录错误信息
- 不影响其他 workspace 的检测
- 失败的检测不计入每日限制

### 6.4 资源状态判断逻辑
```go
func GetResourceStatus(resource *WorkspaceResource, driftResult *DriftResult) string {
    // 1. 从未 apply 过 -> unapplied
    if resource.LastAppliedAt == nil {
        return "unapplied"
    }
    
    // 2. 检查 drift 结果
    if driftResult != nil && driftResult.HasDrift {
        for _, r := range driftResult.DriftDetails.Resources {
            if r.ResourceID == resource.ID && r.HasDrift {
                return "drift"
            }
        }
    }
    
    // 3. 已 apply 且无 drift -> synced
    return "synced"
}
```

---

## 7. 时间估算

| 阶段 | 预计时间 |
|------|----------|
| Phase 1: 数据库变更 | 0.5 天 |
| Phase 2: 后端核心服务 | 2 天 |
| Phase 3: API 接口 | 0.5 天 |
| Phase 4: 前端实现 | 1 天 |
| Phase 5: 测试与优化 | 1 天 |
| **总计** | **5 天** |

---

## 8. 后续扩展

1. **Drift 通知**：当检测到 drift 时发送通知
2. **Drift 历史**：保留历史检测记录
3. **自动修复**：提供一键 apply 修复 drift 的功能
4. **Drift 报告**：生成 drift 检测报告
