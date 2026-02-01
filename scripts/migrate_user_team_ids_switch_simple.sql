-- =============================================
-- User和Team ID优化 - 阶段4: 简化切换
-- 直接删除旧表,重命名新表
--  此操作会删除旧表,请确保已备份
-- =============================================

-- 开始事务
BEGIN;

-- =============================================
-- 步骤1: 删除旧表(CASCADE会自动删除相关外键和索引)
-- =============================================

DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS teams CASCADE;
DROP TABLE IF EXISTS team_members CASCADE;
DROP TABLE IF EXISTS user_organizations CASCADE;
DROP TABLE IF EXISTS team_tokens CASCADE;

-- =============================================
-- 步骤2: 重命名新表为正式表名
-- =============================================

ALTER TABLE users_new RENAME TO users;
ALTER TABLE teams_new RENAME TO teams;
ALTER TABLE team_members_new RENAME TO team_members;
ALTER TABLE user_organizations_new RENAME TO user_organizations;
ALTER TABLE team_tokens_new RENAME TO team_tokens;

-- =============================================
-- 步骤3: 添加外键约束
-- =============================================

-- teams表外键
ALTER TABLE teams 
ADD CONSTRAINT teams_created_by_fkey 
FOREIGN KEY (created_by) REFERENCES users(user_id) ON DELETE SET NULL;

ALTER TABLE teams
ADD CONSTRAINT teams_org_id_fkey
FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE;

-- team_members表外键
ALTER TABLE team_members 
ADD CONSTRAINT team_members_user_id_fkey 
FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE;

ALTER TABLE team_members 
ADD CONSTRAINT team_members_team_id_fkey 
FOREIGN KEY (team_id) REFERENCES teams(team_id) ON DELETE CASCADE;

ALTER TABLE team_members 
ADD CONSTRAINT team_members_joined_by_fkey 
FOREIGN KEY (joined_by) REFERENCES users(user_id) ON DELETE SET NULL;

-- user_organizations表外键
ALTER TABLE user_organizations 
ADD CONSTRAINT user_organizations_user_id_fkey 
FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE;

ALTER TABLE user_organizations 
ADD CONSTRAINT user_organizations_org_id_fkey 
FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE;

-- team_tokens表外键
ALTER TABLE team_tokens 
ADD CONSTRAINT team_tokens_team_id_fkey 
FOREIGN KEY (team_id) REFERENCES teams(team_id) ON DELETE CASCADE;

ALTER TABLE team_tokens 
ADD CONSTRAINT team_tokens_created_by_fkey 
FOREIGN KEY (created_by) REFERENCES users(user_id) ON DELETE SET NULL;

ALTER TABLE team_tokens 
ADD CONSTRAINT team_tokens_revoked_by_fkey 
FOREIGN KEY (revoked_by) REFERENCES users(user_id) ON DELETE SET NULL;

-- =============================================
-- 步骤4: 验证数据
-- =============================================

DO $$
DECLARE
    user_count INTEGER;
    team_count INTEGER;
    tm_count INTEGER;
    tt_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO user_count FROM users;
    SELECT COUNT(*) INTO team_count FROM teams;
    SELECT COUNT(*) INTO tm_count FROM team_members;
    SELECT COUNT(*) INTO tt_count FROM team_tokens;
    
    RAISE NOTICE '=== 表切换完成 ===';
    RAISE NOTICE 'users: % 条记录', user_count;
    RAISE NOTICE 'teams: % 条记录', team_count;
    RAISE NOTICE 'team_members: % 条记录', tm_count;
    RAISE NOTICE 'team_tokens: % 条记录', tt_count;
    
    IF user_count >= 2 AND team_count >= 3 THEN
        RAISE NOTICE '✓ 数据验证通过';
    ELSE
        RAISE EXCEPTION '数据验证失败,请检查';
    END IF;
END $$;

-- 提交事务
COMMIT;

-- =============================================
-- 完成提示
-- =============================================

DO $$
BEGIN
    RAISE NOTICE '';
    RAISE NOTICE '========================================';
    RAISE NOTICE '表切换成功完成!';
    RAISE NOTICE '========================================';
    RAISE NOTICE '';
    RAISE NOTICE '新ID格式已生效:';
    RAISE NOTICE '  - User ID: user-{10位随机字符}';
    RAISE NOTICE '  - Team ID: team-{10位随机字符}';
    RAISE NOTICE '';
    RAISE NOTICE '下一步:';
    RAISE NOTICE '1. 重启应用服务';
    RAISE NOTICE '2. 测试所有功能';
    RAISE NOTICE '3. 清理映射表';
    RAISE NOTICE '';
END $$;
