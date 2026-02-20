# IAC Agent v3.2 完整实施指南

## 一、背景与目标

### 1.1 项目背景
- **现状**: Server 端已实现完整的 Terraform 执行逻辑 (`TerraformExecutor`)，作为 Local 模式运行
- **需求**: 将相同的执行逻辑打包成独立 Agent，支持远程部署，通过 API 与 Server 通信
- **原则**: 代码最大化复用，Local 模式和 Agent 模式共享同一套执行逻辑

### 1.2 核心目标
-  复用现有 `TerraformExecutor` 代码，避免重复开发
-  支持 Agent 独立部署（Static 模式和 K8s 模式）
-  实现 C&C (Command & Control) 双向通信通道
-  增量日志上传，避免大日志丢失
-  支持任务取消和优雅退出

### 1.3 设计原则
- **代码复用**: Server 端 Local 模式代码必须保留，Agent 复用相同逻辑
- **最小改动**: 通过接口抽象实现不同的数据访问方式
- **可靠性**: 增量日志上传 + Server 端缓存，确保日志不丢失
- **实时性**: WebSocket C&C 通道，支持即时任务下发和控制

## 二、架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────┐
│                     Client (Web UI)                      │
└─────────────────────────┬───────────────────────────────┘
                          │ HTTP/WebSocket
                          ▼
┌─────────────────────────────────────────────────────────┐
│                      IAC Server                          │
│  ┌─────────────────────────────────────────────────┐    │
│  │  Local Mode (内置执行器)                         │    │
│  │  - TerraformExecutor                            │    │
│  │  - LocalDataAccessor (直接访问 DB)              │    │
│  └─────────────────────────────────────────────────┘    │
│                                                          │
│  ┌─────────────────────────────────────────────────┐    │
│  │  Agent API Endpoints                            │    │
│  │  - /api/v1/agents/control (WebSocket)           │    │
│  │  - /api/v1/agents/tasks/{id}/data               │    │
│  │  - /api/v1/agents/tasks/{id}/logs/chunk         │    │
│  │  - /api/v1/agents/tasks/{id}/status             │    │
│  └─────────────────────────────────────────────────┘    │
└─────────────────────────┬───────────────────────────────┘
                          ▲ Agent 主动连接 (出站)
                          │ WebSocket + HTTPS
                          │
┌─────────────────────────────────────────────────────────┐
│                      IAC Agent                           │
│  ┌─────────────────────────────────────────────────┐    │
│  │  Remote Mode (独立执行器)                        │    │
│  │  - TerraformExecutor (复用)                     │    │
│  │  - RemoteDataAccessor (通过 API 访问)           │    │
│  │  - C&C Manager (WebSocket 客户端)               │    │
│  └─────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

**重要说明**: 
- Agent 主动向 Server 发起 WebSocket 连接（出站连接）
- Server 不需要访问 Agent 的端口
- 所有通信都通过 Agent 发起的连接进行
- 适合部署在 NAT 后面或防火墙内的环境

### 2.2 执行模式对比

| 特性 | Local 模式 | Agent 模式 |
|------|-----------|-----------|
| 部署方式 | Server 内置 | 独立部署 |
| 数据访问 | 直接访问数据库 | 通过 API 访问 |
| 任务获取 | 内存队列 | C&C 通道推送 |
| 日志处理 | 直接写入数据库 | 增量上传到 Server |
| 适用场景 | 小规模、单机部署 | 大规模、分布式部署 |

### 2.3 项目结构

```
iac-platform/
├── backend/
│   ├── services/
│   │   ├── terraform_executor.go          # 核心执行逻辑 (共享)
│   │   ├── output_stream_manager.go       # 日志流管理 (共享)
│   │   ├── terraform_logger.go            # 日志记录器 (共享)
│   │   ├── data_accessor.go               # 数据访问接口 (新增)
│   │   ├── local_data_accessor.go         # Local 模式实现 (新增)
│   │   └── task_queue_manager.go          # Local 模式任务队列
│   │
│   ├── internal/handlers/
│   │   └── agent_handler.go               # Agent API 处理器 (扩展)
│   │
│   └── cmd/
│       ├── server/
│       │   └── main.go                    # Server 入口
│       └── agent/
│           └── main.go                    # Agent 入口 (新增)
│
└── agent/                                  # Agent 专用代码 (新增)
    ├── client/
    │   ├── api_client.go                  # Server API 客户端
    │   └── remote_data_accessor.go        # Remote 模式实现
    ├── control/
    │   ├── cc_manager.go                  # C&C 通道管理
    │   ├── heartbeat.go                   # 心跳上报
    │   └── task_receiver.go               # 任务接收处理
    └── worker/
        └── worker_pool.go                  # 并发控制

```

## 三、核心组件设计

### 3.1 数据访问层抽象

```go
// backend/services/data_accessor.go
type DataAccessor interface {
    // Workspace 相关
    GetWorkspace(workspaceID string) (*models.Workspace, error)
    GetWorkspaceResources(workspaceID string) ([]models.WorkspaceResource, error)
    GetWorkspaceVariables(workspaceID string, varType string) ([]models.WorkspaceVariable, error)
    
    // State 相关
    GetLatestStateVersion(workspaceID string) (*models.WorkspaceStateVersion, error)
    SaveStateVersion(version *models.WorkspaceStateVersion) error
    
    // Task 相关
    UpdateTask(task *models.WorkspaceTask) error
    SaveTaskLog(taskID uint, phase, content, level string) error
}
```

### 3.2 TerraformExecutor 改造

```go
// backend/services/terraform_executor.go
type TerraformExecutor struct {
    dataAccessor  DataAccessor          // 替代原来的 db *gorm.DB
    streamManager *OutputStreamManager
    signalManager *SignalManager
}

// 构造函数支持两种模式
func NewTerraformExecutor(accessor DataAccessor, streamManager *OutputStreamManager) *TerraformExecutor {
    return &TerraformExecutor{
        dataAccessor:  accessor,
        streamManager: streamManager,
        signalManager: GetSignalManager(),
    }
}
```

### 3.3 C&C 通道协议

```json
// Agent → Server (心跳)
{
  "type": "heartbeat",
  "payload": {
    "agent_id": "agent-xxx",
    "status": "idle",
    "plan_running": 2,
    "plan_limit": 3,
    "apply_running": false,
    "current_tasks": [123, 456],
    "cpu_usage": 0.45,
    "mem_usage": 0.62
  }
}

// Server → Agent (任务下发)
{
  "type": "run_task",
  "payload": {
    "task_id": 123,
    "workspace_id": "ws-xxx",
    "action": "plan"
  }
}

// Server → Agent (任务取消)
{
  "type": "cancel_task",
  "payload": {
    "task_id": 123
  }
}
```

## 四、API 接口设计

### 4.1 现有接口 (保留)

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/agents/register | Agent 注册 |
| GET | /api/v1/agents/{agent_id} | 获取 Agent 信息 |
| DELETE | /api/v1/agents/{agent_id} | 注销 Agent |

### 4.2 新增接口

| 方法 | 路径 | 说明 |
|------|------|------|
| WS | /api/v1/agents/control | C&C WebSocket 通道 |
| GET | /api/v1/agents/{agent_id}/tasks/pending | 获取待执行任务 |
| GET | /api/v1/agents/tasks/{task_id}/data | 获取任务执行数据 |
| POST | /api/v1/agents/tasks/{task_id}/logs/chunk | 增量日志上传 |
| PUT | /api/v1/agents/tasks/{task_id}/status | 更新任务状态 |
| POST | /api/v1/agents/tasks/{task_id}/state | 保存 State 版本 |

### 4.3 接口详细说明

#### 4.3.1 获取任务执行数据
```http
GET /api/v1/agents/tasks/{task_id}/data

Response:
{
  "task": {
    "id": 123,
    "workspace_id": "ws-xxx",
    "action": "plan",
    "context": {...}
  },
  "workspace": {
    "workspace_id": "ws-xxx",
    "name": "prod-workspace",
    "terraform_version": "1.8.5",
    "provider_config": {...},
    "tf_code": {...}
  },
  "resources": [...],
  "variables": [...],
  "state_version": {...}
}
```

#### 4.3.2 增量日志上传
```http
POST /api/v1/agents/tasks/{task_id}/logs/chunk

Request:
{
  "offset": 1024,
  "data": "base64_encoded_gzip_data",
  "checksum": "sha256_hash"
}

Response:
{
  "status": "ok",
  "next_offset": 2048
}
```

## 五、关键优化策略

### 5.1 日志系统优化

#### 增量上传机制
- **Agent 端**: 每 5MB 或 30 秒触发一次上传
- **压缩传输**: 使用 gzip 压缩，减少带宽消耗
- **断点续传**: 支持网络中断后从上次位置继续

#### Server 端磁盘缓存
- **本地文件缓存**: 使用磁盘文件系统作为缓存层，避免引入 Redis 依赖
- **分片存储**: 日志按任务 ID 和时间戳分片存储，便于管理
- **异步持久化**: 批量写入数据库，减少 I/O 压力
- **自动清理**: 24 小时后自动清理过期缓存文件

### 5.2 任务调度优化

#### 公平调度
- Plan 任务可并发执行，受 `max_plan_concurrency` 限制
- Apply 任务独占执行，同时只能有一个
- 不实现任务抢占，Plan+Apply 不会打断 Plan 任务

#### 任务取消
- 优雅取消: Context 取消 → SIGTERM → SIGKILL
- 资源清理: 释放锁、回滚状态、上传部分日志

### 5.3 K8s 模式特殊处理

#### Token 生命周期管理
- 监控 Deployment 状态
- Pod 下线后自动撤销 Token
- 防止 Token 泄露和滥用

#### Pod 优雅退出
- 接收 SIGTERM 信号
- 停止接收新任务
- 等待当前任务完成（最多 5 分钟）
- 清理资源后退出

## 六、实施计划

### Phase 1: 基础架构改造 (Week 1)
- [ ] 实现 DataAccessor 接口
- [ ] 改造 TerraformExecutor 使用接口
- [ ] 实现 LocalDataAccessor
- [ ] 验证 Local 模式正常工作

### Phase 2: Agent API 开发 (Week 2)
- [ ] 实现 C&C WebSocket Handler
- [ ] 实现任务数据获取 API
- [ ] 实现日志上传 API
- [ ] 实现状态更新 API

### Phase 3: Agent 客户端开发 (Week 3)
- [ ] 实现 RemoteDataAccessor
- [ ] 实现 C&C Manager
- [ ] 实现心跳机制
- [ ] 实现任务接收和执行

### Phase 4: 集成测试 (Week 4)
- [ ] Local 模式测试
- [ ] Static Agent 模式测试
- [ ] K8s Agent 模式测试
- [ ] 性能和压力测试

### Phase 5: 部署和文档 (Week 5)
- [ ] 编写部署文档
- [ ] 编写运维手册
- [ ] 生产环境部署
- [ ] 监控和告警配置

## 七、部署方案

### 7.1 编译产物

```bash
# Server (包含 Local 模式)
go build -o iac-server ./backend/cmd/server

# Agent (独立部署)
go build -o iac-agent ./backend/cmd/agent
```

### 7.2 Static 模式部署

Agent 启动只需要通过环境变量配置，无需配置文件：

```bash
# 必需的环境变量
export IAC_API_ENDPOINT="https://iac-platform.example.com"  # API 端点
export IAC_AGENT_TOKEN="pool-token-xxx"                      # Agent Token (Pool Token)
export IAC_AGENT_NAME="agent-01"                             # Agent 名称

# 可选的环境变量
export IAC_POOL_ID="pool-prod"                               # Pool ID (可从 Token 自动获取)
export IAC_MAX_PLAN_CONCURRENCY="3"                          # 最大 Plan 并发数 (默认: 3)
export IAC_LOG_LEVEL="info"                                  # 日志级别 (默认: info)
export IAC_LOG_FILE="/var/log/iac-agent.log"                 # 日志文件 (默认: stdout)

# 启动 Agent
./iac-agent
```

Agent 启动流程：
1. 读取环境变量 `IAC_API_ENDPOINT`、`IAC_AGENT_TOKEN`、`IAC_AGENT_NAME`
2. 使用 Token 向 API 端点注册
3. 建立 C&C WebSocket 连接
4. 开始心跳和任务接收

### 7.3 Agent 主程序示例

```go
// backend/cmd/agent/main.go
package main

import (
    "context"
    "log"
    "os"
    "iac-platform/agent/client"
    "iac-platform/agent/control"
    "iac-platform/services"
)

func main() {
    // 1. 读取环境变量
    apiEndpoint := os.Getenv("IAC_API_ENDPOINT")
    agentToken := os.Getenv("IAC_AGENT_TOKEN")
    agentName := os.Getenv("IAC_AGENT_NAME")
    
    if apiEndpoint == "" || agentToken == "" || agentName == "" {
        log.Fatal("Required environment variables not set: IAC_API_ENDPOINT, IAC_AGENT_TOKEN, IAC_AGENT_NAME")
    }
    
    // 2. 创建 API 客户端
    apiClient := client.NewAgentAPIClient(apiEndpoint, agentToken)
    
    // 3. 注册 Agent
    agentID, poolID, err := apiClient.Register(agentName)
    if err != nil {
        log.Fatalf("Failed to register agent: %v", err)
    }
    log.Printf("Agent registered: ID=%s, Pool=%s", agentID, poolID)
    
    // 4. 创建数据访问器 (Remote 模式)
    dataAccessor := services.NewRemoteDataAccessor(apiClient)
    
    // 5. 创建流管理器
    streamManager := services.NewOutputStreamManager()
    
    // 6. 创建执行器 (复用 TerraformExecutor)
    executor := services.NewTerraformExecutor(dataAccessor, streamManager)
    
    // 7. 创建 C&C 管理器
    ccManager := control.NewCCManager(apiClient, executor, streamManager)
    ccManager.AgentID = agentID
    ccManager.PoolID = poolID
    
    // 8. 启动 C&C 通道
    if err := ccManager.Connect(); err != nil {
        log.Fatalf("Failed to connect C&C channel: %v", err)
    }
    
    // 9. 启动心跳
    go ccManager.HeartbeatLoop()
    
    // 10. 启动任务接收
    go ccManager.TaskReceiveLoop()
    
    log.Printf("Agent started successfully")
    
    // 11. 等待退出信号
    ccManager.WaitForShutdown()
}
```

**注意**: K8s 模式的部署配置已在 Agent Pool 设计文档中详细说明，请参考相关文档。

## 八、监控与运维

### 8.1 关键指标

| 指标 | 说明 | 告警阈值 |
|------|------|----------|
| agent_status | Agent 在线状态 | offline > 1min |
| task_queue_size | 待执行任务数 | > 100 |
| task_execution_time | 任务执行时长 | P95 > 30min |
| log_upload_lag | 日志上传延迟 | > 5min |
| cpu_usage | CPU 使用率 | > 80% |
| memory_usage | 内存使用率 | > 90% |

### 8.2 故障处理

#### Agent 离线
1. 检查网络连接
2. 检查 Token 是否有效
3. 查看 Agent 日志
4. 重启 Agent 服务

#### 日志丢失
1. 检查磁盘缓存目录
2. 查看 Agent 本地日志
3. 手动触发日志上传
4. 恢复历史日志

#### 任务卡住
1. 通过 C&C 发送取消命令
2. 检查 Terraform 进程
3. 清理工作目录
4. 重新执行任务

## 九、安全考虑

### 9.1 数据传输
- 所有 API 使用 HTTPS
- WebSocket 使用 WSS
- 日志数据压缩加密传输

### 9.2 资源隔离
- 每个任务独立工作目录
- 任务完成后清理敏感数据
- 不缓存 Provider/State/Code

**注意**: Agent 认证与授权机制已在 Agent Pool 设计文档中详细说明，请参考相关文档。

## 十、总结

### 10.1 核心优势
- **代码复用**: 最大化复用现有 TerraformExecutor
- **灵活部署**: 支持 Local/Static/K8s 多种模式
- **高可靠性**: 增量日志上传，数据不丢失
- **实时控制**: C&C 通道支持即时任务管理

### 10.2 关键创新
- DataAccessor 接口抽象，统一不同模式
- WebSocket C&C 通道，实现双向通信
- 增量日志上传，解决大日志问题
- K8s Token 自动管理，提高安全性

### 10.3 后续优化
- 支持更多云平台（AWS ECS, Azure Container Instances）
- 实现 Agent 自动扩缩容
- 添加任务优先级队列
- 支持断点续传和任务恢复

---

*文档版本: v1.0*  
*更新日期: 2025-10-30*  
*作者: IAC Platform Team*
