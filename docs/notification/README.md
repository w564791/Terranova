# 通知系统功能设计文档

> **AI 助手注意**: 
> - 任务进度跟踪位于本文档末尾的 **"11. 实现进度跟踪"** 章节
> - 每完成一个子任务后，请更新对应的复选框状态
> - 开始新任务前，请先阅读进度跟踪章节了解当前状态

## 1. 概述

通知系统是 IaC 平台的一个核心功能，允许在 Terraform 运行（Run）生命周期的特定阶段发送通知到外部服务。该功能的设计模式参考了 Run Task 的实现，支持 Global 级别和 Workspace 级别的配置。

### 1.1 核心概念

- **Notification Configuration（通知配置）**：在全局级别定义的通知服务集成，包含名称、类型、Endpoint URL、认证配置等
- **Global Notification（全局通知）**：自动应用于所有 Workspace 的通知配置
- **Workspace Notification（工作空间通知）**：将通知配置应用到特定 Workspace，可以选择性添加或覆盖全局配置

**通知类型（第一版本）：**
- **Webhook**：普通 HTTP POST 请求，支持自定义 Headers
- **Lark Robot**：飞书/Lark 机器人，支持签名验证（HMAC-SHA256）

### 1.2 触发事件

| 事件 | 说明 | 触发时机 |
|------|------|----------|
| **task_created** | 任务创建 | 新任务创建时 |
| **task_planning** | 开始 Plan | Terraform Plan 开始时 |
| **task_planned** | Plan 完成 | Terraform Plan 完成时 |
| **task_applying** | 开始 Apply | Terraform Apply 开始时 |
| **task_completed** | 任务完成 | 任务成功完成时 |
| **task_failed** | 任务失败 | 任务执行失败时 |
| **task_cancelled** | 任务取消 | 任务被取消时 |
| **approval_required** | 需要审批 | 任务需要人工审批时 |
| **approval_timeout** | 审批超时 | 审批等待超时时 |
| **drift_detected** | 检测到漂移 | 检测到资源漂移时 |

### 1.3 工作流程概述

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     Notification System Workflow Overview                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. 配置阶段 (Setup)                                                         │
│     ├── 创建 Notification Configuration（全局级别）                          │
│     ├── 配置通知类型（Webhook/Lark Robot）                                   │
│     ├── 配置认证方式（HMAC/Token/自定义Headers）                             │
│     └── 关联到 Workspace（指定触发事件）或设为全局                           │
│                                                                              │
│  2. 触发阶段 (Trigger)                                                       │
│     ├── 任务状态变更时检查通知配置                                           │
│     ├── 收集通知数据（任务信息、Workspace信息、变更统计等）                  │
│     ├── 根据通知类型构建请求体                                               │
│     └── 异步发送通知（不阻塞主流程）                                         │
│                                                                              │
│  3. 发送阶段 (Send)                                                          │
│     ├── 根据通知类型添加认证信息                                             │
│     ├── 发送 HTTP POST 请求                                                  │
│     ├── 记录发送结果（成功/失败）                                            │
│     └── 失败时支持重试（可配置）                                             │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. 数据库设计

### 2.1 Notification Configuration 表（通知配置）

```sql
-- 通知配置表
CREATE TABLE IF NOT EXISTS notification_configs (
    id SERIAL PRIMARY KEY,
    notification_id VARCHAR(50) UNIQUE NOT NULL,  -- 语义化ID，如 "notif-lark-ops"
    name VARCHAR(100) NOT NULL,                    -- 名称，只能包含字母、数字、破折号和下划线
    description TEXT,                              -- 描述（可选）
    
    -- 通知类型
    notification_type VARCHAR(20) NOT NULL,        -- 类型: webhook, lark_robot
    
    -- Endpoint 配置
    endpoint_url VARCHAR(500) NOT NULL,            -- Endpoint URL
    
    -- 认证配置（根据类型不同使用不同字段）
    -- Webhook: 可选的 HMAC 密钥
    -- Lark Robot: 签名密钥（secret）
    secret_encrypted TEXT,                         -- 密钥（加密存储，可选）
    
    -- 自定义 Headers（JSON 格式）
    -- 默认包含 Content-Type: application/json
    custom_headers JSONB DEFAULT '{"Content-Type": "application/json"}',
    
    -- 状态
    enabled BOOLEAN DEFAULT true,                  -- 是否启用
    
    -- 全局配置
    is_global BOOLEAN DEFAULT false,               -- 是否为全局通知（自动应用于所有 Workspace）
    
    -- 全局通知默认触发事件（仅当 is_global=true 时有效）
    -- 逗号分隔，如 "task_completed,task_failed"
    global_events VARCHAR(500) DEFAULT 'task_completed,task_failed',
    
    -- 重试配置
    retry_count INTEGER DEFAULT 3,                 -- 重试次数
    retry_interval_seconds INTEGER DEFAULT 30,     -- 重试间隔（秒）
    
    -- 超时配置
    timeout_seconds INTEGER DEFAULT 30,            -- 请求超时（秒）
    
    -- 组织/团队归属
    organization_id VARCHAR(50),                   -- 组织ID（可选）
    team_id VARCHAR(50),                           -- 团队ID（可选）
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 约束
    CONSTRAINT notification_configs_name_check CHECK (name ~ '^[a-zA-Z0-9_-]+$'),
    CONSTRAINT notification_configs_type_check CHECK (notification_type IN ('webhook', 'lark_robot')),
    CONSTRAINT notification_configs_timeout_check CHECK (timeout_seconds >= 5 AND timeout_seconds <= 120),
    CONSTRAINT notification_configs_retry_check CHECK (retry_count >= 0 AND retry_count <= 10)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_notification_configs_name ON notification_configs(name);
CREATE INDEX IF NOT EXISTS idx_notification_configs_type ON notification_configs(notification_type);
CREATE INDEX IF NOT EXISTS idx_notification_configs_organization ON notification_configs(organization_id);
CREATE INDEX IF NOT EXISTS idx_notification_configs_team ON notification_configs(team_id);
CREATE INDEX IF NOT EXISTS idx_notification_configs_enabled ON notification_configs(enabled);
CREATE INDEX IF NOT EXISTS idx_notification_configs_is_global ON notification_configs(is_global) WHERE is_global = true;

COMMENT ON TABLE notification_configs IS '通知配置表，存储通知服务集成配置';
COMMENT ON COLUMN notification_configs.notification_id IS '语义化ID，如 notif-lark-ops';
COMMENT ON COLUMN notification_configs.notification_type IS '通知类型: webhook(普通Webhook), lark_robot(飞书机器人)';
COMMENT ON COLUMN notification_configs.secret_encrypted IS '密钥（AES-256加密存储），Webhook用于HMAC签名，Lark Robot用于签名验证';
COMMENT ON COLUMN notification_configs.custom_headers IS '自定义HTTP Headers，JSON格式';
COMMENT ON COLUMN notification_configs.is_global IS '是否为全局通知，自动应用于所有 Workspace';
COMMENT ON COLUMN notification_configs.global_events IS '全局通知默认触发事件，逗号分隔';
```

### 2.2 Workspace Notification 表（工作空间通知关联）

```sql
-- Workspace 通知关联表
CREATE TABLE IF NOT EXISTS workspace_notifications (
    id SERIAL PRIMARY KEY,
    workspace_notification_id VARCHAR(50) UNIQUE NOT NULL,  -- 语义化ID
    workspace_id VARCHAR(50) NOT NULL,                       -- 关联的 Workspace ID
    notification_id VARCHAR(50) NOT NULL,                    -- 关联的 Notification ID
    
    -- 触发事件配置（逗号分隔）
    -- 如 "task_completed,task_failed,approval_required"
    events VARCHAR(500) NOT NULL DEFAULT 'task_completed,task_failed',
    
    -- 状态
    enabled BOOLEAN DEFAULT true,
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 外键约束
    CONSTRAINT fk_workspace_notifications_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    CONSTRAINT fk_workspace_notifications_notification FOREIGN KEY (notification_id) 
        REFERENCES notification_configs(notification_id) ON DELETE CASCADE,
    
    -- 唯一约束：同一个 workspace 的同一个 notification 只能配置一次
    UNIQUE(workspace_id, notification_id)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_workspace_notifications_workspace ON workspace_notifications(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_notifications_notification ON workspace_notifications(notification_id);
CREATE INDEX IF NOT EXISTS idx_workspace_notifications_enabled ON workspace_notifications(enabled);

COMMENT ON TABLE workspace_notifications IS 'Workspace 通知关联表，配置 Workspace 使用的通知';
COMMENT ON COLUMN workspace_notifications.events IS '触发事件，逗号分隔，如 task_completed,task_failed';
```

### 2.3 Notification Log 表（通知发送记录）

```sql
-- 通知发送记录表
CREATE TABLE IF NOT EXISTS notification_logs (
    id SERIAL PRIMARY KEY,
    log_id VARCHAR(50) UNIQUE NOT NULL,            -- 语义化ID
    
    -- 关联
    task_id BIGINT,                                -- 关联的 workspace_task ID（可选）
    workspace_id VARCHAR(50),                      -- 关联的 Workspace ID
    notification_id VARCHAR(50) NOT NULL,          -- 关联的 Notification ID
    workspace_notification_id VARCHAR(50),         -- 关联的 Workspace Notification ID（可选，全局通知时为空）
    
    -- 事件信息
    event VARCHAR(50) NOT NULL,                    -- 触发事件
    
    -- 发送状态
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- 状态: pending, sending, success, failed
    
    -- 请求/响应
    request_payload JSONB,                         -- 发送的请求体
    request_headers JSONB,                         -- 发送的请求头（脱敏后）
    response_status_code INTEGER,                  -- 响应状态码
    response_body TEXT,                            -- 响应体（截断保存）
    error_message TEXT,                            -- 错误信息
    
    -- 重试信息
    retry_count INTEGER DEFAULT 0,                 -- 已重试次数
    next_retry_at TIMESTAMP,                       -- 下次重试时间
    
    -- 时间
    sent_at TIMESTAMP,                             -- 发送时间
    completed_at TIMESTAMP,                        -- 完成时间
    
    -- 元数据
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 外键约束
    CONSTRAINT fk_notification_logs_notification FOREIGN KEY (notification_id) 
        REFERENCES notification_configs(notification_id) ON DELETE CASCADE,
    
    -- 约束
    CONSTRAINT notification_logs_status_check CHECK (status IN ('pending', 'sending', 'success', 'failed'))
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_notification_logs_task ON notification_logs(task_id);
CREATE INDEX IF NOT EXISTS idx_notification_logs_workspace ON notification_logs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_notification_logs_notification ON notification_logs(notification_id);
CREATE INDEX IF NOT EXISTS idx_notification_logs_event ON notification_logs(event);
CREATE INDEX IF NOT EXISTS idx_notification_logs_status ON notification_logs(status);
CREATE INDEX IF NOT EXISTS idx_notification_logs_created_at ON notification_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_notification_logs_next_retry ON notification_logs(next_retry_at) WHERE status = 'failed' AND next_retry_at IS NOT NULL;

COMMENT ON TABLE notification_logs IS '通知发送记录表，存储每次通知发送的结果';
COMMENT ON COLUMN notification_logs.status IS '状态: pending(等待), sending(发送中), success(成功), failed(失败)';
COMMENT ON COLUMN notification_logs.retry_count IS '已重试次数';
COMMENT ON COLUMN notification_logs.next_retry_at IS '下次重试时间，用于重试调度';
```

---

## 3. API 设计

### 3.1 Notification Configuration 管理 API（全局）

#### 3.1.1 创建 Notification Configuration

```
POST /api/v1/notifications
```

**请求体（Webhook 类型）：**
```json
{
  "name": "ops-webhook",
  "description": "Operations team webhook notification",
  "notification_type": "webhook",
  "endpoint_url": "https://ops.example.com/webhook/iac",
  "secret": "optional-hmac-secret",
  "custom_headers": {
    "Content-Type": "application/json",
    "X-Custom-Header": "custom-value"
  },
  "is_global": false,
  "retry_count": 3,
  "retry_interval_seconds": 30,
  "timeout_seconds": 30,
  "organization_id": "org-default"
}
```

**请求体（Lark Robot 类型）：**
```json
{
  "name": "lark-ops-bot",
  "description": "Lark robot for ops team",
  "notification_type": "lark_robot",
  "endpoint_url": "https://open.larksuite.com/open-apis/bot/v2/hook/0933679c-a1b4-444e-b497-7d7760b35d67",
  "secret": "1mWfXHVFomCovwdhc13mxf",
  "custom_headers": {
    "Content-Type": "application/json"
  },
  "is_global": true,
  "global_events": "task_completed,task_failed",
  "retry_count": 3,
  "timeout_seconds": 30,
  "organization_id": "org-default"
}
```

**响应：**
```json
{
  "notification_id": "notif-lark-ops-bot",
  "name": "lark-ops-bot",
  "description": "Lark robot for ops team",
  "notification_type": "lark_robot",
  "endpoint_url": "https://open.larksuite.com/open-apis/bot/v2/hook/0933679c-a1b4-444e-b497-7d7760b35d67",
  "secret_set": true,
  "custom_headers": {
    "Content-Type": "application/json"
  },
  "enabled": true,
  "is_global": true,
  "global_events": "task_completed,task_failed",
  "retry_count": 3,
  "retry_interval_seconds": 30,
  "timeout_seconds": 30,
  "organization_id": "org-default",
  "workspace_count": 0,
  "created_at": "2025-01-06T10:00:00Z"
}
```

#### 3.1.2 获取 Notification Configuration 列表

```
GET /api/v1/notifications?organization_id=org-default&page=1&page_size=20
```

**响应：**
```json
{
  "notifications": [
    {
      "notification_id": "notif-lark-ops-bot",
      "name": "lark-ops-bot",
      "description": "Lark robot for ops team",
      "notification_type": "lark_robot",
      "endpoint_url": "https://open.larksuite.com/open-apis/bot/v2/hook/****",
      "secret_set": true,
      "enabled": true,
      "is_global": true,
      "global_events": "task_completed,task_failed",
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

#### 3.1.3 获取单个 Notification Configuration

```
GET /api/v1/notifications/:notification_id
```

#### 3.1.4 更新 Notification Configuration

```
PUT /api/v1/notifications/:notification_id
```

**请求体：**
```json
{
  "name": "lark-ops-bot-v2",
  "description": "Updated Lark robot",
  "endpoint_url": "https://open.larksuite.com/open-apis/bot/v2/hook/new-hook-id",
  "secret": "new-secret",
  "enabled": true,
  "is_global": true,
  "global_events": "task_completed,task_failed,approval_required"
}
```

#### 3.1.5 删除 Notification Configuration

```
DELETE /api/v1/notifications/:notification_id
```

#### 3.1.6 测试 Notification Configuration

```
POST /api/v1/notifications/:notification_id/test
```

**请求体：**
```json
{
  "event": "task_completed",
  "test_message": "This is a test notification from IaC Platform"
}
```

**响应：**
```json
{
  "success": true,
  "status_code": 200,
  "response_time_ms": 150,
  "message": "Test notification sent successfully"
}
```

### 3.2 Workspace Notification API

#### 3.2.1 为 Workspace 添加 Notification

```
POST /api/v1/workspaces/:workspace_id/notifications
```

**请求体：**
```json
{
  "notification_id": "notif-lark-ops-bot",
  "events": "task_completed,task_failed,approval_required"
}
```

**响应：**
```json
{
  "workspace_notification_id": "wn-ws001-lark-ops-bot",
  "workspace_id": "ws-001",
  "notification_id": "notif-lark-ops-bot",
  "notification_name": "lark-ops-bot",
  "notification_type": "lark_robot",
  "events": "task_completed,task_failed,approval_required",
  "enabled": true,
  "created_at": "2025-01-06T10:00:00Z"
}
```

#### 3.2.2 获取 Workspace 的 Notification 列表

```
GET /api/v1/workspaces/:workspace_id/notifications
```

**响应：**
```json
{
  "workspace_notifications": [
    {
      "workspace_notification_id": "wn-ws001-lark-ops-bot",
      "notification": {
        "notification_id": "notif-lark-ops-bot",
        "name": "lark-ops-bot",
        "notification_type": "lark_robot",
        "description": "Lark robot for ops team"
      },
      "events": "task_completed,task_failed,approval_required",
      "enabled": true,
      "is_global": false
    },
    {
      "workspace_notification_id": null,
      "notification": {
        "notification_id": "notif-global-webhook",
        "name": "global-webhook",
        "notification_type": "webhook",
        "description": "Global webhook notification"
      },
      "events": "task_completed,task_failed",
      "enabled": true,
      "is_global": true
    }
  ]
}
```

#### 3.2.3 更新 Workspace Notification

```
PUT /api/v1/workspaces/:workspace_id/notifications/:workspace_notification_id
```

**请求体：**
```json
{
  "events": "task_completed,task_failed",
  "enabled": true
}
```

#### 3.2.4 删除 Workspace Notification

```
DELETE /api/v1/workspaces/:workspace_id/notifications/:workspace_notification_id
```

### 3.3 Notification Log API

#### 3.3.1 获取 Workspace 的通知日志

```
GET /api/v1/workspaces/:workspace_id/notification-logs?page=1&page_size=20
```

**响应：**
```json
{
  "logs": [
    {
      "log_id": "nlog-001",
      "task_id": 123,
      "notification_name": "lark-ops-bot",
      "notification_type": "lark_robot",
      "event": "task_completed",
      "status": "success",
      "response_status_code": 200,
      "sent_at": "2025-01-06T10:01:00Z",
      "completed_at": "2025-01-06T10:01:01Z"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 1
  }
}
```

#### 3.3.2 获取任务的通知日志

```
GET /api/v1/workspaces/:workspace_id/tasks/:task_id/notification-logs
```

---

## 4. 通知类型详细设计

### 4.1 Webhook 类型

#### 4.1.1 请求格式

```
POST {endpoint_url}
Content-Type: application/json
X-IaC-Event: task_completed
X-IaC-Signature: sha256=<signature>  (如果配置了 secret)
{custom_headers}
```

**请求体：**
```json
{
  "event": "task_completed",
  "timestamp": "2025-01-06T10:00:00Z",
  "task": {
    "id": 123,
    "type": "plan_and_apply",
    "status": "completed",
    "description": "Deploy production infrastructure",
    "created_by": "user-001",
    "created_at": "2025-01-06T09:50:00Z",
    "completed_at": "2025-01-06T10:00:00Z",
    "app_url": "https://iac-platform.example.com/workspaces/ws-production/tasks/123"
  },
  "workspace": {
    "id": "ws-production",
    "name": "production",
    "terraform_version": "1.5.0",
    "app_url": "https://iac-platform.example.com/workspaces/ws-production"
  },
  "changes": {
    "add": 5,
    "change": 2,
    "destroy": 1
  },
  "organization_id": "org-default",
  "team_id": "team-ops"
}
```

#### 4.1.2 HMAC 签名（可选）

如果配置了 `secret`，将使用 HMAC-SHA256 计算签名：

```go
// 计算 HMAC-SHA256 签名
func calculateWebhookSignature(payload []byte, secret string) string {
    h := hmac.New(sha256.New, []byte(secret))
    h.Write(payload)
    return "sha256=" + hex.EncodeToString(h.Sum(nil))
}
```

签名将添加到请求头 `X-IaC-Signature` 中。

### 4.2 Lark Robot 类型

#### 4.2.1 签名计算

Lark Robot 使用特殊的签名方式，需要在请求体中包含 `timestamp` 和 `sign` 字段：

```go
// Lark Robot 签名计算
func GenLarkSign(secret string, timestamp int64) (string, error) {
    // timestamp + key 做 sha256，然后 base64 编码
    stringToSign := fmt.Sprintf("%v", timestamp) + "\n" + secret
    
    var data []byte
    h := hmac.New(sha256.New, []byte(stringToSign))
    _, err := h.Write(data)
    if err != nil {
        return "", err
    }
    
    signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
    return signature, nil
}
```

#### 4.2.2 请求格式

```
POST {endpoint_url}
Content-Type: application/json
```

**请求体（消息卡片格式）：**
```json
{
  "timestamp": "1599360473",
  "sign": "xxxxxxxxxxxxxxxxxxxxx",
  "msg_type": "interactive",
  "card": {
    "header": {
      "title": {
        "content": "🚀 IaC Platform - Task Completed",
        "tag": "plain_text"
      },
      "template": "green"
    },
    "elements": [
      {
        "tag": "div",
        "text": {
          "content": "**Workspace:** production\n**Task:** #123 - Deploy production infrastructure\n**Status:** ✅ Completed\n**Changes:** +5 ~2 -1",
          "tag": "lark_md"
        }
      },
      {
        "tag": "hr"
      },
      {
        "tag": "div",
        "text": {
          "content": "**Created by:** user-001\n**Duration:** 10 minutes",
          "tag": "lark_md"
        }
      },
      {
        "tag": "action",
        "actions": [
          {
            "tag": "button",
            "text": {
              "content": "View Details",
              "tag": "lark_md"
            },
            "url": "https://iac-platform.example.com/workspaces/ws-production/tasks/123",
            "type": "primary"
          }
        ]
      }
    ]
  }
}
```

#### 4.2.3 不同事件的消息模板

**任务完成（task_completed）- 绿色主题：**
```json
{
  "header": {
    "title": { "content": "✅ Task Completed", "tag": "plain_text" },
    "template": "green"
  }
}
```

**任务失败（task_failed）- 红色主题：**
```json
{
  "header": {
    "title": { "content": "❌ Task Failed", "tag": "plain_text" },
    "template": "red"
  }
}
```

**需要审批（approval_required）- 橙色主题：**
```json
{
  "header": {
    "title": { "content": "⏳ Approval Required", "tag": "plain_text" },
    "template": "orange"
  }
}
```

**开始执行（task_planning/task_applying）- 蓝色主题：**
```json
{
  "header": {
    "title": { "content": "🔄 Task In Progress", "tag": "plain_text" },
    "template": "blue"
  }
}
```

---

## 5. 后端实现设计

### 5.1 Go 模型定义

```go
// backend/internal/models/notification.go

package models

import (
    "time"
)

// NotificationType 通知类型
type NotificationType string

const (
    NotificationTypeWebhook   NotificationType = "webhook"
    NotificationTypeLarkRobot NotificationType = "lark_robot"
)

// NotificationEvent 通知事件
type NotificationEvent string

const (
    NotificationEventTaskCreated       NotificationEvent = "task_created"
    NotificationEventTaskPlanning      NotificationEvent = "task_planning"
    NotificationEventTaskPlanned       NotificationEvent = "task_planned"
    NotificationEventTaskApplying      NotificationEvent = "task_applying"
    NotificationEventTaskCompleted     NotificationEvent = "task_completed"
    NotificationEventTaskFailed        NotificationEvent = "task_failed"
    NotificationEventTaskCancelled     NotificationEvent = "task_cancelled"
    NotificationEventApprovalRequired  NotificationEvent = "approval_required"
    NotificationEventApprovalTimeout   NotificationEvent = "approval_timeout"
    NotificationEventDriftDetected     NotificationEvent = "drift_detected"
)

// NotificationLogStatus 通知日志状态
type NotificationLogStatus string

const (
    NotificationLogStatusPending NotificationLogStatus = "pending"
    NotificationLogStatusSending NotificationLogStatus = "sending"
    NotificationLogStatusSuccess NotificationLogStatus = "success"
    NotificationLogStatusFailed  NotificationLogStatus = "failed"
)

// NotificationConfig 通知配置
type NotificationConfig struct {
    ID             uint             `json:"id" gorm:"primaryKey"`
    NotificationID string           `json:"notification_id" gorm:"column:notification_id;type:varchar(50);uniqueIndex"`
    Name           string           `json:"name" gorm:"type:varchar(100);not null"`
    Description    string           `json:"description" gorm:"type:text"`
    
    // 通知类型
    NotificationType NotificationType `json:"notification_type" gorm:"type:varchar(20);not null"`
    
    // Endpoint 配置
    EndpointURL string `json:"endpoint_url" gorm:"type:varchar(500);not null"`
    
    // 认证配置
    SecretEncrypted string `json:"-" gorm:"column:secret_encrypted;type:text"`
    
    // 自定义 Headers
    CustomHeaders JSONB `json:"custom_headers" gorm:"type:jsonb;default:'{\"Content-Type\": \"application/json\"}'"`
    
    // 状态
    Enabled bool `json:"enabled" gorm:"default:true"`
    
    // 全局配置
    IsGlobal     bool   `json:"is_global" gorm:"default:false"`
    GlobalEvents string `json:"global_events" gorm:"type:varchar(500);default:'task_completed,task_failed'"`
    
    // 重试配置
    RetryCount           int `json:"retry_count" gorm:"default:3"`
    RetryIntervalSeconds int `json:"retry_interval_seconds" gorm:"default:30"`
    
    // 超时配置
    TimeoutSeconds int `json:"timeout_seconds" gorm:"default:30"`
    
    // 组织/团队归属
    OrganizationID *string `json:"organization_id" gorm:"type:varchar(50);index"`
    TeamID         *string `json:"team_id" gorm:"type:varchar(50);index"`
    
    // 元数据
    CreatedBy *string   `json:"created_by" gorm:"type:varchar(50)"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

func (NotificationConfig) TableName() string {
    return "notification_configs"
}

// NotificationConfigResponse API 响应结构
type NotificationConfigResponse struct {
    ID                   uint             `json:"id"`
    NotificationID       string           `json:"notification_id"`
    Name                 string           `json:"name"`
    Description          string           `json:"description"`
    NotificationType     NotificationType `json:"notification_type"`
    EndpointURL          string           `json:"endpoint_url"`
    SecretSet            bool             `json:"secret_set"`
    CustomHeaders        JSONB            `json:"custom_headers"`
    Enabled              bool             `json:"enabled"`
    IsGlobal             bool             `json:"is_global"`
    GlobalEvents         string           `json:"global_events,omitempty"`
    RetryCount           int              `json:"retry_count"`
    RetryIntervalSeconds int              `json:"retry_interval_seconds"`
    TimeoutSeconds       int              `json:"timeout_seconds"`
    OrganizationID       *string          `json:"organization_id"`
    TeamID               *string          `json:"team_id"`
    WorkspaceCount       int              `json:"workspace_count"`
    CreatedBy            *string          `json:"created_by"`
    CreatedAt            time.Time        `json:"created_at"`
    UpdatedAt            time.Time        `json:"updated_at"`
}

// ToResponse 转换为 API 响应
func (n *NotificationConfig) ToResponse(workspaceCount int) NotificationConfigResponse {
    return NotificationConfigResponse{
        ID:                   n.ID,
        NotificationID:       n.NotificationID,
        Name:                 n.Name,
        Description:          n.Description,
        NotificationType:     n.NotificationType,
        EndpointURL:          n.EndpointURL,
        SecretSet:            n.SecretEncrypted != "",
        CustomHeaders:        n.CustomHeaders,
        Enabled:              n.Enabled,
        IsGlobal:             n.IsGlobal,
        GlobalEvents:         n.GlobalEvents,
        RetryCount:           n.RetryCount,
        RetryIntervalSeconds: n.RetryIntervalSeconds,
        TimeoutSeconds:       n.TimeoutSeconds,
        OrganizationID:       n.OrganizationID,
        TeamID:               n.TeamID,
        WorkspaceCount:       workspaceCount,
        CreatedBy:            n.CreatedBy,
        CreatedAt:            n.CreatedAt,
        UpdatedAt:            n.UpdatedAt,
    }
}

// WorkspaceNotification Workspace 通知关联
type WorkspaceNotification struct {
    ID                      uint      `json:"id" gorm:"primaryKey"`
    WorkspaceNotificationID string    `json:"workspace_notification_id" gorm:"column:workspace_notification_id;type:varchar(50);uniqueIndex"`
    WorkspaceID             string    `json:"workspace_id" gorm:"type:varchar(50);not null;index"`
    NotificationID          string    `json:"notification_id" gorm:"type:varchar(50);not null;index"`
    Events                  string    `json:"events" gorm:"type:varchar(500);not null;default:'task_completed,task_failed'"`
    Enabled                 bool      `json:"enabled" gorm:"default:true"`
    CreatedBy               *string   `json:"created_by" gorm:"type:varchar(50)"`
    CreatedAt               time.Time `json:"created_at"`
    UpdatedAt               time.Time `json:"updated_at"`
    
    // 关联
    Notification *NotificationConfig `json:"notification,omitempty" gorm:"foreignKey:NotificationID;references:NotificationID"`
    Workspace    *Workspace          `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID;references:WorkspaceID"`
}

func (WorkspaceNotification) TableName() string {
    return "workspace_notifications"
}

// NotificationLog 通知发送记录
type NotificationLog struct {
    ID                      uint                  `json:"id" gorm:"primaryKey"`
    LogID                   string                `json:"log_id" gorm:"column:log_id;type:varchar(50);uniqueIndex"`
    TaskID                  *uint                 `json:"task_id" gorm:"index"`
    WorkspaceID             *string               `json:"workspace_id" gorm:"type:varchar(50);index"`
    NotificationID          string                `json:"notification_id" gorm:"type:varchar(50);not null;index"`
    WorkspaceNotificationID *string               `json:"workspace_notification_id" gorm:"type:varchar(50);index"`
    Event                   NotificationEvent     `json:"event" gorm:"type:varchar(50);not null"`
    Status                  NotificationLogStatus `json:"status" gorm:"type:varchar(20);not null;default:pending"`
    RequestPayload          JSONB                 `json:"request_payload" gorm:"type:jsonb"`
    RequestHeaders          JSONB                 `json:"request_headers" gorm:"type:jsonb"`
    ResponseStatusCode      *int                  `json:"response_status_code"`
    ResponseBody            string                `json:"response_body" gorm:"type:text"`
    ErrorMessage            string                `json:"error_message" gorm:"type:text"`
    RetryCount              int                   `json:"retry_count" gorm:"default:0"`
    NextRetryAt             *time.Time            `json:"next_retry_at"`
    SentAt                  *time.Time            `json:"sent_at"`
    CompletedAt             *time.Time            `json:"completed_at"`
    CreatedAt               time.Time             `json:"created_at"`
    UpdatedAt               time.Time             `json:"updated_at"`
    
    // 关联
    Notification *NotificationConfig `json:"notification,omitempty" gorm:"foreignKey:NotificationID;references:NotificationID"`
}

func (NotificationLog) TableName() string {
    return "notification_logs"
}

// CreateNotificationRequest 创建通知配置请求
type CreateNotificationRequest struct {
    Name                 string           `json:"name" binding:"required"`
    Description          string           `json:"description"`
    NotificationType     NotificationType `json:"notification_type" binding:"required"`
    EndpointURL          string           `json:"endpoint_url" binding:"required"`
    Secret               string           `json:"secret"`
    CustomHeaders        map[string]string `json:"custom_headers"`
    IsGlobal             bool             `json:"is_global"`
    GlobalEvents         string           `json:"global_events"`
    RetryCount           int              `json:"retry_count"`
    RetryIntervalSeconds int              `json:"retry_interval_seconds"`
    TimeoutSeconds       int              `json:"timeout_seconds"`
    OrganizationID       *string          `json:"organization_id"`
    TeamID               *string          `json:"team_id"`
}

// UpdateNotificationRequest 更新通知配置请求
type UpdateNotificationRequest struct {
    Name                 *string            `json:"name"`
    Description          *string            `json:"description"`
    EndpointURL          *string            `json:"endpoint_url"`
    Secret               *string            `json:"secret"`
    CustomHeaders        *map[string]string `json:"custom_headers"`
    Enabled              *bool              `json:"enabled"`
    IsGlobal             *bool              `json:"is_global"`
    GlobalEvents         *string            `json:"global_events"`
    RetryCount           *int               `json:"retry_count"`
    RetryIntervalSeconds *int               `json:"retry_interval_seconds"`
    TimeoutSeconds       *int               `json:"timeout_seconds"`
}

// CreateWorkspaceNotificationRequest 创建 Workspace 通知请求
type CreateWorkspaceNotificationRequest struct {
    NotificationID string `json:"notification_id" binding:"required"`
    Events         string `json:"events"`
}

// UpdateWorkspaceNotificationRequest 更新 Workspace 通知请求
type UpdateWorkspaceNotificationRequest struct {
    Events  *string `json:"events"`
    Enabled *bool   `json:"enabled"`
}

// TestNotificationRequest 测试通知请求
type TestNotificationRequest struct {
    Event       string `json:"event"`
    TestMessage string `json:"test_message"`
}
```

### 5.2 通知发送服务

```go
// backend/services/notification_sender.go

package services

import (
    "bytes"
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"

    "iac-platform/internal/models"
    "gorm.io/gorm"
)

type NotificationSender struct {
    db         *gorm.DB
    httpClient *http.Client
    baseURL    string
}

func NewNotificationSender(db *gorm.DB, baseURL string) *NotificationSender {
    return &NotificationSender{
        db:      db,
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}

// SendNotification 发送通知
func (s *NotificationSender) SendNotification(
    ctx context.Context,
    config *models.NotificationConfig,
    event models.NotificationEvent,
    task *models.WorkspaceTask,
    workspace *models.Workspace,
) error {
    // 创建日志记录
    log := &models.NotificationLog{
        LogID:          generateLogID(),
        NotificationID: config.NotificationID,
        Event:          event,
        Status:         models.NotificationLogStatusPending,
    }
    if task != nil {
        log.TaskID = &task.ID
    }
    if workspace != nil {
        log.WorkspaceID = &workspace.WorkspaceID
    }
    
    if err := s.db.Create(log).Error; err != nil {
        return fmt.Errorf("failed to create notification log: %w", err)
    }
    
    // 根据通知类型发送
    var err error
    switch config.NotificationType {
    case models.NotificationTypeWebhook:
        err = s.sendWebhook(ctx, config, event, task, workspace, log)
    case models.NotificationTypeLarkRobot:
        err = s.sendLarkRobot(ctx, config, event, task, workspace, log)
    default:
        err = fmt.Errorf("unsupported notification type: %s", config.NotificationType)
    }
    
    return err
}

// sendWebhook 发送 Webhook 通知
func (s *NotificationSender) sendWebhook(
    ctx context.Context,
    config *models.NotificationConfig,
    event models.NotificationEvent,
    task *models.WorkspaceTask,
    workspace *models.Workspace,
    log *models.NotificationLog,
) error {
    // 构建请求体
    payload := s.buildWebhookPayload(event, task, workspace)
    payloadBytes, err := json.Marshal(payload)
    if err != nil {
        return s.updateLogError(log, err)
    }
    
    log.RequestPayload = payload
    log.Status = models.NotificationLogStatusSending
    log.SentAt = timePtr(time.Now())
    s.db.Save(log)
    
    // 创建请求
    req, err := http.NewRequestWithContext(ctx, "POST", config.EndpointURL, bytes.NewReader(payloadBytes))
    if err != nil {
        return s.updateLogError(log, err)
    }
    
    // 设置 Headers
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-IaC-Event", string(event))
    
    // 添加自定义 Headers
    if config.CustomHeaders != nil {
        for key, value := range config.CustomHeaders {
            if v, ok := value.(string); ok {
                req.Header.Set(key, v)
            }
        }
    }
    
    // 添加 HMAC 签名
    if config.SecretEncrypted != "" {
        secret := decryptSecret(config.SecretEncrypted)
        signature := s.calculateWebhookSignature(payloadBytes, secret)
        req.Header.Set("X-IaC-Signature", signature)
    }
    
    // 发送请求
    resp, err := s.httpClient.Do(req)
    if err != nil {
        return s.updateLogError(log, err)
    }
    defer resp.Body.Close()
    
    // 更新日志
    log.ResponseStatusCode = &resp.StatusCode
    if resp.StatusCode >= 200 && resp.StatusCode < 300 {
        log.Status = models.NotificationLogStatusSuccess
    } else {
        log.Status = models.NotificationLogStatusFailed
        log.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
    }
    log.CompletedAt = timePtr(time.Now())
    
    return s.db.Save(log).Error
}

// sendLarkRobot 发送 Lark Robot 通知
func (s *NotificationSender) sendLarkRobot(
    ctx context.Context,
    config *models.NotificationConfig,
    event models.NotificationEvent,
    task *models.WorkspaceTask,
    workspace *models.Workspace,
    log *models.NotificationLog,
) error {
    // 构建 Lark 消息卡片
    payload := s.buildLarkCardPayload(event, task, workspace)
    
    // 添加签名（如果配置了 secret）
    if config.SecretEncrypted != "" {
        secret := decryptSecret(config.SecretEncrypted)
        timestamp := time.Now().Unix()
        sign, err := s.genLarkSign(secret, timestamp)
        if err != nil {
            return s.updateLogError(log, err)
        }
        payload["timestamp"] = fmt.Sprintf("%d", timestamp)
        payload["sign"] = sign
    }
    
    payloadBytes, err := json.Marshal(payload)
    if err != nil {
        return s.updateLogError(log, err)
    }
    
    log.RequestPayload = payload
    log.Status = models.NotificationLogStatusSending
    log.SentAt = timePtr(time.Now())
    s.db.Save(log)
    
    // 创建请求
    req, err := http.NewRequestWithContext(ctx, "POST", config.EndpointURL, bytes.NewReader(payloadBytes))
    if err != nil {
        return s.updateLogError(log, err)
    }
    
    req.Header.Set("Content-Type", "application/json")
    
    // 发送请求
    resp, err := s.httpClient.Do(req)
    if err != nil {
        return s.updateLogError(log, err)
    }
    defer resp.Body.Close()
    
    // 更新日志
    log.ResponseStatusCode = &resp.StatusCode
    if resp.StatusCode >= 200 && resp.StatusCode < 300 {
        log.Status = models.NotificationLogStatusSuccess
    } else {
        log.Status = models.NotificationLogStatusFailed
        log.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
    }
    log.CompletedAt = timePtr(time.Now())
    
    return s.db.Save(log).Error
}

// genLarkSign 生成 Lark 签名
func (s *NotificationSender) genLarkSign(secret string, timestamp int64) (string, error) {
    stringToSign := fmt.Sprintf("%v", timestamp) + "\n" + secret
    
    var data []byte
    h := hmac.New(sha256.New, []byte(stringToSign))
    _, err := h.Write(data)
    if err != nil {
        return "", err
    }
    
    signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
    return signature, nil
}

// calculateWebhookSignature 计算 Webhook HMAC 签名
func (s *NotificationSender) calculateWebhookSignature(payload []byte, secret string) string {
    h := hmac.New(sha256.New, []byte(secret))
    h.Write(payload)
    return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// buildWebhookPayload 构建 Webhook 请求体
func (s *NotificationSender) buildWebhookPayload(
    event models.NotificationEvent,
    task *models.WorkspaceTask,
    workspace *models.Workspace,
) map[string]interface{} {
    payload := map[string]interface{}{
        "event":     event,
        "timestamp": time.Now().Format(time.RFC3339),
    }
    
    if task != nil {
        payload["task"] = map[string]interface{}{
            "id":          task.ID,
            "type":        task.TaskType,
            "status":      task.Status,
            "description": task.Description,
            "created_by":  task.CreatedBy,
            "created_at":  task.CreatedAt,
            "app_url":     fmt.Sprintf("%s/workspaces/%s/tasks/%d", s.baseURL, task.WorkspaceID, task.ID),
        }
    }
    
    if workspace != nil {
        payload["workspace"] = map[string]interface{}{
            "id":                workspace.WorkspaceID,
            "name":              workspace.Name,
            "terraform_version": workspace.TerraformVersion,
            "app_url":           fmt.Sprintf("%s/workspaces/%s", s.baseURL, workspace.WorkspaceID),
        }
    }
    
    return payload
}

// buildLarkCardPayload 构建 Lark 消息卡片
func (s *NotificationSender) buildLarkCardPayload(
    event models.NotificationEvent,
    task *models.WorkspaceTask,
    workspace *models.Workspace,
) map[string]interface{} {
    // 根据事件类型选择主题颜色和标题
    var title, template string
    switch event {
    case models.NotificationEventTaskCompleted:
        title = "✅ Task Completed"
        template = "green"
    case models.NotificationEventTaskFailed:
        title = "❌ Task Failed"
        template = "red"
    case models.NotificationEventApprovalRequired:
        title = "⏳ Approval Required"
        template = "orange"
    case models.NotificationEventTaskPlanning, models.NotificationEventTaskApplying:
        title = "🔄 Task In Progress"
        template = "blue"
    default:
        title = "📢 IaC Platform Notification"
        template = "blue"
    }
    
    // 构建内容
    var contentParts []string
    if workspace != nil {
        contentParts = append(contentParts, fmt.Sprintf("**Workspace:** %s", workspace.Name))
    }
    if task != nil {
        contentParts = append(contentParts, fmt.Sprintf("**Task:** #%d", task.ID))
        if task.Description != "" {
            contentParts = append(contentParts, fmt.Sprintf("**Description:** %s", task.Description))
        }
        contentParts = append(contentParts, fmt.Sprintf("**Status:** %s", task.Status))
    }
    
    content := strings.Join(contentParts, "\n")
    
    // 构建卡片
    card := map[string]interface{}{
        "header": map[string]interface{}{
            "title": map[string]interface{}{
                "content": title,
                "tag":     "plain_text",
            },
            "template": template,
        },
        "elements": []interface{}{
            map[string]interface{}{
                "tag": "div",
                "text": map[string]interface{}{
                    "content": content,
                    "tag":     "lark_md",
                },
            },
        },
    }
    
    // 添加查看详情按钮
    if task != nil && workspace != nil {
        card["elements"] = append(card["elements"].([]interface{}),
            map[string]interface{}{
                "tag": "action",
                "actions": []interface{}{
                    map[string]interface{}{
                        "tag": "button",
                        "text": map[string]interface{}{
                            "content": "View Details",
                            "tag":     "lark_md",
                        },
                        "url":  fmt.Sprintf("%s/workspaces/%s/tasks/%d", s.baseURL, workspace.WorkspaceID, task.ID),
                        "type": "primary",
                    },
                },
            },
        )
    }
    
    return map[string]interface{}{
        "msg_type": "interactive",
        "card":     card,
    }
}

// updateLogError 更新日志错误状态
func (s *NotificationSender) updateLogError(log *models.NotificationLog, err error) error {
    log.Status = models.NotificationLogStatusFailed
    log.ErrorMessage = err.Error()
    log.CompletedAt = timePtr(time.Now())
    s.db.Save(log)
    return err
}
```

---

## 6. 前端界面设计

### 6.1 全局通知管理页面

**位置：** `/global/settings/notifications`

**功能：**
- 列表展示所有通知配置
- 创建新通知配置
- 编辑/删除通知配置
- 测试通知发送
- 查看关联的 Workspace 数量

### 6.2 Workspace 通知配置页面

**位置：** `/workspaces/:id?tab=settings&section=notifications`

**功能：**
- 列表展示 Workspace 关联的通知（包括全局通知）
- 添加通知到 Workspace
- 配置触发事件
- 启用/禁用通知

### 6.3 通知日志页面

**位置：** `/workspaces/:id/notification-logs`

**功能：**
- 查看通知发送历史
- 按事件类型筛选
- 查看发送详情（请求/响应）

---

## 7. 实现计划

### 7.1 Phase 1: 基础设施（1-2天）

- [ ] 创建数据库迁移脚本
- [ ] 创建 Go 模型定义
- [ ] 创建基础 CRUD API

### 7.2 Phase 2: 全局通知管理（2-3天）

- [ ] 实现 Notification Configuration CRUD API
- [ ] 实现前端管理页面
- [ ] 实现密钥加密存储
- [ ] 实现测试通知功能

### 7.3 Phase 3: Workspace 通知配置（2-3天）

- [ ] 实现 Workspace Notification 关联 API
- [ ] 实现前端配置页面
- [ ] 添加 Settings 子菜单

### 7.4 Phase 4: 通知发送服务（3-4天）

- [ ] 实现 Webhook 发送
- [ ] 实现 Lark Robot 发送（含签名）
- [ ] 实现重试机制
- [ ] 集成到任务执行流程

### 7.5 Phase 5: 日志和监控（2天）

- [ ] 实现通知日志 API
- [ ] 实现前端日志查看页面
- [ ] 添加发送统计

---

## 8. 安全考虑

### 8.1 密钥管理

- Secret 使用 AES-256 加密存储
- 密钥只能写入，不能读取
- API 响应中只返回 `secret_set: true/false`

### 8.2 请求验证

- Webhook 支持 HMAC-SHA256 签名
- Lark Robot 使用官方签名算法
- 支持自定义 Headers 用于额外认证

### 8.3 访问控制

- 通知配置管理需要管理员权限
- Workspace 通知配置需要 Workspace 管理权限
- 日志查看需要 Workspace 读取权限

---

## 9. 监控和告警

### 9.1 指标

- 通知发送次数（按类型、事件分组）
- 通知成功/失败率
- 通知响应时间
- 重试次数统计

### 9.2 告警

- 通知连续失败告警
- 响应超时告警
- 外部服务不可用告警

---

## 10. 参考资料

- [Lark Custom Bot Usage Guide](https://open.larksuite.com/document/client-docs/bot-v3/add-custom-bot?lang=en-US)
- [Webhook Best Practices](https://webhooks.fyi/best-practices)

---

## 11. 实现进度跟踪

> **AI 助手必读**: 
> 1. 开始任务前，先阅读本章节了解当前进度
> 2. 完成子任务后，立即更新对应的复选框状态（`[ ]` → `[x]`）
> 3. 如果任务被中断，在"当前状态"部分记录中断点
> 4. 每个子任务完成后，在"完成记录"部分添加完成时间和备注

### 11.1 当前状态

**总体进度**: 8/20 子任务完成 (40%)

**当前阶段**: Phase 1-3 完成，开始 Phase 4 通知发送服务

**最后更新**: 2025-12-12

**中断点**: 无

### 11.2 任务清单

#### Phase 1: 基础设施 (预估: 1-2天) ✅ 完成

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 1.1 | 创建数据库迁移脚本 | ✅ | `scripts/create_notification_tables.sql` | 已完成 |
| 1.2 | 执行数据库迁移 | ✅ | - | 已完成 |
| 1.3 | 创建 Go 模型定义 | ✅ | `backend/internal/models/notification.go` | 已完成 |

#### Phase 2: 后端 - 全局通知 API (预估: 2-3天) ✅ 完成

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 2.1 | 创建 Notification Handler | ✅ | `backend/internal/handlers/notification_handler.go` | CRUD API |
| 2.2 | 实现密钥加密 | ✅ | 使用现有 crypto 包 | AES-256 加密 |
| 2.3 | 注册路由 | ✅ | `backend/internal/router/router_notification.go` | 已完成 |
| 2.4 | 添加权限定义 | ⬜ | `scripts/add_notification_permissions.sql` | 待完成 |
| 2.5 | 实现测试通知 API | ⬜ | | 待集成 NotificationSender |

#### Phase 3: 后端 - Workspace 通知 API (预估: 2-3天) ✅ 完成

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 3.1 | 创建 Workspace Notification Handler | ✅ | `backend/internal/handlers/workspace_notification_handler.go` | 已完成 |
| 3.2 | 注册路由 | ✅ | `backend/internal/router/router_workspace.go` | 已完成 |

#### Phase 4: 后端 - 通知发送服务 (预估: 3-4天)

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 4.1 | 创建 Notification Sender 服务 | ⬜ | `backend/services/notification_sender.go` | 核心发送逻辑 |
| 4.2 | 实现 Webhook 发送 | ⬜ | | 含 HMAC 签名 |
| 4.3 | 实现 Lark Robot 发送 | ⬜ | | 含签名验证 |
| 4.4 | 实现重试机制 | ⬜ | `backend/services/notification_retry_worker.go` | 后台重试 |
| 4.5 | 集成到任务执行流程 | ⬜ | `backend/services/terraform_executor.go` | |

#### Phase 5: 前端 - 全局管理页面 (预估: 2-3天)

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 5.1 | 创建 Notification 管理页面 | ⬜ | `frontend/src/pages/admin/NotificationManagement.tsx` | CRUD 界面 |
| 5.2 | 创建样式文件 | ⬜ | `frontend/src/pages/admin/NotificationManagement.module.css` | |
| 5.3 | 添加路由配置 | ⬜ | `frontend/src/App.tsx` | /global/settings/notifications |
| 5.4 | 添加导航菜单 | ⬜ | `frontend/src/components/Layout.tsx` | |

#### Phase 6: 前端 - Workspace 配置页面 (预估: 2天)

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 6.1 | 创建 Workspace Notification 配置组件 | ⬜ | `frontend/src/components/WorkspaceNotificationConfig.tsx` | |
| 6.2 | 集成到 Workspace Settings | ⬜ | `frontend/src/pages/WorkspaceSettings.tsx` | |

#### Phase 7: 前端 - 日志查看 (预估: 1天)

| # | 任务 | 状态 | 文件路径 | 备注 |
|---|------|------|----------|------|
| 7.1 | 创建 Notification Log 组件 | ⬜ | `frontend/src/components/NotificationLogs.tsx` | |
| 7.2 | 集成到任务详情页 | ⬜ | `frontend/src/pages/TaskDetail.tsx` | |

### 11.3 完成记录

| 日期 | 任务编号 | 任务名称 | 完成人 | 备注 |
|------|----------|----------|--------|------|
| 2025-12-12 | - | 设计文档编写 | AI | `docs/notification/README.md` |
| 2025-12-12 | 1.1-1.3 | Phase 1 基础设施 | AI | 数据库表、Go模型 |
| 2025-12-12 | 2.1-2.3 | Phase 2 全局通知 API | AI | Handler、路由 |
| 2025-12-12 | 3.1-3.2 | Phase 3 Workspace 通知 API | AI | Handler、路由 |

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

待创建的文件：
- [ ] `docs/notification/README.md` - 设计文档（本文件）✅ 已创建
- [ ] `scripts/create_notification_tables.sql` - 数据库迁移脚本
- [ ] `backend/internal/models/notification.go` - Go 模型定义
- [ ] `backend/internal/handlers/notification_handler.go` - Notification API Handler
- [ ] `backend/internal/handlers/workspace_notification_handler.go` - Workspace Notification Handler
- [ ] `backend/internal/router/router_notification.go` - Notification 路由配置
- [ ] `backend/services/notification_sender.go` - 通知发送服务
- [ ] `backend/services/notification_retry_worker.go` - 重试工作器
- [ ] `scripts/add_notification_permissions.sql` - 权限定义

待创建的前端组件：
- [ ] `frontend/src/pages/admin/NotificationManagement.tsx` - 全局管理页面
- [ ] `frontend/src/pages/admin/NotificationManagement.module.css` - 管理页面样式
- [ ] `frontend/src/components/WorkspaceNotificationConfig.tsx` - Workspace 配置组件
- [ ] `frontend/src/components/NotificationLogs.tsx` - 日志查看组件
