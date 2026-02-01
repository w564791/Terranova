# IaC平台 (Infrastructure as Code Platform)

一个企业级的基础设施即代码管理平台，提供完整的Terraform工作流管理、多租户权限控制、智能表单渲染和资源可视化能力。

## 🎯 核心特色

### 🤖 AI驱动的智能化
- **AI Schema生成**: 自动解析Terraform Module生成OpenAPI Schema
- **AI错误分析**: 基于AWS Bedrock Claude 3.5 Sonnet的智能错误诊断
- **0门槛操作**: 完全屏蔽HCL语法，纯表单化操作

### 🏢 企业级多租户架构
- **三层权限模型**: Organization → Project → Workspace
- **细粒度权限控制**: READ/WRITE/ADMIN三级权限，支持团队授权
- **临时权限系统**: 基于Webhook的任务级临时授权
- **完整审计日志**: 所有权限变更和资源访问可追溯

### 🔄 灵活的执行模式
- **Server模式**: 平台直接执行，适合开发测试
- **Agent模式**: 独立Agent执行，安全隔离，适合生产环境
- **K8s模式**: 动态Pod执行，弹性伸缩，适合云原生环境

### 📝 强大的表单系统
- **OpenAPI驱动**: 基于OpenAPI 3.1标准的Schema定义
- **12种Widget类型**: 支持复杂对象、动态键、JSON编辑等
- **无限嵌套**: 支持复杂对象的深度嵌套表单渲染
- **级联规则**: 字段间智能联动，动态显示/隐藏/禁用
- **外部数据源**: 支持从API动态加载选项数据

### 🔍 资源管理与可视化
- **CMDB树状结构**: 按workspace → module → resource层级展示
- **全局资源搜索**: 支持按ID、名称快速定位资源
- **资源版本控制**: 完整的资源变更历史追踪
- **State管理**: State版本控制、锁机制、差异对比

### 🛡️ 安全与合规
- **敏感数据保护**: State敏感数据加密存储
- **资源锁定**: 防止并发修改冲突
- **VCS集成**: 支持GitHub/GitLab等主流版本控制系统
- **Run Tasks集成**: 内置安全、成本、合规检查

## 🏗️ 技术架构

```
┌─────────────────────────────────────────────────────────────┐
│                        前端层 (React)                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ 动态表单渲染  │  │ 资源可视化   │  │ 权限管理     │      │
│  │ OpenAPI驱动  │  │ CMDB树状图   │  │ IAM控制台    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                            ↕ REST API / WebSocket
┌─────────────────────────────────────────────────────────────┐
│                      后端层 (Golang)                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Workspace    │  │ IAM权限      │  │ Agent管理    │      │
│  │ 生命周期管理  │  │ 三层模型     │  │ 任务调度     │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Schema管理   │  │ State管理    │  │ CMDB索引     │      │
│  │ AI解析引擎   │  │ 版本控制     │  │ 资源搜索     │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                            ↕
┌─────────────────────────────────────────────────────────────┐
│                    执行层 (Terraform)                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Local执行器  │  │ Agent执行器  │  │ K8s执行器    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                            ↕
┌─────────────────────────────────────────────────────────────┐
│                      存储层                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ PostgreSQL   │  │ S3/OSS       │  │ Redis        │      │
│  │ 元数据存储   │  │ State存储    │  │ 缓存/队列    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

## 🛠️ 技术栈

### 后端
- **语言**: Go 1.21+
- **框架**: Gin
- **ORM**: GORM
- **数据库**: PostgreSQL 15+
- **缓存**: Redis
- **AI集成**: AWS Bedrock (Claude 3.5 Sonnet)
- **Terraform**: terraform-exec

### 前端
- **框架**: React 18+ with TypeScript
- **UI库**: Ant Design
- **状态管理**: Redux Toolkit
- **表单处理**: React Hook Form
- **构建工具**: Vite

### 基础设施
- **容器化**: Docker + Docker Compose
- **编排**: Kubernetes (可选)
- **CI/CD**: GitHub Actions
- **监控**: Prometheus + Grafana

## 📚 核心功能模块

### 1. 工作空间管理 (Workspace)
- **生命周期管理**: Plan → Apply → Destroy完整流程
- **多执行模式**: Server/Agent/K8s三种模式
- **任务队列**: 并发控制、优先级调度
- **State管理**: 版本控制、锁机制、回滚能力
- **变量管理**: 支持敏感数据加密存储
- **通知系统**: Webhook集成、事件驱动

### 2. 权限系统 (IAM)
- **三层模型**: Organization → Project → Workspace
- **主体类型**: User、Team、Application
- **权限等级**: NONE(拒绝) / READ / WRITE / ADMIN
- **权限继承**: 上层权限自动向下继承
- **临时权限**: 基于Webhook的任务级授权
- **审计日志**: 完整的权限变更和访问记录

### 3. 模块管理 (Module)
- **Schema驱动**: 基于OpenAPI 3.1标准
- **AI解析**: 自动从Terraform代码生成Schema
- **动态表单**: 12种Widget类型，支持复杂嵌套
- **级联规则**: 字段间智能联动
- **外部数据源**: 动态加载选项数据
- **版本管理**: Module版本控制和更新通知

### 4. Agent架构
- **多模式支持**: Server/Agent/K8s
- **负载均衡**: 最少任务数选择策略
- **健康检查**: 心跳监控、自动故障恢复
- **安全隔离**: Token认证、权限控制
- **弹性伸缩**: K8s模式支持动态扩缩容

### 5. CMDB资源管理
- **树状结构**: workspace → module → resource层级展示
- **全局搜索**: 按ID、名称快速定位资源
- **资源索引**: 自动解析State构建资源索引
- **智能命名**: 多策略提取云资源名称
- **跨workspace搜索**: 支持全局资源查找

### 6. Manifest编排
- **可视化编排**: 拖拽式模块连线
- **依赖管理**: 自动处理模块间依赖关系
- **输出引用**: 支持模块输出的引用传递
- **版本控制**: Manifest版本管理
- **部署管理**: 统一部署多个关联资源

### 7. 安全特性
- **State加密**: 敏感数据加密存储
- **权限控制**: 细粒度的资源访问控制
- **审计日志**: 完整的操作审计追踪
- **资源锁定**: 防止并发修改冲突
- **临时授权**: 基于时间的临时权限

## 🚀 快速开始

### 环境要求
- Go 1.21+
- Node.js 18+
- PostgreSQL 15+
- Redis 6+
- Docker & Docker Compose

### 本地开发

1. **克隆项目**
```bash
git clone <repository-url>
cd iac-platform
```

2. **启动依赖服务**
```bash
docker-compose up -d postgres redis
```

3. **初始化数据库**
```bash
psql -h localhost -U postgres -d iac_platform -f docs/12-database-schema.sql
```

4. **启动后端**
```bash
cd backend
go mod tidy
go run main.go
```

5. **启动前端**
```bash
cd frontend
npm install
npm run dev
```

6. **访问应用**
- 前端: http://localhost:3000
- 后端API: http://localhost:8080
- API文档: http://localhost:8080/swagger/index.html

## 📋 主要功能特性

### 动态表单系统
- ✅ 基于OpenAPI 3.1 Schema的表单渲染
- ✅ 12种Widget类型（text, number, select, switch, tags, key-value, object, object-list, dynamic-object, json-editor, password, datetime, code-editor）
- ✅ 支持无限嵌套的复杂对象
- ✅ 级联规则引擎（字段联动）
- ✅ 外部数据源集成（动态选项加载）
- ✅ 跨字段验证规则
- ✅ tf2openapi转换工具

### 工作空间功能
- ✅ Plan/Apply/Destroy完整流程
- ✅ 三种执行模式（Server/Agent/K8s）
- ✅ State版本控制和回滚
- ✅ 变量管理（支持敏感数据）
- ✅ 任务队列和并发控制
- ✅ WebSocket实时状态推送
- ✅ 结构化日志输出

### 权限系统
- ✅ 三层权限模型（Organization/Project/Workspace）
- ✅ 团队管理和成员授权
- ✅ 细粒度权限控制（READ/WRITE/ADMIN）
- ✅ 临时权限系统（基于Webhook）
- ✅ 完整审计日志
- ✅ 权限继承机制

### CMDB功能
- ✅ 资源树状结构展示
- ✅ 全局资源搜索
- ✅ 自动State解析和索引
- ✅ 智能资源命名
- ✅ 跨workspace资源查找

### AI功能
- ✅ AI错误分析（基于Claude 3.5 Sonnet）
- ✅ 智能根因分析和解决方案
- ✅ QPS限制保护
- ✅ 自定义分析Prompt

## 🔧 开发指南

### 开始开发前必读
1. 阅读 `docs/01-QUICK_START_FOR_AI.md` 了解项目概览
2. 查看 `docs/workspace/00-overview.md` 了解核心架构
3. 参考 `docs/iam/02-iac-platform-permission-system-design.md` 了解权限系统
4. 查看 `docs/module/openapi-schema-design.md` 了解表单系统

### 重要文档
- **Workspace模块**: `docs/workspace/` - 工作空间完整设计
- **IAM权限**: `docs/iam/` - 权限系统设计和实现
- **Module系统**: `docs/module/` - 模块和表单系统
- **Agent架构**: `docs/agent/` - Agent执行模式设计
- **CMDB功能**: `docs/cmdb/` - 资源管理和搜索
- **安全特性**: `docs/security/` - 安全相关设计

### 开发流程
1. **需求确认**: 检查相关设计文档，避免重复开发
2. **接口设计**: 遵循API规范，保持一致性
3. **数据库设计**: 使用已定义的表结构
4. **最小化实现**: 只实现当前任务必需的功能
5. **测试验证**: 确保功能正常工作
6. **文档更新**: 更新相关文档

## 📊 项目状态

**当前版本**: v1.0.0  
**开发状态**: 核心功能已完成，持续优化中

### ✅ 已完成功能

#### 工作空间管理
- ✅ Workspace CRUD操作
- ✅ Plan/Apply/Destroy完整流程
- ✅ 三种执行模式（Server/Agent/K8s）
- ✅ 任务队列和并发控制
- ✅ WebSocket实时状态推送
- ✅ 结构化日志输出
- ✅ 任务触发器（Run Triggers）
- ✅ Provider配置管理

#### State管理
- ✅ State版本控制和历史
- ✅ State上传和回滚
- ✅ State预览和搜索
- ✅ State敏感数据保护
- ✅ Workspace锁定机制

#### 资源管理
- ✅ 资源CRUD操作
- ✅ 资源版本控制
- ✅ 资源快照和恢复
- ✅ 资源依赖管理
- ✅ 资源编辑协作（锁定/心跳/接管）
- ✅ 资源漂移检测

#### 模块系统
- ✅ Module CRUD操作
- ✅ OpenAPI Schema管理（V2）
- ✅ Schema解析服务（tf2openapi）
- ✅ 动态表单渲染（12种Widget）
- ✅ Module Demo管理
- ✅ Demo版本控制

#### 表单系统
- ✅ 基于OpenAPI 3.1的表单渲染
- ✅ 12种Widget类型（text, number, select, switch, tags, key-value, object, object-list, dynamic-object, json-editor, password, datetime, code-editor）
- ✅ 级联规则引擎
- ✅ 外部数据源支持
- ✅ 跨字段验证

#### Agent架构
- ✅ Agent注册和心跳
- ✅ Agent Pool管理
- ✅ 任务锁定和续期
- ✅ K8s Job执行模式
- ✅ Agent清理服务
- ✅ Pool Token管理

#### CMDB功能
- ✅ 资源索引自动同步
- ✅ 资源树状结构展示
- ✅ 全局资源搜索
- ✅ Module层级解析
- ✅ 智能资源命名

#### IAM权限系统
- ✅ Organization管理
- ✅ Project管理
- ✅ Team管理
- ✅ User管理
- ✅ Role管理
- ✅ 权限授予和撤销
- ✅ Application管理
- ✅ 审计日志

#### AI功能
- ✅ AI配置管理（多配置支持）
- ✅ AI错误分析（Bedrock/OpenAI Compatible）
- ✅ 分析结果存储和查询
- ✅ QPS限制保护
- ✅ 可用模型和区域查询

#### Manifest编排
- ✅ Manifest CRUD操作
- ✅ 可视化编排器
- ✅ Module连线和依赖
- ✅ Manifest部署管理
- ✅ 部署资源追踪

#### 其他功能
- ✅ 变量管理（版本控制）
- ✅ Workspace Outputs管理
- ✅ Run Tasks集成
- ✅ 通知配置管理
- ✅ Terraform版本管理
- ✅ 平台配置管理
- ✅ Swagger API文档

#### Run Task集成
- ✅ Run Task全局管理
- ✅ Workspace Run Task配置
- ✅ 四个执行阶段（Pre-plan/Post-plan/Pre-apply/Post-apply）
- ✅ 执行级别控制（Advisory/Mandatory）
- ✅ 回调机制和超时处理
- ✅ 一次性Access Token
- ✅ HMAC签名验证
- ✅ Run Task结果展示

#### 通知系统
- ✅ 通知配置管理（全局和Workspace级别）
- ✅ 两种通知类型（Webhook/Lark Robot）
- ✅ 10种触发事件（task_created/planned/completed/failed等）
- ✅ 事件驱动通知发送
- ✅ HMAC签名和Lark签名
- ✅ 通知日志和重试机制
- ✅ 全局通知自动应用

#### Secrets管理
- ✅ 通用Secrets存储表
- ✅ 多资源类型支持（Agent Pool/Workspace/Module/System）
- ✅ AES-256-GCM加密
- ✅ 访问审计和使用追踪
- ✅ 过期管理和轮转策略

#### 其他已完成功能
- ✅ Run Triggers（任务触发器）
- ✅ 任务评论系统
- ✅ 资源锁定和编辑协作
- ✅ 接管请求机制
- ✅ Pool Token管理
- ✅ Login Session管理
- ✅ User/Team Token管理
- ✅ Remote Data集成
- ✅ VCS Provider配置

### ❌ 未实现功能（仅设计）

#### 高级特性
- ❌ Drift自动检测调度（手动检测已实现）
- ❌ AI驱动的自动修复建议
- ❌ 成本预测分析
- ❌ GitOps完整集成（VCS集成已部分实现）
- ❌ Workspace Template
- ❌ 多租户完整隔离（数据库层面）

### 🎯 下一步计划
1. 实现Drift自动检测调度
2. 增强AI自动修复建议
3. 完善GitOps集成
4. 实现成本预测分析
5. 优化性能和监控

## 🤝 贡献指南

### 开发规范
- **Go**: 遵循Go官方代码规范，使用gofmt格式化
- **TypeScript**: 使用ESLint + Prettier，严格类型检查
- **提交规范**: 使用Conventional Commits
- **测试要求**: 单元测试覆盖率>80%

### 提交流程
1. Fork项目并创建功能分支
2. 按照开发指南进行开发
3. 确保测试通过和代码格式正确
4. 提交Pull Request并描述变更内容
5. 等待代码review和合并

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情

## 📞 联系方式

- 项目维护者: [Your Name]
- 邮箱: [your.email@example.com]
- 问题反馈: [GitHub Issues](https://github.com/your-org/iac-platform/issues)

---

**注意**: 这是一个企业级IaC管理平台，所有核心功能已完成设计和实现。开发时请严格按照文档规范进行，确保系统的稳定性和安全性。
