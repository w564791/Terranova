import api, { getAuthOrgId } from './api';

// Project 类型定义
export interface Project {
  id: number;
  org_id: number;
  name: string;
  display_name: string;
  description: string;
  is_default: boolean;
  is_active: boolean;
  settings: Record<string, unknown>;
  created_by: string;
  created_at: string;
  updated_at: string;
  workspace_count?: number;
}

function requireActiveOrganizationId(): number {
  const orgId = getAuthOrgId();
  if (orgId == null) {
    throw new Error('当前未选择组织');
  }
  return orgId;
}

// 获取当前 active organization 的项目列表（带工作空间数量）
export const getProjects = async (): Promise<Project[]> => {
  const orgId = requireActiveOrganizationId();
  const response = await api.get('/projects', {
    params: { org_id: orgId }
  });
  // 注意：api.ts 的响应拦截器已经返回 response.data，所以这里直接使用 response
  const data = response as any;
  return data.projects || [];
};

// 获取项目下的工作空间
export const getProjectWorkspaces = async (projectId: number): Promise<{
  workspaces: unknown[];
  total: number;
}> => {
  const response = await api.get(`/projects/${projectId}/workspaces`);
  // 注意：api.ts 的响应拦截器已经返回 response.data
  return response as any;
};

// 获取工作空间所属的项目
export const getWorkspaceProject = async (workspaceId: string): Promise<Project | null> => {
  const response = await api.get(`/workspaces/${workspaceId}/project`);
  // 注意：api.ts 的响应拦截器已经返回 response.data
  const data = response as any;
  return data.project || null;
};

// 设置工作空间所属的项目
export const setWorkspaceProject = async (workspaceId: string, projectId: number): Promise<void> => {
  await api.put(`/workspaces/${workspaceId}/project`, {
    project_id: projectId
  });
};

// 从项目中移除工作空间
export const removeWorkspaceFromProject = async (workspaceId: string): Promise<void> => {
  await api.delete(`/workspaces/${workspaceId}/project`);
};

// 创建项目请求
export interface CreateProjectRequest {
  name: string;
  display_name: string;
  description?: string;
}

// 创建项目
export const createProject = async (data: CreateProjectRequest): Promise<Project> => {
  const orgId = requireActiveOrganizationId();
  const response = await api.post(
    '/iam/projects',
    { ...data, org_id: orgId },
    { params: { org_id: orgId } },
  );
  // 注意：api.ts 的响应拦截器已经返回 response.data
  return response as any;
};

// 删除项目（默认项目后端会拒绝）
export const deleteProject = async (projectId: number): Promise<void> => {
  await api.delete(`/iam/projects/${projectId}`);
};

export default {
  getProjects,
  getProjectWorkspaces,
  getWorkspaceProject,
  setWorkspaceProject,
  removeWorkspaceFromProject,
  createProject,
  deleteProject,
};
