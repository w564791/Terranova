# OpenAI Compatible API 支持

## 概述

AI 错误分析功能现已支持 OpenAI Compatible API，可以使用 OpenAI、Azure OpenAI、Ollama 等兼容服务。

## 支持的服务类型

### 1. AWS Bedrock
- 使用 IAM 认证
- 支持 Claude 等模型
- 支持 Cross-Region Inference Profile

### 2. OpenAI
- 标准 OpenAI API
- Base URL: `https://api.openai.com/v1`
- 支持 GPT-4、GPT-3.5-turbo 等模型

### 3. Azure OpenAI
- Azure 托管的 OpenAI 服务
- Base URL: `https://your-resource.openai.azure.com`
- 使用 Azure 部署名称作为模型 ID

### 4. Ollama
- 本地运行的开源模型
- Base URL: `http://localhost:11434/v1`
- 支持 Llama2、Mistral 等模型

## 配置参数

### 必需参数（所有服务）
- **service_type**: 服务类型（bedrock/openai/azure_openai/ollama）
- **model_id**: 模型 ID
- **enabled**: 是否启用
- **rate_limit_seconds**: 频率限制（秒）

### Bedrock 特有参数
- **aws_region**: AWS 区域
- **use_inference_profile**: 是否使用 Cross-Region Inference Profile

### OpenAI Compatible 特有参数
- **base_url**: API 基础 URL
- **api_key**: API 密钥（加密存储）

### 可选参数
- **custom_prompt**: 自定义 Prompt（追加到默认 Prompt 之后）

## 安全特性

1. **API Key 加密存储**
   - API Key 在数据库中加密存储
   - 查询时不返回实际 API Key，显示为 `********`
   - 更新时留空表示不修改

2. **全局唯一启用**
   - 同一时间只能有一个 AI 配置被启用
   - 启用新配置会自动禁用其他所有配置
   - 确保系统行为一致性

## 数据库变更

```sql
-- 添加 OpenAI Compatible 支持字段
ALTER TABLE ai_configs ADD COLUMN base_url VARCHAR(500);
ALTER TABLE ai_configs ADD COLUMN api_key TEXT;
```

## API 调用流程

### Bedrock
1. 使用 AWS SDK 加载凭证
2. 创建 Bedrock Runtime 客户端
3. 调用 InvokeModel API
4. 解析 Claude 格式响应

### OpenAI Compatible
1. 构建标准 OpenAI 请求格式
2. 发送 POST 请求到 `{base_url}/chat/completions`
3. 设置 Authorization header: `Bearer {api_key}`
4. 解析标准 OpenAI 响应格式

## 使用示例

### OpenAI 配置
```json
{
  "service_type": "openai",
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-...",
  "model_id": "gpt-4",
  "enabled": true,
  "rate_limit_seconds": 10
}
```

### Azure OpenAI 配置
```json
{
  "service_type": "azure_openai",
  "base_url": "https://your-resource.openai.azure.com",
  "api_key": "your-azure-key",
  "model_id": "your-deployment-name",
  "enabled": true,
  "rate_limit_seconds": 10
}
```

### Ollama 配置
```json
{
  "service_type": "ollama",
  "base_url": "http://localhost:11434/v1",
  "api_key": "ollama",
  "model_id": "llama2",
  "enabled": true,
  "rate_limit_seconds": 10
}
```

## 前端界面

### 服务类型选择
- 下拉菜单选择服务类型
- 根据选择动态显示相应字段
- Bedrock 显示 Region 和模型选择器
- OpenAI Compatible 显示 Base URL、API Key 和模型 ID 输入框

### 配置列表
- 显示服务类型标签
- 根据服务类型显示不同的配置信息
- Bedrock 显示 Region
- OpenAI Compatible 显示 Base URL
- 显示频率限制和启用状态

## 注意事项

1. **全局唯一性**
   - 启用新配置会自动禁用其他配置
   - 前端显示明确提示

2. **API Key 安全**
   - 编辑时不显示实际 API Key
   - 留空表示不修改
   - 仅在创建时必填

3. **频率限制**
   - 建议设置 10-60 秒
   - 防止 API 滥用
   - 每个用户独立计算

4. **错误处理**
   - API 调用失败会返回详细错误信息
   - 超时设置为 60 秒
   - 自动处理 JSON 解析错误

## 兼容性

- 兼容所有实现 OpenAI Chat Completions API 的服务
- 包括但不限于：
  - OpenAI
  - Azure OpenAI
  - Ollama
  - vLLM
  - LocalAI
  - 其他 OpenAI Compatible 服务

## 后续优化

1. 支持多个 AI 配置同时启用（按优先级）
2. 添加 API 调用统计和监控
3. 支持更多 AI 服务提供商
4. 添加配置测试功能
5. 支持自定义请求参数（temperature、max_tokens 等）
