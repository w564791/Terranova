import axios from 'axios';

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
  
  return `${protocol}//${hostname}:${apiPort}/api/v1`;
};

const API_BASE_URL = getApiBaseUrl();

console.log('🔗 API Base URL:', API_BASE_URL);

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// 请求拦截器 - 添加认证token
api.interceptors.request.use(
  (config) => {
    // 每次请求时都重新从localStorage读取token,确保获取最新值
    const token = localStorage.getItem('token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
      // console.log('🔐 Adding Authorization header:', token.substring(0, 20) + '...');
    } else {
      console.warn(' No token found in localStorage');
    }
    return config;
  },
  (error) => {
    console.error('❌ Request interceptor error:', error);
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
    if (error.response?.status === 401 && !window.location.pathname.includes('/login')) {
      localStorage.removeItem('token');
      window.location.href = '/login';
    }
    // 提取错误消息：优先使用 error.response.data.error，其次使用 error.message
    const errorMessage = error.response?.data?.error || error.response?.data?.message || error.message || '未知错误';
    return Promise.reject(errorMessage);
  }
);

export default api;
