## v0.3.1

v0.3.0 发布后的 Bug 修复：CMDB 同步卡死、Agent 时区偏差、失败任务 UI 显示、AI 错误分析上下文缺失。

### Bug Fixes

- **CMDB 自动同步被 CAS 锁永久阻塞** — `SyncCMDBAfterApply` 成功后不设置 `cmdb_sync_status = idle`，依赖用户打开 embedding 页面才自动转换，导致后续所有 apply 的 CMDB 同步被 CAS `WHERE cmdb_sync_status != 'syncing'` 拦截；修复：同步完成后立即设 idle，CAS 增加 10 分钟超时容错 (`task_queue_manager.go`)
- **CMDB sync 状态后台巡检** — 新增 `cleanupStaleCMDBSyncStatus`，每 ~60s 检查卡死的 syncing 状态：>5 分钟无活跃 embedding 任务重置为 idle，>10 分钟无条件重置；防止 CAS 锁永久阻塞自动同步 (`task_queue_manager.go`)
- **Agent 模式 `completed_at` 时区偏差 8 小时** — Agent Pod 缺少 `TZ=Asia/Singapore` 环境变量，`time.Now()` 返回 UTC，存入 `timestamp without time zone` 列后 wall clock 与 `created_at`/`started_at` 不一致；修复：服务端 `UpdateTaskStatus` 将 Agent 传入的 `completed_at` 转为 `time.Local`，Dockerfile 新增 `ENV TZ=Asia/Singapore` (`agent_handler.go`, `Dockerfile.agent`)
- **失败任务不显示 Plan/Apply 资源结构** — `StructuredRunOutput` 的 `loadResourceChanges` useEffect 和 `isPlanComplete()` 排除了 `failed` 状态，导致失败任务的资源变更数据不加载、结构化视图不渲染；Apply 模式也缺少 `failed` 分支 (`StructuredRunOutput.tsx`)
- **失败资源显示 "Creating..." 而非 "Failed"** — 任务到达终态后，导致失败的资源 `apply_status` 残留为 `applying`（后端 parser 无 error 行正则）；前端在加载资源数据后，对终态任务（failed/cancelled）将 `applying` 修正为 `failed` (`StructuredRunOutput.tsx`)
- **AI 错误分析缺少资源变更上下文** — prompt 仅包含 `error_message`，AI 无法得知哪些资源已创建、哪个资源失败；修复：查询 `workspace_task_resource_changes` 表，在 prompt 中追加 Apply 执行结果（失败资源 + 已成功资源列表） (`ai_analysis_service.go`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.3.0...v0.3.1
