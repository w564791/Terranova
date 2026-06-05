## v0.7.0

Manifest 编辑器重构：从「拖拽画布」彻底迁移到 VS Code Web 工作区。文件以草稿/发布快照统一存储于 manifest_files，编辑体验对齐 VS Code（文件树、多 tab、跨文件搜索替换、HCL 语法高亮/诊断/补全/转到定义、版本 diff）。部署模型简化为纯元信息的 install/upgrade/uninstall，deployment 可关联 varset 与应急 variable_overrides 并真正进入 plan/apply。workspace 与 manifest 通过软链接字段关联，执行支持 subpath 子目录。同时引入 Apply 资源 ID 实时逐个回填、Structured 视图 after_unknown 修复等执行体验改进。

### Breaking Changes

#### Manifest 存储与版本模型重构（画布 → 文件快照）

- **重构** Manifest 内容存储从画布字段（`canvas_data` / `nodes` / `edges` / `hcl_content`）改为文件级存储
  - 新增 `manifest_files` 表：草稿与发布快照统一存放
    - 草稿区：`version_id IS NULL AND owner_user_id 非空`（用户私有草稿）
    - 发布快照：`version_id 非空 AND owner_user_id IS NULL`（不可变快照）
  - 部分唯一索引规避 PostgreSQL UNIQUE 多 NULL 陷阱（`uq_mf_draft` / `uq_mf_published`）
- **移除** `manifest_versions` 表的画布字段：`canvas_data` / `nodes` / `edges` / `hcl_content` / `is_draft`
  - 版本行降级为「文件快照的元信息行」，文件内容统一查 `manifest_files`
  - 删除旧 `version='draft'` 行（草稿改由 `manifest_files` 承载）
- **弃用** `manifest_deployment_resources` 表：新模型部署为纯元信息操作，不再写节点→资源映射
  - 资源到 module 的映射改由 `workspace_resources.tf_code` / manifest 文件浅解析得出
  - 本期仅加 DEPRECATED 注释，不 drop，保留历史数据

#### Manifest 部署模型与 workspace 关联

- **重构** `manifest_deployments.workspace_id` 类型从 `int` 改为 `varchar(50)`，对齐全平台语义化 ID（ws-xxx）
  - 迁移自动 backfill：旧 int FK（指向 `workspaces.id`）翻译为 `workspaces.workspace_id`
  - drop 旧 FK，改走业务层校验
- **新增** workspace 软链接字段：`manifest_deployment_id` / `manifest_active_tag` / `manifest_subpath`
  - `manifest_deployment_id` + `manifest_active_tag` 同生同死（CHECK 约束保证一致性）
  - `manifest_subpath`：terraform 执行根目录，空 = manifest 根
- **新增** 单 manifest 约束：同一 workspace 仅允许一条 `status='active'` 的 deployment（部分唯一索引）
- **API 变更**：Manifest v2 路由整体迁移
  - 文件操作：`GET/PUT/DELETE /organizations/:org_id/manifests/:id/files/*path`、`_move` / `_move_dir` / `_delete_dir`
  - 草稿操作：`/draft/_reset_from`（还原到指定版本）、`/draft/_export`
  - 版本：`GET/POST /v2/versions`、`/v2/versions/:id/diff`、`/v2/versions/:id/workdirs`、`/v2/draft/diff`
  - 部署：`/v2/deployments/install`、`:id/upgrade`、`:id/uninstall`、`:id/variable-preview`
  - 关联查询：`/variable-sets/:varset_id/manifest-deployments`、`/workspaces/:id/manifest-summary`
  - module 辅助：`/manifest-editor/modules`、`/modules/:id/demos`、`/modules/:id/inputs`

### Enhancements

#### Manifest VS Code Web 编辑器（前端）

- **新增** `ManifestEditorV2` 工作区，替代旧 `ManifestEditor` / `ManifestCreate` / `ManifestDeploy`（React Flow 画布）
- **新增** 文件树：VS Code 风格内联新建/重命名、目录 CRUD、删除内联确认、右键菜单（关闭其他/左右/已保存/全部）、拖拽
- **新增** 编辑器体验：多 tab、dirty 脏标记（挂 model 级）、binary 文件处理、inlay、Cmd/Ctrl+S 劫持保存、编辑器导航后退/前进、标题栏后退/前进、绿灯全屏、左上红灯关闭
- **新增** HCL 语言能力：
  - 轻量语法诊断（红波浪线，`hclDiagnostics.ts`）
  - 通用自动补全（Tier1 关键字/骨架 + Tier2 var./local. 引用 + Tier3 平台 module 属性，`hclCompletion.ts`）
  - `var.` / `local.` 转到定义（支持跨文件，`hclDefinitions.ts`）
  - 语法高亮 + 4 个 Monaco provider（`hclLanguage.ts` / `hclProviders.ts`）
- **新增** 跨文件搜索/替换（VS Code 风格，作用于当前草稿，`SearchPanel.tsx` / `searchEngine.ts`），搜索面板视觉对齐 VS Code、侧栏可拖拽调宽
- **新增** 版本对比 + 未提交变更 diff（VS Code 风格），切换到历史视图自动显示未提交变更
- **新增** 未提交更改撤销按钮（还原到最新版本）
- **新增** 编辑器顶栏就地编辑 manifest 名称/描述
- **新增** 顶栏徽标反映真实状态 + 左侧「已部署 workspace」视图
- **新增** module 补全可填 source / version 字段 + 版本历史面板

#### Manifest 部署（前端 + 后端）

- **重构** 部署改为全宽面板（`DeployPanel.tsx`），install 一步配 workdir（去弹窗）
- **新增** 部署页「部署并运行」（Plan+Apply）+ 版本一致提示
- **新增** 部署按钮统一为「部署/更新」+「...并运行」，无变更时只显示「运行」
- **新增** Run 草稿预览：在 subpath 子目录执行（不再退回根目录）
- **新增** demo 列表按当前版本过滤（去跨版本并集）+ 显示 demo 描述
- **修复** upgrade 不再只看版本号——varset 关联变化也可触发更新
- **修复** 部署写动作叠加 workspace 权限校验 + 资源锁定后端拦截
- **修复** 部署 varsets 与 variable_overrides 真正进入 plan/apply 执行

#### Manifest 变量注入与执行（后端）

- **新增** `manifest_deployment_varsets` 表：deployment 关联的 varset（per-deployment，priority 数字大者优先）
- **新增** `workspace_tasks.variable_overrides`：deployment 应急覆盖快照（任务创建时固化，执行时 overlay 到 Terraform 变量，最高优先级，扁平 key=value）
- **新增** `workspace_tasks.external_files`：Manifest [Run] 按钮临时文件（`[{path, content_b64}]`），executor 走 Run 分支用此而不读 `manifest_files`，任务跑完即抛
- **重构** `VariableResolutionService` 引入 `ResolveExecutionWithExtra` / `ResolveDisplayWithExtra`：
  - deployment varset 折进优先级链（workspace-attached 之后、workspace own 之前，按 priority ASC）
  - `variable_overrides` 最高优先级，执行时 overlay
- **修复** workspace variables 页显示 deployment 关联的 varset 变量（此前仅执行路径注入，页面看不到）
- **新增** `manifest_hcl_parser`：浅解析 manifest 文件，提取 module 实例 source / input variable 元信息
- **新增** `manifest_subpath` 服务：`NormalizeManifestSubpath`（禁绝对路径、`.`/`..` 段、超长）+ `ListWorkdirs`（列出含 `.tf` 的目录前缀），供 deployment install 与 workspace CRUD 共用
- **新增** 发布时静态解析 input variables 元信息
- **新增** Manifest 操作 audit_logs 写入（`manifest_audit.go`）

#### 执行与日志体验

- **新增** Apply 时 Resource ID 实时逐个回填（`resource_id_update` WebSocket 广播），替代全部完成后批量刷新
- **修复** Structured 视图：CREATE 资源 `after_unknown` 字段不显示、嵌套字段不显示、tags 重复
  - 新增 `workspace_task_resource_changes.after_unknown` 列（标记 known after apply 字段）
- **修复** terraform plan/apply/init 日志被截断（stdout 读取竞态）
- **修复** `apply_pending` 状态 classic 日志空白/延迟显示
- **修复** apply 期间资源卡片不实时刷新（纯 WS 驱动无兜底）
- **修复** plan-summary 轮询不泄漏

#### CMDB

- **新增** 搜索查询长度限制：`CMDBSearchLog.BeforeCreate` 按 rune 截断超长查询（120 字符），防止粘贴整段摘要导致日志膨胀/展示越界
- **新增** 词云超长截断

#### 基础设施

- **新增** `LimitRequestBodySize` 中间件：限制单次请求体大小（413），用于 manifest 文件 PUT 等端点
- **新增** `RequireWorkspacePermission`：handler 内部对「运行时才知道的 workspace」做权限校验（manifest install/upgrade/uninstall）
- **修复** vite build 在 Docker 内 OOM（monaco-vscode-api 14MB chunk）；React 保留在主 chunk 避免 antd hoisting 报错
- **重构** `manifest_handler.go` 精简至顶层 CRUD（删除约 2400 行旧画布 handler）；清理旧 React Flow 画布编辑器与画布 schema 死代码

### Database Migration

- `manifest_files`（新表）：草稿 / 发布快照统一存储，部分唯一索引 + 列表索引
- `manifest_deployment_varsets`（新表）：deployment ↔ varset 关联
- `manifest_versions`：
  - 新增 `changelog text`
  - 新增 `uq_manifest_versions_name (manifest_id, version)` 唯一索引
  - 新增 `chk_manifest_versions_semver`（version 格式 CHECK，NOT VALID）
  - 移除画布字段 `canvas_data` / `nodes` / `edges` / `hcl_content` / `is_draft`
- `manifest_deployments`：
  - `workspace_id` 类型 `int → varchar(50)`（自动 backfill + drop 旧 FK）
  - 新增 `chk_md_status_valid`（status CHECK，NOT VALID）
  - 新增 `uq_manifest_deployments_workspace_active`（单 active deployment）
- `workspaces`：新增 `manifest_deployment_id` / `manifest_active_tag` / `manifest_subpath` + 一致性 CHECK
- `workspace_tasks`：新增 `external_files jsonb` / `variable_overrides jsonb`
- `workspace_task_resource_changes`：新增 `after_unknown jsonb`
- `manifest_deployment_resources`：标记 DEPRECATED（未 drop）
- 迁移脚本：
  - `backend/migrations/add_manifest_v2.sql`（核心 schema）
  - `backend/migrations/cleanup_manifest_canvas.sql`（清理旧画布字段）
  - `backend/migrations/add_task_variable_overrides.sql`
  - `backend/migrations/add_after_unknown.sql`
  - `manifests/db/init_seed_data.sql`（schema 增量同步进种子）
