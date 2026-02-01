-- Add status column to modules table
ALTER TABLE modules ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'active';

-- Update existing modules to have active status
UPDATE modules SET status = 'active' WHERE status IS NULL OR status = '';

-- Add comment
COMMENT ON COLUMN modules.status IS 'Module status: active or inactive';
