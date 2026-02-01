-- =============================================
-- User和Team ID优化 - 阶段4: 切换表名
-- 将新表切换为正式表名,旧表重命名为备份
--  此操作需要停止应用服务
-- =============================================

-- 开始事务
BEGIN;

-- =============================================
-- 步骤1: 重命名旧表为备份表
-- =============================================

ALTER TABLE users RENAME TO users_old;
ALTER TABLE teams RENAME TO teams_old;
ALTER TABLE team_members RENAME TO team_members_old;
ALTER TABLE user_organizations RENAME TO user_organizations_old;
ALTER TABLE team_tokens RENAME TO team_tokens_old;

-- =============================================
-- 步骤2: 重命名新表为正式表名
-- =============================================

ALTER TABLE users_new RENAME TO users;
ALTER TABLE teams_new RENAME TO teams;
ALTER TABLE team_members_new RENAME TO team_members;
ALTER TABLE user_organizations_new RENAME TO user_organizations;
ALTER TABLE team_tokens_new RENAME TO team_tokens;

-- =============================================
-- 步骤3: 重命名索引
-- =============================================

-- users表索引
ALTER INDEX IF EXISTS idx_users_new_username RENAME TO idx_users_username;
ALTER INDEX IF EXISTS idx_users_new_email RENAME TO idx_users_email;

-- teams表索引
ALTER INDEX IF EXISTS idx_teams_new_org RENAME TO idx_teams_org;
ALTER INDEX IF EXISTS idx_teams_new_name RENAME TO idx_teams_name;

-- team_members表索引
ALTER INDEX IF EXISTS idx_team_members_new_team RENAME TO idx_team_members_team;
ALTER INDEX IF EXISTS idx_team_members_new_user RENAME TO idx_team_members_user;
ALTER INDEX IF EXISTS idx_team_members_new_joined_by RENAME TO idx_team_members_joined_by;

-- user_organizations表索引
ALTER INDEX IF EXISTS idx_user_organizations_new_user RENAME TO idx_user_organizations_user;
ALTER INDEX IF EXISTS idx_user_organizations_new_org RENAME TO idx_user_organizations_org;

-- team_tokens表索引
ALTER INDEX IF EXISTS idx_team_tokens_new_team RENAME TO idx_team_tokens_team;
ALTER INDEX IF EXISTS idx_team_tokens_new_hash RENAME TO idx_team_tokens_hash;

-- =============================================
-- 步骤4: 添加外键约束
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
-- 步骤5: 创建额外索引
-- =============================================

CREATE INDEX IF NOT EXISTS idx_team_tokens_is_active ON team_tokens(is_active);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- =============================================
-- 步骤6: 验证数据
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
    RAISE NOTICE '下一步:';
    RAISE NOTICE '1. 更新应用代码中的Model定义';
    RAISE NOTICE '2. 重启应用服务';
    RAISE NOTICE '3. 测试所有功能';
    RAISE NOTICE '4. 确认稳定后删除旧表';
    RAISE NOTICE '';
    RAISE NOTICE '回滚方法(如需要):';
    RAISE NOTICE '  ALTER TABLE users RENAME TO users_failed;';
    RAISE NOTICE '  ALTER TABLE users_old RENAME TO users;';
    RAISE NOTICE '  (对其他表执行类似操作)';
    RAISE NOTICE '';
END $$;
