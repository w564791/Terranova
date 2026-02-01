import api from './api';

// TypeScript 接口定义
export interface ModuleDemo {
  id: number;
  module_id: number;
  name: string;
  description: string;
  current_version_id?: number;
  is_active: boolean;
  usage_notes: string;
  created_by?: number;
  created_at: string;
  updated_at: string;
  current_version?: ModuleDemoVersion;
  creator?: {
    id: number;
    username: string;
  };
}

export interface ModuleDemoVersion {
  id: number;
  demo_id: number;
  version: number;
  is_latest: boolean;
  config_data: Record<string, any>;
  change_summary: string;
  change_type: 'create' | 'update' | 'rollback';
  diff_from_previous: string;
  created_by?: number;
  created_at: string;
  creator?: {
    id: number;
    username: string;
  };
}

export interface CreateDemoRequest {
  name: string;
  description: string;
  usage_notes?: string;
  config_data: Record<string, any>;
  version_id?: string;  // 关联的模块版本 ID
}

export interface UpdateDemoRequest {
  name?: string;
  description?: string;
  usage_notes?: string;
  config_data: Record<string, any>;
  change_summary?: string;
}

export interface CompareVersionsResponse {
  version1: ModuleDemoVersion;
  version2: ModuleDemoVersion;
  diff: string;
  has_changes: boolean;
}

// API 函数
export const moduleDemoService = {
  // 获取模块的所有 Demo
  // versionId 可选，不传则返回默认版本的 Demo
  getDemosByModuleId: async (moduleId: number, versionId?: string): Promise<ModuleDemo[]> => {
    console.log('🔍 Fetching demos for module:', moduleId, 'version:', versionId);
    const params = versionId ? { version_id: versionId } : {};
    const response = await api.get(`/modules/${moduleId}/demos`, { params });
    console.log('🔍 API response:', response);
    
    // 处理不同的响应格式
    let demos: ModuleDemo[] = [];
    const data = response as any;
    
    if (Array.isArray(data)) {
      demos = data;
    } else if (data?.data && Array.isArray(data.data)) {
      demos = data.data;
    } else if (data?.items && Array.isArray(data.items)) {
      demos = data.items;
    } else if (data?.demos && Array.isArray(data.demos)) {
      demos = data.demos;
    }
    
    console.log('🔍 Parsed demos:', demos);
    return demos;
  },

  // 创建新 Demo
  createDemo: async (moduleId: number, data: CreateDemoRequest): Promise<ModuleDemo> => {
    const result = await api.post(`/modules/${moduleId}/demos`, data);
    return result as any;
  },

  // 获取 Demo 详情
  getDemoById: async (demoId: number): Promise<ModuleDemo> => {
    const data = await api.get(`/demos/${demoId}`);
    return data as any;
  },

  // 更新 Demo（创建新版本）
  updateDemo: async (demoId: number, data: UpdateDemoRequest): Promise<ModuleDemo> => {
    const result = await api.put(`/demos/${demoId}`, data);
    return result as any;
  },

  // 删除 Demo
  deleteDemo: async (demoId: number): Promise<void> => {
    await api.delete(`/demos/${demoId}`);
  },

  // 获取版本历史
  getVersions: async (demoId: number): Promise<ModuleDemoVersion[]> => {
    const data = await api.get(`/demos/${demoId}/versions`);
    return (data as any) || [];
  },

  // 获取特定版本详情
  getVersionById: async (versionId: number): Promise<ModuleDemoVersion> => {
    const data = await api.get(`/demo-versions/${versionId}`);
    return data as any;
  },

  // 对比两个版本
  compareVersions: async (
    demoId: number,
    version1Id: number,
    version2Id: number
  ): Promise<CompareVersionsResponse> => {
    const data = await api.get(`/demos/${demoId}/compare`, {
      params: {
        version1: version1Id,
        version2: version2Id,
      },
    });
    return data as any;
  },

  // 回滚到指定版本
  rollbackToVersion: async (demoId: number, versionId: number): Promise<ModuleDemo> => {
    const data = await api.post(`/demos/${demoId}/rollback`, {
      version_id: versionId,
    });
    return data as any;
  },
};

export default moduleDemoService;
