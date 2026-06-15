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

### 其他变更 (已提交)

#### Manifest 编辑器文件粘贴/拖拽

- **新增** 递归 `FileSystemEntry` 遍历:支持拖拽文件夹粘贴,保留目录层级结构
- **新增** 上传进度:状态栏显示文件上传进度百分比

#### State 上传后异步 CMDB 同步

- **新增** `state_service.go`:state 上传完成后异步触发 CMDB 资源同步,确保 workspace 资源被 embedding 索引和搜索收录

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
- `frontend/src/pages/admin/ManifestEditorV2/ManifestEditorV2.tsx` — 文件夹拖拽粘贴 + 上传进度

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
