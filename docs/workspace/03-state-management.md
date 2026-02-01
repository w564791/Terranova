# Workspace模块 - State管理

> **文档版本**: v1.0  
> **创建日期**: 2025-10-09  
> **状态**: 完整设计

## 📘 概述

State管理是Workspace模块的核心功能之一，负责Terraform状态文件的存储、版本控制、锁定和回滚。

## 🎯 核心功能

### 1. State存储

**存储位置**:
- PostgreSQL: 元数据（版本号、校验和、大小）
- S3/本地文件: 实际State文件内容

**实现状态**:  已完成

**核心组件**:
- `WorkspaceStateVersion`: State版本模型
- `StateVersionController`: State版本控制器

### 2. 版本控制

**版本策略**:
- 每次Apply成功后创建新版本
- 版本号自动递增
- 保留完整历史记录

**版本信息**:
```go
type WorkspaceStateVersion struct {
    ID          uint      `json:"id"`
    WorkspaceID uint      `json:"workspace_id"`
    Version     int       `json:"version"`
    Content     JSONB     `json:"content"`
    Checksum    string    `json:"checksum"`
    SizeBytes   int       `json:"size_bytes"`
    TaskID      *uint     `json:"task_id"`
    CreatedBy   *uint     `json:"created_by"`
    CreatedAt   time.Time `json:"created_at"`
}
```

### 3. State锁定

**锁定机制**:
- 执行Plan/Apply时自动锁定
- 防止并发修改
- 锁定超时自动释放

**锁定字段**:
```go
type Workspace struct {
    IsLocked   bool       `json:"is_locked"`
    LockedBy   *uint      `json:"locked_by"`
    LockedAt   *time.Time `json:"locked_at"`
    LockReason string     `json:"lock_reason"`
}
```

### 4. 版本回滚

**回滚策略**:
- 支持回滚到任意历史版本
- 回滚前创建当前版本快照
- 回滚后需要重新Apply

**实现状态**:  已完成

## 🔄 State生命周期

```
1. 初始化 → 2. 首次Apply → 3. 创建Version 1
    ↓
4. 后续Apply → 5. 创建新版本 → 6. 版本号递增
    ↓
7. 需要回滚 → 8. 选择历史版本 → 9. 恢复State
    ↓
10. 重新Apply → 11. 创建新版本 → 12. 继续迭代
```

## 📊 API接口

### 1. 获取State版本列表

```http
GET /api/workspaces/:id/state-versions
```

**响应**:
```json
{
  "versions": [
    {
      "id": 1,
      "version": 3,
      "checksum": "sha256:abc123...",
      "size_bytes": 1024,
      "created_at": "2025-10-09T10:00:00Z"
    }
  ],
  "total": 10
}
```

### 2. 获取特定版本

```http
GET /api/workspaces/:id/state-versions/:version_id
```

### 3. 下载State文件

```http
GET /api/workspaces/:id/state-versions/:version_id/download
```

### 4. 回滚到指定版本

```http
POST /api/workspaces/:id/state-versions/:version_id/rollback
```

### 5. 对比两个版本

```http
GET /api/workspaces/:id/state-versions/compare?from=1&to=2
```

## 🔒 锁定机制

### 自动锁定

**触发条件**:
- 执行Plan任务
- 执行Apply任务
- 手动锁定

**锁定流程**:
```go
func (s *WorkspaceService) LockWorkspace(id uint, reason string) error {
    return s.db.Model(&Workspace{}).
        Where("id = ? AND is_locked = false", id).
        Updates(map[string]interface{}{
            "is_locked":   true,
            "locked_at":   time.Now(),
            "lock_reason": reason,
        }).Error
}
```

### 自动解锁

**触发条件**:
- 任务完成（成功或失败）
- 锁定超时（默认30分钟）
- 手动解锁

## 📈 版本对比

### 差异检测

**对比内容**:
- 资源变更（新增/修改/删除）
- 输出变更
- 依赖关系变更

**差异格式**:
```json
{
  "resources": {
    "added": ["aws_instance.web"],
    "modified": ["aws_security_group.main"],
    "deleted": ["aws_s3_bucket.old"]
  },
  "outputs": {
    "added": ["instance_ip"],
    "modified": [],
    "deleted": ["old_output"]
  }
}
```

## 💾 存储策略

### PostgreSQL存储

**存储内容**:
- 版本元数据
- 小型State文件（< 1MB）

**优点**:
- 查询快速
- 事务支持
- 易于备份

### S3存储

**存储内容**:
- 大型State文件（> 1MB）
- 历史版本归档

**优点**:
- 成本低
- 容量大
- 高可用

## 🔧 配置示例

### State后端配置

```json
{
  "state_backend": "s3",
  "state_config": {
    "bucket": "iac-platform-states",
    "region": "us-east-1",
    "key_prefix": "workspaces/",
    "encryption": true
  }
}
```

### 本地存储配置

```json
{
  "state_backend": "local",
  "state_config": {
    "path": "/var/lib/iac-platform/states"
  }
}
```

## 📝 最佳实践

### 1. 版本管理
- 定期清理旧版本（保留最近30个）
- 重要版本打标签
- 定期备份到外部存储

### 2. 锁定管理
- 设置合理的锁定超时时间
- 任务失败后及时解锁
- 监控长时间锁定的Workspace

### 3. 安全性
- State文件加密存储
- 访问权限控制
- 敏感信息脱敏

### 4. 性能优化
- 大文件使用S3存储
- 启用压缩
- 使用CDN加速下载

## 🚀 未来扩展

1. **增量存储**: 只存储State差异
2. **智能清理**: 基于策略自动清理旧版本
3. **多区域复制**: State文件跨区域备份
4. **State分析**: AI分析State变化趋势

---

**相关文档**:
- [00-overview.md](./00-overview.md) - 总览和架构
- [04-task-workflow.md](./04-task-workflow.md) - 任务工作流
- [08-database-design.md](./08-database-design.md) - 数据库设计
