import React, { useState, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { useDispatch, useSelector } from 'react-redux';
import type { RootState } from '../store';
import { loginStart, loginSuccess, loginFailure } from '../store/slices/authSlice';
import { authService, setupService } from '../services/auth';
import { ssoService, type SSOProvider } from '../services/ssoService';
import styles from './Login.module.css';

const Login: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const dispatch = useDispatch();
  const { loading, isAuthenticated } = useSelector((state: RootState) => state.auth);
  
  // Get target path from URL params before login
  const searchParams = new URLSearchParams(location.search);
  const returnUrl = searchParams.get('returnUrl');
  const from = returnUrl ? decodeURIComponent(returnUrl) : '/';
  
  const [ssoProviders, setSsoProviders] = useState<SSOProvider[]>([]);
  const [ssoLoading, setSsoLoading] = useState<string>(''); // 正在加载的 provider key
  const [disableLocalLogin, setDisableLocalLogin] = useState(false);

  // 加载 SSO Provider 列表和配置
  useEffect(() => {
    const loadSSOProviders = async () => {
      try {
        const response: any = await ssoService.getProviders();
        const data = response.data || response;
        if (data && data.providers) {
          // 新格式：{ providers: [...], disable_local_login: bool }
          setSsoProviders(data.providers || []);
          setDisableLocalLogin(data.disable_local_login || false);
        } else if (Array.isArray(data)) {
          // 兼容旧格式
          setSsoProviders(data);
        }
      } catch (error) {
        // SSO 不可用时静默失败
        console.debug('SSO providers not available:', error);
      }
    };
    loadSSOProviders();
  }, []);

  // SSO 登录处理
  const handleSSOLogin = async (providerKey: string) => {
    setSsoLoading(providerKey);
    try {
      // 保存 provider key 和返回 URL 到 localStorage（回调时使用）
      localStorage.setItem('sso_provider', providerKey);
      localStorage.setItem('sso_return_url', from);

      const response: any = await ssoService.login(providerKey, from);
      const data = response.data || response;
      
      if (data.auth_url) {
        // 重定向到 Provider 授权页面
        window.location.href = data.auth_url;
      } else {
        console.error('No auth_url in response');
        setSsoLoading('');
      }
    } catch (error: any) {
      console.error('SSO login error:', error);
      setSsoLoading('');
      setErrors({ password: error.message || 'SSO 登录失败' });
    }
  };

  // 获取 SSO Provider 图标
  const getSSOIcon = (icon: string, providerType: string) => {
    const iconMap: Record<string, string> = {
      github: 'M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z',
      google: 'M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 01-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z',
      microsoft: 'M1 1h10v10H1V1zm12 0h10v10H13V1zM1 13h10v10H1V13zm12 0h10v10H13V13z',
      auth0: 'M12 2L2 7v10l10 5 10-5V7L12 2z',
    };
    const key = icon || providerType;
    return iconMap[key] || iconMap['auth0'];
  };

  // Check if system is initialized, redirect to setup if not
  useEffect(() => {
    const checkSetup = async () => {
      try {
        const response: any = await setupService.getStatus();
        const statusData = response.data || response;
        if (!statusData.initialized) {
          navigate('/setup', { replace: true });
          return;
        }
      } catch (error) {
        // If setup check fails, continue to login page
        console.error('Failed to check setup status:', error);
      }
    };
    if (!isAuthenticated) {
      checkSetup();
    }
  }, [navigate, isAuthenticated]);
  
  // If already logged in, auto redirect to target page
  useEffect(() => {
    if (isAuthenticated) {
      navigate(from, { replace: true });
    }
  }, [isAuthenticated, navigate, from]);
  
  // Restore user input from localStorage (username only, not password)
  const [formData, setFormData] = useState(() => {
    const savedUsername = localStorage.getItem('loginUsername') || '';
    return { username: savedUsername, password: '' };
  });
  const [errors, setErrors] = useState<{ username?: string; password?: string }>({});

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    // Simple validation
    const newErrors: { username?: string; password?: string } = {};
    if (!formData.username) newErrors.username = '请输入用户名';
    if (!formData.password) newErrors.password = '请输入密码';
    
    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }
    
    setErrors({});
    dispatch(loginStart());
    
    try {
      const response = await authService.login(formData);
      console.log('Login response:', response);
      
      // 检查是否需要MFA验证
      if (response.data.mfa_required) {
        console.log('MFA required, redirecting to MFA verify page');
        console.log('Backend returned required_backup_codes:', response.data.required_backup_codes);
        navigate('/login/mfa', {
          state: {
            mfa_token: response.data.mfa_token,
            username: response.data.user?.username || formData.username,
            required_backup_codes: response.data.required_backup_codes !== undefined ? response.data.required_backup_codes : 1,
          },
          replace: true,
        });
        return;
      }
      
      // 检查是否需要设置MFA
      if (response.data.mfa_setup_required) {
        console.log('MFA setup required, redirecting to MFA setup page');
        navigate('/setup/mfa', {
          state: {
            mfa_token: response.data.mfa_token,
            username: response.data.user?.username || formData.username,
            from_login: true,
          },
          replace: true,
        });
        return;
      }
      
      const token = response.data.token;
      const user = response.data.user;
      
      console.log('Token:', token?.substring(0, 30) + '...');
      console.log('User:', user);
      
      if (!token || !user) {
        console.error('Missing token or user in response!', response);
        throw new Error('Incomplete login response data');
      }
      
      dispatch(loginSuccess({
        user: user,
        token: token,
      }));
      
      console.log('Token saved to localStorage:', localStorage.getItem('token')?.substring(0, 30) + '...');
      
      localStorage.removeItem('loginUsername');
      navigate(from, { replace: true });
    } catch (error: any) {
      dispatch(loginFailure());
      const errorMessage = error.message || 'Login failed, please check username and password';
      
      if (errorMessage.includes('用户') || errorMessage.includes('User not found')) {
        setErrors({ username: '用户名不存在' });
      } else if (errorMessage.includes('密码') || errorMessage.includes('password')) {
        setErrors({ password: '密码错误' });
      } else {
        setErrors({ password: errorMessage });
      }
    }
  };

  const handleInputChange = (field: string, value: string) => {
    setFormData(prev => ({ ...prev, [field]: value }));
    
    // Save username to localStorage (not password)
    if (field === 'username') {
      localStorage.setItem('loginUsername', value);
    }
    
    // Clear error but keep user input
    if (errors[field as keyof typeof errors]) {
      setErrors(prev => ({ ...prev, [field]: undefined }));
    }
  };

  return (
    <div className={styles.container}>
      {/* 左侧品牌区域 */}
      <div className={styles.brandSection}>
        <h1 className={styles.brandTitle}>
          强大。安全。卓越。
          <br />
          <span className={styles.brandHighlight}>IaC Platform</span>
          <br />
          基础设施即代码管理平台。
        </h1>
        <p className={styles.brandSubtitle}>
          统一管理您的云基础设施，实现自动化部署、版本控制和团队协作。
          让基础设施管理变得简单、可靠、高效。
        </p>
      </div>

      {/* 右侧表单区域 */}
      <div className={styles.formSection}>
        <div className={styles.formHeader}>
          <h2 className={styles.formTitle}>登录账号</h2>
        </div>

        {/* SSO 登录按钮 */}
        {ssoProviders.length > 0 && (
          <div className={styles.ssoSection}>
            {ssoProviders.map((provider) => (
              <button
                key={provider.provider_key}
                type="button"
                className={styles.ssoButton}
                onClick={() => handleSSOLogin(provider.provider_key)}
                disabled={!!ssoLoading}
              >
                <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                  <path d={getSSOIcon(provider.icon, provider.provider_type)} />
                </svg>
                <span>
                  {ssoLoading === provider.provider_key
                    ? '跳转中...'
                    : provider.display_name}
                </span>
              </button>
            ))}
            {!disableLocalLogin && (
              <div className={styles.divider}>
                <span className={styles.dividerText}>或使用账号密码登录</span>
              </div>
            )}
          </div>
        )}

        {/* 本地密码登录表单（禁用本地登录时隐藏，但 SSO 不可用时仍显示） */}
        {(!disableLocalLogin || ssoProviders.length === 0) && (
          <form className={styles.form} onSubmit={handleSubmit}>
            <div className={styles.inputGroup}>
              <label className={styles.label}>用户名</label>
              <input
                type="text"
                placeholder="请输入用户名"
                className={`${styles.input} ${errors.username ? styles.inputError : ''}`}
                value={formData.username}
                onChange={(e) => handleInputChange('username', e.target.value)}
              />
              {errors.username && (
                <span className={styles.errorText}>{errors.username}</span>
              )}
            </div>
            
            <div className={styles.inputGroup}>
              <label className={styles.label}>密码</label>
              <input
                type="password"
                placeholder="请输入密码"
                className={`${styles.input} ${errors.password ? styles.inputError : ''}`}
                value={formData.password}
                onChange={(e) => handleInputChange('password', e.target.value)}
              />
              {errors.password && (
                <span className={styles.errorText}>{errors.password}</span>
              )}
            </div>
            
            <button
              type="submit"
              className={styles.button}
              disabled={loading}
            >
              {loading ? '登录中...' : '登录'}
            </button>
          </form>
        )}

        {/* 禁用本地登录时的提示 */}
        {disableLocalLogin && ssoProviders.length > 0 && (
          <p className={styles.disableLocalLoginHint}>
            本地密码登录已禁用，请使用 SSO 登录
          </p>
        )}
      </div>
    </div>
  );
};

export default Login;