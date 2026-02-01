import api from './api';

// AI 助手提示词
export interface AIPrompt {
  id: string;
  title: string;
  prompt: string;
  created_at: string;
}

export interface Module {
  id: number;
  name: string;
  description: string;
  version: string;
  provider: string;
  source: string;
  module_source?: string;
  branch?: string;
  status: 'active' | 'inactive';
  ai_prompts?: AIPrompt[];
  created_at: string;
  updated_at: string;
}

export interface CreateModuleRequest {
  name: string;
  description: string;
  provider: string;
  source?: string;
  module_source?: string;
  repository_url: string;
  branch?: string;
}

export const moduleService = {
  // 获取模块列表
  getModules: async (): Promise<{ data: Module[] }> => {
    return api.get('/modules');
  },

  // 获取单个模块
  getModule: async (id: number): Promise<{ data: Module }> => {
    return api.get(`/modules/${id}`);
  },

  // 创建模块
  createModule: async (data: CreateModuleRequest): Promise<{ data: Module }> => {
    return api.post('/modules', data);
  },

  // 更新模块
  updateModule: async (id: number, data: Partial<CreateModuleRequest>): Promise<{ data: Module }> => {
    return api.put(`/modules/${id}`, data);
  },

  // 删除模块
  deleteModule: async (id: number): Promise<void> => {
    return api.delete(`/modules/${id}`);
  },

  // 同步模块
  syncModule: async (id: number): Promise<{ message: string }> => {
    return api.post(`/modules/${id}/sync`);
  },

  // 获取模块的 AI 提示词
  getModulePrompts: async (id: number): Promise<{ data: { items: AIPrompt[]; total: number } }> => {
    return api.get(`/modules/${id}/prompts`);
  },
};
