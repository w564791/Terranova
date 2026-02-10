import React, { useEffect, useState, useRef } from 'react';
import { useNavigate, useLocation, useSearchParams } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { loginSuccess } from '../store/slices/authSlice';
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
      // 检查是否有直接传递的 token（重定向模式）
      const directToken = searchParams.get('token');
      if (directToken) {
        // 重定向模式：token 直接在 URL 参数中
        localStorage.setItem('token', directToken);
        dispatch(loginSuccess({
          user: { id: 0, username: '', email: '', role: '' },
          token: directToken,
        }));
        navigate('/', { replace: true });
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

        // 检查是否需要设置 MFA（带 token 的情况：先登录再跳转）
        if (data.mfa_setup_required && data.token) {
          localStorage.setItem('token', data.token);
          localStorage.removeItem('sso_provider');
          dispatch(loginSuccess({
            user: data.user,
            token: data.token,
          }));
          navigate('/settings/mfa', {
            state: {
              from_login: true,
              force_setup: true,
            },
            replace: true,
          });
          return;
        }

        // 检查是否需要设置 MFA（不带 token 的情况：用 mfa_token）
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

        if (data.token && data.user) {
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
          setError('登录响应数据不完整');
        }
      } catch (err: any) {
        console.error('SSO callback error:', err);
        setError(err.message || 'SSO 登录失败，请重试');
      } finally {
        setLoading(false);
      }
    };

    handleCallback();
  }, [searchParams, dispatch, navigate]);

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
          <h2 style={{ color: '#1e293b', margin: '0 0 12px 0' }}>SSO 登录失败</h2>
          <p style={{ color: '#64748b', margin: '0 0 24px 0', fontSize: 14 }}>{error}</p>
          <button
            onClick={() => navigate('/login', { replace: true })}
            style={{
              padding: '12px 24px',
              background: '#3b82f6',
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