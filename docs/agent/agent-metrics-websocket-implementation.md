# Agent Metrics WebSocket 实时监控实现

## 概述

本文档描述了通过WebSocket实时传输agent心跳metrics（CPU使用率、内存使用率、运行任务）的实现方案。

## 架构设计

### 1. 数据流向

```
Agent (心跳) → PingAgent API → AgentMetricsHub → WebSocket → Frontend
```

### 2. 核心组件

#### 2.1 AgentMetricsHub (`backend/internal/websocket/agent_metrics_hub.go`)

- 管理按pool_id分组的WebSocket连接
- 存储每个agent的最新metrics（内存中，不持久化）
- 广播metrics更新到订阅该pool的所有前端客户端
- 自动清理超过5分钟未更新的metrics

#### 2.2 AgentMetricsWSHandler (`backend/internal/handlers/agent_metrics_ws_handler.go`)

- 处理WebSocket连接升级
- 管理客户端连接的生命周期
- 路由: `/ws/agent-pools/{pool_id}/metrics`

#### 2.3 Agent Model 更新 (`backend/internal/models/agent.go`)

```go
type AgentPingRequest struct {
    Status       string        `json:"status"`
    CPUUsage     float64       `json:"cpu_usage"`      // 0-100
    MemoryUsage  float64       `json:"memory_usage"`   // 0-100
    RunningTasks []RunningTask `json:"running_tasks"`
}

type RunningTask struct {
    TaskID      uint   `json:"task_id"`
    TaskType    string `json:"task_type"`
    WorkspaceID string `json:"workspace_id"`
    StartedAt   string `json:"started_at"`
}
```

#### 2.4 PingAgent Handler 更新

- 接收agent心跳时包含的metrics数据
- 通过AgentMetricsHub广播到WebSocket订阅者

## 实现步骤

### 后端实现

1.  创建 `AgentMetricsHub` - WebSocket hub管理
2.  创建 `AgentMetricsWSHandler` - WebSocket连接处理
3.  更新 `Agent` model - 添加metrics字段到ping请求
4.  更新 `AgentHandler` - 接收并广播metrics
5. ⏳ 在 `main.go` 中初始化 `AgentMetricsHub`
6. ⏳ 在 router 中添加 WebSocket 路由
7. ⏳ 更新 `NewAgentHandler` 调用传入 `metricsHub`

### 前端实现

1. ⏳ 创建 WebSocket 连接管理
2. ⏳ 在 AgentPoolDetail 页面订阅 metrics
3. ⏳ 实现 CPU/Memory 使用率可视化组件
4. ⏳ 实现颜色编码的横向柱状图
   - 绿色: 0-70%
   - 黄色: 70-90%
   - 红色: 90-100%
5. ⏳ 显示运行中的任务列表

## WebSocket 消息格式

### 初始连接消息 (服务端 → 客户端)

```json
{
  "type": "initial_metrics",
  "pool_id": "pool-xxx",
  "metrics": [
    {
      "agent_id": "agent-xxx",
      "agent_name": "agent-1",
      "cpu_usage": 45.5,
      "memory_usage": 62.3,
      "running_tasks": [
        {
          "task_id": 123,
          "task_type": "plan",
          "workspace_id": "ws-xxx",
          "started_at": "2025-01-07T10:30:00Z"
        }
      ],
      "last_update_time": "2025-01-07T10:35:00Z",
      "status": "busy"
    }
  ]
}
```

### Metrics 更新消息 (服务端 → 客户端)

```json
{
  "type": "metrics_update",
  "pool_id": "pool-xxx",
  "metrics": {
    "agent_id": "agent-xxx",
    "agent_name": "agent-1",
    "cpu_usage": 48.2,
    "memory_usage": 65.1,
    "running_tasks": [...],
    "last_update_time": "2025-01-07T10:36:00Z",
    "status": "busy"
  }
}
```

### Agent 离线消息 (服务端 → 客户端)

```json
{
  "type": "agent_offline",
  "pool_id": "pool-xxx",
  "metrics": {
    "agent_id": "agent-xxx",
    "status": "offline"
  }
}
```

## 使用示例

### Agent 端发送心跳

```bash
curl -X POST http://localhost:8080/api/v1/agents/{agent_id}/ping \
  -H "X-App-Key: xxx" \
  -H "X-App-Secret: xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "status": "busy",
    "cpu_usage": 45.5,
    "memory_usage": 62.3,
    "running_tasks": [
      {
        "task_id": 123,
        "task_type": "plan",
        "workspace_id": "ws-xxx",
        "started_at": "2025-01-07T10:30:00Z"
      }
    ]
  }'
```

### 前端订阅 metrics

```typescript
const ws = new WebSocket(`ws://localhost:8080/ws/agent-pools/${poolId}/metrics`);

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  
  if (message.type === 'initial_metrics') {
    // 初始化显示所有agent的metrics
    setAgentMetrics(message.metrics);
  } else if (message.type === 'metrics_update') {
    // 更新单个agent的metrics
    updateAgentMetrics(message.metrics);
  } else if (message.type === 'agent_offline') {
    // 标记agent为离线
    markAgentOffline(message.metrics.agent_id);
  }
};
```

## 注意事项

1. **不持久化**: Metrics数据仅存储在内存中，服务重启后会丢失
2. **自动清理**: 超过5分钟未更新的metrics会被自动清理
3. **实时性**: Agent心跳间隔决定了metrics更新频率（建议30-60秒）
4. **连接管理**: 前端需要处理WebSocket断线重连
5. **性能考虑**: 大量agent时考虑限流和批量更新

## 下一步

1. 完成 main.go 和 router 的更新
2. 实现前端 WebSocket 连接和UI组件
3. 测试完整的数据流
4. 优化性能和错误处理
