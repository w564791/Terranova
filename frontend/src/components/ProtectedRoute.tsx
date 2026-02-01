import { Navigate, useLocation } from 'react-router-dom';
import { useSelector } from 'react-redux';
import type { RootState } from '../store';
import NoPermission from '../pages/NoPermission';

interface ProtectedRouteProps {
  children: React.ReactNode;
}

const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children }) => {
  const { token, user, isAuthenticated } = useSelector((state: RootState) => state.auth);
  const location = useLocation();
  
  if (!token) {
    // 保存当前完整路径（包括search和hash）到URL参数
    const returnUrl = encodeURIComponent(location.pathname + location.search + location.hash);
    return <Navigate to={`/login?returnUrl=${returnUrl}`} replace />;
  }
  
  // 如果token存在但user还未加载，显示加载状态
  if (!user) {
    return (
      <div style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        height: '100vh',
        fontSize: '16px',
        color: '#666'
      }}>
        加载用户信息...
      </div>
    );
  }
  
  // IAM权限系统：
  // - 首页（/）：由后端dashboard API的IAM权限控制（需要ORGANIZATION READ）
  // - 管理页面（/admin/*）：仍然需要admin角色
  const isAdminPage = location.pathname.startsWith('/admin');
  
  if (isAdminPage && user.role !== 'admin') {
    return <NoPermission />;
  }
  
  // 非管理页面允许所有已认证用户访问，权限由后端API控制
  return <>{children}</>;
};

export default ProtectedRoute;
