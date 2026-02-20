-- Migration: Agents and Agent Pools to Semantic ID
-- Purpose: Migrate both agents and agent_pools tables to use semantic string IDs
-- Agent ID Format: agent-{16位随机a-z0-9}
-- Pool ID Format: pool-{16位随机a-z0-9}
-- Date: 2025-10-29

-- ============================================
-- PART 1: Migrate agent_pools table
-- ============================================

-- Step 1.1: Add new pool_id column to agent_pools
ALTER TABLE agent_pools 
ADD COLUMN pool_id VARCHAR(50);

-- Step 1.2: Generate semantic pool_id for existing record
UPDATE agent_pools 
SET pool_id = 'pool-default00000001'
WHERE id = 1;

-- Step 1.3: Make pool_id NOT NULL
ALTER TABLE agent_pools 
ALTER COLUMN pool_id SET NOT NULL;

-- Step 1.4: Add unique constraint on pool_id
ALTER TABLE agent_pools 
ADD CONSTRAINT agent_pools_pool_id_unique UNIQUE (pool_id);

-- Step 1.5: Drop foreign key from agents table (will recreate later)
ALTER TABLE agents 
DROP CONSTRAINT IF EXISTS agents_pool_id_fkey;

-- Step 1.6: Drop foreign key from workspaces table
ALTER TABLE workspaces 
DROP CONSTRAINT IF EXISTS workspaces_agent_pool_id_fkey;

-- Step 1.7: Modify agents.pool_id to VARCHAR(50)
ALTER TABLE agents 
ALTER COLUMN pool_id TYPE VARCHAR(50);

-- Step 1.8: Modify workspaces.agent_pool_id to VARCHAR(50)
ALTER TABLE workspaces 
ALTER COLUMN agent_pool_id TYPE VARCHAR(50);

-- Step 1.9: Drop old id column from agent_pools
ALTER TABLE agent_pools 
DROP CONSTRAINT IF EXISTS agent_pools_pkey CASCADE;

ALTER TABLE agent_pools 
DROP COLUMN id;

DROP SEQUENCE IF EXISTS agent_pools_id_seq;

-- Step 1.10: Set pool_id as primary key
ALTER TABLE agent_pools 
ADD PRIMARY KEY (pool_id);

-- Step 1.11: Add missing fields for agent pool management
ALTER TABLE agent_pools 
ADD COLUMN IF NOT EXISTS organization_id VARCHAR(50) REFERENCES organizations(org_id) ON DELETE CASCADE,
ADD COLUMN IF NOT EXISTS is_shared BOOLEAN DEFAULT false,
ADD COLUMN IF NOT EXISTS max_agents INTEGER DEFAULT 10,
ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'active';

-- Step 1.12: Ensure created_by and updated_by are VARCHAR(50)
DO $$ 
BEGIN
    -- Check and convert created_by if needed
    IF EXISTS (SELECT 1 FROM information_schema.columns 
               WHERE table_name = 'agent_pools' AND column_name = 'created_by' 
               AND data_type != 'character varying') THEN
        ALTER TABLE agent_pools ALTER COLUMN created_by TYPE VARCHAR(50);
    END IF;
    
    -- Add created_by if it doesn't exist
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name = 'agent_pools' AND column_name = 'created_by') THEN
        ALTER TABLE agent_pools ADD COLUMN created_by VARCHAR(50);
    END IF;
    
    -- Add updated_by if it doesn't exist
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name = 'agent_pools' AND column_name = 'updated_by') THEN
        ALTER TABLE agent_pools ADD COLUMN updated_by VARCHAR(50);
    END IF;
END $$;

-- Step 1.13: Create indexes for agent_pools
CREATE INDEX IF NOT EXISTS idx_agent_pools_organization ON agent_pools(organization_id);
CREATE INDEX IF NOT EXISTS idx_agent_pools_status ON agent_pools(status);
CREATE INDEX IF NOT EXISTS idx_agent_pools_pool_type ON agent_pools(pool_type);

-- ============================================
-- PART 2: Migrate agents table
-- ============================================

-- Step 2.1: Add new agent_id column to agents
ALTER TABLE agents 
ADD COLUMN agent_id VARCHAR(50);

-- Step 2.2: Since there's no existing data, we can skip data migration
-- If there were data, we would generate semantic IDs here

-- Step 2.3: Make agent_id NOT NULL
ALTER TABLE agents 
ALTER COLUMN agent_id SET NOT NULL;

-- Step 2.4: Add unique constraint on agent_id
ALTER TABLE agents 
ADD CONSTRAINT agents_agent_id_unique UNIQUE (agent_id);

-- Step 2.5: Drop old id column from agents
ALTER TABLE agents 
DROP CONSTRAINT IF EXISTS agents_pkey CASCADE;

ALTER TABLE agents 
DROP COLUMN id;

DROP SEQUENCE IF EXISTS agents_id_seq;

-- Step 2.6: Set agent_id as primary key
ALTER TABLE agents 
ADD PRIMARY KEY (agent_id);

-- Step 2.7: Add missing fields for agent management (based on documentation)
ALTER TABLE agents 
ADD COLUMN IF NOT EXISTS application_id INTEGER REFERENCES applications(id) ON DELETE CASCADE,
ADD COLUMN IF NOT EXISTS ip_address VARCHAR(50),
ADD COLUMN IF NOT EXISTS version VARCHAR(50),
ADD COLUMN IF NOT EXISTS last_ping_at TIMESTAMP;

-- Rename last_heartbeat to last_ping_at if it exists
DO $$ 
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns 
               WHERE table_name = 'agents' AND column_name = 'last_heartbeat') THEN
        ALTER TABLE agents RENAME COLUMN last_heartbeat TO last_ping_at;
    END IF;
END $$;

-- Step 2.8: Add registered_at timestamp
ALTER TABLE agents 
ADD COLUMN IF NOT EXISTS registered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;

-- Step 2.9: Ensure created_by and updated_by are VARCHAR(50)
DO $$ 
BEGIN
    -- Add created_by if it doesn't exist
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name = 'agents' AND column_name = 'created_by') THEN
        ALTER TABLE agents ADD COLUMN created_by VARCHAR(50);
    END IF;
    
    -- Add updated_by if it doesn't exist
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name = 'agents' AND column_name = 'updated_by') THEN
        ALTER TABLE agents ADD COLUMN updated_by VARCHAR(50);
    END IF;
END $$;

-- Step 2.10: Create indexes for agents
CREATE INDEX IF NOT EXISTS idx_agents_pool ON agents(pool_id);
CREATE INDEX IF NOT EXISTS idx_agents_application ON agents(application_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_last_ping ON agents(last_ping_at);

-- ============================================
-- PART 3: Recreate foreign key constraints
-- ============================================

-- Step 3.1: Add foreign key from agents to agent_pools
ALTER TABLE agents 
ADD CONSTRAINT agents_pool_id_fkey 
FOREIGN KEY (pool_id) REFERENCES agent_pools(pool_id) ON DELETE SET NULL;

-- Step 3.2: Add foreign key from workspaces to agent_pools
ALTER TABLE workspaces 
ADD CONSTRAINT workspaces_agent_pool_id_fkey 
FOREIGN KEY (agent_pool_id) REFERENCES agent_pools(pool_id) ON DELETE SET NULL;

-- ============================================
-- PART 4: Add table and column comments
-- ============================================

-- Agent Pools comments
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
COMMENT ON COLUMN agent_pools.updated_by IS 'User ID who last updated this pool';

-- Agents comments
COMMENT ON TABLE agents IS 'Agent instances registered in the system';
COMMENT ON COLUMN agents.agent_id IS 'Semantic agent ID format: agent-{16位随机a-z0-9}';
COMMENT ON COLUMN agents.application_id IS 'Application that owns this agent';
COMMENT ON COLUMN agents.pool_id IS 'Agent pool this agent belongs to';
COMMENT ON COLUMN agents.name IS 'Agent name for identification';
COMMENT ON COLUMN agents.token_hash IS 'Hashed authentication token';
COMMENT ON COLUMN agents.status IS 'Agent status: idle, busy, offline';
COMMENT ON COLUMN agents.ip_address IS 'Agent IP address';
COMMENT ON COLUMN agents.version IS 'Agent version';
COMMENT ON COLUMN agents.last_ping_at IS 'Last heartbeat timestamp';
COMMENT ON COLUMN agents.capabilities IS 'Agent capabilities in JSON format';
COMMENT ON COLUMN agents.metadata IS 'Additional metadata in JSON format';
COMMENT ON COLUMN agents.registered_at IS 'Agent registration timestamp';
COMMENT ON COLUMN agents.created_by IS 'User ID who created this agent record';
COMMENT ON COLUMN agents.updated_by IS 'User ID who last updated this agent record';

-- ============================================
-- PART 5: Verification queries
-- ============================================

SELECT '=== Agent Pools Table Structure ===' as info;
SELECT column_name, data_type, character_maximum_length, is_nullable 
FROM information_schema.columns 
WHERE table_name = 'agent_pools' 
ORDER BY ordinal_position;

SELECT '=== Agent Pools Data ===' as info;
SELECT * FROM agent_pools;

SELECT '=== Agents Table Structure ===' as info;
SELECT column_name, data_type, character_maximum_length, is_nullable 
FROM information_schema.columns 
WHERE table_name = 'agents' 
ORDER BY ordinal_position;

SELECT '=== Agents Data ===' as info;
SELECT * FROM agents;

SELECT '=== Foreign Key Constraints ===' as info;
SELECT 
    tc.table_name, 
    kcu.column_name, 
    ccu.table_name AS foreign_table_name, 
    ccu.column_name AS foreign_column_name
FROM information_schema.table_constraints AS tc 
JOIN information_schema.key_column_usage AS kcu 
    ON tc.constraint_name = kcu.constraint_name
JOIN information_schema.constraint_column_usage AS ccu 
    ON ccu.constraint_name = tc.constraint_name
WHERE tc.constraint_type = 'FOREIGN KEY' 
AND (tc.table_name IN ('agents', 'workspaces', 'agent_pools'))
ORDER BY tc.table_name, kcu.column_name;
