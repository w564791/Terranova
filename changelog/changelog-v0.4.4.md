## v0.4.4

任务队列阻塞修复。

### Bug Fixes

- **decision_required 未阻塞后续任务** — `decision_required` 状态的 plan_and_apply 任务没有阻塞后续 plan_and_apply 任务的执行，可能导致并发 apply。现已加入 blocking 状态列表 (`task_queue_manager.go`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.3...v0.4.4
