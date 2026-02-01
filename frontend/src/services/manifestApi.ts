import api from './api';

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

export interface ManifestVersion {
  id: string;
  manifest_id: string;
  version: string;
  canvas_data: ManifestCanvasData;
  nodes: ManifestNode[];
  edges: ManifestEdge[];
  variables: ManifestVariable[];
  hcl_content?: string;
  is_draft: boolean;
  created_by: string;
  created_by_name?: string;
  created_at: string;
}

export interface ManifestDeployment {
  id: string;
  manifest_id: string;
  version_id: string;
  workspace_id: number;
  variable_overrides?: Record<string, any>;
  status: string;
  last_task_id?: number;
  deployed_by: string;
  deployed_at?: string;
  created_at: string;
  updated_at: string;
  workspace_name?: string;
  workspace_semantic_id?: string; // ws-xxx 格式
  deployed_by_name?: string;
  version_name?: string;
}

export interface ManifestNode {
  id: string;
  type: 'module' | 'variable';
  module_id?: number;
  is_linked: boolean;
  link_status: 'linked' | 'unlinked' | 'mismatch';
  module_source?: string;
  module_version?: string;
  instance_name: string;
  resource_name: string;
  raw_source?: string;
  raw_version?: string;
  raw_config?: Record<string, any>;
  position: { x: number; y: number };
  config: Record<string, any>;
  config_complete: boolean;
  ports: ManifestPort[];
}

export interface ManifestPort {
  id: string;
  type: 'input' | 'output';
  name: string;
  data_type?: string;
  description?: string;
}

export interface ManifestEdge {
  id: string;
  type: 'dependency' | 'variable_binding';
  source: { node_id: string; port_id?: string };
  target: { node_id: string; port_id?: string };
  expression?: string;
}

export interface ManifestVariable {
  name: string;
  type: string;
  description?: string;
  default?: any;
  required: boolean;
  sensitive?: boolean;
}

export interface ManifestCanvasData {
  viewport: { x: number; y: number };
  zoom: number;
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

export interface SaveManifestVersionRequest {
  canvas_data: ManifestCanvasData;
  nodes: ManifestNode[];
  edges: ManifestEdge[];
  variables?: ManifestVariable[];
}

export interface PublishManifestVersionRequest {
  version: string;
}

export interface CreateManifestDeploymentRequest {
  version_id: string;
  workspace_id: string;
  variable_overrides?: Record<string, any>;
  auto_apply?: boolean;
  plan_only?: boolean;
}

export interface UpdateManifestDeploymentRequest {
  version_id?: string;
  variable_overrides?: Record<string, any>;
  auto_apply?: boolean;
  plan_only?: boolean;
}

export interface ManifestListResponse {
  items: Manifest[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface ManifestVersionListResponse {
  items: ManifestVersion[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface ManifestDeploymentListResponse {
  items: ManifestDeployment[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

// ========== API 函数 ==========

// Manifest CRUD
// 注意：api 拦截器已经返回 response.data，所以这里直接返回结果
export const listManifests = async (
  orgId: string,
  params?: { page?: number; page_size?: number; status?: string }
): Promise<ManifestListResponse> => {
  return api.get(`/organizations/${orgId}/manifests`, { params });
};

export const getManifest = async (orgId: string, id: string): Promise<Manifest> => {
  return api.get(`/organizations/${orgId}/manifests/${id}`);
};

export const createManifest = async (
  orgId: string,
  data: CreateManifestRequest
): Promise<Manifest> => {
  return api.post(`/organizations/${orgId}/manifests`, data);
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

// 版本管理
export const listManifestVersions = async (
  orgId: string,
  manifestId: string
): Promise<ManifestVersionListResponse> => {
  return api.get(`/organizations/${orgId}/manifests/${manifestId}/versions`);
};

export const getManifestVersion = async (
  orgId: string,
  manifestId: string,
  versionId: string
): Promise<ManifestVersion> => {
  return api.get(
    `/organizations/${orgId}/manifests/${manifestId}/versions/${versionId}`
  );
};

export const saveManifestDraft = async (
  orgId: string,
  manifestId: string,
  data: SaveManifestVersionRequest
): Promise<ManifestVersion> => {
  return api.put(`/organizations/${orgId}/manifests/${manifestId}/draft`, data);
};

export const publishManifestVersion = async (
  orgId: string,
  manifestId: string,
  data: PublishManifestVersionRequest
): Promise<ManifestVersion> => {
  return api.post(`/organizations/${orgId}/manifests/${manifestId}/versions`, data);
};

// 部署管理
export const listManifestDeployments = async (
  orgId: string,
  manifestId: string
): Promise<ManifestDeploymentListResponse> => {
  return api.get(`/organizations/${orgId}/manifests/${manifestId}/deployments`);
};

export const getManifestDeployment = async (
  orgId: string,
  manifestId: string,
  deploymentId: string
): Promise<ManifestDeployment> => {
  return api.get(
    `/organizations/${orgId}/manifests/${manifestId}/deployments/${deploymentId}`
  );
};

export const createManifestDeployment = async (
  orgId: string,
  manifestId: string,
  data: CreateManifestDeploymentRequest
): Promise<ManifestDeployment> => {
  return api.post(
    `/organizations/${orgId}/manifests/${manifestId}/deployments`,
    data
  );
};

export const updateManifestDeployment = async (
  orgId: string,
  manifestId: string,
  deploymentId: string,
  data: UpdateManifestDeploymentRequest
): Promise<ManifestDeployment> => {
  return api.put(
    `/organizations/${orgId}/manifests/${manifestId}/deployments/${deploymentId}`,
    data
  );
};

export const deleteManifestDeployment = async (
  orgId: string,
  manifestId: string,
  deploymentId: string,
  options?: { uninstall?: boolean; force?: boolean }
): Promise<any> => {
  const params = new URLSearchParams();
  if (options?.uninstall) params.append('uninstall', 'true');
  if (options?.force) params.append('force', 'true');
  const queryString = params.toString();
  const url = `/organizations/${orgId}/manifests/${manifestId}/deployments/${deploymentId}${queryString ? '?' + queryString : ''}`;
  return api.delete(url);
};

// 导入导出
export const exportManifestHCL = async (
  orgId: string,
  manifestId: string,
  versionId?: string
): Promise<string> => {
  const params = versionId ? { version_id: versionId } : {};
  return api.get(`/organizations/${orgId}/manifests/${manifestId}/export`, {
    params,
    responseType: 'text',
  });
};

// 导出 ZIP 包 (包含 manifest.json 和 .tf 文件)
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

export const importManifestHCL = async (
  orgId: string,
  hclContent: string,
  name: string
): Promise<Manifest> => {
  return api.post(`/organizations/${orgId}/manifests/import`, {
    hcl_content: hclContent,
    name,
  });
};

// 导入 manifest.json
export const importManifestJSON = async (
  orgId: string,
  manifestJSON: any,
  name?: string
): Promise<Manifest> => {
  return api.post(`/organizations/${orgId}/manifests/import-json`, {
    manifest_json: manifestJSON,
    name,
  });
};

// Workspace 视角
export const getWorkspaceManifestDeployment = async (
  workspaceId: string
): Promise<ManifestDeployment> => {
  return api.get(`/workspaces/${workspaceId}/manifest-deployment`);
};

// 部署资源
export interface DeploymentResource {
  node_id: string;
  resource_id: string;
  resource_db_id: number; // 数据库 ID，用于跳转
  resource_name: string;
  resource_type: string;
  is_active: boolean;
  description: string;
  created_at: string;
  is_drifted: boolean;   // 是否漂移
  config_hash: string;   // 部署时的 hash
  current_hash: string;  // 当前 hash
}

export const getManifestDeploymentResources = async (
  orgId: string,
  manifestId: string,
  deploymentId: string
): Promise<DeploymentResource[]> => {
  return api.get(
    `/organizations/${orgId}/manifests/${manifestId}/deployments/${deploymentId}/resources`
  );
};
