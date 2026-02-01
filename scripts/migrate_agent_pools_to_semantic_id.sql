-- Migration: Agent Pools to Semantic ID
-- Purpose: Migrate agent_pools table from integer id to semantic string pool_id
-- Format: pool-{16位随机a-z0-9}
-- Date: 2025-10-29

-- Step 1: Add new pool_id column
ALTER TABLE agent_pools 
ADD COLUMN pool_id VARCHAR(50);

-- Step 2: Generate semantic pool_id for existing record
-- For the existing default-pool, we'll use a fixed semantic ID
UPDATE agent_pools 
SET pool_id = 'pool-default00000001'
WHERE id = 1;

-- Step 3: Make pool_id NOT NULL after data migration
ALTER TABLE agent_pools 
ALTER COLUMN pool_id SET NOT NULL;

-- Step 4: Add unique constraint on pool_id
ALTER TABLE agent_pools 
ADD CONSTRAINT agent_pools_pool_id_unique UNIQUE (pool_id);

-- Step 5: Drop foreign key constraints from dependent tables
ALTER TABLE agents 
DROP CONSTRAINT IF EXISTS agents_pool_id_fkey;

ALTER TABLE workspaces 
DROP CONSTRAINT IF EXISTS workspaces_agent_pool_id_fkey;

-- Step 6: Modify agents.pool_id to VARCHAR(50)
ALTER TABLE agents 
ALTER COLUMN pool_id TYPE VARCHAR(50);

-- Step 7: Modify workspaces.agent_pool_id to VARCHAR(50)
ALTER TABLE workspaces 
ALTER COLUMN agent_pool_id TYPE VARCHAR(50);

-- Step 8: Add new foreign key constraints referencing pool_id
ALTER TABLE agents 
ADD CONSTRAINT agents_pool_id_fkey 
FOREIGN KEY (pool_id) REFERENCES agent_pools(pool_id) ON DELETE SET NULL;

ALTER TABLE workspaces 
ADD CONSTRAINT workspaces_agent_pool_id_fkey 
FOREIGN KEY (agent_pool_id) REFERENCES agent_pools(pool_id) ON DELETE SET NULL;

-- Step 9: Drop old id column and its sequence
ALTER TABLE agent_pools 
DROP CONSTRAINT IF EXISTS agent_pools_pkey CASCADE;

ALTER TABLE agent_pools 
DROP COLUMN id;

DROP SEQUENCE IF EXISTS agent_pools_id_seq;

-- Step 10: Set pool_id as primary key
ALTER TABLE agent_pools 
ADD PRIMARY KEY (pool_id);

-- Step 11: Add missing fields for agent pool management
ALTER TABLE agent_pools 
ADD COLUMN IF NOT EXISTS organization_id VARCHAR(50) REFERENCES organizations(org_id) ON DELETE CASCADE;

ALTER TABLE agent_pools 
ADD COLUMN IF NOT EXISTS is_shared BOOLEAN DEFAULT false;

ALTER TABLE agent_pools 
ADD COLUMN IF NOT EXISTS max_agents INTEGER DEFAULT 10;

ALTER TABLE agent_pools 
ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'active';

-- Step 12: Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_agent_pools_organization ON agent_pools(organization_id);
CREATE INDEX IF NOT EXISTS idx_agent_pools_status ON agent_pools(status);
CREATE INDEX IF NOT EXISTS idx_agent_pools_pool_type ON agent_pools(pool_type);

-- Step 13: Add comments for documentation
COMMENT ON TABLE agent_pools IS 'Agent pools for managing agent instances';
COMMENT ON COLUMN agent_pools.pool_id IS 'Semantic pool ID format: pool-{16位随机a-z0-9}';
COMMENT ON COLUMN agent_pools.name IS 'Pool name for display';
COMMENT ON COLUMN agent_pools.description IS 'Pool description';
COMMENT ON COLUMN agent_pools.pool_type IS 'Pool type: static, k8s, etc.';
COMMENT ON COLUMN agent_pools.k8s_config IS 'Kubernetes configuration for k8s pool type';
COMMENT ON COLUMN agent_pools.organization_id IS 'Organization that owns this pool';
COMMENT ON COLUMN agent_pools.is_shared IS 'Whether this pool is shared across organizations';
COMMENT ON COLUMN agent_pools.max_agents IS 'Maximum number of agents allowed in this pool';
COMMENT ON COLUMN agent_pools.status IS 'Pool status: active, inactive, maintenance';
COMMENT ON COLUMN agent_pools.created_by IS 'User ID who created this pool';
COMMENT ON COLUMN agent_pools.created_at IS 'Creation timestamp';
COMMENT ON COLUMN agent_pools.updated_at IS 'Last update timestamp';

-- Verification queries
SELECT 'Agent Pools Table Structure:' as info;
SELECT column_name, data_type, character_maximum_length, is_nullable 
FROM information_schema.columns 
WHERE table_name = 'agent_pools' 
ORDER BY ordinal_position;

SELECT 'Agent Pools Data:' as info;
SELECT * FROM agent_pools;

SELECT 'Foreign Key Constraints:' as info;
SELECT tc.table_name, kcu.column_name, ccu.table_name AS foreign_table_name, ccu.column_name AS foreign_column_name
FROM information_schema.table_constraints AS tc 
JOIN information_schema.key_column_usage AS kcu ON tc.constraint_name = kcu.constraint_name
JOIN information_schema.constraint_column_usage AS ccu ON ccu.constraint_name = tc.constraint_name
WHERE tc.constraint_type = 'FOREIGN KEY' 
AND (tc.table_name IN ('agents', 'workspaces') AND ccu.table_name = 'agent_pools'
     OR tc.table_name = 'agent_pools');
