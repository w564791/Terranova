## v0.8.0-beta1

前端 UI v3 主题系统全面上线:新增 v2/v3 切换机制,CSS 变量覆盖架构确保 v2 零影响;资源编辑引入 HCL 双向实时编辑器(overlay 方案 + Monaco Editor 双引擎),支持 Terraform 系统参数分离与 Schema 外字段检测;版本管理增加升级/降级流程、版本锁定策略与 Schema 感知的版本对比;HCL 工具链增强 jsonencode() 双向转换、Terraform 表达式解析;后端 terraform init 自动检测 locked provider 错误并 -upgrade 重试;Manifest 资源管理增强:manifest 管理的资源禁止直接删除(后端 409 + 前端拦截),深链跳转支持版本/subpath 参数与发布版本 fallback。

### 新增功能

#### UI v3 主题系统

- **新增** `useUIVersion` hook + `localStorage` 持久化:v2/v3 状态存储在 `ui-version-preference` 键,支持 URL 参数 `?ui=v3` 同步,切换后所有页面即时生效
- **新增** `v3-theme.css`:基于 CSS 变量的主题覆盖层,在 `<html data-ui-version="v3">` 选择器下生效,不影响 v2 的任何样式
- **新增** `FormRendererV3`:OpenAPI 表单渲染器包装组件,通过 `UIVersionContext` 向子组件(如 SwitchWidget)注入 v3 状态
- **新增** TopBar 和 Layout 双入口:独立路由页面(ViewResource/EditResource/AddResources)通过 TopBar 提供切换按钮,Layout 包裹的页面通过 Layout Header 提供切换按钮
- **新增** `FormRendererV3.module.css`:覆盖 antd 组件样式(Input 圆角 10px、Section 毛玻璃 header、Switch 蓝色主色、Tag 圆角 6px、只读模式精确背景色替代 grayscale filter)

#### HCL 编辑器(overlay 方案)

- **新增** `HCLEditor` 组件:替代 `JsonEditor`,实现 HCL 双向实时编辑。查看模式显示语法高亮的 `<pre>`,点击后透明 textarea 覆盖在高亮层上编辑,编辑内容实时解析为 JSON 同步到表单
- **新增** `hclFormatter.ts`:JSON → HCL 格式化工具,支持 `skipDefaults` 选项过滤 Schema 默认值、`systemParams` 选项渲染 Terraform 系统参数(for_each/count/depends_on/providers/lifecycle)
- **新增** `hclParser.ts`:HCL → JSON 解析器,`parseHCLModule` 返回 `{ moduleName, systemParams, userConfig }` 三层分离结构;`detectExtraFields` 对比 Schema 定义检测额外字段
- **新增** 语法高亮 CSS:`.hcl-keyword`(紫)、`.hcl-string`(绿)、`.hcl-bool`(红)、`.hcl-number`(橙)、`.hcl-attr`(蓝)、`.hcl-eq`(灰)、`.hcl-bracket`(白)、`.hcl-comment`(灰斜体)
- **修复** overlay 方案恢复:移除独立 `clickOverlay` 层,改为 `scrollArea` 直接绑定 `onClick`;`<pre>` 滚动同步到 `textarea`;blur 事件判断新焦点是否仍在编辑器容器内(点击滚动条/代码行不关闭编辑);textarea 高度自动匹配内容(`ResizeObserver` 监听 `<pre>`)

#### HCL 编辑器(Monaco 方案)

- **新增** `MonacoHclEditor` 可复用组件:基于 Monaco Editor 的 HCL 编辑器,复用 ManifestEditor 的 HCL 语言支持(语法高亮、自动补全、Hover 提示、跳转到定义、Inlay Hints、Code Actions)
- **新增** Monaco CSS 手动注入:`ensureMonacoEditorCss()` 将 `monaco-editor/min/vs/editor/editor.main.css` 以 `<style>` 标签注入 `<head>`,解决 Vite 打包后 Monaco 核心样式丢失问题
- **新增** 编辑器初始化稳定性:使用 `useRef` 缓存初始值(`initialValueRef`/`initialReadOnlyRef`/`initialDefinitionIndexRef`),`useEffect` 依赖项稳定为 `[layoutEditor]`,避免 Monaco 实例重复创建;`onChange` 通过 `onChangeRef` 转发,避免闭包陷阱
- **新增** 动态高度布局:`layoutEditor` 统一计算容器高度(`minHeight`/`maxHeight`),`ResizeObserver` 监听容器尺寸变化并重新 layout

#### HCL jsonencode() 支持

- **新增** `hclFormatter` jsonencode 渲染:schema `format: "json"` 的字段自动渲染为 `jsonencode({ ... })` 语法;普通字符串字段如果内容是合法 JSON 也自动转换为 `jsonencode()`
- **新增** `formatJsonEncodeValue`:jsonencode 内部值格式化,支持嵌套对象/数组/布尔/数字/null,使用 HCL 风格 `key = value` 语法(逗号分隔)
- **新增** `hclParser` jsonencode 解析:识别 `jsonencode(...)` 表达式,解析内部 HCL 风格对象/数组,返回 JSON 字符串(与 JSON.stringify 兼容)
- **新增** `parseHCLObject` / `parseHCLArray` / `parseHCLInnerValue`:jsonencode 内部的 HCL 子语言解析器,支持 `key = value` 逗号分隔、嵌套对象和数组
- **新增** HCL key 引号处理:`formatHCLKey` 对含特殊字符(点号、连字符、冒号等)的 key 自动加双引号,普通标识符保持裸写
- **新增** AWS 风格冒号标识符:parser `parseIdentifier` 正则扩展为 `[a-zA-Z0-9_:-]`,支持 `aws:PrincipalArn` 等 AWS IAM policy key

#### Terraform 系统参数管理

- **新增** `TF_SYSTEM_PARAMS` 常量集合:定义 `source`、`version`、`for_each`、`count`、`depends_on`、`providers`、`lifecycle` 为系统参数,解析时自动分离,不进入表单渲染
- **新增** 额外字段检测:用户在 HCL 中编辑了 Schema 未定义的字段时,显示黄色警告栏(⚠ 发现 N 个 Schema 未定义的字段:xxx),提供「保留」和「丢弃」两个操作按钮
- **新增** 系统参数 HCL 渲染:`jsonToHCL` 将 `systemParams` 渲染在 `source`/`version` 之后、用户配置之前,确保 HCL 输出完整
- **新增** 系统参数状态保持:EditResource 增加 `hclSystemParams` 状态,加载资源时从 tf_code 中提取系统参数并保存;HCL 编辑变更时同步更新;提交时系统参数与用户配置合并写入 module 配置
- **修复** 系统参数与额外字段交叉误判:`detectExtraFieldsInData` 排除 `TF_SYSTEM_PARAMS` 中的 key,避免 `for_each`/`depends_on` 等被误报为额外字段;`filterDataToSchema` 保留系统参数不被丢弃

#### Terraform 表达式支持

- **新增** `getTerraformExpression`:检测字符串值是否为 Terraform 表达式(`${...}` 格式),是则返回裸表达式内容
- **新增** `formatSystemValue`:系统参数专用格式化函数,对 `${var.xxx}` 等表达式值不加引号直接输出(如 `for_each = var.instances`),普通字符串仍加双引号
- **新增** `parseExpression`:HCL parser 增加表达式解析,支持 `${...}` 插值语法的识别和保留,支持嵌套字符串和括号深度跟踪
- **新增** Schema 多路径查找:`getSchemaFields` 扩展为依次查找 `rawSchema.openapi_schema` → `schema.openapi_schema` → `rawSchema.schema_data` → `schema.schema_data` → 直接 `properties`,兼容不同 API 返回格式

#### 版本管理与升级流程

- **新增** 版本选择器(ViewResource):Module 版本徽章旁显示 ↕ 下拉按钮,点击展开版本列表(按 semver 降序),当前版本显示紫色「当前」标签,最新版本显示绿色「最新」标签
- **新增** 升级快捷提示:当资源版本 < 模块最新版本时,版本徽章旁显示蓝色药丸 `[5.9.1 → 5.9.2 ↗]`,点击触发升级流程
- **新增** 版本切换确认对话框:点击任意非当前版本 → 弹出 ConfirmDialog,标题根据 semver 比较动态显示「升级模块版本」或「切换模块版本」,确认后跳转到 EditResource 编辑页面(带 `?upgrade_to=versionId` 参数)
- **新增** EditResource 升级模式:检测 URL `upgrade_to` 参数,自动加载目标版本的 Schema 渲染表单,预填变更摘要为「升级模块版本: X → Y」,用户确认配置后提交保存,模块版本正式变更

#### Schema 感知版本对比

- **新增** `ResourceVersionDiff` Schema 加载:对比面板打开时自动加载资源的 module schema(通过 `resource → module_source → module_id → schemaV2`),用于识别 `typejsonstring` 字段
- **新增** `isJsonStringField`:根据 schema `format: "json"` 判断字段是否为 JSON 字符串类型,对比展示时自动 pretty-print
- **新增** `formatJsonStringAsHCL`:jsonencode 格式的版本对比渲染,确保 JSON 字符串字段以结构化方式展示差异
- **新增** LCS 逐行 diff 算法:`computeLineDiff` 基于最长公共子序列算法对比两个版本的文本,输出 `added`/`removed`/`unchanged` 行级差异
- **新增** `ResourceVersionDiff.module.css`:版本对比面板样式(差异行背景色、行号、diff 类型标记)

#### URL 驱动的版本对比模式

- **新增** ViewResource URL 对比参数:支持 `?mode=compare&compare_from=N&compare_to=M` URL 参数直接打开对比视图,页面加载时自动读取参数并触发对比
- **新增** 旧 URL 兼容:无 `compare_from`/`compare_to` 时回退到 `?version=N` + 当前版本的旧格式

#### JSON 编辑器语法高亮

- **新增** `JsonEditorWidget` 单遍 token 扫描高亮:`highlightJson` 函数逐字符扫描,一次遍历完成 key/string/number/bool/null/bracket 的分类和着色,避免多遍 regex 互相污染(double-matching)
- **新增** `JsonEditorWidget.module.css`:v3 主题下 JSON 编辑器样式覆盖,语法高亮配色与 HCL 编辑器一致

### 优化改进

#### 额外字段确认流程重构

- **重构** 额外字段检测逻辑:抽取 `getSchemaFields` / `detectExtraFieldsInData` / `filterDataToSchema` / `getCurrentHclData` 四个通用函数,消除重复的 schema 查找和字段过滤代码
- **重构** 提交时额外字段检查:`handleSubmit` 在实际提交前检测当前数据(非缓存状态)中的额外字段,避免 HCL 已修改但 `pendingExtraFields` 状态滞后的问题
- **重构** `pendingSubmit` → `pendingSubmitAction`:从布尔值改为 `boolean | null`,同时传递 `submitData` 给 `handleSubmit`,确保「保留/丢弃」操作后提交的是用户确认过的数据而非重新读取的 HCL
- **优化** 变更摘要验证顺序调整:额外字段检测提前到变更摘要验证之前,避免用户填完摘要后发现需要处理额外字段,处理完还要重新确认摘要

#### Monaco 编辑器滚动隔离

- **优化** `alwaysConsumeMouseWheel: false`:Monaco 滚动条配置改为不吞没滚轮事件,允许滚轮在编辑器到顶/到底时传递给页面
- **优化** `overscroll-behavior: auto`:容器 CSS 从 `contain` 改为 `auto`,配合 JS 层面的精细滚轮控制
- **新增** 双层滚轮事件处理:capture 阶段的 `handoffWheelAtBoundary`(编辑器到边界时将滚轮交给页面滚动容器,自动查找最近的 `overflow-y: auto/scroll` 祖先元素) + bubble 阶段的 `isolateWheelInsideEditor`(编辑器内部滚动时阻止冒泡)
- **新增** `findVerticalScrollParent`:向上遍历 DOM 树查找最近的可垂直滚动祖先元素,作为滚轮边界交接的目标

#### 版本锁定策略

- **优化** EditResource 版本选择器改为只读显示:编辑模式下版本选择器替换为文本标签,锁定到资源当前版本,严禁跨版本渲染;版本不匹配时强制 HCL 视图并显示红色警告
- **优化** ViewResource 表单视图禁用:版本不匹配时「表单视图」按钮 disabled + opacity 0.5,显示红色警告提示并提供升级链接
- **优化** 版本匹配检查:ViewResource 和 EditResource 加载时调用 `listVersions` API,检查资源版本是否存在于模块版本列表中,不存在则设置 `resourceVersionFound=false` 并降级处理

#### Bool 开关修复

- **修复** SwitchWidget V3 模式 valuePropName 问题:`Form.Item` 的 `name` 属性将 `checked`/`onChange` 注入到直接子元素 `<div>` 而非 Switch,导致开关无法操作
- **修复** 改为 Switch 作为 `Form.Item` 直接子元素(带 `valuePropName="checked"`),状态文字「已启用/未启用」通过独立组件 `SwitchV3Label` 用 `Form.useWatch` 读取值显示

#### 页面 v3 适配全量扫描

- **优化** EditResourceDialog:从仅支持 v1 `DynamicForm` 扩展为支持 v2 `OpenAPIFormRenderer` + v3 `FormRendererV3` + HCL 编辑,新增 `isV2Schema` 检测和版本选择器
- **优化** DemoPreview:新增 `FormRendererV3` + `HCLView` 条件渲染,按钮标签改为 `{isV3 ? 'HCL 视图' : 'JSON视图'}`,Schema 加载增加 `isV2` 检测
- **优化** AddResources:配置步骤和预览步骤的按钮标签改为 v3 条件渲染,配置步骤的 `JsonEditor` 替换为 `HCLEditor`(v3 模式),错误提示改为 `{isV3 ? 'HCL' : 'JSON'}视图`
- **优化** CreateDemo:新增 `HCLEditor` 导入和条件渲染,错误提示改为 v3 条件渲染
- **优化** SchemaManagement:`tab=json` 在 v3 模式下显示为「配置 HCL」,渲染 `HCLEditor` 替代 textarea JSON 编辑器;预览弹窗使用 `HCLEditor` readOnly 模式
- **优化** WorkspaceResources:硬编码标签 `Terraform配置 (JSON)` 改为 `Terraform配置 (HCL)`
- **优化** ResourceVersionDiff:版本对比差异值在 v3 模式下使用 `jsonToHCL` 格式化显示

#### Toast 消息统一

- **优化** ViewResource/EditResource/AddResources 三个页面的错误提示和 toast 消息:硬编码的 `JSON视图` 改为模板字符串 `` `${isV3 ? 'HCL' : 'JSON'}视图` ``,确保 v3 模式下消息文本一致

#### URL 参数同步

- **优化** EditResource `viewMode` 持久化:切换到 HCL 视图时 URL 追加 `?view=hcl`(v3)或 `?view=json`(v2),刷新页面后从 URL 恢复视图状态,点击表单视图时删除 `view` 参数

#### Console.log 清理

- **优化** EditResource:移除 100+ 条 emoji 前缀的 `console.log` 调试语句(📊/🔄/📝 等),保留 `console.error` 和 `console.warn`
- **优化** ViewResource:移除 55+ 条 `console.log`,保留错误和警告日志
- **优化** AddResources/CreateDemo/DemoPreview/EditResourceDialog:全量清理 `console.log`,保留 `console.error`

#### 文档与命名澄清

- **新增** `schemaV2.ts` 文件头部注释:明确说明 "Schema V2" 是后端 API 数据格式版本(OpenAPI 标准格式),与前端 UI 主题版本(v2/v3)是两个完全不同的概念,避免混淆

### 后端

#### Terraform init locked provider 自动重试

- **新增** `isLockedProviderError`:检测 terraform init 错误信息中是否包含 `"locked provider"` 或 `"must use terraform init -upgrade"`,判断是否为 locked provider 版本不匹配错误
- **新增** `forceUpgrade` 标志:重试循环中检测到 locked provider 错误后设置 `forceUpgrade = true`,下次重试跳过指数退避直接带 `-upgrade` 参数执行
- **优化** `terraformInitOnce` 签名:新增 `forceUpgrade bool` 参数,与 `shouldUseUpgrade`(基于 provider_config_hash 变更检测)取 OR,确保 locked provider 场景下 `-upgrade` 一定生效
- **优化** 日志区分:首次运行 `-upgrade`(provider config changed)与 locked provider 强制 `-upgrade` 使用不同的日志消息,便于排查

### Bug Fixes

- **修复** HCL 编辑器 overlay 文本对齐:移除独立 `clickOverlay` div(该层导致 textarea 与 `<pre>` 的滚动位置不同步),改为 `scrollArea` 直接绑定 `onClick` 触发编辑;`<pre>` 增加 `onScroll` 事件同步到 textarea
- **修复** HCL 编辑器 blur 误关闭:点击编辑器内部元素(滚动条、代码行)时 `relatedTarget` 仍在容器内,不再关闭编辑模式,而是 `requestAnimationFrame` 恢复 textarea 焦点
- **修复** Monaco 编辑器 CSS 加载:Vite 打包后 Monaco 核心编辑器样式丢失,通过 `ensureMonacoEditorCss()` 手动将 `monaco-editor/min/vs/editor/editor.main.css` 内联到 `<head>`,确保编辑器 UI 正确渲染
- **修复** Monaco 编辑器初始化稳定性:原代码 `useEffect` 依赖 `[]` 但引用了 `value`/`readOnly`/`definitionIndex` 等 prop,prop 变化后 Monaco 实例不会更新;改为 `useRef` 缓存初始值 + 后续 `useEffect` 单独同步,确保 Monaco 实例只创建一次
- **修复** Monaco 编辑器 `onChange` 闭包:原 `onDidChangeModelContent` 回调直接引用 `onChange`,如果 `onChange` 被重新创建则回调仍引用旧函数;改为 `onChangeRef` 转发,始终调用最新的 `onChange`
- **修复** HCL 系统参数丢失:EditResource 加载资源时未提取 `for_each`/`depends_on` 等系统参数,导致 HCL 编辑器中这些字段丢失;现在加载时解析并保存到 `hclSystemParams` 状态,HCL 渲染时传入 `systemParams` 选项
- **修复** HCL 系统参数被误报为额外字段:提交时 `detectExtraFieldsInData` 将 `for_each` 等系统参数也标记为 Schema 外字段;现在排除 `TF_SYSTEM_PARAMS` 中的 key
- **修复** 额外字段确认后数据不一致:原 `handleKeepExtraFields`/`handleDiscardExtraFields` 在继续提交时重新读取 HCL(可能已被用户修改),导致提交的不是用户确认的数据;现在通过 `submitData` 参数传递确认后的数据快照
- **修复** Terraform 表达式被加引号:`for_each = "${var.instances}"` 等表达式值被格式化为字符串而非裸表达式;现在 `formatSystemValue` 检测 `${...}` 模式并直接输出表达式内容
- **修复** 额外字段检测 schema 查找不全:只查 `rawSchema.openapi_schema` 一种路径,部分 API 返回 `schema_data` 或直接 `properties` 时检测失效;现在依次查找多种路径格式

### 技术细节

#### 架构设计

- **v2/v3 隔离机制**:通过 `<html data-ui-version>` 属性 + CSS 变量覆盖实现主题隔离,v2 模式下所有 v3 样式规则不生效;`FormRendererV3` 包装组件不修改原 `FormRenderer.tsx` 的任何逻辑
- **HCL 编辑器双引擎**:overlay 方案(HCLEditor)用于轻量场景,透明 `<textarea>` 覆盖在语法高亮 `<pre>` 上;Monaco 方案(MonacoHclEditor)用于需要完整 IDE 功能的场景(自动补全、跳转定义、Hover 等)。两个方案共存,EditResource 默认使用 Monaco
- **HCL overlay 方案**:查看模式渲染高亮 `<pre>`,编辑模式透明 `<textarea>` 绝对定位覆盖在 `<pre>` 上,`color: transparent` + `caret-color: #e2e8f0` 仅显示光标;两个元素共享 `scrollArea` 滚动容器,通过 `onScroll` 事件同步滚动位置
- **Monaco 滚动隔离三层策略**:(1) Monaco `alwaysConsumeMouseWheel: false` 允许边界传递;(2) capture 阶段 `handoffWheelAtBoundary` 在编辑器到顶/到底时将 deltaY 转发给页面滚动容器;(3) bubble 阶段 `isolateWheelInsideEditor` 在编辑器内部滚动时阻止冒泡
- **jsonencode 双向转换**:formatter 侧通过 schema `format: "json"` 或字符串内容检测自动渲染为 `jsonencode()`;parser 侧识别 `jsonencode(` 前缀后用 HCL 子语言解析器处理内部结构,返回 JSON 字符串
- **版本匹配策略**:资源版本必须在模块版本列表中存在才能渲染表单,不存在时降级到 HCL 视图;升级流程不直接修改资源,而是跳转到编辑页面让用户确认后再提交
- **JSON 单遍高亮**:避免多遍 regex 互相污染(double-matching),一次字符扫描完成所有 token 分类

#### 新增文件

- `frontend/src/hooks/useUIVersion.ts` — v2/v3 状态管理 hook
- `frontend/src/styles/v3-theme.css` — v3 CSS 变量主题
- `frontend/src/contexts/UIVersionContext.tsx` — v3 React Context
- `frontend/src/components/OpenAPIFormRenderer/FormRendererV3.tsx` — v3 表单渲染器包装
- `frontend/src/components/OpenAPIFormRenderer/FormRendererV3.module.css` — v3 样式覆盖
- `frontend/src/components/HCLEditor/HCLEditor.tsx` — HCL 双向编辑器(overlay 方案)
- `frontend/src/components/HCLEditor/HCLEditor.module.css` — HCL 编辑器样式 + 语法高亮
- `frontend/src/components/MonacoHclEditor/MonacoHclEditor.tsx` — Monaco HCL 编辑器组件
- `frontend/src/components/MonacoHclEditor/MonacoHclEditor.module.css` — Monaco 编辑器样式
- `frontend/src/components/MonacoHclEditor/index.ts` — Monaco 编辑器导出入口
- `frontend/src/components/OpenAPIFormRenderer/widgets/JsonEditorWidget.module.css` — JSON 编辑器语法高亮样式
- `frontend/src/components/ResourceVersionDiff.module.css` — 版本对比面板样式
- `frontend/src/pages/AddResources.module.css` — 额外字段对话框样式
- `frontend/src/utils/hclFormatter.ts` — JSON → HCL 格式化(含 jsonencode 渲染)
- `frontend/src/utils/hclParser.ts` — HCL → JSON 解析(含 jsonencode 解析 + 表达式支持)
- `frontend/src/components/HCLView/HCLView.tsx` — HCL 只读查看组件
- `frontend/src/components/HCLView/HCLView.module.css` — HCL 查看组件样式
- `frontend/src/components/SourceVersionCard/SourceVersionCard.tsx` — 版本信息卡片组件
- `frontend/src/components/SourceVersionCard/SourceVersionCard.module.css` — 版本卡片样式
- `docs/ui-v3-plan.md` — UI v3 实施计划文档
- `docs/ui-v3-hcl-system-params.md` — HCL 系统参数设计文档

#### 修改文件

- `frontend/src/components/OpenAPIFormRenderer/widgets/SwitchWidget.tsx` — v3 模式 Switch 修复 + 状态文字
- `frontend/src/components/OpenAPIFormRenderer/widgets/JsonEditorWidget.tsx` — 单遍 JSON 语法高亮 + v3 主题适配
- `frontend/src/components/ResourceVersionDiff.tsx` — Schema 加载 + jsonencode 格式化 + LCS 逐行 diff
- `frontend/src/pages/ViewResource.tsx` — 版本选择器、升级提示、HCLView、URL 对比参数、console.log 清理
- `frontend/src/pages/EditResource.tsx` — Monaco 编辑器集成、系统参数保持、额外字段检测重构、版本锁定
- `frontend/src/pages/AddResources.tsx` — HCLEditor 替换 JsonEditor、按钮标签条件化、console.log 清理
- `frontend/src/pages/CreateDemo.tsx` — HCLEditor 集成、错误提示条件化
- `frontend/src/pages/SchemaManagement.tsx` — HCLEditor 集成、tab 名称条件化
- `frontend/src/pages/WorkspaceResources.tsx` — 标签文案修正
- `frontend/src/components/EditResourceDialog.tsx` — v2/v3 Schema 渲染器 + HCLEditor
- `frontend/src/components/DemoPreview.tsx` — FormRendererV3 + HCLView
- `frontend/src/services/schemaV2.ts` — 命名说明注释 + `detectSchemaVersion` 标记 `@deprecated`
- `backend/services/terraform_executor.go` — locked provider 检测 + `-upgrade` 自动重试

### Manifest 资源管理增强

#### Manifest 管理资源删除保护

- **新增** 后端删除保护:`DeleteResourceWithOptions` 检测 `manifest_deployment_id`,如果资源由 Manifest 管理则返回错误,阻止直接删除
- **新增** 409 Conflict 响应:`DeleteResource` controller 识别 "managed by a manifest deployment" 错误,返回 HTTP 409 而非 500,前端可精确区分
- **新增** 前端删除拦截:ResourcesTab `handleDeleteResource` 检测 `manifest_deployment_id`,阻止删除并 toast 提示「此资源由 Manifest 管理,请在 manifest 编辑器中操作」
- **新增** 删除按钮 disabled 状态:Manifest 管理的资源删除按钮置灰 + hover 提示,防止误操作

#### Workspace Manifest Summary 增强

- **新增** `version_id` 字段:workspace manifest summary API 返回当前部署的版本 ID,供前端构建版本感知的深链
- **新增** `listVersionFiles` API:读取已发布版本的文件树(只读),用于深链跳转时在发布版本中查找资源块

#### 深链跳转增强

- **新增** 版本感知深链:`buildManifestEditorUrl` 统一构建编辑器 URL,携带 `version`、`subpath`、`resource` 参数,从 workspace 资源列表跳转到 Manifest 编辑器时精确定位
- **新增** 发布版本 fallback:深链定位时先在草稿文件中查找资源块,找不到则通过 `listVersionFiles` 加载发布版本文件构建索引,在发布版本中找到后打开对应草稿文件并定位行号
- **新增** subpath 过滤:`isTopLevelTfUnderSubpath` 辅助函数,深链构建索引时只纳入 subpath 下的顶层 .tf 文件,避免跨 subpath 误匹配
- **优化** 深链 key 去重:`deepLinkDoneRef` 从布尔值改为深链 key 字符串,支持同一编辑器内连续跳转不同资源/文件,不再因 key 不同而跳过

#### Manifest 徽章简化

- **优化** Manifest 徽章 UI:移除 📦 emoji、manifest 名称和 active_tag 显示,精简为「Manifest」文字标签,点击跳转到编辑器
- **优化** Manifest 编辑器跳转统一:ResourcesTab 所有跳转点(badge、banner 按钮、资源行点击)统一使用 `buildManifestEditorUrl`,消除重复的 URL 拼接逻辑

#### 测试

- **新增** `resource_service_manifest_test.go`:Manifest 管理资源删除保护的单元测试,覆盖有 manifest 部署 ID 的资源被拒绝删除、无 manifest 的资源正常删除等场景

#### 新增/修改文件

- `backend/controllers/resource_controller.go` — 409 Conflict 响应
- `backend/internal/handlers/manifest_deployments_v2_handler.go` — version_id 字段
- `backend/services/resource_service.go` — manifest_deployment_id 字段 + 删除保护
- `backend/services/resource_service_manifest_test.go` — 删除保护单元测试(新增)
- `frontend/src/hooks/useWorkspaceManifestSummary.ts` — version_id 类型
- `frontend/src/pages/ResourcesTab.tsx` — 删除拦截 + buildManifestEditorUrl + 徽章简化
- `frontend/src/pages/admin/ManifestEditorV2/ManifestEditorV2.tsx` — 深链版本/subpath/fallback
- `frontend/src/pages/admin/ManifestEditorV2/manifestApi.ts` — listVersionFiles API
