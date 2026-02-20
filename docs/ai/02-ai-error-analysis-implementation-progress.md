# AI 错误分析功能实现进度

## 实施日期
2025-10-16

## 已完成工作

### Phase 1: 数据库和配置 
- [x] 创建数据库迁移脚本 `scripts/migrate_ai_error_analysis.sql`
- [x] 创建 `ai_configs` 表 - AI 配置
- [x] 创建 `ai_error_analyses` 表 - 分析结果
- [x] 创建 `ai_analysis_rate_limits` 表 - QPS 限制
- [x] 插入默认配置（Bedrock, us-east-1, Claude 3.5 Sonnet）
- [x] 执行数据库迁移

### Phase 2: 后端 - AI 配置管理 
- [x] 创建模型文件 `backend/internal/models/ai_config.go`
  - AIConfig - AI 配置模型
  - AIErrorAnalysis - 错误分析结果模型
  - AIAnalysisRateLimit - 速率限制模型
  - BedrockModel - Bedrock 模型信息
- [x] 创建配置服务 `backend/services/ai_config_service.go`
  - GetConfig() - 获取配置
  - UpdateConfig() - 更新配置
  - GetAvailableModels() - 获取可用模型列表
  - GetAvailableRegions() - 获取可用区域列表
  - SaveAnalysis() - 保存分析结果
  - GetAnalysis() - 获取分析结果
  - GetAnalysisWithSolutions() - 获取解析后的分析结果

### Phase 3: 后端 - AI 分析服务 
- [x] 创建分析服务 `backend/services/ai_analysis_service.go`
  - CheckRateLimit() - 检查速率限制（10秒/次）
  - UpdateRateLimit() - 更新速率限制记录
  - BuildPrompt() - 构建分析 prompt
  - AnalyzeError() - 分析错误（主函数）
  - callBedrock() - 调用 AWS Bedrock API
- [x] 集成 AWS Bedrock SDK
  - aws-sdk-go-v2/aws
  - aws-sdk-go-v2/config
  - aws-sdk-go-v2/service/bedrock
  - aws-sdk-go-v2/service/bedrockruntime
- [x] 实现 QPS 限制（每用户 10 秒 1 次）
- [x] 实现 Prompt 构建逻辑
  - 默认 prompt（不可修改）
  - 用户自定义 prompt 追加

### Phase 4: 后端 - AI 控制器和路由 
- [x] 创建控制器 `backend/controllers/ai_controller.go`
  - GetConfig() - GET /api/v1/admin/ai-config
  - UpdateConfig() - PUT /api/v1/admin/ai-config
  - GetAvailableModels() - GET /api/v1/admin/ai-config/models
  - GetAvailableRegions() - GET /api/v1/admin/ai-config/regions
  - AnalyzeError() - POST /api/v1/ai/analyze-error
  - GetTaskAnalysis() - GET /api/v1/workspaces/:id/tasks/:task_id/error-analysis
- [x] 添加 API 路由到 `backend/internal/router/router.go`
  - Admin 路由组：AI 配置管理
  - AI 路由组：错误分析
  - Workspaces 路由：获取任务分析结果

### 技术实现细节
- 使用 gorm.DB 依赖注入模式
- 遵循项目现有代码风格
- 实现完整的错误处理
- 支持 IAM 认证（AWS 凭证）
- JSON 格式的 API 响应
- 速率限制错误返回 429 状态码

### Phase 5: 前端 - AI 配置页面 
- [x] 创建 `/admin/ai-config` 页面
- [x] 实现 Region 下拉选择
- [x] 实现模型级联选择（根据 Region 加载）
- [x] 实现自定义 Prompt 输入框
- [x] 实现启用/禁用开关
- [x] 实现配置保存功能
- [x] 显示默认 Prompt（只读）
- [x] 添加使用说明
- [x] 创建 API 服务层 (`frontend/src/services/ai.ts`)
- [x] 创建页面组件 (`frontend/src/pages/AIConfig.tsx`)
- [x] 创建样式文件 (`frontend/src/pages/AIConfig.module.css`)
- [x] 添加路由配置
- [x] 添加导航菜单项（系统管理 > AI配置）

### Phase 6: 前端 - 错误分析 UI 
- [x] 创建 AIErrorAnalysis 组件 (`frontend/src/components/AIErrorAnalysis.tsx`)
- [x] 创建样式文件 (`frontend/src/components/AIErrorAnalysis.module.css`)
- [x] 在任务详情页错误卡片集成组件
- [x] 实现折叠/展开功能
- [x] 实现分析中状态显示（加载动画）
- [x] 实现分析结果显示
  - 错误类型
  - 根本原因
  - 解决方案列表
  - 预防措施
  - 严重程度
  - 分析耗时
- [x] 实现重新分析功能
- [x] 实现 QPS 限制倒计时显示
- [x] 页面加载时自动获取已有分析结果
- [x] 移动端适配
- [x] 仅在非取消状态显示 AI 分析

## 待完成工作

### Phase 7: 测试和优化（预计 1小时）
- [ ] 配置 AWS 凭证
- [ ] 测试 Bedrock API 调用
- [ ] 测试 QPS 限制功能
- [ ] 测试配置更新
- [ ] 测试模型列表获取
- [ ] 优化 UI 交互体验
- [ ] 错误处理测试
- [ ] 移动端测试

## API 端点总结

### 管理端点（需要认证）
```
GET    /api/v1/admin/ai-config           # 获取 AI 配置
PUT    /api/v1/admin/ai-config           # 更新 AI 配置
GET    /api/v1/admin/ai-config/regions   # 获取可用区域列表
GET    /api/v1/admin/ai-config/models    # 获取可用模型列表（需要 region 参数）
```

### 分析端点（需要认证）
```
POST   /api/v1/ai/analyze-error          # 分析错误
GET    /api/v1/workspaces/:id/tasks/:task_id/error-analysis  # 获取任务分析结果
```

## 数据库表

### ai_configs
- id, service_type, aws_region, model_id, custom_prompt, enabled
- 存储 AI 服务配置

### ai_error_analyses
- id, task_id, user_id, error_message, error_type, root_cause, solutions, prevention, severity, analysis_duration
- 存储分析结果，每个任务只保存最新结果（UNIQUE task_id）

### ai_analysis_rate_limits
- id, user_id, last_analysis_at
- 存储用户最后分析时间，用于 QPS 限制

## 依赖包
```
github.com/aws/aws-sdk-go-v2/aws
github.com/aws/aws-sdk-go-v2/config
github.com/aws/aws-sdk-go-v2/service/bedrock
github.com/aws/aws-sdk-go-v2/service/bedrockruntime
```

## 下一步行动
1. 开始实现前端 AI 配置页面
2. 实现任务详情页的 AI 分析 UI
3. 进行端到端测试

## 注意事项
- AWS Bedrock 需要配置 IAM 凭证
- 支持的区域：us-east-1, us-west-2, ap-southeast-1, ap-northeast-1, eu-central-1, eu-west-1
- 默认模型：anthropic.claude-3-5-sonnet-20240620-v1:0
- QPS 限制：每用户 10 秒 1 次分析
- 分析结果精简输出（根本原因≤50字，解决方案≤30字）
