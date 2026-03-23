## v0.4.5

风险决策 ABORT 支持。

### New Features

- **ABORT 决策自动取消任务** — 用户在风险决策中选择"终止本次变更"时，任务自动取消并解锁 workspace，自动写入 comment 记录 (`ai_summary_controller.go`)

### Bug Fixes

- **SECURITY_GROUP_CHANGE 缺少 ABORT 选项** — 四个决策场景现在都包含"终止本次变更"选项 (`execute_summary_workflow.md`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.4...v0.4.5
