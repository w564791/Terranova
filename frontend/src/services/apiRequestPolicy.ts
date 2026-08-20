/**
 * API 请求策略的纯函数。
 *
 * 保持 URL 归一化、组织范围识别与受信任 API origin 的判断在同一处，避免
 * axios、SSE 和下载等原始 fetch 调用各自漂移。
 */
export const API_PATH_PREFIX = '/api/v1';

const organizationScopedRoutePrefixes = new Set([
  'iam',
  'workspaces',
  'projects',
  'variable-sets',
  'modules',
  'module-demos',
  'run-tasks',
  'agents',
  'agent-pools',
  'provider-templates',
  'terraform-versions',
  'ai-configs',
  'users',
  'dashboard',
  'ai',
  'cmdb',
]);

function getPathname(input: string): string {
  try {
    return new URL(input, 'http://terranova.invalid').pathname;
  } catch {
    return input.split(/[?#]/, 1)[0] || '/';
  }
}

function hasApiPathPrefix(pathname: string): boolean {
  return /^\/?api\/v1(?:\/|$)/.test(pathname);
}

function stripApiPathPrefix(pathname: string): string {
  return pathname.replace(/^\/?api\/v1(?=\/|$)/, '') || '/';
}

function isLegacyLocalApiOrigin(url: URL): boolean {
  return url.hostname === 'localhost' || url.hostname === '127.0.0.1' || url.hostname === '[::1]';
}

/** 判断一个 API 路径是否要求 active organization。 */
export function isOrganizationScopedApiPath(input: string): boolean {
  const pathname = getPathname(input);
  const relativePath = stripApiPathPrefix(pathname).replace(/^\/+/, '');
  const routePrefix = relativePath.split('/', 1)[0];
  return organizationScopedRoutePrefixes.has(routePrefix);
}

/**
 * 将相对 API 路径和历史硬编码的 /api/v1 URL 归一化到当前 API base URL。
 * 其它绝对 URL 原样保留，供调用方请求公开资源；认证信息不会被转发给它们。
 */
export function resolveApiRequestUrl(input: string | URL, apiBaseUrl: string): URL {
  const rawUrl = input instanceof URL ? input.toString() : input;
  const normalizedApiBaseUrl = apiBaseUrl.replace(/\/$/, '');

  try {
    const parsed = new URL(rawUrl);
    if (
      hasApiPathPrefix(parsed.pathname) &&
      (isTrustedApiOrigin(parsed, apiBaseUrl) || isLegacyLocalApiOrigin(parsed))
    ) {
      const path = stripApiPathPrefix(parsed.pathname);
      return new URL(normalizedApiBaseUrl + path + parsed.search);
    }
    return parsed;
  } catch {
    let path = rawUrl;
    const pathname = getPathname(path);
    if (hasApiPathPrefix(pathname)) {
      path = stripApiPathPrefix(path);
    }
    if (!path.startsWith('/')) {
      path = '/' + path;
    }
    return new URL(normalizedApiBaseUrl + path);
  }
}

/** 仅当前配置的 API origin 可以接收本应用的 Bearer token。 */
export function isTrustedApiOrigin(requestUrl: URL, apiBaseUrl: string): boolean {
  return requestUrl.origin === new URL(apiBaseUrl).origin;
}

/**
 * 按 axios 的 URL 规则解析请求：绝对 URL 会覆盖 baseURL，不能按 /api/v1
 * 路径重写，否则外域 URL 可能被误判为受信任 API。
 */
export function isTrustedApiRequestUrl(input: string | URL, apiBaseUrl: string): boolean {
  const rawUrl = input instanceof URL ? input.toString() : input;
  let requestUrl: URL;
  try {
    requestUrl = new URL(rawUrl);
  } catch {
    requestUrl = new URL(rawUrl, apiBaseUrl);
  }
  return isTrustedApiOrigin(requestUrl, apiBaseUrl);
}
