# Workspace模块 - 通知系统

> **文档版本**: v1.0  
> **创建日期**: 2025-10-09  
> **状态**: 第一版基础Webhook，第二版完整系统

## 📘 概述

通知系统负责在关键事件发生时通知用户和外部系统。第一版实现基础Webhook通知，第二版扩展到Prometheus、Loki、S3等多种目标。

## 🎯 第一版：基础Webhook通知

### 核心事件

**9个关键事件**:
1. `workspace_created` - Workspace创建
2. `plan_started` - Plan任务开始
3. `plan_completed` - Plan任务完成
4. `plan_failed` - Plan任务失败
5. `apply_started` - Apply任务开始
6. `apply_completed` - Apply任务完成
7. `apply_failed` - Apply任务失败
8. `drift_detected` - 检测到漂移（第二版）
9. `drift_resolved` - 漂移已修复（第二版）

### Webhook配置

**数据模型**:
```go
type WebhookConfig struct {
    ID          uint     `json:"id"`
    WorkspaceID uint     `json:"workspace_id"`
    Name        string   `json:"name"`
    URL         string   `json:"url"`
    Events      []string `json:"events"` // 订阅的事件列表
    Secret      string   `json:"secret"` // 用于签名验证
    Enabled     bool     `json:"enabled"`
    CreatedAt   time.Time `json:"created_at"`
}
```

**配置示例**:
```json
{
  "name": "Slack Notification",
  "url": "https://hooks.slack.com/services/xxx/yyy/zzz",
  "events": ["plan_completed", "apply_completed", "apply_failed"],
  "secret": "webhook_secret_key",
  "enabled": true
}
```

### Payload格式

**通用结构**:
```json
{
  "event": "apply_completed",
  "timestamp": "2025-10-09T10:00:00Z",
  "workspace": {
    "id": 1,
    "name": "production-infra",
    "state": "completed"
  },
  "task": {
    "id": 123,
    "type": "apply",
    "status": "success",
    "duration": 45.2
  },
  "user": {
    "id": 1,
    "email": "admin@example.com"
  },
  "metadata": {}
}
```

### 签名验证

**HMAC-SHA256签名**:
```go
func (s *NotificationService) SignPayload(payload []byte, secret string) string {
    h := hmac.New(sha256.New, []byte(secret))
    h.Write(payload)
    return hex.EncodeToString(h.Sum(nil))
}

func (s *NotificationService) SendWebhook(config *WebhookConfig, event Event) error {
    payload, _ := json.Marshal(event)
    signature := s.SignPayload(payload, config.Secret)
    
    req, _ := http.NewRequest("POST", config.URL, bytes.NewBuffer(payload))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Webhook-Signature", signature)
    req.Header.Set("X-Event-Type", event.Type)
    
    resp, err := http.DefaultClient.Do(req)
    return err
}
```

### 重试机制

**重试策略**:
- 最多重试3次
- 指数退避：1s, 2s, 4s
- 记录失败日志

```go
func (s *NotificationService) SendWithRetry(config *WebhookConfig, event Event) error {
    maxRetries := 3
    backoff := time.Second
    
    for i := 0; i < maxRetries; i++ {
        err := s.SendWebhook(config, event)
        if err == nil {
            return nil
        }
        
        if i < maxRetries-1 {
            time.Sleep(backoff)
            backoff *= 2
        }
    }
    
    return errors.New("max retries exceeded")
}
```

## 🚀 第二版：完整通知系统

### 多种通知目标

#### 1. Prometheus

**用途**: 指标监控

**指标示例**:
```
workspace_plan_duration_seconds{workspace="prod"} 45.2
workspace_apply_success_total{workspace="prod"} 100
workspace_apply_failure_total{workspace="prod"} 5
```

#### 2. Loki

**用途**: 日志聚合

**日志格式**:
```json
{
  "stream": {
    "workspace": "production-infra",
    "event": "apply_completed"
  },
  "values": [
    ["1696838400000000000", "Apply completed successfully"]
  ]
}
```

#### 3. S3

**用途**: 报告存储

**存储路径**:
```
s3://reports-bucket/workspaces/{workspace_id}/reports/{date}/{report_id}.json
```

#### 4. Email

**用途**: 邮件通知

**模板**:
```html
<h2>Workspace Apply Completed</h2>
<p>Workspace: production-infra</p>
<p>Status: Success</p>
<p>Duration: 45.2s</p>
```

#### 5. Slack/Teams

**用途**: 团队协作通知

**消息格式**:
```json
{
  "text": "Apply completed for production-infra",
  "attachments": [{
    "color": "good",
    "fields": [
      {"title": "Status", "value": "Success"},
      {"title": "Duration", "value": "45.2s"}
    ]
  }]
}
```

### 事件路由

**路由配置**:
```yaml
notification_targets:
  - name: slack
    type: webhook
    url: https://hooks.slack.com/xxx
    events: [apply_completed, apply_failed]
    
  - name: prometheus
    type: prometheus
    endpoint: http://prometheus:9090
    events: [plan_completed, apply_completed]
    
  - name: loki
    type: loki
    endpoint: http://loki:3100
    events: [*]  # 所有事件
    
  - name: s3-reports
    type: s3
    bucket: reports-bucket
    events: [plan_completed, apply_completed]
```

## 📊 API接口

### Webhook管理

```http
# 创建Webhook
POST /api/v1/workspaces/:id/webhooks
{
  "name": "Slack Notification",
  "url": "https://hooks.slack.com/xxx",
  "events": ["apply_completed"],
  "secret": "xxx"
}

# 获取Webhook列表
GET /api/v1/workspaces/:id/webhooks

# 更新Webhook
PUT /api/v1/workspaces/:id/webhooks/:webhook_id

# 删除Webhook
DELETE /api/v1/workspaces/:id/webhooks/:webhook_id

# 测试Webhook
POST /api/v1/workspaces/:id/webhooks/:webhook_id/test
```

### 通知历史

```http
# 获取通知历史
GET /api/v1/workspaces/:id/notifications
?event=apply_completed&status=success&limit=50

# 重试失败的通知
POST /api/v1/workspaces/:id/notifications/:notification_id/retry
```

## 🔧 实现示例

### NotificationService

```go
type NotificationService struct {
    db      *gorm.DB
    targets map[string]NotificationTarget
}

type NotificationTarget interface {
    Send(event Event) error
    Name() string
}

func (s *NotificationService) Send(eventType string, data interface{}) error {
    event := Event{
        Type:      eventType,
        Timestamp: time.Now(),
        Data:      data,
    }
    
    // 获取订阅此事件的所有Webhook
    var webhooks []WebhookConfig
    s.db.Where("enabled = ? AND ? = ANY(events)", true, eventType).
        Find(&webhooks)
    
    // 异步发送通知
    for _, webhook := range webhooks {
        go func(wh WebhookConfig) {
            err := s.SendWithRetry(&wh, event)
            if err != nil {
                log.Error("Failed to send webhook:", err)
            }
        }(webhook)
    }
    
    return nil
}
```

## 📝 最佳实践

### 1. 安全性
- 使用HTTPS
- 验证签名
- 限制重试次数
- 记录审计日志

### 2. 可靠性
- 异步发送
- 重试机制
- 超时控制
- 错误处理

### 3. 性能
- 批量发送
- 连接池
- 限流控制

### 4. 监控
- 发送成功率
- 响应时间
- 失败原因

---

**相关文档**:
- [00-overview.md](./00-overview.md) - 总览和架构
- [04-task-workflow.md](./04-task-workflow.md) - 任务工作流
- [05-drift-detection.md](./05-drift-detection.md) - 漂移检测
