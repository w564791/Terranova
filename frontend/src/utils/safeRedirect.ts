/**
 * 登录后跳转目标校验(防 open redirect):只接受同源相对路径——以 '/' 开头、
 * 不以 '//' 或 '/\' 开头、不含控制字符;否则返回 fallback。
 */
export function safeRedirectPath(raw: string | null | undefined, fallback = '/'): string {
  if (!raw) return fallback;
  let p = raw.trim();
  // 兼容被 encodeURIComponent 过的值(旧 returnUrl 参数)
  if (p.startsWith('%2F') || p.startsWith('%2f')) {
    try {
      p = decodeURIComponent(p);
    } catch {
      return fallback;
    }
  }
  if (!p.startsWith('/') || p.startsWith('//') || p.startsWith('/\\')) return fallback;
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f]/.test(p)) return fallback;
  // 再用 URL 解析确认仍是同源(兜底各种奇怪写法)
  try {
    const u = new URL(p, window.location.origin);
    if (u.origin !== window.location.origin) return fallback;
    return u.pathname + u.search + u.hash;
  } catch {
    return fallback;
  }
}

/** 登录页路径(与 App.tsx 路由一致) */
export const LOGIN_PATH = '/login';

/** 当前是否在登录 / 初始化 / MFA 等认证页(这些页面上不做 401 跳转,避免循环) */
export function isOnAuthPage(pathname: string = window.location.pathname): boolean {
  return pathname.includes('/login') || pathname.includes('/setup') || pathname.includes('/mfa') || pathname.includes('/sso/callback');
}
