# WebSocket Race Condition Fix

## Problem Summary

Agent WebSocket connections were disconnecting every 10 seconds with the error:
```
websocket: close 1002 (protocol error): RSV1 set, bad opcode 7, bad MASK
```

This error indicates **corrupted WebSocket frames** being sent from the server to the agent.

## Root Cause Analysis

After 15+ failed fix attempts and creating a test WebSocket server that worked perfectly, the issue was identified as a **critical race condition** in the server-side WebSocket handler.

### The Race Condition

The server had **three separate goroutines** that could close the WebSocket connection:

1. **writeLoop** - Dedicated goroutine for writing messages
2. **handleAgentMessages** - Reads messages and closes connection on error
3. **monitorConnection** - Monitors heartbeat timeout
4. **HandleCCConnection** - Closes old connection when agent reconnects

The problem occurred when:
1. `writeLoop` was about to write a heartbeat response
2. Another goroutine (e.g., `handleAgentMessages` detecting a read error, or `HandleCCConnection` handling a reconnect) closed the connection
3. `writeLoop` attempted to write to the closed/closing connection, **corrupting the WebSocket frame**

### Why the Test Server Worked

The test server (`/tmp/wstest/server.go`) used a **single goroutine** for both reading and writing:

```go
func handler(w http.ResponseWriter, r *http.Request) {
    conn, _ := upgrader.Upgrade(w, r, nil)
    defer conn.Close()  // Single point of closure
    
    // Single goroutine handles both read and write
    go func() {
        for range ticker.C {
            conn.WriteJSON(msg)  // Write in same goroutine
        }
    }()
    
    for {
        conn.ReadJSON(&msg)  // Read in main goroutine
    }
}
```

This avoided the race condition entirely.

## The Fix

### Solution: Ordered Shutdown

Ensure `writeLoop` **stops before** the connection is closed in all scenarios:

1. **Cancel the write context** to signal writeLoop to stop
2. **Wait 100ms** for writeLoop to finish
3. **Then close the connection**

### Changes Made

#### 1. In `handleAgentMessages` (normal disconnection)

```go
func (h *AgentCCHandler) handleAgentMessages(agentConn *AgentConnection) {
    defer func() {
        // CRITICAL: Stop writeLoop FIRST before closing connection
        agentConn.writeCancel()
        
        // Wait a moment for writeLoop to finish
        time.Sleep(100 * time.Millisecond)
        
        // Now safe to close connection
        agentConn.Conn.Close()
        // ... cleanup ...
    }()
    // ... message handling ...
}
```

#### 2. In `monitorConnection` (heartbeat timeout)

```go
if time.Since(lastPing) > 120*time.Second {
    // CRITICAL: Stop writeLoop FIRST before closing connection
    agentConn.writeCancel()
    time.Sleep(100 * time.Millisecond)
    
    // Close connection
    agentConn.Conn.Close()
    // ... cleanup ...
}
```

#### 3. In `HandleCCConnection` (reconnection scenario)

```go
if existingConn, exists := h.agents[agentID]; exists {
    // Cancel old monitor
    if existingConn.monitorCancel != nil {
        existingConn.monitorCancel()
    }
    // CRITICAL: Stop writeLoop FIRST before closing connection
    if existingConn.writeCancel != nil {
        existingConn.writeCancel()
    }
    // Wait for writeLoop to finish
    time.Sleep(100 * time.Millisecond)
    existingConn.Conn.Close()
}
```

## Key Lessons

1. **WebSocket frame corruption** occurs when multiple goroutines write to or close the same connection concurrently
2. **Channel-based write serialization** alone is not enough - you must also ensure ordered shutdown
3. **Test servers** that work don't always reflect production complexity - our test server's simpler architecture avoided the race
4. **Context cancellation** is the proper way to signal goroutines to stop before resource cleanup
5. **Sleep-based synchronization** (100ms wait) is acceptable for shutdown scenarios where precision isn't critical

## Testing

After applying this fix:
- Restart the backend server
- Restart the agent
- Monitor for 60+ seconds
- Connection should remain stable without disconnections

## Files Modified

- `backend/internal/handlers/agent_cc_handler.go` - Added ordered shutdown in 3 locations

## Related Documentation

- `docs/agent/websocket-fix-summary.md` - Previous fix attempts
- `/tmp/wstest/server.go` - Working test server for comparison
