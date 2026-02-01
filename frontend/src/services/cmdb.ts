import api from './api';

// CMDB统计信息
export interface CMDBStats {
  total_workspaces: number;
  total_resources: number;
  total_modules: number;
  resource_type_stats: ResourceTypeStat[];
  last_synced_at?: string;
}

export interface ResourceTypeStat {
  resource_type: string;
  count: number;
}

// 资源搜索结果
export interface ResourceSearchResult {
  workspace_id: string;
  workspace_name?: string;
  terraform_address: string;
  resource_type: string;
  resource_name: string;
  cloud_resource_id?: string;
  cloud_resource_name?: string;
  cloud_resource_arn?: string;  // 云资源全局标识符（AWS ARN / Azure Resource ID / GCP Resource Name）
  description?: string;
  module_path?: string;
  root_module_name?: string;
  platform_resource_id?: number;
  platform_resource_name?: string;
  jump_url?: string;
  match_rank: number;
  // 外部数据源相关字段
  source_type?: string;           // terraform 或 external
  external_source_id?: string;    // 外部数据源ID
  external_source_name?: string;  // 外部数据源名称
  cloud_provider?: string;        // 云提供商
  cloud_account_id?: string;      // 云账户ID
  cloud_account_name?: string;    // 云账户名称
  cloud_region?: string;          // 云区域
}

// 资源树节点
export interface ResourceTreeNode {
  type: 'module' | 'resource';
  name: string;
  path?: string;
  terraform_address?: string;
  terraform_type?: string;
  terraform_name?: string;
  cloud_id?: string;
  cloud_name?: string;
  cloud_arn?: string;  // 云资源全局标识符（AWS ARN / Azure Resource ID / GCP Resource Name）
  description?: string;
  mode?: string;
  resource_count?: number;
  children?: ResourceTreeNode[];
  platform_resource_id?: number;
  jump_url?: string;
}

// Workspace资源树
export interface WorkspaceResourceTree {
  workspace_id: string;
  workspace_name: string;
  total_resources: number;
  tree: ResourceTreeNode[];
}

// 资源详情
export interface ResourceDetail {
  id: number;
  workspace_id: string;
  terraform_address: string;
  resource_type: string;
  resource_name: string;
  resource_mode: string;
  index_key?: string;
  cloud_resource_id?: string;
  cloud_resource_name?: string;
  cloud_resource_arn?: string;
  description?: string;
  module_path?: string;
  module_depth: number;
  parent_module_path?: string;
  root_module_name?: string;
  attributes?: Record<string, unknown>;
  tags?: Record<string, string>;
  provider?: string;
  state_version_id?: number;
  last_synced_at: string;
  created_at: string;
}

// 搜索响应
export interface SearchResponse {
  query: string;
  count: number;
  results: ResourceSearchResult[];
}

// 搜索建议项
export interface SearchSuggestion {
  value: string;        // 建议值（用于搜索）
  label: string;        // 显示标签
  type: 'id' | 'name' | 'description' | 'arn';  // 类型
  resource_type: string; // 资源类型
  source_type?: string;  // 数据源类型：terraform 或 external
  is_external?: boolean; // 是否为外部数据源
}

// 搜索建议响应
export interface SuggestionsResponse {
  suggestions: SearchSuggestion[];
}

// Vector 搜索响应
export interface VectorSearchResponse {
  query: string;
  count: number;
  results: ResourceSearchResult[];
  search_method: 'vector' | 'keyword';
  fallback_reason?: string;
}

// CMDB API服务
export const cmdbService = {
  // 搜索资源（关键字搜索）
  searchResources: async (
    query: string,
    options?: {
      workspace_id?: string;
      resource_type?: string;
      limit?: number;
    }
  ): Promise<SearchResponse> => {
    const params = new URLSearchParams({ q: query });
    if (options?.workspace_id) params.append('workspace_id', options.workspace_id);
    if (options?.resource_type) params.append('resource_type', options.resource_type);
    if (options?.limit) params.append('limit', options.limit.toString());
    
    // api拦截器已经返回response.data，不需要再访问.data
    return api.get(`/cmdb/search?${params.toString()}`);
  },

  // Vector 搜索（支持自动降级）
  vectorSearch: async (
    query: string,
    options?: {
      workspace_ids?: string[];
      resource_type?: string;
      limit?: number;
    }
  ): Promise<VectorSearchResponse> => {
    // 注意：vector-search 在 /ai 路由组下
    const response: any = await api.post('/ai/cmdb/vector-search', {
      query,
      workspace_ids: options?.workspace_ids,
      resource_type: options?.resource_type,
      limit: options?.limit || 50,
    });
    // 后端返回 {code: 200, data: {...}}
    return response.data || response;
  },

  // 获取CMDB统计信息
  getStats: async (): Promise<CMDBStats> => {
    return api.get('/cmdb/stats');
  },

  // 获取资源类型列表
  getResourceTypes: async (): Promise<{ resource_types: ResourceTypeStat[] }> => {
    return api.get('/cmdb/resource-types');
  },

  // 获取Workspace资源树
  getWorkspaceResourceTree: async (workspaceId: string): Promise<WorkspaceResourceTree> => {
    return api.get(`/cmdb/workspaces/${workspaceId}/tree`);
  },

  // 获取资源详情
  getResourceDetail: async (workspaceId: string, address: string): Promise<ResourceDetail> => {
    return api.get(`/cmdb/workspaces/${workspaceId}/resources`, {
      params: { address }
    });
  },

  // 同步Workspace资源索引
  syncWorkspace: async (workspaceId: string): Promise<{ message: string; workspace_id: string }> => {
    return api.post(`/cmdb/workspaces/${workspaceId}/sync`);
  },

  // 同步所有Workspace资源索引
  syncAllWorkspaces: async (): Promise<{ message: string }> => {
    return api.post('/cmdb/sync-all');
  },

  // 获取所有Workspace的资源数量
  getWorkspaceResourceCounts: async (): Promise<{ counts: WorkspaceResourceCount[] }> => {
    return api.get('/cmdb/workspace-counts');
  },

  // 获取搜索建议
  getSearchSuggestions: async (
    prefix: string,
    limit?: number
  ): Promise<SuggestionsResponse> => {
    const params = new URLSearchParams({ q: prefix });
    if (limit) params.append('limit', limit.toString());
    return api.get(`/cmdb/suggestions?${params.toString()}`);
  },
};

// Workspace资源数量
export interface WorkspaceResourceCount {
  workspace_id: string;
  workspace_name: string;
  resource_count: number;
  last_synced_at?: string;
}

// ===== 外部数据源相关类型 =====

// 认证Header输入
export interface AuthHeaderInput {
  key: string;
  value?: string;  // 创建/更新时传入
}

// 认证Header显示
export interface AuthHeaderDisplay {
  key: string;
  has_value: boolean;  // 是否已设置值
}

// 字段映射配置
export interface FieldMapping {
  resource_type?: string;
  resource_name?: string;
  cloud_resource_id?: string;
  cloud_resource_name?: string;
  cloud_resource_arn?: string;
  description?: string;
  tags?: string;
  attributes?: string;
}

// 创建外部数据源请求
export interface CreateExternalSourceRequest {
  name: string;
  description?: string;
  api_endpoint: string;
  http_method?: 'GET' | 'POST';
  request_body?: string;
  auth_headers?: AuthHeaderInput[];
  response_path?: string;
  field_mapping: Record<string, string>;
  primary_key_field: string;
  cloud_provider?: string;
  account_id?: string;
  account_name?: string;
  region?: string;
  sync_interval_minutes?: number;
  resource_type_filter?: string;
}

// 更新外部数据源请求
export interface UpdateExternalSourceRequest {
  name?: string;
  description?: string;
  api_endpoint?: string;
  http_method?: 'GET' | 'POST';
  request_body?: string;
  auth_headers?: AuthHeaderInput[];
  response_path?: string;
  field_mapping?: Record<string, string>;
  primary_key_field?: string;
  cloud_provider?: string;
  account_id?: string;
  account_name?: string;
  region?: string;
  sync_interval_minutes?: number;
  is_enabled?: boolean;
  resource_type_filter?: string;
}

// 外部数据源响应
export interface ExternalSourceResponse {
  source_id: string;
  name: string;
  description?: string;
  api_endpoint: string;
  http_method: string;
  request_body?: string;
  auth_headers?: AuthHeaderDisplay[];
  response_path?: string;
  field_mapping: Record<string, string>;
  primary_key_field: string;
  cloud_provider?: string;
  account_id?: string;
  account_name?: string;
  region?: string;
  sync_interval_minutes: number;
  is_enabled: boolean;
  resource_type_filter?: string;
  created_by?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
  last_sync_at?: string;
  last_sync_status?: 'running' | 'success' | 'failed';
  last_sync_message?: string;
  last_sync_count: number;
}

// 外部数据源列表响应
export interface ExternalSourceListResponse {
  sources: ExternalSourceResponse[];
  total: number;
}

// 测试连接响应
export interface TestConnectionResponse {
  success: boolean;
  message: string;
  sample_count?: number;
  sample_data?: unknown[];
}

// 同步日志
export interface SyncLogResponse {
  id: number;
  source_id: string;
  started_at: string;
  completed_at?: string;
  status: 'running' | 'success' | 'failed';
  resources_synced: number;
  resources_added: number;
  resources_updated: number;
  resources_deleted: number;
  error_message?: string;
}

// 同步日志列表响应
export interface SyncLogListResponse {
  logs: SyncLogResponse[];
  total: number;
}

// 外部数据源API服务
export const externalSourceService = {
  // 列出所有外部数据源
  listExternalSources: async (): Promise<ExternalSourceListResponse> => {
    return api.get('/cmdb/external-sources');
  },

  // 创建外部数据源
  createExternalSource: async (data: CreateExternalSourceRequest): Promise<ExternalSourceResponse> => {
    return api.post('/cmdb/external-sources', data);
  },

  // 获取外部数据源详情
  getExternalSource: async (sourceId: string): Promise<ExternalSourceResponse> => {
    return api.get(`/cmdb/external-sources/${sourceId}`);
  },

  // 更新外部数据源
  updateExternalSource: async (sourceId: string, data: UpdateExternalSourceRequest): Promise<ExternalSourceResponse> => {
    return api.put(`/cmdb/external-sources/${sourceId}`, data);
  },

  // 删除外部数据源
  deleteExternalSource: async (sourceId: string): Promise<void> => {
    return api.delete(`/cmdb/external-sources/${sourceId}`);
  },

  // 手动触发同步
  syncExternalSource: async (sourceId: string): Promise<{ message: string }> => {
    return api.post(`/cmdb/external-sources/${sourceId}/sync`);
  },

  // 测试连接
  testConnection: async (sourceId: string): Promise<TestConnectionResponse> => {
    return api.post(`/cmdb/external-sources/${sourceId}/test`);
  },

  // 获取同步日志
  getSyncLogs: async (sourceId: string, limit?: number): Promise<SyncLogListResponse> => {
    const params = limit ? `?limit=${limit}` : '';
    return api.get(`/cmdb/external-sources/${sourceId}/sync-logs${params}`);
  },
};

// Embedding 状态
export interface EmbeddingStatus {
  workspace_id: string;
  total_resources: number;
  with_embedding: number;
  pending_tasks: number;
  processing_tasks: number;
  failed_tasks: number;
  progress: number;
  estimated_time: string;
}

// 获取 Workspace 的 embedding 状态
export const getWorkspaceEmbeddingStatus = async (workspaceId: string): Promise<EmbeddingStatus> => {
  // api 拦截器已经返回 response.data，所以直接访问 data 属性
  const response: any = await api.get(`/workspaces/${workspaceId}/embedding-status`);
  console.log('[getWorkspaceEmbeddingStatus] response:', response);
  // 后端返回 {code: 200, data: {...}}，api 拦截器返回 response.data
  // 所以 response 可能是 {code: 200, data: {...}} 或直接是 {...}
  if (response && response.data) {
    return response.data;
  }
  return response;
};

// 获取外部数据源的 embedding 状态（使用特殊的 __external__ workspace_id）
export const getExternalSourceEmbeddingStatus = async (): Promise<EmbeddingStatus> => {
  return getWorkspaceEmbeddingStatus('__external__');
};

// 重建 Workspace 的 embedding（全量重建）
export const rebuildWorkspaceEmbedding = async (workspaceId: string): Promise<EmbeddingStatus> => {
  const response: any = await api.post(`/workspaces/${workspaceId}/embedding/rebuild`);
  if (response && response.data) {
    return response.data;
  }
  return response;
};

// 重建外部数据源的 embedding（全量重建）
export const rebuildExternalSourceEmbedding = async (): Promise<EmbeddingStatus> => {
  return rebuildWorkspaceEmbedding('__external__');
};

// 获取全局 embedding 配置状态
export const getEmbeddingConfigStatus = async (): Promise<{
  configured: boolean;
  has_api_key: boolean;
  model_id: string;
  service_type: string;
  message: string;
  help?: string;
}> => {
  // api 拦截器已经返回 response.data，所以直接访问 data 属性
  const response: any = await api.get('/ai/embedding/config-status');
  return response.data || response;
};

// ===== Embedding 缓存相关 API =====

// 缓存统计信息
export interface EmbeddingCacheStats {
  total_count: number;
  total_hits: number;
  avg_hit_count: number;
  memory_cache_size: number;
  oldest_entry?: string;
  newest_entry?: string;
  top_keywords?: { keyword: string; hit_count: number }[];
}

// 预热进度
export interface WarmupProgress {
  is_running: boolean;
  total_keywords: number;
  processed_count: number;
  cached_count: number;
  new_count: number;
  failed_count: number;
  current_batch: number;
  total_batches: number;
  started_at?: string;
  completed_at?: string;
  last_error?: string;
  internal_count: number;  // 内部 CMDB 关键词数
  external_count: number;  // 外部 CMDB 关键词数
  static_count: number;    // 静态词库数
}

// 预热 Embedding 缓存
export const warmupEmbeddingCache = async (force: boolean = false): Promise<{ message: string }> => {
  const response: any = await api.post(`/admin/embedding-cache/warmup?force=${force}`);
  return response.data || response;
};

// 获取预热进度
export const getWarmupProgress = async (): Promise<WarmupProgress> => {
  const response: any = await api.get('/admin/embedding-cache/warmup/progress');
  return response.data || response;
};

// 获取缓存统计
export const getEmbeddingCacheStats = async (): Promise<EmbeddingCacheStats> => {
  const response: any = await api.get('/admin/embedding-cache/stats');
  return response.data || response;
};

// 清空缓存
export const clearEmbeddingCache = async (): Promise<{ message: string }> => {
  const response: any = await api.delete('/admin/embedding-cache/clear');
  return response.data || response;
};

export default cmdbService;
