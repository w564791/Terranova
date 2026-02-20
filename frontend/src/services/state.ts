import api from './api';

export interface StateVersion {
  id: number;
  workspace_id: string;
  version: number;
  created_at: string;
  created_by: string;
  created_by_name?: string; // 用户名（后端 JOIN users 表获取）
  checksum: string;
  size_bytes: number;
  lineage: string;
  serial: number;
  is_imported: boolean;
  import_source: string;
  is_rollback: boolean;
  rollback_from_version?: number;
  description: string;
  task_id?: number;
  resource_count: number;
}

export interface StateVersionsResponse {
  versions: StateVersion[];
  total: number;
  current_version: number;
  limit: number;
  offset: number;
}

export interface RetrieveStateContentResponse {
  data: {
    version: number;
    content: Record<string, any>;
  };
  audit: {
    accessed_at: string;
    accessed_by: string;
  };
}

export interface UploadStateRequest {
  state: Record<string, any>;
  force: boolean;
  description?: string;
}

export interface UploadStateResponse {
  message: string;
  version: number;
  warnings: string[];
  state_version: StateVersion;
}

export interface RollbackStateRequest {
  target_version: number;
  reason: string;
  force?: boolean;
}

export interface RollbackStateResponse {
  message: string;
  new_version: number;
  rollback_from_version: number;
  description: string;
  warnings: string[];
}

/**
 * State API Service
 */
export const stateAPI = {
  /**
   * 获取 State 版本历史
   */
  getStateVersions: async (
    workspaceId: string,
    limit: number = 50,
    offset: number = 0
  ): Promise<StateVersionsResponse> => {
    // api 拦截器已经返回 response.data，直接返回即可
    return api.get(
      `/workspaces/${workspaceId}/state/versions`,
      {
        params: { limit, offset },
      }
    );
  },

  /**
   * 获取指定版本的 State 元数据（不含 content）
   */
  getStateVersion: async (
    workspaceId: string,
    version: number
  ): Promise<StateVersion> => {
    return api.get(
      `/workspaces/${workspaceId}/state/versions/${version}`
    );
  },

  /**
   * 获取指定版本的 State 完整内容（需要 WORKSPACE_STATE_SENSITIVE 权限）
   * 此接口返回完整的 state 内容，包含敏感数据
   */
  retrieveStateContent: async (
    workspaceId: string,
    version: number
  ): Promise<RetrieveStateContentResponse> => {
    return api.get(
      `/workspaces/${workspaceId}/state/versions/${version}/retrieve`
    );
  },

  /**
   * 下载指定版本的 State 文件
   */
  downloadStateVersion: async (
    workspaceId: string,
    version: number
  ): Promise<Blob> => {
    return api.get(
      `/workspaces/${workspaceId}/state/versions/${version}/download`,
      {
        responseType: 'blob',
      }
    );
  },

  /**
   * 上传 State（JSON 格式）
   */
  uploadState: async (
    workspaceId: string,
    request: UploadStateRequest
  ): Promise<UploadStateResponse> => {
    return api.post(
      `/workspaces/${workspaceId}/state/upload`,
      request
    );
  },

  /**
   * 上传 State 文件（multipart/form-data）
   */
  uploadStateFile: async (
    workspaceId: string,
    file: File,
    force: boolean,
    description?: string
  ): Promise<UploadStateResponse> => {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('force', force.toString());
    if (description) {
      formData.append('description', description);
    }

    return api.post(
      `/workspaces/${workspaceId}/state/upload-file`,
      formData,
      {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
      }
    );
  },

  /**
   * 回滚 State 到指定版本
   */
  rollbackState: async (
    workspaceId: string,
    request: RollbackStateRequest
  ): Promise<RollbackStateResponse> => {
    return api.post(
      `/workspaces/${workspaceId}/state/rollback`,
      request
    );
  },
};

/**
 * 辅助函数：触发文件下载
 */
export const triggerDownload = (blob: Blob, filename: string) => {
  const url = window.URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  window.URL.revokeObjectURL(url);
};

/**
 * 辅助函数：格式化文件大小
 */
export const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
};

/**
 * 辅助函数：获取来源标签
 */
export const getSourceLabel = (importSource: string): string => {
  const labels: Record<string, string> = {
    user_upload: '用户上传',
    terraform_apply: 'Terraform Apply',
    rollback: '回滚',
    api: 'API',
  };
  return labels[importSource] || importSource;
};
