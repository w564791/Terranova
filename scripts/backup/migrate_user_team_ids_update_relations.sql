-- =============================================
-- User和Team ID优化 - 更新关联表
-- 使用映射表更新所有关联表的外键
-- =============================================

-- =============================================
-- 阶段3: 更新team_members表
-- =============================================

-- 1. 创建team_members的新表
CREATE TABLE IF NOT EXISTS team_members_new (
    id SERIAL PRIMARY KEY,
    team_id VARCHAR(20) NOT NULL,
    user_id VARCHAR(20) NOT NULL,
    role VARCHAR(20) DEFAULT 'MEMBER',
    joined_at TIMESTAMP DEFAULT NOW(),
    joined_by VARCHAR(20),
    UNIQUE(team_id, user_id)
);

-- 2. 迁移team_members数据
INSERT INTO team_members_new (id, team_id, user_id, role, joined_at, joined_by)
SELECT 
    tm.id,
    (SELECT new_id FROM team_id_mapping WHERE old_id = tm.team_id),
    (SELECT new_id FROM user_id_mapping WHERE old_id = tm.user_id),
    tm.role,
    tm.joined_at,
    CASE 
        WHEN tm.joined_by IS NOT NULL THEN (SELECT new_id FROM user_id_mapping WHERE old_id = tm.joined_by)
        ELSE NULL
    END
FROM team_members tm
WHERE NOT EXISTS (
    SELECT 1 FROM team_members_new tmn 
    WHERE tmn.id = tm.id
);

-- 3. 创建索引
CREATE INDEX IF NOT EXISTS idx_team_members_new_team ON team_members_new(team_id);
CREATE INDEX IF NOT EXISTS idx_team_members_new_user ON team_members_new(user_id);
CREATE INDEX IF NOT EXISTS idx_team_members_new_joined_by ON team_members_new(joined_by);

-- 4. 添加外键约束(暂时不添加,等切换表名后再添加)
-- 这样可以避免循环依赖问题

-- =============================================
-- 阶段3: 更新user_organizations表
-- =============================================

-- 5. 创建user_organizations的新表
CREATE TABLE IF NOT EXISTS user_organizations_new (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(20) NOT NULL,
    org_id INTEGER NOT NULL,
    joined_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(user_id, org_id)
);

-- 6. 迁移user_organizations数据
INSERT INTO user_organizations_new (id, user_id, org_id, joined_at)
SELECT 
    uo.id,
    (SELECT new_id FROM user_id_mapping WHERE old_id = uo.user_id),
    uo.org_id,
    uo.joined_at
FROM user_organizations uo
WHERE NOT EXISTS (
    SELECT 1 FROM user_organizations_new uon 
    WHERE uon.id = uo.id
);

-- 7. 创建索引
CREATE INDEX IF NOT EXISTS idx_user_organizations_new_user ON user_organizations_new(user_id);
CREATE INDEX IF NOT EXISTS idx_user_organizations_new_org ON user_organizations_new(org_id);

-- =============================================
-- 阶段3: 更新team_tokens表
-- =============================================

-- 8. 创建team_tokens的新表
CREATE TABLE IF NOT EXISTS team_tokens_new (
    id SERIAL PRIMARY KEY,
    team_id VARCHAR(20) NOT NULL,
    token_name VARCHAR(100) NOT NULL,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(20),
    revoked_at TIMESTAMP,
    revoked_by VARCHAR(20),
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP
);

-- 9. 迁移team_tokens数据
INSERT INTO team_tokens_new (id, team_id, token_name, token_hash, is_active, created_at, created_by, revoked_at, revoked_by, last_used_at, expires_at)
SELECT 
    tt.id,
    (SELECT new_id FROM team_id_mapping WHERE old_id = tt.team_id),
    tt.token_name,
    tt.token_hash,
    tt.is_active,
    tt.created_at,
    CASE 
        WHEN tt.created_by IS NOT NULL THEN (SELECT new_id FROM user_id_mapping WHERE old_id = tt.created_by)
        ELSE NULL
    END,
    tt.revoked_at,
    CASE 
        WHEN tt.revoked_by IS NOT NULL THEN (SELECT new_id FROM user_id_mapping WHERE old_id = tt.revoked_by)
        ELSE NULL
    END,
    tt.last_used_at,
    tt.expires_at
FROM team_tokens tt
WHERE NOT EXISTS (
    SELECT 1 FROM team_tokens_new ttn 
    WHERE ttn.id = tt.id
);

-- 10. 创建索引
CREATE INDEX IF NOT EXISTS idx_team_tokens_new_team ON team_tokens_new(team_id);
CREATE INDEX IF NOT EXISTS idx_team_tokens_new_hash ON team_tokens_new(token_hash);

-- =============================================
-- 验证数据
-- =============================================

DO $$
DECLARE
    old_tm_count INTEGER;
    new_tm_count INTEGER;
    old_uo_count INTEGER;
    new_uo_count INTEGER;
    old_tt_count INTEGER;
    new_tt_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO old_tm_count FROM team_members;
    SELECT COUNT(*) INTO new_tm_count FROM team_members_new;
    SELECT COUNT(*) INTO old_uo_count FROM user_organizations;
    SELECT COUNT(*) INTO new_uo_count FROM user_organizations_new;
    SELECT COUNT(*) INTO old_tt_count FROM team_tokens;
    SELECT COUNT(*) INTO new_tt_count FROM team_tokens_new;
    
    RAISE NOTICE '=== 关联表迁移完成 ===';
    RAISE NOTICE 'team_members: 旧表 % 条, 新表 % 条', old_tm_count, new_tm_count;
    RAISE NOTICE 'user_organizations: 旧表 % 条, 新表 % 条', old_uo_count, new_uo_count;
    RAISE NOTICE 'team_tokens: 旧表 % 条, 新表 % 条', old_tt_count, new_tt_count;
    
    IF old_tm_count = new_tm_count AND old_uo_count = new_uo_count AND old_tt_count = new_tt_count THEN
        RAISE NOTICE '✓ 所有关联表数据迁移成功';
    ELSE
        RAISE WARNING '部分表数据数量不匹配,请检查';
    END IF;
END $$;

-- =============================================
-- 查询示例
-- =============================================

-- 查看team_members新表数据
-- SELECT * FROM team_members_new LIMIT 10;

-- 查看user_organizations新表数据
-- SELECT * FROM user_organizations_new LIMIT 10;

-- 查看team_tokens新表数据
-- SELECT * FROM team_tokens_new LIMIT 10;

-- =============================================
-- 注意事项
-- =============================================

-- 1. 此脚本创建关联表的新版本,不会删除或修改旧表
-- 2. 外键约束将在切换表名后添加
-- 3. 执行前请确保已运行migrate_user_team_ids_new_table.sql
-- 4. 下一步需要切换表名并添加外键约束
