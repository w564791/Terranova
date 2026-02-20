-- =============================================
-- User和Team ID优化 - 新表迁移方案
-- 创建新表,迁移数据,使用VARCHAR类型的user_id和team_id
-- =============================================

-- =============================================
-- 阶段1: 创建新表和ID生成函数
-- =============================================

-- 1. 创建ID生成函数
CREATE OR REPLACE FUNCTION generate_user_id() RETURNS VARCHAR(20) AS $$
DECLARE
    chars TEXT := 'abcdefghijklmnopqrstuvwxyz0123456789';
    result TEXT := 'user-';
    i INTEGER;
    id_exists BOOLEAN;
BEGIN
    -- 循环直到生成唯一ID
    LOOP
        result := 'user-';
        FOR i IN 1..10 LOOP
            result := result || substr(chars, floor(random() * length(chars) + 1)::int, 1);
        END LOOP;
        
        -- 检查ID是否已存在(检查新表和旧表)
        SELECT EXISTS(
            SELECT 1 FROM users_new WHERE user_id = result
            UNION
            SELECT 1 FROM users WHERE id::text = result
        ) INTO id_exists;
        
        IF NOT id_exists THEN
            RETURN result;
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION generate_team_id() RETURNS VARCHAR(20) AS $$
DECLARE
    chars TEXT := 'abcdefghijklmnopqrstuvwxyz0123456789';
    result TEXT := 'team-';
    i INTEGER;
    id_exists BOOLEAN;
BEGIN
    -- 循环直到生成唯一ID
    LOOP
        result := 'team-';
        FOR i IN 1..10 LOOP
            result := result || substr(chars, floor(random() * length(chars) + 1)::int, 1);
        END LOOP;
        
        -- 检查ID是否已存在(检查新表和旧表)
        SELECT EXISTS(
            SELECT 1 FROM teams_new WHERE team_id = result
            UNION
            SELECT 1 FROM teams WHERE id::text = result
        ) INTO id_exists;
        
        IF NOT id_exists THEN
            RETURN result;
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- 2. 创建新的users表
CREATE TABLE IF NOT EXISTS users_new (
    user_id VARCHAR(20) PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) DEFAULT 'user',
    is_active BOOLEAN DEFAULT TRUE,
    is_system_admin BOOLEAN DEFAULT FALSE,
    oauth_provider VARCHAR(50),
    oauth_id VARCHAR(200),
    last_login_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 3. 创建新的teams表
CREATE TABLE IF NOT EXISTS teams_new (
    team_id VARCHAR(20) PRIMARY KEY,
    org_id INTEGER NOT NULL,
    name VARCHAR(100) NOT NULL,
    display_name VARCHAR(200),
    description TEXT,
    is_system BOOLEAN DEFAULT FALSE,
    created_by VARCHAR(20),  -- 将引用users_new.user_id
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(org_id, name)
);

-- 4. 创建ID映射表
CREATE TABLE IF NOT EXISTS user_id_mapping (
    old_id INTEGER PRIMARY KEY,
    new_id VARCHAR(20) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS team_id_mapping (
    old_id INTEGER PRIMARY KEY,
    new_id VARCHAR(20) NOT NULL UNIQUE
);

-- 5. 创建索引
CREATE INDEX IF NOT EXISTS idx_users_new_username ON users_new(username);
CREATE INDEX IF NOT EXISTS idx_users_new_email ON users_new(email);
CREATE INDEX IF NOT EXISTS idx_teams_new_org ON teams_new(org_id);
CREATE INDEX IF NOT EXISTS idx_teams_new_name ON teams_new(org_id, name);

-- =============================================
-- 阶段2: 迁移数据
-- =============================================

-- 6. 迁移users数据
INSERT INTO users_new (user_id, username, email, password_hash, role, is_active, is_system_admin, oauth_provider, oauth_id, last_login_at, created_at, updated_at)
SELECT 
    generate_user_id(),
    username,
    email,
    password_hash,
    role,
    is_active,
    COALESCE(is_system_admin, FALSE),
    oauth_provider,
    oauth_id,
    last_login_at,
    created_at,
    updated_at
FROM users
WHERE NOT EXISTS (
    SELECT 1 FROM users_new un WHERE un.username = users.username
);

-- 7. 填充user_id映射表
INSERT INTO user_id_mapping (old_id, new_id)
SELECT u.id, un.user_id
FROM users u
JOIN users_new un ON u.username = un.username
WHERE NOT EXISTS (
    SELECT 1 FROM user_id_mapping um WHERE um.old_id = u.id
);

-- 8. 迁移teams数据
INSERT INTO teams_new (team_id, org_id, name, display_name, description, is_system, created_by, created_at, updated_at)
SELECT 
    generate_team_id(),
    t.org_id,
    t.name,
    t.display_name,
    t.description,
    t.is_system,
    CASE 
        WHEN t.created_by IS NOT NULL THEN (SELECT new_id FROM user_id_mapping WHERE old_id = t.created_by)
        ELSE NULL
    END,
    t.created_at,
    t.updated_at
FROM teams t
WHERE NOT EXISTS (
    SELECT 1 FROM teams_new tn WHERE tn.org_id = t.org_id AND tn.name = t.name
);

-- 9. 填充team_id映射表
INSERT INTO team_id_mapping (old_id, new_id)
SELECT t.id, tn.team_id
FROM teams t
JOIN teams_new tn ON t.org_id = tn.org_id AND t.name = tn.name
WHERE NOT EXISTS (
    SELECT 1 FROM team_id_mapping tm WHERE tm.old_id = t.id
);

-- 10. 添加外键约束
ALTER TABLE teams_new 
ADD CONSTRAINT teams_new_created_by_fkey 
FOREIGN KEY (created_by) REFERENCES users_new(user_id) ON DELETE SET NULL;

ALTER TABLE teams_new
ADD CONSTRAINT teams_new_org_id_fkey
FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE;

-- =============================================
-- 验证数据
-- =============================================

DO $$
DECLARE
    old_user_count INTEGER;
    new_user_count INTEGER;
    old_team_count INTEGER;
    new_team_count INTEGER;
    user_mapping_count INTEGER;
    team_mapping_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO old_user_count FROM users;
    SELECT COUNT(*) INTO new_user_count FROM users_new;
    SELECT COUNT(*) INTO old_team_count FROM teams;
    SELECT COUNT(*) INTO new_team_count FROM teams_new;
    SELECT COUNT(*) INTO user_mapping_count FROM user_id_mapping;
    SELECT COUNT(*) INTO team_mapping_count FROM team_id_mapping;
    
    RAISE NOTICE '=== 数据迁移完成 ===';
    RAISE NOTICE 'Users: 旧表 % 条, 新表 % 条, 映射 % 条', old_user_count, new_user_count, user_mapping_count;
    RAISE NOTICE 'Teams: 旧表 % 条, 新表 % 条, 映射 % 条', old_team_count, new_team_count, team_mapping_count;
    
    IF old_user_count != new_user_count THEN
        RAISE WARNING 'Users数据数量不匹配!';
    END IF;
    
    IF old_team_count != new_team_count THEN
        RAISE WARNING 'Teams数据数量不匹配!';
    END IF;
    
    IF old_user_count != user_mapping_count THEN
        RAISE WARNING 'User ID映射数量不匹配!';
    END IF;
    
    IF old_team_count != team_mapping_count THEN
        RAISE WARNING 'Team ID映射数量不匹配!';
    END IF;
    
    IF old_user_count = new_user_count AND old_user_count = user_mapping_count AND
       old_team_count = new_team_count AND old_team_count = team_mapping_count THEN
        RAISE NOTICE '✓ 所有数据迁移成功,数量一致';
    END IF;
END $$;

-- =============================================
-- 查询示例
-- =============================================

-- 查看映射关系
-- SELECT * FROM user_id_mapping LIMIT 10;
-- SELECT * FROM team_id_mapping LIMIT 10;

-- 查看新表数据
-- SELECT * FROM users_new LIMIT 10;
-- SELECT * FROM teams_new LIMIT 10;

-- =============================================
-- 注意事项
-- =============================================

-- 1. 此脚本创建新表并迁移数据,不会删除或修改旧表
-- 2. 映射表用于后续更新关联表的外键
-- 3. 执行前请确保已备份数据库
-- 4. 下一步需要更新所有关联表的外键字段
-- 5. 最后切换表名: users->users_old, users_new->users
