## WebSocket协作系统实现状态

### 已完成的工作

#### 1. 数据库层 
- [x] 创建 `takeover_requests` 表 (`scripts/create_takeover_requests_table.sql`)
  - 存储接管请求状态（pending, approved, rejected, expired）
  - 支持多服务器环境的中心协调
  - 包含索引优化查询性能

#### 2. 模型层 
- [x] 创建 `TakeoverRequest` 模型 (`backend/internal/models/takeover_request.go`)
  - 完整的字段定义
  - 辅助方法：IsExpired(), IsPending()

#### 3. WebSocket基础设施 
- [x] Hub管理器 (`backend/internal/websocket/hub.go`)
  - 管理所有WebSocket连接
  - 按session_id索引客户端
  - 支持点对点消息和广播
  - 线程安全的连接管理
  
- [x] Client客户端 (`backend/internal/websocket/client.go`)
  - 读写分离的消息处理
  - 自动心跳检测（Ping/Pong）
  - 优雅的连接关闭

- [x] WebSocket Handler (`backend/internal/handlers/websocket_handler.go`)
  - 处理WebSocket连接升级
  - 提供连接状态查询API

#### 4. 系统集成 
- [x] 在 `main.go` 中初始化WebSocket Hub
- [x] 在 `router.go` 中添加WebSocket路由
  - `GET /api/v1/ws/editing/:session_id` - WebSocket连接
  - `GET /api/v1/ws/sessions` - 查询已连接会话

### 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                     多服务器环境                              │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  Server 1          Server 2          Server 3                │
│  ┌──────┐         ┌──────┐         ┌──────┐                │
│  │ Hub  │         │ Hub  │         │ Hub  │                 │
│  │ WS1  │         │ WS2  │         │ WS3  │                 │
│  └──┬───┘         └──┬───┘         └──┬───┘                │
│     │                │                │                      │
│     └────────────────┼────────────────┘                     │
│                      │                                       │
│                      ▼                                       │
│            ┌──────────────────┐                             │
│            │   PostgreSQL     │                             │
│            │  (中心协调者)     │                             │
│            │                  │                             │
│            │ takeover_requests│                             │
│            └──────────────────┘                             │
│                                                               │
└─────────────────────────────────────────────────────────────┘

工作流程：
1. 数据库存储所有状态（必需，支持多服务器）
2. WebSocket提供实时推送（可选，性能优化）
3. 前端轮询作为降级方案（WebSocket断开时）
```

### 待实现的功能

#### 5. 接管请求服务层 ⏳
需要在 `backend/services/resource_editing_service.go` 中添加：

```go
// RequestTakeover 请求接管编辑
func (s *ResourceEditingService) RequestTakeover(
    resourceID uint,
    requesterUserID string,
    requesterName string,
    requesterSessionID string,
    targetSessionID string,
) (*models.TakeoverRequest, error)

// RespondToTakeover 响应接管请求
func (s *ResourceEditingService) RespondToTakeover(
    requestID uint,
    approved bool,
) error

// GetPendingRequests 获取待处理的接管请求
func (s *ResourceEditingService) GetPendingRequests(
    targetSessionID string,
) ([]models.TakeoverRequest, error)

// GetRequestStatus 获取请求状态
func (s *ResourceEditingService) GetRequestStatus(
    requestID uint,
) (*models.TakeoverRequest, error)

// CleanupExpiredRequests 清理过期请求（后台任务）
func (s *ResourceEditingService) CleanupExpiredRequests() error
```

#### 6. 接管请求Handler ⏳
创建 `backend/internal/handlers/takeover_handler.go`：

```go
// RequestTakeover 请求接管
// POST /api/v1/workspaces/:id/resources/:resource_id/editing/takeover-request
func (h *TakeoverHandler) RequestTakeover(c *gin.Context)

// RespondToTakeover 响应接管请求
// POST /api/v1/workspaces/:id/resources/:resource_id/editing/takeover-response
func (h *TakeoverHandler) RespondToTakeover(c *gin.Context)

// GetPendingRequests 获取待处理请求
// GET /api/v1/workspaces/:id/resources/:resource_id/editing/pending-requests
func (h *TakeoverHandler) GetPendingRequests(c *gin.Context)

// GetRequestStatus 获取请求状态
// GET /api/v1/workspaces/:id/resources/:resource_id/editing/request-status/:request_id
func (h *TakeoverHandler) GetRequestStatus(c *gin.Context)
```

#### 7. 路由配置 ⏳
在 `backend/internal/router/router_workspace.go` 中添加接管请求路由

#### 8. 前端WebSocket服务 ⏳
创建 `frontend/src/services/websocket.ts`：

```typescript
class WebSocketService {
  connect(sessionId: string): void
  disconnect(): void
  on(event: string, callback: (data: any) => void): void
  off(event: string, callback: (data: any) => void): void
  send(type: string, data: any): void
}
```

#### 9. 前端UI组件 ⏳
- `frontend/src/components/TakeoverRequestDialog.tsx` - 被接管方确认对话框
- `frontend/src/components/TakeoverWaitingDialog.tsx` - 接管方等待对话框

#### 10. EditResource集成 ⏳
在 `frontend/src/pages/EditResource.tsx` 中：
- 建立WebSocket连接
- 监听接管请求事件
- 处理接管响应
- 集成轮询降级方案

### 实现优先级

**Phase 1: 核心功能（必需）**
1. 接管请求服务层方法
2. 接管请求Handler
3. 路由配置
4. 数据库表创建（运行SQL脚本）

**Phase 2: 前端集成（必需）**
5. WebSocket服务
6. UI组件
7. EditResource集成

**Phase 3: 优化和测试（推荐）**
8. 后台清理过期请求任务
9. 错误处理和边界情况
10. 多用户协作测试

### 技术要点

1. **多服务器支持**
   - 数据库作为唯一真实来源
   - WebSocket仅用于实时推送优化
   - 前端轮询作为降级方案

2. **状态管理**
   - pending: 等待响应
   - approved: 已同意
   - rejected: 已拒绝
   - expired: 已超时（30秒）

3. **安全性**
   - 所有API需要JWT认证
   - 验证用户权限
   - 防止恶意接管

4. **用户体验**
   - 30秒倒计时
   - 清晰的提示信息
   - 区分同用户/不同用户场景

### 下一步行动

运行以下命令创建数据库表：
```bash
psql -U postgres -d iac_platform -f scripts/create_takeover_requests_table.sql
```

然后继续实现Phase 1的服务层和Handler。
