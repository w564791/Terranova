## v0.3.4

嵌套字段布局、Widget 值存储修正、模块版本自动创建与删除修复。

### Features

- **嵌套字段（子参数）列宽布局** — Object/ObjectList/DynamicObject 类型的嵌套字段支持 `x-colSpan` 属性，子参数可按列宽并排渲染（如两个 `colSpan=12` 的字段同行显示）；顶层字段分组同样支持 `colSpan` 分行渲染 (`FormRenderer.tsx`, `ObjectWidget.tsx`, `ObjectListWidget.tsx`, `DynamicObjectWidget.tsx`)
- **Schema 编辑器嵌套字段列宽控制** — NestedFieldInlineEditor UI 面板新增"列宽"下拉框，可为子参数设置 `x-colSpan`（24/12/8/6/16/18）(`OpenAPISchemaEditor/index.tsx`)
- **Schema 编辑器嵌套字段拖拽布局** — NestedFieldsEditor 新增"布局"视图，支持拖拽子参数到同一行，自动计算等分 `x-colSpan`；使用 `DndContext` + droppable 行实现 (`OpenAPISchemaEditor/index.tsx`, `OpenAPISchemaEditor.module.css`)
- **模块版本自动补全** — `ListVersions` 接口在检测到模块无版本记录时，自动创建默认版本并关联已有 Schema/Demo，兼容多版本功能上线前的旧模块 (`module_version_service.go`)

### Bug Fixes

- **模块删除外键冲突** — 修复删除模块时 `module_versions.active_schema_id` 外键阻止删除 schemas 的问题；删除顺序改为：清除 FK 引用 → 删除 schemas → 删除 module_versions → 删除 module (`module_service.go`)
- **敏感变量未解密** — 修复 Local 模式和 Agent 模式下敏感变量未解密的问题，原因是原生 SQL 查询绕过了 GORM AfterFind Hook，改为查询后手动解密 (`local_data_accessor.go`, `agent_handler.go`)
- **模块创建时版本未生效** — 修复 `CreateModule` 设置 `default_version_id` 时使用 `tx.Model(module)` 导致更新可能未命中的问题，改为 `tx.Model(&models.Module{}).Where("id = ?", module.ID)` 精确更新 (`module_service.go`)
- **JsonEditorWidget 值类型错误** — 修复 JSON 编辑器始终以字符串存储值的问题；当 schema.type 为 object/array 时存储为解析后的对象，引用表达式（`module.*`, `var.*` 等）保持字符串 (`JsonEditorWidget.tsx`)
- **Schema 编辑器切换 Widget 时默认值不同步** — 切换到 json-editor 时自动适配默认值输入框，切换 Widget 类型时清除不兼容的默认值 (`OpenAPISchemaEditor/index.tsx`)
- **ImportModule 错误提示失效** — 修复 API interceptor 以字符串 reject 而非 Error 对象导致 catch 中 `err.message` 为 undefined 的问题 (`ImportModule.tsx`)
- **ImportModule 硬编码地址** — 移除 `fetch('http://localhost:8080/...')` 硬编码调用，统一使用 `api.post()` 走动态 baseURL (`ImportModule.tsx`)
- **ImportModule 重复模块检测失败** — 修复模块列表解析未处理嵌套 `data.items` 结构导致重复检测始终失败的问题 (`ImportModule.tsx`)
- **TF 文件类型过滤** — `parseTF` 请求增加 `file_type_filter: 'variables_outputs'` 参数，避免解析非 variable/output 定义 (`schemaV2.ts`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.3.3...v0.3.4
