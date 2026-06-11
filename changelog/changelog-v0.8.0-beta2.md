## v0.8.0-beta2

Manifest 编辑器 V2 交互体验全面升级:发布版本支持跳过检查、AI 修复结果逐条可应用;Run 和部署面板从弹窗/覆盖式改为 VS Code 风格右侧停靠面板,Run 面板集成 WebSocket 实时日志流 + HTTP 轮询兜底;状态栏新增运行任务快捷入口;移除全局复制限制,面板内容可自由选中复制。

### 新增功能

#### 发布版本跳过检查

- **新增** `PublishVersionDialog` 跳过检查按钮:在「开始检查」旁边增加「跳过检查」按钮,用户可跳过 AI 检查直接发布版本;点击后设置 `checkSummary = { done: false, skipped: true, issues: [] }` 解锁发布表单
- **优化** 提示文案:从「发布前请先对草稿执行一次检查」改为「发布前建议对草稿执行一次检查」,弱化强制性语气

#### AI 修复逐条应用

- **新增** `CheckPanel` 逐条修复追踪:将全局 `fixApplied: boolean` 改为 `fixedIndices: Set<number>`,每条 issue 的修复按钮独立启用/禁用,修复一条不影响其他条目的修复操作
- **新增** issues 变更自动重置:当 issues 列表变化(重新检查)时自动清空 `fixedIndices`,无需手动重置
- **优化** 修复按钮 title:已应用的条目显示「该修复已应用」,未应用的显示「应用该修复」
- **重构** 父组件 `handleCheckApplyFix`:移除从 issues 数组中 filter 已修复项的逻辑(保持索引稳定),移除 `checkFixApplied` 状态和 `setCheckFixApplied` 调用

#### Run 面板重写

- **重构** `RunDialog` 从 Ant Design Modal 改为 VS Code 右侧停靠面板:复用 `chatPanelStyle` 布局(与 AI 生成面板一致),暗色主题,右侧 360px 固定宽度
- **新增** WebSocket 实时日志流:提交 plan-only 任务后不跳转 workspace,面板内通过 `useTerraformOutput` hook 接入 `ws://.../tasks/:id/output/stream`,实时展示任务输出
- **新增** 日志分色渲染:`output` 白色、`error` 红色、`stage_marker` 绿色加粗,连接状态徽章实时反映(实时输出中/任务完成/连接断开)
- **新增** HTTP 轮询兜底:`useTaskPolling` hook 在 WebSocket 关闭或无实时数据时,每 3 秒轮询 `GET /workspaces/:wsId/tasks/:taskId`,从 `plan_output`/`apply_output`/`error_message` 提取日志显示
- **新增** 任务状态感知:轮询检测到终态(success/failed/cancelled/error)后自动停止,状态徽章显示对应颜色和图标,终态任务支持手动刷新
- **新增** 上次运行任务持久化:`lastRunTask` 状态提升到父组件 ManifestEditorV2,Run 任务创建后通过 `onRunTaskCreated` 回调更新,关闭面板后保留任务 ID
- **新增** 状态栏快捷入口:statusBar 右下角显示 `codicon-output` 图标 + `Task #ID`,点击直接打开 Run 面板查看上次运行日志
- **新增** `viewLast` 自动跳转:从状态栏点击时设置 `viewLast=true`,面板打开后自动跳到上次任务的日志视图,无需手动选择
- **修复** task_id 取值路径:API 响应为 `{ task: { id: N } }` 但前端只读 `resp.task_id ?? resp.id`,改为 `resp.task?.id ?? resp.task_id ?? resp.id`

#### 部署面板重写

- **重构** `DeployPanel` 从全屏覆盖式改为 VS Code 右侧停靠面板:复用 `chatPanelStyle` 布局,暗色主题,右侧 360px 固定宽度
- **重构** 移除所有 Ant Design 组件(`Form`/`Select`/`Tag`/`Alert`/`Button`/`Space`),替换为自定义暗色内联样式
- **新增** Variable Sets 自定义多选:chip + checkbox dropdown 实现,替代 Ant Design `Select mode="multiple"`
- **新增** 卸载确认内联化:从 `Modal.confirm`(React 19 下静默失效)改为面板内红色确认区域 + 取消/确认按钮
- **优化** 按钮高度统一:提取 `btnBaseStyle` 作为所有按钮基类,统一 `border: 1px solid transparent` + `boxSizing: border-box` + `lineHeight: 20px`,primary/secondary/danger/disabled 全部继承,解决不同状态下按钮高度不一致问题
- **保留** 全部原有功能:install/upgrade/uninstall/运行/工作目录选择/版本变量展示

#### 右侧面板互斥系统

- **新增** 四面板互斥:AI 生成/检查/Run/部署 共享右侧停靠区,同时只能展开一个,打开任一自动关闭其他
- **新增** 面板宽度状态:`runPanelWidth`/`deployPanelWidth` 独立状态,编辑器 `marginRight` 取所有面板宽度最大值(`Math.max`)
- **新增** Monaco 编辑器 layout 联动:所有面板宽度变化时触发 `requestAnimationFrame` → `editorRef.layout()` + `diffEditorRef.layout()`
- **重构** DeployPanel 渲染位置:从 editorArea 内部移出到与 CheckPanel 并列,不再盖住编辑区(visibility:hidden 逻辑移除)

#### 状态栏任务图标

- **新增** statusBar 运行任务入口:`lastRunTask` 非空时显示 `codicon-output` 图标 + `Task #ID`,cursor:pointer 可点击,hover 显示 tooltip「查看上次运行: Task #ID」

### 优化改进

#### 移除全局复制限制

- **修复** `.root` 容器 `user-select: none` 级联到所有子元素(包括面板内容、日志输出、检查结果),导致用户无法选中复制任何文本
- **优化** 选择性 `user-select: none`:仅在 `.titleBar`/`.toolbar`/`.activityBar`/`.sideBar`/`.statusBar`/`.tab`/`.ctxMenu` 等 chrome 元素上单独设置,内容区域保持可选
- **保留** `.tab` 和 `.ctxMenu` 的 `user-select: none`(编辑器标签和右键菜单属于交互控件,选中无意义)

#### CSS 清理

- **移除** `.deployPanel`/`.deployPanelHeader`/`.deployPanelBody`/`.deployPanelFooter` CSS module 样式(已改为内联样式 via `manifestAiStyles.ts`)
- **更新** 文件头部注释:从「覆盖编辑区」改为「右侧停靠面板(含 WebSocket 日志流)」

### 修改文件

- `frontend/src/pages/admin/ManifestEditorV2/PublishVersionDialog.tsx` — 跳过检查按钮 + onSkipCheck 回调
- `frontend/src/pages/admin/ManifestEditorV2/CheckPanel.tsx` — 逐条修复追踪(fixedIndices) + handleApplyFix/handleRecheck
- `frontend/src/pages/admin/ManifestEditorV2/RunDialog.tsx` — 右侧面板重写 + WebSocket 日志流 + HTTP 轮询兜底 + 上次任务持久化
- `frontend/src/pages/admin/ManifestEditorV2/DeployPanel.tsx` — 右侧面板重写 + 暗色内联样式 + 按钮高度统一 + Variable Sets 自定义多选
- `frontend/src/pages/admin/ManifestEditorV2/ManifestEditorV2.tsx` — 右侧面板互斥系统 + 面板宽度状态 + 状态栏任务图标 + lastRunTask/runViewLast 状态
- `frontend/src/pages/admin/ManifestEditorV2/ManifestEditorV2.module.css` — 移除 .root user-select:none + chrome 元素单独设置 + 清理 deployPanel 样式

### 技术细节

#### WebSocket 日志流架构

- **实时优先 + HTTP 兜底**:`TaskOutputLog` 组件同时使用 `useTerraformOutput`(WebSocket)和 `useTaskPolling`(HTTP),WebSocket 有数据时优先显示,WebSocket 关闭或无数据时自动切换到 HTTP 轮询的 `plan_output`
- **状态徽章决策树**:`isCompleted + hasWsLines` → 任务完成(实时日志) → `isConnected` → 实时输出中 → `polling` → 获取日志中 → `isTerminal` → 任务完成/失败 → 连接断开
- **自动重连**:`useTerraformOutput` 内置 10 次重连机制,指数退避(max 30s),任务未完成时自动重连

#### 按钮样式统一方案

- **`btnBaseStyle` 基类**:所有按钮共享 `padding: 4px 14px` + `border: 1px solid transparent` + `borderRadius: 2` + `lineHeight: 20px` + `boxSizing: border-box`
- **继承扩展**:`btnPrimaryStyle`/`btnSecondaryStyle`/`btnDangerStyle` 通过 `{ ...btnBaseStyle, background, color }` 继承,danger 类型额外设置 `borderColor: '#f14c4c'`
- **disabled 状态**:`btnDisabledStyle` 继承 `btnPrimaryStyle` 覆盖 `background`/`color`/`cursor`

#### 面板互斥实现

- **useEffect 依赖触发**:每个面板打开时通过 `useEffect` 检测并关闭其他面板,如 `runOpen` 变化时检查 `aiPanelWidth`/`checkPanelOpen`/`deployOpen` 并逐个关闭
- **宽度计算**:`Math.max(checkPanelOpen ? CHECK_PANEL_WIDTH : 0, aiPanelWidth, runPanelWidth, deployPanelWidth)` 确保编辑器让出足够空间
- **cleanup 恢复**:每个面板组件卸载时通过 `onPanelWidthChange?.(0)` 通知父组件恢复全宽

---

### AI Skill 系统重构:Module Skill 精确加载 + Domain/Module 分离选择

Manifest AI 资源生成/检查的 Skill 系统全面重构:Domain Skill(AI 语义选择最佳实践)与 Module Skill(精确版本匹配配置知识)彻底分离;Module Skill 数据源从 `skills` 表的遗留 `module_%d_auto` 记录迁移到 `module_version_skills` 表;AI 一次调用同时选择 domain skills 和 modules(含版本);生成/检查流程增加独立"Skill选择"步骤,前端 pipeline 从 4 步扩展为 5 步。

#### Module Skill 选择架构重构

- **重构** `manifestDomainSkillSelector.Select` 新增 `modules` 参数:AI 同时选择 domain skills + modules,一次 AI 调用完成两个选择,不增加延迟
- **新增** `manifestModuleInfo` 结构体:传给 AI 的 module 信息包含 `Name`/`Source`/`Versions[]`/`Description`,AI 可看到所有可用版本
- **新增** `selectedModule` 结构体:AI 返回 `{name, version}` 格式,支持用户指定版本(如"使用5.14.0")
- **重构** AI prompt 增加"可用 Modules"段:列出每个 module 的 name/source/可用版本/description,指导 AI 返回 `selected_modules` 数组
- **重构** AI 输出格式:`selected_modules` 从 `string[]` 改为 `{name, version}[]`,支持版本信息传递

#### Module Skill 精确加载

- **新增** `loadModuleSkillsByNames`:按 AI 选中的 module(含版本) 加载 `module_version_skills` 内容,优先使用指定版本,不存在时回退默认版本
- **优化** Module Skill 内容组合:同时加载 `schema_generated_content` + `custom_content`,与 `GetCombinedContent()` 逻辑一致
- **新增** `listAllModules` 版本列表:额外查询 `module_versions` 表获取每个 module 的可用版本,传给 AI 做选择
- **重构** `manifestModuleCandidate`:`Version string` → `Versions []string`,不再依赖 `modules.version` 单字段

#### Manifest 检查流程对齐

- **重构** `resolveModuleVersionSkillsForCheck`:从 HCL 解析 module source → 查 module → 默认版本 → `module_version_skills`,同样组合 `schema_generated_content` + `custom_content`
- **新增** `AutoLoadModuleSkill` 守卫:检查流程的 module skill 加载受开关控制,关闭时不加载

#### Pipeline 步骤扩展(4→5 步)

- **新增** 步骤 4 "Skill选择":Domain Skill AI 语义选择 + Module Skill 精确匹配,独立耗时展示
- **更新** `manifestGenStepName`/`manifestCheckStepName`:新增"Skill选择"步骤名
- **新增** 前端 `GEN_STEP_DESC`/`CHECK_STEP_DESC`:增加"Skill选择"描述
- **新增** `ManifestAiTools.tsx` pipeline 展示:与 `CheckPanel` 一致的步骤列表(绿色勾 + 耗时 + skill 标签),header 显示完成摘要 + 折叠 toggle
- **新增** `ManifestGenerateResult.CompletedSteps`:后端 complete SSE 事件携带全量步骤数据
- **新增** `manifestAi.ts` `completedSteps` 字段:前端从 complete 事件读取步骤列表

#### 遗留代码清理

- **删除** `generateNewModuleSkill`/`regenerateModuleSkill`:`SkillAssembler` 不再往 `skills` 表写入 `module_%d_auto` 记录
- **删除** `ShouldRegenerate`/`GenerateSkillFromModule`:`ModuleSkillGenerator` 不再提供基于 `skills` 表的生成/重生成逻辑
- **删除** `BatchGenerateModuleSkills`/`incrementSkillVersion`:controller 不再提供批量生成到 `skills` 表的端点
- **删除** `moduleSkillGen` 字段:`SkillAssembler` 不再持有 `ModuleSkillGenerator` 实例
- **重构** `GetOrGenerateModuleSkill`:仅从 `module_version_skills` 表加载(优先 `DefaultVersionID`,fallback `is_default=true`),无记录返回 nil
- **简化** `ModuleSkillController`:Generate/Get 端点统一调用 `GetOrGenerateModuleSkill`,移除 `force` 参数和 `skills` 表查询逻辑

#### AI Config 前端修复

- **修复** `AIConfigForm.tsx` `auto_load_module_skill` 开关可见性:新增 `MANIFEST_RESOURCE_GENERATION`/`MANIFEST_CHECK` 到 OR 条件,manifest 能力的 AI config 编辑页显示该开关

#### AI 面板复制修复

- **修复** `manifestAiStyles.ts` `chatPanelStyle` 增加 `userSelect: 'text'`:覆盖 `.toolbar` 继承的 `user-select:none`,AI 生成面板内可正常选中复制

### 修改文件

- `backend/services/manifest_domain_skill.go` — Select 新增 modules 参数 + selectedModule 结构体 + prompt/output 格式重构
- `backend/services/manifest_ai_service.go` — loadModuleSkillsByNames(版本匹配+回退) + listAllModules(版本列表) + 5 步流程
- `backend/services/manifest_check_service.go` — resolveModuleVersionSkillsForCheck + AutoLoadModuleSkill 守卫 + 5 步流程
- `backend/services/manifest_check_service_test.go` — 测试改用 module_version_skills 路径 + custom_content 列
- `backend/services/skill_assembler.go` — 删除 generateNewModuleSkill/regenerateModuleSkill/moduleSkillGen + GetOrGenerateModuleSkill 仅读 module_version_skills
- `backend/services/skill_assembler_test.go` — 移除 module_auto 相关测试用例
- `backend/services/module_skill_generator.go` — 删除 ShouldRegenerate/GenerateSkillFromModule
- `backend/controllers/module_skill_controller.go` — 简化 Generate/Get 端点 + 删除 BatchGenerate
- `backend/controllers/manifest_ai_controller.go` — complete 事件携带 CompletedSteps
- `frontend/src/pages/AIConfigForm.tsx` — auto_load_module_skill 开关对 manifest 能力可见
- `frontend/src/pages/admin/ManifestEditorV2/ManifestAiTools.tsx` — pipeline 步骤展示 + Skill选择描述
- `frontend/src/pages/admin/ManifestEditorV2/CheckPanel.tsx` — Skill选择描述
- `frontend/src/pages/admin/ManifestEditorV2/manifestAiStyles.ts` — userSelect: 'text'
- `frontend/src/services/manifestAi.ts` — completedSteps 字段
