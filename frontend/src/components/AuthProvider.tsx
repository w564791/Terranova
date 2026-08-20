import { useEffect, useState } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import type { RootState } from '../store';
import { loginSuccess, logout } from '../store/slices/authSlice';
import api from '../services/api';
import { iamService } from '../services/iam';

interface AuthProviderProps {
  children: React.ReactNode;
}

const AuthProvider: React.FC<AuthProviderProps> = ({ children }) => {
  const dispatch = useDispatch();
  const [checkingSetup, setCheckingSetup] = useState(true);
  const [organizationBootstrappedFor, setOrganizationBootstrappedFor] = useState<string | null>(null);

  // 首次加载时检查系统初始化状态
  useEffect(() => {
    const checkSetupStatus = async () => {
      try {
        const response: any = await api.get('/setup/status');
        const statusData = response.data || response;
        if (!statusData.initialized) {
          // 系统未初始化，跳转到 setup 页面
          if (!window.location.pathname.includes('/setup')) {
            window.location.href = '/setup';
            return;
          }
        }
      } catch (error) {
        // 检查失败，继续正常流程
        console.error('Failed to check setup status:', error);
      } finally {
        setCheckingSetup(false);
      }
    };
    checkSetupStatus();
  }, []);

  const { token, isAuthenticated, user } = useSelector((state: RootState) => state.auth);

  // IAM 的所有组织级请求都依赖 active org。它必须在进入业务页面前经由
  // 无 org_id 的 bootstrap 端点建立，而不是让每个页面各自选“第一个组织”。
  useEffect(() => {
    let cancelled = false;

    const bootstrapOrganization = async () => {
      if (!token || !user) {
        if (!cancelled) setOrganizationBootstrappedFor(null);
        return;
      }

      try {
        await iamService.bootstrapActiveOrganization();
      } catch (error) {
        // 没有组织成员关系时保留空上下文，由具体页面展示其权限/空状态；
        // 不把 bootstrap 的网络错误误判为登录失效。
        console.error('Failed to bootstrap active organization:', error);
      } finally {
        if (!cancelled) setOrganizationBootstrappedFor(String(user.id));
      }
    };

    void bootstrapOrganization();
    return () => {
      cancelled = true;
    };
  }, [token, user?.id]);

  useEffect(() => {
    const verifyToken = async () => {
      if (token && !user) {
        // 只有当有token但没有用户信息时才验证（页面刷新的情况）
        // 如果已经有用户信息（通过loginSuccess设置），则不需要再次验证
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
    if (!checkingSetup) {
      verifyToken();
    }
    
    // 已取消定时刷新：用户信息只在组件挂载时获取一次
    // const interval = setInterval(verifyToken, 30000);
    // return () => clearInterval(interval);
  }, [token, user, dispatch, checkingSetup]);

  // 正在检查系统初始化状态
  if (checkingSetup) {
    return (
      <div style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        height: '100vh',
        fontSize: '16px',
        color: 'var(--ink-2)'
      }}>
        检查系统状态...
      </div>
    );
  }

  // 如果有token但还没有用户信息，显示加载状态
  const needsOrganizationBootstrap = Boolean(
    token && user && organizationBootstrappedFor !== String(user.id),
  );

  if ((token && !user) || needsOrganizationBootstrap) {
    return (
      <div style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        height: '100vh',
        fontSize: '16px',
        color: 'var(--ink-2)'
      }}>
        验证登录状态...
      </div>
    );
  }

  return <>{children}</>;
};

export default AuthProvider;
