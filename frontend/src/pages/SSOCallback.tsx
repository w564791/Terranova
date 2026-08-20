import React, { useEffect, useState, useRef } from 'react';
import { useNavigate, useLocation, useSearchParams } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { loginSuccess, logout } from '../store/slices/authSlice';
import api from '../services/api';
import { isValidAuthenticatedUserId } from '../services/authIdentity';
import { ssoService } from '../services/ssoService';

const SSOCallback: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const dispatch = useDispatch();
  const [searchParams] = useSearchParams();
  const [error, setError] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const processedRef = useRef(false); // 防止 StrictMode 重复调用

  useEffect(() => {
    if (processedRef.current) return;
    processedRef.current = true;

    const handleCallback = async () => {
      // 优先 hash（防 Referer/日志泄露），兼容历史 query
      const hashParams = new URLSearchParams(location.hash.replace(/^#/, ''));

      // MFA 参数：hash 优先，兼容旧 query
      const redirectMfaRequired = hashParams.get('mfa_required') || searchParams.get('mfa_required');
      const redirectMfaSetupRequired = hashParams.get('mfa_setup_required') || searchParams.get('mfa_setup_required');
      const redirectMfaToken = hashParams.get('mfa_token') || searchParams.get('mfa_token');

      if (redirectMfaRequired === 'true' && redirectMfaToken) {
        navigate('/login/mfa', {
          state: {
            mfa_token: redirectMfaToken,
            username: '',
            required_backup_codes: 1,
          },
          replace: true,
        });
        return;
      }

      if (redirectMfaSetupRequired === 'true' && redirectMfaToken) {
        navigate('/setup/mfa', {
          state: {
            mfa_token: redirectMfaToken,
            username: '',
            from_login: true,
          },
          replace: true,
        });
        return;
      }

      // 检查是否有直接传递的 token（重定向模式，无需 MFA）
      const directToken = hashParams.get('token') || searchParams.get('token');
      if (directToken) {
        // 从地址栏移除 token；后续请求只读取 localStorage 中刚设置的值。
        window.history.replaceState(null, '', location.pathname);
        // 先清理可能残留的前一用户状态，再用 token 获取真实身份。不能用占位
        // user 登录，否则 active-org owner 会被错误绑定为同一个 id。
        dispatch(logout());
        localStorage.setItem('token', directToken);
        try {
          const profileResponse: any = await api.get('/auth/me');
          const user = profileResponse?.data || profileResponse;
          if (!user || !isValidAuthenticatedUserId(user.id)) {
            throw new Error('SSO 登录未返回有效用户信息');
          }

          dispatch(loginSuccess({ user, token: directToken }));
          localStorage.removeItem('sso_provider');
          navigate('/', { replace: true });
        } catch (err: any) {
          dispatch(logout());
          console.error('Failed to load SSO user profile:', err);
          setError(
            typeof err === 'string'
              ? err
              : err?.message || '无法验证 SSO 登录身份，请重试',
          );
          setLoading(false);
        }
        return;
      }

      // 检查是否有错误
      const errorParam = searchParams.get('error');
      if (errorParam) {
        const errorDesc = searchParams.get('error_description') || 'SSO 登录失败';
        setError(errorDesc);
        setLoading(false);
        return;
      }

      // API 模式：用 code 和 state 换取 token
      const code = searchParams.get('code');
      const state = searchParams.get('state');

      if (!code || !state) {
        setError('缺少必要的回调参数');
        setLoading(false);
        return;
      }

      // 从 URL 路径中提取 provider key
      // URL 格式: /sso/callback/:provider 或通过 state 关联
      // 这里我们从 localStorage 中获取 provider（在发起登录时保存）
      const providerKey = localStorage.getItem('sso_provider') || '';
      if (!providerKey) {
        setError('无法确定 SSO 提供商');
        setLoading(false);
        return;
      }

      try {
        const response: any = await ssoService.callback(providerKey, code, state);
        const data = response.data || response;

        // 检查是否需要 MFA 验证
        if (data.mfa_required) {
          localStorage.removeItem('sso_provider');
          navigate('/login/mfa', {
            state: {
              mfa_token: data.mfa_token,
              username: data.user?.username || '',
              required_backup_codes: data.required_backup_codes !== undefined ? data.required_backup_codes : 1,
            },
            replace: true,
          });
          return;
        }

        // 检查是否需要设置 MFA（使用 mfa_token，不发放 JWT）
        if (data.mfa_setup_required) {
          localStorage.removeItem('sso_provider');
          navigate('/setup/mfa', {
            state: {
              mfa_token: data.mfa_token,
              username: data.user?.username || '',
              from_login: true,
            },
            replace: true,
          });
          return;
        }

        if (data.token && data.user && isValidAuthenticatedUserId(data.user.id)) {
          localStorage.setItem('token', data.token);
          localStorage.removeItem('sso_provider');

          dispatch(loginSuccess({
            user: data.user,
            token: data.token,
          }));

          // 跳转到首页或之前的页面
          const returnUrl = localStorage.getItem('sso_return_url') || '/';
          localStorage.removeItem('sso_return_url');
          navigate(returnUrl, { replace: true });
        } else {
          // Both SSO callback modes must establish a real principal before
          // marking the browser authenticated. In particular, do not let a
          // provider response with an id: 0 placeholder retain another user's
          // active organization context.
          dispatch(logout());
          setError('登录响应数据不完整或未返回有效用户信息');
        }
      } catch (err: any) {
        console.error('SSO callback error:', err);
        setError(err.message || 'SSO 登录失败，请重试');
      } finally {
        setLoading(false);
      }
    };

    handleCallback();
  }, [location, searchParams, dispatch, navigate]);

  if (loading) {
    return (
      <div style={{
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'center',
        alignItems: 'center',
        height: '100vh',
        background: '#1e2a4a',
        color: 'white',
      }}>
        <div style={{
          width: 40,
          height: 40,
          border: '3px solid rgba(255,255,255,0.3)',
          borderTop: '3px solid white',
          borderRadius: '50%',
          animation: 'spin 1s linear infinite',
        }} />
        <p style={{ marginTop: 16, fontSize: 16 }}>正在完成 SSO 登录...</p>
        <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
      </div>
    );
  }

  if (error) {
    return (
      <div style={{
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'center',
        alignItems: 'center',
        height: '100vh',
        background: '#1e2a4a',
        color: 'white',
      }}>
        <div style={{
          background: 'white',
          borderRadius: 16,
          padding: '48px',
          maxWidth: 420,
          textAlign: 'center',
          boxShadow: '0 25px 50px -12px rgba(0,0,0,0.25)',
        }}>
          <div style={{ fontSize: 48, marginBottom: 16 }}>&#x26A0;</div>
          <h2 style={{ color: 'var(--ink)', margin: '0 0 12px 0' }}>SSO 登录失败</h2>
          <p style={{ color: 'var(--ink-2)', margin: '0 0 24px 0', fontSize: 14 }}>{error}</p>
          <button
            onClick={() => navigate('/login', { replace: true })}
            style={{
              padding: '12px 24px',
              background: 'var(--brand)',
              color: 'white',
              border: 'none',
              borderRadius: 8,
              fontSize: 14,
              fontWeight: 500,
              cursor: 'pointer',
            }}
          >
            返回登录页面
          </button>
        </div>
      </div>
    );
  }

  return null;
};

export default SSOCallback;
