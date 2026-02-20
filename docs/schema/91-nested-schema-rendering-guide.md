# 多层嵌套Schema渲染指南

## 概述
本文档说明如何正确渲染多层嵌套的Schema结构，特别是像`lifecycle_rule`这样的复杂嵌套对象。

## Schema嵌套类型

### 1. TypeListObject (type=11)
使用`elem`字段定义数组元素的结构：

```json
{
  "lifecycle_rule": {
    "type": 11,  // TypeListObject
    "elem": {
      "enabled": {
        "type": 1,  // TypeBool
        "required": true,
        "default": true
      },
      "abort_incomplete_multipart_upload_days": {
        "type": 2,  // TypeInt
        "hidden_default": true,
        "default": 7
      },
      "expiration": {
        "type": 8,  // TypeObject
        "properties": {
          "days": {
            "type": 2,
            "default": 90
          },
          "expired_object_delete_marker": {
            "type": 1,
            "default": false
          }
        }
      },
      "transition": {
        "type": 11,  // 嵌套的TypeListObject
        "elem": {
          "days": {
            "type": 2,
            "required": true
          },
          "storage_class": {
            "type": 4,  // TypeString
            "required": true
          }
        }
      }
    }
  }
}
```

### 2. TypeObject (type=8)
使用`properties`字段定义固定结构：

```json
{
  "expiration": {
    "type": 8,  // TypeObject
    "properties": {
      "days": {
        "type": 2,
        "default": 90
      },
      "date": {
        "type": 4,
        "description": "RFC3339 format date"
      }
    }
  }
}
```

### 3. TypeMap (type=6)
自由的key-value对，没有固定结构：

```json
{
  "tags": {
    "type": 6,  // TypeMap
    "description": "用户自定义标签"
  }
}
```

## 前端渲染逻辑

### FormField组件的递归渲染

```typescript
// 处理TypeListObject (type=11)
case 'array':
  const itemSchema = schema.elem || schema.items?.properties;
  
  return (
    <div className={styles.arrayField}>
      {arrayValue.map((item, index) => (
        <div key={index} className={styles.arrayItem}>
          {/* 递归渲染每个字段 */}
          {Object.entries(itemSchema).map(([propName, propSchema]) => (
            <FormField
              key={propName}
              name={propName}
              schema={propSchema}  // 这里可能又是嵌套的object或array
              value={item?.[propName]}
              onChange={(propValue) => {
                // 更新嵌套值
              }}
            />
          ))}
        </div>
      ))}
    </div>
  );
```

## 嵌套层级示例

以`lifecycle_rule`为例，展示完整的嵌套层级：

```
lifecycle_rule (TypeListObject - 第1层)
├── enabled (TypeBool)
├── id (TypeString)
├── abort_incomplete_multipart_upload_days (TypeInt)
├── expiration (TypeObject - 第2层)
│   ├── days (TypeInt)
│   ├── date (TypeString)
│   └── expired_object_delete_marker (TypeBool)
├── transition (TypeListObject - 第2层)
│   └── [array items]
│       ├── days (TypeInt)
│       ├── date (TypeString)
│       └── storage_class (TypeString)
├── noncurrent_version_expiration (TypeObject - 第2层)
│   ├── days (TypeInt)
│   └── newer_noncurrent_versions (TypeInt)
└── filter (TypeObject - 第2层)
    ├── prefix (TypeString)
    ├── object_size_greater_than (TypeInt)
    ├── object_size_less_than (TypeInt)
    └── tag (TypeObject - 第3层)
        ├── key (TypeString)
        └── value (TypeString)
```

## 关键实现点

### 1. 类型映射
后端数字类型需要映射为前端字符串类型：
- `type: 11` → `type: 'array'` (使用elem字段)
- `type: 8` → `type: 'object'` (使用properties字段)
- `type: 6` → `type: 'map'` (自由键值对)

### 2. 递归渲染
FormField组件必须能够递归渲染自身：
- 当遇到`object`类型时，递归渲染`properties`
- 当遇到`array`类型时，递归渲染`elem`或`items.properties`
- 支持无限层级嵌套

### 3. 数据结构
确保数据结构与Schema匹配：
```javascript
// lifecycle_rule的数据结构
{
  lifecycle_rule: [
    {
      enabled: true,
      id: "rule1",
      expiration: {
        days: 90,
        expired_object_delete_marker: false
      },
      transition: [
        {
          days: 30,
          storage_class: "GLACIER"
        }
      ],
      filter: {
        prefix: "logs/",
        tag: {
          key: "Environment",
          value: "Dev"
        }
      }
    }
  ]
}
```

## 测试要点

1. **基础渲染**：确保第一层字段正确显示
2. **嵌套对象**：验证TypeObject的properties正确渲染
3. **嵌套数组**：验证TypeListObject的elem正确渲染
4. **深层嵌套**：测试3层以上的嵌套结构
5. **数据绑定**：确保所有层级的数据双向绑定正常
6. **添加/删除**：数组项的添加和删除功能正常

## 常见问题

### Q: 为什么有些字段没有显示？
A: 检查以下几点：
1. Schema中是否有`elem`字段（TypeListObject）
2. 类型转换是否正确（数字→字符串）
3. `hidden_default`字段是否被隐藏

### Q: 嵌套层级太深导致性能问题？
A: 可以考虑：
1. 使用React.memo优化组件渲染
2. 实现虚拟滚动（对于大数组）
3. 懒加载深层嵌套的内容

### Q: 如何处理循环引用？
A: 在Schema设计时避免循环引用，或在前端添加深度限制。

## 总结

正确的多层嵌套渲染需要：
1.  完整的类型定义（支持elem字段）
2.  递归的FormField组件
3.  正确的类型映射（数字→字符串）
4.  处理TypeListObject的elem字段
5.  处理TypeObject的properties字段
6.  支持无限层级的递归渲染
