-- Create pool_tokens table for Agent Pool Token management
-- Supports both static tokens and K8s temporary tokens

CREATE TABLE IF NOT EXISTS pool_tokens (
    -- Primary key using token hash
    token_hash VARCHAR(64) PRIMARY KEY,
    
    -- Token metadata
    token_name VARCHAR(100) NOT NULL,
    token_type VARCHAR(20) NOT NULL CHECK (token_type IN ('static', 'k8s_temporary')),
    
    -- Pool reference
    pool_id VARCHAR(50) NOT NULL REFERENCES agent_pools(pool_id) ON DELETE CASCADE,
    
    -- Token status
    is_active BOOLEAN DEFAULT true NOT NULL,
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    created_by VARCHAR(50),
    revoked_at TIMESTAMP,
    revoked_by VARCHAR(50),
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP,
    
    -- K8s specific fields
    k8s_job_name VARCHAR(255),
    k8s_pod_name VARCHAR(255),
    k8s_namespace VARCHAR(100) DEFAULT 'terraform',
    
    -- Indexes
    CONSTRAINT idx_pool_tokens_pool_id_active 
        CHECK (pool_id IS NOT NULL)
);

-- Create indexes for efficient queries
CREATE INDEX idx_pool_tokens_pool_id ON pool_tokens(pool_id);
CREATE INDEX idx_pool_tokens_type ON pool_tokens(token_type);
CREATE INDEX idx_pool_tokens_active ON pool_tokens(is_active);
CREATE INDEX idx_pool_tokens_expires_at ON pool_tokens(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX idx_pool_tokens_k8s_job ON pool_tokens(k8s_job_name) WHERE k8s_job_name IS NOT NULL;

-- Add comment
COMMENT ON TABLE pool_tokens IS 'Agent Pool tokens for both static and K8s temporary authentication';
COMMENT ON COLUMN pool_tokens.token_type IS 'Type of token: static (long-lived) or k8s_temporary (auto-revoked)';
COMMENT ON COLUMN pool_tokens.token_hash IS 'SHA-256 hash of the token, used as primary key';
COMMENT ON COLUMN pool_tokens.k8s_namespace IS 'K8s namespace for the job, fixed to terraform';
