import api from './api';

export interface SSOProvider {
  provider_key: string;
  provider_type: string;
  display_name: string;
  description: string;
  icon: string;
  display_order: number;
}

export interface SSOLoginResponse {
  auth_url: string;
}

export interface SSOCallbackResponse {
  token: string;
  expires_at: string;
  is_new_user: boolean;
  user: {
    id: string;
    username: string;
    email: string;
    role: string;
  };
}

export interface SSOIdentity {
  id: number;
  user_id: string;
  provider: string;
  provider_user_id: string;
  provider_email: string;
  provider_name: string;
  provider_avatar: string;
  is_primary: boolean;
  is_verified: boolean;
  last_used_at: string;
  created_at: string;
}

export const ssoService = {
  // 获取可用的 SSO Provider 列表
  getProviders: () => api.get<{ code: number; data: SSOProvider[] }>('/auth/sso/providers'),

  // 发起 SSO 登录（获取授权 URL）
  login: (providerKey: string, redirectUrl?: string) =>
    api.get<{ code: number; data: SSOLoginResponse }>(
      `/auth/sso/${providerKey}/login`,
      { params: { redirect_url: redirectUrl || '/' } }
    ),

  // 处理 SSO 回调（用 code 和 state 换取 token）
  callback: (providerKey: string, code: string, state: string) =>
    api.get<{ code: number; data: SSOCallbackResponse }>(
      `/auth/sso/${providerKey}/callback`,
      { params: { code, state } }
    ),

  // 获取当前用户绑定的身份列表
  getIdentities: () => api.get<{ code: number; data: SSOIdentity[] }>('/auth/sso/identities'),

  // 绑定新的 SSO 身份
  linkIdentity: (providerKey: string, redirectUrl?: string) =>
    api.post<{ code: number; data: SSOLoginResponse }>('/auth/sso/identities/link', {
      provider_key: providerKey,
      redirect_url: redirectUrl || '/settings',
    }),

  // 解绑 SSO 身份
  unlinkIdentity: (identityId: number) =>
    api.delete(`/auth/sso/identities/${identityId}`),

  // 设置主要登录方式
  setPrimaryIdentity: (identityId: number) =>
    api.put(`/auth/sso/identities/${identityId}/primary`),
};

// ============================================
// 管理员 SSO API
// ============================================

export interface SSOProviderConfig {
  id?: number;
  provider_key: string;
  provider_type: string;
  display_name: string;
  description?: string;
  icon?: string;
  oauth_config: string; // JSON string
  authorize_endpoint?: string;
  token_endpoint?: string;
  userinfo_endpoint?: string;
  callback_url: string;
  auto_create_user: boolean;
  default_role: string;
  allowed_domains?: string[];
  is_enabled: boolean;
  is_enterprise: boolean;
  organization_id?: string;
  display_order: number;
  show_on_login_page: boolean;
  created_by?: string;
  created_at?: string;
  updated_at?: string;
}

export interface SSOGlobalConfig {
  disable_local_login: boolean;
}

export interface SSOLoginLog {
  id: number;
  user_id: string;
  identity_id: number;
  provider_key: string;
  provider_user_id: string;
  provider_email: string;
  status: string;
  error_message: string;
  ip_address: string;
  user_agent: string;
  created_at: string;
}

export const ssoAdminService = {
  // Provider 管理
  getProviders: () => api.get('/admin/sso/providers'),
  getProvider: (id: number) => api.get(`/admin/sso/providers/${id}`),
  createProvider: (data: Partial<SSOProviderConfig>) => api.post('/admin/sso/providers', data),
  updateProvider: (id: number, data: Partial<SSOProviderConfig>) => api.put(`/admin/sso/providers/${id}`, data),
  deleteProvider: (id: number) => api.delete(`/admin/sso/providers/${id}`),

  // 全局配置
  getConfig: () => api.get<{ code: number; data: SSOGlobalConfig }>('/admin/sso/config'),
  updateConfig: (data: SSOGlobalConfig) => api.put('/admin/sso/config', data),

  // 登录日志
  getLogs: (page: number, pageSize: number, providerKey?: string) =>
    api.get('/admin/sso/logs', { params: { page, page_size: pageSize, provider_key: providerKey } }),
};
