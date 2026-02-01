import { useEffect } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import type { RootState } from '../store';
import { loginSuccess, logout } from '../store/slices/authSlice';
import api from '../services/api';

interface AuthProviderProps {
  children: React.ReactNode;
}

const AuthProvider: React.FC<AuthProviderProps> = ({ children }) => {
  const dispatch = useDispatch();
  const { token, isAuthenticated } = useSelector((state: RootState) => state.auth);

  useEffect(() => {
    const verifyToken = async () => {
      if (token) {
        try {
          // 每次都从后端获取最新的用户信息（不缓存权限）
          const response = await api.get('/auth/me');
          dispatch(loginSuccess({
            user: response.data,
            token: token
          }));
        } catch (error) {
          // Token无效，清除登录状态
          dispatch(logout());
        }
      }
    };

    // 每次组件挂载时都重新获取用户信息
    verifyToken();
    
    // 已取消定时刷新：用户信息只在组件挂载时获取一次
    // const interval = setInterval(verifyToken, 30000);
    // return () => clearInterval(interval);
  }, [token, dispatch]);

  // 如果有token但还未验证完成，显示加载状态
  if (token && !isAuthenticated) {
    return (
      <div style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        height: '100vh',
        fontSize: '16px',
        color: '#666'
      }}>
        验证登录状态...
      </div>
    );
  }

  return <>{children}</>;
};

export default AuthProvider;
