# Terraform输出实时流式传输设计

> **文档版本**: v1.0  
> **创建日期**: 2025-10-11  
> **状态**: 设计完成，待实施  
> **优先级**: P0（最高优先级）

## 📘 概述

本文档设计Terraform执行输出的实时流式传输方案，使用户能够在IaC平台控制台实时查看terraform plan/apply的执行进度和输出。

## 🎯 需求分析

### 核心需求

1. **实时性**：用户需要实时看到terraform执行进度
2. **多用户**：多个用户可能同时查看同一个任务的输出
3. **完整性**：输出不能丢失，需要保存完整日志
4. **性能**：不能影响terraform执行性能
5. **可靠性**：连接断开后能够重连并继续查看

### 与系统日志的区别

| 特性 | Terraform输出流 | 系统日志（07文档） |
|------|----------------|-------------------|
| 目的 | 用户实时查看执行进度 | 平台运维和监控 |
| 内容 | terraform stdout/stderr | 平台操作审计、系统事件 |
| 用户 | 平台用户（开发者） | 运维人员、监控系统 |
| 传输 | WebSocket实时流 | HTTP API + 外部系统 |
| 存储 | task.plan_output/apply_output | task_logs表 + ES/Loki |
| 实时性 | <100ms | 不要求实时 |

## 🏗️ 架构设计

### 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                         前端浏览器                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  用户A       │  │  用户B       │  │  用户C       │      │
│  │  WebSocket   │  │  WebSocket   │  │  WebSocket   │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
└─────────┼──────────────────┼──────────────────┼─────────────┘
          │                  │                  │
          │    WebSocket连接（多个客户端）      │
          │                  │                  │
┌─────────┼──────────────────┼──────────────────┼─────────────┐
│         ▼                  ▼                  ▼              │
│  ┌────────────────────────────────────────────────────┐     │
│  │         WebSocket Controller                       │     │
│  │  - 管理客户端连接                                   │     │
│  │  - 订阅/取消订阅输出流                              │     │
│  └────────────────────┬───────────────────────────────┘     │
│                       │                                      │
│                       ▼                                      │
│  ┌────────────────────────────────────────────────────┐     │
│  │         OutputStream Manager                       │     │
│  │  - 管理所有任务的输出流                             │     │
│  │  - 广播消息到所有订阅者                             │     │
│  │  - 缓冲历史消息（最近1000行）                       │     │
│  └────────────────────┬───────────────────────────────┘     │
│                       │                                      │
│                       ▼                                      │
│  ┌────────────────────────────────────────────────────┐     │
│  │         Terraform Executor                         │     │
│  │  - 使用Pipe实时捕获stdout/stderr                   │     │
│  │  - 逐行读取并广播                                   │     │
│  │  - 同时保存完整输出到数据库                         │     │
│  └────────────────────┬───────────────────────────────┘     │
│                       │                                      │
│                       ▼                                      │
│              ┌─────────────────┐                             │
│              │ Terraform进程   │                             │
│              │ (plan/apply)    │                             │
│              └─────────────────┘                             │
└─────────────────────────────────────────────────────────────┘
```

### 数据流

```
Terraform进程 
    ↓ stdout/stderr
实时捕获（Pipe + Scanner）
    ↓ 逐行读取
OutputStream Manager
    ├─→ 广播到所有WebSocket客户端（实时）
    ├─→ 保存到内存缓冲区（最近1000行）
    └─→ 保存到数据库（完整输出）
```

## 📊 核心组件设计

### 1. OutputStream（输出流）

```go
// OutputStream 单个任务的输出流
type OutputStream struct {
    TaskID      uint
    Clients     map[string]*Client    // clientID -> Client
    Buffer      *RingBuffer           // 环形缓冲区（最近1000行）
    mutex       sync.RWMutex
    closed      bool
    startTime   time.Time
}

// Client WebSocket客户端
type Client struct {
    ID          string
    Channel     chan OutputMessage
    ConnectedAt time.Time
}

// OutputMessage 输出消息
type OutputMessage struct {
    Type      string    `json:"type"`       // output, error, completed, stage_marker
    Line      string    `json:"line"`       // 输出行内容
    Timestamp time.Time `json:"timestamp"`  // 时间戳
    LineNum   int       `json:"line_num"`   // 行号
    Stage     string    `json:"stage,omitempty"`      // 阶段名称（仅stage_marker类型）
    Status    string    `json:"status,omitempty"`     // begin或end（仅stage_marker类型）
}

// RingBuffer 环形缓冲区（保存最近N行）
type RingBuffer struct {
    lines    []OutputMessage
    capacity int
    head     int
    size     int
    mutex    sync.RWMutex
}

// Subscribe 订阅输出流
func (s *OutputStream) Subscribe(clientID string) (*Client, []OutputMessage) {
    s.mutex.Lock()
    defer s.mutex.Unlock()
    
    if s.closed {
        return nil, nil
    }
    
    // 创建客户端
    client := &Client{
        ID:          clientID,
        Channel:     make(chan OutputMessage, 100),
        ConnectedAt: time.Now(),
    }
    
    s.Clients[clientID] = client
    
    // 返回历史消息（最近1000行）
    history := s.Buffer.GetAll()
    
    log.Printf("Client %s subscribed to task %d, sent %d history lines", 
        clientID, s.TaskID, len(history))
    
    return client, history
}

// Unsubscribe 取消订阅
func (s *OutputStream) Unsubscribe(clientID string) {
    s.mutex.Lock()
    defer s.mutex.Unlock()
    
    if client, ok := s.Clients[clientID]; ok {
        close(client.Channel)
        delete(s.Clients, clientID)
        log.Printf("Client %s unsubscribed from task %d", clientID, s.TaskID)
    }
}

// Broadcast 广播消息到所有客户端
func (s *OutputStream) Broadcast(msg OutputMessage) {
    s.mutex.RLock()
    defer s.mutex.RUnlock()
    
    // 保存到缓冲区
    s.Buffer.Add(msg)
    
    // 广播到所有客户端
    for clientID, client := range s.Clients {
        select {
        case client.Channel <- msg:
            // 发送成功
        default:
            // 通道满了，记录警告但不阻塞
            log.Printf("Warning: Client %s channel full, dropping message", clientID)
        }
    }
}

// Close 关闭输出流
func (s *OutputStream) Close() {
    s.mutex.Lock()
    defer s.mutex.Unlock()
    
    if s.closed {
        return
    }
    
    s.closed = true
    
    // 关闭所有客户端通道
    for _, client := range s.Clients {
        close(client.Channel)
    }
    
    s.Clients = make(map[string]*Client)
    
    log.Printf("OutputStream for task %d closed", s.TaskID)
}

// GetStats 获取统计信息
func (s *OutputStream) GetStats() map[string]interface{} {
    s.mutex.RLock()
    defer s.mutex.RUnlock()
    
    return map[string]interface{}{
        "task_id":       s.TaskID,
        "clients_count": len(s.Clients),
        "buffer_size":   s.Buffer.Size(),
        "uptime":        time.Since(s.startTime).Seconds(),
        "closed":        s.closed,
    }
}
```

### 2. OutputStreamManager（流管理器）

```go
// OutputStreamManager 管理所有任务的输出流
type OutputStreamManager struct {
    streams map[uint]*OutputStream  // taskID -> stream
    mutex   sync.RWMutex
}

// NewOutputStreamManager 创建流管理器
func NewOutputStreamManager() *OutputStreamManager {
    return &OutputStreamManager{
        streams: make(map[uint]*OutputStream),
    }
}

// GetOrCreate 获取或创建输出流
func (m *OutputStreamManager) GetOrCreate(taskID uint) *OutputStream {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    if stream, ok := m.streams[taskID]; ok {
        return stream
    }
    
    stream := &OutputStream{
        TaskID:    taskID,
        Clients:   make(map[string]*Client),
        Buffer:    NewRingBuffer(1000), // 保存最近1000行
        startTime: time.Now(),
    }
    
    m.streams[taskID] = stream
    
    log.Printf("Created OutputStream for task %d", taskID)
    
    return stream
}

// Get 获取输出流
func (m *OutputStreamManager) Get(taskID uint) *OutputStream {
    m.mutex.RLock()
    defer m.mutex.RUnlock()
    
    return m.streams[taskID]
}

// Close 关闭输出流
func (m *OutputStreamManager) Close(taskID uint) {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    if stream, ok := m.streams[taskID]; ok {
        stream.Close()
        delete(m.streams, taskID)
        log.Printf("Closed OutputStream for task %d", taskID)
    }
}

// GetAllStats 获取所有流的统计信息
func (m *OutputStreamManager) GetAllStats() []map[string]interface{} {
    m.mutex.RLock()
    defer m.mutex.RUnlock()
    
    stats := make([]map[string]interface{}, 0, len(m.streams))
    for _, stream := range m.streams {
        stats = append(stats, stream.GetStats())
    }
    
    return stats
}

// Cleanup 清理超时的流（定期调用）
func (m *OutputStreamManager) Cleanup(timeout time.Duration) {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    now := time.Now()
    for taskID, stream := range m.streams {
        if stream.closed && now.Sub(stream.startTime) > timeout {
            delete(m.streams, taskID)
            log.Printf("Cleaned up OutputStream for task %d", taskID)
        }
    }
}
```

### 3. TerraformExecutor改造

```go
// TerraformExecutor 添加流管理器
type TerraformExecutor struct {
    db            *gorm.DB
    streamManager *OutputStreamManager
}

// NewTerraformExecutor 创建执行器
func NewTerraformExecutor(db *gorm.DB, streamManager *OutputStreamManager) *TerraformExecutor {
    return &TerraformExecutor{
        db:            db,
        streamManager: streamManager,
    }
}

// ExecutePlan 执行Plan（改造版 - 带阶段标记）
func (e *TerraformExecutor) ExecutePlan(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // ... 前面的准备工作 ...
    
    // 创建输出流
    stream := e.streamManager.GetOrCreate(task.ID)
    defer e.streamManager.Close(task.ID)
    
    // 用于收集完整输出
    var fullOutput strings.Builder
    var outputMutex sync.Mutex
    var wg sync.WaitGroup
    lineNum := 0
    
    // ========== 阶段1: Fetching ==========
    e.broadcastStageMarker(stream, "fetching", "begin", &fullOutput, &outputMutex)
    // ... 获取配置、生成文件等 ...
    e.broadcastStageMarker(stream, "fetching", "end", &fullOutput, &outputMutex)
    
    // ========== 阶段2: Init ==========
    e.broadcastStageMarker(stream, "init", "begin", &fullOutput, &outputMutex)
    if err := e.TerraformInit(ctx, workDir, task, &workspace); err != nil {
        e.broadcastStageMarker(stream, "init", "end", &fullOutput, &outputMutex)
        return err
    }
    e.broadcastStageMarker(stream, "init", "end", &fullOutput, &outputMutex)
    
    // ========== 阶段3: Planning ==========
    e.broadcastStageMarker(stream, "planning", "begin", &fullOutput, &outputMutex)
    
    // 构建命令
    args := []string{"plan", "-out=" + planFile, "-no-color", "-var-file=variables.tfvars"}
    cmd := exec.CommandContext(ctx, "terraform", args...)
    cmd.Dir = workDir
    cmd.Env = e.buildEnvironmentVariables(&workspace)
    
    // 创建Pipe
    stdoutPipe, _ := cmd.StdoutPipe()
    stderrPipe, _ := cmd.StderrPipe()
    
    // 启动命令
    if err := cmd.Start(); err != nil {
        e.broadcastStageMarker(stream, "planning", "end", &fullOutput, &outputMutex)
        return err
    }
    
    // 实时读取stdout
    wg.Add(1)
    go func() {
        defer wg.Done()
        e.streamOutput(stdoutPipe, stream, &fullOutput, &outputMutex, &lineNum, "stdout")
    }()
    
    // 实时读取stderr
    wg.Add(1)
    go func() {
        defer wg.Done()
        e.streamOutput(stderrPipe, stream, &fullOutput, &outputMutex, &lineNum, "stderr")
    }()
    
    // 等待命令完成
    cmdErr := cmd.Wait()
    
    // 等待所有输出读取完成
    wg.Wait()
    
    e.broadcastStageMarker(stream, "planning", "end", &fullOutput, &outputMutex)
    
    if cmdErr != nil {
        return fmt.Errorf("terraform plan failed: %w", cmdErr)
    }
    
    // ========== 阶段4: Saving Plan Data ==========
    e.broadcastStageMarker(stream, "saving_plan", "begin", &fullOutput, &outputMutex)
    // ... 保存Plan数据 ...
    e.broadcastStageMarker(stream, "saving_plan", "end", &fullOutput, &outputMutex)
    
    // 发送完成消息
    stream.Broadcast(OutputMessage{
        Type:      "completed",
        Timestamp: time.Now(),
    })
    
    // 保存完整输出到数据库
    task.PlanOutput = fullOutput.String()
    
    // ... 后续处理 ...
    
    return nil
}

// streamOutput 实时流式读取输出
func (e *TerraformExecutor) streamOutput(
    pipe io.ReadCloser,
    stream *OutputStream,
    fullOutput *strings.Builder,
    mutex *sync.Mutex,
    lineNum *int,
    source string,
) {
    scanner := bufio.NewScanner(pipe)
    
    for scanner.Scan() {
        line := scanner.Text()
        
        mutex.Lock()
        *lineNum++
        currentLineNum := *lineNum
        mutex.Unlock()
        
        // 创建消息
        msg := OutputMessage{
            Type:      "output",
            Line:      line,
            Timestamp: time.Now(),
            LineNum:   currentLineNum,
        }
        
        // 广播到所有WebSocket客户端
        stream.Broadcast(msg)
        
        // 保存到完整输出
        mutex.Lock()
        fullOutput.WriteString(line)
        fullOutput.WriteString("\n")
        mutex.Unlock()
    }
    
    if err := scanner.Err(); err != nil {
        log.Printf("Error reading %s: %v", source, err)
        
        // 发送错误消息
        stream.Broadcast(OutputMessage{
            Type:      "error",
            Line:      fmt.Sprintf("Error reading %s: %v", source, err),
            Timestamp: time.Now(),
        })
    }
}

// broadcastStageMarker 广播阶段标记
func (e *TerraformExecutor) broadcastStageMarker(
    stream *OutputStream,
    stage string,
    status string, // "begin" or "end"
    fullOutput *strings.Builder,
    mutex *sync.Mutex,
) {
    timestamp := time.Now()
    marker := fmt.Sprintf("========== %s %s at %s ==========",
        strings.ToUpper(stage),
        strings.ToUpper(status),
        timestamp.Format("2006-01-02 15:04:05.000"))
    
    // 创建阶段标记消息
    msg := OutputMessage{
        Type:      "stage_marker",
        Line:      marker,
        Timestamp: timestamp,
        Stage:     stage,
        Status:    status,
    }
    
    // 广播到所有客户端
    stream.Broadcast(msg)
    
    // 保存到完整输出
    mutex.Lock()
    fullOutput.WriteString(marker)
    fullOutput.WriteString("\n")
    mutex.Unlock()
}
```

### 4. WebSocket Controller

```go
// backend/controllers/terraform_output_controller.go
package controllers

import (
    "log"
    "strconv"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/gorilla/websocket"
    "github.com/google/uuid"
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        // TODO: 生产环境需要验证origin
        return true
    },
}

type TerraformOutputController struct {
    streamManager *services.OutputStreamManager
}

// StreamTaskOutput WebSocket实时输出
func (c *TerraformOutputController) StreamTaskOutput(ctx *gin.Context) {
    taskIDStr := ctx.Param("task_id")
    taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
    if err != nil {
        ctx.JSON(400, gin.H{"error": "invalid task_id"})
        return
    }
    
    // 升级到WebSocket
    ws, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
    if err != nil {
        log.Printf("WebSocket upgrade failed: %v", err)
        return
    }
    defer ws.Close()
    
    // 生成客户端ID
    clientID := uuid.New().String()
    log.Printf("Client %s connecting to task %d", clientID, taskID)
    
    // 获取输出流
    stream := c.streamManager.Get(uint(taskID))
    if stream == nil {
        // 任务可能还没开始或已完成，尝试创建流
        stream = c.streamManager.GetOrCreate(uint(taskID))
    }
    
    // 订阅输出流（同时获取历史消息）
    client, history := stream.Subscribe(clientID)
    if client == nil {
        ws.WriteJSON(map[string]string{
            "type":  "error",
            "error": "failed to subscribe to stream",
        })
        return
    }
    defer stream.Unsubscribe(clientID)
    
    // 发送连接成功消息
    ws.WriteJSON(map[string]interface{}{
        "type":    "connected",
        "task_id": taskID,
        "client_id": clientID,
    })
    
    // 发送历史消息
    for _, msg := range history {
        if err := ws.WriteJSON(msg); err != nil {
            log.Printf("Failed to send history: %v", err)
            return
        }
    }
    
    // 设置心跳
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    // 实时转发新消息
    for {
        select {
        case msg, ok := <-client.Channel:
            if !ok {
                // 通道关闭，任务完成
                return
            }
            
            if err := ws.WriteJSON(msg); err != nil {
                log.Printf("WebSocket write failed: %v", err)
                return
            }
            
        case <-ticker.C:
            // 发送心跳
            if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
                log.Printf("Ping failed: %v", err)
                return
            }
        }
    }
}

// GetStreamStats 获取流统计信息（调试用）
func (c *TerraformOutputController) GetStreamStats(ctx *gin.Context) {
    stats := c.streamManager.GetAllStats()
    ctx.JSON(200, gin.H{
        "streams": stats,
        "count":   len(stats),
    })
}
```

## 🎨 前端实现

### React Hook

```typescript
// frontend/src/hooks/useTerraformOutput.ts
import { useState, useEffect, useRef, useCallback } from 'react';

interface OutputMessage {
  type: 'output' | 'error' | 'completed' | 'connected';
  line?: string;
  timestamp?: string;
  line_num?: number;
}

export const useTerraformOutput = (taskId: number) => {
  const [lines, setLines] = useState<OutputMessage[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [isCompleted, setIsCompleted] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout>();

  const connect = useCallback(() => {
    const wsUrl = `ws://localhost:8080/api/v1/tasks/${taskId}/output/stream`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      console.log('Connected to terraform output stream');
      setIsConnected(true);
      setError(null);
    };

    ws.onmessage = (event) => {
      const data: OutputMessage = JSON.parse(event.data);
      
      switch (data.type) {
        case 'connected':
          console.log('Stream connected');
          break;
          
        case 'output':
        case 'error':
          setLines(prev => [...prev, data]);
          break;
          
        case 'completed':
          console.log('Task completed');
          setIsCompleted(true);
          break;
      }
    };

    ws.onerror = (error) => {
      console.error('WebSocket error:', error);
      setError('连接错误');
      setIsConnected(false);
    };

    ws.onclose = () => {
      console.log('WebSocket closed');
      setIsConnected(false);
      
      // 如果任务未完成，5秒后自动重连
      if (!isCompleted) {
        reconnectTimeoutRef.current = setTimeout(() => {
          console.log('Reconnecting...');
          connect();
        }, 5000);
      }
    };
  }, [taskId, isCompleted]);

  useEffect(() => {
    connect();

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.close();
      }
    };
  }, [connect]);

  return { lines, isConnected, isCompleted, error };
};
```

### 输出查看器组件

```typescript
// frontend/src/components/TerraformOutputViewer.tsx
import React, { useEffect, useRef } from 'react';
import { useTerraformOutput } from '../hooks/useTerraformOutput';
import styles from './TerraformOutputViewer.module.css';

interface Props {
  taskId: number;
}

const TerraformOutputViewer: React.FC<Props> = ({ taskId }) => {
  const { lines, isConnected, isCompleted, error } = useTerraformOutput(taskId);
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);

  // 自动滚动到底部
  useEffect(() => {
    if (autoScroll) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [lines, autoScroll]);

  // 检测用户是否手动滚动
  const handleScroll = () => {
    if (!containerRef.current) return;
    
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50;
    
    setAutoScroll(isAtBottom);
  };

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <span>Terraform Output</span>
        <div className={styles.status}>
          {isConnected && !isCompleted && (
            <span className={styles.running}>
              <span className={styles.pulse}>●</span> Running
            </span>
          )}
          {isCompleted && (
            <span className={styles.completed}>✓ Completed</span>
          )}
          {!isConnected && !isCompleted && (
            <span className={styles.disconnected}>
              ○ {error || 'Connecting...'}
            </span>
          )}
          <span className={styles.lineCount}>{lines.length} lines</span>
        </div>
      </div>
      
      <div 
        ref={containerRef}
        className={styles.output}
        onScroll={handleScroll}
      >
        {lines.map((msg, index) => (
          <div 
            key={index} 
            className={`${styles.line} ${msg.type === 'error' ? styles.error : ''}`}
          >
            <span className={styles.lineNum}>{msg.line_num || index + 1}</span>
            <span className={styles.content}>{msg.line}</span>
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
      
      {!autoScroll && (
        <button 
          className={styles.scrollButton}
          onClick={() => {
            setAutoScroll(true);
            bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
          }}
        >
          ↓ 滚动到底部
        </button>
      )}
    </div>
  );
};

export default TerraformOutputViewer;
```

## 📋 API接口

### 1. WebSocket接口（实时任务）

```
WS /api/v1/tasks/:task_id/output/stream
```

**用途**：实时查看正在执行的任务输出

**连接流程**：
1. 客户端发起WebSocket连接
2. 服务器返回连接成功消息 + 历史消息（最近1000行）
3. 服务器实时推送新消息
4. 任务完成后发送completed消息并关闭连接

**消息格式**：

```json
// 连接成功
{
  "type": "connected",
  "task_id": 123,
  "client_id": "uuid"
}

// 输出行
{
  "type": "output",
  "line": "Terraform will perform the following actions:",
  "timestamp": "2025-10-11T19:30:00Z",
  "line_num": 42
}

// 错误行
{
  "type": "error",
  "line": "Error: Invalid configuration",
  "timestamp": "2025-10-11T19:30:05Z",
  "line_num": 50
}

// 任务完成
{
  "type": "completed",
  "timestamp": "2025-10-11T19:35:00Z"
}
```

### 2. HTTP接口（历史任务）

#### 获取历史任务日志

```
GET /api/v1/tasks/:task_id/logs
```

**用途**：查看已完成任务的完整日志

**查询参数**：
- `type`: 日志类型，可选值：`plan`, `apply`, `all`（默认：`all`）
- `format`: 返回格式，可选值：`json`, `text`（默认：`json`）

**响应示例（JSON格式）**：

```json
{
  "task_id": 123,
  "task_type": "plan",
  "status": "success",
  "created_at": "2025-10-11T19:30:00Z",
  "completed_at": "2025-10-11T19:35:00Z",
  "duration": 300,
  "logs": {
    "plan": {
      "output": "Terraform will perform the following actions:\n...",
      "line_count": 150,
      "size_bytes": 8192
    },
    "apply": null
  }
}
```

**响应示例（Text格式）**：

```
Content-Type: text/plain

Terraform will perform the following actions:

  # aws_s3_bucket.example will be created
  + resource "aws_s3_bucket" "example" {
      + bucket = "my-bucket"
      ...
```

#### 下载历史任务日志

```
GET /api/v1/tasks/:task_id/logs/download
```

**用途**：下载完整日志文件

**查询参数**：
- `type`: 日志类型，可选值：`plan`, `apply`, `all`（默认：`all`）

**响应**：
- Content-Type: `application/octet-stream`
- Content-Disposition: `attachment; filename="task-123-logs.txt"`

#### 获取任务列表及日志摘要

```
GET /api/v1/workspaces/:workspace_id/tasks
```

**用途**：获取workspace的所有任务，包含日志摘要

**查询参数**：
- `status`: 任务状态过滤
- `task_type`: 任务类型过滤
- `limit`: 返回数量（默认：20）
- `offset`: 偏移量（默认：0）

**响应示例**：

```json
{
  "tasks": [
    {
      "id": 123,
      "task_type": "plan",
      "status": "success",
      "created_at": "2025-10-11T19:30:00Z",
      "completed_at": "2025-10-11T19:35:00Z",
      "duration": 300,
      "log_summary": {
        "plan_lines": 150,
        "apply_lines": 0,
        "has_errors": false,
        "last_line": "Plan: 5 to add, 0 to change, 0 to destroy."
      }
    }
  ],
  "total": 50,
  "limit": 20,
  "offset": 0
}
```

### 3. 调试接口

#### 获取流统计信息

```
GET /api/v1/terraform/streams/stats
```

**用途**：查看所有活跃流的统计信息（调试用）

**响应示例**：

```json
{
  "streams": [
    {
      "task_id": 123,
      "clients_count": 3,
      "buffer_size": 856,
      "uptime": 125.5,
      "closed": false
    }
  ],
  "count": 1
}
```

## 📋 执行阶段标记设计

### 阶段标记格式

每个执行阶段都会输出开始和结束标记：

```
========== FETCHING BEGIN at 2025-10-11 19:30:00.123 ==========
[fetching阶段的日志输出...]
========== FETCHING END at 2025-10-11 19:30:05.456 ==========

========== INIT BEGIN at 2025-10-11 19:30:05.500 ==========
[init阶段的日志输出...]
========== INIT END at 2025-10-11 19:30:15.789 ==========

========== PLANNING BEGIN at 2025-10-11 19:30:15.800 ==========
[planning阶段的日志输出...]
========== PLANNING END at 2025-10-11 19:31:45.234 ==========

========== APPLYING BEGIN at 2025-10-11 19:32:00.000 ==========
[applying阶段的日志输出...]
========== APPLYING END at 2025-10-11 19:35:30.567 ==========
```

### 完整执行阶段列表

根据15-terraform-execution-detail.md，完整的执行阶段包括：

**Plan任务阶段**：
1. `fetching` - 获取配置和准备工作目录
2. `init` - Terraform初始化
3. `pre_plan` - Plan前置处理（可选）
4. `planning` - 执行terraform plan
5. `post_plan` - Plan后置处理（可选）
6. `saving_plan` - 保存Plan数据到数据库
7. `cost_estimation` - 成本估算（可选，未来扩展）
8. `policy_check` - 策略检查（可选，未来扩展）

**Apply任务阶段**：
1. `fetching` - 获取配置和准备工作目录
2. `init` - Terraform初始化
3. `restoring_plan` - 从数据库恢复Plan文件
4. `pre_apply` - Apply前置处理（可选）
5. `applying` - 执行terraform apply
6. `post_apply` - Apply后置处理（可选）
7. `saving_state` - 保存State到数据库

### 阶段标记消息类型

```typescript
interface StageMarkerMessage {
  type: 'stage_marker';
  line: string;           // 格式化的标记文本
  timestamp: string;      // ISO 8601时间戳
  stage: string;          // 阶段名称
  status: 'begin' | 'end'; // 开始或结束
}
```

### 前端显示优化

```typescript
// frontend/src/components/TerraformOutputViewer.tsx
const TerraformOutputViewer: React.FC<Props> = ({ taskId }) => {
  const { lines, isConnected, isCompleted, error } = useTerraformOutput(taskId);
  
  return (
    <div className={styles.output}>
      {lines.map((msg, index) => {
        // 阶段标记特殊样式
        if (msg.type === 'stage_marker') {
          return (
            <div key={index} className={styles.stageMarker}>
              <span className={styles.stageIcon}>
                {msg.status === 'begin' ? '▶' : '✓'}
              </span>
              <span className={styles.stageName}>{msg.stage}</span>
              <span className={styles.stageStatus}>{msg.status}</span>
              <span className={styles.stageTime}>
                {new Date(msg.timestamp).toLocaleTimeString()}
              </span>
            </div>
          );
        }
        
        // 普通输出行
        return (
          <div key={index} className={styles.line}>
            <span className={styles.lineNum}>{msg.line_num}</span>
            <span className={styles.content}>{msg.line}</span>
          </div>
        );
      })}
    </div>
  );
};
```

### CSS样式

```css
/* TerraformOutputViewer.module.css */
.stageMarker {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  margin: 8px 0;
  background: linear-gradient(90deg, #f0f9ff 0%, #e0f2fe 100%);
  border-left: 4px solid #3b82f6;
  font-weight: 600;
  color: #1e40af;
  font-family: var(--font-mono);
}

.stageIcon {
  margin-right: 12px;
  font-size: 16px;
  color: #3b82f6;
}

.stageName {
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-right: 12px;
}

.stageStatus {
  margin-right: auto;
  font-size: 14px;
  color: #64748b;
}

.stageTime {
  font-size: 12px;
  color: #94a3b8;
  font-weight: normal;
}
```

### 阶段时间统计

```go
// 阶段执行时间统计
type StageTimings struct {
    Fetching      time.Duration `json:"fetching"`
    Init          time.Duration `json:"init"`
    PrePlan       time.Duration `json:"pre_plan,omitempty"`
    Planning      time.Duration `json:"planning"`
    PostPlan      time.Duration `json:"post_plan,omitempty"`
    SavingPlan    time.Duration `json:"saving_plan"`
    RestoringPlan time.Duration `json:"restoring_plan,omitempty"`
    PreApply      time.Duration `json:"pre_apply,omitempty"`
    Applying      time.Duration `json:"applying,omitempty"`
    PostApply     time.Duration `json:"post_apply,omitempty"`
    SavingState   time.Duration `json:"saving_state,omitempty"`
    Total         time.Duration `json:"total"`
}

// 记录阶段时间
func (e *TerraformExecutor) recordStageTime(
    task *models.WorkspaceTask,
    stage string,
    duration time.Duration,
) {
    if task.Context == nil {
        task.Context = make(map[string]interface{})
    }
    
    if task.Context["stage_timings"] == nil {
        task.Context["stage_timings"] = &StageTimings{}
    }
    
    timings := task.Context["stage_timings"].(*StageTimings)
    
    switch stage {
    case "fetching":
        timings.Fetching = duration
    case "init":
        timings.Init = duration
    case "planning":
        timings.Planning = duration
    case "applying":
        timings.Applying = duration
    // ... 其他阶段
    }
    
    e.db.Save(task)
}
```

### 阶段进度展示

```typescript
// 前端显示阶段进度
interface StageProgress {
  name: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  startTime?: string;
  endTime?: string;
  duration?: number;
}

const StageProgressBar: React.FC<{ stages: StageProgress[] }> = ({ stages }) => {
  return (
    <div className={styles.progressBar}>
      {stages.map((stage, index) => (
        <div key={index} className={styles.stageItem}>
          <div className={`${styles.stageIndicator} ${styles[stage.status]}`}>
            {stage.status === 'completed' && '✓'}
            {stage.status === 'running' && '⟳'}
            {stage.status === 'failed' && '✗'}
          </div>
          <div className={styles.stageInfo}>
            <div className={styles.stageName}>{stage.name}</div>
            {stage.duration && (
              <div className={styles.stageDuration}>
                {(stage.duration / 1000).toFixed(1)}s
              </div>
            )}
          </div>
        </div>
      ))}
    </div>
  );
};
```

## 🗄️ 历史日志存储与查询

### 数据库设计

任务完成后，完整日志保存在`workspace_tasks`表：

```sql
-- workspace_tasks表已有字段
plan_output TEXT,      -- Plan完整输出（包含阶段标记）
apply_output TEXT,     -- Apply完整输出（包含阶段标记）
error_message TEXT,    -- 错误信息
context JSONB,         -- 包含stage_timings等元数据
```

### 历史日志Controller

```go
// backend/controllers/task_log_controller.go
package controllers

type TaskLogController struct {
    db *gorm.DB
}

// GetTaskLogs 获取历史任务日志
func (c *TaskLogController) GetTaskLogs(ctx *gin.Context) {
    taskID := ctx.Param("task_id")
    logType := ctx.DefaultQuery("type", "all")
    format := ctx.DefaultQuery("format", "json")
    
    var task models.WorkspaceTask
    if err := c.db.First(&task, taskID).Error; err != nil {
        ctx.JSON(404, gin.H{"error": "task not found"})
        return
    }
    
    // 检查权限
    // ...
    
    if format == "text" {
        // 返回纯文本格式
        c.returnTextLogs(ctx, &task, logType)
        return
    }
    
    // 返回JSON格式
    response := gin.H{
        "task_id":      task.ID,
        "task_type":    task.TaskType,
        "status":       task.Status,
        "created_at":   task.CreatedAt,
        "completed_at": task.CompletedAt,
        "duration":     task.Duration,
        "logs":         gin.H{},
    }
    
    if logType == "plan" || logType == "all" {
        if task.PlanOutput != "" {
            response["logs"].(gin.H)["plan"] = gin.H{
                "output":     task.PlanOutput,
                "line_count": strings.Count(task.PlanOutput, "\n"),
                "size_bytes": len(task.PlanOutput),
            }
        }
    }
    
    if logType == "apply" || logType == "all" {
        if task.ApplyOutput != "" {
            response["logs"].(gin.H)["apply"] = gin.H{
                "output":     task.ApplyOutput,
                "line_count": strings.Count(task.ApplyOutput, "\n"),
                "size_bytes": len(task.ApplyOutput),
            }
        }
    }
    
    ctx.JSON(200, response)
}

// returnTextLogs 返回纯文本格式日志
func (c *TaskLogController) returnTextLogs(
    ctx *gin.Context,
    task *models.WorkspaceTask,
    logType string,
) {
    var output strings.Builder
    
    if logType == "plan" || logType == "all" {
        if task.PlanOutput != "" {
            output.WriteString("=== PLAN OUTPUT ===\n")
            output.WriteString(task.PlanOutput)
            output.WriteString("\n\n")
        }
    }
    
    if logType == "apply" || logType == "all" {
        if task.ApplyOutput != "" {
            output.WriteString("=== APPLY OUTPUT ===\n")
            output.WriteString(task.ApplyOutput)
            output.WriteString("\n\n")
        }
    }
    
    if task.ErrorMessage != "" {
        output.WriteString("=== ERROR ===\n")
        output.WriteString(task.ErrorMessage)
    }
    
    ctx.Header("Content-Type", "text/plain; charset=utf-8")
    ctx.String(200, output.String())
}

// DownloadTaskLogs 下载任务日志
func (c *TaskLogController) DownloadTaskLogs(ctx *gin.Context) {
    taskID := ctx.Param("task_id")
    logType := ctx.DefaultQuery("type", "all")
    
    var task models.WorkspaceTask
    if err := c.db.First(&task, taskID).Error; err != nil {
        ctx.JSON(404, gin.H{"error": "task not found"})
        return
    }
    
    // 检查权限
    // ...
    
    var output strings.Builder
    
    // 添加元数据
    output.WriteString(fmt.Sprintf("Task ID: %d\n", task.ID))
    output.WriteString(fmt.Sprintf("Task Type: %s\n", task.TaskType))
    output.WriteString(fmt.Sprintf("Status: %s\n", task.Status))
    output.WriteString(fmt.Sprintf("Created: %s\n", task.CreatedAt.Format(time.RFC3339)))
    if task.CompletedAt != nil {
        output.WriteString(fmt.Sprintf("Completed: %s\n", task.CompletedAt.Format(time.RFC3339)))
        output.WriteString(fmt.Sprintf("Duration: %ds\n", task.Duration))
    }
    output.WriteString("\n" + strings.Repeat("=", 80) + "\n\n")
    
    // 添加日志内容
    if logType == "plan" || logType == "all" {
        if task.PlanOutput != "" {
            output.WriteString("PLAN OUTPUT:\n")
            output.WriteString(strings.Repeat("-", 80) + "\n")
            output.WriteString(task.PlanOutput)
            output.WriteString("\n\n")
        }
    }
    
    if logType == "apply" || logType == "all" {
        if task.ApplyOutput != "" {
            output.WriteString("APPLY OUTPUT:\n")
            output.WriteString(strings.Repeat("-", 80) + "\n")
            output.WriteString(task.ApplyOutput)
            output.WriteString("\n\n")
        }
    }
    
    if task.ErrorMessage != "" {
        output.WriteString("ERROR:\n")
        output.WriteString(strings.Repeat("-", 80) + "\n")
        output.WriteString(task.ErrorMessage)
    }
    
    filename := fmt.Sprintf("task-%d-logs.txt", task.ID)
    ctx.Header("Content-Type", "application/octet-stream")
    ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
    ctx.String(200, output.String())
}

// GetTaskListWithLogSummary 获取任务列表及日志摘要
func (c *TaskLogController) GetTaskListWithLogSummary(ctx *gin.Context) {
    workspaceID := ctx.Param("workspace_id")
    status := ctx.Query("status")
    taskType := ctx.Query("task_type")
    limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))
    offset, _ := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
    
    query := c.db.Model(&models.WorkspaceTask{}).
        Where("workspace_id = ?", workspaceID)
    
    if status != "" {
        query = query.Where("status = ?", status)
    }
    
    if taskType != "" {
        query = query.Where("task_type = ?", taskType)
    }
    
    var total int64
    query.Count(&total)
    
    var tasks []models.WorkspaceTask
    query.Order("created_at DESC").
        Limit(limit).
        Offset(offset).
        Find(&tasks)
    
    // 构建响应
    taskList := make([]gin.H, 0, len(tasks))
    for _, task := range tasks {
        planLines := strings.Count(task.PlanOutput, "\n")
        applyLines := strings.Count(task.ApplyOutput, "\n")
        hasErrors := task.ErrorMessage != ""
        
        // 获取最后一行
        lastLine := ""
        if task.ApplyOutput != "" {
            lines := strings.Split(strings.TrimSpace(task.ApplyOutput), "\n")
            if len(lines) > 0 {
                lastLine = lines[len(lines)-1]
            }
        } else if task.PlanOutput != "" {
            lines := strings.Split(strings.TrimSpace(task.PlanOutput), "\n")
            if len(lines) > 0 {
                lastLine = lines[len(lines)-1]
            }
        }
        
        taskList = append(taskList, gin.H{
            "id":           task.ID,
            "task_type":    task.TaskType,
            "status":       task.Status,
            "created_at":   task.CreatedAt,
            "completed_at": task.CompletedAt,
            "duration":     task.Duration,
            "log_summary": gin.H{
                "plan_lines":  planLines,
                "apply_lines": applyLines,
                "has_errors":  hasErrors,
                "last_line":   lastLine,
            },
        })
    }
    
    ctx.JSON(200, gin.H{
        "tasks":  taskList,
        "total":  total,
        "limit":  limit,
        "offset": offset,
    })
}
```

### 前端历史日志查看器

```typescript
// frontend/src/components/HistoricalLogViewer.tsx
import React, { useState, useEffect } from 'react';
import styles from './HistoricalLogViewer.module.css';

interface Props {
  taskId: number;
}

const HistoricalLogViewer: React.FC<Props> = ({ taskId }) => {
  const [logs, setLogs] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [logType, setLogType] = useState<'all' | 'plan' | 'apply'>('all');

  useEffect(() => {
    fetchLogs();
  }, [taskId, logType]);

  const fetchLogs = async () => {
    setLoading(true);
    setError(null);
    
    try {
      const response = await fetch(
        `/api/v1/tasks/${taskId}/logs?type=${logType}&format=text`
      );
      
      if (!response.ok) {
        throw new Error('Failed to fetch logs');
      }
      
      const text = await response.text();
      setLogs(text);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleDownload = () => {
    window.open(`/api/v1/tasks/${taskId}/logs/download?type=${logType}`, '_blank');
  };

  if (loading) {
    return <div className={styles.loading}>加载日志中...</div>;
  }

  if (error) {
    return <div className={styles.error}>加载失败: {error}</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.tabs}>
          <button
            className={logType === 'all' ? styles.active : ''}
            onClick={() => setLogType('all')}
          >
            全部
          </button>
          <button
            className={logType === 'plan' ? styles.active : ''}
            onClick={() => setLogType('plan')}
          >
            Plan
          </button>
          <button
            className={logType === 'apply' ? styles.active : ''}
            onClick={() => setLogType('apply')}
          >
            Apply
          </button>
        </div>
        <button className={styles.downloadBtn} onClick={handleDownload}>
          ⬇ 下载日志
        </button>
      </div>
      
      <div className={styles.logContent}>
        <pre>{logs}</pre>
      </div>
    </div>
  );
};

export default HistoricalLogViewer;
```

### 智能日志查看器（自动切换）

```typescript
// frontend/src/components/SmartLogViewer.tsx
import React, { useState, useEffect } from 'react';
import TerraformOutputViewer from './TerraformOutputViewer';
import HistoricalLogViewer from './HistoricalLogViewer';

interface Props {
  taskId: number;
}

const SmartLogViewer: React.FC<Props> = ({ taskId }) => {
  const [taskStatus, setTaskStatus] = useState<string>('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchTaskStatus();
  }, [taskId]);

  const fetchTaskStatus = async () => {
    try {
      const response = await fetch(`/api/v1/tasks/${taskId}`);
      const data = await response.json();
      setTaskStatus(data.status);
    } catch (err) {
      console.error('Failed to fetch task status:', err);
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return <div>加载中...</div>;
  }

  // 如果任务正在运行，使用WebSocket实时查看
  if (taskStatus === 'running' || taskStatus === 'pending') {
    return <TerraformOutputViewer taskId={taskId} />;
  }

  // 如果任务已完成，使用HTTP查看历史日志
  return <HistoricalLogViewer taskId={taskId} />;
};

export default SmartLogViewer;
```

## 🔧 实施计划

### Phase 1: 核心功能（2-3天）

**Day 1**：
- [ ] 实现RingBuffer
- [ ] 实现OutputStream
- [ ] 实现OutputStreamManager
- [ ] 单元测试

**Day 2**：
- [ ] 改造TerraformExecutor
- [ ] 实现WebSocket Controller
- [ ] 实现历史日志HTTP API
- [ ] 集成到路由

**Day 3**：
- [ ] 前端实时查看Hook和组件
- [ ] 前端历史查看组件
- [ ] 智能切换组件
- [ ] 端到端测试

### Phase 2: 优化功能（1-2天）

**Day 4**：
- [ ] 添加断线重连
- [ ] 添加心跳检测
- [ ] 日志下载功能
- [ ] 性能优化

**Day 5**：
- [ ] 添加监控指标
- [ ] 压力测试
- [ ] 文档完善

## 📊 性能考虑

### 内存管理

1. **环形缓冲区**：每个任务最多保存1000行历史消息
2. **自动清理**：任务完成后30分钟自动清理流
3. **客户端限制**：每个任务最多100个并发客户端

### 并发控制

1. **读写锁**：使用RWMutex保护共享数据
2. **通道缓冲**：每个客户端通道缓冲100条消息
3. **非阻塞发送**：通道满时丢弃消息而不阻塞

### 网络优化

1. **消息压缩**：可选启用WebSocket压缩
2. **心跳检测**：30秒心跳，检测死连接
3. **自动重连**：断线后5秒自动重连

## 🔒 安全考虑

### 认证授权

```go
// 添加JWT认证中间件
func (c *TerraformOutputController) StreamTaskOutput(ctx *gin.Context) {
    // 1. 验证JWT token
    userID := ctx.GetUint("user_id")
    
    // 2. 验证用户是否有权限查看该任务
    taskID := ctx.GetUint("task_id")
    if !c.checkPermission(userID, taskID) {
        ctx.JSON(403, gin.H{"error": "permission denied"})
        return
    }
    
    // 3. 升级WebSocket
    // ...
}
```

### 资源限制

1. **连接数限制**：每个用户最多10个并发WebSocket连接
2. **速率限制**：每个IP每分钟最多建立30个连接
3. **超时控制**：空闲连接10分钟后自动断开

## 📝 监控指标

### Prometheus指标

```go
var (
    activeStreams = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "terraform_output_active_streams",
        Help: "Number of active output streams",
    })
    
    activeClients = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "terraform_output_active_clients",
        Help: "Number of active WebSocket clients",
    })
    
    messagesTotal = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "terraform_output_messages_total",
        Help: "Total
