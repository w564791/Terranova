-- 为 ai_configs 表添加 Skill 模式相关字段
-- 执行时间: 2026-01-28
-- 说明: 支持 Skill 组合模式的 AI 配置

-- 添加 mode 字段
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS mode VARCHAR(20) DEFAULT 'prompt';

-- 添加 skill_composition 字段
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS skill_composition JSONB;

-- 添加字段注释
COMMENT ON COLUMN ai_configs.mode IS '配置模式：prompt（提示词模式）或 skill（Skill组合模式）';
COMMENT ON COLUMN ai_configs.skill_composition IS 'Skill组合配置，mode=skill时使用';

-- 验证字段添加成功
SELECT column_name, data_type, column_default 
FROM information_schema.columns 
WHERE table_name = 'ai_configs' 
AND column_name IN ('mode', 'skill_composition');