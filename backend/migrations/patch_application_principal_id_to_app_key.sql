-- 将 org_permissions 中 APPLICATION 主体的数字 principal_id 迁移为 app_key
-- 运行时 AgentAuth / grant 已统一 app_key；Checker 仍兼容双 id，本脚本用于数据归一。
-- 幂等：仅当 principal_id 为纯数字且能匹配 applications.id 时更新。

BEGIN;

-- app_key is `app_` + 32 hex characters. Legacy permission tables used
-- varchar(20), so widening must happen before any numeric-id -> app_key write.
ALTER TABLE org_permissions ALTER COLUMN principal_id TYPE varchar(64);
ALTER TABLE project_permissions ALTER COLUMN principal_id TYPE varchar(64);
ALTER TABLE workspace_permissions ALTER COLUMN principal_id TYPE varchar(64);

UPDATE org_permissions op
SET principal_id = a.app_key
FROM applications a
WHERE op.principal_type = 'APPLICATION'
  AND a.id::text = op.principal_id
  AND a.app_key IS NOT NULL
  AND a.app_key <> ''
  AND op.principal_id ~ '^[0-9]+$';

-- 项目/工作区级 APPLICATION grant（若存在历史脏数据；正常模型不应写入）
UPDATE project_permissions pp
SET principal_id = a.app_key
FROM applications a
WHERE pp.principal_type = 'APPLICATION'
  AND a.id::text = pp.principal_id
  AND a.app_key IS NOT NULL
  AND a.app_key <> ''
  AND pp.principal_id ~ '^[0-9]+$';

UPDATE workspace_permissions wp
SET principal_id = a.app_key
FROM applications a
WHERE wp.principal_type = 'APPLICATION'
  AND a.id::text = wp.principal_id
  AND a.app_key IS NOT NULL
  AND a.app_key <> ''
  AND wp.principal_id ~ '^[0-9]+$';

COMMIT;

-- 验收：不应再存在纯数字 APPLICATION principal_id（或仅剩找不到 application 的孤儿）
-- SELECT principal_id, COUNT(*) FROM org_permissions
-- WHERE principal_type = 'APPLICATION' AND principal_id ~ '^[0-9]+$'
-- GROUP BY 1;
