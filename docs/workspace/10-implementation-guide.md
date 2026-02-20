# Workspace模块 - 实现指导

> **文档版本**: v1.0  
> **创建日期**: 2025-10-09  
> **状态**: 完整指导

## 📘 概述

本文档提供Workspace模块的实现指导，包括开发顺序、最佳实践、代码规范和常见问题解决方案。

## 🎯 开发顺序建议

### Phase 1: 基础功能（1-2周）

**优先级**: 高

1. **数据库设计** (1天)
   - 创建迁移脚本
   - 执行数据库迁移
   - 验证表结构

2. **模型层** (1天)
   - Workspace模型
   - WorkspaceTask模型
   - WorkspaceStateVersion模型

3. **基础服务层** (2-3天)
   - WorkspaceService
   - WorkspaceLifecycleService
   - 状态转换逻辑

4. **基础API** (2-3天)
   - WorkspaceController
   - CRUD接口
   - 锁定/解锁接口

5. **前端基础页面** (2-3天)
   - Workspace列表
   - 创建/编辑页面
   - 详情页面

### Phase 2: 核心功能（2-3周）

**优先级**: 高

1. **Local执行模式** (3-4天)
   - TerraformExecutor
   - LocalExecutorService
   - TaskWorker

2. **任务管理** (2-3天)
   - WorkspaceTaskController
   - Plan/Apply API
   - 任务状态管理

3. **State版本控制** (2-3天)
   - StateVersionController
   - 版本CRUD
   - 回滚功能

4. **前端任务管理** (2-3天)
   - 任务列表
   - 任务详情
   - 状态徽章

### Phase 3: Agent/K8s模式（3-4周）

**优先级**: 中

1. **Agent服务层** (2-3天)
   - AgentService  已完成
   - AgentPoolService  已完成
   - TaskLockService  已完成

2. **Agent控制器** (2-3天)
   - AgentController
   - AgentPoolController
   - API实现

3. **K8s配置** (2-3天)
   - K8sConfigService
   - K8sConfigController
   - 配置管理

4. **执行器实现** (3-4天)
   - AgentExecutorService
   - K8sExecutorService
   - 任务分发

### Phase 4: 扩展功能（2-3周）

**优先级**: 低

1. **通知系统** (2-3天)
   - NotificationService
   - Webhook配置
   - 事件触发

2. **日志系统** (2-3天)
   - LogService
   - 日志查询
   - WebSocket流

3. **Drift检测** (3-4天)
   - DriftDetectionService
   - AI分析集成
   - 报告生成

## 💻 代码规范

### Go代码规范

**命名规范**:
```go
// 包名：小写，简短
package services

// 类型名：大驼峰
type WorkspaceService struct {}

// 方法名：大驼峰（公开），小驼峰（私有）
func (s *WorkspaceService) CreateWorkspace() {}
func (s *WorkspaceService) validateWorkspace() {}

// 变量名：小驼峰
var workspaceID uint
```

**错误处理**:
```go
// 返回错误
func (s *WorkspaceService) GetWorkspace(id uint) (*Workspace, error) {
    var workspace Workspace
    if err := s.db.First(&workspace, id).Error; err != nil {
        return nil, fmt.Errorf("failed to get workspace: %w", err)
    }
    return &workspace, nil
}

// 使用自定义错误
var ErrWorkspaceNotFound = errors.New("workspace not found")
```

**日志记录**:
```go
import "log"

log.Printf("Creating workspace: %s", name)
log.Printf("Error: %v", err)
```

### TypeScript代码规范

**命名规范**:
```typescript
// 接口：大驼峰，I前缀
interface IWorkspace {
  id: number;
  name: string;
}

// 类型：大驼峰
type WorkspaceState = 'created' | 'planning' | 'completed';

// 函数：小驼峰
function createWorkspace() {}

// 常量：大写下划线
const API_BASE_URL = 'http://localhost:8080';
```

**异步处理**:
```typescript
// 使用async/await
async function fetchWorkspaces(): Promise<Workspace[]> {
  try {
    const response = await api.get('/workspaces');
    return response.data;
  } catch (error) {
    console.error('Failed to fetch workspaces:', error);
    throw error;
  }
}
```

## 🔧 最佳实践

### 1. 数据库操作

**使用事务**:
```go
func (s *WorkspaceService) CreateWorkspaceWithTasks(workspace *Workspace) error {
    tx := s.db.Begin()
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
        }
    }()
    
    if err := tx.Create(workspace).Error; err != nil {
        tx.Rollback()
        return err
    }
    
    // 创建初始任务
    task := &WorkspaceTask{WorkspaceID: workspace.ID}
    if err := tx.Create(task).Error; err != nil {
        tx.Rollback()
        return err
    }
    
    return tx.Commit().Error
}
```

**避免N+1查询**:
```go
// 不好的做法
workspaces, _ := db.Find(&[]Workspace{})
for _, ws := range workspaces {
    db.Where("workspace_id = ?", ws.ID).Find(&tasks)
}

// 好的做法
db.Preload("Tasks").Find(&workspaces)
```

### 2. API设计

**RESTful规范**:
```
GET    /api/v1/workspaces       # 列表
POST   /api/v1/workspaces       # 创建
GET    /api/v1/workspaces/:id   # 详情
PUT    /api/v1/workspaces/:id   # 更新
DELETE /api/v1/workspaces/:id   # 删除
```

**统一响应格式**:
```go
type Response struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data,omitempty"`
    Error   *Error      `json:"error,omitempty"`
}

func SuccessResponse(c *gin.Context, data interface{}) {
    c.JSON(200, Response{Success: true, Data: data})
}

func ErrorResponse(c *gin.Context, code int, err error) {
    c.JSON(code, Response{
        Success: false,
        Error: &Error{Message: err.Error()},
    })
}
```

### 3. 错误处理

**分层错误处理**:
```go
// Service层：返回业务错误
func (s *WorkspaceService) GetWorkspace(id uint) (*Workspace, error) {
    var workspace Workspace
    if err := s.db.First(&workspace, id).Error; err != nil {
        if err == gorm.ErrRecordNotFound {
            return nil, ErrWorkspaceNotFound
        }
        return nil, err
    }
    return &workspace, nil
}

// Controller层：转换为HTTP错误
func (c *WorkspaceController) GetWorkspace(ctx *gin.Context) {
    workspace, err := c.service.GetWorkspace(id)
    if err != nil {
        if err == ErrWorkspaceNotFound {
            ErrorResponse(ctx, 404, err)
            return
        }
        ErrorResponse(ctx, 500, err)
        return
    }
    SuccessResponse(ctx, workspace)
}
```

### 4. 并发控制

**使用互斥锁**:
```go
type WorkspaceService struct {
    db    *gorm.DB
    mutex sync.RWMutex
}

func (s *WorkspaceService) LockWorkspace(id uint) error {
    s.mutex.Lock()
    defer s.mutex.Unlock()
    
    // 执行锁定操作
    return s.db.Model(&Workspace{}).
        Where("id = ?", id).
        Update("is_locked", true).Error
}
```

### 5. 测试

**单元测试**:
```go
func TestCreateWorkspace(t *testing.T) {
    // 准备
    db := setupTestDB()
    service := NewWorkspaceService(db)
    
    // 执行
    workspace := &Workspace{Name: "test"}
    err := service.CreateWorkspace(workspace)
    
    // 断言
    assert.NoError(t, err)
    assert.NotZero(t, workspace.ID)
}
```

## 🐛 常见问题

### 1. State锁定冲突

**问题**: 多个Apply任务同时执行导致State冲突

**解决方案**:
- 使用Workspace锁定机制
- Apply前检查锁定状态
- 使用数据库事务

### 2. 任务队列堆积

**问题**: TaskWorker处理速度慢，任务堆积

**解决方案**:
- 增加Worker数量
- 使用Agent模式分布式执行
- 优化Terraform执行

### 3. State文件过大

**问题**: State文件超过数据库字段限制

**解决方案**:
- 使用S3存储大文件
- 数据库只存储元数据
- 实现分页加载

## 📚 参考资源

### 官方文档
- [Terraform文档](https://www.terraform.io/docs)
- [HCP Terraform](https://www.terraform.io/cloud-docs)
- [Gin框架](https://gin-gonic.com/docs/)
- [GORM](https://gorm.io/docs/)

### 代码示例
- `backend/services/` - 服务层实现
- `backend/controllers/` - 控制器实现
- `frontend/src/pages/` - 前端页面

---

**相关文档**:
- [00-overview.md](./00-overview.md) - 总览和架构
- [08-database-design.md](./08-database-design.md) - 数据库设计
- [09-api-specification.md](./09-api-specification.md) - API规范
