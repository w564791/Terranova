# Manifest AI 助手设计方案

## 一、概述

### 1.1 背景
Manifest 编辑器(VS Code Web)已具备文件树、草稿、版本、部署能力。为降低编写 Terraform HCL 的门槛、提升草稿质量,给编辑器加入 AI 能力:根据自然语言生成/修复资源、检查草稿基本问题。AI 方案复用平台既有的 capability 驱动框架,不重造轮子。

### 1.2 核心目标
- **生成/修复**: 自然语言描述 → 生成 HCL,插入光标/选区;选中代码可基于选区修复
- **检查(Check)**: 检查当前文件/选区的基本问题,输出可点击、可一键修复的结构化问题列表
- **质量对齐**: 让生成/检查对齐 form_generation 的 AI 质量(domain skill 动态选择、module skill 上下文、schema 校验)
- **资源定位**: workspace 资源跳转到 manifest 编辑器对应块

### 1.3 设计决策
- **复用 AI 框架不改 form**: 平台 AI 框架是 capability 驱动(已有 14 个 capability 跑同一地基)。manifest 新增独立 capability 与 service,复用底层积木(意图断言 / SkillAssembler / callAI / 用量日志 / SSE 协议),**不改动 form_generation 任何代码**,避免回归。
- **不复用 form 业务层**: form 的 `AICMDBSkillService` 硬绑单 moduleID + SchemaSolver,输出表单 config。manifest 是自由 HCL、多 module/无 module,新建 `ManifestAIService`/`ManifestCheckService`,产出 HCL/问题列表。
- **流程骨架与 form 一致**: 初始化 → 意图断言(安全守卫)→ 上下文准备 → 组装 Skill → AI 调用,全程 SSE 分步推进度。
- **AIConfig 运行时配置**: 两个 capability 走全局兜底或专用配置(id 22/23),`mode=skill`,UI 可调 skill 组合。

---

## 二、核心概念

### 2.1 两个 capability / 两个 SSE 端点

| capability | 端点 | 用途 |
|-----------|------|------|
| `manifest_resource_generation` | `POST /api/v1/ai/manifest/generate-resource-sse` | 生成/修复资源,产出 HCL |
| `manifest_check` | `POST /api/v1/ai/manifest/check-sse` | 检查草稿,产出问题列表(含修复) |

### 2.2 SSE 协议
复用 `ProgressEvent`,新增 omitempty 字段不影响 form:
- `hcl` — 生成结果(HCL 文本)
- `issues[]` — check 结果,`ManifestIssue{ file, line, level, message, fix? }`
- `fix` — `ManifestFix{ file, start_line, end_line, new_text }`(行范围替换)

事件类型 `progress`/`complete`/`error` 与 form 完全一致,前端共用 SSE 解析器(`consumeSSE`)。

---

## 三、生成流程(manifest_resource_generation)

### 3.1 步骤
1. **初始化**: `GetConfigForCapability("manifest_resource_generation")`
2. **意图断言**: 复用 `AssertIntent`,unsafe → blocked
3. **Module 库召回**: LIKE 搜索召回候选 module(name/module_source/description),取其输入变量;候选 + 变量拼进 prompt 供 AI 择优引用。无候选则生成原生资源。AI 自己选用哪个/不弹 need_selection。
4. **组装 + AI 生成**: domain skill 动态选择覆盖 composition → 加载候选 module skill 作上下文 → `AssemblePrompt` → `callAI` → 提取 HCL → `LogSkillUsage`
5. **Schema 校验(仅 module 块)**: 见 3.3

### 3.2 结果落地
HCL 通过 Monaco `executeEdits` 插入光标/替换选区。面板为 VS Code 风格右侧停靠聊天面板:上下文 chip(选区优先,显示 `文件:行范围`)、停止/关闭按钮、分步进度。

### 3.3 Schema 校验 + 反馈循环
生成后对 HCL 中**引用了仓库已有 module** 的块校验参数(resource 块不校验):
- 解析 HCL module 块 → 按 `source` 查 module,按 `version`(无则最新/默认版)取 schema
- 仓库不存在的 module / 原生 resource → 跳过
- **每个 module 块一个 goroutine 并发校验**,限制最大并发;不合规且 NeedAIFix → AIFeedbackLoop 修正 → 参数写回 HCL
- 任何失败/超时降级:保留 AI 原始 HCL,不阻断

---

## 四、检查流程(manifest_check)

### 4.1 步骤
1. **初始化**: `GetConfigForCapability("manifest_check")`
2. **意图断言**: check 打包用户**选中内容**发给 AI,选区可能含 prompt injection,**不跳过**安全断言
3. **打包**: 有选区只打选区;无选区打当前文件 + 跨文件引用到的关联文件(见 4.3)。多文件分段 `### 文件:` + 每行真实行号前缀(AI 直接引用前缀行号,避免行号漂移)
4. **组装 + AI 检查**: domain skill 动态选择 → 加载相关 module skill → `AssemblePrompt` → `callAI` → 解析问题列表(含 fix)→ `LogSkillUsage`

### 4.2 按钮文案与输出
- 按钮随选区实时切换:有选区「检查选中」,无选区「检查文件」
- 问题列表渲染在底部面板(VS Code Problems 风格),可点击跳转到 file:line
- 每条可修复项后带「修复」按钮:点击 `executeEdits` 按行范围替换(可跨文件)→ 移除该项 → 其余修复置灰 + 提示重新检查(放弃脆弱的撤销追踪,内容变了就重检)
- 解析失败回错误而非空数组(不假报"0 问题")

### 4.3 跨文件检查
无选区时,用块定位解析器找当前文件引用了、但定义在别处的符号所在文件,一并带给 AI(上限 ≤5)。覆盖五类引用:`var.` / `local.` / `module.` / `data.` / `<type>.<name>`(如 `aws_instance.test`)。选区检查不扩展(选区是局部意图)。

---

## 五、共享能力:块定位解析器(hclBlockIndex)

独立于 `hclDefinitions` 的 `DefinitionIndex`(后者被补全/诊断/跳转依赖,不可改),为 AI 与跳转新建:
- `buildBlockIndex(files)` — 扫草稿 .tf,建 `resource_id → {file, line}` 索引,识别五类定义(var/local/module/data/resource)。locals 支持单行 `locals{x=1}`、多行对象、内联多 key。
- `findExternalRefs(file, content, index)` — 跨文件检查用
- `locateBlock(index, resourceId)` — workspace 跳转、问题点击定位用(支持带属性后缀地址逐段回退)

---

## 六、开关原则(基本原则)

**必须遵守 aiConfig 里的开关,否则开关失效。** 对齐 id=12 的开关语义,不新增开关:

| 开关 | 位置 | 控制 |
|------|------|------|
| `use_optimized` | aiConfig | 是否跑 domain skill 的 **AI 动态选择** |
| `auto_load_module_skill` | skill_composition | 是否加载 module skill |
| `domain_skill_mode` | skill_composition | 不开 AI 选择时,domain skill 按 fixed/auto/hybrid 走 AssemblePrompt |

manifest config(id 22/23)默认两开关都关、mode=hybrid,即默认行为 = 按 hybrid 组装、不动态选 domain、不加载 module skill;要启用需在 AIConfig 打开对应开关。

## 七、Domain Skill 动态选择(对齐 form,不复用 form 代码)

仅当 `aiConfig.UseOptimized == true` 才启用。manifest 自建 `manifestDomainSkillSelector`(依赖 db/configService/aiFormService):
- 取所有 active domain skill 的 name+description
- 走 `domain_skill_selection` capability:AI 按 skill **description** 选择相关 domain skill(中性 prompt,**不排除 CMDB 资源类** —— CMDB 相关 skill 对检查现有资源引用有价值)
- 结果覆盖 `composition.DomainSkills`(保留 foundation/task);失败/无配置降级到 mode 默认,不阻断
- 开关关闭时完全不介入,domain skill 由 AssemblePrompt 按 `domain_skill_mode` 决定

注:form 的 `selectDomainSkillsByAI` 有 `phase=second` 排除 CMDB 的逻辑,是 form 资源已定阶段特有,manifest 不复用。

## 八、Module Skill 加载

仅当 `composition.AutoLoadModuleSkill == true` 才启用。开启时:生成前靠 Module 召回候选拿到 moduleID,逐个 `GetOrGenerateModuleSkill(moduleID)` 加载 module 配置知识进 prompt 上下文;check 同理(被检查内容引用平台 module 时加载)。受召回候选数上限约束。开关关闭则完全不加载。

---

## 九、Workspace 资源跳转 manifest

workspace 资源列表里 manifest 来源的资源(`manifest_deployment_id` 非空),点击整行跳 `/admin/manifests-v2/{id}/edit?org=&resource=<resource_id>`,编辑器深链定位到对应 module/resource 块第一行。非 manifest 资源仍走 ViewResource。manifestSummary 未加载完时提示稍候,不 fallthrough 到会报"无法获取Module信息"的旧详情页。

---

## 十、关键设计点与防护

- **行号前缀**: AI 数行号不可靠,check 打包给每行加真实行号前缀,AI 直接引用,根治行号漂移/选区偏移。
- **修复行号硬校验**: 应用修复前夹紧/拒绝越界行号,防止替换错位损坏草稿。
- **多文件 fix 归属**: 多文件时 issue/fix 无 file 默认归主文件;仅 fix 显式指向未知文件才丢弃。
- **意图断言不可跳过**: 生成和检查(含选区)都走安全守卫。
- **全链路降级**: domain skill 选择 / module skill 加载 / schema 校验任一失败都降级,不阻断主流程。

---

## 十一、数据与配置

- **skill**: 两个 task skill(`manifest_resource_generation_workflow` / `manifest_check_workflow`),落盘 `skill/manifest/task/`,三处同步(DB / 种子 `init_seed_data.sql` / migration `add_manifest_ai_skills.sql`)。
- **AIConfig**: 两条专用配置(id 22 opus / 23 sonnet,`mode=skill`,`enabled=false` 精确匹配),migration `add_manifest_ai_configs.sql`。service 优先读 `aiConfig.SkillComposition`,UI 可调。
- **前端 capability 注册**: `ai.ts` 的 `CAPABILITIES`/`LABELS`/`DESCRIPTIONS` + `AIConfigForm.tsx` 的 Skill 模式白名单(三处缺一则 UI 不显示场景或 skill 开关)。

---

## 十二、相关文件

**后端**
- `services/manifest_ai_service.go` — 生成流程
- `services/manifest_check_service.go` — 检查流程
- `services/manifest_ai_helpers.go` — extractHCL / 关键词 / 进度 tracker
- `services/manifest_domain_skill.go` — domain skill 选择(本批新增)
- `services/manifest_hcl_parser.go` — module 块参数提取(本批扩展)
- `services/schema_solver.go` — 按 version 取 schema(本批扩展)
- `controllers/manifest_ai_controller.go` — 两个 SSE handler
- `services/progress_event.go` — ProgressEvent + ManifestIssue/ManifestFix
- `internal/router/router_ai.go` — 路由

**前端**
- `services/manifestAi.ts` — SSE 调用 + 解析器
- `pages/admin/ManifestEditorV2/ManifestAiTools.tsx` — AI 工具 UI
- `pages/admin/ManifestEditorV2/hclBlockIndex.ts` — 块定位解析器
- `pages/admin/ManifestEditorV2/ManifestEditorV2.tsx` — bridge + 深链
- `pages/ResourcesTab.tsx` — 资源跳转
- `services/ai.ts` / `pages/AIConfigForm.tsx` — capability 注册

---

## 十三、实施批次

1. **第一批(已交付)**: 生成/修复 + check 基础(SSE、意图断言、Module 召回、问题列表、AIConfig/skill)。
2. **第二批(已交付)**: 跨文件检查、修复按钮、按钮文案、workspace 跳转、块定位解析器;含 review 修复(行号越界、locals 索引、多文件 fix 防护、跳转兜底)。
3. **第三批(规划中)**: domain skill 动态选择、module skill 加载、schema 校验+反馈循环(仅 module 块,并发)、修复按钮回归修复。
4. **后续**: manifest 生成接入 CMDB 查询(连带 need_selection 交互,独立批次)。
