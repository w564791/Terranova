import axios from 'axios';
import {
  isOrganizationScopedApiPath,
  isTrustedApiOrigin,
  isTrustedApiRequestUrl,
  resolveApiRequestUrl,
} from './apiRequestPolicy';

// 自动根据当前访问的域名/IP构建 API 地址
// 如果设置了环境变量，则使用环境变量
// 否则自动使用当前访问的 host + 8080 端口
const getApiBaseUrl = () => {
  // 优先使用环境变量配置
  if (import.meta.env.VITE_API_BASE_URL) {
    return import.meta.env.VITE_API_BASE_URL;
  }
  
  // 自动根据当前访问的域名/IP构建 API 地址
  const protocol = window.location.protocol; // http: 或 https:
  const hostname = window.location.hostname; // localhost 或 IP 或域名
  
  // 如果是开发环境的默认端口（5173），使用 8080 作为 API 端口
  // 如果是生产环境，假设 API 在同一域名下
  const apiPort = window.location.port === '5173' ? '8080' : window.location.port;
  const host = apiPort ? `${hostname}:${apiPort}` : hostname;
  
  return `${protocol}//${host}/api/v1`;
};

export const API_BASE_URL = getApiBaseUrl();

console.log('API Base URL:', API_BASE_URL);

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

/** IAM 鉴权 org：与后端 auth_org_id / query org_id 对齐 */
export const IAM_ORG_STORAGE_KEY = 'iam_auth_org_id';
export const IAM_ORG_CHANGE_EVENT = 'iam:active-org-changed';
const IAM_ORG_OWNER_STORAGE_KEY = 'iam_auth_org_owner_id';

function notifyAuthOrgChange(): void {
  window.dispatchEvent(new Event(IAM_ORG_CHANGE_EVENT));
}

export function getAuthOrgId(): number | null {
  const raw = localStorage.getItem(IAM_ORG_STORAGE_KEY);
  if (!raw) return null;
  const n = Number(raw);
  return Number.isFinite(n) && n > 0 ? n : null;
}

export function setAuthOrgId(orgId: number): void {
  if (orgId > 0) {
    localStorage.setItem(IAM_ORG_STORAGE_KEY, String(orgId));
    notifyAuthOrgChange();
  }
}

/**
 * 将 active organization 与登录用户绑定。这样浏览器中上一个用户留下的
 * org 上下文不会被下一个用户继承；同一用户刷新页面时则保留选择。
 */
export function initializeAuthOrgForUser(userId: number | string): void {
  const ownerId = String(userId);
  const previousOwnerId = localStorage.getItem(IAM_ORG_OWNER_STORAGE_KEY);
  if (previousOwnerId !== ownerId) {
    clearAuthOrgId();
  }
  localStorage.setItem(IAM_ORG_OWNER_STORAGE_KEY, ownerId);
}

/** 仅清理 active org；用户身份归属标记仍保留，供同一会话后续重新 bootstrap。 */
export function clearActiveOrgId(): void {
  localStorage.removeItem(IAM_ORG_STORAGE_KEY);
  notifyAuthOrgChange();
}

/** 清理当前会话的组织上下文（logout、token 失效和登录失败均应调用）。 */
export function clearAuthOrgId(): void {
  clearActiveOrgId();
  localStorage.removeItem(IAM_ORG_OWNER_STORAGE_KEY);
}

/** 需要自动附带 org_id 的业务路径（与组织级 IAM 对齐） */
function shouldAttachAuthOrgId(url: string): boolean {
  return isOrganizationScopedApiPath(url);
}

/**
 * 供仍需读取原始 Response（SSE、二进制下载或旧 fetch 调用）的 API fetch。
 * 它与 axios client 使用同一 base URL、Bearer token 和 active org 规则，避免
 * 页面绕过 axios 后漏传 org_id 或误请求 Vite 开发服务器。
 */
export async function apiFetch(input: string | URL, init: RequestInit = {}): Promise<Response> {
  const requestUrl = resolveApiRequestUrl(input, API_BASE_URL);
  const isTrustedApiRequest = isTrustedApiOrigin(requestUrl, API_BASE_URL);

  if (isTrustedApiRequest && shouldAttachAuthOrgId(requestUrl.pathname)) {
    const orgId = getAuthOrgId();
    if (orgId != null && !requestUrl.searchParams.has('org_id')) {
      requestUrl.searchParams.set('org_id', String(orgId));
    }
  }

  const headers = new Headers(init.headers);
  const token = localStorage.getItem('token');
  if (isTrustedApiRequest) {
    if (token && !headers.has('Authorization')) {
      headers.set('Authorization', `Bearer ${token}`);
    }
  } else if (/^Bearer\s+/i.test(headers.get('Authorization') || '')) {
    // apiFetch 允许读取公开的绝对 URL，但绝不能把本应用的 bearer 泄露给它们。
    headers.delete('Authorization');
  }

  const response = await fetch(requestUrl.toString(), {
    ...init,
    headers,
  });

  // Raw Response 调用也要遵守与 axios 相同的会话失效语义，避免 token 已过期
  // 时 active-org 仍残留在浏览器里。
  const onAuthPage =
    window.location.pathname.includes('/login') ||
    window.location.pathname.includes('/setup') ||
    window.location.pathname.includes('/mfa');
  if (isTrustedApiRequest && response.status === 401 && !onAuthPage) {
    localStorage.removeItem('token');
    clearAuthOrgId();
    window.location.href = '/login';
  }
  return response;
}

// 请求拦截器 - 添加认证 token；组织级 API 自动附带 org_id
api.interceptors.request.use(
  (config) => {
    const isTrustedApiRequest = isTrustedApiRequestUrl(
      config.url || '',
      config.baseURL || API_BASE_URL,
    );

    if (!isTrustedApiRequest) {
      const authorization = config.headers.get('Authorization');
      if (typeof authorization === 'string' && /^Bearer\s+/i.test(authorization)) {
        // axios 允许绝对 URL 覆盖 baseURL；外域请求不能继承平台凭证。
        config.headers.delete('Authorization');
      }
      return config;
    }

    // 每次请求时都重新从localStorage读取token,确保获取最新值
    const token = localStorage.getItem('token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    } else {
      console.warn('No token found in localStorage');
    }

    // 后端组织级 IAM 在多租户下要求 org_id；业务 API（workspaces 等）与 /iam/* 对齐
    const url = config.url || '';
    const needsOrgId = shouldAttachAuthOrgId(url);
    if (needsOrgId) {
      const orgId = getAuthOrgId();
      if (orgId != null) {
        const params = config.params;
        if (params instanceof URLSearchParams) {
          if (!params.has('org_id')) {
            params.set('org_id', String(orgId));
          }
        } else {
          const p = (params && typeof params === 'object' ? { ...params } : {}) as Record<string, unknown>;
          if (p.org_id === undefined || p.org_id === null || p.org_id === '') {
            p.org_id = orgId;
          }
          config.params = p;
        }
      }
    }

    return config;
  },
  (error) => {
    console.error('Request interceptor error:', error);
    return Promise.reject(error);
  }
);

// 响应拦截器 - 处理错误
api.interceptors.response.use(
  (response) => {
    // 调试日志：检查 CMDB API 响应
    if (response.config.url?.includes('cmdb')) {
      console.log('[api.ts] CMDB API response:', response.config.url, response.data);
    }
    return response.data;
  },
  (error) => {
    // MFA验证相关的401错误不应该重定向到登录页
    const isMFAVerify = error.config?.url?.includes('/auth/mfa/verify');
    const isAuthMe = error.config?.url?.includes('/auth/me');
    const isDashboard = error.config?.url?.includes('/dashboard');
    
    // 不要在以下情况下自动重定向到登录页：
    // 1. MFA验证请求
    // 2. /auth/me 请求（由AuthProvider处理）
    // 3. Dashboard请求（可能是token还没保存完成）
    // 4. 已经在登录/setup/mfa页面
    const shouldNotRedirect = isMFAVerify || isAuthMe || isDashboard ||
      window.location.pathname.includes('/login') || 
      window.location.pathname.includes('/setup') || 
      window.location.pathname.includes('/mfa');
    
    if (error.response?.status === 401 && !shouldNotRedirect) {
      console.log('[api.ts] 401 error, redirecting to login');
      localStorage.removeItem('token');
      clearAuthOrgId();
      window.location.href = '/login';
    }
    // 提取错误消息：优先使用 error.response.data.error，其次使用 error.message
    const errorMessage = error.response?.data?.error || error.response?.data?.message || error.message || '未知错误';
    return Promise.reject(errorMessage);
  }
);

export default api;
