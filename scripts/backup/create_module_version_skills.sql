-- Module Version Skills 数据库迁移脚本
-- 版本: 1.0
-- 日期: 2026-01-28
-- 描述: 创建 module_version_skills 表，用于存储与 Module 版本关联的 Skill

-- ========== 1. 创建 module_version_skills 表 ==========
CREATE TABLE IF NOT EXISTS module_version_skills (
    id VARCHAR(36) PRIMARY KEY,
    module_version_id VARCHAR(36) NOT NULL,
    
    -- Schema 生成的 Skill（AI 自动生成）
    schema_generated_content TEXT,
    schema_generated_at TIMESTAMP,
    schema_version_used INTEGER,
    
    -- 用户自定义的 Skill
    custom_content TEXT,
    
    -- 继承信息
    inherited_from_version_id VARCHAR(36),
    
    -- 元数据
    is_active BOOLEAN DEFAULT true,
    created_by VARCHAR(20),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT fk_mvs_module_version FOREIGN KEY (module_version_id) 
        REFERENCES module_versions(id) ON DELETE CASCADE,
    CONSTRAINT fk_mvs_inherited_from FOREIGN KEY (inherited_from_version_id) 
        REFERENCES module_versions(id) ON DELETE SET NULL
);

-- 创建唯一索引（每个版本只能有一个 Skill 记录）
CREATE UNIQUE INDEX IF NOT EXISTS idx_mvs_module_version_id ON module_version_skills(module_version_id);

-- 创建普通索引
CREATE INDEX IF NOT EXISTS idx_mvs_inherited_from ON module_version_skills(inherited_from_version_id);
CREATE INDEX IF NOT EXISTS idx_mvs_is_active ON module_version_skills(is_active);
CREATE INDEX IF NOT EXISTS idx_mvs_created_at ON module_version_skills(created_at);

-- 添加注释
COMMENT ON TABLE module_version_skills IS 'Module 版本关联的 AI Skill 表';
COMMENT ON COLUMN module_version_skills.id IS 'Skill 唯一标识（UUID）';
COMMENT ON COLUMN module_version_skills.module_version_id IS '关联的 Module 版本 ID';
COMMENT ON COLUMN module_version_skills.schema_generated_content IS 'AI 根据 Schema 自动生成的 Skill 内容（Markdown 格式）';
COMMENT ON COLUMN module_version_skills.schema_generated_at IS 'Schema Skill 生成时间';
COMMENT ON COLUMN module_version_skills.schema_version_used IS '生成时使用的 Schema 版本号';
COMMENT ON COLUMN module_version_skills.custom_content IS '用户自定义的额外 Skill 内容（Markdown 格式）';
COMMENT ON COLUMN module_version_skills.inherited_from_version_id IS '如果是继承的，记录来源版本 ID';
COMMENT ON COLUMN module_version_skills.is_active IS '是否激活';
COMMENT ON COLUMN module_version_skills.created_by IS '创建者用户 ID';

-- ========== 2. 验证表创建成功 ==========
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'module_version_skills') THEN
        RAISE EXCEPTION 'module_version_skills 表创建失败';
    END IF;
    
    RAISE NOTICE 'module_version_skills 表创建成功！';
END $$;