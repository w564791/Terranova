-- 创建terraform_versions表
CREATE TABLE IF NOT EXISTS terraform_versions (
    id SERIAL PRIMARY KEY,
    version VARCHAR(50) NOT NULL UNIQUE,
    download_url TEXT NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    enabled BOOLEAN DEFAULT true,
    deprecated BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_terraform_versions_enabled ON terraform_versions(enabled);
CREATE INDEX IF NOT EXISTS idx_terraform_versions_version ON terraform_versions(version);
CREATE INDEX IF NOT EXISTS idx_terraform_versions_deprecated ON terraform_versions(deprecated);

-- 插入默认数据
INSERT INTO terraform_versions (version, download_url, checksum, enabled, deprecated) VALUES
('1.5.0', 'https://releases.hashicorp.com/terraform/1.5.0/terraform_1.5.0_linux_amd64.zip', 'ad0c696c870c8525357b5127680cd79c0bdf58179af9acd091d43b1d6482da4a', true, false),
('1.4.6', 'https://releases.hashicorp.com/terraform/1.4.6/terraform_1.4.6_linux_amd64.zip', '3e9c46d6f37338e90d5018c156d89961b0ffb0f355249679593aff99f9abe2a2', true, false),
('1.3.9', 'https://releases.hashicorp.com/terraform/1.3.9/terraform_1.3.9_linux_amd64.zip', 'a73326ea8fb06f6976597e005f4b3a5d575f5d7d14e41d7f8f34e1f8e3b3c3e8', true, true)
ON CONFLICT (version) DO NOTHING;

-- 添加注释
COMMENT ON TABLE terraform_versions IS 'Terraform版本管理表';
COMMENT ON COLUMN terraform_versions.id IS '主键ID';
COMMENT ON COLUMN terraform_versions.version IS '版本号（如1.5.0）';
COMMENT ON COLUMN terraform_versions.download_url IS '下载链接';
COMMENT ON COLUMN terraform_versions.checksum IS 'SHA256校验和';
COMMENT ON COLUMN terraform_versions.enabled IS '是否启用';
COMMENT ON COLUMN terraform_versions.deprecated IS '是否已弃用';
COMMENT ON COLUMN terraform_versions.created_at IS '创建时间';
COMMENT ON COLUMN terraform_versions.updated_at IS '更新时间';
