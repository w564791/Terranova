import api from './api';

// Drift 配置
export interface DriftConfig {
  drift_check_enabled: boolean;
  drift_check_start_time: string;  // HH:MM 格式
  drift_check_end_time: string;    // HH:MM 格式
  drift_check_interval: number;    // 分钟
  // 继续检测设置
  continue_on_failure: boolean;    // 失败后继续检测
  continue_on_success: boolean;    // 成功后继续检测
}

// Drift 检测结果
export interface DriftResult {
  id: number;
  workspace_id: string;
  current_task_id?: number;
  has_drift: boolean;
  drift_count: number;
  total_resources: number;
  drift_details: DriftDetails | null;
  check_status: 'success' | 'failed' | 'pending' | 'running' | 'skipped';
  error_message?: string;
  last_check_at: string | null;
  created_at: string;
  updated_at: string;
}

// Drift 详情
export interface DriftDetails {
  resources: DriftResource[];
  summary: {
    total_resources: number;
    drifted_resources: number;
    total_children: number;
    drifted_children: number;
  };
}

// 资源 Drift 信息
export interface DriftResource {
  resource_id: string;
  resource_name: string;
  has_drift: boolean;
  drifted_children: DriftedChild[];
}

// 子资源 Drift 信息
export interface DriftedChild {
  address: string;
  change_type: 'update' | 'create' | 'delete' | 'read';
  before: Record<string, unknown>;
  after: Record<string, unknown>;
}

// 资源 Drift 状态
export interface ResourceDriftStatus {
  resource_id: string;
  resource_name: string;
  drift_status: 'synced' | 'drifted' | 'unapplied' | 'unknown';
  has_drift: boolean;
  drifted_children_count: number;
  last_applied_at: string | null;
  last_check_at: string | null;
}

// 获取 Drift 配置
export const getDriftConfig = async (workspaceId: string): Promise<DriftConfig> => {
  // api 拦截器已经返回 response.data，所以直接返回
  return api.get(`/workspaces/${workspaceId}/drift-config`);
};

// 更新 Drift 配置
export const updateDriftConfig = async (workspaceId: string, config: DriftConfig): Promise<void> => {
  await api.put(`/workspaces/${workspaceId}/drift-config`, config);
};

// 获取 Drift 状态
export const getDriftStatus = async (workspaceId: string): Promise<DriftResult> => {
  // api 拦截器已经返回 response.data，所以直接返回
  return api.get(`/workspaces/${workspaceId}/drift-status`);
};

// 触发 Drift 检测的响应
export interface TriggerDriftCheckResponse {
  task_id: number;
  message: string;
}

// 手动触发 Drift 检测
export const triggerDriftCheck = async (workspaceId: string): Promise<TriggerDriftCheckResponse> => {
  return api.post(`/workspaces/${workspaceId}/drift-check`);
};

// 取消 Drift 检测
export const cancelDriftCheck = async (workspaceId: string): Promise<void> => {
  await api.delete(`/workspaces/${workspaceId}/drift-check`);
};

// 获取资源 Drift 状态列表
export const getResourceDriftStatuses = async (workspaceId: string): Promise<ResourceDriftStatus[]> => {
  // api 拦截器已经返回 response.data，所以直接返回
  return api.get(`/workspaces/${workspaceId}/resources-drift`);
};
