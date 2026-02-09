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
    if (!formData.username) newErrors.username = 'Please enter username';
    if (!formData.password) newErrors.password = 'Please enter password';
    
    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }
    
    setErrors({});
    dispatch(loginStart());
    
    try {
      const response = await authService.login(formData);
      console.log(' Login response:', response);
      
      // 后端返回: {code: 200, data: {token, user}, message, timestamp}
      // axios拦截器返回response.data,所以response就是这个对象
      // token和user在response.data里
      const token = response.data.token;
      const user = response.data.user;
      
      console.log(' Token:', token?.substring(0, 30) + '...');
      console.log(' User:', user);
      
      if (!token || !user) {
        console.error('❌ Missing token or user in response!', response);
        throw new Error('Incomplete login response data');
      }
      
      dispatch(loginSuccess({
        user: user,
        token: token,
      }));
      
      // Redux会自动保存,但我们再次确认
      console.log(' Token saved to localStorage:', localStorage.getItem('token')?.substring(0, 30) + '...');
      
      localStorage.removeItem('loginUsername');
      navigate(from, { replace: true });
    } catch (error: any) {
      dispatch(loginFailure());
      const errorMessage = error.message || 'Login failed, please check username and password';
      
      if (errorMessage.includes('用户') || errorMessage.includes('User not found')) {
        setErrors({ username: 'Username not found' });
      } else if (errorMessage.includes('密码') || errorMessage.includes('password')) {
        setErrors({ password: 'Incorrect password' });
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
      <div className={styles.card}>
        <div className={styles.header}>
          <h1 className={styles.title}>Welcome to IaC Platform</h1>
          <p className={styles.subtitle}>Infrastructure Management Platform</p>
        </div>
        
        <form className={styles.form} onSubmit={handleSubmit}>
          <div className={styles.inputGroup}>
            <input
              type="text"
              placeholder="Username"
              className={styles.input}
              value={formData.username}
              onChange={(e) => handleInputChange('username', e.target.value)}
              style={{ borderColor: errors.username ? '#ef4444' : undefined }}
            />
            {errors.username && (
              <div style={{ color: '#ef4444', fontSize: '12px', marginTop: '4px' }}>
                {errors.username}
              </div>
            )}
          </div>
          
          <div className={styles.inputGroup}>
            <input
              type="password"
              placeholder="Password"
              className={styles.input}
              value={formData.password}
              onChange={(e) => handleInputChange('password', e.target.value)}
              style={{ borderColor: errors.password ? '#ef4444' : undefined }}
            />
            {errors.password && (
              <div style={{ color: '#ef4444', fontSize: '12px', marginTop: '4px' }}>
                {errors.password}
              </div>
            )}
          </div>
          
          <button
            type="submit"
            className={styles.button}
            disabled={loading}
          >
            {loading ? 'Logging in...' : 'Login'}
          </button>
        </form>
      </div>
    </div>
  );
};

export default Login;
