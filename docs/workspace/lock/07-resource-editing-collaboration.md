# 资源编辑协作系统设计文档

## 1. 概述

本文档描述了IaC平台资源编辑协作系统的设计与实现方案，旨在解决多用户同时编辑资源时的冲突问题，并提供友好的用户体验。

### 1.1 核心目标

- **安全协作**: 防止多用户同时修改导致资源覆盖或冲突
- **友好体验**: 用户可中断编辑并恢复（drift机制），减少误操作
- **多窗口支持**: 同一用户不同session也能提示，避免重复编辑
- **无操作提示**: 用户长期保持界面无操作时提示，1分钟后主动断开该用户的编辑状态，其它用户可以编辑

### 1.2 技术方案

- **锁机制**: 乐观锁（版本号校验）
- **心跳频率**: 5秒一次HTTP心跳
- **超时策略**: 1分钟提示，2分钟自动释放
- **状态同步**: HTTP轮询（5秒间隔）
- **草稿保存**: 自动保存编辑内容到drift表

---

## 2. 数据模型设计

### 2.1 资源锁表 (resource_locks)

记录当前正在编辑资源的用户和会话信息。

```sql
CREATE TABLE resource_locks (
    id SERIAL PRIMARY KEY,
    resource_id INTEGER NOT NULL REFERENCES workspace_resources(id) ON DELETE CASCADE,
    editing_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id VARCHAR(100) NOT NULL,
    lock_type VARCHAR(20) NOT NULL DEFAULT 'optimistic',
    version INTEGER NOT NULL DEFAULT 1,
    last_heartbeat TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(resource_id)
);

CREATE INDEX idx_resource_locks_resource ON resource_locks(resource_id);
CREATE INDEX idx_resource_locks_user ON resource_locks(editing_user_id);
CREATE INDEX idx_resource_locks_heartbeat ON resource_locks(last_heartbeat);
```

**字段说明**:
- `resource_id`: 被锁定的资源ID
- `editing_user_id`: 当前编辑用户ID
- `session_id`: 浏览器会话ID（用于区分同用户多窗口）
- `lock_type`: 锁类型（optimistic/pessimistic），当前版本使用乐观锁
- `version`: 资源版本号，用于乐观锁校验
- `last_heartbeat`: 最后心跳时间，用于判断锁是否过期

### 2.2 资源草稿表 (resource_drifts)

保存用户的临时编辑内容，支持中断恢复。

```sql
CREATE TABLE resource_drifts (
    id SERIAL PRIMARY KEY,
    resource_id INTEGER NOT NULL REFERENCES workspace_resources(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id VARCHAR(100) NOT NULL,
    drift_content JSONB NOT NULL,
    base_version INTEGER NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    last_heartbeat TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(resource_id, user_id, session_id)
);

CREATE INDEX idx_resource_drifts_resource ON resource_drifts(resource_id);
CREATE INDEX idx_resource_drifts_user ON resource_drifts(user_id);
CREATE INDEX idx_resource_drifts_status ON resource_drifts(status);
CREATE INDEX idx_resource_drifts_heartbeat ON resource_drifts(last_heartbeat);
```

**字段说明**:
- `resource_id`: 资源ID
- `user_id`: 编辑用户ID
- `session_id`: 会话ID
- `drift_content`: 草稿内容（JSON格式，包含formData和changeSummary）
- `base_version`: 草稿基于的资源版本号
- `status`: 状态（active/expired/submitted）
- `last_heartbeat`: 最后心跳时间

---

## 3. API接口设计

### 3.1 编辑会话管理

#### 3.1.1 开始编辑

**请求**:
```
POST /api/v1/workspaces/:id/resources/:resourceId/editing/start
Content-Type: application/json

{
  "session_id": "uuid-v4-string"
}
```

**响应**:
```json
{
  "success": true,
  "data": {
    "lock": {
      "id": 1,
      "resource_id": 20,
      "editing_user_id": 5,
      "session_id": "abc-123",
      "version": 3,
      "last_heartbeat": "2025-01-18T07:30:00Z"
    },
    "drift": {
      "id": 10,
      "drift_content": {...},
      "base_version": 3,
      "has_version_conflict": false
    },
    "other_editors": [
      {
        "user_id": 6,
        "user_name": "张三",
        "session_id": "def-456",
        "is_same_user": false,
        "last_heartbeat": "2025-01-18T07:29:55Z"
      }
    ]
  }
}
```

**说明**:
- 创建或更新资源锁
- 检查是否有该用户的未过期drift
- 如果有drift，检查版本是否冲突
- 返回其他正在编辑的用户信息

#### 3.1.2 心跳更新

**请求**:
```
POST /api/v1/workspaces/:id/resources/:resourceId/editing/heartbeat
Content-Type: application/json

{
  "session_id": "uuid-v4-string"
}
```

**响应**:
```json
{
  "success": true,
  "data": {
    "lock_valid": true,
    "version": 3,
    "warning": false
  }
}
```

**说明**:
- 更新锁的last_heartbeat时间
- 更新drift的last_heartbeat时间
- 返回锁是否仍然有效

#### 3.1.3 结束编辑

**请求**:
```
POST /api/v1/workspaces/:id/resources/:resourceId/editing/end
Content-Type: application/json

{
  "session_id": "uuid-v4-string"
}
```

**响应**:
```json
{
  "success": true,
  "message": "编辑会话已结束"
}
```

**说明**:
- 删除资源锁
- 保留drift（用户可能需要恢复）

#### 3.1.4 获取编辑状态

**请求**:
```
GET /api/v1/workspaces/:id/resources/:resourceId/editing/status?session_id=xxx
```

**响应**:
```json
{
  "success": true,
  "data": {
    "is_locked": true,
    "current_version": 3,
    "editors": [
      {
        "user_id": 5,
        "user_name": "李四",
        "session_id": "abc-123",
        "is_current_user": true,
        "is_current_session": true,
        "last_heartbeat": "2025-01-18T07:30:00Z",
        "time_since_heartbeat": 5
      }
    ]
  }
}
```

### 3.2 草稿管理

#### 3.2.1 保存草稿

**请求**:
```
POST /api/v1/workspaces/:id/resources/:resourceId/drift/save
Content-Type: application/json

{
  "session_id": "uuid-v4-string",
  "drift_content": {
    "formData": {...},
    "changeSummary": "更新配置"
  }
}
```

**响应**:
```json
{
  "success": true,
  "data": {
    "drift_id": 10,
    "base_version": 3,
    "saved_at": "2025-01-18T07:30:00Z"
  }
}
```

#### 3.2.2 获取草稿

**请求**:
```
GET /api/v1/workspaces/:id/resources/:resourceId/drift?session_id=xxx
```

**响应**:
```json
{
  "success": true,
  "data": {
    "drift": {
      "id": 10,
      "drift_content": {...},
      "base_version": 3,
      "created_at": "2025-01-18T07:25:00Z",
      "updated_at": "2025-01-18T07:30:00Z"
    },
    "current_version": 3,
    "has_version_conflict": false
  }
}
```

#### 3.2.3 删除草稿

**请求**:
```
DELETE /api/v1/workspaces/:id/resources/:resourceId/drift?session_id=xxx
```

**响应**:
```json
{
  "success": true,
  "message": "草稿已删除"
}
```

#### 3.2.4 接管编辑

**请求**:
```
POST /api/v1/workspaces/:id/resources/:resourceId/drift/takeover
Content-Type: application/json

{
  "session_id": "new-session-id",
  "old_session_id": "old-session-id"
}
```

**响应**:
```json
{
  "success": true,
  "data": {
    "new_lock": {...},
    "new_drift": {...}
  }
}
```

**说明**:
- 将旧session的锁和drift转移到新session
- 旧session会收到接管通知（通过状态轮询）

---

## 4. 前端实现

### 4.1 核心状态管理

在`EditResource.tsx`中添加以下状态:

```typescript
// 编辑会话状态
const [sessionId] = useState(() => generateUUID());
const [editingStatus, setEditingStatus] = useState<EditingStatus | null>(null);
const [otherEditors, setOtherEditors] = useState<EditorInfo[]>([]);
const [hasVersionConflict, setHasVersionConflict] = useState(false);

// 心跳和轮询定时器
const heartbeatTimerRef = useRef<NodeJS.Timeout | null>(null);
const statusPollTimerRef = useRef<NodeJS.Timeout | null>(null);

// 草稿自动保存
const driftSaveTimerRef = useRef<NodeJS.Timeout | null>(null);
```

### 4.2 生命周期管理

#### 4.2.1 进入编辑页面

```typescript
useEffect(() => {
  const initEditing = async () => {
    try {
      // 1. 调用开始编辑API
      const response = await api.post(
        `/workspaces/${id}/resources/${resourceId}/editing/start`,
        { session_id: sessionId }
      );
      
      // 2. 检查是否有drift
      if (response.data.drift) {
        const drift = response.data.drift;
        
        // 3. 检查版本冲突
        if (drift.has_version_conflict) {
          setHasVersionConflict(true);
          // 显示版本冲突对话框
          showDriftRecoveryDialog(drift, true);
        } else {
          // 显示恢复草稿对话框
          showDriftRecoveryDialog(drift, false);
        }
      }
      
      // 4. 显示其他编辑者
      setOtherEditors(response.data.other_editors || []);
      
      // 5. 启动心跳
      startHeartbeat();
      
      // 6. 启动状态轮询
      startStatusPolling();
      
    } catch (error) {
      showToast('初始化编辑会话失败', 'error');
    }
  };
  
  initEditing();
  
  return () => {
    // 清理定时器
    stopHeartbeat();
    stopStatusPolling();
    // 结束编辑会话
    endEditing();
  };
}, []);
```

#### 4.2.2 心跳机制

```typescript
const startHeartbeat = () => {
  heartbeatTimerRef.current = setInterval(async () => {
    try {
      await api.post(
        `/workspaces/${id}/resources/${resourceId}/editing/heartbeat`,
        { session_id: sessionId }
      );
    } catch (error) {
      console.error('心跳失败:', error);
      // 心跳失败可能意味着会话已过期
      showToast('编辑会话已过期，请刷新页面', 'warning');
    }
  }, 5000); // 5秒
};

const stopHeartbeat = () => {
  if (heartbeatTimerRef.current) {
    clearInterval(heartbeatTimerRef.current);
    heartbeatTimerRef.current = null;
  }
};
```

#### 4.2.3 状态轮询

```typescript
const startStatusPolling = () => {
  statusPollTimerRef.current = setInterval(async () => {
    try {
      const response = await api.get(
        `/workspaces/${id}/resources/${resourceId}/editing/status`,
        { params: { session_id: sessionId } }
      );
      
      const status = response.data.data;
      
      // 更新其他编辑者列表
      setOtherEditors(status.editors.filter(e => !e.is_current_session));
      
      // 检查是否被接管
      const currentSession = status.editors.find(e => e.is_current_session);
      if (!currentSession) {
        showToast('编辑已被其他窗口接管', 'warning');
        // 禁用编辑功能
        setEditingDisabled(true);
      }
      
      // 检查版本是否更新
      if (status.current_version > resource.current_version?.version) {
        showToast('资源已被其他用户更新', 'info');
        setHasVersionConflict(true);
      }
      
    } catch (error) {
      console.error('状态轮询失败:', error);
    }
  }, 5000); // 5秒
};

const stopStatusPolling = () => {
  if (statusPollTimerRef.current) {
    clearInterval(statusPollTimerRef.current);
    statusPollTimerRef.current = null;
  }
};
```

#### 4.2.4 草稿自动保存

```typescript
useEffect(() => {
  // 防抖保存
  if (driftSaveTimerRef.current) {
    clearTimeout(driftSaveTimerRef.current);
  }
  
  driftSaveTimerRef.current = setTimeout(async () => {
    try {
      await api.post(
        `/workspaces/${id}/resources/${resourceId}/drift/save`,
        {
          session_id: sessionId,
          drift_content: {
            formData,
            changeSummary
          }
        }
      );
    } catch (error) {
      console.error('保存草稿失败:', error);
    }
  }, 2000); // 2秒防抖
  
}, [formData, changeSummary]);
```

### 4.3 UI组件

#### 4.3.1 编辑状态栏

```typescript
interface EditingStatusBarProps {
  otherEditors: EditorInfo[];
  hasVersionConflict: boolean;
  isDisabled: boolean;
}

const EditingStatusBar: React.FC<EditingStatusBarProps> = ({
  otherEditors,
  hasVersionConflict,
  isDisabled
}) => {
  const getStatusColor = () => {
    if (isDisabled) return 'red';
    if (hasVersionConflict) return 'orange';
    if (otherEditors.some(e => e.is_same_user)) return 'yellow';
    if (otherEditors.length > 0) return 'red';
    return 'green';
  };
  
  const getStatusText = () => {
    if (isDisabled) return '编辑已被接管';
    if (hasVersionConflict) return '资源版本已更新，无法提交';
    if (otherEditors.some(e => e.is_same_user)) {
      return '您在其他窗口正在编辑';
    }
    if (otherEditors.length > 0) {
      return `${otherEditors[0].user_name}正在编辑`;
    }
    return '可以安全编辑';
  };
  
  return (
    <div className={`status-bar status-${getStatusColor()}`}>
      <div className="status-indicator" />
      <span>{getStatusText()}</span>
      {otherEditors.length > 0 && (
        <button onClick={() => showEditorsDialog()}>
          查看详情
        </button>
      )}
    </div>
  );
};
```

#### 4.3.2 草稿恢复对话框

```typescript
interface DriftRecoveryDialogProps {
  drift: DriftInfo;
  hasVersionConflict: boolean;
  onRecover: () => void;
  onDiscard: () => void;
  onCancel: () => void;
}

const DriftRecoveryDialog: React.FC<DriftRecoveryDialogProps> = ({
  drift,
  hasVersionConflict,
  onRecover,
  onDiscard,
  onCancel
}) => {
  return (
    <Dialog>
      <h3>发现未提交的草稿</h3>
      {hasVersionConflict && (
        <div className="warning">
           资源已被其他用户修改，草稿基于旧版本（v{drift.base_version}）
        </div>
      )}
      <p>
        草稿保存于: {formatDate(drift.updated_at)}
      </p>
      <div className="actions">
        <button onClick={onRecover}>
          {hasVersionConflict ? '查看草稿内容' : '恢复草稿'}
        </button>
        <button onClick={onDiscard}>删除草稿</button>
        <button onClick={onCancel}>取消</button>
      </div>
    </Dialog>
  );
};
```

#### 4.3.3 接管确认对话框

```typescript
interface TakeoverConfirmDialogProps {
  otherSession: EditorInfo;
  onConfirm: () => void;
  onCancel: () => void;
}

const TakeoverConfirmDialog: React.FC<TakeoverConfirmDialogProps> = ({
  otherSession,
  onConfirm,
  onCancel
}) => {
  return (
    <Dialog>
      <h3>接管编辑确认</h3>
      <p>
        您在另一个窗口正在编辑此资源
        <br />
        最后活动时间: {formatTimeAgo(otherSession.last_heartbeat)}
      </p>
      <p className="warning">
        接管后，另一个窗口将无法继续编辑
      </p>
      <div className="actions">
        <button onClick={onConfirm}>接管编辑</button>
        <button onClick={onCancel}>取消</button>
      </div>
    </Dialog>
  );
};
```

### 4.4 提交时版本校验

```typescript
const handleSubmit = async () => {
  // 1. 验证变更摘要
  if (!changeSummary.trim()) {
    showToast('请输入变更摘要', 'warning');
    return;
  }
  
  // 2. 检查版本冲突
  if (hasVersionConflict) {
    showToast('资源版本已更新，无法提交。请刷新页面查看最新版本', 'error');
    return;
  }
  
  try {
    setSubmitting(true);
    
    // 3. 构建更新数据
    const updatedTFCode = {
      module: {
        [`${resource?.resource_type}_${resource?.resource_name}`]: [
          {
            source: moduleSource,
            ...formData
          }
        ]
      }
    };
    
    // 4. 提交更新（后端会再次校验版本）
    await api.put(`/workspaces/${id}/resources/${resourceId}`, {
      tf_code: updatedTFCode,
      variables: resource?.current_version?.variables || {},
      change_summary: changeSummary.trim(),
      expected_version: resource?.current_version?.version // 乐观锁版本号
    });
    
    // 5. 删除草稿
    await api.delete(
      `/workspaces/${id}/resources/${resourceId}/drift`,
      { params: { session_id: sessionId } }
    );
    
    // 6. 结束编辑会话
    await api.post(
      `/workspaces/${id}/resources/${resourceId}/editing/end`,
      { session_id: sessionId }
    );
    
    showToast('资源更新成功', 'success');
    navigate(`/workspaces/${id}?tab=resources`);
    
  } catch (error: any) {
    if (error.response?.status === 409) {
      // 版本冲突
      showToast('提交失败：资源已被其他用户修改', 'error');
      setHasVersionConflict(true);
    } else {
      showToast(extractErrorMessage(error), 'error');
    }
  } finally {
    setSubmitting(false);
  }
};
```

---

## 5. 后端实现

### 5.1 Service层

创建`backend/services/resource_editing_service.go`:

```go
package services

import (
    "time"
    "iac-platform/internal/models"
    "gorm.io/gorm"
)

type ResourceEditingService struct {
    db *gorm.DB
}

func NewResourceEditingService(db *gorm.DB) *ResourceEditingService {
    return &ResourceEditingService{db: db}
}

// StartEditing 开始编辑
func (s *ResourceEditingService) StartEditing(
    resourceID uint,
    userID uint,
    sessionID string,
) (*models.ResourceLock, *models.ResourceDrift, []EditorInfo, error) {
    // 实现逻辑...
}

// Heartbeat 心跳更新
func (s *ResourceEditingService) Heartbeat(
    resourceID uint,
    userID uint,
    sessionID string,
) error {
    // 实现逻辑...
}

// EndEditing 结束编辑
func (s *ResourceEditingService) EndEditing(
    resourceID uint,
    userID uint,
    sessionID string,
) error {
    // 实现逻辑...
}

// GetEditingStatus 获取编辑状态
func (s *ResourceEditingService) GetEditingStatus(
    resourceID uint,
    sessionID string,
) (*EditingStatusResponse, error) {
    // 实现逻辑...
}

// SaveDrift 保存草稿
func (s *ResourceEditingService) SaveDrift(
    resourceID uint,
    userID uint,
    sessionID string,
    content map[string]interface{},
) (*models.ResourceDrift, error) {
    // 实现逻辑...
}

// GetDrift 获取草稿
func (s *ResourceEditingService) GetDrift(
    resourceID uint,
    userID uint,
    sessionID string,
) (*models.ResourceDrift, bool, error) {
    // 实现逻辑...
    // 返回: drift, hasVersionConflict, error
}

// DeleteDrift 删除草稿
func (s *ResourceEditingService) DeleteDrift(
    resourceID uint,
    userID uint,
    sessionID string,
) error {
    // 实现逻辑...
}

// TakeoverEditing 接管编辑
func (s *ResourceEditingService) TakeoverEditing(
    resourceID uint,
    userID uint,
    newSessionID string,
    oldSessionID string,
) error {
    // 实现逻辑...
}

// CleanupExpiredLocks 清理过期锁（后台任务）
func (s *ResourceEditingService) CleanupExpiredLocks() error {
    // 删除2分钟无心跳的锁
    return s.db.Where("last_heartbeat < ?", time.Now().Add(-2*time.Minute)).
        Delete(&models.ResourceLock{}).Error
}
```

### 5.2 Controller层

在`backend/controllers/resource_controller.go`中添加编辑协作相关接口。

### 5.3 后台清理任务

在`backend/main.go`中添加定时任务:

```go
// 启动后台清理任务
go func() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        if err := editingService.CleanupExpiredLocks(); err != nil {
            log.Printf("清理过期锁失败: %v", err)
        }
    }
}()
```

---

## 6. 用户交互流程

### 6.1 正常编辑流程

```
1. 用户打开编辑页面
   ↓
2. 前端调用 /editing/start
   ↓
3. 后端创建锁和drift（如果有旧drift则返回）
   ↓
4. 前端显示编辑界面，启动心跳和状态轮询
   ↓
5. 用户编辑，自动保存draft
   ↓
6. 用户提交，校验版本号
   ↓
7. 提交成功，删除drift，结束编辑会话
```

### 6.2 版本冲突流程

```
1. 用户A正在编辑资源（v3）
   ↓
2. 用户B提交了修改，资源更新到v4
   ↓
3. 用户A的状态轮询检测到版本更新
   ↓
4. 前端显示警告：资源已被更新
   ↓
5. 用户A尝试提交
   ↓
6. 后端校验版本号不匹配，返回409错误
   ↓
7. 前端提示：无法提交，请刷新页面查看最新版本
```

### 6.3 多窗口接管流程

```
1. 用户在窗口A编辑资源
   ↓
2. 用户在窗口B打开同一资源
   ↓
3. 窗口B检测到同用户其他session正在编辑
   ↓
4. 显示接管确认对话框
   ↓
5. 用户确认接管
   ↓
6. 调用 /drift/takeover API
   ↓
7. 窗口A的状态轮询检测到被接管
   ↓
8. 窗口A显示警告并禁用编辑
```

### 6.4 草稿恢复流程

```
1. 用户打开编辑页面
   ↓
2. 后端检测到有未过期drift
   ↓
3. 检查drift版本号与当前版本
   ↓
4a. 版本一致：显示"恢复草稿"对话框
    ↓
    用户选择恢复 → 加载drift内容
    
4b. 版本冲突：显示"草稿已过期"对话框
    ↓
    用户选择查看 → 只读模式查看
    或
    用户选择删除 → 删除drift
```

---

## 7. 测试场景

### 7.1 单用户场景

- [ ] 正常编辑和提交
- [ ] 编辑中断后恢复
- [ ] 多窗口编辑和接管
- [ ] 心跳超时自动释放

### 7.2 多用户场景

- [ ] 两个用户同时编辑同一资源
- [ ] 一个用户提交后另一个用户的版本冲突处理
- [ ] 状态实时更新和提示

### 7.3 异常场景

- [ ] 网络断开后重连
- [ ] 浏览器崩溃后恢复
- [ ] 后端重启后状态恢复

---

## 8. 性能优化

### 8.1 数据库优化

- 在`resource_id`、`user_id`、`last_heartbeat`字段上创建索引
- 定期清理过期的drift记录（status='expired'）
- 使用数据库连接池

### 8.2 前端优化

- 心跳和状态轮询使用防抖
- 草稿保存使用防抖（2秒）
- 避免不必要的状态更新

### 8.3 后端优化

- 使用Redis缓存编辑状态（可选）
- 批量清理过期锁
- 异步处理非关键操作

---

## 9. 安全考虑

### 9.1 权限校验

- 所有API都需要验证用户身份
- 只有资源所属workspace的成员才能编辑
- 接管
