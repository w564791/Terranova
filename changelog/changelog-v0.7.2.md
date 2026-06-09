## v0.7.2

Manifest 编辑器发布流程增加检查门控 + 检查面板从底部迁移到右侧(与 AI 生成面板统一布局)。后端检查能力增强:自动加载 HCL 引用的 module skill,排除 module_auto 进入 AI 自由选择池,修复 Qwen 服务类型不支持的问题。MetaRules prompt 精简为三层职责定义。

### Enhancements

#### 发布前检查门控

- **新增** 发布版本弹窗两步流程:点击「发布版本」→ 提示「请先检查」→ 点击「开始检查」后关闭弹窗,展开右侧检查面板;检查完成后重新打开弹窗显示检查摘要 + 版本表单,解锁发布按钮
- **新增** `PublishCheckSummary` 状态:记录检查结果(done/skipped/issues),供发布弹窗决定是否解锁表单

#### 检查面板统一右侧布局

- **新增** `CheckPanel` 通用右侧面板组件:复用 `manifestAiStyles` 的 `chatPanelStyle`,与 AI 生成面板布局完全一致(挤占式 360px)
- **重构** `ManifestAiTools` 从「自包含组件」变为「触发器 + 生成面板」:剥离全部检查逻辑(状态/runCheck/applyFix/底部面板),检查按钮改为 `onRequestCheck` 回调,组件从 851 行缩减到 ~590 行
- **重构** 检查状态提升到 `ManifestEditorV2` 统一管理:`runCheckCore(source: 'ai' | 'publish')` 支持两种检查模式共享右侧面板槽位;AI 检查支持选区(有选区只检查选区,无选区全文件 + 跨文件引用),发布检查始终全量
- **重构** 面板互斥:AI 生成面板和检查面板不能同时展开,后开的覆盖先开的;编辑器区 `marginRight` 根据活跃面板动态调整
- **修复** 选区检查行号传递:原代码 `startLine`(camelCase)与 API `CheckFilePayload.start_line`(snake_case)不匹配导致后端收到 undefined,默认为 1;现在正确传递 `start_line`,后端生成准确的行号前缀

#### 后端检查能力增强

- **新增** `resolveReferencedModuleSkillNames`:从被检查的 HCL 内容中精确解析 module source,只加载实际引用到的平台 module skill(module_auto 不进入 AI 自由选择池,避免误选无关模块)
- **新增** `ParseManifestModuleSourcesForCheck`:从 check 提交的待检查文件里提取 module source,不套用 terraform 执行目录的顶层过滤,避免漏召回嵌套目录的 module
- **新增** `resolveModulesBySources` + `ensureAutoModuleSkillName`:匹配平台 active module → 生成/复用 module_auto skill,支持 `module_source` 和 `source` 双字段查询
- **重构** `parseManifestModuleSources`:抽取共享解析逻辑,通过 `shouldParse` 回调区分 resources 扫描(subpath 过滤)和 check 扫描(全 .tf)

#### AI 框架

- **重构** MetaRules prompt 精简为三层职责定义:Foundation Layer(最高优先级硬约束)→ Domain Layer(领域最佳实践)→ Task Layer(当前任务流程),去掉原 Best Practice / Module Constraints 的分层,冲突解决原则更清晰
- **修复** `manifestDomainSkillSelector.Select` 排除 `module_auto` skill 进入 AI 自由选择候选池,避免检查 S3 等普通资源时误选安全组等无关模块约束

### Bug Fixes

- **修复** `AIFormService.callAI` 不支持 `"qwen"` 服务类型:switch case 漏掉 qwen,配置为 Qwen/DashScope 的 AIConfig 调用时直接走 default 报错;加上 `"qwen"` 路由到 `callOpenAICompatibleForForm`(Qwen/DashScope 使用 OpenAI 兼容 API)
- **修复** 选区检查 `start_line` 字段名不匹配(camelCase vs snake_case),导致后端收到 undefined 默认为 1,行号前缀全部从第 1 行开始而非选区起始行

### Files Changed

- `backend/services/ai_form_service.go` — callAI 增加 qwen 支持
- `backend/services/manifest_check_service.go` — module skill 自动加载
- `backend/services/manifest_domain_skill.go` — 排除 module_auto 进入 AI 选择池
- `backend/services/manifest_hcl_parser.go` — ParseManifestModuleSourcesForCheck + 共享解析
- `backend/services/manifest_hcl_parser_test.go` — 新增 check 解析测试
- `backend/services/skill_assembler.go` — MetaRules 三层精简
- `frontend/.../ManifestAiTools.tsx` — 剥离检查逻辑,按钮改回调
- `frontend/.../ManifestEditorV2.tsx` — 统一检查状态管理 + CheckPanel 渲染
- `frontend/.../PublishVersionDialog.tsx` — VS Code 暗色风格 + 检查门控
- `frontend/.../CheckPanel.tsx` — 新增通用检查右侧面板
