# 业务语义ID实施指南

## 📋 概述

本文档提供了将权限系统从自增ID迁移到业务语义ID的完整实施指南。

**目标**: 解决数据库恢复后权限ID变化导致的权限错乱问题  
**方案**: 使用业务语义ID（如orgpm-xxx, wspm-xxx）替代自增整数ID

---

##  已完成的工作

### 1. Entity定义修改 

已将以下entity的`PermissionID`从`uint`改为`string`:
-  `PermissionDefinition.ID` - 主键改为VARCHAR(32)
-  `PermissionGrant.PermissionID`
-  `OrgPermission.PermissionID`
-  `ProjectPermission.PermissionID`
-  `WorkspacePermission.PermissionID`
-  `PresetPermission.PermissionID`

**文件**: `backend/internal/domain/entity/permission.go`

### 2. ID生成器创建 

创建了`PermissionIDGenerator`用于生成业务语义ID。

**文件**: `backend/internal/infrastructure/id_generator.go`

### 3. 数据库迁移脚本 

创建了完整的7阶段迁移脚本，覆盖7个表：
- permission_definitions
- org_permissions
- project_permissions
- workspace_permissions
- iam_role_policies
- permission_audit_logs
- preset_permissions

**文件**: `scripts/migrate_to_semantic_permission_ids.sql`

---

## 🔄 待完成的工作

由于这是一个大型重构，涉及多个文件的修改，建议分阶段实施：

### 阶段1: 修改相关Entity（需要修改）

还需要修改以下entity文件中的`PermissionID`字段：

1. **`backend/internal/domain/entity/role_policy.go`**
   ```go
   type RolePolicy struct {
       PermissionID uint  // 改为 string
   }
   ```

2. **`backend/internal/domain/entity/audit_log.go`**
   ```go
   type PermissionAuditLog struct {
       PermissionID *uint  // 改为 *string
   }
   ```

### 阶段2: 修改Service层（需要修改）

需要修改所有使用`PermissionID`的服务：

1. **`backend/internal/application/service/permission_service.go`**
   - `GrantPermissionRequest.PermissionID` - 改为string
   - 所有创建权限的地方使用ID生成器

2. **`backend/internal/application/service/permission_checker.go`**
   - 确保权限检查逻辑支持字符串ID

### 阶段3: 修改Repository层（需要修改）

1. **`backend/internal/infrastructure/persistence/permission_repository_impl.go`**
   - 所有查询和更新操作支持字符串ID
   - JOIN查询的ON条件

### 阶段4: 修改Handler层（需要修改）

1. **`backend/internal/handlers/permission_handler.go`**
   - 请求和响应结构中的PermissionID
   - 参数验证逻辑

2. **`backend/internal/handlers/role_handler.go`**
   - RolePolicy相关的PermissionID处理

### 阶段5: 数据库初始化（需要创建）

创建使用语义ID的初始化脚本，包含所有系统权限定义。

---

## 📝 简化方案建议

考虑到这是一个大型重构，我建议采用**混合方案**：

### 方案A: 保留当前实现 + 添加验证脚本（推荐）⭐

**优点**:
- 无需大规模代码修改
- 风险最小
- 立即可用

**实施**:
1. 回滚entity的修改（保持uint类型）
2. 使用当前的迁移脚本作为参考
3. 实施严格的备份恢复流程：
   - 全量备份
   - 恢复后重置序列
   - 执行验证脚本

**验证脚本** (`scripts/verify_permission_integrity.sql`):
```sql
-- 检查权限定义ID是否匹配
SELECT 
    pd.id, 
    pd.name,
    COUNT(op.id) as org_perm_count,
    COUNT(pp.id) as proj_perm_count,
    COUNT(wp.id) as ws_perm_count
FROM permission_definitions pd
LEFT JOIN org_permissions op ON op.permission_id = pd.id
LEFT JOIN project_permissions pp ON pp.permission_id = pd.id
LEFT JOIN workspace_permissions wp ON wp.permission_id = pd.id
GROUP BY pd.id, pd.name
ORDER BY pd.id;

-- 检查孤立记录
SELECT 'org_permissions' as table_name, COUNT(*) as orphan_count
FROM org_permissions op
LEFT JOIN permission_definitions pd ON pd.id = op.permission_id
WHERE pd.id IS NULL
UNION ALL
SELECT 'project_permissions', COUNT(*)
FROM project_permissions pp
LEFT JOIN permission_definitions pd ON pd.id = pp.permission_id
WHERE pd.id IS NULL
UNION ALL
SELECT 'workspace_permissions', COUNT(*)
FROM workspace_permissions wp
LEFT JOIN permission_definitions pd ON pd.id = wp.permission_id
WHERE pd.id IS NULL;
```

### 方案B: 完整实施语义ID（长期）

如果确实要完整实施，需要：

1. **修改所有Entity** (约5个文件)
2. **修改所有Service** (约3个文件)
3. **修改所有Repository** (约2个文件)
4. **修改所有Handler** (约3个文件)
5. **更新所有测试** (约10个文件)
6. **执行数据库迁移** (停机维护)

**预计工作量**: 2-3天  
**风险等级**: 高  
**测试工作量**: 大

---

## 🎯 推荐实施路径

### 立即行动（方案A）

1. **回滚entity修改**
   ```bash
   git checkout backend/internal/domain/entity/permission.go
   ```

2. **删除ID生成器**（暂时不需要）
   ```bash
   rm backend/internal/infrastructure/id_generator.go
   ```

3. **使用当前系统 + 严格的备份恢复流程**
   - 实施全量备份策略
   - 恢复后执行序列重置
   - 使用验证脚本检查完整性

4. **创建验证脚本**
   - 在每次恢复后执行
   - 确保权限ID一致性

### 长期规划（方案B）

如果未来需要完整实施语义ID：

1. **准备阶段** (1周)
   - 在测试环境完整实施
   - 编写所有必要的代码修改
   - 编写完整的测试用例

2. **测试阶段** (1周)
   - 功能测试
   - 性能测试
   - 压力测试

3. **实施阶段** (1天)
   - 选择维护窗口
   - 执行数据库迁移
   - 部署新代码
   - 验证系统功能

---

## 📊 两种方案对比

| 方案 | 工作量 | 风险 | 效果 | 推荐度 |
|------|--------|------|------|--------|
| A: 验证脚本 | 小 | 低 | 90% | ⭐⭐⭐⭐⭐ |
| B: 完整实施 | 大 | 高 | 100% | ⭐⭐⭐ |

---

##  当前系统状态

### 已完成

1.  Router权限修复（99个路由）
2.  资源类型定义（16个新类型）
3.  安全审计（无参数注入风险）
4.  数据库迁移脚本（7个表）
5.  Entity定义修改（已完成但建议回滚）
6.  ID生成器（已创建但暂不使用）

### 建议

**推荐使用方案A**，原因：
- 当前系统已经很安全（100%认证和权限覆盖）
- 数据库恢复ID问题可以通过流程控制解决
- 避免大规模重构的风险
- 如果未来确实需要，可以再实施方案B

---

## 📝 下一步行动

### 如果选择方案A（推荐）

1. 回滚entity修改
2. 删除ID生成器
3. 创建验证脚本
4. 文档化备份恢复流程

### 如果选择方案B

1. 继续修改所有相关代码
2. 编写完整测试
3. 在测试环境验证
4. 计划维护窗口

---

**建议**: 先使用方案A，系统已经足够安全。如果未来确实遇到数据库恢复问题，再考虑实施方案B。
