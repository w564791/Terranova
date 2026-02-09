import React, { useState, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { useDispatch, useSelector } from 'react-redux';
import type { RootState } from '../store';
import { loginStart, loginSuccess, loginFailure } from '../store/slices/authSlice';
import { authService, setupService } from '../services/auth';
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
      console.log(' Login response:', response);
      
      // 检查是否需要MFA验证
      if (response.data.mfa_required) {
        console.log('🔐 MFA required, redirecting to MFA verify page');
        console.log('🔐 Backend returned required_backup_codes:', response.data.required_backup_codes);
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
        console.log('🔐 MFA setup required, redirecting to MFA setup page');
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
      
      console.log(' Token:', token?.substring(0, 30) + '...');
      console.log(' User:', user);
      
      if (!token || !user) {
        console.error('Missing token or user in response!', response);
        throw new Error('Incomplete login response data');
      }
      
      dispatch(loginSuccess({
        user: user,
        token: token,
      }));
      
      console.log(' Token saved to localStorage:', localStorage.getItem('token')?.substring(0, 30) + '...');
      
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
      </div>
    </div>
  );
};

export default Login;