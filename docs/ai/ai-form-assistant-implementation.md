# AI 表单助手实现文档

## 概述

AI 表单助手为 OpenAPI v3 Schema 驱动的 Module 表单提供智能配置生成能力。用户可以通过自然语言描述需求，AI 会根据 Module 的 Schema 约束生成符合规范的配置值。

## 架构设计

### 后端组件

```
backend/
├── services/
│   └── ai_form_service.go      # AI 表单服务核心逻辑
├── controllers/
│   └── ai_form_controller.go   # API 控制器
└── internal/router/
    └── router_ai.go            # 路由配置
```

### 前端组件

```
frontend/src/
├── services/
│   └── aiForm.ts               # AI 表单 API 服务
└── components/OpenAPIFormRenderer/
    ├── AIFormAssistant/
    │   ├── AIConfigGenerator.tsx       # AI 配置生成器组件
    │   ├── AIConfigGenerator.module.css # 样式
    │   └── index.ts                    # 导出
    ├── FormRenderer.tsx        # 集成 AI 助手
    └── types.ts                # 类型定义（AIAssistantConfig）
```

## API 设计

### POST /api/ai/form/generate

生成表单配置。

**请求体：**
```json
{
  "module_id": 1,
  "user_description": "创建一个生产环境的 S3 存储桶，启用版本控制和加密",
  "context_ids": {
    "workspace_id": "ws-xxx",
    "organization_id": "org-xxx",
    "manifest_id": "manifest-xxx"
  }
}
```

**响应体：**
```json
{
  "status": "complete",  // 或 "need_more_info"
  "config": {
    "bucket_name": "my-production-bucket",
    "versioning_enabled": true,
    "encryption_enabled": true,
    "vpc_id": "<YOUR_VPC_ID>"
  },
  "placeholders": [
    {
      "field": "vpc_id",
      "placeholder": "<YOUR_VPC_ID>",
      "description": "请填写您的 VPC ID，格式如：vpc-xxxxxxxxx",
      "help_link": "https://docs.aws.amazon.com/..."
    }
  ],
  "message": "配置生成完成"
}
```

## 安全设计

### 1. 输入清洗

防止 Prompt Injection 攻击：

```go
func (s *AIFormService) sanitizeUserInput(input string) string {
    // 1. 长度限制（1000 字符）
    // 2. 移除危险模式（指令覆盖、角色扮演、代码注入）
    // 3. 只保留安全字符
    // 4. 规范化空白
}
```

### 2. 输出验证

验证 AI 输出符合 Schema 约束：

```go
func (s *AIFormService) validateAIOutput(output string, schema map[string]interface{}) (map[string]interface{}, error) {
    // 1. 提取 JSON
    // 2. 验证类型
    // 3. 验证约束（枚举、长度、范围）
    // 4. 检查可疑内容
}
```

### 3. 占位符机制

对于 AI 无法确定的值（如资源 ID），使用占位符：

- `<YOUR_VPC_ID>` - VPC ID
- `<YOUR_SUBNET_ID>` - Subnet ID
- `<YOUR_AMI_ID>` - AMI ID
- `<YOUR_SECURITY_GROUP_ID>` - Security Group ID
- `<YOUR_KMS_KEY_ID>` - KMS Key ID
- `<YOUR_IAM_ROLE_ARN>` - IAM Role ARN
- `<YOUR_ACCOUNT_ID>` - AWS Account ID

## 使用方式

### 在 FormRenderer 中启用 AI 助手

```tsx
<FormRenderer
  schema={moduleSchema}
  initialValues={formValues}
  onChange={handleChange}
  aiAssistant={{
    enabled: true,
    moduleId: module.id,
    workspaceId: workspace?.id,
    organizationId: organization?.id,
    manifestId: manifest?.id,
  }}
/>
```

### AIAssistantConfig 类型

```typescript
interface AIAssistantConfig {
  enabled: boolean;
  moduleId: number;           // 必须传入
  workspaceId?: string;       // 可选上下文
  organizationId?: string;
  manifestId?: string;
  position?: 'inline' | 'panel' | 'floating';
  capabilities?: ('generate' | 'suggest' | 'validate')[];
}
```

## 复用场景

AI 表单助手可以在以下场景复用：

1. **Manifest 编辑器** - 为 Module 节点生成配置
2. **Module Demo** - 快速生成示例配置
3. **Workspace Resource** - 创建/编辑资源配置
4. **独立 Module 表单** - 任何使用 OpenAPI Schema 的表单

## AI 配置要求

需要在 `ai_configs` 表中配置支持 `form_generation` 能力的 AI 服务：

```sql
INSERT INTO ai_configs (
  name, service_type, model_id, aws_region, 
  capabilities, priority, is_enabled
) VALUES (
  'Form Generation', 'bedrock', 'anthropic.claude-3-sonnet-20240229-v1:0', 'us-east-1',
  '["form_generation"]', 10, true
);
```

## 已集成的页面

### ManifestEditor

在 Manifest 编辑器中，当选中一个 Module 节点时，右侧属性面板会显示 AI 助手按钮：

```tsx
// frontend/src/pages/admin/ManifestEditor.tsx
<ModuleFormRenderer
  schema={nodeSchema.openapi_schema}
  initialValues={selectedNode.data?.config || {}}
  onChange={(values) => { ... }}
  aiAssistant={selectedNode.data?.module_id ? {
    enabled: true,
    moduleId: selectedNode.data.module_id,
    manifestId: manifestId,
  } : undefined}
  manifest={{ ... }}
/>
```

### 其他页面

通过 `ModuleFormRenderer` 组件，以下页面也可以启用 AI 助手：

- **DemoDetail** - Demo 详情页
- **EditDemo** - Demo 编辑页
- **DemoForm** - Demo 表单组件

只需传入 `aiAssistant` prop 即可启用。

## 后续扩展

1. **字段级建议** - 为单个字段提供智能建议
2. **配置验证** - AI 辅助验证配置合理性
3. **配置优化** - 根据最佳实践优化配置
4. **多轮对话** - 支持追问和澄清
5. **更多页面集成** - AddResources, EditResource, CreateDemo 等页面
