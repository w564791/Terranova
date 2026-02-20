# 接管编辑确认功能实现计划

## 功能需求

实现被接管方的实时确认机制：
- 当用户A在窗口1编辑资源时
- 用户A或者用户B在窗口2尝试接管编辑
- 窗口1（被接管方）实时收到通知，弹出确认对话框
- 窗口1有30秒倒计时来决定是否同意被接管
- 如果拒绝或超时未响应，接管失败
- 如果同意，接管成功

### 场景分类

#### 场景1：同一用户多窗口接管
- 用户A在窗口1编辑
- 用户A在窗口2尝试接管
- 窗口1收到通知："您在另一个窗口尝试接管编辑"
- 这是合理的操作，但仍需确认避免误操作

#### 场景2：不同用户接管
- 用户A在窗口1编辑
- 用户B在窗口2尝试接管
- 窗口1收到通知："用户B尝试接管编辑"
- 需要用户A明确同意才能接管
- 如果用户A拒绝，用户B应该收到拒绝通知

## 技术方案

### 架构说明

**多服务器环境支持**：
- 系统可能部署多个后端服务器实例（负载均衡）
- WebSocket连接可能分布在不同的服务器上
- **数据库作为中心协调者**：所有接管请求状态存储在数据库中
- 前端通过**轮询数据库**检测接管请求和响应状态
- WebSocket仅用于**可选的实时推送优化**，不是必需的

### 1. 后端实现

#### 1.1 数据库表设计（核心）
```sql
-- 接管请求表
CREATE TABLE takeover_requests (
    id SERIAL PRIMARY KEY,
    resource_id INTEGER NOT NULL,
    requester_user_id VARCHAR(255) NOT NULL,
    requester_name VARCHAR(255) NOT NULL,
    requester_session VARCHAR(255) NOT NULL,
    target_user_id VARCHAR(255) NOT NULL,
    target_session VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',  -- pending, approved, rejected, expired
    is_same_user BOOLEAN NOT NULL DEFAULT false,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_target_session (target_session, status),
    INDEX idx_requester_session (requester_session, status),
    INDEX idx_resource_status (resource_id, status),
    INDEX idx_expires_at (expires_at, status)
);
```

#### 1.2 WebSocket服务（可选优化）
```go
// backend/internal/websocket/hub.go
type Hub struct {
    clients    map[string]*Client  // sessionID -> Client
    broadcast  chan Message
    register   chan *Client
    unregister chan *Client
}

type Client struct {
    hub       *Hub
    conn      *websocket.Conn
    send      chan []byte
    sessionID string
    userID    string
}

type Message struct {
    Type      string      `json:"type"`
    SessionID string      `json:"session_id"`
    Data      interface{} `json:"data"`
}
```

#### 1.2 接管请求状态
```go
// backend/internal/models/takeover_request.go
type TakeoverRequest struct {
    ID               uint      `gorm:"primaryKey"`
    ResourceID       uint      `json:"resource_id"`
    RequesterUserID  string    `json:"requester_user_id"`      // 请求接管的用户ID
    RequesterName    string    `json:"requester_name"`         // 请求者用户名
    RequesterSession string    `json:"requester_session"`      // 请求者的session_id
    TargetUserID     string    `json:"target_user_id"`         // 被接管的用户ID
    TargetSession    string    `json:"target_session"`         // 被接管的session_id
    Status           string    `json:"status"`                 // pending, approved, rejected, expired
    IsSameUser       bool      `json:"is_same_user"`           // 是否同一用户
    ExpiresAt        time.Time `json:"expires_at"`             // 30秒后过期
    CreatedAt        time.Time `json:"created_at"`
    UpdatedAt        time.Time `json:"updated_at"`
}
```

#### 1.3 API端点
```
POST   /api/v1/workspaces/:id/resources/:resource_id/editing/takeover-request
       请求体: { "target_session_id": "xxx" }
       返回: { "request_id": 123, "status": "pending", "expires_at": "..." }

POST   /api/v1/workspaces/:id/resources/:resource_id/editing/takeover-response
       请求体: { "request_id": 123, "approved": true/false }
       返回: { "status": "approved/rejected" }

GET    /api/v1/ws/editing/:session_id  (WebSocket连接)
       消息类型:
       - takeover_request: 接管请求通知
       - takeover_approved: 接管被批准
       - takeover_rejected: 接管被拒绝
       - takeover_expired: 接管请求超时
```

#### 1.4 接管流程（后端）

```go
// 1. 用户B点击接管编辑
func (s *ResourceEditingService) RequestTakeover(
    resourceID uint,
    requesterUserID string,
    requesterSessionID string,
    targetSessionID string,
) (*TakeoverRequest, error) {
    // 查找目标session的锁
    var targetLock models.ResourceLock
    if err := s.db.Where("resource_id = ? AND session_id = ?", 
        resourceID, targetSessionID).First(&targetLock).Error; err != nil {
        return nil, errors.New("目标session不存在")
    }
    
    // 创建接管请求
    request := TakeoverRequest{
        ResourceID:       resourceID,
        RequesterUserID:  requesterUserID,
        RequesterSession: requesterSessionID,
        TargetUserID:     targetLock.EditingUserID,
        TargetSession:    targetSessionID,
        Status:           "pending",
        IsSameUser:       requesterUserID == targetLock.EditingUserID,
        ExpiresAt:        time.Now().Add(30 * time.Second),
    }
    
    if err := s.db.Create(&request).Error; err != nil {
        return nil, err
    }
    
    // 通过WebSocket通知被接管方
    hub.SendToSession(targetSessionID, Message{
        Type: "takeover_request",
        Data: request,
    })
    
    return &request, nil
}

// 2. 被接管方响应
func (s *ResourceEditingService) RespondToTakeover(
    requestID uint,
    approved bool,
) error {
    var request TakeoverRequest
    if err := s.db.First(&request, requestID).Error; err != nil {
        return err
    }
    
    // 检查是否已过期
    if time.Now().After(request.ExpiresAt) {
        request.Status = "expired"
        s.db.Save(&request)
        return errors.New("请求已过期")
    }
    
    if approved {
        request.Status = "approved"
        s.db.Save(&request)
        
        // 执行接管
        s.TakeoverEditing(
            request.ResourceID,
            request.RequesterUserID,
            request.RequesterSession,
            request.TargetSession,
        )
        
        // 通知请求方接管成功
        hub.SendToSession(request.RequesterSession, Message{
            Type: "takeover_approved",
            Data: request,
        })
    } else {
        request.Status = "rejected"
        s.db.Save(&request)
        
        // 通知请求方接管被拒绝
        hub.SendToSession(request.RequesterSession, Message{
            Type: "takeover_rejected",
            Data: request,
        })
    }
    
    return nil
}
```

### 2. 前端实现

#### 2.1 WebSocket连接管理
```typescript
// frontend/src/services/websocket.ts
class WebSocketService {
  private ws: WebSocket | null = null;
  private sessionId: string;
  private listeners: Map<string, (data: any) => void> = new Map();
  private reconnectTimer: number | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;

  connect(sessionId: string) {
    this.sessionId = sessionId;
    this.ws = new WebSocket(`ws://localhost:8080/api/v1/ws/editing/${sessionId}`);
    
    this.ws.onopen = () => {
      console.log(' WebSocket connected');
      this.reconnectAttempts = 0;
    };
    
    this.ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      this.handleMessage(message);
    };
    
    this.ws.onclose = () => {
      console.log('❌ WebSocket disconnected');
      this.attemptReconnect();
    };
    
    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
  }

  private handleMessage(message: any) {
    const { type, data } = message;
    const callbacks = this.listeners.get(type);
    if (callbacks) {
      callbacks.forEach(cb => cb(data));
    }
  }

  on(event: string, callback: (data: any) => void) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, []);
    }
    this.listeners.get(event)!.push(callback);
  }

  off(event: string, callback: (data: any) => void) {
    const callbacks = this.listeners.get(event);
    if (callbacks) {
      const index = callbacks.indexOf(callback);
      if (index > -1) {
        callbacks.splice(index, 1);
      }
    }
  }

  send(type: string, data: any) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type, data }));
    }
  }

  private attemptReconnect() {
    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++;
      this.reconnectTimer = window.setTimeout(() => {
        console.log(`🔄 Reconnecting... (${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
        this.connect(this.sessionId);
      }, 3000);
    }
  }

  disconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.listeners.clear();
  }
}

export const websocketService = new WebSocketService();
```

#### 2.2 被接管方确认对话框
```typescript
// frontend/src/components/TakeoverRequestDialog.tsx
interface TakeoverRequestDialogProps {
  request: {
    id: number;
    requester_name: string;
    requester_user_id: string;
    is_same_user: boolean;
  };
  onApprove: () => void;
  onReject: () => void;
}

const TakeoverRequestDialog: React.FC<TakeoverRequestDialogProps> = ({
  request,
  onApprove,
  onReject,
}) => {
  const [countdown, setCountdown] = useState(30);
  
  useEffect(() => {
    const timer = setInterval(() => {
      setCountdown(prev => {
        if (prev <= 1) {
          onReject(); // 超时自动拒绝
          return 0;
        }
        return prev - 1;
      });
    }, 1000);
    
    return () => clearInterval(timer);
  }, [onReject]);
  
  return (
    <div className={styles.overlay}>
      <div className={styles.dialog}>
        <div className={styles.header}>
          <h3 className={styles.title}>接管编辑请求</h3>
        </div>
        
        <div className={styles.content}>
          <div className={styles.infoBox}>
            <p className={styles.mainText}>
              {request.is_same_user 
                ? '您在另一个窗口尝试接管此编辑会话' 
                : `用户 ${request.requester_name} 尝试接管此编辑会话`}
            </p>
          </div>
          
          <div className={styles.warningBox}>
            <span className={styles.warningIcon}></span>
            <p className={styles.warningText}>
              如果同意接管，当前窗口将无法继续编辑。
              <br />
              您的未保存内容将被保留为草稿。
            </p>
          </div>
          
          <div className={styles.countdownBox}>
            <p className={styles.countdownText}>
              请在 <strong className={styles.countdownNumber}>{countdown}</strong> 秒内做出决定
            </p>
            <p className={styles.countdownHint}>
              超时将自动拒绝接管请求
            </p>
          </div>
        </div>
        
        <div className={styles.actions}>
          <button
            className={styles.btnDanger}
            onClick={onReject}
            type="button"
          >
            拒绝接管
          </button>
          <button
            className={styles.btnPrimary}
            onClick={onApprove}
            type="button"
          >
            同意接管
          </button>
        </div>
      </div>
    </div>
  );
};
```

#### 2.3 接管方等待对话框
```typescript
// frontend/src/components/TakeoverWaitingDialog.tsx
interface TakeoverWaitingDialogProps {
  targetUserName: string;
  isSameUser: boolean;
  onCancel: () => void;
}

const TakeoverWaitingDialog: React.FC<TakeoverWaitingDialogProps> = ({
  targetUserName,
  isSameUser,
  onCancel,
}) => {
  const [countdown, setCountdown] = useState(30);
  
  useEffect(() => {
    const timer = setInterval(() => {
      setCountdown(prev => {
        if (prev <= 1) {
          return 0;
        }
        return prev - 1;
      });
    }, 1000);
    
    return () => clearInterval(timer);
  }, []);
  
  return (
    <div className={styles.overlay}>
      <div className={styles.dialog}>
        <div className={styles.header}>
          <h3 className={styles.title}>等待确认</h3>
        </div>
        
        <div className={styles.content}>
          <div className={styles.infoBox}>
            <p className={styles.mainText}>
              {isSameUser 
                ? '正在等待您的另一个窗口确认接管...' 
                : `正在等待 ${targetUserName} 确认接管...`}
            </p>
          </div>
          
          <div className={styles.countdownBox}>
            <p className={styles.countdownText}>
              剩余时间: <strong className={styles.countdownNumber}>{countdown}</strong> 秒
            </p>
          </div>
          
          <div className={styles.loadingIndicator}>
            <div className={styles.spinner}></div>
            <p>等待对方响应...</p>
          </div>
        </div>
        
        <div className={styles.actions}>
          <button
            className={styles.btnSecondary}
            onClick={onCancel}
            type="button"
          >
            取消请求
          </button>
        </div>
      </div>
    </div>
  );
};
```

#### 2.4 EditResource集成（完整流程）
```typescript
// 在EditResource中
const [showTakeoverRequestDialog, setShowTakeoverRequestDialog] = useState(false);
const [takeoverRequest, setTakeoverRequest] = useState<any>(null);
const [showTakeoverWaitingDialog, setShowTakeoverWaitingDialog] = useState(false);

// WebSocket连接
useEffect(() => {
  if (!isCloneMode && sessionId) {
    websocketService.connect(sessionId);
    
    // 监听接管请求（被接管方）
    const handleTakeoverRequest = (data: any) => {
      setTakeoverRequest(data);
      setShowTakeoverRequestDialog(true);
    };
    
    // 监听接管结果（接管方）
    const handleTakeoverApproved = () => {
      setShowTakeoverWaitingDialog(false);
      showToast('接管成功', 'success');
      // 刷新页面或重新初始化编辑会话
      window.location.reload();
    };
    
    const handleTakeoverRejected = () => {
      setShowTakeoverWaitingDialog(false);
      showToast('对方拒绝了接管请求', 'warning');
    };
    
    const handleTakeoverExpired = () => {
      setShowTakeoverWaitingDialog(false);
      showToast('接管请求已超时', 'warning');
    };
    
    websocketService.on('takeover_request', handleTakeoverRequest);
    websocketService.on('takeover_approved', handleTakeoverApproved);
    websocketService.on('takeover_rejected', handleTakeoverRejected);
    websocketService.on('takeover_expired', handleTakeoverExpired);
    
    return () => {
      websocketService.off('takeover_request', handleTakeoverRequest);
      websocketService.off('takeover_approved', handleTakeoverApproved);
      websocketService.off('takeover_rejected', handleTakeoverRejected);
      websocketService.off('takeover_expired', handleTakeoverExpired);
      websocketService.disconnect();
    };
  }
}, [sessionId, isCloneMode]);

// 修改原有的接管逻辑
const handleTakeoverClick = async () => {
  if (!sessionToTakeover) return;
  
  try {
    // 发送接管请求
    const response = await api.post(
      `/workspaces/${id}/resources/${resourceId}/editing/takeover-request`,
      { target_session_id: sessionToTakeover.session_id }
    );
    
    // 显示等待对话框
    setShowTakeoverWaitingDialog(true);
    setShowTakeoverDialog(false);
  } catch (error) {
    showToast('发送接管请求失败', 'error');
  }
};

// 被接管方响应
const handleApproveTakeover = async () => {
  if (!takeoverRequest) return;
  
  try {
    await api.post(
      `/workspaces/${id}/resources/${resourceId}/editing/takeover-response`,
      { request_id: takeoverRequest.id, approved: true }
    );
    
    setShowTakeoverRequestDialog(false);
    showToast('已同意接管', 'info');
    
    // 清理并返回资源查看页面
    const storageKey = `editing_session_${id}_${resourceId}`;
    sessionStorage.removeItem(storageKey);
    navigate(`/workspaces/${id}/resources/${resourceId}`);
  } catch (error) {
    showToast('响应接管请求失败', 'error');
  }
};

const handleRejectTakeover = async () => {
  if (!takeoverRequest) return;
  
  try {
    await api.post(
      `/workspaces/${id}/resources/${resourceId}/editing/takeover-response`,
      { request_id: takeoverRequest.id, approved: false }
    );
    
    setShowTakeoverRequestDialog(false);
    showToast('已拒绝接管', 'info');
  } catch (error) {
    showToast('响应接管请求失败', 'error');
  }
};
```

### 3. 完整流程图（基于数据库轮询）

```
用户A窗口1（编辑中）                用户A/B窗口2（尝试接管）              数据库（中心协调）
     |                                    |                                |
     | 3秒轮询检查接管请求                 |                                |
     | GET /takeover-requests?            |                                |
     |     target_session=xxx             |                                |
     |                                    |                                |
     |                                    |--- POST takeover-request ----->|
     |                                    |    写入数据库                   |
     |                                    |<--- 返回request_id ------------|
     |                                    |                                |
     |                                    | 显示等待对话框                  |
     |                                    | 开始轮询请求状态                |
     |                                    | GET /takeover-requests/:id     |
     |                                    |                                |
     |<--- 轮询检测到pending请求 ---------|                                |
     |     (最多3秒延迟)                  |                                |
     |                                    |                                |
     | 显示确认对话框                      |                                |
     | (30秒倒计时)                       |                                |
     |                                    |                                |
     |--- POST takeover-response -------->|                                |
     |    更新数据库状态                   |                                |
     |    (approved/rejected)             |                                |
     |                                    |                                |
     |                                    |<--- 轮询检测到状态变化 ---------|
     |                                    |     (最多3秒延迟)              |
     |                                    |                                |
     |                                    | 如果approved:                  |
     |                                    |   - 关闭等待对话框              |
     |                                    |   - 刷新页面，开始编辑          |
     |                                    |                                |
     |                                    | 如果rejected:                  |
     |                                    |   - 显示"对方拒绝"提示          |
     |                                    |   - 返回资源查看页面            |
     |                                    |                                |
     | 如果approved:                      |                                |
     |   - 保存草稿                       |                                |
     |   - 返回资源查看页面                |                                |
     |                                    |                                |
     
注：WebSocket可作为可选优化，但核心依赖数据库轮询，确保多服务器环境下的可靠性
```

### 4. 实现步骤

1. **后端WebSocket服务** (2-3小时)
   - 实现Hub和Client管理
   - 添加WebSocket路由和握手
   - 实现消息广播和点对点发送
   - 处理连接断开和重连

2. **后端接管请求管理** (1-2小时)
   - 创建TakeoverRequest数据表和模型
   - 实现RequestTakeover API（创建请求）
   - 实现RespondToTakeover API（响应请求）
   - 添加请求超时自动处理（后台任务）
   - 修改TakeoverEditing逻辑支持请求确认

3. **前端WebSocket服务** (1-2小时)
   - 实现WebSocketService类
   - 添加自动重连机制
   - 实现事件监听和消息发送
   - 处理连接状态管理

4. **前端UI组件** (1-2小时)
   - 创建TakeoverRequestDialog（被接管方）
   - 创建TakeoverWaitingDialog（接管方）
   - 添加相关CSS样式
   - 实现倒计时和自动超时逻辑

5. **EditResource集成** (1小时)
   - 集成WebSocket连接
   - 修改原有接管逻辑
   - 添加状态管理
   - 处理各种响应场景

6. **测试和调试** (1-2小时)
   - 同一用户多窗口测试
   - 不同用户接管测试
   - 超时处理测试
   - 网络断开重连测试
   - 边界情况测试

**总计：7-12小时开发时间**

## 推荐方案：基于数据库轮询（适合多服务器环境）

### 核心优势
1. **多服务器兼容**：数据库作为唯一真实来源，所有服务器实例共享状态
2. **实现简单**：利用现有的3秒状态轮询机制
3. **可靠性高**：不依赖WebSocket连接状态
4. **易于调试**：所有状态变化都记录在数据库中

### 实现步骤（推荐）

#### 阶段1：数据库和API（必需）- 2小时
1. 创建 `takeover_requests` 表
2. 创建 TakeoverRequest 模型
3. 实现 RequestTakeover API
4. 实现 RespondToTakeover API
5. 添加 GetPendingRequests API（用于轮询）

#### 阶段2：前端轮询和UI（必需）- 2小时
1. 在现有3秒轮询中添加接管请求检测
2. 创建 TakeoverRequestDialog 组件（被接管方）
3. 创建 TakeoverWaitingDialog 组件（接管方）
4. 修改接管逻辑使用新的请求-响应流程

#### 阶段3：WebSocket优化（可选）- 3-4小时
1. 实现WebSocket Hub
2. 添加实时推送
3. 减少轮询频率（从3秒改为10秒）
4. WebSocket断开时自动降级到轮询

**基础方案（仅数据库轮询）：4小时**
**完整方案（数据库+WebSocket）：7-8小时**

### 数据库轮询API设计

```go
// GET /api/v1/workspaces/:id/resources/:resource_id/editing/pending-requests
// 查询参数: ?target_session=xxx
// 返回: 
{
  "requests": [
    {
      "id": 123,
      "requester_name": "User B",
      "requester_user_id": "user-b-id",
      "is_same_user": false,
      "expires_at": "2025-10-28T19:30:00Z",
      "created_at": "2025-10-28T19:29:30Z"
    }
  ]
}

// GET /api/v1/workspaces/:id/resources/:resource_id/editing/request-status/:request_id
// 返回:
{
  "id": 123,
  "status": "approved",  // pending, approved, rejected, expired
  "updated_at": "2025-10-28T19:29:45Z"
}
```

### 前端轮询集成

```typescript
// 在现有的statusPollTimerRef轮询中添加
statusPollTimerRef.current = window.setInterval(async () => {
  try {
    // 1. 原有的编辑状态检查
    const status = await ResourceEditingService.getEditingStatus(...);
    setOtherEditors(status.editors.filter(e => !e.is_current_session));
    
    // 2. 新增：检查是否有pending的接管请求（被接管方）
    const pendingRequests = await api.get(
      `/workspaces/${id}/resources/${resourceId}/editing/pending-requests?target_session=${sessionId}`
    );
    
    if (pendingRequests.data.requests && pendingRequests.data.requests.length > 0) {
      const request = pendingRequests.data.requests[0];
      setTakeoverRequest(request);
      setShowTakeoverRequestDialog(true);
    }
    
    // 3. 新增：如果正在等待接管响应，检查请求状态（接管方）
    if (waitingForTakeoverRequestId) {
      const requestStatus = await api.get(
        `/workspaces/${id}/resources/${resourceId}/editing/request-status/${waitingForTakeoverRequestId}`
      );
      
      if (requestStatus.data.status === 'approved') {
        setShowTakeoverWaitingDialog(false);
        showToast('接管成功', 'success');
        window.location.reload();
      } else if (requestStatus.data.status === 'rejected') {
        setShowTakeoverWaitingDialog(false);
        setWaitingForTakeoverRequestId(null);
        showToast('对方拒绝了接管请求', 'warning');
      } else if (requestStatus.data.status === 'expired') {
        setShowTakeoverWaitingDialog(false);
        setWaitingForTakeoverRequestId(null);
        showToast('接管请求已超时', 'warning');
      }
    }
  } catch (error) {
    console.error('状态轮询失败:', error);
  }
}, 3000); // 保持3秒轮询
```

## 最终建议

**推荐实现基于数据库轮询的方案**：
-  完全支持多服务器环境
-  实现简单，开发时间短（4小时）
-  可靠性高，不依赖WebSocket
-  3秒延迟可接受（用户有30秒决策时间）
-  后续可无缝升级到WebSocket优化

WebSocket可以作为第二阶段的性能优化，但不是必需的。
