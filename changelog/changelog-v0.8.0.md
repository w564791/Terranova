本版本是 **Manifest 编辑器 V2 + UI v3 + HCL 双向编辑器** 的完整发布,涵盖 beta1 ~ beta4 的全部变更。核心交付:资源编辑引入 HCL 实时双向编辑器(overlay + Monaco 双引擎);Manifest 编辑器全面升级为 VS Code 风格的 IDE 体验(右侧停靠面板、WebSocket 日志流、文件树变更标记、gutter 行级 diff、深链定位);AI Skill 系统重构为 Domain/Module 分离 + 精确版本加载;AI 调用接入 Bedrock prompt caching;任务详情与 Plan diff 大幅重构;跨 workspace manifest output 引用、权限校验、种子 SQL 等多处修复。

---

## Highlights

### 1. HCL 双向实时编辑器(beta1)

资源编辑从 JSON 编辑升级为 HCL 双向实时编辑,两套引擎共存:

- **Monaco 引擎**(`MonacoHclEditor`):完整 IDE 能力(语法高亮、自动补全、Hover、跳转定义、Inlay Hints、Code Actions),EditResource 默认使用
- **overlay 引擎**(`HCLEditor`):透明 `<textarea>` 覆盖在语法高亮 `<pre>` 上,轻量场景用
- **jsonencode() 双向转换**:`format: "json"` 字段自动渲染为 `jsonencode({ ... })`,parser 反向解析;支持 AWS 风格冒号标识符(`aws:PrincipalArn`)
- **Terraform 系统参数分离**:`for_each`/`count`/`depends_on`/`providers`/`lifecycle` 自动分离,不进入表单;HCL 中写了 Schema 外字段时显示黄色警告 + 保留/丢弃操作
- **Terraform 表达式**:`${var.xxx}` 等表达式不加引号直接输出
- **Monaco 滚动隔离**:三层策略(`alwaysConsumeMouseWheel:false` + capture 边界交接 + bubble 内部隔离),编辑器与页面滚动不互相吞没

### 2. UI v3 主题系统(beta1)

- **v2/v3 切换**:`useUIVersion` hook + `localStorage` 持久化 + URL 参数 `?ui=v3` 同步
- **CSS 变量隔离架构**:`<html data-ui-version="v3">` 选择器下覆盖,v2 零影响
- **全量页面适配**:EditResourceDialog / DemoPreview / AddResources / CreateDemo / SchemaManagement / WorkspaceResources / ResourceVersionDiff 全部 v3 条件渲染

### 3. 版本管理与升级/降级流程(beta1)

- 版本选择器(按 semver 降序),版本低于最新时显示 `[X → Y ↗]` 升级药丸
- 升级/降级确认对话框 → 跳转 EditResource 带版本编辑 → 用户确认后正式变更版本
- **版本锁定策略**:编辑模式锁定到资源当前版本,严禁跨版本渲染;版本不匹配时强制 HCL 视图 + 红色警告
- **Schema 感知版本对比**:`typejsonstring` 字段识别、jsonencode 格式化、LCS 逐行 diff;支持 `?mode=compare&compare_from=N&compare_to=M` URL 直达

### 4. Manifest 编辑器 V2 交互体验(beta2 / beta3)

**右侧停靠面板系统**(四面板互斥:AI 生成 / 检查 / Run / 部署):

- **Run 面板**:WebSocket 实时日志流(`ws://.../tasks/:id/output/stream`)+ HTTP 轮询兜底;日志分色;状态徽章实时反映;终态自动停止
- **部署面板**:从全屏覆盖式改为右侧停靠,移除全部 antd 组件,Variable Sets chip 多选,卸载确认内联化
- **发布版本跳过检查**:可跳过 AI 检查直接发布
- **AI 修复逐条应用**:`fixedIndices: Set` 每条 issue 独立启用/禁用
- **面板拖拽宽度**(beta3):250-700px 可拖,共享宽度设置
- **状态栏任务入口**:`lastRunTask` 持久化 + 点击直达上次运行日志

**变更可视化**(beta3):

- **文件树状态徽章**:新建 `U`(绿)/ 修改 `M`(橙),VS Code SCM 风格
- **Gutter 行级 diff**:彩色竖条标记新增(绿)/ 修改(橙),与上次发布版本对比,实时更新

**编辑器/文件操作**:

- **文件夹拖拽/粘贴上传**:递归 `FileSystemEntry` 遍历保留目录结构 + 上传进度(beta4)
- **移除全局复制限制**:仅 chrome 元素禁选,内容区域可自由选中复制(beta2)

### 5. AI Skill 系统重构(beta2 / beta3)

- **Domain/Module Skill 分离**:Domain Skill(AI 语义选最佳实践)与 Module Skill(精确版本匹配配置知识)彻底分离;一次 AI 调用同时选择 domain skills + modules(含版本)
- **Module Skill 精确加载**:数据源从 `skills` 表遗留 `module_%d_auto` 迁移到 `module_version_skills` 表;按 AI 选中 module + 版本加载,优先指定版本,回退默认版本
- **Pipeline 扩展(4 → 5 步)**:新增独立"Skill 选择"步骤,前端耗时展示
- **遗留代码清理**:删除 `generateNewModuleSkill` / `BatchGenerateModuleSkills` / `skills` 表写入路径

**AI 能力增强**(beta3):

- **AI 对话上下文历史**:`ConversationTurn` 结构,最近 12 轮对话注入 prompt(单条限 2000 字符)
- **Bedrock Claude prompt caching**:system prompt 加 `cache_control`,相同前缀 5 分钟内复用享 90% input token 折扣;OpenAI 因 prompt 稳定也受益
- **可配置 caching**:`ai_configs.cache_enabled` 控制开关(beta4)
- **检查用户意见输入**:`UserInstruction` 字段(限 2000 字符),底部 textarea + context chip(文件/选区),Cmd/Ctrl+Enter 触发
- **检查面板重构为对话**:会话管理(创建/切换/删除)+ 历史消息流 + kind 过滤

### 6. Manifest 资源管理增强(beta1 / beta4)

- **删除保护**:manifest 管理的资源禁止直接删除,后端 409 + 前端拦截 + 按钮 disabled(beta1)
- **深链跳转**:版本感知深链(`buildManifestEditorUrl` 携带 version/subpath/resource),草稿找不到时 fallback 到发布版本(beta1)
- **Workspace Manifest Summary**:增加 `version_id` 字段 + `listVersionFiles` API(beta1)
- **跨 workspace manifest output 引用修复**(beta4):manifest workspace 的 output 由 `.tf` 管理,原用 `workspace_outputs` 表过滤导致引用拿到空 outputs;改为 manifest 分支透传 state 全部 outputs,前端按 `manifest_active_tag` 隐藏 manifest 下不该出现的子 tab

### 7. 任务详情页 + Plan diff 重构(beta4)

- **资源行与变更图标**:移除彩色状态条,折叠箭头改尖括号,放大 `+`/`~`/`−`,移除文字 badge
- **操作按钮统一品牌色**:confirm/cancel/add-comment/override 主操作统一品牌蓝,次操作改 ghost,移除渐变
- **统计区扁平化 + output 变更深色块**:任务标题与统计区改无边框内联,output changes 改深色终端块风格
- **Plan 详情 UI 重构**:create/delete 复杂值统一 HCL 展示;update/replace 改行级 `+`/`−`/`~` 标记 HCL 树(`normalize → sort → diff → render`);未变更字段默认折叠、扁平铺开
- **known-after-apply 修复**:UPDATE/REPLACE diff 中 before 有值、after 缺失且 `after_unknown=true` 的字段判为 `modified`(渲染 `~ before -> known after apply`),不再误判为删除红色
- **影响分析块**:移除左侧风险色条,改为各级别背景色

### 8. 后端增强

- **terraform init locked provider 自动重试**:检测 `"locked provider"` 错误后自动带 `-upgrade` 重试(beta1)
- **state 上传后异步 CMDB 同步**:确保 workspace 资源被 embedding 索引和搜索收录(beta4)
- **权限校验 bypass 对齐**:`CheckPermission` handler 补系统管理员 bypass,与中间件逻辑对齐(beta4)
- **后端日志增强**:Bedrock 请求指纹(model/system_hash/tools_hash/messages 数),CheckDraftSSE 增加 `user_instruction`/`session_id`/`history_count`(beta3)

### 9. 数据一致性治理(beta4)

- **种子 SQL schema drift 修复**:`workspace_tasks` 补 `snapshot_manifest_version_id` 列;`ai_configs` 的 `cache_enabled` 默认值 `false → true`;新增 `ai_configs` ID 22/23 幂等 INSERT
- **Skill 数据一致性**:32 个 skill 的 DB `content` 字段统一移除 frontmatter,只保留正文;`.md` 文件 frontmatter 格式规范化 + HTML 注释边界;补建 `cmdb_query_plan_workflow.md` / `cmdb_resource_types.md` / `schema_validation_rules.md` / `platform_introduction.md`;`infrastructure_risk_baseline` 新增 3 条 Critical 风险规则

### 10. Bug Fixes

- **Manifest 编辑器 demo 标签(inlay hint)重复**:`module "x" {` 行尾 `· N demos` 标签编辑时一行渲染多份。两个原因:① HMR 重算模块抹掉模块级 `registered` 守卫,但 Monaco 全局 provider 表不重置,导致每次热更叠加一份 provider —— 改用 `globalThis` 存 disposable + 重注册前 dispose 旧的;② `provideInlayHints` 忽略 Monaco 传入的 `range`,返回范围外 hint —— 改为只返回落在请求可见范围内的 hint(beta4)
- **行级变更色块(added/modified)分类错乱**:`computeLineDiff` 反向 LCS 回溯贪心匹配最右同内容行 + `pubIdx` 单游标分类,导致纯新增空行/重复行错标 modified、插入内容与已有行重复时被误判为 unchanged(新行无色)。改为正向回溯产出有序 match/insert/delete 序列(左对齐匹配)+ 按 hunk 配对分类:纯插入 = added,插入+删除混合 = modified(beta4)
- HCL overlay 文本对齐 / blur 误关闭 / Monaco CSS 加载 / Monaco 初始化稳定性 / onChange 闭包(beta1)
- HCL 系统参数丢失与误报为额外字段 / 额外字段确认后数据不一致 / Terraform 表达式被加引号 / schema 查找路径不全(beta1)
- SwitchWidget v3 `valuePropName` 导致开关无法操作(beta1)
- Manifest 编辑器面板内容无法选中复制(beta2)
- 跨 workspace manifest output 引用拿到空 outputs / workspace 接口漏序列化 `manifest_active_tag`(beta4)

---

## 版本演进(beta 阶段)

| 版本 | 主题 |
| --- | --- |
| v0.8.0-beta1 | UI v3 主题系统、HCL 双向编辑器、版本管理升级流程、Manifest 资源删除保护与深链 |
| v0.8.0-beta2 | Manifest 编辑器右侧停靠面板(Run/部署)、AI Skill Domain/Module 分离重构 |
| v0.8.0-beta3 | 面板拖拽宽度、AI 对话上下文、Bedrock prompt caching、用户检查意见、文件变更标记、gutter diff |
| v0.8.0-beta4 | 种子 SQL 同步、跨 workspace manifest output 引用修复、任务详情 UI 重构、Plan diff 重构、demo 标签与色块修复 |

详细的逐 beta 变更见 `changelog-v0.8.0-beta1.md` ~ `changelog-v0.8.0-beta4.md`。
