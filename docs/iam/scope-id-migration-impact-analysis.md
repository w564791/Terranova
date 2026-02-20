# Scope ID 迁移影响范围分析报告

## 执行摘要

在完成 workspace 从自增 ID 迁移到语义化 ID (workspace_id) 后，现在需要评估 `scope_id` 字段的迁移影响。`scope_id` 是 IAM 权限系统中的核心字段，用于标识权限作用域（Organization/Project/Workspace）的具体 ID。

**关键发现**：
- `scope_id` 当前为 `INTEGER` 类型
- 影响 5 个数据库表
- 涉及 54 处后端代码引用
- 涉及 43 处前端代码引用
- **高风险**：与 workspace_id 迁移类似，需要大规模代码改造

---

## 1. 数据库层面影响

### 1.1 受影响的数据库表

根据数据库查询结果，以下表包含 `scope_id` 字段：

| 表名 | 字段类型 | 用途 | 关联关系 |
|------|---------|------|---------|
| `iam_user_roles` | INTEGER | 用户角色分配的作用域ID | 核心表 |
| `iam_team_roles` | INTEGER | 团队角色分配的作用域ID | 核心表 |
| `permission_audit_logs` | INTEGER | 权限审计日志的作用域ID | 审计表 |
| `permission_audit_logs_backup` | INTEGER | 权限审计日志备份 | 备份表 |
| `v_user_effective_roles` | INTEGER | 用户有效角色视图 | 视图 |

### 1.2 Scope ID 的语义

`scope_id` 的实际含义取决于 `scope_type` 字段：

```sql
-- scope_type 可能的值
- 'ORGANIZATION' -> scope_id 指向 organizations.id (INTEGER)
- 'PROJECT'      -> scope_id 指向 projects.id (INTEGER)
- 'WORKSPACE'    -> scope_id 指向 workspaces.id (现在是 VARCHAR(50))
```

**关键问题**：当 `scope_type = 'WORKSPACE'` 时，`scope_id` 需要存储语义化 ID（如 `ws-abc123`），但当前字段类型为 `INTEGER`。

---

## 2. 后端代码影响

### 2.1 实体定义（Entity）

#### 2.1.1 UserRole (backend/internal/domain/entity/user_role.go)
```go
type UserRole struct {
    ScopeType  string     `gorm:"type:varchar(20)" json:"scope_type"`
    ScopeID    uint       `gorm:"not null" json:"scope_id"`  //  需要改为 string
    // ...
}
```

#### 2.1.2 Permission (backend/internal/domain/entity/permission.go)
```go
type PermissionGrant struct {
    ScopeType       valueobject.ScopeType       `json:"scope_type"`
    ScopeID         uint                        `json:"scope_id"`  //  需要改为 string
    // ...
}

type OrgPermission struct {
    OrgID           uint                        `json:"org_id"`  // ✓ 保持 uint
    // ...
}

type ProjectPermission struct {
    ProjectID       uint                        `json:"project_id"`  // ✓ 保持 uint
    // ...
}

type WorkspacePermission struct {
    WorkspaceID     uint                        `json:"workspace_id"`  //  需要改为 string
    // ...
}
```

### 2.2 服务层（Service）

#### 受影响的文件：
1. **permission_checker.go** - 权限检查服务
   - `CheckPermissionRequest` 结构体包含 `ScopeID uint` 和 `ScopeIDStr string`
   - 已经有部分支持语义化 ID 的代码

2. **permission_service.go** - 权限管理服务
   - 多个请求结构体使用 `scope_id uint`
   - 需要全面改造

3. **audit_service.go** - 审计服务
   - 审计日志查询使用 `scope_id`

### 2.3 处理器层（Handler）

#### 受影响的文件：
1. **permission_handler.go** - 权限处理器
   - API 路由：`/api/v1/permissions/{scope_type}/{scope_id}`
   - 多个请求结构体使用 `scope_id uint`

2. **role_handler.go** - 角色处理器
   - 角色分配时需要 `scope_id`

3. **audit_handler.go** - 审计处理器
   - 审计日志查询 API

### 2.4 中间件（Middleware）

**iam_permission.go** - IAM 权限中间件
- 从路径参数或查询参数获取 `scope_id`
- 需要支持语义化 ID 解析

### 2.5 仓储层（Repository）

**permission_repository_impl.go**
- 查询用户/团队角色时使用 `scope_id`
- SQL 查询需要适配不同类型的 ID

---

## 3. 前端代码影响

### 3.1 类型定义（frontend/src/services/iam.ts）

```typescript
// 当前定义
interface PermissionGrant {
  scope_id: number;  //  需要改为 string | number
  scope_type: ScopeType;
  // ...
}

interface CheckPermissionRequest {
  scope_id: number;  //  需要改为 string | number
  scope_type: ScopeType;
  required_level: PermissionLevel;
}
```

### 3.2 受影响的页面组件

1. **TaskDetail.tsx** - 任务详情页
   - 权限检查时传递 `scope_id: workspaceId`

2. **TeamDetail.tsx** - 团队详情页
   - 显示团队权限时使用 `scope_id`
   - 需要根据 `scope_type` 获取对应的名称

3. **GrantPermission.tsx** - 授权页面
   - 表单中包含 `scope_id` 字段
   - 需要支持不同类型的 ID 输入

4. **PermissionManagement.tsx** - 权限管理页
   - 列表显示 `scope_id`

5. **Layout.tsx** - 布局组件
   - 权限检查函数使用 `scope_id`

---

## 4. 迁移方案

### 4.1 方案 A：统一使用字符串类型（推荐）

**优点**：
- 彻底解决类型不一致问题
- 支持所有类型的语义化 ID
- 代码逻辑统一

**缺点**：
- 改动范围大
- 需要数据迁移
- Organization 和 Project 也需要迁移到语义化 ID

**实施步骤**：
1. 修改数据库表结构：`scope_id INTEGER` → `scope_id VARCHAR(50)`
2. 数据迁移：
   - Organization: `1` → `org-xxx`
   - Project: `1` → `proj-xxx`
   - Workspace: `1` → `ws-xxx`
3. 修改所有实体定义：`uint` → `string`
4. 修改所有服务、处理器、仓储代码
5. 修改前端类型定义和 API 调用

### 4.2 方案 B：混合类型支持（过渡方案）

**优点**：
- 可以逐步迁移
- 向后兼容
- 风险较低

**缺点**：
- 代码复杂度增加
- 需要维护两套逻辑
- 长期技术债务

**实施步骤**：
1. 添加新字段 `scope_id_str VARCHAR(50)`
2. 保留旧字段 `scope_id INTEGER`
3. 代码同时支持两种类型
4. 逐步迁移数据
5. 最终删除旧字段

### 4.3 方案 C：仅迁移 Workspace 相关（最小改动）

**优点**：
- 改动范围最小
- 只影响 Workspace 相关功能
- 实施快速

**缺点**：
- 类型不统一
- 需要特殊处理 Workspace 场景
- 代码逻辑复杂

**实施步骤**：
1. 修改 `WorkspacePermission` 表的 `workspace_id` 字段
2. 在代码中特殊处理 `scope_type = 'WORKSPACE'` 的情况
3. 其他 scope_type 保持 INTEGER

---

## 5. 详细影响清单

### 5.1 数据库迁移脚本

```sql
-- 方案 A：统一字符串类型
ALTER TABLE iam_user_roles ALTER COLUMN scope_id TYPE VARCHAR(50);
ALTER TABLE iam_team_roles ALTER COLUMN scope_id TYPE VARCHAR(50);
ALTER TABLE permission_audit_logs ALTER COLUMN scope_id TYPE VARCHAR(50);

-- 数据迁移（需要根据 scope_type 分别处理）
UPDATE iam_user_roles 
SET scope_id = CASE 
    WHEN scope_type = 'ORGANIZATION' THEN 'org-' || lpad(scope_id::text, 16, '0')
    WHEN scope_type = 'PROJECT' THEN 'proj-' || lpad(scope_id::text, 16, '0')
    WHEN scope_type = 'WORKSPACE' THEN (SELECT id FROM workspaces WHERE old_id = scope_id::integer)
END;
```

### 5.2 后端代码修改清单

| 文件 | 修改内容 | 优先级 |
|------|---------|--------|
| entity/user_role.go | ScopeID uint → string | 高 |
| entity/permission.go | ScopeID uint → string | 高 |
| service/permission_checker.go | 统一使用 ScopeIDStr | 高 |
| service/permission_service.go | 所有请求结构体 | 高 |
| handler/permission_handler.go | API 参数解析 | 高 |
| handler/role_handler.go | 角色分配逻辑 | 高 |
| middleware/iam_permission.go | scope_id 解析 | 高 |
| repository/permission_repository_impl.go | SQL 查询 | 高 |

### 5.3 前端代码修改清单

| 文件 | 修改内容 | 优先级 |
|------|---------|--------|
| services/iam.ts | 类型定义 | 高 |
| pages/TaskDetail.tsx | 权限检查 | 高 |
| pages/admin/TeamDetail.tsx | 权限显示 | 中 |
| pages/admin/GrantPermission.tsx | 表单处理 | 高 |
| pages/admin/PermissionManagement.tsx | 列表显示 | 中 |
| components/Layout.tsx | 权限检查 | 中 |

---

## 6. 风险评估

### 6.1 高风险点

1. **数据一致性**
   - 迁移过程中可能出现数据不一致
   - 外键约束可能失效

2. **类型转换错误**
   - 代码中大量使用 `uint` 类型断言
   - 可能出现运行时错误

3. **API 兼容性**
   - 前端调用 API 时传递的参数类型变化
   - 可能导致 API 调用失败

4. **权限检查失效**
   - 如果迁移不完整，可能导致权限检查失败
   - 安全风险

### 6.2 缓解措施

1. **完整备份**
   - 迁移前完整备份数据库
   - 准备回滚方案

2. **分阶段测试**
   - 在测试环境完整验证
   - 逐步灰度发布

3. **监控告警**
   - 添加详细日志
   - 监控错误率

4. **回滚方案**
   - 准备数据回滚脚本
   - 准备代码回滚方案

---

## 7. 工作量评估

### 7.1 数据库迁移
- 编写迁移脚本：2 天
- 测试验证：2 天
- **小计**：4 天

### 7.2 后端开发
- 实体层修改：1 天
- 服务层修改：3 天
- 处理器层修改：2 天
- 仓储层修改：2 天
- 中间件修改：1 天
- 单元测试：2 天
- **小计**：11 天

### 7.3 前端开发
- 类型定义：0.5 天
- API 服务层：1 天
- 页面组件：3 天
- 测试：1.5 天
- **小计**：6 天

### 7.4 测试与验证
- 集成测试：3 天
- 回归测试：3 天
- 性能测试：1 天
- **小计**：7 天

### 7.5 总计
- **开发时间**：21 天
- **测试时间**：7 天
- **总计**：28 天（约 5-6 周）

---

## 8. 建议

### 8.1 短期建议（当前阶段）

**不建议立即进行 scope_id 迁移**，理由如下：

1. **Workspace ID 迁移刚完成**
   - 系统需要稳定运行一段时间
   - 需要观察是否有遗留问题

2. **改动范围过大**
   - 涉及 IAM 核心功能
   - 风险高于收益

3. **Organization 和 Project 尚未迁移**
   - 如果只迁移 Workspace，会导致类型不一致
   - 需要整体规划

### 8.2 中期建议（3-6 个月后）

**如果确实需要迁移，建议采用方案 B（混合类型支持）**：

1. **第一阶段**：添加 `scope_id_str` 字段
   - 保持向后兼容
   - 逐步迁移数据

2. **第二阶段**：代码同时支持两种类型
   - 新功能使用字符串类型
   - 旧功能保持兼容

3. **第三阶段**：完全迁移
   - 所有数据迁移完成
   - 删除旧字段

### 8.3 长期建议（1 年后）

**考虑整体 ID 规范统一**：

1. **统一所有实体 ID 为语义化 ID**
   - Organization: `org-xxx`
   - Project: `proj-xxx`
   - Workspace: `ws-xxx`
   - User: `usr-xxx`
   - Team: `team-xxx`

2. **制定 ID 生成规范**
   - 统一前缀规则
   - 统一长度规则
   - 统一字符集

3. **建立迁移框架**
   - 可复用的迁移工具
   - 标准化的迁移流程

---

## 9. 结论

### 9.1 当前状态

- `scope_id` 字段当前为 `INTEGER` 类型
- 影响 5 个数据库表、54 处后端代码、43 处前端代码
- 与 workspace_id 迁移存在类型不一致问题

### 9.2 核心问题

当 `scope_type = 'WORKSPACE'` 时，`scope_id` 需要存储语义化 ID（VARCHAR），但当前字段类型为 INTEGER，存在类型冲突。

### 9.3 最终建议

**阶段性处理方案**：

1. **当前（0-3 个月）**：
   - 不进行 scope_id 迁移
   - 在代码层面做类型转换处理
   - 监控 Workspace ID 迁移的稳定性

2. **中期（3-6 个月）**：
   - 评估是否需要迁移
   - 如需迁移，采用方案 B（混合类型）
   - 制定详细的迁移计划

3. **长期（6-12 个月）**：
   - 考虑整体 ID 规范统一
   - 制定全局迁移策略
   - 建立标准化流程

### 9.4 临时解决方案

在不修改数据库结构的情况下，可以在代码层面做以下处理：

```go
// 在权限检查时，根据 scope_type 处理不同类型的 ID
func (s *PermissionChecker) CheckPermission(ctx context.Context, req *CheckPermissionRequest) error {
    var scopeIDUint uint
    
    if req.ScopeType == valueobject.ScopeTypeWorkspace {
        // Workspace 使用语义化 ID，需要查询获取数字 ID
        workspace, err := s.workspaceRepo.GetByID(ctx, req.ScopeIDStr)
        if err != nil {
            return err
        }
        scopeIDUint = workspace.OldID // 假设保留了旧的数字 ID
    } else {
        // Organization 和 Project 直接使用数字 ID
        scopeIDUint = req.ScopeID
    }
    
    // 使用 scopeIDUint 进行权限查询
    // ...
}
```

这种方案可以在不修改数据库的情况下，暂时解决类型不一致的问题，但长期来看仍需要进行彻底的迁移。

---

## 附录

### A. 相关文档
- [Workspace ID 迁移分析](./workspace-id-migration-analysis.md)
- [Workspace ID 实施计划](./workspace-id-implementation-plan.md)
- [ID 规范文档](../../11-id-specification.md)

### B. 数据库表结构

```sql
-- iam_user_roles 表结构
CREATE TABLE iam_user_roles (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(20) NOT NULL,
    role_id INTEGER NOT NULL,
    scope_type VARCHAR(20) NOT NULL,
    scope_id INTEGER NOT NULL,  --  问题字段
    assigned_by VARCHAR(20),
    assigned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    reason TEXT,
    UNIQUE(user_id, role_id, scope_type, scope_id)
);

-- iam_team_roles 表结构
CREATE TABLE iam_team_roles (
    id SERIAL PRIMARY KEY,
    team_id VARCHAR(20) NOT NULL,
    role_id INTEGER NOT NULL,
    scope_type VARCHAR(20) NOT NULL,
    scope_id INTEGER NOT NULL,  --  问题字段
    assigned_by VARCHAR(20),
    assigned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    reason TEXT,
    UNIQUE(team_id, role_id, scope_type, scope_id)
);
```

### C. 代码示例

详见各章节的代码片段。
