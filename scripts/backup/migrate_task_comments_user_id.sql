-- 迁移 task_comments 表的 user_id 字段

ALTER TABLE task_comments 
    ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

-- 验证
SELECT column_name, data_type, character_maximum_length 
FROM information_schema.columns 
WHERE table_name = 'task_comments' AND column_name = 'user_id';
