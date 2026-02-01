-- ============================================================================
-- 添加 provider_config_hash 相关字段
-- 用于优化 terraform init -upgrade 的执行
-- 只在 provider 配置变更时才使用 -upgrade 参数
-- ============================================================================

-- 添加 provider_config_hash 字段
-- 存储 provider_config 的 SHA256 hash，在保存 provider_config 时自动计算
ALTER TABLE workspaces 
ADD COLUMN IF NOT EXISTS provider_config_hash VARCHAR(64);

-- 添加 last_init_hash 字段
-- 存储上次成功执行 terraform init -upgrade 时的 provider_config_hash
ALTER TABLE workspaces 
ADD COLUMN IF NOT EXISTS last_init_hash VARCHAR(64);

-- 添加 last_init_terraform_version 字段
-- 存储上次成功执行 terraform init 时的 terraform 版本
ALTER TABLE workspaces 
ADD COLUMN IF NOT EXISTS last_init_terraform_version VARCHAR(20);

-- 添加注释
COMMENT ON COLUMN workspaces.provider_config_hash IS 'SHA256 hash of provider_config, updated when provider_config changes';
COMMENT ON COLUMN workspaces.last_init_hash IS 'provider_config_hash value when last successful terraform init with -upgrade';
COMMENT ON COLUMN workspaces.last_init_terraform_version IS 'terraform version when last successful terraform init';

-- 为现有的 workspace 计算 provider_config_hash（可选，首次运行时会自动使用 -upgrade）
-- 这里不执行，让系统在首次运行时自动处理
