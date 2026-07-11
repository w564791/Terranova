本版本核心交付 **Manifest 编辑器 Provider 类型补全体系**：terraform init 后自动落库 provider 资源/数据源类型目录（按 manifest+subpath 共享），编辑器运行时加载用于 Tier 3 补全；引入 VS Code 扩展框架加载 HashiCorp 官方 TextMate grammar 实现精准语法高亮；补全/跳转/诊断能力全面增强（resource/data/module 引用解析、Terraform 内置函数、Problems 面板、Quick Open、面包屑导航）。

---

## Highlights

### 1. Provider Schema 落库与编辑器类型补全

**后端 post_init 阶段（terraform init 后自动执行）：**

- **新增 `post_init` 执行阶段**：`terraform_executor.go` 在 init 段落后插入 `runPostInitStage`，Plan 和 Apply 流程均接入；失败只 warn，永不 fail task
- **Provider 版本指纹**：从 `.terraform.lock.hcl` 提取 provider source+version，规范化后 SHA256 生成 `provider_versions_key`；版本未变则跳过 schema 提取与写库（no-op）
- **类型目录提取**：调用 `terraform providers schema -json`，解析 `resource_schemas` / `data_source_schemas` 提取类型名列表；90 秒超时保护
- **按 manifest+subpath 共享存储**：`manifest_provider_schemas` 表以 `(manifest_id, subpath, schema_kind)` 为唯一键，多 workspace 部署同一 manifest 共享一行缓存
- **Agent API 双端点**：`GET .../manifest-provider-schema/meta`（版本指纹查询）+ `PUT .../manifest-provider-schema`（写入类型目录），服务端按 workspace 解析 manifest_id+subpath，Agent 无需感知 manifest 结构
- **Manifest 编辑器 API**：`GET .../manifests/:id/provider-schemas?subpath=`（前端拉取类型目录）

**前端编辑器接入：**

- **运行时类型目录注入**：`setProviderTypeCatalog()` 在编辑器打开/subpath 切换时从 API 加载，存入模块级 `runtimeCatalog`
- **Tier 3 resource/data 类型补全**：resource 块内输入 `resource "aws_` 时按 runtime catalog 过滤候选（此前只能靠 module 输入变量补全）
- **状态栏 schema 版本展示**：显示 `content_hash` 短码（无缓存时为 `—`），title 提示来源与更新策略

### 2. VS Code 扩展框架与 TextMate 高亮

- **扩展注册表**（`extensions/registry.ts`）：声明式扩展目录，支持 `browser` / `lsp-backend` 两种 kind；当前注册 4 个扩展（theme-defaults / hashicorp-syntax / terranova-hcl-intel / hashicorp-terraform-ls），LSP 默认关闭
- **扩展引导加载**（`extensions/bootstrap.ts`）：在 vscode services initialize 后按 registry 顺序加载，单个扩展失败不阻断编辑器
- **HashiCorp 官方 TextMate grammar**：`loaders/hashicorpSyntax.ts` 加载从 `hashicorp/syntax` 仓库拉取的 `.tmLanguage.json`，为 `terraform` / `terraform-vars` / `hcl` 三种语言 ID 提供精准语法高亮
- **Monarch 降级兜底**：TextMate 未就绪或加载失败时仍使用 Monarch 正则高亮，保证基本可读性
- **Grammar 更新脚本**：`scripts/update-hcl-grammar.mjs` 从 GitHub raw 拉取最新 grammar；`make update-hcl-grammar` / `npm run update:hcl-grammar` 一键更新
- **新增依赖**：`@codingame/monaco-vscode-extensions-service-override` + `@codingame/monaco-vscode-files-service-override`，vscode 初始化接入 extensions / files service

### 3. 语言 ID 对齐与多语言注册

- **语言 ID 拆分**：原统一 `hcl` 拆分为 `hcl`（.hcl）/ `terraform`（.tf）/ `terraform-vars`（.tfvars），对齐 HashiCorp 官方 language id 映射
- **Provider 多语言注册**：补全（Completion）、跳转（Definition）、Hover、InlayHint、CodeAction、诊断（Diagnostics）等 provider 在 `HCL_LANGUAGE_IDS`（3 个 id）上统一注册
- **`languageOfPath()` 更新**：`.tf` → `terraform`，`.tfvars` → `terraform-vars`，`.hcl` → `hcl`

### 4. 补全能力增强

- **Terraform 内置函数补全**：`length` / `lookup` / `merge` / `coalesce` / `try` / `can` / `format` / `join` / `split` / `join` 等 30+ 函数，在表达式位置（`${}` 内或等号右侧）触发
- **Resource/Data 类型补全**：resource 块第一行 `resource "TYPE" "NAME"` 位置，按 runtime catalog 提供 resource 类型候选；data 块同理
- **引用补全扩展（Tier 2）**：从仅 `var.` / `local.` 扩展到 `module.NAME` / `data.TYPE.NAME` / `TYPE.NAME`（provider_resource 形态），优先使用跨文件工作区索引，回退当前 buffer
- **Module 模板补全优化**：sortText 改 `z_` 前缀排在关键字骨架之后（避免淹没 `resource` / `variable` 等）；增加场景过滤（仅顶层空行/`module` 关键字位置推送，块内属性位置不推送）；支持按输入片段过滤 source
- **退格 re-trigger**：`hclSuggestRetrigger.ts` — 退格把 `var.x.` → `var.x` → `var.` 时主动 re-trigger 补全弹窗（Monaco 默认不会再弹）
- **补全配置调优**：`wordBasedSuggestions: 'off'`（避免全文单词候选淹没 HCL 关键字/snippet）；`quickSuggestions.strings: true`（resource 引号内出类型补全）
- **Module 输入字段增强**：`ModuleInputField` 新增 `type_label`（HCL 风格展示如 `list(string)` / `map(string)`）/ `enum`（枚举可选值）/ `title`（OpenAPI title）；`extractModuleInputs` 增强 array/items / map(additionalProperties) / `$ref` 解析；结果按 required 优先 + name 字母序稳定排序

### 5. 跳转与引用解析增强

- **Definition 跳转扩展**：从仅 `var.` / `local.` 扩展到 `module.NAME` / `data.TYPE.NAME` / `TYPE.NAME`（resource 引用），跨文件跳转统一走 opener 路由
- **Hover 提示扩展**：同上，显示定义位置（当前文件 / 跨文件路径+行号）
- **引用解析优化**：定义行本身不当作引用（避免 `variable "x"` 里的 x 被当成 `var.x`）；多候选按匹配长度降序优先更长匹配

### 6. 诊断增强

- **Heredoc 文件诊断改善**：原 heredoc 存在时整文件放弃所有诊断；现仅跳过 structural（括号配对），重复定义与未定义引用仍正常报告
- **Problems 面板**（`ProblemsPanel.tsx` + `collectProblems.ts`）：侧栏新增 Problems 视图，聚合 Monaco markers 按文件分组展示（错误/警告计数），点击定位到行列
- **状态栏问题计数**：底部状态栏显示 error / warning 数量，点击打开 Problems 面板

### 7. 编辑器交互增强

- **Quick Open（Cmd/Ctrl+P）**（`QuickOpen.tsx`）：VS Code 风格快速打开文件对话框，模糊搜索文件名（subsequence 评分 + 连续匹配加分 + 文件名优先），↑↓ 选择 + Enter 打开 + Esc 关闭
- **编辑器面包屑**：编辑区顶部显示当前文件路径分段面包屑（`folder/subfolder/main.tf`）
- **导航快捷键对齐 VS Code**：跨文件导航后退/前进从 `Cmd/Ctrl+←/→` 改为 `Alt+←/→`（保留 Cmd+←/→ 行首行尾肌肉记忆）
- **Gutter diff 防抖**：大文件 LCS 计算昂贵，编辑时改为 250ms 防抖（此前每次按键全量重算）

### 8. 数据库

- **新增 `manifest_provider_schemas` 表**：migration `add_manifest_provider_schemas.sql`，存储 provider 类型目录缓存；`(manifest_id, subpath, schema_kind)` 唯一约束，`provider_versions_key` 索引用于快速版本比对
- **种子 SQL 同步**：`init_seed_data.sql` 追加 CREATE TABLE + 索引 + COMMENT，与 migration 对齐

### 9. 构建与工程化

- **Makefile 新增**：`update-hcl-grammar` / `update-editor-assets` target，前端打包前可拉取最新 HashiCorp grammar
- **package.json 新增**：`update:hcl-grammar` / `update:editor-assets` / `build:ci` script
- **CI build:ci**：`npm run update:hcl-grammar && npx vite build`，确保 CI 构建使用最新 grammar
- **tsconfig**：新增 `resolveJsonModule: true`（grammar JSON 导入）
- **vite.config.ts**：`monacoVscodeApiPackages` 追加 extensions / files service override

### 10. Bug Fixes

- **Module demo 补全场景误推**：`hclProviders.ts` Completion 通道场景 B 原在所有非引号位置推送 module 模板，导致块内属性位置也刷出 module 列表，淹没正常补全。改为仅顶层空行 / `module` 关键字位置推送，并排除正在输入 `resource` / `variable` 等核心关键字的情况
- **诊断 heredoc 过度放弃**：heredoc 存在时整文件不报任何诊断（包括重复定义与未定义引用），改为仅跳过 structural 诊断

---

## 修改文件

### 后端

- `backend/services/terraform_executor.go` — 新增 `runPostInitStage`，Plan / Apply 流程 init 后接入 provider schema capture
- `backend/services/provider_schema_capture.go` — **新增**：lock 文件解析、`terraform providers schema -json` 提取、版本指纹比对、upsert 逻辑
- `backend/services/provider_schema_capture_test.go` — **新增**：lock 解析单元测试
- `backend/internal/handlers/agent_handler.go` — 新增 `GetManifestProviderSchemaMeta` / `UpsertManifestProviderSchema` handler（Agent API）
- `backend/internal/handlers/manifest_provider_schema_handler.go` — **新增**：编辑器端 `GetProviderSchemas` handler
- `backend/internal/handlers/manifest_editor_handler.go` — `extractModuleInputs` 增强（type_label / enum / title / array+map 解析 / 排序）；`ModuleInputField` 结构扩展
- `backend/internal/handlers/manifest_editor_inputs_test.go` — **新增**：extractModuleInputs 单元测试
- `backend/internal/models/manifest_provider_schema.go` — **新增**：`ManifestProviderSchema` GORM 模型 + `ProviderVersionRef` 结构
- `backend/migrations/add_manifest_provider_schemas.sql` — **新增**：建表 migration
- `backend/services/data_accessor.go` — DataAccessor 接口新增 `GetManifestProviderSchemaMetaByWorkspace` / `UpsertManifestProviderSchemaByWorkspace`
- `backend/services/local_data_accessor.go` — LocalDataAccessor 实现：`resolveManifestSubpathKey` + meta 查询 + upsert
- `backend/services/remote_data_accessor.go` — RemoteDataAccessor 实现：透传 AgentAPIClient
- `backend/services/agent_api_client.go` — AgentAPIClient 新增 `GetManifestProviderSchemaMeta` / `UpsertManifestProviderSchema`
- `backend/internal/router/router_agent.go` — Agent 路由注册 manifest-provider-schema GET/PUT
- `backend/internal/router/router_manifest.go` — Manifest 路由注册 provider-schemas GET
- `manifests/db/init_seed_data.sql` — 追加 `manifest_provider_schemas` CREATE TABLE + 索引

### 前端

- `frontend/src/pages/admin/ManifestEditorV2/ManifestEditorV2.tsx` — Problems 面板 / QuickOpen / 面包屑 / Alt 导航 / gutter diff 防抖 / schema 版本拉取与状态栏 / suggestRetrigger / 补全配置调整
- `frontend/src/pages/admin/ManifestEditorV2/ManifestEditorV2.module.css` — Problems 面板 / QuickOpen / 面包屑 / 状态栏可点击样式
- `frontend/src/pages/admin/ManifestEditorV2/ProblemsPanel.tsx` — **新增**：问题面板组件
- `frontend/src/pages/admin/ManifestEditorV2/QuickOpen.tsx` — **新增**：快速打开文件组件
- `frontend/src/pages/admin/ManifestEditorV2/collectProblems.ts` — **新增**：Monaco markers 聚合
- `frontend/src/pages/admin/ManifestEditorV2/hclSuggestRetrigger.ts` — **新增**：退格 re-trigger 补全
- `frontend/src/pages/admin/ManifestEditorV2/hclLanguage.ts` — 语言 ID 拆分（hcl / terraform / terraform-vars）+ `HCL_LANGUAGE_IDS` 常量
- `frontend/src/pages/admin/ManifestEditorV2/hclCompletion.ts` — runtime provider catalog + Tier 3 resource/data 类型补全 + Terraform 内置函数补全 + 引用补全扩展 + 注册改多语言
- `frontend/src/pages/admin/ManifestEditorV2/hclDefinitions.ts` — 跳转/引用扩展（module / data / resource）+ `lookupDefinition` / `refLabel` + 多语言注册
- `frontend/src/pages/admin/ManifestEditorV2/hclProviders.ts` — Completion 场景优化 + 多语言注册 + manifestInsertDemo 命令移出循环
- `frontend/src/pages/admin/ManifestEditorV2/hclDiagnostics.ts` — heredoc 诊断策略调整（仅跳 structural，保留 duplicate + undefined ref）
- `frontend/src/pages/admin/ManifestEditorV2/initServices.ts` — extensions / files service override + `bootstrapExtensions` 调用
- `frontend/src/pages/admin/ManifestEditorV2/manifestApi.ts` — 新增 `getProviderSchemas` API + `languageOfPath` 更新
- `frontend/src/pages/admin/ManifestEditorV2/moduleDemoApi.ts` — `ModuleInputField` 接口扩展（type_label / enum / title）
- `frontend/src/pages/admin/ManifestEditorV2/extensions/registry.ts` — **新增**：扩展注册表
- `frontend/src/pages/admin/ManifestEditorV2/extensions/bootstrap.ts` — **新增**：扩展引导加载
- `frontend/src/pages/admin/ManifestEditorV2/extensions/types.ts` — **新增**：扩展类型定义
- `frontend/src/pages/admin/ManifestEditorV2/extensions/loaders/` — **新增**：hashicorpSyntax / themeDefaults / terranovaIntel / hashicorpTerraformLs loader
- `frontend/src/components/MonacoHclEditor/MonacoHclEditor.tsx` — 接入 suggestRetrigger
- `frontend/scripts/update-hcl-grammar.mjs` — **新增**：grammar 更新脚本
- `frontend/package.json` — 新增 extensions / files service override 依赖 + npm scripts
- `frontend/vite.config.ts` — monacoVscodeApiPackages 追加
- `frontend/tsconfig.app.json` — `resolveJsonModule: true`
- `frontend/Dockerfile` — 注释更新（provider schema 已改 execute 落库）

### 构建

- `Makefile` — 新增 `update-hcl-grammar` / `update-editor-assets` target
- `.github/workflows/ci.yml` — CI 流水线调整
- `.github/workflows/release.yml` — Release 流水线调整

---

## 技术细节

### Provider Schema 落库策略

- **落库时机**：`terraform init` 之后的 `post_init` 阶段，Plan 和 Apply 均执行
- **版本指纹跳过**：从 `.terraform.lock.hcl` 提取所有 provider 的 `source@version`，规范化排序后 SHA256 生成 `provider_versions_key`；与库中已有记录的 key 比对，相同则跳过提取（`no-op`），避免每次 Run 都跑 `terraform providers schema`
- **存储键**：`(manifest_id, subpath, schema_kind)`，同一份 manifest 源码 + 同一执行子目录共享一行；多 workspace 部署同一 manifest 时，首个触发的 workspace 写入，后续跳过
- **非托管识别**：workspace 非 manifest-managed 时（`resolveManifestSubpathKey` 返回 "not manifest-managed"），跳过 capture 并记 info 日志
- **超时保护**：`terraform providers schema -json` 调用设 90 秒超时，超时后 warn 跳过不 fail task

### 扩展框架设计

- **声明式注册**：`EXTENSION_CATALOG` 数组声明所有已知扩展，每项含 `id` / `kind`（browser / lsp-backend）/ `defaultEnabled` / `load` 函数
- **按需启用**：`ENABLED_EXTENSION_IDS` 白名单（null 时用各扩展 defaultEnabled），便于环境切换
- **加载顺序**：`bootstrapExtensions()` 按注册表顺序 await 每个扩展的 `load()`，单个失败不影响后续
- **Grammar 更新**：`scripts/update-hcl-grammar.mjs` 从 `hashicorp/syntax` GitHub raw 拉取 `.tmLanguage.json` 存入 `extensions/assets/`，CI 构建时通过 `build:ci` 自动执行
