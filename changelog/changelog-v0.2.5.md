## v0.2.5

修复 Workspace Runs 页面时间过滤完全失效的问题，以及 filter count 数字与列表不一致。

### Bug Fixes

- **时间过滤失效** — DB 列为 `timestamp without time zone`，数据以本地时间(UTC+8)存储，但查询使用 UTC 时间。pgx 对 `timestamp` 列的 `discardTimeZone` 直接丢弃时区只保留数值，导致查询条件与存储值差 8 小时，Today/24h/7d/30d 等过滤按钮均返回错误结果 (`4ed8566`)
- **filter count 与列表数量不一致** — `baseCountQuery` 直接传原始字符串而非 `time.Time` 进行时间比较，且所有 filter count 查询均缺少 `is_background` 过滤条件，导致 tab 数字大于实际列表行数 (`4ed8566`)

### Improvements

- **容器时区配置** — Dockerfile 和 K8s Deployment 统一设置 `TZ=Asia/Singapore`，确保 Go 进程时区与 DSN 配置一致 (`4ed8566`)
- **filter count 查询重构** — 废弃手动拼接的 `baseCountQuery`，统一使用 `countBase()` 闭包生成基础查询，确保 workspace 过滤、is_background 过滤、搜索条件、时间范围四项条件与主查询完全对齐 (`4ed8566`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.2.4...v0.2.5
