-- 迁移到业务语义ID体系
-- 目的: 解决数据库恢复后权限ID变化导致的权限错乱问题
-- 方案: 将permission_id从自增整数改为业务语义字符串（如wspm-xxx, orgpm-xxx）
-- 执行日期: 2025-10-24
-- 警告: 此脚本会修改表结构，建议在测试环境先执行

-- ============================================
-- 阶段1: 备份现有数据
-- ============================================

-- 创建备份表
CREATE TABLE IF NOT EXISTS permission_definitions_backup AS 
SELECT * FROM permission_definitions;

CREATE TABLE IF NOT EXISTS org_permissions_backup AS 
SELECT * FROM org_permissions;

CREATE TABLE IF NOT EXISTS project_permissions_backup AS 
SELECT * FROM project_permissions;

CREATE TABLE IF NOT EXISTS workspace_permissions_backup AS 
SELECT * FROM workspace_permissions;

CREATE TABLE IF NOT EXISTS iam_role_policies_backup AS 
SELECT * FROM iam_role_policies;

CREATE TABLE IF NOT EXISTS permission_audit_logs_backup AS 
SELECT * FROM permission_audit_logs;

CREATE TABLE IF NOT EXISTS preset_permissions_backup AS 
SELECT * FROM preset_permissions;

-- ============================================
-- 阶段2: 添加新的ID字段
-- ============================================

-- 为permission_definitions添加新的字符串ID字段
ALTER TABLE permission_definitions 
ADD COLUMN IF NOT EXISTS semantic_id VARCHAR(32);

-- 为权限授予表添加新的字符串ID字段
ALTER TABLE org_permissions 
ADD COLUMN IF NOT EXISTS new_permission_id VARCHAR(32);

ALTER TABLE project_permissions 
ADD COLUMN IF NOT EXISTS new_permission_id VARCHAR(32);

ALTER TABLE workspace_permissions 
ADD COLUMN IF NOT EXISTS new_permission_id VARCHAR(32);

ALTER TABLE iam_role_policies 
ADD COLUMN IF NOT EXISTS new_permission_id VARCHAR(32);

ALTER TABLE permission_audit_logs 
ADD COLUMN IF NOT EXISTS new_permission_id VARCHAR(32);

ALTER TABLE preset_permissions 
ADD COLUMN IF NOT EXISTS new_permission_id VARCHAR(32);

-- ============================================
-- 阶段3: 生成业务语义ID
-- ============================================

-- 为permission_definitions生成语义ID
-- 格式: {scope_prefix}pm-{padded_id}
-- 例如: orgpm-000000000001, wspm-000000000002
UPDATE permission_definitions 
SET semantic_id = CONCAT(
    CASE scope_level
        WHEN 'ORGANIZATION' THEN 'orgpm'
        WHEN 'PROJECT' THEN 'pjpm'
        WHEN 'WORKSPACE' THEN 'wspm'
        ELSE 'pm'
    END,
    '-',
    LPAD(id::TEXT, 12, '0')
)
WHERE semantic_id IS NULL;

-- ============================================
-- 阶段4: 更新外键引用
-- ============================================

-- 更新org_permissions的permission_id引用
UPDATE org_permissions op
SET new_permission_id = pd.semantic_id
FROM permission_definitions pd
WHERE op.permission_id = pd.id;

-- 更新project_permissions的permission_id引用
UPDATE project_permissions pp
SET new_permission_id = pd.semantic_id
FROM permission_definitions pd
WHERE pp.permission_id = pd.id;

-- 更新workspace_permissions的permission_id引用
UPDATE workspace_permissions wp
SET new_permission_id = pd.semantic_id
FROM permission_definitions pd
WHERE wp.permission_id = pd.id;

-- 更新iam_role_policies的permission_id引用
UPDATE iam_role_policies rp
SET new_permission_id = pd.semantic_id
FROM permission_definitions pd
WHERE rp.permission_id = pd.id;

-- 更新permission_audit_logs的permission_id引用（如果不为NULL）
UPDATE permission_audit_logs pal
SET new_permission_id = pd.semantic_id
FROM permission_definitions pd
WHERE pal.permission_id = pd.id
AND pal.permission_id IS NOT NULL;

-- 更新preset_permissions的permission_id引用
UPDATE preset_permissions pp
SET new_permission_id = pd.semantic_id
FROM permission_definitions pd
WHERE pp.permission_id = pd.id;

-- ============================================
-- 阶段5: 验证数据完整性
-- ============================================

-- 检查是否有未更新的记录
SELECT 'org_permissions未更新记录' as check_name, COUNT(*) as count
FROM org_permissions
WHERE new_permission_id IS NULL
UNION ALL
SELECT 'project_permissions未更新记录', COUNT(*)
FROM project_permissions
WHERE new_permission_id IS NULL
UNION ALL
SELECT 'workspace_permissions未更新记录', COUNT(*)
FROM workspace_permissions
WHERE new_permission_id IS NULL
UNION ALL
SELECT 'iam_role_policies未更新记录', COUNT(*)
FROM iam_role_policies
WHERE new_permission_id IS NULL
UNION ALL
SELECT 'permission_audit_logs未更新记录', COUNT(*)
FROM permission_audit_logs
WHERE permission_id IS NOT NULL AND new_permission_id IS NULL
UNION ALL
SELECT 'preset_permissions未更新记录', COUNT(*)
FROM preset_permissions
WHERE new_permission_id IS NULL;

-- 检查是否有孤立的权限授予记录
SELECT 'org_permissions孤立记录' as check_name, COUNT(*) as count
FROM org_permissions op
LEFT JOIN permission_definitions pd ON op.permission_id = pd.id
WHERE pd.id IS NULL
UNION ALL
SELECT 'project_permissions孤立记录', COUNT(*)
FROM project_permissions pp
LEFT JOIN permission_definitions pd ON pp.permission_id = pd.id
WHERE pd.id IS NULL
UNION ALL
SELECT 'workspace_permissions孤立记录', COUNT(*)
FROM workspace_permissions wp
LEFT JOIN permission_definitions pd ON wp.permission_id = pd.id
WHERE pd.id IS NULL;

-- ============================================
-- 阶段6: 切换到新ID（需要停机维护）
-- ============================================

-- 警告: 以下操作会修改主键和外键，需要停机维护
-- 建议在维护窗口期执行

-- 6.1 删除旧的外键约束
-- ALTER TABLE org_permissions DROP CONSTRAINT IF EXISTS org_permissions_permission_id_fkey;
-- ALTER TABLE project_permissions DROP CONSTRAINT IF EXISTS project_permissions_permission_id_fkey;
-- ALTER TABLE workspace_permissions DROP CONSTRAINT IF EXISTS workspace_permissions_permission_id_fkey;
-- ALTER TABLE iam_role_policies DROP CONSTRAINT IF EXISTS iam_role_policies_permission_id_fkey;
-- ALTER TABLE permission_audit_logs DROP CONSTRAINT IF EXISTS permission_audit_logs_permission_id_fkey;
-- ALTER TABLE preset_permissions DROP CONSTRAINT IF EXISTS preset_permissions_permission_id_fkey;

-- 6.2 删除permission_definitions的旧主键
-- ALTER TABLE permission_definitions DROP CONSTRAINT IF EXISTS permission_definitions_pkey;

-- 6.3 重命名字段
-- ALTER TABLE permission_definitions RENAME COLUMN id TO old_id;
-- ALTER TABLE permission_definitions RENAME COLUMN semantic_id TO id;

-- ALTER TABLE org_permissions RENAME COLUMN permission_id TO old_permission_id;
-- ALTER TABLE org_permissions RENAME COLUMN new_permission_id TO permission_id;

-- ALTER TABLE project_permissions RENAME COLUMN permission_id TO old_permission_id;
-- ALTER TABLE project_permissions RENAME COLUMN new_permission_id TO permission_id;

-- ALTER TABLE workspace_permissions RENAME COLUMN permission_id TO old_permission_id;
-- ALTER TABLE workspace_permissions RENAME COLUMN new_permission_id TO permission_id;

-- ALTER TABLE iam_role_policies RENAME COLUMN permission_id TO old_permission_id;
-- ALTER TABLE iam_role_policies RENAME COLUMN new_permission_id TO permission_id;

-- ALTER TABLE permission_audit_logs RENAME COLUMN permission_id TO old_permission_id;
-- ALTER TABLE permission_audit_logs RENAME COLUMN new_permission_id TO permission_id;

-- ALTER TABLE preset_permissions RENAME COLUMN permission_id TO old_permission_id;
-- ALTER TABLE preset_permissions RENAME COLUMN new_permission_id TO permission_id;

-- 6.4 添加新的主键和外键约束
-- ALTER TABLE permission_definitions ADD PRIMARY KEY (id);
-- ALTER TABLE permission_definitions ALTER COLUMN id TYPE VARCHAR(32);

-- ALTER TABLE org_permissions ALTER COLUMN permission_id TYPE VARCHAR(32);
-- ALTER TABLE org_permissions ADD CONSTRAINT org_permissions_permission_id_fkey 
--     FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

-- ALTER TABLE project_permissions ALTER COLUMN permission_id TYPE VARCHAR(32);
-- ALTER TABLE project_permissions ADD CONSTRAINT project_permissions_permission_id_fkey 
--     FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

-- ALTER TABLE workspace_permissions ALTER COLUMN permission_id TYPE VARCHAR(32);
-- ALTER TABLE workspace_permissions ADD CONSTRAINT workspace_permissions_permission_id_fkey 
--     FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

-- ALTER TABLE iam_role_policies ALTER COLUMN permission_id TYPE VARCHAR(32);
-- ALTER TABLE iam_role_policies ADD CONSTRAINT iam_role_policies_permission_id_fkey 
--     FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

-- ALTER TABLE permission_audit_logs ALTER COLUMN permission_id TYPE VARCHAR(32);
-- ALTER TABLE permission_audit_logs ADD CONSTRAINT permission_audit_logs_permission_id_fkey 
--     FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

-- ALTER TABLE preset_permissions ALTER COLUMN permission_id TYPE VARCHAR(32);
-- ALTER TABLE preset_permissions ADD CONSTRAINT preset_permissions_permission_id_fkey 
--     FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

-- 6.5 删除旧的ID字段（可选，建议保留一段时间用于回滚）
-- ALTER TABLE permission_definitions DROP COLUMN IF EXISTS old_id;
-- ALTER TABLE org_permissions DROP COLUMN IF EXISTS old_permission_id;
-- ALTER TABLE project_permissions DROP COLUMN IF EXISTS old_permission_id;
-- ALTER TABLE workspace_permissions DROP COLUMN IF EXISTS old_permission_id;
-- ALTER TABLE iam_role_policies DROP COLUMN IF EXISTS old_permission_id;
-- ALTER TABLE permission_audit_logs DROP COLUMN IF EXISTS old_permission_id;
-- ALTER TABLE preset_permissions DROP COLUMN IF EXISTS old_permission_id;

-- ============================================
-- 阶段7: 验证迁移结果
-- ============================================

-- 验证permission_definitions的新ID格式
SELECT 
    id as semantic_id,
    name,
    resource_type,
    scope_level
FROM permission_definitions
WHERE id LIKE '%pm-%'
ORDER BY id
LIMIT 10;

-- 验证权限授予记录的新ID引用
SELECT 
    op.id,
    op.permission_id as semantic_permission_id,
    pd.name as permission_name,
    op.principal_type,
    op.permission_level
FROM org_permissions op
JOIN permission_definitions pd ON op.permission_id = pd.id
LIMIT 10;

-- 统计迁移结果
SELECT 
    '权限定义总数' as metric,
    COUNT(*) as count
FROM permission_definitions
UNION ALL
SELECT 
    '使用语义ID的权限定义',
    COUNT(*)
FROM permission_definitions
WHERE id LIKE '%pm-%'
UNION ALL
SELECT 
    '组织权限授予记录',
    COUNT(*)
FROM org_permissions
UNION ALL
SELECT 
    '项目权限授予记录',
    COUNT(*)
FROM project_permissions
UNION ALL
SELECT 
    '工作空间权限授予记录',
    COUNT(*)
FROM workspace_permissions;

-- ============================================
-- 说明
-- ============================================

/*
此脚本分为7个阶段：

阶段1-5: 准备阶段（安全，可以在生产环境执行）
- 备份数据
- 添加新字段
- 生成语义ID
- 更新引用
- 验证完整性

阶段6: 切换阶段（需要停机维护，已注释）
- 删除旧约束
- 重命名字段
- 添加新约束
- 这个阶段需要在维护窗口期执行

阶段7: 验证阶段
- 验证迁移结果
- 统计数据

执行建议：
1. 先在测试环境执行完整脚本
2. 在生产环境先执行阶段1-5
3. 验证数据正确性
4. 在维护窗口期执行阶段6
5. 执行阶段7验证结果

回滚方案：
如果需要回滚，可以使用备份表恢复数据：
- DROP TABLE permission_definitions;
- ALTER TABLE permission_definitions_backup RENAME TO permission_definitions;
- 类似操作恢复其他表
*/
