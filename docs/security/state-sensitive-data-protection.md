# State 敏感数据保护方案（API 分离版）

## 1. 背景

Terraform state 文件中可能包含敏感数据，例如：
- 数据库密码
- API 密钥
- 证书私钥
- 访问令牌
- 其他机密信息

目前系统在 State Preview 页面直接展示完整的 state JSON 内容，存在敏感数据泄露风险。

## 2. 设计目标

1. **API 分离**：将 state 元数据和 state 内容分离到不同的 API
2. **新增权限**：添加 `WORKSPACE_STATE_SENSITIVE` 权限，用于控制是否可以查看 state 内容
3. **审计日志**：记录谁在什么时候查看了 state 数据
4. **零影响**：确保对 Execute 流程（Local/Agent/K8s Agent）无影响

## 3. API 设计

### 3.1 现有 API 变更

| API | 变更前 | 变更后 |
|-----|--------|--------|
| `GET /state-versions` | 返回版本列表（含 content） | 返回版本列表（**不含** content） |
| `GET /state-versions/:version` | 返回完整 state | 返回元数据（**不含** content） |
| `GET /state-versions/:version/metadata` | 返回元数据 | 保持不变 |

### 3.2 新增 API

| API | 权限要求 | 说明 |
|-----|----------|------|
| `GET /state-versions/:version/retrieve` | `WORKSPACE_STATE_SENSITIVE` | 返回完整 state 内容（用于查看和下载） |

### 3.3 API 详细设计

#### 3.3.1 获取 State 版本列表（变更）

```
GET /api/v1/workspaces/:id/state-versions
```

**响应**（不再包含 content 字段）：
```json
{
  "data": [
    {
      "id": 123,
      "workspace_id": "ws-xxx",
      "version": 33,
      "checksum": "sha256:abc...",
      "size_bytes": 12345,
      "task_id": 456,
      "created_by": "user-xxx",
      "created_at": "2026-01-09T10:00:00Z"
    }
  ]
}
```

#### 3.3.2 获取 State 版本详情（变更）

```
GET /api/v1/workspaces/:id/state-versions/:version
```

**响应**（不再包含 content 字段）：
```json
{
  "data": {
    "id": 123,
    "workspace_id": "ws-xxx",
    "version": 33,
    "checksum": "sha256:abc...",
    "size_bytes": 12345,
    "task_id": 456,
    "created_by": "user-xxx",
    "created_at": "2026-01-09T10:00:00Z",
    "resource_count": 5,
    "output_count": 3
  }
}
```

#### 3.3.3 获取 State 内容（新增）

```
GET /api/v1/workspaces/:id/state-versions/:version/retrieve
```

**权限要求**：`WORKSPACE_STATE_SENSITIVE` (READ)

**响应**：
```json
{
  "data": {
    "version": 33,
    "content": {
      "version": 4,
      "terraform_version": "1.5.0",
      "resources": [...],
      "outputs": {...}
    }
  },
  "audit": {
    "accessed_at": "2026-01-09T10:00:00Z",
    "accessed_by": "user-xxx"
  }
}
```

**错误响应**（无权限）：
```json
{
  "error": "Permission denied",
  "message": "You don't have WORKSPACE_STATE_SENSITIVE permission to view state content",
  "required_permission": "WORKSPACE_STATE_SENSITIVE"
}
```

## 4. 对执行模式的影响分析

### 4.1 架构概述

系统支持三种执行模式：

| 模式 | 数据访问方式 | State 获取方式 |
|------|-------------|---------------|
| **Local** | 直接访问数据库 | `LocalDataAccessor.GetLatestStateVersion()` |
| **Agent** | 通过 Agent API | `RemoteDataAccessor.GetLatestStateVersion()` |
| **K8s Agent** | 通过 Agent API | `RemoteDataAccessor.GetLatestStateVersion()` |

### 4.2 关键代码分析

Execute 流程中获取 State 的代码（`terraform_executor.go`）：

```go
// PrepareStateFileWithLogging 准备State文件
func (s *TerraformExecutor) PrepareStateFileWithLogging(...) error {
    // 使用 DataAccessor 获取最新的State版本
    stateVersion, err := s.dataAccessor.GetLatestStateVersion(workspace.WorkspaceID)
    // ...
}
```

### 4.3 影响评估

#### 4.3.1 Local 模式

| 组件 | 影响 | 说明 |
|------|------|------|
| `LocalDataAccessor` | **无影响** | 直接查询数据库，不经过 HTTP API |
| `TerraformExecutor` | **无影响** | 使用 DataAccessor 接口 |
| 用户 API | **有变更** | 需要修改 HTTP handler |

#### 4.3.2 Agent 模式

| 组件 | 影响 | 说明 |
|------|------|------|
| `RemoteDataAccessor` | **无影响** | 调用内部 Agent API（`/agent/task-data`） |
| `AgentAPIClient` | **无影响** | 使用 Agent Token 认证，不受用户权限影响 |
| `TerraformExecutor` | **无影响** | 使用 DataAccessor 接口 |

#### 4.3.3 K8s Agent 模式

| 组件 | 影响 | 说明 |
|------|------|------|
| `RemoteDataAccessor` | **无影响** | 与 Agent 模式相同 |
| K8s Job | **无影响** | 使用 Job Token 认证 |
| `TerraformExecutor` | **无影响** | 使用 DataAccessor 接口 |

### 4.4 结论

**Execute 流程完全不受影响**，因为：

1. Execute 流程使用 `DataAccessor` 接口，不经过用户面向的 HTTP API
2. Agent/K8s Agent 模式使用内部 Agent API（`/agent/task-data`），有独立的认证机制
3. 新增的权限检查只影响用户面向的 API（`/api/v1/workspaces/:id/state-versions/:version/retrieve`）

## 5. 实施方案

### 5.1 新增权限定义

创建文件：`scripts/add_state_sensitive_permission.sql`

```sql
-- 添加 WORKSPACE_STATE_SENSITIVE 权限定义
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('wspm-workspace-state-sensitive', 'WORKSPACE_STATE_SENSITIVE', 'WORKSPACE_STATE_SENSITIVE', 'WORKSPACE', 
     'State 内容查看', '查看 State 文件的完整内容（包含敏感数据）', true, NOW())
ON CONFLICT (id) DO NOTHING;
```

### 5.2 后端实现

#### 5.2.1 修改 State Handler

修改文件：`backend/internal/handlers/state_handler.go`

```go
// GetStateVersion 获取指定版本的 State 元数据（不含 content）
// GET /api/workspaces/:id/state-versions/:version
func (h *StateHandler) GetStateVersion(c *gin.Context) {
    workspaceID := c.Param("id")
    versionStr := c.Param("version")

    version, err := strconv.Atoi(versionStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version number"})
        return
    }

    // 获取指定版本（只返回元数据）
    stateVersion, err := h.stateService.GetStateVersionMetadata(workspaceID, version)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "State version not found"})
        return
    }

    // 返回元数据（不含 content）
    c.JSON(http.StatusOK, gin.H{
        "data": map[string]interface{}{
            "id":             stateVersion.ID,
            "workspace_id":   stateVersion.WorkspaceID,
            "version":        stateVersion.Version,
            "checksum":       stateVersion.Checksum,
            "size_bytes":     stateVersion.SizeBytes,
            "task_id":        stateVersion.TaskID,
            "created_by":     stateVersion.CreatedBy,
            "created_at":     stateVersion.CreatedAt,
            "resource_count": h.countResources(stateVersion.Content),
            "output_count":   h.countOutputs(stateVersion.Content),
        },
    })
}

// RetrieveStateVersion 获取指定版本的 State 完整内容
// GET /api/workspaces/:id/state-versions/:version/retrieve
// 需要 WORKSPACE_STATE_SENSITIVE 权限
func (h *StateHandler) RetrieveStateVersion(c *gin.Context) {
    workspaceID := c.Param("id")
    versionStr := c.Param("version")
    userID := c.GetString("user_id")

    version, err := strconv.Atoi(versionStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version number"})
        return
    }

    // 获取完整 state（含 content）
    stateVersion, err := h.stateService.GetStateVersion(workspaceID, version)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "State version not found"})
        return
    }

    // 记录审计日志
    h.logStateAccess(workspaceID, userID, version)

    // 返回完整内容
    c.JSON(http.StatusOK, gin.H{
        "data": map[string]interface{}{
            "version": stateVersion.Version,
            "content": stateVersion.Content,
        },
        "audit": map[string]interface{}{
            "accessed_at": time.Now(),
            "accessed_by": userID,
        },
    })
}
```

#### 5.2.2 修改路由配置

修改文件：`backend/internal/router/router_workspace.go`

```go
// State 版本详情（元数据，不含 content）- 需要基本的 state 读取权限
workspaces.GET("/:id/state-versions/:version", func(c *gin.Context) {
    role, _ := c.Get("role")
    if role == "admin" {
        stateHandler.GetStateVersion(c)
        return
    }
    
    iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
        {ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
        {ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
    })(c)
    
    if !c.IsAborted() {
        stateHandler.GetStateVersion(c)
    }
})

// State 内容获取（新增）- 需要 WORKSPACE_STATE_SENSITIVE 权限
workspaces.GET("/:id/state-versions/:version/retrieve", func(c *gin.Context) {
    role, _ := c.Get("role")
    if role == "admin" {
        stateHandler.RetrieveStateVersion(c)
        return
    }
    
    // 需要 WORKSPACE_STATE_SENSITIVE 权限
    iamMiddleware.RequirePermission(middleware.PermissionRequirement{
        ResourceType:  "WORKSPACE_STATE_SENSITIVE",
        ScopeType:     "WORKSPACE",
        RequiredLevel: "READ",
    })(c)
    
    if !c.IsAborted() {
        stateHandler.RetrieveStateVersion(c)
    }
})
```

### 5.3 前端实现

#### 5.3.1 修改 State 服务

修改文件：`frontend/src/services/state.ts`

```typescript
// 获取 state 版本元数据（不含 content）
export const getStateVersionMetadata = async (workspaceId: string, version: number) => {
  const response = await api.get(`/workspaces/${workspaceId}/state-versions/${version}`);
  return response.data;
};

// 获取 state 完整内容（需要 WORKSPACE_STATE_SENSITIVE 权限）
export const retrieveStateContent = async (workspaceId: string, version: number) => {
  const response = await api.get(`/workspaces/${workspaceId}/state-versions/${version}/retrieve`);
  return response.data;
};
```

#### 5.3.2 修改 StatePreview 组件

**设计原则**：页面整体布局保持不变，只是在 state 内容区域居中显示 "Retrieve State" 按钮，用户点击后才调用接口获取数据。

修改文件：`frontend/src/pages/StatePreview.tsx`

```tsx
// 状态定义
const [stateContent, setStateContent] = useState<any>(null);
const [retrieveStatus, setRetrieveStatus] = useState<'idle' | 'loading' | 'success' | 'error' | 'no_permission'>('idle');
const [errorMessage, setErrorMessage] = useState<string>('');

// 页面加载时只获取元数据，不自动获取 state 内容
useEffect(() => {
  fetchStateMetadata();
}, [workspaceId, version]);

// 用户点击 "Retrieve State" 按钮后才获取内容
const handleRetrieveState = async () => {
  setRetrieveStatus('loading');
  try {
    const result = await retrieveStateContent(workspaceId, version);
    setStateContent(result.data.content);
    setRetrieveStatus('success');
  } catch (err: any) {
    if (err.response?.status === 403) {
      setRetrieveStatus('no_permission');
      setErrorMessage('您没有 WORKSPACE_STATE_SENSITIVE 权限，无法查看 State 文件内容。');
    } else {
      setRetrieveStatus('error');
      setErrorMessage(extractErrorMessage(err));
    }
  }
};

// UI 渲染 - 页面布局保持不变，只修改内容区域
return (
  <div className={styles.container}>
    {/* 页面头部保持不变 */}
    <div className={styles.header}>
      <h1>State Version #{version}</h1>
      {/* 元数据信息 */}
      <div className={styles.metadata}>
        <span>Size: {formatBytes(metadata?.size_bytes)}</span>
        <span>Resources: {metadata?.resource_count}</span>
        <span>Outputs: {metadata?.output_count}</span>
      </div>
    </div>

    {/* State 内容区域 */}
    <div className={styles.content}>
      {/* 初始状态：显示居中的 Retrieve 按钮 */}
      {retrieveStatus === 'idle' && (
        <div className={styles.retrievePrompt}>
          <FileSearchOutlined style={{ fontSize: 48, color: '#1890ff', marginBottom: 16 }} />
          <p>State 内容包含敏感数据，需要显式请求查看</p>
          <Button 
            type="primary" 
            size="large"
            icon={<EyeOutlined />}
            onClick={handleRetrieveState}
          >
            Retrieve State
          </Button>
        </div>
      )}

      {/* 加载中 */}
      {retrieveStatus === 'loading' && (
        <div className={styles.retrievePrompt}>
          <Spin size="large" />
          <p style={{ marginTop: 16 }}>正在获取 State 内容...</p>
        </div>
      )}

      {/* 无权限 */}
      {retrieveStatus === 'no_permission' && (
        <div className={styles.retrievePrompt}>
          <LockOutlined style={{ fontSize: 48, color: '#faad14', marginBottom: 16 }} />
          <Alert
            type="warning"
            message="无权限查看 State 内容"
            description={errorMessage}
            style={{ maxWidth: 400 }}
          />
        </div>
      )}

      {/* 错误 */}
      {retrieveStatus === 'error' && (
        <div className={styles.retrievePrompt}>
          <Alert
            type="error"
            message="获取 State 失败"
            description={errorMessage}
            action={<Button onClick={handleRetrieveState}>重试</Button>}
            style={{ maxWidth: 400 }}
          />
        </div>
      )}

      {/* 成功获取：显示 State 内容 */}
      {retrieveStatus === 'success' && stateContent && (
        <JsonViewer data={stateContent} />
      )}
    </div>
  </div>
);
```

**样式定义**（`StatePreview.module.css`）：

```css
/* 居中提示区域 */
.retrievePrompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 400px;
  text-align: center;
  color: #666;
}

.retrievePrompt p {
  margin-bottom: 16px;
  font-size: 14px;
}
```

#### 5.3.3 修改 State 下载功能

State 下载功能也需要使用 `/retrieve` 接口，因为下载需要获取完整的 state 内容。

修改文件：`frontend/src/services/state.ts`

```typescript
// 下载 state 文件（需要 WORKSPACE_STATE_SENSITIVE 权限）
export const downloadStateFile = async (workspaceId: string, version: number) => {
  // 使用 retrieve 接口获取完整内容
  const response = await api.get(`/workspaces/${workspaceId}/state-versions/${version}/retrieve`);
  const content = response.data.data.content;
  
  // 创建下载
  const blob = new Blob([JSON.stringify(content, null, 2)], { type: 'application/json' });
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${workspaceId}-v${version}.tfstate`;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  window.URL.revokeObjectURL(url);
};
```

修改文件：`frontend/src/pages/StatePreview.tsx`（下载按钮）

```tsx
// 下载按钮只在有权限时显示
{hasPermission === true && (
  <Button 
    icon={<DownloadOutlined />}
    onClick={() => downloadStateFile(workspaceId, version)}
  >
    下载 State
  </Button>
)}

// 无权限时显示禁用的下载按钮
{hasPermission === false && (
  <Tooltip title="需要 WORKSPACE_STATE_SENSITIVE 权限">
    <Button 
      icon={<DownloadOutlined />}
      disabled
    >
      下载 State
    </Button>
  </Tooltip>
)}
```

## 6. 实施步骤

### 6.1 第一阶段：数据库准备

1. 执行 `scripts/add_state_sensitive_permission.sql` 添加新权限定义
2. 验证权限定义已正确插入

### 6.2 第二阶段：后端实现

1. 修改 `backend/internal/handlers/state_handler.go`
   - 修改 `GetStateVersion` 不返回 content
   - 新增 `RetrieveStateVersion` 方法
2. 修改 `backend/internal/router/router_workspace.go`
   - 添加新路由 `/state-versions/:version/retrieve`
3. 添加单元测试

### 6.3 第三阶段：前端实现

1. 修改 `frontend/src/services/state.ts`
2. 修改 `frontend/src/pages/StatePreview.tsx`
3. 添加权限不足时的 UI 提示

### 6.4 第四阶段：测试验证

1. 测试 Local 模式 Execute 流程正常
2. 测试 Agent 模式 Execute 流程正常
3. 测试 K8s Agent 模式 Execute 流程正常
4. 测试无权限用户无法查看 state 内容
5. 测试有权限用户可以查看 state 内容
6. 测试 Admin 用户可以查看 state 内容
7. 验证审计日志记录

## 7. 回滚方案

如果需要回滚此功能：

1. 恢复 `GetStateVersion` 返回 content
2. 移除 `/retrieve` 路由
3. 前端恢复原有逻辑
4. 权限定义可以保留，不影响系统运行

## 8. 安全考虑

1. **最小权限原则**：默认不返回 state 内容，需要显式授权
2. **审计追踪**：所有 state 内容访问都记录审计日志
3. **权限分离**：查看元数据和查看内容使用不同权限
4. **向后兼容**：现有的 Execute 流程不受影响
