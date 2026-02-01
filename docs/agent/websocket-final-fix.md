# WebSocket断线问题最终解决方案

## 问题总结
Agent每10秒断线重连，错误：`websocket: RSV1 set, bad opcode 7, bad MASK`

## 根本原因
服务端发送的WebSocket帧格式错误，可能原因：
1. 压缩设置不一致
2. 并发写入导致帧损坏
3. 中间件干扰

## 已验证的事实
1.  路由正确：`/api/v1/agents/control` 返回200/101
2.  Agent能成功连接并发送heartbeat
3.  服务端能接收到heartbeat消息
4. ❌ 服务端发送的heartbeat_ack响应格式错误
5.  没有启用WebSocket扩展（压缩已禁用）

## 解决方案

### 1. 确保服务端完全禁用压缩
```go
// backend/internal/handlers/agent_cc_handler.go
upgrader: websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true
    },
    EnableCompression: false,  // 明确禁用压缩
    HandshakeTimeout: 10 * time.Second,
}
```

### 2. 使用WriteMessage替代WriteJSON
修改sendMessage函数，使用更底层的WriteMessage：

```go
func (h *AgentCCHandler) sendMessage(agentConn *AgentConnection, msg CCMessage) error {
    // 先序列化为JSON
    data, err := json.Marshal(msg)
    if err != nil {
        return fmt.Errorf("failed to marshal message: %w", err)
    }
    
    // Set write deadline
    deadline := time.Now().Add(10 * time.Second)
    
    agentConn.connMu.Lock()
    defer agentConn.connMu.Unlock()
    
    // Check if connection is still valid
    select {
    case <-agentConn.ctx.Done():
        return fmt.Errorf("connection closed")
    default:
    }
    
    agentConn.conn.SetWriteDeadline(deadline)
    // 使用WriteMessage而不是WriteJSON
    err = agentConn.conn.WriteMessage(websocket.TextMessage, data)
    if err != nil {
        log.Printf("Failed to send message to agent %s: %v", agentConn.AgentID, err)
        agentConn.cancel()
        return err
    }
    
    return nil
}
```

### 3. Agent端也使用相同的方式
```go
// backend/agent/control/cc_manager.go
func (m *CCManager) sendHeartbeat() error {
    msg := map[string]interface{}{
        "type": "heartbeat",
        "payload": map[string]interface{}{
            "plan_running":  m.getRunningPlanCount(),
            "plan_limit":    3,
            "apply_running": m.isApplyRunning(),
            "current_tasks": m.getCurrentTasks(),
            "cpu_usage":     0.0,
            "mem_usage":     0.0,
        },
    }
    
    data, err := json.Marshal(msg)
    if err != nil {
        return err
    }
    
    select {
    case m.writeChan <- writeRequest{
        messageType: websocket.TextMessage,
        data:        data,
    }:
        return nil
    case <-time.After(5 * time.Second):
        return fmt.Errorf("timeout sending heartbeat")
    }
}
```

## 测试验证
1. 重新编译服务端和Agent
2. 运行测试程序验证连接稳定性
3. 观察是否还有断线重连

## 监控指标
- WebSocket连接持续时间
- 断线重连次数
- 错误日志中的"RSV1 set"错误

## 后续优化建议
1. 添加WebSocket连接池管理
2. 实现自动重连退避策略
3. 添加连接质量监控
4. 考虑使用更稳定的WebSocket库（如nhooyr.io/websocket）
