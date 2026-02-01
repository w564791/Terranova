# Hidden Default 功能说明

## 概述
`hidden_default` 是一个重要的Schema属性，用于实现"渐进式表单"的用户体验。

## 功能说明

### 什么是 hidden_default？
- `hidden_default: true` 表示该字段默认不显示在表单中
- 用户可以通过点击"显示高级选项"按钮来显示这些字段
- 适用于不常用但偶尔需要配置的高级参数

### 工作原理

1. **后端Schema定义**（demo/s3_module.go）:
```go
AbortIncompleteMultipartUploadDays: func() Schema {
    s := defaultSchema()
    s.Type = TypeInt
    s.HiddenDefault = true  // 标记为高级选项
    s.Default = 7
    s.Required = false
    s.Description = "Configuration block that specifies..."
    return s
}()
```

2. **生成的JSON**:
```json
"abort_incomplete_multipart_upload_days": {
  "type": 2,
  "hidden_default": true,  // 这个字段默认隐藏
  "default": 7,
  "description": "...",
  ...
}
```

3. **前端渲染逻辑**（frontend/src/components/DynamicForm/index.tsx）:
```typescript
// 分组字段：基础字段和高级字段
const basicFields = Object.entries(schema).filter(([fieldName, fieldSchema]) => 
  !fieldSchema.hidden_default || values[fieldName] !== undefined
);

const advancedFields = Object.entries(schema).filter(([fieldName, fieldSchema]) => 
  fieldSchema.hidden_default && values[fieldName] === undefined
);
```

## 用户体验

### 初始状态
- 只显示基础字段（`hidden_default: false` 或未设置的字段）
- 高级字段被隐藏，但显示一个"显示高级选项"按钮

### 点击"显示高级选项"后
- 所有`hidden_default: true`的字段会显示出来
- 用户可以根据需要配置这些高级参数

### 已配置的高级字段
- 如果用户已经为某个`hidden_default: true`的字段设置了值
- 该字段会始终显示，不会被隐藏

## S3 Module中的应用

在S3 Module中，以下类型的字段通常设置为`hidden_default: true`：

1. **生命周期配置的高级选项**
   - `abort_incomplete_multipart_upload_days`
   - `noncurrent_version_expiration`
   - `noncurrent_version_transition`

2. **不常用的策略配置**
   - 各种`attach_*_policy`字段（默认为false）

3. **高级加密和安全选项**
   - `object_lock_configuration`
   - `intelligent_tiering`

4. **监控和分析配置**
   - `metric_configuration`
   - `inventory_configuration`
   - `analytics_configuration`

## 实现细节

### 判断逻辑
```javascript
// 字段应该显示的条件：
// 1. 不是hidden_default字段
// 2. 或者是hidden_default但用户已经设置了值
// 3. 或者用户点击了"显示高级选项"

const shouldShowField = (fieldSchema, fieldValue, showAdvanced) => {
  return !fieldSchema.hidden_default || 
         fieldValue !== undefined || 
         showAdvanced;
};
```

### 统计高级字段数量
```javascript
const advancedFieldsCount = Object.values(schema)
  .filter(fieldSchema => 
    fieldSchema.hidden_default && 
    values[fieldName] === undefined
  ).length;
```

## 最佳实践

1. **合理使用hidden_default**
   - 常用字段：`hidden_default: false` 或不设置
   - 高级选项：`hidden_default: true`

2. **提供合理的默认值**
   - 即使字段隐藏，也应该有合理的默认值
   - 例如：`abort_incomplete_multipart_upload_days`默认为7天

3. **清晰的描述**
   - 高级字段更需要详细的描述
   - 帮助用户理解何时需要配置这些选项

## 总结

`hidden_default`功能实现了：
-  简化初始表单，降低用户认知负担
-  保留高级配置能力，满足专业用户需求
-  渐进式披露，提供更好的用户体验
-  已配置的高级选项始终可见，避免用户困惑
