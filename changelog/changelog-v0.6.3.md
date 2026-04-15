## v0.6.3

Task Queue 槽位状态管理修复、跨副本 Agent Metrics 广播、Risk Scorer 误报优化、K8s Pod 扩缩容与竞态保护。

### Bug Fixes

#### Task Queue 槽位状态管理

- **修复** `apply_pending` 任务（plan 完成等待 apply）完成后 slot 被错误释放：现在根据任务状态选择 `ReserveSlotForApplyPending` 或 `ReleaseTaskSlot`
- **修复** apply 执行时 reserved 槽位未转回 running 状态：`pushTaskToAgent` 中正确 transition slot status
- **修复** 多副本场景下 `ReserveSlotForApplyPending` 在内存中找不到 slot 时直接返回：现在通过 DB 查询 agent/pod 映射重建 reserved slot

#### K8s Pod 扩缩容

- **修复** `AutoScalePods` 仅统计 running 槽位，遗漏 reserved 槽位占用：现在将 running 和 reserved 均视为 occupied，确保 apply_pending 任务也能触发扩容

#### K8s Pod 竞态保护

- **修复** `syncTaskStatusToSlots` 在 DB 查询与 slot 分配之间存在竞态，可能误释放刚分配的 slot：记录查询时间点，仅释放查询前已存在的 slot

### Enhancements

#### Agent Metrics 跨副本广播

- **新增** 通过 PG NOTIFY 实现跨副本 metrics 广播，支持多副本部署场景下前端实时接收任意副本的 agent metrics
- **优化** WebSocket 认证方式：使用 `Sec-WebSocket-Protocol` 传递 token，与项目中其他 WebSocket 连接保持一致

#### Risk Scorer (AI Summary)

- **优化** `hasCMDBNotFound` 区分 corrective change 与真实风险：当 `query_resource_attributes` 返回 found=false 时，结合 `query_resource_code_diff` 判断查询目标是否仅在旧代码中出现（修 typo 等修复性变更），避免误触发 R2
