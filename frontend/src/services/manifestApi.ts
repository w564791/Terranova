import api from './api';

// Manifest 列表/管理页(ManifestManagement.tsx)用的轻量 API。
// 文件/版本/部署的完整操作已迁移到 pages/admin/ManifestEditorV2/manifestApi.ts(VS Code Web 编辑器),
// 旧画布相关的类型与函数(canvas/nodes/edges、draft/version/deployment CRUD、HCL import/export)
// 已随重构移除。

// ========== 类型定义 ==========

export interface Manifest {
  id: string;
  organization_id: string;
  name: string;
  description: string;
  status: 'draft' | 'published' | 'archived';
  created_by: string;
  created_by_name?: string;
  created_at: string;
  updated_at: string;
  latest_version?: ManifestVersion;
  deployment_count?: number;
}

// 列表页只读展示用的版本元信息(新模型:画布字段已废弃)
export interface ManifestVersion {
  id: string;
  manifest_id: string;
  version: string;
  changelog?: string;
  created_by: string;
  created_by_name?: string;
  created_at: string;
}

// ========== 请求/响应类型 ==========

export interface CreateManifestRequest {
  name: string;
  description?: string;
}

export interface UpdateManifestRequest {
  name?: string;
  description?: string;
  status?: 'draft' | 'published' | 'archived';
}

export interface ManifestListResponse {
  items: Manifest[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

// ========== API 函数 ==========
// 注意：api 拦截器已经返回 response.data，所以这里直接返回结果

export const listManifests = async (
  orgId: string,
  params?: { page?: number; page_size?: number; status?: string }
): Promise<ManifestListResponse> => {
  return api.get(`/organizations/${orgId}/manifests`, { params });
};

export const createManifest = async (
  orgId: string,
  data: CreateManifestRequest
): Promise<Manifest> => {
  return api.post(`/organizations/${orgId}/manifests`, data);
};

export const getManifest = async (orgId: string, id: string): Promise<Manifest> => {
  return api.get(`/organizations/${orgId}/manifests/${id}`);
};

export const updateManifest = async (
  orgId: string,
  id: string,
  data: UpdateManifestRequest
): Promise<Manifest> => {
  return api.put(`/organizations/${orgId}/manifests/${id}`, data);
};

export const deleteManifest = async (orgId: string, id: string): Promise<void> => {
  await api.delete(`/organizations/${orgId}/manifests/${id}`);
};

// 导出 ZIP 包(新模型:已发布版本 / 当前用户草稿的 .tf 文件)
export const exportManifestZip = async (
  orgId: string,
  manifestId: string,
  versionId?: string
): Promise<Blob> => {
  const params = versionId ? { version_id: versionId } : {};
  const response = await api.get(`/organizations/${orgId}/manifests/${manifestId}/export-zip`, {
    params,
    responseType: 'blob',
  });
  return response as unknown as Blob;
};
