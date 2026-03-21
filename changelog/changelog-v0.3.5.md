## v0.3.5

Schema 版本控制重构与嵌套字段布局持久化修复。

### Refactor

- **废弃 active_schema_id** — 移除 `ModuleVersion.active_schema_id` 指针机制，改用 latest-wins 策略（`ORDER BY updated_at DESC LIMIT 1`）；新增 `schema_helpers.go` 提供 `GetLatestSchema` / `GetLatestSchemaV2` 统一解析函数，所有消费者（GetSchemaV2、Skill 生成、版本对比、继承等）统一调用 (`schema_helpers.go`, `module_schema_v2_handler.go`, `module_version_service.go`, `module_version_skill_service.go`)
- **Schema 版本号 per-ModuleVersion 作用域** — `GetNextSchemaVersionForModuleVersion` 按 `module_version_id` 独立计数，避免不同 ModuleVersion 之间版本号碰撞 (`schema_helpers.go`)
- **SetActiveSchema 端点废弃** — 返回 `410 Gone`，系统自动使用最新 Schema (`module_schema_v2_handler.go`)
- **前端移除 active_schema_id 依赖** — SchemaManagement 使用 `sortedSchemas[0]` 选择活跃 Schema；CreateDemo 移除 `active_schema_id` 检查；AddResources 简化 Schema 选择逻辑 (`SchemaManagement.tsx`, `CreateDemo.tsx`, `AddResources.tsx`)

### Bug Fixes

- **嵌套字段布局刷新后丢失** — 根因：Go `json.Marshal(map[string]interface{})` 按字母序排列 key，导致嵌套字段的 JSON key 顺序在后端往返后改变；虽然 `x-colSpan` 值正确持久化，但字段顺序变化导致行分配不同，视觉上表现为"布局丢失"。修复：拖拽布局时在每个嵌套字段写入 `x-order` 属性记录用户排列顺序，渲染时按 `x-order` 排序而非依赖 JSON key 顺序 (`OpenAPISchemaEditor/index.tsx`, `ObjectListWidget.tsx`, `ObjectWidget.tsx`, `DynamicObjectWidget.tsx`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.3.4...v0.3.5
