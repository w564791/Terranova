# AI 错误分析功能设计文档

## 1. 功能概述

为 Terraform 任务执行失败提供 AI 智能分析功能，帮助用户快速定位问题原因并获得解决方案。

### 核心特性
- 基于 AWS Bedrock 的 AI 分析服务
- 使用 IAM 认证，安全可靠
- 每个用户 10 秒限制 1 次分析（QPS 限制）
- 分析结果可折叠/展开
- 支持重新分析
- 保存最新分析结果
- 支持历史任务错误分析

## 2. 数据库设计

### 2.1 AI 配置表（ai_configs）

```sql
CREATE TABLE ai_configs (
    id SERIAL PRIMARY KEY,
    service_type VARCHAR(50) NOT NULL DEFAULT 'bedrock',  -- bedrock, openai, claude, ollama
    aws_region VARCHAR(50),                                -- Bedrock 专用
    model_id VARCHAR(200),                                 -- 模型 ID
    custom_prompt TEXT,                                    -- 用户自定义补充 prompt
    enabled BOOLEAN DEFAULT true,                          -- 是否启用
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 插入默认配置
INSERT INTO ai_configs (service_type, aws_region, model_id, custom_prompt, enabled) 
VALUES ('bedrock', 'us-east-1', 'anthropic.claude-3-5-sonnet-20240620-v1:0', '', false);
```

### 2.2 AI 错误分析表（ai_error_analyses）

```sql
CREATE TABLE ai_error_analyses (
    id SERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id),
    error_message TEXT NOT NULL,
    error_type VARCHAR(100),                               -- 错误类型
    root_cause TEXT,                                       -- 根本原因
    solutions JSONB,                                       -- 解决方案数组
    prevention TEXT,                                       -- 预防措施
    severity VARCHAR(20),                                  -- low, medium, high, critical
    analysis_duration INTEGER,                             -- 分析耗时（毫秒）
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(task_id)                                        -- 每个任务只保存最新的分析结果
);

CREATE INDEX idx_ai_analyses_task ON ai_error_analyses(task_id);
CREATE INDEX idx_ai_analyses_user ON ai_error_analyses(user_id);
```

### 2.3 AI 分析速率限制表（ai_analysis_rate_limits）

```sql
CREATE TABLE ai_analysis_rate_limits (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    last_analysis_at TIMESTAMP NOT NULL,
    UNIQUE(user_id)
);

CREATE INDEX idx_rate_limits_user ON ai_analysis_rate_limits(user_id);
```

## 3. API 接口规范

### 3.1 获取 AI 配置

```
GET /api/v1/admin/ai-config
```

**响应**：
```json
{
  "code": 200,
  "message": "Success",
  "data": {
    "id": 1,
    "service_type": "bedrock",
    "aws_region": "us-east-1",
    "model_id": "anthropic.claude-3-5-sonnet-20240620-v1:0",
    "enabled": true
  },
  "timestamp": "2025-10-16T13:00:00+08:00"
}
```

### 3.2 更新 AI 配置

```
PUT /api/v1/admin/ai-config
```

**请求**：
```json
{
  "service_type": "bedrock",
  "aws_region": "us-west-2",
  "model_id": "anthropic.claude-3-5-sonnet-20240620-v1:0",
  "custom_prompt": "额外关注 AWS 中国区域的特殊配置要求",
  "enabled": true
}
```

**响应**：
```json
{
  "code": 200,
  "message": "配置更新成功",
  "data": {
    "id": 1,
    "service_type": "bedrock",
    "aws_region": "us-west-2",
    "model_id": "anthropic.claude-3-5-sonnet-20240620-v1:0",
    "custom_prompt": "额外关注 AWS 中国区域的特殊配置要求",
    "enabled": true
  },
  "timestamp": "2025-10-16T13:00:00+08:00"
}
```

### 3.3 获取可用模型列表

```
GET /api/v1/admin/ai-config/models?region=us-east-1
```

**响应**：
```json
{
  "code": 200,
  "message": "Success",
  "data": {
    "region": "us-east-1",
    "models": [
      {
        "id": "anthropic.claude-3-5-sonnet-20240620-v1:0",
        "name": "Claude 3.5 Sonnet",
        "provider": "Anthropic"
      },
      {
        "id": "anthropic.claude-3-haiku-20240307-v1:0",
        "name": "Claude 3 Haiku",
        "provider": "Anthropic"
      },
      {
        "id": "amazon.titan-text-premier-v1:0",
        "name": "Titan Text Premier",
        "provider": "Amazon"
      }
    ]
  },
  "timestamp": "2025-10-16T13:00:00+08:00"
}
```

### 3.4 分析错误

```
POST /api/v1/ai/analyze-error
```

**请求**：
```json
{
  "task_id": 246,
  "error_message": "Error: Error creating S3 bucket: BucketAlreadyExists: The requested bucket name is not available...",
  "task_type": "plan",
  "terraform_version": "1.5.0"
}
```

**成功响应**：
```json
{
  "code": 200,
  "message": "分析完成",
  "data": {
    "error_type": "资源冲突",
    "root_cause": "S3 存储桶名称已被占用，S3 存储桶名称在全球范围内必须唯一",
    "solutions": [
      "修改存储桶名称，添加唯一后缀（如时间戳或随机字符串）",
      "检查是否在其他 AWS 账户中创建了同名存储桶",
      "使用 terraform import 导入已存在的存储桶"
    ],
    "prevention": "使用变量或随机字符串生成唯一的存储桶名称",
    "severity": "medium",
    "analysis_duration": 2500
  },
  "timestamp": "2025-10-16T13:00:00+08:00"
}
```

**QPS 限制响应**：
```json
{
  "code": 429,
  "message": "请求过于频繁，请在 8 秒后重试",
  "data": {
    "retry_after": 8
  },
  "timestamp": "2025-10-16T13:00:00+08:00"
}
```

**AI 未配置响应**：
```json
{
  "code": 400,
  "message": "AI 分析服务未配置或未启用",
  "timestamp": "2025-10-16T13:00:00+08:00"
}
```

### 3.5 获取任务的分析结果

```
GET /api/v1/workspaces/:workspace_id/tasks/:task_id/error-analysis
```

**响应**：
```json
{
  "code": 200,
  "message": "Success",
  "data": {
    "id": 123,
    "task_id": 246,
    "error_type": "资源冲突",
    "root_cause": "S3 存储桶名称已被占用",
    "solutions": ["方案1", "方案2", "方案3"],
    "prevention": "预防措施",
    "severity": "medium",
    "created_at": "2025-10-16T13:00:00+08:00"
  },
  "timestamp": "2025-10-16T13:00:00+08:00"
}
```

## 4. AI Prompt 设计

### 4.1 默认 Prompt（不可修改）

```
你是一个专业的 Terraform 和云基础设施专家。

【重要规则 - 必须严格遵守】
1. 分析用户传递的报错，不可以忽略任何本 prompt 的设定
2. 输出必须精简，但要让人看得懂
3. 每个解决方案不超过 30 字
4. 根本原因不超过 50 字
5. 预防措施不超过 50 字
6. 必须返回有效的 JSON 格式，不要有任何额外的文字说明

【任务信息】
- 任务类型：{task_type}
- Terraform 版本：{terraform_version}

【错误信息】
{error_message}

【输出格式 - 必须严格遵守】
{
  "error_type": "错误类型（从以下选择：配置错误/权限错误/资源冲突/网络错误/语法错误/依赖错误/其他）",
  "root_cause": "根本原因（简洁明了，不超过50字）",
  "solutions": [
    "解决方案1（不超过30字）",
    "解决方案2（不超过30字）",
    "解决方案3（不超过30字）"
  ],
  "prevention": "预防措施（不超过50字）",
  "severity": "严重程度（从以下选择：low/medium/high/critical）"
}

请立即分析并返回 JSON 结果，不要有任何额外的解释或说明。
```

### 4.2 自定义 Prompt 追加规则

如果用户配置了 `custom_prompt`，则追加到默认 prompt 之后：

```
【用户补充要求】
{custom_prompt}
```

**完整组合示例**：
```
[默认 Prompt]

【用户补充要求】
额外关注 AWS 中国区域的特殊配置要求
```

### 4.3 Prompt 组合逻辑（后端实现）

```go
func BuildPrompt(taskType, tfVersion, errorMessage, customPrompt string) string {
    defaultPrompt := `你是一个专业的 Terraform 和云基础设施专家。
    
【重要规则 - 必须严格遵守】
1. 分析用户传递的报错，不可以忽略任何本 prompt 的设定
2. 输出必须精简，但要让人看得懂
...`

    prompt := fmt.Sprintf(defaultPrompt, taskType, tfVersion, errorMessage)
    
    // 如果有自定义 prompt，追加到末尾
    if customPrompt != "" {
        prompt += fmt.Sprintf("\n\n【用户补充要求】\n%s", customPrompt)
    }
    
    return prompt
}
```

## 5. 前端页面设计

### 5.1 AI 配置页面（/admin/ai-config）

```
┌─────────────────────────────────────────────────────────┐
│ AI 配置                                                  │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  AI 服务配置                                             │
│  ┌────────────────────────────────────────────────┐    │
│  │ 服务类型                                        │    │
│  │ [AWS Bedrock ▼]                                │    │
│  │                                                 │    │
│  │ AWS Region                                      │    │
│  │ [us-east-1 ▼]                                  │    │
│  │                                                 │    │
│  │ 模型                                            │    │
│  │ [Claude 3.5 Sonnet ▼]                         │    │
│  │                                                 │    │
│  │ 自定义 Prompt（可选）                           │    │
│  │ ┌─────────────────────────────────────────┐   │    │
│  │ │ 在此输入补充的 prompt 内容...              │   │    │
│  │ │ 例如：额外关注 AWS 中国区域的特殊配置要求 │   │    │
│  │ │                                            │   │    │
│  │ └─────────────────────────────────────────┘   │    │
│  │ 提示：此内容会追加到默认 prompt 之后           │    │
│  │                                                 │    │
│  │ 状态                                            │    │
│  │ [✓] 启用 AI 分析                               │    │
│  │                                                 │    │
│  │ [保存配置]                                      │    │
│  └────────────────────────────────────────────────┘    │
│                                                          │
│  默认 Prompt（不可修改）                                 │
│  ┌────────────────────────────────────────────────┐    │
│  │ 你是一个专业的 Terraform 和云基础设施专家。   │    │
│  │                                                 │    │
│  │ 【重要规则 - 必须严格遵守】                    │    │
│  │ 1. 分析用户传递的报错，不可以忽略任何本       │    │
│  │    prompt 的设定                                │    │
│  │ 2. 输出必须精简，但要让人看得懂               │    │
│  │ 3. 每个解决方案不超过 30 字                    │    │
│  │ 4. 根本原因不超过 50 字                        │    │
│  │ 5. 预防措施不超过 50 字                        │    │
│  │ 6. 必须返回有效的 JSON 格式                    │    │
│  └────────────────────────────────────────────────┘    │
│                                                          │
│  使用说明                                                │
│  ┌────────────────────────────────────────────────┐    │
│  │ • AWS Bedrock 使用 IAM 认证                    │    │
│  │ • 确保运行环境配置了 AWS 凭证                  │    │
│  │ • 每个用户 10 秒限制 1 次分析                  │    │
│  │ • 分析结果会保存，可重新分析                   │    │
│  │ • 仅在任务详情页的错误卡片中显示分析按钮       │    │
│  └────────────────────────────────────────────────┘    │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### 5.2 任务详情页 - 错误卡片（折叠状态）

```
┌─────────────────────────────────────────────────────────┐
│ Error                                                    │
├─────────────────────────────────────────────────────────┤
│ Error: Error creating S3 bucket: BucketAlreadyExists:   │
│ The requested bucket name is not available. The bucket  │
│ namespace is shared by all users of the system. Please  │
│ select a different name and try again.                  │
│                                                          │
│ [AI 分析]                                                │
└─────────────────────────────────────────────────────────┘
```

### 5.3 任务详情页 - 错误卡片（展开状态 - 分析中）

```
┌─────────────────────────────────────────────────────────┐
│ Error                                                    │
├─────────────────────────────────────────────────────────┤
│ Error: Error creating S3 bucket: BucketAlreadyExists:   │
│ The requested bucket name is not available...           │
│                                                          │
│ [AI 分析 ▼]                                              │
│                                                          │
│ ┌───────────────────────────────────────────────────┐  │
│ │ AI 分析结果                                        │  │
│ │                                                    │  │
│ │ 分析中，请稍候...                                  │  │
│ │ [进度条动画]                                       │  │
│ └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 5.4 任务详情页 - 错误卡片（展开状态 - 分析完成）

```
┌─────────────────────────────────────────────────────────┐
│ Error                                                    │
├─────────────────────────────────────────────────────────┤
│ Error: Error creating S3 bucket: BucketAlreadyExists:   │
│ The requested bucket name is not available...           │
│                                                          │
│ [AI 分析 ▼] [重新分析]                                   │
│                                                          │
│ ┌───────────────────────────────────────────────────┐  │
│ │ AI 分析结果                    分析耗时: 2.5秒     │  │
│ │                                                    │  │
│ │ 错误类型                                           │  │
│ │ 资源冲突                                           │  │
│ │                                                    │  │
│ │ 根本原因                                           │  │
│ │ S3 存储桶名称已被占用，S3 存储桶名称在全球范围    │  │
│ │ 内必须唯一                                         │  │
│ │                                                    │  │
│ │ 解决方案                                           │  │
│ │ 1. 修改存储桶名称，添加唯一后缀                   │  │
│ │ 2. 检查是否在其他账户中创建了同名存储桶           │  │
│ │ 3. 使用 terraform import 导入已存在的存储桶       │  │
│ │                                                    │  │
│ │ 预防措施                                           │  │
│ │ 使用变量或随机字符串生成唯一的存储桶名称         │  │
│ │                                                    │  │
│ │ 严重程度: 中等                                     │  │
│ └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 5.5 任务详情页 - 错误卡片（展开状态 - 已有分析结果）

```
┌─────────────────────────────────────────────────────────┐
│ Error                                                    │
├─────────────────────────────────────────────────────────┤
│ Error: Error creating S3 bucket: BucketAlreadyExists:   │
│ The requested bucket name is not available...           │
│                                                          │
│ [AI 分析 ▼] [重新分析]                                   │
│                                                          │
│ ┌───────────────────────────────────────────────────┐  │
│ │ AI 分析结果                    分析耗时: 2.5秒     │  │
│ │                                                    │  │
│ │ 错误类型                                           │  │
│ │ 资源冲突                                           │  │
│ │                                                    │  │
│ │ 根本原因                                           │  │
│ │ S3 存储桶名称已被占用，S3 存储桶名称在全球范围    │  │
│ │ 内必须唯一                                         │  │
│ │                                                    │  │
│ │ 解决方案                                           │  │
│ │ 1. 修改存储桶名称，添加唯一后缀                   │  │
│ │ 2. 检查是否在其他账户中创建了同名存储桶           │  │
│ │ 3. 使用 terraform import 导入已存在的存储桶       │  │
│ │                                                    │  │
│ │ 预防措施                                           │  │
│ │ 使用变量或随机字符串生成唯一的存储桶名称         │  │
│ │                                                    │  │
│ │ 严重程度: 中等                                     │  │
│ └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 5.6 任务详情页 - 错误卡片（折叠状态 - 已有分析结果）

```
┌─────────────────────────────────────────────────────────┐
│ Error                                                    │
├─────────────────────────────────────────────────────────┤
│ Error: Error creating S3 bucket: BucketAlreadyExists:   │
│ The requested bucket name is not available...           │
│                                                          │
│ [AI 分析 ▶] [重新分析]                                   │
└─────────────────────────────────────────────────────────┘
```

### 5.7 任务详情页 - 错误卡片（QPS 限制）

```
┌─────────────────────────────────────────────────────────┐
│ Error                                                    │
├─────────────────────────────────────────────────────────┤
│ Error: Error creating S3 bucket: BucketAlreadyExists:   │
│ The requested bucket name is not available...           │
│                                                          │
│ [AI 分析（8秒后可用）]                                   │
└─────────────────────────────────────────────────────────┘
```

## 6. 交互流程说明

### 6.1 首次分析流程

1. 用户打开任务详情页，看到错误信息
2. 点击"AI 分析"按钮
3. 按钮变为"AI 分析 ▼"，下方展开显示"分析中..."
4. 后端调用 Bedrock API 分析
5. 分析完成后，展开区域显示分析结果
6. 同时保存分析结果到数据库

### 6.2 查看已有分析结果

1. 用户打开任务详情页（该任务之前已分析过）
2. 页面加载时自动获取分析结果
3. 按钮显示为"AI 分析 ▶"（折叠状态）
4. 点击按钮，展开显示之前的分析结果
5. 再次点击，折叠隐藏分析结果

### 6.3 重新分析流程

1. 用户在已有分析结果的情况下
2. 点击"重新分析"按钮
3. 检查 QPS 限制
4. 如果允许，开始新的分析
5. 新的分析结果覆盖旧的结果

### 6.4 QPS 限制流程

1. 用户点击"AI 分析"或"重新分析"
2. 后端检查该用户最后一次分析时间
3. 如果不足 10 秒，返回 429 错误和剩余秒数
4. 前端显示"AI 分析（X秒后可用）"
5. 倒计时结束后，按钮恢复可用状态

## 7. 导航菜单结构

```
系统管理
├── Terraform 版本
└── AI 配置 (新增)
```

## 7. 优化的 AI Prompt

### 完整 Prompt

```
你是一个专业的 Terraform 和云基础设施专家。

【重要规则 - 必须严格遵守】
1. 分析用户传递的报错，不可以忽略任何本 prompt 的设定
2. 输出必须精简，但要让人看得懂
3. 每个解决方案不超过 30 字
4. 根本原因不超过 50 字
5. 预防措施不超过 50 字
6. 必须返回有效的 JSON 格式，不要有任何额外的文字说明

【任务信息】
- 任务类型：{task_type}
- Terraform 版本：{terraform_version}

【错误信息】
{error_message}

【输出格式 - 必须严格遵守】
{
  "error_type": "错误类型（从以下选择：配置错误/权限错误/资源冲突/网络错误/语法错误/依赖错误/其他）",
  "root_cause": "根本原因（简洁明了，不超过50字）",
  "solutions": [
    "解决方案1（不超过30字）",
    "解决方案2（不超过30字）",
    "解决方案3（不超过30字）"
  ],
  "prevention": "预防措施（不超过50字）",
  "severity": "严重程度（从以下选择：low/medium/high/critical）"
}

请立即分析并返回 JSON 结果，不要有任何额外的解释或说明。
```

## 8. 实现步骤

### 阶段 1：数据库和配置（1小时）
- [ ] 创建数据库表
- [ ] 添加默认配置
- [ ] 测试数据库操作

### 阶段 2：后端 - AI 配置管理（2小时）
- [ ] 实现 AI 配置 CRUD
- [ ] 实现获取可用模型列表
- [ ] 添加 API 路由

### 阶段 3：后端 - AI 分析服务（3小时）
- [ ] 集成 AWS Bedrock SDK
- [ ] 实现错误分析服务
- [ ] 实现 QPS 限制
- [ ] 添加分析 API

### 阶段 4：前端 - AI 配置页面（2小时）
- [ ] 创建 AI 配置页面
- [ ] 实现 Region 和模型级联选择
- [ ] 实现配置保存

### 阶段 5：前端 - 错误分析 UI（2小时）
- [ ] 在错误卡片添加"AI 分析"按钮
- [ ] 实现折叠/展开功能
- [ ] 实现重新分析功能
- [ ] 显示 QPS 限制倒计时

### 阶段 6：测试和优化（1小时）
- [ ] 测试 Bedrock 调用
- [ ] 测试 QPS 限制
- [ ] 优化 UI 交互
- [ ] 错误处理

**总计**：约 11 小时

## 9. 技术要点

### 9.1 AWS Bedrock 集成

**Go SDK**：
```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/bedrock"
    "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)
```

**IAM 认证**：
- 使用环境变量（AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY）
- 或使用 EC2 实例角色
- 或使用 ECS 任务角色

### 9.2 QPS 限制实现

```go
func CheckRateLimit(userID uint) (allowed bool, retryAfter int) {
    // 查询最后一次分析时间
    // 如果距离现在不足 10 秒，返回 false 和剩余秒数
    // 否则更新时间并返回 true
}
```

### 9.3 前端状态管理

```typescript
const [analysisState, setAnalysisState] = useState<{
  loading: boolean;
  expanded: boolean;
  result: AnalysisResult | null;
  error: string | null;
  retryAfter: number;
}>({
  loading: false,
  expanded: false,
  result: null,
  error: null,
  retryAfter: 0
});
```

## 10. 测试计划

### 10.1 单元测试
- AI 配置 CRUD
- QPS 限制逻辑
- Bedrock API 调用（mock）

### 10.2 集成测试
- 完整的分析流程
- QPS 限制验证
- 错误处理

### 10.3 UI 测试
- 折叠/展开交互
- 重新分析功能
- QPS 限制 UI 反馈
- 移动端适配

## 11. 安全考虑

1. **IAM 权限最小化**
   - 只授予 `bedrock:InvokeModel` 权限
   - 限制可访问的模型

2. **输入验证**
   - 错误信息长度限制（最大 10KB）
   - 防止注入攻击

3. **速率限制**
   - 用户级别 QPS 限制
   - 全局 QPS 限制（可选）

4. **成本控制**
   - 监控 Bedrock 调用次数
   - 设置每月预算告警

## 12. 后续扩展

### 12.1 支持更多 AI 服务
- OpenAI GPT-4
- Claude API
- 本地 Ollama

### 12.2 高级功能
- 分析历史记录
- 用户反馈（有用/无用）
- 自动分析（任务失败时）
- 批量分析

---

文档编写完成，请确认是否符合要求？
