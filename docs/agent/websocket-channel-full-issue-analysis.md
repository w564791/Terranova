# WebSocket Channel Full Issue Analysis

## 问题描述

服务器日志中出现大量警告信息：
```
Warning: Client ad850271-135c-449e-96b8-b3c0896ed88e channel full, dropping message
```

## 问题原因分析

### 1. 当前实现

在 `backend/internal/websocket/client.go` 中：
```go
send: make(chan []byte, 256),  // 缓冲区大小为256
```

在 `backend/internal/websocket/hub.go` 的 `sendToClient` 方法中：
```go
select {
case client.send <- data:
    log.Printf("📤 Message sent to session %s: type=%s", client.sessionID, message.Type)
default:
    // 发送缓冲区已满，关闭连接
    log.Printf("  Client send buffer full, closing connection: session=%s", client.sessionID)
    h.mu.Lock()
    delete(h.clients, client.sessionID)
    close(client.send)
    h.mu.Unlock()
}
```

### 2. 问题根源

**Channel满的原因：**

1. **消息生产速度 > 消费速度**
   - 服务器端发送消息的速度超过了客户端接收和处理的速度
   - 256个消息的缓冲区被填满

2. **客户端网络慢或处理慢**
   - 客户端网络延迟高
   - 客户端处理消息的速度慢
   - 客户端可能暂时失去响应

3. **大量消息突发**
   - 在短时间内产生大量WebSocket消息
   - 例如：Terraform执行时的实时日志输出
   - 资源状态变更的频繁推送

4. **日志记录问题**
   - 当前代码在 `default` 分支中只记录警告，但实际上已经关闭了连接
   - 这个警告日志本身就说明连接已经被强制关闭

## 影响

1. **消息丢失**：缓冲区满时，新消息被丢弃
2. **连接断开**：当前实现会直接关闭连接
3. **用户体验差**：前端可能看不到实时更新
4. **重连风暴**：客户端频繁重连可能加重服务器负担

## 解决方案

### 方案1：增加缓冲区大小（临时方案）

**优点：**
- 实现简单，改动小
- 可以缓解短期突发流量

**缺点：**
- 治标不治本
- 增加内存占用
- 只是延迟问题发生

**实现：**
```go
// 从 256 增加到 1024 或更大
send: make(chan []byte, 1024),
```

### 方案2：消息优先级和丢弃策略（推荐）

**优点：**
- 保留重要消息
- 避免连接断开
- 更好的用户体验

**缺点：**
- 实现复杂度中等
- 需要定义消息优先级

**实现思路：**
1. 定义消息优先级（高、中、低）
2. 缓冲区满时，丢弃低优先级消息
3. 保持连接不断开
4. 记录详细的丢弃统计

### 方案3：消息合并和批处理（推荐）

**优点：**
- 减少消息数量
- 提高传输效率
- 降低网络开销

**缺点：**
- 需要修改消息结构
- 可能增加延迟

**实现思路：**
1. 相同类型的消息进行合并
2. 批量发送日志消息
3. 使用消息聚合器

### 方案4：背压机制（最佳方案）

**优点：**
- 根本解决问题
- 自适应调节
- 保护系统稳定性

**缺点：**
- 实现复杂度高
- 需要全面测试

**实现思路：**
1. 监控channel使用率
2. 当使用率超过阈值时，通知生产者降速
3. 实现消息采样（例如：每N条发送一条）
4. 添加流控机制

### 方案5：改进日志记录（立即实施）

**当前问题：**
- 日志信息不准确，说"dropping message"但实际是关闭连接
- 没有记录客户端ID，难以追踪

**改进：**
```go
default:
    // 发送缓冲区已满，丢弃消息但保持连接
    log.Printf("  Warning: Client %s channel full, dropping message (type=%s, buffer=%d/%d)", 
        client.sessionID, message.Type, len(client.send), cap(client.send))
    // 不关闭连接，只丢弃消息
}
```

## 推荐实施方案

### 短期方案（立即实施）

1. **增加缓冲区大小**：从256增加到1024
2. **改进日志记录**：提供更详细的诊断信息
3. **不关闭连接**：改为只丢弃消息，保持连接

### 中期方案（1-2周）

1. **实现消息优先级**
2. **添加消息统计**：记录发送成功率、丢弃率
3. **实现消息合并**：特别是日志类消息

### 长期方案（1个月）

1. **实现完整的背压机制**
2. **添加监控和告警**
3. **优化消息生产者**：在源头控制消息速率

## 监控建议

添加以下监控指标：

1. **Channel使用率**：`len(client.send) / cap(client.send)`
2. **消息丢弃率**：丢弃消息数 / 总消息数
3. **连接断开率**：因缓冲区满而断开的连接数
4. **消息延迟**：消息从生产到发送的时间
5. **客户端响应时间**：Ping/Pong延迟

## 代码示例

### 示例1：改进的sendToClient方法

```go
func (h *Hub) sendToClient(client *Client, message Message) {
    data, err := json.Marshal(message)
    if err != nil {
        log.Printf("❌ Failed to marshal message: %v", err)
        return
    }

    // 检查缓冲区使用率
    bufferUsage := float64(len(client.send)) / float64(cap(client.send))
    
    select {
    case client.send <- data:
        log.Printf("📤 Message sent to session %s: type=%s, buffer=%.1f%%", 
            client.sessionID, message.Type, bufferUsage*100)
    default:
        // 缓冲区满，记录警告但不关闭连接
        log.Printf("  Warning: Client %s channel full (%.1f%%), dropping message: type=%s", 
            client.sessionID, bufferUsage*100, message.Type)
        
        // 可选：记录到监控系统
        // metrics.IncrementDroppedMessages(client.sessionID, message.Type)
        
        // 不关闭连接，让客户端继续接收后续消息
    }
}
```

### 示例2：带优先级的消息

```go
type Message struct {
    Type      string      `json:"type"`
    SessionID string      `json:"session_id"`
    Data      interface{} `json:"data"`
    Priority  int         `json:"-"` // 不序列化，仅内部使用
}

const (
    PriorityLow    = 0
    PriorityNormal = 1
    PriorityHigh   = 2
)

func (h *Hub) sendToClient(client *Client, message Message) {
    data, err := json.Marshal(message)
    if err != nil {
        log.Printf("❌ Failed to marshal message: %v", err)
        return
    }

    select {
    case client.send <- data:
        // 发送成功
    default:
        // 缓冲区满
        if message.Priority >= PriorityHigh {
            // 高优先级消息：尝试丢弃一个低优先级消息
            // 这需要更复杂的实现，使用优先级队列
            log.Printf("  High priority message blocked for client %s", client.sessionID)
        } else {
            // 低优先级消息：直接丢弃
            log.Printf("  Dropping low priority message for client %s: type=%s", 
                client.sessionID, message.Type)
        }
    }
}
```

## 总结

这个问题的核心是**生产者-消费者速度不匹配**。最好的解决方案是：

1. **短期**：增加缓冲区 + 改进日志 + 不关闭连接
2. **中期**：实现消息优先级和合并
3. **长期**：实现完整的背压机制

建议先实施短期方案快速缓解问题，然后逐步实施中长期方案从根本上解决问题。
