import api from './api';

export interface VariableSet {
  id: number;
  varset_id: string;
  name: string;
  description: string;
  scope: 'global' | 'specific';
  variable_count?: number;
  assignment_count?: number;
  created_at: string;
  updated_at: string;
  created_by: string | null;
}

export interface VarsetVariable {
  id: number;
  variable_id: string;
  varset_id: string;
  key: string;
  value: string;
  variable_type: 'terraform' | 'environment';
  value_format: 'string' | 'hcl';
  sensitive: boolean;
  description: string;
  created_at: string;
  updated_at: string;
}

export interface VarsetAssignment {
  id: number;
  varset_id: string;
  scope_type: 'project' | 'workspace';
  project_id: number | null;
  workspace_id: string | null;
  attached_at: string;
  attached_by: string | null;
}

export interface EffectiveVariable {
  variable_id: string;
  key: string;
  value: string;
  variable_type: 'terraform' | 'environment';
  value_format: 'string' | 'hcl';
  sensitive: boolean;
  description: string;
  source_type: 'workspace' | 'varset';
  source_id: string;
  source_name: string;
  scope_level: 'global' | 'project' | 'workspace-specific' | 'workspace';
  is_overridden: boolean;
  overridden_by?: {
    variable_id: string;
    source_type: string;
    source_id: string;
  };
}

export const variableSetService = {
  list: async (scope?: string): Promise<{ items: VariableSet[] }> => {
    const params = scope ? { scope } : undefined;
    const response = await api.get('/variable-sets', { params });
    return response as any;
  },

  get: async (varsetId: string): Promise<VariableSet> => {
    const response = await api.get(`/variable-sets/${varsetId}`);
    return response as any;
  },

  create: async (data: Partial<VariableSet>): Promise<VariableSet> => {
    const response = await api.post('/variable-sets', data);
    return response as any;
  },

  update: async (varsetId: string, data: Partial<VariableSet>): Promise<VariableSet> => {
    const response = await api.put(`/variable-sets/${varsetId}`, data);
    return response as any;
  },

  updateScope: async (varsetId: string, scope: 'global' | 'specific'): Promise<VariableSet> => {
    const response = await api.put(`/variable-sets/${varsetId}/scope`, { scope });
    return response as any;
  },

  delete: async (varsetId: string): Promise<void> => {
    await api.delete(`/variable-sets/${varsetId}`);
  },

  listVariables: async (varsetId: string, type?: string): Promise<VarsetVariable[]> => {
    const params = type ? { type } : undefined;
    const response = await api.get(`/variable-sets/${varsetId}/variables`, { params });
    return response as any;
  },

  createVariable: async (varsetId: string, data: Partial<VarsetVariable>): Promise<VarsetVariable> => {
    const response = await api.post(`/variable-sets/${varsetId}/variables`, data);
    return response as any;
  },

  updateVariable: async (varsetId: string, varId: string, data: Partial<VarsetVariable>): Promise<VarsetVariable> => {
    const response = await api.put(`/variable-sets/${varsetId}/variables/${varId}`, data);
    return response as any;
  },

  deleteVariable: async (varsetId: string, varId: string): Promise<void> => {
    await api.delete(`/variable-sets/${varsetId}/variables/${varId}`);
  },

  listAssignments: async (varsetId: string): Promise<{ items: VarsetAssignment[] }> => {
    const response = await api.get(`/variable-sets/${varsetId}/assignments`);
    return response as any;
  },

  createAssignment: async (varsetId: string, data: Partial<VarsetAssignment>): Promise<VarsetAssignment> => {
    const response = await api.post(`/variable-sets/${varsetId}/assignments`, data);
    return response as any;
  },

  deleteAssignment: async (varsetId: string, assignmentId: number): Promise<void> => {
    await api.delete(`/variable-sets/${varsetId}/assignments/${assignmentId}`);
  },

  getEffectiveVariables: async (workspaceId: string): Promise<EffectiveVariable[]> => {
    const response = await api.get(`/workspaces/${workspaceId}/effective-variables`);
    return response as any;
  },
};

export default variableSetService;
