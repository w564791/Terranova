import api from './api';

// MFA状态
export interface MFAStatus {
  mfa_enabled: boolean;
  mfa_verified_at?: string;
  backup_codes_count: number;
  enforcement_policy: string;
  is_required: boolean;
  is_locked: boolean;
  locked_until?: string;
}

// MFA设置响应
export interface MFASetupResponse {
  secret: string;
  qr_code: string;
  otpauth_uri: string;
  backup_codes: string[];
}

// MFA配置
export interface MFAConfig {
  enabled: boolean;
  enforcement: string;
  enforcement_enabled_at?: string;
  issuer: string;
  grace_period_days: number;
  max_failed_attempts: number;
  lockout_duration_minutes: number;
}

// MFA统计
export interface MFAStatistics {
  total_users: number;
  mfa_enabled_users: number;
  mfa_pending_users: number;
}

// 获取当前用户MFA状态
export const getMFAStatus = () => {
  return api.get<{ code: number; data: MFAStatus }>('/user/mfa/status');
};

// 初始化MFA设置
export const setupMFA = () => {
  return api.post<{ code: number; data: MFASetupResponse }>('/user/mfa/setup');
};

// 验证并启用MFA
export const verifyAndEnableMFA = (code: string) => {
  return api.post<{ code: number; message: string; data: { mfa_enabled: boolean; mfa_verified_at: string } }>(
    '/user/mfa/verify',
    { code }
  );
};

// 禁用MFA
export const disableMFA = (code: string, password: string) => {
  return api.post<{ code: number; message: string }>('/user/mfa/disable', { code, password });
};

// 重新生成备用恢复码
export const regenerateBackupCodes = (code: string) => {
  return api.post<{ code: number; data: { backup_codes: string[] } }>(
    '/user/mfa/backup-codes/regenerate',
    { code }
  );
};

// 登录时MFA验证
export const verifyMFALogin = (mfa_token: string, code: string) => {
  return api.post<{
    code: number;
    message: string;
    data: {
      token: string;
      expires_at: string;
      user: {
        id: string;
        username: string;
        email: string;
        role: string;
      };
    };
  }>('/auth/mfa/verify', { mfa_token, code });
};

// 管理员API

// 获取MFA全局配置
export const getMFAConfig = () => {
  return api.get<{ code: number; data: { config: MFAConfig; statistics: MFAStatistics } }>(
    '/global/settings/mfa'
  );
};

// 更新MFA全局配置
export const updateMFAConfig = (config: Partial<MFAConfig>) => {
  return api.put<{ code: number; message: string }>('/global/settings/mfa', config);
};

// 获取指定用户MFA状态
export const getUserMFAStatus = (userId: string) => {
  return api.get<{ code: number; data: MFAStatus }>(`/admin/users/${userId}/mfa/status`);
};

// 重置用户MFA
export const resetUserMFA = (userId: string) => {
  return api.post<{ code: number; message: string }>(`/admin/users/${userId}/mfa/reset`);
};
