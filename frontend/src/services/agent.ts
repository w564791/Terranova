import api from './api';

// ===== Types =====

export interface Agent {
  agent_id: string;
  application_id: number;
  pool_id?: string;
  name: string;
  status: 'idle' | 'busy' | 'offline';
  ip_address?: string;
  version?: string;
  last_ping_at?: string;
  registered_at: string;
  created_at: string;
  updated_at: string;
}

export interface AgentPool {
  pool_id: string;
  name: string;
  description?: string;
  pool_type: 'static' | 'k8s';
  k8s_config?: string;
  one_time_unfreeze_until?: string;
  one_time_unfreeze_by?: string;
  one_time_unfreeze_at?: string;
  organization_id?: string;
  is_shared: boolean;
  max_agents: number;
  status: 'active' | 'inactive' | 'maintenance';
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface AgentPoolWithCount extends AgentPool {
  agent_count: number;
}

export interface AgentAllowedWorkspace {
  id: number;
  agent_id: string;
  workspace_id: string;
  status: 'active' | 'revoked';
  allowed_by?: string;
  allowed_at: string;
  revoked_at?: string;
}

export interface WorkspaceAllowedAgent {
  id: number;
  workspace_id: string;
  agent_id: string;
  status: 'active' | 'revoked';
  is_current: boolean;
  allowed_by: string;
  allowed_at: string;
  activated_at?: string;
  revoked_at?: string;
}

export interface AgentWithAllowance extends Agent {
  allowed_at: string;
  allowance_status: string;
}

export interface CurrentAgentResponse {
  workspace_id: string;
  agent: Agent;
  set_at: string;
  is_online: boolean;
}

export interface PoolAllowedWorkspace {
  id: number;
  pool_id: string;
  workspace_id: string;
  workspace_name?: string;
  status: string;
  allowed_at: string;
  allowed_by?: string;
  allowed_by_name?: string;
  revoked_at?: string;
  revoked_by?: string;
}

export interface PoolWithAgentCount {
  pool_id: string;
  name: string;
  description?: string;
  pool_type: 'static' | 'k8s';
  allowed_at: string;
  agent_count: number;
  online_count: number;
}

export interface CurrentPoolResponse {
  workspace_id: string;
  pool: {
    pool_id: string;
    name: string;
    description?: string;
    pool_type: 'static' | 'k8s';
    agent_count: number;
    online_count: number;
  };
}

// ===== Agent Pool APIs =====

export const agentPoolAPI = {
  // List all agent pools
  list: async (params?: { status?: string }): Promise<{ pools: AgentPoolWithCount[]; total: number }> => {
    const response = await api.get('/agent-pools', { params });
    console.log('API response:', response);
    console.log('API response.data:', response.data);
    // If response.data is undefined, the interceptor already unwrapped it
    return response.data || response;
  },

  // Get agent pool details
  get: async (poolId: string, includeOffline: boolean = false): Promise<{ pool: AgentPool; agents: Agent[]; total: number }> => {
    const params = includeOffline ? { include_offline: 'true' } : {};
    const response = await api.get(`/agent-pools/${poolId}`, { params });
    return response.data || response;
  },

  // Create agent pool
  create: async (data: { name: string; description?: string; pool_type: 'static' | 'k8s' }): Promise<AgentPool> => {
    const response = await api.post('/agent-pools', data);
    return response.data || response;
  },

  // Update agent pool
  update: async (poolId: string, data: { name?: string; description?: string; is_active?: boolean }): Promise<AgentPool> => {
    const response = await api.put(`/agent-pools/${poolId}`, data);
    return response.data || response;
  },

  // Delete agent pool
  delete: async (poolId: string): Promise<void> => {
    await api.delete(`/agent-pools/${poolId}`);
  },
};

// ===== Workspace Agent Authorization APIs =====

export const workspaceAgentAPI = {
  // Get available agents for workspace
  getAvailableAgents: async (workspaceId: string, params?: { status?: string }): Promise<{ workspace_id: string; agents: AgentWithAllowance[]; total: number }> => {
    const response = await api.get(`/workspaces/${workspaceId}/available-agents`, { params });
    return response.data;
  },

  // Allow agent to access workspace
  allowAgent: async (workspaceId: string, agentId: string): Promise<void> => {
    await api.post(`/workspaces/${workspaceId}/allow-agent`, { agent_id: agentId });
  },

  // Set current agent for workspace
  setCurrentAgent: async (workspaceId: string, agentId: string): Promise<void> => {
    await api.post(`/workspaces/${workspaceId}/set-current-agent`, { agent_id: agentId });
  },

  // Get current agent for workspace
  getCurrentAgent: async (workspaceId: string): Promise<CurrentAgentResponse> => {
    const response = await api.get(`/workspaces/${workspaceId}/current-agent`);
    return response.data;
  },

  // Revoke agent access from workspace
  revokeAgent: async (workspaceId: string, agentId: string): Promise<void> => {
    await api.delete(`/workspaces/${workspaceId}/allowed-agents/${agentId}`);
  },
};

// ===== Agent APIs (for admin/monitoring) =====

export const agentAPI = {
  // Note: These APIs require AppKey/AppSecret headers, typically used by agents themselves
  // For admin monitoring, we would need separate admin APIs

  // Get agent info (requires AppKey/AppSecret)
  get: async (agentId: string, headers: { 'X-App-Key': string; 'X-App-Secret': string }): Promise<Agent> => {
    const response = await api.get(`/agents/${agentId}`, { headers });
    return response.data;
  },

  // Get agent's allowed workspaces (requires AppKey/AppSecret)
  getAllowedWorkspaces: async (
    agentId: string,
    headers: { 'X-App-Key': string; 'X-App-Secret': string },
    params?: { status?: string }
  ): Promise<{ agent_id: string; workspaces: AgentAllowedWorkspace[]; total: number }> => {
    const response = await api.get(`/agents/${agentId}/allowed-workspaces`, { headers, params });
    return response.data;
  },
};

// ===== Pool Authorization APIs =====

export const poolAuthorizationAPI = {
  // Allow workspaces to access pool
  allowWorkspaces: async (poolId: string, workspaceIds: string[]): Promise<void> => {
    await api.post(`/agent-pools/${poolId}/allow-workspaces`, { workspace_ids: workspaceIds });
  },

  // Get pool's allowed workspaces
  getAllowedWorkspaces: async (poolId: string, params?: { status?: string }): Promise<{ pool_id: string; workspaces: PoolAllowedWorkspace[]; total: number }> => {
    const response = await api.get(`/agent-pools/${poolId}/allowed-workspaces`, { params });
    return response.data || response;
  },

  // Revoke workspace access from pool
  revokeWorkspace: async (poolId: string, workspaceId: string): Promise<void> => {
    await api.delete(`/agent-pools/${poolId}/allowed-workspaces/${workspaceId}`);
  },
};

// ===== Workspace Pool APIs =====

export const workspacePoolAPI = {
  // Get available pools for workspace
  getAvailablePools: async (workspaceId: string): Promise<{ workspace_id: string; pools: PoolWithAgentCount[]; total: number }> => {
    const response = await api.get(`/workspaces/${workspaceId}/available-pools`);
    return response.data || response;
  },

  // Set current pool for workspace
  setCurrentPool: async (workspaceId: string, poolId: string): Promise<void> => {
    await api.post(`/workspaces/${workspaceId}/set-current-pool`, { pool_id: poolId });
  },

  // Get current pool for workspace
  getCurrentPool: async (workspaceId: string): Promise<CurrentPoolResponse> => {
    const response = await api.get(`/workspaces/${workspaceId}/current-pool`);
    return response.data || response;
  },
};

// ===== Pool Token APIs =====

export interface PoolToken {
  token_name: string;
  pool_id: string;
  token_type: 'static' | 'k8s_temporary';
  is_active: boolean;
  created_by?: string;
  created_at: string;
  expires_at?: string;
  revoked_at?: string;
  revoked_by?: string;
  last_used_at?: string;
}

export interface PoolTokenCreateResponse {
  token_name: string;
  pool_id: string;
  token: string;
  token_type: string;
  expires_at?: string;
  created_at: string;
}

export const poolTokenAPI = {
  // Create pool token
  create: async (poolId: string, data: { token_name: string; expires_at?: string }): Promise<PoolTokenCreateResponse> => {
    const response = await api.post(`/agent-pools/${poolId}/tokens`, data);
    return response.data || response;
  },

  // List pool tokens
  list: async (poolId: string): Promise<{ tokens: PoolToken[]; total: number }> => {
    const response = await api.get(`/agent-pools/${poolId}/tokens`);
    return response.data || response;
  },

  // Revoke pool token
  revoke: async (poolId: string, tokenName: string): Promise<void> => {
    await api.delete(`/agent-pools/${poolId}/tokens/${tokenName}`);
  },

  // Activate one-time unfreeze (for K8s pools)
  activateOneTimeUnfreeze: async (poolId: string): Promise<{ message: string; unfreeze_until: string; unfreeze_by: string; unfreeze_activated: string }> => {
    const response = await api.post(`/agent-pools/${poolId}/one-time-unfreeze`);
    return response.data || response;
  },
};

export default {
  agentPool: agentPoolAPI,
  workspaceAgent: workspaceAgentAPI,
  agent: agentAPI,
  poolAuthorization: poolAuthorizationAPI,
  workspacePool: workspacePoolAPI,
  poolToken: poolTokenAPI,
};
