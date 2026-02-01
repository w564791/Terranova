# Run Task 功能设计文档

> **AI 助手注意**: 
> - 任务进度跟踪位于本文档末尾的 **"11. 实现进度跟踪"** 章节
> - 每完成一个子任务后，请更新对应的复选框状态
> - 开始新任务前，请先阅读进度跟踪章节了解当前状态

## 1. 概述

Run Task 是一个类似 Terraform Enterprise 的功能，允许 IaC 平台在 Terraform 运行（Run）生命周期的特定阶段主动发起对第三方服务的 HTTP POST 调用。这种调用可以用于验证 Terraform 配置、分析执行计划、扫描漏洞、执行自定义操作或其他集成。

### 1.1 核心概念

- **Run Task（全局定义）**：在组织/团队级别定义的外部服务集成，包含名称、Endpoint URL、HMAC密钥等配置
- **Global Run Task（全局任务）**：自动应用于所有 Workspace 的 Run Task
- **Workspace Run Task（工作空间应用）**：将全局 Run Task 应用到特定 Workspace，配置执行阶段和执行级别

**注意**：Run Task 与 Task Agent 是完全不同的概念：
- **Run Task**：平台主动调用第三方服务，对当前任务进行审查（安全扫描、成本估算、合规检查等）
- **Task Agent**：执行 Terraform Plan/Apply 的工作节点

### 1.2 执行阶段

| 阶段 | 说明 | 触发时机 | 可用数据 |
|------|------|----------|----------|
| **Pre-plan** | 在 Terraform 生成计划之前 | Plan 开始前 | 配置版本、变量 |
| **Post-plan** | 在 Terraform 创建计划之后 | Plan 完成后，Apply 确认前 | 配置版本、变量、Plan JSON |
| **Pre-apply** | 在 Terraform 应用计划之前 | Apply 开始前 | 配置版本、变量、Plan JSON |
| **Post-apply** | 在 Terraform 应用计划之后 | Apply 完成后 | 配置版本、变量、Apply 结果 |

### 1.3 执行级别

| 级别 | 说明 | 行为 |
|------|------|------|
| **Advisory** | 建议性 | 失败时产生警告，但不阻止执行 |
| **Mandatory** | 强制性 | 失败时停止执行 |

**最终 Run 状态**：由所有关联任务中最严格的执行级别决定。如果有 Mandatory 任务失败，即使 Advisory 任务成功，Run 也会失败。

### 1.4 工作流程概述

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Run Task Workflow Overview                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. 配置阶段 (Setup)                                                         │
│     ├── 创建 Run Task（组织级别）                                            │
│     ├── 配置端点 URL、HMAC 密钥、超时时间                                    │
│     └── 关联到 Workspace（指定阶段和执行级别）                               │
│                                                                              │
│  2. 触发与执行阶段 (Trigger & Execution)                                     │
│     ├── Run 到达触发阶段时暂停                                               │
│     ├── 收集数据（配置版本、变量、Plan JSON 等）                             │
│     ├── 生成一次性 Access Token                                              │
│     ├── 发送 POST 请求到端点 URL（包含 Payload + 回调 URL）                  │
│     └── 所有任务并行执行                                                     │
│                                                                              │
│  3. 响应与决策阶段 (Response & Decision)                                     │
│     ├── 第三方服务使用 Access Token 获取详细数据                             │
│     ├── 第三方服务分析数据并调用回调 URL 返回结果                            │
│     ├── 根据执行级别决定 Run 是否继续                                        │
│     └── 任务日志记录在 UI 中                                                 │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. 数据库设计

### 2.1 Run Task 表（全局定义）

```sql
-- Run Task 全局定义表
CREATE TABLE run_tasks (
    id SERIAL PRIMARY KEY,
    run_task_id VARCHAR(50) UNIQUE NOT NULL,  -- 语义化ID，如 "rt-security-scan"
    name VARCHAR(100) NOT NULL,                -- 名称，只能包含字母、数字、破折号和下划线
    description TEXT,                          -- 描述（可选）
    endpoint_url VARCHAR(500) NOT NULL,        -- Endpoint URL，Run Tasks 会 POST 到这个 URL
    hmac_key_encrypted TEXT,                   -- HMAC密钥（加密存储，可选）
    enabled BOOLEAN DEFAULT true,              -- 是否启用
    
    -- 超时配置
    timeout_seconds INTEGER DEFAULT 600,       -- 超时时间（秒），默认10分钟
    
    -- 全局任务配置
    is_global BOOLEAN DEFAULT false,           -- 是否为全局任务（自动应用于所有 Workspace）
    
    -- 组织/团队归属
    organization_id VARCHAR(50),               -- 组织ID（可选）
    team_id VARCHAR(50),                       -- 团队ID（可选）
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 约束
    CONSTRAINT run_tasks_name_check CHECK (name ~ '^[a-zA-Z0-9_-]+$'),
    CONSTRAINT run_tasks_timeout_check CHECK (timeout_seconds >= 60 AND timeout_seconds <= 3600)
);

-- 索引
CREATE INDEX idx_run_tasks_name ON run_tasks(name);
CREATE INDEX idx_run_tasks_organization ON run_tasks(organization_id);
CREATE INDEX idx_run_tasks_team ON run_tasks(team_id);
CREATE INDEX idx_run_tasks_enabled ON run_tasks(enabled);
CREATE INDEX idx_run_tasks_is_global ON run_tasks(is_global) WHERE is_global = true;
```

### 2.2 Workspace Run Task 表（工作空间应用）

```sql
-- Workspace Run Task 关联表
CREATE TABLE workspace_run_tasks (
    id SERIAL PRIMARY KEY,
    workspace_run_task_id VARCHAR(50) UNIQUE NOT NULL,  -- 语义化ID
    workspace_id VARCHAR(50) NOT NULL REFERENCES workspaces(workspace_id),
    run_task_id VARCHAR(50) NOT NULL REFERENCES run_tasks(run_task_id),
    
    -- 执行配置
    stage VARCHAR(20) NOT NULL,                -- 执行阶段: pre_plan, post_plan, pre_apply, post_apply
    enforcement_level VARCHAR(20) NOT NULL DEFAULT 'advisory',  -- 执行级别: advisory, mandatory
    
    -- 状态
    enabled BOOLEAN DEFAULT true,
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 约束
    CONSTRAINT workspace_run_tasks_stage_check CHECK (stage IN ('pre_plan', 'post_plan', 'pre_apply', 'post_apply')),
    CONSTRAINT workspace_run_tasks_enforcement_check CHECK (enforcement_level IN ('advisory', 'mandatory')),
    UNIQUE(workspace_id, run_task_id, stage)  -- 同一个workspace的同一个run_task在同一阶段只能配置一次
);

-- 索引
CREATE INDEX idx_workspace_run_tasks_workspace ON workspace_run_tasks(workspace_id);
CREATE INDEX idx_workspace_run_tasks_run_task ON workspace_run_tasks(run_task_id);
CREATE INDEX idx_workspace_run_tasks_stage ON workspace_run_tasks(stage);
CREATE INDEX idx_workspace_run_tasks_enabled ON workspace_run_tasks(enabled);
```

### 2.3 Run Task 执行记录表

```sql
-- Run Task 执行记录表
CREATE TABLE run_task_results (
    id SERIAL PRIMARY KEY,
    result_id VARCHAR(50) UNIQUE NOT NULL,     -- 语义化ID
    
    -- 关联
    task_id BIGINT NOT NULL,                   -- 关联的 workspace_task ID
    workspace_run_task_id VARCHAR(50) NOT NULL REFERENCES workspace_run_tasks(workspace_run_task_id),
    
    -- 执行信息
    stage VARCHAR(20) NOT NULL,                -- 执行阶段
    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- 状态: pending, running, passed, failed, error, timeout, skipped
    
    -- 一次性 Access Token
    access_token VARCHAR(500),                 -- 一次性验证令牌（JWT格式）
    access_token_expires_at TIMESTAMP,         -- Token过期时间
    access_token_used BOOLEAN DEFAULT false,   -- Token是否已使用
    
    -- 请求/响应
    request_payload JSONB,                     -- 发送给外部服务的请求
    response_payload JSONB,                    -- 外部服务的响应
    callback_url VARCHAR(500),                 -- 回调URL（用于异步结果）
    
    -- 结果详情
    message TEXT,                              -- 结果消息
    url VARCHAR(500),                          -- 详情链接（外部服务提供）
    
    -- 超时配置
    timeout_seconds INTEGER DEFAULT 600,       -- 超时时间（秒）
    timeout_at TIMESTAMP,                      -- 超时时间点
    
    -- 时间
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    
    -- 元数据
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 约束
    CONSTRAINT run_task_results_status_check CHECK (status IN ('pending', 'running', 'passed', 'failed', 'error', 'timeout', 'skipped'))
);

-- 索引
CREATE INDEX idx_run_task_results_task ON run_task_results(task_id);
CREATE INDEX idx_run_task_results_workspace_run_task ON run_task_results(workspace_run_task_id);
CREATE INDEX idx_run_task_results_status ON run_task_results(status);
CREATE INDEX idx_run_task_results_stage ON run_task_results(stage);
CREATE INDEX idx_run_task_results_created_at ON run_task_results(created_at);
CREATE INDEX idx_run_task_results_timeout_at ON run_task_results(timeout_at) WHERE status = 'running';
```

### 2.4 Run Task Outcomes 表（符合 TFE 规范）

```sql
-- Run Task Outcomes 表（存储详细的检查结果，符合 TFE 规范）
CREATE TABLE run_task_outcomes (
    id SERIAL PRIMARY KEY,
    
    -- 关联
    run_task_result_id VARCHAR(50) NOT NULL REFERENCES run_task_results(result_id),
    
    -- Outcome 标识（第三方服务提供）
    outcome_id VARCHAR(100) NOT NULL,          -- 第三方服务提供的唯一标识，如 "PRTNR-CC-TF-127"
    
    -- 描述
    description VARCHAR(500) NOT NULL,         -- 一行描述
    body TEXT,                                 -- Markdown 格式的详细内容（建议 < 1MB，最大 5MB）
    url VARCHAR(500),                          -- 详情链接
    
    -- 标签（JSON 格式，支持 severity 和 status 特殊处理）
    tags JSONB,                                -- 标签对象，如 {"Status": [{"label": "Failed", "level": "error"}], "Severity": [...]}
    
    -- 元数据
    created_at TIMESTAMP DEFAULT NOW()
);

-- 索引
CREATE INDEX idx_run_task_outcomes_result ON run_task_outcomes(run_task_result_id);
CREATE INDEX idx_run_task_outcomes_outcome_id ON run_task_outcomes(outcome_id);
```

**Tags 结构说明：**

```json
{
  "Status": [
    { "label": "Failed", "level": "error" }
  ],
  "Severity": [
    { "label": "High", "level": "error" },
    { "label": "Recoverable", "level": "info" }
  ],
  "Cost Centre": [
    { "label": "IT-OPS" }
  ]
}
```

**Tag Level 说明：**
- `none`（默认）：普通文本
- `info`：蓝色图标
- `warning`：黄色图标
- `error`：红色图标

**注意**：`body` 字段支持 Markdown 格式，前端展示时需要进行 XSS 过滤（使用 DOMPurify 等库）。

---

## 3. API 设计

### 3.1 Run Task 管理 API（全局）

#### 3.1.1 创建 Run Task

```
POST /api/v1/run-tasks
```

**请求体：**
```json
{
  "name": "security-scan",
  "description": "Security scanning service",
  "endpoint_url": "https://security.example.com/api/scan",
  "hmac_key": "secret-key-123",
  "organization_id": "org-default",
  "team_id": "team-ops"
}
```

**响应：**
```json
{
  "run_task_id": "rt-security-scan",
  "name": "security-scan",
  "description": "Security scanning service",
  "endpoint_url": "https://security.example.com/api/scan",
  "hmac_key_set": true,
  "enabled": true,
  "organization_id": "org-default",
  "team_id": "team-ops",
  "created_at": "2025-01-06T10:00:00Z"
}
```

#### 3.1.2 获取 Run Task 列表

```
GET /api/v1/run-tasks?organization_id=org-default&page=1&page_size=20
```

**响应：**
```json
{
  "run_tasks": [
    {
      "run_task_id": "rt-security-scan",
      "name": "security-scan",
      "description": "Security scanning service",
      "endpoint_url": "https://security.example.com/api/scan",
      "hmac_key_set": true,
      "enabled": true,
      "workspace_count": 5,
      "created_at": "2025-01-06T10:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 1
  }
}
```

#### 3.1.3 获取单个 Run Task

```
GET /api/v1/run-tasks/:run_task_id
```

#### 3.1.4 更新 Run Task

```
PUT /api/v1/run-tasks/:run_task_id
```

**请求体：**
```json
{
  "name": "security-scan-v2",
  "description": "Updated security scanning service",
  "endpoint_url": "https://security.example.com/api/v2/scan",
  "hmac_key": "new-secret-key",
  "enabled": true
}
```

#### 3.1.5 删除 Run Task

```
DELETE /api/v1/run-tasks/:run_task_id
```

### 3.2 Workspace Run Task API

#### 3.2.1 为 Workspace 添加 Run Task

```
POST /api/v1/workspaces/:workspace_id/run-tasks
```

**请求体：**
```json
{
  "run_task_id": "rt-security-scan",
  "stage": "post_plan",
  "enforcement_level": "mandatory"
}
```

**响应：**
```json
{
  "workspace_run_task_id": "wrt-ws001-security-scan-post-plan",
  "workspace_id": "ws-001",
  "run_task_id": "rt-security-scan",
  "run_task_name": "security-scan",
  "stage": "post_plan",
  "enforcement_level": "mandatory",
  "enabled": true,
  "created_at": "2025-01-06T10:00:00Z"
}
```

#### 3.2.2 获取 Workspace 的 Run Task 列表

```
GET /api/v1/workspaces/:workspace_id/run-tasks
```

**响应：**
```json
{
  "workspace_run_tasks": [
    {
      "workspace_run_task_id": "wrt-ws001-security-scan-post-plan",
      "run_task": {
        "run_task_id": "rt-security-scan",
        "name": "security-scan",
        "description": "Security scanning service"
      },
      "stage": "post_plan",
      "enforcement_level": "mandatory",
      "enabled": true
    },
    {
      "workspace_run_task_id": "wrt-ws001-cost-estimate-post-plan",
      "run_task": {
        "run_task_id": "rt-cost-estimate",
        "name": "cost-estimate",
        "description": "Cost estimation service"
      },
      "stage": "post_plan",
      "enforcement_level": "advisory",
      "enabled": true
    }
  ]
}
```

#### 3.2.3 更新 Workspace Run Task

```
PUT /api/v1/workspaces/:workspace_id/run-tasks/:workspace_run_task_id
```

**请求体：**
```json
{
  "stage": "pre_apply",
  "enforcement_level": "advisory",
  "enabled": true
}
```

#### 3.2.4 删除 Workspace Run Task

```
DELETE /api/v1/workspaces/:workspace_id/run-tasks/:workspace_run_task_id
```

### 3.3 Run Task 执行结果 API

#### 3.3.1 获取任务的 Run Task 结果

```
GET /api/v1/workspaces/:workspace_id/tasks/:task_id/run-task-results
```

**响应：**
```json
{
  "run_task_results": [
    {
      "result_id": "rtr-001",
      "run_task": {
        "run_task_id": "rt-security-scan",
        "name": "security-scan"
      },
      "stage": "post_plan",
      "status": "passed",
      "message": "No security issues found",
      "url": "https://security.example.com/reports/123",
      "started_at": "2025-01-06T10:01:00Z",
      "completed_at": "2025-01-06T10:01:30Z"
    },
    {
      "result_id": "rtr-002",
      "run_task": {
        "run_task_id": "rt-cost-estimate",
        "name": "cost-estimate"
      },
      "stage": "post_plan",
      "status": "passed",
      "message": "Estimated monthly cost: $150",
      "url": "https://cost.example.com/estimates/456",
      "started_at": "2025-01-06T10:01:00Z",
      "completed_at": "2025-01-06T10:01:15Z"
    }
  ]
}
```

#### 3.3.2 Run Task 回调 API（外部服务调用）

```
PATCH /api/v1/run-task-results/:result_id/callback
```

**请求体：**
```json
{
  "status": "passed",
  "message": "All checks passed",
  "url": "https://external-service.com/results/123"
}
```

---

## 4. 执行流程设计

### 4.1 任务执行流程（集成 Run Task）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Task Execution Flow                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────┐                                                                │
│  │  Start   │                                                                │
│  └────┬─────┘                                                                │
│       │                                                                      │
│       ▼                                                                      │
│  ┌──────────────────┐     ┌─────────────────────────────────────────────┐   │
│  │  Pre-Plan Stage  │────▶│  Execute Pre-Plan Run Tasks                 │   │
│  └────────┬─────────┘     │  - Call external services                   │   │
│           │               │  - Wait for results (sync/async)            │   │
│           │               │  - Check enforcement level                  │   │
│           │               │  - Block if mandatory task fails            │   │
│           │               └─────────────────────────────────────────────┘   │
│           ▼                                                                  │
│  ┌──────────────────┐                                                        │
│  │  Terraform Plan  │                                                        │
│  └────────┬─────────┘                                                        │
│           │                                                                  │
│           ▼                                                                  │
│  ┌──────────────────┐     ┌─────────────────────────────────────────────┐   │
│  │  Post-Plan Stage │────▶│  Execute Post-Plan Run Tasks                │   │
│  └────────┬─────────┘     │  - Send plan data to external services      │   │
│           │               │  - Security scan, cost estimation, etc.     │   │
│           │               │  - Block if mandatory task fails            │   │
│           │               └─────────────────────────────────────────────┘   │
│           ▼                                                                  │
│  ┌──────────────────┐                                                        │
│  │  Apply Pending   │  (Wait for user confirmation if not auto-apply)       │
│  └────────┬─────────┘                                                        │
│           │                                                                  │
│           ▼                                                                  │
│  ┌──────────────────┐     ┌─────────────────────────────────────────────┐   │
│  │  Pre-Apply Stage │────▶│  Execute Pre-Apply Run Tasks                │   │
│  └────────┬─────────┘     │  - Final checks before apply                │   │
│           │               │  - Approval workflows                       │   │
│           │               │  - Block if mandatory task fails            │   │
│           │               └─────────────────────────────────────────────┘   │
│           ▼                                                                  │
│  ┌──────────────────┐                                                        │
│  │ Terraform Apply  │                                                        │
│  └────────┬─────────┘                                                        │
│           │                                                                  │
│           ▼                                                                  │
│  ┌──────────────────┐     ┌─────────────────────────────────────────────┐   │
│  │ Post-Apply Stage │────▶│  Execute Post-Apply Run Tasks               │   │
│  └────────┬─────────┘     │  - Notifications                            │   │
│           │               │  - Documentation updates                    │   │
│           │               │  - CMDB sync                                │   │
│           │               │  - Advisory only (cannot block)             │   │
│           │               └─────────────────────────────────────────────┘   │
│           ▼                                                                  │
│  ┌──────────┐                                                                │
│  │   End    │                                                                │
│  └──────────┘                                                                │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Run Task 完整调用流程

Run Task 的调用流程是一个异步的、基于回调的机制：

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                           Run Task Complete Invocation Flow                              │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  IaC Platform                              Run Task Platform                             │
│       │                                           │                                      │
│       │  ┌─────────────────────────────────────┐  │                                      │
│       │  │ 1. 触发 Run Task                    │  │                                      │
│       │  │    - 生成一次性 Access Token        │  │                                      │
│       │  │    - 创建 Run Task Result 记录      │  │                                      │
│       │  │    - 设置超时计时器                 │  │                                      │
│       │  └─────────────────────────────────────┘  │                                      │
│       │                                           │                                      │
│       │  2. POST {endpoint_url}                   │                                      │
│       │  {                                        │                                      │
│       │    "payload_version": 1,                  │                                      │
│       │    "stage": "post_plan",                  │                                      │
│       │    "access_token": "eyJhbGci...",         │  ← 一次性验证令牌                    │
│       │    "task_result_id": "rtr-001",           │                                      │
│       │    "task_result_callback_url": "...",     │  ← 回调URL                           │
│       │    "task_result_enforcement_level": "...",│                                      │
│       │    "run_id": 123,                         │                                      │
│       │    "workspace_id": "ws-001",              │                                      │
│       │    "plan_json_api_url": "...",            │  ← 获取变更数据的URL                 │
│       │    "timeout_seconds": 600                 │  ← 超时时间（秒）                    │
│       │  }                                        │                                      │
│       │─────────────────────────────────────────▶│                                      │
│       │                                           │                                      │
│       │  3. 200 OK (Acknowledge)                  │                                      │
│       │◀─────────────────────────────────────────│                                      │
│       │                                           │                                      │
│       │                                           │  ┌─────────────────────────────────┐│
│       │                                           │  │ 4. Run Task 平台处理            ││
│       │                                           │  │    a. 使用 access_token 调用    ││
│       │                                           │  │       plan_json_api_url         ││
│       │                                           │  │    b. 获取所有变更数据          ││
│       │                                           │  │    c. 分析每个资源              ││
│       │                                           │  │    d. 生成分析结果              ││
│       │                                           │  └─────────────────────────────────┘│
│       │                                           │                                      │
│       │  5. GET {plan_json_api_url}               │                                      │
│       │     Authorization: Bearer {access_token}  │                                      │
│       │◀─────────────────────────────────────────│                                      │
│       │                                           │                                      │
│       │  6. 200 OK                                │                                      │
│       │  {                                        │                                      │
│       │    "resource_changes": [...],             │  ← 所有变更数据                      │
│       │    "variables": {...},                    │                                      │
│       │    "outputs": {...}                       │                                      │
│       │  }                                        │                                      │
│       │─────────────────────────────────────────▶│                                      │
│       │                                           │                                      │
│       │                                           │  ┌─────────────────────────────────┐│
│       │                                           │  │ 7. 分析完成，准备回调           ││
│       │                                           │  └─────────────────────────────────┘│
│       │                                           │                                      │
│       │  8. PATCH {task_result_callback_url}      │                                      │
│       │     Authorization: Bearer {access_token}  │                                      │
│       │  {                                        │                                      │
│       │    "status": "passed" | "failed",         │                                      │
│       │    "message": "Analysis complete",        │                                      │
│       │    "url": "https://...",                  │  ← 详情链接                          │
│       │    "resource_results": [                  │  ← 资源级别结果                      │
│       │      {                                    │                                      │
│       │        "resource_address": "aws_s3...",   │                                      │
│       │        "status": "passed",                │                                      │
│       │        "message": "No issues found"       │                                      │
│       │      },                                   │                                      │
│       │      {                                    │                                      │
│       │        "resource_address": "aws_iam...",  │                                      │
│       │        "status": "failed",                │                                      │
│       │        "message": "Policy too permissive",│                                      │
│       │        "severity": "high"                 │                                      │
│       │      }                                    │                                      │
│       │    ]                                      │                                      │
│       │  }                                        │                                      │
│       │◀─────────────────────────────────────────│                                      │
│       │                                           │                                      │
│       │  9. 200 OK                                │                                      │
│       │─────────────────────────────────────────▶│                                      │
│       │                                           │                                      │
│       │  ┌─────────────────────────────────────┐  │                                      │
│       │  │ 10. IaC 平台处理回调结果            │  │                                      │
│       │  │     - 更新 Run Task Result 状态     │  │                                      │
│       │  │     - 保存资源级别结果              │  │                                      │
│       │  │     - 根据 enforcement_level 决定   │  │                                      │
│       │  │       是否阻塞任务执行              │  │                                      │
│       │  │     - 作废 access_token             │  │                                      │
│       │  └─────────────────────────────────────┘  │                                      │
│       │                                           │                                      │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

### 4.3 超时处理流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Timeout Handling Flow                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  IaC Platform                              Run Task Platform                 │
│       │                                           │                          │
│       │  1. POST {endpoint_url}                   │                          │
│       │─────────────────────────────────────────▶│                          │
│       │                                           │                          │
│       │  2. 200 OK                                │                          │
│       │◀─────────────────────────────────────────│                          │
│       │                                           │                          │
│       │  ┌─────────────────────────────────────┐  │                          │
│       │  │ 3. 启动超时计时器                   │  │                          │
│       │  │    timeout = run_task.timeout_seconds │  │                          │
│       │  │    (默认 600 秒 = 10 分钟)          │  │                          │
│       │  └─────────────────────────────────────┘  │                          │
│       │                                           │                          │
│       │           ... 等待回调 ...                │                          │
│       │                                           │                          │
│       │  ┌─────────────────────────────────────┐  │                          │
│       │  │ 4. 超时！                           │  │                          │
│       │  │    - 更新状态为 "timeout"           │  │                          │
│       │  │    - 作废 access_token              │  │                          │
│       │  │    - 根据 enforcement_level 决定    │  │                          │
│       │  │      是否阻塞任务                   │  │                          │
│       │  └─────────────────────────────────────┘  │                          │
│       │                                           │                          │
│       │  5. 如果后续收到回调，返回 410 Gone       │                          │
│       │◀ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─│                          │
│       │                                           │                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.4 Webhook 请求格式（适配 IaC Platform）

根据我们平台的实际数据模型设计，请求体格式如下：

```json
{
  "payload_version": 1,
  "stage": "post_plan",
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  
  // 平台能力声明
  "capabilities": {
    "outcomes": true
  },
  
  // Run Task 结果相关
  "task_result_id": "rtr-001",
  "task_result_callback_url": "https://iac-platform.example.com/api/v1/task-results/rtr-001/callback",
  "task_result_enforcement_level": "mandatory",
  
  // Task 信息（对应 workspace_tasks 表）
  "task_id": 123,
  "task_type": "plan_and_apply",
  "task_status": "apply_pending",
  "task_description": "Deploy production infrastructure",
  "task_created_at": "2025-01-06T10:00:00Z",
  "task_created_by": "user-001",
  "task_app_url": "https://iac-platform.example.com/workspaces/ws-production/tasks/123",
  
  // Workspace 信息
  "workspace_id": "ws-production",
  "workspace_name": "production",
  "workspace_workdir": "/workspace",
  "workspace_terraform_version": "1.5.0",
  "workspace_execution_mode": "plan_and_apply",
  "workspace_app_url": "https://iac-platform.example.com/workspaces/ws-production",
  
  // 团队信息（可选）
  "team_id": "team-ops",
  
  // Plan 数据 URL（仅 post_plan/pre_apply/post_apply 阶段）
  "plan_json_api_url": "https://iac-platform.example.com/api/v1/workspaces/ws-production/tasks/123/plan-json",
  
  // Plan 变更统计（仅 post_plan/pre_apply/post_apply 阶段）
  "plan_changes": {
    "add": 5,
    "change": 2,
    "destroy": 1
  },
  
  // 资源变更列表 URL（仅 post_plan/pre_apply/post_apply 阶段）
  "resource_changes_api_url": "https://iac-platform.example.com/api/v1/workspaces/ws-production/tasks/123/resource-changes"
}
```

**请求头：**
| Header | Value | Description |
|--------|-------|-------------|
| `Content-Type` | `application/json` | 请求体类型 |
| `User-Agent` | `IaC-Platform/1.0` | 标识请求来源 |
| `X-TFC-Task-Signature` | `sha512=<signature>` | HMAC 签名（如果配置了 HMAC Key） |

**字段说明：**

| 字段 | 类型 | 阶段 | 说明 |
|------|------|------|------|
| `payload_version` | integer | 所有 | 固定为 `1` |
| `stage` | string | 所有 | `pre_plan`, `post_plan`, `pre_apply`, `post_apply` |
| `access_token` | string | 所有 | 一次性 Bearer Token，用于回调和获取数据 |
| `capabilities.outcomes` | boolean | 所有 | 平台是否支持详细的 outcomes 结果 |
| `task_result_id` | string | 所有 | Run Task Result ID |
| `task_result_callback_url` | string | 所有 | 回调 URL |
| `task_result_enforcement_level` | string | 所有 | `advisory` 或 `mandatory` |
| `task_id` | integer | 所有 | 任务 ID（workspace_tasks.id） |
| `task_type` | string | 所有 | 任务类型：`plan`, `apply`, `plan_and_apply` |
| `task_status` | string | 所有 | 任务状态：`pending`, `running`, `apply_pending` 等 |
| `task_description` | string | 所有 | 任务描述 |
| `task_created_at` | string | 所有 | 任务创建时间（ISO 8601） |
| `task_created_by` | string | 所有 | 任务创建者 |
| `task_app_url` | string | 所有 | 任务的 UI 链接 |
| `workspace_id` | string | 所有 | Workspace 语义化 ID |
| `workspace_name` | string | 所有 | Workspace 名称 |
| `workspace_workdir` | string | 所有 | Terraform 工作目录 |
| `workspace_terraform_version` | string | 所有 | Terraform 版本 |
| `workspace_execution_mode` | string | 所有 | 执行模式：`plan_only` 或 `plan_and_apply` |
| `workspace_app_url` | string | 所有 | Workspace 的 UI 链接 |
| `team_id` | string | 所有 | 团队 ID（可选） |
| `plan_json_api_url` | string | post_plan/pre_apply/post_apply | 获取 Plan JSON 的 URL |
| `plan_changes` | object | post_plan/pre_apply/post_apply | Plan 变更统计 |
| `resource_changes_api_url` | string | post_plan/pre_apply/post_apply | 获取资源变更列表的 URL |

### 4.5 回调请求格式（JSON:API 规范）

第三方服务处理完成后，需要调用 `task_result_callback_url` 返回结果。

**进度更新（running 状态）：**
```json
{
  "data": {
    "type": "task-results",
    "attributes": {
      "status": "running",
      "message": "Analyzing 15 resources..."
    }
  }
}
```

**最终结果（passed/failed 状态）：**
```json
{
  "data": {
    "type": "task-results",
    "attributes": {
      "status": "passed",
      "message": "4 passed, 0 skipped, 0 failed",
      "url": "https://external.service.dev/results/123"
    },
    "relationships": {
      "outcomes": {
        "data": [
          {
            "type": "task-result-outcomes",
            "attributes": {
              "outcome-id": "PRTNR-CC-TF-127",
              "description": "S3 Bucket encryption check passed",
              "tags": {
                "Status": [
                  { "label": "Passed", "level": "info" }
                ],
                "Severity": [
                  { "label": "Low", "level": "info" }
                ]
              },
              "body": "# S3 Bucket Encryption\n\nAll S3 buckets have encryption enabled.",
              "url": "https://external.service.dev/result/PRTNR-CC-TF-127"
            }
          },
          {
            "type": "task-result-outcomes",
            "attributes": {
              "outcome-id": "PRTNR-CC-TF-128",
              "description": "IAM Policy too permissive",
              "tags": {
                "Status": [
                  { "label": "Failed", "level": "error" }
                ],
                "Severity": [
                  { "label": "High", "level": "error" }
                ]
              },
              "body": "# IAM Policy Issue\n\n## Problem\nThe IAM policy `aws_iam_policy.admin` grants `*:*` permissions.\n\n## Recommendation\nRestrict permissions to only required actions.",
              "url": "https://external.service.dev/result/PRTNR-CC-TF-128"
            }
          }
        ]
      }
    }
  }
}
```

**Outcome 属性说明：**

| 属性 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `outcome-id` | string | 是 | 第三方服务提供的唯一标识 |
| `description` | string | 是 | 一行描述 |
| `body` | string | 否 | Markdown 格式的详细内容（建议 < 1MB，最大 5MB） |
| `url` | string | 否 | 详情链接 |
| `tags` | object | 否 | 标签对象，`severity` 和 `status` 有特殊处理 |

**Tag Level 说明：**

| Level | 说明 | 显示效果 |
|-------|------|----------|
| `none` | 默认 | 普通文本 |
| `info` | 信息 | 蓝色图标 |
| `warning` | 警告 | 黄色图标 |
| `error` | 错误 | 红色图标 |

### 4.4 HMAC 签名验证

```go
// 计算 HMAC-SHA512 签名
func calculateHMAC(payload []byte, key string) string {
    h := hmac.New(sha512.New, []byte(key))
    h.Write(payload)
    return hex.EncodeToString(h.Sum(nil))
}

// 请求头
// X-TFC-Task-Signature: sha512=<signature>
```

---

## 5. 前端界面设计

### 5.1 全局 Run Task 管理页面

**位置：** `/admin/run-tasks`

**功能：**
- 列表展示所有 Run Task
- 创建新 Run Task
- 编辑/删除 Run Task
- 查看关联的 Workspace 数量

**UI 组件：**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Run Tasks                                                    [+ Create]    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ NAME              │ ENDPOINT URL                    │ WORKSPACES │ STATUS││
│  ├───────────────────┼─────────────────────────────────┼────────────┼───────┤│
│  │ 🔗 security-scan  │ https://security.example.com/.. │     5      │ ✓     ││
│  │ 🔗 cost-estimate  │ https://cost.example.com/...    │     3      │ ✓     ││
│  │ 🔗 compliance     │ https://compliance.example.com/ │     2      │ ✗     ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 创建 Run Task 对话框

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Create a run task                                                     [X]  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Add or create a run task that will be assigned to this workspace.          │
│  Learn more about run tasks.                                                 │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ Name *                                                                  ││
│  │ Can only contain letters, numbers, dashes and underscores.              ││
│  │ ┌─────────────────────────────────────────────────────────────────────┐ ││
│  │ │ e.g. Example                                                        │ ││
│  │ └─────────────────────────────────────────────────────────────────────┘ ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ Description (Optional)                                                  ││
│  │ ┌─────────────────────────────────────────────────────────────────────┐ ││
│  │ │ e.g. A description looks like this                                  │ ││
│  │ └─────────────────────────────────────────────────────────────────────┘ ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ Endpoint URL *                                                          ││
│  │ Run Tasks will POST to this URL.                                        ││
│  │ ┌─────────────────────────────────────────────────────────────────────┐ ││
│  │ │ https://www.example.io/...                                          │ ││
│  │ └─────────────────────────────────────────────────────────────────────┘ ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ HMAC key (Optional)                                                     ││
│  │ A secret key that may be required by the service to verify request      ││
│  │ authenticity.                                                           ││
│  │ ┌─────────────────────────────────────────────────────────────────────┐ ││
│  │ │ sensitive - write only                                              │ ││
│  │ └─────────────────────────────────────────────────────────────────────┘ ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
│  ┌──────────┐  ┌──────────┐                                                 │
│  │  Create  │  │  Cancel  │                                                 │
│  └──────────┘  └──────────┘                                                 │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.3 Workspace Run Task 配置页面

**位置：** `/workspaces/:id?tab=settings&section=run-tasks`

**功能：**
- 列表展示 Workspace 关联的 Run Task
- 添加 Run Task 到 Workspace
- 配置执行阶段和执行级别
- 启用/禁用 Run Task

**UI 组件：**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Run Tasks                                                   [+ Add Task]   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Configure run tasks for this workspace. Run tasks allow external services  │
│  to pass or fail Terraform runs.                                            │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ TASK NAME        │ STAGE       │ ENFORCEMENT │ STATUS │ ACTIONS        ││
│  ├──────────────────┼─────────────┼─────────────┼────────┼────────────────┤│
│  │ 🔗 security-scan │ Post-plan   │ Mandatory   │ ✓      │ [Edit] [Delete]││
│  │ 🔗 cost-estimate │ Post-plan   │ Advisory    │ ✓      │ [Edit] [Delete]││
│  │ 🔗 notify-slack  │ Post-apply  │ Advisory    │ ✓      │ [Edit] [Delete]││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.4 配置 Run Task 对话框

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Configure Run Task                                                    [X]  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ 🔗 AI-Flow                                                        [▼]  ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
│  Run stage                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ ○ Pre-plan                                                              ││
│  │   Before Terraform generates the plan.                                  ││
│  │                                                                         ││
│  │ ● Post-plan                                                             ││
│  │   After Terraform creates the plan.                                     ││
│  │                                                                         ││
│  │ ○ Pre-apply                                                             ││
│  │   Before Terraform applies the plan.                                    ││
│  │                                                                         ││
│  │ ○ Post-apply                                                            ││
│  │   After Terraform applies the plan.                                     ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
│  Enforcement level                                                           │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ ● Advisory                                                              ││
│  │   Failed run tasks produce a warning.                                   ││
│  │                                                                         ││
│  │ ○ Mandatory                                                             ││
│  │   Failed run tasks stop the run.                                        ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
│  ┌──────────┐  ┌──────────┐                                                 │
│  │   Save   │  │  Cancel  │                                                 │
│  └──────────┘  └──────────┘                                                 │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.5 任务详情页 Run Task 结果展示

**位置：** `/workspaces/:id/tasks/:task_id`

**功能：**
- 在任务详情页展示 Run Task 执行结果
- 按阶段分组显示
- 显示状态、消息和详情链接

**UI 组件：**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Run Tasks                                                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Post-plan                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ ✓ security-scan                                              Passed     ││
│  │   No security issues found                                              ││
│  │   View details →                                                        ││
│  ├─────────────────────────────────────────────────────────────────────────┤│
│  │ ✓ cost-estimate                                              Passed     ││
│  │   Estimated monthly cost: $150                                          ││
│  │   View details →                                                        ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
│  Pre-apply                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ ⏳ approval-workflow                                         Running    ││
│  │   Waiting for approval...                                               ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 6. 后端实现设计

### 6.1 Go 模型定义

```go
// backend/internal/models/run_task.go

package models

import (
    "time"
)

// RunTaskStage 执行阶段
type RunTaskStage string

const (
    RunTaskStagePrePlan   RunTaskStage = "pre_plan"
    RunTaskStagePostPlan  RunTaskStage = "post_plan"
    RunTaskStagePreApply  RunTaskStage = "pre_apply"
    RunTaskStagePostApply RunTaskStage = "post_apply"
)

// RunTaskEnforcementLevel 执行级别
type RunTaskEnforcementLevel string

const (
    RunTaskEnforcementAdvisory  RunTaskEnforcementLevel = "advisory"
    RunTaskEnforcementMandatory RunTaskEnforcementLevel = "mandatory"
)

// RunTaskResultStatus 执行结果状态
type RunTaskResultStatus string

const (
    RunTaskResultPending RunTaskResultStatus = "pending"
    RunTaskResultRunning RunTaskResultStatus = "running"
    RunTaskResultPassed  RunTaskResultStatus = "passed"
    RunTaskResultFailed  RunTaskResultStatus = "failed"
    RunTaskResultError   RunTaskResultStatus = "error"
    RunTaskResultSkipped RunTaskResultStatus = "skipped"
)

// RunTask 全局 Run Task 定义
type RunTask struct {
    ID               uint      `json:"id" gorm:"primaryKey"`
    RunTaskID        string    `json:"run_task_id" gorm:"type:varchar(50);uniqueIndex"`
    Name             string    `json:"name" gorm:"type:varchar(100);not null"`
    Description      string    `json:"description" gorm:"type:text"`
    EndpointURL      string    `json:"endpoint_url" gorm:"type:varchar(500);not null"`
    HMACKeyEncrypted string    `json:"-" gorm:"type:text"`
    Enabled          bool      `json:"enabled" gorm:"default:true"`
    OrganizationID   *string   `json:"organization_id" gorm:"type:varchar(50);index"`
    TeamID           *string   `json:"team_id" gorm:"type:varchar(50);index"`
    CreatedBy        *string   `json:"created_by" gorm:"type:varchar(50)"`
    CreatedAt        time.Time `json:"created_at"`
    UpdatedAt        time.Time `json:"updated_at"`
}

func (RunTask) TableName() string {
    return "run_tasks"
}

// WorkspaceRunTask Workspace 关联的 Run Task
type WorkspaceRunTask struct {
    ID                 uint                    `json:"id" gorm:"primaryKey"`
    WorkspaceRunTaskID string                  `json:"workspace_run_task_id" gorm:"type:varchar(50);uniqueIndex"`
    WorkspaceID        string                  `json:"workspace_id" gorm:"type:varchar(50);not null;index"`
    RunTaskID          string                  `json:"run_task_id" gorm:"type:varchar(50);not null;index"`
    Stage              RunTaskStage            `json:"stage" gorm:"type:varchar(20);not null"`
    EnforcementLevel   RunTaskEnforcementLevel `json:"enforcement_level" gorm:"type:varchar(20);not null;default:advisory"`
    Enabled            bool                    `json:"enabled" gorm:"default:true"`
    CreatedBy          *string                 `json:"created_by" gorm:"type:varchar(50)"`
    CreatedAt          time.Time               `json:"created_at"`
    UpdatedAt          time.Time               `json:"updated_at"`

    // 关联
    RunTask   *RunTask   `json:"run_task,omitempty" gorm:"foreignKey:RunTaskID;references:RunTaskID"`
    Workspace *Workspace `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID;references:WorkspaceID"`
}

func (WorkspaceRunTask) TableName() string {
    return "workspace_run_tasks"
}

// RunTaskResult Run Task 执行结果
type RunTaskResult struct {
    ID                 uint                `json:"id" gorm:"primaryKey"`
    ResultID           string              `json:"result_id" gorm:"type:varchar(50);uniqueIndex"`
    TaskID             uint                `json:"task_id" gorm:"not null;index"`
    WorkspaceRunTaskID string              `json:"workspace_run_task_id" gorm:"type:varchar(50);not null;index"`
    Stage              RunTaskStage        `json:"stage" gorm:"type:varchar(20);not null"`
    Status             RunTaskResultStatus `json:"status" gorm:"type:varchar(20);not null;default:pending"`
    RequestPayload     JSONB               `json:"request_payload" gorm:"type:jsonb"`
    ResponsePayload    JSONB               `json:"response_payload" gorm:"type:jsonb"`
    CallbackURL        string              `json:"callback_url" gorm:"type:varchar(500)"`
    Message            string              `json:"message" gorm:"type:text"`
    URL                string              `json:"url" gorm:"type:varchar(500)"`
    StartedAt          *time.Time          `json:"started_at"`
    CompletedAt        *time.Time          `json:"completed_at"`
    CreatedAt          time.Time           `json:"created_at"`
    UpdatedAt          time.Time           `json:"updated_at"`

    // 关联
    Task             *WorkspaceTask    `json:"task,omitempty" gorm:"foreignKey:TaskID"`
    WorkspaceRunTask *WorkspaceRunTask `json:"workspace_run_task,omitempty" gorm:"foreignKey:WorkspaceRunTaskID;references:WorkspaceRunTaskID"`
}

func (RunTaskResult) TableName() string {
    return "run_task_results"
}
```

### 6.2 Run Task 执行服务

```go
// backend/services/run_task_executor.go

package services

import (
    "bytes"
    "context"
    "crypto/hmac"
    "crypto/sha512"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "iac-platform/internal/models"
    "gorm.io/gorm"
)

type RunTaskExecutor struct {
    db         *gorm.DB
    httpClient *http.Client
    baseURL    string
}

func NewRunTaskExecutor(db *gorm.DB, baseURL string) *RunTaskExecutor {
    return &RunTaskExecutor{
        db: db,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
        baseURL: baseURL,
    }
}

// ExecuteRunTasksForStage 执行指定阶段的所有 Run Task
func (e *RunTaskExecutor) ExecuteRunTasksForStage(
    ctx context.Context,
    task *models.WorkspaceTask,
    stage models.RunTaskStage,
) (bool, error) {
    // 1. 获取 Workspace 关联的该阶段的 Run Task
    var workspaceRunTasks []models.WorkspaceRunTask
    err := e.db.Preload("RunTask").
        Where("workspace_id = ? AND stage = ? AND enabled = true", task.WorkspaceID, stage).
        Find(&workspaceRunTasks).Error
    if err != nil {
        return false, fmt.Errorf("failed to get workspace run tasks: %w", err)
    }

    if len(workspaceRunTasks) == 0 {
        return true, nil // 没有配置 Run Task，直接通过
    }

    // 2. 为每个 Run Task 创建执行记录并发送请求
    allPassed := true
    for _, wrt := range workspaceRunTasks {
        if wrt.RunTask == nil || !wrt.RunTask.Enabled {
            continue
        }

        // 创建执行记录
        result := &models.RunTaskResult{
            ResultID:           generateResultID(),
            TaskID:             task.ID,
            WorkspaceRunTaskID: wrt.WorkspaceRunTaskID,
            Stage:              stage,
            Status:             models.RunTaskResultPending,
            CallbackURL:        fmt.Sprintf("%s/api/v1/run-task-results/%s/callback", e.baseURL, result.ResultID),
        }
        if err := e.db.Create(result).Error; err != nil {
            return false, fmt.Errorf("failed to create run task result: %w", err)
        }

        // 发送请求到外部服务
        passed, err := e.invokeRunTask(ctx, task, &wrt, result)
        if err != nil {
            result.Status = models.RunTaskResultError
            result.Message = err.Error()
            e.db.Save(result)
        }

        // 检查是否阻塞
        if !passed && wrt.EnforcementLevel == models.RunTaskEnforcementMandatory {
            allPassed = false
        }
    }

    return allPassed, nil
}

// invokeRunTask 调用外部 Run Task 服务
func (e *RunTaskExecutor) invokeRunTask(
    ctx context.Context,
    task *models.WorkspaceTask,
    wrt *models.WorkspaceRunTask,
    result *models.RunTaskResult,
) (bool, error) {
    // 构建请求 payload
    payload := map[string]interface{}{
        "payload_version":              1,
        "stage":                        wrt.Stage,
        "task_result_id":               result.ResultID,
        "task_result_callback_url":     result.CallbackURL,
        "task_result_enforcement_level": wrt.EnforcementLevel,
        "run_id":                       task.ID,
        "workspace_id":                 task.WorkspaceID,
        "is_speculative":               false,
    }

    // 添加 plan_json_api_url（仅 post_plan 阶段）
    if wrt.Stage == models.RunTaskStagePostPlan {
        payload["plan_json_api_url"] = fmt.Sprintf("%s/api/v1/workspaces/%s/tasks/%d/plan-json",
            e.baseURL, task.WorkspaceID, task.ID)
    }

    payloadBytes, err := json.Marshal(payload)
    if err != nil {
        return false, fmt.Errorf("failed to marshal payload: %w", err)
    }

    // 保存请求 payload
    result.RequestPayload = payload
    result.Status = models.RunTaskResultRunning
    result.StartedAt = timePtr(time.Now())
    e.db.Save(result)

    // 创建 HTTP 请求
    req, err := http.NewRequestWithContext(ctx, "POST", wrt.RunTask.EndpointURL, bytes.NewReader(payloadBytes))
    if err != nil {
        return false, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")

    // 添加 HMAC 签名（如果配置了）
    if wrt.RunTask.HMACKeyEncrypted != "" {
        hmacKey := decryptHMACKey(wrt.RunTask.HMACKeyEncrypted)
        signature := calculateHMAC(payloadBytes, hmacKey)
        req.Header.Set("X-TFC-Task-Signature", "sha512="+signature)
    }

    // 发送请求
    resp, err := e.httpClient.Do(req)
    if err != nil {
        return false, fmt.Errorf("failed to send request: %w", err)
    }
    defer resp.Body.Close()

    // 检查响应状态
    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
        return false, fmt.Errorf("external service returned status %d", resp.StatusCode)
    }

    // 对于同步响应，直接处理结果
    // 对于异步响应，等待回调
    return true, nil
}

// HandleCallback 处理外部服务的回调
func (e *RunTaskExecutor) HandleCallback(resultID string, status string, message string, url string) error {
    var result models.RunTaskResult
    if err := e.db.Where("result_id = ?", resultID).First(&result).Error; err != nil {
        return fmt.Errorf("result not found: %w", err)
    }

    result.Status = models.RunTaskResultStatus(status)
    result.Message = message
    result.URL = url
    result.CompletedAt = timePtr(time.Now())

    return e.db.Save(&result).Error
}

// calculateHMAC 计算 HMAC-SHA512 签名
func calculateHMAC(payload []byte, key string) string {
    h := hmac.New(sha512.New, []byte(key))
    h.Write(payload)
    return hex.EncodeToString(h.Sum(nil))
}
```

### 6.3 集成到任务执行流程

```go
// 在 TerraformExecutor 中集成 Run Task

func (e *TerraformExecutor) ExecutePlan(ctx context.Context, task *models.WorkspaceTask) error {
    // 1. 执行 Pre-plan Run Tasks
    passed, err := e.runTaskExecutor.ExecuteRunTasksForStage(ctx, task, models.RunTaskStagePrePlan)
    if err != nil {
        return fmt.Errorf("pre-plan run tasks failed: %w", err)
    }
    if !passed {
        return fmt.Errorf("pre-plan run tasks blocked execution")
    }

    // 2. 执行 Terraform Plan
    // ... existing plan logic ...

    // 3. 执行 Post-plan Run Tasks
    passed, err = e.runTaskExecutor.ExecuteRunTasksForStage(ctx, task, models.RunTaskStagePostPlan)
    if err != nil {
        return fmt.Errorf("post-plan run tasks failed: %w", err)
    }
    if !passed {
        return fmt.Errorf("post-plan run tasks blocked execution")
    }

    return nil
}

func (e *TerraformExecutor) ExecuteApply(ctx context.Context, task *models.WorkspaceTask) error {
    // 1. 执行 Pre-apply Run Tasks
    passed, err := e.runTaskExecutor.ExecuteRunTasksForStage(ctx, task, models.RunTaskStagePreApply)
    if err != nil {
        return fmt.Errorf("pre-apply run tasks failed: %w", err)
    }
    if !passed {
        return fmt.Errorf("pre-apply run tasks blocked execution")
    }

    // 2. 执行 Terraform Apply
    // ... existing apply logic ...

    // 3. 执行 Post-apply Run Tasks（仅 Advisory，不阻塞）
    _, _ = e.runTaskExecutor.ExecuteRunTasksForStage(ctx, task, models.RunTaskStagePostApply)

    return nil
}
```

---

## 7. 实现计划

### 7.1 Phase 1: 基础设施（1-2天）

- [ ] 创建数据库迁移脚本
- [ ] 创建 Go 模型定义
- [ ] 创建基础 CRUD API

### 7.2 Phase 2: 全局 Run Task 管理（2-3天）

- [ ] 实现 Run Task CRUD API
- [ ] 实现前端管理页面
- [ ] 实现 HMAC 密钥加密存储

### 7.3 Phase 3: Workspace Run Task 配置（2-3天）

- [ ] 实现 Workspace Run Task 关联 API
- [ ] 实现前端配置页面
- [ ] 添加 Settings 子菜单

### 7.4 Phase 4: 执行集成（3-4天）

- [ ] 实现 Run Task 执行服务
- [ ] 集成到任务执行流程
- [ ] 实现回调处理
- [ ] 实现超时和重试机制

### 7.5 Phase 5: 结果展示（2-3天）

- [ ] 实现结果查询 API
- [ ] 实现任务详情页结果展示
- [ ] 实现实时状态更新

### 7.6 Phase 6: 测试和文档（2天）

- [ ] 编写单元测试
- [ ] 编写集成测试
- [ ] 完善 API 文档

---

## 8. 安全考虑

### 8.1 HMAC 密钥管理

- HMAC 密钥使用 AES-256 加密存储
- 密钥只能写入，不能读取
- 支持密钥轮换

### 8.2 回调验证

- 回调 URL 包含唯一的 result_id
- 可选：验证回调来源 IP
- 可选：回调请求签名验证

### 8.3 访问控制

- Run Task 管理需要管理员权限
- Workspace Run Task 配置需要 Workspace 管理权限
- 回调 API 使用独立的认证机制

---

## 9. 监控和告警

### 9.1 指标

- Run Task 调用次数
- Run Task 成功/失败率
- Run Task 响应时间
- 回调超时次数

### 9.2 告警

- Run Task 连续失败告警
- 回调超时告警
- 外部服务不可用告警

---

## 10. 参考资料

- [Terraform Cloud Run Tasks](https://developer.hashicorp.com/terraform/cloud-docs/workspaces/settings/run-tasks)
- [Run Tasks Integration](https://developer.hashicorp.com/terraform/cloud-docs/integrations/run-tasks)
- [Run Tasks API](https://developer.hashicorp.com/terraform/cloud-docs/api-docs/run-tasks)

---

## 11. 实现进度跟踪

> **AI 助手必读**: 
> 1. 开始任务前，先阅读本章节了解当前进度
> 2. 完成子任务后，立即更新对应的复选框状态（`[ ]` → `[x]`）
> 3. 如果任务被中断，在"当前状态"部分记录中断点
> 4. 每个子任务完成后，在"完成记录"部分添加完成时间和备注

### 11.1 当前状态

**总体进度**: 22/22 子任务完成 (100%)

**当前阶段**: 全部完成

**最后更新**: 2025-01-07

**中断点**: 无

### 11.2 任务清单

#### Phase 1: 基础设施 (预估: 1-2天) ✅ 已完成

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 1.1 | 创建数据库迁移脚本 | ✅ | `scripts/create_run_tasks_tables.sql` | 已完成 |
| 1.2 | 执行数据库迁移 | ✅ | - | 已执行，创建了 4 个表 |
| 1.3 | 创建 Go 模型定义 | ✅ | `backend/internal/models/run_task.go` | 已完成 |

#### Phase 2: 后端 - 全局 Run Task API (预估: 2-3天) ✅ 已完成

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 2.1 | 创建 Run Task Handler | ✅ | `backend/internal/handlers/run_task_handler.go` | CRUD API |
| 2.2 | 实现 HMAC 密钥加密 | ✅ | 使用现有 `backend/internal/crypto/variable_crypto.go` | AES-256 加密 |
| 2.3 | 注册路由 | ✅ | `backend/internal/router/router_run_task.go` | 添加 /api/v1/run-tasks 路由 |
| 2.4 | 添加权限定义 | ✅ | `scripts/add_run_task_permissions.sql` | RUN_TASKS 权限 |

#### Phase 3: 后端 - Workspace Run Task API (预估: 2-3天) ✅ 已完成

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 3.1 | 创建 Workspace Run Task Handler | ✅ | `backend/internal/handlers/workspace_run_task_handler.go` | CRUD API |
| 3.2 | 注册路由 | ✅ | `backend/internal/router/router_workspace.go` | 添加 /api/v1/workspaces/:id/run-tasks 路由 |

#### Phase 4: 后端 - 执行服务 (预估: 3-4天) 🔄 进行中

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 4.1 | 创建 Run Task Executor 服务 | ✅ | `backend/services/run_task_executor.go` | 核心执行逻辑 |
| 4.2 | 创建回调 Handler | ✅ | `backend/internal/handlers/run_task_callback_handler.go` | 处理第三方回调 |
| 4.3 | 实现超时检测 | ✅ | `backend/services/run_task_timeout_checker.go` | 定时任务 |
| 4.4 | 集成到任务执行流程 | ✅ | `backend/services/terraform_executor.go` | 添加了辅助方法和结构体字段 |
| 4.5 | 创建 Access Token 服务 | ✅ | `backend/services/run_task_token_service.go` | JWT 生成和验证 |

#### Phase 5: 前端 - 全局管理页面 (预估: 2-3天) ✅ 已完成

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 5.1 | 创建 Run Task 管理页面 | ✅ | `frontend/src/pages/admin/RunTaskManagement.tsx` | CRUD 界面 |
| 5.2 | 创建 Run Task 管理样式 | ✅ | `frontend/src/pages/admin/RunTaskManagement.module.css` | - |
| 5.3 | 添加路由配置 | ✅ | `frontend/src/App.tsx` | /global/settings/run-tasks |
| 5.4 | 添加导航菜单 | ✅ | `frontend/src/components/Layout.tsx` | 全局设置菜单 |

#### Phase 6: 前端 - Workspace 配置页面 (预估: 2-3天) ✅ 已完成

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 6.1 | 创建 Workspace Run Task 配置组件 | ✅ | `frontend/src/components/WorkspaceRunTaskConfig.tsx` | 包含配置对话框 |
| 6.2 | 集成到 Workspace Settings | ✅ | `frontend/src/pages/WorkspaceSettings.tsx` | Settings > Run Tasks |

#### Phase 7: 前端 - 结果展示 (预估: 1-2天) ✅ 已完成

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 7.1 | 创建 Run Task 结果组件 | ✅ | `frontend/src/components/RunTaskResults.tsx` | 按阶段分组展示 |
| 7.2 | 集成到任务详情页 | ✅ | `frontend/src/pages/TaskDetail.tsx` | 在 triggerInfo 后显示 |

### 11.3 完成记录

| 日期 | 任务编号 | 任务名称 | 完成人 | 备注 |
|------|----------|----------|--------|------|
| 2025-01-06 | 1.1 | 创建数据库迁移脚本 | AI | `scripts/create_run_tasks_tables.sql` |
| 2025-01-06 | 1.2 | 执行数据库迁移 | AI | 创建了 run_tasks, workspace_run_tasks, run_task_results, run_task_outcomes 4 个表 |
| 2025-01-06 | 1.3 | 创建 Go 模型定义 | AI | `backend/internal/models/run_task.go` - 包含模型、响应结构、请求结构 |
| 2025-01-07 | 2.1 | 创建 Run Task Handler | AI | `backend/internal/handlers/run_task_handler.go` - CRUD API |
| 2025-01-07 | 2.2 | 实现 HMAC 密钥加密 | AI | 使用现有 crypto 包的 EncryptValue/DecryptValue |
| 2025-01-07 | 2.3 | 注册路由 | AI | `backend/internal/router/router_run_task.go` - /api/v1/run-tasks |
| 2025-01-07 | 2.4 | 添加权限定义 | AI | `scripts/add_run_task_permissions.sql` - RUN_TASKS 权限 |
| 2025-01-07 | 3.1 | 创建 Workspace Run Task Handler | AI | `backend/internal/handlers/workspace_run_task_handler.go` |
| 2025-01-07 | 3.2 | 注册路由 | AI | `backend/internal/router/router_workspace.go` - setupWorkspaceRunTaskRoutes |
| 2025-01-07 | 4.1 | 创建 Run Task Executor 服务 | AI | `backend/services/run_task_executor.go` |
| 2025-01-07 | 4.2 | 创建回调 Handler | AI | `backend/internal/handlers/run_task_callback_handler.go` |
| 2025-01-07 | 4.3 | 实现超时检测 | AI | `backend/services/run_task_timeout_checker.go` |
| 2025-01-07 | 4.5 | 创建 Access Token 服务 | AI | `backend/services/run_task_token_service.go` |
| 2025-01-07 | 5.1 | 创建 Run Task 管理页面 | AI | `frontend/src/pages/admin/RunTaskManagement.tsx` |
| 2025-01-07 | 5.2 | 创建样式文件 | AI | `frontend/src/pages/admin/RunTaskManagement.module.css` |
| 2025-01-07 | 5.3 | 添加路由配置 | AI | `frontend/src/App.tsx` - /global/settings/run-tasks |
| 2025-01-07 | 5.4 | 添加导航菜单 | AI | `frontend/src/components/Layout.tsx` - 全局设置菜单 |
| 2025-01-07 | 6.1 | 创建 Workspace Run Task 配置组件 | AI | `frontend/src/components/WorkspaceRunTaskConfig.tsx` |
| 2025-01-07 | 7.1 | 创建 Run Task 结果组件 | AI | `frontend/src/components/RunTaskResults.tsx` |
| 2025-01-07 | 6.2 | 集成到 Workspace Settings | AI | `frontend/src/pages/WorkspaceSettings.tsx` - Run Tasks tab |
| 2025-01-07 | 7.2 | 集成到任务详情页 | AI | `frontend/src/pages/TaskDetail.tsx` - RunTaskResults 组件 |
| 2025-01-07 | 4.4 | 集成到任务执行流程 | AI | `backend/services/terraform_executor.go` - 添加 runTaskExecutor 字段和辅助方法 |

### 11.4 执行指南

#### 启动新任务

```bash
# AI 助手执行以下步骤：
# 1. 阅读本文档的 "11.2 任务清单" 找到下一个待完成任务（⬜ 状态）
# 2. 执行任务
# 3. 更新任务状态为 ✅
# 4. 在 "11.3 完成记录" 添加记录
# 5. 更新 "11.1 当前状态" 的进度
```

#### 任务中断处理

如果任务被中断（如上下文窗口用尽），请：
1. 在 "11.1 当前状态" 的 "中断点" 记录当前位置
2. 记录任何未保存的重要信息

#### 继续任务

新会话开始时：
1. 阅读 "11.1 当前状态" 了解中断点
2. 阅读 "11.2 任务清单" 找到下一个待完成任务
3. 继续执行

### 11.5 文件清单

已创建的文件：
- [x] `docs/run-task/README.md` - 设计文档（本文件）
- [x] `scripts/create_run_tasks_tables.sql` - 数据库迁移脚本
- [x] `backend/internal/models/run_task.go` - Go 模型定义
- [x] `backend/internal/handlers/run_task_handler.go` - Run Task API Handler
- [x] `backend/internal/router/router_run_task.go` - Run Task 路由配置
- [x] `scripts/add_run_task_permissions.sql` - Run Task 权限定义

已创建的前端组件：
- [x] `frontend/src/pages/admin/RunTaskManagement.tsx` - 全局管理页面
- [x] `frontend/src/pages/admin/RunTaskManagement.module.css` - 管理页面样式
- [x] `frontend/src/components/WorkspaceRunTaskConfig.tsx` - Workspace 配置组件
- [x] `frontend/src/components/RunTaskResults.tsx` - 结果展示组件

已全部集成：
- [x] 将 WorkspaceRunTaskConfig 集成到 WorkspaceSettings.tsx ✅
- [x] 将 RunTaskResults 集成到 TaskDetail.tsx ✅
