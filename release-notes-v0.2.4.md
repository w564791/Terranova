## v0.2.4

Local 模式稳定性修复，解决多副本部署下 apply 阶段的三个 Bug 及竞态条件，补齐 Local 模式与 Agent 模式的功能对齐。

### Bug Fixes

- **terraform apply "Inconsistent dependency lock file" 错误** — `ExecuteApply` 在新工作目录下缺少 `restoreTerraformLockHCL` 调用，多副本调度时 provider 版本不一致导致 apply 失败 (`9067ba4`, `ca8fefc`)
- **apply 完成后 `workspace.tf_state` 未更新** — `ExecuteApply` 未从 DB 复制 `workspace.ID`，GORM 以 `WHERE id=0` 更新 0 行，state 静默丢失；同时将 `GetWorkspace` 失败改为 fail fast，避免后续写入无效 (`ca8fefc`)
- **多副本任务重复执行** — advisory lock 在 `executeTask` goroutine 启动前即释放，其他 pod 可在窗口期重复拾取同一任务；改为 DB 级 CAS 原子标记 `running` 后再启动 goroutine (`ca8fefc`)
- **时间戳 UTC 偏差** — `timePtr()` 移除多余的 `.UTC()` 转换，修复前端显示时间偏移问题 (`9067ba4`)

### Improvements

- **Local 模式 Apply 通知对齐** — Local 模式执行 plan/apply 时发送 `task_planning`/`task_applying` 通知，与 Agent 模式行为一致 (`9067ba4`)
- **Local 模式 Drift 清除对齐** — apply 成功后自动清除 workspace 的 drift 标记（非 `--target` 场景），与 Agent 模式逻辑一致 (`9067ba4`)
- **多副本实时日志转发** — 通过 PG NOTIFY/LISTEN 将日志事件跨副本转发，解决 APIGateway 部署下前端无法收到实时日志的问题 (`9067ba4`)
- **executeTask panic recovery** — goroutine 内 panic 时自动将 task 标记为 `failed`，防止任务永久卡在 `running` 状态 (`ca8fefc`)
- **JSONB 快照变量解析修复** — `ResolveVariableSnapshots` 正确处理 JSONB `_array` 包装，避免变量数据解析失败 (`9067ba4`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.2.3...v0.2.4
