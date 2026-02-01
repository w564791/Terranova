-- 添加 terraform_lock_hcl 字段到 workspaces 表
-- 用于存储 .terraform.lock.hcl 文件内容，加速 terraform init

ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS terraform_lock_hcl TEXT;

COMMENT ON COLUMN workspaces.terraform_lock_hcl IS 'Terraform lock file content (.terraform.lock.hcl) for provider version pinning';
