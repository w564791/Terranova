# Agent API 网络重试机制优化

## 背景

在弱网环境下，Agent 与 IAC Server 之间的 HTTP 通信可能因为网络抖动而失败，导致任务执行失败。

### 问题现象

```
Error: failed to resolve variable snapshots: failed to parse snapshot data in any known format
```

任务 #1018 失败后，用户手动重试创建任务 #1019 成功执行。两个任务的快照数据格式完全相同，说明问题不在数据本身，而是网络通信的间歇性问题。

## 解决方案

### 1. 重试配置

在 `AgentAPIClient` 中添加了可配置的重试机制：

```go
type RetryConfig struct {
    MaxRetries  int           // 最大重试次数（默认 3）
    BaseDelay   time.Duration // 基础延迟时间（默认 1s）
    MaxDelay    time.Duration // 最大延迟时间（默认 10s）
    RetryOn5xx  bool          // 是否在 5xx 错误时重试
    RetryOnConn bool          // 是否在连接错误时重试
}
```

### 2. 指数退避算法

重试间隔采用指数退避策略：
- 第 1 次重试：等待 1 秒
- 第 2 次重试：等待 2 秒
- 第 3 次重试：等待 4 秒
- 最大延迟不超过 10 秒

### 3. 连接池优化

优化了 HTTP Transport 配置：

```go
transport := &http.Transport{
    MaxIdleConns:        100,
    MaxIdleConnsPerHost: 10,
    IdleConnTimeout:     90 * time.Second,
    DisableCompression:  false,
    DialContext: (&net.Dialer{
        Timeout:   10 * time.Second,
        KeepAlive: 30 * time.Second,
    }).DialContext,
}
```

### 4. 超时时间调整

HTTP 客户端超时从 30 秒增加到 60 秒，以适应大数据传输场景。

## 启用重试的 API

以下关键 API 已启用重试机制：

| API 方法 | 用途 | 幂等性 |
|---------|------|--------|
| `GetTaskDataWithRetry` | 获取任务数据 | ✅ 只读 |
| `GetPlanTaskWithRetry` | 获取 Plan 任务（最关键） | ✅ 只读 |
| `SaveTaskStateWithRetry` | 保存 State | ✅ 幂等 |
| `UpdateTaskStatusWithRetry` | 更新任务状态 | ✅ 幂等 |
| `UploadPlanDataWithRetry` | 上传 Plan 数据 | ✅ 覆盖写入 |
| `UploadPlanJSONWithRetry` | 上传 Plan JSON | ✅ 覆盖写入 |

## 风险评估

### 低风险原因

1. **幂等性保证**：所有启用重试的 API 都是幂等的
2. **有限重试**：最多重试 3 次，不会无限消耗资源
3. **指数退避**：避免对服务端造成冲击
4. **向后兼容**：原有的非重试方法仍然保留

### 最坏情况

- 最大额外延迟：1s + 2s + 4s = 7 秒
- 仅在网络不稳定时触发

## 日志输出

重试时会输出日志，便于问题排查：

```
[AgentAPIClient] Request failed (attempt 1/4): connection refused, retrying in 1s
[AgentAPIClient] Request failed (attempt 2/4): connection refused, retrying in 2s
[AgentAPIClient] Request succeeded after 2 retries
```

## 相关文件

- `backend/services/agent_api_client.go` - 重试机制实现
- `backend/services/remote_data_accessor.go` - 使用重试方法

## 后续优化建议

1. **监控指标**：添加重试次数的 Prometheus 指标
2. **配置化**：将重试参数移到配置文件中
3. **断路器**：考虑添加断路器模式，防止雪崩