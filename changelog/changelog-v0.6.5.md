## v0.6.5

Provider 实例化重构：同一模板可在同一 Workspace 多次实例化（不同 alias/overrides）、alias 从模板级下沉到实例级、三种 Provider 配置模式互斥（template / custom / none）、数据自动迁移。

### Breaking Changes

#### Provider 配置模型重构（provider_template_ids → provider_instances）

- **重构** Workspace Provider 配置从「模板 ID 列表 + 按 ID 覆盖」改为「实例数组」模型
  - 旧模型：`provider_template_ids: [1, 2]` + `provider_overrides: {"1": {"alias": "west", "region": "us-west-2"}}`
  - 新模型：`provider_instances: [{template_id: 1, alias: "west", overrides: {"region": "us-west-2"}}, ...]`
  - 同一模板可在同一 workspace 多次实例化，每个实例独立设置 alias 和 overrides（支持多 region 等场景）
- **移除** `provider_templates` 表的 `alias` 列：alias 不再属于模板级属性，改为 workspace 实例级配置
- **移除** Workspace 的 `provider_template_ids` 和 `provider_overrides` 列，合并为 `provider_instances` JSONB 列
- **迁移** `backend/migrations/add_provider_instances.sql` 提供幂等数据迁移：
  - 自动将旧 `provider_template_ids` + `provider_overrides`（含模板级 alias）合并为 `provider_instances`
  - 迁移完成后自动 DROP 旧列
- **API 变更**：
  - `GET /workspaces/:id` 返回 `provider_instances` 数组，不再返回 `provider_template_ids` / `provider_overrides`
  - `POST /workspaces` 接受 `provider_instances` 字段，与 `provider_config` 互斥（同时提交返回 400）
  - `PATCH /workspaces/:id` 支持三种模式互斥切换：
    - `template` → 提交 `provider_instances: [{...}]`
    - `custom` → 提交 `provider_config: {...}`（自动清空 `provider_instances`）
    - `none` → 提交 `provider_instances: []`（空数组触发清空 `provider_config`）

### Enhancements

#### Provider 实例管理（后端）

- **新增** `ProviderInstance` 模型：`{template_id, alias, overrides}`
- **新增** `ValidateInstanceAliases`：按 provider type 校验 alias 唯一性
  - 同一 type 下最多一个无 alias 的默认 provider
  - 其余实例 alias 不能重名
  - 创建和更新 workspace 时均执行校验
- **重构** `ResolveProviderConfig` → `ResolveProviderConfigFromInstances`：
  - 从实例数组解析 provider 配置，输出 `provider.tf.json` 兼容格式
  - 实例级 overrides 优先级最高，覆盖模板默认值
  - 引用已删除模板的实例自动跳过
- **更新** `CheckTemplateInUse`：使用 `jsonb_build_array` + `jsonb_build_object` 构造查询条件，避免 Go 侧 marshal 类型歧义
- **更新** `WorkspaceListItem` 新增 `provider_instances` 字段，列表页也能获取实例信息
- **更新** 所有 provider 配置解析入口统一切换到 `GetProviderInstances()` + `ResolveProviderConfigFromInstances()`：
  - `workspace_controller.go`（GET/POST/PATCH）
  - `workspace_task_controller.go`（任务快照）
  - `agent_handler.go`（Agent 获取任务数据）
  - `manifest_handler.go`（Manifest 任务快照）
  - `terraform_executor.go`（本地执行 + 资源版本快照）

#### Provider 实例管理（前端）

- **重构** `ProviderSettings.tsx` 从「模板勾选列表」改为「实例卡片列表」：
  - 新增 "+ Add Provider" 下拉按钮，按 type 分组展示可选模板
  - 点击模板添加为实例，同一模板可多次添加
  - 每个实例卡片独立编辑 alias 和 overrides
  - 每个实例卡片支持 Remove 删除
  - 空状态提示优化，引导用户添加实例
- **新增** 实例相关 CSS 样式（`ProviderSettings.module.css`）：
  - `addProviderRow` / `addProviderButton` / `addProviderDropdown`：添加按钮和下拉菜单
  - `instanceList` / `instanceCard` / `instanceHeader` / `instanceTitle`：实例卡片布局
  - `typeBadge` / `removeInstanceButton`：类型标签和移除按钮
- **更新** `ProviderInstance` 类型定义（`frontend/src/services/workspaces.ts`）
- **更新** `ProviderTemplate` 类型定义移除 `alias` 字段（`frontend/src/services/admin.ts`）

#### Provider 模板管理（前端）

- **移除** `ProviderTemplatesAdmin.tsx` 中的 alias 表单字段和表格列
- **移除** 模板创建/更新请求中的 `alias` 字段
- **说明** alias 现归属 workspace 实例级，模板仅作为 blueprint

### Database Migration

- `workspaces` 表：
  - 新增 `provider_instances jsonb` 列
  - 移除 `provider_template_ids jsonb` 列
  - 移除 `provider_overrides jsonb` 列
- `provider_templates` 表：
  - 移除 `alias varchar(50)` 列
- 迁移脚本 `backend/migrations/add_provider_instances.sql` 提供幂等数据搬迁
