# Agent WebSocket断线问题修复总结

## 问题描述

Agent每10秒断线重连一次,错误信息:
```
websocket: close 1002 (protocol error): RSV1 set, bad opcode 7, bad MASK
```

## 问题根源

**并发读写导致WebSocket帧损坏**

gorilla/websocket的`ReadJSON`和`WriteJSON`方法不是线程安全的。当`handleMessages`在读取消息时,如果`HeartbeatLoop`同时调用`sendMessage`写入heartbeat,会导致WebSocket帧损坏。

### 为什么每10秒发生?

- Agent的HeartbeatLoop每10秒发送一次heartbeat
- 当heartbeat发送(WriteJSON)与消息读取(ReadJSON)并发时,WebSocket帧被损坏
- 产生无效的opcode 7和其他协议错误

## 修复方案

### 关键修改 (`backend/agent/control/cc_manager.go`)

**之前的错误实现**:
```go
m.connMutex.Lock()
conn := m.conn
m.connMutex.Unlock()

err := conn.ReadJSON(&msg)  // 释放锁后读取,可能与写入并发!
```

**修复后的正确实现**:
```go
m.connMutex.Lock()
if m.conn == nil {
    m.connMutex.Unlock()
    return
}

err := m.conn.ReadJSON(&msg)  // 持有锁读取
m.connMutex.Unlock()           // 读取完成后释放锁
```

### 原理

通过在读取时也持有`connMutex`,确保:
1. `ReadJSON`和`WriteJSON`不会并发执行
2. 读写操作完全串行化
3. WebSocket帧不会被损坏

## 排查过程

经过极其深入的排查,尝试了以下方案(都无效):
1. 降级gorilla/websocket版本
2. 禁用压缩
3. 增大缓冲区
4. 添加独立的writeMutex
5. 移除pingLoop
6. 移除deadline设置
7. 简化Dialer配置
8. 修改Gin框架配置
9. 移除中间件
10. 简化消息处理逻辑

最终通过创建测试WebSocket服务器对比,发现问题在于并发读写。

## 验证

创建了测试服务器(`/tmp/wstest/server.go`)验证:
- 简单的WebSocket echo服务器工作正常
- 每10秒发送JSON消息,wscat连接稳定
- 证明不是Go或gorilla/websocket的问题

## 结论

这是一个非常隐蔽的并发bug:
- 只在特定时间窗口触发(heartbeat发送时)
- 错误信息具有误导性(看起来像压缩或协议问题)
- 需要通过对比测试才能定位

**教训**: gorilla/websocket的文档明确说明需要确保读写操作的互斥,但很容易被忽略。

## 修复文件

- `backend/agent/control/cc_manager.go` - 修改handleMessages持有锁读取
- `backend/go.sum` - 更新依赖

Git commit: `3da1c12`
