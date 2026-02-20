# 高可用多副本部署方案

## 1. 背景

当前后端服务为单实例部署，所有 API 请求处理和后台协程运行在同一个 Pod 中。为实现高可用和负载均衡，需要支持多副本部署。

### 1.1 目标

- **高可用**：单个 Pod 故障时服务不中断，自动故障转移
- **负载均衡**：多副本同时处理 API 请求，分摊流量压力
- **正确性**：后台定时任务不重复执行，跨副本互斥锁有效

### 1.2 约束

- 不引入新的外部依赖（不加 Redis、消息队列等），仅使用 PostgreSQL + K8s 原生能力
- Agent 通过 K8s Service RR 轮询接入，无法控制连接到哪个副本
- Agent 注册信息和在线状态已持久化到数据库

---

## 2. 现状分析

### 2.1 当前架构

```
┌──────────────┐
│   单个 Pod    │
│              │
│  HTTP API    │  ← 端口 8080
│  WebSocket   │  ← 前端推送
│  C&C Server  │  ← 端口 8090，Agent 连接
│              │
│  后台协程 x10+│
└──────┬───────┘
       │
┌──────┴───────┐
│  PostgreSQL  │
└──────────────┘
```

### 2.2 后台协程清单

以下协程在 `backend/main.go` 启动时创建，多副本部署时**不能**重复运行：

| 协程 | 文件位置 | 间隔 | 多副本风险 |
|------|---------|------|-----------|
| `DriftCheckScheduler.Start()` | `services/drift_check_scheduler.go` | 1 分钟 | 重复创建 drift 检测任务 |
| `K8sDeploymentService.StartAutoScaler()` | `services/k8s_deployment_service.go` | 5 秒 | 重复扩缩容，Pod 数量异常 |
| `TaskQueueManager.StartPendingTasksMonitor()` | `services/task_queue_manager.go:815` | 10 秒 | 同一任务被多副本重试 |
| `CMDBSyncScheduler.Start()` | `services/cmdb_sync_scheduler.go` | 1 分钟 | 重复同步 CMDB 数据 |
| `AgentCleanupService.Start()` | `services/agent_cleanup_service.go` | 5 分钟 | 重复清理（低风险但浪费） |
| `RunTaskTimeoutChecker.Start()` | `services/run_task_timeout_checker.go` | 30 秒 | 可能重复标记超时 |
| `EmbeddingWorker.Start()` | `services/embedding_worker.go` | 后台 | 重复处理 embedding |
| 锁/草稿清理 | `main.go:179-201` | 1 分钟 | 重复清理 |

以下组件多副本运行**没有问题**（或需要每个副本都运行）：

| 组件 | 说明 |
|------|------|
| HTTP API Server | 无状态，可负载均衡 |
| WebSocket Hub | 每个副本维护自己连接的前端客户端 |
| Agent C&C Handler | 每个副本维护自己连接的 Agent |
| OutputStreamManager | 清理本副本的输出流 |

### 2.3 资源编辑锁现状

资源编辑使用了协作编辑锁机制，涉及以下组件：

**数据模型**（均存储在 PostgreSQL 中）：

| 表 | 用途 |
|---|------|
| `resource_locks` | 编辑锁：`resource_id` + `session_id` 唯一，含 `editing_user_id`、`last_heartbeat` |
| `resource_drifts` | 编辑草稿：JSONB 存储编辑内容，含 `base_version` 用于乐观锁冲突检测 |
| `takeover_requests` | 接管请求：用户间的编辑权转移，含 `status`（pending/approved/rejected/expired）和 `expires_at` |

**前端交互流程**：
1. 进入编辑页 → `POST /editing/start` 创建锁
2. 每 5 秒 → `POST /editing/heartbeat` 续约
3. 每 5 秒 → `GET /editing/status` 轮询其他编辑者
4. 内容变更 500ms 防抖 → `POST /drift/save` 自动保存草稿
5. 离开页面 → `POST /editing/end` 释放锁
6. 接管请求/响应 → 通过 WebSocket 推送通知

**多副本评估**：

锁和草稿数据全部存储在 PostgreSQL 中，**基本的锁获取/释放/心跳在多副本下没有问题**。任何副本都能正确读写锁状态。

但存在一个关键问题：**WebSocket 通知跨副本不可达**。

当前 Takeover 流程依赖 WebSocket 推送：
- 用户 B 发起 takeover 请求 → 后端通过 `wsHub.SendToSession(targetSession, ...)` 通知用户 A
- 用户 A 响应（approve/reject）→ 后端通过 WebSocket 通知用户 B
- 超时自动接管 → 通过 WebSocket 发送 `force_takeover` 给被接管方

多副本下，如果用户 A 的 WebSocket 连接在 Pod 1，而 takeover 请求处理在 Pod 2，`wsHub.SendToSession()` 在 Pod 2 的本地 Hub 中找不到用户 A 的连接，**通知丢失**。

此外，编辑状态的实时提示（如"用户 X 正在编辑"）如果通过 WebSocket 推送，也存在同样的跨副本问题。当前前端通过 5 秒轮询 `/editing/status` 获取其他编辑者信息，这条路径是无状态的，多副本下正常工作。

### 2.4 关键问题汇总

1. **内存锁跨副本无效**：`TaskQueueManager` 中的 `sync.Map` workspace 锁是进程内存锁，多副本下同一 workspace 的任务可能并发执行
2. **任务分发跨副本**：任务可能在副本 A 创建，但目标 Agent 连接在副本 B
3. **定时任务重复执行**：所有定时协程在每个副本都会启动
4. **WebSocket 通知跨副本不可达**：Takeover 通知、任务状态推送等 WebSocket 消息无法送达其他副本上的客户端

---

## 3. 方案设计：K8s Lease Leader Election + PG Advisory Lock

### 3.1 整体架构

```
                    ┌──────────────┐
                    │  K8s Service  │
                    │  (RR 轮询)    │
                    └──────┬───────┘
               ┌───────────┼───────────┐
               ▼           ▼           ▼
          ┌─────────┐ ┌─────────┐ ┌─────────┐
          │ Pod A   │ │ Pod B   │ │ Pod C   │
          │ (Leader)│ │(Follower)│ │(Follower)│
          │         │ │         │ │         │
          │ API  ✓  │ │ API  ✓  │ │ API  ✓  │
          │ WS   ✓  │ │ WS   ✓  │ │ WS   ✓  │
          │ C&C  ✓  │ │ C&C  ✓  │ │ C&C  ✓  │
          │ 定时  ✓ │ │ 定时  ✗ │ │ 定时  ✗ │
          └────┬────┘ └────┬────┘ └────┬────┘
               └───────────┼───────────┘
                    ┌──────┴──────┐
                    │ PostgreSQL  │
                    │             │
                    │ • 数据存储   │
                    │ • Advisory  │
                    │   Lock      │
                    │ • NOTIFY /  │
                    │   LISTEN    │
                    └─────────────┘
```

### 3.2 两个核心机制

#### 机制 A：K8s Lease Leader Election

**用途**：决定哪个副本运行后台定时协程。

**原理**：
- 使用 `client-go` 内置的 `leaderelection` 包（已在 go.mod 依赖中）
- 所有副本竞争同一个 K8s Lease 资源
- 获取到 Lease 的副本成为 leader，启动所有定时协程
- Leader 持续续约（默认每 10 秒），如果续约失败（Pod 崩溃），其他副本在 ~15 秒内接管

**实现要点**：

```go
// 伪代码 - main.go 中的改造
func main() {
    // ... 初始化数据库、API 路由等（所有副本都执行）

    // 启动 leader election
    leaderCtx, leaderCancel := context.WithCancel(ctx)
    go runLeaderElection(leaderCtx, func(ctx context.Context) {
        // 成为 leader 时启动后台协程
        driftCheckScheduler.Start(ctx)
        k8sDeploymentService.StartAutoScaler(ctx)
        taskQueueManager.StartPendingTasksMonitor(ctx)
        cmdbSyncScheduler.Start(ctx)
        agentCleanupService.Start(ctx)
        runTaskTimeoutChecker.Start(ctx)
        embeddingWorker.Start(ctx)
        startBackgroundCleanup(ctx)
    }, func() {
        // 失去 leader 时停止后台协程
        leaderCancel()
    })

    // API 服务器（所有副本都运行）
    startHTTPServer()
}
```

**K8s RBAC 配置**：

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: iac-platform-leader-election
  namespace: <namespace>
rules:
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "create", "update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: iac-platform-leader-election
  namespace: <namespace>
subjects:
  - kind: ServiceAccount
    name: iac-platform
    namespace: <namespace>
roleRef:
  kind: Role
  name: iac-platform-leader-election
  apiGroup: rbac.authorization.k8s.io
```

**Lease 参数**：

| 参数 | 值 | 说明 |
|------|---|------|
| LeaseDuration | 15s | 锁的有效期 |
| RenewDeadline | 10s | 续约超时 |
| RetryPeriod | 2s | 竞选重试间隔 |
| Lease Name | `iac-platform-leader` | Lease 资源名称 |
| Lease Namespace | 与 Pod 相同 | 从 Downward API 获取 |

#### 机制 B：PG Advisory Lock 替换内存锁

**用途**：替换 `TaskQueueManager` 中的 `sync.Map` workspace 锁，保证跨副本同一 workspace 的任务不并发执行。

**原理**：
- PostgreSQL advisory lock 是数据库级别的分布式锁
- `pg_try_advisory_lock(key)` 非阻塞尝试获取锁
- `pg_advisory_unlock(key)` 释放锁
- 连接断开时自动释放（Pod 崩溃安全）

**改造范围**：

```go
// 当前代码 (task_queue_manager.go)
type TaskQueueManager struct {
    workspaceLocks sync.Map // ← 内存锁，跨副本无效
}

// 改造后
type TaskQueueManager struct {
    db *gorm.DB // 使用 PG advisory lock
}

// 获取 workspace 锁
func (m *TaskQueueManager) tryLockWorkspace(workspaceID uint) (bool, error) {
    var locked bool
    err := m.db.Raw("SELECT pg_try_advisory_lock(?)", workspaceID).Scan(&locked).Error
    return locked, err
}

// 释放 workspace 锁
func (m *TaskQueueManager) unlockWorkspace(workspaceID uint) error {
    return m.db.Exec("SELECT pg_advisory_unlock(?)", workspaceID).Error
}
```

### 3.3 跨副本任务分发：PG NOTIFY/LISTEN

**问题**：任务在副本 A（leader）的 PendingTasksMonitor 中被调度，但目标 Agent 的 WebSocket 连接在副本 B。

**方案**：使用 PostgreSQL 的 NOTIFY/LISTEN 机制实现跨副本通信。

**流程**：

```
1. Leader 的 PendingTasksMonitor 发现待执行任务
2. 查数据库确定目标 Agent（agent_id）
3. Leader 检查本地 C&C 连接中是否有该 Agent
   ├── 有 → 直接通过本地 WebSocket 下发
   └── 无 → 通过 PG NOTIFY 广播任务分发消息
4. 所有副本监听 PG LISTEN 通道
5. 持有目标 Agent 连接的副本接收到通知后执行分发
```

**实现要点**：

```go
// 通知通道名称
const TaskDispatchChannel = "task_dispatch"

// 发送通知（Leader 端）
func (m *TaskQueueManager) notifyTaskDispatch(agentID string, taskID uint) error {
    payload := fmt.Sprintf("%s:%d", agentID, taskID)
    return m.db.Exec("SELECT pg_notify(?, ?)", TaskDispatchChannel, payload).Error
}

// 监听通知（所有副本启动时）
func (h *RawAgentCCHandler) listenTaskDispatch(ctx context.Context) {
    // 使用 lib/pq 或 pgx 的 LISTEN 功能
    // 收到通知后检查本地是否持有该 Agent 连接
    // 如果持有，执行任务下发
}
```

**PG NOTIFY 的局限性与应对**：

| 局限 | 影响 | 应对策略 |
|------|------|---------|
| 消息不持久化 | 连接断开时丢失 | PendingTasksMonitor 每 10 秒轮询兜底 |
| 消息大小限制 8000 字节 | payload 不能太大 | 只传 agent_id + task_id，详情从数据库读 |
| 无确认机制 | 不知道是否被消费 | 任务状态在数据库中，未消费的下次轮询会重试 |

### 3.4 Agent C&C 连接管理

**当前状态**：Agent 通过 WebSocket 连接到 C&C Server（端口 8090），连接信息保存在内存 `map[string]*RawAgentConnection`。

**多副本改造**：

1. **Agent 心跳更新数据库**：Agent 每次心跳时，更新数据库中的 `last_heartbeat_at` 和 `connected_pod`（当前连接的 Pod 标识）
2. **Pod 标识**：通过 K8s Downward API 注入 Pod 名称到环境变量
3. **连接感知**：任务分发时先查数据库确认 Agent 连接在哪个 Pod，再决定本地下发还是 PG NOTIFY

```yaml
# Deployment 中注入 Pod 名称
env:
  - name: POD_NAME
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
```

### 3.5 前端 WebSocket 跨副本广播

**当前状态**：前端通过 WebSocket 连接到后端获取实时推送（任务状态更新、日志流等）。

**多副本影响**：
- 前端用户连接到副本 A，但任务执行的状态变更可能发生在副本 B
- 需要跨副本广播 WebSocket 消息

**方案**：使用 PG NOTIFY/LISTEN 做跨副本 WebSocket 消息桥接

```
1. 任何副本产生需要推送的事件时，通过 PG NOTIFY 广播
2. 所有副本监听同一通道
3. 收到通知后，检查本地 WebSocket Hub 是否有目标客户端
4. 有则推送，无则忽略
```

**实现要点**：

```go
// 通知通道
const WSBroadcastChannel = "ws_broadcast"

// 消息结构
type WSBroadcastMessage struct {
    TargetType string `json:"target_type"` // "session", "user", "broadcast"
    TargetID   string `json:"target_id"`   // session_id 或 user_id
    EventType  string `json:"event_type"`  // "takeover_request", "task_status", etc.
    Payload    string `json:"payload"`     // JSON 编码的事件数据
    SourcePod  string `json:"source_pod"`  // 发送方 Pod，接收方可跳过自身
}
```

**发送方改造**：将现有的 `wsHub.SendToSession()` 调用替换为先尝试本地发送，失败则通过 PG NOTIFY 广播：

```go
func (hub *Hub) SendToSessionOrBroadcast(sessionID string, msg Message) {
    // 1. 尝试本地发送
    if hub.sendToLocalSession(sessionID, msg) {
        return // 本地找到，发送成功
    }
    // 2. 本地没有，通过 PG NOTIFY 广播给其他副本
    hub.pgPubSub.Notify(WSBroadcastChannel, WSBroadcastMessage{
        TargetType: "session",
        TargetID:   sessionID,
        EventType:  msg.Type,
        Payload:    msg.Data,
        SourcePod:  os.Getenv("POD_NAME"),
    })
}
```

### 3.6 资源编辑锁多副本改造

**锁数据层面**：无需改造。`resource_locks`、`resource_drifts`、`takeover_requests` 均存储在 PostgreSQL，多副本下 ACID 保证一致性。

**需要改造的部分**：Takeover WebSocket 通知。

**当前问题**：

```
用户 A 的浏览器 ──WebSocket──> Pod 1 (Hub 中有 session-A)
用户 B 的浏览器 ──WebSocket──> Pod 2 (Hub 中有 session-B)

用户 B 在 Pod 2 发起 takeover：
  Pod 2: wsHub.SendToSession("session-A", takeoverRequest)
  Pod 2 的 Hub 中没有 session-A → 通知丢失 ✗
```

**改造后**：

```
用户 B 在 Pod 2 发起 takeover：
  Pod 2: hub.SendToSessionOrBroadcast("session-A", takeoverRequest)
    → 本地 Hub 没有 session-A
    → PG NOTIFY "ws_broadcast" → 所有副本收到
  Pod 1: 收到通知，本地 Hub 有 session-A → 推送成功 ✓
```

**改造范围**：

以下位置的 `wsHub.SendToSession()` 调用需要替换为 `SendToSessionOrBroadcast()`：

| 文件 | 调用场景 |
|------|---------|
| `internal/handlers/takeover_handler.go` | 发起 takeover 请求时通知目标用户 |
| `internal/handlers/takeover_handler.go` | 响应 takeover 时通知请求方 |
| `internal/handlers/takeover_handler.go` | 超时自动接管时通知被接管方（force_takeover） |
| `internal/websocket/hub.go` | 任务状态变更推送 |

**前端无需改动**：前端的心跳（5 秒）和状态轮询（5 秒）都是 HTTP 请求，走负载均衡到任意副本均可正常工作。WebSocket 只用于接收推送通知，改造后端广播机制即可。

**清理协程**：`CleanupExpiredLocks()`、`CleanupOldDrifts()`、`CleanupExpiredRequests()` 均为幂等的 DELETE 操作（`WHERE last_heartbeat < ...`），多副本同时运行不会有问题（首个成功删除，其他看到 0 rows affected）。但作为资源优化，可以将其归入 leader-only 协程。

---

## 4. 改造清单

### 4.1 新增模块

| 模块 | 文件 | 说明 |
|------|------|------|
| Leader Election | `backend/pkg/leaderelection/leader.go` | 封装 K8s Lease leader election |
| PG Distributed Lock | `backend/pkg/pglock/advisory_lock.go` | 封装 PG advisory lock |
| PG PubSub | `backend/pkg/pgpubsub/notify.go` | 封装 PG NOTIFY/LISTEN |

### 4.2 改造现有模块

| 模块 | 文件 | 改造内容 |
|------|------|---------|
| main.go | `backend/main.go` | 后台协程启动加 leader 判断，启动 PG LISTEN |
| TaskQueueManager | `services/task_queue_manager.go` | `sync.Map` → PG advisory lock |
| TaskQueueManager | `services/task_queue_manager.go` | 任务分发加 PG NOTIFY 跨副本通知 |
| RawAgentCCHandler | `internal/handlers/agent_cc_handler_raw.go` | 监听 PG LISTEN，处理跨副本任务分发 |
| Agent 心跳处理 | `internal/handlers/agent_cc_handler_raw.go` | 心跳时写入 `connected_pod` |
| WebSocket Hub | `internal/websocket/hub.go` | 新增 `SendToSessionOrBroadcast()`，本地找不到时走 PG NOTIFY |
| Takeover Handler | `internal/handlers/takeover_handler.go` | 所有 `wsHub.SendToSession()` 替换为 `SendToSessionOrBroadcast()` |
| 所有定时协程 | 各 scheduler/service 文件 | 接受 `context.Context` 参数，支持取消 |

### 4.3 新增 K8s 配置

| 资源 | 说明 |
|------|------|
| ServiceAccount | Pod 使用的服务账号 |
| Role | Lease 资源的 get/create/update 权限 |
| RoleBinding | 绑定 ServiceAccount 与 Role |
| Deployment | replicas 从 1 改为 2+，注入 POD_NAME 环境变量 |

### 4.4 数据库变更

| 变更 | 说明 |
|------|------|
| agents 表加 `connected_pod` 字段 | 记录 Agent 当前连接的 Pod |

---

## 5. 定时协程改造细节

当前所有定时协程都是 `go func()` 启动后永远运行。改造后需要支持通过 `context.Context` 取消。

### 5.1 改造模式

```go
// 改造前
func (s *DriftCheckScheduler) Start() {
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            s.checkDrift()
        }
    }()
}

// 改造后
func (s *DriftCheckScheduler) Start(ctx context.Context) {
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                log.Println("DriftCheckScheduler stopped: no longer leader")
                return
            case <-ticker.C:
                s.checkDrift()
            }
        }
    }()
}
```

### 5.2 Leader 切换流程

```
Pod A 是 Leader，运行所有定时协程
    │
    ▼ Pod A 崩溃 / 网络分区
    │
    ├── K8s Lease 过期（~15 秒）
    │
    ▼ Pod B 获取 Lease，成为新 Leader
    │
    ├── Pod B 的 OnStartedLeading 回调触发
    ├── 启动所有定时协程（新的 context）
    │
    ▼ 服务恢复，定时任务继续运行
```

**切换期间的影响**：
- ~15 秒内无定时任务运行（可接受，因为定时任务本身间隔就是 10 秒 ~ 5 分钟）
- API 请求不受影响（所有副本持续处理）
- Agent C&C 连接不受影响（连接在各副本本地维护）
- 如果 Agent 连接在崩溃的 Pod 上，Agent 会自动重连到其他副本

---

## 6. 故障场景分析

| 场景 | 影响 | 恢复方式 |
|------|------|---------|
| Leader Pod 崩溃 | 定时任务暂停 ~15 秒 | 其他副本自动接管 Lease |
| Follower Pod 崩溃 | 该副本上的 Agent/WebSocket 连接断开 | Agent 自动重连；前端自动重连 WebSocket |
| PostgreSQL 不可用 | 所有副本无法工作 | 等待 PG 恢复（与当前单副本一致） |
| 网络分区（Pod 与 K8s API） | Leader 可能丢失 Lease | 旧 leader 停止定时任务，新 leader 接管 |
| PG NOTIFY 消息丢失 | 某次任务分发或 takeover 通知未送达 | 任务：PendingTasksMonitor 10 秒后重试；Takeover：35 秒超时后自动批准 |
| Takeover 通知跨副本 | 用户 A 在 Pod 1，takeover 在 Pod 2 处理 | PG NOTIFY 广播到所有副本，持有连接的副本推送 |
| 编辑者所在 Pod 崩溃 | 编辑锁心跳停止 | 60 秒后锁自动过期被清理，其他用户可编辑 |

---

## 7. 实施顺序

建议按以下顺序分阶段实施：

### 阶段一：基础设施

1. 新增 `pkg/leaderelection` 模块，封装 K8s Lease
2. 新增 `pkg/pglock` 模块，封装 PG advisory lock
3. 新增 `pkg/pgpubsub` 模块，封装 PG NOTIFY/LISTEN
4. 配置 K8s RBAC（ServiceAccount, Role, RoleBinding）

### 阶段二：定时协程改造

5. 所有定时协程添加 `context.Context` 参数支持
6. `main.go` 中接入 leader election，leader 启动定时协程，follower 不启动

### 阶段三：分布式锁

7. `TaskQueueManager` 的 `sync.Map` 替换为 PG advisory lock

### 阶段四：跨副本通信

8. Agent 心跳写入 `connected_pod` 字段
9. 任务分发接入 PG NOTIFY/LISTEN
10. WebSocket Hub 新增 `SendToSessionOrBroadcast()`，接入 PG NOTIFY/LISTEN
11. Takeover Handler 中的 `wsHub.SendToSession()` 替换为 `SendToSessionOrBroadcast()`

### 阶段五：部署与验证

12. Deployment 配置更新（replicas、POD_NAME 注入）
13. 多副本集成测试
