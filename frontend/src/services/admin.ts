import api from './api';

// IaC引擎类型（运行时推断，不存储在数据库）
export type IaCEngineType = 'terraform' | 'opentofu';

// IaC引擎版本
export interface TerraformVersion {
  id: number;
  version: string;
  download_url: string;
  checksum: string;
  enabled: boolean;
  deprecated: boolean;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

// 获取引擎显示名称
export const getEngineDisplayName = (engineType: IaCEngineType): string => {
  switch (engineType) {
    case 'opentofu':
      return 'OpenTofu';
    default:
      return 'Terraform';
  }
};

// 根据下载链接检测引擎类型（运行时推断）
export const detectEngineTypeFromURL = (downloadURL: string): IaCEngineType => {
  const url = downloadURL.toLowerCase();
  if (url.includes('opentofu') || url.includes('tofu_') || url.includes('/tofu/')) {
    return 'opentofu';
  }
  return 'terraform';
};

export interface CreateTerraformVersionRequest {
  version: string;
  download_url: string;
  checksum: string;
  enabled?: boolean;
  deprecated?: boolean;
}

export interface UpdateTerraformVersionRequest {
  download_url?: string;
  checksum?: string;
  enabled?: boolean;
  deprecated?: boolean;
}

export interface TerraformVersionsResponse {
  items: TerraformVersion[];
  total: number;
}

// Provider模板
export interface ProviderTemplate {
  id: number;
  name: string;
  type: string;
  source: string;
  alias: string;
  config: Record<string, any>;
  version: string;
  constraint_op: string;
  is_default: boolean;
  enabled: boolean;
  description: string;
  created_by: number | null;
  created_at: string;
  updated_at: string;
}

export interface CreateProviderTemplateRequest {
  name: string;
  type: string;
  source: string;
  alias?: string;
  config: Record<string, any>;
  version?: string;
  constraint_op?: string;
  enabled?: boolean;
  description?: string;
}

export interface UpdateProviderTemplateRequest {
  name?: string;
  type?: string;
  source?: string;
  alias?: string;
  config?: Record<string, any>;
  version?: string;
  constraint_op?: string;
  enabled?: boolean;
  description?: string;
}

export interface ProviderTemplatesResponse {
  items: ProviderTemplate[];
  total: number;
}

export const adminService = {
  // 获取所有版本
  getTerraformVersions: async (params?: {
    enabled?: boolean;
    deprecated?: boolean;
  }): Promise<TerraformVersionsResponse> => {
    const response = await api.get('/global/settings/terraform-versions', { params });
    // API直接返回数据，不需要.data
    return response.data || response;
  },

  // 获取单个版本
  getTerraformVersion: async (id: number): Promise<TerraformVersion> => {
    const response = await api.get(`/global/settings/terraform-versions/${id}`);
    return response.data;
  },

  // 创建版本
  createTerraformVersion: async (
    data: CreateTerraformVersionRequest
  ): Promise<TerraformVersion> => {
    const response = await api.post('/global/settings/terraform-versions', data);
    return response.data;
  },

  // 更新版本
  updateTerraformVersion: async (
    id: number,
    data: UpdateTerraformVersionRequest
  ): Promise<TerraformVersion> => {
    const response = await api.put(`/global/settings/terraform-versions/${id}`, data);
    return response.data;
  },

  // 删除版本
  deleteTerraformVersion: async (id: number): Promise<void> => {
    await api.delete(`/global/settings/terraform-versions/${id}`);
  },

  // 获取默认版本 ⭐ 新增
  getDefaultVersion: async (): Promise<TerraformVersion> => {
    const response = await api.get('/global/settings/terraform-versions/default');
    return response.data;
  },

  // 设置默认版本 ⭐ 新增
  setDefaultVersion: async (id: number): Promise<TerraformVersion> => {
    const response = await api.post(`/global/settings/terraform-versions/${id}/set-default`);
    return response.data;
  },

  // Provider模板 CRUD
  getProviderTemplates: async (params?: {
    enabled?: boolean;
    type?: string;
  }): Promise<ProviderTemplatesResponse> => {
    const response = await api.get('/global/settings/provider-templates', { params });
    return response.data || response;
  },

  getProviderTemplate: async (id: number): Promise<ProviderTemplate> => {
    const response = await api.get(`/global/settings/provider-templates/${id}`);
    return response.data;
  },

  createProviderTemplate: async (
    data: CreateProviderTemplateRequest
  ): Promise<ProviderTemplate> => {
    const response = await api.post('/global/settings/provider-templates', data);
    return response.data;
  },

  updateProviderTemplate: async (
    id: number,
    data: UpdateProviderTemplateRequest
  ): Promise<ProviderTemplate> => {
    const response = await api.put(`/global/settings/provider-templates/${id}`, data);
    return response.data;
  },

  deleteProviderTemplate: async (id: number): Promise<void> => {
    await api.delete(`/global/settings/provider-templates/${id}`);
  },

  setDefaultProviderTemplate: async (id: number): Promise<ProviderTemplate> => {
    const response = await api.post(`/global/settings/provider-templates/${id}/set-default`);
    return response.data;
  },
};
