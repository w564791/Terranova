import api from './api';

// 资源类型
export type ResourceType = 'agent_pool' | 'workspace' | 'module' | 'system' | 'team' | 'user';

// 密文类型
export type SecretType = 'hcp';

// Secret接口定义
export interface Secret {
  secret_id: string;
  secret_type: SecretType;
  resource_type: ResourceType;
  resource_id?: string;
  key: string;
  description?: string;
  tags?: string[];
  created_by?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
  last_used_at?: string;
  expires_at?: string;
  is_active: boolean;
}

// 创建Secret请求
export interface CreateSecretRequest {
  key: string;
  value: string;
  secret_type?: SecretType;
  description?: string;
  tags?: string[];
  expires_at?: string;
}

// 创建Secret响应（仅此时包含value）
export interface CreateSecretResponse extends Secret {
  value: string; // 仅创建时返回
}

// 更新Secret请求
export interface UpdateSecretRequest {
  value?: string;
  description?: string;
  tags?: string[];
}

// Secret列表响应
export interface SecretListResponse {
  secrets: Secret[];
  total: number;
}

// Secrets API
export const secretsAPI = {
  /**
   * 创建密文
   * @param resourceType 资源类型
   * @param resourceId 资源ID
   * @param data 创建请求数据
   * @returns 创建响应（包含明文value，仅此一次）
   */
  create: async (
    resourceType: ResourceType,
    resourceId: string,
    data: CreateSecretRequest
  ): Promise<CreateSecretResponse> => {
    const response = await api.post(`/${resourceType}/${resourceId}/secrets`, data);
    return response.data;
  },

  /**
   * 列出密文
   * @param resourceType 资源类型
   * @param resourceId 资源ID
   * @param isActive 是否仅显示激活的密文
   * @returns 密文列表
   */
  list: async (
    resourceType: ResourceType,
    resourceId: string,
    isActive?: boolean
  ): Promise<SecretListResponse> => {
    const params = isActive !== undefined ? { is_active: isActive } : {};
    const response = await api.get(`/${resourceType}/${resourceId}/secrets`, { params });
    // 处理响应：可能是 response.data 或直接是 response
    const data = response.data || response;
    return data;
  },

  /**
   * 获取密文详情
   * @param resourceType 资源类型
   * @param resourceId 资源ID
   * @param secretId 密文ID
   * @returns 密文详情（不包含value）
   */
  get: async (
    resourceType: ResourceType,
    resourceId: string,
    secretId: string
  ): Promise<Secret> => {
    const response = await api.get(`/${resourceType}/${resourceId}/secrets/${secretId}`);
    return response.data;
  },

  /**
   * 更新密文
   * @param resourceType 资源类型
   * @param resourceId 资源ID
   * @param secretId 密文ID
   * @param data 更新数据
   * @returns 更新后的密文
   */
  update: async (
    resourceType: ResourceType,
    resourceId: string,
    secretId: string,
    data: UpdateSecretRequest
  ): Promise<Secret> => {
    const response = await api.put(`/${resourceType}/${resourceId}/secrets/${secretId}`, data);
    return response.data;
  },

  /**
   * 删除密文
   * @param resourceType 资源类型
   * @param resourceId 资源ID
   * @param secretId 密文ID
   */
  delete: async (
    resourceType: ResourceType,
    resourceId: string,
    secretId: string
  ): Promise<void> => {
    await api.delete(`/${resourceType}/${resourceId}/secrets/${secretId}`);
  },
};

export default secretsAPI;
