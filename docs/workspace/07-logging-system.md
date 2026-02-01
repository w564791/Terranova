# Workspace模块 - 日志系统

> **文档版本**: v1.0  
> **创建日期**: 2025-10-09  
> **状态**: 第一版基础日志，第二版完整系统

## 📘 概述

日志系统负责记录、存储和查询Workspace和任务执行的所有日志。第一版实现基础日志功能，第二版扩展到Elasticsearch、Loki、S3等多后端支持。

## 🎯 第一版：基础日志系统

### 日志类型

**4种日志类型**:
1. **任务日志**: Plan/Apply执行日志
2. **系统日志**: 平台系统日志
3. **审计日志**: 用户操作审计
4. **错误日志**: 错误和异常

### 日志结构

**统一格式**:
```json
{
  "timestamp": "2025-10-09T10:00:00.123Z",
  "level": "info",
  "source": "task_worker",
  "workspace_id": 1,
  "task_id": 123,
  "message": "Executing terraform plan",
  "metadata": {
    "execution_mode": "local",
    "terraform_version": "1.6.0"
  }
}
```

### 日志级别

```go
type LogLevel string

const (
    LogLevelDebug   LogLevel = "debug"
    LogLevelInfo    LogLevel = "info"
    LogLevelWarning LogLevel = "warning"
    LogLevelError   LogLevel = "error"
    LogLevelFatal   LogLevel = "fatal"
)
```

### 数据模型

```go
type TaskLog struct {
    ID          uint      `json:"id"`
    WorkspaceID uint      `json:"workspace_id"`
    TaskID      uint      `json:"task_id"`
    Level       LogLevel  `json:"level"`
    Source      string    `json:"source"`
    Message     string    `json:"message"`
    Metadata    JSONB     `json:"metadata"`
    CreatedAt   time.Time `json:"created_at"`
}
```

### 日志记录

**Logger接口**:
```go
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warning(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    Fatal(msg string, fields ...Field)
}

type Field struct {
    Key   string
    Value interface{}
}
```

**使用示例**:
```go
logger.Info("Starting terraform plan",
    Field{"workspace_id", workspaceID},
    Field{"task_id", taskID},
    Field{"execution_mode", "local"},
)
```

### 日志存储

**第一版：PostgreSQL**
- 简单实现
- 易于查询
- 适合中小规模

**存储策略**:
- 保留最近30天
- 自动清理旧日志
- 压缩归档

```go
func (s *LogService) CleanOldLogs() error {
    cutoff := time.Now().AddDate(0, 0, -30)
    return s.db.Where("created_at < ?", cutoff).
        Delete(&TaskLog{}).Error
}
```

## 📊 API接口

### 日志查询

```http
# 获取任务日志
GET /api/v1/workspaces/:id/tasks/:task_id/logs
?level=info&limit=100&offset=0

# 实时日志流（WebSocket）
WS /api/v1/workspaces/:id/tasks/:task_id/logs/stream

# 下载日志
GET /api/v1/workspaces/:id/tasks/:task_id/logs/download

# 搜索日志
POST /api/v1/workspaces/:id/logs/search
{
  "query": "error",
  "start_time": "2025-10-09T00:00:00Z",
  "end_time": "2025-10-09T23:59:59Z",
  "level": "error"
}
```

### 响应格式

```json
{
  "logs": [
    {
      "timestamp": "2025-10-09T10:00:00Z",
      "level": "info",
      "message": "Terraform plan completed",
      "metadata": {}
    }
  ],
  "total": 150,
  "has_more": true
}
```

## 🚀 第二版：完整日志系统

### 多后端支持

#### 1. Elasticsearch

**用途**: 全文搜索和分析

**索引结构**:
```json
{
  "mappings": {
    "properties": {
      "timestamp": {"type": "date"},
      "level": {"type": "keyword"},
      "workspace_id": {"type": "integer"},
      "task_id": {"type": "integer"},
      "message": {"type": "text"},
      "metadata": {"type": "object"}
    }
  }
}
```

**查询示例**:
```json
{
  "query": {
    "bool": {
      "must": [
        {"match": {"message": "error"}},
        {"range": {"timestamp": {"gte": "now-1h"}}}
      ]
    }
  }
}
```

#### 2. Loki

**用途**: 轻量级日志聚合

**标签**:
```
{workspace="prod", task="123", level="error"}
```

**LogQL查询**:
```
{workspace="prod"} |= "error" | json | line_format "{{.message}}"
```

#### 3. S3

**用途**: 长期归档

**存储路径**:
```
s3://logs-bucket/workspaces/{workspace_id}/tasks/{task_id}/{date}/logs.json.gz
```

**归档策略**:
- 30天后归档到S3
- 压缩存储
- 生命周期管理

#### 4. HTTPS转发

**用途**: 转发到外部系统

**配置**:
```yaml
log_forwarding:
  - name: splunk
    type: https
    endpoint: https://splunk.example.com/services/collector
    headers:
      Authorization: "Splunk xxx"
    batch_size: 100
    flush_interval: 10s
```

### 日志路由

**路由配置**:
```yaml
log_backends:
  - name: postgres
    type: postgres
    retention_days: 30
    levels: [debug, info, warning, error, fatal]
    
  - name: elasticsearch
    type: elasticsearch
    endpoint: http://elasticsearch:9200
    index: workspace-logs
    levels: [warning, error, fatal]
    
  - name: loki
    type: loki
    endpoint: http://loki:3100
    levels: [info, warning, error, fatal]
    
  - name: s3-archive
    type: s3
    bucket: logs-archive
    retention_days: 365
    levels: [error, fatal]
```

## 🔍 日志分析

### 实时监控

**监控指标**:
- 错误率
- 日志量
- 响应时间
- 存储使用

**告警规则**:
```yaml
alerts:
  - name: high_error_rate
    condition: error_rate > 10%
    duration: 5m
    action: send_notification
    
  - name: log_storage_full
    condition: storage_usage > 90%
    action: cleanup_old_logs
```

### 日志聚合

**聚合查询**:
```sql
SELECT 
    DATE_TRUNC('hour', created_at) as hour,
    level,
    COUNT(*) as count
FROM task_logs
WHERE workspace_id = 1
GROUP BY hour, level
ORDER BY hour DESC
```

### 日志可视化

**Grafana Dashboard**:
- 日志量趋势
- 错误率图表
- 日志级别分布
- Top错误消息

## 🔧 实现示例

### LogService

```go
type LogService struct {
    db       *gorm.DB
    backends []LogBackend
}

type LogBackend interface {
    Write(log *TaskLog) error
    Query(filter LogFilter) ([]TaskLog, error)
    Name() string
}

func (s *LogService) Write(log *TaskLog) error {
    // 写入所有后端
    for _, backend := range s.backends {
        go func(b LogBackend) {
            if err := b.Write(log); err != nil {
                log.Printf("Failed to write to %s: %v", b.Name(), err)
            }
        }(backend)
    }
    return nil
}

func (s *LogService) StreamLogs(taskID uint, ch chan<- *TaskLog) error {
    // 实时流式传输日志
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    
    lastID := uint(0)
    
    for range ticker.C {
        var logs []TaskLog
        err := s.db.Where("task_id = ? AND id > ?", taskID, lastID).
            Order("id ASC").
            Limit(100).
            Find(&logs).Error
        
        if err != nil {
            return err
        }
        
        for _, log := range logs {
            ch <- &log
            lastID = log.ID
        }
    }
    
    return nil
}
```

### WebSocket日志流

```go
func (c *TaskController) StreamLogs(ctx *gin.Context) {
    ws, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
    if err != nil {
        return
    }
    defer ws.Close()
    
    taskID := ctx.GetUint("task_id")
    logChan := make(chan *TaskLog, 100)
    
    go logService.StreamLogs(taskID, logChan)
    
    for log := range logChan {
        if err := ws.WriteJSON(log); err != nil {
            break
        }
    }
}
```

## 📝 最佳实践

### 1. 日志格式
- 使用结构化日志（JSON）
- 包含上下文信息
- 统一时间格式（ISO 8601）
- 添加关联ID

### 2. 性能优化
- 异步写入
- 批量处理
- 缓冲队列
- 压缩存储

### 3. 安全性
- 敏感信息脱敏
- 访问控制
- 加密传输
- 审计追踪

### 4. 运维
- 定期清理
- 监控告警
- 备份恢复
- 容量规划

---

**相关文档**:
- [00-overview.md](./00-overview.md) - 总览和架构
- [04-task-workflow.md](./04-task-workflow.md) - 任务工作流
- [06-notification-system.md](./06-notification-system.md) - 通知系统
