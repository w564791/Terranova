-- Skill 系统数据库迁移脚本
-- 版本: 1.0
-- 日期: 2026-01-28
-- 描述: 创建 skills 和 skill_usage_logs 表，修改 ai_configs 表

-- ========== 1. 创建 skills 表 ==========
CREATE TABLE IF NOT EXISTS skills (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    layer VARCHAR(20) NOT NULL CHECK (layer IN ('foundation', 'domain', 'task')),
    content TEXT NOT NULL,
    version VARCHAR(50) DEFAULT '1.0.0',
    is_active BOOLEAN DEFAULT true,
    priority INTEGER DEFAULT 0,
    source_type VARCHAR(50) NOT NULL CHECK (source_type IN ('manual', 'module_auto', 'hybrid')),
    source_module_id INTEGER,
    metadata JSONB DEFAULT '{}',
    created_by VARCHAR(20),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT fk_skills_source_module FOREIGN KEY (source_module_id) REFERENCES modules(id) ON DELETE SET NULL
);

-- 创建唯一索引
CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_name ON skills(name);

-- 创建普通索引
CREATE INDEX IF NOT EXISTS idx_skills_layer ON skills(layer);
CREATE INDEX IF NOT EXISTS idx_skills_source_module_id ON skills(source_module_id);
CREATE INDEX IF NOT EXISTS idx_skills_is_active ON skills(is_active);
CREATE INDEX IF NOT EXISTS idx_skills_source_type ON skills(source_type);

-- 添加注释
COMMENT ON TABLE skills IS 'AI Skill 知识单元表';
COMMENT ON COLUMN skills.id IS 'Skill 唯一标识（UUID）';
COMMENT ON COLUMN skills.name IS 'Skill 唯一名称标识，如 platform_introduction';
COMMENT ON COLUMN skills.display_name IS '显示名称，如 平台介绍';
COMMENT ON COLUMN skills.layer IS '层级：foundation（基础层）/ domain（领域层）/ task（任务层）';
COMMENT ON COLUMN skills.content IS 'Markdown 格式的 Skill 内容';
COMMENT ON COLUMN skills.version IS '语义化版本号，如 1.2.3';
COMMENT ON COLUMN skills.is_active IS '是否激活';
COMMENT ON COLUMN skills.priority IS '同层级内的加载优先级，数字越小越先加载';
COMMENT ON COLUMN skills.source_type IS '来源类型：manual（手动创建）/ module_auto（Module 自动生成）/ hybrid（自动生成后手动修改）';
COMMENT ON COLUMN skills.source_module_id IS '如果是 Module 生成，关联的 Module ID';
COMMENT ON COLUMN skills.metadata IS '元数据：tags, description, author 等';
COMMENT ON COLUMN skills.created_by IS '创建者用户 ID';

-- ========== 2. 创建 skill_usage_logs 表 ==========
CREATE TABLE IF NOT EXISTS skill_usage_logs (
    id VARCHAR(36) PRIMARY KEY,
    skill_ids JSONB NOT NULL,
    capability VARCHAR(100) NOT NULL,
    workspace_id VARCHAR(20),
    user_id VARCHAR(20) NOT NULL,
    module_id INTEGER,
    execution_time_ms INTEGER,
    user_feedback INTEGER CHECK (user_feedback >= 1 AND user_feedback <= 5),
    ai_model VARCHAR(100),
    context_summary TEXT,
    response_summary TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_skill_usage_logs_capability ON skill_usage_logs(capability);
CREATE INDEX IF NOT EXISTS idx_skill_usage_logs_workspace_id ON skill_usage_logs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_skill_usage_logs_user_id ON skill_usage_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_skill_usage_logs_created_at ON skill_usage_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_skill_usage_logs_skill_ids ON skill_usage_logs USING GIN (skill_ids);

-- 添加注释
COMMENT ON TABLE skill_usage_logs IS 'Skill 使用日志表，用于追踪 Skill 使用情况和效果';
COMMENT ON COLUMN skill_usage_logs.id IS '日志唯一标识（UUID）';
COMMENT ON COLUMN skill_usage_logs.skill_ids IS '本次使用的所有 Skill ID 数组';
COMMENT ON COLUMN skill_usage_logs.capability IS '触发的功能：form_generation, intent_assertion, cmdb_query_plan 等';
COMMENT ON COLUMN skill_usage_logs.workspace_id IS '关联的 Workspace ID';
COMMENT ON COLUMN skill_usage_logs.user_id IS '用户 ID';
COMMENT ON COLUMN skill_usage_logs.module_id IS '关联的 Module ID';
COMMENT ON COLUMN skill_usage_logs.execution_time_ms IS '执行时长（毫秒）';
COMMENT ON COLUMN skill_usage_logs.user_feedback IS '用户评分 1-5';
COMMENT ON COLUMN skill_usage_logs.ai_model IS '使用的 AI 模型';
COMMENT ON COLUMN skill_usage_logs.context_summary IS '调用时的上下文摘要';
COMMENT ON COLUMN skill_usage_logs.response_summary IS 'AI 响应摘要';

-- ========== 3. 修改 ai_configs 表，新增 Skill 模式相关字段 ==========
-- 添加 mode 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'ai_configs' AND column_name = 'mode'
    ) THEN
        ALTER TABLE ai_configs ADD COLUMN mode VARCHAR(20) DEFAULT 'prompt' CHECK (mode IN ('prompt', 'skill'));
        COMMENT ON COLUMN ai_configs.mode IS '配置模式：prompt（提示词模式）或 skill（Skill 组合模式）';
    END IF;
END $$;

-- 添加 skill_composition 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'ai_configs' AND column_name = 'skill_composition'
    ) THEN
        ALTER TABLE ai_configs ADD COLUMN skill_composition JSONB;
        COMMENT ON COLUMN ai_configs.skill_composition IS 'Skill 组合配置，mode=skill 时使用';
    END IF;
END $$;

-- ========== 4. 验证表创建成功 ==========
DO $$
BEGIN
    -- 检查 skills 表
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'skills') THEN
        RAISE EXCEPTION 'skills 表创建失败';
    END IF;
    
    -- 检查 skill_usage_logs 表
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'skill_usage_logs') THEN
        RAISE EXCEPTION 'skill_usage_logs 表创建失败';
    END IF;
    
    -- 检查 ai_configs 表的新字段
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'ai_configs' AND column_name = 'mode'
    ) THEN
        RAISE EXCEPTION 'ai_configs.mode 字段添加失败';
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'ai_configs' AND column_name = 'skill_composition'
    ) THEN
        RAISE EXCEPTION 'ai_configs.skill_composition 字段添加失败';
    END IF;
    
    RAISE NOTICE 'Skill 系统数据库迁移成功！';
END $$;