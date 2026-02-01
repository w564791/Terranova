IAC Agent + Server 方案文档（Markdown），保证：
•	Agent 模式 Pull 执行任务
•	plan / plan+apply 并发控制
•	日志上传始终发生
•	WebSocket 仅作为实时查看窗口
•	无客户端订阅时 Agent 仍上传日志

⸻

IAC Agent 设计文档（Pull 模式 · Agent 模式 · v3.1）

一、总体目标

IAC Agent 是平台的 Terraform 执行单元，特点：
•	Pull 模式：Agent 从平台 API 拉取任务执行；
•	任务类型区分：plan / plan+apply，并发受限；
•	日志上传：任务执行产生的日志始终上传至 Server 并持久化；
•	实时查看：WebSocket 仅提供实时日志查看功能，无订阅时不影响日志存储；
•	高安全性：JWT 认证、任务隔离、最小暴露。

⸻

二、架构概览

Client (Web UI / CLI)
│
│ WebSocket / HTTP (实时查看日志)
▼
┌──────────────────────────┐
│        IAC Server        │
│  ┌────────────────────┐  │
│  │ Log Hub / Router    │  │
│  │ Task API / Ping     │  │
│  └────────────────────┘  │
└─────────────┬────────────┘
│ HTTPS + JWT
▼
┌──────────────────────────┐
│        IAC Agent         │
│  ┌────────────────────┐  │
│  │ PingManager         │  │
│  │ TaskScheduler       │  │
│  │ PlanWorkerPool      │  │
│  │ ApplyWorker         │  │
│  │ LogStreamer         │  │
│  │ StateManager        │  │
│  └────────────────────┘  │
└──────────────────────────┘

⸻

三、任务类型与并发控制

任务类型	并发规则
plan	并行执行，数量受 Agent Pool max_plan_concurrency 限制
plan+apply	同时只能执行 1 个 Agent 内任务

Agent Pool 配置示例

{
“pool_id”: “default-pool”,
“max_plan_concurrency”: 3,
“allow_apply_concurrent”: false,
“labels”: {“region”: “idc1”}
}

⸻

四、Agent 状态模型

type AgentState struct {
Busy             bool      // 是否有任务在执行
PlanRunningCount int       // plan 并发计数
PlanLimit        int       // plan 最大并发数
ApplyRunning     bool      // plan+apply 是否占用
CurrentTasks     []string  // 当前任务 ID 列表
}

⸻

五、核心工作流程

1. 初始化
  •	启动 Agent 并读取配置（API 地址、JWT、Agent 名称）
  •	调用 /agents/register 注册信息
  •	初始化：
  •	PingManager（定时心跳）
  •	TaskScheduler（任务拉取与执行）
  •	LogStreamer（日志上传/实时推送）
1. 心跳 /ping
  •	Agent 周期性上报状态：

{
“agent_id”: “idc-agent-01”,
“busy”: true,
“plan_tasks_running”: 2,
“plan_task_limit”: 3,
“apply_task_running”: true,
“current_tasks”: [“plan-t-01”, “plan-t-02”, “apply-t-03”],
“cpu_usage”: 0.41,
“mem_usage”: 0.52,
“version”: “1.0.4”
}

```
•	平台根据心跳判断 Agent 是否空闲、健康及任务调度能力。
```

1. 任务拉取 /tasks/pull
  •	周期性拉取任务：
  •	如果 apply 任务在执行中，则不拉取任何任务。
  •	plan 任务未达到并发上限，则继续拉取 plan 任务。
  •	Task JSON 示例：

{
“task_id”: “t-001”,
“workspace_id”: “prod-ws”,
“action”: “plan_apply”,
“terraform_version”: “1.8.5”,
“tf_json”: “{…}”,
“variables”: “{…}”,
“state_json”: “{…}”
}

1. 执行阶段
  •	独立临时目录执行 Terraform
  •	调用 Terraform：

terraform init
terraform plan -out=plan.out
terraform apply -auto-approve plan.out

```
•	plan+apply 任务独占 ApplyWorker，plan 任务使用 PlanWorkerPool 并行执行
•	执行状态通过 /ping 上报 busy=true，执行完毕更新为 busy=false
```

⸻

六、日志上传与实时查看

1. Agent → Server 日志上传
  •	Agent 必须上传执行日志（持久化），确保无客户端订阅时仍保留完整日志
  •	WebSocket 用于实时推送：
  •	URL: /ws/agent/logs?task_id=xxx&stream_mode=auto
  •	stream_mode=auto：
  •	Server 有订阅者 → 实时中继给客户端
  •	Server 无订阅者 → 仍上传日志，但可减少实时 WebSocket 传输压力
  •	日志消息示例：

{
“task_id”: “t-001”,
“timestamp”: “2025-10-30T09:12:33.123Z”,
“level”: “info”,
“line”: “Terraform plan start …”
}

1. Server → Client
  •	客户端订阅日志 /ws/logs?task_id=xxx
  •	Server 将 agent 上传的日志广播给订阅客户端
  •	实时查看窗口不影响日志存储和任务完整性
1. 日志持久化与回放
  •	Server 将日志存储到数据库、Redis 或对象存储
  •	客户端可随时回放历史日志
  •	支持断线续传与 offset 机制

⸻

七、无客户端订阅处理策略
•	Agent 始终上传日志（持久化）
•	WebSocket 仅用于实时查看
•	无客户端订阅时：
•	Server 可控制 WebSocket 数据流降低压力
•	任务日志仍完整存储，保证可回放

⸻

八、安全与隔离

项目	机制
认证	JWT Token 鉴权
隔离	每任务独立执行目录
日志权限	客户端需有 workspace 权限
网络	Agent 主动发起请求，无入站暴露

⸻

九、扩展方向

功能	描述
多租户隔离	不同 org/tenant 任务独立 Agent
并发自适应	动态调整 plan 并发上限
多副本 Server	使用 Redis/Kafka 做日志广播
任务中断	Server 可通过 /ping 下发中断指令
历史日志回放	从缓存或持久化存储获取

⸻

十、流程总结（时序）

Agent 启动 → 注册 → 心跳(Ping) → 拉取任务(Pull)
→ 执行计划(Terraform) → 日志上传(持久化) + WebSocket 中继
→ Server 广播给客户端（如有订阅） → 任务完成 → 状态更新(Busy=false)

帮我查看 agent 的设计方案有没有需要优化的地方


