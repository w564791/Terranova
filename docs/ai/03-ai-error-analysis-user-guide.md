# AI 错误分析功能使用指南

## 功能概述

AI 错误分析功能基于 AWS Bedrock 的 Claude 3.5 Sonnet 模型，为 Terraform 任务执行失败提供智能分析，帮助用户快速定位问题原因并获得解决方案。

## 前置条件

### 1. AWS Bedrock 访问权限

需要配置 AWS 凭证，有以下几种方式：

**方式 1：环境变量**
```bash
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_REGION="us-east-1"
```

**方式 2：AWS 配置文件**
```bash
aws configure
```

**方式 3：IAM 角色（推荐用于生产环境）**
- EC2 实例角色
- ECS 任务角色
- EKS Pod 角色

### 2. IAM 权限要求

确保 IAM 用户或角色具有以下权限：

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "bedrock:ListFoundationModels",
        "bedrock:InvokeModel"
      ],
      "Resource": "*"
    }
  ]
}
```

## 配置步骤

### 1. 访问 AI 配置页面

1. 登录系统
2. 点击左侧导航栏"系统管理"
3. 点击"AI配置"（🤖 图标）
4. 进入 `/admin/ai-config` 页面

### 2. 配置 AI 服务

**步骤 1：选择 AWS Region**
- 从下拉列表选择 AWS 区域
- 支持的区域：
  - us-east-1（美国东部）
  - us-west-2（美国西部）
  - ap-southeast-1（新加坡）
  - ap-northeast-1（东京）
  - eu-central-1（法兰克福）
  - eu-west-1（爱尔兰）

**步骤 2：选择 AI 模型**
- 选择 Region 后，系统会自动加载该区域可用的模型
- 推荐模型：
  - Claude 3.5 Sonnet（默认）- 最佳性能和准确度
  - Claude 3 Haiku - 更快速度
  - Titan Text Premier - AWS 原生模型

**步骤 3：自定义 Prompt（可选）**
- 可以添加额外的分析要求
- 例如："额外关注 AWS 中国区域的特殊配置要求"
- 此内容会追加到默认 Prompt 之后

**步骤 4：启用服务**
- 勾选"启用 AI 分析"复选框
- 点击"保存配置"按钮

## 使用 AI 分析

### 1. 查看失败的任务

1. 进入工作空间详情页
2. 点击"Runs"标签
3. 选择一个失败的任务（状态为"Errored"）

### 2. 执行 AI 分析

在任务详情页的错误卡片中：

**首次分析**
1. 点击"AI 分析"按钮
2. 等待分析完成（通常 2-5 秒）
3. 查看分析结果

**查看已有分析**
1. 如果之前已分析过，按钮显示为"AI 分析 ▶"
2. 点击展开查看之前的分析结果
3. 再次点击可折叠隐藏

**重新分析**
1. 点击"重新分析"按钮
2. 受 QPS 限制（10 秒/次）
3. 如果限制未到期，会显示倒计时

### 3. 分析结果说明

AI 分析结果包含以下信息：

**错误类型**
- 配置错误
- 权限错误
- 资源冲突
- 网络错误
- 语法错误
- 依赖错误
- 其他

**根本原因**
- 简洁明了的问题描述（≤50字）
- 直指问题核心

**解决方案**
- 3 个具体的解决方案
- 每个方案≤30字
- 按优先级排序

**预防措施**
- 如何避免类似问题（≤50字）
- 最佳实践建议

**严重程度**
- 严重（Critical）- 红色
- 高（High）- 橙色
- 中等（Medium）- 黄色
- 低（Low）- 绿色

**分析耗时**
- 显示 AI 分析所用时间

## 使用限制

### QPS 限制
- 每个用户 10 秒限制 1 次分析
- 超过限制会显示倒计时
- 倒计时结束后可继续使用

### 结果存储
- 每个任务只保存最新的分析结果
- 重新分析会覆盖之前的结果
- 历史任务的分析结果会一直保留

### 适用范围
- 仅在任务失败时显示 AI 分析按钮
- 取消的任务不显示 AI 分析
- 需要先在配置页面启用 AI 服务

## 使用场景示例

### 场景 1：S3 存储桶名称冲突

**错误信息**
```
Error: Error creating S3 bucket: BucketAlreadyExists: 
The requested bucket name is not available...
```

**AI 分析结果**
- 错误类型：资源冲突
- 根本原因：S3 存储桶名称已被占用，S3 存储桶名称在全球范围内必须唯一
- 解决方案：
  1. 修改存储桶名称，添加唯一后缀
  2. 检查是否在其他账户中创建了同名存储桶
  3. 使用 terraform import 导入已存在的存储桶
- 预防措施：使用变量或随机字符串生成唯一的存储桶名称
- 严重程度：中等

### 场景 2：IAM 权限不足

**错误信息**
```
Error: Error creating EC2 instance: UnauthorizedOperation: 
You are not authorized to perform this operation...
```

**AI 分析结果**
- 错误类型：权限错误
- 根本原因：当前 IAM 用户或角色缺少创建 EC2 实例的权限
- 解决方案：
  1. 为 IAM 用户添加 ec2:RunInstances 权限
  2. 检查 IAM 策略是否正确附加
  3. 确认是否有资源级别的权限限制
- 预防措施：使用最小权限原则，提前规划 IAM 策略
- 严重程度：高

### 场景 3：网络配置错误

**错误信息**
```
Error: Error creating RDS instance: InvalidVPCNetworkStateFault: 
The DB subnet group doesn't meet Availability Zone coverage requirement...
```

**AI 分析结果**
- 错误类型：配置错误
- 根本原因：数据库子网组未覆盖足够的可用区
- 解决方案：
  1. 在子网组中添加更多可用区的子网
  2. 确保至少有 2 个不同可用区的子网
  3. 检查 VPC 和子网配置是否正确
- 预防措施：创建子网组时确保覆盖多个可用区
- 严重程度：中等

## 最佳实践

### 1. 配置建议
- 使用默认的 Claude 3.5 Sonnet 模型（最佳效果）
- 根据实际需求添加自定义 Prompt
- 定期检查 AWS 凭证是否有效

### 2. 使用建议
- 任务失败后立即使用 AI 分析
- 结合分析结果和实际日志排查问题
- 将有用的解决方案记录到文档中

### 3. 成本控制
- 每次分析会调用 Bedrock API（产生费用）
- 利用 QPS 限制避免频繁调用
- 分析结果会保存，避免重复分析

## 故障排查

### 问题 1：AI 分析按钮不显示
**原因**：
- AI 服务未启用
- 任务状态不是失败状态
- 任务被取消

**解决**：
- 检查 `/admin/ai-config` 页面是否启用
- 确认任务状态为"Errored"

### 问题 2：分析失败
**可能原因**：
- AWS 凭证未配置或已过期
- 没有 Bedrock 访问权限
- 选择的 Region 不支持所选模型
- 网络连接问题

**解决**：
- 检查 AWS 凭证配置
- 验证 IAM 权限
- 尝试更换 Region 或模型
- 检查网络连接

### 问题 3：QPS 限制
**现象**：
- 显示"AI 分析（X秒后可用）"
- 无法立即重新分析

**说明**：
- 这是正常的速率限制
- 每个用户 10 秒限制 1 次分析
- 等待倒计时结束即可

### 问题 4：模型列表加载失败
**可能原因**：
- AWS 凭证问题
- 选择的 Region 不支持 Bedrock
- 网络问题

**解决**：
- 检查 AWS 凭证
- 尝试其他 Region
- 查看后端日志

## 技术细节

### API 端点
```
# 配置管理
GET    /api/v1/admin/ai-config
PUT    /api/v1/admin/ai-config
GET    /api/v1/admin/ai-config/regions
GET    /api/v1/admin/ai-config/models?region=us-east-1

# 错误分析
POST   /api/v1/ai/analyze-error
GET    /api/v1/workspaces/:id/tasks/:task_id/error-analysis
```

### 数据存储
- 配置存储在 `ai_configs` 表
- 分析结果存储在 `ai_error_analyses` 表
- QPS 限制记录在 `ai_analysis_rate_limits` 表

### Prompt 设计
- 默认 Prompt 经过优化，确保输出精简
- 强制要求 JSON 格式输出
- 限制字数确保快速阅读

## 常见问题

**Q: AI 分析准确吗？**
A: AI 分析基于 Claude 3.5 Sonnet 模型，准确度较高，但仍需结合实际情况判断。建议将 AI 分析作为参考，而非唯一依据。

**Q: 可以修改默认 Prompt 吗？**
A: 默认 Prompt 不可修改，但可以添加自定义 Prompt 作为补充。

**Q: 分析结果会保存多久？**
A: 分析结果会永久保存，直到任务被删除。

**Q: 可以批量分析多个任务吗？**
A: 当前版本不支持批量分析，需要逐个任务分析。

**Q: 支持其他 AI 服务吗？**
A: 当前版本仅支持 AWS Bedrock，未来可能支持 OpenAI、Claude API 等。

**Q: 分析费用如何计算？**
A: 费用由 AWS Bedrock 收取，按 API 调用次数和 token 数量计费。具体费用请参考 AWS Bedrock 定价页面。

## 更新日志

### v1.0.0 (2025-10-16)
- 初始版本发布
- 支持 AWS Bedrock Claude 3.5 Sonnet
- 实现 QPS 限制（10秒/次）
- 支持自定义 Prompt
- 完整的前后端实现
- 移动端适配

## 反馈和支持

如有问题或建议，请联系系统管理员。
