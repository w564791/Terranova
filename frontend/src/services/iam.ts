import api from './api';

// ==================== 类型定义 ====================

// 权限等级
export type PermissionLevel = 'NONE' | 'READ' | 'WRITE' | 'ADMIN';

// 作用域类型
export type ScopeType = 'ORGANIZATION' | 'PROJECT' | 'WORKSPACE';

// 主体类型
export type PrincipalType = 'USER' | 'TEAM' | 'APPLICATION';

// 资源类型
export type ResourceType =
  | 'APPLICATION_REGISTRATION'
  | 'ORGANIZATION_SETTINGS'
  | 'USER_MANAGEMENT'
  | 'ALL_PROJECTS'
  | 'PROJECT_SETTINGS'
  | 'PROJECT_TEAM_MANAGEMENT'
  | 'PROJECT_WORKSPACES'
  | 'TASK_DATA_ACCESS'
  | 'WORKSPACE_EXECUTION'
  | 'WORKSPACE_STATE'
  | 'WORKSPACE_VARIABLES';

// 组织
export interface Organization {
  id: number;
  name: string;
  display_name: string;
  description: string;
  is_active: boolean;
  settings: Record<string, any>;
  created_by?: number;
  created_at: string;
  updated_at: string;
}

// 项目
export interface Project {
  id: number;
  org_id: number;
  name: string;
  display_name: string;
  description: string;
  is_default: boolean;
  is_active: boolean;
  settings: Record<string, any>;
  created_by?: number;
  created_at: string;
  updated_at: string;
}

// 团队
export interface Team {
  id: string;
  org_id: number;
  name: string;
  display_name: string;
  description: string;
  is_system: boolean;
  created_by?: number;
  created_at: string;
  updated_at: string;
}

// 团队成员
export interface TeamMember {
  id: number;
  team_id: number;
  user_id: number;
  role: 'MEMBER' | 'MAINTAINER';
  joined_at: string;
  joined_by?: number;
}

// 应用
export interface Application {
  id: number;
  org_id: number;
  name: string;
  app_key: string;
  description: string;
  callback_urls: Record<string, any>;
  is_active: boolean;
  created_by?: number;
  created_at: string;
  expires_at?: string;
  last_used_at?: string;
}

// 权限定义
export interface PermissionDefinition {
  id: number;
  name: string;
  resource_type: ResourceType;
  scope_level: ScopeType;
  display_name: string;
  description: string;
  is_system: boolean;
  created_at: string;
}

// 权限授予
export interface PermissionGrant {
  id: number;
  scope_type: ScopeType;
  scope_id: number;
  scope_id_str?: string; // 语义化ID（如workspace的ws-xxx）
  principal_type: PrincipalType;
  principal_id: number;
  permission_id: number;
  permission_level: PermissionLevel;
  granted_by?: number;
  granted_at: string;
  expires_at?: string;
  reason: string;
  source: string;
}

// ==================== 请求类型 ====================

export interface CheckPermissionRequest {
  resource_type: ResourceType;
  scope_type: ScopeType;
  scope_id: string; // 支持语义化ID（如ws-xxx）和数字ID（会被转换为字符串）
  required_level: PermissionLevel;
}

export interface GrantPermissionRequest {
  scope_type: ScopeType;
  scope_id: number;
  principal_type: PrincipalType;
  principal_id: number;
  permission_id: number;
  permission_level: PermissionLevel;
  expires_at?: string;
  reason?: string;
}

export interface BatchGrantPermissionItem {
  permission_id: string;
  permission_level: PermissionLevel;
}

export interface BatchGrantPermissionRequest {
  scope_type: ScopeType;
  scope_id: number | string; // 支持数字 ID 和语义化 ID
  principal_type: PrincipalType;
  principal_id: string | number;
  permissions: BatchGrantPermissionItem[];
  expires_at?: string;
  reason?: string;
}

export interface GrantPresetRequest {
  scope_type: ScopeType;
  scope_id: number;
  principal_type: PrincipalType;
  principal_id: number;
  preset_name: 'READ' | 'WRITE' | 'ADMIN';
  reason?: string;
}

export interface CreateOrganizationRequest {
  name: string;
  display_name: string;
  description?: string;
  settings?: Record<string, any>;
}

export interface UpdateOrganizationRequest {
  display_name: string;
  description?: string;
  is_active: boolean;
  settings?: Record<string, any>;
}

export interface CreateProjectRequest {
  org_id: number;
  name: string;
  display_name: string;
  description?: string;
  settings?: Record<string, any>;
}

export interface UpdateProjectRequest {
  display_name: string;
  description?: string;
  is_active: boolean;
  settings?: Record<string, any>;
}

export interface CreateTeamRequest {
  org_id: number;
  name: string;
  display_name: string;
  description?: string;
}

export interface AddTeamMemberRequest {
  user_id: string | number;
  role: 'MEMBER' | 'MAINTAINER';
}

export interface CreateApplicationRequest {
  org_id: number;
  name: string;
  description?: string;
  callback_urls?: Record<string, any>;
  expires_at?: string;
}

export interface UpdateApplicationRequest {
  name?: string;
  description?: string;
  callback_urls?: Record<string, any>;
  is_active?: boolean;
  expires_at?: string;
}

// ==================== API服务 ====================

export const iamService = {
  // ==================== 权限管理 ====================

  // 检查权限
  checkPermission: async (request: CheckPermissionRequest): Promise<any> => {
    return await api.post('/iam/permissions/check', request);
  },

  // 授予权限
  grantPermission: async (request: GrantPermissionRequest): Promise<any> => {
    return await api.post('/iam/permissions/grant', request);
  },

  // 批量授予权限
  batchGrantPermissions: async (request: BatchGrantPermissionRequest): Promise<{ message: string; success_count: number; failed_count: number; errors: string[] }> => {
    return await api.post('/iam/permissions/batch-grant', request);
  },

  // 授予预设权限
  grantPresetPermissions: async (request: GrantPresetRequest): Promise<any> => {
    return await api.post('/iam/permissions/grant-preset', request);
  },

  // 撤销权限
  revokePermission: async (scopeType: ScopeType, id: number): Promise<any> => {
    return await api.delete(`/iam/permissions/${scopeType}/${id}`);
  },

  // 列出权限
  listPermissions: async (scopeType: ScopeType, scopeId: number): Promise<{ permissions: PermissionGrant[]; total: number }> => {
    return await api.get(`/iam/permissions/${scopeType}/${scopeId}`);
  },

  // 列出权限定义
  listPermissionDefinitions: async (): Promise<{ definitions: PermissionDefinition[]; total: number }> => {
    return await api.get('/iam/permissions/definitions');
  },

  // ==================== 组织管理 ====================

  // 创建组织
  createOrganization: async (request: CreateOrganizationRequest): Promise<Organization> => {
    return await api.post('/iam/organizations', request);
  },

  // 获取组织列表
  listOrganizations: async (isActive?: boolean): Promise<{ organizations: Organization[]; total: number }> => {
    const params = isActive !== undefined ? { is_active: isActive } : {};
    return await api.get('/iam/organizations', { params });
  },

  // 获取组织详情
  getOrganization: async (id: number): Promise<Organization> => {
    return await api.get(`/iam/organizations/${id}`);
  },

  // 更新组织
  updateOrganization: async (id: number, request: UpdateOrganizationRequest): Promise<any> => {
    return await api.put(`/iam/organizations/${id}`, request);
  },

  // 删除组织
  deleteOrganization: async (id: number): Promise<any> => {
    return await api.delete(`/iam/organizations/${id}`);
  },

  // ==================== 项目管理 ====================

  // 创建项目
  createProject: async (request: CreateProjectRequest): Promise<Project> => {
    return await api.post('/iam/projects', request);
  },

  // 获取项目列表
  listProjects: async (orgId: number): Promise<{ projects: Project[]; total: number }> => {
    return await api.get('/iam/projects', { params: { org_id: orgId } });
  },

  // 获取项目详情
  getProject: async (id: number): Promise<Project> => {
    return await api.get(`/iam/projects/${id}`);
  },

  // 更新项目
  updateProject: async (id: number, request: UpdateProjectRequest): Promise<any> => {
    return await api.put(`/iam/projects/${id}`, request);
  },

  // 删除项目
  deleteProject: async (id: number): Promise<any> => {
    return await api.delete(`/iam/projects/${id}`);
  },

  // ==================== 团队管理 ====================

  // 创建团队
  createTeam: async (request: CreateTeamRequest): Promise<Team> => {
    return await api.post('/iam/teams', request);
  },

  // 获取团队列表
  listTeams: async (orgId: number): Promise<{ teams: Team[]; total: number }> => {
    return await api.get('/iam/teams', { params: { org_id: orgId } });
  },

  // 获取团队详情
  getTeam: async (id: string): Promise<Team> => {
    return await api.get(`/iam/teams/${id}`);
  },

  // 删除团队
  deleteTeam: async (id: string): Promise<any> => {
    return await api.delete(`/iam/teams/${id}`);
  },

  // 添加团队成员
  addTeamMember: async (teamId: string, request: AddTeamMemberRequest): Promise<any> => {
    return await api.post(`/iam/teams/${teamId}/members`, request);
  },

  // 移除团队成员
  removeTeamMember: async (teamId: string, userId: number): Promise<any> => {
    return await api.delete(`/iam/teams/${teamId}/members/${userId}`);
  },

  // 列出团队成员
  listTeamMembers: async (teamId: string): Promise<{ members: TeamMember[]; total: number }> => {
    return await api.get(`/iam/teams/${teamId}/members`);
  },

  // ==================== 团队Token管理 ====================

  // 创建团队Token
  createTeamToken: async (teamId: string, tokenName: string, expiresInDays: number): Promise<{ message: string; token: any }> => {
    return await api.post(`/iam/teams/${teamId}/tokens`, { 
      token_name: tokenName,
      expires_in_days: expiresInDays 
    });
  },

  // 列出团队Token
  listTeamTokens: async (teamId: string): Promise<{ tokens: any[] }> => {
    return await api.get(`/iam/teams/${teamId}/tokens`);
  },

  // 吊销团队Token
  revokeTeamToken: async (teamId: string, tokenId: number): Promise<{ message: string }> => {
    return await api.delete(`/iam/teams/${teamId}/tokens/${tokenId}`);
  },

  // ==================== 应用管理 ====================

  // 创建应用
  createApplication: async (request: CreateApplicationRequest): Promise<{ application: Application; app_secret: string; message: string }> => {
    return await api.post('/iam/applications', request);
  },

  // 获取应用列表
  listApplications: async (orgId: number, isActive?: boolean): Promise<{ applications: Application[]; total: number }> => {
    const params: any = { org_id: orgId };
    if (isActive !== undefined) {
      params.is_active = isActive;
    }
    return await api.get('/iam/applications', { params });
  },

  // 获取应用详情
  getApplication: async (id: number): Promise<Application> => {
    return await api.get(`/iam/applications/${id}`);
  },

  // 更新应用
  updateApplication: async (id: number, request: UpdateApplicationRequest): Promise<any> => {
    return await api.put(`/iam/applications/${id}`, request);
  },

  // 删除应用
  deleteApplication: async (id: number): Promise<any> => {
    return await api.delete(`/iam/applications/${id}`);
  },

  // 重新生成密钥
  regenerateSecret: async (id: number): Promise<{ app_secret: string; message: string }> => {
    return await api.post(`/iam/applications/${id}/regenerate-secret`);
  },

  // ==================== 审计日志 ====================

  // 查询权限变更历史
  queryPermissionHistory: async (params: {
    scope_type: ScopeType;
    scope_id: number;
    start_time?: string;
    end_time?: string;
    limit?: number;
  }): Promise<{ logs: any[]; total: number }> => {
    return await api.get('/iam/audit/permission-history', { params });
  },

  // 查询访问历史
  queryAccessHistory: async (params: {
    user_id?: number;
    resource_type?: string;
    method?: string;
    http_code_operator?: string;
    http_code_value?: number;
    start_time?: string;
    end_time?: string;
    limit?: number;
  }): Promise<{ logs: any[]; total: number }> => {
    return await api.get('/iam/audit/access-history', { params });
  },

  // 查询被拒绝的访问
  queryDeniedAccess: async (params: {
    start_time?: string;
    end_time?: string;
    limit?: number;
  }): Promise<{ logs: any[]; total: number }> => {
    return await api.get('/iam/audit/denied-access', { params });
  },

  // ==================== 用户管理 ====================

  // 获取用户统计
  getUserStats: async (): Promise<any> => {
    return await api.get('/iam/users/stats');
  },

  // 列出用户
  listUsers: async (params: {
    role?: string;
    is_active?: boolean;
    search?: string;
    limit?: number;
    offset?: number;
  }): Promise<{ users: any[]; total: number }> => {
    return await api.get('/iam/users', { params });
  },

  // 获取用户详情
  getUser: async (id: number): Promise<any> => {
    return await api.get(`/iam/users/${id}`);
  },

  // 更新用户
  updateUser: async (id: number, data: { role?: string; is_active?: boolean }): Promise<any> => {
    return await api.put(`/iam/users/${id}`, data);
  },

  // 激活用户
  activateUser: async (id: number): Promise<any> => {
    return await api.post(`/iam/users/${id}/activate`);
  },

  // 停用用户
  deactivateUser: async (id: number): Promise<any> => {
    return await api.post(`/iam/users/${id}/deactivate`);
  },

  // 创建用户
  createUser: async (data: {
    username: string;
    email: string;
    password: string;
    role: string;
  }): Promise<any> => {
    return await api.post('/iam/users', data);
  },

  // 删除用户
  deleteUser: async (id: number): Promise<any> => {
    return await api.delete(`/iam/users/${id}`);
  },

  // ==================== 审计配置 ====================

  // 获取审计配置
  getAuditConfig: async (): Promise<{ enabled: boolean; include_body: boolean; include_headers: boolean }> => {
    return await api.get('/iam/audit/config');
  },

  // 更新审计配置
  updateAuditConfig: async (config: { enabled: boolean; include_body: boolean; include_headers: boolean }): Promise<any> => {
    return await api.put('/iam/audit/config', config);
  },
};
