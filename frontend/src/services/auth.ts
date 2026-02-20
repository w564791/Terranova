import api from './api';

export interface LoginRequest {
  username: string;
  password: string;
}

export interface RegisterRequest {
  username: string;
  email: string;
  password: string;
}

export interface ResetPasswordRequest {
  current_password: string;
  new_password: string;
}

export interface SetupInitRequest {
  username: string;
  email: string;
  password: string;
}

export interface SetupStatusResponse {
  initialized: boolean;
  has_admin: boolean;
  message?: string;
}

export const authService = {
  login: (data: LoginRequest) => api.post('/auth/login', data),
  register: (data: RegisterRequest) => api.post('/auth/register', data),
  resetPassword: (data: ResetPasswordRequest) => api.post('/user/reset-password', data),
  logout: () => api.post('/auth/logout'),
  refreshToken: () => api.post('/auth/refresh'),
};

export const setupService = {
  getStatus: () => api.get<{ code: number; data: SetupStatusResponse }>('/setup/status'),
  initAdmin: (data: SetupInitRequest) => api.post('/setup/init', data),
};
