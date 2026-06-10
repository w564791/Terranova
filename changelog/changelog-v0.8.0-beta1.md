## v0.8.0-beta1

前端 UI v3 主题系统全面上线:新增 v2/v3 切换机制,CSS 变量覆盖架构确保 v2 零影响;资源编辑引入 HCL 双向实时编辑器替代 JSON,支持 Terraform 系统参数分离与 Schema 外字段检测;版本管理增加升级/降级流程与版本锁定策略;全量扫描修复 module 和 workspace 子页面的 v3 适配问题。

### 新增功能

#### UI v3 主题系统

- **新增** `useUIVersion` hook + `localStorage` 持久化:v2/v3 状态存储在 `ui-version-preference` 键,支持 URL 参数 `?ui=v3` 同步,切换后所有页面即时生效
- **新增** `v3-theme.css`:基于 CSS 变量的主题覆盖层,在 `<html data-ui-version="v3">` 选择器下生效,不影响 v2 的任何样式
- **新增** `FormRendererV3`:OpenAPI 表单渲染器包装组件,通过 `UIVersionContext` 向子组件(如 SwitchWidget)注入 v3 状态
- **新增** TopBar 和 Layout 双入口:独立路由页面(ViewResource/EditResource/AddResources)通过 TopBar 提供切换按钮,Layout 包裹的页面通过 Layout Header 提供切换按钮
- **新增** `FormRendererV3.module.css`:覆盖 antd 组件样式(Input 圆角 10px、Section 毛玻璃 header、Switch 蓝色主色、Tag 圆角 6px、只读模式精确背景色替代 grayscale filter)

#### HCL 编辑器

- **新增** `HCLEditor` 组件:替代 `JsonEditor`,实现 HCL 双向实时编辑。查看模式显示语法高亮的 `<pre>`,点击后透明 textarea 覆盖在高亮层上编辑,编辑内容实时解析为 JSON 同步到表单
- **新增** `hclFormatter.ts`:JSON → HCL 格式化工具,支持 `skipDefaults` 选项过滤 Schema 默认值、`systemParams` 选项渲染 Terraform 系统参数(for_each/count/depends_on/providers/lifecycle)
- **新增** `hclParser.ts`:HCL → JSON 解析器,`parseHCLModule` 返回 `{ moduleName, systemParams, userConfig }` 三层分离结构;`detectExtraFields` 对比 Schema 定义检测额外字段
- **新增** 语法高亮 CSS:`.hcl-keyword`(紫)、`.hcl-string`(绿)、`.hcl-bool`(红)、`.hcl-number`(橙)、`.hcl-attr`(蓝)、`.hcl-eq`(灰)、`.hcl-bracket`(白)、`.hcl-comment`(灰斜体)
- **新增** HCL 滚动修复:`scrollArea` 设为 `overflow: auto` 作为唯一滚动容器,`<pre>` 和 `<textarea>` 同步滚动,支持触摸板和滚动条

#### Terraform 系统参数管理

- **新增** `TF_SYSTEM_PARAMS` 常量集合:定义 `source`、`version`、`for_each`、`count`、`depends_on`、`providers`、`lifecycle` 为系统参数,解析时自动分离,不进入表单渲染
- **新增** 额外字段检测:用户在 HCL 中编辑了 Schema 未定义的字段时,显示黄色警告栏(⚠ 发现 N 个 Schema 未定义的字段:xxx),提供「保留」和「丢弃」两个操作按钮
- **新增** 系统参数 HCL 渲染:`jsonToHCL` 将 `systemParams` 渲染在 `source`/`version` 之后、用户配置之前,确保 HCL 输出完整

#### 版本管理与升级流程

- **新增** 版本选择器(ViewResource):Module 版本徽章旁显示 ↕ 下拉按钮,点击展开版本列表(按 semver 降序),当前版本显示紫色「当前」标签,最新版本显示绿色「最新」标签
- **新增** 升级快捷提示:当资源版本 < 模块最新版本时,版本徽章旁显示蓝色药丸 `[5.9.1 → 5.9.2 ↗]`,点击触发升级流程
- **新增** 版本切换确认对话框:点击任意非当前版本 → 弹出 ConfirmDialog,标题根据 semver 比较动态显示「升级模块版本」或「切换模块版本」,确认后跳转到 EditResource 编辑页面(带 `?upgrade_to=versionId` 参数)
- **新增** EditResource 升级模式:检测 URL `upgrade_to` 参数,自动加载目标版本的 Schema 渲染表单,预填变更摘要为「升级模块版本: X → Y」,用户确认配置后提交保存,模块版本正式变更

### 优化改进

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

### 技术细节

#### 架构设计

- **v2/v3 隔离机制**:通过 `<html data-ui-version>` 属性 + CSS 变量覆盖实现主题隔离,v2 模式下所有 v3 样式规则不生效;`FormRendererV3` 包装组件不修改原 `FormRenderer.tsx` 的任何逻辑
- **HCL 编辑器 overlay 方案**:查看模式渲染高亮 `<pre>`,编辑模式透明 `<textarea>` 绝对定位覆盖在 `<pre>` 上,`color: transparent` + `caret-color: #e2e8f0` 仅显示光标;两个元素共享 `scrollArea` 滚动容器,通过 `onScroll` 事件同步滚动位置
- **版本匹配策略**:资源版本必须在模块版本列表中存在才能渲染表单,不存在时降级到 HCL 视图;升级流程不直接修改资源,而是跳转到编辑页面让用户确认后再提交

#### 新增文件

- `frontend/src/hooks/useUIVersion.ts` — v2/v3 状态管理 hook
- `frontend/src/styles/v3-theme.css` — v3 CSS 变量主题
- `frontend/src/contexts/UIVersionContext.tsx` — v3 React Context
- `frontend/src/components/OpenAPIFormRenderer/FormRendererV3.tsx` — v3 表单渲染器包装
- `frontend/src/components/OpenAPIFormRenderer/FormRendererV3.module.css` — v3 样式覆盖
- `frontend/src/components/HCLEditor/HCLEditor.tsx` — HCL 双向编辑器
- `frontend/src/components/HCLEditor/HCLEditor.module.css` — HCL 编辑器样式 + 语法高亮
- `frontend/src/utils/hclFormatter.ts` — JSON → HCL 格式化
- `frontend/src/utils/hclParser.ts` — HCL → JSON 解析 + 系统参数分离
- `docs/ui-v3-plan.md` — UI v3 实施计划文档

#### 修改文件

- `frontend/src/components/OpenAPIFormRenderer/widgets/SwitchWidget.tsx` — v3 模式 Switch 修复 + 状态文字
- `frontend/src/pages/ViewResource.tsx` — 版本选择器、升级提示、HCLView、URL 参数同步、console.log 清理
- `frontend/src/pages/EditResource.tsx` — 版本锁定、升级模式、HCLEditor、URL 参数同步、console.log 清理
- `frontend/src/pages/AddResources.tsx` — HCLEditor 替换 JsonEditor、按钮标签条件化、console.log 清理
- `frontend/src/pages/CreateDemo.tsx` — HCLEditor 集成、错误提示条件化
- `frontend/src/pages/SchemaManagement.tsx` — HCLEditor 集成、tab 名称条件化
- `frontend/src/pages/WorkspaceResources.tsx` — 标签文案修正
- `frontend/src/components/EditResourceDialog.tsx` — v2/v3 Schema 渲染器 + HCLEditor
- `frontend/src/components/DemoPreview.tsx` — FormRendererV3 + HCLView
- `frontend/src/components/ResourceVersionDiff.tsx` — v3 模式 HCL 格式化差异值
- `frontend/src/services/schemaV2.ts` — `detectSchemaVersion` 标记 `@deprecated`,统一使用 `schemaVersionDetector.ts`

### 已知问题

- **HCL 编辑器语法高亮**:编辑模式下 textarea 透明文本覆盖在高亮 `<pre>` 上,长文本滚动同步可能存在轻微延迟(已通过 `onScroll` 事件优化,触摸板和滚动条均正常工作)
- **版本切换确认对话框**:当前使用 `ConfirmDialog` 组件,后续可考虑自定义更丰富的升级/降级提示 UI(如显示版本变更摘要、Schema 差异等)
- **TypeScript 编译警告**:项目中存在大量预先存在的 TS6133(未使用变量)警告,与 v3 修改无关,后续可统一清理

### 依赖变更

- 无新增后端依赖
- 无新增前端 npm 依赖
- 复用现有 `useSearchParams`、`useMemo`、`useState` 等 React hooks
- 复用现有 `listVersions`、`getVersionSchemas` 等 API 服务
