# UI v3 优化方案 — 实施计划

> **Demo 文件：** [`frontend/ui-demo-v2.html`](../frontend/ui-demo-v2.html)
> **创建日期：** 2026-06-09
> **状态：** 待确认

---

## 一、设计目标

| 维度 | 当前 v2 问题 | v3 方案 |
|------|-------------|---------|
| Description 层级 | 12px gray-500，与 label 14px 差距不够 | 12px gray-400 + label 13.5px gray-700，色彩浓度差拉开层级 |
| 配色系统 | Google Material `#4285F4` 与 Tailwind `#3b82f6` 混用 | 统一 Tailwind Slate/Gray 中性色 + `#3b82f6` 交互蓝 |
| Bool 开关 | 绿色 `#10b981` 不一致，无状态文字，嵌套场景值注入失败 | 蓝色主色 + 状态文字"已启用/未启用"，统一 Switch 行为 |
| 只读模式 | `filter: grayscale(30%)` 全局灰化 | 精确设置 background/border/text，无 filter 失真 |
| 分组 header | 纯色灰底无图标 | 渐变毛玻璃底 + 标题 + 分隔竖线 + 字段数 |
| 数据视图 | JSON 视图 | HCL 视图（Terraform 原生语言）+ 语法高亮 |
| Source/Version | 被 `const { source, version, ...configData }` 过滤掉 | 表单顶部 Source/Version 信息卡 |
| Input 圆角 | 8px | 10px |
| Card 阴影 | 无 | 极浅阴影 `0 1px 3px rgba(0,0,0,0.04)` |
| Console.log | 155+ 条调试语句 | 移除或 gate behind debug flag |

---

## 二、架构原则

### 核心约束：**v2/v3 开关切换，不影响 v2 任何功能**

```
用户切换 v3 → 使用新的 CSS 变量主题 + 新组件包装层
用户切换 v2 → 完全走原有代码路径，零改动
```

### 实现策略：CSS 变量覆盖 + 组件包装层

**不改 FormRenderer.tsx 的 JSX 结构和逻辑**，而是：

1. **新增 `FormRendererV3` 包装组件**：内部调用 `FormRenderer`，但包裹一个 `<div data-ui-version="v3">` 容器
2. **新增 `FormRendererV3.module.css`**：用 `[data-ui-version="v3"]` 选择器覆盖所有样式
3. **新增 Widget 包装层**：对需要改 JSX 的 widget（SwitchWidget、以及 HCL 视图组件）做条件渲染
4. **版本切换存储**：`localStorage` 持久化用户偏好

---

## 三、Schema 依赖关系全景图

### 3.1 Schema 数据模型

```
Module (模块)
  └── ModuleVersion (模块版本)
       └── Schema (Schema)
            ├── schema_version: v1 | v2
            ├── status: active | draft | deprecated
            ├── schema_data: JSONB (V1 格式)
            ├── openapi_schema: JSONB (V2 格式 — OpenAPI 3.1.0)
            ├── ui_config: JSONB (从 openapi_schema 提取)
            ├── variables_tf: TEXT (原始 Terraform HCL)
            ├── inherited_from_schema_id: int (继承来源)
            └── version: int (自增版本号)
```

### 3.2 Schema ↔ Module Version 关系

- 一个 `ModuleVersion` 可以有多个 `Schema`（通过 `module_version_id` 外键）
- `Schema.version` 在同一个 `ModuleVersion` 下自增
- 创建新的 `ModuleVersion` 时，通过 `inheritSchema` 从上一个版本继承 schema（设置 `inherited_from_schema_id`）
- 前端切换 module version 时，重新调用 `GET /modules/:id/schemas` 获取该版本下的 schema 列表

### 3.3 V1/V2 双版本系统

| 维度 | V1 | V2 |
|------|----|----|
| 数据格式 | 扁平 JSON，type 用数字（1=bool, 2=int...） | OpenAPI 3.1.0 + `x-iac-platform` 扩展 |
| 前端渲染 | `DynamicForm`（纯 HTML/CSS，零 antd） | `OpenAPIFormRenderer`（深度耦合 antd） |
| 后端解析 | `parseVariablesFile`（简单正则） | `ParseTFWithOutputs`（完整 HCL 解析器） |
| 迁移路径 | `schemaMigrator.ts`（前端）+ `convertV1ToV2`（后端） | — |
| 版本检测 | `schemaVersionDetector.ts` + `schemaV2.ts`（双重实现，需统一） | — |

### 3.4 OpenAPI Schema 内部结构（V2）

```json
{
  "openapi": "3.1.0",
  "components": {
    "schemas": {
      "ModuleInput": {
        "type": "object",
        "required": ["instance_type", "ami_id"],
        "properties": {
          "instance_type": {
            "type": "string",
            "description": "选择 EC2 实例的类型和大小",
            "enum": ["t3.medium", "t3.large"],
            "x-widget": "select",
            "x-group": "basic",
            "x-order": 1,
            "x-col-span": 24
          }
        }
      }
    }
  },
  "x-iac-platform": {
    "ui": {
      "groups": [
        { "id": "basic", "title": "基础配置", "level": "basic", "layout": "sections", "order": 1 },
        { "id": "advanced", "title": "高级配置", "level": "advanced", "layout": "accordion", "order": 100 }
      ],
      "fields": {
        "instance_type": { "widget": "select", "group": "basic", "order": 1, "colSpan": 24, "label": "实例类型" }
      },
      "layout": { "type": "sections", "position": "top" }
    },
    "external": {
      "sources": {
        "ami_list": { "type": "api", "url": "/api/v1/external-data/ami-list", "transform": "..." }
      }
    },
    "cascade": {
      "rules": [
        { "trigger": { "field": "instance_type", "operator": "eq", "value": "t3.large" },
          "actions": [{ "type": "show", "fields": ["dedicated_host"] }] }
      ]
    },
    "outputs": {
      "items": [
        { "name": "instance_id", "type": "string", "value": "${module.ec2.instance_id}" }
      ]
    }
  }
}
```

### 3.5 分组系统

**三种布局模式：**

| 模式 | 触发条件 | 渲染 |
|------|---------|------|
| `tabs` | 任一 group 使用 `layout: 'tabs'` | 单个 `<Tabs>` 组件 |
| `accordion` | 所有 group 使用 `accordion` | 单个 `<Collapse>` 组件 |
| `sections` | 所有 group 使用 `sections` | 始终展开的 section 卡片 |
| `mixed` | 混合使用不同非 tabs 布局 | 各 group 独立渲染 |

**GroupConfig 类型不一致问题：**
- `schemaV2.ts`：`{ id, title, icon?, order, defaultExpanded? }`
- `types.ts`（FormRenderer）：额外有 `layout`, `level`, `hiddenByDefault`
- `OpenAPISchemaEditor`：用 `label` 而非 `title`
- FormRenderer 通过 `g.label || g.title || g.id` 桥接

**默认分组：** `basic`（基础配置）和 `advanced`（高级配置），在三处硬编码定义。

### 3.6 字段排序与布局

- **字段排序**：`x-iac-platform.ui.fields.<name>.order`，默认 999
- **列宽**：`x-col-span`（24 列网格），默认 24
- **布局视图**：OpenAPISchemaEditor 内置可视化拖拽布局编辑器（`@dnd-kit`）
- **行分割**：`splitFieldsIntoRows()` 根据 colSpan 累计值分行

### 3.7 级联规则系统

**14 种触发操作符：** `eq`, `ne`, `gt`, `lt`, `gte`, `lte`, `in`, `notIn`, `empty`, `notEmpty`, `contains`, `startsWith`, `endsWith`, `matches`

**9 种动作类型：** `show`, `hide`, `enable`, `disable`, `setValue`, `setOptions`, `setRequired`, `clearValue`, `reloadSource`

**字段级快捷语法：** `showWhen`, `hideWhen`, `requiredWith`, `conflictsWith`（自动转为 CascadeRule）

### 3.8 Widget 系统（16 种类型，15 个组件文件）

| Widget 类型 | 文件 | antd 依赖 | 关键特性 |
|------------|------|----------|---------|
| text | TextWidget | Form, Input, Tag, Tooltip | 占位符检测，`/` 引用触发 |
| textarea | TextareaWidget | Form, Input, Tag, Tooltip | 数组模式（逐行），引用支持 |
| number | NumberWidget | Form, InputNumber, Input | 引用切换文本模式 |
| select | SelectWidget | Form, Select, Spin | 静态 enum + 外部数据源 |
| multi-select | SelectWidget | 同上 | mode="multiple" |
| **switch** | **SwitchWidget** | **Form, Switch, Tag** | **valuePropName="checked"，hint 标签** |
| tags | TagsWidget | Form, Select, Tag | tokenSeparators，引用 |
| key-value | KeyValueWidget | Form, Input, Button, Space | 动态键值对 |
| object | ObjectWidget | Form, Card, Collapse, Row, Col | 递归渲染，basic/advanced 分组 |
| object-list | ObjectListWidget | Form, Card, Button, Collapse, Tabs | 增删复制，≥3 项自动折叠 |
| dynamic-object | DynamicObjectWidget | Form, Card, Button, Empty | 自动生成 key，Schema 推断 |
| json-editor | JsonEditorWidget | Form, Input, Button, Alert | JSON 校验 + 格式化 |
| password | PasswordWidget | Form, Input, Progress | 强度指示器 |
| datetime | DateTimeWidget | Form, DatePicker | dayjs，多模式 |
| code-editor | CodeEditorWidget | Form, Input, Button, Select | 多语言，全屏，复制 |
| cmdb-select | CMDBSelectWidget | Form, AutoComplete, Select | CMDB 资源搜索 |

**关键共享依赖：** `ModuleReferencePopover` 被 10/15 个 widget 引用。

### 3.9 外部数据源

- **类型：** `api` | `static` | `terraform`（未实现）
- **变量替换：** `${providers.aws.region}`, `${fields.vpc_id}`, `${workspace.id}`
- **数据转换：** 简化 JMESPath / JSONPath
- **缓存：** 可配 TTL（默认 300s）
- **依赖追踪：** `dependsOn` 字段变更触发重新加载

### 3.10 后端校验引擎（SchemaSolver）

**11 步校验流水线：**
1. enum 校验 → 2. 类型校验 → 3. 字符串约束 → 4. 数字约束 → 5. 数组约束
6. conflicts_with → 7. depends_on → 8. required → 9. implies 规则
10. conditional 规则 → 11. map must_include

---

## 四、影响范围分析（Blast Radius）

### 4.1 直接受影响的文件

| 模块 | TSX/TS 文件 | CSS 文件 | 合计 |
|------|-----------|---------|------|
| OpenAPIFormRenderer（V2 渲染器） | 22 | 2 | 24 |
| DynamicForm（V1 渲染器） | 8 | 8 | 16 |
| OpenAPISchemaEditor（编辑器） | 1 | 1 | 2 |
| ModuleFormRenderer（兼容包装） | 1 | 0 | 1 |
| ModuleReferencePopover（共享组件） | 1 | 1 | 2 |
| **合计** | **33** | **12** | **45** |

### 4.2 受影响的页面（9 个）

| 页面 | 使用的渲染器 |
|------|------------|
| ViewResource.tsx | OpenAPIFormRenderer + FormPreview(V1) |
| EditResource.tsx | OpenAPIFormRenderer + DynamicForm(V1) + AIFormAssistant |
| AddResources.tsx | OpenAPIFormRenderer + DynamicForm(V1) + AIFormAssistant |
| SchemaManagement.tsx | OpenAPIFormRenderer + DynamicForm(V1) + OpenAPISchemaEditor |
| CreateDemo.tsx | OpenAPIFormRenderer + AIFormAssistant |
| EditDemo.tsx | ModuleFormRenderer |
| DemoDetail.tsx | ModuleFormRenderer |
| ImportModule.tsx | OpenAPISchemaEditor |
| EditResourceDialog.tsx | DynamicForm(V1) |

### 4.3 Ant Design 使用统计

- **OpenAPIFormRenderer 生态**：深度耦合 antd（Form, Input, Select, Switch, Collapse, Tabs, Card, Row/Col, Tooltip, Tag 等 20+ 组件）
- **DynamicForm（V1）**：零 antd 依赖（纯 HTML/CSS）
- **OpenAPISchemaEditor**：零 antd 依赖（@dnd-kit + Monaco Editor + 纯 HTML）
- **全局 antd 覆盖**：`FormRenderer.module.css` 有 19 处 `:global(.ant-*)` 选择器

---

## 五、实施方案

### Phase 1：基础设施（不影响现有功能）

#### 1.1 版本切换机制

**新增文件：**
- `frontend/src/hooks/useUIVersion.ts` — 版本管理 hook

```typescript
// localStorage key: 'ui-version-preference'
// 默认值: 'v2'
// 返回: { version, setVersion, isV3 }

export function useUIVersion() {
  const [version, setVersion] = useState(
    () => localStorage.getItem('ui-version-preference') || 'v2'
  );
  const setVersionAndPersist = (v: string) => {
    localStorage.setItem('ui-version-preference', v);
    setVersion(v);
  };
  return {
    version,
    setVersion: setVersionAndPersist,
    isV3: version === 'v3',
  };
}
```

**切换入口：** 在页面顶部工具栏或设置区域添加 v2/v3 切换开关。

#### 1.2 V3 CSS 变量主题

**新增文件：**
- `frontend/src/styles/v3-theme.css`

```css
/* 在 :root 下新增 v3 专属变量，不修改现有变量 */
:root {
  /* v3 中性色 — Tailwind Slate */
  --v3-gray-50: #f8fafc;
  --v3-gray-100: #f1f5f9;
  --v3-gray-200: #e2e8f0;
  --v3-gray-300: #cbd5e1;
  --v3-gray-400: #9ca3af;
  --v3-gray-500: #6b7280;
  --v3-gray-600: #4b5563;
  --v3-gray-700: #374151;
  --v3-gray-800: #1f2937;
  --v3-gray-900: #111827;

  /* v3 交互色 */
  --v3-blue-400: #60a5fa;
  --v3-blue-500: #3b82f6;
  --v3-blue-600: #2563eb;

  /* v3 圆角 */
  --v3-radius-input: 10px;
  --v3-radius-card: 16px;
  --v3-radius-section: 14px;

  /* v3 间距 */
  --v3-field-gap: 20px;
  --v3-label-input-gap: 6px;

  /* v3 阴影 */
  --v3-shadow-card: 0 1px 3px rgba(0,0,0,0.04), 0 1px 2px rgba(0,0,0,0.02);
  --v3-shadow-section: 0 1px 3px rgba(0,0,0,0.04);
}
```

#### 1.3 HCL 转换工具

**新增文件：**
- `frontend/src/utils/hclFormatter.ts`

```typescript
// jsonToHCL(data, moduleSource, moduleVersion, moduleName) → string
// 支持：string, number, boolean, array, object, map, null
// 语法高亮通过 CSS class 实现（非运行时染色）
```

核心逻辑：
- `string` → `"value"`
- `number` → `123`
- `boolean` → `true` / `false`
- `array` → `["a", "b"]` 或多行 `[\n  "a",\n  "b",\n]`
- `object` → `{ key = "value" }` 或多行 `{ key = "value" }`
- `map` → 同 object 格式
- `null` → 跳过或注释

### Phase 2：FormRenderer V3 包装层

#### 2.1 FormRendererV3 组件

**新增文件：**
- `frontend/src/components/OpenAPIFormRenderer/FormRendererV3.tsx`
- `frontend/src/components/OpenAPIFormRenderer/FormRendererV3.module.css`

**策略：** 不修改 `FormRenderer.tsx`，而是：
1. `FormRendererV3` 内部调用 `FormRenderer`，包裹 `<div data-ui-version="v3">`
2. `FormRendererV3.module.css` 通过 `[data-ui-version="v3"]` 选择器覆盖样式
3. 对需要 JSX 变更的组件（SwitchWidget、HCL 视图），通过 props 条件渲染

```typescript
// FormRendererV3.tsx
const FormRendererV3: React.FC<FormRendererProps> = (props) => {
  return (
    <div data-ui-version="v3" className={styles.v3Wrapper}>
      <FormRenderer {...props} />
    </div>
  );
};
```

#### 2.2 样式覆盖清单

| 覆盖项 | 选择器 | v2 值 | v3 值 |
|--------|--------|-------|-------|
| Input 圆角 | `.ant-input`, `.ant-select-selector` | 8px | 10px |
| Section header | `.sectionHeader` | `background: gray-50` | `linear-gradient(135deg, ...)` + `backdrop-filter` |
| Field gap | `.fieldGroup` | `gap: 16px` | `gap: 20px` |
| Label-Input 间距 | `.ant-form-item` margin | 4px | 6px |
| Description 颜色 | `.ant-form-item-explain` | `gray-500` | `gray-400` |
| Switch 颜色 | `.ant-switch-checked` | `#10b981` | `#3b82f6` |
| 只读模式 | `.formRendererReadOnly` | `grayscale(30%)` | 精确 background/border |
| Card 阴影 | `.formSection` | none | `var(--v3-shadow-card)` |
| Field count badge | `.fieldCount` | blue-100/blue-600 | slate-100/slate-500 |

#### 2.3 SwitchWidget V3 增强

**修改文件：** `SwitchWidget.tsx`

通过检测 `data-ui-version` context 或新增 prop 来条件渲染：

```typescript
// v3: 添加状态文字
const SwitchWidgetV3 = ({ checked, ... }) => (
  <div className="pro-switch-row">
    <Switch checked={checked} ... />
    <span className={checked ? 'on' : 'off'}>
      {checked ? '已启用' : '未启用'}
    </span>
  </div>
);
```

### Phase 3：数据视图增强

#### 3.1 HCL 视图组件

**新增文件：**
- `frontend/src/components/HCLView/HCLView.tsx`
- `frontend/src/components/HCLView/HCLView.module.css`
- `frontend/src/utils/hclFormatter.ts`

**集成点：** `ViewResource.tsx` 的数据视图区域，将 "JSON视图" 按钮改为 "HCL 视图"（v3 模式下），v2 模式保持 JSON。

```typescript
// ViewResource.tsx 中的条件渲染
{isV3 ? (
  <HCLView data={displayData} source={moduleSource} version={moduleVersion} />
) : (
  <pre>{JSON.stringify(displayData, null, 2)}</pre>
)}
```

#### 3.2 Source/Version 信息卡

**修改页面：** `ViewResource.tsx`, `EditResource.tsx`

在表单顶部（v3 模式下）添加：

```typescript
{isV3 && moduleSource && (
  <div className={styles.sourceCard}>
    <span>Source: {moduleSource}</span>
    <span>Version: {moduleVersion}</span>
  </div>
)}
```

注意：不再用 `const { source, version, ...configData }` 过滤掉这两个字段，而是单独展示。

### Phase 4：代码清理

#### 4.1 移除 console.log

**文件：** `EditResource.tsx`（100 处）, `ViewResource.tsx`（55 处）

策略：批量移除 emoji 前缀的 `console.log`，保留 `console.error` 和 `console.warn`。

#### 4.2 统一 schema 版本检测

**删除：** `schemaV2.ts` 中的 `detectSchemaVersion` 重复实现
**统一使用：** `schemaVersionDetector.ts` 的实现

---

## 六、实施顺序与依赖关系

```
Phase 1（基础设施，互不依赖，可并行）
  ├── 1.1 useUIVersion hook
  ├── 1.2 v3-theme.css
  └── 1.3 hclFormatter.ts

Phase 2（依赖 Phase 1）
  ├── 2.1 FormRendererV3 包装组件
  ├── 2.2 FormRendererV3.module.css 样式覆盖
  └── 2.3 SwitchWidget V3 增强

Phase 3（依赖 Phase 2）
  ├── 3.1 HCL 视图组件 + 集成到 ViewResource
  └── 3.2 Source/Version 信息卡 + 集成到 View/EditResource

Phase 4（独立，可随时进行）
  ├── 4.1 移除 console.log
  └── 4.2 统一 schema 版本检测
```

---

## 七、风险控制

| 风险 | 缓解措施 |
|------|---------|
| v3 CSS 覆盖影响 v2 | 所有 v3 样式限定在 `[data-ui-version="v3"]` 选择器内 |
| SwitchWidget 改动影响 V1 DynamicForm | V1 DynamicForm 不使用 SwitchWidget（纯 HTML button） |
| HCL 格式化不支持复杂嵌套 | 第一版支持 string/number/bool/array/object/map，复杂类型降级为注释提示 |
| antd `:global()` 选择器优先级 | v3 CSS 使用更具体的选择器 + `!important` 仅在必要时 |
| 版本切换后缓存问题 | 切换版本时清除 React 组件 key 强制重新渲染 |

---

## 八、不在本次范围内

- OpenAPISchemaEditor 3462 行单体拆分（独立任务，影响范围更大）
- V1 DynamicForm 全面移除（需等 V1 使用率降到零）
- Schema 加载逻辑去重（独立重构任务）
- 后端聚合 API（需要后端改动）
- React Query / SWR 缓存层（独立架构升级）

---

## 九、文件清单

### 新增文件（8 个）

| 文件 | 用途 |
|------|------|
| `frontend/src/hooks/useUIVersion.ts` | 版本切换 hook |
| `frontend/src/styles/v3-theme.css` | V3 CSS 变量主题 |
| `frontend/src/utils/hclFormatter.ts` | JSON → HCL 转换 |
| `frontend/src/components/OpenAPIFormRenderer/FormRendererV3.tsx` | V3 包装组件 |
| `frontend/src/components/OpenAPIFormRenderer/FormRendererV3.module.css` | V3 样式覆盖 |
| `frontend/src/components/HCLView/HCLView.tsx` | HCL 视图组件 |
| `frontend/src/components/HCLView/HCLView.module.css` | HCL 视图样式 |
| `docs/ui-v3-plan.md` | 本文档 |

### 修改文件（4 个）

| 文件 | 修改内容 |
|------|---------|
| `frontend/src/components/OpenAPIFormRenderer/widgets/SwitchWidget.tsx` | V3 条件渲染状态文字 |
| `frontend/src/pages/ViewResource.tsx` | V3 模式下 HCL 视图 + Source/Version 卡 + 移除 console.log |
| `frontend/src/pages/EditResource.tsx` | V3 模式下 Source/Version 卡 + 移除 console.log |
| `frontend/src/App.tsx`（或 Layout 组件） | 添加 v2/v3 切换入口 |

### 不动的文件

- `FormRenderer.tsx`（核心渲染逻辑不变）
- `DynamicForm/`（V1 完全不动）
- `OpenAPISchemaEditor/`（本期不动）
- 所有 backend 文件
- `schemaV2.ts`（类型定义和服务层不变）
- `CascadeEngine.ts`（级联引擎不变）
- `ExternalDataSourceManager.ts`（外部数据源不变）
- 所有 widget 文件（SwitchWidget 除外）
