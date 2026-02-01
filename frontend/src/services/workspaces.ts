import api from './api';

export interface Workspace {
  id: number;
  workspace_id?: string;
  name: string;
  description: string;
  execution_mode: 'local' | 'agent' | 'k8s';
  agent_pool_id?: number;
  k8s_config_id?: number;
  current_pool_id?: string;
  auto_apply: boolean;
  plan_only: boolean;
  terraform_version: string;
  workdir: string;
  state_backend: string;
  state_config?: Record<string, any>;
  tags?: Record<string, any>;
  variables?: Record<string, any>;
  provider_config?: Record<string, any>;
  notify_settings?: Record<string, any>;
  state: string;
  is_locked: boolean;
  locked_by?: number;
  locked_by_username?: string;
  locked_at?: string;
  lock_reason?: string;
  ui_mode?: 'console' | 'structured';
  show_unchanged_resources?: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateWorkspaceRequest {
  name: string;
  description?: string;
  execution_mode: 'local' | 'agent' | 'k8s';
  agent_pool_id?: number;
  k8s_config_id?: number;
  auto_apply?: boolean;
  plan_only?: boolean;
  terraform_version?: string;
  workdir?: string;
  state_backend: string;
  state_config?: Record<string, any>;
  tags?: Record<string, any>;
  variables?: Record<string, any>;
  provider_config?: Record<string, any>;
  notify_settings?: Record<string, any>;
  ui_mode?: 'console' | 'structured';
  show_unchanged_resources?: boolean;
}

export interface ExecutionModeOption {
  value: string;
  label: string;
  description: string;
}

export interface StateBackendOption {
  value: string;
  label: string;
  description: string;
}

export interface AgentPool {
  id: number;
  name: string;
  description?: string;
}

export interface K8sConfig {
  id: number;
  name: string;
  description?: string;
}

export interface WorkspaceFormData {
  terraform_versions: string[];
  agent_pools: AgentPool[];
  k8s_configs: K8sConfig[];
  execution_modes: ExecutionModeOption[];
  state_backends: StateBackendOption[];
}

export const workspaceService = {
  // 获取工作空间列表
  getWorkspaces: async (): Promise<{ data: Workspace[] }> => {
    return api.get('/workspaces');
  },

  // 获取单个工作空间
  getWorkspace: async (id: string | number): Promise<{ data: Workspace }> => {
    return api.get(`/workspaces/${id}`);
  },

  // 创建工作空间
  createWorkspace: async (data: CreateWorkspaceRequest): Promise<{ data: Workspace }> => {
    return api.post('/workspaces', data);
  },

  // 更新工作空间
  updateWorkspace: async (id: string | number, data: Partial<CreateWorkspaceRequest>): Promise<{ data: Workspace }> => {
    return api.put(`/workspaces/${id}`, data);
  },

  // 删除工作空间
  deleteWorkspace: async (id: string | number): Promise<void> => {
    return api.delete(`/workspaces/${id}`);
  },

  // 获取表单数据
  getFormData: async (): Promise<{ data: WorkspaceFormData }> => {
    return api.get('/workspaces/form-data');
  },

  // 锁定工作空间
  lockWorkspace: async (id: string | number, reason: string): Promise<void> => {
    return api.post(`/workspaces/${id}/lock`, { reason });
  },

  // 解锁工作空间
  unlockWorkspace: async (id: string | number): Promise<void> => {
    return api.post(`/workspaces/${id}/unlock`);
  },
};
