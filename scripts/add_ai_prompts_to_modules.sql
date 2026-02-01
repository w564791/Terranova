-- 添加 ai_prompts 字段到 modules 表
-- 用于存储模块的 AI 助手提示词列表

ALTER TABLE modules 
ADD COLUMN IF NOT EXISTS ai_prompts JSONB DEFAULT '[]';

-- 添加注释
COMMENT ON COLUMN modules.ai_prompts IS 'AI 助手提示词列表，格式: [{"id": "uuid", "title": "标题", "prompt": "提示词内容", "created_at": "时间"}]';

-- 示例数据结构:
-- [
--   {
--     "id": "uuid-1",
--     "title": "创建生产环境 EC2",
--     "prompt": "在 exchange VPC 的东京1a创建一台 ec2，安全组使用 java-private，主机名称使用 xxx，使用 t3.medium 类型",
--     "created_at": "2026-01-26T12:00:00Z"
--   },
--   {
--     "id": "uuid-2",
--     "title": "创建开发环境配置",
--     "prompt": "创建一个开发环境的配置，使用 t3.small 实例类型，启用调试模式",
--     "created_at": "2026-01-26T12:30:00Z"
--   }
-- ]
