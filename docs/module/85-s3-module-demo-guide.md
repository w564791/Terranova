# S3 Module Demo 开发指南

## 📋 概述

** 文档状态**: 本文档在新的动态Schema生成架构下**部分过时**，但仍作为**S3标准示例参考**保留。

**🎯 当前价值**:
- S3模块作为项目标准示例的规范说明
- 复杂Schema结构的参考模板
- 前端表单渲染的测试基准
- 开发规范和质量标准

**🔄 架构变更**: 项目已从硬编码Schema改为完全动态生成，详见 `dynamic-schema-testing-guide.md`。

##  重要说明

**S3 Module作为项目标准Demo示例**：
- 🎯 **后续所有功能开发都以S3模块为标准示例**
- 🎨 **前端表单渲染测试统一使用S3 Schema**
- 🤖 **后端Schema自动生成以S3模块为模板**
- 📝 **所有文档和演示都基于S3模块结构**
- 🧪 **功能测试和验证都使用S3配置参数**

这确保了项目的一致性和可维护性，所有开发者都使用相同的示例进行开发和测试。

## 🗂️ Demo文件说明

### files/s3_module
包含完整的AWS S3模块Schema定义，包括：
- **基础配置**: 存储桶名称、前缀、ACL、策略等
- **安全配置**: 加密、访问控制、公共访问阻止等  
- **生命周期管理**: 对象过期、存储类转换等
- **高级功能**: 版本控制、复制、通知等
- **标签管理**: 必需标签和默认标签

### files/types
定义了AWS资源的所有属性常量，为Schema生成提供标准化的字段名称。

## 🏗️ 实现的功能

### 1. Schema管理后端API

#### 控制器 (`controllers/schema_controller.go`)
- `GET /modules/{module_id}/schemas` - 获取模块的Schema列表
- `POST /modules/{module_id}/schemas` - 创建新Schema
- `GET /schemas/{id}` - 获取Schema详情
- `PUT /schemas/{id}` - 更新Schema

#### 服务层 (`services/schema_service.go`)
- Schema CRUD操作
- `CreateS3DemoSchema()` - 基于demo文件创建S3 Schema
- JSON Schema数据的序列化/反序列化

### 2. 动态表单系统增强

#### 嵌套对象支持
```typescript
// 支持复杂的嵌套结构
{
  "tags": {
    "type": "object",
    "required": true,
    "properties": {
      "business-line": {
        "type": "string",
        "required": true
      },
      "managed-by": {
        "type": "string", 
        "required": true
      }
    }
  }
}
```

#### 表单字段类型
- `string` - 文本输入
- `number` - 数字输入  
- `boolean` - 复选框
- `object` - 嵌套对象（递归渲染）
- `select` - 下拉选择（通过options配置）

### 3. Schema管理前端页面

#### 功能特性
- **Schema列表**: 显示模块的所有Schema版本
- **状态管理**: active/draft状态切换
- **AI标识**: 标记AI生成的Schema
- **实时预览**: 基于Schema生成动态表单
- **配置生成**: 将表单数据转换为Terraform配置

#### 页面结构
```
SchemaManagement/
├── 侧边栏 - Schema版本列表
└── 主区域 - 动态表单渲染
```

## 🎯 S3 Demo Schema结构

基于完整的S3模块定义，创建了简化版演示Schema：

```json
{
  "name": {
    "type": "string",
    "required": false,
    "description": "S3存储桶名称"
  },
  "bucket_prefix": {
    "type": "string",
    "required": false, 
    "description": "存储桶名称前缀"
  },
  "acl": {
    "type": "string",
    "required": false,
    "options": ["private", "public-read", "public-read-write"],
    "description": "访问控制列表"
  },
  "force_destroy": {
    "type": "boolean",
    "required": false,
    "default": false,
    "description": "是否强制删除存储桶"
  },
  "tags": {
    "type": "object",
    "required": true,
    "description": "资源标签",
    "properties": {
      "business-line": {
        "type": "string",
        "required": true,
        "description": "业务线"
      },
      "managed-by": {
        "type": "string",
        "required": true, 
        "description": "管理者"
      }
    }
  },
  "versioning": {
    "type": "object",
    "required": false,
    "description": "版本控制配置",
    "properties": {
      "enabled": {
        "type": "boolean",
        "required": false,
        "default": false,
        "description": "是否启用版本控制"
      }
    }
  }
}
```

## 🚀 使用流程

### 1. 访问Schema管理
1. 进入模块详情页面
2. 点击"管理Schema"按钮
3. 进入Schema管理页面

### 2. 创建演示Schema
1. 如果模块没有Schema，系统会提示创建
2. 点击"创建演示Schema"自动生成S3 Demo Schema
3. Schema基于 `files/s3_module` 的简化版本

### 3. 使用动态表单
1. 选择活跃的Schema版本
2. 在右侧表单中填写配置参数
3. 支持嵌套对象（如tags、versioning）
4. 点击"生成配置"查看结果

### 4. 表单验证
- 必填字段验证
- 类型验证（字符串、数字、布尔值）
- 嵌套对象的递归验证

## 🔧 技术实现要点

### 1. Schema数据存储
```sql
-- schemas表结构
CREATE TABLE schemas (
    id SERIAL PRIMARY KEY,
    module_id INTEGER REFERENCES modules(id),
    version VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'draft',
    ai_generated BOOLEAN DEFAULT false,
    schema_data JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

### 2. 动态表单递归渲染
```typescript
// FormField组件支持递归渲染嵌套对象
case 'object':
  if (schema.properties) {
    return (
      <div className={styles.objectField}>
        {Object.entries(schema.properties).map(([propName, propSchema]) => (
          <FormField
            key={propName}
            name={propName}
            schema={propSchema}
            value={value?.[propName]}
            onChange={(propValue) => {
              const newValue = { ...value };
              newValue[propName] = propValue;
              onChange(newValue);
            }}
          />
        ))}
      </div>
    );
  }
```

### 3. 类型安全的Schema定义
```typescript
export interface FormSchema {
  [key: string]: {
    type: 'string' | 'number' | 'boolean' | 'object' | 'array';
    required?: boolean;
    description?: string;
    default?: any;
    options?: string[];
    properties?: FormSchema; // 支持嵌套
  };
}
```

## 📈 扩展计划

### 1. 完整S3 Schema支持
- 实现 `files/s3_module` 中的所有字段
- 支持更复杂的嵌套结构（lifecycle_rule等）
- 添加字段间的依赖关系

### 2. AI Schema生成
- 集成OpenAI API
- 自动解析Terraform Module文件
- 生成完整的Schema定义

### 3. 配置验证和生成
- Schema验证引擎
- Terraform配置文件生成
- 配置预览和diff功能

### 4. 更多字段类型
- `array` - 数组字段
- `select` - 枚举选择
- `date` - 日期选择
- `file` - 文件上传

## 🎯 新架构下的测试步骤

** 重要**: 请使用 `dynamic-schema-testing-guide.md` 进行完整的功能测试。

**本文档的S3示例仍可用于**:
1. **理解S3模块结构** - 作为复杂Schema的参考
2. **前端组件测试** - 验证表单渲染能力
3. **开发规范参考** - 了解S3标准示例要求
4. **质量基准** - 确保新功能支持S3级别的复杂度

**快速验证S3支持**:
1. 创建名为"s3"的AWS模块
2. 同步Module文件 (POST /modules/:id/sync)
3. 生成Schema (POST /module-schemas/:id/generate)
4. 验证生成的Schema包含本文档描述的字段结构

## 📝 开发总结

通过S3 Module Demo的开发，成功实现了：

1. **完整的Schema管理流程** - 从定义到使用
2. **动态表单系统** - 支持复杂嵌套结构
3. **类型安全的实现** - TypeScript + Go的类型系统
4. **可扩展的架构** - 支持更多模块和字段类型

这为后续的AI解析、Terraform执行等功能奠定了坚实基础。

## 🎯 后续开发规范

### 统一使用S3示例
所有后续功能开发必须以S3模块为标准示例：

#### 1. 前端表单渲染测试
```typescript
// 统一使用S3 Schema进行测试
const testSchema = {
  name: { type: 'string', description: 'S3存储桶名称' },
  tags: { 
    type: 'object',
    properties: {
      'business-line': { type: 'string', required: true },
      'managed-by': { type: 'string', required: true }
    }
  }
};
```

#### 2. 后端Schema自动生成
```go
// 基于files/s3_module结构生成Schema
func GenerateS3Schema() Schema {
    // 使用S3模块的完整定义
    return s3ModuleSchema
}
```

#### 3. AI解析功能开发
- 输入：S3 Terraform Module文件
- 输出：基于S3结构的Schema JSON
- 测试：使用S3模块的各种配置场景

#### 4. Terraform执行测试
- 配置生成：基于S3 Schema的表单数据
- 执行验证：创建真实的S3资源（测试环境）
- 状态管理：S3资源的生命周期管理

#### 5. 文档和演示
- 所有截图和演示都使用S3模块界面
- 配置示例都基于S3参数
- 用户手册以S3创建流程为主线

### 开发检查清单
开发新功能时必须确认：
- [ ] 是否使用S3模块作为测试示例
- [ ] 是否兼容S3 Schema的复杂结构
- [ ] 是否支持S3模块的所有字段类型
- [ ] 文档是否以S3为示例进行说明
- [ ] 测试用例是否覆盖S3的典型场景

---

## 📚 相关文件和规范

### 核心文件
- `files/s3_module` - **标准S3模块定义** (所有开发的基准)
- `files/types` - AWS属性常量定义
- `backend/controllers/schema_controller.go` - Schema API
- `backend/services/schema_service.go` - Schema服务
- `frontend/src/pages/SchemaManagement.tsx` - Schema管理页面
- `frontend/src/components/DynamicForm/` - 动态表单组件

### 开发规范文档
- `docs/s3-module-demo-guide.md` - **本文档** (S3标准示例指南)
- `docs/api-specification.md` - API接口规范
- `docs/project-status.md` - 项目状态跟踪
- `docs/development-guide.md` - 完整开发指南

### 后续开发必读
1. **新功能开发前** - 必须阅读本文档的"后续开发规范"部分
2. **测试用例编写** - 必须基于S3模块的真实场景
3. **文档编写** - 必须以S3为示例进行说明
4. **演示准备** - 必须使用S3模块进行功能展示

**重要提醒**: 
- S3模块不仅是一个示例，更是项目的开发标准和质量基准！
- 在新的动态Schema架构下，S3模块通过Module文件同步和AI解析自动生成
- 本文档的Schema结构定义仍是验证AI解析质量的重要参考
- 所有新功能开发必须确保能正确处理S3级别的复杂度

**🔗 相关文档**:
- `dynamic-schema-testing-guide.md` - 完整的动态Schema测试流程
- `development-guide.md` - 新的动态Schema架构说明
- `api-specification.md` - 更新的API接口规范