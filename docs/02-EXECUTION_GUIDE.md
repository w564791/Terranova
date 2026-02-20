# 数据库迁移执行指南

##  重要说明

MCP postgres工具是只读的，无法执行CREATE/ALTER等写操作。您需要手动执行SQL脚本。

---

## 🎯 执行方案选择

### 方案A: 新环境初始化（推荐）⭐

**适用场景**: 新部署、测试环境、可以重建数据库

**步骤**:

1. **停止应用**
```bash
# 停止后端服务
pkill -f "go run main.go" 或 systemctl stop iac-platform
```

2. **删除旧的权限表**（谨慎！）
```bash
psql -U postgres -d iac_platform << 'EOF'
DROP TABLE IF EXISTS permission_audit_logs CASCADE;
DROP TABLE IF EXISTS preset_permissions CASCADE;
DROP TABLE IF EXISTS iam_role_policies CASCADE;
DROP TABLE IF EXISTS workspace_permissions CASCADE;
DROP TABLE IF EXISTS project_permissions CASCADE;
DROP TABLE IF EXISTS org_permissions CASCADE;
DROP TABLE IF EXISTS permission_definitions CASCADE;
EOF
```

3. **启动应用**（GORM会自动创建新表结构）
```bash
cd backend
go run main.go
# 或
systemctl start iac-platform
```

4. **执行初始化脚本**
```bash
psql -U postgres -d iac_platform -f scripts/init_permissions_with_semantic_ids.sql
```

5. **验证**
```bash
psql -U postgres -d iac_platform -c "SELECT id, name, scope_level FROM permission_definitions ORDER BY id LIMIT 10;"
```

---

### 方案B: 迁移现有数据（保留数据）

**适用场景**: 生产环境、有重要数据需要保留

**步骤**:

1. **备份数据库**
```bash
pg_dump -U postgres iac_platform > backup_$(date +%Y%m%d_%H%M%S).sql
```

2. **执行迁移脚本（阶段1-5）**
```bash
psql -U postgres -d iac_platform -f scripts/migrate_to_semantic_permission_ids.sql
```

3. **验证数据完整性**
```bash
psql -U postgres -d iac_platform << 'EOF'
-- 检查未更新的记录
SELECT 'org_permissions未更新' as check_name, COUNT(*) as count
FROM org_permissions WHERE new_permission_id IS NULL
UNION ALL
SELECT 'workspace_permissions未更新', COUNT(*)
FROM workspace_permissions WHERE new_permission_id IS NULL;
EOF
```

4. **编辑迁移脚本，取消注释阶段6**
```bash
# 编辑 scripts/migrate_to_semantic_permission_ids.sql
