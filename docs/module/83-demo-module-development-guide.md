# S3 Module 开发规范

## 🚨 重要规则

### 对于S3 Demo的任何开发，生成JSON数据使用test函数而不是手动生成JSON数据

每个参数都有标准的Schema结构定义（参见 `files/types` 中的 `type Schema struct`），必须通过编程方式生成JSON，确保数据结构的一致性和准确性。

## 📋 Schema 结构定义

```go
type Schema struct {
    Type                  ValueType   `json:"type"`                      // 字段类型
    Required              bool        `json:"required"`                  // 是否必需
    ForceNew              bool        `json:"force_new"`                 // 更改时是否重建资源
    DiffSuppressOnRefresh bool        `json:"diff_suppress_on_refresh"` // 刷新时是否抑制差异
    Default               interface{} `json:"default"`                   // 默认值
    Description           string      `json:"description"`               // 描述信息
    InputDefault          string      `json:"input_default"`             // 输入默认值
    Elem                  interface{} `json:"elem"`                      // 元素类型（用于集合）
    MaxItems              int         `json:"max_items"`                 // 最大元素数量
    MaxValue              int         `json:"max_value"`                 // 最大值
    MinItems              int         `json:"min_items"`                 // 最小元素数量
    MinValue              int         `json:"min_value"`                 // 最小值
    ComputedWhen          []string    `json:"computed_when"`             // 计算条件
    ConflictsWith         []string    `json:"conflicts_with"`            // 冲突字段
    ExactlyOneOf          []string    `json:"exactly_one_of"`            // 互斥选择
    AtLeastOneOf          []string    `json:"at_least_one_of"`           // 至少一个
    RequiredWith          []string    `json:"required_with"`             // 关联必需
    Deprecated            string      `json:"deprecated"`                // 弃用说明
    Sensitive             bool        `json:"sensitive"`                 // 敏感信息
    WriteOnly             bool        `json:"write_only"`                // 只写字段
    MustInclude           []string    `json:"must_include"`              // 必须包含的值
    UniqItems             bool        `json:"uniq_items"`                // 元素唯一性
    Color                 Color       `json:"color"`                     // 颜色标识
    HiddenDefault         bool        `json:"hidden_default"`            // 默认隐藏
}
```

## 🔧 ValueType 类型定义

```go
type ValueType int

const (
    TypeInvalid ValueType = iota
    TypeBool          // 布尔类型
    TypeInt           // 整数类型
    TypeFloat         // 浮点数类型
    TypeString        // 字符串类型
    TypeList          // 列表类型
    TypeMap           // Map类型（用户可自由添加key-value对）
    TypeSet           // 集合类型
    TypeObject        // 对象类型（固定结构，不可添加新key）
    TypeJsonString    // JSON字符串类型
    TypeText          // 文本类型
    TypeListObject    // 对象列表类型
)
```

## 🎯 TypeMap vs TypeObject 的关键区别

### TypeMap
- **特点**: 用户可以自由添加任意key-value对
- **用途**: 标签系统、自定义配置
- **示例**: `tags`, `default_tags`
- **JSON表现**: 
  ```json
  {
    "type": "map",
    "description": "用户可以自由添加任意标签"
  }
  ```

### TypeObject
- **特点**: 固定的属性结构，预定义的properties
- **用途**: 结构化配置
- **示例**: `versioning`, `logging`, `website`
- **JSON表现**:
  ```json
  {
    "type": "object",
    "properties": {
      "enabled": {...},
      "mfa_delete": {...}
    }
  }
  ```

## 📝 生成Schema JSON的正确方式

### ❌ 错误方式 - 手动编写JSON
```json
// 不要这样做！
{
  "name": {
    "type": "string",
    "required": false,
    "description": "..."
  }
}
```

###  正确方式 - 使用Test函数生成
```go
// s3_module_test.go
package aws

import (
    "encoding/json"
    "testing"
    "io/ioutil"
)

func TestGenerateS3ModuleSchemaJSON(t *testing.T) {
    // 使用 GetS3ModuleSchema() 获取完整的schema定义
    schema := GetS3ModuleSchema()
    
    // 转换为JSON
    jsonData, err := json.MarshalIndent(schema, "", "  ")
    if err != nil {
        t.Fatalf("Failed to marshal schema: %v", err)
    }
    
    // 保存到文件
    err = ioutil.WriteFile("s3_module_schema.json", jsonData, 0644)
    if err != nil {
        t.Fatalf("Failed to write schema file: %v", err)
    }
    
    t.Logf("Schema JSON generated successfully")
}

// 生成数据库可用的schema格式
func TestGenerateS3SchemaForDB(t *testing.T) {
    moduleSchema := GetS3ModuleSchema()
    
    // 转换为数据库schema格式
    dbSchema := map[string]interface{}{
        "module_id": 6,
        "version": "2.0.0",
        "status": "active",
        "schema_data": convertToDBFormat(moduleSchema.Schema),
    }
    
    jsonData, err := json.MarshalIndent(dbSchema, "", "  ")
    if err != nil {
        t.Fatalf("Failed to marshal DB schema: %v", err)
    }
    
    // 保存到文件
    err = ioutil.WriteFile("s3_db_schema.json", jsonData, 0644)
    if err != nil {
        t.Fatalf("Failed to write DB schema file: %v", err)
    }
}

// 转换函数示例
func convertToDBFormat(s3Module S3Module) map[string]interface{} {
    result := make(map[string]interface{})
    
    // 使用反射或手动转换每个字段
    // 这里需要将Go的Schema结构转换为前端可用的JSON格式
    
    return result
}
```

## 🔍 验证Schema完整性

### 检查清单
- [ ] 所有80+个参数都已包含
- [ ] TypeMap字段正确标识（如tags）
- [ ] TypeObject字段包含完整的properties定义
- [ ] TypeListObject字段包含items结构定义
- [ ] 默认值设置正确
- [ ] 必需字段标记准确
- [ ] 描述信息完整清晰

## 📊 S3 Module 参数分类

### 基础配置 (5个)
- `name` - TypeString
- `bucket_prefix` - TypeString
- `acl` - TypeString (with options)
- `policy` - TypeJsonString
- `force_destroy` - TypeBool

### 标签系统 (2个) - TypeMap
- `tags` - TypeMap (用户自由添加)
- `default_tags` - TypeMap (预设默认值)

### 策略附加 (15个) - TypeBool
- `attach_elb_log_delivery_policy`
- `attach_lb_log_delivery_policy`
- `attach_access_log_delivery_policy`
- 等等...

### 复杂配置 - TypeObject
- `website` - 静态网站配置
- `versioning` - 版本控制配置
- `logging` - 日志配置

### 数组配置 - TypeList/TypeListObject
- `cors_rule` - TypeListObject
- `lifecycle_rule` - TypeListObject (最复杂，包含多层嵌套)

## 🚀 开发流程

1. **修改Go代码**: 在 `files/s3_module` 中更新Schema定义
2. **运行Test函数**: 生成JSON数据
3. **验证JSON**: 确保所有字段正确
4. **导入数据库**: 将生成的schema_data存入数据库
5. **测试前端渲染**: 验证表单正确显示所有参数

##  注意事项

1. **永远不要手动编写Schema JSON**
2. **任何Schema修改都必须通过Go代码进行**
3. **使用test函数验证和生成最终的JSON数据**
4. **确保TypeMap和TypeObject的区别正确体现**
5. **复杂嵌套结构要完整保留层级关系**

## 📚 相关文档

- `files/s3_module` - S3 Module的Go定义
- `files/types` - Schema类型定义
- `docs/dynamic-schema-guide.md` - 动态Schema架构指南
- `docs/s3-demo-verification-guide.md` - S3演示验证指南
