# Schema编辑功能开发指南

## 概述

本文档描述Schema编辑功能的设计和实现方案。该功能允许用户在SchemaManagement页面编辑Schema配置，但根据Schema的来源类型区分编辑权限。

## 核心设计理念

### Schema来源分类

根据Schema的**来源**区分编辑方式：

#### 1. JSON导入的Schema - 只读模式 📄
- **来源**：用户通过"导入JSON"按钮导入
- **特点**：完整的Schema配置（包含所有30+字段）
- **编辑方式**：**不支持表单编辑**，只能查看或重新导入
- **原因**：
  - JSON Schema可能包含复杂的字段关系
  - 表单编辑可能破坏Schema的完整性
  - 用户如果需要修改，应该修改JSON后重新导入

#### 2. TF文件解析的Schema - 表单编辑模式 ✏️
- **来源**：用户通过"TF文件"导入，系统解析生成
- **特点**：基础Schema配置（只包含TF variable支持的字段）
- **编辑方式**：**支持表单编辑**
- **原因**：
  - TF variable只有有限的参数
  - 表单编辑不会破坏Schema结构
  - 用户可以微调解析结果

#### 3. AI生成的Schema - 表单编辑模式 🤖
- **来源**：用户通过"AI生成"按钮生成
- **特点**：AI解析生成的Schema
- **编辑方式**：**支持表单编辑**
- **原因**：
  - AI可能解析不准确，需要人工调整
  - 用户可以优化AI生成的结果

## 数据库Schema变更

### 添加source_type字段

```sql
-- 在schemas表中添加source_type字段
ALTER TABLE schemas ADD COLUMN source_type VARCHAR(20) DEFAULT 'json_import';

-- 可能的值：
-- 'json_import' - JSON导入
-- 'tf_parse' - TF文件解析
-- 'ai_generate' - AI生成
```

### 更新现有数据

```sql
-- 将现有的AI生成的Schema标记为ai_generate
UPDATE schemas SET source_type = 'ai_generate' WHERE ai_generated = true;

-- 其他的默认为json_import
UPDATE schemas SET source_type = 'json_import' WHERE source_type IS NULL;
```

## 后端实现

### 1. 更新Schema模型

**文件**: `backend/internal/models/schema.go`

```go
type Schema struct {
    ID          uint      `json:"id" gorm:"primaryKey"`
    ModuleID    uint      `json:"module_id"`
    Version     string    `json:"version"`
    Status      string    `json:"status"`
    SchemaData  string    `json:"schema_data" gorm:"type:jsonb"`
    AIGenerated bool      `json:"ai_generated"`
    SourceType  string    `json:"source_type"` // 新增字段
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

### 2. 更新Schema创建逻辑

**文件**: `backend/controllers/schema_controller.go`

```go
// CreateSchema - 创建Schema时设置source_type
func (sc *SchemaController) CreateSchema(c *gin.Context) {
    var req struct {
        SchemaData interface{} `json:"schema_data"`
        Version    string      `json:"version"`
        Status     string      `json:"status"`
        SourceType string      `json:"source_type"` // 接收source_type
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // 默认值
    if req.SourceType == "" {
        req.SourceType = "json_import"
    }
    
    // 创建Schema...
}
```

### 3. 更新Schema更新API

**文件**: `backend/controllers/schema_controller.go`

```go
// UpdateSchema - 更新Schema
func (sc *SchemaController) UpdateSchema(c *gin.Context) {
    schemaID := c.Param("id")
    
    var req struct {
        SchemaData interface{} `json:"schema_data"`
        Version    string      `json:"version"`
        Status     string      `json:"status"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // 更新Schema...
    // 注意：不允许修改source_type
}
```

## 前端实现

### 1. 更新Schema接口类型

**文件**: `frontend/src/pages/SchemaManagement.tsx`

```tsx
interface Schema {
  id: number;
  module_id: number;
  version: string;
  status: string;
  ai_generated: boolean;
  source_type: 'json_import' | 'tf_parse' | 'ai_generate'; // 新增
  schema_data: FormSchema;
  created_at: string;
  updated_at: string;
}
```

### 2. 判断Schema是否可编辑

```tsx
const isEditable = (schema: Schema): boolean => {
  return schema.source_type === 'tf_parse' || schema.source_type === 'ai_generate';
};
```

### 3. 更新按钮显示逻辑

```tsx
<div className={styles.headerActions}>
  {/* 导入JSON */}
  <button onClick={() => setShowImportDialog(true)}>
    📄 导入JSON
  </button>
  
  {/* AI生成 */}
  <button onClick={generateSchemaFromModule}>
    🤖 AI生成
  </button>
  
  {/* 编辑/查看Schema */}
  {activeSchema && (
    <>
      {isEditable(activeSchema) ? (
        <button onClick={() => setShowSchemaEditor(true)}>
          ✏️ 编辑Schema
        </button>
      ) : (
        <button onClick={() => setShowSchemaViewer(true)}>
          👁️ 查看Schema
        </button>
      )}
    </>
  )}
</div>
```

### 4. Schema列表显示来源标识

```tsx
<div className={styles.schemaItem}>
  <span className={styles.schemaVersion}>v{schema.version}</span>
  <span className={styles.sourceTag}>
    {schema.source_type === 'json_import' && '📄 JSON'}
    {schema.source_type === 'tf_parse' && '📝 TF'}
    {schema.source_type === 'ai_generate' && '🤖 AI'}
  </span>
  <span className={`${styles.schemaStatus} ${styles[schema.status]}`}>
    {schema.status}
  </span>
</div>
```

### 5. 更新导入逻辑

#### JSON导入

```tsx
const handleImportSchema = async (schemaData: any, version: string) => {
  try {
    const response = await api.post(`/modules/${moduleId}/schemas`, {
      schema_data: schemaData,
      version: version,
      status: 'active',
      source_type: 'json_import'  // 标记为JSON导入
    });
    // ...
  } catch (error) {
    // ...
  }
};
```

#### TF文件解析（ImportModule页面）

```tsx
const handleSchemaSave = async (editedSchema: any) => {
  try {
    // 创建Module...
    
    // 创建Schema
    await fetch(`http://localhost:8080/api/v1/modules/${moduleId}/schemas`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${localStorage.getItem('token')}`
      },
      body: JSON.stringify({
        schema_data: editedSchema,
        version: '1.0.0',
        status: 'active',
        source_type: 'tf_parse'  // 标记为TF解析
      })
    });
    // ...
  } catch (error) {
    // ...
  }
};
```

### 6. Schema编辑流程

```tsx
const [showSchemaEditor, setShowSchemaEditor] = useState(false);

// 编辑Schema
const handleEditSchema = () => {
  if (!isEditable(activeSchema)) {
    showToast('该Schema不支持编辑，请重新导入', 'warning');
    return;
  }
  setShowSchemaEditor(true);
};

// 保存编辑后的Schema
const handleSaveEditedSchema = async (editedSchema: any) => {
  try {
    const response = await api.put(`/modules/${moduleId}/schemas/${activeSchema.id}`, {
      schema_data: editedSchema,
      version: activeSchema.version,
      status: activeSchema.status
    });
    
    // 刷新Schema列表
    await fetchSchemas();
    setShowSchemaEditor(false);
    showToast('Schema更新成功！', 'success');
  } catch (error) {
    const message = extractErrorMessage(error);
    showToast(message, 'error');
  }
};
```

## UI设计

### SchemaManagement页面布局

```
┌─────────────────────────────────────────────────────────┐
│ Schema管理                                               │
│ 版本: 1.0.0  [active]  [📝 TF]                          │
│                                                          │
│ [📄 导入JSON]  [🤖 AI生成]  [✏️ 编辑Schema]            │
└─────────────────────────────────────────────────────────┘
│                                                          │
│ ┌──────────┐  ┌────────────────────────────────────┐   │
│ │Schema列表│  │ 配置表单                            │   │
│ │          │  │                                     │   │
│ │ v1.0.0   │  │ [动态表单渲染]                      │   │
│ │ 📝 TF    │  │                                     │   │
│ │ active   │  │                                     │   │
│ │          │  │                                     │   │
│ │ v1.1.0   │  │                                     │   │
│ │ 📄 JSON  │  │                                     │   │
│ │ draft    │  │                                     │   │
│ └──────────┘  └────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

### Schema编辑器（SchemaEditor）

```
┌─────────────────────────────────────────────────────────┐
│ Schema 编辑器                    共找到 15 个变量        │
├─────────────────────────────────────────────────────────┤
│ [搜索字段...]                                            │
├─────────────────────────────────────────────────────────┤
│ 变量名      │ 类型   │ 必填 │ 默认值 │ 描述   │ 操作    │
├─────────────────────────────────────────────────────────┤
│ bucket_name │ String │ ✓   │ -      │ S3名称 │ [编辑]  │
│ region      │ String │ ✗   │ us-e.. │ 区域   │ [编辑]  │
│ ...         │        │     │        │        │         │
├─────────────────────────────────────────────────────────┤
│                              [取消]  [保存Schema]        │
└─────────────────────────────────────────────────────────┘
```

## 测试用例

### 1. JSON导入Schema测试
```
1. 点击"导入JSON"按钮
2. 粘贴完整的JSON Schema
3. 保存后，Schema列表显示 "📄 JSON" 标识
4. 点击该Schema，按钮显示为"👁️ 查看Schema"
5. 点击"查看Schema"，显示只读视图
```

### 2. TF文件解析Schema测试
```
1. 在ImportModule页面选择"TF文件"
2. 上传variables.tf文件
3. 系统解析并显示SchemaEditor
4. 编辑字段后保存
5. 在SchemaManagement页面，Schema列表显示 "📝 TF" 标识
6. 点击该Schema，按钮显示为"✏️ 编辑Schema"
7. 点击"编辑Schema"，可以修改字段
```

### 3. AI生成Schema测试
```
1. 点击"AI生成"按钮
2. AI自动生成Schema
3. Schema列表显示 "🤖 AI" 标识
4. 点击该Schema，按钮显示为"✏️ 编辑Schema"
5. 可以编辑AI生成的Schema
```

## 开发检查清单

### 后端
- [ ] 添加source_type字段到schemas表
- [ ] 更新Schema模型
- [ ] 更新CreateSchema API支持source_type
- [ ] 更新UpdateSchema API
- [ ] 更新AI生成逻辑设置source_type为'ai_generate'

### 前端
- [ ] 更新Schema接口类型定义
- [ ] 实现isEditable判断函数
- [ ] 更新SchemaManagement页面按钮逻辑
- [ ] 添加Schema来源标识显示
- [ ] 更新JSON导入逻辑设置source_type
- [ ] 更新TF解析导入逻辑设置source_type
- [ ] 实现Schema编辑功能
- [ ] 实现Schema查看功能（只读）
- [ ] 添加Schema更新API调用

### 测试
- [ ] 测试JSON导入Schema（只读）
- [ ] 测试TF文件解析Schema（可编辑）
- [ ] 测试AI生成Schema（可编辑）
- [ ] 测试Schema编辑保存
- [ ] 测试Schema列表显示

## 注意事项

1. **source_type不可修改**：一旦Schema创建，source_type不应该被修改
2. **向后兼容**：现有的Schema默认为'json_import'
3. **权限控制**：只有可编辑的Schema才显示"编辑"按钮
4. **用户提示**：当用户尝试编辑不可编辑的Schema时，给出友好提示

## 版本历史

### v1.0.0 (2025-09-30)
- 初始版本
- 实现Schema来源分类
- 实现基于来源的编辑权限控制
