-- Application workspace tag 访问过滤（选项 A）
-- workspace_tag_filter: JSON 对象，如 {"env":"prod"} 或 {"env":["prod","staging"]}
-- 空/NULL = 不按 tag 限制（仅 org 级 WORKSPACES grant）

ALTER TABLE applications
  ADD COLUMN IF NOT EXISTS workspace_tag_filter JSONB DEFAULT NULL;

COMMENT ON COLUMN applications.workspace_tag_filter IS
  'Application 可访问 workspace 的 tag 匹配规则（AND）；空=不限制';
