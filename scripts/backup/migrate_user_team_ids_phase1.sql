-- =============================================
-- User和Team ID优化 - 阶段1: 准备期
-- 添加新ID字段并为现有记录生成新ID
-- =============================================

-- 1. 添加新ID字段
ALTER TABLE users ADD COLUMN IF NOT EXISTS new_id VARCHAR(20);
ALTER TABLE teams ADD COLUMN IF NOT EXISTS new_id VARCHAR(20);

-- 2. 创建ID生成函数
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
        
        -- 检查ID是否已存在
        SELECT EXISTS(SELECT 1 FROM users u WHERE u.new_id = result) INTO id_exists;
        
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
        
        -- 检查ID是否已存在
        SELECT EXISTS(SELECT 1 FROM teams t WHERE t.new_id = result) INTO id_exists;
        
        IF NOT id_exists THEN
            RETURN result;
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- 3. 为现有记录生成新ID
UPDATE users SET new_id = generate_user_id() WHERE new_id IS NULL;
UPDATE teams SET new_id = generate_team_id() WHERE new_id IS NULL;

-- 4. 添加唯一约束和索引
ALTER TABLE users ADD CONSTRAINT users_new_id_unique UNIQUE (new_id);
ALTER TABLE teams ADD CONSTRAINT teams_new_id_unique UNIQUE (new_id);
CREATE INDEX IF NOT EXISTS idx_users_new_id ON users(new_id);
CREATE INDEX IF NOT EXISTS idx_teams_new_id ON teams(new_id);

-- 5. 添加触发器,确保新记录自动生成新ID
CREATE OR REPLACE FUNCTION auto_generate_user_id() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.new_id IS NULL THEN
        NEW.new_id := generate_user_id();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_auto_generate_user_id ON users;
CREATE TRIGGER trigger_auto_generate_user_id
BEFORE INSERT ON users
FOR EACH ROW
EXECUTE FUNCTION auto_generate_user_id();

CREATE OR REPLACE FUNCTION auto_generate_team_id() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.new_id IS NULL THEN
        NEW.new_id := generate_team_id();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_auto_generate_team_id ON teams;
CREATE TRIGGER trigger_auto_generate_team_id
BEFORE INSERT ON teams
FOR EACH ROW
EXECUTE FUNCTION auto_generate_team_id();

-- 6. 验证数据
DO $$
DECLARE
    user_count INTEGER;
    user_new_id_count INTEGER;
    team_count INTEGER;
    team_new_id_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO user_count FROM users;
    SELECT COUNT(*) INTO user_new_id_count FROM users WHERE new_id IS NOT NULL;
    SELECT COUNT(*) INTO team_count FROM teams;
    SELECT COUNT(*) INTO team_new_id_count FROM teams WHERE new_id IS NOT NULL;
    
    RAISE NOTICE '=== 阶段1迁移完成 ===';
    RAISE NOTICE 'Users: % 条记录, % 条已生成新ID', user_count, user_new_id_count;
    RAISE NOTICE 'Teams: % 条记录, % 条已生成新ID', team_count, team_new_id_count;
    
    IF user_count != user_new_id_count THEN
        RAISE EXCEPTION 'Users表存在未生成新ID的记录';
    END IF;
    
    IF team_count != team_new_id_count THEN
        RAISE EXCEPTION 'Teams表存在未生成新ID的记录';
    END IF;
    
    RAISE NOTICE '所有记录已成功生成新ID';
END $$;
