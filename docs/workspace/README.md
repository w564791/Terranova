# Workspace模块完整文档集

> **文档版本**: v2.0  
> **最后更新**: 2025-10-09  
> **状态**: 已整合历史需求和新产品设计

## 📚 文档导航

###  已完成文档

本文档集包含以下16个完整文档：

1. **[00-overview.md](./00-overview.md)** - 总览与架构
   - 系统架构设计
   - 核心模块介绍
   - 功能优先级规划
   
2. **[01-lifecycle.md](./01-lifecycle.md)** - 生命周期与状态机
   - 完整状态机设计
   - 状态转换规则
   - 并发控制机制

3. **[02-execution-modes.md](./02-execution-modes.md)** - 执行模式概述
   - Local/Agent/K8s执行模式对比
   - 执行器接口设计
   - 模式选择指南

4. **[02-agent-k8s-implementation.md](./02-agent-k8s-implementation.md)** - Agent和K8s实现详解
   - Agent执行模式完整设计
   - K8s执行模式完整设计
   - Token管理系统
   - Agent Pool和选择策略
   - 任务锁机制设计
   - 完整的API接口设计（24个新接口）

5. **[03-state-management.md](./03-state-management.md)** - State管理
   - State存储和版本控制
   - 锁机制和回滚
   - 重试机制

6. **[04-task-workflow.md](./04-task-workflow.md)** - 任务流程
   - Plan/Apply任务流程
   - 插入任务设计（未来扩展）
   - 任务队列管理

7. **[05-drift-detection.md](./05-drift-detection.md)** - Drift检测
   - AI漂移检测（后续迭代）
   - 智能分析和报告
   - 自动修复建议

8. **[06-notification-system.md](./06-notification-system.md)** - 通知系统
   - 通知系统架构
   - 事件和目标配置
   - Webhook集成

9. **[07-logging-system.md](./07-logging-system.md)** - 日志系统
   - 日志系统设计
   - 多后端输出
   - 日志查询和分析

10. **[08-database-design.md](./08-database-design.md)** - 数据库设计
    - 数据库表结构
    - 索引和约束
    - 数据迁移

11. **[09-api-specification.md](./09-api-specification.md)** - API规范
    - REST API接口规范
    - 请求/响应格式
    - 错误处理

12. **[10-implementation-guide.md](./10-implementation-guide.md)** - 实现指导
    - 实现指导和最佳实践
    - 开发顺序建议
    - 代码示例

13. **[11-frontend-design.md](./11-frontend-design.md)** - 前端设计
    - 前端页面设计
    - 6个标签页详细设计
    - 交互规范和用户体验

14. **[12-global-configuration.md](./12-global-configuration.md)** - 全局配置
    - 全局配置管理
    - Agent Pool配置
    - K8s配置
    - Terraform版本配置

15. **[15-terraform-execution-detail.md](./15-terraform-execution-detail.md)** - Terraform执行流程详细设计 ⭐ 新增
    - 完整的11阶段执行流程（基于TFE标准）
    - 工作目录管理和文件结构
    - Plan和Apply强耦合设计
    - 钩子系统和阶段配置
    - 完整的代码实现示例
    - 日志管理和监控指标
    - 错误处理和重试策略

16. **[16-advanced-stages-design.md](./16-advanced-stages-design.md)** - 高级执行阶段设计 ⭐ 新增
    - OPA策略检查设计（未来扩展）
    - 成本估算功能设计（未来扩展）
    - Sentinel策略检查设计（未来扩展）
    - 完整的数据库设计和API规范
    - 策略示例和集成方案
    - 实施路线图和优先级

17. **[terraform-execution-development-progress.md](./terraform-execution-development-progress.md)** - Terraform执行引擎开发进度 ⭐ 新增
    - 核心功能开发计划（3-4周）
    - 详细任务清单和进度跟踪
    - Week 1-4实施路线图
    - 测试计划和开发规范

18. **[18-terraform-execution-testing-guide.md](./18-terraform-execution-testing-guide.md)** - Terraform执行引擎功能测试指南 ⭐ 新增
    - 完整的功能测试流程
    - 4个测试场景
    - 验证清单（30+项）
    - 常见问题和解决方案
    - 性能基准

19. **[19-new-run-workflow-design.md](./19-new-run-workflow-design.md)** - New Run工作流设计 ⭐ 新增
    - New run对话框设计（3个选项）
    - 添加资源流程（3个步骤）
    - 版本管理规则
    - Apply Method行为说明
    - 前端实现方案
    - 实施计划（4个Phase）

20. **[20-implementation-readiness-check.md](./20-implementation-readiness-check.md)** - 实施就绪检查 ⭐ 新增
    - 文档完整性评估
    - 可实施性评估
    - 缺失内容补充（辅助函数、模型定义）
    - 实施路线图和风险评估
    - 实施就绪度评分：94/100

21. **[development-progress.md](./development-progress.md)** - 开发进度
    - 开发进度跟踪
    - 按标签页组织的开发任务
    - Sprint优先级

22. **[24-execution-flow-integration-progress.md](./24-execution-flow-integration-progress.md)** - 执行流程连调进度 ⭐ 新增
    - 连调进度跟踪
    - 已解决问题记录
    - 测试记录和验证结果
    - 代码修改记录
    - 下一步计划

### 📝 快速导航

以下内容提供快速参考，详细信息请查看对应文档。

## 🚀 02. 执行模式详解

### Local模式
- 在服务器本地执行Terraform
- 适合开发测试环境
- 详见 [workspace-enhancement-complete-guide.md](../workspace-enhancement-complete-guide.md) 第1.1节

### Agent模式
- 分布式执行
- Agent注册和心跳机制
- 详见 [workspace-enhancement-complete-guide.md](../workspace-enhancement-complete-guide.md) 第1.2节

### K8s Pod模式
- 动态创建Pod
- Secret挂载token自动注册
- 详见 [workspace-enhancement-complete-guide.md](../workspace-enhancement-complete-guide.md) 第1.3节

## 💾 03. State管理与版本控制

### State存储
- PostgreSQL存储metadata
- S3存储实际state文件
- 详见 [workspace-enhancement-complete-guide.md](../workspace-enhancement-complete-guide.md) 第4节

### 版本控制
- 自动版本递增
- 历史版本查询
- 版本回滚支持
- 详见 [workspace-enhancement-complete-guide.md](../workspace-enhancement-complete-guide.md) 第4.3节

### 重试机制
- 3次自动重试
- 手动重试按钮
- State下载能力
- 详见 [workspace-enhancement-complete-guide.md](../workspace-enhancement-complete-guide.md) 第4.2节

## 🔄 04. 任务流程

### Plan任务
- 执行terraform plan
- 生成差异报告
- 可视化展示
- 详见 [workspace-enhancement-complete-guide.md](../workspace-enhancement-complete-guide.md) 第7.1节

### Apply任务
- 自动/手动Apply模式
- 重新执行plan
- State保存
- 详见 [workspace-enhancement-complete-guide.md](../workspace-enhancement-complete-guide.md) 第7.2节

### 插入任务（未来扩展）
- 审批流程
- 安全扫描
- 合规检查
- 详见产品设计文档第四节

## 🔍 05. AI Drift检测（后续迭代）

### 功能设计
- 周期性检测
- AI分析差异
- 智能报告
- 详见产品设计文档第五节

### 通知事件
- `on_drift_detected`
- `on_drift_resolved`

## 📢 06. 通知系统

### 第一版：基础Webhook
```yaml
notify_settings:
  on_start:
    - type: https
      url: https://ci-system/hooks/start
  on_plan_done:
    - type: https
      url: https://approval-system/hooks/plan
  on_completed:
    - type: https
      url: https://notification-system/hooks/success
  on_failed:
    - type: https
      url: https://alert-system/hooks/failure
```

### 事件阶段
1. `on_start` - 任务启动
2. `on_planning` - 开始plan
3. `on_plan_done` - plan完成
4. `on_waiting_apply` - 等待apply
5. `on_apply_start` - 开始apply
6. `on_completed` - 任务成功
7. `on_failed` - 任意阶段失败
8. `on_drift_detected` - 检测到漂移（后续）
9. `on_drift_resolved` - 漂移修复（后续）

### 未来扩展
- Prometheus指标推送
- Loki日志流
- S3报告存储
- Email/Slack/Teams通知

## 📜 07. 日志系统

### 第一版：基础日志
```go
type LogEntry struct {
    Workspace   string    `json:"workspace"`
    ExecutionID string    `json:"execution_id"`
    Phase       string    `json:"phase"`
    Timestamp   time.Time `json:"timestamp"`
    Message     string    `json:"message"`
    Level       string    `json:"level"`
}
```

### 日志级别
- DEBUG
- INFO
- WARN
- ERROR

### 未来扩展
- Elasticsearch索引
- Loki实时流
- S3归档
- HTTPS转发

## 🗄️ 08. 数据库设计

### 核心表结构

#### workspaces表
```sql
CREATE TABLE workspaces (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    tags JSONB, -- 标签系统
    
    -- 执行配置
    execution_mode VARCHAR(20) DEFAULT 'local',
    agent_id INTEGER REFERENCES agents(id),
    auto_apply BOOLEAN DEFAULT false,
    plan_only BOOLEAN DEFAULT false,
    
    -- Terraform配置
    terraform_version VARCHAR(20) DEFAULT 'latest',
    workdir VARCHAR(500) DEFAULT '/workspace',
    variables JSONB,
    system_variables JSONB,
    
    -- 锁定状态
    is_locked BOOLEAN DEFAULT false,
    locked_by INTEGER REFERENCES users(id),
    locked_at TIMESTAMP,
    lock_reason TEXT,
    
    -- State配置
    state_backend VARCHAR(20) DEFAULT 'local',
    state_config JSONB,
    tf_code JSONB,
    tf_state JSONB,
    
    -- Provider配置
    provider_config JSONB,
    init_config JSONB,
    
    -- 通知和日志配置
    notify_settings JSONB,
    log_config JSONB,
    
    -- 生命周期状态
    state VARCHAR(20) DEFAULT 'created',
    
    -- 元数据
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(name, created_by)
);
```

#### workspace_tasks表
```sql
CREATE TABLE workspace_tasks (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
    task_type VARCHAR(20) NOT NULL, -- plan, apply
    status VARCHAR(20) DEFAULT 'pending',
    
    -- 执行信息
    execution_mode VARCHAR(20) NOT NULL,
    agent_id INTEGER REFERENCES agents(id),
    k8s_pod_name VARCHAR(100),
    k8s_namespace VARCHAR(100),
    
    -- 输出
    plan_output TEXT,
    apply_output TEXT,
    error_message TEXT,
    
    -- 时间统计
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration INTEGER,
    
    -- 重试
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

#### workspace_state_versions表
```sql
CREATE TABLE workspace_state_versions (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
    content JSONB NOT NULL,
    version INTEGER NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    size_bytes INTEGER,
    task_id INTEGER REFERENCES workspace_tasks(id),
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(workspace_id, version)
);
```

完整SQL详见 [workspace-enhancement-complete-guide.md](../workspace-enhancement-complete-guide.md) 数据库设计章节。

## 🔌 09. API接口规范

### Workspace管理

```
# 创建Workspace
POST /api/v1/workspaces
{
    "name": "prod-network",
    "description": "Production network infrastructure",
    "tags": ["production", "network"],
    "execution_mode": "agent",
    "agent_id": 1,
    "auto_apply": false,
    "plan_only": false,
    "terraform_version": "1.6.0",
    "variables": {...},
    "provider_config": {...}
}

# 更新Workspace
PUT /api/v1/workspaces/:id

# 获取Workspace
GET /api/v1/workspaces/:id

# 列表Workspace
GET /api/v1/workspaces?tags=production&state=completed

# 删除Workspace
DELETE /api/v1/workspaces/:id

# 锁定Workspace
POST /api/v1/workspaces/:id/lock
{
    "reason": "Maintenance in progress"
}

# 解锁Workspace
POST /api/v1/workspaces/:id/unlock
```

### 任务管理

```
# 创建Plan任务
POST /api/v1/workspaces/:id/tasks/plan

# 创建Apply任务
POST /api/v1/workspaces/:id/tasks/apply

# 获取任务详情
GET /api/v1/workspaces/:workspace_id/tasks/:task_id

# 获取任务列表
GET /api/v1/workspaces/:workspace_id/tasks

# 取消任务
POST /api/v1/workspaces/:workspace_id/tasks/:task_id/cancel

# 重试任务
POST /api/v1/workspaces/:workspace_id/tasks/:task_id/retry
```

### State管理

```
# 获取State版本列表
GET /api/v1/workspaces/:id/state-versions

# 下载指定版本State
GET /api/v1/workspaces/:id/state-versions/:version/download

# 回滚到指定版本
POST /api/v1/workspaces/:id/state-versions/:version/rollback
```

## 🛠️ 10. 实现指导

### 开发顺序建议

#### Phase 1: 核心功能（2-3周）
1.  Workspace CRUD + Tags
2.  生命周期状态机
3.  Local执行模式
4.  Plan/Apply基础流程
5.  State版本控制

#### Phase 2: 扩展功能（2-3周）
6.  Agent执行模式
7.  K8s执行模式
8.  Workspace锁定
9.  基础Webhook通知
10.  Plan差异可视化

#### Phase 3: 优化功能（1-2周）
11.  重试机制
12.  并发控制
13.  监控指标
14.  错误处理

#### Phase 4: 未来扩展
15. ⏳ 插入任务流
16. ⏳ AI Drift检测
17. ⏳ 完整通知系统
18. ⏳ 完整日志系统

### 技术栈

**后端**:
- Go 1.21+
- Gin框架
- GORM
- PostgreSQL 15+
- S3兼容存储

**前端**:
- React 18+
- TypeScript
- CSS Modules
- Monaco Editor（代码编辑）

**基础设施**:
- Kubernetes（可选）
- Prometheus（监控）
- Loki（日志，可选）

### 代码结构

```
backend/
├── models/
│   ├── workspace.go
│   ├── workspace_task.go
│   └── workspace_state_version.go
├── services/
│   ├── workspace_service.go
│   ├── task_executor.go
│   ├── state_manager.go
│   ├── lock_manager.go
│   └── notify_system.go
├── controllers/
│   ├── workspace_controller.go
│   └── task_controller.go
└── executors/
    ├── executor.go (interface)
    ├── local_executor.go
    ├── agent_executor.go
    └── k8s_executor.go

frontend/
├── pages/
│   ├── Workspaces.tsx
│   ├── WorkspaceDetail.tsx
│   ├── TaskDetail.tsx
│   └── StateVersions.tsx
└── components/
    ├── WorkspaceForm.tsx
    ├── TaskList.tsx
    ├── PlanDiff.tsx
    └── StateVersionList.tsx
```

## 📊 关键指标

### 性能指标
- Plan任务平均执行时间: < 30s
- Apply任务平均执行时间: < 2min
- State保存成功率: > 99.9%
- API响应时间: < 200ms

### 可靠性指标
- 系统可用性: > 99.9%
- 任务成功率: > 95%
- 重试成功率: > 80%

## 🔗 相关资源

### 内部文档
- [workspace-enhancement-complete-guide.md](../workspace-enhancement-complete-guide.md) - 历史需求文档
- [产品设计文档.md](../../🌍 AI 驱动 Terraform 平台 — Workspace 模块产品设计文档.md) - 新产品设计

### 外部参考
- [HCP Terraform Documentation](https://developer.hashicorp.com/terraform/cloud-docs)
- [Terraform State](https://developer.hashicorp.com/terraform/language/state)
- [Kubernetes Jobs](https://kubernetes.io/docs/concepts/workloads/controllers/job/)

## 📝 更新日志

| 版本 | 日期 | 变更说明 |
|------|------|----------|
| v2.0 | 2025-10-09 | 整合历史需求和新产品设计，创建完整文档集 |
| v1.0 | 2025-01-02 | 初始版本 |

## 🤖 AI助手开发指南

### 开始开发前必读

如果你是AI助手，准备开始Workspace模块的开发，请按以下步骤进行：

#### 1. 📖 阅读核心文档（必读）

**第一步 - 了解架构**:
- 阅读 [00-overview.md](./00-overview.md) - 了解整体架构和核心目标
- 阅读 [development-progress.md](./development-progress.md) - 了解当前开发进度和任务优先级

**第二步 - 了解规范**:
- **后端开发**: 阅读 [09-api-specification.md](./09-api-specification.md) - API接口规范
- **前端开发**: 阅读 [11-frontend-design.md](./11-frontend-design.md) - 前端设计规范
- **数据库**: 阅读 [08-database-design.md](./08-database-design.md) - 数据库表结构

**第三步 - 了解实现细节**:
- 阅读 [10-implementation-guide.md](./10-implementation-guide.md) - 实现指导和最佳实践
- 根据任务类型阅读相关文档（01-07）

#### 2. 📋 获取开发任务

从 [development-progress.md](./development-progress.md) 中查看：
- **当前Sprint优先级** - 了解高/中/低优先级任务
- **按标签页组织的任务** - 根据6个标签页选择任务
- **任务清单** - 查看具体的待办事项

#### 3. 🔍 查找API规范

**后端API规范**:
- 主文档: [09-api-specification.md](./09-api-specification.md)
- Agent/K8s API: [02-agent-k8s-implementation.md](./02-agent-k8s-implementation.md)
- 全局配置API: [12-global-configuration.md](./12-global-configuration.md)

**前端规范**:
- 主文档: [11-frontend-design.md](./11-frontend-design.md)
- 表单规范: `../frontend-form-style-guide.md`
- UX规则: `../.amazonq/prompts/frontend-ux-rules.md`

#### 4. 📝 提交开发进度

**更新模块进度**:
1. 完成任务后，更新 [development-progress.md](./development-progress.md)
2. 在对应任务前标记 `[x]` 表示完成
3. 更新"更新日志"章节，记录完成的工作
4. 提交Git commit，格式：`feat: 完成XXX功能`

**更新项目总进度**:
1. 更新 `../project-status.md` - 项目总体进度
2. 更新对应模块的完成度百分比
3. 添加里程碑记录

#### 5. 🎯 开发流程示例

```bash
# 1. 选择任务（例如：实现Overview API）
# 从 development-progress.md 查看任务详情

# 2. 阅读相关文档
# - 09-api-specification.md（API规范）
# - 08-database-design.md（数据库设计）
# - 11-frontend-design.md（前端设计）

# 3. 实现功能
# - 创建/修改后端代码
# - 创建/修改前端代码
# - 编写测试

# 4. 测试验证
# - 运行单元测试
# - 手动测试功能
# - 验证API响应

# 5. 更新文档
# - 更新 development-progress.md
# - 标记任务为完成 [x]
# - 添加更新日志

# 6. 提交代码
git add .
git commit -m "feat: 实现Overview API

 完成内容:
- 实现WorkspaceOverviewService
- 实现资源统计功能
- 创建Overview API端点

📊 进度更新:
- Overview标签页后端任务: 0% → 33%"

# 7. 更新项目总进度
# 编辑 ../project-status.md
```

#### 6. 📚 常用文档快速索引

| 开发任务 | 需要阅读的文档 |
|---------|---------------|
| 实现新API | 09-api-specification.md, 08-database-design.md |
| 前端页面 | 11-frontend-design.md, frontend-form-style-guide.md |
| 数据库变更 | 08-database-design.md, development-progress.md |
| Agent/K8s | 02-agent-k8s-implementation.md, 02-execution-modes.md |
| State管理 | 03-state-management.md, 09-api-specification.md |
| 通知系统 | 06-notification-system.md, 12-global-configuration.md |

#### 7.  重要提醒

-  **永远保留用户输入** - 遵循前端UX规则
-  **使用统一通知系统** - Toast/ConfirmDialog
-  **遵循API规范** - 统一的请求/响应格式
-  **更新进度文档** - 及时更新development-progress.md
-  **编写清晰的commit** - 包含功能描述和进度更新

## 🤝 贡献指南

1. 阅读本README了解整体架构
2. 根据开发任务阅读相关章节
3. 参考代码示例进行实现
4. 更新文档反映实际实现

## 📞 联系方式

如有疑问，请通过以下方式联系：
- 项目Issue: [GitHub Issues]
- 技术讨论: [Slack Channel]
- 邮件: dev@iac-platform.com

---

**开始开发**: 建议从 [10. 实现指导](#-10-实现指导) 章节开始
