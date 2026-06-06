## v0.7.1

Manifest 编辑器引入 AI 助手:基于平台 capability 框架的「资源生成/修复」与「草稿检查」两项能力,均走 SSE 实时分步进度,与 form_generation 共用底层积木(意图断言、SkillAssembler、callAI、用量日志)。生成根据自然语言产出 HCL 并插入光标/选区、优先复用 Module 库;检查输出可点击跳转、可一键修复的结构化问题列表,并对引用了仓库已有 module 的块做 schema 校验。AI 面板为 VS Code 风格右侧停靠、挤占式布局(不遮挡编辑器),对话历史按 (manifest + 用户) 隔离持久化、支持多会话切换。workspace 资源列表中 manifest 来源的资源点击可直接跳转到编辑器对应块。

### Enhancements

#### Manifest AI 助手 — 资源生成/修复

- **新增** `manifest_resource_generation` 能力 + SSE 端点 `POST /api/v1/ai/manifest/generate-resource-sse`
  - 流程:初始化 → 意图断言(安全守卫)→ 列出 Module 候选 → 组装 Skill + AI 生成 → schema 校验
  - 自然语言描述生成 Terraform HCL,通过 Monaco `executeEdits` 插入光标/替换选区
  - **优先复用 Module 库**:把全部 active module(name/source/description)列入 prompt 交 AI 择优,匹配则生成 module 引用块、否则生成原生 resource(中文描述也能正确对应,如「s3桶」→ s3-bucket module)
  - 生成后对引用了仓库已有 module 的块做 schema 校验(按 source + version 取 schema,无 version 用默认版),每块一 goroutine 并发(限流),问题以 warning 返回;resource 块与仓库无的 module 跳过
- **新增** AI 工具面板(`ManifestAiTools`):VS Code 风格右侧停靠聊天面板,空态引导 + 分步进度 + 上下文 chip(选区优先,显示文件:行范围)+ 停止/关闭

#### Manifest AI 助手 — 草稿检查

- **新增** `manifest_check` 能力 + SSE 端点 `POST /api/v1/ai/manifest/check-sse`
  - 流程:初始化 → 意图断言(选区可能含 prompt injection,不跳过)→ 打包 → 组装 Skill + AI 检查
  - 输出结构化问题列表(file / line / level / message),底部「问题」面板渲染,点击跳转到对应 file:line
  - **一键修复**:AI 给出结构化修复(行范围替换)的问题带「修复」按钮,点击应用到编辑器并移除该项;应用任一修复后其余置灰并提示重新检查(放弃脆弱的撤销追踪)
  - **跨文件检查**:无选区时除当前文件外,自动带上当前文件引用了、但定义在别处的符号所在文件(var/local/module/data/resource 五类,上限 5 个)
  - **行号前缀**:打包时给每行加真实行号前缀,AI 直接引用而非自行计数,根治行号漂移/选区偏移
  - **按钮文案随选区切换**:有选区「检查选中」,无选区「检查文件」
  - check pipeline 分步展示(默认折叠,可展开看各步耗时 + 加载的 Skill),完成后保留「完成 · N 步 · 耗时」摘要
  - 检查结果可一键复制为纯文本

#### Manifest AI 助手 — 会话持久化

- **新增** AI 对话会话:按 (manifest + 用户) 隔离,后端持久化,支持多会话新建/切换/删除/查看历史
  - 会话 API:`GET/POST /api/v1/ai/manifest/sessions`、`GET /sessions/:sid/messages`、`DELETE /sessions/:sid`,所有查询强制 `user_id` 过滤(用户隔离)
  - 生成/检查完成后落「用户输入 + AI 产出」两条消息;历史回放按 kind 渲染(生成显示 HCL、检查显示可跳转问题列表)
  - 进入面板默认续最近一条会话;非属主 session_id 静默跳过,不写入他人会话

#### Manifest 面板布局 + 资源跳转

- **重构** AI 面板由悬浮(absolute)改为**挤占式**:展开时编辑器区右移收窄让位、折叠恢复全宽,不再遮挡代码;切换后调 `editor.layout()` 重算 Monaco 尺寸
- **新增** workspace 资源列表中 manifest 来源资源点击跳转:跳到 manifest 编辑器并定位到对应 module/resource 块首行(`?resource=` 深链),不再进入会报「无法获取 Module 信息」的旧详情页;manifest 信息加载中时提示稍候而非误跳

#### AI 框架 / Skill

- **新增** Domain Skill 的 AI 动态选择(manifest 侧,受 `use_optimized` 开关控制):按 skill description 选相关 domain skill,中性 prompt 不排除 CMDB 类;关闭时按 `domain_skill_mode` 走
- **新增** Module Skill 加载(受 `auto_load_module_skill` 开关控制):开启时把召回候选 module 的配置知识载入 prompt
- **新增** manifest 两个 task skill(`manifest_resource_generation_workflow` / `manifest_check_workflow`):确立「Foundation 层 = 最高优先级硬约束、冲突以 Foundation 为准、逐资源逐规则核对」纪律;检查 skill 强调自检静默、只报真实违规、合规不产 issue(避免把核对过程当问题输出)
- **新增** manifest 两个 capability 的专用 AIConfig + 启用 MetaRules(prompt 顶部注入优先级层级 + 冲突以 Foundation 为准 + 按层分段标注)
- **新增** 前端 AIConfig 表单注册 manifest 两个场景 + 纳入 Skill 模式 / 优化版开关白名单

#### SchemaSolver / HCL 解析

- **新增** `NewSchemaSolverWithVersion`:支持按 `module_version_id` 取指定版本 schema(不改旧构造器,form 行为不变)
- **新增** `ParseManifestModuleBlocks`:从 HCL 文本解析 module 块全参数(cty → Go),记录出现过的参数名以避免把「引用变量的必填参数」误报为缺失

### Bug Fixes

- **修复** workspace 中 manifest 来源资源打开报「无法获取 Module 信息」(旧 ViewResource 假设所有资源都有 `tf_code.module`,manifest 资源没有)

### Database Migration

- `manifest_ai_sessions`(新表):AI 会话,按 (manifest + 用户) 隔离,`id varchar(64)`,索引 `idx_mas_lookup (manifest_id, user_id, updated_at DESC)`
- `manifest_ai_messages`(新表):会话消息,`role`/`kind`/`content(jsonb)`,索引 `idx_mam_session (session_id, created_at)`
- `manifest_ai_sessions` / `manifest_ai_messages`:`id` 与 `session_id` 列宽 `varchar(64)`(容纳 `mas-`/`mam-` 前缀 + uuid)
- 迁移脚本:
  - `backend/migrations/add_manifest_ai_sessions.sql`(会话/消息表)
  - `backend/migrations/add_manifest_ai_skills.sql`(两个 task skill,幂等 ON CONFLICT)
  - `backend/migrations/add_manifest_ai_configs.sql`(两条专用 AIConfig + MetaRules)
  - `manifests/db/init_seed_data.sql`(schema 增量 + skill 种子同步)
