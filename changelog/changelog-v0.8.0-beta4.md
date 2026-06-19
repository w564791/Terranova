## v0.8.0-beta4

数据库种子数据同步修复 + Skill 数据一致性治理:以本地运行数据库为准,补齐种子 SQL 中缺失的表结构、`ai_configs` 配置和 `skills` 数据;修复 3 处 migration 未同步到 seed SQL 导致的 schema drift;统一 Skill 本地 `.md` 文件与 DB `content` 的 frontmatter 边界;补充 `infrastructure_risk_baseline` 新增规则。

### 种子 SQL 同步修复

#### 表结构对齐 (Schema Drift Fix)

- **修复** `workspace_tasks` 表:CREATE TABLE 补充 `snapshot_manifest_version_id character varying(36)` 列,与 `add_snapshot_manifest_version_id.sql` migration 对齐
- **修复** `ai_configs` 表:CREATE TABLE 中 `cache_enabled` 默认值从 `false` 改为 `true`,与 `add_cache_enabled.sql` migration 对齐
- **问题** 使用种子 SQL 全新初始化数据库时,缺失列或默认值不一致会导致 migration 执行异常或功能行为偏差

#### 缺失数据补充

- **新增** `ai_configs` ID 22 (`manifest_resource_generation`) 和 ID 23 (`manifest_check`) 的 INSERT 语句,使用 `ON CONFLICT (id) DO NOTHING` 幂等写入
- **修复** `skills` COPY 块中 22 条 skill 的 `content` 字段:移除误包含的 frontmatter (`---\n...---\n`),只保留正文内容
- **修复** `skills` 中 `execute_summary_workflow` 的 content:移除 `-----` 格式的 frontmatter
- **效果** 全新初始化后 skills 表的 32 条数据与本地运行数据库完全一致

### Skill 文件治理

#### Frontmatter 边界规范化

- **新增** 32 个 `.md` 文件的 frontmatter 内部统一添加 HTML 注释:`<!-- 该部分内容只是为了说明skill用途以及作用域,不要复制到skill正文里 -->`
- **修复** `execute_summary_workflow.md`:frontmatter 分隔符从 `-----` 改为标准 `---`,`## name:` 改为 `name:`
- **修复** `skill_quality_rule_evaluation.md` / `skill_quality_semantic_evaluation.md`:同上格式修正
- **效果** frontmatter 元数据(用途/作用域)与 skill 正文有明确边界,防止 skill 加载时将元数据误当作 prompt 内容

#### 数据库 Content 清理

- **修复** 32 个 skill 的 DB `content` 字段:统一移除 frontmatter,只保留正文内容
- **修复** `infrastructure_risk_baseline`:DB content 同步到本地 `.md` 文件,新增"任何端口范围缩小的行为"、"源地址更改"、"计算资源"三个 Critical 风险规则
- **效果** `skills.content` 字段不再包含 frontmatter 元数据,与 SkillAssembler 加载逻辑一致

#### 缺失本地文件补建

- **新增** `skill/cmdb/task/cmdb_query_plan_workflow.md` — CMDB 查询计划生成工作流
- **新增** `skill/resource_generation/domain/cmdb_resource_types.md` — CMDB 资源类型映射表
- **新增** `skill/resource_generation/domain/schema_validation_rules.md` — Schema 验证规则
- **新增** `skill/resource_generation/foundation/platform_introduction.md` — 平台基础介绍

### 跨 workspace manifest output 引用修复(b011098)

manifest-managed workspace 的 output 由 `.tf` 文件管理,不走 `workspace_outputs` 表,导致两个问题:

- **后端** `getCrossWorkspaceState` 原用 `workspace_outputs` 的 key 过滤 state outputs,manifest workspace 该表为空 -> 其他 workspace 通过 `terraform_remote_state` 引用时拿到空 outputs。Select 列追加 `manifest_deployment_id`、`manifest_active_tag` 判定 `isManifest`:manifest 分支直接透传 state 全部 outputs,正常模式仍按声明 key 过滤。workspace 级授权(`outputs_sharing` + 访问名单)仍前置 gate,日志区分 `mode=manifest/configured`
- **前端** workspace 详情接口漏序列化 `manifest_active_tag`,前端 `isManifest` 恒为 false。`GetWorkspace` 补回该字段后:`WorkspaceOutputs` 在 manifest 下隐藏 output 增删改子 tab、默认切到 remote-data;`WorkspaceRemoteDataConfig` 在 manifest 下仅保留 Outputs Sharing 卡,隐藏 Remote Data References 引用配置

### 权限校验 bypass 对齐(b21a364)

- **修复** `CheckPermission` handler 缺少中间件层的系统管理员 bypass:系统管理员命中后直接返回 `IsAllowed=true` / `EffectiveLevel=admin` / `Source=system_admin`,与中间件 bypass 逻辑对齐

### 任务详情页 UI 精修(ca330a6)

#### 资源行与变更图标

- **移除** 资源行左侧彩色状态条;折叠箭头由三角(▶/▼)改为尖括号(›/∨)
- **放大** `+`/`~`/`−` action 图标(不再加粗);移除 Create/Update/Delete 文字 badge,保留类型 tag
- **改写** 资源变更摘要为窄条按比例分段条;run triggers 从顶部统计卡移入 plan 卡,以超链接 workspace 列表展示(仅当存在时)

#### 操作按钮统一品牌色

- **统一** confirm / cancel / add comment / cancel-previous / override 按钮:主操作用品牌蓝(`--color-blue-600`),次操作改为无边框 ghost 样式
- **移除** 原渐变背景与 hover 位移阴影(绿/红/橙/紫渐变),改为实色 + 灰边

#### 统计区扁平化 + 输出变更深色块

- **重构** 任务标题与统计区:从带卡片背景的独立块改为两行无边框布局,统计项内联 `num | label` 并以竖线分隔;内容外边距由 20px 调到 28px
- **重构** output changes 段落改为深色终端块风格(`StructuredRunOutput` / `TaskTimeline` / `ApplyingView` 配套样式调整)
- **清理** 不再使用的 `.statsCards` / `.statCard` 等响应式规则

### Plan diff / 影响分析 修复(后续微调)

- **修复** UPDATE/REPLACE diff 中 known-after-apply 字段被标红:ca330a6 重构 `diffNode` 时丢失了旧版对 `after_unknown` 的处理,导致 before 有值、after 缺失且 `after_unknown[key]=true` 的字段被当成删除,渲染成红色 `−`。改为把 `after_unknown` 线程化进 `diffNode`,此类字段判为 `modified`,渲染 `~ before -> known after apply`;真删除仍保持红色。同时修正多行 before 块时 `-> known after apply` 标记跑到第一行的问题(before 块与尾标记包进 inline 流容器,`.diffTrailing` 用 `vertical-align: bottom` 贴到块最后一行)
- **移除** 影响分析块(plan summary)的左侧风险色条:`.impactBlock` 去掉 `border-left`,圆角改回四周 6px;`.impact_critical/high/medium/low` 仅保留各级别 `background-color`,不再随级别上左竖条色。标题字色级别区分不变

### Plan 详情 UI 重构

#### 复杂值统一 HCL 展示(create/delete)

- **重构** create/delete 资源的属性渲染:复杂对象/数组统一走 `toHcl` 序列化为多行 HCL 块,移除旧的 `[ { } ]` 结构化缩进分支(`renderArrayBlock`)
- **新增** `toHcl` 递归过滤空值(null/空数组/空字符串/空对象),去掉 `and = []`、`object_size_greater_than = null` 这类无意义噪音
- **新增** 键字母序排序,保证任意输入下字段顺序一致
- **修复** delete 资源复杂值颜色:新增 `.jsonValueDelete`(红),`renderValue` 按 variant 取色,修正 delete 复杂值此前被渲染成绿色的问题
- **新增** `.jsonValue` 行间距(line-height 1.6),多行 HCL 块在 create/delete 下也可读

#### UPDATE/REPLACE 行级标记 diff

- **重构** update/replace 资源:移除 before/after 两列对比,改为行级 `+`/`−`/`~` 标记的 HCL 树
- **新增** 数据流水线 `normalize → sort → diff → render`:
  - `normalize`:把 `jsonencode` 等字符串化 JSON 解析回对象/数组(覆盖 json / string-json / jsonencode 几种值类型),其余原样
  - `sort`:对象键字母序排序,保证 diff 对齐与展示顺序一致
  - `diffNode`:递归深 diff,对象键并集逐键比较,数组按索引配对(余量标 added/removed)
- **新增** 标量修改内联显示 `~ key = before -> after`(旧值灰、新值蓝);计算属性(after 为空)显示 `(known after apply)`
- **新增** 未变更字段默认折叠(`Show N unchanged elements` toggle),点击展开时**扁平铺开**(去掉 `max-height`/`overflow` 滚动窗口,跟随页面滚动)
- **删除** 不再使用的 `computeChanges` / `renderNestedChanges` / `renderValueComparison` / `renderHeaderIcon` / `isComplexObject`

#### Action 资源配置值

- **重构** `Action Invocations` 的 Configuration 渲染:去掉"一参数一灰底块",改为扁平行(`simpleAttrsGrid`/`simpleAttrRow`),与资源详情一致
- **新增** key 列对齐(`keyColVar`),行间距统一(line-height 1.6)
- **新增** 结构化值(对象/数组/jsonencode 字符串)统一走 HCL,排序、过滤空值,多行展示
- **保持** Action 紫色主题不变(配置值仍用 `#1f2937`,未引入资源 diff 的绿/红/蓝)
- **保留** "Triggers Actions:" 等固定关键字标签不动

### 其他变更 (已提交)

#### Manifest 编辑器文件粘贴/拖拽

- **新增** 递归 `FileSystemEntry` 遍历:支持拖拽文件夹粘贴,保留目录层级结构
- **新增** 上传进度:状态栏显示文件上传进度百分比

#### State 上传后异步 CMDB 同步

- **新增** `state_service.go`:state 上传完成后异步触发 CMDB 资源同步,确保 workspace 资源被 embedding 索引和搜索收录

### Manifest 编辑器 demo 标签重复 + 变更色块错乱修复

`module "x" {` 行尾的 `· N demos` demo 选择标签在编辑时一行渲染多份,两个独立原因一并修复:

- **修复** HMR 叠加 provider:`hclProviders.ts` / `hclCompletion.ts` / `hclDefinitions.ts` 原用模块级 `let registered` 防重复注册,Vite HMR 重算模块时该变量被抹掉,而 Monaco 全局 provider 注册表不重置,导致每次保存源码热更都叠加一份 provider。改为把 disposable 存到 `globalThis`,重注册前先 dispose 旧的(与 `initServices.ts` 对 vscode-api `initialize` 的兜底思路一致)
- **修复** InlayHint 范围外重复:`provideInlayHints` 原忽略 Monaco 传入的 `range`,每次都扫整个 model 返回所有 hint;滚动 / 草稿自动保存触发模型版本变化、对新可见范围重新请求时,范围外 hint 会让同一个标签渲染多份。改为接收并尊重 `range`,只返回落在请求可见范围内的 hint(边界外扩 ±3 行)
- **修复** 行级变更色块(added/modified)分类错乱:`computeLineDiff` 原用反向 LCS 回溯(贪心匹配最右的同内容行)+ `pubIdx` 单游标分类,导致纯新增空行 / 重复行 / 尾部追加被错标成 modified;插入内容与已有行重复时还会被 LCS 匹配成 unchanged(新行无色、原行反被标 modified)。改为正向回溯产出有序 match/insert/delete 操作序列(左对齐匹配),再按改动 hunk 配对分类——纯插入 = added,插入 + 删除混合 = modified

### 修改文件

- `manifests/db/init_seed_data.sql` — workspace_tasks 补列 + ai_configs 默认值修正 + 新增 ID 22/23 INSERT + skills COPY 块去 frontmatter
- `skill/security/foundation/infrastructure_risk_baseline.md` — 同步 DB 新增 Critical 风险规则 + frontmatter 注释
- `skill/execute_summary/task/execute_summary_workflow.md` — frontmatter 格式修正 + 注释
- `skill/quality_assessment/task/skill_quality_rule_evaluation.md` — frontmatter 格式修正 + 注释
- `skill/quality_assessment/task/skill_quality_semantic_evaluation.md` — frontmatter 格式修正 + 注释
- 其余 27 个 `skill/**/*.md` — 仅添加 frontmatter 注释
- `skill/cmdb/task/cmdb_query_plan_workflow.md` — 新建
- `skill/resource_generation/domain/cmdb_resource_types.md` — 新建
- `skill/resource_generation/domain/schema_validation_rules.md` — 新建
- `skill/resource_generation/foundation/platform_introduction.md` — 新建
- `backend/services/state_service.go` — state 上传后异步 CMDB 同步
- `frontend/src/pages/admin/ManifestEditorV2/ManifestEditorV2.tsx` — 文件夹拖拽粘贴 + 上传进度;行级变更色块 `computeLineDiff` 改为正向回溯 + hunk 配对分类(added/modified)
- `frontend/src/components/PlanCompleteView.tsx` — create/delete 复杂值统一 HCL 展示;update/replace 行级标记 diff(normalize/sort/diff);删除冗余渲染函数
- `frontend/src/components/PlanCompleteView.module.css` — `.jsonValueDelete` / `.diffTree` / `.diffRow` / `.diffKey` / `.diffBrace` / `.diffValueBefore` 等样式
- `frontend/src/pages/admin/ManifestEditorV2/hclProviders.ts` — provider disposable 改存 globalThis 防叠加;`provideInlayHints` 尊重请求 range
- `frontend/src/pages/admin/ManifestEditorV2/hclCompletion.ts` — 同上 disposable 防叠加
- `frontend/src/pages/admin/ManifestEditorV2/hclDefinitions.ts` — 同上 disposable 防叠加

### 技术细节

#### Schema Drift 问题根因

- **问题**:migration 文件(`add_cache_enabled.sql`、`add_snapshot_manifest_version_id.sql`)添加了新列,但 `init_seed_data.sql` 的 CREATE TABLE 语句未同步更新
- **影响**:升级路径(upgrade)通过 migration 正常,但全新初始化(init)路径缺失列,导致后端启动报错或功能异常
- **方案**:逐项对比本地运行 DB 的 `\d` 输出与 seed SQL 的 CREATE TABLE,补齐所有缺失列和默认值差异
- **规则**:后续新增 migration 时必须同步更新 `init_seed_data.sql` 中对应的 CREATE TABLE 语句

#### Skill Frontmatter 与 Content 边界

- **问题**:部分 skill 的 DB `content` 字段包含了 `---\nname: ...\n---\n` 的 frontmatter,而另一部分不包含,导致 SkillAssembler 加载时行为不一致
- **方案**:统一规范 — `.md` 文件保留 frontmatter(用于人类阅读和工具解析),DB `content` 字段只存正文(AI 实际使用的 prompt 内容)
- **标注**:frontmatter 内添加 HTML 注释说明其用途,防止编辑时误将元数据复制到正文
