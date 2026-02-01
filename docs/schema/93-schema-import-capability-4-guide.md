# Schema导入能力4：JSON Schema直接导入功能

## 📋 功能概述

**能力4**允许用户直接上传或粘贴JSON格式的Schema配置，无需任何解析或转换，直接保存到数据库即可使用。

## 🎯 实现的功能

### 1. 前端组件
-  **SchemaImportDialog** - JSON Schema导入弹窗组件
-  **JsonEditor** - 带语法高亮和验证的JSON编辑器
-  集成到SchemaManagement页面
-  支持加载示例Schema
-  实时JSON格式验证
-  版本号输入

### 2. 后端API
-  `POST /modules/{module_id}/schemas` - 创建Schema接口（已存在）
-  Schema数据验证和存储
-  自动设置为active状态

### 3. 用户体验
-  现代化弹窗设计
-  友好的错误提示
-  加载示例功能
-  格式化JSON按钮
-  行号显示
-  语法高亮

## 🧪 测试步骤

### 前置条件
1. 前端服务运行在 http://localhost:5173
2. 后端服务运行在 http://localhost:8080
3. 数据库中已有Module记录（如S3模块，ID=1）

### 测试流程

#### 步骤1：访问Schema管理页面
```
访问: http://localhost:5173/modules/1/schemas
```

#### 步骤2：点击"导入JSON Schema"按钮
- 在页面顶部右侧找到 "📄 导入JSON" 按钮
- 或者在空状态页面点击 "📄 导入JSON Schema" 按钮

#### 步骤3：使用示例Schema
点击弹窗中的"加载示例"按钮，会自动填充以下示例：

```json
{
  "bucket_name": {
    "type": "string",
    "required": true,
    "description": "S3存储桶名称",
    "placeholder": "my-bucket-name"
  },
  "region": {
    "type": "string",
    "required": true,
    "description": "AWS区域",
    "default": "us-west-2",
    "options": ["us-east-1", "us-west-2", "eu-west-1", "ap-southeast-1"]
  },
  "versioning_enabled": {
    "type": "boolean",
    "required": false,
    "description": "是否启用版本控制",
    "default": false
  },
  "tags": {
    "type": "map",
    "required": false,
    "description": "资源标签",
    "default": {}
  }
}
```

#### 步骤4：设置版本号
- 默认版本号为 "1.0.0"
- 可以修改为其他版本号，如 "1.0.1", "2.0.0" 等

#### 步骤5：导入Schema
- 点击"导入Schema"按钮
- 等待导入完成
- 查看成功提示通知

#### 步骤6：验证导入结果
- Schema应该出现在左侧Schema列表中
- 右侧应该显示基于Schema生成的动态表单
- 表单应该包含所有定义的字段

### 测试用例

#### 测试用例1：基本导入
```json
{
  "name": {
    "type": "string",
    "required": true,
    "description": "资源名称"
  }
}
```
**预期结果**：成功导入，表单显示一个必填的文本输入框

#### 测试用例2：复杂Schema
```json
{
  "bucket_name": {
    "type": "string",
    "required": true,
    "description": "S3存储桶名称"
  },
  "versioning": {
    "type": "object",
    "required": false,
    "description": "版本控制配置",
    "properties": {
      "enabled": {
        "type": "boolean",
        "default": false
      },
      "mfa_delete": {
        "type": "boolean",
        "default": false
      }
    }
  },
  "tags": {
    "type": "map",
    "required": false,
    "description": "资源标签"
  }
}
```
**预期结果**：成功导入，表单显示嵌套对象和Map类型字段

#### 测试用例3：JSON格式错误
```json
{
  "name": {
    "type": "string"
    "required": true  // 缺少逗号
  }
}
```
**预期结果**：显示JSON格式错误提示，无法导入

#### 测试用例4：空版本号
- 清空版本号输入框
- 尝试导入

**预期结果**：显示"请输入版本号"错误提示

## 📊 Schema字段类型支持

导入的JSON Schema支持以下字段类型：

### 基础类型
- `string` - 字符串
- `number` - 数字
- `boolean` - 布尔值

### 复杂类型
- `object` - 对象（固定结构）
- `map` - 映射（用户可自由添加key-value）
- `array` - 数组

### 字段属性
- `type` - 字段类型（必需）
- `required` - 是否必填
- `description` - 字段描述
- `default` - 默认值
- `placeholder` - 占位符文本
- `options` - 选项列表（用于下拉选择）
- `hiddenDefault` - 是否默认隐藏（高级选项）
- `properties` - 对象的子属性
- `items` - 数组元素的Schema

## 🎨 UI特性

### JsonEditor组件特性
1. **语法高亮**
   - 键名：蓝色
   - 字符串值：绿色
   - 数字：橙色
   - 布尔值：紫色
   - null：灰色

2. **实时验证**
   - 输入时自动验证JSON格式
   - 显示错误行号和列号
   - 错误行高亮显示

3. **格式化功能**
   - 一键格式化JSON
   - 自动缩进和换行
   - 美化显示

4. **行号显示**
   - 左侧显示行号
   - 错误行红色标记
   - 便于定位问题

### 弹窗特性
1. **响应式设计**
   - 桌面端：800px宽度
   - 移动端：全屏显示
   - 自适应布局

2. **交互优化**
   - 点击遮罩层关闭
   - ESC键关闭（待实现）
   - 平滑动画效果

3. **错误处理**
   - 友好的错误提示
   - 保留用户输入
   - 不清空表单数据

## 🔧 技术实现

### 前端实现
```typescript
// 组件位置
frontend/src/components/DynamicForm/SchemaImportDialog.tsx
frontend/src/components/DynamicForm/SchemaImportDialog.module.css

// 集成位置
frontend/src/pages/SchemaManagement.tsx

// API调用
POST /modules/${moduleId}/schemas
Body: {
  schema_data: {...},
  version: "1.0.0",
  status: "active"
}
```

### 后端实现
```go
// 控制器
backend/controllers/schema_controller.go
func (c *SchemaController) CreateSchema(ctx *gin.Context)

// 服务层
backend/services/schema_service.go
func (s *SchemaService) CreateSchema(moduleID uint, req *models.CreateSchemaRequest)

// 数据模型
backend/internal/models/schema.go
type CreateSchemaRequest struct {
    SchemaData json.RawMessage
    Version    string
    Status     string
}
```

##  验收标准

### 功能验收
- [x] 用户可以打开JSON导入弹窗
- [x] 用户可以粘贴JSON Schema
- [x] 用户可以加载示例Schema
- [x] 用户可以格式化JSON
- [x] 系统可以验证JSON格式
- [x] 系统可以保存Schema到数据库
- [x] 导入后自动刷新Schema列表
- [x] 导入后自动切换到新Schema
- [x] 显示成功/失败通知

### UI验收
- [x] 弹窗设计现代化
- [x] JSON编辑器有语法高亮
- [x] 错误提示清晰友好
- [x] 响应式布局适配
- [x] 动画效果流畅

### 代码质量
- [x] TypeScript类型完整
- [x] CSS模块化隔离
- [x] 错误处理完善
- [x] 代码注释清晰

## 🚀 后续优化

### 短期优化
1. 添加ESC键关闭弹窗
2. 添加文件上传功能（.json文件）
3. 添加Schema模板库
4. 支持从URL导入Schema

### 长期优化
1. Schema可视化编辑器
2. Schema版本对比功能
3. Schema导出功能
4. Schema验证规则增强

## 📝 使用示例

### 示例1：简单的EC2实例Schema
```json
{
  "instance_type": {
    "type": "string",
    "required": true,
    "description": "EC2实例类型",
    "default": "t2.micro",
    "options": ["t2.micro", "t2.small", "t2.medium", "t2.large"]
  },
  "ami_id": {
    "type": "string",
    "required": true,
    "description": "AMI镜像ID",
    "placeholder": "ami-xxxxxxxxx"
  },
  "key_name": {
    "type": "string",
    "required": false,
    "description": "SSH密钥对名称"
  },
  "monitoring": {
    "type": "boolean",
    "required": false,
    "description": "启用详细监控",
    "default": false
  }
}
```

### 示例2：RDS数据库Schema
```json
{
  "db_name": {
    "type": "string",
    "required": true,
    "description": "数据库名称"
  },
  "engine": {
    "type": "string",
    "required": true,
    "description": "数据库引擎",
    "options": ["mysql", "postgres", "mariadb", "oracle", "sqlserver"]
  },
  "engine_version": {
    "type": "string",
    "required": true,
    "description": "引擎版本"
  },
  "instance_class": {
    "type": "string",
    "required": true,
    "description": "实例类型",
    "default": "db.t3.micro"
  },
  "allocated_storage": {
    "type": "number",
    "required": true,
    "description": "存储空间(GB)",
    "default": 20
  },
  "backup_retention_period": {
    "type": "number",
    "required": false,
    "description": "备份保留天数",
    "default": 7
  },
  "multi_az": {
    "type": "boolean",
    "required": false,
    "description": "启用多可用区",
    "default": false
  }
}
```

## 🎉 总结

**能力4：JSON Schema直接导入**功能已完整实现，包括：

1.  完整的前端UI组件
2.  JSON编辑器和验证
3.  后端API集成
4.  数据库存储
5.  用户体验优化

用户现在可以通过简单的复制粘贴操作，快速导入自定义的Schema配置，无需任何编程或配置文件知识。

---

**下一步**：实现能力1 - 解析.tf文件生成Schema
