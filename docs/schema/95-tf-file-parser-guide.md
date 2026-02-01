# TF文件解析和Schema编辑功能指南

## 📋 功能概述

**实现日期**: 2025-09-30  
**功能**: 用户上传.tf文件 → 后端解析variables → 生成初始Schema → 前端展示Schema编辑器 → 用户微调 → 保存Schema

## 🎯 需求说明

### 1. 支持的导入方式（4种）

1. **JSON Schema直接导入**  已完成
2. **Variable文件解析** 🚧 本次实现
3. **TAR.GZ包导入** 📋 待实现
4. **Git仓库导入** 📋 待实现

### 2. Terraform Variable参数支持

需要支持Terraform的所有variable参数：

```hcl
variable "example" {
  type        = string              # 类型约束
  default     = "default_value"     # 默认值
  description = "描述信息"           # 描述
  validation {                      # 验证规则
    condition     = length(var.example) > 4
    error_message = "错误信息"
  }
  sensitive   = false               # 是否敏感
  nullable    = true                # 是否可为null
}
```

### 3. Schema字段映射

**Terraform Variable → Schema字段映射：**

| Terraform参数 | Schema字段 | 说明 |
|--------------|-----------|------|
| type | Type | 类型（string/number/bool/object/list等） |
| default | Default | 默认值 |
| description | Description | 描述信息 |
| validation.condition | - | 暂不支持（复杂表达式） |
| validation.error_message | - | 暂不支持 |
| sensitive | Sensitive | 是否敏感字段 |
| nullable | - | 暂不支持（Go Schema无此字段） |

**额外的Schema字段（使用defaultSchema默认值）：**
- Required: false（默认）
- ForceNew: false（默认）
- HiddenDefault: true（默认，高级选项）
- Color: InfoColor（默认）
- 其他字段使用defaultSchema()的默认值

### 4. UI交互设计

#### 4.1 表格视图（主界面）
```
┌─────────────────────────────────────────────────────────────┐
│ 解析结果：共找到 5 个变量                                      │
├──────────┬──────────┬──────────┬──────────────────────────────┤
│ 变量名    │ 类型     │ 必填     │ 操作                          │
├──────────┼──────────┼──────────┼──────────────────────────────┤
│ bucket   │ string   │ ✓       │ [编辑] [删除]                 │
│ region   │ string   │ ✗       │ [编辑] [删除]                 │
│ tags     │ map      │ ✗       │ [编辑] [删除]                 │
│ enabled  │ boolean  │ ✗       │ [编辑] [删除]                 │
│ count    │ number   │ ✗       │ [编辑] [删除]                 │
└──────────┴──────────┴──────────┴──────────────────────────────┘
```

#### 4.2 编辑表单（点击编辑后展开）
```
┌─────────────────────────────────────────────────────────────┐
│ 编辑变量: bucket                                              │
├─────────────────────────────────────────────────────────────┤
│ 基础信息                                                      │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ 变量名: bucket                                           │ │
│ │ 类型: [string ▼]                                         │ │
│ │ 必填: [✓]                                                │ │
│ │ 描述: S3存储桶名称                                        │ │
│ │ 默认值: my-bucket                                        │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                              │
│ 高级选项                                                      │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ 敏感字段: [✗]                                            │ │
│ │ 强制重建: [✗]                                            │ │
│ │ 默认隐藏: [✓]                                            │ │
│ │ 颜色标记: [Info ▼]                                       │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                              │
│ [保存] [取消]                                                │
└─────────────────────────────────────────────────────────────┘
```

## 🔧 技术实现

### 1. 后端实现

#### 1.1 TF文件解析器

**文件位置**: `backend/internal/parsers/tf_parser.go`

```go
package parsers

import (
    "regexp"
    "strings"
)

type TFVariable struct {
    Name        string
    Type        string
    Default     interface{}
    Description string
    Sensitive   bool
    Nullable    bool
    Validation  *TFValidation
}

type TFValidation struct {
    Condition    string
    ErrorMessage string
}

// ParseVariablesFile 解析variables.tf文件
func ParseVariablesFile(content string) ([]TFVariable, error) {
    // 使用正则表达式提取variable块
    // 解析每个variable的属性
    // 返回TFVariable列表
}

// ConvertToSchema 将TFVariable转换为Schema
func ConvertToSchema(tfVar TFVariable) map[string]interface{} {
    schema := defaultSchema()
    
    // 映射类型
    schema["type"] = mapTerraformType(tfVar.Type)
    
    // 映射其他字段
    schema["default"] = tfVar.Default
    schema["description"] = tfVar.Description
    schema["sensitive"] = tfVar.Sensitive
    
    return schema
}
```

#### 1.2 API端点

**路由**: `POST /api/v1/modules/parse-tf`

**请求体**:
```json
{
  "tf_content": "variable \"bucket\" {\n  type = string\n  ...\n}"
}
```

**响应**:
```json
{
  "code": 200,
  "data": {
    "variables": [
      {
        "name": "bucket",
        "type": "string",
        "default": null,
        "description": "S3 bucket name",
        "sensitive": false
      }
    ],
    "schema": {
      "bucket": {
        "type": "string",
        "required": false,
        "description": "S3 bucket name",
        "default": null,
        "sensitive": false,
        "hidden_default": true
      }
    }
  }
}
```

### 2. 前端实现

#### 2.1 Schema编辑器组件

**文件位置**: `frontend/src/components/DynamicForm/SchemaEditor.tsx`

```typescript
interface SchemaEditorProps {
  initialSchema: Record<string, any>;
  onSave: (schema: Record<string, any>) => void;
  onCancel: () => void;
}

const SchemaEditor: React.FC<SchemaEditorProps> = ({
  initialSchema,
  onSave,
  onCancel
}) => {
  const [schema, setSchema] = useState(initialSchema);
  const [editingField, setEditingField] = useState<string | null>(null);
  
  return (
    <div className={styles.schemaEditor}>
      {/* 表格视图 */}
      <SchemaTable 
        schema={schema}
        onEdit={(fieldName) => setEditingField(fieldName)}
        onDelete={(fieldName) => handleDelete(fieldName)}
      />
      
      {/* 编辑表单（模态框） */}
      {editingField && (
        <SchemaFieldEditor
          fieldName={editingField}
          fieldSchema={schema[editingField]}
          onSave={(updatedField) => handleSaveField(updatedField)}
          onCancel={() => setEditingField(null)}
        />
      )}
      
      {/* 操作按钮 */}
      <div className={styles.actions}>
        <button onClick={() => onSave(schema)}>保存Schema</button>
        <button onClick={onCancel}>取消</button>
      </div>
    </div>
  );
};
```

#### 2.2 字段编辑器组件

**文件位置**: `frontend/src/components/DynamicForm/SchemaFieldEditor.tsx`

```typescript
interface SchemaFieldEditorProps {
  fieldName: string;
  fieldSchema: any;
  onSave: (field: any) => void;
  onCancel: () => void;
}

const SchemaFieldEditor: React.FC<SchemaFieldEditorProps> = ({
  fieldName,
  fieldSchema,
  onSave,
  onCancel
}) => {
  const [field, setField] = useState(fieldSchema);
  
  return (
    <div className={styles.modal}>
      <div className={styles.modalContent}>
        <h3>编辑变量: {fieldName}</h3>
        
        {/* 基础信息 */}
        <section>
          <h4>基础信息</h4>
          <FormField label="类型">
            <select 
              value={field.type} 
              onChange={(e) => setField({...field, type: e.target.value})}
            >
              <option value="string">String</option>
              <option value="number">Number</option>
              <option value="boolean">Boolean</option>
              <option value="object">Object</option>
              <option value="list">List</option>
              <option value="map">Map</option>
            </select>
          </FormField>
          
          <FormField label="必填">
            <input 
              type="checkbox" 
              checked={field.required}
              onChange={(e) => setField({...field, required: e.target.checked})}
            />
          </FormField>
          
          <FormField label="描述">
            <textarea 
              value={field.description}
              onChange={(e) => setField({...field, description: e.target.value})}
            />
          </FormField>
          
          <FormField label="默认值">
            <input 
              value={field.default || ''}
              onChange={(e) => setField({...field, default: e.target.value})}
            />
          </FormField>
        </section>
        
        {/* 高级选项 */}
        <section>
          <h4>高级选项</h4>
          <FormField label="敏感字段">
            <input 
              type="checkbox" 
              checked={field.sensitive}
              onChange={(e) => setField({...field, sensitive: e.target.checked})}
            />
          </FormField>
          
          <FormField label="强制重建">
            <input 
              type="checkbox" 
              checked={field.force_new}
              onChange={(e) => setField({...field, force_new: e.target.checked})}
            />
          </FormField>
          
          <FormField label="默认隐藏">
            <input 
              type="checkbox" 
              checked={field.hidden_default}
              onChange={(e) => setField({...field, hidden_default: e.target.checked})}
            />
          </FormField>
        </section>
        
        {/* 操作按钮 */}
        <div className={styles.actions}>
          <button onClick={() => onSave(field)}>保存</button>
          <button onClick={onCancel}>取消</button>
        </div>
      </div>
    </div>
  );
};
```

#### 2.3 集成到ImportModule页面

**修改**: `frontend/src/pages/ImportModule.tsx`

```typescript
// TF文件导入处理
const handleTfFileImport = async () => {
  if (!tfFile && !tfContent.trim()) {
    error('请上传.tf文件或粘贴内容');
    return;
  }

  try {
    setLoading(true);

    // 读取文件内容
    let content = tfContent;
    if (tfFile) {
      content = await tfFile.text();
    }

    // 调用解析API
    const parseResponse = await fetch('http://localhost:8080/api/v1/modules/parse-tf', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${localStorage.getItem('token')}`
      },
      body: JSON.stringify({ tf_content: content })
    });

    if (!parseResponse.ok) {
      throw new Error('TF文件解析失败');
    }

    const parseResult = await parseResponse.json();
    
    // 显示Schema编辑器
    setShowSchemaEditor(true);
    setParsedSchema(parseResult.data.schema);
    
  } catch (err: any) {
    error('解析失败: ' + (err.message || '未知错误'));
  } finally {
    setLoading(false);
  }
};

// Schema编辑完成后保存
const handleSchemaSave = async (editedSchema: any) => {
  try {
    // 创建Module
    const moduleData = {
      name: moduleName,
      provider: provider,
      description: description,
      repository_url: 'tf-file-import',
      branch: '1.0.0'
    };

    const moduleResponse = await moduleService.createModule(moduleData);
    const moduleId = moduleResponse.data.id;

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
        status: 'active'
      })
    });

    success('模块和Schema创建成功！');
    navigate(`/modules/${moduleId}/schemas`);
    
  } catch (err: any) {
    error('保存失败: ' + (err.message || '未知错误'));
  }
};
```

## 📊 完整流程

### 用户操作流程

```
1. 访问 /modules/import
2. 选择"TF文件"导入方式
3. 上传variables.tf文件或粘贴内容
4. 点击"解析"按钮
5. 系统解析TF文件，生成初始Schema
6. 显示Schema编辑器（表格视图）
7. 用户点击"编辑"按钮编辑某个字段
8. 弹出编辑表单，用户修改字段属性
9. 保存字段修改
10. 重复7-9直到所有字段都满意
11. 点击"保存Schema"
12. 系统创建Module和Schema
13. 跳转到Schema管理页面
```

### 系统处理流程

```
1. 接收TF文件内容
2. 使用正则表达式解析variable块
3. 提取每个variable的属性
4. 转换为Schema格式
5. 应用defaultSchema()的默认值
6. 返回解析结果给前端
7. 前端展示Schema编辑器
8. 用户编辑Schema
9. 前端发送最终Schema到后端
10. 后端创建Module记录
11. 后端创建Schema记录
12. 返回成功响应
```

## 🎨 UI设计规范

### 颜色方案
- 主色调：蓝色（#3B82F6）
- 成功：绿色（#10B981）
- 警告：黄色（#F59E0B）
- 错误：红色（#EF4444）
- 中性：灰色（#6B7280）

### 组件样式
- 表格：卡片式设计，带阴影
- 按钮：圆角，悬停效果
- 表单：清晰的标签和输入框
- 模态框：居中显示，半透明背景

## 📝 注意事项

### 1. TF文件解析限制
- 只解析variable块
- 不支持复杂的validation表达式
- 不支持动态类型推断

### 2. Schema字段限制
- nullable参数暂不支持（Go Schema无此字段）
- validation规则暂不支持（需要复杂的表达式解析）

### 3. 用户体验
- 提供清晰的错误提示
- 支持预览功能
- 允许用户取消操作
- 保存前确认

## 🔄 未来优化

### 可能的改进
1. **AI辅助** - 使用AI自动优化Schema配置
2. **模板系统** - 提供常用的Schema模板
3. **批量编辑** - 支持批量修改多个字段
4. **导入导出** - 支持Schema的导入导出
5. **版本对比** - 显示Schema的变更历史

### 扩展功能
1. **validation支持** - 解析和支持validation规则
2. **类型推断** - 智能推断复杂类型
3. **依赖分析** - 分析字段间的依赖关系
4. **文档生成** - 自动生成Schema文档

## 📚 相关文档

- [Module导入功能指南](./schema-import-capability-4-guide.md)
- [实时名称检查功能](./module-import-realtime-check-guide.md)
- [开发指南](./development-guide.md)
- [Demo Module开发规范](./demo-module-development-guide.md)

---

**最后更新**: 2025-09-30  
**功能状态**: 🚧 开发中
