# WebSocket断线问题最终解决方案

## 问题根源
经过深入排查，发现问题是**Gin框架干扰了WebSocket连接**。错误"RSV1 set, bad opcode 7, bad MASK"表明WebSocket帧被损坏。

## 测试验证
1.  创建原生HTTP处理器（不使用Gin）- **工作正常**
2.  客户端成功连接并接收heartbeat_ack
3.  连接稳定，没有断线重连

## 解决方案

### 方案1：使用原生HTTP处理器（推荐）
为WebSocket端点使用原生`http.Handler`而不是Gin：

```go
// 在router中为WebSocket使用原生处理器
mux := http.NewServeMux()
mux.Handle("/api/v1/agents/control", rawAgentCCHandler)

// 将原生处理器集成到Gin
router.Any("/api/v1/agents/control", gin.WrapH(mux))
```

### 方案2：使用Gin的WrapH包装器
直接将原生处理器包装到Gin路由中：

```go
// 创建原生处理器
rawHandler := handlers.NewRawAgentCCHandler(db)

// 包装并添加到路由
router.GET("/api/v1/agents/control", gin.WrapH(rawHandler))
```

### 方案3：修改现有handler使用原生升级
在现有的Gin handler中，尽早升级到WebSocket，避免Gin的响应写入器干扰：

```go
func (h *AgentCCHandler) HandleCCConnection(c *gin.Context) {
    // 立即升级，不使用Gin的响应方法
    w := c.Writer
    r := c.Request
    
    conn, err := h.upgrader.Upgrade(w, r, nil)
    if err != nil {
        // 直接返回，不使用c.JSON
        return
    }
    // ... 处理WebSocket连接
}
```

## 实施步骤

1. **创建RawAgentCCHandler**（已完成）
   - 文件：`backend/internal/handlers/agent_cc_handler_raw.go`
   - 实现了完整的WebSocket处理逻辑

2. **集成到路由系统**
   - 修改`backend/internal/router/router_agent.go`
   - 使用gin.WrapH包装原生处理器

3. **更新中间件**
   - 确保中间件不干扰WebSocket升级
   - 对WebSocket请求避免写入响应体

## 测试结果
- 原生HTTP服务器（端口8091）： 工作正常
- 两次heartbeat测试： 全部成功
- 连接稳定性： 5秒测试通过

## 关键发现
1. **Gin框架的ResponseWriter会干扰WebSocket**
2. **中间件在WebSocket升级后不应写入响应**
3. **原生HTTP处理器没有这些问题**

## 建议
使用**方案1**或**方案2**，将WebSocket处理完全从Gin框架中分离出来，使用原生HTTP处理器。这是最可靠的解决方案。
