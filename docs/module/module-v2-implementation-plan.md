# Module V2 OpenAPI Schema 实施计划

## 1. 项目概述

### 1.1 目标
- 使用OpenAPI v3规范重构Module前端渲染逻辑
- 支持Schema版本升级（v1 → v2）
- 兼容现有v1方案，默认使用v2
- 集成tf2openapi工具到平台
- 保留Demo功能
- 支持Schema的UI/JSON编辑

### 1.2 当前状态分析

**已有组件：**
- `frontend/src/pages/Modules.tsx` - 模块列表页
- `frontend/src/pages/CreateModule.tsx` - 创建模块页
- `frontend/src/pages/ImportModule.tsx` - 导入模块页（支持JSON/TF/Git）
- `frontend/src/pages/ModuleDetail.tsx` - 模块详情页
- `frontend/src/pages/ModuleSchemas.tsx` - Schema管理页
- `frontend/src/components/DynamicForm/` - 动态表单组件
- `frontend/src/components/DemoList.tsx` - Demo列表
- `frontend/src/components/DemoForm.tsx` - Demo表单
- `frontend/src/components/DemoSelector.tsx` - Demo选择器
- `frontend/src/services/modules.ts` - 模块服务
- `frontend/src/services/moduleDemos.ts` - Demo服务

**后端模型：**
- `Module` - 模块基本信息
- `Schema` - Schema定义（SchemaData为JSONB）
- `ModuleDemo` - Demo数据

**已完成的OpenAPI设计：**
- `backend/cmd/tools/tf2openapi/main.go` - TF转OpenAPI工具
- `docs/module/openapi-schema-design.md` - 设计文档
- `docs/module/schema-form-renderer.html` - 渲染器预览

## 2. 工作量评估

### 2.1 后端开发 (预计 3-4 天)

| 任务 | 复杂度 | 预计时间 | 说明 |
|------|--------|----------|------|
| Schema模型升级 | 中 | 0.5天 | 添加schema_version字段，支持v1/v2 |
| tf2openapi API集成 | 中 | 1天 | 将工具逻辑封装为API |
| Schema解析服务 | 中 | 0.5天 | 解析variables.tf并返回OpenAPI Schema |
| Schema CRUD增强 | 低 | 0.5天 | 支持部分更新、字段级编辑 |
| 外部数据源API | 高 | 1天 | AMI/VPC/Subnet等数据源API |
| 数据迁移脚本 | 低 | 0.5天 | v1 Schema迁移到v2格式 |

### 2.2 前端开发 (预计 5-7 天)

| 任务 | 复杂度 | 预计时间 | 说明 |
|------|--------|----------|------|
| **Schema导入向导** | 高 | 1.5天 | |
| - Variables.tf上传界面 | 中 | 0.5天 | 拖拽上传、粘贴支持 |
| - 注释规范说明弹窗 | 低 | 0.25天 | 显示支持的注释格式 |
| - 参数自定义配置界面 | 高 | 0.75天 | 每个参数的UI配置 |
| **Schema编辑器** | 高 | 2天 | |
| - UI可视化编辑器 | 高 | 1天 | 字段拖拽、分组管理 |
| - JSON编辑器增强 | 中 | 0.5天 | Monaco编辑器+验证 |
| - 字段级CRUD | 中 | 0.5天 | 添加/删除/修改单个字段 |
| **表单渲染器V2** | 高 | 2天 | |
| - OpenAPI Schema解析 | 中 | 0.5天 | 解析x-iac-platform扩展 |
| - Widget组件库 | 高 | 1天 | 所有Widget类型实现 |
| - 级联规则引擎 | 中 | 0.5天 | 字段联动逻辑 |
| **兼容性处理** | 中 | 1天 | |
| - v1/v2 Schema检测 | 低 | 0.25天 | 自动识别Schema版本 |
| - v1渲染器保留 | 低 | 0.25天 | 旧Schema继续使用v1 |
| - 迁移提示UI | 低 | 0.5天 | 提示用户升级到v2 |
| **Demo功能保留** | 低 | 0.5天 | |
| - Demo与v2 Schema兼容 | 低 | 0.5天 | 确保Demo数据格式兼容 |

### 2.3 测试与文档 (预计 1-2 天)

| 任务 | 预计时间 |
|------|----------|
| 单元测试 | 0.5天 |
| 集成测试 | 0.5天 |
| 用户文档更新 | 0.5天 |
| 迁移指南 | 0.5天 |

### 2.4 总工作量

| 阶段 | 预计时间 |
|------|----------|
| 后端开发 | 3-4 天 |
| 前端开发 | 5-7 天 |
| 测试与文档 | 1-2 天 |
| **总计** | **9-13 天** |

## 3. 实施计划

### Phase 1: 基础设施 (第1-2天)

1. **数据库Schema变更**
   ```sql
   ALTER TABLE schemas ADD COLUMN schema_version VARCHAR(10) DEFAULT 'v1';
   ALTER TABLE schemas ADD COLUMN openapi_schema JSONB;
   ALTER TABLE schemas ADD COLUMN variables_tf TEXT;
   ```

2. **后端API开发**
   - `POST /api/v1/modules/parse-tf-v2` - 解析variables.tf返回OpenAPI Schema
   - `GET /api/v1/modules/:id/schemas/v2` - 获取v2 Schema
   - `PUT /api/v1/modules/:id/schemas/v2` - 更新v2 Schema
   - `PATCH /api/v1/modules/:id/schemas/v2/fields/:fieldName` - 更新单个字段

### Phase 2: Schema导入向导 (第3-4天)

1. **Variables.tf上传组件**
   - 文件拖拽上传
   - 文本粘贴支持
   - 实时预览解析结果

2. **注释规范说明**
   - 弹窗显示支持的注释格式
   - 示例代码展示
   - 链接到完整文档

3. **参数配置界面**
   - 解析后的参数列表
   - 每个参数可配置：
     - 分组 (basic/advanced)
     - Widget类型
     - 标签/别名
     - 帮助文本
     - 验证规则

### Phase 3: Schema编辑器 (第5-6天)

1. **UI可视化编辑器**
   - 字段列表（可拖拽排序）
   - 分组管理
   - 字段属性编辑面板

2. **JSON编辑器**
   - Monaco Editor集成
   - JSON Schema验证
   - 语法高亮

3. **双向同步**
   - UI编辑 → JSON更新
   - JSON编辑 → UI更新

### Phase 4: 表单渲染器V2 (第7-9天)

1. **Schema解析器**
   - 解析OpenAPI 3.1 Schema
   - 解析x-iac-platform扩展
   - 构建表单配置

2. **Widget组件库**
   - TextWidget
   - NumberWidget
   - SelectWidget (支持外部数据源)
   - SwitchWidget
   - TagsWidget
   - KeyValueWidget
   - ObjectWidget
   - ObjectListWidget
   - JsonEditorWidget

3. **级联规则引擎**
   - 字段显示/隐藏
   - 字段启用/禁用
   - 值联动

### Phase 5: 兼容性与迁移 (第10-11天)

1. **版本检测**
   - 自动识别v1/v2 Schema
   - 选择对应渲染器

2. **迁移工具**
   - v1 → v2 转换脚本
   - 迁移提示UI
   - 批量迁移支持

### Phase 6: 测试与文档 (第12-13天)

1. **测试**
   - 单元测试
   - 集成测试
   - E2E测试

2. **文档**
   - 用户指南
   - 迁移指南
   - API文档

## 4. 技术方案

### 4.1 Schema版本识别

```typescript
function detectSchemaVersion(schema: any): 'v1' | 'v2' {
  // v2 Schema 特征：包含 openapi 字段
  if (schema.openapi && schema.openapi.startsWith('3.')) {
    return 'v2';
  }
  // v2 Schema 特征：包含 x-iac-platform 扩展
  if (schema['x-iac-platform']) {
    return 'v2';
  }
  // 默认为 v1
  return 'v1';
}
```

### 4.2 渲染器选择

```typescript
function renderModuleForm(schema: any, data: any) {
  const version = detectSchemaVersion(schema);
  
  if (version === 'v2') {
    return <OpenAPIFormRenderer schema={schema} data={data} />;
  } else {
    return <LegacyFormRenderer schema={schema} data={data} />;
  }
}
```

### 4.3 外部数据源集成

```typescript
interface ExternalDataSource {
  id: string;
  type: 'api' | 'static';
  api: string;
  params?: Record<string, string>;
  cache?: { ttl: number };
  transform?: { type: string; expression: string };
}

async function loadExternalData(source: ExternalDataSource, context: FormContext) {
  const params = resolveParams(source.params, context);
  const response = await api.get(source.api, { params });
  return transformData(response.data, source.transform);
}
```

## 5. 文件结构

```
frontend/src/
├── components/
│   ├── ModuleSchemaV2/
│   │   ├── SchemaImportWizard.tsx      # 导入向导
│   │   ├── VariablesTfUploader.tsx     # TF文件上传
│   │   ├── AnnotationGuide.tsx         # 注释规范说明
│   │   ├── FieldConfigPanel.tsx        # 字段配置面板
│   │   ├── SchemaVisualEditor.tsx      # 可视化编辑器
│   │   ├── SchemaJsonEditor.tsx        # JSON编辑器
│   │   └── index.tsx
│   ├── OpenAPIFormRenderer/
│   │   ├── FormRenderer.tsx            # 主渲染器
│   │   ├── SchemaParser.tsx            # Schema解析
│   │   ├── CascadeEngine.tsx           # 级联引擎
│   │   ├── widgets/
│   │   │   ├── TextWidget.tsx
│   │   │   ├── NumberWidget.tsx
│   │   │   ├── SelectWidget.tsx
│   │   │   ├── SwitchWidget.tsx
│   │   │   ├── TagsWidget.tsx
│   │   │   ├── KeyValueWidget.tsx
│   │   │   ├── ObjectWidget.tsx
│   │   │   ├── ObjectListWidget.tsx
│   │   │   └── JsonEditorWidget.tsx
│   │   └── index.tsx
│   └── DynamicForm/                    # 保留v1组件
├── pages/
│   ├── ModuleSchemaEditor.tsx          # Schema编辑页面
│   └── ...
└── services/
    ├── schemaParser.ts                 # Schema解析服务
    └── externalDataSource.ts           # 外部数据源服务

backend/
├── cmd/tools/tf2openapi/               # 已有工具
├── internal/
│   ├── handlers/
│   │   └── module_schema_v2_handler.go # v2 Schema API
│   └── models/
│       └── schema.go                   # 更新Schema模型
└── services/
    └── schema_parser_service.go        # Schema解析服务
```

## 6. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| v1/v2兼容性问题 | 高 | 充分测试，保留v1渲染器 |
| 外部数据源API延迟 | 中 | 添加缓存，异步加载 |
| 复杂Schema解析错误 | 中 | 完善错误处理，提供手动编辑 |
| 用户迁移阻力 | 低 | 提供自动迁移工具，保持向后兼容 |

## 7. 验收标准

1. ✅ 用户可以上传/粘贴variables.tf文件
2. ✅ 系统自动解析并生成OpenAPI Schema
3. ✅ 用户可以自定义每个参数的UI配置
4. ✅ 支持UI可视化编辑和JSON编辑
5. ✅ 表单渲染器正确渲染v2 Schema
6. ✅ v1 Schema继续正常工作
7. ✅ Demo功能正常
8. ✅ 支持Schema的增量更新

## 8. 开发进度跟踪

### 当前状态: 🚧 开发中

### Phase 1: 基础设施 ✅ 完成

| 任务 | 状态 | 完成时间 |
|------|------|----------|
| 数据库Schema变更脚本 | ✅ 完成 | 2024-12-28 |
| 后端Schema模型更新 | ✅ 完成 | 2024-12-28 |
| tf2openapi API集成 | ✅ 完成 | 2024-12-28 |
| Schema解析服务 | ✅ 完成 | 2024-12-28 |

### Phase 2: Schema导入向导 ✅ 完成

| 任务 | 状态 | 完成时间 |
|------|------|----------|
| Variables.tf上传组件 | ✅ 完成 | 2024-12-28 |
| 注释规范说明弹窗 | ✅ 完成 | 2024-12-28 |
| 参数配置界面 | ✅ 完成 | 2024-12-28 |

### Phase 3: Schema编辑器 ✅ 完成

| 任务 | 状态 | 完成时间 |
|------|------|----------|
| UI可视化编辑器 | ✅ 完成 | 2024-12-28 |
| JSON编辑器增强 | ✅ 完成 | 2024-12-28 |
| 字段级CRUD | ✅ 完成 | 2024-12-28 |

### Phase 4: 表单渲染器V2 ✅ 完成

| 任务 | 状态 | 完成时间 |
|------|------|----------|
| OpenAPI Schema解析 | ✅ 完成 | 2024-12-28 |
| Widget组件库 | ✅ 完成 | 2024-12-28 |
| 级联规则引擎 | ⏳ 基础完成 | 2024-12-28 |

### Phase 5: 兼容性与迁移 ✅ 完成

| 任务 | 状态 | 完成时间 |
|------|------|----------|
| v1/v2 Schema检测 | ✅ 完成 | 2024-12-28 |
| v1渲染器保留 | ✅ 完成 | 2024-12-28 |
| 迁移提示UI | ✅ 完成 | 2024-12-28 |

### Phase 6: 测试与文档 ⬜ 待开始

| 任务 | 状态 | 完成时间 |
|------|------|----------|
| 单元测试 | ⬜ 待开始 | - |
| 集成测试 | ⬜ 待开始 | - |
| 用户文档 | ⬜ 待开始 | - |

---

**图例:** ✅ 完成 | ⏳ 进行中 | ⬜ 待开始 | ❌ 阻塞
