-- Create module_demos table
CREATE TABLE IF NOT EXISTS module_demos (
    id SERIAL PRIMARY KEY,
    module_id INTEGER NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    current_version_id INTEGER,
    is_active BOOLEAN DEFAULT true,
    usage_notes TEXT,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create module_demo_versions table
CREATE TABLE IF NOT EXISTS module_demo_versions (
    id SERIAL PRIMARY KEY,
    demo_id INTEGER NOT NULL REFERENCES module_demos(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    is_latest BOOLEAN DEFAULT false,
    config_data JSONB NOT NULL,
    change_summary TEXT,
    change_type VARCHAR(20), -- create, update, rollback
    diff_from_previous TEXT,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Add foreign key for current_version_id after module_demo_versions is created
ALTER TABLE module_demos 
ADD CONSTRAINT fk_module_demos_current_version 
FOREIGN KEY (current_version_id) REFERENCES module_demo_versions(id);

-- Create indexes
CREATE INDEX idx_module_demos_module ON module_demos(module_id);
CREATE INDEX idx_module_demos_active ON module_demos(is_active);
CREATE INDEX idx_module_demo_versions_demo ON module_demo_versions(demo_id);
CREATE INDEX idx_module_demo_versions_latest ON module_demo_versions(is_latest);
CREATE INDEX idx_module_demo_versions_version ON module_demo_versions(demo_id, version);

-- Add comments
COMMENT ON TABLE module_demos IS 'Module demonstration configurations';
COMMENT ON TABLE module_demo_versions IS 'Version history for module demos';
COMMENT ON COLUMN module_demo_versions.config_data IS 'JSON configuration data corresponding to form values';
COMMENT ON COLUMN module_demo_versions.change_type IS 'Type of change: create, update, rollback';
COMMENT ON COLUMN module_demo_versions.diff_from_previous IS 'JSON diff from previous version';
