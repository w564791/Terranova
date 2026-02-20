

IAC Agent 设计文档 (v3.2 · C&C 模式)
一、总体目标
本文档定义了 IAC Agent v3.2 的设计。Agent 是平台的 Terraform 执行单元。
核心特性：

- Agent 主动模式：所有连接均由 Agent 主动发起 (Outbound)，完美适配防火墙内部署。
- C&C 通道：Agent 启动后，建立一个永久的 C&C WebSocket (命令与控制)，用于状态上报（替代 Ping）和接收 Server 的即时命令。
- 并发控制：plan 任务可并发，plan+apply 任务独占 Agent。
- 双日志模型：
 - 持久化日志：在 plan 或 apply 阶段完成后，Agent 通过 HTTP POST 上传完整日志文件，用于归档和回放。
 - 实时日志：易挥发的日志流，仅在有客户端查看时按需建立，不用于存储。
   二、架构概览
   Client (Web UI)
   │
   │ 1. Client WS (订阅日志)
   ▼
   ┌──────────────────────────┐
   │        IAC Server        │
   │  ┌────────────────────┐  │
   │  │ API (HTTP/S)        │  │
   │  │ Client Log Hub      │  │
   │  │ Agent C&C Hub (WSS) │  │
   │  │ Agent Stream Hub(WSS)│  │
   │  └────────────────────┘  │
   └─────────────┬───┬───┬────┘
   │   │   │

1. C&C (WSS)│   │   │ 3. Log Stream (WSS, 按需)
  (永久, Agent->Srv)│   │   (易挥发, Agent->Srv)
  │   │
  │   │ 4. API (HTTPS, Agent->Srv)
  │   │   (注册, 上传持久化日志)
  ▼   ▼
  ┌──────────────────────────┐
  │        IAC Agent         │
  │  ┌────────────────────┐  │
  │  │ C&C Manager         │<─ (核心)
  │  │ TaskScheduler       │  │
  │  │ WorkerPool (Plan)   │  │
  │  │ Worker (Apply)      │  │
  │  │ LogStreamer (OnDemand)│  │
  │  └────────────────────┘  │
  └──────────────────────────┘

三、任务类型与并发控制
Agent 在 C&C 通道上报的状态，将包含其并发能力。

- Agent 状态 (IV):
 type AgentState struct {
 PlanRunningCount int       // plan 并发计数
 PlanLimit        int       // plan 最大并发数
 ApplyRunning     bool      // plan+apply 是否占用
 }
- 并发规则：
 - ApplyRunning == true 时，Agent 不接受任何新任务。
 - ApplyRunning == false 时：
   - PlanRunningCount < PlanLimit：Agent 可接受 plan 任务。
   - PlanRunningCount == 0：Agent 可接受 plan+apply 任务。
- Server 调度：Server 根据 Agent 上报的状态，决定是否通过 C&C 通道下发任务。
 四、核心工作流程（C&C 模式）

1. Agent 启动与 C&C 建立

- Agent 启动，调用 POST /agents/register 注册。
- Agent 立即建立一个永久的 C&C WebSocket (WS 1) 连接到 Server (ws://…/agent/control)。
- 此 WS 1 用于：
 - 心跳：Agent 周期性通过 WS 1 上报 AgentState（取代 HTTP Ping）。
 - 命令接收：Agent 监听来自 Server 的命令（如 run_task, start_stream）。
- Agent 必须实现此 WS 1 的断线自动重连。

1. 任务下发（Server Push）

- Server 有一个 plan 任务 (T-001) 待执行。
- Server 查找一个空闲的 Agent (例如 Agent-A，其上报状态为 PlanRunningCount < PlanLimit)。
- Server 主动通过 Agent-A 的 C&C WS (WS 1) 连接发送命令：
 {
 “command”: “run_task”,
 “task_data”: {
 “task_id”: “T-001”,
 “action”: “plan”,
 “tf_json”: “…”,
 “variables”: “…”
 }
 }
- Agent 的 C&C Manager 收到命令，交给 TaskScheduler 开始执行 T-001。

1. 任务执行与持久化日志（阶段性）

- Agent 在独立目录中执行 T-001 (plan)。
- Agent 将所有 stdout 和 stderr 同时重定向到：
 - 一个本地文件（例如 T-001.log）。
 - （可选）一个内部的 stdout 管道，为实时查看做准备。
- plan 任务执行完毕。
- Agent 此时才通过 HTTP POST 上传日志文件：
 POST /api/tasks/T-001/logs/upload (Body: T-001.log)
- Server 收到日志，将其持久化到数据库或对象存储。
- Agent 在 C&C (WS 1) 上报 T-001 任务完成，并更新自身状态。
 五、日志流程：实时查看（按需）
 此流程是完全独立的，与持久化日志（四.3）无关。

1. 客户端发起订阅

- T-001 正在 Agent-A 上运行。
- 用户在 UI 上点击“实时查看”。
- Client 通过客户端 WS (WS 2) 连接 Server，请求订阅 T-001。

1. Server 通知 Agent (C&C)

- Server 发现 T-001 正在 Agent-A 上运行。
- Server 立即通过 Agent-A 的 C&C 通道 (WS 1) 发送命令：
 { “command”: “start_realtime_stream”, “task_id”: “T-001” }

1. Agent 建立实时通道 (按需 WSS)

- Agent-A 的 C&C Manager 收到命令。
- Agent-A 立即启动 LogStreamer。
- LogStreamer 建立一个新的、临时的 WebSocket (WS 3)，连接到 Server：
 ws://…/agent/stream/T-001
- Agent-A 开始将 T-001 进程的 stdout/stderr 实时读出，并逐行发送到 WS 3。
 - (这就是您说的“go routine 阻塞在那里，有查看需求才建立链接”)

1. Server 中继

- Server 的 Agent Stream Hub 收到来自 WS 3 的日志行。
- Server 不存储这些日志。
- Server 立即将这些日志行广播给所有通过 WS 2 订阅了 T-001 的 Client。

1. 订阅结束

- 所有 Client 都关闭了 T-001 的查看窗口（WS 2 断开）。
- Server 发现 T-001 的订阅者变为 0。
- Server 通过 C&C 通道 (WS 1) 发送命令：
 { “command”: “stop_realtime_stream”, “task_id”: “T-001” }
- Agent-A 收到命令，关闭临时的 WS 3，LogStreamer 停止工作。
- 注意：这不影响正在写入本地的持久化日志文件 (T-001.log)。
 六、总结
 此 v3.2 方案满足您的所有要求：
- Agent 主动：所有连接 (C&C WS 1, Log WS 3, HTTP POST) 均由 Agent 发起。
- 即时通信：Server 可通过 C&C (WS 1) 随时向 Agent 下发命令。
- 日志解耦：
 - 持久化：通过 HTTP POST 在阶段完成后上传，100% 可靠。
 - 实时性：通过 WS 3 按需建立，易挥发，不存储，零消耗（无人查看时）。
- 无新增组件：架构中没有引入 Redis/Kafka。
 新增了一些内容

