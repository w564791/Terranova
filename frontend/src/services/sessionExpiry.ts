/**
 * 统一的会话过期(HTTP 401)处理:清 token / active org / Redux 登录态,
 * 只提示一次"登录已过期，请重新登录",并带 ?redirect=<当前路径+查询> 跳转登录页。
 * 已在认证页时不跳转(防循环);并发多个 401 只处理第一次。
 */
import { message } from 'antd';
import { clearAuthOrgId } from './api';
import { LOGIN_PATH, isOnAuthPage } from '../utils/safeRedirect';

const SESSION_EXPIRED_KEY = 'session-expired';
let handling = false;

/** 认证接口本身的 401(如密码错误、MFA 码错误)由页面内联展示,不当作会话过期 */
export function isAuthEndpoint(url: string | undefined): boolean {
  if (!url) return false;
  return (
    url.includes('/auth/login') ||
    url.includes('/auth/sso/') ||
    url.includes('/auth/mfa/') ||
    url.includes('/auth/register') ||
    url.includes('/auth/refresh')
  );
}

export function handleSessionExpired(): void {
  if (handling || isOnAuthPage()) return;
  handling = true;

  localStorage.removeItem('token');
  clearAuthOrgId();
  message.error({ content: '登录已过期，请重新登录', key: SESSION_EXPIRED_KEY, duration: 3 });

  const current = window.location.pathname + window.location.search;
  const target = current && current !== '/' ? `${LOGIN_PATH}?redirect=${encodeURIComponent(current)}` : LOGIN_PATH;
  const go = () => {
    window.location.href = target;
  };
  // 稍等让提示渲染出来,再清 Redux 登录态并整页跳转(先清 Redux 会让 ProtectedRoute 抢先 SPA 跳转)。
  // 复用 Redux logout;动态 import 避免 api <-> store 循环依赖。整页跳转本身也会重置内存状态。
  window.setTimeout(() => {
    import('../store')
      .then(({ store }) => import('../store/slices/authSlice').then(({ logout }) => store.dispatch(logout())))
      .catch(() => undefined)
      .finally(go);
  }, 800);
}
