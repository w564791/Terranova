import api from './api';

// 编辑者信息
export interface EditorInfo {
  user_id: number;
  user_name: string;
  session_id: string;
  is_same_user: boolean;
  is_current_session: boolean;
  last_heartbeat: string;
  time_since_heartbeat: number;
}

// 编辑状态响应
export interface EditingStatusResponse {
  is_locked: boolean;
  current_version: number;
  editors: EditorInfo[];
}

// 草稿信息
export interface DriftInfo {
  id: number;
  resource_id: number;
  user_id: number;
  session_id: string;
  drift_content: {
    formData: any;
    changeSummary: string;
  };
  base_version: number;
  status: string;
  created_at: string;
  updated_at: string;
}

// 开始编辑响应
export interface StartEditingResponse {
  lock: any;
  drift?: DriftInfo;
  other_editors: EditorInfo[];
  has_drift: boolean;
  has_version_conflict: boolean;
}

// 资源编辑协作服务
export class ResourceEditingService {
  /**
   * 开始编辑
   */
  static async startEditing(
    workspaceId: number | string,
    resourceId: number,
    sessionId: string
  ): Promise<StartEditingResponse> {
    const response = await api.post(
      `/workspaces/${workspaceId}/resources/${resourceId}/editing/start`,
      { session_id: sessionId }
    );
    console.log('🔍 StartEditing API Response:', response.data);
    // 后端返回格式: {success: true, data: {...}}
    return response.data.data || response.data;
  }

  /**
   * 心跳更新
   */
  static async heartbeat(
    workspaceId: number | string,
    resourceId: number,
    sessionId: string
  ): Promise<void> {
    await api.post(
      `/workspaces/${workspaceId}/resources/${resourceId}/editing/heartbeat`,
      { session_id: sessionId }
    );
  }

  /**
   * 结束编辑
   */
  static async endEditing(
    workspaceId: number | string,
    resourceId: number,
    sessionId: string
  ): Promise<void> {
    await api.post(
      `/workspaces/${workspaceId}/resources/${resourceId}/editing/end`,
      { session_id: sessionId }
    );
  }

  /**
   * 获取编辑状态
   */
  static async getEditingStatus(
    workspaceId: number | string,
    resourceId: number,
    sessionId: string
  ): Promise<EditingStatusResponse> {
    const response = await api.get(
      `/workspaces/${workspaceId}/resources/${resourceId}/editing/status`,
      { params: { session_id: sessionId } }
    );
    return response.data.data || response.data;
  }

  /**
   * 保存草稿
   */
  static async saveDrift(
    workspaceId: number | string,
    resourceId: number,
    sessionId: string,
    driftContent: { formData: any; changeSummary: string }
  ): Promise<{ drift_id: number; base_version: number; saved_at: string }> {
    const response = await api.post(
      `/workspaces/${workspaceId}/resources/${resourceId}/drift/save`,
      {
        session_id: sessionId,
        drift_content: driftContent,
      }
    );
    return response.data.data;
  }

  /**
   * 获取草稿
   */
  static async getDrift(
    workspaceId: number | string,
    resourceId: number,
    sessionId: string
  ): Promise<{
    drift: DriftInfo | null;
    current_version: number;
    has_version_conflict: boolean;
  } | null> {
    const response = await api.get(
      `/workspaces/${workspaceId}/resources/${resourceId}/drift`,
      { params: { session_id: sessionId } }
    );
    return response.data.data;
  }

  /**
   * 删除草稿
   */
  static async deleteDrift(
    workspaceId: number | string,
    resourceId: number,
    sessionId: string
  ): Promise<void> {
    await api.delete(
      `/workspaces/${workspaceId}/resources/${resourceId}/drift`,
      { params: { session_id: sessionId } }
    );
  }

  /**
   * 接管编辑
   */
  static async takeoverEditing(
    workspaceId: number | string,
    resourceId: number,
    newSessionId: string,
    oldSessionId: string
  ): Promise<void> {
    await api.post(
      `/workspaces/${workspaceId}/resources/${resourceId}/drift/takeover`,
      {
        session_id: newSessionId,
        old_session_id: oldSessionId,
      }
    );
  }
}

/**
 * 生成UUID v4
 */
export function generateUUID(): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

/**
 * 格式化时间差
 */
export function formatTimeAgo(timestamp: string): string {
  const now = new Date();
  // 处理时区问题:如果timestamp包含+08:00等时区信息,直接解析
  // 如果不包含,假设是UTC时间
  const past = new Date(timestamp);
  
  // 计算时间差(毫秒)
  const diffMs = now.getTime() - past.getTime();
  const seconds = Math.floor(diffMs / 1000);

  // 如果时间差为负数或过大,说明时区有问题
  if (seconds < 0 || seconds > 86400 * 365) {
    return '刚刚';
  }

  if (seconds < 60) {
    return `${seconds}秒前`;
  }

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    return `${minutes}分钟前`;
  }

  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return `${hours}小时前`;
  }

  const days = Math.floor(hours / 24);
  return `${days}天前`;
}
