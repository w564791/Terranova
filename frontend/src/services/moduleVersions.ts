import api from './api';

// 模块版本类型定义
export interface ModuleVersion {
  id: string;                    // modv-xxx 语义化 ID
  module_id: number;
  version: string;               // Terraform Module 版本 (如 6.1.5)
  source?: string;
  module_source?: string;
  is_default: boolean;
  status: string;                // active, deprecated, archived
  active_schema_id?: number;     // 当前使用的 Schema ID
  inherited_from_version_id?: string;
  schema_count?: number;
  active_schema_version?: string; // Schema 版本号（如 "45"）
  demo_count?: number;
  created_by?: string;
  created_by_name?: string;
  created_at: string;
  updated_at: string;
}

export interface ModuleVersionListResponse {
  items: ModuleVersion[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface CreateModuleVersionRequest {
  version: string;               // 新 TF 版本号
  source?: string;               // 可选覆盖 source
  module_source?: string;        // 可选覆盖 module_source
  inherit_schema_from?: string;  // 从哪个版本继承 Schema（版本 ID）
  set_as_default?: boolean;      // 是否设为默认版本
}

export interface UpdateModuleVersionRequest {
  source?: string;
  module_source?: string;
  status?: string;               // active, deprecated, archived
}

export interface SetDefaultVersionRequest {
  version_id: string;
}

export interface InheritDemosRequest {
  from_version_id: string;
  demo_ids?: number[];           // 可选，不传则继承全部
}

export interface VersionCompareResult {
  from_version: string;
  to_version: string;
  from_schema_id: number;
  to_schema_id: number;
  added_fields: string[];
  removed_fields: string[];
  common_fields: string[];
  stats: {
    added: number;
    removed: number;
    common: number;
  };
}

// 注意：api 拦截器已经返回 response.data，所以这里直接返回数据

// 获取模块的所有版本
export const listVersions = async (
  moduleId: number,
  page: number = 1,
  pageSize: number = 20
): Promise<ModuleVersionListResponse> => {
  return api.get(`/modules/${moduleId}/versions`, {
    params: { page, page_size: pageSize }
  }) as unknown as ModuleVersionListResponse;
};

// 获取版本详情
export const getVersion = async (
  moduleId: number,
  versionId: string
): Promise<ModuleVersion> => {
  return api.get(`/modules/${moduleId}/versions/${versionId}`) as unknown as ModuleVersion;
};

// 获取默认版本
export const getDefaultVersion = async (
  moduleId: number
): Promise<ModuleVersion> => {
  return api.get(`/modules/${moduleId}/default-version`) as unknown as ModuleVersion;
};

// 创建新版本
export const createVersion = async (
  moduleId: number,
  data: CreateModuleVersionRequest
): Promise<ModuleVersion> => {
  return api.post(`/modules/${moduleId}/versions`, data) as unknown as ModuleVersion;
};

// 更新版本信息
export const updateVersion = async (
  moduleId: number,
  versionId: string,
  data: UpdateModuleVersionRequest
): Promise<ModuleVersion> => {
  return api.put(`/modules/${moduleId}/versions/${versionId}`, data) as unknown as ModuleVersion;
};

// 删除版本
export const deleteVersion = async (
  moduleId: number,
  versionId: string
): Promise<void> => {
  await api.delete(`/modules/${moduleId}/versions/${versionId}`);
};

// 设置默认版本
export const setDefaultVersion = async (
  moduleId: number,
  versionId: string
): Promise<void> => {
  await api.put(`/modules/${moduleId}/default-version`, { version_id: versionId });
};

// 继承 Demos（创建版本时使用）
export const inheritDemos = async (
  moduleId: number,
  targetVersionId: string,
  data: InheritDemosRequest
): Promise<{ message: string; inherited_count: number }> => {
  return api.post(
    `/modules/${moduleId}/versions/${targetVersionId}/inherit-demos`,
    data
  ) as unknown as { message: string; inherited_count: number };
};

// 导入 Demos（从其他版本导入）
export interface ImportDemosRequest {
  from_version_id: string;
  demo_ids?: number[];           // 可选，不传则导入全部
}

export const importDemos = async (
  moduleId: number,
  targetVersionId: string,
  data: ImportDemosRequest
): Promise<{ message: string; imported_count: number }> => {
  return api.post(
    `/modules/${moduleId}/versions/${targetVersionId}/import-demos`,
    data
  ) as unknown as { message: string; imported_count: number };
};

// 获取版本的所有 Schema
export const getVersionSchemas = async (
  moduleId: number,
  versionId: string
): Promise<any[]> => {
  return api.get(`/modules/${moduleId}/versions/${versionId}/schemas`) as unknown as any[];
};

// 获取版本的所有 Demo
export const getVersionDemos = async (
  moduleId: number,
  versionId: string
): Promise<any[]> => {
  return api.get(`/modules/${moduleId}/versions/${versionId}/demos`) as unknown as any[];
};

// 比较两个版本的 Schema 差异
export const compareVersions = async (
  moduleId: number,
  fromVersionId: string,
  toVersionId: string
): Promise<VersionCompareResult> => {
  return api.get(`/modules/${moduleId}/versions/compare`, {
    params: { from: fromVersionId, to: toVersionId }
  }) as unknown as VersionCompareResult;
};

// 迁移现有模块数据（管理员操作）
export const migrateExistingModules = async (): Promise<{ message: string }> => {
  return api.post('/modules/migrate-versions') as unknown as { message: string };
};
