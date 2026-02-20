# S3模块Demo验证指南

## 🎯 验证目标

验证S3模块的各个参数在前端动态表单中的正确渲染，确保支持：
- 基础字段类型（string, boolean, number）
- 复杂嵌套对象（tags, versioning, logging）
- 数组字段（lifecycle_rule, cors_rule）
- 高级选项的显示/隐藏
- 字段验证和默认值

## 📋 S3模块Demo信息

**模块ID**: 7  
**模块名称**: s3-demo  
**Schema ID**: 3  
**Schema版本**: 1.0.0  

## 🧪 验证步骤

### 1. 访问S3模块
1. 打开浏览器访问 http://localhost:5173
2. 使用 admin/admin123 登录
3. 点击"模块管理"
4. 找到"s3-demo"模块卡片
5. 点击进入模块详情页

### 2. 进入Schema管理
1. 在模块详情页点击"Schema管理"按钮
2. 应该看到一个active状态的Schema (v1.0.0)
3. 右侧显示基于Schema的动态表单

### 3. 验证基础字段渲染

#### 3.1 字符串字段
- **name**: 文本输入框，非必填，占位符显示
- **bucket_prefix**: 文本输入框，非必填
- **acl**: 下拉选择框，包含选项：private, public-read等

#### 3.2 布尔字段
- **force_destroy**: 复选框，默认false

#### 3.3 必填字段验证
- **tags**: 标记为必填，应显示红色星号或必填提示

### 4. 验证复杂嵌套对象

#### 4.1 tags对象
```json
{
  "Environment": "下拉选择 (dev/staging/prod)",
  "Project": "文本输入，必填",
  "Owner": "文本输入，可选"
}
```

验证点：
- [ ] tags展开显示3个子字段
- [ ] Environment显示为下拉选择
- [ ] Project标记为必填
- [ ] Owner为可选字段

#### 4.2 versioning对象（高级选项）
```json
{
  "enabled": "复选框，默认false"
}
```

验证点：
- [ ] 默认隐藏（hiddenDefault: true）
- [ ] 点击"显示高级选项"后可见
- [ ] enabled显示为复选框

### 5. 验证数组字段（高级选项）

#### 5.1 lifecycle_rule数组
验证点：
- [ ] 默认隐藏在高级选项中
- [ ] 显示"添加项目"按钮
- [ ] 点击添加后显示对象表单：
  - enabled: 复选框（默认true）
  - id: 文本输入（必填）
  - expiration: 嵌套对象
    - days: 数字输入（默认30）
  - transition: 嵌套数组
- [ ] 支持删除已添加的项目

#### 5.2 cors_rule数组
验证点：
- [ ] 默认隐藏在高级选项中
- [ ] 添加项目后显示：
  - allowed_methods: 多选下拉（GET/PUT/POST/DELETE/HEAD）
  - allowed_origins: 字符串数组
  - max_age_seconds: 数字输入（默认3600）

### 6. 验证高级选项功能

#### 6.1 显示/隐藏切换
- [ ] 默认只显示基础字段（hiddenDefault: false）
- [ ] 高级字段默认隐藏（hiddenDefault: true）
- [ ] "显示高级选项"按钮正常工作
- [ ] 切换后高级字段正确显示/隐藏

#### 6.2 高级字段列表
应该隐藏的字段：
- versioning
- lifecycle_rule  
- cors_rule
- logging
- block_public_acls
- block_public_policy

### 7. 验证表单交互

#### 7.1 数据输入
1. 填写基础字段：
   - name: "my-test-bucket"
   - acl: "private"
   - force_destroy: true
   - tags.Environment: "dev"
   - tags.Project: "iac-platform"

2. 显示高级选项并填写：
   - versioning.enabled: true
   - 添加一个lifecycle_rule
   - 添加一个cors_rule

#### 7.2 表单验证
- [ ] 必填字段验证正常
- [ ] 数字字段只接受数字输入
- [ ] 下拉选择限制在预定义选项内

#### 7.3 数据生成
1. 点击"生成配置"按钮
2. 检查控制台输出的JSON数据
3. 验证数据结构完整性

##  验证检查清单

### 基础功能
- [ ] S3模块在模块列表中显示
- [ ] 模块详情页正常加载
- [ ] Schema管理页面正常显示
- [ ] 动态表单基于Schema正确渲染

### 字段类型支持
- [ ] string类型：文本输入框
- [ ] boolean类型：复选框
- [ ] number类型：数字输入框
- [ ] object类型：嵌套字段组
- [ ] array类型：可添加/删除的项目列表

### 高级功能
- [ ] 必填字段验证
- [ ] 默认值正确设置
- [ ] 下拉选择选项正确
- [ ] 高级选项显示/隐藏
- [ ] 嵌套对象递归渲染
- [ ] 数组项目管理（添加/删除）

### 数据处理
- [ ] 表单数据实时更新
- [ ] JSON输出格式正确
- [ ] 复杂嵌套结构完整

## 🐛 常见问题排查

### 1. 模块不显示
- 检查数据库连接
- 确认模块数据正确插入
- 查看浏览器控制台错误

### 2. Schema不加载
- 确认Schema数据正确
- 检查API响应格式
- 验证JSON Schema格式

### 3. 表单渲染异常
- 检查DynamicForm组件
- 确认字段类型支持
- 查看组件错误日志

### 4. 高级选项不工作
- 验证hiddenDefault字段
- 检查显示切换逻辑
- 确认CSS样式正确

## 📊 预期结果

完成验证后，应该确认：

1. **S3模块完整支持** - 所有字段类型正确渲染
2. **复杂结构处理** - 嵌套对象和数组正常工作
3. **用户体验良好** - 高级选项、验证、默认值等功能完善
4. **数据完整性** - 生成的配置数据结构正确

这个S3模块demo将作为项目的标准参考，确保动态Schema生成系统能够处理真实世界的复杂Terraform模块。

## 🔗 相关文档

- `dynamic-schema-testing-guide.md` - 完整的动态Schema测试流程
- `s3-module-demo-guide.md` - S3模块开发规范参考
- `development-guide.md` - 项目架构和设计说明