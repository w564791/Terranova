import api from './api';

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

// 获取项目列表（带工作空间数量）
export const getProjects = async (orgId: number = 1): Promise<Project[]> => {
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
  org_id: number;
  name: string;
  display_name: string;
  description?: string;
}

// 创建项目
export const createProject = async (data: CreateProjectRequest): Promise<Project> => {
  const response = await api.post('/iam/projects', data);
  // 注意：api.ts 的响应拦截器已经返回 response.data
  return response as any;
};

export default {
  getProjects,
  getProjectWorkspaces,
  getWorkspaceProject,
  setWorkspaceProject,
  removeWorkspaceFromProject,
  createProject
};
