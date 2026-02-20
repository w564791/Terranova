# IaC Platform (Infrastructure as Code Platform)

<p align="center">
  <img src="docs/iac-platform.svg" alt="IaC Platform Logo" width="200"/>
</p>

<p align="center">
  <strong>企业级基础设施即代码管理平台</strong><br>
  AI驱动 · 表单化操作 · 多租户架构 · 全流程管控
</p>

<p align="center">
  <a href="#-部署">部署</a> •
  <a href="#-核心亮点">核心亮点</a> •
  <a href="#-功能特性">功能特性</a> •
  <a href="#-本地开发">本地开发</a> •
  <a href="#-技术架构">技术架构</a> •
  <a href="#-文档">文档</a>
</p>

---

## 🚀 部署

| 方式 | 适用场景 | 说明 |
|------|---------|------|
| **[Docker Compose 快速部署](docker-compose.example.yml)** | POC / 演示 / 评估 | 单机运行，无需 K8s，最快体验平台全部功能 |
| **[Kubernetes 生产部署](manifests/README.md)** | 生产环境 | TLS 加密、HA 高可用、网络策略、OPA 安全策略 |

**快速体验推荐 Docker Compose**，仅需 Docker 环境即可启动：

```bash
# 复制配置文件
cp docker-compose.example.yml docker-compose.yml

# 启动（前台运行，Ctrl+C 停止）
docker compose up

# 或后台运行
docker compose up -d
```

启动后访问：http://localhost

---

## 🌟 核心亮点

### 🖥️ UI 界面管理
- **完全脱离命令行**: 纯 Web 界面操作，无需 HCL 语法知识
- **全局搜索支持**: 所有功能模块均支持搜索，快速定位资源
- **表单化操作**: 基于 OpenAPI Schema 的智能表单渲染

### 👥 角色分离设计
| 角色 | 职责 |
|------|------|
| **平台管理员** | 全局配置、Agent 管理、系统设置 |
| **策略管理员** | Run Tasks、安全策略、合规检查 |
| **高级工程师** | Module 设计、Schema 定义、Skill 配置 |
| **交付工程师** | Workspace 管理、资源部署、日常运维 |

---

## 📋 功能特性

### 🔧 全局设置

#### Agent 管理
- **调度与应急解冻**: 适配非活跃网段的自动关机能力
- **活跃度监控**: 实时查看 Agent 状态、当前运行任务 ID
- **认证轮转**: 支持 Agent 认证凭据自动轮转
- **自动扩容**: 支持 K8S 环境自动扩缩容
- **白名单策略**: Agent 支持白名单访问控制
- **自定义模板**: 支持自定义 Agent 部署模板

#### IaC Engine
- **双引擎支持**: 同时支持 OpenTofu 和 Terraform
- **版本管理**: 
  - 全局默认版本配置
  - 多版本并行支持
  - Workspace 级别灵活配置

#### Run Tasks
- **四阶段 Hook**: Pre-plan / Post-plan / Pre-apply / Post-apply
- **多场景支持**: 代码安全审查、合规检查、成本预估等
- **审批流程**: 单次授权审批流（规划中）
- **执行模式**: 
  - **强制模式**: 检查失败直接终止任务
  - **非强制模式**: 仅告警不阻断

#### 全局通知
支持以下事件类型的通知推送：
- `Completed` - 任务完成
- `Created` - 任务创建
- `task_planning` - 计划中
- `task_planned` - 计划完成
- `task_applying` - 应用中
- `Failed` - 任务失败
- `Cancelled` - 任务取消
- `Approval` - 审批请求
- `Drift` - 漂移检测

---

### 🤖 AI 能力

#### 错误分析
- 针对主流程错误信息提供 AI 智能分析
- 基于 AWS Bedrock Claude 3.5 Sonnet
- 自动生成根因分析和解决方案

#### Module Skill 系统
- **自动生成**: 使用内置 Module Skill Generator
- **OpenAPI V3 驱动**: 基于 OpenAPI V3 自动生成 Module Skill
- **Skill 使用流程**:
  ```
  Module → 自动生成 Schema Skill
      ↓
  用户需求 → 需求安全断言
      ↓
  AI 判断是否需要查询 CMDB ←→ AI 判断必要的 Skill 组装（基线 Skill 不可跳过）
      ↓
  并行查询 CMDB
      ↓
  Task Skill 加载 Module Skill
      ↓
  LLM 生成候选参数
      ↓
  平台 SchemaSolver 做最终裁决（待实施）
  ```

#### 向量化 CMDB
- AI 检索向量化 CMDB 数据
- 支持自然语言资源查询
- 更好地支持资源生成数据源

#### 安全断言
- 所有大模型调用前进行安全判定
- AI 权限继承自用户权限
- 防止越权操作

#### 表单资源生成
- 基于自然语言需求查找 CMDB
- 根据 Module Skill 自动生成资源配置
- 配合 Schema 做为生成规范

---

### 🔐 RBAC 权限系统

#### 角色管理
- **内建角色**: 预置标准角色模板
- **自定义角色**: 支持创建自定义角色
- **用户组**: 支持 Group 管理
- **审计日志**: 完整的权限变更追踪

#### Token 管理
| Token 类型 | 特性 |
|-----------|------|
| **个人 Token** | 仅登录状态有效，登出失效，再次登录恢复 |
| **团队 Token** | 无登录状态要求，持久有效 |

---

### 📦 Module 管理

#### 版本化 Demo 和 Schema
- 每个版本独立的 Demo 和 Schema
- 支持版本继承
- Terraform 版本级别管理

#### Schema 系统（核心亮点）
- **表单化提交**: 支持 Form 形式的 Terraform 提交
- **参数分组**: 支持 Group 组织参数
- **参数关联规则**:
  - 互斥关系：XX 存在则 BB 不能存在
  - 依赖关系：A 存在则 B 必须存在
  - 支持 List 类型参数值

#### 值填充来源
| 来源 | 说明 |
|------|------|
| **CMDB** | 参数显示 CMDB 样式提示 |
| **Workspace Output** | 使用 `/` 快速呼出菜单 |
| **远程 Workspace** | 支持配置的远程 Workspace 调用 |

#### AI 辅助
- AI 生成代码
- 自动 Form 表单生成
- 错误自动修复

#### OpenAPI V3 可视化编辑
- 自动解析 Terraform Variables
- 提供 Schema 编辑能力
- 可视化配置界面

#### 提示词管理
- 新增提示词 CRUD 功能
- 展示在 AI 助手呼出界面
- 教用户如何使用 Module

#### Claude Skill 能力下放
- Task 层交给 Module 维护者
- 提供 AI 生成 Module Skill 能力

---

### 🗄️ CMDB 资源管理

#### 自动同步
- 每次 Apply（无论成功与否）自动更新内置 CMDB
- 仅保留必要字段（ARN、Name、Tag 等）
- 轻量化设计
- 支持手动 Sync

#### 外置 CMDB
- 支持外置 CMDB 同步
- 多数据源集成

#### 数据结构
- **树状结构**: 层级化资源展示
- **向量化数据**: 支持自然语言资源生成数据源

---

### 🏠 Workspace 管理

#### 纵览界面
- 最近一次任务状态
- Drift 缩略信息
- 最近添加的资源列表

#### Run 列表
- 历史运行列表展示
- 任务状态追踪

#### Run Details
- 历史运行详细信息
- Structure 变更内容展示
- **日志查看**:
  - Classic 日志模式
  - 分进度查看
  - 错误自动弹出 AI 分析按钮

#### State 管理
- **卡片形式归类查看**: 类似资源列表展示
- **详细信息展开**: 类似 `tf state show` 的详细视图
- **JSON 原始数据**: 支持原始数据查看
- **安全机制**: 
  - 进入详情页需显式 Retrieve
  - 需要额外 IAM 权限
- **版本管理**: State 版本控制
- **导入校验**: State 导入验证能力

#### 资源管理
| 功能 | 说明 |
|------|------|
| **查看模式** | 表单模式 / JSON 模式，支持来回切换 |
| **变更方式** | 表单变更 / JSON 变更 |
| **版本控制** | 单资源版本控制，支持快速回滚 |
| **软删除** | 支持资源软删除 |
| **版本对比** | 资源版本差异对比 |
| **AI 辅助** | 支持 AI 新增和编辑 |
| **导出能力** | 资源配置导出 |
| **资源锁** | 多人编辑时可申请锁，申请者可选择是否同意 |
| **草稿机制** | 编辑时支持草稿，落后版本提示 |

#### 变量管理
- **版本管理**: 每个任务运行快照当前变量
- **API 支持**: 支持提交任意变量版本
- **串行任务流**: 支持任务流编排
- **变量类型**: Terraform 变量 + 系统变量
- **敏感变量**: 支持 Sensitive 变量加密

#### Output 管理
- 链接其它 Workspace 的 Output
- 白名单配置
- 支持全局允许
- 支持 Static Output

#### Drift 检测
- **后台静默检查**: 资源漂移自动检测
- **触发方式**: 手动 / 自动
- **自动周期**: 支持自定义检测周期

#### Workspace 配置
| 配置项 | 说明 |
|--------|------|
| **Workspace Lock** | 工作空间锁定 |
| **Provider 版本限制** | Workspace 级别 Provider 版本控制 |
| **Run Task 配置** | 全局 Run Task 不可单方面禁用 |
| **Run Trigger** | 被上游 Workspace 触发的能力 |
| **通知功能** | Workspace 级别通知配置 |

#### 执行引擎版本管理
- 平台级别 Terraform/OpenTofu 版本管理
- 多版本并行支持

#### 🎯 里程碑
> 为 AI 创建资源时，打通了 CMDB 和 AI，AI 继承用户权限，支持以自然语言方式生成配置，配合 Schema 做为生成规范，以更安全/合规的方式提供标准化的资源创建。

---

### 📐 Manifests 编排

#### 可视化编辑
- **拖拽编辑**: 拖拽式模块编排
- **关联关系查看**: 可视化依赖关系

#### 完整能力继承
- 支持 Workspace 里的 Form/JSON 所有能力
- 包含 AI 辅助功能

#### 多次部署
- 支持 Manifest 多次部署
- 部署历史追踪

---

## 🏗️ 技术架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           前端层 (React + TypeScript)                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │ 动态表单渲染  │  │ 资源可视化   │  │ 权限管理     │  │ AI 助手      │ │
│  │ OpenAPI驱动  │  │ CMDB树状图   │  │ IAM控制台    │  │ Skill系统    │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
                              ↕ REST API / WebSocket
┌─────────────────────────────────────────────────────────────────────────┐
│                           后端层 (Golang + Gin)                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │ Workspace    │  │ IAM权限      │  │ Agent管理    │  │ AI Engine    │ │
│  │ 生命周期管理  │  │ RBAC模型     │  │ 任务调度     │  │ Skill生成    │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘ │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │ Schema管理   │  │ State管理    │  │ CMDB索引     │  │ Run Tasks    │ │
│  │ OpenAPI解析  │  │ 版本控制     │  │ 向量搜索     │  │ Hook系统     │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
                              ↕
┌─────────────────────────────────────────────────────────────────────────┐
│                        执行层 (Terraform/OpenTofu)                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                   │
│  │ Server执行器 │  │ Agent执行器  │  │ K8s执行器    │                   │
│  │ 本地执行     │  │ 远程隔离     │  │ 弹性伸缩     │                   │
│  └──────────────┘  └──────────────┘  └──────────────┘                   │
└─────────────────────────────────────────────────────────────────────────┘
                              ↕
┌─────────────────────────────────────────────────────────────────────────┐
│                             存储层                                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │ PostgreSQL   │  │ S3/OSS       │  │ Redis        │  │ Vector DB    │ │
│  │ 元数据存储   │  │ State存储    │  │ 缓存/队列    │  │ 向量索引     │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 🛠️ 技术栈

### 后端
| 技术 | 版本/说明 |
|------|----------|
| **语言** | Go 1.21+ |
| **框架** | Gin |
| **ORM** | GORM |
| **数据库** | PostgreSQL 15+ |
| **缓存** | Redis 6+ |
| **AI** | AWS Bedrock (Claude 3.5 Sonnet) |
| **IaC** | Terraform / OpenTofu |

### 前端
| 技术 | 版本/说明 |
|------|----------|
| **框架** | React 18+ with TypeScript |
| **UI库** | Ant Design |
| **状态管理** | Redux Toolkit |
| **表单处理** | React Hook Form |
| **构建工具** | Vite |

### 基础设施
| 技术 | 说明 |
|------|------|
| **容器化** | Docker + Docker Compose |
| **编排** | Kubernetes (可选) |
| **CI/CD** | GitHub Actions |
| **监控** | Prometheus + Grafana |

---

## 🛠️ 本地开发

### 环境要求
- Go 1.25+
- Node.js 22+
- PostgreSQL 18+ (pgvector)
- Docker & Docker Compose

### 启动开发环境

```bash
# 1. 克隆项目
git clone <repository-url>
cd iac-platform

# 2. 启动数据库
cp docker-compose.example.yml docker-compose.yml
docker compose up -d postgres

# 3. 启动后端
cd backend
go mod tidy
go run main.go

# 4. 启动前端
cd frontend
npm install
npm run dev
```

### 访问地址
| 服务 | 地址 |
|------|------|
| 前端 | http://localhost:5173 |
| 后端 API | http://localhost:8080 |

---

## 📚 文档

### 快速入门
- [快速开始指南](docs/01-QUICK_START_FOR_AI.md)
- [执行指南](docs/02-EXECUTION_GUIDE.md)
- [开发指南](docs/03-development-guide.md)

### 核心模块
| 模块 | 文档路径 |
|------|----------|
| **Workspace** | `docs/workspace/` |
| **IAM 权限** | `docs/iam/` |
| **Module 系统** | `docs/module/` |
| **Agent 架构** | `docs/agent/` |
| **CMDB** | `docs/cmdb/` |
| **AI 功能** | `docs/ai/` |
| **Run Tasks** | `docs/run-task/` |
| **Manifest** | `docs/manifest/` |
| **安全特性** | `docs/security/` |

---

## 📊 项目状态

**当前版本**: v1.0.0  
**开发状态**: 核心功能已完成，持续优化中

### ✅ 已完成功能

<details>
<summary><b>全局设置</b></summary>

- ✅ Agent 调度与应急解冻
- ✅ Agent 活跃度监控
- ✅ Agent 认证轮转
- ✅ K8S 自动扩缩容
- ✅ Agent 白名单策略
- ✅ 自定义 Agent 模板
- ✅ OpenTofu/Terraform 双引擎支持
- ✅ 全局/Workspace 版本配置
- ✅ Run Tasks 四阶段 Hook
- ✅ 强制/非强制执行模式
- ✅ 全局通知系统

</details>

<details>
<summary><b>AI 能力</b></summary>

- ✅ 错误分析（Claude 3.5 Sonnet）
- ✅ Module Skill 自动生成
- ✅ 向量化 CMDB 检索
- ✅ 安全断言机制
- ✅ 表单资源生成
- ✅ AI 权限继承

</details>

<details>
<summary><b>RBAC 权限</b></summary>

- ✅ 内建角色/自定义角色
- ✅ 用户组管理
- ✅ 审计日志
- ✅ 个人 Token（登录状态绑定）
- ✅ 团队 Token（持久有效）

</details>

<details>
<summary><b>Module 管理</b></summary>

- ✅ 版本化 Demo 和 Schema
- ✅ 表单化 Terraform 提交
- ✅ 参数分组和关联规则
- ✅ CMDB/Output 值填充
- ✅ AI 代码生成
- ✅ OpenAPI V3 可视化编辑
- ✅ 提示词 CRUD
- ✅ Module Skill 能力下放

</details>

<details>
<summary><b>CMDB</b></summary>

- ✅ 自动同步（Apply 触发）
- ✅ 轻量化字段存储
- ✅ 外置 CMDB 同步
- ✅ 树状结构展示
- ✅ 向量化数据支持

</details>

<details>
<summary><b>Workspace</b></summary>

- ✅ 纵览界面
- ✅ Run 列表和详情
- ✅ 分进度日志查看
- ✅ AI 错误分析按钮
- ✅ State 卡片式查看
- ✅ State 版本管理
- ✅ 资源表单/JSON 双模式
- ✅ 资源版本控制和回滚
- ✅ 资源锁和草稿机制
- ✅ 变量版本管理
- ✅ Output 链接
- ✅ Drift 检测（手动/自动）
- ✅ Workspace 配置管理

</details>

<details>
<summary><b>Manifests</b></summary>

- ✅ 拖拽式可视化编辑
- ✅ 关联关系查看
- ✅ Form/JSON 完整能力
- ✅ AI 辅助
- ✅ 多次部署支持

</details>

### 🚧 规划中功能

- ⏳ 单次授权审批流
- ⏳ SchemaSolver 最终裁决
- ⏳ 成本预测分析
- ⏳ GitOps 完整集成

---

## 🤝 贡献指南

### 开发规范
- **Go**: 遵循 Go 官方代码规范，使用 gofmt 格式化
- **TypeScript**: 使用 ESLint + Prettier，严格类型检查
- **提交规范**: 使用 Conventional Commits
- **测试要求**: 单元测试覆盖率 > 80%

### 提交流程
1. Fork 项目并创建功能分支
2. 按照开发指南进行开发
3. 确保测试通过和代码格式正确
4. 提交 Pull Request 并描述变更内容
5. 等待代码 Review 和合并

---

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情

---

## 📞 联系方式

- **问题反馈**: [GitHub Issues](https://github.com/your-org/iac-platform/issues)
- **功能建议**: [GitHub Discussions](https://github.com/your-org/iac-platform/discussions)

---

<p align="center">
  <sub>Built with ❤️ for Infrastructure as Code</sub>
</p>