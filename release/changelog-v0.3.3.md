## v0.3.3

Provider 全局模板功能：支持全局 Provider 模板管理、多同类型 Provider、Alias、动态配置解析、Workspace 级别 Override。

### Features

- **Provider 全局模板管理** — 新增 Provider Templates CRUD，支持模板的创建、编辑、删除、设为默认；管理入口作为独立 Global Settings 导航项 (`provider_template_controller.go`, `ProviderTemplatesAdmin.tsx`)
- **多同类型 Provider 支持** — 同一 Workspace 可引用多个相同类型的 Provider（如多个 AWS），通过 Alias 区分；`ResolveProviderConfig` 以 append 方式合并同类型 Provider 而非覆盖 (`provider_template_service.go`)
- **Provider Alias 字段** — Provider 模板新增可选 Alias 字段，用于同类型多 Provider 场景下的区分标识；端到端实现包含模型、API、前端表单及展示 (`provider_template.go`, `ProviderTemplatesAdmin.tsx`, `ProviderSettings.tsx`)
- **Workspace Provider 模板选择与 Override** — Workspace 设置页支持从全局模板中选择 Provider，并可对每个模板的配置进行 Workspace 级别的 Override；Override 仅影响当前 Workspace (`ProviderSettings.tsx`)
- **Config JSON 编辑器** — Provider 配置字段替换为 JSON 编辑器，支持语法高亮、格式化、错误提示 (`ProviderTemplatesAdmin.tsx`, `ProviderSettings.tsx`)

### Enhancements

- **动态 Provider 配置解析** — Workspace 不再存储已解析的 `provider_config` 快照，改为在读取时从全局模板动态解析；Task 在创建时获取当时最新的模板配置作为快照，确保任务执行一致性 (`workspace_controller.go`, `workspace_task_controller.go`, `terraform_executor.go`, `manifest_handler.go`, `agent_handler.go`)
- **JSONB 辅助方法** — 新增 `GetTemplateIDs()`、`GetOverridesMap()` 方法简化模板 ID 和 Override 数据的提取 (`workspace.go`)
- **Override 按模板 ID 匹配** — Override 查找从按 type 匹配改为按 template ID 匹配，避免同类型多 Provider 时 Override 混淆 (`provider_template_service.go`)
- **版本约束"不限制"选项** — Provider 模板版本约束支持"不限制"，清除约束后使用最新可用版本 (`ProviderTemplatesAdmin.tsx`)
- **敏感字段过滤** — Provider API 响应中深度过滤敏感字段，防止密钥泄露 (`provider_template_controller.go`)
- **Override 默认折叠** — Workspace Provider 设置中 Override 配置区域默认折叠，减少视觉干扰 (`ProviderSettings.tsx`)

### Bug Fixes

- **JSONB 序列化修复** — 修复 `provider_template_ids` 存储为 `{}` 而非 `[1]` 的问题，GORM map 更新使用 `gorm.Expr("?::jsonb", ...)` 确保正确类型转换 (`workspace_controller.go`)
- **K8s Agent 模式 Provider 配置** — 修复 K8s Agent 获取任务数据时未动态解析 Provider 配置的问题，Agent 此前读取的是数据库中已清空的 `provider_config` 字段 (`agent_handler.go`)
- **JSONB Scan 兼容性** — 修复 JSONB 对 JSON null 和标量值的 Scan 处理，防止任务查询失败 (`workspace.go`)
- **非字符串 Override 值显示** — 修复 Override 配置中对象/数组值显示为 `[object Object]` 的问题 (`ProviderSettings.tsx`)

### Migration

- 新增 `provider_templates` 表及 Workspace 关联字段 (`add_provider_templates.sql`)
- 新增 `alias` 字段迁移 (`add_provider_template_alias.sql`)
- 更新种子数据 (`init_seed_data.sql`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.3.2...v0.3.3
