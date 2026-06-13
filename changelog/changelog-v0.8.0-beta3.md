## v0.8.0-beta3

Manifest 编辑器交互体验持续优化:右侧面板支持拖拽调整宽度并共享宽度设置;AI 对话支持上下文历史传递;Bedrock Claude 启用 prompt caching(90% 成本折扣);AI 检查增加用户自定义意见输入;文件树新增变更状态标记;编辑器 gutter 显示行级变更指示条。

### 新增功能

#### 面板拖拽调整宽度

- **新增** 右侧面板拖拽条(`.panelResizer`):AI 生成/检查/Run/部署四个面板均支持左缘拖拽调整宽度(250-700px 范围)
- **新增** 共享宽度机制:`sharedPanelWidthRef` 记录用户偏好的面板宽度,拖拽任一面板后其他面板自动使用相同宽度
- **优化** 面板互斥逻辑:使用 ref 跟踪前一个状态,仅在"刚打开"时关闭其他面板,避免循环依赖导致多个面板同时关闭
- **优化** 宽度传递:子面板的 `onPanelWidthChange` 直接调用而非通过 useEffect,消除与互斥逻辑的竞态

#### AI 对话上下文历史

- **新增** `ConversationTurn` 结构体:前端传递 `{ role, content }` 格式的历史消息给后端
- **新增** `formatConversationHistory`:将历史格式化为 Markdown 并注入 prompt,单条内容限制 2000 字符
- **优化** 历史构建:`buildConversationHistory` 从本地 session 消息提取最近 12 轮对话(用户描述 + AI HCL 摘要)
- **优化** 检查面板:`buildCheckHistory` 仅提取 check 类型消息,避免与 generate 历史混合

#### Prompt Cache(Bedrock Claude)

- **新增** `cache_control` 标记:system prompt 使用 `{ type: "text", text: ..., cache_control: { type: "ephemeral" } }` 数组格式
- **新增** cache 指标解析:响应中解析 `cache_creation_input_tokens` / `cache_read_input_tokens`,记录到日志
- **优化** prompt 结构:`callBedrockForForm` 将完整 prompt 移到 system message,user message 仅保留"请根据以上指示完成任务。",提升缓存命中率
- **收益** 相同前缀 5 分钟内复用可享受 90% input token 折扣;OpenAI 自动缓存因 prompt 结构稳定也能受益

#### AI 检查用户意见输入

- **新增** `UserInstruction` 字段:`CheckDraftRequest` 增加用户自定义检查意见(如"重点检查安全组"),限制 2000 字符
- **新增** 检查面板输入框:底部 textarea 支持输入关注点,Cmd/Ctrl+Enter 触发检查
- **新增** 上下文 chip:输入框上方显示当前检查的文件/选区信息(文件名 + 行号范围),实时更新
- **优化** prompt 注入:用户意见以"## 用户检查意见"段落注入 prompt,引导 AI 重点检查相关内容
- **优化** 误判过滤:带 `fix` 字段的 issue 不被 `isNonIssueNarrative` 误判为伪 issue

#### 文件变更状态标记

- **新增** 文件树状态徽章:新建文件显示绿色 **U**(Untracked),修改文件显示橙色 **M**(Modified),VS Code SCM 风格
- **新增** 变更追踪:`originalFilesRef` 记录初始文件列表,`originalContentRef` 记录首次加载内容,`onDidChangeContent` 实时对比更新 `modifiedFiles`
- **新增** 新建文件标记:`commitCreateFile` 成功后将路径加入 `newFiles` 集合

#### Gutter Diff 指示条

- **新增** 行级变更标记:编辑器 gutter 显示彩色竖条,绿色表示新增行,橙色表示修改行,与上次发布版本对比
- **新增** `computeLineDiff`:基于 LCS 算法计算每行状态(added/modified/unchanged),区分"新文件"与"未变更文件"
- **新增** 竞态处理:`diffFilesSetRef` 记录 diff 结果中的文件,`diffDraft` 加载完成后对当前打开文件重新应用装饰
- **新增** 实时更新:`onDidChangeContent` 时重新计算 diff 并调用 `deltaDecorations` 更新标记

### 优化改进

#### 检查面板重构为对话框

- **重构** `CheckPanel` 为完整对话体验:会话管理(创建/切换/删除)+ 历史消息流 + 输入框 + 发送按钮
- **新增** 会话隔离:按 manifest + 用户隔离,`ensureSession` 自动创建/复用会话
- **新增** 历史过滤:只显示 `kind === 'check'` 类型的消息,避免与 generate 历史混合
- **优化** 空状态:首次打开时显示引导文案"输入关注点或直接开始检查"

#### 检查面板选区动态跟踪

- **新增** 选区变化订阅:`checkPanelOpen` 为 true 时订阅 `aiBridge.onSelectionChange`,实时更新 context chip
- **新增** 文件切换跟踪:`currentFile` 变化时更新 context chip,覆盖切换文件但未改变选区的场景

#### 后端日志增强

- **新增** 请求指纹:`logBedrockRequestFingerprint` 记录 model/system_hash/tools_hash/messages 数量,便于排查
- **优化** 检查日志:`CheckDraftSSE` 日志增加 `user_instruction`/`session_id`/`history_count` 字段

### 修改文件

- `backend/controllers/manifest_ai_controller.go` — ConversationTurn + ManifestAIContext + UserInstruction + History 字段 + formatConversationHistory
- `backend/services/ai_caller.go` — cache_control 标记 + cache 指标解析 + logBedrockRequestFingerprint
- `backend/services/ai_form_service.go` — prompt 移到 system message + cache 指标日志
- `backend/services/manifest_ai_service.go` — GenerateResourceWithProgress 接受 conversationHistory 参数
- `backend/services/manifest_check_service.go` — CheckDraftWithProgress 接受 userInstruction + conversationHistory + 误判过滤修复
- `frontend/src/pages/admin/ManifestEditorV2/CheckPanel.tsx` — 对话框重构 + 会话管理 + 历史过滤 + 用户输入 + context chip
- `frontend/src/pages/admin/ManifestEditorV2/ManifestAiTools.tsx` — buildConversationHistory + 历史过滤(kind === 'generate')
- `frontend/src/pages/admin/ManifestEditorV2/ManifestEditorV2.tsx` — panelResizer + sharedPanelWidthRef + 互斥逻辑优化 + 文件变更追踪 + gutter diff
- `frontend/src/pages/admin/ManifestEditorV2/ManifestEditorV2.module.css` — .panelResizer + .statusBadge + .lineAdded/.lineModified
- `frontend/src/services/manifestAi.ts` — ConversationTurn + History + UserInstruction 字段

### 技术细节

#### 面板互斥循环依赖修复

- **问题**:多个 `useEffect` 相互触发,导致同一次渲染中两个面板同时关闭
- **方案**:使用 `useRef` 跟踪每个面板的前一个状态,只在"刚打开"(从 0/false 变为正/true)时执行关闭操作,使用 `else if` 链避免多个 effect 同时执行
- **效果**:四个面板可以自由切换,不会出现"都关闭"的竞态

#### Gutter Diff 竞态处理

- **问题**:`diffDraft()` 和 `openFile()` 都是异步的,谁先完成不确定;`publishedContentCache` 只存 changed/added 文件,unchanged 文件不在缓存中,`computeLineDiff` 把 `undefined` 一律当作"新文件"
- **方案**:新增 `diffFilesSetRef` 集合记录哪些文件确实在 diff 结果中,`computeLineDiff` 增加 `isInDiff` 参数区分"未变更"(无装饰)与"新文件"(全部 added)
- **效果**:刷新页面时无论哪个异步操作先完成,gutter 指示条都显示正确

#### LCS Diff 算法修正

- **问题**:原回溯算法用 `j > 0` 判断"修改"还是"新增",错误地将所有非 LCS 行都标记为 added
- **方案**:两阶段算法 — 先标记哪些行在 LCS 中(matchedCurrent),不在 LCS 中的行用 `pubIdx` 跟踪旧行位置:有对应旧行 → modified,没有 → added
- **效果**:颜色逻辑与 demo 一致 — 绿色表示纯新增,橙色表示修改
