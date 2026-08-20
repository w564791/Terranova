-- Patch: task_temporary_permissions 双键匹配辅助索引
-- 关联：docs/iam/35 B-3 / 临时权限 user_id 字符串列
-- 说明：user_id 列在库中已是 varchar(20)；本补丁补索引与空串归一。

-- 空串视为 NULL，避免错误命中
UPDATE task_temporary_permissions SET user_id = NULL WHERE user_id = '';

CREATE INDEX IF NOT EXISTS idx_temp_perms_task_user_id
  ON task_temporary_permissions (task_id, user_id)
  WHERE user_id IS NOT NULL AND user_id <> '';
