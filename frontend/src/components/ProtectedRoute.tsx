import { Navigate, useLocation } from 'react-router-dom';
import { useSelector } from 'react-redux';
import type { RootState } from '../store';

interface ProtectedRouteProps {
  children: React.ReactNode;
}

const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children }) => {
  const { token, user } = useSelector((state: RootState) => state.auth);
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
  
  // IAM权限系统：所有页面的访问控制由后端API的IAM权限中间件处理
  // 前端仅负责认证检查（token有效性），权限检查交给后端
  return <>{children}</>;
};

export default ProtectedRoute;
