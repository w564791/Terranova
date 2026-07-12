本版本包含四大交付线：**Manifest 编辑器 Provider 类型补全体系**（terraform init 后自动落库 provider 资源/数据源类型目录，编辑器运行时加载用于 Tier 3 补全；引入 VS Code 扩展框架加载 HashiCorp 官方 TextMate grammar 实现精准语法高亮；补全/跳转/诊断能力全面增强）+ **Grok (xAI) Provider 集成 + Provider 级 Fallback 容灾 + 自定义 Capability 场景**（新增 xAI Grok 官方 API 作为 AI Provider；专用 config 访问失败时自动降级到全局 default，仅切换模型/凭证保留任务 Skill；前端支持新增自定义场景标识）+ **CMDB 搜索结果 AI 解读 + 相关性筛查**（新增 `cmdb_search_summary` capability，搜索召回结果经 AI 生成总览/重点/分组/改写建议，并按查询意图自动剔除低相关项；前端 CMDB 搜索页全面接入 SSE 进度与筛查交互）+ **前端设计系统 v3 — Design Token 统一 + UI 基元组件 + Ant Design 主题集成**（建立 `theme.css` 单一 token 源，语义化 surface/ink/brand/green/red/amber 色阶；新增 Dialog/Button/Feedback CSS 基元；Ant Design 5 `ConfigProvider` 全局主题对齐深青品牌色；~228 文件从硬编码 `--color-*` 迁移至语义 token，旧变量自动 alias 兼容）。

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

### 11. Grok (xAI) 官方 Provider 集成

新增 `service_type=grok` 作为 AI Provider，走 xAI 官方 OpenAI 兼容 `chat/completions` API，支持专属 `reasoning_effort` 参数（low / medium / high 三档，Grok reasoning 不可关闭）。

**后端：**

- **`GrokCaller` 结构**（`ai_caller.go`）：嵌入 `OpenAICaller`，重写 `ChatWithTools` 支持 tool calling；`buildGrokRequest` 注入 `reasoning_effort` 并移除 reasoning 模型不兼容的 `temperature` 参数；`parseGrokResponse` 解析 `reasoning_content` + 标准 content + tool_calls + token metrics
- **公共传输层抽取**：`doGrokChatCompletion()` 统一 HTTP 传输（POST `{baseURL}/chat/completions`），`CallGrokSimple()` 面向单轮 user prompt 场景（Form / ModuleSkill / TestConfig 共用），避免 HTTP 调用代码在多处重复
- **Effort-based timeout**：`GrokTimeoutForEffort()` 按 effort 档动态超时 — high=600s / medium=300s / low=120s
- **Effort 规范化**：`NormalizeGrokReasoningEffort()` 统一处理大小写/空白/非法值，默认 high
- **API Key 环境变量兜底**：`XAI_API_KEY` 环境变量，`fillAPIKeyFallback` 自动注入
- **Create/Update/Test 全链路 grok 分支**：创建和更新时自动规范化 effort 和兜底 API Key；`TestConfig` 走 `CallGrokSimple(maxTokens=100)` 连通性探测
- **Form 生成接入**：`AIFormService.callAI` 新增 grok 分支，复用 `CallGrokSimple(maxTokens=4000)`
- **Module Skill 生成接入**：`ModuleSkillAIService.callAIOnce` 新增 grok 分支，复用 `CallGrokSimple(maxTokens=4096)`
- **Model List API 兜底**：`ListOpenAIModels` handler 按 `baseURL` 含 `x.ai` 自动选择 `XAI_API_KEY` 环境变量
- **DB Migration**：`add_grok_reasoning_effort.sql` — `grok_reasoning_effort varchar(20) NOT NULL DEFAULT 'high'`

**前端：**

- **Service Type 选择器**：新增 `Grok (xAI 官方)` 选项，切换时自动设置 `base_url=https://api.x.ai/v1`
- **Reasoning Effort Radio**：紫色区块 UI，三档单选（低 / 中 / 高），选中态紫色边框 + 背景，附中文描述标签
- **Grok 专属说明面板**：底部 Tips 区块说明 OpenAI 兼容协议、Base URL 默认值、环境变量、reasoning 不可关闭等
- **编辑态自动拉模型**：编辑已有 grok 配置时自动从 API 拉取模型列表

### 12. Provider 级 Fallback 容灾机制

专用 AI config 访问失败时（认证/限流/超时/网络等），自动降级到全局 default config（`enabled=true, capabilities=["*"]`），**仅切换 Provider（模型/凭证），保留原任务 Skill/prompt 不变**。

**核心组件：**

- **`IsAIAccessError(err)`**：白名单机制区分访问错误 vs 业务错误。40+ access hints 覆盖 auth/throttle/timeout/network/HTTP status code/Bedrock/aws credentials 等；明确排除 output validation/parse/unmarshal/输出格式等业务错误，避免内容质量问题触发无意义重试
- **`ResolveProviderFallback(capability, primary, err)`**：四重守卫 — embedding 不 fallback（向量模型语义不同）、已是 default 不 fallback（避免自环）、非访问错误不 fallback、default 与 primary 相同不 fallback
- **`FallbackAICaller`**：包装 primary + fallback 两个 AICaller，**sticky** 语义 — 首次 fallback 后后续调用固定走 default，保证 Agent Loop 内模型一致
- **`NewCallerWithFallback(skillCfg, capability)`**：工厂方法，为 Agent Loop 场景创建带 fallback 的 caller
- **`callAIForCapability(capability, skillCfg, prompt)`**：`AIFormService` 新增方法，单次调用 + 非 sticky fallback（适用于非循环场景）

**全服务接入（12 处调用点改造）：**

| 模式 | 服务 | capability 标识 |
|---|---|---|
| `callAIForCapability` | AIFormService | `form_generation` / `intent_assertion` |
| `callAIForCapability` | AICMDBService | `cmdb_query_plan` |
| `callAIForCapability` | AICMDBSkillService | `form_generation` / `cmdb_need_assessment` |
| `callAIForCapability` | ManifestAIService | `manifest_resource_generation` |
| `callAIForCapability` | ManifestCheckService | `manifest_check` |
| `callAIForCapability` | manifestDomainSkillSelector | `domain_skill_selection` |
| `callAIForCapability` | AIFeedbackLoop | `form_generation` |
| `NewCallerWithFallback` | AIAnalysisService | `error_analysis` |
| `NewCallerWithFallback` | AISummaryService ×2 | `summary` |
| `NewCallerWithFallback` | ResourceSummaryService | `cmdb_resource_summary` |
| `NewCallerWithFallback` | SkillLLMEvaluator | `skill_rule/semantic_evaluation` |
| `NewCallerWithFallback` | SummaryLLMEvaluator | `summary_rule/semantic_evaluation` |

### 13. 自定义 Capability 场景 + 新预置场景

**新预置场景：**

- `change_analysis`（变更分析）— 分析 Plan 变更内容和影响
- `result_analysis`（结果分析）— 分析 Apply 执行结果

**自定义 Capability 支持：**

- **前端新增场景面板**：「+ 新增场景」按钮 → 展开表单（场景标识 key + 显示名称），key 校验规则 `/^[a-z][a-z0-9_]{1,63}$/`
- **自定义标签展示**：自定义场景在列表中显示橙色「自定义」标签 + key 标识
- **`getCapabilityLabel()`**：统一获取场景展示名（支持自定义标签覆盖），替代直接引用 `CAPABILITY_LABELS`
- **`KNOWN_CAPABILITY_VALUES`**：从 `CAPABILITIES` 常量导出的预置场景列表
- **编辑态同步**：加载已有配置时自动识别 DB 中存在但不在预置列表的 capability，标记为自定义
- **"加载默认模板" 按钮**：无内置模板时自动隐藏（自定义场景无 `DEFAULT_CAPABILITY_PROMPTS`）
- **Default 描述更新**：「设置为default」→「设为全局兜底（default）」，提示文案明确说明 default 是访问层兜底而非任务默认 Skill

---

## 测试覆盖（v0.8.1 新增）

### `ai_caller_test.go`

- **`TestNormalizeGrokReasoningEffort`**：6 个 case — 空字符串/大写/合法值/非法值/none 均正确规范化
- **`TestNewAICallerFromConfig_GrokType`**：验证 grok config 创建 `GrokCaller`，baseURL 回落默认，effort 正确传递
- **`TestGrokCaller_buildGrokRequest`**：验证 `reasoning_effort` 注入 + `temperature` 移除 + model 正确

### `ai_config_fallback_test.go` — **新增文件**

- **`TestIsAIAccessError`**：10 个 case — nil / Bedrock AccessDenied / ValidationException / timeout / 401 / model not found 返回 true；output validation / 输出格式 / 业务错误返回 false；wrapped error 正确穿透
- **`TestIsDefaultAIConfig`**：4 个 case — nil / enabled=false / 无 `*` / 正确 default
- **`TestFallbackAICaller_RetriesOnAccessError`**：primary 访问失败 → 自动切到 fallback 并返回正确结果
- **`TestFallbackAICaller_NoRetryOnBusinessError`**：业务错误直接返回，不触发 fallback
- **`TestFallbackAICaller_StickyAfterFallback`**：首次 fallback 后第二次调用直接走 fallback，不再试 primary

---

## 修改文件（v0.8.1 新增）

### 后端

- `backend/services/ai_caller.go` — `GrokCaller` 结构 + `ChatWithTools` / `buildGrokRequest` / `parseGrokResponse`；公共 helpers `NormalizeGrokReasoningEffort` / `GrokTimeoutForEffort` / `ResolveGrokBaseURL` / `applyGrokReasoningToBody` / `doGrokChatCompletion` / `CallGrokSimple`；`FallbackAICaller` 结构 + sticky `ChatWithTools`；`NewAICallerFromConfig` 新增 grok case
- `backend/services/ai_config_service.go` — `GetDefaultConfig` / `isDefaultAIConfig` / `IsAIAccessError` / `ResolveProviderFallback` / `NewCallerWithFallback`；Create/Update/Test/fillAPIKeyFallback 全链路 grok 分支；`testGrok` 连通性探测；`GetConfigForCapability` 兜底逻辑改用 `GetDefaultConfig`
- `backend/services/ai_form_service.go` — grok 分支走 `CallGrokSimple`；`callAIForCapability` 新方法（fallback 包装）；`generateConfigInternal` / `AssertIntent` 改用 `callAIForCapability`
- `backend/services/module_skill_ai_service.go` — `callAI` 改为 `callAI` + `callAIOnce`（fallback 包装）；`callAIOnce` grok 走 `CallGrokSimple`；`callOpenAICompatible` 兼容 grok
- `backend/services/ai_analysis_service.go` — `NewAICallerFromConfig` → `NewCallerWithFallback`（error_analysis）
- `backend/services/ai_summary_service.go` — 两处 `NewAICallerFromConfig` → `NewCallerWithFallback`（summary）
- `backend/services/resource_summary_service.go` — `NewAICallerFromConfig` → `NewCallerWithFallback`（cmdb_resource_summary）
- `backend/services/skill_llm_evaluator.go` — `callLLM` 新增 capability 参数；两处调用传入 `skill_rule_evaluation` / `skill_semantic_evaluation`；`NewAICallerFromConfig` → `NewCallerWithFallback`
- `backend/services/summary_llm_evaluator.go` — 同上，`summary_rule_evaluation` / `summary_semantic_evaluation`
- `backend/services/ai_cmdb_service.go` — `callAI` → `callAIForCapability`（cmdb_query_plan）
- `backend/services/ai_cmdb_skill_service.go` — 三处 `callAI` → `callAIForCapability`（form_generation / cmdb_need_assessment）
- `backend/services/ai_cmdb_skill_service_sse.go` — 两处 `callAI` → `callAIForCapability`（form_generation）
- `backend/services/manifest_ai_service.go` — `callAI` → `callAIForCapability`（manifest_resource_generation）
- `backend/services/manifest_check_service.go` — `callAI` → `callAIForCapability`（manifest_check）
- `backend/services/manifest_domain_skill.go` — `callAI` → `callAIForCapability`（domain_skill_selection）
- `backend/services/schema_solver_loop.go` — `callAI` → `callAIForCapability`（form_generation）
- `backend/services/ai_caller_test.go` — 新增 3 个 Grok 相关测试
- `backend/services/ai_config_fallback_test.go` — **新增**：5 个 fallback 机制测试
- `backend/controllers/ai_controller.go` — `ListOpenAIModels` API Key 兜底按 baseURL 判断 provider
- `backend/internal/models/ai_config.go` — 新增 `GrokReasoningEffort` 字段
- `backend/internal/config/config.go` — 新增 `XAIAPIKey` 环境变量
- `backend/migrations/add_grok_reasoning_effort.sql` — **新增**：migration
- `manifests/db/init_seed_data.sql` — CREATE TABLE 同步 `grok_reasoning_effort` 列

### 前端

- `frontend/src/pages/AIConfigForm.tsx` — Grok service type 选项 + Base URL 默认值 + Reasoning Effort radio UI + Grok help 面板 + 编辑态自动拉模型 + 自定义 Capability 新增面板 + `getCapabilityLabel` 替代直接引用 + default 描述更新 + "加载默认模板" 按钮条件渲染
- `frontend/src/pages/AIConfigList.tsx` — `CAPABILITY_LABELS` → `getCapabilityLabel()`
- `frontend/src/services/ai.ts` — `GROK_REASONING_EFFORTS` / `GROK_REASONING_EFFORT_LABELS` / `DEFAULT_GROK_BASE_URL` 常量；`AIConfig` 接口新增 `grok_reasoning_effort`；`CAPABILITIES` 新增 `change_analysis` / `result_analysis`；`KNOWN_CAPABILITY_VALUES` / `isValidCapabilityKey` / `getCapabilityLabel` 导出

---

### 14. CMDB 搜索结果 AI 解读 + 相关性筛查

**新增 `cmdb_search_summary` capability**，与 `cmdb_query_plan`（表单/业务流程的查询计划）完全分离，仅服务于 `/cmdb?tab=search` 搜索结果页的友好解读。

**后端：**

- **`cmdb_search_summary_service.go`（新增）**：完整的解读 + 筛查服务
  - 常量控制：送入大模型最多 12 条（`searchSummaryAIMaxItems`），确定性主题筛查最多 40 条（`searchSummaryIntentMaxItems`），单条摘要截断 72 字符
  - `Generate(ctx, query, results)` 入口：调用 `GetConfigForCapability` 获取专用 AI config，构建 prompt 后调用模型，解析 JSON 输出
  - `parseSearchSummaryJSON` 多策略解析：先直接 Unmarshal，失败后尝试提取 `{...}` 子串（容错 AI 输出的 markdown 包裹）
  - `buildSearchSummaryPrompt` 动态构建 prompt：注入 `{query}` / `{result_count}` / `{results_json}` 占位符
  - 空结果处理：`overview` 说明未找到 + `suggestions` 给出改写建议
  - `SearchSummaryResult` / `SearchSummaryInputResource` / `SearchSummaryHighlight` / `SearchSummaryGroup` / `SearchSummaryDropped` 完整类型定义

- **`cmdb_search_summary_service_test.go`（新增）**：覆盖 prompt 构建、JSON 解析（含 markdown 包裹容错）、空结果处理、截断逻辑

- **`embedding_controller.go`**：新增两个 handler
  - `SearchSummary`：同步 JSON 接口（兼容保留）
  - `SearchSummarySSE`：SSE 进度流式接口，三步进度（准备上下文 → AI 解读与筛查 → 完成），complete 事件携带完整 `SearchSummary` 结果

- **`router_ai.go`**：注册两个新路由，IAM 权限守卫（`AI_ANALYSIS` READ/WRITE/ADMIN）
  - `POST /ai/cmdb/search-summary`
  - `POST /ai/cmdb/search-summary-sse`

- **`progress_event.go`**：`ProgressEvent` 新增 `SearchSummary *SearchSummaryResult` 字段，complete 事件携带解读结果

**前端：**

- **`cmdb.ts`（Service 层）**：
  - 新增 `SearchSummaryResult` / `SearchSummaryProgressEvent` / `SearchSummaryDropped` 等类型定义
  - `buildSearchSummaryPayload()` 精简字段（去 workspace/account 噪声），截断长文本，限 40 条
  - `searchSummary()` 同步兜底接口
  - `searchSummarySSE()` SSE 流式接口：fetch + ReadableStream，逐块解析 SSE 事件，`onProgress` 回调实时通知进度
  - `ResourceSearchResult` 接口新增 `resource_summary` / `similarity` 可选字段

- **`CMDB.tsx`（页面）**：
  - 新增状态：`aiSummary` / `aiSummaryLoading` / `aiSummaryError` / `aiSummaryStep` / `aiSummaryProgress` / `resultsGate`（blocked/revealed）/ `skipAIFilter` / `showDroppedResults`
  - `fetchAISummary()` 核心逻辑：调用 SSE 接口，实时进度回调，complete 时解析 dropped 列表构建 `dropIndexSet` / `dropReasonByIndex`
  - `keptResults` / `droppedResults` 计算：按 `dropped` 数组分流，支持"查看已剔除"切换
  - `handleSkipAIFilter` 跳过按钮：用户可跳过 AI 筛查直接看全部结果
  - `normalizeAISearchSuggestion()` 清洗建议词：去除"建议搜索："/"试试："等引导前缀，只保留纯查询词
  - `performSearch` 改造：搜索后自动触发 AI 解读，零结果时直接放行（`resultsGate=revealed`），新搜索自动 abort 上一次 SSE
  - 结果列表改造：按 `showDroppedResults` 切换全量/筛查视图，被剔除项显示紫色"已剔除"badge + 原因 tooltip
  - AI 解读卡片：overview 总览 / highlights 重点列表 / groups 分组 chip / suggestions 可点击回填搜索 / dropped 筛查工具栏

- **`CMDB.module.css`（样式）**：~280 行新增样式
  - AI 解读卡片：渐变背景 + 圆角 + indigo 主色调
  - 进度步骤条：三态（pending/active/done）胶囊样式
  - 结果屏蔽占位：loading + "跳过，直接看结果"按钮
  - 筛查工具栏：已剔除数提示 + "显示已剔除/仅看相关"切换
  - 建议词按钮：圆角胶囊，hover 变色
  - 剔除 badge：紫色背景 + 原因 tooltip
  - 列表项剔除态：`resultItemDropped` 半透明 + 划线

**数据库：**

- **AI Config 种子数据（`init_seed_data.sql`）**：新增 id=24 的 `cmdb_search_summary` 专用配置，mode=prompt，使用 Claude Sonnet 4
- **3 个 Migration 文件**：
  - `add_cmdb_search_summary_ai_config.sql`：幂等 INSERT（`WHERE NOT EXISTS` capabilities 判重）
  - `update_cmdb_search_summary_screening.sql`：更新 prompt 增加 dropped 筛查规则
  - `update_cmdb_search_summary_suggestions.sql`：强化 suggestions 规则（必须为纯查询词，禁止说明前缀）

**基础设施：**

- **`docker-compose.nginx.conf`**：Nginx `/api/` location 新增 SSE 支持配置
  - `proxy_buffering off`：关闭代理缓冲，progress 事件实时刷出
  - `proxy_cache off` + `chunked_transfer_encoding on`

### 15. AI Access Error 分类增强

扩展 `IsAIAccessError()` 白名单，覆盖 DashScope 国际站偶发的对端提前断开连接场景：

- 新增 access hints：`unexpected eof` / `eof` / `broken pipe` / `http2: server sent goaway` / `request failed`
- 新增测试用例：`unexpected EOF`（DashScope intl URL）/ `connection reset by peer`

**影响**：这些错误现在正确触发 Provider 级 Fallback 容灾（切换到 default config），而非作为业务错误直接返回失败。

### 16. Embedding Worker 优化

`processPendingTasks()` 新增快速退出检查：

- 在查询 embedding 配置之前，先 `SELECT count(*) > 0` 检查是否有待处理任务
- 无任务时直接 return，避免每次都查询 AI config 状态 + 打日志
- 减少无 embedding 需求时的 DB 查询和日志噪声

### 17. AIConfigForm 交互优化

- 默认配置（`capabilities=["*"]`）时隐藏"+ 新增场景"按钮，改为灰色提示文案："默认配置（capabilities=*）无需新增场景；请创建「非默认」专用配置"
- 修复条件渲染：`!formData.enabled &&` → `!formData.enabled ?` 三元表达式

---

## 修改文件（v0.8.1 CMDB 搜索解读 + 增强）

### 后端

- `backend/controllers/embedding_controller.go` — 新增 `SearchSummary` / `SearchSummarySSE` handler + 请求绑定/SSE 发送 helpers
- `backend/internal/router/router_ai.go` — 注册 `/ai/cmdb/search-summary` + `/ai/cmdb/search-summary-sse` 路由
- `backend/services/cmdb_search_summary_service.go` — **新增**：CMDB 搜索结果 AI 解读 + 筛查服务
- `backend/services/cmdb_search_summary_service_test.go` — **新增**：解读服务单元测试
- `backend/services/progress_event.go` — `ProgressEvent` 新增 `SearchSummary` 字段
- `backend/services/ai_config_service.go` — `IsAIAccessError` 新增 5 个 access hints（EOF/pipe/goaway/request failed）
- `backend/services/ai_config_fallback_test.go` — 新增 2 个测试用例（unexpected EOF / connection reset）
- `backend/services/embedding_worker.go` — `processPendingTasks` 新增无任务快速退出
- `backend/migrations/add_cmdb_search_summary_ai_config.sql` — **新增**：cmdb_search_summary AI config migration
- `backend/migrations/update_cmdb_search_summary_screening.sql` — **新增**：prompt 增加 dropped 筛查规则
- `backend/migrations/update_cmdb_search_summary_suggestions.sql` — **新增**：强化 suggestions 纯查询词规则
- `manifests/db/init_seed_data.sql` — 种子数据新增 ai_config id=24 + 同步 CREATE TABLE

### 前端

- `frontend/src/pages/CMDB.tsx` — AI 解读卡片 + SSE 进度 + 筛查交互 + 结果列表改造
- `frontend/src/pages/CMDB.module.css` — ~280 行新增样式（AI 卡片/进度条/筛查工具栏/剔除态）
- `frontend/src/services/cmdb.ts` — `searchSummary` / `searchSummarySSE` 接口 + 类型定义 + payload 构建
- `frontend/src/services/ai.ts` — `CAPABILITIES` / `CAPABILITY_LABELS` / `CAPABILITY_DESCRIPTIONS` / `DEFAULT_CAPABILITY_PROMPTS` 新增 `cmdb_search_summary`
- `frontend/src/pages/AIConfigForm.tsx` — 默认配置隐藏"新增场景"按钮 + 条件渲染修复

### 基础设施

- `docker-compose.nginx.conf` — Nginx SSE 代理配置（proxy_buffering off + proxy_cache off + chunked_transfer_encoding on）

---

## 技术细节（v0.8.1 CMDB 搜索解读）

### CMDB Search Summary 架构

```
用户搜索 → vectorSearch 返回结果 → fetchAISummary()
  ↓
searchSummarySSE (POST /ai/cmdb/search-summary-sse)
  ↓
[Step 1: 准备上下文] → 精简 payload（12 条 top + 字段截断）
  ↓
[Step 2: AI 解读与筛查] → cmdb_search_summary_service.Generate()
  ↓
  - GetConfigForCapability("cmdb_search_summary")
  - buildSearchSummaryPrompt (注入 query/results)
  - AI 调用（带 fallback 容灾）
  - parseSearchSummaryJSON（多策略解析 + 容错）
  ↓
[Step 3: 完成] → SSE complete 事件携带 SearchSummaryResult
  ↓
前端解析 dropped → 分流 keptResults / droppedResults → UI 渲染
```

### SSE 进度协议

对齐 manifest/form SSE 事件格式：
- `event: progress` — 步骤进度（step/total_steps/step_name/message/elapsed_ms）
- `event: complete` — 完成，`search_summary` 字段携带完整结果
- `event: error` — 错误，`error` 字段携带错误信息

### 筛查策略

- **AI 筛查**：模型返回 `dropped` 数组，每项含 `index`（对应请求 results 下标）+ `reason`
- **Fail-open 原则**：不确定时保留（"只剔除明显不符合查询意图的条目"）
- **不全部剔除**：prompt 规则明确"不要把全部结果都 drop 掉"
- **Suggestions 清洗**：前端 `normalizeAISearchSuggestion()` 去除引导前缀，只保留可回填搜索框的纯查询词

### Payload 优化

- 前端 `buildSearchSummaryPayload()` 只传必要字段（resource_type / resource_name / cloud_resource_id / resource_summary / similarity / is_resource_deleted），去掉 workspace/account 等噪声
- 单条文本截断：summary 72 字符 / name 64 字符 / description 60 字符
- 最多 40 条参与筛查，12 条送入 AI prompt（后端二次截断）

### 18. 前端设计系统 v3 — Design Token 统一与 UI 基元组件

建立全应用统一的设计 token 体系（`theme.css` 作为 single source of truth），引入语义化色阶命名（surface/ink/brand/green/red/amber），新增 Dialog/Button/Feedback CSS 基元组件，集成 Ant Design 5 `ConfigProvider` 全局主题，并将 ~228 个前端文件从硬编码 `--color-*` 变量迁移至语义 token；旧变量通过 alias 映射保持向后兼容。

**Design Token 体系：**

- **`theme.css`（新增）**：全局 `:root` token 定义 —
  - Surfaces & ink：`--bg` / `--surface` / `--surface-2` / `--surface-3` / `--line` / `--line-2` / `--ink` / `--ink-2` / `--ink-3` / `--ink-faint`
  - Brand scale（深青 #1c6e8c）：`--brand-100` ~ `--brand-700` + 语义别名 `--brand` / `--brand-ink` / `--brand-soft` / `--brand-line`
  - 独立语义色：green / red / amber（各自含 soft / line / hover / active 阶）
  - Blue 仅限非按钮场景（link / focus ring / progress）：`--blue` / `--blue-soft` / `--blue-line`
  - 同色系 alpha focus ring：`--ring-brand` / `--ring-green` / `--ring-red` / `--ring-amber` / `--ring-blue`
  - Status aliases：`--status-success` / `--status-danger` / `--status-warning` / `--status-info`（toast/banner/badge 统一引用）
  - Primary action aliases：`--color-primary` → `--brand`，兼容旧代码
  - Legacy 兼容层：`--color-gray-*` → surface/ink，`--color-blue-*` → brand scale，`--color-green-*` / `--color-red-*` → semantic 色，保证未迁移组件自动切到新色板
- **`tokens.ts`（新增）**：JS/TS inline style 专用 token 对象 — `colors` / `rings` / `statusColors` + `statusColor(type)` helper，与 `theme.css` 手动同步
- **`variables.css`（精简）**：移除全部硬编码 token，改为 `@import './theme.css'`；保留为兼容 shim（`App.css` 仍 import 此文件）
- **`v3-theme.css`（对齐）**：`[data-ui-version="v3"]` 作用域下的色阶从 Tailwind Slate / 蓝色系改为引用 `theme.css` 语义 token（`var(--bg)` / `var(--brand)` / `var(--green)` 等），v2/v3 色板统一

**CSS 基元组件：**

- **`buttons.css`（新增）**：`.btn` 基元 — 4 色（neutral/brand/green/red）× 3 档（solid/outline/ghost）× 4 尺寸（xs/sm/md/lg）矩阵；含 loading spinner、icon-only、count badge、btn-group / toolbar 组合；兼容旧 class `.btn.primary` / `.btn.brand` 映射到 `solid brand`
- **`dialog.css`（新增）**：`.tn-overlay` + `.tn-dialog` 弹窗壳 — 圆角卡片（12px），sm/md/lg/xl 四档宽度，default/info/warning/danger 四种 tone；含 Ant Design Modal / Popconfirm 对齐样式
- **`feedback.css`（新增）**：Toast（左色条卡片）/ Notice（填充条）/ Banner（inline alert）/ Badge（pill）四种反馈基元，各含 success/error/warning/info 变体
- **`antd-overrides.css`（新增）**：Ant Design CSS 软覆盖 — 统一 focus ring（`--ring-brand`）、Modal 圆角、Notification 左色条、Tag 语义色、Progress fill 色

**UI 基元组件：**

- **`ui/Dialog.tsx`（新增）**：Portal-based 统一弹窗壳 — 支持 `tone` / `size` / `closeOnOverlay` / `closeOnEsc` / `showClose` / `aria-label`；ESC 关闭 + body scroll lock + overlay click dismiss
- **`ui/Button.tsx`（新增）**：设计系统按钮组件（封装 `.btn` class 矩阵）
- **`ui/index.ts`（新增）**：统一导出

**Ant Design 5 主题集成：**

- **`antd-theme.ts`（新增）**：完整 `ThemeConfig` — 全局 token（colorPrimary=brand / colorSuccess=green / colorError=red / colorWarning=amber / colorLink=blue / 中性色=ink 系列 / borderRadius=6 / fontFamily=Inter）；组件级覆盖（Button / Modal / Popconfirm / Message / Notification / Tag / Input / Select / Tabs / Switch）
- **`App.tsx`（修改）**：顶层包裹 `<ConfigProvider locale={zhCN} theme={antdTheme}>`，全应用 Ant Design 组件自动对齐深青品牌色

**全局样式清理：**

- **`index.css`（精简）**：移除全部内联 token 定义（~90 行），改为 `@import` theme.css / buttons.css / feedback.css / dialog.css / antd-overrides.css；`:root` 仅保留 font / color / background 引用语义 token；移除 `button` / `h1` / `@media` 等 Vite 模板残留
- **`App.css`（迁移）**：`--color-gray-*` → `--ink` / `--bg`

**组件迁移（~228 文件）：**

- 所有 `.module.css` 文件中的 `var(--color-gray-*)` / `var(--color-blue-*)` / `var(--color-green-*)` / `var(--color-red-*)` 批量迁移至语义 token（`var(--ink)` / `var(--bg)` / `var(--brand)` / `var(--green)` / `var(--red)` 等）
- `ConfirmDialog.module.css` 删除 — 改用 `ui/Dialog` 基元 + `dialog.css` class
- `ConfirmDialog.tsx` 重构 — 基于 `Dialog` shell + 设计系统按钮，新增 `size` / `showClose` prop，deprecated `onClose` 保留兼容
- 多个组件 `.tsx` 文件中的 inline style 硬编码色值改用 `tokens.ts` 导出
- `SimpleNotification.tsx` / `SimpleToast.tsx` / `FeedbackBanner.tsx` 迁移至 feedback.css 基元 class

### 19. 样式 README 文档

- **`styles/README.md`（新增）**：设计系统使用指南 — token 层级说明、CSS 基元用法、组件集成示例

---

## 修改文件（v0.8.1 设计系统 + Token 迁移）

### 前端 — 设计系统核心

- `frontend/src/styles/theme.css` — **新增**：全局 design token 定义（surface/ink/brand/semantic 色阶 + focus ring + status alias + legacy 兼容层 + spacing/radius/shadow/font）
- `frontend/src/styles/tokens.ts` — **新增**：JS/TS token 对象（colors / rings / statusColors / statusColor helper）
- `frontend/src/styles/variables.css` — 移除硬编码 token，改为 `@import './theme.css'` 兼容 shim
- `frontend/src/styles/v3-theme.css` — 色阶引用对齐 `theme.css` 语义 token
- `frontend/src/styles/buttons.css` — **新增**：`.btn` 基元（4 色 × 3 档 × 4 尺寸 + loading/icon/badge/group/toolbar）
- `frontend/src/styles/dialog.css` — **新增**：`.tn-overlay` / `.tn-dialog` 弹窗壳 + Ant Design Modal/Popconfirm 对齐
- `frontend/src/styles/feedback.css` — **新增**：Toast / Notice / Banner / Badge 反馈基元
- `frontend/src/styles/antd-overrides.css` — **新增**：Ant Design focus ring / Modal / Notification / Tag / Progress 软覆盖
- `frontend/src/styles/antd-theme.ts` — **新增**：Ant Design 5 ThemeConfig（全局 token + 组件级覆盖）
- `frontend/src/styles/README.md` — **新增**：设计系统使用文档
- `frontend/src/components/ui/Dialog.tsx` — **新增**：Portal 弹窗壳组件
- `frontend/src/components/ui/Button.tsx` — **新增**：设计系统按钮组件
- `frontend/src/components/ui/index.ts` — **新增**：统一导出

### 前端 — 入口与全局样式

- `frontend/src/App.tsx` — 顶层包裹 `<ConfigProvider locale={zhCN} theme={antdTheme}>`
- `frontend/src/App.css` — `--color-gray-*` → `--ink` / `--bg` 迁移
- `frontend/src/index.css` — 移除内联 token，改为 `@import` 五个设计系统文件；移除 Vite 模板残留
- `frontend/src/main.tsx` — v3-theme import 注释更新

### 前端 — 组件迁移（CSS token 替换）

- `frontend/src/components/ConfirmDialog.tsx` — 基于 `Dialog` shell 重构，新增 `size` / `showClose` prop
- `frontend/src/components/ConfirmDialog.module.css` — **删除**（改用 `dialog.css` 基元）
- `frontend/src/components/SimpleNotification.tsx` — 迁移至 feedback.css 基元
- `frontend/src/components/SimpleToast.tsx` — 迁移至 feedback.css 基元
- `frontend/src/components/FeedbackBanner.tsx` — 迁移至 feedback.css 基元
- `frontend/src/components/TopBar.tsx` / `Layout.tsx` / `AuthProvider.tsx` / `ProtectedRoute.tsx` — token 迁移
- `frontend/src/components/DynamicForm/` — 全部 `.module.css` token 迁移
- `frontend/src/components/OpenAPIFormRenderer/` — 全部 `.module.css` + widget `.tsx` token 迁移
- `frontend/src/components/OpenAPISchemaEditor/` — `.module.css` + `index.tsx` token 迁移
- `frontend/src/components/ModuleSchemaV2/` — `.module.css` + `.tsx` token 迁移
- `frontend/src/components/ModuleReference/` — `.module.css` + `.tsx` token 迁移
- `frontend/src/components/SourceVersionCard/` — `.module.css` token 迁移
- `frontend/src/components/StateResourceViewer/` — `.module.css` token 迁移
- `frontend/src/components/HCLEditor/` / `HCLView/` — `.module.css` token 迁移
- `frontend/src/components/JsonDiff/` — `.module.css` token 迁移
- `frontend/src/components/DriftConfig/` / `DriftStatusTag/` — token 迁移
- 其余 ~100+ 组件 `.module.css` 文件 — 统一 `--color-*` → 语义 token 替换
- `frontend/src/pages/` — 全部页面 `.module.css` + 部分 `.tsx` 文件 token 迁移
- `frontend/src/pages/admin/` — 全部管理页面 `.module.css` + 部分 `.tsx` token 迁移
- `frontend/src/pages/admin/ManifestEditorV2/` — `.module.css` + `.tsx` token 迁移

---

## 技术细节（v0.8.1 设计系统）

### Token 架构分层

```
theme.css (single source of truth)
  ├── 语义 token: --bg / --surface / --ink / --brand / --green / --red / --amber / --blue
  ├── 语义别名: --status-success / --color-primary / --brand-ink / --brand-soft
  ├── Legacy 兼容: --color-gray-* → surface/ink, --color-blue-* → brand, --color-green/red-* → semantic
  ├── 布局 token: --spacing-* / --radius-* / --shadow-* / --font-*
  └── Focus ring: --ring-brand / --ring-green / --ring-red / --ring-amber / --ring-blue

tokens.ts (JS mirror)
  ├── colors: { bg, surface, ink, brand, green, red, amber, blue, ... }
  ├── rings: { brand, green, red, amber, blue }
  └── statusColors: { success, error, danger, warning, info }

antd-theme.ts (Ant Design bridge)
  └── ThemeConfig: global token + per-component overrides

v3-theme.css (v3 scope overlay)
  └── [data-ui-version="v3"] { --v3-gray-*: var(--bg/ink), --v3-blue-*: var(--brand-*) }
```

### 色板设计决策

- **Brand 深青 (#1c6e8c)** 作为唯一主色 — 主操作按钮、Tab 选中态、品牌强调均使用，替代旧蓝色系 (#3B82F6)
- **Blue 限定非按钮场景** — 仅用于链接文字、输入聚焦环、进度条 fill，避免与 brand 按钮混淆
- **语义色独立于品牌色** — green（成功）/ red（危险）/ amber（警告）不随 brand 变化，保证状态含义稳定
- **Legacy alias 策略** — `--color-blue-600` → `var(--brand-600)`，未迁移组件自动从蓝色切到深青，无需逐文件修改即可生效

### CSS 基元设计

- **Button 矩阵** — 通过组合 class（`.btn .solid .brand`）而非 CSS-in-JS 实现，与 Ant Design Button 互不干扰
- **Dialog shell** — `createPortal` 到 `document.body`，避免父级 `overflow: hidden` 裁切；统一 `.tn-overlay` + `.tn-dialog` class 便于全局样式覆盖
- **Feedback 基元** — Toast（左色条卡片）、Notice（填充条）、Banner（inline alert）、Badge（pill）四种形态覆盖所有反馈场景，每种形态含 4 色变体
