## v0.2.6

修复执行流程中的 6 个 CRITICAL 和关键 HIGH 问题，涵盖临界区安全、RunTask 全生命周期认证、取消路径资源泄漏、缺失 RunTask 阶段等。

### Critical Fixes

- **C1: Apply 临界区改用 defer 自动退出** — `EnterCriticalSection("applying")` 后紧跟 `defer ExitCriticalSection`，确保所有 error/cancel/panic 路径均自动释放临界区，消除死锁风险 (`terraform_executor.go`)
- **C2: 任务取消路径资源泄漏** — `saveTaskCancellation` 对齐 `saveTaskFailure` 的完整清理逻辑：apply 取消时解锁 workspace、保存 partial state、清理工作目录；plan 取消时清理工作目录；发送 `task_cancelled` 通知 (`terraform_executor.go`)
- **C3: Webhook 全部失败时 mandatory 检查缺失** — 重构 `ExecuteRunTasksForStage`，使用单一 channel 收集结果并保留 enforcement 信息，webhook 发送失败时检查对应 RunTask 是否为 mandatory，mandatory 失败立即阻断执行 (`run_task_executor.go`)
- **C4: RunTask Callback 端点无认证** — Callback 端点新增 Bearer token 验证，从 Authorization header 提取 JWT，校验签名和有效期，并验证 token 中的 `result_id` 与路径参数一致 (`run_task_callback_handler.go`)
- **C5: RunTask Access Token 生成与加密存储** — `RunTaskExecutor` 集成 `RunTaskTokenService`，在创建 `RunTaskResult` 后生成 JWT access token（HMAC-SHA256，有效期取 RunTask timeout，最长 24h），使用 AES-256-GCM (`crypto.EncryptValue`) 加密后存入数据库，webhook payload 中解密传输 (`run_task_executor.go`)
- **C6: 缺失 pre_plan / pre_apply / post_apply 三个 RunTask 阶段** — 在 `ExecutePlan` init 完成后插入 `pre_plan` 阶段，在 `ExecuteApply` plan 恢复后插入 `pre_apply` 阶段，在 apply 成功后插入 `post_apply` 阶段（post_apply mandatory 失败仅记录警告，不回滚已完成的 apply）(`terraform_executor.go`)

### Bug Fixes

- **H1: Plan/Apply 早期错误 task 卡在 running** — `GetWorkspace` 失败时 logger 尚未创建，改为直接 `UpdateTask` 设置 failed 状态；`PrepareWorkspace` 失败时调用 `saveTaskFailure`，确保 task 不会永久停留在 running 状态 (`terraform_executor.go`)
- **H3: Mandatory RunTask 失败日志不完整** — post-plan mandatory 失败从直接 `UpdateTask` 改为调用 `saveTaskFailure`，确保完整的日志保存和清理逻辑 (`terraform_executor.go`)
- **H4: Apply 输出 Goroutine 缺少 panic recovery** — stdout/stderr reader goroutine 添加 `defer recover()`，防止异常日志行导致整个 apply 流程 panic (`terraform_executor.go`)
- **H7: Workspace 锁定失败仅 warn 未阻断** — Plan 完成后 workspace 锁定失败改为调用 `saveTaskFailure` 并返回 error，防止在 workspace 未锁定状态下进入 apply (`terraform_executor.go`)
- **Workspace 锁定/解锁列名错误** — `LocalDataAccessor.LockWorkspace` 和 `UnlockWorkspace` 使用了不存在的列名 `"locked"`，修正为 `"is_locked"` 匹配 GORM model 定义和实际 DB schema；同时修复 `locked_at` 从字符串 `"NOW()"` 改为 `time.Now()`，`lock_reason` 解锁时从 `nil` 改为空字符串 (`local_data_accessor.go`)
- **RunTask 取消后仍阻塞等待 callback** — `TaskQueueManager` 新增 `taskCancels` 注册表，`executeTask` 启动时注册 context cancel 函数；`CancelTask` 控制器调用 `CancelTaskExecution` 取消 context；`waitForCallbacks` 改用 `select` + ticker 替代 `time.Sleep`，context 取消时立即退出并将 pending/running 的 `RunTaskResult` 标记为 error (`task_queue_manager.go`, `workspace_task_controller.go`, `run_task_executor.go`)

### Improvements

- **RunTask Token 生命周期管理** — callback 完成（passed/failed）后清除 token；超时后清除 token；任务取消后清除 token；token 使用 `JWT_SECRET` 签名，加密存储在数据库 (`run_task_executor.go`, `run_task_timeout_checker.go`)
- **RunTask 公开数据端点** — 新增 token 认证的 `GET /api/v1/run-task-results/:result_id/plan-json` 和 `GET /api/v1/run-task-results/:result_id/resource-changes`，外部 RunTask 服务无需平台 JWT 即可通过 access token 获取 plan 数据；webhook payload 中的 URL 指向新端点 (`router_run_task.go`, `run_task_callback_handler.go`)
- **RunTask 取消时 context 传播** — 所有 RunTask 阶段（pre_plan/post_plan/pre_apply）的错误处理中增加 `ctx.Err() == context.Canceled` 检查，取消时调用 `saveTaskCancellation` 而非 `saveTaskFailure` (`terraform_executor.go`)
- **Dockerfile 基础镜像固定版本** — `amazonlinux:latest` 改为 `amazonlinux:2023`，避免基础镜像意外升级带来的兼容性问题 (`Dockerfile`, `Dockerfile.server`)
- **runtask-test-platform URL 重写** — 测试平台新增 `CALLBACK_BASE_URL` 环境变量，自动将 webhook payload 中的 callback/data URL 的 host 部分替换为本地可达地址，解决本地测试平台无法访问 K8s 内部地址的问题 (`cmd/runtask-test-platform/main.go`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.2.5...v0.2.6
