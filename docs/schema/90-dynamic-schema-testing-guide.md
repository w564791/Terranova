# 动态Schema生成功能测试指南

## 📋 测试概述

本文档描述如何测试新实现的动态Schema生成功能，验证从硬编码到数据库驱动的架构升级。

## 🎯 测试目标

验证以下核心功能：
1. Module文件同步功能
2. 基于variables.tf的Schema自动生成
3. Schema版本管理和状态控制
4. 前端动态表单渲染

## 🧪 测试步骤

### 1. 启动开发环境

```bash
# 启动数据库
make dev-up

# 启动后端
cd backend && go run main.go

# 启动前端
cd frontend && npm run dev
```

### 2. 创建测试Module

访问 http://localhost:5173，登录后：

1. 点击"模块管理"
2. 点击"创建模块"
3. 填写以下信息：
   - 名称: `s3`
   - 提供商: `AWS`
   - 描述: `AWS S3存储桶模块`
   - 仓库URL: `https://github.com/terraform-aws-modules/terraform-aws-s3-bucket`
   - 分支: `main`

### 3. 同步Module文件

1. 在模块列表中找到刚创建的S3模块
2. 点击模块卡片进入详情页
3. 点击"同步文件"按钮
4. 验证同步状态变为"已同步"

**预期结果**: 
- 同步状态从"pending"变为"synced"
- 可以看到Module文件内容（variables.tf, main.tf, outputs.tf）

### 4. 生成Schema

1. 在模块详情页点击"Schema管理"
2. 如果没有Schema，会看到"暂无Schema"提示
3. 点击"AI生成Schema"按钮
4. 等待Schema生成完成

**预期结果**:
- Schema成功生成并存储到数据库
- 可以看到基于variables.tf解析的字段
- Schema标记为"AI生成"和"active"状态

### 5. 验证动态表单渲染

1. 在Schema管理页面查看生成的表单
2. 验证以下字段类型：
   - `name`: string类型，非必填
   - `tags`: object类型，必填，包含Environment子字段
   - `force_destroy`: boolean类型，默认false
   - `lifecycle_rule`: array类型，标记为高级选项
   - `versioning`: object类型
   - `cors_rule`: array类型，高级选项

**预期结果**:
- 表单字段正确渲染
- 高级选项默认隐藏
- 嵌套对象和数组正确处理

### 6. 测试Schema版本管理

1. 再次点击"AI生成Schema"
2. 验证新Schema版本生成
3. 检查旧Schema状态变为"deprecated"

**预期结果**:
- 新Schema版本号递增（如1.0.1）
- 只有一个active状态的Schema
- 旧Schema自动设为deprecated

## 🔍 API测试

### 测试Module文件同步API

```bash
# 同步Module文件
curl -X POST "http://localhost:8080/api/v1/modules/1/sync" \
  -H "Authorization: Bearer YOUR_TOKEN"

# 获取Module文件内容
curl -X GET "http://localhost:8080/api/v1/modules/1/files" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### 测试Schema生成API

```bash
# 生成Schema
curl -X POST "http://localhost:8080/api/v1/module-schemas/1/generate" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "ai_provider": "openai",
    "model": "gpt-4",
    "options": {
      "include_advanced": true,
      "generate_defaults": true
    }
  }'

# 获取Schema列表
curl -X GET "http://localhost:8080/api/v1/module-schemas/1" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

##  验证要点

### 1. 数据库验证

```sql
-- 检查Module文件是否正确存储
SELECT id, name, provider, sync_status, module_files FROM modules WHERE name = 's3';

-- 检查Schema是否正确生成
SELECT id, module_id, version, status, ai_generated, schema_data FROM schemas WHERE module_id = 1;
```

### 2. Schema结构验证

生成的Schema应包含以下结构：

```json
{
  "name": {
    "type": "string",
    "required": false,
    "description": "S3 bucket name"
  },
  "tags": {
    "type": "object",
    "required": true,
    "description": "Resource tags",
    "properties": {
      "Environment": {
        "type": "string",
        "required": true,
        "options": ["dev", "staging", "prod"]
      }
    }
  },
  "lifecycle_rule": {
    "type": "array",
    "required": false,
    "hiddenDefault": true,
    "description": "Lifecycle rules for S3 bucket"
  }
}
```

### 3. 前端表单验证

- [ ] 基础字段正确显示
- [ ] 高级选项默认隐藏
- [ ] 点击"显示高级选项"可展开
- [ ] 嵌套对象字段正确渲染
- [ ] 数组字段支持添加/删除项目
- [ ] 表单验证正常工作

## 🐛 常见问题

### 1. Schema生成失败

**问题**: 点击"AI生成Schema"后返回错误
**解决**: 
- 检查Module文件是否已同步
- 确认variables.tf文件存在且格式正确
- 查看后端日志了解具体错误

### 2. 表单渲染异常

**问题**: Schema生成成功但表单显示异常
**解决**:
- 检查Schema JSON格式是否正确
- 确认前端DynamicForm组件支持所有字段类型
- 查看浏览器控制台错误信息

### 3. 版本管理问题

**问题**: 多次生成Schema版本号不递增
**解决**:
- 检查generateNextVersion函数逻辑
- 确认数据库中Schema记录正确
- 验证版本号解析逻辑

## 📊 性能测试

### 1. Schema生成性能

- 测试大型Module文件的解析时间
- 验证复杂嵌套结构的处理效率
- 检查数据库写入性能

### 2. 前端渲染性能

- 测试大型Schema的表单渲染时间
- 验证深度嵌套表单的响应性能
- 检查内存使用情况

## 🎉 测试完成标准

当以下所有项目都通过时，动态Schema生成功能测试完成：

- [ ] Module文件同步功能正常
- [ ] Schema自动生成功能正常
- [ ] 版本管理功能正常
- [ ] 前端表单渲染正常
- [ ] API接口响应正确
- [ ] 数据库存储正确
- [ ] 性能表现良好
- [ ] 错误处理完善

## 📝 测试报告模板

```
# 动态Schema生成功能测试报告

## 测试环境
- 后端版本: [版本号]
- 前端版本: [版本号]
- 数据库版本: PostgreSQL 15+
- 测试时间: [日期]

## 测试结果
- Module文件同步: /❌
- Schema自动生成: /❌
- 版本管理: /❌
- 前端表单渲染: /❌
- API接口: /❌
- 性能表现: /❌

## 发现的问题
1. [问题描述]
2. [问题描述]

## 建议改进
1. [改进建议]
2. [改进建议]
```

---

**注意**: 这是一个重大架构升级，从硬编码Schema改为完全动态生成。测试时请特别关注数据一致性和功能完整性。